package sources

import (
	"testing"
	"time"
)

func TestMarkRecuts(t *testing.T) {
	// Verifies: FLT-DLV-23.06, SDLC-VIS-11.07
	comps := func(api, cp string) map[string]string {
		return map[string]string{"api-server": api, "control-plane": cp}
	}
	b1 := &Release{Digest: "d1", Date: "2026-11-26T10:00:00Z", Components: comps("r1", "r2")}
	b2 := &Release{Digest: "d2", Date: "2026-11-26T11:00:00Z", Components: comps("r1", "r3")}
	b3 := &Release{Digest: "d3", Date: "2026-11-26T12:00:00Z", Components: comps("r1", "r3")}
	// A stale mark from an earlier read is cleared.
	b4 := &Release{Digest: "d4", Date: "2026-11-26T13:00:00Z", Components: comps("r4", "r3"), SameAs: "d3"}
	// No component revisions: never placed, never a re-cut.
	b5 := &Release{Digest: "d5", Date: "2026-11-26T14:00:00Z"}
	markRecuts(map[string]*Release{"d1": b1, "d2": b2, "d3": b3, "d4": b4, "d5": b5, "nil": nil})

	want := map[*Release]string{b1: "", b2: "", b3: "d2", b4: "", b5: ""}
	for r, w := range want {
		if r.SameAs != w {
			t.Errorf("%s SameAs = %q, want %q", r.Digest, r.SameAs, w)
		}
	}
}

func TestSDLCMatrixCarriesRecut(t *testing.T) {
	// Verifies: SDLC-VIS-11.07
	rel1 := &Release{Version: "v1", Digest: "d1", Date: "2026-09-01T00:00:00Z"}
	rel2 := &Release{Version: "v2", Digest: "d2", Date: "2026-09-15T00:00:00Z", SameAs: "d1"}
	promo := PromotionPayload{
		Order:        []string{"env-a"},
		Environments: map[string]Environment{"env-a": {Active: rel2, Cluster: "cluster-a", Env: "prod"}},
		Releases:     map[string]*Release{"d1": rel1, "d2": rel2},
	}
	s := newSDLCAt(fakeProvider{}, fakePromo{payload: promo, hasData: true}, fakeIssues{}, time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC))
	got := map[string]string{}
	for _, row := range fetch(t, s).Matrix.Rows {
		got[row.Version] = row.SameAs
	}
	if got["v2"] != "d1" || got["v1"] != "" {
		t.Errorf("matrix SameAs = %v, want v2 named a re-cut of d1 and v1 not", got)
	}
}
