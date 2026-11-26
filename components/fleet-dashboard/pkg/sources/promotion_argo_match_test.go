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

// argoApp builds an Argo Application for the matching tests. destName is the
// destination cluster name; labels are delivery labels without the prefix.
func argoApp(ns, name, destName, destNS string, labels map[string]string, health string) *unstructured.Unstructured {
	l := map[string]any{}
	for k, v := range labels {
		l[deliveryLabelPrefix+k] = v
	}
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "argoproj.io/v1alpha1",
		"kind":       "Application",
		"metadata":   map[string]any{"name": name, "namespace": ns, "labels": l},
		"spec": map[string]any{"destination": map[string]any{
			"name":      destName,
			"namespace": destNS,
		}},
		"status": map[string]any{"health": map[string]any{"status": health}},
	}}
}

// withCapabilities sets the delivery capabilities annotation on an Application.
func withCapabilities(app *unstructured.Unstructured, caps string) *unstructured.Unstructured {
	app.SetAnnotations(map[string]string{capabilitiesAnnotation: caps})
	return app
}

// matchFixture builds a Promotion over a strategy with environments alpha and
// beta, plus the given Applications.
func matchFixture(t *testing.T, apps ...*unstructured.Unstructured) PromotionPayload {
	t.Helper()
	psGVR := schema.GroupVersionResource{Group: "promoter.argoproj.io", Version: "v1alpha1", Resource: "promotionstrategies"}
	ctpGVR := schema.GroupVersionResource{Group: "promoter.argoproj.io", Version: "v1alpha1", Resource: "changetransferpolicies"}
	appGVR := schema.GroupVersionResource{Group: "argoproj.io", Version: "v1alpha1", Resource: "applications"}
	ps := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "promoter.argoproj.io/v1alpha1",
		"kind":       "PromotionStrategy",
		"metadata":   map[string]any{"name": "hypershell", "namespace": "promoter-ns"},
		"status": map[string]any{"environments": []any{
			map[string]any{"branch": "env/alpha"},
			map[string]any{"branch": "env/beta"},
		}},
	}}
	objs := []runtime.Object{ps}
	for _, a := range apps {
		objs = append(objs, a)
	}
	dyn := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
		psGVR:  "PromotionStrategyList",
		ctpGVR: "ChangeTransferPolicyList",
		appGVR: "ApplicationList",
	}, objs...)
	c := &config.Config{
		PromoterNamespace:     "promoter-ns",
		PromotionStrategyName: "hypershell",
		PromoterGroup:         "promoter.argoproj.io",
		PromoterVersion:       "v1alpha1",
		ArgoGroup:             "argoproj.io",
		ArgoVersion:           "v1alpha1",
		ArgoBaseURL:           "https://argo.example",
	}
	got, err := NewPromotion(c, dyn, nil, nil, nil, nil).Promotion(context.Background())
	if err != nil {
		t.Fatalf("Promotion: %v", err)
	}
	return got.(PromotionPayload)
}

// An Application named just "<instance>" that deploys into the instance's
// namespace is matched by where it deploys.
// Verifies: FLT-MAP-09.01
func TestArgoMatchByDestinationBareName(t *testing.T) {
	payload := matchFixture(t,
		argoApp("cluster-b", "beta", "cluster-b", "beta",
			map[string]string{"role": "spoke", "provider": "ibm", "cluster": "cluster-b", "env": "int"}, "Healthy"),
	)
	beta := payload.Environments["beta"]
	if beta.ArgoApp != "beta" || beta.Role != "spoke" || beta.Provider != "ibm" || beta.Cluster != "cluster-b" || beta.ArgoHealth != "Healthy" {
		t.Fatalf("beta not matched by destination: %+v", beta)
	}
}

// A helper Application deploying into the instance namespace under another name
// never takes the instance's place, and a labelled Application beats an
// unlabelled one.
// Verifies: FLT-MAP-09.02
func TestArgoMatchPrefersCoreApplication(t *testing.T) {
	payload := matchFixture(t,
		argoApp("cluster-a", "release-analysis", "cluster-a", "alpha", nil, "Degraded"),
		argoApp("cluster-a", "cluster-a-alpha", "cluster-a", "alpha",
			map[string]string{"role": "hub", "provider": "ibm", "cluster": "cluster-a"}, "Healthy"),
		argoApp("cluster-a", "alpha", "cluster-a", "alpha", nil, "Missing"),
	)
	alpha := payload.Environments["alpha"]
	if alpha.ArgoApp != "cluster-a-alpha" || alpha.Role != "hub" || alpha.ArgoHealth != "Healthy" {
		t.Fatalf("alpha matched the wrong Application: %+v", alpha)
	}
}

// An Application without delivery labels still names its physical cluster, from
// its destination cluster; "in-cluster" is not a cluster name.
// Verifies: FLT-MAP-09.03
func TestArgoMatchClusterFromDestination(t *testing.T) {
	payload := matchFixture(t,
		argoApp("cluster-a", "alpha", "cluster-a", "alpha", nil, "Healthy"),
		argoApp("local", "beta", "in-cluster", "beta", nil, "Healthy"),
	)
	if got := payload.Environments["alpha"].Cluster; got != "cluster-a" {
		t.Errorf("alpha cluster = %q, want cluster-a", got)
	}
	if got := payload.Environments["beta"].Cluster; got != "" {
		t.Errorf("beta cluster = %q, want empty for in-cluster", got)
	}
}

// An instance that Argo CD deploys but that is not a promotion environment is
// reported under unpromoted, with its cluster. Apps that match only by name,
// deploy elsewhere, run on Argo CD's own cluster, or are unlabelled cluster
// add-ons are not.
// Verifies: FLT-MAP-09.04
func TestArgoMatchReportsUnpromoted(t *testing.T) {
	payload := matchFixture(t,
		withCapabilities(argoApp("cluster-c", "gamma", "cluster-c", "gamma", nil, "Healthy"), "provider/ibm"),
		argoApp("cluster-a", "cluster-a-tools", "cluster-a", "tools-system", nil, "Healthy"),
		argoApp("cluster-a", "keycloak-alpha", "cluster-a", "keycloak-system", nil, "Healthy"),
		// A cluster add-on named "<cluster>-<add-on>" without a role label.
		argoApp("cluster-d", "cluster-d-ldap-sync", "cluster-d", "ldap-sync", nil, "Healthy"),
		// An Application on Argo CD's own cluster.
		argoApp("gitops", "dirsrv", "in-cluster", "dirsrv", nil, "Healthy"),
		// A labelled spoke with a "<namespace>-<instance>" name qualifies.
		argoApp("cluster-e", "cluster-e-delta", "cluster-e", "delta",
			map[string]string{"role": "spoke", "provider": "aws"}, "Healthy"),
	)
	gamma, ok := payload.Unpromoted["gamma"]
	if !ok {
		t.Fatalf("gamma missing from unpromoted: %+v", payload.Unpromoted)
	}
	if gamma.Cluster != "cluster-c" || gamma.ArgoHealth != "Healthy" || gamma.ArgoURL == "" || !equalStrings(gamma.Capabilities, []string{"provider/ibm"}) {
		t.Errorf("gamma = %+v", gamma)
	}
	if delta := payload.Unpromoted["delta"]; delta.Cluster != "cluster-e" || delta.Provider != "aws" || delta.Role != "spoke" {
		t.Errorf("delta = %+v", delta)
	}
	if len(payload.Unpromoted) != 2 {
		t.Errorf("unpromoted = %+v, want only gamma and delta", payload.Unpromoted)
	}
}
