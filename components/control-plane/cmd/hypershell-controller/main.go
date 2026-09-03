package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/control-plane/internal/auth"
	"github.com/openshift-online/hypershell/components/control-plane/internal/config"
	"github.com/openshift-online/hypershell/components/control-plane/internal/exposure"
	"github.com/openshift-online/hypershell/components/control-plane/internal/gateway"
	"github.com/openshift-online/hypershell/components/control-plane/internal/keycloak"
	cpotel "github.com/openshift-online/hypershell/components/control-plane/internal/otel"
	"github.com/openshift-online/hypershell/components/control-plane/internal/reconciler"
	"github.com/openshift-online/hypershell/components/control-plane/internal/serviceaccountkeycloak"
	"github.com/openshift-online/hypershell/components/control-plane/internal/serviceaccountprovisioner"
	"github.com/openshift-online/hypershell/components/control-plane/internal/watcher"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayclient "sigs.k8s.io/gateway-api/pkg/client/clientset/versioned"
)

const defaultManifestsDir = "/manifests/gateway"

func managedDatabaseWatchEligible(clientset *kubernetes.Clientset, dynamicClient dynamic.Interface) bool {
	return clientset != nil && dynamicClient != nil
}

// supervisedRestartCap bounds the backoff between restarts of a supervised
// background component, matching the cap watchLoop uses for stream
// reconnects.
const supervisedRestartCap = 30 * time.Second

// runSupervised runs fn in a loop so a failure in one background component
// (a watch stream, a reconciler's Run loop, the service-account provisioner)
// cannot take down the others by exiting the whole process. Every fn here is
// expected to block until ctx is done and return ctx.Err() at that point,
// exactly as watchLoop and the reconciler Run methods already do; a return
// before then is treated as a failure of that component alone; it is retried
// with the same capped exponential backoff watchLoop uses for stream
// reconnects rather than propagated to the process.
func runSupervised(ctx context.Context, name string, fn func(context.Context) error) {
	backoff := time.Second
	for {
		err := fn(ctx)
		if err == nil || ctx.Err() != nil {
			return
		}
		log.Printf("WARN %s exited with error, restarting in %s: %v", name, backoff, err)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff *= 2
		if backoff > supervisedRestartCap {
			backoff = supervisedRestartCap
		}
	}
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("loading config: %v", err)
	}

	log.Printf("INFO hypershell-controller starting")
	log.Printf("INFO grpc=%s api=%s namespace=%s database_provider=%s", cfg.GRPCServerAddr, cfg.APIServerURL, cfg.Namespace, cfg.DatabaseProvider)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	otelShutdown, otelErr := cpotel.Init(ctx)
	if otelErr != nil {
		log.Printf("WARN OpenTelemetry initialization failed, continuing without telemetry: %v", otelErr)
	}
	defer cpotel.Shutdown(otelShutdown)

	dialOpts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	}
	dialOpts = append(dialOpts, cpotel.GRPCDialOptions()...)

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

	if k8sConfig != nil {
		k8sConfig = cpotel.InstrumentK8sConfig(k8sConfig)
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

	// DATABASE_PROVIDER=cnpg is a hard startup precondition: the control plane
	// must fail cleanly here, before any watch/reconcile loop starts, when the
	// exact CNPG API resources this codebase depends on (clusters, databases,
	// databaseroles in postgresql.cnpg.io/v1) are not served, rather than
	// deferring the failure to the first CNPG-backed reconciliation deep
	// inside the gateway/database reconcilers. DATABASE_PROVIDER=deployment (the
	// default) never reaches this check and has no CNPG dependency at all.
	if cfg.DatabaseProvider == config.DatabaseProviderCNPG {
		if clientset == nil {
			log.Fatalf("DATABASE_PROVIDER=cnpg requires an in-cluster Kubernetes client to verify the CNPG API prerequisites")
		}
		if err := gateway.RequireCNPGAPI(clientset); err != nil {
			log.Fatalf("%v", err)
		}
		log.Printf("INFO CNPG API prerequisites verified for DATABASE_PROVIDER=cnpg")
	}

	// The Gateway Exposure port decouples route-address resolution and readiness
	// observation from the concrete ingress backend. Select the adapter by the
	// SAME effective ingress mode the reconciler uses to emit ingress resources
	// (gateway.IngressMode), not by raw CRD presence: IBM Cloud ROKS ships the
	// Gateway API CRDs but runs Route mode, so keying off CRD presence would wire
	// the Gateway API readiness observer for a gateway that is actually exposed
	// through an OpenShift Route -- leaving it stuck in Provisioning forever
	// ("per-tenant Gateway not found"). In "none" mode the port stays nil and
	// routed gateways are gated on Deployment readiness alone.
	// See specs/platform/openshell-gateway-routing.spec.md.
	var exposurePort exposure.Port
	if clientset != nil && k8sConfig != nil {
		hasGatewayAPI := gateway.DetectGatewayAPI(clientset)
		isOpenShift := gateway.DetectOpenShift(clientset)
		switch gateway.IngressMode(hasGatewayAPI, isOpenShift) {
		case gateway.IngressModeGatewayAPI:
			gwClient, gwErr := gatewayclient.NewForConfig(k8sConfig)
			if gwErr != nil {
				log.Fatalf("creating gateway-api client: %v", gwErr)
			}
			exposurePort = exposure.NewGatewayAPIExposure(gwClient)
			log.Printf("INFO gateway exposure port enabled (gateway-api adapter)")
		case gateway.IngressModeRoute:
			exposurePort = exposure.NewRouteExposure(dynamicClient)
			log.Printf("INFO gateway exposure port enabled (route adapter)")
		default:
			log.Printf("INFO gateway exposure port disabled (ingress mode none); routed gateways gated on Deployment readiness only")
		}
	}

	clusterReconciler := reconciler.NewManagedClusterReconciler()
	var databaseReconciler watcher.Handler[*pb.ManagedDatabase]
	if managedDatabaseWatchEligible(clientset, dynamicClient) {
		databaseReconciler = reconciler.NewManagedDatabaseReconciler(dynamicClient, clientset, conn, cfg.Namespace)
	} else {
		log.Printf("WARN ManagedDatabase watch disabled: both Kubernetes typed and dynamic clients are required")
	}
	releaseReconciler := reconciler.NewGatewayReleaseReconciler()
	networkReconciler := reconciler.NewGatewayNetworkReconciler()

	manifestsDir := os.Getenv("GATEWAY_MANIFESTS_DIR")
	if manifestsDir == "" {
		manifestsDir = defaultManifestsDir
	}

	var keycloakConfig *gateway.KeycloakConfig
	if clientset != nil {
		kcSecret, kcErr := clientset.CoreV1().Secrets(cfg.Namespace).Get(ctx, "hypershell-keycloak-admin", metav1.GetOptions{})
		if kcErr == nil {
			keycloakConfig = &gateway.KeycloakConfig{
				ServerURL:    string(kcSecret.Data["server-url"]),
				Realm:        string(kcSecret.Data["realm"]),
				ClientID:     string(kcSecret.Data["client-id"]),
				ClientSecret: string(kcSecret.Data["client-secret"]),
			}
			log.Printf("INFO keycloak admin secret found: server=%s realm=%s", keycloakConfig.ServerURL, keycloakConfig.Realm)
		} else {
			log.Printf("INFO keycloak admin secret not found, keycloak integration disabled: %v", kcErr)
		}
	}

	var roleBindingReconciler watcher.Handler[*pb.RoleBinding]
	var serviceAccountProvider *serviceaccountkeycloak.Client
	if keycloakConfig != nil {
		kcClient := keycloak.NewClient(
			keycloakConfig.ServerURL,
			keycloakConfig.Realm,
			keycloakConfig.ClientID,
			keycloakConfig.ClientSecret,
		)
		roleBindingReconciler = reconciler.NewRoleBindingReconciler(kcClient, conn)
		serviceAccountProvider = serviceaccountkeycloak.NewClient(
			keycloakConfig.ServerURL,
			keycloakConfig.Realm,
			keycloakConfig.ClientID,
			keycloakConfig.ClientSecret,
		)
		log.Printf("INFO role binding reconciler enabled with keycloak integration")
	}

	var gatewayReconciler watcher.Handler[*pb.Gateway]

	if clientset != nil && dynamicClient != nil {
		gr, grErr := reconciler.NewGatewayReconciler(dynamicClient, clientset, conn, manifestsDir, cfg.Namespace, keycloakConfig, exposurePort)
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

	watchCount := 4 // managed clusters, gateway releases, gateways, networks
	if databaseReconciler != nil {
		watchCount++
	}
	if roleBindingReconciler != nil {
		watchCount++
	}

	// Each background component below runs under runSupervised on its own
	// goroutine: a failure in one (a dropped watch stream, a reconciler's Run
	// loop returning, the service-account provisioner's listener dying) is
	// logged and retried in place, and never takes the others down with it.
	// wg lets main block until every component has actually observed ctx
	// cancellation and returned, instead of exiting out from under them.
	var wg sync.WaitGroup
	supervise := func(name string, fn func(context.Context) error) {
		wg.Go(func() {
			runSupervised(ctx, name, fn)
		})
	}

	if cfg.ServiceAccountProvisionerAddress != "" {
		provisionerServer := serviceaccountprovisioner.NewServer(serviceAccountProvider)
		transportConfig := serviceaccountprovisioner.TransportConfig{
			Address: cfg.ServiceAccountProvisionerAddress,
		}
		supervise("service-account provisioner", func(ctx context.Context) error {
			return serviceaccountprovisioner.ListenAndServe(ctx, transportConfig, provisionerServer)
		})
		log.Printf("INFO service-account provisioner launched on %s (in-cluster, NetworkPolicy-restricted)", cfg.ServiceAccountProvisionerAddress)
	} else {
		log.Printf("INFO service-account provisioner disabled")
	}

	supervise("ManagedCluster watch", func(ctx context.Context) error {
		return watcher.WatchManagedClusters(ctx, conn, clusterReconciler)
	})
	if databaseReconciler != nil {
		supervise("ManagedDatabase watch", func(ctx context.Context) error {
			return watcher.WatchManagedDatabases(ctx, conn, databaseReconciler)
		})
	}
	supervise("GatewayRelease watch", func(ctx context.Context) error {
		return watcher.WatchGatewayReleases(ctx, conn, releaseReconciler)
	})
	supervise("Gateway watch", func(ctx context.Context) error {
		return watcher.WatchGateways(ctx, conn, gatewayReconciler)
	})
	supervise("GatewayNetwork watch", func(ctx context.Context) error {
		return watcher.WatchGatewayNetworks(ctx, conn, networkReconciler)
	})
	if roleBindingReconciler != nil {
		supervise("RoleBinding watch", func(ctx context.Context) error {
			return watcher.WatchRoleBindings(ctx, conn, roleBindingReconciler)
		})
	}

	log.Printf("INFO all %d watch streams launched", watchCount)

	// The continuous gateway health reconciler keeps each Gateway's phase and
	// status synchronized with observed workload health (Running <-> Degraded).
	// It requires an in-cluster Kubernetes client to observe Deployments.
	if clientset != nil {
		healthReconciler := reconciler.NewGatewayHealthReconciler(clientset, dynamicClient, conn, exposurePort, keycloakConfig)
		supervise("gateway health reconciler", healthReconciler.Run)
		log.Printf("INFO gateway health reconciler launched")
	} else {
		log.Printf("WARN no kubernetes client available, gateway health reconciliation disabled")
	}

	// The sandbox-count reconciler maintains each Gateway's active_sandbox_count
	// from an event-driven watch on sandbox pods (with a periodic self-heal from
	// its cache), instead of a repeated full-namespace pod LIST. It requires an
	// in-cluster Kubernetes client to watch pods.
	if clientset != nil {
		sandboxCountReconciler := reconciler.NewSandboxCountReconciler(clientset, conn, 0)
		supervise("sandbox count reconciler", sandboxCountReconciler.Run)
		log.Printf("INFO sandbox count reconciler launched")
	} else {
		log.Printf("WARN no kubernetes client available, sandbox count reconciliation disabled")
	}

	// The namespace GC reconciler reaps gateway namespaces the control plane
	// created but that no longer have a live Gateway (e.g. a delete event missed
	// while the control plane was down, or a gateway that failed to bootstrap).
	// It requires an in-cluster Kubernetes client.
	if clientset != nil && cfg.NamespaceGCEnabled {
		gcReconciler := reconciler.NewNamespaceGCReconciler(
			clientset, conn, cfg.NamespaceGCInterval, cfg.NamespaceGCGracePeriod, cfg.Namespace,
		)
		supervise("namespace GC reconciler", gcReconciler.Run)
		log.Printf("INFO namespace GC reconciler launched (interval=%s grace=%s)",
			cfg.NamespaceGCInterval, cfg.NamespaceGCGracePeriod)
	} else if clientset != nil {
		log.Printf("INFO namespace GC reconciler disabled (GATEWAY_NAMESPACE_GC_ENABLED=false)")
	}

	<-ctx.Done()
	log.Printf("INFO shutdown signal received, waiting for background components to stop")
	wg.Wait()

	log.Printf("INFO hypershell-controller stopped")
}
