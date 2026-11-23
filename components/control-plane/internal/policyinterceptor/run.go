package policyinterceptor

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"k8s.io/client-go/kubernetes"

	pev1 "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/policyevents/v1"
	gi "github.com/openshift-online/hypershell/components/control-plane/internal/openshellclient/gen/gatewayinterceptorv1"
)

// socketMode lets the gateway container connect whatever UID it runs as. The
// socket lives on a pod-local emptyDir and every call is authenticated with the
// gateway's extension JWT (PLAT-PGP-09), so file permissions are not the
// security boundary.
const socketMode fs.FileMode = 0o666

// ListenUnix removes a stale socket file left by a previous container run and
// listens on path.
func ListenUnix(path string) (net.Listener, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create socket directory: %w", err)
	}
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&fs.ModeSocket == 0 {
			return nil, fmt.Errorf("refusing to remove %s: not a socket", path)
		}
		if err := os.Remove(path); err != nil {
			return nil, fmt.Errorf("remove stale socket: %w", err)
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("stat socket: %w", err)
	}
	lis, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", path, err)
	}
	if err := os.Chmod(path, socketMode); err != nil {
		_ = lis.Close()
		return nil, fmt.Errorf("chmod socket: %w", err)
	}
	return lis, nil
}

// Deps are the external dependencies Run wires; tests substitute fakes.
type Deps struct {
	Kube        kubernetes.Interface
	EventClient PolicyEventClient
	Lookup      SandboxLookup
	Log         *slog.Logger
}

// Run starts the interceptor gRPC server, the bundle watcher, the event
// reporter, and the health server, and blocks until ctx is done. It returns
// the first fatal error.
func Run(ctx context.Context, cfg Config, deps Deps) error {
	log := deps.Log
	if log == nil {
		log = slog.Default()
	}
	verifier, err := NewVerifier(cfg.JWTPublicKeyFile, cfg.JWTIssuer, cfg.JWTAudience)
	if err != nil {
		return err
	}
	metrics := NewMetrics()
	store := NewStore(cfg.GatewayID)
	buffer := NewEventBuffer(cfg.EventBuffer, metrics.DroppedEvents.Inc)
	health := NewHealth(metrics)

	srv := &Server{
		GatewayID:   cfg.GatewayID,
		Audience:    cfg.JWTAudience,
		Defaults:    cfg.Defaults,
		PoliciesURL: cfg.PoliciesURL,
		Store:       store,
		Events:      buffer,
		Metrics:     metrics,
		Lookup:      deps.Lookup,
		Log:         log,
	}
	grpcServer := grpc.NewServer(grpc.ChainUnaryInterceptor(verifier.UnaryServerInterceptor(func(method string, err error) {
		metrics.AuthRejections.Inc()
		log.WarnContext(ctx, "rejected unauthenticated interceptor call", "method", method, "reason", err)
	})))
	gi.RegisterGatewayInterceptorServer(grpcServer, srv)

	lis, err := ListenUnix(cfg.SocketPath)
	if err != nil {
		return err
	}

	eventClient := deps.EventClient
	if eventClient == nil {
		conn, err := grpc.NewClient(cfg.EventsAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			_ = lis.Close()
			return fmt.Errorf("dial policy event service %s: %w", cfg.EventsAddr, err)
		}
		defer func() { _ = conn.Close() }()
		eventClient = pev1.NewPolicyEventServiceClient(conn)
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	errCh := make(chan error, 3)
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := grpcServer.Serve(lis); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			errCh <- fmt.Errorf("interceptor gRPC server: %w", err)
		}
	}()
	health.SetReady(true)
	log.InfoContext(ctx, "policy interceptor listening", "socket", cfg.SocketPath, "gateway_id", cfg.GatewayID)

	httpServer := health.NewHTTPServer(cfg.HealthAddr)
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("health server: %w", err)
		}
	}()

	reporter := &Reporter{
		Client:    eventClient,
		Buffer:    buffer,
		GatewayID: cfg.GatewayID,
		TokenFile: cfg.EventsTokenFile,
		Revision:  store.AppliedRevision,
		Metrics:   metrics,
		Log:       log,
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		reporter.Run(ctx)
	}()

	if deps.Kube != nil {
		watcher := &BundleWatcher{
			Client:    deps.Kube,
			Namespace: cfg.Namespace,
			Store:     store,
			Metrics:   metrics,
			Log:       log,
			OnApplied: func(string) { buffer.Kick() },
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := watcher.Run(ctx); err != nil {
				errCh <- fmt.Errorf("policy bundle watcher: %w", err)
			}
		}()
	}

	var runErr error
	select {
	case <-ctx.Done():
	case runErr = <-errCh:
	}
	health.SetReady(false)
	cancel()

	stopped := make(chan struct{})
	go func() {
		grpcServer.GracefulStop()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		grpcServer.Stop()
	}
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.WarnContext(shutdownCtx, "health server shutdown", "error", err)
	}
	wg.Wait()
	return runErr
}
