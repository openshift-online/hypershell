package gateway

import (
	"errors"
	"testing"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
)

// fakeDiscovery answers ServerResourcesForGroupVersion from fixed tables. Every
// other discovery method panics, so a test that reaches one is a bug.
type fakeDiscovery struct {
	discovery.DiscoveryInterface
	byGV  map[string]*metav1.APIResourceList
	errGV map[string]error
}

func (f *fakeDiscovery) ServerResourcesForGroupVersion(gv string) (*metav1.APIResourceList, error) {
	if f.errGV != nil {
		if err, ok := f.errGV[gv]; ok {
			return nil, err
		}
	}
	if f.byGV != nil {
		if list, ok := f.byGV[gv]; ok {
			return list, nil
		}
	}
	return nil, k8serrors.NewNotFound(schema.GroupResource{Group: gv}, "")
}

func presentList(gv, kind string) *metav1.APIResourceList {
	return &metav1.APIResourceList{
		GroupVersion: gv,
		APIResources: []metav1.APIResource{{Name: kind + "s", Kind: kind}},
	}
}

// Verifies: CTRL-CORE-18.01, CTRL-CORE-18.02, OSG-TLS-01.01, OSG-ROUT-02.01
func TestEvaluateDependencyStates(t *testing.T) {
	staleErr := errors.New("stale GroupVersion discovery: projectcalico.org/v3")
	tests := []struct {
		name string
		disc *fakeDiscovery
		ref  DependencyRef
		want DependencyState
	}{
		{
			name: "present when the required kind is served",
			disc: &fakeDiscovery{byGV: map[string]*metav1.APIResourceList{
				"cert-manager.io/v1": presentList("cert-manager.io/v1", "Certificate"),
			}},
			ref:  DependencyCertManager,
			want: DependencyPresent,
		},
		{
			name: "confirmed absent when the group is not served (NotFound)",
			disc: &fakeDiscovery{},
			ref:  DependencyCertManager,
			want: DependencyAbsent,
		},
		{
			name: "confirmed absent when the group is served without the kind",
			disc: &fakeDiscovery{byGV: map[string]*metav1.APIResourceList{
				"gateway.networking.k8s.io/v1": presentList("gateway.networking.k8s.io/v1", "HTTPRoute"),
			}},
			ref:  DependencyGatewayAPI,
			want: DependencyAbsent,
		},
		{
			name: "unknown on a transport error",
			disc: &fakeDiscovery{errGV: map[string]error{
				"gateway.networking.k8s.io/v1": errors.New("connection refused"),
			}},
			ref:  DependencyGatewayAPI,
			want: DependencyUnknown,
		},
		{
			name: "unknown on a stale-group discovery error for the queried group",
			disc: &fakeDiscovery{errGV: map[string]error{
				"cert-manager.io/v1": staleErr,
			}},
			ref:  DependencyCertManager,
			want: DependencyUnknown,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := evaluateDependency(tt.disc, tt.ref)
			if got.State != tt.want {
				t.Fatalf("state = %v, want %v (err %v)", got.State, tt.want, got.Err)
			}
			if tt.want == DependencyUnknown && got.Err == nil {
				t.Fatal("unknown result must carry the discovery error")
			}
		})
	}
}

// A stale or unreachable unrelated group never changes another dependency's
// state: each is evaluated for its own group/version, so a stale
// projectcalico.org/v3 leaves cert-manager and Gateway API present.
// Verifies: CTRL-CORE-17.01, CTRL-CORE-18.04, CTRL-CORE-19.01, OSG-CORE-14.05
func TestEvaluateDependencyIsolatesUnrelatedStaleGroup(t *testing.T) {
	disc := &fakeDiscovery{
		byGV: map[string]*metav1.APIResourceList{
			"cert-manager.io/v1":           presentList("cert-manager.io/v1", "Certificate"),
			"gateway.networking.k8s.io/v1": presentList("gateway.networking.k8s.io/v1", "GRPCRoute"),
		},
		errGV: map[string]error{
			"projectcalico.org/v3": errors.New("stale GroupVersion discovery: projectcalico.org/v3"),
		},
	}
	if got := evaluateDependency(disc, DependencyCertManager); got.State != DependencyPresent {
		t.Fatalf("cert-manager state = %v, want present", got.State)
	}
	if got := evaluateDependency(disc, DependencyGatewayAPI); got.State != DependencyPresent {
		t.Fatalf("gateway API state = %v, want present", got.State)
	}
}

// A transient discovery failure reports unknown/unavailable without discarding a
// prior successful determination.
// Verifies: CTRL-CORE-18, CTRL-CORE-18.04
func TestDependencyDiscoveryRetainsLastGood(t *testing.T) {
	present := &fakeDiscovery{byGV: map[string]*metav1.APIResourceList{
		"gateway.networking.k8s.io/v1": presentList("gateway.networking.k8s.io/v1", "GRPCRoute"),
	}}
	dd := NewDependencyDiscovery(present)
	if got := dd.Evaluate(DependencyGatewayAPI); got.State != DependencyPresent {
		t.Fatalf("first evaluate state = %v, want present", got.State)
	}

	// Swap the backing discovery for one that fails, and confirm the result is
	// unknown but remembers the last-known-good present state.
	dd.disc = &fakeDiscovery{errGV: map[string]error{
		"gateway.networking.k8s.io/v1": errors.New("connection reset by peer"),
	}}
	got := dd.Evaluate(DependencyGatewayAPI)
	if got.State != DependencyUnknown {
		t.Fatalf("second evaluate state = %v, want unknown", got.State)
	}
	if got.LastKnown != DependencyPresent {
		t.Fatalf("LastKnown = %v, want present", got.LastKnown)
	}
}
