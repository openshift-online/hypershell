package reconciler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
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
	defer func() { endSpan(nil) }()

	log.Printf("INFO reconciling ManagedCluster %s (event=%d)", event.ResourceID, event.Type)
	return nil
}

// Control-plane-owned GatewayRelease status values (see
// gateway-release-reconciliation.spec.md). The reconciler settles a release's
// status to reflect the reconciled validation outcome. Available means the image
// reference is well-formed and the release may be used by gateways; Invalid is
// prefixed onto a short reason describing why validation failed.
const (
	releaseStatusAvailable = "Available"
	releaseStatusInvalid   = "Invalid"
)

// gatewayEnqueuer requests a gateway be re-reconciled through the shared gateway
// reconcile queue. The release reconciler uses it to propagate an image change to
// referencing gateways. EnqueueForced bypasses the gateway reconciler's phase
// gate (as recovery seeds do) so a gateway already Running is re-reconciled to
// pick up the new desired image rather than being skipped. *watcher.GatewayReconcileQueue
// satisfies it.
type gatewayEnqueuer interface {
	EnqueueForced(watcher.Event[*pb.Gateway])
}

// GatewayReleaseReconciler reconciles GatewayRelease resources. A release owns no
// Kubernetes resources, so reconciliation means: validate the release image,
// write a deterministic status back to the API server, and -- when a known
// release's effective image changes -- request reconciliation of every gateway
// that references the release so the cluster converges toward the new version.
// Resolving release_id -> image at gateway deploy time and rollout safety are
// owned by sibling specs; this reconciler only guarantees the referencing
// gateways are re-reconciled.
type GatewayReleaseReconciler struct {
	mu sync.Mutex
	// lastImage records the last validated image observed per release ID so an
	// update that does not change the effective image does not fan out, and so the
	// first observation of a release (e.g. on controller start or a fresh create)
	// establishes a baseline without re-provisioning gateways that are already
	// running. Guarded by mu.
	lastImage map[string]string

	gateways pb.GatewayServiceClient
	releases pb.GatewayReleaseServiceClient
	gwQueue  gatewayEnqueuer
	// clusterID scopes the release fan-out's gateway listing to this control
	// plane's own cluster. On a managed-cluster spoke (non-empty) the GatewayRelease
	// watch runs on every control plane, so an unscoped list would match and force
	// foreign gateways into the local reconcile queue, breaking pull-model
	// isolation; empty means single-cluster (no server-side filter).
	clusterID string
}

// NewGatewayReleaseReconciler builds the release reconciler. conn is the API
// server gRPC connection used to write release status and list referencing
// gateways; gwQueue is the shared gateway reconcile queue used to propagate image
// changes. Either dependency may be nil (e.g. when the controller runs without a
// Kubernetes client), in which case propagation is skipped but validation and
// status write-back still run. clusterID is this control plane's managed-cluster
// identity (empty in single-cluster mode); it scopes the fan-out's gateway
// listing so a spoke never force-reconciles another cluster's gateways.
func NewGatewayReleaseReconciler(conn *grpc.ClientConn, gwQueue gatewayEnqueuer, clusterID string) *GatewayReleaseReconciler {
	r := &GatewayReleaseReconciler{
		lastImage: make(map[string]string),
		gwQueue:   gwQueue,
		clusterID: clusterID,
	}
	if conn != nil {
		r.gateways = pb.NewGatewayServiceClient(conn)
		r.releases = pb.NewGatewayReleaseServiceClient(conn)
	}
	return r
}

func (r *GatewayReleaseReconciler) Handle(ctx context.Context, event watcher.Event[*pb.GatewayRelease]) error {
	// Per-release serialization is owned by the reconcile queue that drives this
	// handler (WatchGatewayReleases), so no in-handler active-set guard is needed;
	// adding one back would risk returning nil (success) on a spurious skip and
	// masking a dropped reconcile from the queue's retry/backoff.
	_, endSpan := cpotel.StartReconcileSpan(ctx, "GatewayRelease", event.Type.String(), event.Resource.GetMetadata().GetTraceparent())
	var reconcileErr error
	defer func() { endSpan(reconcileErr) }()

	// A release owns no cluster resources, so a delete is a terminal, idempotent
	// no-op with respect to Kubernetes: running gateways deployed from the release
	// are left untouched. Forget the baseline so a later create of a new release
	// (KSUIDs are never reused, but be defensive) starts clean.
	if event.Type == watcher.EventDeleted {
		r.forget(event.ResourceID)
		log.Printf("INFO gateway release %s deleted; no cluster resources to remove", event.ResourceID)
		return nil
	}

	rel := event.Resource
	if rel == nil {
		log.Printf("WARN gateway release event %s has nil resource, skipping", event.ResourceID)
		return nil
	}

	// Validate the image reference using the same rules applied to gateway
	// workloads (well-formed reference, no shell-injection metacharacters). An
	// empty image is rejected too.
	image := rel.GetImage()
	validationErr := gateway.ValidateImageReference(image)

	desiredStatus := releaseStatusAvailable
	if validationErr != nil {
		desiredStatus = fmt.Sprintf("%s: %s", releaseStatusInvalid, validationErr)
	}

	// Deterministic, idempotent status write-back: only update when the persisted
	// status differs from the reconciled outcome.
	if rel.GetStatus() != desiredStatus {
		if err := r.updateStatus(ctx, event.ResourceID, desiredStatus); err != nil {
			reconcileErr = fmt.Errorf("update gateway release %s status: %w", event.ResourceID, err)
			return reconcileErr
		}
	}

	if validationErr != nil {
		// An invalid release is not propagated to any gateway. The last valid image
		// baseline is intentionally retained (not forgotten): a later correction to
		// an image different from that baseline is then detected as a genuine change
		// and fans out, while a correction back to the same image correctly no-ops.
		// Forgetting here would reclassify the correction as a first observation and
		// silently skip the fan-out.
		log.Printf("INFO gateway release %s invalid image: %v", event.ResourceID, validationErr)
		return nil
	}

	// Fan out only when a previously-observed release's effective image changed.
	// The first observation records a baseline without fanning out: a brand-new
	// release has no referencing gateways yet, and on controller restart every
	// release would otherwise force-reconcile every running gateway.
	prev, seen := r.lastImageFor(event.ResourceID)
	if seen && prev != image {
		if err := r.propagateToGateways(ctx, event.ResourceID, image); err != nil {
			reconcileErr = fmt.Errorf("propagate gateway release %s to referencing gateways: %w", event.ResourceID, err)
			// Leave the baseline unchanged so the retry re-detects the change and
			// re-attempts the fan-out.
			return reconcileErr
		}
	}
	r.rememberImage(event.ResourceID, image)
	return nil
}

// updateStatus writes the release's reconciled status back to the API server. It
// is a no-op when the release client is not configured.
func (r *GatewayReleaseReconciler) updateStatus(ctx context.Context, id, status string) error {
	if r.releases == nil {
		return nil
	}
	_, err := r.releases.UpdateGatewayRelease(ctx, &pb.UpdateGatewayReleaseRequest{
		Id:     id,
		Status: &status,
	})
	return err
}

// propagateToGateways enqueues every gateway that references the release for
// reconciliation. It is a no-op when the gateway client or the shared queue is
// not configured (e.g. the controller has no Kubernetes client).
func (r *GatewayReleaseReconciler) propagateToGateways(ctx context.Context, releaseID, image string) error {
	if r.gateways == nil || r.gwQueue == nil {
		return nil
	}
	gws, err := r.listGatewaysForRelease(ctx, releaseID)
	if err != nil {
		return err
	}
	for _, gw := range gws {
		r.gwQueue.EnqueueForced(watcher.Event[*pb.Gateway]{
			Type:       watcher.EventUpdated,
			ResourceID: gw.GetMetadata().GetId(),
			Resource:   gw,
		})
	}
	log.Printf("INFO gateway release %s image changed to %s; enqueued %d referencing gateway(s) for reconciliation", releaseID, image, len(gws))
	return nil
}

// listGatewaysForRelease returns every gateway whose release_id references the
// given release. It reuses listAllGateways so the listing is scoped to this
// control plane's cluster (via r.clusterID): on a managed-cluster spoke this
// prevents matching and force-enqueuing gateways owned by other clusters, which
// would violate pull-model isolation. Filtering by release_id is done
// client-side; a server-side filter is a scale follow-up.
func (r *GatewayReleaseReconciler) listGatewaysForRelease(ctx context.Context, releaseID string) ([]*pb.Gateway, error) {
	all, err := listAllGateways(ctx, r.gateways, r.clusterID)
	if err != nil {
		return nil, err
	}
	var matching []*pb.Gateway
	for _, gw := range all {
		if gw.GetReleaseId() == releaseID {
			matching = append(matching, gw)
		}
	}
	return matching, nil
}

func (r *GatewayReleaseReconciler) lastImageFor(id string) (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	img, ok := r.lastImage[id]
	return img, ok
}

func (r *GatewayReleaseReconciler) rememberImage(id, image string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastImage[id] = image
}

func (r *GatewayReleaseReconciler) forget(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.lastImage, id)
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
	hasGatewayAPI         bool
	ingressMode           string
	controlPlaneNamespace string
	keycloakClient        *keycloak.Client
	keycloakConfig        *gateway.KeycloakConfig
	exposure              exposure.Port
	externalCAIssuerName  string
	externalCAIssuerKind  string
	ingressBaseDomain     string
	// database locates the mounted admin credentials every gateway database is
	// provisioned with; validated once at controller startup.
	database  gateway.DatabaseConfig
	clusterID string
}

func NewGatewayReconciler(
	dynamicClient dynamic.Interface,
	clientset *kubernetes.Clientset,
	grpcConn *grpc.ClientConn,
	helmClient *helm.ShellClient,
	controlPlaneNamespace string,
	keycloakConfig *gateway.KeycloakConfig,
	exposurePort exposure.Port,
	externalCAIssuerName string,
	externalCAIssuerKind string,
	database gateway.DatabaseConfig,
	clusterID string,
) (*GatewayReconciler, error) {
	if database.AdminCredentialsDir == "" {
		return nil, fmt.Errorf("gateway database admin credentials directory is required")
	}

	isOpenShift := gateway.DetectOpenShift(clientset)
	hasCertManager := gateway.DetectCertManager(clientset)
	hasGatewayAPI := gateway.DetectGatewayAPI(clientset)
	ingressMode := gateway.IngressMode(hasGatewayAPI, isOpenShift)

	var kcClient *keycloak.Client
	if keycloakConfig != nil {
		kcClient = keycloak.NewClient(
			keycloakConfig.ServerURL,
			keycloakConfig.Realm,
			keycloakConfig.ClientID,
			keycloakConfig.ClientSecret,
		)
		log.Printf("INFO keycloak integration enabled: server=%s realm=%s", keycloakConfig.ServerURL, keycloakConfig.Realm)
	}

	// Derive ingress base domain from environment
	ingressBaseDomain := os.Getenv("INGRESS_BASE_DOMAIN")
	if ingressBaseDomain == "" {
		ingressBaseDomain = "gateway.cluster.local"
		log.Printf("WARN INGRESS_BASE_DOMAIN not set, using default: %s", ingressBaseDomain)
	}

	// Validate external CA issuer for Route passthrough mode
	if !hasGatewayAPI && externalCAIssuerName == "" {
		log.Printf("WARN EXTERNAL_CA_ISSUER_NAME not set but Gateway API unavailable; Route passthrough mode will fail cert validation")
	}

	log.Printf("INFO gateway reconciler initialized: helm=%s openshift=%v certmanager=%v gatewayapi=%v ingressMode=%s keycloak=%v ingress=%s ca-issuer=%s",
		helmClient.ChartPath, isOpenShift, hasCertManager, hasGatewayAPI, ingressMode, kcClient != nil, ingressBaseDomain, externalCAIssuerName)

	return &GatewayReconciler{
		active:                make(map[string]struct{}),
		dynamicClient:         dynamicClient,
		clientset:             clientset,
		grpcConn:              grpcConn,
		helmClient:            helmClient,
		isOpenShift:           isOpenShift,
		hasCertManager:        hasCertManager,
		hasGatewayAPI:         hasGatewayAPI,
		ingressMode:           ingressMode,
		controlPlaneNamespace: controlPlaneNamespace,
		keycloakClient:        kcClient,
		keycloakConfig:        keycloakConfig,
		exposure:              exposurePort,
		externalCAIssuerName:  externalCAIssuerName,
		externalCAIssuerKind:  externalCAIssuerKind,
		ingressBaseDomain:     ingressBaseDomain,
		database:              database,
		clusterID:             clusterID,
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
		log.Printf("WARN gateway event %s has nil resource, skipping", event.ResourceID)
		return nil
	}
	previousPhase := gw.GetPhase()
	if event.PhaseBeforeRetry != "" {
		previousPhase = event.PhaseBeforeRetry
	}
	suppressGatewayProvisionObservation(event.ResourceID, previousPhase)
	ctx, endSpan := cpotel.StartReconcileSpan(ctx, "Gateway", event.Type.String(), gw.GetMetadata().GetTraceparent())
	cpotel.SetResourceStatusStale(ctx, r.clusterID, "Gateway", event.ResourceID,
		event.Type != watcher.EventDeleted && gw.GetReleaseId() != gw.GetObservedReleaseId())
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attribute.String("hypershell.resource_id", event.ResourceID))
	var reconcileErr error
	defer func() { endSpan(reconcileErr) }()

	if event.Type == watcher.EventDeleted {
		forgetGatewayProvisionObservation(event.ResourceID)
		var deleteErrs []error

		namespace, namespaceErr := gatewayNamespace(gw)
		if namespaceErr != nil {
			// Without a recorded namespace there is nothing deterministic to clean
			// up; NamespaceGC remains the namespace backstop.
			log.Printf("WARN gateway %s deleted but %v; skipping namespace cleanup", event.ResourceID, namespaceErr)
		} else {
			log.Printf("INFO gateway %s deleted, cleaning up resources in namespace %s", event.ResourceID, namespace)
			opts := gateway.ReconcileOpts{
				IsOpenShift:           r.isOpenShift,
				HasCertManager:        r.hasCertManager,
				HasGatewayAPI:         r.hasGatewayAPI,
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
					log.Printf("ERROR gateway %s: failed to record IncompleteFinalization Event for %s %q: %v", event.ResourceID, resourceKind, resourceName, err)
				}
			}
			if r.keycloakClient != nil {
				clientID, err := existingGatewayKeycloakClientID(event.ResourceID, gw)
				if err != nil {
					// Invalid stored identity never becomes valid on retry, so failing
					// here would pin the delete tombstone after namespace and database
					// cleanup. Keycloak is not contacted. Record the leak durably (not
					// just a log line) so the orphaned Keycloak clients are visible to
					// operators for recovery.
					log.Printf("ERROR gateway %s stored identity cannot be resolved (%v); skipping Keycloak cleanup; Keycloak clients may remain for operator recovery", event.ResourceID, err)
					opts.RecordOrphan(ctx, "KeycloakClient", fmt.Sprintf("gateway %s (%s)", gw.Name, event.ResourceID),
						fmt.Sprintf("stored identity cannot be resolved (%v); Keycloak clients could not be deleted", err))
				} else {
					opts.KeycloakClient = r.keycloakClient
					opts.GatewayClientID = clientID
				}
			}
			if gw.GetOidc() != "" && r.keycloakClient == nil {
				// A deconfigured provisioner never comes back, so failing here would
				// pin the delete tombstone after namespace and database cleanup.
				// Log the recorded identity for operator recovery instead.
				clientID, idErr := existingGatewayKeycloakClientID(event.ResourceID, gw)
				if idErr != nil {
					log.Printf("ERROR gateway %s identity cleanup requires the Keycloak client; stored identity cannot be resolved (%v); Keycloak clients may remain for operator recovery", event.ResourceID, idErr)
					opts.RecordOrphan(ctx, "KeycloakClient", fmt.Sprintf("gateway %s (%s)", gw.Name, event.ResourceID),
						fmt.Sprintf("Keycloak provisioner is deconfigured and stored identity cannot be resolved (%v); Keycloak clients could not be deleted", idErr))
				} else {
					log.Printf("ERROR gateway %s identity cleanup requires the Keycloak client; leaving Keycloak clients %q and %q for operator recovery", event.ResourceID, clientID, clientID+"-console")
					opts.RecordOrphan(ctx, "KeycloakClient", clientID,
						"Keycloak provisioner is deconfigured; the gateway Keycloak client could not be deleted")
					opts.RecordOrphan(ctx, "KeycloakClient", clientID+"-console",
						"Keycloak provisioner is deconfigured; the console Keycloak client could not be deleted")
				}
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
				log.Printf("INFO gateway %s resources cleaned up from namespace %s", event.ResourceID, namespace)
			}

			// Namespace deletion and database deletion are independent cleanup
			// operations. Attempt both and aggregate failures so one partial failure
			// cannot silently leak the other resource.
			deleted, err := gateway.DeleteManagedNamespace(ctx, r.clientset, namespace, r.controlPlaneNamespace)
			if err != nil {
				deleteErrs = append(deleteErrs, fmt.Errorf("delete gateway namespace %s: %w", namespace, err))
			} else if !deleted {
				gateway.DeleteLabeledNamespaceResources(ctx, r.dynamicClient, namespace, opts)
			}
		}

		reconcileErr = errors.Join(deleteErrs...)
		return reconcileErr
	}

	log.Printf("INFO reconciling Gateway %s name=%q namespace=%s (event=%d)",
		event.ResourceID, gw.Name, gw.Namespace, event.Type)

	// The phase gate prevents redundant re-provisioning (re-applying manifests)
	// of a Gateway that has already been acted upon. Running, Provisioning, and
	// Degraded gateways are owned by the continuous health reconciler, which
	// keeps their phase synchronized with workload health via a separate path
	// that this gate does not suppress. See openshell-gateway-health.spec.md.
	//
	// Keycloak client attributes are external desired state, however, and need a
	// lightweight drift reconciliation even when Kubernetes reprovisioning is
	// gated. In particular, controller startup seeds existing Running gateways;
	// reconciling before the return below lets newly introduced client settings
	// converge without forcing a full gateway rollout.
	if gw.Phase != nil && (*gw.Phase == string(gatewayhealth.PhaseRunning) || *gw.Phase == string(gatewayhealth.PhaseProvisioning) || *gw.Phase == string(gatewayhealth.PhaseDegraded)) {
		if err := r.reconcileExistingGatewayKeycloakClient(ctx, event.ResourceID, gw); err != nil {
			var identityErr *gatewayKeycloakClientIdentityError
			if errors.As(err, &identityErr) {
				// Invalid persisted identity is terminal desired-state validation, not a
				// transient Keycloak outage. Publish a fixed marker once and stop retrying;
				// retry only when the status write itself fails.
				if gw.GetStatus() == gatewayKeycloakClientInvalidStatus {
					return nil
				}
				if statusErr := r.updateGatewayStatus(ctx, event.ResourceID, gatewayKeycloakClientInvalidStatus); statusErr != nil {
					return watcher.PreservePayloadForRetry(errors.Join(
						fmt.Errorf("validate existing Keycloak client identity: %w", err),
						fmt.Errorf("publish invalid Keycloak client configuration status: %w", statusErr),
					))
				}
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
			return watcher.PreservePayloadForRetry(fmt.Errorf("reconcile Keycloak client for gateway %q: %w", gw.Name, err))
		}
		if r.keycloakClient != nil && isGatewayKeycloakClientStatus(gw.GetStatus()) {
			if err := r.updateGatewayStatus(ctx, event.ResourceID, ""); err != nil {
				return watcher.PreservePayloadForRetry(fmt.Errorf("clear Keycloak client status for gateway %q: %w", gw.Name, err))
			}
		}
		log.Printf("DEBUG gateway %s phase=%s, skipping full reconciliation", event.ResourceID, *gw.Phase)
		return nil
	}

	namespace, err := gatewayNamespace(gw)
	if err != nil {
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
	}

	// Select the release image, direct image, or platform default before Helm
	// values are built. See specs/platform/gateway-version-selection.spec.md.
	image, err := r.selectGatewayImage(ctx, gw)
	if err != nil {
		reconcileErr = fmt.Errorf("select image for gateway %s: %w", gw.Name, err)
		return reconcileErr
	}
	gwConfig.Image = image
	// Record the release the image was resolved from so the applied release is
	// stamped onto the Deployment and the health loop advances observed_release_id
	// only to what was actually rolled out. Empty for a direct-image gateway.
	gwConfig.ReleaseID = gw.ReleaseId

	supervisorImage := selectSupervisorImage(gw)
	if supervisorImage == "" {
		reconcileErr = fmt.Errorf("supervisor image is not configured for gateway %s; set GATEWAY_SUPERVISOR_IMAGE or specify a supervisor_image", gw.Name)
		return reconcileErr
	}
	gwConfig.SupervisorImage = supervisorImage

	if gw.Oidc != nil && *gw.Oidc != "" {
		var oidcConfig gateway.OIDCConfig
		if err := json.Unmarshal([]byte(*gw.Oidc), &oidcConfig); err != nil {
			reconcileErr = fmt.Errorf("invalid oidc config for gateway %s: %w", gw.Name, err)
			return reconcileErr
		}
		gwConfig.OIDC = oidcConfig
	}

	if gw.Route != nil {
		routeConfig, err := parseGatewayRouteConfig(*gw.Route)
		if err != nil {
			reconcileErr = fmt.Errorf("invalid route config for gateway %s: %w", gw.Name, err)
			return reconcileErr
		}
		gwConfig.Route = routeConfig
	}

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
		IsOpenShift:           r.isOpenShift,
		HasCertManager:        r.hasCertManager,
		HasGatewayAPI:         r.hasGatewayAPI,
		Database:              r.database,
		ControlPlaneNamespace: r.controlPlaneNamespace,
		GatewayID:             event.ResourceID,
		UpdateRouteAddress:    r.makeRouteAddressUpdater(event.ResourceID),
		UpdateConsoleAddress:  r.makeConsoleAddressUpdater(event.ResourceID),
		Keycloak:              r.keycloakConfig,
		KeycloakClient:        r.keycloakClient,
		GatewayName:           gw.Name,
		UpdateOIDC:            r.makeOIDCUpdater(event.ResourceID),
		Exposure:              r.exposure,
		RouteStillDesired:     r.makeRouteStillDesired(event.ResourceID),
		ExternalCAIssuerName:  r.externalCAIssuerName,
		ExternalCAIssuerKind:  r.externalCAIssuerKind,
		IngressBaseDomain:     r.ingressBaseDomain,
	}

	conditions := gateway.InitConditions(r.keycloakConfig != nil)
	r.updateProvisioningConditions(ctx, event.ResourceID, conditions)

	opts.ReportProgress = func(step, status, message string) {
		gateway.SetCondition(conditions, step, status, message)
		r.updateProvisioningConditions(ctx, event.ResourceID, conditions)
	}

	r.updateGatewayPhase(ctx, event.ResourceID, string(gatewayhealth.PhaseProvisioning))

	if err := gateway.ReconcileGateway(ctx, r.dynamicClient, r.clientset, r.helmClient, nsConfig, opts); err != nil {
		var renderErr *gateway.RenderedConfigValidationError
		if errors.As(err, &renderErr) {
			reason := fmt.Sprintf("generated configuration validation failed: %v", renderErr.Err)
			if failedGateway := r.updateGatewayHealth(ctx, event.ResourceID, string(gatewayhealth.PhaseFailed), reason); failedGateway != nil {
				observeGatewayProvisionFailure(ctx, event.ResourceID)
			}
			log.Printf("ERROR gateway %s generated configuration invalid in namespace %s: %v", gw.Name, namespace, renderErr.Err)
		} else if r.updateGatewayPhase(ctx, event.ResourceID, string(gatewayhealth.PhaseFailed)) {
			observeGatewayProvisionFailure(ctx, event.ResourceID)
		}
		reconcileErr = fmt.Errorf("reconcile gateway %s: %w", gw.Name, err)
		return reconcileErr
	}

	// Step 5: GatewayHealthy
	gateway.SetCondition(conditions, gateway.ConditionGatewayHealthy, gateway.StatusInProgress, "")
	r.updateProvisioningConditions(ctx, event.ResourceID, conditions)

	if r.exposure != nil && isRoutedGateway(gw) {
		routeHost := ""
		if gw.Route != nil {
			var rc struct{ Host string }
			_ = json.Unmarshal([]byte(*gw.Route), &rc)
			routeHost = rc.Host
		}
		addr, err := r.exposure.ResolveAddress(ctx, exposure.Request{
			Namespace: namespace,
			Host:      routeHost,
		})
		if err != nil {
			log.Printf("WARN gateway %s: failed to resolve route address: %v", gw.Name, err)
		} else if addr != "" {
			if err := r.updateRouteAddress(ctx, event.ResourceID, addr); err != nil {
				log.Printf("WARN gateway %s: failed to publish route address: %v", gw.Name, err)
			} else {
				log.Printf("INFO gateway %s: published route address %s", gw.Name, addr)
			}
		}
	}

	// Manifests are applied, but the gateway is not Running until its workload is
	// observed Ready. Wait within the provisioning readiness window; if the
	// Deployment never becomes ready, set Degraded and record why.
	ready, reason := gateway.WaitForGatewayReady(ctx, r.clientset, namespace, 2*time.Minute)
	if !ready {
		gateway.SetCondition(conditions, gateway.ConditionGatewayHealthy, gateway.StatusFailed, "Gateway health check timed out - the gateway workload is not yet ready")
		r.updateProvisioningConditions(ctx, event.ResourceID, conditions)
		r.updateGatewayHealth(ctx, event.ResourceID, string(gatewayhealth.PhaseDegraded), reason)
		log.Printf("WARN gateway %s applied but not ready in namespace %s: %s", gw.Name, namespace, reason)
		return nil
	}

	// The Deployment is Ready. A routed gateway is not Running until its external
	// exposure is also observed Ready. Poll the exposure here within a bounded
	// window so the gateway is promoted to Running promptly once its route is
	// programmed - rather than waiting up to a full health-reconciler tick, which
	// would leave the connection command and console button hidden for seconds
	// after the pods are ready. If the window elapses, park at Provisioning and
	// let the continuous health reconciler keep enforcing the full route-readiness
	// grace window (promoting to Running, or Degraded once it expires). A
	// non-routed gateway - or any gateway on a cluster without the exposure port -
	// is Running on Deployment readiness alone. See
	// openshell-gateway-health.spec.md § Phase Reflects Workload and Route Readiness.
	routed := isRoutedGateway(gw)
	client := pb.NewGatewayServiceClient(r.grpcConn)
	if r.exposure != nil && routed {
		if r.waitForRouteReady(ctx, namespace) {
			gateway.SetCondition(conditions, gateway.ConditionGatewayHealthy, gateway.StatusComplete, "")
			r.updateProvisioningConditions(ctx, event.ResourceID, conditions)
			// The observation guard rejects work that started in Running or Degraded.
			if runningGateway := r.updateGatewayHealth(ctx, event.ResourceID, string(gatewayhealth.PhaseRunning), gatewayhealth.StatusHealthy); runningGateway != nil {
				observeGatewayProvisionSuccess(ctx, runningGateway)
			}
			// The new revision has passed its workload and route health gates: now
			// report the release actually rolled out. This path just rendered the
			// Deployment from gw.ReleaseId (gwConfig.ReleaseID above), so the applied
			// release is gw.GetReleaseId(). A write-back failure is surfaced so the
			// reconcile is retried rather than leaving the gateway falsely reporting
			// the new release. See gateway-release-rollout.spec.md.
			if err := advanceObservedRelease(ctx, client, gw.GetMetadata().GetId(), gw.GetObservedReleaseId(), gw.GetReleaseId()); err != nil {
				log.Printf("WARN gateway %s: %v", gw.Name, err)
				return err
			}
			log.Printf("INFO gateway %s provisioned and route ready in namespace %s", gw.Name, namespace)
		} else {
			// Route gate not yet passed: hold at Provisioning and do NOT advance the
			// observed release. The continuous health reconciler promotes to Running
			// (and advances the observed release) once the route is ready.
			r.updateGatewayHealth(ctx, event.ResourceID, string(gatewayhealth.PhaseProvisioning), "Deployment ready; awaiting route readiness")
			log.Printf("INFO gateway %s deployment ready in namespace %s; awaiting route readiness", gw.Name, namespace)
		}
	} else {
		gateway.SetCondition(conditions, gateway.ConditionGatewayHealthy, gateway.StatusComplete, "")
		r.updateProvisioningConditions(ctx, event.ResourceID, conditions)
		// The observation guard rejects work that started in Running or Degraded.
		if runningGateway := r.updateGatewayHealth(ctx, event.ResourceID, string(gatewayhealth.PhaseRunning), gatewayhealth.StatusHealthy); runningGateway != nil {
			observeGatewayProvisionSuccess(ctx, runningGateway)
		}
		// The new revision has passed its health gate: report the release rolled out.
		// This path just rendered the Deployment from gw.ReleaseId, so the applied
		// release is gw.GetReleaseId().
		if err := advanceObservedRelease(ctx, client, gw.GetMetadata().GetId(), gw.GetObservedReleaseId(), gw.GetReleaseId()); err != nil {
			log.Printf("WARN gateway %s: %v", gw.Name, err)
			return err
		}
		log.Printf("INFO gateway %s provisioned and ready in namespace %s", gw.Name, namespace)
	}

	// The console can start after the gateway is ready. Poll the console in the
	// background so the address appears without waiting for the next health tick.
	// The health reconciler continues to publish and retract the address.
	if routed && r.ingressMode != gateway.IngressModeNone {
		go r.publishConsoleAddressWhenReady(ctx, event.ResourceID, gw)
	}
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
	if r.keycloakClient == nil {
		return nil
	}
	clientID, err := existingGatewayKeycloakClientID(gatewayID, gw)
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
	log.Printf("INFO reconciled Keycloak client %q (uuid=%q)", clientID, clientUUID)
	return nil
}

// existingGatewayKeycloakClientID returns the identity recorded when the client
// was provisioned. Gateway names are mutable, so recomputing from the current name
// can miss the real client after a rename. client_id is present on newer rows;
// audience carries the same value on legacy rows. Persisted values cross an API
// trust boundary, so they are accepted only when they agree, contain no control
// characters, and match a gateway-owned historical identity format: either the
// gateway ID itself or a prefix followed by "-<gatewayID>". The prefix is not
// otherwise restricted because historical clients were provisioned from the raw
// user-visible gateway name. Only gateways without either persisted value fall
// back to the provisioning format based on the current name.
func existingGatewayKeycloakClientID(gatewayID string, gw *pb.Gateway) (string, error) {
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
	if clientID != "" && audience != "" && clientID != audience {
		return "", invalidGatewayKeycloakClientIdentity("persisted OIDC client_id and audience do not match")
	}

	persisted := clientID
	if persisted == "" {
		persisted = audience
	}
	if persisted != "" {
		if !isGatewayOwnedKeycloakClientID(persisted, gatewayID) {
			return "", invalidGatewayKeycloakClientIdentity("persisted OIDC client identity is not owned by the gateway")
		}
		return persisted, nil
	}

	if gw.GetName() == "" {
		return "", invalidGatewayKeycloakClientIdentity("gateway name is required for Keycloak reconciliation")
	}
	fallback := fmt.Sprintf("%s-%s", gw.GetName(), gatewayID)
	if containsControlCharacter(fallback) {
		return "", invalidGatewayKeycloakClientIdentity("gateway-derived Keycloak client identity contains control characters")
	}
	if !isGatewayOwnedKeycloakClientID(fallback, gatewayID) {
		return "", invalidGatewayKeycloakClientIdentity("gateway-derived Keycloak client identity is not owned by the gateway")
	}
	return fallback, nil
}

func isGatewayOwnedKeycloakClientID(clientID, gatewayID string) bool {
	if clientID == "" || containsControlCharacter(clientID) {
		return false
	}
	if clientID == gatewayID {
		return true
	}
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
func (r *GatewayReconciler) waitForRouteReady(ctx context.Context, namespace string) bool {
	return r.pollRouteReady(ctx, namespace, provisioningRouteReadyInterval, provisioningRouteReadyWait)
}

// pollRouteReady observes the exposure immediately and then every interval until
// it reports Ready or the window elapses, mirroring WaitForGatewayReady so a
// route that is already (or quickly) programmed promotes without waiting a full
// interval. A transient observation error is logged and retried, never treated
// as not-ready-forever. Interval and window are parameters so tests can drive it
// without real-time waits.
func (r *GatewayReconciler) pollRouteReady(ctx context.Context, namespace string, interval, window time.Duration) bool {
	return poll(ctx, interval, window, func() bool {
		rr, err := r.exposure.ObserveReadiness(ctx, exposure.Request{Namespace: namespace})
		if err != nil {
			log.Printf("WARN gateway route readiness for %s: %v", namespace, err)
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
			log.Printf("WARN console publisher: get gateway %s: %v", gatewayID, err)
			return false
		}
		current := resp.GetGateway()
		if current == nil || !isRoutedGateway(current) {
			// No longer routed (or gone): teardown owns the console_address now.
			// End the poll rather than publishing against the stale snapshot.
			return true
		}
		return syncConsoleAddress(ctx, r.clientset, r.dynamicClient, client, gatewayID, current, r.ingressMode)
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
		return isRoutedGateway(resp.GetGateway()), nil
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

// parseGatewayRouteConfig parses the route field. A route object is enabled by
// default for compatibility with existing empty and host-only route objects.
// An explicit enabled=false value disables it. An empty or null value has no
// route.
func parseGatewayRouteConfig(raw string) (gateway.RouteConfig, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "null" {
		return gateway.RouteConfig{}, nil
	}

	config := gateway.RouteConfig{Enabled: true}
	if err := json.Unmarshal([]byte(trimmed), &config); err != nil {
		return gateway.RouteConfig{}, err
	}
	return config, nil
}

// isRoutedGateway reports whether a Gateway enables external route exposure.
func isRoutedGateway(gw *pb.Gateway) bool {
	if gw.Route == nil {
		return false
	}
	routeConfig, err := parseGatewayRouteConfig(*gw.Route)
	if err != nil {
		// The provisioning path reports invalid configuration. Preserve the
		// exposure until that path can resolve the invalid value.
		return true
	}
	return routeConfig.Enabled
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

// listAllGateways pages through the gRPC gateway inventory and returns every
// gateway. The list endpoint is server-side paginated (default page size 20),
// so callers that must reason about the whole fleet (the namespace reaper and
// the health reconciler) cannot rely on a single unpaged request.
func listAllGateways(ctx context.Context, client pb.GatewayServiceClient, clusterID string) ([]*pb.Gateway, error) {
	var all []*pb.Gateway
	for page := int32(1); ; page++ {
		resp, err := client.ListGateways(ctx, &pb.ListGatewaysRequest{
			Page:      page,
			Size:      gatewayListPageSize,
			ClusterId: watcher.OptionalClusterID(clusterID),
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
		log.Printf("WARN failed to update gateway %s health to %s (%s): %v", gatewayID, phase, status, err)
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
		log.Printf("WARN failed to update gateway %s phase to %s: %v", gatewayID, phase, err)
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
		log.Printf("WARN failed to update provisioning conditions for gateway %s: %v", gatewayID, err)
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

// advanceObservedRelease reports the release the control plane has rolled out and
// observed healthy by setting the Gateway's observed_release_id to appliedRelease
// -- the release actually rendered onto the ready workload -- once the new
// revision has passed its health gates. Callers MUST pass the applied release, not
// the desired release_id: advancing to a desired release the workload has not yet
// rolled out would falsely report it ready (the exact failure the spec forbids).
// It is a no-op when appliedRelease is empty (a direct-image gateway, whose
// observed release stays empty) or when observed_release_id already matches, so it
// issues no redundant write for an unchanged release. A write-back failure is
// returned so the caller can retry rather than leave the gateway falsely reporting
// the new release as rolled out. See gateway-release-rollout.spec.md.
func advanceObservedRelease(ctx context.Context, client pb.GatewayServiceClient, gatewayID, observedRelease, appliedRelease string) error {
	if appliedRelease == "" || gatewayID == "" || observedRelease == appliedRelease {
		return nil
	}
	if _, err := client.UpdateGateway(ctx, &pb.UpdateGatewayRequest{
		Id:                gatewayID,
		ObservedReleaseId: &appliedRelease,
	}); err != nil {
		return fmt.Errorf("advance observed_release_id for gateway %s to %s: %w", gatewayID, appliedRelease, err)
	}
	log.Printf("INFO gateway %s observed release advanced to %s", gatewayID, appliedRelease)
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
// selected exposure resource must both be Ready. It does not publish an address
// without a selected ingress mode. It leaves the address unchanged after a
// temporary observation error. It returns whether the console can serve.
func syncConsoleAddress(ctx context.Context, clientset kubernetes.Interface, dynamicClient dynamic.Interface, client pb.GatewayServiceClient, gatewayID string, gw *pb.Gateway, ingressMode string) bool {
	if gatewayID == "" || ingressMode == gateway.IngressModeNone || !isRoutedGateway(gw) {
		return false
	}
	namespace, err := gatewayNamespace(gw)
	if err != nil {
		log.Printf("WARN console address for %s: %v", gatewayID, err)
		return false
	}
	url, hasURL := gateway.ConsoleURL(namespace)
	ready := false
	if hasURL {
		ready, _, err = gateway.DeploymentReadiness(ctx, clientset, namespace, gateway.ConsoleDeploymentName)
		if err != nil {
			log.Printf("WARN console readiness for %s: %v", namespace, err)
			return false
		}
	}
	if ready {
		// The selected public exposure must be Ready before the reconciler
		// publishes the address.
		exposureReady, reason, exposureErr := gateway.ConsoleExposureReady(ctx, dynamicClient, namespace, ingressMode)
		if exposureErr != nil {
			log.Printf("WARN console exposure readiness for %s: %v", namespace, exposureErr)
			return false
		}
		if !exposureReady {
			log.Printf("INFO console for %s not servable yet: %s", namespace, reason)
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
		log.Printf("WARN failed to set console address for %s to %q: %v", gatewayID, desired, err)
		return false
	}
	log.Printf("INFO console address for %s set to %q (consoleReady=%v)", gatewayID, desired, ready)
	return ready
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

// selectGatewayImage selects the release image, direct image, or platform
// default, in that order. Image selection must succeed before Helm deployment.
// See specs/platform/gateway-version-selection.spec.md.
func (r *GatewayReconciler) selectGatewayImage(ctx context.Context, gw *pb.Gateway) (string, error) {
	if gw.ReleaseId != "" {
		return r.resolveReleaseImage(ctx, gw)
	}
	if gw.Image != nil && *gw.Image != "" {
		return *gw.Image, nil
	}
	image := (gateway.StaticImageDefaults{}).DefaultGatewayImage()
	if image == "" {
		return "", fmt.Errorf("gateway image is not configured; set GATEWAY_IMAGE or specify a gateway image or release_id")
	}
	return image, nil
}

// resolveReleaseImage resolves a Gateway's release_id to the image published by
// its referenced GatewayRelease (database-backed version selection). A
// release_id that cannot be resolved to a release with a non-empty image is a
// reconcile failure, not a silent fallback to a default or empty image: the
// error is returned so the reconcile is retried.
// See specs/platform/gateway-version-selection.spec.md.
func (r *GatewayReconciler) resolveReleaseImage(ctx context.Context, gw *pb.Gateway) (string, error) {
	client := pb.NewGatewayReleaseServiceClient(r.grpcConn)
	resp, err := client.GetGatewayRelease(ctx, &pb.GetGatewayReleaseRequest{Id: gw.ReleaseId})
	if err != nil {
		return "", fmt.Errorf("resolve GatewayRelease %s: %w", gw.ReleaseId, err)
	}

	rel := resp.GatewayRelease
	if rel == nil {
		return "", fmt.Errorf("gateway configuration error: GatewayRelease %s returned empty payload", gw.ReleaseId)
	}
	if rel.Image == "" {
		return "", fmt.Errorf("GatewayRelease %s has no image", gw.ReleaseId)
	}

	return rel.Image, nil
}

func (r *GatewayReconciler) makeOIDCUpdater(gatewayID string) func(ctx context.Context, oidcJSON string) error {
	return func(ctx context.Context, oidcJSON string) error {
		client := pb.NewGatewayServiceClient(r.grpcConn)
		_, err := client.UpdateGateway(ctx, &pb.UpdateGatewayRequest{
			Id:   gatewayID,
			Oidc: &oidcJSON,
		})
		if err != nil {
			return fmt.Errorf("update gateway %s oidc: %w", gatewayID, err)
		}
		return nil
	}
}

type StubGatewayReconciler struct{}

func NewStubGatewayReconciler() *StubGatewayReconciler {
	return &StubGatewayReconciler{}
}

func (r *StubGatewayReconciler) Handle(ctx context.Context, event watcher.Event[*pb.Gateway]) error {
	log.Printf("INFO [stub] reconciling Gateway %s (event=%d)", event.ResourceID, event.Type)
	return nil
}

// GatewayNetwork topology vocabulary. See
// specs/platform/gateway-network-reconciliation.spec.md.
const (
	networkTopologyMesh     = "mesh"
	networkTopologyHubSpoke = "hub-spoke"
)

// GatewayNetwork control-plane-owned status values. A network owns no Kubernetes
// resources in this scope, so status reflects configuration validity only, not
// provisioned connectivity.
const (
	networkStatusValid   = "Valid"
	networkStatusInvalid = "Invalid"
)

// GatewayNetworkReconciler reconciles GatewayNetwork resources. A network owns no
// Kubernetes resources in this scope, so reconciliation means: validate the
// network's topology vocabulary and topology/hub coherence, validate that a
// designated hub_gateway_id references an existing Gateway, and write a
// deterministic status back to the API server. Applying real gateway-to-gateway
// connectivity (mesh/tunnel provisioning) is future work owned by a sibling spec
// once product defines the network membership model and connectivity technology;
// this reconciler only records whether the declared configuration is well-formed.
type GatewayNetworkReconciler struct {
	mu     sync.Mutex
	active map[string]struct{}

	gateways pb.GatewayServiceClient
	networks pb.GatewayNetworkServiceClient
}

// NewGatewayNetworkReconciler builds the network reconciler. conn is the API
// server gRPC connection used to look up the designated hub gateway and to write
// network status back. conn may be nil (e.g. in unit tests), in which case the
// hub existence check and status write-back are skipped but the rest of
// validation still runs.
func NewGatewayNetworkReconciler(conn *grpc.ClientConn) *GatewayNetworkReconciler {
	r := &GatewayNetworkReconciler{active: make(map[string]struct{})}
	if conn != nil {
		r.gateways = pb.NewGatewayServiceClient(conn)
		r.networks = pb.NewGatewayNetworkServiceClient(conn)
	}
	return r
}

func (r *GatewayNetworkReconciler) Handle(ctx context.Context, event watcher.Event[*pb.GatewayNetwork]) error {
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

	_, endSpan := cpotel.StartReconcileSpan(ctx, "GatewayNetwork", event.Type.String(), event.Resource.GetMetadata().GetTraceparent())
	var reconcileErr error
	defer func() { endSpan(reconcileErr) }()

	// A network owns no cluster resources, so a delete is a terminal, idempotent
	// no-op with respect to Kubernetes: gateways designated by the network are
	// left untouched.
	if event.Type == watcher.EventDeleted {
		log.Printf("INFO gateway network %s deleted; no cluster resources to remove", event.ResourceID)
		return nil
	}

	net := event.Resource
	if net == nil {
		log.Printf("WARN gateway network event %s has nil resource, skipping", event.ResourceID)
		return nil
	}

	// Validate the declared configuration. A transient dependency failure (e.g. a
	// transient hub lookup error) is returned so the failure is surfaced (logged
	// by the watch loop) rather than silently swallowed or settled to a misleading
	// Invalid. The network watch is inline log-only (no reconcile queue) and does
	// not replay state on reconnect, so a surfaced error re-converges only when the
	// network is next mutated, not automatically.
	desiredStatus, retryErr := r.validate(ctx, net)
	if retryErr != nil {
		reconcileErr = fmt.Errorf("validate gateway network %s: %w", event.ResourceID, retryErr)
		return reconcileErr
	}

	// Deterministic, idempotent status write-back: only update when the persisted
	// status differs from the reconciled outcome.
	if net.GetStatus() != desiredStatus {
		if err := r.updateStatus(ctx, event.ResourceID, desiredStatus); err != nil {
			reconcileErr = fmt.Errorf("update gateway network %s status: %w", event.ResourceID, err)
			return reconcileErr
		}
	}
	return nil
}

// validate applies the network's structural and referential coherence rules and
// returns the deterministic desired status (networkStatusValid, or
// "networkStatusInvalid: reason"). It returns a non-nil error only for a
// transient dependency failure that should be surfaced rather than swallowed; a
// definitive not-found for the hub gateway is a deterministic Invalid, not an
// error.
func (r *GatewayNetworkReconciler) validate(ctx context.Context, net *pb.GatewayNetwork) (string, error) {
	invalid := func(reason string) string {
		return fmt.Sprintf("%s: %s", networkStatusInvalid, reason)
	}

	topology := net.GetTopology()
	switch topology {
	case "":
		return invalid("topology is required"), nil
	case networkTopologyMesh, networkTopologyHubSpoke:
		// recognized
	default:
		return invalid(fmt.Sprintf("unrecognized topology %q", topology)), nil
	}

	hubID := net.GetHubGatewayId()
	if topology == networkTopologyHubSpoke && hubID == "" {
		return invalid("hub-spoke network requires a hub_gateway_id"), nil
	}

	if hubID != "" {
		// A configured hub must reference an existing Gateway. Skip the lookup when
		// no gateway client is configured (started without an API-server gRPC
		// connection, e.g. in unit tests).
		if r.gateways == nil {
			return networkStatusValid, nil
		}
		_, err := r.gateways.GetGateway(ctx, &pb.GetGatewayRequest{Id: hubID})
		if err != nil {
			if status.Code(err) == codes.NotFound {
				return invalid(fmt.Sprintf("hub gateway %q does not exist", hubID)), nil
			}
			// Transient failure: surface as an error rather than settle to a
			// misleading Invalid.
			return "", err
		}
	}

	return networkStatusValid, nil
}

// updateStatus writes the network's reconciled status back to the API server. It
// is a no-op when the network client is not configured.
func (r *GatewayNetworkReconciler) updateStatus(ctx context.Context, id, desired string) error {
	if r.networks == nil {
		return nil
	}
	_, err := r.networks.UpdateGatewayNetwork(ctx, &pb.UpdateGatewayNetworkRequest{
		Id:     id,
		Status: &desired,
	})
	return err
}
