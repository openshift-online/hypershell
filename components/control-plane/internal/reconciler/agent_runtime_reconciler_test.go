package reconciler

import (
	"context"
	"strings"
	"testing"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/api-server/pkg/secretstore"
	"github.com/openshift-online/hypershell/components/control-plane/internal/watcher"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

type statusWrite struct {
	id      string
	status  string
	message *string
}

// gatewayIdentityValue is the JSON object a gateway-identity SecretSource stores:
// the control plane reads it and places each field as a Secret key the coordinator
// env vars reference.
const gatewayIdentityValue = `{"endpoint":"https://gw.example.com","issuer":"https://iss.example.com","client_id":"cid","audience":"aud","client_secret":"shh"}`

// newTestReconciler builds a reconciler backed by a fake Kubernetes client and a
// read-only secret store seeded with seed (path -> stored value), plus a recording
// status writer.
func newTestReconciler(seed map[string]string) (*AgentRuntimeReconciler, *[]statusWrite) {
	kube := fake.NewSimpleClientset()
	writes := &[]statusWrite{}
	r := &AgentRuntimeReconciler{
		kube:    kube,
		secrets: secretstore.NewMemoryReader(seed),
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

// seedForTestAgentRuntime returns the stored values for testAgentRuntime's sources.
func seedForTestAgentRuntime() map[string]string {
	return map[string]string{
		"secret/gateway": gatewayIdentityValue,
		"secret/github":  "-----BEGIN KEY-----\nabc\n-----END KEY-----",
	}
}

func TestAgentRuntimeReconcile_CreatesClusterResources(t *testing.T) {
	ar := testAgentRuntime()
	ns := agentRuntimeNamespace(ar)
	r, writes := newTestReconciler(seedForTestAgentRuntime())

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

	// One Kubernetes Secret per SecretSource, placed directly from the store.
	secList, err := r.kube.CoreV1().Secrets(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("list secrets: %v", err)
	}
	if len(secList.Items) != 2 {
		t.Fatalf("placed secret count = %d, want 2", len(secList.Items))
	}

	// The gateway-identity value is expanded into per-field keys.
	gwSecret, err := r.kube.CoreV1().Secrets(ns).Get(ctx, secretName(ar.SecretSources[0]), metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get gateway-identity secret: %v", err)
	}
	if got := string(gwSecret.Data["client_secret"]); got != "shh" {
		t.Fatalf("gateway-identity client_secret = %q, want shh", got)
	}
	// The app-private-key raw value lands under key.pem (what the mounted file uses).
	ghSecret, err := r.kube.CoreV1().Secrets(ns).Get(ctx, secretName(ar.SecretSources[1]), metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get app-private-key secret: %v", err)
	}
	if _, ok := ghSecret.Data["key.pem"]; !ok {
		t.Fatalf("app-private-key secret must carry key.pem, got keys %v", secretKeys(ghSecret))
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

// When a SecretSource value cannot be read from the store the runtime is degraded
// and the failing SecretSource is named (control-plane.spec.md "SecretSource read
// or placement failure degrades the AgentRuntime").
func TestAgentRuntimeReconcile_DegradedOnSecretReadFailure(t *testing.T) {
	ar := testAgentRuntime()
	// Seed only the second source so the first read fails.
	r, writes := newTestReconciler(map[string]string{"secret/github": "pem"})

	err := r.Handle(context.Background(), watcher.Event[*pb.AgentRuntime]{
		Type: watcher.EventUpdated, ResourceID: ar.GetMetadata().GetId(), Resource: ar,
	})
	if err == nil {
		t.Fatalf("expected a retryable error when a SecretSource value cannot be read")
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
	r, writes := newTestReconciler(seedForTestAgentRuntime())

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
	r, _ := newTestReconciler(seedForTestAgentRuntime())

	for i := 0; i < 2; i++ {
		if err := r.Handle(context.Background(), watcher.Event[*pb.AgentRuntime]{
			Type: watcher.EventUpdated, ResourceID: ar.GetMetadata().GetId(), Resource: ar,
		}); err != nil {
			t.Fatalf("Handle pass %d returned error: %v", i, err)
		}
	}

	secList, err := r.kube.CoreV1().Secrets(ns).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		t.Fatalf("list secrets: %v", err)
	}
	if len(secList.Items) != 2 {
		t.Fatalf("placed secret count after re-reconcile = %d, want 2", len(secList.Items))
	}
}

// Removing a SecretSource prunes its placed Secret rather than leaking it.
func TestAgentRuntimeReconcile_PrunesRemovedSecret(t *testing.T) {
	ar := testAgentRuntime()
	ns := agentRuntimeNamespace(ar)
	r, _ := newTestReconciler(seedForTestAgentRuntime())

	if err := r.Handle(context.Background(), watcher.Event[*pb.AgentRuntime]{
		Type: watcher.EventUpdated, ResourceID: ar.GetMetadata().GetId(), Resource: ar,
	}); err != nil {
		t.Fatalf("first provision: %v", err)
	}
	removedName := secretName(ar.SecretSources[1])

	// Drop the second SecretSource and reconcile again.
	ar.SecretSources = ar.SecretSources[:1]
	if err := r.Handle(context.Background(), watcher.Event[*pb.AgentRuntime]{
		Type: watcher.EventUpdated, ResourceID: ar.GetMetadata().GetId(), Resource: ar,
	}); err != nil {
		t.Fatalf("second provision: %v", err)
	}

	if _, err := r.kube.CoreV1().Secrets(ns).Get(context.Background(), removedName, metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("removed SecretSource's Secret should be pruned, got err=%v", err)
	}
	secList, err := r.kube.CoreV1().Secrets(ns).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		t.Fatalf("list secrets: %v", err)
	}
	if len(secList.Items) != 1 {
		t.Fatalf("placed secret count after prune = %d, want 1", len(secList.Items))
	}
}

// A reconcile of an already-Running runtime whose secrets place cleanly must write
// no status (no Provisioning regression, no redundant Running), so the status
// write-back does not become a self-triggering hot loop.
func TestAgentRuntimeReconcile_RunningConvergesWithoutStatusWrite(t *testing.T) {
	ar := testAgentRuntime()
	running := agentRuntimeStatusRunning
	ar.Status = &running
	r, writes := newTestReconciler(seedForTestAgentRuntime())

	if err := r.Handle(context.Background(), watcher.Event[*pb.AgentRuntime]{
		Type: watcher.EventUpdated, ResourceID: ar.GetMetadata().GetId(), Resource: ar,
	}); err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}

	if len(*writes) != 0 {
		t.Fatalf("expected no status writes for a converged Running runtime, got %d: %+v", len(*writes), *writes)
	}
}

// buildSecretData translates the single stored value into the Secret keys the
// coordinator env wiring expects, mirroring the removed ExternalSecret mapping.
func TestBuildSecretData(t *testing.T) {
	t.Run("gateway-identity expands JSON fields", func(t *testing.T) {
		data, err := buildSecretData(&pb.SecretSource{Name: "gw", Purpose: "gateway-identity"}, gatewayIdentityValue)
		if err != nil {
			t.Fatalf("buildSecretData: %v", err)
		}
		if string(data["issuer"]) != "https://iss.example.com" {
			t.Fatalf("issuer = %q", data["issuer"])
		}
	})
	t.Run("api-key is placed under api-key", func(t *testing.T) {
		data, err := buildSecretData(&pb.SecretSource{Name: "k", Purpose: "api-key"}, "sekret")
		if err != nil {
			t.Fatalf("buildSecretData: %v", err)
		}
		if string(data["api-key"]) != "sekret" {
			t.Fatalf("api-key = %q", data["api-key"])
		}
	})
	t.Run("key_mappings extract and rename", func(t *testing.T) {
		src := &pb.SecretSource{Name: "m", Purpose: "api-key", KeyMappings: ptr(`[{"source_key":"tok","target_key":"API_TOKEN"}]`)}
		data, err := buildSecretData(src, `{"tok":"abc","ignored":"x"}`)
		if err != nil {
			t.Fatalf("buildSecretData: %v", err)
		}
		if string(data["API_TOKEN"]) != "abc" {
			t.Fatalf("API_TOKEN = %q", data["API_TOKEN"])
		}
		if _, ok := data["ignored"]; ok {
			t.Fatalf("unmapped key must not be placed")
		}
	})
	t.Run("gateway-identity rejects non-JSON value", func(t *testing.T) {
		if _, err := buildSecretData(&pb.SecretSource{Name: "gw", Purpose: "gateway-identity"}, "not-json"); err == nil {
			t.Fatalf("expected an error for a non-JSON gateway-identity value")
		}
	})
}

// Two SecretSources whose placed-secret names share a long prefix must still
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

func secretKeys(s *corev1.Secret) []string {
	keys := make([]string, 0, len(s.Data))
	for k := range s.Data {
		keys = append(keys, k)
	}
	return keys
}

func ptr[T any](v T) *T { return &v }
