package e2e

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift-online/hypershell/tests/e2e/harness"
)

// p2_9KataRuntime validates that a sandbox honors a configured Kata RuntimeClass
// (E2E-COV-03). The RuntimeClass is a cluster-wide control-plane setting
// (GATEWAY_SANDBOX_RUNTIME_CLASS / server.defaultRuntimeClassName), not a
// per-gateway field (openshell-gateway.spec.md), so every sandbox on a
// Kata-qualified cluster already carries it; this reuses the primary gateway
// rather than provisioning a dedicated one. Gated on E2E_QUALIFY_KATA=1
// because the Kind gate hosts no kata-containers RuntimeClass.
func (s *E2ESuite) p2_9KataRuntime(t *testing.T) {
	if envOrDefault("E2E_QUALIFY_KATA", "0") != "1" {
		t.Skip("Kata runtime qualification requires E2E_QUALIFY_KATA=1 and a kata-containers RuntimeClass configured on the cluster's control plane")
	}
	if s.skipKubeChecks {
		t.Skip("no per-cluster kube context; skipping kata runtime checks")
	}
	ctx := t.Context()
	runtimeClass := envOrDefault("E2E_KATA_RUNTIME_CLASS", "kata-containers")
	sandboxTimeout := durationSecondsEnv("E2E_SANDBOX_TIMEOUT", 120*time.Second)

	name := fmt.Sprintf("kata-%06d", time.Now().UnixNano()%1000000)
	if out, err := s.cli(t, s.primaryLocalName, "sandbox", "create", "--from", s.sandboxImage, "--name", name); err != nil {
		s.Logf("kata sandbox create returned an error (continuing to poll): %v\n%s", err, out)
	}
	defer s.deleteSandboxAndWaitForCount(t, s.primaryLocalName, name, s.primary.ID, 0)

	podSubstr := "default--" + name
	var pod *corev1.Pod
	s.Require().NoError(harness.Poll(ctx, 3*time.Second, sandboxTimeout, func(ctx context.Context) (bool, error) {
		pods, err := s.clients.Kube.CoreV1().Pods(s.primary.Namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			return false, nil
		}
		for i := range pods.Items {
			if strings.Contains(pods.Items[i].Name, podSubstr) {
				pod = &pods.Items[i]
				return true, nil
			}
		}
		return false, nil
	}), "kata sandbox pod appears")
	s.Require().NotNil(pod, "kata sandbox pod")
	s.Require().NotNil(pod.Spec.RuntimeClassName, "kata sandbox pod carries a runtimeClassName")
	s.Assert().Equal(runtimeClass, *pod.Spec.RuntimeClassName, "sandbox pod runtimeClassName matches the configured Kata RuntimeClass")

	if err := s.waitPodRunning(ctx, s.primary.Namespace, podSubstr, sandboxTimeout); err != nil {
		t.Skipf("RuntimeClass %q is configured but this cluster could not schedule it (no matching node handler): %v", runtimeClass, err)
	}
}
