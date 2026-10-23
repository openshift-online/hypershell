package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	provisionerpb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/provisioner/v1"
	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/api-server/pkg/secretstore"
	"github.com/openshift-online/hypershell/components/control-plane/internal/auth"
	"github.com/openshift-online/hypershell/components/control-plane/internal/config"
	"github.com/openshift-online/hypershell/components/control-plane/internal/directory"
	"github.com/openshift-online/hypershell/components/control-plane/internal/exposure"
	"github.com/openshift-online/hypershell/components/control-plane/internal/gateway"
	"github.com/openshift-online/hypershell/components/control-plane/internal/grpctransport"
	"github.com/openshift-online/hypershell/components/control-plane/internal/helm"
	"github.com/openshift-online/hypershell/components/control-plane/internal/keycloak"
	cpotel "github.com/openshift-online/hypershell/components/control-plane/internal/otel"
	"github.com/openshift-online/hypershell/components/control-plane/internal/reconciler"
	"github.com/openshift-online/hypershell/components/control-plane/internal/registration"
	"github.com/openshift-online/hypershell/components/control-plane/internal/serviceaccountkeycloak"
	"github.com/openshift-online/hypershell/components/control-plane/internal/serviceaccountprovisioner"
	"github.com/openshift-online/hypershell/components/control-plane/internal/supervisor"
	"github.com/openshift-online/hypershell/components/control-plane/internal/watcher"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayclient "sigs.k8s.io/gateway-api/pkg/client/clientset/versioned"
)

// registerWithBackoff calls regClient.Register with exponential backoff until it
// succeeds. 403 and 409 responses are non-retryable: a 403 means the control
// plane lacks the managed-cluster-registrar Keycloak role, and a 409 means its
// name is held by a record it does not own (or its subject is registered under
// another name) and an operator must resolve it. Both return a fatal error
// immediately. See specs/platform/managed-cluster-registration.spec.md
// ("Fail-Closed Startup").
func registerWithBackoff(ctx context.Context, regClient registrar, initialBackoff time.Duration) (string, error) {
	backoff := initialBackoff
	const maxBackoff = 60 * time.Second
	for {
		clusterID, err := regClient.Register(ctx)
		if err == nil {
			return clusterID, nil
		}

		if errors.Is(err, registration.ErrForbidden) {
			return "", fmt.Errorf("managed-cluster-registrar role not assigned in Keycloak; assign the role and restart: %w", err)
		}
		if errors.Is(err, registration.ErrConflict) {
			return "", fmt.Errorf("managed cluster name cannot be registered by this control plane; an operator must remove the conflicting ManagedCluster record (and re-point its gateways) or change HYPERSHELL_MANAGED_CLUSTER_NAME: %w", err)
		}

		log.Printf("WARN managed-cluster registration failed (retrying in %s): %v", backoff, err)
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("registration cancelled: %w", ctx.Err())
		case <-time.After(backoff):
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}
}

// registrar is the registration call registerWithBackoff drives; a seam for tests.
type registrar interface {
	Register(ctx context.Context) (string, error)
}

// instanceLabelBackfillTimeout bounds the one-shot startup backfill that stamps
// this instance's identity label onto its legacy gateway namespaces, so a stalled
// API server cannot delay the GC reconciler's launch indefinitely.
const instanceLabelBackfillTimeout = 2 * time.Minute

func helmBinaryPath() string {
	if v := os.Getenv("HELM_BINARY"); v != "" {
		return v
	}
	return "/usr/local/bin/helm"
}

// buildInfo returns a single-line summary of the binary's provenance for the
// startup log: Go toolchain, target platform, and the VCS revision and build
// time recorded by the Go linker. It helps correlate a running controller with
// the exact commit and image it was built from. Values are best-effort; when
// build metadata is absent (for example a plain `go run`) they report "unknown".
func buildInfo() string {
	revision, vcsTime := "unknown", "unknown"
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision":
				revision = setting.Value
			case "vcs.time":
				vcsTime = setting.Value
			}
		}
	}
	return fmt.Sprintf("revision=%s vcs-time=%s go=%s platform=%s/%s",
		revision, vcsTime, runtime.Version(), runtime.GOOS, runtime.GOARCH)
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("loading config: %v", err)
	}

	// Gateway database provisioning is a hard startup precondition: the admin
	// credentials Secret must be mounted and well-formed (verify-full, PEM CA)
	// before any reconcile loop starts, so a misconfigured controller fails here
	// with a clear message instead of failing every gateway later. The server is
	// not contacted at startup; reachability is checked per reconcile with retries.
	if err := gateway.ValidateAdminCredentialsDir(cfg.GatewayDatabaseAdminDir); err != nil {
		log.Fatalf("gateway database admin credentials (GATEWAY_DATABASE_ADMIN_DIR=%s): %v", cfg.GatewayDatabaseAdminDir, err)
	}
	databaseConfig := gateway.DatabaseConfig{AdminCredentialsDir: cfg.GatewayDatabaseAdminDir}

	grpcTransport := grpctransport.Classify(cfg.GRPCServerAddr)
	log.Printf("INFO hypershell-controller starting")
	log.Printf("INFO build %s", buildInfo())
	log.Printf("INFO grpc=%s (transport=%s) api=%s namespace=%s managed-cluster-name=%s",
		cfg.GRPCServerAddr, grpcTransport, cfg.APIServerURL, cfg.Namespace, cfg.ManagedClusterName)

	// Verify helm binary is available
	helmBin := helmBinaryPath()
	if err := helm.VerifyHelmAvailable(context.Background(), helmBin); err != nil {
		log.Fatalf("helm binary verification failed: %v", err)
	}

	// Verify Helm chart is available
	if err := helm.VerifyChartPath(cfg.HelmChartPath); err != nil {
		log.Fatalf("helm chart verification failed: %v", err)
	}
	log.Printf("INFO helm chart verified at %s", cfg.HelmChartPath)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	otelShutdown, otelErr := cpotel.Init(ctx, cfg.Namespace)
	if otelErr != nil {
		log.Printf("WARN OpenTelemetry initialization failed, continuing without telemetry: %v", otelErr)
	}
	defer cpotel.Shutdown(otelShutdown)

	// Every RPC, including the long-lived watch streams, carries the control
	// plane's OIDC bearer token; the API server does not exempt watch streams
	// from authentication. The transport (plaintext in-cluster, TLS otherwise) is
	// selected from the dial address; a TLS handshake failure surfaces as an
	// ordinary connection error that the watchers retry with backoff.
	tokenProvider := auth.NewTokenProvider(cfg.OIDCIssuer, cfg.OIDCClientID, cfg.OIDCClientSecret)
	if cfg.OIDCTokenEndpoint != "" {
		tokenProvider.SetTokenEndpoint(cfg.OIDCTokenEndpoint)
		log.Printf("INFO using explicit OIDC token endpoint: %s", cfg.OIDCTokenEndpoint)
	}
	dialOpts := []grpc.DialOption{
		grpc.WithTransportCredentials(grpctransport.Credentials(cfg.GRPCServerAddr)),
		grpc.WithPerRPCCredentials(auth.NewGRPCCredentials(tokenProvider)),
		// HTTP/2 keepalive on the one shared client connection all watch streams
		// multiplex over. Without it a wedged or half-open connection carrying only
		// idle watch streams never surfaces an error, so a blocked Recv waits
		// forever and the owner Keycloak grant is never reassigned. PermitWithoutStream
		// keeps the probe running even when no RPC is active. See
		// specs/platform/control-plane.spec.md ("Watch Stream Liveness").
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                cfg.GRPCKeepaliveTime,
			Timeout:             cfg.GRPCKeepaliveTimeout,
			PermitWithoutStream: true,
		}),
	}
	dialOpts = append(dialOpts, cpotel.GRPCDialOptions()...)

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

	// Label this control-plane namespace so the per-cluster provider Vault's
	// provisioner-role namespace selector admits it (HYPERSHELL-369). Best-effort: if it
	// fails, per-tenant mount provisioning fails closed later with a clear error.
	if clientset != nil {
		if err := gateway.EnsureControlPlaneNamespaceLabel(ctx, clientset, cfg.Namespace); err != nil {
			log.Printf("WARN could not label control-plane namespace %s (provider-Vault provisioner login may be denied until resolved): %v", cfg.Namespace, err)
		}
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

	// Initialize Helm client for gateway deployments
	helmClient := &helm.ShellClient{
		ChartPath:  cfg.HelmChartPath,
		HelmBinary: helmBin,
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

	// The RoleBinding reconciler needs the registered cluster id, which is only
	// known after registration below; the Keycloak client is built here and the
	// reconciler once the id is in hand.
	var kcClient *keycloak.Client
	var roleBindingReconciler watcher.Handler[*pb.RoleBinding]
	var serviceAccountProvider *serviceaccountkeycloak.Client
	if keycloakConfig != nil {
		kcClient = keycloak.NewClient(
			keycloakConfig.ServerURL,
			keycloakConfig.Realm,
			keycloakConfig.ClientID,
			keycloakConfig.ClientSecret,
		)
		serviceAccountProvider = serviceaccountkeycloak.NewClient(
			keycloakConfig.ServerURL,
			keycloakConfig.Realm,
			keycloakConfig.ClientID,
			keycloakConfig.ClientSecret,
		)
		log.Printf("INFO role binding reconciler enabled with keycloak integration")
	}

	var gatewayReconciler watcher.Handler[*pb.Gateway]
	// Each background component below runs under supervisor.Run on its own
	// goroutine: a failure in one (a dropped watch stream, a reconciler's Run
	// loop returning, the service-account provisioner's listener dying) is
	// logged and retried in place, and never takes the others down with it.
	// wg lets main block until every component has actually observed ctx
	// cancellation and returned, instead of exiting out from under them.
	var wg sync.WaitGroup
	supervise := func(name string, fn func(context.Context) error) {
		wg.Go(func() {
			supervisor.Run(ctx, name, fn)
		})
	}

	if cfg.ServiceAccountProvisionerAddress != "" {
		provisionerServer := serviceaccountprovisioner.NewServer(serviceAccountProvider)
		transportConfig := serviceaccountprovisioner.TransportConfig{
			Address: cfg.ServiceAccountProvisionerAddress,
		}
		// The Keycloak realm directory projection (GTW-ACC-09) rides the same private
		// in-cluster channel. It needs Keycloak, so it is served only when a
		// Keycloak client is configured; the API server dials this one address for
		// both the provisioner and the directory.
		var directorySvc provisionerpb.DirectoryServiceServer
		if kcClient != nil {
			projection := directory.NewProjection(kcClient, cfg.DirectoryRefreshInterval)
			directorySvc = directory.NewServer(projection)
			supervise("keycloak directory projection", projection.Run)
		}
		supervise("service-account provisioner", func(ctx context.Context) error {
			return serviceaccountprovisioner.ListenAndServe(ctx, transportConfig, provisionerServer, directorySvc)
		})
		log.Printf("INFO service-account provisioner launched on %s (in-cluster, NetworkPolicy-restricted)", cfg.ServiceAccountProvisionerAddress)
	} else {
		log.Printf("INFO service-account provisioner disabled")
	}

	// Mandatory cluster identity: resolve cluster_id by registration before any
	// gRPC watch or gateway-scoped reconciler starts. There is no unregistered
	// mode; a failure here either retries (transient) or exits (403/409). The
	// service-account provisioner above is not cluster-scoped and is already
	// serving, so the pod's probes stay green while registration retries (for
	// example while the hub API server is still starting).
	regClient := registration.NewClient(cfg.APIServerURL, cfg.ManagedClusterName, cfg.ManagedClusterCapabilities, tokenProvider)
	clusterID, regErr := registerWithBackoff(ctx, regClient, time.Second)
	if regErr != nil {
		if ctx.Err() != nil {
			log.Printf("INFO shutdown signal received before registration completed; stopping")
			wg.Wait()
			return
		}
		log.Fatalf("FATAL managed-cluster registration failed: %v", regErr)
	}
	log.Printf("INFO registered as cluster_id=%s (name=%s); scoping gateway watch, seed, health, sandbox counts, backfill, and namespace GC to it", clusterID, cfg.ManagedClusterName)

	if clientset != nil && dynamicClient != nil {
		gr, grErr := reconciler.NewGatewayReconciler(
			dynamicClient,
			clientset,
			conn,
			helmClient,
			cfg.Namespace,
			keycloakConfig,
			exposurePort,
			cfg.ExternalCAIssuerName,
			cfg.ExternalCAIssuerKind,
			databaseConfig,
			clusterID,
			cfg.GatewayResources,
			cfg.SandboxRuntimeClass,
		)
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

	// The gateway reconcile queue is driven by the gateway watch stream. It is
	// created after registration so the reconciler and all queued work use the
	// registered cluster identity.
	gatewayQueue := watcher.NewGatewayReconcileQueue(ctx, gatewayReconciler, cfg.GatewayReconcileWorkers)
	defer gatewayQueue.Stop()

	watchCount := 3 // managed clusters, gateways, agent runtimes
	if kcClient != nil {
		watchCount++ // role bindings
	}

	// Heartbeat: re-register every 60s to update last_seen_at on the hub.
	go func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		var consecutiveFailures int
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				heartbeatID, err := regClient.Register(ctx)
				if err != nil {
					consecutiveFailures++
					if consecutiveFailures >= 5 {
						log.Printf("ERROR heartbeat has failed %d consecutive times; hub may be unreachable: %v", consecutiveFailures, err)
					} else {
						log.Printf("WARN heartbeat registration failed: %v", err)
					}
					continue
				}
				consecutiveFailures = 0
				// The record this process filters by is gone (deleted, or the
				// hub database was reset) and registration created a new one.
				// Every watch and reconciler is scoped to the old id, so restart
				// to pick up the new identity rather than serve a stale filter.
				if heartbeatID != clusterID {
					log.Fatalf("FATAL registered cluster_id changed from %s to %s (ManagedCluster record was recreated); exiting so the control plane restarts with the new identity", clusterID, heartbeatID)
				}
			}
		}
	}()

	if kcClient != nil {
		roleBindingReconciler = reconciler.NewRoleBindingReconciler(kcClient, conn, clusterID)
	}

	supervise("ManagedCluster watch", func(ctx context.Context) error {
		return watcher.WatchManagedClusters(ctx, conn, clusterReconciler, cfg.WatchIdleTimeout)
	})
	supervise("Gateway watch", func(ctx context.Context) error {
		return watcher.WatchGateways(ctx, conn, gatewayQueue, clusterID, cfg.WatchIdleTimeout)
	})
	if roleBindingReconciler != nil {
		supervise("RoleBinding watch", func(ctx context.Context) error {
			return watcher.WatchRoleBindings(ctx, conn, roleBindingReconciler, clusterID, cfg.WatchIdleTimeout, cfg.RoleBindingResyncInterval)
		})
	}

	// AgentRuntime watch (issue #521): cluster-scoped, seed-on-reconnect transport
	// feeding the Wave B reconciler. It applies cluster-side scheduling resources
	// (Namespace, ServiceAccount, NetworkPolicy, one Kubernetes Secret per
	// SecretSource placed directly from the read-only secret store, coordinator
	// CronJob) and drives the status lifecycle. clientset may be nil on a control
	// plane without cluster access, in which case the reconciler skips cluster-side
	// provisioning.
	//
	// Gateway-side workspace provisioning (HYPERSHELL-412): the control plane realizes each
	// HyperShell ext kind as its OpenShell native kind by consuming OpenShell's EXISTING
	// provisioning API (the operations the `openshell` CLI drives), through the
	// reconciler.NewOpenShellProviderRealizer adapter over the gateway.OpenShellProvisioner.
	// The superseded bespoke AgentWorkspaceProvisioning gRPC contract and its
	// HYPERSHELL_GATEWAY_PROVISIONING_ENDPOINT gate have been removed. The AgentRuntime
	// watch payload embeds the workspace name, ProviderBindings, memberships, and inference
	// route (data-model.spec.md "AgentRuntime Reconcile Payload Completeness"), so the
	// realizer can provision from a single event. It is activated through SetProviderRealizer
	// once a live OpenShell API session factory is wired; until then it stays nil and the
	// reconciler keeps the transitional raw-Secret placement so the live fleet does not
	// regress.
	// Pass a typed-nil *kubernetes.Clientset as a genuinely nil interface so the
	// reconciler's nil check works (a nil pointer in an interface is not == nil).
	var agentRuntimeKube kubernetes.Interface
	if clientset != nil {
		agentRuntimeKube = clientset
	}
	// Read-only secret-store client: the control plane reads each SecretSource value
	// and places a Kubernetes Secret directly (no ExternalSecret/ESO). Configuration,
	// not code, selects the backend (HYPERSHELL_SECRET_STORE_*); the default is a
	// disabled reader that rejects reads, so a runtime declaring a SecretSource is
	// degraded until a store is configured.
	secretReader, secretReaderErr := secretstore.NewReaderFromEnv()
	if secretReaderErr != nil {
		log.Printf("WARN secret store reader misconfigured, SecretSource placement will fail: %v", secretReaderErr)
		secretReader = secretstore.NewErrorReader(secretReaderErr)
	}
	agentRuntimeReconciler := reconciler.NewAgentRuntimeReconciler(agentRuntimeKube, secretReader, conn, clusterID)

	// Gateway-side provider realization over OpenShell's EXISTING API is wired through the
	// gateway.OpenShellProvisioner + reconciler.NewOpenShellProviderRealizer seam (with the
	// bounded control-plane GitHub rotator for app-installation-token credentials). Live
	// activation needs a gateway.OpenShellSessionFactory that opens a session to OpenShell's
	// provisioning API as the gateway admin identity. That live adapter depends on the
	// OpenShell upstream client surface, which is NOT reachable from this repository (only
	// the pinned ref and the `openshell` CLI are present); it is tracked as a follow-up. Until
	// that adapter is wired the realizer stays nil and the reconciler keeps the transitional
	// raw-Secret placement so the live fleet does not regress (realize-before-prune applies
	// once the realizer is set).
	log.Printf("INFO gateway-side provider realization over OpenShell's existing API is not yet active (live OpenShell API session factory pending upstream); using transitional raw-Secret placement")

	supervise("AgentRuntime watch", func(ctx context.Context) error {
		return watcher.WatchAgentRuntimes(ctx, conn, agentRuntimeReconciler, clusterID, cfg.WatchIdleTimeout)
	})

	log.Printf("INFO all %d watch streams launched", watchCount)

	// The declarative `.hypershell` import reconciler (RepositoryReconciler) is the
	// single git-to-API sync component: for each registered Repository it reads only
	// the import_path subtree over the GitHub API, renders it in memory through the
	// shared pkg/kustomize library, and reconciles (create/update/prune) the
	// Repository-owned ext resources into the API over the gRPC write path. It does
	// NOT provision cluster-side; each imported Kind is reconciled into the cluster
	// by its own per-Kind reconciler. It is OFF unless explicitly enabled and a
	// platform GitHub App credential is provisioned, so an unconfigured control
	// plane stays inert. See specs/platform/control-plane.spec.md.
	if cfg.RepositoryImportEnabled {
		platformToken := ""
		if cfg.GitHubTokenFile != "" {
			tokenBytes, tokenErr := os.ReadFile(cfg.GitHubTokenFile)
			if tokenErr != nil {
				log.Printf("WARN repository import: reading GitHub token file %s failed; default platform credential unavailable (private repositories with a secret_source_id still work): %v", cfg.GitHubTokenFile, tokenErr)
			} else {
				platformToken = strings.TrimSpace(string(tokenBytes))
			}
		} else {
			log.Printf("INFO repository import: no HYPERSHELL_GITHUB_TOKEN_FILE set; only repositories with a per-Repository secret_source_id credential (or public repositories) can be read")
		}
		importResolver := reconciler.NewSecretStoreCredentialResolverWithGitHubApp(secretReader, pb.NewSecretSourceServiceClient(conn), platformToken, nil, cfg.GitHubAPIBase)
		importFetcher := reconciler.NewGitHubSubtreeFetcher(nil, cfg.GitHubAPIBase)
		importWriter := reconciler.NewGRPCImportWriter(conn)
		repositoryReconciler := reconciler.NewRepositoryReconciler(
			pb.NewRepositoryServiceClient(conn),
			importResolver,
			importFetcher,
			importWriter,
			cfg.RepositoryImportInterval,
		)
		supervise("repository import reconciler", repositoryReconciler.Run)
		// Event-driven import: the Repository watch stream reconciles a created or
		// changed Repository in real time, so the reconciler above only has to run a
		// slow periodic drift re-check for upstream changes that emit no Repository
		// event. See specs/platform/control-plane.spec.md.
		supervise("Repository watch", func(ctx context.Context) error {
			return watcher.WatchRepositories(ctx, conn, repositoryReconciler, cfg.WatchIdleTimeout)
		})
		log.Printf("INFO repository import reconciler launched (drift re-check interval=%s, event-driven via Repository watch)", cfg.RepositoryImportInterval)
	} else {
		log.Printf("INFO repository import reconciler disabled (HYPERSHELL_REPOSITORY_IMPORT_ENABLED=false)")
	}

	// The continuous gateway health reconciler keeps each Gateway's phase and
	// status synchronized with observed workload health (Running <-> Degraded).
	// It requires an in-cluster Kubernetes client to observe Deployments.
	if clientset != nil {
		healthReconciler := reconciler.NewGatewayHealthReconciler(clientset, dynamicClient, conn, exposurePort, keycloakConfig, clusterID, cfg.Namespace)
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
		sandboxCountReconciler := reconciler.NewSandboxCountReconciler(clientset, conn, 0, clusterID)
		supervise("sandbox count reconciler", sandboxCountReconciler.Run)
		log.Printf("INFO sandbox count reconciler launched")

		if dynamicClient != nil {
			sandboxAttentionReconciler := reconciler.NewSandboxAttentionReconciler(
				clientset, dynamicClient, conn, 0, clusterID, cfg.Namespace,
			)
			supervise("sandbox attention reconciler", sandboxAttentionReconciler.Run)
			log.Printf("INFO sandbox attention reconciler launched")

			sandboxEventsReconciler := reconciler.NewSandboxEventsReconciler(clientset, dynamicClient, conn, 0, clusterID, 0)
			supervise("sandbox events reconciler", sandboxEventsReconciler.Run)
			log.Printf("INFO sandbox events reconciler launched")
		} else {
			log.Printf("WARN no dynamic client available, sandbox attention and events reconciliation disabled")
		}
	} else {
		log.Printf("WARN no kubernetes client available, sandbox count reconciliation disabled")
	}

	// The namespace GC reconciler reaps gateway namespaces the control plane
	// created but that no longer have a live Gateway (e.g. a delete event missed
	// while the control plane was down, or a gateway that failed to bootstrap).
	// It requires an in-cluster Kubernetes client.
	if clientset != nil && cfg.NamespaceGCEnabled {
		// Before the first sweep, backfill this instance's identity label onto the
		// legacy gateway namespaces it still owns per its API server. A Gateway in
		// a steady-state phase is skipped by the reconciler's phase gate on start,
		// so its pre-label namespace is otherwise never migrated and a later
		// missed-delete would leak it past the instance-scoped sweep. This runs
		// synchronously so the first sweep sees the freshly-labeled namespaces; it
		// is best-effort and never blocks startup on failure.
		backfillCtx, cancelBackfill := context.WithTimeout(ctx, instanceLabelBackfillTimeout)
		reconciler.RunInstanceLabelBackfill(backfillCtx, clientset, conn, cfg.Namespace, clusterID)
		cancelBackfill()

		gcReconciler := reconciler.NewNamespaceGCReconciler(
			clientset, conn, cfg.NamespaceGCInterval, cfg.NamespaceGCGracePeriod, cfg.Namespace, clusterID,
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
