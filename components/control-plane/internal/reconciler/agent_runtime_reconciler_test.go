package reconciler

import (
	"context"
	"strings"
	"testing"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/control-plane/internal/watcher"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"
)

type statusWrite struct {
	id      string
	status  string
	message *string
}

// newTestReconciler builds a reconciler backed by fake Kubernetes clients and a
// recording status writer.
func newTestReconciler(objects ...runtime.Object) (*AgentRuntimeReconciler, *[]statusWrite) {
	kube := fake.NewSimpleClientset()
	dyn := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{externalSecretGVR: "ExternalSecretList"},
		objects...,
	)
	writes := &[]statusWrite{}
	r := &AgentRuntimeReconciler{
		kube: kube,
		dyn:  dyn,
		statusWriter: func(_ context.Context, id, status string, message *string) error {
			*writes = append(*writes, statusWrite{id: id, status: status, message: message})
			return nil
		},
	}
	return r, writes
}

// readyExternalSecret builds an ExternalSecret for a SecretSource whose Ready
// condition is True, modelling a secret the external-secrets operator has already
// synced. The reconciler promotes an AgentRuntime to Running only once every
// SecretSource reports Ready=True, so tests that expect Running seed these.
func readyExternalSecret(ar *pb.AgentRuntime, source *pb.SecretSource, ns string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": externalSecretGVR.Group + "/" + externalSecretGVR.Version,
		"kind":       "ExternalSecret",
		"metadata": map[string]interface{}{
			"name":      secretName(source),
			"namespace": ns,
			"labels":    map[string]interface{}{agentRuntimeIDLabel: ar.GetMetadata().GetId()},
		},
		"status": map[string]interface{}{
			"conditions": []interface{}{
				map[string]interface{}{"type": "Ready", "status": "True", "reason": "SecretSynced", "message": "secret synced"},
			},
		},
	}}
}

// readyExternalSecrets builds a Ready ExternalSecret for every SecretSource on ar.
func readyExternalSecrets(ar *pb.AgentRuntime, ns string) []runtime.Object {
	objs := make([]runtime.Object, 0, len(ar.GetSecretSources()))
	for _, s := range ar.GetSecretSources() {
		objs = append(objs, readyExternalSecret(ar, s, ns))
	}
	return objs
}

func testAgentRuntime() *pb.AgentRuntime {
	cron := "0 * * * *"
	image := "registry.example.com/coordinator:latest"
	return &pb.AgentRuntime{
		Metadata:         &pb.ObjectReference{Id: "ar-123"},
		Name:             "amber-reviews",
		Cron:             &cron,
		CoordinatorImage: &image,
		SecretSources: []*pb.SecretSource{
			{Name: "gateway-id", Purpose: "gateway-identity", Backend: "vault", Path: "secret/gateway"},
			{Name: "github-key", Purpose: "app-private-key", Backend: "vault", Path: "secret/github"},
		},
	}
}

func TestAgentRuntimeReconcile_CreatesClusterResources(t *testing.T) {
	ar := testAgentRuntime()
	ns := agentRuntimeNamespace(ar)
	// Seed each SecretSource's ExternalSecret as already synced so the reconcile
	// reaches Running rather than requeueing for a pending sync.
	r, writes := newTestReconciler(readyExternalSecrets(ar, ns)...)

	if err := r.Handle(context.Background(), watcher.Event[*pb.AgentRuntime]{
		Type: watcher.EventUpdated, ResourceID: ar.GetMetadata().GetId(), Resource: ar,
	}); err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}

	ctx := context.Background()
	if _, err := r.kube.CoreV1().Namespaces().Get(ctx, ns, metav1.GetOptions{}); err != nil {
		t.Fatalf("namespace not created: %v", err)
	}

	sa, err := r.kube.CoreV1().ServiceAccounts(ns).Get(ctx, agentServiceAccountName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("service account not created: %v", err)
	}
	if sa.AutomountServiceAccountToken == nil || *sa.AutomountServiceAccountToken {
		t.Fatalf("service account must disable token auto-mount")
	}

	np, err := r.kube.NetworkingV1().NetworkPolicies(ns).Get(ctx, agentDenyIngressPolicyName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("network policy not created: %v", err)
	}
	if len(np.Spec.Ingress) != 0 {
		t.Fatalf("network policy must deny all ingress, got %d rules", len(np.Spec.Ingress))
	}

	cj, err := r.kube.BatchV1().CronJobs(ns).Get(ctx, agentCronJobName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("cronjob not created: %v", err)
	}
	if cj.Spec.Schedule != *ar.Cron {
		t.Fatalf("cronjob schedule = %q, want %q", cj.Spec.Schedule, *ar.Cron)
	}
	container := cj.Spec.JobTemplate.Spec.Template.Spec.Containers[0]
	if container.Image != *ar.CoordinatorImage {
		t.Fatalf("coordinator image = %q, want %q", container.Image, *ar.CoordinatorImage)
	}
	// gateway-identity wires 5 env vars; app-private-key wires 1 (file path).
	if gotEnv := len(container.Env); gotEnv != 6 {
		t.Fatalf("coordinator env count = %d, want 6", gotEnv)
	}
	if !hasEnv(container.Env, "OPENSHELL_OIDC_CLIENT_SECRET") {
		t.Fatalf("missing gateway-identity env var OPENSHELL_OIDC_CLIENT_SECRET")
	}
	if !hasEnv(container.Env, "APP_PRIVATE_KEY_FILE") {
		t.Fatalf("missing app-private-key env var APP_PRIVATE_KEY_FILE")
	}

	// One ExternalSecret per SecretSource.
	esList, err := r.dyn.Resource(externalSecretGVR).Namespace(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("list external secrets: %v", err)
	}
	if len(esList.Items) != 2 {
		t.Fatalf("external secret count = %d, want 2", len(esList.Items))
	}

	// Status lifecycle: Provisioning then Running.
	if len(*writes) < 2 {
		t.Fatalf("expected at least 2 status writes, got %d", len(*writes))
	}
	if (*writes)[0].status != agentRuntimeStatusProvisioning {
		t.Fatalf("first status = %q, want Provisioning", (*writes)[0].status)
	}
	last := (*writes)[len(*writes)-1]
	if last.status != agentRuntimeStatusRunning {
		t.Fatalf("final status = %q, want Running", last.status)
	}
}

func TestAgentRuntimeReconcile_DegradedOnSecretSyncFailure(t *testing.T) {
	ar := testAgentRuntime()
	ns := agentRuntimeNamespace(ar)
	// Seed a failing ExternalSecret for the first source so the reconciler observes
	// the sync failure on apply.
	failing := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": externalSecretGVR.Group + "/" + externalSecretGVR.Version,
		"kind":       "ExternalSecret",
		"metadata": map[string]interface{}{
			"name":      secretName(ar.SecretSources[0]),
			"namespace": ns,
			"labels":    map[string]interface{}{agentRuntimeIDLabel: ar.GetMetadata().GetId()},
		},
		"status": map[string]interface{}{
			"conditions": []interface{}{
				map[string]interface{}{"type": "Ready", "status": "False", "reason": "SecretSyncedError", "message": "backend path not found"},
			},
		},
	}}
	r, writes := newTestReconciler(failing)

	// A sync failure returns a retryable error so the watch queue requeues the
	// runtime and self-heals once the secret syncs.
	if err := r.Handle(context.Background(), watcher.Event[*pb.AgentRuntime]{
		Type: watcher.EventUpdated, ResourceID: ar.GetMetadata().GetId(), Resource: ar,
	}); err == nil {
		t.Fatalf("expected a retryable error on secret sync failure")
	}

	last := (*writes)[len(*writes)-1]
	if last.status != agentRuntimeStatusDegraded {
		t.Fatalf("final status = %q, want Degraded", last.status)
	}
	if last.message == nil || !strings.Contains(*last.message, "gateway-id") {
		t.Fatalf("degraded message must name the failing SecretSource, got %v", last.message)
	}
}

func TestAgentRuntimeReconcile_DeleteRemovesNamespace(t *testing.T) {
	ar := testAgentRuntime()
	ns := agentRuntimeNamespace(ar)
	r, writes := newTestReconciler(readyExternalSecrets(ar, ns)...)

	if err := r.Handle(context.Background(), watcher.Event[*pb.AgentRuntime]{
		Type: watcher.EventUpdated, ResourceID: ar.GetMetadata().GetId(), Resource: ar,
	}); err != nil {
		t.Fatalf("provision: %v", err)
	}

	if err := r.Handle(context.Background(), watcher.Event[*pb.AgentRuntime]{
		Type: watcher.EventDeleted, ResourceID: ar.GetMetadata().GetId(), Resource: ar,
	}); err != nil {
		t.Fatalf("delete: %v", err)
	}

	if _, err := r.kube.CoreV1().Namespaces().Get(context.Background(), ns, metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("namespace should be deleted, got err=%v", err)
	}
	// A Terminating status was written during delete.
	foundTerminating := false
	for _, w := range *writes {
		if w.status == agentRuntimeStatusTerminating {
			foundTerminating = true
		}
	}
	if !foundTerminating {
		t.Fatalf("expected a Terminating status write during delete")
	}
}

func TestAgentRuntimeReconcile_Idempotent(t *testing.T) {
	ar := testAgentRuntime()
	ns := agentRuntimeNamespace(ar)
	r, _ := newTestReconciler(readyExternalSecrets(ar, ns)...)

	for i := 0; i < 2; i++ {
		if err := r.Handle(context.Background(), watcher.Event[*pb.AgentRuntime]{
			Type: watcher.EventUpdated, ResourceID: ar.GetMetadata().GetId(), Resource: ar,
		}); err != nil {
			t.Fatalf("Handle pass %d returned error: %v", i, err)
		}
	}

	esList, err := r.dyn.Resource(externalSecretGVR).Namespace(ns).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		t.Fatalf("list external secrets: %v", err)
	}
	if len(esList.Items) != 2 {
		t.Fatalf("external secret count after re-reconcile = %d, want 2", len(esList.Items))
	}
}

// A reconcile of an already-Running runtime whose secrets are healthy must write
// no status (no Provisioning regression, no redundant Running), so the status
// write-back does not become a self-triggering hot loop.
func TestAgentRuntimeReconcile_RunningConvergesWithoutStatusWrite(t *testing.T) {
	ar := testAgentRuntime()
	ns := agentRuntimeNamespace(ar)
	running := agentRuntimeStatusRunning
	ar.Status = &running
	// Secrets are already synced, so the converged reconcile confirms Running and
	// writes nothing.
	r, writes := newTestReconciler(readyExternalSecrets(ar, ns)...)

	if err := r.Handle(context.Background(), watcher.Event[*pb.AgentRuntime]{
		Type: watcher.EventUpdated, ResourceID: ar.GetMetadata().GetId(), Resource: ar,
	}); err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}

	if len(*writes) != 0 {
		t.Fatalf("expected no status writes for a converged Running runtime, got %d: %+v", len(*writes), *writes)
	}
}

// A freshly provisioned runtime whose ExternalSecrets have not yet reported a
// successful sync must NOT latch Running: it requeues (returns a retryable error)
// so a sync that later fails is still observed, instead of sitting Running with a
// broken coordinator.
func TestAgentRuntimeReconcile_FreshSecretsPendingRequeues(t *testing.T) {
	r, writes := newTestReconciler()
	ar := testAgentRuntime()

	err := r.Handle(context.Background(), watcher.Event[*pb.AgentRuntime]{
		Type: watcher.EventUpdated, ResourceID: ar.GetMetadata().GetId(), Resource: ar,
	})
	if err == nil {
		t.Fatalf("expected a retryable error while secrets are still syncing")
	}
	for _, w := range *writes {
		if w.status == agentRuntimeStatusRunning {
			t.Fatalf("runtime must not be marked Running before its secrets sync")
		}
	}
	// It is stamped Provisioning (the in-progress marker), not Running/Degraded.
	if len(*writes) == 0 || (*writes)[0].status != agentRuntimeStatusProvisioning {
		t.Fatalf("expected a Provisioning status write, got %+v", *writes)
	}
}

// Once the external-secrets operator reports a successful sync on a later pass, the
// previously-pending runtime self-heals to Running - the requeue armed by the
// pending pass is what re-drives this check.
func TestAgentRuntimeReconcile_PendingSelfHealsToRunning(t *testing.T) {
	r, writes := newTestReconciler()
	ar := testAgentRuntime()
	ns := agentRuntimeNamespace(ar)
	ev := watcher.Event[*pb.AgentRuntime]{Type: watcher.EventUpdated, ResourceID: ar.GetMetadata().GetId(), Resource: ar}

	// First pass: secrets are still syncing, so the reconcile requeues.
	if err := r.Handle(context.Background(), ev); err == nil {
		t.Fatalf("expected a retryable error on the pending first pass")
	}

	// The operator reports Ready=True on both ExternalSecrets.
	client := r.dyn.Resource(externalSecretGVR).Namespace(ns)
	for _, s := range ar.SecretSources {
		obj, err := client.Get(context.Background(), secretName(s), metav1.GetOptions{})
		if err != nil {
			t.Fatalf("get external secret %s: %v", secretName(s), err)
		}
		if err := unstructured.SetNestedSlice(obj.Object, []interface{}{
			map[string]interface{}{"type": "Ready", "status": "True", "reason": "SecretSynced", "message": "synced"},
		}, "status", "conditions"); err != nil {
			t.Fatalf("set status: %v", err)
		}
		if _, err := client.Update(context.Background(), obj, metav1.UpdateOptions{}); err != nil {
			t.Fatalf("update external secret: %v", err)
		}
	}

	// Second pass (the armed requeue): secrets are synced, so it promotes to Running.
	if err := r.Handle(context.Background(), ev); err != nil {
		t.Fatalf("expected a clean reconcile once secrets synced, got %v", err)
	}
	last := (*writes)[len(*writes)-1]
	if last.status != agentRuntimeStatusRunning {
		t.Fatalf("final status = %q, want Running", last.status)
	}
}

// Two SecretSources whose synced-secret names share a long prefix must still
// produce distinct, DNS-1123-valid (<=63 char) pod volume names: truncating the
// "secret-" prefix away must not drop the disambiguating hash and collide them
// (which the CronJob API would reject).
func TestSecretFileMount_PreservesHashUniqueness(t *testing.T) {
	base := strings.Repeat("a", 54)
	// Model two secretName outputs that differ only in their trailing hash.
	s1 := base + "-00000001"
	s2 := base + "-00000002"

	v1, _ := secretFileMount(s1, "/mnt/one")
	v2, _ := secretFileMount(s2, "/mnt/two")
	n1, n2 := v1[0].Name, v2[0].Name

	if len(n1) > 63 || len(n2) > 63 {
		t.Fatalf("volume names must be <=63 chars, got %d and %d", len(n1), len(n2))
	}
	if n1 == n2 {
		t.Fatalf("volume names collided despite distinct secrets: %q", n1)
	}
	for _, n := range []string{n1, n2} {
		if dns1123Invalid.MatchString(n) || strings.HasPrefix(n, "-") || strings.HasSuffix(n, "-") {
			t.Fatalf("volume name %q is not a valid DNS-1123 label", n)
		}
	}
}

func TestAgentRuntimeNamespace_DNS1123(t *testing.T) {
	ar := &pb.AgentRuntime{Metadata: &pb.ObjectReference{Id: "AbC123XyZ"}}
	ns := agentRuntimeNamespace(ar)
	if ns != "agent-abc123xyz" {
		t.Fatalf("namespace = %q, want agent-abc123xyz", ns)
	}
}

func hasEnv(env []corev1.EnvVar, name string) bool {
	for _, e := range env {
		if e.Name == name {
			return true
		}
	}
	return false
}
