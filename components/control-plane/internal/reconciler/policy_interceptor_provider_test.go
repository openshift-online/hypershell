package reconciler

import (
	"context"
	"errors"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
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

	source := &FleetDefaultsSource{settings: &fakeSettingsClient{defaults: settingsDefaults("500m")}}
	provider := NewPolicyInterceptorProvider("quay.io/x/cp@sha256:0123", "cp:9444", "https://console.example", source)
	cfg, err := provider(context.Background(), "gw-1", "openshell-0123456789abcdef", "managed")
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

// Verifies: PLAT-PGP-09.01, PLAT-PGP-09.03
func TestOwnImagePinsDigestOnlyForTheSpecRepository(t *testing.T) {
	cases := []struct {
		name, spec, imageID, want string
	}{
		{"matching registry repo gets the digest pin", "quay.io/x/cp:latest", "quay.io/x/cp@sha256:0123", "quay.io/x/cp@sha256:0123"},
		{"docker-pullable prefix is stripped", "quay.io/x/cp:latest", "docker-pullable://quay.io/x/cp@sha256:0123", "quay.io/x/cp@sha256:0123"},
		{"familiar and normalized names match", "cp:dev", "docker.io/library/cp@sha256:0123", "docker.io/library/cp@sha256:0123"},
		{"archive-loaded import name falls back to the spec image", "localhost/hypershell-controller:dev", "docker.io/library/import-2026-11-26@sha256:9ff3", "localhost/hypershell-controller:dev"},
		{"other repository falls back to the spec image", "quay.io/x/cp:latest", "quay.io/y/cp@sha256:0123", "quay.io/x/cp:latest"},
		{"bare digest falls back to the spec image", "quay.io/x/cp:latest", "sha256:0123", "quay.io/x/cp:latest"},
		{"empty image ID falls back to the spec image", "quay.io/x/cp:latest", "", "quay.io/x/cp:latest"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "hypershell-controller-0"},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "controller", Image: tc.spec}}},
				Status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{{
					Name: "controller", ImageID: tc.imageID,
				}}},
			}
			got, err := imageFromPod(pod, "controller")
			if err != nil || got != tc.want {
				t.Fatalf("own image = %q, %v; want %q", got, err, tc.want)
			}
		})
	}
}

// Verifies: PLAT-PGP-08.03, CTRL-CORE-22.04
func TestSidecarDefaultsComeFromPlatformPolicySettings(t *testing.T) {
	settings := &fakeSettingsClient{defaults: settingsDefaults("750m")}
	source := &FleetDefaultsSource{settings: settings}
	provider := NewPolicyInterceptorProvider("quay.io/x/cp@sha256:0123", "cp:9444", "", source)

	// Nothing received yet: the provider reads the settings from the api-server.
	cfg, err := provider(context.Background(), "gw-1", "openshell-0123456789abcdef", "managed")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Defaults["HYPERSHELL_POLICY_DEFAULT_CPU_REQUEST"] != "750m" || cfg.Defaults["HYPERSHELL_POLICY_DEFAULT_MEMORY_LIMIT"] != "4Gi" {
		t.Fatalf("sidecar env must carry the settings defaults, got %v", cfg.Defaults)
	}

	// A settings update received by the bundle reconcile is what the next
	// sidecar render uses, without another api-server read.
	if _, err := source.Update(&pb.PlatformPolicySettings{Defaults: settingsDefaults("1")}); err != nil {
		t.Fatal(err)
	}
	calls := settings.calls
	cfg, err = provider(context.Background(), "gw-1", "openshell-0123456789abcdef", "managed")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Defaults["HYPERSHELL_POLICY_DEFAULT_CPU_REQUEST"] != "1" || settings.calls != calls {
		t.Fatalf("sidecar env must follow the received settings, got %v (api reads %d -> %d)", cfg.Defaults, calls, settings.calls)
	}
}

// Verifies: CTRL-CORE-22.04, PLAT-PGP-12.02
func TestSidecarIsNotRenderedWithoutSettingsDefaults(t *testing.T) {
	source := &FleetDefaultsSource{settings: &fakeSettingsClient{}}
	provider := NewPolicyInterceptorProvider("quay.io/x/cp@sha256:0123", "cp:9444", "", source)
	cfg, err := provider(context.Background(), "gw-1", "openshell-0123456789abcdef", "managed")
	if !errors.Is(err, ErrNoFleetDefaults) || cfg != nil {
		t.Fatalf("without settings defaults the provider must refuse to render a sidecar, got %+v, %v", cfg, err)
	}
	if _, err := source.Update(&pb.PlatformPolicySettings{Defaults: &pb.PlatformPolicyFleetDefaults{CpuRequest: "lots", CpuLimit: "2", MemoryRequest: "1Gi", MemoryLimit: "4Gi"}}); err == nil {
		t.Fatalf("an unparsable settings default must be rejected, never applied")
	}
}
