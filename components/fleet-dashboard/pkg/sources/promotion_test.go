package sources

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/openshift-online/hypershell/components/fleet-dashboard/pkg/config"
)

// TestPromotionDerivation verifies the port of dump-promotion.py: environment
// order comes from the PromotionStrategy status, env keys are derived by
// trimming the env/ branch prefix (never assumed), up-to-date is dry==dry, PR
// state joins by branch, and Argo joins by the name==namespace-key rule with
// delivery labels surfaced.
func TestPromotionDerivation(t *testing.T) {
	psGVR := schema.GroupVersionResource{Group: "promoter.argoproj.io", Version: "v1alpha1", Resource: "promotionstrategies"}
	ctpGVR := schema.GroupVersionResource{Group: "promoter.argoproj.io", Version: "v1alpha1", Resource: "changetransferpolicies"}
	appGVR := schema.GroupVersionResource{Group: "argoproj.io", Version: "v1alpha1", Resource: "applications"}

	ps := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "promoter.argoproj.io/v1alpha1",
		"kind":       "PromotionStrategy",
		"metadata":   map[string]any{"name": "hypershell", "namespace": "promoter-ns"},
		"status": map[string]any{
			"environments": []any{
				map[string]any{
					"branch":   "env/alpha",
					"active":   map[string]any{"dry": map[string]any{"sha": "aaaaaaaaaaaa"}, "hydrated": map[string]any{"sha": "h1"}},
					"proposed": map[string]any{"dry": map[string]any{"sha": "aaaaaaaaaaaa"}},
				},
				map[string]any{
					"branch":   "env/beta",
					"active":   map[string]any{"dry": map[string]any{"sha": "bbbbbbbbbbbb"}},
					"proposed": map[string]any{"dry": map[string]any{"sha": "cccccccccccc"}, "commitStatuses": []any{map[string]any{"key": "some-check", "phase": "pending"}}},
				},
			},
		},
	}}

	ctp := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "promoter.argoproj.io/v1alpha1",
		"kind":       "ChangeTransferPolicy",
		"metadata":   map[string]any{"name": "beta-ctp", "namespace": "promoter-ns"},
		"spec":       map[string]any{"activeBranch": "env/beta"},
		"status":     map[string]any{"pullRequest": map[string]any{"state": "open", "url": "https://example/pr/1"}},
	}}

	app := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "argoproj.io/v1alpha1",
		"kind":       "Application",
		"metadata": map[string]any{
			"name":      "promoter-ns-alpha",
			"namespace": "promoter-ns",
			"labels": map[string]any{
				deliveryLabelPrefix + "role":     "hub",
				deliveryLabelPrefix + "provider": "aws",
			},
		},
		"status": map[string]any{
			"health": map[string]any{"status": "Healthy"},
			"sync":   map[string]any{"status": "Synced"},
		},
	}}

	scheme := runtime.NewScheme()
	dyn := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, map[schema.GroupVersionResource]string{
		psGVR:  "PromotionStrategyList",
		ctpGVR: "ChangeTransferPolicyList",
		appGVR: "ApplicationList",
	}, ps, ctp, app)

	c := &config.Config{
		PromoterNamespace:     "promoter-ns",
		PromotionStrategyName: "hypershell",
		PromoterGroup:         "promoter.argoproj.io",
		PromoterVersion:       "v1alpha1",
		ArgoGroup:             "argoproj.io",
		ArgoVersion:           "v1alpha1",
		ArgoBaseURL:           "https://argo.example",
	}
	p := NewPromotion(c, dyn, nil, nil, nil)

	got, err := p.Promotion(context.Background())
	if err != nil {
		t.Fatalf("Promotion: %v", err)
	}
	payload := got.(PromotionPayload)

	if want := []string{"alpha", "beta"}; len(payload.Order) != 2 || payload.Order[0] != want[0] || payload.Order[1] != want[1] {
		t.Fatalf("order = %v, want %v", payload.Order, want)
	}

	alpha := payload.Environments["alpha"]
	if !alpha.UpToDate {
		t.Errorf("alpha should be up to date (active dry == proposed dry)")
	}
	if alpha.Role != "hub" || alpha.Provider != "aws" {
		t.Errorf("alpha delivery labels not surfaced: role=%q provider=%q", alpha.Role, alpha.Provider)
	}
	if alpha.ArgoHealth != "Healthy" || alpha.ArgoSync != "Synced" {
		t.Errorf("alpha argo status not surfaced: health=%q sync=%q", alpha.ArgoHealth, alpha.ArgoSync)
	}
	if want := "https://argo.example/applications/promoter-ns/promoter-ns-alpha?resource=kind%3AAnalysisRun%2Ckind%3APod%2Cname%3A*e2e*%2Cname%3A*hyp*-release*"; alpha.ArgoURL != want {
		t.Errorf("alpha argoUrl = %q, want %q", alpha.ArgoURL, want)
	}

	beta := payload.Environments["beta"]
	if beta.UpToDate {
		t.Errorf("beta should not be up to date (active dry != proposed dry)")
	}
	if beta.PRState != "open" || beta.PRURL == "" {
		t.Errorf("beta PR not joined by branch: state=%q url=%q", beta.PRState, beta.PRURL)
	}
	if len(beta.ProposedGates) != 1 || beta.ProposedGates[0].Key != "some-check" {
		t.Errorf("beta proposed gates = %+v, want one some-check", beta.ProposedGates)
	}
}

// fakeResolver resolves any SHA to a prebuilt Release, keyed by SHA, so tests can
// drive canonicalRelease's dedup-by-identity behaviour directly.
type fakeResolver map[string]*Release

func (f fakeResolver) Resolve(_ context.Context, sha string) *Release {
	if r, ok := f[sha]; ok {
		return r
	}
	return nil
}

// TestCanonicalReleaseDedupByDigest proves the core "out of date" fix: two
// distinct gitops dry SHAs that render the same release bundle (same digest)
// collapse onto a single shared *Release and a single map entry, so envs on the
// same bundle read up-to-date even when their gitops commits differ.
func TestCanonicalReleaseDedupByDigest(t *testing.T) {
	const digest = "sha256:6145e7f19d28d502b57f21aac8a60d4b07049390810264078e9bbc3e7aebd608"
	p := &Promotion{versioner: fakeResolver{
		"aaaaaaaa": {Version: "v20260930", Digest: digest, SHA: "aaaaaaaa"},
		"bbbbbbbb": {Version: "v20260930", Digest: digest, SHA: "bbbbbbbb"},
		"cccccccc": {Version: "cccccccc", SHA: "cccccccc"}, // no digest: short-SHA fallback
	}}
	releases := map[string]*Release{}
	ctx := context.Background()

	a := p.canonicalRelease(ctx, "aaaaaaaa", releases)
	b := p.canonicalRelease(ctx, "bbbbbbbb", releases)
	if a == nil || b == nil {
		t.Fatal("canonicalRelease returned nil for a resolvable sha")
	}
	if a != b {
		t.Errorf("same-digest SHAs must share one *Release: got %p and %p", a, b)
	}
	if a.SHA != "aaaaaaaa" {
		t.Errorf("first resolution should win: SHA = %q, want aaaaaaaa", a.SHA)
	}
	// A different bundle (no digest) stays its own entry, keyed by SHA.
	c := p.canonicalRelease(ctx, "cccccccc", releases)
	if c == a {
		t.Error("a distinct bundle must not collapse into the digest entry")
	}
	if _, ok := releases[digest]; !ok {
		t.Errorf("digest-keyed entry missing: have keys %v", keysOf(releases))
	}
	if _, ok := releases["cccccccc"]; !ok {
		t.Errorf("short-SHA entry missing: have keys %v", keysOf(releases))
	}
	if len(releases) != 2 {
		t.Errorf("want 2 canonical releases (one per bundle identity), got %d: %v", len(releases), keysOf(releases))
	}

	// An unresolvable SHA yields nil and adds no entry.
	if p.canonicalRelease(ctx, "zzzzzzzz", releases) != nil {
		t.Error("unresolvable SHA must resolve to nil")
	}
	if len(releases) != 2 {
		t.Errorf("unresolvable SHA must not add an entry, got %d", len(releases))
	}
}

// TestSameRelease proves UpToDate's identity test: two releases match by bundle
// digest (so differing gitops SHAs on the same bundle are up-to-date), fall back
// to SHA when no digest, and a nil (unresolvable) side is never up-to-date.
func TestSameRelease(t *testing.T) {
	const digest = "sha256:6145e7f19d28d502b57f21aac8a60d4b07049390810264078e9bbc3e7aebd608"
	cases := []struct {
		name             string
		active, proposed *Release
		want             bool
	}{
		{
			name:     "same digest, different gitops SHA -> up to date",
			active:   &Release{Digest: digest, SHA: "aaaaaaaa"},
			proposed: &Release{Digest: digest, SHA: "bbbbbbbb"},
			want:     true,
		},
		{
			name:     "different digest -> behind",
			active:   &Release{Digest: digest, SHA: "aaaaaaaa"},
			proposed: &Release{Digest: "sha256:other", SHA: "bbbbbbbb"},
			want:     false,
		},
		{
			name:     "no digest, same SHA -> up to date (short-SHA fallback)",
			active:   &Release{SHA: "cccccccc"},
			proposed: &Release{SHA: "cccccccc"},
			want:     true,
		},
		{
			name:     "no digest, different SHA -> behind",
			active:   &Release{SHA: "cccccccc"},
			proposed: &Release{SHA: "dddddddd"},
			want:     false,
		},
		{name: "nil proposed -> behind", active: &Release{Digest: digest}, proposed: nil, want: false},
		{name: "nil active -> behind", active: nil, proposed: &Release{Digest: digest}, want: false},
		{name: "both nil -> behind", active: nil, proposed: nil, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sameRelease(tc.active, tc.proposed); got != tc.want {
				t.Errorf("sameRelease = %v, want %v", got, tc.want)
			}
		})
	}
}

func keysOf(m map[string]*Release) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}
