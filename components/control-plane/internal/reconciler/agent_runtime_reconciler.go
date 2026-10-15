package reconciler

import (
	"context"
	"fmt"
	"log"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/control-plane/internal/watcher"
	"google.golang.org/grpc"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
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
// one ExternalSecret per SecretSource, and the coordinator CronJob to the target
// cluster, then writes the status lifecycle (Pending -> Provisioning ->
// Running/Degraded) back to the API server. On a delete it writes Terminating and
// removes the resources.
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
	// kube and dyn are the target cluster's typed and dynamic clients. They are
	// nil when the control plane has no in-cluster Kubernetes access, in which case
	// cluster-side provisioning is skipped (mirroring the other reconcilers).
	kube kubernetes.Interface
	dyn  dynamic.Interface

	grpcConn  *grpc.ClientConn
	clusterID string

	// statusWriter persists an AgentRuntime status transition. It is overridable in
	// tests; in production it writes through the AgentRuntime gRPC service. A nil
	// writer disables status write-back (no gRPC connection).
	statusWriter func(ctx context.Context, id, status string, message *string) error
}

// NewAgentRuntimeReconciler builds the Wave B reconciler. kube and dyn may be nil
// when the control plane has no cluster access; grpcConn may be nil in tests that
// inject their own status writer.
func NewAgentRuntimeReconciler(kube kubernetes.Interface, dyn dynamic.Interface, grpcConn *grpc.ClientConn, clusterID string) *AgentRuntimeReconciler {
	r := &AgentRuntimeReconciler{
		kube:      kube,
		dyn:       dyn,
		grpcConn:  grpcConn,
		clusterID: clusterID,
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

	if r.kube == nil || r.dyn == nil {
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
// Running (cluster-side success) or Degraded (a SecretSource failed to sync).
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

	// One ExternalSecret per SecretSource. Track the names so a stale ExternalSecret
	// whose SecretSource was removed is pruned.
	desiredSecrets := map[string]struct{}{}
	for _, source := range ar.GetSecretSources() {
		obj, err := buildExternalSecret(ar, namespace, source)
		if err != nil {
			return r.fail(ctx, ar, fmt.Errorf("build external secret for %q: %w", source.GetName(), err))
		}
		if err := applyExternalSecret(ctx, r.dyn, namespace, obj); err != nil {
			return r.fail(ctx, ar, fmt.Errorf("apply external secret for %q: %w", source.GetName(), err))
		}
		desiredSecrets[obj.GetName()] = struct{}{}
	}
	if err := r.pruneExternalSecrets(ctx, ar, namespace, desiredSecrets); err != nil {
		return r.fail(ctx, ar, fmt.Errorf("prune external secrets: %w", err))
	}

	cronJob := buildCoordinatorCronJob(ar, namespace, ar.GetSecretSources())
	if err := applyCronJob(ctx, r.kube, namespace, cronJob); err != nil {
		return r.fail(ctx, ar, fmt.Errorf("apply cronjob: %w", err))
	}

	// Evaluate the SecretSources' ExternalSecret sync state. The sync result lives on
	// a separate ExternalSecret object that fires no AgentRuntime watch event when it
	// changes, so neither a failure nor a not-yet-synced secret would re-drive this
	// runtime on its own. Both cases therefore return a retryable error so the watch
	// queue requeues with backoff until the secrets reach a terminal Ready state.
	syncState := r.evaluateSecretSyncs(ctx, ar, namespace)
	switch {
	case syncState.degraded:
		// A SecretSource failed to sync (data-model.spec.md "SecretSource Sync
		// Failure Degrades AgentRuntime"). Mark Degraded and requeue so recovery is
		// observed and the runtime is promoted to Running once the secret syncs.
		msg := fmt.Sprintf("SecretSource %q failed to sync: %s", syncState.failedSource, syncState.failedMessage)
		r.setStatus(ctx, ar, agentRuntimeStatusDegraded, &msg)
		log.Printf("WARN AgentRuntime %s degraded: %s", ar.GetMetadata().GetId(), msg)
		return fmt.Errorf("agent runtime %s degraded: %s", ar.GetMetadata().GetId(), msg)
	case syncState.pending:
		// At least one just-created ExternalSecret has not yet reported a successful
		// sync. Keep the runtime out of Running (it stays Provisioning) and return a
		// retryable error: latching Running here would leave the runtime stuck
		// Running with a broken coordinator if the secret's sync then fails
		// permanently, because no AgentRuntime event would ever re-arm the check.
		log.Printf("INFO AgentRuntime %s waiting on SecretSource %q to sync; requeueing", ar.GetMetadata().GetId(), syncState.pendingSource)
		return fmt.Errorf("agent runtime %s waiting on secret source %q to sync", ar.GetMetadata().GetId(), syncState.pendingSource)
	}

	r.setStatus(ctx, ar, agentRuntimeStatusRunning, nil)
	log.Printf("INFO AgentRuntime %s provisioned in namespace %s", ar.GetMetadata().GetId(), namespace)
	return nil
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

// pruneExternalSecrets deletes ExternalSecrets owned by this AgentRuntime whose
// SecretSource is no longer declared, so removing a SecretSource removes its synced
// secret rather than leaking it.
func (r *AgentRuntimeReconciler) pruneExternalSecrets(ctx context.Context, ar *pb.AgentRuntime, namespace string, desired map[string]struct{}) error {
	client := r.dyn.Resource(externalSecretGVR).Namespace(namespace)
	list, err := client.List(ctx, metav1.ListOptions{LabelSelector: agentRuntimeIDLabel + "=" + ar.GetMetadata().GetId()})
	if err != nil {
		// A missing ExternalSecret CRD or list failure must not strand provisioning;
		// it is logged and tolerated. The next reconcile retries the prune.
		log.Printf("WARN AgentRuntime %s: unable to list external secrets for prune in %s: %v", ar.GetMetadata().GetId(), namespace, err)
		return nil
	}
	for i := range list.Items {
		name := list.Items[i].GetName()
		if _, keep := desired[name]; keep {
			continue
		}
		if err := client.Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
			return fmt.Errorf("delete stale external secret %s/%s: %w", namespace, name, err)
		}
		log.Printf("INFO AgentRuntime %s: pruned stale external secret %s/%s", ar.GetMetadata().GetId(), namespace, name)
	}
	return nil
}

// agentRuntimeSecretSyncState is the aggregate observed sync state of an
// AgentRuntime's SecretSources' ExternalSecrets.
type agentRuntimeSecretSyncState struct {
	// degraded is set when a SecretSource's ExternalSecret reports a sync failure.
	degraded      bool
	failedSource  string
	failedMessage string
	// pending is set when at least one SecretSource's ExternalSecret has not yet
	// confirmed a successful (Ready=True) sync and none has failed - the new-runtime
	// case where the secret was just created. It must requeue rather than latch
	// Running (see handleUpsert).
	pending       bool
	pendingSource string
}

// evaluateSecretSyncs inspects each SecretSource's ExternalSecret and returns the
// aggregate sync state. A failure takes precedence over a pending sync; a
// missing/statusless or not-yet-Ready ExternalSecret is pending (still syncing),
// not failed.
func (r *AgentRuntimeReconciler) evaluateSecretSyncs(ctx context.Context, ar *pb.AgentRuntime, namespace string) agentRuntimeSecretSyncState {
	client := r.dyn.Resource(externalSecretGVR).Namespace(namespace)
	var state agentRuntimeSecretSyncState
	for _, source := range ar.GetSecretSources() {
		obj, err := client.Get(ctx, secretName(source), metav1.GetOptions{})
		if err != nil {
			// Not readable yet (just created / not found): still syncing.
			if !state.pending {
				state.pending = true
				state.pendingSource = source.GetName()
			}
			continue
		}
		if failed, msg := externalSecretSyncFailed(obj); failed {
			state.degraded = true
			state.failedSource = source.GetName()
			state.failedMessage = msg
			return state
		}
		if !externalSecretSyncReady(obj) && !state.pending {
			state.pending = true
			state.pendingSource = source.GetName()
		}
	}
	return state
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
