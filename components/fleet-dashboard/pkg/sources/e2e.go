package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/openshift-online/hypershell/components/fleet-dashboard/pkg/config"
)

// e2e-run-log-visibility.spec (HYPERSHELL-402) consumer. This source reads the
// opt-in structured run reporting the Go E2E suite emits
// (e2e-testing.spec "Opt-In Structured Run Reporting", schema hypershell.e2e.v1)
// from a central Loki receiver and shapes it into the glass dashboard's e2e-run
// view: recent runs (bounded query_range history) and, via e2e_stream.go, a live
// tail. The log pipeline (the Loki receiver, retention, and the
// ClusterLogForwarder) is owned by the GitOps delivery repository and consumed
// here, never restated.

// schemaE2EV1 is the structured event envelope this consumer understands. It
// mirrors harness.SchemaE2EV1 on the suite side; a record with any other schema
// is not a structured e2e event and is ignored.
const schemaE2EV1 = "hypershell.e2e.v1"

// Structured event names (mirror the suite reporter).
const (
	e2eEventRunStarted   = "run_started"
	e2eEventStepStarted  = "step_started"
	e2eEventStepFinished = "step_finished"
	e2eEventLog          = "log"
	e2eEventRunFinished  = "run_finished"
)

// Run verdicts. A run with no terminal run_finished record reads as unknown
// (incomplete), never as passed (E2E-LOG, terminal integrity).
const (
	verdictPassed  = "passed"
	verdictFailed  = "failed"
	verdictUnknown = "unknown"
)

// Step states in the reduced view. not-run (a fail-fast step never reached) is
// distinct from skipped (a mode-skipped step); neither reads as passed.
const (
	stepStatePassed  = "passed"
	stepStateFailed  = "failed"
	stepStateSkipped = "skipped"
	stepStateRunning = "running"
	stepStateNotRun  = "not-run"
)

// maxDetailLogLines bounds how many log lines a run detail carries, so a chatty
// run cannot produce an unbounded payload. The suite already bounds each line.
const maxDetailLogLines = 2000

// e2eEvent is one parsed structured suite event (the JSON inside the ViaQ
// message field). Only the fields this consumer reads are modeled.
type e2eEvent struct {
	Schema         string       `json:"schema"`
	Event          string       `json:"event"`
	RunID          string       `json:"runId"`
	Revision       string       `json:"revision"`
	SourceRevision string       `json:"sourceRevision"`
	Cluster        string       `json:"cluster"`
	Namespace      string       `json:"namespace"`
	Sequence       int64        `json:"sequence"`
	Timestamp      string       `json:"timestamp"`
	StepID         string       `json:"stepId"`
	Name           string       `json:"name"`
	Outcome        string       `json:"outcome"`
	DurationMs     *int64       `json:"durationMs"`
	Message        string       `json:"message"`
	Truncated      bool         `json:"truncated"`
	PlannedSteps   []E2EPlanned `json:"plannedSteps"`
}

// E2EPlanned names one step in a run's planned set.
type E2EPlanned struct {
	StepID string `json:"stepId"`
	Name   string `json:"name"`
}

// E2ERunsPayload is the /api/e2e/runs plane data (wrapped in cache.Snapshot).
type E2ERunsPayload struct {
	// Enabled is false when the feature is unconfigured; the UI shows the feature
	// off rather than an error (E2E-LOG-01/02).
	Enabled bool `json:"enabled"`
	// Runs is the recent runs, newest-first, capped at the configured limit.
	Runs []E2ERunSummary `json:"runs"`
	// Truncated is true when more runs exist in the window than the limit returns,
	// or a tenant's history hit the Loki line limit (E2E-LOG-04).
	Truncated     bool  `json:"truncated"`
	WindowSeconds int64 `json:"windowSeconds"`
	Limit         int   `json:"limit"`
}

// E2ERunSummary is one recent run in the list.
type E2ERunSummary struct {
	Bundle *Release `json:"bundle,omitempty"`
	// ID addresses the run including its tenant and namespace so ids cannot
	// collide across clusters (E2E-LOG-01). Form: "<tenant>/<namespace>/<runId>".
	ID     string `json:"id"`
	RunID  string `json:"runId"`
	Tenant string `json:"tenant"`
	// Cluster is the TRUSTED cluster identity derived from the Loki tenant the
	// record was read from (E2E-LOG-08). BodyCluster is the record body's cluster
	// field, kept as display metadata only, never as an authorization input.
	Cluster     string `json:"cluster"`
	BodyCluster string `json:"bodyCluster,omitempty"`
	Namespace   string `json:"namespace"`
	// Revision is the hydrated/deployed revision; SourceRevision is the
	// application test-source revision, displayed separately (E2E-LOG-09).
	Revision       string `json:"revision"`
	SourceRevision string `json:"sourceRevision,omitempty"`
	StartedAt      string `json:"startedAt"`
	EndedAt        string `json:"endedAt,omitempty"`
	DurationMs     *int64 `json:"durationMs,omitempty"`
	Verdict        string `json:"verdict"`
	// ReportingComplete is false when no terminal run_finished was observed (the
	// run is incomplete/unknown, not a silent pass).
	ReportingComplete bool `json:"reportingComplete"`
	// HasStructuredSteps is false for a run with no step events (for example a
	// pre-reporting Bash run); such a run is not rendered as a stepless pass
	// (E2E-LOG-04).
	HasStructuredSteps bool `json:"hasStructuredSteps"`
	StepCount          int  `json:"stepCount"`
}

// E2ERunDetail is the /api/e2e/runs/{id} plane data: a summary plus ordered
// steps and a bounded log history.
type E2ERunDetail struct {
	E2ERunSummary
	PlannedSteps []E2EPlanned `json:"plannedSteps"`
	Steps        []E2EStep    `json:"steps"`
	// Logs is intentionally NOT populated by the run-detail endpoint: steps and
	// verdict come from the small, always-fast lifecycle query, and the raw-log
	// history is read a bounded page at a time through the logs endpoint (E2E-LOG-19,
	// E2E-LOG-20). The field is retained because the framework-free reducers still
	// model a run's logs for the live/snapshot fold; a detail served over the wire
	// carries it empty.
	Logs []E2ELogLine `json:"logs"`
	// LogsTruncated is retained for reducer/wire parity but is NOT set by the
	// run-detail endpoint: the detail carries no logs (E2E-LOG-20), and log-history
	// completeness is reported per page by the logs endpoint's hasMore (E2E-LOG-19),
	// not by a detail-level flag.
	LogsTruncated bool `json:"logsTruncated"`
}

// E2ELogPage is one bounded page of a run's raw-log history, served by the
// paginated logs endpoint (E2E-LOG-19). Entries are chronological-ascending
// (oldest first) for display; NextCursor is the oldest timestamp in the page (a
// decimal nanosecond string) to request the immediately older page, and HasMore
// reports that the page was full so an older page may exist.
type E2ELogPage struct {
	Entries    []E2ELogLine `json:"entries"`
	NextCursor string       `json:"nextCursor,omitempty"`
	HasMore    bool         `json:"hasMore"`
}

// E2EStep is one reduced step with its derived state and timing.
type E2EStep struct {
	StepID     string `json:"stepId"`
	Name       string `json:"name"`
	State      string `json:"state"`
	StartedAt  string `json:"startedAt,omitempty"`
	EndedAt    string `json:"endedAt,omitempty"`
	DurationMs *int64 `json:"durationMs,omitempty"`
}

// E2ELogLine is one reduced log record.
type E2ELogLine struct {
	Sequence  int64  `json:"sequence"`
	Timestamp string `json:"timestamp"`
	StepID    string `json:"stepId,omitempty"`
	Message   string `json:"message"`
	Truncated bool   `json:"truncated,omitempty"`
}

// E2E is the e2e-run data plane. It reads the structured stream through the
// LokiClient port (E2E-LOG-03); the Loki adapter is the only implementation
// shipped here. It is nil-safe-constructed: NewE2E returns nil when the feature
// is unconfigured, so the caller mounts the endpoints in a feature-off state.
type HydratedVersionResolver interface {
	ResolveHydrated(context.Context, string) *Release
}

type E2E struct {
	versioner     HydratedVersionResolver
	loki          LokiClient
	tenants       []string
	selector      string
	window        time.Duration
	limit         int
	logPageSize   int
	logTimeMargin time.Duration
	logger        *slog.Logger

	// broker shares bounded upstream tails across subscribed browsers within this
	// replica (E2E-LOG-05); created lazily by the stream handler.
	broker *tailBroker
}

// NewE2E builds the e2e-run source from config, or returns nil when the feature
// is disabled (FD_E2E_LOKI_URL empty, E2E-LOG-02).
func NewE2E(c *config.Config, versioner HydratedVersionResolver) *E2E {
	if c.E2ELokiURL == "" {
		return nil
	}
	loki := newLokiAdapter(c)
	e := newE2EWithClient(c, loki)
	e.versioner = versioner
	return e
}

// newE2EWithClient is the test seam: it builds the source around an injected
// LokiClient so behavior is validated without a live Loki.
func newE2EWithClient(c *config.Config, loki LokiClient) *E2E {
	e := &E2E{
		loki:          loki,
		tenants:       append([]string(nil), c.E2ELokiTenants...),
		selector:      c.E2ELokiSelector,
		window:        c.E2EHistoryWindow,
		limit:         c.E2ERecentRunLimit,
		logPageSize:   c.E2ELogPageSize,
		logTimeMargin: c.E2ELogTimeMargin,
		logger:        slog.Default(),
	}
	e.broker = newTailBroker(loki, e.selector, e.logger)
	return e
}

// Runs is the cache.FetchFunc for the /api/e2e/runs plane: a bounded
// query_range sweep per tenant over the lookback window, reduced into recent
// runs newest-first and capped at the configured limit.
func (e *E2E) Runs(ctx context.Context) (any, error) {
	end := time.Now()
	start := end.Add(-e.window)

	var all []E2ERunSummary
	truncated := false
	var firstErr error
	for _, tenant := range e.tenants {
		// The per-tenant line cap bounds the query; a tenant that returns exactly
		// the cap may have more history, which we report as truncated rather than
		// present as complete (E2E-LOG-04).
		entries, hitLimit, err := e.loki.QueryRange(ctx, tenant, lifecycleQuery(e.selector), start, end, lokiQueryLimit)
		if err != nil {
			// Degrade, do not fail the whole sweep: one bad tenant must not blank the
			// others. Remember the first error so a fully-failed sweep surfaces it.
			if firstErr == nil {
				firstErr = fmt.Errorf("query tenant %s: %w", tenant, err)
			}
			e.logger.WarnContext(ctx, "e2e recent-runs query failed for tenant", "tenant", tenant, "err", err)
			continue
		}
		if hitLimit {
			truncated = true
		}
		for _, run := range reduceRunsForTenant(tenant, entries) {
			// A run with no namespace cannot be addressed by a collision-free id
			// (tenant/namespace/runId), so its detail would be unreachable. Skip it
			// from the list rather than present an entry that 404s when opened.
			if run.Namespace == "" {
				e.logger.WarnContext(ctx, "skipping e2e run with empty namespace", "tenant", tenant, "runId", run.RunID)
				continue
			}
			all = append(all, run.E2ERunSummary)
		}
	}
	if len(all) == 0 && firstErr != nil {
		// Every tenant failed and there is nothing to serve: surface an error so the
		// cache flags the snapshot stale rather than publishing an empty list as if it
		// were complete. The detailed firstErr (already logged per tenant above)
		// carries the internal Loki identity, so the client sees only the generic
		// sentinel (E2E-LOG-08).
		return nil, ErrLogBackendUnavailable
	}

	sort.Slice(all, func(i, j int) bool {
		if all[i].StartedAt != all[j].StartedAt {
			return all[i].StartedAt > all[j].StartedAt // RFC3339Nano sorts lexically
		}
		return all[i].ID < all[j].ID
	})
	if e.limit > 0 && len(all) > e.limit {
		all = all[:e.limit]
		truncated = true
	}
	resolved := map[string]*Release{}
	for i := range all {
		if e.versioner != nil {
			release, found := resolved[all[i].Revision]
			if !found {
				release = e.versioner.ResolveHydrated(ctx, all[i].Revision)
				resolved[all[i].Revision] = release
			}
			all[i].Bundle = release
		}
	}
	return E2ERunsPayload{
		Enabled:       true,
		Runs:          all,
		Truncated:     truncated,
		WindowSeconds: int64(e.window.Seconds()),
		Limit:         e.limit,
	}, nil
}

// RunDetail returns the ordered steps and verdict for one run addressed by its
// collision-free id. The run id's tenant is the trusted cluster identity; the
// record body cluster is ignored for authorization (E2E-LOG-08).
//
// Steps and verdict come from the small, always-fast lifecycle query ALONE and are
// DECOUPLED from the raw-log history (E2E-LOG-20): the detail does not read or
// embed raw logs, so a slow or failed log read can never blank the steps that were
// cheap to compute, and a chatty run's detail never risks the all-at-once log
// timeout. The raw-log history is read a bounded page at a time through RunLogs
// (E2E-LOG-19); the log pane's degraded state is driven by that endpoint, not by
// the detail. The genuine backend-unavailable sentinel is reserved for the case
// where even this lifecycle query cannot load.
func (e *E2E) RunDetail(ctx context.Context, id string) (*E2ERunDetail, error) {
	tenant, namespace, runID, err := parseRunID(id)
	if err != nil {
		return nil, err
	}
	if !e.tenantAllowed(tenant) {
		// A tenant not in the allowlist is not addressable; treat it as not found
		// rather than querying an arbitrary tenant (E2E-LOG-03/08).
		return nil, ErrRunNotFound
	}
	end := time.Now()
	start := end.Add(-e.window)

	runSelector := e.selector + " |= " + strconv.Quote(runID)
	// Only the lifecycle query runs here; its line count is small by construction, so
	// its limit flag is not a raw-log-truncation signal and is intentionally ignored.
	lifecycle, _, err := e.loki.QueryRange(ctx, tenant, lifecycleQuery(runSelector), start, end, lokiQueryLimit)
	if err != nil {
		// The lifecycle/steps query itself failed, so there is nothing cheap to serve:
		// this is the genuine backend-unavailable case (E2E-LOG-20). Log the detailed
		// error (it carries the internal Loki identity) server-side and return the
		// generic sentinel so the browser never sees the service DNS, LogQL, or tenant
		// (E2E-LOG-08).
		e.logger.WarnContext(ctx, "e2e run detail lifecycle query failed", "tenant", tenant, "err", err)
		return nil, ErrLogBackendUnavailable
	}

	for _, run := range reduceRunsForTenant(tenant, lifecycle) {
		if run.RunID == runID && run.Namespace == namespace {
			if e.versioner != nil {
				run.Bundle = e.versioner.ResolveHydrated(ctx, run.Revision)
			}
			return run, nil
		}
	}
	return nil, ErrRunNotFound
}

// maxLogPageSize is the server-side clamp on a client-supplied logs page size, so
// a client cannot re-introduce the all-at-once timeout by requesting a huge page
// (E2E-LOG-19). It stays well below the measured single-CPU Loki cliff.
const maxLogPageSize = 500

// clampLogPageSize resolves the effective page size: the client's limit when
// positive, else the configured default, clamped to maxLogPageSize (and never
// below 1).
func clampLogPageSize(requested, def int) int {
	size := requested
	if size <= 0 {
		size = def
	}
	if size <= 0 {
		size = 1
	}
	if size > maxLogPageSize {
		size = maxLogPageSize
	}
	return size
}

// RunLogs reads one bounded page of a run's raw-log history (E2E-LOG-19), newest
// page first and then older pages by the before cursor, so every request stays
// well below the Loki query timeout even on a single-CPU Loki. It reuses the
// exact run address, tenant allowlist, and authorization as RunDetail (the same
// ErrRunNotFound on an unknown tenant).
//
// stepID is an optional selected-step scope (E2E-LOG-23): when non-empty the page
// is filtered to that one step - the step id is pushed into the Loki query as a
// line filter so the limit applies to that step's lines rather than a run-wide
// page (E2E-LOG-24), and the time range is bounded by the step's lifecycle window
// (E2E-LOG-25). When stepID is empty the endpoint preserves the run-wide behavior
// unchanged.
//
// before is an optional nanosecond-timestamp cursor (exclusive upper bound;
// empty means the newest page). limit is clamped to the server-side maximum. The
// page is read direction=backward (newest first) and reversed to
// chronological-ascending for display. Because Loki has no opaque offset token
// and multiple entries can share one nanosecond timestamp, the query uses a
// one-nanosecond inclusive overlap at the boundary; the returned entries carry a
// stable per-entry sequence the caller de-dupes on, so a page boundary neither
// drops nor duplicates an entry.
func (e *E2E) RunLogs(ctx context.Context, id, stepID, before string, limit int) (*E2ELogPage, error) {
	tenant, namespace, runID, err := parseRunID(id)
	if err != nil {
		return nil, err
	}
	if !e.tenantAllowed(tenant) {
		return nil, ErrRunNotFound
	}
	size := clampLogPageSize(limit, e.logPageSize)

	var beforeNs int64
	hasBefore := false
	if before != "" {
		ns, perr := strconv.ParseInt(before, 10, 64)
		if perr != nil {
			return nil, fmt.Errorf("invalid before cursor %q", before)
		}
		beforeNs = ns
		hasBefore = true
	}

	// Resolve the query time range. Run-wide history uses a window relative to the
	// cursor (E2E-LOG-19); a selected step bounds the range to that step's lifecycle
	// window with a configurable ingestion/skew margin and a STABLE lower bound that
	// does not move backward as the cursor pages older (E2E-LOG-25).
	var start, end time.Time
	if stepID != "" {
		lower, upper, ok, werr := e.stepLogWindow(ctx, tenant, namespace, runID, stepID)
		if werr != nil {
			e.logger.WarnContext(ctx, "e2e step log window query failed", "tenant", tenant, "err", werr)
			return nil, ErrLogBackendUnavailable
		}
		if ok {
			start, end = lower, upper
		} else {
			// Documented fallback: lifecycle timing is missing or incomplete, so fall
			// back to the configured history window rather than silently omitting valid
			// logs (E2E-LOG-25). The step filter still scopes the page to the step.
			end = time.Now()
			start = end.Add(-e.window)
		}
	} else {
		end = time.Now()
		start = end.Add(-e.window)
	}

	if hasBefore {
		// One-nanosecond inclusive overlap: end is exclusive in query_range, so
		// end = before + 1ns keeps the boundary timestamp in range and no entry that
		// shares the cursor's exact nanosecond is dropped. The caller de-dupes the
		// re-delivered boundary entries by sequence (E2E-LOG-19). The cursor only ever
		// narrows the upper bound; the lower bound stays fixed (E2E-LOG-25).
		cursorEnd := time.Unix(0, beforeNs+1)
		if cursorEnd.Before(end) {
			end = cursorEnd
		}
		if stepID == "" {
			// Run-wide history keeps its relative lookback window around the cursor.
			start = end.Add(-e.window)
		}
	}

	runSelector := e.selector + " |= " + strconv.Quote(runID)
	if stepID != "" {
		// Push the step scope into the query so Loki applies the page limit to this
		// step's lines, not a run-wide page filtered afterwards (E2E-LOG-24). The step
		// id rides a line filter on the pipeline/body, not a stream label, so run and
		// step ids never become high-cardinality labels (E2E-LOG-24.03).
		//
		// The filter matches the serialized JSON field value token - the step id
		// wrapped in the (ViaQ-escaped) quote pair as it appears in the structured
		// event's "stepId":"<id>" field, i.e. the literal bytes \"<id>\" in the Loki
		// line. Matching the quoted value rather than a bare substring disambiguates
		// step ids that are prefixes of one another (e.g. "build" vs "build-image")
		// and low-entropy names that could otherwise appear in another step's message,
		// so the bounded page is not diluted before the limit. Go-side validation below
		// still confirms the parsed event's step id matches exactly (defense in depth).
		runSelector += " |= " + strconv.Quote(`\"`+stepID+`\"`)
	}
	entries, hitLimit, err := e.loki.QueryRange(ctx, tenant, runSelector, start, end, size)
	if err != nil {
		e.logger.WarnContext(ctx, "e2e run logs query failed", "tenant", tenant, "err", err)
		return nil, ErrLogBackendUnavailable
	}

	// entries are newest-first (Loki backward). The oldest raw entry's timestamp is
	// the cursor for the next (older) page, independent of whether that entry reduced
	// to a log line, so paging stays monotonic. Collect only well-formed log events
	// for this run.
	var oldestNs int64
	haveOldest := false
	logs := make([]E2ELogLine, 0, len(entries))
	for _, entry := range entries {
		if !haveOldest || entry.TimestampNs < oldestNs {
			oldestNs = entry.TimestampNs
			haveOldest = true
		}
		ev, ok := parseViaQEvent(entry.Line)
		if !ok || ev.Event != e2eEventLog || ev.RunID != runID {
			continue
		}
		if ev.Namespace != "" && namespace != "" && ev.Namespace != namespace {
			continue
		}
		// Selected-step scope (E2E-LOG-23/24): a log event is attributed to the step
		// only when its step id matches exactly. An older event missing the optional
		// stepId field therefore cannot belong to a named step and is intentionally
		// excluded from a step-scoped page (E2E-LOG-24), while the run-wide page keeps
		// it. The pre-limit line filter above is a substring match, so this is the
		// authoritative, defensive filter.
		if stepID != "" && ev.StepID != stepID {
			continue
		}
		logs = append(logs, E2ELogLine{
			Sequence:  ev.Sequence,
			Timestamp: ev.Timestamp,
			StepID:    ev.StepID,
			Message:   ev.Message,
			Truncated: ev.Truncated,
		})
	}
	// Reverse newest-first into chronological-ascending for display (E2E-LOG-19).
	for i, j := 0, len(logs)-1; i < j; i, j = i+1, j-1 {
		logs[i], logs[j] = logs[j], logs[i]
	}

	page := &E2ELogPage{Entries: logs, HasMore: hitLimit}
	if hitLimit && haveOldest {
		// Guard the pathological case where more than one page of entries share a
		// single nanosecond: the next cursor would equal the current one and paging
		// would never advance (E2E-LOG-19 "SHALL NOT assume unique timestamps"). Stop
		// paging rather than hand the client a cursor that loops.
		if hasBefore && oldestNs == beforeNs {
			page.HasMore = false
		} else {
			page.NextCursor = strconv.FormatInt(oldestNs, 10)
		}
	}
	return page, nil
}

// stepLogWindow resolves the lifecycle-bounded Loki time range for a selected
// step's raw-log history (E2E-LOG-25). It reads the same small, always-fast
// lifecycle query the run detail uses (E2E-LOG-20) to recover the step's start and
// end times, then returns:
//
//   - lower: the step's start minus the configured ingestion/skew margin. This is a
//     STABLE lower bound: it depends only on the step's lifecycle, not on the paging
//     cursor, so paging older never widens the window backwards (E2E-LOG-25).
//   - upper: for a completed step, the step's end plus the margin, so the query does
//     not scan unrelated later history; for a still-running step (no end time), the
//     current time, so freshly emitted lines remain visible (E2E-LOG-25).
//
// ok is false when the step is absent or its start time is missing/unparseable, so
// the caller applies the documented fallback window rather than omitting logs. The
// suite event timestamps and the Loki ingestion timestamps are not identical; the
// margin absorbs that skew and ingestion delay (E2E-LOG-25).
func (e *E2E) stepLogWindow(ctx context.Context, tenant, namespace, runID, stepID string) (lower, upper time.Time, ok bool, err error) {
	end := time.Now()
	start := end.Add(-e.window)
	runSelector := e.selector + " |= " + strconv.Quote(runID)
	lifecycle, _, qerr := e.loki.QueryRange(ctx, tenant, lifecycleQuery(runSelector), start, end, lokiQueryLimit)
	if qerr != nil {
		return time.Time{}, time.Time{}, false, qerr
	}
	for _, run := range reduceRunsForTenant(tenant, lifecycle) {
		if run.RunID != runID || run.Namespace != namespace {
			continue
		}
		for _, step := range run.Steps {
			if step.StepID != stepID {
				continue
			}
			st, serr := time.Parse(time.RFC3339Nano, step.StartedAt)
			if step.StartedAt == "" || serr != nil {
				// A planned-but-not-started step has no start time to bound by; fall back.
				return time.Time{}, time.Time{}, false, nil
			}
			lower = st.Add(-e.logTimeMargin)
			if step.State == stepStateRunning || step.EndedAt == "" {
				upper = time.Now()
				return lower, upper, true, nil
			}
			et, eerr := time.Parse(time.RFC3339Nano, step.EndedAt)
			if eerr != nil {
				upper = time.Now()
				return lower, upper, true, nil
			}
			upper = et.Add(e.logTimeMargin)
			return lower, upper, true, nil
		}
	}
	return time.Time{}, time.Time{}, false, nil
}

// tenantAllowed reports whether tenant is in the configured allowlist.
func (e *E2E) tenantAllowed(tenant string) bool {
	for _, t := range e.tenants {
		if t == tenant {
			return true
		}
	}
	return false
}

// ErrRunNotFound is returned when an addressed run is absent from the window, so
// the handler can map it to HTTP 404.
var ErrRunNotFound = fmt.Errorf("run not found")

// ErrLogBackendUnavailable is the client-safe error returned when a Loki query
// fails. The raw Loki error carries the internal service DNS name, the LogQL
// query, and the tenant; surfacing it to the browser would breach the
// fleet-identity firewall (E2E-LOG-08), so the detailed error is logged
// server-side and only this generic sentinel crosses the BFF boundary.
var ErrLogBackendUnavailable = fmt.Errorf("e2e log backend unavailable")

// runID builds and parses the collision-free run address.
func makeRunID(tenant, namespace, runID string) string {
	return tenant + "/" + namespace + "/" + runID
}

// parseRunID splits a run address into tenant, namespace, and run id. The run id
// may itself contain slashes (it is the trailing component); tenant and
// namespace are the first two components.
func parseRunID(id string) (tenant, namespace, runID string, err error) {
	parts := strings.SplitN(id, "/", 3)
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return "", "", "", fmt.Errorf("invalid run id %q", id)
	}
	return parts[0], parts[1], parts[2], nil
}

// parseViaQEvent parses a Loki log LINE as a ViaQ record envelope, then parses
// the suite's structured event from its message field (E2E-LOG-07). It never
// assumes the Loki line is directly the event JSON. A line that is not a
// well-formed ViaQ record carrying a hypershell.e2e.v1 message yields ok=false
// so the caller skips it without dropping the rest of the stream.
func parseViaQEvent(line string) (e2eEvent, bool) {
	var envelope struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal([]byte(line), &envelope); err != nil || envelope.Message == "" {
		return e2eEvent{}, false
	}
	var ev e2eEvent
	if err := json.Unmarshal([]byte(envelope.Message), &ev); err != nil {
		return e2eEvent{}, false
	}
	if ev.Schema != schemaE2EV1 || ev.RunID == "" {
		return e2eEvent{}, false
	}
	return ev, true
}

// reduceRunsForTenant parses every entry's ViaQ envelope and message, groups the
// well-formed structured events by run id, and reduces each group into a run
// detail. Malformed records are isolated (E2E-LOG-07): they are skipped and the
// remaining records still reduce.
func reduceRunsForTenant(tenant string, entries []LokiEntry) []*E2ERunDetail {
	byRun := map[string][]e2eEvent{}
	for _, entry := range entries {
		ev, ok := parseViaQEvent(entry.Line)
		if !ok {
			continue
		}
		byRun[ev.RunID] = append(byRun[ev.RunID], ev)
	}
	out := make([]*E2ERunDetail, 0, len(byRun))
	for _, events := range byRun {
		out = append(out, reduceRun(tenant, events))
	}
	return out
}

// reduceRun reduces one run's structured events into a detail. Events are
// ordered and deduplicated by sequence so reconnect overlap and out-of-order
// delivery do not double-count or misorder (E2E-LOG-06). The trusted cluster is
// the tenant; the body cluster is kept only as display metadata.
func reduceRun(tenant string, events []e2eEvent) *E2ERunDetail {
	ordered := dedupeBySequence(events)

	detail := &E2ERunDetail{}
	detail.Tenant = tenant
	detail.Cluster = tenant
	detail.Verdict = verdictUnknown

	stepByID := map[string]*E2EStep{}
	var stepOrder []string
	ensureStep := func(id, name string) *E2EStep {
		s, ok := stepByID[id]
		if !ok {
			s = &E2EStep{StepID: id, Name: name, State: stepStateNotRun}
			stepByID[id] = s
			stepOrder = append(stepOrder, id)
		}
		if name != "" {
			s.Name = name
		}
		return s
	}

	sawStepEvent := false
	logs := make([]E2ELogLine, 0)
	logsTruncated := false

	for _, ev := range ordered {
		// Identity is carried on every record; take the first non-empty values.
		detail.RunID = firstNonEmpty(detail.RunID, ev.RunID)
		detail.Namespace = firstNonEmpty(detail.Namespace, ev.Namespace)
		detail.Revision = firstNonEmpty(detail.Revision, ev.Revision)
		detail.SourceRevision = firstNonEmpty(detail.SourceRevision, ev.SourceRevision)
		detail.BodyCluster = firstNonEmpty(detail.BodyCluster, ev.Cluster)

		switch ev.Event {
		case e2eEventRunStarted:
			detail.PlannedSteps = ev.PlannedSteps
			detail.StartedAt = firstNonEmpty(detail.StartedAt, ev.Timestamp)
			for _, p := range ev.PlannedSteps {
				ensureStep(p.StepID, p.Name)
			}
		case e2eEventStepStarted:
			sawStepEvent = true
			s := ensureStep(ev.StepID, ev.Name)
			s.State = stepStateRunning
			s.StartedAt = ev.Timestamp
			detail.StartedAt = firstNonEmpty(detail.StartedAt, ev.Timestamp)
		case e2eEventStepFinished:
			sawStepEvent = true
			s := ensureStep(ev.StepID, ev.Name)
			s.State = stepStateFromOutcome(ev.Outcome)
			s.EndedAt = ev.Timestamp
			s.DurationMs = ev.DurationMs
		case e2eEventRunFinished:
			detail.Verdict = verdictFromOutcome(ev.Outcome)
			detail.ReportingComplete = true
			detail.EndedAt = ev.Timestamp
			detail.DurationMs = ev.DurationMs
		case e2eEventLog:
			if len(logs) < maxDetailLogLines {
				logs = append(logs, E2ELogLine{
					Sequence:  ev.Sequence,
					Timestamp: ev.Timestamp,
					StepID:    ev.StepID,
					Message:   ev.Message,
					Truncated: ev.Truncated,
				})
			} else {
				logsTruncated = true
			}
		}
	}

	// Steps render in planned order first, then any additional steps seen in event
	// order (setup/cleanup, or steps not in the planned set).
	detail.Steps = orderSteps(detail.PlannedSteps, stepOrder, stepByID)
	detail.Logs = logs
	detail.LogsTruncated = logsTruncated
	detail.HasStructuredSteps = sawStepEvent
	detail.StepCount = len(detail.Steps)
	detail.ID = makeRunID(tenant, detail.Namespace, detail.RunID)
	return detail
}

// orderSteps lists planned steps in planned order, then any remaining seen steps
// in first-seen order, so a consumer gets a stable ordering.
func orderSteps(planned []E2EPlanned, seenOrder []string, byID map[string]*E2EStep) []E2EStep {
	out := make([]E2EStep, 0, len(byID))
	emitted := map[string]bool{}
	for _, p := range planned {
		if s, ok := byID[p.StepID]; ok {
			out = append(out, *s)
			emitted[p.StepID] = true
		} else {
			out = append(out, E2EStep{StepID: p.StepID, Name: p.Name, State: stepStateNotRun})
			emitted[p.StepID] = true
		}
	}
	for _, id := range seenOrder {
		if emitted[id] {
			continue
		}
		out = append(out, *byID[id])
		emitted[id] = true
	}
	return out
}

// dedupeBySequence sorts events by sequence and drops duplicates, so overlap
// from a reconnect backfill is idempotent (E2E-LOG-06).
func dedupeBySequence(events []e2eEvent) []e2eEvent {
	sorted := append([]e2eEvent(nil), events...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Sequence < sorted[j].Sequence })
	out := make([]e2eEvent, 0, len(sorted))
	seen := map[int64]bool{}
	for _, ev := range sorted {
		if seen[ev.Sequence] {
			continue
		}
		seen[ev.Sequence] = true
		out = append(out, ev)
	}
	return out
}

func stepStateFromOutcome(outcome string) string {
	switch outcome {
	case verdictPassed:
		return stepStatePassed
	case verdictFailed:
		return stepStateFailed
	case "skipped":
		return stepStateSkipped
	default:
		return stepStateFailed
	}
}

func verdictFromOutcome(outcome string) string {
	if outcome == verdictPassed {
		return verdictPassed
	}
	return verdictFailed
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// Lifecycle events have their own bounded query so chatty logs cannot evict a
// run's start, plan, or verdict from the recent history or detail baseline.
func lifecycleQuery(selector string) string {
	return selector + ` |~ "run_started|step_started|step_finished|run_finished"`
}
func (e *E2E) runEntries(ctx context.Context, tenant, runID string, start, end time.Time) ([]LokiEntry, bool, error) {
	selector := e.selector + " |= " + strconv.Quote(runID)
	lifecycle, structuralLimit, err := e.loki.QueryRange(ctx, tenant, lifecycleQuery(selector), start, end, lokiQueryLimit)
	if err != nil {
		return nil, false, err
	}
	logs, logLimit, err := e.loki.QueryRange(ctx, tenant, selector, start, end, lokiQueryLimit)
	if err != nil {
		return nil, false, err
	}
	return append(lifecycle, logs...), structuralLimit || logLimit, nil
}
