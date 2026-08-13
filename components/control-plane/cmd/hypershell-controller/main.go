package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/control-plane/internal/auth"
	"github.com/openshift-online/hypershell/components/control-plane/internal/config"
	"github.com/openshift-online/hypershell/components/control-plane/internal/reconciler"
	"github.com/openshift-online/hypershell/components/control-plane/internal/watcher"
)

const defaultManifestsDir = "/manifests/gateway"

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("loading config: %v", err)
	}

	log.Printf("INFO hypershell-controller starting")
	log.Printf("INFO grpc=%s api=%s namespace=%s", cfg.GRPCServerAddr, cfg.APIServerURL, cfg.Namespace)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	dialOpts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	}

	oidcIssuer := os.Getenv("OIDC_ISSUER")
	if oidcIssuer != "" {
		oidcClientID := os.Getenv("OIDC_CLIENT_ID")
		if oidcClientID == "" {
			oidcClientID = "hypershell-control-plane"
		}
		oidcClientSecret := os.Getenv("OIDC_CLIENT_SECRET")
		if oidcClientSecret == "" {
			log.Fatalf("OIDC_CLIENT_SECRET is required when OIDC_ISSUER is set")
		}

		tp := auth.NewTokenProvider(oidcIssuer, oidcClientID, oidcClientSecret)
		if endpoint := os.Getenv("OIDC_TOKEN_ENDPOINT"); endpoint != "" {
			tp.SetTokenEndpoint(endpoint)
			log.Printf("INFO using explicit OIDC token endpoint: %s", endpoint)
		}
		dialOpts = append(dialOpts, grpc.WithPerRPCCredentials(auth.NewGRPCCredentials(tp)))
		log.Printf("INFO OIDC authentication enabled for gRPC connections")
	} else {
		log.Printf("INFO OIDC authentication disabled for gRPC connections")
	}

	conn, err := grpc.NewClient(cfg.GRPCServerAddr, dialOpts...)
	if err != nil {
		log.Fatalf("connecting to gRPC server: %v", err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			log.Printf("ERROR closing gRPC connection: %v", err)
		}
	}()

	k8sConfig, err := rest.InClusterConfig()
	if err != nil {
		log.Printf("WARN not running in-cluster, gateway reconciliation will be limited: %v", err)
	}

	var clientset *kubernetes.Clientset
	var dynamicClient dynamic.Interface

	if k8sConfig != nil {
		clientset, err = kubernetes.NewForConfig(k8sConfig)
		if err != nil {
			log.Fatalf("creating kubernetes clientset: %v", err)
		}

		dynamicClient, err = dynamic.NewForConfig(k8sConfig)
		if err != nil {
			log.Fatalf("creating dynamic client: %v", err)
		}
	}

	fleetReconciler := reconciler.NewFleetReconciler()
	clusterReconciler := reconciler.NewManagedClusterReconciler()
	databaseReconciler := reconciler.NewManagedDatabaseReconciler()
	releaseReconciler := reconciler.NewGatewayReleaseReconciler()
	networkReconciler := reconciler.NewGatewayNetworkReconciler()

	manifestsDir := os.Getenv("GATEWAY_MANIFESTS_DIR")
	if manifestsDir == "" {
		manifestsDir = defaultManifestsDir
	}

	var gatewayReconciler watcher.Handler[*pb.Gateway]

	if clientset != nil && dynamicClient != nil {
		gr, grErr := reconciler.NewGatewayReconciler(dynamicClient, clientset, conn, manifestsDir, cfg.Namespace)
		if grErr != nil {
			log.Printf("WARN gateway reconciler disabled: %v", grErr)
			gatewayReconciler = reconciler.NewStubGatewayReconciler()
		} else {
			gatewayReconciler = gr
		}
	} else {
		log.Printf("WARN no kubernetes client available, using stub gateway reconciler")
		gatewayReconciler = reconciler.NewStubGatewayReconciler()
	}

	errCh := make(chan error, 7)

	go func() { errCh <- watcher.WatchFleets(ctx, conn, fleetReconciler) }()
	go func() { errCh <- watcher.WatchManagedClusters(ctx, conn, clusterReconciler) }()
	go func() { errCh <- watcher.WatchManagedDatabases(ctx, conn, databaseReconciler) }()
	go func() { errCh <- watcher.WatchGatewayReleases(ctx, conn, releaseReconciler) }()
	go func() { errCh <- watcher.WatchGateways(ctx, conn, gatewayReconciler) }()
	go func() { errCh <- watcher.WatchGatewayNetworks(ctx, conn, networkReconciler) }()

	log.Printf("INFO all 6 watch streams launched")

	// The continuous gateway health reconciler keeps each Gateway's phase and
	// status synchronized with observed workload health (Running <-> Degraded).
	// It requires an in-cluster Kubernetes client to observe Deployments.
	if clientset != nil {
		healthReconciler := reconciler.NewGatewayHealthReconciler(clientset, conn)
		go func() { errCh <- healthReconciler.Run(ctx) }()
		log.Printf("INFO gateway health reconciler launched")
	} else {
		log.Printf("WARN no kubernetes client available, gateway health reconciliation disabled")
	}

	watchErr := <-errCh
	cancel()

	if watchErr != nil && watchErr != context.Canceled {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", watchErr)
		os.Exit(1)
	}

	log.Printf("INFO hypershell-controller stopped")
}
