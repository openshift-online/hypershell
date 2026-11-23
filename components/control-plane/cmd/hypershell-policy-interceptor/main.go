// Command hypershell-policy-interceptor is the HyperShell policy interceptor
// sidecar (PLAT-PGP-09, OSG-CORE-19). It runs as a native sidecar in each
// gateway pod, serves the OpenShell GatewayInterceptor contract on a pod-local
// unix socket, watches the hypershell-policy-bundle ConfigMap, and reports
// audit findings to the control plane. It ships in the control-plane image next
// to hypershell-controller.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/openshift-online/hypershell/components/control-plane/internal/logging"
	"github.com/openshift-online/hypershell/components/control-plane/internal/policyinterceptor"
	"github.com/openshift-online/hypershell/components/control-plane/pkg/version"
)

func main() {
	if err := run(); err != nil {
		slog.Error("policy interceptor exited", "error", err)
		os.Exit(1)
	}
}

func run() error {
	level, levelErr := logging.ParseLevel(os.Getenv("HYPERSHELL_LOG_LEVEL"))
	format, formatErr := logging.ParseFormat(os.Getenv("HYPERSHELL_LOG_FORMAT"))
	logging.Install(os.Stderr, level, format)
	if levelErr != nil {
		slog.Warn("invalid HYPERSHELL_LOG_LEVEL; using default", "error", levelErr)
	}
	if formatErr != nil {
		slog.Warn("invalid HYPERSHELL_LOG_FORMAT; using default", "error", formatErr)
	}

	cfg, err := policyinterceptor.LoadConfig(os.LookupEnv)
	if err != nil {
		return err
	}
	slog.Info("starting policy interceptor", "version", version.Version, "gateway_id", cfg.GatewayID,
		"namespace", cfg.Namespace, "bindings", len(policyinterceptor.Bindings()))

	restCfg, err := rest.InClusterConfig()
	if err != nil {
		return fmt.Errorf("load in-cluster Kubernetes config: %w", err)
	}
	kube, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return fmt.Errorf("create Kubernetes client: %w", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	return policyinterceptor.Run(ctx, cfg, policyinterceptor.Deps{
		Kube:   kube,
		Lookup: policyinterceptor.NoSandboxLookup{},
		Log:    slog.Default(),
	})
}
