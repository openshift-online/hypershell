package reconciler

import (
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift-online/hypershell/components/control-plane/internal/helm"
)

// Verifies: PLAT-PGP-09.01
func TestSidecarUsesControllerOwnImageDigest(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "hypershell-controller-0"},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "controller", Image: "quay.io/x/cp:latest"}}},
		Status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{{
			Name: "controller", ImageID: "docker-pullable://quay.io/x/cp@sha256:0123",
		}}},
	}
	got, err := imageFromPod(pod, "controller")
	if err != nil || got != "quay.io/x/cp@sha256:0123" {
		t.Fatalf("own image = %q, %v", got, err)
	}
	pod.Status.ContainerStatuses[0].ImageID = "sha256:0123"
	if got, _ := imageFromPod(pod, "controller"); got != "quay.io/x/cp:latest" {
		t.Fatalf("a bare digest must fall back to the spec image, got %q", got)
	}

	provider := NewPolicyInterceptorProvider("quay.io/x/cp@sha256:0123", "cp:9444", "https://console.example", NewPolicyDefaultsGuard(nil, "ns", FleetDefaults{CPURequest: "500m", CPULimit: "2", MemoryRequest: "1Gi", MemoryLimit: "4Gi"}))
	cfg, err := provider("gw-1", "openshell-0123456789abcdef", "managed")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Image != "quay.io/x/cp@sha256:0123" || cfg.Defaults["HYPERSHELL_POLICY_DEFAULT_CPU_REQUEST"] != "500m" || !strings.HasSuffix(cfg.PoliciesURL, "/gateways/gw-1/policies") {
		t.Fatalf("provider config = %+v", cfg)
	}
	if err := helm.ValidateInterceptorRegistration(cfg); err != nil {
		t.Fatalf("the generated registration must validate: %v", err)
	}
}
