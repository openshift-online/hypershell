package reconciler

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/api-server/pkg/secretstore"
	"github.com/openshift-online/hypershell/components/control-plane/internal/watcher"
	"google.golang.org/grpc"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// AgentRuntime status lifecycle values this reconciler writes back to the API
// server, from data-model.spec.md "AgentRuntime Status Lifecycle". The initial
// Pending value is set by the API server at create time; the reconciler drives the
// transitions from there.
const (
	agentRuntimeStatusPending      = "Pending"
	agentRuntimeStatusProvisioning = "Provisioning"
	agentRuntimeStatusRunning      = "Running"
	agentRuntimeStatusDegraded     = "Degraded"
	agentRuntimeStatusTerminating  = "Terminating"
)

// ProviderMaterial is one SecretSource's durable provider material (a GitHub App
// PEM, a Vertex service-account key) resolved cluster-local, paired with the
// SecretSource it came from so a ProviderRealizer can map it to the matching
// ProviderBinding.
type ProviderMaterial struct {
	Source   *pb.SecretSource
	Material string
}

// ProviderRealizer realizes an AgentRuntime's credentials as OpenShell providers on
// the agent's gateway/workspace, from durable material the control plane read
// cluster-local (agent-secret-late-binding.spec.md "Credentials Realized as OpenShell
// Providers, Not Rendered Secrets"). OpenShell owns rotation through the provider
// refresh strategy; a realizer never mints or rotates the usable credential and never
// renders a raw Kubernetes Secret.
//
// It is a seam. The gateway provider client that implements it (the gateway-side
// workspace/provider provisioning over the gateway gRPC API) is the
// gateway.WorkspaceProvisioner adapter, activated through SetProviderRealizer when the
// gateway provisioning endpoint is configured. The AgentRuntime watch payload now
// embeds the workspace name, ProviderBindings, memberships, and inference route
// (data-model.spec.md "AgentRuntime Reconcile Payload Completeness"). Until a realizer
// is configured the reconciler keeps the transitional raw-Secret placement so the live
// agent fleet does not regress; once a realizer is wired, provider material is realized
// as providers instead of placed as Secrets.
type ProviderRealizer interface {
	// RealizeProviders creates or updates the OpenShell providers for the runtime
	// from the resolved provider material. It is idempotent.
	RealizeProviders(ctx context.Context, ar *pb.AgentRuntime, materials []ProviderMaterial) error
	// DeleteProviders removes the runtime's OpenShell providers.
	DeleteProviders(ctx context.Context, ar *pb.AgentRuntime) error
}

// AgentRuntimeReconciler reconciles AgentRuntime cluster-side scheduling resources
// (issue #521 Wave B, requirements #3 and #5). On an upsert it applies the
// namespace, non-auto-mounting ServiceAccount, default-deny-ingress NetworkPolicy,
// the coordinator gateway-identity Secret, and the coordinator CronJob to the target
// cluster, then writes the status lifecycle (Pending -> Provisioning ->
// Running/Degraded) back to the API server. On a delete it writes Terminating and
// removes the resources.
//
// Credential model (agent-secret-late-binding.spec.md, HYPERSHELL-409): an agent's
// provider credentials are realized as OpenShell providers from durable material read
// cluster-local, and OpenShell owns rotation; the control plane does NOT render a raw
// Kubernetes Secret for them and never mints or rotates the usable credential. The
// coordinator's own gateway-identity (its OIDC login to the gateway) is the one
// credential the coordinator consumes directly, read cluster-local and wired as
// coordinator environment. For gateway-identity the reconciler reads the referenced
// value through its read-only pkg/secretstore client and writes a Kubernetes Secret
// itself; it creates no ExternalSecret, depends on no External Secrets Operator, and
// runs no ESO-style periodic resync.
//
// Provider realization is gateway-side and delegated to a ProviderRealizer. That
// client is not wired yet (issue #526) and the AgentRuntime watch payload does not
// yet carry its ProviderBindings; until a realizer is configured, provider-material
// SecretSources keep the transitional raw-Secret placement so the live fleet does not
// regress. The Running transition reflects cluster-side success (and provider
// realization when a realizer is configured).
//
// Handle holds no per-resource dedup state: the watch reconcile queue serializes
// work per AgentRuntime id (a key in flight is never handed to a second worker),
// so a mutex-guarded in-flight set here would be dead code - and, if it ever
// fired, would drop the coalesced latest state instead of reconciling it.
type AgentRuntimeReconciler struct {
	// kube is the target cluster's typed client. It is nil when the control plane
	// has no in-cluster Kubernetes access, in which case cluster-side provisioning is
	// skipped (mirroring the other reconcilers).
	kube kubernetes.Interface
	// secrets is the read-only secret-store client the reconciler reads each
	// SecretSource value from (to place the gateway-identity Secret and to resolve
	// provider material for realization). It is never used to write to the store. A
	// disabled store (the default when none is configured) rejects reads, so an
	// AgentRuntime that declares a SecretSource is degraded until a store is
	// configured; an AgentRuntime with no SecretSource is unaffected.
	secrets secretstore.Reader

	// providers realizes provider-material SecretSources as OpenShell providers on the
	// agent's gateway/workspace. It is nil until the gateway provider client is wired
	// (issue #526); while nil the reconciler keeps the transitional raw-Secret
	// placement for provider material so the live fleet does not regress. Settable in
	// tests to exercise the realization path.
	providers ProviderRealizer

	grpcConn  *grpc.ClientConn
	clusterID string

	// secretStoreMount is the KV v2 mount the agent-secret store is served under. It
	// is used only to render the actionable SecretPending upload path (mount + logical
	// path) in the status message; it never changes where a value is read from (that is
	// the configured read client). Defaults to "secret".
	secretStoreMount string

	// clusterName is the human-facing ManagedCluster name (HYPERSHELL_MANAGED_CLUSTER_NAME)
	// surfaced in the actionable SecretPending message so an operator knows which
	// cluster's Vault to upload to (agent-secret-late-binding.spec.md "Discoverable
	// Upload Target"). It falls back to clusterID when unset.
	clusterName string

	// statusWriter persists an AgentRuntime status transition. It is overridable in
	// tests; in production it writes through the AgentRuntime gRPC service. A nil
	// writer disables status write-back (no gRPC connection).
	statusWriter func(ctx context.Context, id, status string, message *string) error
}

// NewAgentRuntimeReconciler builds the Wave B reconciler. kube may be nil when the
// control plane has no cluster access; secrets is the read-only secret-store client;
// grpcConn may be nil in tests that inject their own status writer.
func NewAgentRuntimeReconciler(kube kubernetes.Interface, secrets secretstore.Reader, grpcConn *grpc.ClientConn, clusterID string) *AgentRuntimeReconciler {
	mount := os.Getenv("HYPERSHELL_SECRET_STORE_VAULT_MOUNT")
	if mount == "" {
		mount = "secret"
	}
	clusterName := os.Getenv("HYPERSHELL_MANAGED_CLUSTER_NAME")
	if clusterName == "" {
		clusterName = clusterID
	}
	r := &AgentRuntimeReconciler{
		kube:             kube,
		secrets:          secrets,
		grpcConn:         grpcConn,
		clusterID:        clusterID,
		secretStoreMount: mount,
		clusterName:      clusterName,
	}
	if grpcConn != nil {
		r.statusWriter = r.writeStatusViaGRPC
	}
	return r
}

// SetProviderRealizer activates gateway-side provider realization by wiring a live
// ProviderRealizer (the gateway WorkspaceProvisioningPort adapter). When set, the
// reconciler realizes provider-material SecretSources as OpenShell providers on the
// agent's gateway and retires the transitional raw-Secret placement for provider
// material (realize-before-prune). A nil realizer keeps the transitional behavior so
// the live fleet does not regress until the gateway-side server is available. It is
// set once at construction time in main.go, before any reconcile runs.
func (r *AgentRuntimeReconciler) SetProviderRealizer(p ProviderRealizer) {
	r.providers = p
}

func (r *AgentRuntimeReconciler) Handle(ctx context.Context, event watcher.Event[*pb.AgentRuntime]) error {
	ar := event.Resource
	if ar == nil {
		log.Printf("WARN AgentRuntime event %s has nil resource, skipping", event.ResourceID)
		return nil
	}

	if r.kube == nil {
		log.Printf("WARN AgentRuntime %s: no kubernetes client available; cluster-side provisioning disabled", event.ResourceID)
		return nil
	}

	namespace := agentRuntimeNamespace(ar)

	if event.Type == watcher.EventDeleted {
		return r.handleDelete(ctx, ar, namespace)
	}
	return r.handleUpsert(ctx, ar, namespace)
}

// handleUpsert applies all cluster resources and drives the status lifecycle to
// Running (cluster-side success), Degraded (a SecretSource store read genuinely
// failed), or a hold at Provisioning with a SecretPending status message when a
// referenced value has not yet been uploaded to a reachable store.
func (r *AgentRuntimeReconciler) handleUpsert(ctx context.Context, ar *pb.AgentRuntime, namespace string) error {
	log.Printf("INFO reconciling AgentRuntime %s name=%q namespace=%s sources=%d", ar.GetMetadata().GetId(), ar.GetName(), namespace, len(ar.GetSecretSources()))

	// Mark Provisioning before applying so the first reconcile of a new runtime is
	// observable as in-progress rather than stuck at Pending. Only the initial
	// transition writes it: forcing Provisioning on a runtime already past it (for
	// example Running) would regress the status, and because each write fires a
	// watch event that re-reconciles, a Running<->Provisioning flip would become a
	// hot loop. Later passes fall through to the terminal Running/Degraded write,
	// which the setStatus guard suppresses once it stops changing.
	if current := ar.GetStatus(); current == "" || current == agentRuntimeStatusPending {
		r.setStatus(ctx, ar, agentRuntimeStatusProvisioning, nil)
	}

	labels := agentRuntimeLabels(ar)

	if err := applyNamespace(ctx, r.kube, namespace, labels); err != nil {
		return r.fail(ctx, ar, fmt.Errorf("apply namespace: %w", err))
	}
	if err := applyServiceAccount(ctx, r.kube, namespace, labels); err != nil {
		return r.fail(ctx, ar, fmt.Errorf("apply service account: %w", err))
	}
	if err := applyDenyIngressNetworkPolicy(ctx, r.kube, namespace, labels); err != nil {
		return r.fail(ctx, ar, fmt.Errorf("apply network policy: %w", err))
	}

	// Split the SecretSources into those the control plane places as Kubernetes
	// Secrets and those realized as OpenShell providers. Only the coordinator
	// gateway-identity (the coordinator's own OIDC login to the gateway) is placed as
	// a Secret and wired as coordinator environment; provider material is realized as
	// providers (HYPERSHELL-409). While no ProviderRealizer is wired (issue #526) the
	// reconciler keeps the transitional behavior of placing every SecretSource so the
	// live fleet does not regress.
	secretSources, providerSources := r.classifySecretSources(ar.GetSecretSources())

	// Placed Secrets, written directly from the value read out of the store (no
	// ExternalSecret, no ESO). Track the names so a Secret whose SecretSource was
	// removed - or that is now realized as a provider - is pruned.
	desiredSecrets := map[string]struct{}{}
	for _, source := range secretSources {
		// placeSecret returns a classified, value-free reason for the persisted status
		// and a detailed error (used only for the requeue). The status message carries
		// just the SecretSource name (a safe reference) and the reason, never any bytes
		// of the secret value (security.spec.md: a raw stored value never appears in
		// logs, events, or the database).
		reason, err := r.placeSecret(ctx, ar, namespace, source)
		if err != nil {
			if errors.Is(err, secretstore.ErrNotFound) {
				return r.holdSecretPending(ctx, ar, source, err)
			}
			// Unreachable or erroring store (or malformed value): a genuine failure
			// that degrades the runtime.
			msg := fmt.Sprintf("SecretSource %q: %s", source.GetName(), reason)
			r.setStatus(ctx, ar, agentRuntimeStatusDegraded, &msg)
			log.Printf("WARN AgentRuntime %s degraded: %s", ar.GetMetadata().GetId(), msg)
			return fmt.Errorf("reconcile agent runtime %s: place secret for %q: %w", ar.GetMetadata().GetId(), source.GetName(), err)
		}
		desiredSecrets[secretName(source)] = struct{}{}
	}

	// Realize provider material as OpenShell providers BEFORE pruning stragglers, so the
	// transitional raw-Secret placement is retired without ever leaving a window with no
	// credential (realize-before-prune). SecretPending gates PROVIDER CREATION: material
	// absent from a reachable store holds at Provisioning (and returns here, leaving any
	// previously-placed raw Secret in place until realization succeeds); an unreachable
	// store degrades; present material is realized and OpenShell owns rotation thereafter
	// (agent-secret-late-binding.spec.md "SecretPending Condition Gates Provider
	// Creation", "OpenShell Owns Credential Rotation").
	if r.providers != nil && len(providerSources) > 0 {
		if err := r.realizeProviders(ctx, ar, providerSources); err != nil {
			return err
		}
	}

	// Prune Secrets whose SecretSource is no longer declared OR that are now realized as
	// providers: once the realizer is live, provider-material sources are not in
	// desiredSecrets, so the transitional raw provider-material Secret left by the
	// pre-activation behavior is removed here, after its provider has been realized above
	// (control-plane.spec.md "AgentRuntime Reconciliation", cutover prune). The
	// gateway-identity Secret stays because its source remains in desiredSecrets.
	if err := r.pruneSecrets(ctx, ar, namespace, desiredSecrets); err != nil {
		return r.fail(ctx, ar, fmt.Errorf("prune secrets: %w", err))
	}

	cronJob := buildCoordinatorCronJob(ar, namespace, secretSources)
	if err := applyCronJob(ctx, r.kube, namespace, cronJob); err != nil {
		return r.fail(ctx, ar, fmt.Errorf("apply cronjob: %w", err))
	}

	r.setStatus(ctx, ar, agentRuntimeStatusRunning, nil)
	log.Printf("INFO AgentRuntime %s provisioned in namespace %s", ar.GetMetadata().GetId(), namespace)
	return nil
}

// classifySecretSources splits SecretSources into those placed as Kubernetes Secrets
// and those realized as OpenShell providers. When no ProviderRealizer is wired (issue
// #526) every source is placed as a Secret - the transitional behavior that keeps the
// live fleet working until provider realization is available. When a realizer is
// wired, only the coordinator gateway-identity is placed as a Secret; everything else
// is provider material realized as providers.
func (r *AgentRuntimeReconciler) classifySecretSources(sources []*pb.SecretSource) (placed, providers []*pb.SecretSource) {
	if r.providers == nil {
		return sources, nil
	}
	for _, s := range sources {
		if isCoordinatorIdentitySource(s) {
			placed = append(placed, s)
		} else {
			providers = append(providers, s)
		}
	}
	return placed, providers
}

// realizeProviders reads each provider SecretSource's material cluster-local and
// hands the set to the ProviderRealizer. A material absent from a reachable store
// holds the runtime at Provisioning with an actionable SecretPending message (gating
// provider creation); an unreachable store degrades; a realizer error degrades.
func (r *AgentRuntimeReconciler) realizeProviders(ctx context.Context, ar *pb.AgentRuntime, sources []*pb.SecretSource) error {
	materials := make([]ProviderMaterial, 0, len(sources))
	for _, source := range sources {
		material, err := r.readMaterial(ctx, source)
		if err != nil {
			if errors.Is(err, secretstore.ErrNotFound) {
				return r.holdSecretPending(ctx, ar, source, err)
			}
			msg := fmt.Sprintf("SecretSource %q: provider material read failed", source.GetName())
			r.setStatus(ctx, ar, agentRuntimeStatusDegraded, &msg)
			log.Printf("WARN AgentRuntime %s degraded: %s", ar.GetMetadata().GetId(), msg)
			return fmt.Errorf("reconcile agent runtime %s: read provider material for %q: %w", ar.GetMetadata().GetId(), source.GetName(), err)
		}
		materials = append(materials, ProviderMaterial{Source: source, Material: material})
	}
	if err := r.providers.RealizeProviders(ctx, ar, materials); err != nil {
		msg := "provider realization failed"
		r.setStatus(ctx, ar, agentRuntimeStatusDegraded, &msg)
		log.Printf("WARN AgentRuntime %s degraded: %s", ar.GetMetadata().GetId(), msg)
		return fmt.Errorf("reconcile agent runtime %s: realize providers: %w", ar.GetMetadata().GetId(), err)
	}
	return nil
}

// readMaterial reads one SecretSource's durable material from the store (read-only).
// It preserves the error chain (including a wrapped ErrNotFound) so the caller can
// distinguish not-yet-uploaded material (SecretPending) from a store failure. It
// never returns the material in an error.
func (r *AgentRuntimeReconciler) readMaterial(ctx context.Context, source *pb.SecretSource) (string, error) {
	if r.secrets == nil {
		return "", fmt.Errorf("no secret store client configured; cannot read provider material")
	}
	value, err := r.secrets.Get(ctx, source.GetPath())
	if err != nil {
		return "", fmt.Errorf("read provider material from store: %w", err)
	}
	return value, nil
}

// holdSecretPending records the benign, self-healing SecretPending state: a reachable
// store that does not yet hold the referenced material. It holds at Provisioning with
// an actionable instruction (resolved cluster + exact mount/path), places no empty
// credential, does not mark the workspace Running, and returns a retryable error so
// the reconcile queue converges the moment the material appears (no manual trigger).
func (r *AgentRuntimeReconciler) holdSecretPending(ctx context.Context, ar *pb.AgentRuntime, source *pb.SecretSource, cause error) error {
	msg := secretPendingMessage(r.clusterName, r.secretStoreMount, source)
	r.setStatus(ctx, ar, agentRuntimeStatusProvisioning, &msg)
	log.Printf("INFO AgentRuntime %s: SecretPending for %q: %s", ar.GetMetadata().GetId(), source.GetName(), msg)
	return fmt.Errorf("reconcile agent runtime %s: secret %q pending upload: %w", ar.GetMetadata().GetId(), source.GetName(), cause)
}

// placeSecret reads one SecretSource value from the store and places it directly
// into a Kubernetes Secret. It never writes to the store (read-only direction). It
// returns a classified, value-free reason (safe to persist in the AgentRuntime
// status) and a detailed error (used only to requeue); neither ever carries bytes of
// the secret value.
func (r *AgentRuntimeReconciler) placeSecret(ctx context.Context, ar *pb.AgentRuntime, namespace string, source *pb.SecretSource) (reason string, err error) {
	if r.secrets == nil {
		return "no secret store configured", fmt.Errorf("no secret store client configured; cannot read secret value")
	}
	value, err := r.secrets.Get(ctx, source.GetPath())
	if err != nil {
		// Preserve the error chain (including a wrapped ErrNotFound) so the caller can
		// classify a not-yet-uploaded value (SecretPending) apart from a store failure.
		// The reason is only used for the Degraded status; the pending branch builds its
		// own actionable message, so no reason is returned for ErrNotFound.
		if errors.Is(err, secretstore.ErrNotFound) {
			return "", fmt.Errorf("read value from store: %w", err)
		}
		return "read from store failed", fmt.Errorf("read value from store: %w", err)
	}
	desired, err := buildSecret(ar, namespace, source, value)
	if err != nil {
		return "malformed value", fmt.Errorf("build secret: %w", err)
	}
	if err := applySecret(ctx, r.kube, namespace, desired); err != nil {
		return "placement failed", fmt.Errorf("place secret: %w", err)
	}
	return "", nil
}

// secretPendingMessage renders the actionable SecretPending status message: the
// resolved cluster the agent is assigned to and the exact store location to upload the
// value to (the KV v2 mount plus the SecretSource logical path, which is the physical
// policy path secret/data/hypershell/secret-sources/*). It is phrased as an
// instruction, not a bare error (agent-secret-late-binding.spec.md "Discoverable
// Upload Target"). It carries only safe references (name, cluster, path), never any
// bytes of the secret value.
func secretPendingMessage(clusterName, mount string, source *pb.SecretSource) string {
	return fmt.Sprintf("SecretPending: upload SecretSource %q value to %s/data/%s on cluster %s", source.GetName(), mount, source.GetPath(), clusterName)
}

// handleDelete writes Terminating (best-effort; the row may already be gone) and
// removes the cluster resources by deleting the namespace, which cascades.
func (r *AgentRuntimeReconciler) handleDelete(ctx context.Context, ar *pb.AgentRuntime, namespace string) error {
	log.Printf("INFO AgentRuntime %s deleted, removing resources in namespace %s", ar.GetMetadata().GetId(), namespace)
	r.setStatus(ctx, ar, agentRuntimeStatusTerminating, nil)
	// Remove the gateway-side OpenShell providers when a realizer is wired. This is
	// best-effort: a failure here is logged and must not block cluster-side cleanup
	// (the namespace delete), so a gateway outage cannot strand the namespace. The
	// error is surfaced in the log, not silently swallowed.
	if r.providers != nil {
		if err := r.providers.DeleteProviders(ctx, ar); err != nil {
			log.Printf("WARN AgentRuntime %s: delete gateway providers failed (continuing with cluster cleanup): %v", ar.GetMetadata().GetId(), err)
		}
	}
	if err := deleteAgentRuntimeNamespace(ctx, r.kube, namespace); err != nil {
		return fmt.Errorf("delete agent runtime %s namespace %s: %w", ar.GetMetadata().GetId(), namespace, err)
	}
	return nil
}

// pruneSecrets deletes Secrets owned by this AgentRuntime whose SecretSource is no
// longer declared, so removing a SecretSource removes its placed Secret rather than
// leaking it.
func (r *AgentRuntimeReconciler) pruneSecrets(ctx context.Context, ar *pb.AgentRuntime, namespace string, desired map[string]struct{}) error {
	client := r.kube.CoreV1().Secrets(namespace)
	list, err := client.List(ctx, metav1.ListOptions{LabelSelector: agentRuntimeIDLabel + "=" + ar.GetMetadata().GetId()})
	if err != nil {
		// A list failure must not strand provisioning; it is logged and tolerated.
		// The next reconcile retries the prune.
		log.Printf("WARN AgentRuntime %s: unable to list secrets for prune in %s: %v", ar.GetMetadata().GetId(), namespace, err)
		return nil
	}
	for i := range list.Items {
		name := list.Items[i].GetName()
		if _, keep := desired[name]; keep {
			continue
		}
		if err := client.Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
			return fmt.Errorf("delete stale secret %s/%s: %w", namespace, name, err)
		}
		log.Printf("INFO AgentRuntime %s: pruned stale secret %s/%s", ar.GetMetadata().GetId(), namespace, name)
	}
	return nil
}

// fail logs the error, marks the runtime Degraded with the error message, and
// returns the error so the watch queue retries it.
func (r *AgentRuntimeReconciler) fail(ctx context.Context, ar *pb.AgentRuntime, err error) error {
	msg := err.Error()
	r.setStatus(ctx, ar, agentRuntimeStatusDegraded, &msg)
	return fmt.Errorf("reconcile agent runtime %s: %w", ar.GetMetadata().GetId(), err)
}

// setStatus writes a status transition back to the API server, skipping the write
// when the runtime already carries the desired status and message (the client-side
// guard against a status write-back loop). A write failure is logged, not
// propagated: status is observability, and the next reconcile retries it.
func (r *AgentRuntimeReconciler) setStatus(ctx context.Context, ar *pb.AgentRuntime, status string, message *string) {
	if r.statusWriter == nil {
		return
	}
	if ar.GetStatus() == status && stringPtrValueEqual(ar.StatusMessage, message) {
		return
	}
	if err := r.statusWriter(ctx, ar.GetMetadata().GetId(), status, message); err != nil {
		log.Printf("WARN AgentRuntime %s: failed to write status %q: %v", ar.GetMetadata().GetId(), status, err)
	}
}

// writeStatusViaGRPC is the production status writer.
func (r *AgentRuntimeReconciler) writeStatusViaGRPC(ctx context.Context, id, status string, message *string) error {
	client := pb.NewAgentRuntimeServiceClient(r.grpcConn)
	if _, err := client.UpdateAgentRuntimeStatus(ctx, &pb.UpdateAgentRuntimeStatusRequest{
		Id:            id,
		Status:        status,
		StatusMessage: message,
	}); err != nil {
		return fmt.Errorf("update agent runtime %s status to %q: %w", id, status, err)
	}
	return nil
}

// stringPtrValueEqual compares an observed optional string (the proto getter
// returns the pointer) with a desired one, treating nil and empty consistently
// with the server-side guard.
func stringPtrValueEqual(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
