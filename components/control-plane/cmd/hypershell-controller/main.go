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

	"github.com/openshift-online/hypershell/components/control-plane/internal/config"
	"github.com/openshift-online/hypershell/components/control-plane/internal/reconciler"
	"github.com/openshift-online/hypershell/components/control-plane/internal/watcher"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("loading config: %v", err)
	}

	log.Printf("INFO hypershell-controller starting")
	log.Printf("INFO grpc=%s api=%s namespace=%s", cfg.GRPCServerAddr, cfg.APIServerURL, cfg.Namespace)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	conn, err := grpc.NewClient(cfg.GRPCServerAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		log.Fatalf("connecting to gRPC server: %v", err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			log.Printf("ERROR closing gRPC connection: %v", err)
		}
	}()

	fleetReconciler := reconciler.NewFleetReconciler()
	clusterReconciler := reconciler.NewManagedClusterReconciler()
	databaseReconciler := reconciler.NewManagedDatabaseReconciler()
	releaseReconciler := reconciler.NewGatewayReleaseReconciler()
	gatewayReconciler := reconciler.NewGatewayReconciler()
	networkReconciler := reconciler.NewGatewayNetworkReconciler()

	errCh := make(chan error, 6)

	go func() { errCh <- watcher.WatchFleets(ctx, conn, fleetReconciler) }()
	go func() { errCh <- watcher.WatchManagedClusters(ctx, conn, clusterReconciler) }()
	go func() { errCh <- watcher.WatchManagedDatabases(ctx, conn, databaseReconciler) }()
	go func() { errCh <- watcher.WatchGatewayReleases(ctx, conn, releaseReconciler) }()
	go func() { errCh <- watcher.WatchGateways(ctx, conn, gatewayReconciler) }()
	go func() { errCh <- watcher.WatchGatewayNetworks(ctx, conn, networkReconciler) }()

	log.Printf("INFO all 6 watch streams launched")

	watchErr := <-errCh
	cancel()

	if watchErr != nil && watchErr != context.Canceled {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", watchErr)
		os.Exit(1)
	}

	log.Printf("INFO hypershell-controller stopped")
}
