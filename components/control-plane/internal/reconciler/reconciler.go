package reconciler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"
	"unicode"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/api-server/pkg/gatewayhealth"
	"github.com/openshift-online/hypershell/components/control-plane/internal/exposure"
	"github.com/openshift-online/hypershell/components/control-plane/internal/gateway"
	"github.com/openshift-online/hypershell/components/control-plane/internal/helm"
	"github.com/openshift-online/hypershell/components/control-plane/internal/keycloak"
	cpotel "github.com/openshift-online/hypershell/components/control-plane/internal/otel"
	"github.com/openshift-online/hypershell/components/control-plane/internal/watcher"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

type ManagedClusterReconciler struct {
	mu     sync.Mutex
	active map[string]struct{}
}

func NewManagedClusterReconciler() *ManagedClusterReconciler {
	return &ManagedClusterReconciler{active: make(map[string]struct{})}
}

func (r *ManagedClusterReconciler) Handle(ctx context.Context, event watcher.Event[*pb.ManagedCluster]) error {
	r.mu.Lock()
	if _, ok := r.active[event.ResourceID]; ok {
		r.mu.Unlock()
		return nil
	}
	r.active[event.ResourceID] = struct{}{}
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		delete(r.active, event.ResourceID)
		r.mu.Unlock()
	}()

	_, endSpan := cpotel.StartReconcileSpan(ctx, "ManagedCluster", event.Type.String(), event.Resource.GetMetadata().GetTraceparent())
	defer func() { endSpan(cpotel.OutcomeSuccess, "", nil) }()

	slog.InfoContext(ctx, "Reconciling ManagedCluster", "managed_cluster", event.ResourceID, "event_type", event.Type)
	return nil
}

// recordIncompleteFinalizationEvent records a durable, operator-visible
// Kubernetes Event stating that a gateway-owned resource was left unreclaimed
// during deletion with no automatic recovery path. The Event is created in the
// control-plane namespace (not the gateway namespace, which is itself being
// reaped) so it outlives the deleted resources, satisfying the no-silent-orphan
// contract. It fails closed when no control-plane namespace is configured: a
// leaked resource with no durable record is exactly the silent orphan this
// guards against. The message must never carry secrets; callers pass only the
// resource kind, name, and a human-readable reason.
//
// It takes kubernetes.Interface (not the concrete *kubernetes.Clientset the
// reconciler holds) so it is unit-testable with a fake clientset.
func recordIncompleteFinalizationEvent(ctx context.Context, client kubernetes.Interface, cpNamespace, gatewayID, resourceKind, resourceName, reason string) error {
	if cpNamespace == "" {
		return fmt.Errorf("no control-plane namespace configured; cannot record the required IncompleteFinalization Event for gateway %s resource %s %q", gatewayID, resourceKind, resourceName)
	}
	now := metav1.NewTime(time.Now())
	event := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "gateway-finalization-",
			Namespace:    cpNamespace,
		},
		InvolvedObject: corev1.ObjectReference{
			Kind: "Gateway",
			Name: gatewayID,
			// The Event lives in the control-plane namespace so it outlives the
			// reaped gateway resources. Kubernetes requires involvedObject.namespace
			// to match event.namespace for namespaced Events; Name still identifies
			// the gateway whose finalization was incomplete.
			Namespace: cpNamespace,
		},
		Reason:         "IncompleteFinalization",
		Message:        fmt.Sprintf("gateway %s deletion left %s %q unreclaimed with no automatic recovery path: %s", gatewayID, resourceKind, resourceName, reason),
		Type:           corev1.EventTypeWarning,
		Source:         corev1.EventSource{Component: "hypershell-control-plane"},
		FirstTimestamp: now,
		LastTimestamp:  now,
		Count:          1,
	}
	if _, err := client.CoreV1().Events(cpNamespace).Create(ctx, event, metav1.CreateOptions{}); err != nil {
		return err
	}
	return nil
}

type GatewayReconciler struct {
	mu                    sync.Mutex
	active                map[string]struct{}
	dynamicClient         dynamic.Interface
	clientset             *kubernetes.Clientset
	grpcConn              *grpc.ClientConn
	helmClient            *helm.ShellClient
	isOpenShift           bool
	hasCertManager        bool
	controlPlaneNamespace string
	keycloakClient        *keycloak.Client
	keycloakConfig        *gateway.KeycloakConfig
	exposure              exposure.Port
	ingressBaseDomain     string
	// database locates the mounted admin credentials every gateway database is
	// provisioned with; validated once at controller startup.
	database  gateway.DatabaseConfig
	clusterID string
	// gatewayResources overrides the gateway container requests and limits
	// (GATEWAY_RESOURCES); nil uses the chart-values default.
	gatewayResources *corev1.ResourceRequirements
	// sandboxRuntimeClass is the Kubernetes RuntimeClass name applied to every
	// sandbox pod (GATEWAY_SANDBOX_RUNTIME_CLASS); empty uses the cluster default.
	sandboxRuntimeClass string
	// operatorNamespaceLabelKey is the administrator-approved label KEY used to
	// scope operator-mode gateways to their own tenant namespaces
	// (GATEWAY_OPERATOR_NAMESPACE_LABEL_KEY); empty fails operator gateways closed
	// and skips the operator tenant-namespace sweep on delete (OSG-OPWS-02/04/06).
	operatorWorkspace         *helm.OperatorWorkspaceConfig
	operatorNamespaceLabelKey string
	// operatorSandboxRuntimeClass is the administrator-approved sandbox RuntimeClass
	// applied to operator-mode gateways only (GATEWAY_OPERATOR_SANDBOX_RUNTIME_CLASS).
	operatorSandboxRuntimeClass string
	// gatewayClientOverride, when non-nil, supplies the GatewayServiceClient used
	// by the nil-payload delete re-read (CTRL-WDEL-04). Production leaves it nil
	// and dials grpcConn; tests inject a fake without standing up a gRPC server.
	gatewayClientOverride pb.GatewayServiceClient
}

// gatewayServiceClient returns the GatewayServiceClient used for direct gRPC
// reads. It honors a test-injected override and otherwise dials the shared
// connection, matching every other call site in this package.
func (r *GatewayReconciler) gatewayServiceClient() pb.GatewayServiceClient {
	if r.gatewayClientOverride != nil {
		return r.gatewayClientOverride
	}
	return pb.NewGatewayServiceClient(r.grpcConn)
}

// rereadDeletingGateway re-reads a deleting Gateway record with the unscoped
// query (include_deleted) so a nil-payload delete event can reconcile teardown
// from the record (CTRL-WDEL-02, CTRL-WDEL-04). It returns an error when the
// record cannot be loaded so the caller can defer and retry rather than report a
// delete complete while owned resources were never reconciled.
func (r *GatewayReconciler) rereadDeletingGateway(ctx context.Context, id string) (*pb.Gateway, error) {
	resp, err := r.gatewayServiceClient().GetGateway(ctx, &pb.GetGatewayRequest{Id: id, IncludeDeleted: true})
	if err != nil {
		return nil, err
	}
	gw := resp.GetGateway()
	if gw == nil {
		return nil, fmt.Errorf("unscoped re-read returned no gateway record for %s", id)
	}
	return gw, nil
}

// currentRouteAddress returns the gateway's currently persisted routeAddress,
// re-read live rather than trusted from snapshot (the watch-event payload
// captured when a provisioning pass started), so a routeAddress-publication
// fallback reflects state as of now, not as of several minutes ago
// (OSG-ROUT-07.01: "currently persisted"). A transient re-read error falls
// back to snapshot's own field rather than aborting the pass: that is no
// worse than this re-read not existing at all.
func (r *GatewayReconciler) currentRouteAddress(ctx context.Context, gatewayID string, snapshot *pb.Gateway) string {
	resp, err := r.gatewayServiceClient().GetGateway(ctx, &pb.GetGatewayRequest{Id: gatewayID})
	if err != nil {
		slog.WarnContext(ctx, "Gateway: re-read for routeAddress fallback failed; using this pass's own snapshot", "gateway_id", gatewayID, "error", err)
		return snapshot.GetRouteAddress()
	}
	if current := resp.GetGateway(); current != nil {
		return current.GetRouteAddress()
	}
	return snapshot.GetRouteAddress()
}

// recordDeleteOrphans records the no-silent-orphan signal for each named
// owned-resource class that a delete could not reconcile, as a durable,
// operator-visible Event in addition to the retryable error the caller returns
// (GTW-DEL-07). It is used where an out-of-namespace class (the per-gateway
// database, the Keycloak clients) has no namespace-GC recovery path. Callers name
// only the classes the gateway actually owns so operators are not sent chasing a
// resource that was never created.
func (r *GatewayReconciler) recordDeleteOrphans(ctx context.Context, gatewayID, resourceName, reason string, kinds ...string) {
	for _, kind := range kinds {
		if err := recordIncompleteFinalizationEvent(ctx, r.clientset, r.controlPlaneNamespace, gatewayID, kind, resourceName, reason); err != nil {
			slog.ErrorContext(ctx, "Failed to record IncompleteFinalization Event",
				"gateway_id", gatewayID,
				"kind", kind,
				"error", err)
		}
	}
}

func NewGatewayReconciler(
	dynamicClient dynamic.Interface,
	clientset *kubernetes.Clientset,
	grpcConn *grpc.ClientConn,
	helmClient *helm.ShellClient,
	controlPlaneNamespace string,
	keycloakConfig *gateway.KeycloakConfig,
	exposurePort exposure.Port,
	database gateway.DatabaseConfig,
	clusterID string,
	gatewayResources *corev1.ResourceRequirements,
	sandboxRuntimeClass string,
	operatorWorkspace *helm.OperatorWorkspaceConfig,
	operatorNamespaceLabelKey string,
	operatorSandboxRuntimeClass string,
) (*GatewayReconciler, error) {
	if database.AdminCredentialsDir == "" {
		return nil, fmt.Errorf("gateway database admin credentials directory is required")
	}
	if exposurePort == nil {
		return nil, fmt.Errorf("gateway exposure port is required")
	}

	isOpenShift := gateway.DetectOpenShift(clientset)
	hasCertManager := gateway.DetectCertManager(clientset)

	// Keycloak is mandatory: every gateway gets a generated OIDC configuration
	// (the gateway refuses to start without an auth mechanism, issue #809).
	if keycloakConfig == nil {
		return nil, fmt.Errorf("keycloak configuration is required")
	}
	kcClient := keycloak.NewClient(
		keycloakConfig.ServerURL,
		keycloakConfig.Realm,
		keycloakConfig.ClientID,
		keycloakConfig.ClientSecret,
	)

	// Derive ingress base domain from environment
	ingressBaseDomain := os.Getenv("INGRESS_BASE_DOMAIN")
	if ingressBaseDomain == "" {
		ingressBaseDomain = "gateway.cluster.local"
		slog.Warn("INGRESS_BASE_DOMAIN not set, using default", "ingress_base_domain", ingressBaseDomain)
	}

	slog.Info("Gateway reconciler initialized",
		"helm", helmClient.ChartPath,
		"openshift", isOpenShift,
		"certmanager", hasCertManager,
		"keycloak", keycloakConfig.ServerURL,
		"ingress", ingressBaseDomain)

	return &GatewayReconciler{
		active:                      make(map[string]struct{}),
		dynamicClient:               dynamicClient,
		clientset:                   clientset,
		grpcConn:                    grpcConn,
		helmClient:                  helmClient,
		isOpenShift:                 isOpenShift,
		hasCertManager:              hasCertManager,
		controlPlaneNamespace:       controlPlaneNamespace,
		keycloakClient:              kcClient,
		keycloakConfig:              keycloakConfig,
		exposure:                    exposurePort,
		ingressBaseDomain:           ingressBaseDomain,
		database:                    database,
		clusterID:                   clusterID,
		gatewayResources:            gatewayResources,
		sandboxRuntimeClass:         sandboxRuntimeClass,
		operatorWorkspace:           operatorWorkspace,
		operatorNamespaceLabelKey:   operatorNamespaceLabelKey,
		operatorSandboxRuntimeClass: operatorSandboxRuntimeClass,
	}, nil
}

func (r *GatewayReconciler) Handle(ctx context.Context, event watcher.Event[*pb.Gateway]) error {
	r.mu.Lock()
	if _, ok := r.active[event.ResourceID]; ok {
		r.mu.Unlock()
		return nil
	}
	r.active[event.ResourceID] = struct{}{}
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		delete(r.active, event.ResourceID)
		r.mu.Unlock()
	}()

	gw := event.Resource
	if gw == nil {
		if event.Type != watcher.EventDeleted {
			// A create/update event with no payload carries no desired state to
			// reconcile toward, so there is nothing to do and nothing to orphan.
			slog.WarnContext(ctx, "Gateway event has nil resource, skipping", "gateway_id", event.ResourceID)
			return nil
		}
		// Nil-payload delete (CTRL-WDEL-04): the API server delivers nil-payload
		// delete events to unscoped subscribers, so this is reachable in normal
		// operation. Returning success here would orphan the gateway's
		// out-of-namespace resources (its database and Keycloak clients), which
		// have no namespace-GC recovery path. Re-read the deleting record with the
		// unscoped query and reconcile teardown from it; when it cannot be loaded,
		// record the no-silent-orphan signal and return a retryable error so the
		// delete is retried (GTW-DEL-04, GTW-DEL-07).
		reread, rereadErr := r.rereadDeletingGateway(ctx, event.ResourceID)
		if rereadErr != nil {
			slog.ErrorContext(ctx, "Nil-payload gateway delete could not re-read the deleting record; deferring and recording orphan signal", "gateway_id", event.ResourceID, "error", rereadErr)
			// The record cannot be inspected, so over-signal both out-of-namespace
			// classes rather than risk a silent orphan.
			r.recordDeleteOrphans(ctx, event.ResourceID, event.ResourceID,
				fmt.Sprintf("nil-payload delete could not re-read the deleting gateway record (%v); the per-gateway database and Keycloak clients could not be reconciled", rereadErr),
				"PostgreSQLDatabase", "KeycloakClient")
			return fmt.Errorf("re-read deleting gateway %s for nil-payload delete: %w", event.ResourceID, rereadErr)
		}
		slog.InfoContext(ctx, "Nil-payload gateway delete re-read the deleting record; reconciling teardown from it", "gateway_id", event.ResourceID)
		gw = reread
	}
	previousPhase := gw.GetPhase()
	if event.PhaseBeforeRetry != "" {
		previousPhase = event.PhaseBeforeRetry
	}
	suppressGatewayProvisionObservation(event.ResourceID, previousPhase)
	ctx, endSpan := cpotel.StartReconcileSpan(ctx, "Gateway", event.Type.String(), gw.GetMetadata().GetTraceparent())
	if event.Type != watcher.EventDeleted {
		observedGeneration := gw.ObservedGeneration
		cpotel.SetResourceStatusStale(
			ctx,
			r.clusterID,
			"Gateway",
			event.ResourceID,
			observedGeneration == nil || *observedGeneration < gw.GetGeneration(),
		)
	}
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attribute.String("hypershell.resource_id", event.ResourceID))
	var reconcileErr error
	var outcome, reason string
	defer func() {
		// If outcome not explicitly set, classify from error
		if outcome == "" {
			outcome, reason = cpotel.ClassifyReconcileOutcome(reconcileErr)
		}
		endSpan(outcome, reason, reconcileErr)
	}()

	if event.Type == watcher.EventDeleted {
		forgetGatewayProvisionObservation(event.ResourceID)
		var deleteErrs []error

		namespace, namespaceErr := gatewayNamespace(gw)
		if namespaceErr != nil {
			// A missing namespace is an unresolved precondition, not a completed
			// delete: without it the gateway's owned resources (its per-gateway
			// database, Keycloak clients, and in-namespace workloads) cannot be
			// located and reconciled toward absence. Reporting success here would
			// silently orphan the out-of-namespace classes, which have no
			// namespace-GC recovery path. Record the no-silent-orphan signal for
			// those classes and return a retryable error so the delete is retried
			// (GTW-DEL-01, GTW-DEL-04, GTW-DEL-07).
			slog.ErrorContext(ctx, "Gateway deleted but namespace unresolved; owned resources cannot be reconciled toward absence", "gateway_id", event.ResourceID, "error", namespaceErr)
			// The record is loaded here, so signal only the classes this gateway
			// actually owns: every gateway owns a database, but a gateway with no
			// OIDC configured never had Keycloak clients.
			orphanKinds := []string{"PostgreSQLDatabase"}
			if gw.GetOidc() != "" {
				orphanKinds = append(orphanKinds, "KeycloakClient")
			}
			r.recordDeleteOrphans(ctx, event.ResourceID, gw.GetName(),
				fmt.Sprintf("gateway namespace is unresolved (%v); the owned resources could not be reconciled", namespaceErr),
				orphanKinds...)
			deleteErrs = append(deleteErrs, fmt.Errorf("resolve namespace for gateway %s: %w", event.ResourceID, namespaceErr))
		} else {
			slog.InfoContext(ctx, "Gateway deleted, cleaning up resources", "gateway_id", event.ResourceID, "namespace", namespace)
			opts := gateway.ReconcileOpts{
				IsOpenShift:           r.isOpenShift,
				HasCertManager:        r.hasCertManager,
				Database:              r.database,
				ControlPlaneNamespace: r.controlPlaneNamespace,
				GatewayID:             event.ResourceID,
				GatewayName:           gw.Name,
			}
			// A best-effort cleanup failure that leaves a gateway-owned resource
			// behind (a ClusterRoleBinding or Keycloak client with no cascading
			// owner and no reclaiming reconciler) must not be a silent orphan: it is
			// recorded as a durable, operator-visible Event. Recording is itself
			// best-effort within the delete pass -- a failed record is logged but
			// does not fail finalization, since the underlying deletion error was
			// already tolerated as best-effort.
			opts.RecordOrphan = func(rctx context.Context, resourceKind, resourceName, reason string) {
				if err := recordIncompleteFinalizationEvent(rctx, r.clientset, r.controlPlaneNamespace, event.ResourceID, resourceKind, resourceName, reason); err != nil {
					slog.ErrorContext(ctx, "Failed to record IncompleteFinalization Event",
						"gateway_id", event.ResourceID,
						"kind", resourceKind,
						"name", resourceName,
						"error", err)
				}
			}
			clientID, err := gatewayKeycloakClientID(event.ResourceID, gw)
			if err != nil {
				// Invalid stored identity never becomes valid on retry, so failing
				// here would pin the delete tombstone after namespace and database
				// cleanup. Keycloak is not contacted. Record the leak durably (not
				// just a log line) so the orphaned Keycloak clients are visible to
				// operators for recovery.
				slog.ErrorContext(ctx, "Gateway stored identity cannot be resolved; skipping Keycloak cleanup; Keycloak clients may remain for operator recovery", "gateway_id", event.ResourceID, "error", err)
				opts.RecordOrphan(ctx, "KeycloakClient", fmt.Sprintf("gateway %s (%s)", gw.Name, event.ResourceID),
					fmt.Sprintf("stored identity cannot be resolved (%v); Keycloak clients could not be deleted", err))
			} else {
				opts.KeycloakClient = r.keycloakClient
				opts.GatewayClientID = clientID
			}
			var credentialNamespaces []string
			if gw.CredentialDriver != nil && *gw.CredentialDriver != "" {
				if strings.Contains(*gw.CredentialDriver, "kubernetes_secrets") {
					var credCfg gateway.CredentialDriverConfig
					if err := json.Unmarshal([]byte(*gw.CredentialDriver), &credCfg); err == nil {
						if credCfg.KubernetesSecrets != nil && credCfg.KubernetesSecrets.Namespace != "" {
							credentialNamespaces = append(credentialNamespaces, credCfg.KubernetesSecrets.Namespace)
						}
					}
				}
			}
			if err := gateway.DeleteGatewayResources(ctx, r.dynamicClient, r.clientset, r.helmClient, namespace, opts, credentialNamespaces...); err != nil {
				deleteErrs = append(deleteErrs, fmt.Errorf("delete gateway resources in %s: %w", namespace, err))
			} else {
				slog.InfoContext(ctx, "Gateway resources cleaned up", "gateway_id", event.ResourceID, "namespace", namespace)
			}

			// Delete the gateway's per-tenant Vault KV mount (openshell-gw/<namespace>),
			// which removes every provider-credential entry it held in one operation,
			// through the control plane's scoped provisioner identity (HYPERSHELL-369).
			// See openshell-gateway-credentials.spec.md (Provider Credential Lifecycle on Deletion).
			//
			// Use the nil-safe getter, NOT a `CredentialDriver != nil` guard: a gateway created
			// with no credential_driver is stored as NULL yet still resolves to vault (the flat
			// default), so it has an openshell-gw/<ns> mount to reclaim. IsVaultBacked("") is true.
			// A malformed stored driver is NOT assumed to be vault (it records an orphan instead).
			vaultBacked, driverErr := gateway.IsVaultBacked(gw.GetCredentialDriver())
			switch {
			case driverErr != nil:
				// The stored credential_driver could not be parsed, so we cannot assert the
				// gateway was Vault-backed. Record a durable orphan for operator reclaim and
				// let the delete proceed rather than pinning the tombstone on a value we do
				// not understand.
				slog.WarnContext(ctx, "Gateway cannot determine credential driver during deletion; skipping Vault mount reclaim", "gateway_id", event.ResourceID, "error", driverErr)
				opts.RecordOrphan(ctx, "VaultTenantMount", namespace,
					fmt.Sprintf("stored credential_driver could not be parsed (%v); if this gateway was Vault-backed, mount %s may remain", driverErr, gateway.VaultMountForNamespace(namespace)))
			case vaultBacked:
				if err := gateway.DeleteVaultTenantMount(ctx, r.clientset, opts, namespace); err != nil {
					// Orphan-only, like the other permanent-failure cleanups in this function
					// (an unresolvable stored identity above): a failed mount delete (Vault
					// unreachable or decommissioned, provisioner login denied) must NOT pin the
					// delete tombstone forever. Record a durable orphan for operator reclaim and
					// complete the deletion; the gateway database is already dropped, so a
					// left-behind mount holds only unreferenced, un-listable entries.
					slog.WarnContext(ctx, "Gateway per-tenant Vault mount could not be deleted; recording orphan and completing deletion",
						"gateway_id", event.ResourceID,
						"mount", gateway.VaultMountForNamespace(namespace),
						"error", err)
					opts.RecordOrphan(ctx, "VaultTenantMount", namespace,
						fmt.Sprintf("per-tenant Vault mount %s could not be deleted during gateway deletion and may remain: %v", gateway.VaultMountForNamespace(namespace), err))
				} else {
					slog.InfoContext(ctx, "Deleted vault tenant mount", "namespace", namespace)
				}
			}

			// Namespace deletion and database deletion are independent cleanup
			// operations. Attempt both and aggregate failures so one partial failure
			// cannot silently leak the other resource.
			deleted, err := gateway.DeleteManagedNamespace(ctx, r.clientset, namespace, r.controlPlaneNamespace)
			if err != nil {
				deleteErrs = append(deleteErrs, fmt.Errorf("delete gateway namespace %s: %w", namespace, err))
			} else if !deleted {
				if err := gateway.DeleteLabeledNamespaceResources(ctx, r.dynamicClient, namespace, opts); err != nil {
					deleteErrs = append(deleteErrs, fmt.Errorf("sweep labeled resources in %s: %w", namespace, err))
				}
			}
			// A managed-mode gateway created cluster-scoped per-workspace
			// namespaces the Helm uninstall and namespace GC never reap; sweep them
			// here so a deleted gateway does not leak them (openshell-gateway.spec.md,
			// "Gateway Workspace Mode").
			if gw.GetWorkspaceMode() == "managed" {
				if err := gateway.DeleteManagedWorkspaceNamespaces(ctx, r.clientset, namespace); err != nil {
					deleteErrs = append(deleteErrs, fmt.Errorf("delete managed workspace namespaces of %s: %w", namespace, err))
				}
			}
			// An operator-mode gateway's tenant namespaces are created by the external
			// operator OUTSIDE the gateway's own namespace, so neither the namespace
			// cascade nor the namespace GC reaches them; sweep them here, selected by
			// the configured label key set to this gateway's namespace (OSG-OPWS-06).
			// Skip when no operator label is configured (nothing to select on). A
			// failed sweep is retried AND recorded as a durable orphan, since these
			// out-of-namespace namespaces have no other recovery path (GTW-DEL-07).
			if gw.GetWorkspaceMode() == "operator" && r.operatorNamespaceLabelKey != "" {
				if err := gateway.DeleteOperatorWorkspaceNamespaces(ctx, r.clientset, r.operatorNamespaceLabelKey, namespace); err != nil {
					deleteErrs = append(deleteErrs, fmt.Errorf("delete operator workspace namespaces of %s: %w", namespace, err))
					opts.RecordOrphan(ctx, "Namespace", fmt.Sprintf("%s=%s", r.operatorNamespaceLabelKey, namespace),
						fmt.Sprintf("operator tenant workspace namespaces could not be deleted during gateway deletion and may remain: %v", err))
				}
			}
		}

		reconcileErr = errors.Join(deleteErrs...)
		return reconcileErr
	}

	slog.InfoContext(ctx, "Reconciling Gateway",
		"gateway_id", event.ResourceID,
		"gateway_name", gw.Name,
		"namespace", gw.Namespace,
		"event_type", event.Type)

	// The convergence gate prevents redundant re-provisioning: skip re-applying
	// manifests only when the Gateway is converged (observed_generation ==
	// generation). A desired-spec change advances generation past
	// observed_generation, so the change falls through this gate regardless of
	// phase, letting a Degraded gateway re-provision after a spec fix. Health
	// phase/status updates flow through a separate path that this gate does not
	// suppress. See openshell-gateway-health.spec.md.
	//
	// Keycloak client attributes are external desired state, however, and need a
	// lightweight drift reconciliation even when Kubernetes reprovisioning is
	// gated. In particular, controller startup seeds existing converged gateways;
	// reconciling before the return below lets newly introduced client settings
	// converge without forcing a full gateway rollout.
	if gw.ObservedGeneration != nil && *gw.ObservedGeneration == gw.Generation {
		if err := r.reconcileExistingGatewayKeycloakClient(ctx, event.ResourceID, gw); err != nil {
			var identityErr *gatewayKeycloakClientIdentityError
			if errors.As(err, &identityErr) {
				// Invalid persisted identity is terminal desired-state validation, not a
				// transient Keycloak outage. Publish a fixed marker once and stop retrying;
				// retry only when the status write itself fails.
				if gw.GetStatus() == gatewayKeycloakClientInvalidStatus {
					outcome = cpotel.OutcomeFailed
					reason = cpotel.ReasonIdentityInvalid
					return nil
				}
				if statusErr := r.updateGatewayStatus(ctx, event.ResourceID, gatewayKeycloakClientInvalidStatus); statusErr != nil {
					outcome = cpotel.OutcomeRetryable
					reason = cpotel.ReasonGRPCUnavailable
					reconcileErr = errors.Join(
						fmt.Errorf("validate existing Keycloak client identity: %w", err),
						fmt.Errorf("publish invalid Keycloak client configuration status: %w", statusErr),
					)
					return watcher.WithRetryableReason(watcher.PreservePayloadForRetry(reconcileErr), reason)
				}
				outcome = cpotel.OutcomeFailed
				reason = cpotel.ReasonIdentityInvalid
				return nil
			}
			if errors.Is(err, errGatewayKeycloakClientMissing) && gw.GetStatus() != gatewayKeycloakClientMissingStatus {
				if statusErr := r.updateGatewayStatus(ctx, event.ResourceID, gatewayKeycloakClientMissingStatus); statusErr != nil {
					err = errors.Join(err, fmt.Errorf("publish missing Keycloak client status: %w", statusErr))
				}
			}
			// This work is intentionally narrower than gateway provisioning. Keep the
			// gated payload on retry so the queue does not clear phase and expand a
			// transient Keycloak failure into a full Kubernetes reconciliation. The
			// fixed status write above generates another watch event, but the queue's
			// per-key backoff floor prevents that self-event from creating a hot loop.
			outcome = cpotel.OutcomeRetryable
			reason = cpotel.ReasonKeycloakTransient
			if errors.Is(err, errGatewayKeycloakClientMissing) {
				reason = cpotel.ReasonDependencyNotReady
			}
			reconcileErr = fmt.Errorf("reconcile Keycloak client for gateway %q: %w", gw.Name, err)
			return watcher.WithRetryableReason(watcher.PreservePayloadForRetry(reconcileErr), reason)
		}
		if isGatewayKeycloakClientStatus(gw.GetStatus()) {
			if err := r.updateGatewayStatus(ctx, event.ResourceID, ""); err != nil {
				outcome = cpotel.OutcomeRetryable
				reason = cpotel.ReasonGRPCUnavailable
				reconcileErr = fmt.Errorf("clear Keycloak client status for gateway %q: %w", gw.Name, err)
				return watcher.WithRetryableReason(watcher.PreservePayloadForRetry(reconcileErr), reason)
			}
		}
		needsTrustUpdate, err := gateway.DestinationCATrustNeedsUpdate(ctx, r.clientset, r.controlPlaneNamespace, gw.GetNamespace())
		if err != nil {
			outcome = cpotel.OutcomeRetryable
			reason = cpotel.ReasonDependencyNotReady
			reconcileErr = fmt.Errorf("check destination TLS trust: %w", err)
			return watcher.WithRetryableReason(watcher.PreservePayloadForRetry(reconcileErr), reason)
		}
		if !needsTrustUpdate {
			slog.DebugContext(ctx, "Gateway converged, skipping reconciliation", "gateway_id", event.ResourceID, "generation", gw.Generation)
			outcome = cpotel.OutcomeNoop
			return nil
		}
	}

	gatewayClientID, err := gatewayKeycloakClientID(event.ResourceID, gw)
	if err != nil {
		outcome = cpotel.OutcomeFailed
		reason = cpotel.ReasonIdentityInvalid
		reconcileErr = fmt.Errorf("resolve Keycloak client for gateway %q: %w", gw.Name, err)
		return reconcileErr
	}

	// Controller-generated configuration is part of desired state, so writing
	// it advances generation. Persist it before capturing the snapshot this
	// pass applies, then build everything from that snapshot: the generation
	// acknowledged below is then exactly the one whose configuration was
	// deployed. See data-model.spec.md § Gateway Generation Tracking.
	snapshot, err := r.persistGeneratedOIDC(ctx, event.ResourceID, gw, gatewayClientID)
	if err != nil {
		outcome = cpotel.OutcomeRetryable
		reason = cpotel.ReasonGRPCUnavailable
		reconcileErr = fmt.Errorf("reconcile gateway %s: %w", gw.Name, err)
		return reconcileErr
	}
	gw = snapshot

	namespace, err := gatewayNamespace(gw)
	if err != nil {
		outcome = cpotel.OutcomeFailed
		reason = cpotel.ReasonInvalidConfig
		reconcileErr = fmt.Errorf("reconcile gateway %s: %w", gw.Name, err)
		return reconcileErr
	}

	dnsNames := gw.ServerDnsNames
	if len(dnsNames) == 0 {
		dnsNames = []string{
			fmt.Sprintf("openshell-gateway.%s.svc.cluster.local", namespace),
		}
		if gw.ExternalDns != nil && *gw.ExternalDns != "" {
			dnsNames = append(dnsNames, *gw.ExternalDns)
		}
	}

	externalDns := ""
	if gw.ExternalDns != nil {
		externalDns = *gw.ExternalDns
	}

	gwConfig := gateway.GatewayConfig{
		ServerDnsNames: dnsNames,
		ExternalDns:    externalDns,
		WorkspaceMode:  gw.GetWorkspaceMode(),
	}

	// Select the direct image or platform default before Helm values are built.
	image, err := r.selectGatewayImage(gw)
	if err != nil {
		reconcileErr = fmt.Errorf("select image for gateway %s: %w", gw.Name, err)
		return reconcileErr
	}
	gwConfig.Image = image

	supervisorImage := selectSupervisorImage(gw)
	if supervisorImage == "" {
		reconcileErr = fmt.Errorf("supervisor image is not configured for gateway %s; set GATEWAY_SUPERVISOR_IMAGE or specify a supervisor_image", gw.Name)
		return reconcileErr
	}
	gwConfig.SupervisorImage = supervisorImage
	gwConfig.SandboxRuntimeImage = (gateway.StaticImageDefaults{}).DefaultSandboxRuntimeImage()

	if gw.Oidc != nil && *gw.Oidc != "" {
		var oidcConfig gateway.OIDCConfig
		if err := json.Unmarshal([]byte(*gw.Oidc), &oidcConfig); err != nil {
			reconcileErr = fmt.Errorf("invalid oidc config for gateway %s: %w", gw.Name, err)
			return reconcileErr
		}
		gwConfig.OIDC = oidcConfig
	}

	// Routing is platform-managed and enabled by default (PLAT-DATA-23). A user
	// or GitOps import never sets route, so gw.Route is nil for every such
	// gateway; the control plane routes it through the Gateway API
	// and derives its ingress hostname. An explicit route the control plane
	// itself authored still wins.
	routeConfig, err := effectiveRouteConfig(gw)
	if err != nil {
		reconcileErr = fmt.Errorf("invalid route config for gateway %s: %w", gw.Name, err)
		return reconcileErr
	}
	gwConfig.Route = routeConfig

	if gw.CredentialDriver != nil && *gw.CredentialDriver != "" {
		var credDriverConfig gateway.CredentialDriverConfig
		decoder := json.NewDecoder(bytes.NewReader([]byte(*gw.CredentialDriver)))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&credDriverConfig); err != nil {
			reconcileErr = fmt.Errorf("invalid credential driver config for gateway %s: %w", gw.Name, err)
			return reconcileErr
		}
		gwConfig.CredentialDriver = &credDriverConfig
	}

	nsConfig := gateway.NamespaceConfig{
		Name:    namespace,
		Gateway: gwConfig,
	}

	opts := gateway.ReconcileOpts{
		IsOpenShift:                 r.isOpenShift,
		HasCertManager:              r.hasCertManager,
		Database:                    r.database,
		ControlPlaneNamespace:       r.controlPlaneNamespace,
		GatewayID:                   event.ResourceID,
		UpdateRouteAddress:          r.makeRouteAddressUpdater(event.ResourceID),
		UpdateConsoleAddress:        r.makeConsoleAddressUpdater(event.ResourceID),
		Keycloak:                    r.keycloakConfig,
		KeycloakClient:              r.keycloakClient,
		GatewayName:                 gw.Name,
		GatewayClientID:             gatewayClientID,
		Exposure:                    r.exposure,
		RouteStillDesired:           r.makeRouteStillDesired(event.ResourceID),
		IngressBaseDomain:           r.ingressBaseDomain,
		GatewayResources:            r.gatewayResources,
		SandboxRuntimeClass:         r.sandboxRuntimeClass,
		OperatorWorkspace:           r.operatorWorkspace,
		OperatorNamespaceLabelKey:   r.operatorNamespaceLabelKey,
		OperatorSandboxRuntimeClass: r.operatorSandboxRuntimeClass,
	}

	conditions := gateway.InitConditions()
	r.updateProvisioningConditions(ctx, event.ResourceID, conditions)

	opts.ReportProgress = func(step, status, message string) {
		gateway.SetCondition(conditions, step, status, message)
		r.updateProvisioningConditions(ctx, event.ResourceID, conditions)
	}

	r.updateGatewayPhase(ctx, event.ResourceID, string(gatewayhealth.PhaseProvisioning))

	if err := gateway.ReconcileGateway(ctx, r.dynamicClient, r.clientset, r.helmClient, nsConfig, opts); err != nil {
		var renderErr *gateway.ConfigValidationError
		if errors.As(err, &renderErr) {
			failReason := fmt.Sprintf("generated configuration validation failed: %v", renderErr.Err)
			if failedGateway := r.updateGatewayHealth(ctx, event.ResourceID, string(gatewayhealth.PhaseFailed), failReason); failedGateway != nil {
				observeGatewayProvisionFailure(ctx, event.ResourceID)
			}
			slog.ErrorContext(ctx, "Gateway generated configuration invalid", "gateway_name", gw.Name, "namespace", namespace, "error", renderErr.Err)
			outcome = cpotel.OutcomeFailed
			reason = cpotel.ReasonInvalidConfig
		} else if r.updateGatewayPhase(ctx, event.ResourceID, string(gatewayhealth.PhaseFailed)) {
			observeGatewayProvisionFailure(ctx, event.ResourceID)
			outcome = cpotel.OutcomeFailed
			reason = cpotel.ReasonUnknown
		}
		reconcileErr = fmt.Errorf("reconcile gateway %s: %w", gw.Name, err)
		return reconcileErr
	}

	// Step 5: GatewayHealthy
	gateway.SetCondition(conditions, gateway.ConditionGatewayHealthy, gateway.StatusInProgress, "")
	r.updateProvisioningConditions(ctx, event.ResourceID, conditions)

	// routeAddress carries the address down to the route-ready poll below. See
	// routeAddressForPoll for how a resolve/save failure this pass falls back
	// to the Gateway's currently-persisted routeAddress (OSG-ROUT-07.01).
	var routeAddress string
	routeHost := ""
	if gw.Route != nil {
		var rc struct{ Host string }
		_ = json.Unmarshal([]byte(*gw.Route), &rc)
		routeHost = rc.Host
	}
	var resolvedAndSaved string
	addr, err := r.exposure.ResolveAddress(ctx, exposure.Request{
		Namespace: namespace,
		Host:      routeHost,
	})
	if err != nil {
		slog.WarnContext(ctx, "Gateway failed to resolve route address", "gateway_name", gw.Name, "error", err)
	} else if addr != "" {
		if err := r.updateRouteAddress(ctx, event.ResourceID, addr); err != nil {
			slog.WarnContext(ctx, "Gateway failed to publish route address", "gateway_name", gw.Name, "error", err)
		} else {
			resolvedAndSaved = addr
			slog.InfoContext(ctx, "Gateway published route address", "gateway_name", gw.Name, "addr", addr)
		}
	}
	// Fall back to a fresh read of the persisted routeAddress, not gw's own
	// field: gw is the watch-event snapshot captured when this pass started,
	// and the Helm apply above (gateway.ReconcileGateway) can itself run long
	// enough for the continuous health reconciler to concurrently clear or
	// republish the persisted address (for example, tearing down the route if
	// it was disabled mid-pass). Using the stale snapshot here could resurrect
	// an address the health reconciler already retracted. This re-read is
	// taken once, before the Deployment-readiness wait and route-readiness
	// poll below, which can together add minutes more; it narrows the
	// staleness window around the Helm apply but does not eliminate
	// staleness introduced during those later waits.
	routeAddress = routeAddressForPoll(r.currentRouteAddress(ctx, event.ResourceID, gw), resolvedAndSaved)

	// Manifests applied successfully: acknowledge the generation we converged on.
	// gw is the snapshot the manifests were built from (persistGeneratedOIDC), so
	// this is the generation whose complete desired configuration was applied; a
	// newer generation that arrived mid-pass stays pending for its own pass.
	// Readiness/health below only affects phase, not convergence. On failure
	// above we returned without writing, so the change is retried. The gateway is
	// now converged, so the convergence gate suppresses further re-provisioning
	// passes; the continuous health reconciler owns promoting the phase to Running
	// and finalizing the GatewayHealthy provisioning condition to Complete once the
	// workload (and, for a routed gateway, its route) is observed ready.
	r.updateObservedGeneration(ctx, event.ResourceID, gw.Generation)

	// Manifests are applied, but the gateway is not Running until its workload is
	// observed Ready. Wait within the provisioning readiness window; if the
	// Deployment never becomes ready, set Degraded and record why.
	ready, reason := gateway.WaitForGatewayReady(ctx, r.clientset, namespace, 2*time.Minute)
	if !ready {
		gateway.SetCondition(conditions, gateway.ConditionGatewayHealthy, gateway.StatusFailed, "Gateway health check timed out - the gateway workload is not yet ready")
		r.updateProvisioningConditions(ctx, event.ResourceID, conditions)
		r.updateGatewayHealth(ctx, event.ResourceID, string(gatewayhealth.PhaseDegraded), reason)
		slog.WarnContext(ctx, "Gateway applied but not ready", "gateway_name", gw.Name, "namespace", namespace, "reason", reason)
		return nil
	}

	// The Deployment is Ready. The gateway is not Running until its external
	// exposure is also observed Ready. Poll the exposure here within a bounded
	// window so the gateway is promoted to Running promptly once its route is
	// programmed - rather than waiting up to a full health-reconciler tick, which
	// would leave the connection command and console button hidden for seconds
	// after the pods are ready. If the window elapses, park at Provisioning and
	// let the continuous health reconciler keep enforcing the full route-readiness
	// grace window (promoting to Running, or Degraded once it expires). See
	// openshell-gateway-health.spec.md § Phase Reflects Workload and Route Readiness.
	if r.waitForRouteReady(ctx, namespace, routeAddress) {
		gateway.SetCondition(conditions, gateway.ConditionGatewayHealthy, gateway.StatusComplete, "")
		r.updateProvisioningConditions(ctx, event.ResourceID, conditions)
		// The observation guard rejects work that started in Running or Degraded.
		if runningGateway := r.updateGatewayHealth(ctx, event.ResourceID, string(gatewayhealth.PhaseRunning), gatewayhealth.StatusHealthy); runningGateway != nil {
			observeGatewayProvisionSuccess(ctx, runningGateway)
		}
		slog.InfoContext(ctx, "Gateway provisioned and route ready", "gateway_name", gw.Name, "namespace", namespace)
	} else {
		// Route gate not yet passed: hold at Provisioning. The continuous health
		// reconciler promotes to Running once the route is ready.
		r.updateGatewayHealth(ctx, event.ResourceID, string(gatewayhealth.PhaseProvisioning), "Deployment ready; awaiting route readiness")
		slog.InfoContext(ctx, "Gateway deployment ready; awaiting route readiness", "gateway_name", gw.Name, "namespace", namespace)
	}

	// The console can start after the gateway is ready. Poll the console in the
	// background so the address appears without waiting for the next health tick.
	// The health reconciler continues to publish and retract the address.
	go r.publishConsoleAddressWhenReady(ctx, event.ResourceID, gw)
	return nil
}

const (
	gatewayKeycloakClientMissingStatus = "Keycloak client is missing"
	gatewayKeycloakClientInvalidStatus = "Keycloak client configuration is invalid"
)

var errGatewayKeycloakClientMissing = errors.New("gateway Keycloak client is missing")

type gatewayKeycloakClientIdentityError struct {
	err error
}

func (e *gatewayKeycloakClientIdentityError) Error() string { return e.err.Error() }
func (e *gatewayKeycloakClientIdentityError) Unwrap() error { return e.err }

func invalidGatewayKeycloakClientIdentity(format string, args ...any) error {
	return &gatewayKeycloakClientIdentityError{err: fmt.Errorf(format, args...)}
}

func isGatewayKeycloakClientStatus(status string) bool {
	return status == gatewayKeycloakClientMissingStatus || status == gatewayKeycloakClientInvalidStatus
}

// reconcileExistingGatewayKeycloakClient reconciles the desired Keycloak client
// without reapplying the gateway's Kubernetes resources. External client
// attributes can drift or be introduced in newer controller releases. A missing
// client is reported as non-converged rather than silently skipped or partially
// recreated without restoring its user role assignments.
func (r *GatewayReconciler) reconcileExistingGatewayKeycloakClient(ctx context.Context, gatewayID string, gw *pb.Gateway) error {
	clientID, err := gatewayKeycloakClientID(gatewayID, gw)
	if err != nil {
		return err
	}

	clientUUID, err := r.keycloakClient.GetClientUUID(ctx, clientID)
	if err != nil {
		return fmt.Errorf("check existing Keycloak client %q: %w", clientID, err)
	}
	if clientUUID == "" {
		return fmt.Errorf("desired Keycloak client %q is missing: %w", clientID, errGatewayKeycloakClientMissing)
	}
	if err := r.keycloakClient.EnsureDeviceAuthorizationGrant(ctx, clientUUID); err != nil {
		return fmt.Errorf("reconcile device authorization grant on Keycloak client %q: %w", clientID, err)
	}
	if err := r.keycloakClient.EnsureE2ETokenExchange(ctx, clientUUID); err != nil {
		return fmt.Errorf("reconcile e2e token-exchange on Keycloak client %q: %w", clientID, err)
	}
	slog.InfoContext(ctx, "Reconciled Keycloak client", "client_id", clientID, "uuid", clientUUID)
	return nil
}

// gatewayKeycloakClientID returns the gateway's Keycloak client ID. The
// provisioner stores the client ID in both client_id and audience, and every
// path reads that stored value, so a rename never moves the gateway to another
// client. Persisted values cross an API trust boundary, so they are accepted only
// when both are present, they agree, they contain no control characters, and they
// are gateway-owned: either the gateway ID itself (the initial format) or a
// non-empty prefix followed by "-<gatewayID>" (the earlier {name}-{id} format,
// still used by gateways provisioned before the switch). Gateways without either
// persisted value, such as a new gateway before its client is provisioned, use
// the initial client ID: the immutable gateway ID.
func gatewayKeycloakClientID(gatewayID string, gw *pb.Gateway) (string, error) {
	if gatewayID == "" {
		return "", invalidGatewayKeycloakClientIdentity("gateway ID is required for Keycloak reconciliation")
	}
	if containsControlCharacter(gatewayID) {
		return "", invalidGatewayKeycloakClientIdentity("gateway ID contains control characters")
	}
	if gw == nil {
		return "", invalidGatewayKeycloakClientIdentity("gateway is required for Keycloak reconciliation")
	}

	var clientID, audience string
	if gw.GetOidc() != "" {
		var oidc gateway.OIDCConfig
		if err := json.Unmarshal([]byte(gw.GetOidc()), &oidc); err != nil {
			return "", invalidGatewayKeycloakClientIdentity("parse persisted OIDC config: %w", err)
		}
		clientID, audience = oidc.ClientID, oidc.Audience
	}

	for _, persisted := range []string{clientID, audience} {
		if persisted != "" && containsControlCharacter(persisted) {
			return "", invalidGatewayKeycloakClientIdentity("persisted OIDC client identity contains control characters")
		}
	}
	if (clientID == "") != (audience == "") {
		return "", invalidGatewayKeycloakClientIdentity("persisted OIDC client_id and audience must both be set")
	}
	if clientID != audience {
		return "", invalidGatewayKeycloakClientIdentity("persisted OIDC client_id and audience do not match")
	}

	if clientID != "" {
		if !isGatewayOwnedKeycloakClientID(clientID, gatewayID) {
			return "", invalidGatewayKeycloakClientIdentity("persisted OIDC client identity is not owned by the gateway")
		}
		return clientID, nil
	}
	return gatewayID, nil
}

func isGatewayOwnedKeycloakClientID(clientID, gatewayID string) bool {
	if clientID == "" || containsControlCharacter(clientID) {
		return false
	}
	if clientID == gatewayID {
		return true
	}
	// Transitional (OSG-KCLT-01, openshell-gateway-keycloak.transitional.spec.md):
	// the earlier {name}-{id} format. Remove this branch with that spec.
	suffix := "-" + gatewayID
	return strings.HasSuffix(clientID, suffix) && len(clientID) > len(suffix)
}

func containsControlCharacter(value string) bool {
	for _, ch := range value {
		if unicode.IsControl(ch) {
			return true
		}
	}
	return false
}

// provisioningRouteReadyWait bounds how long the provisioning path polls a
// routed gateway's external exposure for readiness before parking it at
// Provisioning. Route programming typically completes within a few seconds of
// Deployment readiness; polling here (rather than waiting for the health loop's
// next tick) lets the connection command and console surface promptly. On
// timeout the health reconciler continues enforcing the full route-readiness
// grace window, so a slow route is not misreported.
const provisioningRouteReadyWait = 90 * time.Second

// provisioningRouteReadyInterval is the cadence at which the provisioning path
// polls a routed gateway's exposure. It is intentionally far tighter than the
// steady-state health interval (30s) so the first Running promotion is prompt;
// the 30s cadence still governs ongoing health once the gateway is settled.
const provisioningRouteReadyInterval = 2 * time.Second

// provisioningConsoleReadyWait bounds how long the provisioning path polls a
// routed gateway's console Deployment for readiness before leaving further
// publication to the health reconciler. It is generous because the console
// images may need pulling on a cold cluster; the poll runs in the background,
// so a long window never blocks the gateway watch loop.
const provisioningConsoleReadyWait = 5 * time.Minute

// provisioningConsoleReadyInterval is the cadence at which the provisioning path
// polls the console Deployment's readiness, tight enough that the console button
// enables within a couple of seconds of the pod becoming ready.
const provisioningConsoleReadyInterval = 2 * time.Second

// waitForRouteReady polls the gateway's external exposure until it reports Ready
// or the bounded provisioning window elapses, returning whether it became Ready.
// routeAddress is the Gateway's routeAddress: the address this provisioning
// pass itself resolved and published when it could, falling back to the
// already-persisted value (gw.GetRouteAddress()) when this pass's own resolve
// or save attempt did not succeed (OSG-ROUT-07.01). The exposure port treats
// an empty value as not-Ready, so a gateway with no published address at all
// never reports Ready here even if its Gateway API resources are otherwise
// healthy.
func (r *GatewayReconciler) waitForRouteReady(ctx context.Context, namespace, routeAddress string) bool {
	return r.pollRouteReady(ctx, namespace, routeAddress, provisioningRouteReadyInterval, provisioningRouteReadyWait)
}

// routeAddressForPoll returns the routeAddress this provisioning pass should
// forward to the route-ready poll. It prefers resolvedAndSaved, the address
// this pass itself resolved and successfully persisted this pass; when the
// pass's own resolve attempt errored or returned empty, or its save failed,
// resolvedAndSaved is empty, and the gateway's already-persisted routeAddress
// (persisted, i.e. gw.GetRouteAddress()) is used instead. Without this
// fallback, a pass that cannot re-resolve the address would make an
// already-published, reachable gateway look unpublished for this pass's poll
// (OSG-ROUT-07.01).
func routeAddressForPoll(persisted, resolvedAndSaved string) string {
	if resolvedAndSaved != "" {
		return resolvedAndSaved
	}
	return persisted
}

// pollRouteReady observes the exposure immediately and then every interval until
// it reports Ready or the window elapses, mirroring WaitForGatewayReady so a
// route that is already (or quickly) programmed promotes without waiting a full
// interval. A transient observation error is logged and retried, never treated
// as not-ready-forever. Interval and window are parameters so tests can drive it
// without real-time waits.
func (r *GatewayReconciler) pollRouteReady(ctx context.Context, namespace, routeAddress string, interval, window time.Duration) bool {
	return poll(ctx, interval, window, func() bool {
		rr, err := r.exposure.ObserveReadiness(ctx, exposure.Request{Namespace: namespace, RouteAddress: routeAddress})
		if err != nil {
			slog.WarnContext(ctx, "Gateway route readiness check failed", "namespace", namespace, "error", err)
			return false
		}
		return rr.Ready
	})
}

// publishConsoleAddressWhenReady polls the gateway's console Deployment on a
// tight cadence and publishes console_address as soon as the console pod can
// serve, so the web UI's console button enables promptly rather than waiting for
// the next health-reconciler tick. It is meant to run in the background and
// stops once the address is published or the bounded window elapses.
//
// It runs on the long-lived watch context, which route removal does not cancel,
// so it must not trust the routed Gateway snapshot captured when provisioning
// started. If the route is removed while the console image is still pulling, the
// health reconciler's teardown owns clearing console_address; a publisher acting
// on the stale snapshot would otherwise re-publish the console link after
// teardown, stranding a trusted address for a gateway that is no longer routed.
// Re-read the current Gateway each poll and stop the moment it is no longer
// routed (or has been deleted), leaving the address to teardown.
func (r *GatewayReconciler) publishConsoleAddressWhenReady(ctx context.Context, gatewayID string, gw *pb.Gateway) {
	client := pb.NewGatewayServiceClient(r.grpcConn)
	poll(ctx, provisioningConsoleReadyInterval, provisioningConsoleReadyWait, func() bool {
		resp, err := client.GetGateway(ctx, &pb.GetGatewayRequest{Id: gatewayID})
		if err != nil {
			if status.Code(err) == codes.NotFound {
				// The gateway was deleted while the console image pulled: there is
				// nothing left to publish against. Stop polling rather than retrying
				// a NotFound until the window elapses.
				return true
			}
			slog.WarnContext(ctx, "Console publisher: get gateway failed", "gateway_id", gatewayID, "error", err)
			return false
		}
		current := resp.GetGateway()
		if current == nil {
			return true
		}
		return syncConsoleAddress(ctx, r.clientset, r.dynamicClient, client, gatewayID, current)
	})
}

// makeRouteStillDesired returns a callback the provisioning path invokes after
// the (up-to-60s) server-TLS wait, before it creates the remaining route- and
// console-owned resources, to confirm the gateway is still routed according to
// its live API-server record. A route removal (or gateway deletion) during that
// wait is observed only by the independent health loop -- the watcher phase gate
// blocks a re-provision -- which tears the gateway down and clears its stored
// addresses; without this re-check the in-flight pass would recreate the
// BackendTLSPolicy, backend-CA ConfigMap, console, and
// Keycloak client behind that teardown, and the health loop's torn-down cache
// (keyed on empty addresses) would then hide the orphans indefinitely. Returns
// false on NotFound (the gateway is gone, so nothing is desired) and propagates
// transient errors so the caller can decide (it proceeds conservatively).
func (r *GatewayReconciler) makeRouteStillDesired(gatewayID string) func(context.Context) (bool, error) {
	return func(ctx context.Context) (bool, error) {
		client := pb.NewGatewayServiceClient(r.grpcConn)
		resp, err := client.GetGateway(ctx, &pb.GetGatewayRequest{Id: gatewayID})
		if err != nil {
			if status.Code(err) == codes.NotFound {
				return false, nil
			}
			return false, err
		}
		return resp.GetGateway() != nil, nil
	}
}

// poll invokes attempt immediately and then every interval until it returns
// true or the window elapses (or the context is cancelled), reporting whether
// attempt ever succeeded. Interval and window are parameters so tests can drive
// it without real-time waits.
func poll(ctx context.Context, interval, window time.Duration, attempt func() bool) bool {
	deadline := time.After(window)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		if attempt() {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-deadline:
			return false
		case <-ticker.C:
		}
	}
}

// effectiveRouteConfig resolves the route config the control plane provisions
// for a gateway. Routing is platform-managed (PLAT-DATA-23) and every gateway is
// routed: a user or GitOps import never sets route, so an absent, empty or null
// route resolves to the zero config and the control plane derives the ingress
// hostname. An explicit route the control plane authored (host, dedicated
// certificate) still wins.
func effectiveRouteConfig(gw *pb.Gateway) (gateway.RouteConfig, error) {
	if gw.Route == nil {
		return gateway.RouteConfig{}, nil
	}
	trimmed := strings.TrimSpace(*gw.Route)
	if trimmed == "" || trimmed == "null" {
		return gateway.RouteConfig{}, nil
	}
	var config gateway.RouteConfig
	if err := json.Unmarshal([]byte(trimmed), &config); err != nil {
		return gateway.RouteConfig{}, err
	}
	return config, nil
}

// gatewayNamespace returns the Kubernetes namespace a Gateway is deployed into.
// The namespace is assigned deterministically at creation (the API server's
// Gateway.BeforeCreate sets `openshell-<hex(ksuid)>`) and is carried on every
// event, so any Gateway that reaches a reconciler has one. It returns an error
// rather than synthesizing a name from gw.Name: a guessed namespace would
// diverge from the real `openshell-<hex(ksuid)>` scheme and, on the delete
// path, could hand a wrong (possibly live) namespace to the destructive
// DeleteManagedNamespace.
func gatewayNamespace(gw *pb.Gateway) (string, error) {
	ns := gw.GetNamespace()
	if ns == "" {
		return "", fmt.Errorf("gateway %s has no namespace", gw.GetMetadata().GetId())
	}
	return ns, nil
}

// gatewayListPageSize is the page size the reconcilers use when paging through
// the full gateway inventory over gRPC. It matches the API server's maximum
// page size so the common (small-fleet) case completes in a single request.
const gatewayListPageSize = 500

// listAllGateways pages through the gRPC gateway inventory of this control
// plane's registered cluster and returns every gateway in it. The list endpoint
// is server-side paginated (default page size 20), so callers that must reason
// about the whole set (the namespace reaper and the health reconciler) cannot
// rely on a single unpaged request. The cluster filter is mandatory: an empty
// clusterID fails with watcher.ErrMissingClusterID, and the api-server rejects an
// unfiltered list from a registered control plane.
func listAllGateways(ctx context.Context, client pb.GatewayServiceClient, clusterID string) ([]*pb.Gateway, error) {
	clusterFilter, err := watcher.ClusterFilter(clusterID)
	if err != nil {
		return nil, err
	}
	var all []*pb.Gateway
	for page := int32(1); ; page++ {
		resp, err := client.ListGateways(ctx, &pb.ListGatewaysRequest{
			Page:      page,
			Size:      gatewayListPageSize,
			ClusterId: clusterFilter,
		})
		if err != nil {
			return nil, err
		}
		items := resp.GetItems()
		all = append(all, items...)

		// Stop once we've collected the whole set (authoritative Total), or the
		// server returns a short/empty page. The latter two are defensive so a
		// misreported Total can never spin this loop forever.
		total := int(resp.GetMetadata().GetTotal())
		if len(items) == 0 || len(items) < gatewayListPageSize || (total > 0 && len(all) >= total) {
			return all, nil
		}
	}
}

// updateGatewayHealth sets the Gateway `phase` and `status` together in a single
// gRPC update so the console and CLI observe a consistent lifecycle state and
// health descriptor. It returns the stored Gateway on success so callers can
// use the API server timestamps. It returns nil if the update fails.
func (r *GatewayReconciler) updateGatewayHealth(ctx context.Context, gatewayID, phase, status string) *pb.Gateway {
	client := pb.NewGatewayServiceClient(r.grpcConn)
	response, err := client.UpdateGateway(ctx, &pb.UpdateGatewayRequest{
		Id:     gatewayID,
		Phase:  &phase,
		Status: &status,
	})
	if err != nil {
		slog.WarnContext(ctx, "Failed to update gateway health", "gateway_id", gatewayID, "phase", phase, "status", status, "error", err)
		return nil
	}
	return response.GetGateway()
}

func (r *GatewayReconciler) updateGatewayPhase(ctx context.Context, gatewayID string, phase string) bool {
	client := pb.NewGatewayServiceClient(r.grpcConn)
	_, err := client.UpdateGateway(ctx, &pb.UpdateGatewayRequest{
		Id:    gatewayID,
		Phase: &phase,
	})
	if err != nil {
		slog.WarnContext(ctx, "Failed to update gateway phase", "gateway_id", gatewayID, "phase", phase, "error", err)
		return false
	}
	return true
}

func conditionStatusToProto(s string) pb.ProvisioningConditionStatus {
	switch s {
	case gateway.StatusPending:
		return pb.ProvisioningConditionStatus_PROVISIONING_CONDITION_STATUS_PENDING
	case gateway.StatusInProgress:
		return pb.ProvisioningConditionStatus_PROVISIONING_CONDITION_STATUS_IN_PROGRESS
	case gateway.StatusComplete:
		return pb.ProvisioningConditionStatus_PROVISIONING_CONDITION_STATUS_COMPLETE
	case gateway.StatusFailed:
		return pb.ProvisioningConditionStatus_PROVISIONING_CONDITION_STATUS_FAILED
	default:
		return pb.ProvisioningConditionStatus_PROVISIONING_CONDITION_STATUS_UNSPECIFIED
	}
}

func (r *GatewayReconciler) updateProvisioningConditions(ctx context.Context, gatewayID string, conditions []gateway.ProvisioningCondition) {
	client := pb.NewGatewayServiceClient(r.grpcConn)
	pbConditions := make([]*pb.ProvisioningCondition, len(conditions))
	for i, c := range conditions {
		pbConditions[i] = &pb.ProvisioningCondition{
			Type:            c.Type,
			ConditionStatus: conditionStatusToProto(c.ConditionStatus),
			Message:         c.Message,
		}
	}
	_, err := client.UpdateGateway(ctx, &pb.UpdateGatewayRequest{
		Id:                     gatewayID,
		ProvisioningConditions: pbConditions,
	})
	if err != nil {
		slog.WarnContext(ctx, "Failed to update provisioning conditions", "gateway_id", gatewayID, "error", err)
	}
}

// updateGatewayStatus changes status without changing phase. Lightweight
// external-state repair uses it so a missing Keycloak client is visible without
// falsely claiming that the Kubernetes workload phase changed.
func (r *GatewayReconciler) updateGatewayStatus(ctx context.Context, gatewayID, gatewayStatus string) error {
	client := pb.NewGatewayServiceClient(r.grpcConn)
	if _, err := client.UpdateGateway(ctx, &pb.UpdateGatewayRequest{
		Id:     gatewayID,
		Status: &gatewayStatus,
	}); err != nil {
		return fmt.Errorf("update gateway %s status: %w", gatewayID, err)
	}
	return nil
}

// consoleAddressFor returns the console_address a gateway should carry given
// whether its console Deployment is Ready: the console URL when Ready, empty
// otherwise. Publishing empty until the console pod can serve keeps the web UI's
// console button hidden, and retracts it if the pod later goes unready.
func consoleAddressFor(ready bool, url string) string {
	if ready {
		return url
	}
	return ""
}

// syncConsoleAddress publishes the gateway's console_address once its console is
// observed servable and clears it otherwise, so the web UI only offers the
// console button when the console can serve. The console Deployment and the
// console HTTPRoute must both be Ready. It leaves the address unchanged after a
// temporary observation error. It returns whether the console can serve.
func syncConsoleAddress(ctx context.Context, clientset kubernetes.Interface, dynamicClient dynamic.Interface, client pb.GatewayServiceClient, gatewayID string, gw *pb.Gateway) bool {
	if gatewayID == "" {
		return false
	}
	namespace, err := gatewayNamespace(gw)
	if err != nil {
		slog.WarnContext(ctx, "Console address failed", "gateway_id", gatewayID, "error", err)
		return false
	}
	// Gateways opted in to the dedicated per-gateway certificate serve their
	// console from the per-tenant Gateway's own domain
	// (console-<ns>.gw-<ns>.<base-domain>), not the shared base domain, so the
	// published address must be derived with the gateway's parsed route config.
	// An unparseable route is treated as not opted in: the provisioning path
	// reports the invalid value, and publishing the shared-domain address keeps
	// the button consistent with what the console exposure actually serves.
	nsConfig := gateway.NamespaceConfig{
		Name:    namespace,
		Gateway: gateway.GatewayConfig{Route: consoleRouteConfig(gw)},
	}
	url, hasURL := gateway.ConsoleURLFor(nsConfig)
	ready := false
	if hasURL {
		ready, _, err = gateway.DeploymentReadiness(ctx, clientset, namespace, gateway.ConsoleDeploymentName)
		if err != nil {
			slog.WarnContext(ctx, "Console readiness check failed", "namespace", namespace, "error", err)
			return false
		}
	}
	if ready {
		// The selected public exposure must be Ready before the reconciler
		// publishes the address. ConsoleExposureReady also verifies the exposure
		// actually serves the derived console host (and, for a dedicated-route
		// gateway, its per-tenant parent), so a console provisioned onto the
		// shared ingress is reported not servable instead of advertising a URL
		// that presents a mismatched certificate.
		exposureReady, reason, exposureErr := gateway.ConsoleExposureReady(ctx, dynamicClient, nsConfig)
		if exposureErr != nil {
			slog.WarnContext(ctx, "Console exposure readiness check failed", "namespace", namespace, "error", exposureErr)
			return false
		}
		if !exposureReady {
			slog.InfoContext(ctx, "Console not servable yet", "namespace", namespace, "reason", reason)
			ready = false
		}
	}
	desired := consoleAddressFor(ready, url)
	if gw.GetConsoleAddress() == desired {
		return ready
	}
	if _, err := client.UpdateGateway(ctx, &pb.UpdateGatewayRequest{
		Id:             gatewayID,
		ConsoleAddress: &desired,
	}); err != nil {
		slog.WarnContext(ctx, "Failed to set console address", "gateway_id", gatewayID, "address", desired, "error", err)
		return false
	}
	slog.InfoContext(ctx, "Console address set", "gateway_id", gatewayID, "address", desired, "console_ready", ready)
	return ready
}

// consoleRouteConfig resolves the route config every console path keys on:
// the gateway's parsed route document, with the dedicated per-gateway
// certificate opt-in intact. Both the console-address publisher
// (syncConsoleAddress) and the console self-heal (selfHealConsole) must derive
// the console host and parent from this one helper, or the two drift apart and
// the self-heal re-parents a dedicated-route console onto the shared ingress
// while the published address keeps pointing at the dedicated host. An absent
// route resolves to the zero config (matching effectiveRouteConfig); an unparseable route is
// reported by the provisioning path and treated here as not opted in, keeping
// the published address and the self-heal consistent with the exposure the
// shared path actually serves.
func consoleRouteConfig(gw *pb.Gateway) gateway.RouteConfig {
	routeConfig, err := effectiveRouteConfig(gw)
	if err != nil {
		slog.Warn("Console route config invalid", "gateway_id", gw.GetMetadata().GetId(), "error", err)
		return gateway.RouteConfig{}
	}
	return routeConfig
}

// updateObservedGeneration records the generation the control plane has
// successfully applied to the cluster, marking the Gateway converged. It is the
// only writer of observed_generation, via the same gRPC back-channel used for
// phase/status/route_address.
func (r *GatewayReconciler) updateObservedGeneration(ctx context.Context, gatewayID string, generation int64) {
	client := pb.NewGatewayServiceClient(r.grpcConn)
	_, err := client.UpdateGateway(ctx, &pb.UpdateGatewayRequest{
		Id:                 gatewayID,
		ObservedGeneration: &generation,
	})
	if err != nil {
		// Intentional soft failure: the gateway stays unconverged (observed_generation
		// unchanged) and re-applies on the next watch event, so no work is lost.
		slog.WarnContext(ctx, "Failed to update gateway observed_generation", "gateway_id", gatewayID, "generation", generation, "error", err)
	}
}

// makeRouteAddressUpdater returns a RouteAddressUpdater callback that PATCHes
// the route_address field on the API-server Gateway via gRPC.
func (r *GatewayReconciler) makeRouteAddressUpdater(gatewayID string) gateway.RouteAddressUpdater {
	return func(ctx context.Context, routeAddress string) error {
		return r.updateRouteAddress(ctx, gatewayID, routeAddress)
	}
}

func (r *GatewayReconciler) updateRouteAddress(ctx context.Context, gatewayID string, routeAddress string) error {
	client := pb.NewGatewayServiceClient(r.grpcConn)
	_, err := client.UpdateGateway(ctx, &pb.UpdateGatewayRequest{
		Id:           gatewayID,
		RouteAddress: &routeAddress,
	})
	if err != nil {
		return fmt.Errorf("update gateway %s route_address to %s: %w", gatewayID, routeAddress, err)
	}
	return nil
}

// makeConsoleAddressUpdater returns a ConsoleAddressUpdater callback that
// PATCHes the console_address field on the API-server Gateway via gRPC.
func (r *GatewayReconciler) makeConsoleAddressUpdater(gatewayID string) gateway.ConsoleAddressUpdater {
	return func(ctx context.Context, consoleAddress string) error {
		return r.updateConsoleAddress(ctx, gatewayID, consoleAddress)
	}
}

func (r *GatewayReconciler) updateConsoleAddress(ctx context.Context, gatewayID string, consoleAddress string) error {
	client := pb.NewGatewayServiceClient(r.grpcConn)
	_, err := client.UpdateGateway(ctx, &pb.UpdateGatewayRequest{
		Id:             gatewayID,
		ConsoleAddress: &consoleAddress,
	})
	if err != nil {
		return fmt.Errorf("update gateway %s console_address to %s: %w", gatewayID, consoleAddress, err)
	}
	return nil
}

// selectSupervisorImage returns the gateway's explicit supervisor image or the
// platform default from GATEWAY_SUPERVISOR_IMAGE. Returns empty when neither is
// configured so the caller can fail fast instead of deploying a chart default.
func selectSupervisorImage(gw *pb.Gateway) string {
	if gw.SupervisorImage != nil && *gw.SupervisorImage != "" {
		return *gw.SupervisorImage
	}
	return (gateway.StaticImageDefaults{}).DefaultSupervisorImage()
}

// selectGatewayImage selects the gateway's direct image or the platform default.
func (r *GatewayReconciler) selectGatewayImage(gw *pb.Gateway) (string, error) {
	if gw.Image != nil && *gw.Image != "" {
		return *gw.Image, nil
	}
	image := (gateway.StaticImageDefaults{}).DefaultGatewayImage()
	if image == "" {
		return "", fmt.Errorf("gateway image is not configured; set GATEWAY_IMAGE or specify a gateway image")
	}
	return image, nil
}

// persistGeneratedOIDC resolves the controller-generated OIDC configuration for
// gw and returns the snapshot the pass must apply.
//
// When the stored configuration already matches, gw itself is the snapshot and
// nothing is written. Otherwise the configuration is written first, and the
// record the API server returns for that write is the snapshot: it is the stored
// state at its generation, including any concurrent desired-state change, so the
// pass applies and acknowledges one consistent generation. A failed write is
// returned so the pass retries before anything is applied (OSG-OIDC-05).
func (r *GatewayReconciler) persistGeneratedOIDC(ctx context.Context, gatewayID string, gw *pb.Gateway, clientID string) (*pb.Gateway, error) {
	desired := gateway.DesiredOIDCConfig(r.keycloakConfig, clientID)
	if stored := gw.GetOidc(); stored != "" {
		var current gateway.OIDCConfig
		if err := json.Unmarshal([]byte(stored), &current); err == nil && current == desired {
			return gw, nil
		}
	}
	oidcJSON, err := json.Marshal(desired)
	if err != nil {
		return nil, fmt.Errorf("marshal oidc config for %s: %w", clientID, err)
	}
	oidc := string(oidcJSON)
	resp, err := r.gatewayServiceClient().UpdateGateway(ctx, &pb.UpdateGatewayRequest{
		Id:   gatewayID,
		Oidc: &oidc,
	})
	if err != nil {
		return nil, fmt.Errorf("persist oidc config for %s: %w", clientID, err)
	}
	snapshot := resp.GetGateway()
	if snapshot == nil {
		return nil, fmt.Errorf("persist oidc config for %s: API server returned no gateway", clientID)
	}
	// Metadata and field names only: the OIDC document carries no secret, but
	// keep its values out of the log anyway.
	slog.InfoContext(ctx, "Persisted generated gateway configuration before apply",
		"gateway_id", gatewayID,
		"changed_fields", "oidc",
		"event_generation", gw.GetGeneration(),
		"apply_generation", snapshot.GetGeneration(),
	)
	return snapshot, nil
}

type StubGatewayReconciler struct{}

func NewStubGatewayReconciler() *StubGatewayReconciler {
	return &StubGatewayReconciler{}
}

func (r *StubGatewayReconciler) Handle(ctx context.Context, event watcher.Event[*pb.Gateway]) error {
	slog.InfoContext(ctx, "Stub reconciling Gateway", "gateway_id", event.ResourceID, "event_type", event.Type)
	return nil
}
