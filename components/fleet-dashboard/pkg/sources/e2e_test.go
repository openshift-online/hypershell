package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/openshift-online/hypershell/components/fleet-dashboard/pkg/config"
)

// fakeLoki is the LokiClient test double: it returns canned history per tenant
// and a scripted tail channel, so the e2e plane is validated without a live Loki
// (e2e-run-log-visibility.spec E2E-LOG-17).
type fakeLoki struct {
	history  map[string][]LokiEntry
	hitLimit bool
	queryErr error
	tailErr  error

	mu        sync.Mutex
	tails     map[string]chan TailMessage
	tailCtx   map[string]context.Context
	tailCalls int
}

func (f *fakeLoki) QueryRange(_ context.Context, tenant, _ string, _, _ time.Time, _ int) ([]LokiEntry, bool, error) {
	if f.queryErr != nil {
		return nil, false, f.queryErr
	}
	return f.history[tenant], f.hitLimit, nil
}

func (f *fakeLoki) Tail(ctx context.Context, tenant, _ string) (<-chan TailMessage, error) {
	if f.tailErr != nil {
		return nil, f.tailErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tailCalls++
	if f.tails == nil {
		f.tails = map[string]chan TailMessage{}
	}
	if f.tailCtx == nil {
		f.tailCtx = map[string]context.Context{}
	}
	f.tailCtx[tenant] = ctx
	ch, ok := f.tails[tenant]
	if !ok {
		ch = make(chan TailMessage, 8)
		f.tails[tenant] = ch
	}
	return ch, nil
}

// tailContext returns the context the broker dialed the tenant's tail with, so a
// test can assert the broker cancels it (no goroutine/context leak).
func (f *fakeLoki) tailContext(tenant string) context.Context {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.tailCtx[tenant]
}

// tailChan returns (creating if needed) the scripted tail channel for a tenant.
func (f *fakeLoki) tailChan(tenant string) chan TailMessage {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.tails == nil {
		f.tails = map[string]chan TailMessage{}
	}
	ch, ok := f.tails[tenant]
	if !ok {
		ch = make(chan TailMessage, 8)
		f.tails[tenant] = ch
	}
	return ch
}

func (f *fakeLoki) calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.tailCalls
}

// viaQLine wraps a structured event in a ViaQ record envelope, as the collector
// delivers it: the suite's JSON line sits in the envelope's message field.
func viaQLine(t *testing.T, ev map[string]any) string {
	t.Helper()
	msg, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	env, err := json.Marshal(map[string]any{
		"message":    string(msg),
		"@timestamp": "2026-10-15T00:00:00Z",
		"kubernetes": map[string]any{"namespace_name": "ignored-label", "pod_name": "suite-pod"},
	})
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	return string(env)
}

func ev(event string, seq int64, extra map[string]any) map[string]any {
	m := map[string]any{
		"schema":    schemaE2EV1,
		"event":     event,
		"runId":     "run-1",
		"revision":  "hydrated-sha",
		"namespace": "hypershell",
		"cluster":   "body-cluster",
		"sequence":  seq,
		"timestamp": time.Unix(0, seq*int64(time.Millisecond)).UTC().Format(time.RFC3339Nano),
	}
	for k, v := range extra {
		m[k] = v
	}
	return m
}

func testE2E(loki LokiClient) *E2E {
	cfg := &config.Config{
		E2ELokiURL:        "http://loki.test",
		E2ELokiTenants:    []string{"hub-00"},
		E2ELokiSelector:   `{log_type="application"}`,
		E2EHistoryWindow:  168 * time.Hour,
		E2ERecentRunLimit: 20,
	}
	return newE2EWithClient(cfg, loki)
}

// Verifies: E2E-LOG-07.01
func TestParseViaQEnvelopeThenMessage(t *testing.T) {
	line := viaQLine(t, ev(e2eEventStepStarted, 3, map[string]any{"stepId": "P1.1", "name": "provisioning"}))
	got, ok := parseViaQEvent(line)
	if !ok {
		t.Fatal("well-formed ViaQ record should parse")
	}
	// Identity and step id come from the body, not from Loki index labels.
	if got.RunID != "run-1" || got.Revision != "hydrated-sha" || got.StepID != "P1.1" {
		t.Fatalf("body fields not read: %+v", got)
	}
}

func TestParseViaQMalformedIsReported(t *testing.T) {
	if _, ok := parseViaQEvent("not json at all"); ok {
		t.Error("non-JSON line must not parse as an event")
	}
	if _, ok := parseViaQEvent(`{"message":"{not json}"}`); ok {
		t.Error("malformed message body must not parse")
	}
	if _, ok := parseViaQEvent(`{"message":"{\"schema\":\"other\"}"}`); ok {
		t.Error("wrong schema must not parse")
	}
	if _, ok := parseViaQEvent(`{"kubernetes":{}}`); ok {
		t.Error("envelope without message must not parse")
	}
}

// Verifies: E2E-LOG-07.02
func TestReduceRunMalformedIsolated(t *testing.T) {
	entries := []LokiEntry{
		{Line: viaQLine(t, ev(e2eEventRunStarted, 1, nil))},
		{Line: "GARBAGE LINE NOT JSON"},
		{Line: viaQLine(t, ev(e2eEventStepStarted, 2, map[string]any{"stepId": "P1.1", "name": "provisioning"}))},
		{Line: viaQLine(t, ev(e2eEventStepFinished, 3, map[string]any{"stepId": "P1.1", "name": "provisioning", "outcome": "passed"}))},
	}
	runs := reduceRunsForTenant("hub-00", entries)
	if len(runs) != 1 {
		t.Fatalf("want 1 run after isolating malformed, got %d", len(runs))
	}
	if len(runs[0].Steps) != 1 || runs[0].Steps[0].State != stepStatePassed {
		t.Fatalf("remaining records should still reduce: %+v", runs[0].Steps)
	}
}

func TestReduceRunOrderingAndDedup(t *testing.T) {
	// Out-of-order, with a duplicated sequence (reconnect overlap).
	entries := []LokiEntry{
		{Line: viaQLine(t, ev(e2eEventStepFinished, 3, map[string]any{"stepId": "P1.1", "name": "provisioning", "outcome": "passed"}))},
		{Line: viaQLine(t, ev(e2eEventStepStarted, 2, map[string]any{"stepId": "P1.1", "name": "provisioning"}))},
		{Line: viaQLine(t, ev(e2eEventStepStarted, 2, map[string]any{"stepId": "P1.1", "name": "provisioning"}))}, // dup
		{Line: viaQLine(t, ev(e2eEventRunStarted, 1, map[string]any{"plannedSteps": []map[string]any{{"stepId": "P1.1", "name": "provisioning"}}}))},
		{Line: viaQLine(t, ev(e2eEventRunFinished, 4, map[string]any{"outcome": "passed"}))},
	}
	run := reduceRun("hub-00", parseEntries(t, entries))
	if run.Verdict != verdictPassed || !run.ReportingComplete {
		t.Fatalf("verdict/complete wrong: %+v", run)
	}
	if len(run.Steps) != 1 || run.Steps[0].State != stepStatePassed {
		t.Fatalf("step reduction wrong: %+v", run.Steps)
	}
}

func TestReduceRunMissingTerminalIsUnknown(t *testing.T) {
	entries := []LokiEntry{
		{Line: viaQLine(t, ev(e2eEventRunStarted, 1, nil))},
		{Line: viaQLine(t, ev(e2eEventStepStarted, 2, map[string]any{"stepId": "P1.1", "name": "provisioning"}))},
	}
	run := reduceRun("hub-00", parseEntries(t, entries))
	if run.Verdict != verdictUnknown {
		t.Errorf("verdict = %q, want unknown (no terminal record)", run.Verdict)
	}
	if run.ReportingComplete {
		t.Error("reportingComplete should be false without run_finished")
	}
	if run.Steps[0].State != stepStateRunning {
		t.Errorf("unfinished step state = %q, want running", run.Steps[0].State)
	}
}

func TestReduceRunSkippedVsNotRun(t *testing.T) {
	// P1.1 is mode-skipped; P1.2 is planned but never started (fail-fast not-run).
	entries := []LokiEntry{
		{Line: viaQLine(t, ev(e2eEventRunStarted, 1, map[string]any{"plannedSteps": []map[string]any{
			{"stepId": "P1.1", "name": "a"}, {"stepId": "P1.2", "name": "b"},
		}}))},
		{Line: viaQLine(t, ev(e2eEventStepStarted, 2, map[string]any{"stepId": "P1.1", "name": "a"}))},
		{Line: viaQLine(t, ev(e2eEventStepFinished, 3, map[string]any{"stepId": "P1.1", "name": "a", "outcome": "skipped"}))},
	}
	run := reduceRun("hub-00", parseEntries(t, entries))
	states := map[string]string{}
	for _, s := range run.Steps {
		states[s.StepID] = s.State
	}
	if states["P1.1"] != stepStateSkipped {
		t.Errorf("P1.1 = %q, want skipped", states["P1.1"])
	}
	if states["P1.2"] != stepStateNotRun {
		t.Errorf("P1.2 = %q, want not-run", states["P1.2"])
	}
	if states["P1.1"] == stepStatePassed || states["P1.2"] == stepStatePassed {
		t.Error("neither skipped nor not-run may read as passed")
	}
}

// Verifies: E2E-LOG-04.03
func TestReduceRunSteplessNotMarkedStructured(t *testing.T) {
	entries := []LokiEntry{
		{Line: viaQLine(t, ev(e2eEventRunStarted, 1, nil))},
		{Line: viaQLine(t, ev(e2eEventRunFinished, 2, map[string]any{"outcome": "passed"}))},
	}
	run := reduceRun("hub-00", parseEntries(t, entries))
	if run.HasStructuredSteps {
		t.Error("a run with no step events must not be marked as having structured steps")
	}
}

// Verifies: E2E-LOG-08.01
func TestReduceRunTrustedClusterIsTenant(t *testing.T) {
	entries := []LokiEntry{
		{Line: viaQLine(t, ev(e2eEventRunStarted, 1, nil))},
	}
	run := reduceRun("hub-00", parseEntries(t, entries))
	if run.Cluster != "hub-00" {
		t.Errorf("trusted cluster = %q, want the tenant hub-00", run.Cluster)
	}
	if run.BodyCluster != "body-cluster" {
		t.Errorf("body cluster = %q, want the body value as display metadata", run.BodyCluster)
	}
}

// Verifies: E2E-LOG-04.02
func TestRunsTruncationReported(t *testing.T) {
	loki := &fakeLoki{
		history:  map[string][]LokiEntry{"hub-00": {{Line: viaQLine(t, ev(e2eEventRunStarted, 1, nil))}}},
		hitLimit: true,
	}
	payload := mustRuns(t, testE2E(loki))
	if !payload.Truncated {
		t.Error("hitting the Loki line limit must flag the history truncated")
	}
	if !payload.Enabled {
		t.Error("configured feature must report enabled")
	}
}

func TestRunsSkipsEmptyNamespaceRun(t *testing.T) {
	// A run whose records carry no namespace cannot be addressed by a collision-free
	// id, so it must be omitted from the list rather than shown as an un-openable row.
	loki := &fakeLoki{history: map[string][]LokiEntry{"hub-00": {
		{Line: viaQLine(t, ev(e2eEventRunStarted, 1, map[string]any{"namespace": ""}))},
	}}}
	payload := mustRuns(t, testE2E(loki))
	if len(payload.Runs) != 0 {
		t.Errorf("want 0 runs (empty-namespace run skipped), got %d", len(payload.Runs))
	}
}

func TestRunsErrorWhenAllTenantsFail(t *testing.T) {
	loki := &fakeLoki{queryErr: context.DeadlineExceeded}
	if _, err := testE2E(loki).Runs(context.Background()); err == nil {
		t.Error("a fully-failed sweep must surface an error so the snapshot flags stale")
	}
}

// Verifies: E2E-LOG-01.02
func TestRunDetailAddressingNoCrossTenantCollision(t *testing.T) {
	// Two tenants share runId run-1; the address disambiguates by tenant/namespace.
	cfg := &config.Config{
		E2ELokiURL:        "http://loki.test",
		E2ELokiTenants:    []string{"hub-00", "hub-01"},
		E2ELokiSelector:   `{}`,
		E2EHistoryWindow:  time.Hour,
		E2ERecentRunLimit: 20,
	}
	loki := &fakeLoki{history: map[string][]LokiEntry{
		"hub-00": {{Line: viaQLine(t, ev(e2eEventStepFinished, 1, map[string]any{"stepId": "P1.1", "name": "a", "outcome": "passed"}))}},
		"hub-01": {{Line: viaQLine(t, ev(e2eEventStepFinished, 1, map[string]any{"stepId": "P1.1", "name": "a", "outcome": "failed"}))}},
	}}
	e := newE2EWithClient(cfg, loki)

	d0, err := e.RunDetail(context.Background(), makeRunID("hub-00", "hypershell", "run-1"))
	if err != nil {
		t.Fatalf("hub-00 detail: %v", err)
	}
	if d0.Cluster != "hub-00" || d0.Steps[0].State != stepStatePassed {
		t.Fatalf("hub-00 resolved wrong run: %+v", d0)
	}
	d1, err := e.RunDetail(context.Background(), makeRunID("hub-01", "hypershell", "run-1"))
	if err != nil {
		t.Fatalf("hub-01 detail: %v", err)
	}
	if d1.Cluster != "hub-01" || d1.Steps[0].State != stepStateFailed {
		t.Fatalf("hub-01 resolved wrong run: %+v", d1)
	}
}

func TestRunsErrorDoesNotLeakLokiIdentity(t *testing.T) {
	// A raw Loki error carries the internal service DNS, the LogQL query, and the
	// tenant. A fully-failed sweep must surface only the generic sentinel so none of
	// that crosses the BFF boundary into the client-facing snapshot (E2E-LOG-08).
	raw := fmt.Errorf(`Get "http://e2e-loki.hypershell-observability.svc:3100/loki/api/v1/query_range?query=%s&X-Scope-OrgID=hub-00": dial tcp refused`, `{log_type="application"}`)
	loki := &fakeLoki{queryErr: raw}
	_, err := testE2E(loki).Runs(context.Background())
	if err == nil {
		t.Fatal("a fully-failed sweep must surface an error")
	}
	if !errors.Is(err, ErrLogBackendUnavailable) {
		t.Errorf("err = %v, want ErrLogBackendUnavailable", err)
	}
	assertNoLokiIdentity(t, err.Error())
}

func TestRunDetailErrorDoesNotLeakLokiIdentity(t *testing.T) {
	raw := fmt.Errorf(`Get "http://e2e-loki.hypershell-observability.svc:3100/loki/api/v1/query_range?query=%s&X-Scope-OrgID=hub-00": dial tcp refused`, `{log_type="application"}`)
	loki := &fakeLoki{queryErr: raw}
	_, err := testE2E(loki).RunDetail(context.Background(), makeRunID("hub-00", "hypershell", "run-1"))
	if err == nil {
		t.Fatal("a failed run-detail query must surface an error")
	}
	if !errors.Is(err, ErrLogBackendUnavailable) {
		t.Errorf("err = %v, want ErrLogBackendUnavailable", err)
	}
	assertNoLokiIdentity(t, err.Error())
}

func assertNoLokiIdentity(t *testing.T, s string) {
	t.Helper()
	for _, leak := range []string{"e2e-loki", "svc:3100", "query_range", "log_type", "X-Scope-OrgID", "hub-00"} {
		if strings.Contains(s, leak) {
			t.Errorf("client-facing error leaked internal Loki identity %q: %s", leak, s)
		}
	}
}

func TestRunDetailDisallowedTenantNotFound(t *testing.T) {
	loki := &fakeLoki{}
	_, err := testE2E(loki).RunDetail(context.Background(), makeRunID("rogue-tenant", "ns", "run-1"))
	if err != ErrRunNotFound {
		t.Errorf("err = %v, want ErrRunNotFound for a tenant outside the allowlist", err)
	}
}

// parseEntries parses raw entries into events for the direct reduceRun tests.
func parseEntries(t *testing.T, entries []LokiEntry) []e2eEvent {
	t.Helper()
	var out []e2eEvent
	for _, e := range entries {
		if parsed, ok := parseViaQEvent(e.Line); ok {
			out = append(out, parsed)
		}
	}
	return out
}

func mustRuns(t *testing.T, e *E2E) E2ERunsPayload {
	t.Helper()
	got, err := e.Runs(context.Background())
	if err != nil {
		t.Fatalf("Runs: %v", err)
	}
	payload, ok := got.(E2ERunsPayload)
	if !ok {
		t.Fatalf("Runs returned %T, want E2ERunsPayload", got)
	}
	return payload
}

// Model a chatty command whose log window no longer contains the start/finish.
type windowedLoki struct {
	fakeLoki
	structural, logs []LokiEntry
	queries          []string
}

func (f *windowedLoki) QueryRange(_ context.Context, _ string, query string, _, _ time.Time, _ int) ([]LokiEntry, bool, error) {
	f.queries = append(f.queries, query)
	if strings.Contains(query, "run_started|") {
		return f.structural, false, nil
	}
	return f.logs, true, nil
}

// TestRunDetailPreservesLifecycleFromLifecycleQuery confirms the detail's steps and
// verdict come from the isolated lifecycle query, which is scoped to the run, so a
// chatty log window can never evict them (E2E-LOG-12). The detail no longer reads
// the raw-log history (E2E-LOG-20), so only the lifecycle query is issued.
func TestRunDetailPreservesLifecycleFromLifecycleQuery(t *testing.T) {
	f := &windowedLoki{structural: []LokiEntry{
		{Line: viaQLine(t, ev("run_started", 1, map[string]any{"plannedSteps": []any{map[string]any{"stepId": "a", "name": "API checks"}}}))},
		{Line: viaQLine(t, ev("step_finished", 2, map[string]any{"stepId": "a", "outcome": "passed"}))},
		{Line: viaQLine(t, ev("run_finished", 9001, map[string]any{"outcome": "passed"}))},
	}, logs: []LokiEntry{{Line: viaQLine(t, ev("log", 9000, map[string]any{"message": "latest diagnostic", "stepId": "a"}))}}}
	e := testE2E(f)
	detail, err := e.RunDetail(context.Background(), "hub-00/hypershell/run-1")
	if err != nil {
		t.Fatal(err)
	}
	if detail.Verdict != "passed" || !detail.ReportingComplete || len(detail.Steps) != 1 {
		t.Fatalf("lost lifecycle: %+v", detail)
	}
	if len(detail.Logs) != 0 {
		t.Errorf("detail must not embed logs, got %d", len(detail.Logs))
	}
	if len(f.queries) != 1 || !strings.Contains(f.queries[0], "run_started|") {
		t.Fatalf("detail should issue only the lifecycle query, got %v", f.queries)
	}
	for _, q := range f.queries {
		if !strings.Contains(q, `|= "run-1"`) {
			t.Fatalf("unscoped run query: %s", q)
		}
	}
}

// splitLoki distinguishes the lifecycle (steps/verdict) query from the raw-log
// query so a test can make the raw-log read fail while the lifecycle read
// succeeds (E2E-LOG-20).
type splitLoki struct {
	fakeLoki
	lifecycle    []LokiEntry
	logs         []LokiEntry
	lifecycleErr error
	logErr       error
	logCalls     int // how many non-lifecycle (raw-log) queries were issued
}

func (f *splitLoki) QueryRange(_ context.Context, _ string, query string, _, _ time.Time, _ int) ([]LokiEntry, bool, error) {
	if strings.Contains(query, "run_started|") {
		return f.lifecycle, false, f.lifecycleErr
	}
	f.logCalls++
	return f.logs, false, f.logErr
}

// TestRunDetailDecoupledFromRawLogs confirms the run detail is decoupled from the
// raw logs (E2E-LOG-20): steps and verdict come from the lifecycle query alone,
// the detail issues no raw-log query, and it embeds no logs. Even with a raw-log
// read that would error, the detail still serves steps and verdict.
// Verifies: E2E-LOG-20.01
func TestRunDetailDecoupledFromRawLogs(t *testing.T) {
	f := &splitLoki{
		lifecycle: []LokiEntry{
			{Line: viaQLine(t, ev("run_started", 1, map[string]any{"plannedSteps": []any{map[string]any{"stepId": "a", "name": "API checks"}}}))},
			{Line: viaQLine(t, ev("step_finished", 2, map[string]any{"stepId": "a", "outcome": "passed"}))},
			{Line: viaQLine(t, ev("run_finished", 3, map[string]any{"outcome": "passed"}))},
		},
		logErr: context.DeadlineExceeded,
	}
	detail, err := testE2E(f).RunDetail(context.Background(), "hub-00/hypershell/run-1")
	if err != nil {
		t.Fatalf("detail must render from the lifecycle query alone: %v", err)
	}
	if detail.Verdict != "passed" || len(detail.Steps) != 1 || detail.Steps[0].State != stepStatePassed {
		t.Fatalf("steps/verdict lost: %+v", detail)
	}
	if len(detail.Logs) != 0 {
		t.Errorf("run detail must not embed the raw-log history, got %d lines", len(detail.Logs))
	}
	if f.logCalls != 0 {
		t.Errorf("run detail issued %d raw-log queries, want 0 (logs come from the logs endpoint)", f.logCalls)
	}
}

// TestRunDetailBackendUnavailableWhenLifecycleFails confirms the genuine
// backend-unavailable state is reserved for a failed lifecycle/steps query
// (E2E-LOG-20.02).
// Verifies: E2E-LOG-20.02
func TestRunDetailBackendUnavailableWhenLifecycleFails(t *testing.T) {
	f := &splitLoki{lifecycleErr: context.DeadlineExceeded}
	_, err := testE2E(f).RunDetail(context.Background(), "hub-00/hypershell/run-1")
	if !errors.Is(err, ErrLogBackendUnavailable) {
		t.Errorf("err = %v, want ErrLogBackendUnavailable when steps cannot load", err)
	}
}

// pagingLoki emulates a backward Loki query_range over a fixed newest-first set,
// honoring the exclusive `end` bound and the `limit`, so RunLogs paging is
// validated without a live Loki (E2E-LOG-19).
type pagingLoki struct {
	fakeLoki
	all []LokiEntry // newest-first (descending TimestampNs)
}

func (f *pagingLoki) QueryRange(_ context.Context, _, _ string, _, end time.Time, limit int) ([]LokiEntry, bool, error) {
	endNs := end.UnixNano()
	var filtered []LokiEntry
	for _, e := range f.all {
		if e.TimestampNs < endNs { // query_range end is exclusive
			filtered = append(filtered, e)
		}
	}
	hitLimit := false
	if limit > 0 && len(filtered) > limit {
		filtered = filtered[:limit]
		hitLimit = true
	}
	return filtered, hitLimit, nil
}

// logEntry builds one raw-log LokiEntry with an explicit Loki timestamp (the
// paging cursor) carrying a structured log event with the given sequence.
func logEntry(t *testing.T, seq, tsNs int64, msg string) LokiEntry {
	t.Helper()
	return LokiEntry{Tenant: "hub-00", TimestampNs: tsNs, Line: viaQLine(t, ev("log", seq, map[string]any{"message": msg}))}
}

// walkLogs pages RunLogs oldest-ward from the newest page, de-duplicating by
// sequence (the client's responsibility across the one-nanosecond boundary
// overlap), until hasMore is false. Returns the de-duplicated sequences in first-
// seen order and the number of pages walked.
func walkLogs(t *testing.T, e *E2E, id string, pageSize int) ([]int64, int) {
	t.Helper()
	seen := map[int64]bool{}
	var order []int64
	before := ""
	pages := 0
	for {
		page, err := e.RunLogs(context.Background(), id, before, pageSize)
		if err != nil {
			t.Fatalf("RunLogs page %d: %v", pages, err)
		}
		pages++
		for _, l := range page.Entries {
			if !seen[l.Sequence] {
				seen[l.Sequence] = true
				order = append(order, l.Sequence)
			}
		}
		if !page.HasMore {
			break
		}
		if page.NextCursor == "" {
			t.Fatal("hasMore is true but nextCursor is empty")
		}
		if page.NextCursor == before {
			t.Fatalf("cursor did not advance (stuck at %s)", before)
		}
		before = page.NextCursor
		if pages > 50 {
			t.Fatal("paging did not terminate")
		}
	}
	return order, pages
}

// TestRunLogsPagingWalk confirms the newest page loads first and older pages load
// by cursor until hasMore is false, covering the whole run (E2E-LOG-19.01).
// Verifies: E2E-LOG-19.01
func TestRunLogsPagingWalk(t *testing.T) {
	f := &pagingLoki{all: []LokiEntry{
		logEntry(t, 5, 500, "e"),
		logEntry(t, 4, 400, "d"),
		logEntry(t, 3, 300, "c"),
		logEntry(t, 2, 200, "b"),
		logEntry(t, 1, 100, "a"),
	}}
	e := testE2E(f)

	first, err := e.RunLogs(context.Background(), "hub-00/hypershell/run-1", "", 2)
	if err != nil {
		t.Fatal(err)
	}
	if !first.HasMore || first.NextCursor == "" {
		t.Fatalf("first page should report more: %+v", first)
	}
	// The newest page is returned chronological-ascending (oldest first) for display.
	if len(first.Entries) != 2 || first.Entries[0].Sequence != 4 || first.Entries[1].Sequence != 5 {
		t.Fatalf("newest page not ascending newest lines: %+v", first.Entries)
	}

	order, pages := walkLogs(t, e, "hub-00/hypershell/run-1", 2)
	if pages < 2 {
		t.Fatalf("expected a multi-page walk, got %d pages", pages)
	}
	if len(order) != 5 {
		t.Fatalf("walk covered %d distinct lines, want 5: %v", len(order), order)
	}
}

// TestRunLogsBoundaryNotDroppedOrDuplicated confirms two entries sharing one
// nanosecond timestamp across a page boundary each appear exactly once after the
// client de-dupes the overlap (E2E-LOG-19.02).
// Verifies: E2E-LOG-19.02
func TestRunLogsBoundaryNotDroppedOrDuplicated(t *testing.T) {
	// seq 2 and seq 3 share timestamp 200; a page size of 3 cuts the group so only
	// seq 3 lands in page 1, leaving seq 2 at the boundary for page 2 to recover
	// through the one-nanosecond overlap.
	f := &pagingLoki{all: []LokiEntry{
		logEntry(t, 5, 400, "e"),
		logEntry(t, 4, 300, "d"),
		logEntry(t, 3, 200, "c"),
		logEntry(t, 2, 200, "b"),
		logEntry(t, 1, 100, "a"),
	}}
	e := testE2E(f)
	order, _ := walkLogs(t, e, "hub-00/hypershell/run-1", 3)
	want := map[int64]int{1: 1, 2: 1, 3: 1, 4: 1, 5: 1}
	got := map[int64]int{}
	for _, s := range order {
		got[s]++
	}
	for seq, n := range want {
		if got[seq] != n {
			t.Fatalf("sequence %d appeared %d times, want %d (dropped or duplicated across boundary): %v", seq, got[seq], n, order)
		}
	}
}

// TestRunLogsTerminatesOnOverfullTimestamp confirms paging terminates (rather than
// looping on a non-advancing cursor) when more entries share a single nanosecond
// than fit in one page. The guard trades the unreachable tail for liveness
// (E2E-LOG-19 "SHALL NOT assume unique timestamps").
func TestRunLogsTerminatesOnOverfullTimestamp(t *testing.T) {
	f := &pagingLoki{all: []LokiEntry{
		logEntry(t, 3, 200, "c"),
		logEntry(t, 2, 200, "b"),
		logEntry(t, 1, 200, "a"),
	}}
	_, pages := walkLogs(t, testE2E(f), "hub-00/hypershell/run-1", 2)
	if pages > 5 {
		t.Fatalf("paging must terminate on an over-full timestamp, walked %d pages", pages)
	}
}

// TestRunLogsLimitClamped confirms a client-supplied limit above the server
// maximum is clamped rather than issuing an unbounded query (E2E-LOG-19.03).
// Verifies: E2E-LOG-19.03
func TestRunLogsLimitClamped(t *testing.T) {
	if got := clampLogPageSize(100_000, 150); got != maxLogPageSize {
		t.Errorf("clampLogPageSize(100000) = %d, want %d", got, maxLogPageSize)
	}
	if got := clampLogPageSize(0, 150); got != 150 {
		t.Errorf("clampLogPageSize(0) = %d, want the default 150", got)
	}
	if got := clampLogPageSize(50, 150); got != 50 {
		t.Errorf("clampLogPageSize(50) = %d, want 50", got)
	}
}

// TestRunLogsDisallowedTenantNotFound confirms the logs endpoint honors the same
// tenant allowlist as the detail endpoint (E2E-LOG-19.04).
// Verifies: E2E-LOG-19.04
func TestRunLogsDisallowedTenantNotFound(t *testing.T) {
	_, err := testE2E(&pagingLoki{}).RunLogs(context.Background(), "rogue/ns/run-1", "", 0)
	if err != ErrRunNotFound {
		t.Errorf("err = %v, want ErrRunNotFound for a tenant outside the allowlist", err)
	}
}

type fakeHydratedResolver struct{ calls []string }

func (f *fakeHydratedResolver) ResolveHydrated(_ context.Context, revision string) *Release {
	f.calls = append(f.calls, revision)
	return &Release{Version: "v1", Digest: "sha256:tested"}
}

func TestE2EBundleEnrichment(t *testing.T) {
	loki := &fakeLoki{history: map[string][]LokiEntry{"hub-00": {
		{Line: viaQLine(t, ev("run_started", 1, nil))},
	}}}
	source := testE2E(loki)
	resolver := &fakeHydratedResolver{}
	source.versioner = resolver
	value, err := source.Runs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	runs := value.(E2ERunsPayload).Runs
	if len(runs) != 1 || runs[0].Bundle == nil || runs[0].Bundle.Digest != "sha256:tested" {
		t.Fatalf("bundle missing: %#v", runs)
	}
	detail, err := source.RunDetail(context.Background(), runs[0].ID)
	if err != nil || detail.Bundle == nil || detail.Bundle.Digest != "sha256:tested" {
		t.Fatalf("detail bundle missing: %#v %v", detail, err)
	}
	for _, revision := range resolver.calls {
		if revision != "hydrated-sha" {
			t.Fatalf("resolved wrong revision: %s", revision)
		}
	}
}
