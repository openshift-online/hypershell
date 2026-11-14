// Command sandbox-resource-interceptor runs the HyperShell sandbox resource
// interceptor: a stateless gRPC sidecar serving the OpenShell
// openshell.gateway_interceptor.v1.GatewayInterceptor contract on a
// gateway-local Unix socket. It is not deployed yet; the deployment wiring
// lands with the S4 slice.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"google.golang.org/grpc"

	"github.com/openshift-online/hypershell/components/sandbox-resource-interceptor/internal/auth"
	"github.com/openshift-online/hypershell/components/sandbox-resource-interceptor/internal/config"
	"github.com/openshift-online/hypershell/components/sandbox-resource-interceptor/internal/metrics"
	"github.com/openshift-online/hypershell/components/sandbox-resource-interceptor/internal/policy"
	"github.com/openshift-online/hypershell/components/sandbox-resource-interceptor/internal/server"
	pb "github.com/openshift-online/hypershell/components/sandbox-resource-interceptor/pkg/api/grpc"
)

// version is stamped at build time (-ldflags "-X main.version=...").
var version = "dev"

// clockLeeway absorbs clock skew between the gateway and this sidecar; they
// share a pod, so it only needs to cover scheduling jitter.
const clockLeeway = 30 * time.Second

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "sandbox-resource-interceptor: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	pol, err := policy.New(cfg.Mode)
	if err != nil {
		return err
	}
	publicKey, err := auth.LoadPublicKey(cfg.PublicKeyPath)
	if err != nil {
		return err
	}
	verifier, err := auth.NewVerifier(server.ExpectedAudience, publicKey, clockLeeway)
	if err != nil {
		return err
	}

	registry := metrics.New(cfg.Mode)
	metricsServer := serveMetrics(cfg.MetricsAddr, registry.Handler())
	defer func() {
		_ = metricsServer.Close()
	}()

	listener, err := listen(cfg.SocketPath)
	if err != nil {
		return err
	}
	defer func() {
		_ = listener.Close()
	}()

	grpcServer := grpc.NewServer(grpc.ChainUnaryInterceptor(auth.UnaryInterceptor(verifier)))
	pb.RegisterGatewayInterceptorServer(grpcServer, server.New(version, pol, registry))

	// The gateway connects to the socket at startup, so serve until the pod
	// is stopped; a crashed sidecar blocks only that gateway's creates until
	// the pod restarts (fail_closed).
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- grpcServer.Serve(listener)
	}()
	logger.Info("listening", "socket", cfg.SocketPath, "audience", server.ExpectedAudience, "mode", string(cfg.Mode), "metrics_addr", cfg.MetricsAddr, "version", version)

	select {
	case err := <-serveErr:
		return fmt.Errorf("gRPC server stopped: %w", err)
	case <-ctx.Done():
		grpcServer.GracefulStop()
		return nil
	}
}

// serveMetrics starts the Prometheus metrics and readiness HTTP listener.
// The metrics port is scraped through the gateway pod's Service; /readyz is
// the container readiness probe target.
func serveMetrics(addr string, handler http.Handler) *http.Server {
	mux := http.NewServeMux()
	mux.Handle("/metrics", handler)
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok\n"))
	})
	server := &http.Server{Addr: addr, Handler: mux}
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("metrics server stopped", "addr", addr, "error", err)
		}
	}()
	return server
}

// listen creates the Unix socket, removing any stale socket file left by a
// previous container instance (the emptyDir volume starts empty in fresh
// pods, but a restarted container reuses it).
func listen(socketPath string) (net.Listener, error) {
	if err := os.MkdirAll(filepath.Dir(socketPath), 0o755); err != nil {
		return nil, fmt.Errorf("creating socket directory: %w", err)
	}
	if _, err := os.Stat(socketPath); err == nil {
		if err := os.Remove(socketPath); err != nil {
			return nil, fmt.Errorf("removing stale socket %s: %w", socketPath, err)
		}
	}
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("listening on %s: %w", socketPath, err)
	}
	return listener, nil
}
