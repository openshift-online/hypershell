package reconciler

import (
	"errors"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/control-plane/internal/watcher"
)

// Verifies: PLAT-DATA-34.01
func TestDeployedBundle_ConfiguredBundleWins(t *testing.T) {
	if got := DeployedBundle(" hypershell-2026.11.27-1 ", "quay.io/x/cp@sha256:abc"); got != "hypershell-2026.11.27-1" {
		t.Fatalf("DeployedBundle = %q, want the configured bundle", got)
	}
}

// Verifies: PLAT-DATA-34.02
func TestDeployedBundle_FallsBackToOwnImageDigest(t *testing.T) {
	const digest = "quay.io/x/cp@sha256:1111111111111111111111111111111111111111111111111111111111111111"
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "controller-0"},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "controller", Image: "quay.io/x/cp:latest"}}},
		Status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{{
			Name: "controller", ImageID: "docker-pullable://" + digest,
		}}},
	}
	own, err := imageFromPod(pod, "controller")
	if err != nil {
		t.Fatal(err)
	}
	if got := DeployedBundle("", own); got != digest {
		t.Fatalf("DeployedBundle = %q, want the controller image digest %q", got, digest)
	}
	if got := DeployedBundle("", ""); got != "" {
		t.Fatalf("DeployedBundle with nothing known = %q, want empty", got)
	}
}

// Verifies: PLAT-DATA-34.01, PLAT-DATA-06.03
func TestUpdateObservedGeneration_WritesDeployedBundle(t *testing.T) {
	client := &recordingGatewayClient{respond: func(*pb.UpdateGatewayRequest) (*pb.UpdateGatewayResponse, error) {
		return &pb.UpdateGatewayResponse{}, nil
	}}
	r := newGeneratedConfigReconciler(client)
	r.SetDeployedBundle("bundle-b2")
	r.updateObservedGeneration(t.Context(), "gw-1", 7)

	if len(client.requests) != 1 {
		t.Fatalf("UpdateGateway requests = %d, want 1", len(client.requests))
	}
	req := client.requests[0]
	if req.GetObservedGeneration() != 7 || req.GetDeployedBundle() != "bundle-b2" {
		t.Fatalf("request = %+v, want observed_generation 7 and deployed_bundle bundle-b2", req)
	}

	// With no identity known, deployed_bundle is left out (unchanged).
	if got := convergedUpdateRequest("gw-1", 7, ""); got.DeployedBundle != nil {
		t.Fatalf("deployed_bundle = %q, want unset", got.GetDeployedBundle())
	}
}

// A failed pass returns before the converged write, so it never sends
// deployed_bundle and the gateway keeps the last successful build's value.
// Verifies: PLAT-DATA-34.03
func TestHandle_FailedPassDoesNotWriteDeployedBundle(t *testing.T) {
	t.Setenv("GATEWAY_OIDC_ISSUER_URL", "")
	client := &recordingGatewayClient{respond: func(*pb.UpdateGatewayRequest) (*pb.UpdateGatewayResponse, error) {
		return nil, errors.New("api server unavailable")
	}}
	r := newGeneratedConfigReconciler(client)
	r.SetDeployedBundle("bundle-b2")
	observed := int64(0)

	err := r.Handle(t.Context(), watcher.Event[*pb.Gateway]{
		Type:       watcher.EventCreated,
		ResourceID: generatedConfigGatewayID,
		Resource: &pb.Gateway{
			Metadata:           &pb.ObjectReference{Id: generatedConfigGatewayID},
			Name:               "failing-gateway",
			Generation:         1,
			ObservedGeneration: &observed,
		},
	})
	if err == nil {
		t.Fatal("Handle succeeded, want the failure surfaced for retry")
	}
	for _, req := range client.requests {
		if req.DeployedBundle != nil {
			t.Fatalf("failed pass wrote deployed_bundle %q", req.GetDeployedBundle())
		}
	}
}
