package reconciler

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/grpc"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/control-plane/internal/policybundle"
)

type fakePolicyClient struct {
	pb.PlatformGatewayPolicyServiceClient
	items   []*pb.PlatformGatewayPolicy
	updates []*pb.UpdatePlatformGatewayPolicyStatusRequest
}

func (f *fakePolicyClient) ListPlatformGatewayPolicies(context.Context, *pb.ListPlatformGatewayPoliciesRequest, ...grpc.CallOption) (*pb.ListPlatformGatewayPoliciesResponse, error) {
	return &pb.ListPlatformGatewayPoliciesResponse{Items: f.items}, nil
}

func (f *fakePolicyClient) UpdatePlatformGatewayPolicyStatus(_ context.Context, req *pb.UpdatePlatformGatewayPolicyStatusRequest, _ ...grpc.CallOption) (*pb.UpdatePlatformGatewayPolicyStatusResponse, error) {
	f.updates = append(f.updates, req)
	// Reflect the write back into the listed policy, as the api-server would.
	for _, p := range f.items {
		if p.GetMetadata().GetId() != req.GetId() {
			continue
		}
		byID := map[string]*pb.PlatformGatewayPolicyGatewayStatus{}
		for _, g := range p.GatewayStatuses {
			byID[g.GetGatewayId()] = g
		}
		for _, g := range req.GetGatewayStatuses() {
			byID[g.GetGatewayId()] = g
		}
		for _, id := range req.GetRemovedGatewayIds() {
			delete(byID, id)
		}
		p.GatewayStatuses = nil
		for _, g := range byID {
			p.GatewayStatuses = append(p.GatewayStatuses, g)
		}
	}
	return &pb.UpdatePlatformGatewayPolicyStatusResponse{}, nil
}

type fakeSettingsClient struct {
	pb.PlatformPolicySettingsServiceClient
	mode     string
	defaults *pb.PlatformPolicyFleetDefaults
	refusal  string
	calls    int
}

func (f *fakeSettingsClient) GetPlatformPolicySettings(context.Context, *pb.GetPlatformPolicySettingsRequest, ...grpc.CallOption) (*pb.GetPlatformPolicySettingsResponse, error) {
	f.calls++
	return &pb.GetPlatformPolicySettingsResponse{Settings: &pb.PlatformPolicySettings{Mode: f.mode, Defaults: f.defaults, DefaultsRefusal: f.refusal}}, nil
}

// settingsDefaults is the fleet defaults block the api-server streams in
// PlatformPolicySettings.
func settingsDefaults(cpuRequest string) *pb.PlatformPolicyFleetDefaults {
	return &pb.PlatformPolicyFleetDefaults{CpuRequest: cpuRequest, CpuLimit: "2", MemoryRequest: "1Gi", MemoryLimit: "4Gi"}
}

type fakePolicyGatewayClient struct {
	pb.GatewayServiceClient
	items []*pb.Gateway
}

func (f *fakePolicyGatewayClient) ListGateways(context.Context, *pb.ListGatewaysRequest, ...grpc.CallOption) (*pb.ListGatewaysResponse, error) {
	return &pb.ListGatewaysResponse{Items: f.items}, nil
}

func newTestPolicyReconciler(t *testing.T, objects ...corev1.Pod) (*PolicyBundleReconciler, *fakePolicyClient, *fake.Clientset) {
	t.Helper()
	kube := fake.NewSimpleClientset()
	for i := range objects {
		if _, err := kube.CoreV1().Pods(objects[i].Namespace).Create(context.Background(), &objects[i], metav1.CreateOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	policies := &fakePolicyClient{items: []*pb.PlatformGatewayPolicy{{
		Metadata:        &pb.ObjectReference{Id: "p1"},
		Name:            "cpu-max",
		EnforcementMode: policybundle.ModeEnforce,
		Cedar:           `forbid (principal, action, resource) when { context.cpuRequestMillis > 1000 };`,
		Target:          &pb.PlatformGatewayPolicyTarget{All: true},
	}}}
	settings := &fakeSettingsClient{mode: policybundle.ModeAudit, defaults: settingsDefaults("500m")}
	r := NewPolicyBundleReconciler(kube, nil, "cluster-1", &FleetDefaultsSource{settings: settings}, true, time.Minute, NewAppliedRevisions())
	r.policies = policies
	r.settings = settings
	r.gateways = &fakePolicyGatewayClient{items: []*pb.Gateway{{Metadata: &pb.ObjectReference{Id: "gw-1"}, Name: "g1", Namespace: "openshell-aaaaaaaaaaaaaaaa"}}}
	return r, policies, kube
}

// Verifies: CTRL-CORE-22.01, CTRL-CORE-22.03, PLAT-PGP-12.01
func TestPolicyReconcileWritesBundleAndAppliedStatus(t *testing.T) {
	r, policies, kube := newTestPolicyReconciler(t)
	ctx := context.Background()
	if err := r.ReconcileOnce(ctx); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	cm, err := kube.CoreV1().ConfigMaps("openshell-aaaaaaaaaaaaaaaa").Get(ctx, policybundle.ConfigMapName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("bundle ConfigMap must be created: %v", err)
	}
	if cm.Labels[policybundle.LabelGatewayID] != "gw-1" || cm.Labels[policybundle.LabelSchemaVersion] != policybundle.SchemaVersion {
		t.Fatalf("bundle labels: %v", cm.Labels)
	}
	bundle, err := policybundle.Decode([]byte(cm.Data[policybundle.DataKey]))
	if err != nil || len(bundle.Enforced) != 1 {
		t.Fatalf("bundle content: %v %+v", err, bundle)
	}
	if got := policies.updates[0].GetGatewayStatuses()[0].GetCondition(); got != PolicyConditionPending {
		t.Fatalf("before the sidecar acknowledges, the condition must be Pending, got %s", got)
	}

	// The sidecar reports the revision: the next pass marks it Applied.
	r.applied.Set("gw-1", bundle.Revision)
	if err := r.ReconcileOnce(ctx); err != nil {
		t.Fatal(err)
	}
	last := policies.updates[len(policies.updates)-1].GetGatewayStatuses()[0]
	if last.GetCondition() != PolicyConditionApplied || last.GetAppliedRevision() != bundle.Revision {
		t.Fatalf("expected Applied with the revision, got %+v", last)
	}

	// An unchanged pass converges: no ConfigMap update and no status write.
	writes := len(policies.updates)
	before := cm.ResourceVersion
	if err := r.ReconcileOnce(ctx); err != nil {
		t.Fatal(err)
	}
	cm2, _ := kube.CoreV1().ConfigMaps("openshell-aaaaaaaaaaaaaaaa").Get(ctx, policybundle.ConfigMapName, metav1.GetOptions{})
	if len(policies.updates) != writes || cm2.ResourceVersion != before {
		t.Fatalf("an unchanged reconcile must not rewrite the bundle or status")
	}
}

// Verifies: PLAT-PGP-16.01
func TestPolicyReconcileReportsNonCompliantRunningSandboxes(t *testing.T) {
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "big-sandbox", Namespace: "openshell-aaaaaaaaaaaaaaaa-default", Labels: map[string]string{sandboxPodLabel: "x"}},
		Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "agent", Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("2")},
		}}}},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}
	r, policies, _ := newTestPolicyReconciler(t, pod)
	if err := r.ReconcileOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := policies.updates[0].GetGatewayStatuses()[0].GetNonCompliantSandboxes()
	if len(got) != 1 || got[0] != "big-sandbox" {
		t.Fatalf("non-compliant sandboxes = %v", got)
	}
}

// Verifies: PLAT-PGP-08.03, CTRL-CORE-22.04
func TestPolicyBundleUsesSettingsProvidedDefaults(t *testing.T) {
	r, policies, kube := newTestPolicyReconciler(t)
	ctx := context.Background()
	// A policy that forbids a CPU request below 600m is valid only against
	// defaults the api-server accepted at or above 600m.
	policies.items = append(policies.items, &pb.PlatformGatewayPolicy{
		Metadata:        &pb.ObjectReference{Id: "p2"},
		Name:            "min-cpu",
		EnforcementMode: policybundle.ModeEnforce,
		Cedar:           `forbid (principal, action, resource) when { context.cpuRequestMillis < 600 };`,
		Target:          &pb.PlatformGatewayPolicyTarget{All: true},
	})
	settings := r.settings.(*fakeSettingsClient)
	settings.defaults = settingsDefaults("700m")
	if err := r.ReconcileOnce(ctx); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	bundleOf := func() policybundle.Bundle {
		cm, err := kube.CoreV1().ConfigMaps("openshell-aaaaaaaaaaaaaaaa").Get(ctx, policybundle.ConfigMapName, metav1.GetOptions{})
		if err != nil {
			t.Fatal(err)
		}
		b, err := policybundle.Decode([]byte(cm.Data[policybundle.DataKey]))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	if present, _ := bundleOf().Contains("min-cpu"); !present {
		t.Fatalf("with settings defaults of 700m the min-cpu policy must be published")
	}
	got, err := r.defaults.Get(ctx)
	if err != nil || got.CPURequest != "700m" || got.Normalized.CPURequestMillis != 700 {
		t.Fatalf("the source must hold the settings defaults, got %+v, %v", got, err)
	}

	// The api-server now distributes 500m: the bundle is validated against it
	// and the policy that forbids that default request is not published.
	settings.defaults = settingsDefaults("500m")
	if err := r.ReconcileOnce(ctx); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if present, _ := bundleOf().Contains("min-cpu"); present {
		t.Fatalf("with settings defaults of 500m the min-cpu policy must not be published")
	}
}

// Verifies: CTRL-CORE-22.04, PLAT-PGP-12.02
func TestPolicyBundleWithoutSettingsDefaultsWritesNoBundle(t *testing.T) {
	r, policies, kube := newTestPolicyReconciler(t)
	ctx := context.Background()
	r.settings.(*fakeSettingsClient).defaults = nil
	err := r.ReconcileOnce(ctx)
	if !errors.Is(err, ErrNoFleetDefaults) {
		t.Fatalf("reconcile without settings defaults must fail with ErrNoFleetDefaults, got %v", err)
	}
	if _, getErr := kube.CoreV1().ConfigMaps("openshell-aaaaaaaaaaaaaaaa").Get(ctx, policybundle.ConfigMapName, metav1.GetOptions{}); getErr == nil {
		t.Fatalf("no bundle may be written without settings defaults, so the sidecar fails closed")
	}
	if len(policies.updates) != 0 {
		t.Fatalf("no status may be written without settings defaults, got %d writes", len(policies.updates))
	}
}
