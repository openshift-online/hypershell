package selfcheck

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestBuildCatalogCoversBoundedEnumeration(t *testing.T) {
	// PLAT-HLTH-02: the catalog produces exactly one entry per bounded check name.
	checks := BuildCatalog(Deps{})
	if len(checks) != len(CheckNames()) {
		t.Fatalf("catalog has %d checks, want %d", len(checks), len(CheckNames()))
	}
	got := map[CheckName]bool{}
	for _, c := range checks {
		if c.Probe == nil {
			t.Errorf("check %q has no probe", c.Name)
		}
		got[c.Name] = true
	}
	for _, want := range CheckNames() {
		if !got[want] {
			t.Errorf("catalog missing bounded check %q", want)
		}
	}
}

func TestProbesDegradeToUnknownWhenUnconfigured(t *testing.T) {
	// PLAT-HLTH-01: a check whose dependency is not wired reports unknown rather
	// than crashing, so the loop still runs and reports.
	ctx := context.Background()
	tests := []struct {
		name  string
		probe ProbeFunc
	}{
		{"db admin secret, no dir", probeDatabaseAdminSecret("")},
		{"reconciler healthy, no clientset", probeReconcilerHealthy(nil)},
		{"namespace prereqs, no clientset", probeNamespacePrerequisites(nil, "hypershell")},
		{"ingress controller, no clientset", probeIngressController(nil)},
		{"images pullable, no images", probeImagesPullable(nil, time.Second)},
		{"oidc issuer, no issuer", probeOIDCIssuer(&http.Client{}, "")},
		{"db reachable, no dir", probeDatabaseReachable("", time.Second)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.probe(ctx); got.Status != StatusUnknown {
				t.Fatalf("status = %q, want unknown (%s)", got.Status, got.Message)
			}
		})
	}
}

func TestProbeDatabaseAdminSecretFailsWhenIncomplete(t *testing.T) {
	// PLAT-HLTH-02.02: the admin-secret-present check reports fail when the mounted
	// directory is incomplete, as a condition distinct from reachability.
	dir := t.TempDir()
	got := probeDatabaseAdminSecret(dir)(context.Background())
	if got.Status != StatusFail {
		t.Fatalf("status = %q, want fail (%s)", got.Status, got.Message)
	}
}

func TestRegistryHostPort(t *testing.T) {
	tests := []struct {
		image string
		want  string
	}{
		{"quay.io/openshift/gateway:v1", "quay.io:443"},
		{"quay.io/openshift/gateway@sha256:abc", "quay.io:443"},
		{"registry.example.com:5000/app:tag", "registry.example.com:5000"},
		{"localhost/app:tag", "localhost:443"},
		{"nginx:latest", "registry-1.docker.io:443"},
		{"library/nginx", "registry-1.docker.io:443"},
	}
	for _, tt := range tests {
		t.Run(tt.image, func(t *testing.T) {
			if got := registryHostPort(tt.image); got != tt.want {
				t.Fatalf("registryHostPort(%q) = %q, want %q", tt.image, got, tt.want)
			}
		})
	}
}

func TestProbeOIDCIssuerReachable(t *testing.T) {
	passing := probeOIDCIssuer(&http.Client{Timeout: time.Second}, "https://issuer.invalid")
	got := passing(context.Background())
	// An unroutable issuer must fail (not crash), proving the probe performs a real
	// reachability check and classifies a failure as a fail condition.
	if got.Status != StatusFail {
		t.Fatalf("status = %q, want fail for an unreachable issuer (%s)", got.Status, got.Message)
	}
}
