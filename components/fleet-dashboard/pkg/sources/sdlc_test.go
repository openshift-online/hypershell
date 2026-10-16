package sources

import (
	"context"
	"testing"
	"time"

	"github.com/openshift-online/hypershell/components/fleet-dashboard/pkg/cache"
	"github.com/openshift-online/hypershell/components/fleet-dashboard/pkg/config"
)

type fakeProvider struct {
	records []SdlcRecord
	err     error
}

func (f fakeProvider) Records(context.Context) ([]SdlcRecord, error) { return f.records, f.err }

type fakePromo struct {
	payload PromotionPayload
	hasData bool
}

func (f fakePromo) Get() cache.Snapshot {
	if !f.hasData {
		return cache.Snapshot{}
	}
	return cache.Snapshot{Data: f.payload}
}

type fakeIssues struct{ m map[int][]int } // work-item number -> associated PR numbers

func (f fakeIssues) AssociatedPRs(_ context.Context, rec SdlcRecord) []int { return f.m[rec.Number] }

func testConfig() *config.Config {
	return &config.Config{SDLCPhaseSLE: map[string]time.Duration{
		"intake":        24 * time.Hour,
		"workable":      24 * time.Hour,
		"spec":          72 * time.Hour,
		"spec-approved": 24 * time.Hour,
		"code":          120 * time.Hour,
		"code-approved": 24 * time.Hour,
	}}
}

func newSDLCAt(provider SdlcStateProvider, promo promotionReader, issues WorkItemPRResolver, now time.Time) *SDLC {
	s := NewSDLC(testConfig(), provider, promo, issues)
	s.now = func() time.Time { return now }
	return s
}

func fetch(t *testing.T, s *SDLC) SDLCPayload {
	t.Helper()
	data, err := s.SDLC(context.Background())
	if err != nil {
		t.Fatalf("SDLC: %v", err)
	}
	p, ok := data.(SDLCPayload)
	if !ok {
		t.Fatalf("payload type %T", data)
	}
	return p
}

// TestSDLCBoardCardAndRework confirms a non-escalated in-review record becomes a
// board card carrying time-in-phase and a rework meter against its phase threshold.
func TestSDLCBoardCardAndRework(t *testing.T) {
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	rec := SdlcRecord{
		Number: 1, Title: "spec in review", Phase: PhaseSpec,
		PhaseSince: now.Add(-2 * time.Hour).Format(time.RFC3339), ReworkCount: 2,
	}
	s := newSDLCAt(fakeProvider{records: []SdlcRecord{rec}}, fakePromo{}, nil, now)
	p := fetch(t, s)

	if len(p.Cards) != 1 || len(p.Lane) != 0 {
		t.Fatalf("want 1 board card, 0 lane; got cards=%d lane=%d", len(p.Cards), len(p.Lane))
	}
	c := p.Cards[0]
	if c.TimeInPhaseSeconds != 7200 {
		t.Errorf("timeInPhase = %d, want 7200", c.TimeInPhaseSeconds)
	}
	if c.Rework == nil || c.Rework.Count != 2 || c.Rework.Threshold != specReworkThreshold {
		t.Errorf("rework = %+v, want {2 %d}", c.Rework, specReworkThreshold)
	}
	if c.History == nil {
		t.Errorf("history must serialize as [] not null")
	}
}

// TestSDLCCodeMeterThreshold confirms the code phase uses the 10-rejection
// threshold, so spec and code share one proportional encoding against their own
// limits (SDLC-VIS-09).
func TestSDLCCodeMeterThreshold(t *testing.T) {
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	rec := SdlcRecord{Number: 2, Phase: PhaseCode, PhaseSince: now.Format(time.RFC3339), ReworkCount: 7}
	s := newSDLCAt(fakeProvider{records: []SdlcRecord{rec}}, fakePromo{}, nil, now)
	p := fetch(t, s)
	if p.Cards[0].Rework.Threshold != codeReworkThreshold {
		t.Errorf("code threshold = %d, want %d", p.Cards[0].Rework.Threshold, codeReworkThreshold)
	}
}

// TestSDLCEscalationLane confirms paused and past-SLE records leave the board for
// the lane, ordered blocked first, then needs-input, then past-SLE (SDLC-VIS-10).
func TestSDLCEscalationLane(t *testing.T) {
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	records := []SdlcRecord{
		{Number: 1, Phase: PhaseSpec, PhaseSince: now.Add(-100 * time.Hour).Format(time.RFC3339)}, // past 72h SLE
		{Number: 2, Phase: PhaseCode, Paused: EscalationBlocked, PhaseSince: now.Format(time.RFC3339)},
		{Number: 3, Phase: PhaseSpec, Paused: EscalationNeedsInput, PhaseSince: now.Format(time.RFC3339)},
		{Number: 4, Phase: PhaseWorkable, PhaseSince: now.Format(time.RFC3339)}, // fresh, stays on board
	}
	s := newSDLCAt(fakeProvider{records: records}, fakePromo{}, nil, now)
	p := fetch(t, s)

	if len(p.Cards) != 1 || p.Cards[0].Number != 4 {
		t.Fatalf("board should hold only the fresh card #4, got %+v", p.Cards)
	}
	if len(p.Lane) != 3 {
		t.Fatalf("lane should hold 3, got %d", len(p.Lane))
	}
	gotOrder := []string{p.Lane[0].Escalation, p.Lane[1].Escalation, p.Lane[2].Escalation}
	want := []string{EscalationBlocked, EscalationNeedsInput, EscalationPastSLE}
	for i := range want {
		if gotOrder[i] != want[i] {
			t.Errorf("lane order = %v, want %v", gotOrder, want)
			break
		}
	}
}

// TestSDLCMatrix confirms the Shipped matrix reuses the promotion bundle/cluster
// truth, maps shipped work items to bundles via the work-item -> PRs hop, places
// bundle-less merged work in the single Unreleased row, and marks the current
// bundle per hub cluster (SDLC-VIS-03/11).
func TestSDLCMatrix(t *testing.T) {
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	rel1 := &Release{Version: "v20260901", Digest: "d1", Date: "2026-09-01T00:00:00Z", PRs: []PR{{Number: 101}}}
	rel2 := &Release{Version: "v20260915", Digest: "d2", Date: "2026-09-15T00:00:00Z", PRs: []PR{{Number: 102}}}
	promo := PromotionPayload{
		Order: []string{"env-a", "env-b"},
		Environments: map[string]Environment{
			"env-a": {Active: rel2, Cluster: "cluster-a", Env: "prod"},
			"env-b": {Active: rel1, Cluster: "cluster-b", Env: "stage"},
		},
		Releases: map[string]*Release{"d1": rel1, "d2": rel2},
	}
	shipped := []SdlcRecord{
		{Number: 501, Title: "in v1", Phase: PhaseShipped, Closed: true, ClosedAt: "2026-08-20T00:00:00Z"},
		{Number: 502, Title: "in v2", Phase: PhaseShipped, Closed: true, ClosedAt: "2026-09-10T00:00:00Z"},
		{Number: 503, Title: "merged, no bundle", Phase: PhaseShipped, Closed: true, ClosedAt: "2026-10-01T00:00:00Z"},
	}
	// Work item -> associated PRs: #501 shipped in PR 101 (bundle d1), #502 in PR 102
	// (bundle d2), #503 has no bundle PR (-> Unreleased).
	issues := fakeIssues{m: map[int][]int{501: {101}, 502: {102}}}
	s := newSDLCAt(fakeProvider{records: shipped}, fakePromo{payload: promo, hasData: true}, issues, now)
	p := fetch(t, s)

	if len(p.Cards) != 0 || len(p.Lane) != 0 {
		t.Fatalf("shipped records should not appear on the board/lane")
	}

	// Hub-cluster columns carry each cluster's current bundle version.
	byKey := map[string]SDLCHubCluster{}
	for _, hc := range p.Matrix.HubClusters {
		byKey[hc.Key] = hc
	}
	if byKey["env-a"].CurrentVersion != "v20260915" || byKey["env-b"].CurrentVersion != "v20260901" {
		t.Errorf("hub cluster current versions wrong: %+v", p.Matrix.HubClusters)
	}

	// Rows: Unreleased first, then v2 (newest), then v1.
	rows := p.Matrix.Rows
	if len(rows) != 3 {
		t.Fatalf("want 3 rows (Unreleased + 2 bundles), got %d", len(rows))
	}
	if !rows[0].Unreleased || len(rows[0].Issues) != 1 || rows[0].Issues[0].Number != 503 {
		t.Errorf("Unreleased row should hold #503 only, got %+v", rows[0])
	}
	if rows[0].Version != "" || len(rows[0].HubClusters) != 0 {
		t.Errorf("Unreleased row must carry no version and no hub markers, got %+v", rows[0])
	}
	if rows[1].Version != "v20260915" || len(rows[1].Issues) != 1 || rows[1].Issues[0].Number != 502 {
		t.Errorf("newest bundle row wrong: %+v", rows[1])
	}
	if len(rows[1].HubClusters) != 1 || rows[1].HubClusters[0] != "env-a" {
		t.Errorf("v2 should be marked current on env-a, got %+v", rows[1].HubClusters)
	}
	if rows[2].Version != "v20260901" || rows[2].Issues[0].Number != 501 {
		t.Errorf("oldest bundle row wrong: %+v", rows[2])
	}
	if rows[1].Issues[0].MergedAt != "2026-09-10T00:00:00Z" {
		t.Errorf("shipped issue should carry its merged timestamp, got %q", rows[1].Issues[0].MergedAt)
	}
}

// TestSDLCMatrixWorkItemMultiplePRs confirms a work item delivered by several PRs
// across bundles is attributed to the NEWEST bundle any of its PRs landed in (the
// release by which it was fully delivered), not the oldest (SDLC-VIS-03).
func TestSDLCMatrixWorkItemMultiplePRs(t *testing.T) {
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	rel1 := &Release{Version: "v1", Digest: "d1", Date: "2026-09-01T00:00:00Z", PRs: []PR{{Number: 101}}}
	rel2 := &Release{Version: "v2", Digest: "d2", Date: "2026-09-15T00:00:00Z", PRs: []PR{{Number: 102}}}
	promo := PromotionPayload{
		Order:        []string{"env-a"},
		Environments: map[string]Environment{"env-a": {Active: rel2, Cluster: "c", Env: "prod"}},
		Releases:     map[string]*Release{"d1": rel1, "d2": rel2},
	}
	shipped := []SdlcRecord{
		{Number: 700, Title: "split across two bundles", Phase: PhaseShipped, Closed: true, ClosedAt: "2026-09-16T00:00:00Z"},
	}
	// #700 links PRs in both d1 (older) and d2 (newer); it should land in d2.
	issues := fakeIssues{m: map[int][]int{700: {101, 102}}}
	s := newSDLCAt(fakeProvider{records: shipped}, fakePromo{payload: promo, hasData: true}, issues, now)
	p := fetch(t, s)

	byVersion := map[string][]SDLCShippedIssue{}
	for _, row := range p.Matrix.Rows {
		if row.Unreleased {
			if len(row.Issues) != 0 {
				t.Errorf("Unreleased should be empty, got %+v", row.Issues)
			}
			continue
		}
		byVersion[row.Version] = row.Issues
	}
	if len(byVersion["v2"]) != 1 || byVersion["v2"][0].Number != 700 {
		t.Errorf("#700 should attribute to newest bundle v2, got v2=%+v", byVersion["v2"])
	}
	if len(byVersion["v1"]) != 0 {
		t.Errorf("#700 must not also appear in older bundle v1, got v1=%+v", byVersion["v1"])
	}
}

// TestSDLCMatrixWithoutPromotion confirms the matrix degrades gracefully when the
// promotion snapshot is cold: all shipped work collects in the Unreleased row and
// no hub-cluster columns are emitted.
func TestSDLCMatrixWithoutPromotion(t *testing.T) {
	now := time.Now()
	shipped := []SdlcRecord{{Number: 1, Phase: PhaseShipped, Closed: true}}
	s := newSDLCAt(fakeProvider{records: shipped}, fakePromo{}, nil, now)
	p := fetch(t, s)
	if len(p.Matrix.HubClusters) != 0 {
		t.Errorf("no hub clusters expected without promotion data")
	}
	if len(p.Matrix.Rows) != 1 || !p.Matrix.Rows[0].Unreleased || len(p.Matrix.Rows[0].Issues) != 1 {
		t.Errorf("all shipped work should sit in Unreleased, got %+v", p.Matrix.Rows)
	}
}

// TestSDLCProviderError confirms a provider failure propagates so the cache serves
// last-good flagged stale (SDLC-VIS-01), rather than a blank successful snapshot.
func TestSDLCProviderError(t *testing.T) {
	s := newSDLCAt(fakeProvider{err: context.DeadlineExceeded}, fakePromo{}, nil, time.Now())
	if _, err := s.SDLC(context.Background()); err == nil {
		t.Fatal("expected provider error to propagate")
	}
}

// TestSDLCPhasesCarrySLE confirms the board phases expose their configured SLE and
// that Shipped is terminal with no SLE line.
func TestSDLCPhasesCarrySLE(t *testing.T) {
	now := time.Now()
	s := newSDLCAt(fakeProvider{}, fakePromo{}, nil, now)
	p := fetch(t, s)
	if len(p.Phases) != len(config.SDLCPhaseKeys)+1 {
		t.Fatalf("want %d phases, got %d", len(config.SDLCPhaseKeys)+1, len(p.Phases))
	}
	byKey := map[string]int{}
	for _, ph := range p.Phases {
		byKey[ph.Key] = ph.SLESeconds
	}
	if byKey["spec"] != int((72 * time.Hour).Seconds()) {
		t.Errorf("spec SLE seconds = %d", byKey["spec"])
	}
	if byKey[PhaseShipped] != 0 {
		t.Errorf("shipped must have no SLE, got %d", byKey[PhaseShipped])
	}
}
