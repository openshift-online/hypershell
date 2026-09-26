package main

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/golang/glog"
	"github.com/openshift-online/rh-trex-ai/pkg/environments"
	pkgserver "github.com/openshift-online/rh-trex-ai/pkg/server"
	"github.com/spf13/cobra"

	"github.com/openshift-online/hypershell/components/api-server/pkg/grpctls"
)

// newServeCommand mirrors the framework's serve command
// (rh-trex-ai/pkg/cmd/serve.go) with one difference: when --grpc-enable-tls is
// set, the gRPC listener is wrapped in TLS by pkg/grpctls before the framework
// server starts serving on it. The framework registers that flag but never
// acts on it unless its shared --enable-tls is also on, and that flag would
// turn the REST listener into HTTPS as well (specs/platform/hub-grpc-tls.spec.md).
func newServeCommand(getSpecData func() ([]byte, error)) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Serve the application",
		Long:  "Serve the application.",
		Run: func(*cobra.Command, []string) {
			runServe(getSpecData)
		},
	}
	if err := environments.Environment().AddFlags(cmd.PersistentFlags()); err != nil {
		glog.Fatalf("Unable to add environment flags to serve command: %s", err.Error())
	}
	return cmd
}

func runServe(getSpecData func() ([]byte, error)) {
	env := environments.Environment()
	if err := env.Initialize(); err != nil {
		glog.Fatalf("Unable to initialize environment: %s", err.Error())
	}

	specData, err := getSpecData()
	if err != nil {
		glog.Fatalf("Unable to load OpenAPI spec: %s", err.Error())
	}

	var servers []pkgserver.Server

	controllersServer := pkgserver.NewDefaultControllersServer(env)
	go controllersServer.Start()

	apiServer := pkgserver.NewDefaultAPIServer(env, specData)
	servers = append(servers, apiServer)
	go apiServer.Start()

	if env.Config.GRPC.EnableGRPC {
		grpcServer := pkgserver.NewDefaultGRPCServer(env)
		servers = append(servers, grpcServer)
		listener, listenErr := grpcServer.Listen()
		if listenErr != nil {
			glog.Fatalf("Unable to start gRPC server: %v", listenErr)
		}
		if env.Config.GRPC.EnableTLS {
			listener, listenErr = grpctls.Wrap(listener, grpctls.Config{
				CertFile: env.Config.GRPC.TLSCertFile,
				KeyFile:  env.Config.GRPC.TLSKeyFile,
			})
			if listenErr != nil {
				glog.Fatalf("Unable to enable gRPC TLS: %v", listenErr)
			}
			glog.Infof("gRPC server listening with TLS at %s (certificate %s)", env.Config.GRPC.BindAddress, env.Config.GRPC.TLSCertFile)
		} else {
			glog.Infof("gRPC server listening at %s (plaintext)", env.Config.GRPC.BindAddress)
		}
		go grpcServer.Serve(listener)
	}

	metricsServer := pkgserver.NewDefaultMetricsServer(env)
	servers = append(servers, metricsServer)
	go metricsServer.Start()

	healthCheckServer := pkgserver.NewDefaultHealthCheckServer(env)
	servers = append(servers, healthCheckServer)
	go healthCheckServer.Start()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	sig := <-sigCh
	glog.Infof("Received signal %v, shutting down", sig)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	controllersServer.Stop()
	glog.Info("Controllers server stopped")

	var wg sync.WaitGroup
	for _, s := range servers {
		wg.Add(1)
		go func(srv pkgserver.Server) {
			defer wg.Done()
			if err := srv.Stop(); err != nil {
				glog.Errorf("Error stopping server: %v", err)
			}
		}(s)
	}

	doneCh := make(chan struct{})
	go func() {
		wg.Wait()
		close(doneCh)
	}()

	select {
	case <-doneCh:
		glog.Info("All servers stopped gracefully")
	case <-shutdownCtx.Done():
		glog.Warning("Shutdown timed out, forcing exit")
	}

	_ = env.Database.SessionFactory.Close()
	glog.Info("Database connections closed")
}
