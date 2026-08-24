package gateway

import (
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	discoveryfake "k8s.io/client-go/discovery/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func apiResourceList(resourceNames ...string) *metav1.APIResourceList {
	list := &metav1.APIResourceList{GroupVersion: cnpgAPIGroupVersion}
	for _, name := range resourceNames {
		list.APIResources = append(list.APIResources, metav1.APIResource{Name: name})
	}
	return list
}

// TestMissingCNPGResources exercises the exact-resource contract directly:
// detection must confirm the specific clusters, databases, and databaseroles
// resources are served, not merely that some postgresql.cnpg.io group exists.
func TestMissingCNPGResources(t *testing.T) {
	tests := []struct {
		name string
		list *metav1.APIResourceList
		want []string
	}{
		{
			name: "nil list: everything missing",
			list: nil,
			want: []string{"clusters", "databases", "databaseroles"},
		},
		{
			name: "empty list: everything missing",
			list: apiResourceList(),
			want: []string{"clusters", "databases", "databaseroles"},
		},
		{
			name: "only clusters served: databases and databaseroles missing",
			list: apiResourceList("clusters"),
			want: []string{"databases", "databaseroles"},
		},
		{
			name: "some unrelated resource present alongside clusters",
			list: apiResourceList("clusters", "poolers"),
			want: []string{"databases", "databaseroles"},
		},
		{
			name: "all three required resources served: nothing missing",
			list: apiResourceList("clusters", "databases", "databaseroles"),
			want: nil,
		},
		{
			name: "all three plus extras: nothing missing",
			list: apiResourceList("clusters", "databases", "databaseroles", "poolers", "backups"),
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := missingCNPGResources(tt.list)
			if len(got) != len(tt.want) {
				t.Fatalf("missingCNPGResources() = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("missingCNPGResources() = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

// fakeDiscoveryClientset builds a fake Kubernetes clientset whose discovery
// reports the given resource lists (nil resources means the group/version is
// not found at all, matching a cluster with no CNPG CRDs installed).
func fakeDiscoveryClientset(t *testing.T, resources ...*metav1.APIResourceList) *k8sfake.Clientset {
	t.Helper()
	cs := k8sfake.NewSimpleClientset()
	fd, ok := cs.Discovery().(*discoveryfake.FakeDiscovery)
	if !ok {
		t.Fatalf("fake clientset discovery is not *discoveryfake.FakeDiscovery")
	}
	fd.Resources = resources
	return cs
}

// TestDetectCNPG_ExactResources pins DetectCNPG to the exact-resource
// contract: it must return false (never panic) when the CNPG API group is
// wholly absent, and also false when postgresql.cnpg.io/v1 exists but is
// missing one of the three resources this codebase depends on -- a stale or
// partial CRD install must not be reported as CNPG being available.
func TestDetectCNPG_ExactResources(t *testing.T) {
	t.Run("no postgresql.cnpg.io group at all", func(t *testing.T) {
		cs := fakeDiscoveryClientset(t)
		if DetectCNPG(cs) {
			t.Fatal("DetectCNPG() = true, want false when no CNPG API group is present")
		}
	})

	t.Run("group present but missing databaseroles", func(t *testing.T) {
		cs := fakeDiscoveryClientset(t, apiResourceList("clusters", "databases"))
		if DetectCNPG(cs) {
			t.Fatal("DetectCNPG() = true, want false when databaseroles is not served")
		}
	})

	t.Run("unrelated API group present is not mistaken for CNPG", func(t *testing.T) {
		cs := fakeDiscoveryClientset(t, &metav1.APIResourceList{
			GroupVersion: "postgresql.cnpg.io/v1beta1",
			APIResources: []metav1.APIResource{{Name: "clusters"}, {Name: "databases"}, {Name: "databaseroles"}},
		})
		if DetectCNPG(cs) {
			t.Fatal("DetectCNPG() = true, want false: resources are served under v1beta1, not the required v1")
		}
	})

	t.Run("all three v1 resources present", func(t *testing.T) {
		cs := fakeDiscoveryClientset(t, apiResourceList("clusters", "databases", "databaseroles"))
		if !DetectCNPG(cs) {
			t.Fatal("DetectCNPG() = false, want true when clusters, databases, and databaseroles are all served")
		}
	})
}

// TestRequireCNPGAPI covers the control-plane startup contract: it must
// return a descriptive, non-nil error (never panic) whenever the exact CNPG
// resources are unavailable, and nil only when all three are served.
func TestRequireCNPGAPI(t *testing.T) {
	t.Run("no CNPG API: returns an error mentioning DATABASE_PROVIDER=cnpg", func(t *testing.T) {
		cs := fakeDiscoveryClientset(t)
		err := RequireCNPGAPI(cs)
		if err == nil {
			t.Fatal("RequireCNPGAPI() = nil, want an error when no CNPG API group is present")
		}
		if !strings.Contains(err.Error(), "DATABASE_PROVIDER=cnpg") {
			t.Fatalf("RequireCNPGAPI() error = %q, want it to mention DATABASE_PROVIDER=cnpg", err.Error())
		}
	})

	t.Run("partial CNPG API: returns an error naming the missing resource", func(t *testing.T) {
		cs := fakeDiscoveryClientset(t, apiResourceList("clusters", "databases"))
		err := RequireCNPGAPI(cs)
		if err == nil {
			t.Fatal("RequireCNPGAPI() = nil, want an error when databaseroles is not served")
		}
		if !strings.Contains(err.Error(), "databaseroles") {
			t.Fatalf("RequireCNPGAPI() error = %q, want it to name the missing databaseroles resource", err.Error())
		}
	})

	t.Run("all required resources present: no error", func(t *testing.T) {
		cs := fakeDiscoveryClientset(t, apiResourceList("clusters", "databases", "databaseroles"))
		if err := RequireCNPGAPI(cs); err != nil {
			t.Fatalf("RequireCNPGAPI() = %v, want nil when clusters, databases, and databaseroles are all served", err)
		}
	})
}
