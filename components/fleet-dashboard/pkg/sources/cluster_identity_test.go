package sources

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

// identityPromServer answers the two info-gauge queries with canned vectors.
// A query for a metric not in replies gets HTTP 500.
func identityPromServer(t *testing.T, replies map[string]string) *Prometheus {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("query")
		for metric, body := range replies {
			if strings.Contains(q, metric) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[` + body + `]}}`))
				return
			}
		}
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"status":"error","error":"no reply"}`))
	}))
	t.Cleanup(srv.Close)
	return &Prometheus{base: srv.URL, clusterLabel: "cluster", client: srv.Client(), logger: slog.Default()}
}

// Verifies: FLT-CAP-01.04
func TestClusterIdentitiesGroupsByNamespace(t *testing.T) {
	p := identityPromServer(t, map[string]string{
		capabilityInfoMetric: `{"metric":{"k8s_namespace_name":"hyp2-spoke1","cluster":"ibm-a","capability":"network/public"},"value":[1,"1"]},` +
			`{"metric":{"k8s_namespace_name":"hyp2-spoke1","cluster":"ibm-a","capability":"provider/ibm"},"value":[1,"1"]},` +
			`{"metric":{"k8s_namespace_name":"hyp2-spoke1","cluster":"ibm-a","capability":"bogus/x"},"value":[1,"1"]},` +
			`{"metric":{"cluster":"ibm-a","capability":"provider/ibm"},"value":[1,"1"]}`,
		controlPlaneInfoMetric: `{"metric":{"k8s_namespace_name":"hyp2-spoke1","cluster":"ibm-a","gateway_image":"quay.io/x/gw:v2"},"value":[1,"1"]},` +
			`{"metric":{"k8s_namespace_name":"hyp2-spoke1","cluster":"ibm-a","gateway_image":"quay.io/x/gw:v1"},"value":[1,"1"]}`,
	})
	got, err := p.ClusterIdentities(context.Background())
	if err != nil {
		t.Fatalf("ClusterIdentities: %v", err)
	}
	want := map[string][]ClusterIdentity{
		// Normalized like the annotation: unknown namespace dropped, provider first.
		// A series with no namespace label is ignored. Of two images, the sorted
		// first is chosen.
		"hyp2-spoke1": {{Cluster: "ibm-a", Capabilities: []string{"provider/ibm", "network/public"}, GatewayImage: "quay.io/x/gw:v1"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ClusterIdentities = %+v, want %+v", got, want)
	}
}

func TestClusterIdentitiesPartialAndTotalFailure(t *testing.T) {
	p := identityPromServer(t, map[string]string{
		controlPlaneInfoMetric: `{"metric":{"k8s_namespace_name":"hyp0","gateway_image":"img"},"value":[1,"1"]}`,
	})
	got, err := p.ClusterIdentities(context.Background())
	if err != nil {
		t.Fatalf("one failing query must not fail the call: %v", err)
	}
	if len(got["hyp0"]) != 1 || got["hyp0"][0].GatewayImage != "img" || got["hyp0"][0].Capabilities != nil {
		t.Fatalf("partial result = %+v", got)
	}

	p = identityPromServer(t, map[string]string{})
	if _, err := p.ClusterIdentities(context.Background()); err == nil {
		t.Fatal("both queries failing must return an error")
	}
}

// Verifies: FLT-CAP-01.06
func TestIdentityFor(t *testing.T) {
	a := ClusterIdentity{Cluster: "c-a", GatewayImage: "a"}
	b := ClusterIdentity{Cluster: "c-b", GatewayImage: "b"}
	ids := map[string][]ClusterIdentity{"solo": {a}, "shared": {a, b}}
	tests := []struct {
		name, ns, cluster string
		want              string
		ok                bool
	}{
		{"single candidate ignores cluster", "solo", "other", "a", true},
		{"shared namespace picks matching cluster", "shared", "c-b", "b", true},
		{"shared namespace without match is unknown", "shared", "c-z", "", false},
		{"shared namespace without card cluster is unknown", "shared", "", "", false},
		{"missing namespace", "nope", "c-a", "", false},
		{"empty namespace", "", "c-a", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := identityFor(ids, tt.ns, tt.cluster)
			if ok != tt.ok || got.GatewayImage != tt.want {
				t.Fatalf("identityFor(%q,%q) = (%q,%v), want (%q,%v)", tt.ns, tt.cluster, got.GatewayImage, ok, tt.want, tt.ok)
			}
		})
	}
}
