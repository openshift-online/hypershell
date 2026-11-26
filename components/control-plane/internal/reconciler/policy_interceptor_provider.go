package reconciler

import (
	"context"
	"fmt"
	"strings"

	"github.com/distribution/reference"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/control-plane/internal/gateway"
	"github.com/openshift-online/hypershell/components/control-plane/internal/helm"
	"github.com/openshift-online/hypershell/components/control-plane/internal/policyinterceptor"
)

// PolicyInterceptorProvider returns the sidecar values for one gateway.
type PolicyInterceptorProvider func(ctx context.Context, gatewayID, namespace, workspaceMode string) (*helm.PolicyInterceptorConfig, error)

// NewPolicyInterceptorProvider builds the per-gateway sidecar configuration
// from the controller's own image (PLAT-PGP-09), the events endpoint, the
// fleet defaults the api-server accepted and distributes through
// PlatformPolicySettings (PLAT-PGP-08), and the static binding table shared
// with the interceptor's Describe (PLAT-PGP-10). Without received defaults it
// returns an error, so the gateway reconcile retries rather than render a
// sidecar with made-up defaults.
func NewPolicyInterceptorProvider(image, eventsAddress, consoleURL string, defaults *FleetDefaultsSource) PolicyInterceptorProvider {
	return func(ctx context.Context, gatewayID, namespace, workspaceMode string) (*helm.PolicyInterceptorConfig, error) {
		issuer, err := helm.PolicyJWTIssuer(workspaceMode, namespace)
		if err != nil {
			return nil, fmt.Errorf("policy interceptor issuer: %w", err)
		}
		d, err := defaults.Get(ctx)
		if err != nil {
			return nil, fmt.Errorf("policy interceptor fleet defaults: %w", err)
		}
		cfg := &helm.PolicyInterceptorConfig{
			Name:          policyinterceptor.InterceptorName,
			Audience:      policyinterceptor.DefaultAudience,
			Image:         image,
			GatewayID:     gatewayID,
			JWTIssuer:     issuer,
			EventsAddress: eventsAddress,
			DefaultsHash:  d.Hash(),
			Defaults: map[string]string{
				policyinterceptor.EnvDefaultCPURequest: d.CPURequest,
				policyinterceptor.EnvDefaultCPULimit:   d.CPULimit,
				policyinterceptor.EnvDefaultMemRequest: d.MemoryRequest,
				policyinterceptor.EnvDefaultMemLimit:   d.MemoryLimit,
			},
		}
		if consoleURL != "" {
			cfg.PoliciesURL = consoleURL + "/gateways/" + gatewayID + "/policies"
		}
		for _, b := range policyinterceptor.Bindings() {
			cfg.Bindings = append(cfg.Bindings, helm.InterceptorBinding{RPC: b.RPC, Phases: b.Phases, FailurePolicy: b.FailurePolicy})
			cfg.KnownRPCs = append(cfg.KnownRPCs, b.RPC)
		}
		return cfg, nil
	}
}

// OwnImage returns the controller's own container image, read from its Pod
// status, pinned by digest when the kubelet reports a digest for the same
// repository as the spec image (PLAT-PGP-09.01, PLAT-PGP-09.03). The
// interceptor sidecar runs this exact build.
func OwnImage(ctx context.Context, kube kubernetes.Interface, namespace, podName, container string) (string, error) {
	if podName == "" {
		return "", fmt.Errorf("HYPERSHELL_POD_NAME is not set")
	}
	pod, err := kube.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("read own pod: %w", err)
	}
	return imageFromPod(pod, container)
}

// imageFromPod returns the container's image for the interceptor sidecar.
//
//  1. A "repository@sha256:..." image ID whose repository matches the spec
//     image's repository pins the exact build (PLAT-PGP-09.01).
//  2. Any other image ID keeps the spec image (PLAT-PGP-09.03). This covers a
//     bare digest, and a placeholder repository such as the
//     "docker.io/library/import-<date>" name containerd gives an image loaded
//     from an archive (Kind with the podman provider), which no registry serves.
//
// Repositories are compared after normalization, so "quay.io/x/cp:latest"
// matches "quay.io/x/cp@sha256:..." and "cp" matches "docker.io/library/cp".
func imageFromPod(pod *corev1.Pod, container string) (string, error) {
	spec := ""
	for _, c := range pod.Spec.Containers {
		if c.Name == container {
			spec = c.Image
		}
	}
	if spec == "" {
		return "", fmt.Errorf("container %q not found in pod %s", container, pod.Name)
	}
	for _, st := range pod.Status.ContainerStatuses {
		if st.Name != container {
			continue
		}
		id := strings.TrimPrefix(st.ImageID, "docker-pullable://")
		at := strings.Index(id, "@sha256:")
		if at <= 0 {
			continue
		}
		if sameRepository(id[:at], spec) {
			return id, nil
		}
	}
	return spec, nil
}

// sameRepository reports whether two image references name the same
// normalized repository. An unparsable reference never matches.
func sameRepository(a, b string) bool {
	ra, err := reference.ParseNormalizedNamed(a)
	if err != nil {
		return false
	}
	rb, err := reference.ParseNormalizedNamed(b)
	if err != nil {
		return false
	}
	return ra.Name() == rb.Name()
}

// SetPolicyInterceptor enables the policy interceptor sidecar on every gateway
// this reconciler deploys. defaults is the source the sidecar values read; the
// reconciler also compares its hash with the rendered pod template, so a
// fleet-defaults change passes the generation gate as drift (PLAT-PGP-08.04).
func (r *GatewayReconciler) SetPolicyInterceptor(p PolicyInterceptorProvider, defaults *FleetDefaultsSource) {
	r.policyInterceptor = p
	r.policyDefaults = defaults
}

// SetPolicyBundleWriter wires the bundle reconciler's per-gateway write
// (PolicyBundleReconciler.EnsureGatewayBundle), which runs before every Helm
// apply of a gateway with the interceptor (PLAT-PGP-12.03, CTRL-CORE-22.06).
func (r *GatewayReconciler) SetPolicyBundleWriter(fn func(ctx context.Context, gw *pb.Gateway, namespace string) error) {
	r.policyBundle = fn
}

// policyBundleWriter binds the bundle write to one gateway for the gateway
// package's ReconcileOpts.EnsurePolicyBundle. Without a writer it returns nil,
// which fails the reconcile so it retries.
func (r *GatewayReconciler) policyBundleWriter(gw *pb.Gateway) func(ctx context.Context, namespace string) error {
	if r.policyBundle == nil {
		return nil
	}
	return func(ctx context.Context, namespace string) error { return r.policyBundle(ctx, gw, namespace) }
}

// policyDefaultsNeedUpdate reports whether the gateway's rendered pod template
// carries a fleet-defaults hash other than the current one. Without the
// interceptor there is nothing to compare.
func (r *GatewayReconciler) policyDefaultsNeedUpdate(ctx context.Context, namespace string) (bool, error) {
	if r.policyInterceptor == nil || r.policyDefaults == nil {
		return false, nil
	}
	d, err := r.policyDefaults.Get(ctx)
	if err != nil {
		return false, fmt.Errorf("policy interceptor fleet defaults: %w", err)
	}
	var kube kubernetes.Interface = r.clientset
	if r.policyKubeOverride != nil {
		kube = r.policyKubeOverride
	}
	return gateway.PolicyDefaultsHashNeedsUpdate(ctx, kube, namespace, d.Hash())
}
