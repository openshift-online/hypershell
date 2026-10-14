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
		active: make(map[string]struct{}),
		kube:   kube,
		dyn:    dyn,
		statusWriter: func(_ context.Context, id, status string, message *string) error {
			*writes = append(*writes, statusWrite{id: id, status: status, message: message})
			return nil
		},
	}
	return r, writes
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
	r, writes := newTestReconciler()
	ar := testAgentRuntime()
	ns := agentRuntimeNamespace(ar)

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
	r, writes := newTestReconciler()
	ar := testAgentRuntime()
	ns := agentRuntimeNamespace(ar)

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
	r, _ := newTestReconciler()
	ar := testAgentRuntime()

	for i := 0; i < 2; i++ {
		if err := r.Handle(context.Background(), watcher.Event[*pb.AgentRuntime]{
			Type: watcher.EventUpdated, ResourceID: ar.GetMetadata().GetId(), Resource: ar,
		}); err != nil {
			t.Fatalf("Handle pass %d returned error: %v", i, err)
		}
	}

	ns := agentRuntimeNamespace(ar)
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
	r, writes := newTestReconciler()
	ar := testAgentRuntime()
	running := agentRuntimeStatusRunning
	ar.Status = &running

	if err := r.Handle(context.Background(), watcher.Event[*pb.AgentRuntime]{
		Type: watcher.EventUpdated, ResourceID: ar.GetMetadata().GetId(), Resource: ar,
	}); err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}

	if len(*writes) != 0 {
		t.Fatalf("expected no status writes for a converged Running runtime, got %d: %+v", len(*writes), *writes)
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
