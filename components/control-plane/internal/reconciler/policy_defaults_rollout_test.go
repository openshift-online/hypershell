package reconciler

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	fakedynamic "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/control-plane/internal/helm"
	"github.com/openshift-online/hypershell/components/control-plane/internal/keycloak"
	"github.com/openshift-online/hypershell/components/control-plane/internal/policybundle"
	"github.com/openshift-online/hypershell/components/control-plane/internal/watcher"
)

var errReachedApply = errors.New("reached the apply path past the generation gate")

// applyRecordingGatewayClient fails the first apply-path write so a test can
// prove a converged gateway passed the generation gate without running Helm.
type applyRecordingGatewayClient struct {
	pb.GatewayServiceClient
	updates int
}

func (c *applyRecordingGatewayClient) UpdateGateway(context.Context, *pb.UpdateGatewayRequest, ...grpc.CallOption) (*pb.UpdateGatewayResponse, error) {
	c.updates++
	return nil, errReachedApply
}

const rolloutNamespace = "openshell-0011223344556677"

// newRolloutReconciler builds a reconciler for a converged Running gateway
// whose rendered pod template carries renderedHash, with the interceptor
// enabled and settings defaults with the given CPU request.
func newRolloutReconciler(t *testing.T, gatewayID, renderedHash, cpuRequest string) (*GatewayReconciler, *applyRecordingGatewayClient) {
	t.Helper()
	mockKC := newMockKeycloakAdminServer(t, gatewayID, "uuid-"+gatewayID, map[string]interface{}{
		"id": "uuid-" + gatewayID, "clientId": gatewayID,
		"attributes": map[string]interface{}{"oauth2.device.authorization.grant.enabled": "true"},
	})
	kube := fake.NewSimpleClientset(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: helm.ReleaseName, Namespace: rolloutNamespace},
		Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{helm.PolicyDefaultsHashAnnotation: renderedHash},
		}}},
	})
	client := &applyRecordingGatewayClient{}
	r := &GatewayReconciler{
		active:                make(map[string]struct{}),
		dynamicClient:         fakedynamic.NewSimpleDynamicClient(runtime.NewScheme()),
		policyKubeOverride:    kube,
		controlPlaneNamespace: "hypershell-system",
		keycloakClient:        keycloak.NewClient(mockKC.server.URL, mockKC.realm, mockKC.adminClient, mockKC.adminSecret),
		gatewayClientOverride: client,
	}
	source := &FleetDefaultsSource{settings: &fakeSettingsClient{defaults: settingsDefaults(cpuRequest)}}
	r.SetPolicyInterceptor(NewPolicyInterceptorProvider("quay.io/x/cp@sha256:0123", "cp:9444", "", source), source)
	return r, client
}

func runningGatewayEvent(gatewayID string) watcher.Event[*pb.Gateway] {
	phase := "Running"
	obs := int64(1)
	return watcher.Event[*pb.Gateway]{
		Type:       watcher.EventUpdated,
		ResourceID: gatewayID,
		Resource: &pb.Gateway{
			Metadata:           &pb.ObjectReference{Id: gatewayID},
			Name:               "my-gateway",
			Namespace:          rolloutNamespace,
			Phase:              &phase,
			Generation:         1,
			ObservedGeneration: &obs,
		},
	}
}

func hashOf(t *testing.T, cpuRequest string) string {
	t.Helper()
	d, err := FleetDefaultsFromSettings(&pb.PlatformPolicySettings{Defaults: settingsDefaults(cpuRequest)})
	if err != nil {
		t.Fatal(err)
	}
	return d.Hash()
}

// Verifies: PLAT-PGP-08.04, CTRL-CORE-22.05
func TestConvergedGatewayWithStaleDefaultsHashPassesTheGenerationGate(t *testing.T) {
	r, client := newRolloutReconciler(t, "gw-stale", hashOf(t, "500m"), "700m")
	err := r.Handle(context.Background(), runningGatewayEvent("gw-stale"))
	if !errors.Is(err, errReachedApply) || client.updates != 1 {
		t.Fatalf("a Running gateway rendered with old defaults must be re-rendered, got err=%v applies=%d", err, client.updates)
	}
}

// Verifies: PLAT-PGP-08.04, CTRL-CORE-22.05
func TestConvergedGatewayWithCurrentDefaultsHashIsNotUpgraded(t *testing.T) {
	r, client := newRolloutReconciler(t, "gw-current", hashOf(t, "700m"), "700m")
	if err := r.Handle(context.Background(), runningGatewayEvent("gw-current")); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if client.updates != 0 {
		t.Fatalf("unchanged defaults must not re-render a converged gateway, got %d applies", client.updates)
	}
	// An equivalent quantity hashes the same, so it is not drift either.
	r, client = newRolloutReconciler(t, "gw-equivalent", hashOf(t, "0.7"), "700m")
	if err := r.Handle(context.Background(), runningGatewayEvent("gw-equivalent")); err != nil || client.updates != 0 {
		t.Fatalf("equivalent defaults must not re-render, got err=%v applies=%d", err, client.updates)
	}
}

// Verifies: PLAT-PGP-08.04, CTRL-CORE-22.05, OSG-CORE-19.03
func TestSidecarValuesCarryDefaultsEnvAndHash(t *testing.T) {
	source := &FleetDefaultsSource{settings: &fakeSettingsClient{defaults: settingsDefaults("700m")}}
	cfg, err := NewPolicyInterceptorProvider("quay.io/x/cp@sha256:0123", "cp:9444", "", source)(context.Background(), "gw-1", rolloutNamespace, "managed")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Defaults["HYPERSHELL_POLICY_DEFAULT_CPU_REQUEST"] != "700m" || cfg.DefaultsHash != hashOf(t, "700m") {
		t.Fatalf("sidecar values must carry the new env and its hash, got env=%v hash=%q", cfg.Defaults, cfg.DefaultsHash)
	}
}

// Verifies: PLAT-PGP-08.04, CTRL-CORE-22.05
func TestFleetDefaultsChangeTriggersGatewayRequeue(t *testing.T) {
	source := &FleetDefaultsSource{settings: &fakeSettingsClient{}}
	var calls [][2]string
	source.OnChange(func(prev, next string) { calls = append(calls, [2]string{prev, next}) })
	for _, cpu := range []string{"500m", "500m", "0.5"} {
		if _, err := source.Update(&pb.PlatformPolicySettings{Defaults: settingsDefaults(cpu)}); err != nil {
			t.Fatal(err)
		}
	}
	if len(calls) != 0 {
		t.Fatalf("first receipt and unchanged (or equivalent) defaults must not requeue gateways, got %v", calls)
	}
	if _, err := source.Update(&pb.PlatformPolicySettings{Defaults: settingsDefaults("700m")}); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 || calls[0] != [2]string{hashOf(t, "500m"), hashOf(t, "700m")} {
		t.Fatalf("a defaults change must requeue gateways once with the old and new hash, got %v", calls)
	}
}

// Verifies: PLAT-PGP-08.05, CTRL-CORE-22.05
func TestPolicyBundleRecordsDefaultsHash(t *testing.T) {
	r, _, kube := newTestPolicyReconciler(t)
	r.settings.(*fakeSettingsClient).defaults = settingsDefaults("700m")
	ctx := context.Background()
	if err := r.ReconcileOnce(ctx); err != nil {
		t.Fatal(err)
	}
	cm, err := kube.CoreV1().ConfigMaps("openshell-aaaaaaaaaaaaaaaa").Get(ctx, "hypershell-policy-bundle", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	b, err := decodeBundle(cm.Data["bundle.json"])
	if err != nil {
		t.Fatal(err)
	}
	if b.Defaults == nil || b.Defaults.CPURequest != "700m" || b.Defaults.Hash != hashOf(t, "700m") {
		t.Fatalf("bundle must record the defaults and their hash, got %+v", b.Defaults)
	}
}

func decodeBundle(raw string) (policybundle.Bundle, error) { return policybundle.Decode([]byte(raw)) }

// Verifies: CTRL-CORE-22.06, PLAT-PGP-12.03
func TestGatewayBundleIsWrittenBeforeTheGatewayApply(t *testing.T) {
	pr, _, kube := newTestPolicyReconciler(t)
	ctx := context.Background()
	r := &GatewayReconciler{}
	if r.policyBundleWriter(&pb.Gateway{}) != nil {
		t.Fatal("without a bundle writer the gateway apply must get none, so it fails and retries")
	}
	r.SetPolicyBundleWriter(pr.EnsureGatewayBundle)
	gw := &pb.Gateway{Metadata: &pb.ObjectReference{Id: "gw-new"}, Name: "new"}
	if err := r.policyBundleWriter(gw)(ctx, "openshell-bbbbbbbbbbbbbbbb"); err != nil {
		t.Fatalf("write bundle: %v", err)
	}
	cm, err := kube.CoreV1().ConfigMaps("openshell-bbbbbbbbbbbbbbbb").Get(ctx, policybundle.ConfigMapName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("the gateway's bundle must exist before its Helm apply: %v", err)
	}
	b, err := decodeBundle(cm.Data[policybundle.DataKey])
	if err != nil || b.GatewayID != "gw-new" || len(b.Enforced) != 1 || b.Defaults == nil {
		t.Fatalf("bundle = %+v, %v", b, err)
	}
	// Writing again is update-or-create and converges.
	before := cm.ResourceVersion
	if err := r.policyBundleWriter(gw)(ctx, "openshell-bbbbbbbbbbbbbbbb"); err != nil {
		t.Fatal(err)
	}
	cm, _ = kube.CoreV1().ConfigMaps("openshell-bbbbbbbbbbbbbbbb").Get(ctx, policybundle.ConfigMapName, metav1.GetOptions{})
	if cm.ResourceVersion != before {
		t.Fatal("an unchanged bundle must not be rewritten")
	}
}

// Verifies: CTRL-CORE-22.06, PLAT-PGP-12.03
func TestGatewayBundleWriteRetriesWithoutSettingsDefaults(t *testing.T) {
	pr, _, kube := newTestPolicyReconciler(t)
	pr.settings.(*fakeSettingsClient).defaults = nil
	ctx := context.Background()
	err := pr.EnsureGatewayBundle(ctx, &pb.Gateway{Metadata: &pb.ObjectReference{Id: "gw-new"}}, "openshell-bbbbbbbbbbbbbbbb")
	if !errors.Is(err, ErrNoFleetDefaults) {
		t.Fatalf("without settings defaults the bundle write must fail retryably, got %v", err)
	}
	if _, err := kube.CoreV1().ConfigMaps("openshell-bbbbbbbbbbbbbbbb").Get(ctx, policybundle.ConfigMapName, metav1.GetOptions{}); err == nil {
		t.Fatal("no bundle may be written without settings defaults")
	}
}

// Verifies: OSG-HLTH-03.08
func TestConvergedGatewayWhoseLastPassFailedIsReprovisioned(t *testing.T) {
	failed := pb.ProvisioningConditionStatus_PROVISIONING_CONDITION_STATUS_FAILED
	complete := pb.ProvisioningConditionStatus_PROVISIONING_CONDITION_STATUS_COMPLETE
	cases := map[string]func(*pb.Gateway){
		"phase Failed": func(gw *pb.Gateway) {
			phase := "Failed"
			gw.Phase = &phase
		},
		"GatewayDeployed condition Failed": func(gw *pb.Gateway) {
			gw.ProvisioningConditions = []*pb.ProvisioningCondition{
				{Type: "EnvironmentReady", ConditionStatus: complete},
				{Type: "GatewayDeployed", ConditionStatus: failed, Message: "Gateway deployment failed - unable to configure network routing"},
			}
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			// Current defaults hash: only the failed pass can reopen the gate.
			r, client := newRolloutReconciler(t, "gw-failed", hashOf(t, "700m"), "700m")
			event := runningGatewayEvent("gw-failed")
			mutate(event.Resource)
			err := r.Handle(context.Background(), event)
			if !errors.Is(err, errReachedApply) || client.updates != 1 {
				t.Fatalf("a converged gateway whose last pass failed must re-provision, got err=%v applies=%d", err, client.updates)
			}
		})
	}
}

// Verifies: OSG-HLTH-03.02
func TestLastProvisioningPassFailedIgnoresCompleteConditions(t *testing.T) {
	phase := "Running"
	gw := &pb.Gateway{Phase: &phase, ProvisioningConditions: []*pb.ProvisioningCondition{
		{Type: "GatewayDeployed", ConditionStatus: pb.ProvisioningConditionStatus_PROVISIONING_CONDITION_STATUS_COMPLETE},
	}}
	if lastProvisioningPassFailed(gw) {
		t.Fatal("a Running gateway with complete conditions is converged")
	}
}
