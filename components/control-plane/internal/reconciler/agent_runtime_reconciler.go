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

// AgentRuntimeReconciler reconciles AgentRuntime cluster-side scheduling resources
// (issue #521 Wave B, requirements #3 and #5). On an upsert it applies the
// namespace, non-auto-mounting ServiceAccount, default-deny-ingress NetworkPolicy,
// one Kubernetes Secret per SecretSource, and the coordinator CronJob to the target
// cluster, then writes the status lifecycle (Pending -> Provisioning ->
// Running/Degraded) back to the API server. On a delete it writes Terminating and
// removes the resources.
//
// Secret placement is direct and one-shot: for each SecretSource the reconciler
// reads the referenced value from the configured secret store through its read-only
// pkg/secretstore client and writes a Kubernetes Secret itself. It does NOT create
// an ExternalSecret and does NOT depend on External Secrets Operator, and it runs no
// ESO-style periodic resync or rotation of the placed value (security.spec.md
// "Secret Store Access Is Single-Directional per Component"; control-plane.spec.md
// "AgentRuntime Cluster Resource Provisioning").
//
// Gateway-side workspace provisioning (requirement #4, data-model.spec.md
// "Gateway-Side Provisioning") is NOT performed here: it is blocked on the gateway
// workspace API design (issue #526) and is therefore excluded from Wave B. The
// Running transition here reflects cluster-side success only; once #4 lands it will
// additionally gate on the AgentWorkspace being provisioned.
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
	// SecretSource value from before placing it as a Kubernetes Secret. It is never
	// used to write to the store. A disabled store (the default when none is
	// configured) rejects reads, so an AgentRuntime that declares a SecretSource is
	// degraded until a store is configured; an AgentRuntime with no SecretSource is
	// unaffected.
	secrets secretstore.Reader

	grpcConn  *grpc.ClientConn
	clusterID string

	// secretStoreMount is the KV v2 mount the agent-secret store is served under. It
	// is used only to render the actionable SecretPending upload path (mount + logical
	// path) in the status message; it never changes where a value is read from (that is
	// the configured read client). Defaults to "secret".
	secretStoreMount string

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
	r := &AgentRuntimeReconciler{
		kube:             kube,
		secrets:          secrets,
		grpcConn:         grpcConn,
		clusterID:        clusterID,
		secretStoreMount: mount,
	}
	if grpcConn != nil {
		r.statusWriter = r.writeStatusViaGRPC
	}
	return r
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

	// One Kubernetes Secret per SecretSource, placed directly from the value read
	// out of the secret store (no ExternalSecret, no ESO). Track the names so a
	// Secret whose SecretSource was removed is pruned. A read or placement failure
	// degrades the runtime and surfaces the failing SecretSource by name
	// (control-plane.spec.md "SecretSource read or placement failure degrades the
	// AgentRuntime"); since placement is synchronous the result is known on this
	// pass, so there is no separate operator-driven sync state to poll.
	desiredSecrets := map[string]struct{}{}
	for _, source := range ar.GetSecretSources() {
		// placeSecret returns a classified, value-free reason for the persisted status
		// and a detailed error (used only for the requeue). The status message carries
		// just the SecretSource name (a safe reference) and the reason, never any bytes
		// of the secret value (security.spec.md: a raw stored value never appears in
		// logs, events, or the database).
		reason, err := r.placeSecret(ctx, ar, namespace, source)
		if err != nil {
			// Reachable store, value not uploaded yet: a benign, self-healing
			// SecretPending state, NOT a failure. Hold at Provisioning with an
			// actionable instruction (resolved cluster + exact mount/path), place no
			// empty credential, do not mark the workspace Running, and return so the
			// reconcile queue retries on its cadence and converges the moment the value
			// appears (agent-secret-late-binding.spec.md "SecretPending Condition and
			// Self-Healing Retry"; control-plane.spec.md "AgentRuntime Reconciliation").
			if errors.Is(err, secretstore.ErrNotFound) {
				msg := secretPendingMessage(r.clusterID, r.secretStoreMount, source)
				r.setStatus(ctx, ar, agentRuntimeStatusProvisioning, &msg)
				log.Printf("INFO AgentRuntime %s: SecretPending for %q: %s", ar.GetMetadata().GetId(), source.GetName(), msg)
				return fmt.Errorf("reconcile agent runtime %s: secret %q pending upload: %w", ar.GetMetadata().GetId(), source.GetName(), err)
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
	if err := r.pruneSecrets(ctx, ar, namespace, desiredSecrets); err != nil {
		return r.fail(ctx, ar, fmt.Errorf("prune secrets: %w", err))
	}

	cronJob := buildCoordinatorCronJob(ar, namespace, ar.GetSecretSources())
	if err := applyCronJob(ctx, r.kube, namespace, cronJob); err != nil {
		return r.fail(ctx, ar, fmt.Errorf("apply cronjob: %w", err))
	}

	r.setStatus(ctx, ar, agentRuntimeStatusRunning, nil)
	log.Printf("INFO AgentRuntime %s provisioned in namespace %s", ar.GetMetadata().GetId(), namespace)
	return nil
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
		if errors.Is(err, secretstore.ErrNotFound) {
			return "value not yet uploaded", fmt.Errorf("read value from store: %w", err)
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
func secretPendingMessage(clusterID, mount string, source *pb.SecretSource) string {
	return fmt.Sprintf("SecretPending: upload SecretSource %q value to %s/data/%s on cluster %s", source.GetName(), mount, source.GetPath(), clusterID)
}

// handleDelete writes Terminating (best-effort; the row may already be gone) and
// removes the cluster resources by deleting the namespace, which cascades.
func (r *AgentRuntimeReconciler) handleDelete(ctx context.Context, ar *pb.AgentRuntime, namespace string) error {
	log.Printf("INFO AgentRuntime %s deleted, removing resources in namespace %s", ar.GetMetadata().GetId(), namespace)
	r.setStatus(ctx, ar, agentRuntimeStatusTerminating, nil)
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
