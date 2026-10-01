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
