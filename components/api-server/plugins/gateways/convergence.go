package gateways

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/openshift-online/hypershell/components/api-server/pkg/gatewayhealth"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/db"
)

// Change-convergence tracking for SLO 2 (service-level-indicators.spec.md,
// OBS-SLI-01 through OBS-SLI-04). The API server is the durable source of truth
// for change convergence (OBS-SLI-01): every observable desired-state change of
// a Gateway opens a convergence window in PostgreSQL, and that window resolves to
// exactly one outcome, counted exactly once, on the status-write path or at its
// ten-minute deadline (OBS-SLI-02). The durable totals back
// hypershell_change_convergence_total (OBS-SLI-03) and the provision metrics
// (OBS-SLI-04); nothing is derived from control-plane process memory.

// convergenceBudget is the ten-minute window every change is judged against
// (OBS-SLI-02). A change that converges within the budget of its clock start
// counts within_10m; one that has not converged when the budget elapses counts
// late or failed at the deadline.
const convergenceBudget = 10 * time.Minute

// Change types (OBS-SLI-01 / OBS-SLI-03). The counter carries only these three.
const (
	changeTypeCreate = "create"
	changeTypeUpdate = "update"
	changeTypeDelete = "delete"
)

// Outcomes (OBS-SLI-02 / OBS-SLI-03). The counter carries only these three.
const (
	outcomeWithin10m = "within_10m"
	outcomeLate      = "late"
	outcomeFailed    = "failed"
)

// Provision outcomes (OBS-SLI-04). One terminal outcome per gateway.
const (
	provisionOutcomeSuccess = "success"
	provisionOutcomeFailure = "failure"
)

// Transition kinds recorded in gateway_status_transitions (OBS-SLI-01).
const (
	transitionTypePhase     = "phase"
	transitionTypeCondition = "condition"
)

// changeTypes and outcomes enumerate the bounded label domains so the counter is
// emitted gapless (every series present, even at zero) and never exceeds
// |change_type| x |outcome| = 9 series (OBS-SLI-03).
var (
	changeTypes = []string{changeTypeCreate, changeTypeUpdate, changeTypeDelete}
	outcomes    = []string{outcomeWithin10m, outcomeLate, outcomeFailed}
)

// convergenceStore owns the durable convergence state. It shares the Gateway
// session factory so its writes commit in the same request transaction as the
// Gateway create/replace/delete that triggered them (atomic with the row write),
// and it shares the advisory-lock factory so the deadline sweep can run on one
// replica only (OBS-SLI-02).
type convergenceStore struct {
	sessionFactory *db.SessionFactory
	lockFactory    db.LockFactory
	now            func() time.Time
}

func newConvergenceStore(sessionFactory *db.SessionFactory, lockFactory db.LockFactory) *convergenceStore {
	return &convergenceStore{
		sessionFactory: sessionFactory,
		lockFactory:    lockFactory,
		now:            time.Now,
	}
}

func (s *convergenceStore) g(ctx context.Context) *gorm.DB {
	return (*s.sessionFactory).New(ctx)
}

// openWindow is the open convergence window for one gateway.
type openWindow struct {
	ChangeType     string
	ClockStart     time.Time
	OpenGeneration int64
}

// onCreate opens a create convergence window the instant a Gateway is first
// persisted, with its clock starting at the create time (OBS-SLI-01.01). It is
// idempotent: a replayed create never reopens or re-clocks an existing window.
func (s *convergenceStore) onCreate(ctx context.Context, gatewayID string, createdAt time.Time) error {
	now := s.now()
	if createdAt.IsZero() {
		createdAt = now
	}
	return s.g(ctx).Exec(`
		INSERT INTO gateway_convergence
			(gateway_id, gateway_created_at, open_change_type, open_generation, clock_start, last_phase, created_at, updated_at)
		VALUES (?, ?, ?, 1, ?, '', ?, ?)
		ON CONFLICT (gateway_id) DO NOTHING`,
		gatewayID, createdAt, changeTypeCreate, createdAt, now, now).Error
}

// onStatusWrite reconciles the durable convergence state with a persisted
// Gateway status write (the control plane's phase / observed_generation /
// provisioning-condition update, OBS-SLI-01). It records transition timestamps,
// advances or opens the convergence window when the desired generation changed,
// records the provision metrics, and resolves the window within_10m when the
// newest generation has converged inside its budget. It runs in the caller's
// request transaction, under the per-gateway advisory lock Replace already holds.
func (s *convergenceStore) onStatusWrite(ctx context.Context, current, next *Gateway) error {
	now := s.now()

	// A desired-spec change advances generation. Open an update window when the
	// gateway was converged, or keep the first clock start of an already-open run
	// so repeated edits cannot reset the clock (OBS-SLI-01.02 / OBS-SLI-02.05).
	if next.Generation > current.Generation {
		if err := s.openOrAdvanceWindow(ctx, next.ID, next.GatewayCreatedAtOr(current), changeTypeUpdate, next.Generation, now); err != nil {
			return err
		}
	}

	if err := s.recordTransitions(ctx, current, next, now); err != nil {
		return err
	}

	if err := s.recordProvision(ctx, current, next, now); err != nil {
		return err
	}

	// Converged means observed_generation == generation AND phase == Running for
	// the newest generation (OBS-SLI-01). Resolve within_10m only inside the
	// budget; a convergence at or past the deadline is left for the sweep, which
	// has already (or will) count it late (OBS-SLI-02.02 / 02.04).
	if isConverged(next) {
		if _, err := s.resolveWithinBudget(ctx, next.ID, now); err != nil {
			return err
		}
	}
	return nil
}

// onDeleteRequested opens a delete convergence window when a Gateway deletion is
// requested (OBS-SLI-01). The delete converges only after finalization completes
// (onDeleteFinalized); until then the window is subject to the deadline sweep.
func (s *convergenceStore) onDeleteRequested(ctx context.Context, gateway *Gateway) error {
	now := s.now()
	createdAt := gateway.CreatedAt
	if createdAt.IsZero() {
		createdAt = now
	}
	return s.g(ctx).Exec(`
		INSERT INTO gateway_convergence
			(gateway_id, gateway_created_at, open_change_type, open_generation, clock_start, last_phase, deletion_requested_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, '', ?, ?, ?)
		ON CONFLICT (gateway_id) DO UPDATE SET
			open_change_type = ?,
			open_generation = EXCLUDED.open_generation,
			clock_start = ?,
			deletion_requested_at = ?,
			resolved_outcome = NULL,
			resolved_at = NULL,
			updated_at = EXCLUDED.updated_at`,
		gateway.ID, createdAt, changeTypeDelete, gateway.Generation, now, now, now, now,
		changeTypeDelete, now, now).Error
}

// onDeleteFinalized resolves the delete window when the control plane reports
// finalization complete (every Gateway-owned resource reclaimed, GTW-DEL-01).
// It counts within_10m inside the budget; a finalization at or past the deadline
// was already counted by the sweep (OBS-SLI-01.03 / OBS-SLI-02.04).
func (s *convergenceStore) onDeleteFinalized(ctx context.Context, gatewayID string) error {
	now := s.now()
	_, err := s.resolveWithinBudget(ctx, gatewayID, now)
	return err
}

// openOrAdvanceWindow opens a new convergence window for a gateway that was
// converged, or, when a window is already open, advances only the judged
// generation while preserving the original change type and clock start, so an
// unbroken unconverged run keeps its first clock (OBS-SLI-02.05).
func (s *convergenceStore) openOrAdvanceWindow(ctx context.Context, gatewayID string, createdAt time.Time, changeType string, generation int64, now time.Time) error {
	return s.g(ctx).Exec(`
		INSERT INTO gateway_convergence
			(gateway_id, gateway_created_at, open_change_type, open_generation, clock_start, last_phase, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, '', ?, ?)
		ON CONFLICT (gateway_id) DO UPDATE SET
			open_change_type = CASE WHEN gateway_convergence.open_change_type IS NULL
				THEN EXCLUDED.open_change_type ELSE gateway_convergence.open_change_type END,
			clock_start = CASE WHEN gateway_convergence.open_change_type IS NULL
				THEN EXCLUDED.clock_start ELSE gateway_convergence.clock_start END,
			open_generation = EXCLUDED.open_generation,
			resolved_outcome = CASE WHEN gateway_convergence.open_change_type IS NULL
				THEN NULL ELSE gateway_convergence.resolved_outcome END,
			resolved_at = CASE WHEN gateway_convergence.open_change_type IS NULL
				THEN NULL ELSE gateway_convergence.resolved_at END,
			updated_at = EXCLUDED.updated_at`,
		gatewayID, createdAt, changeType, generation, now, now, now).Error
}

// resolveWithinBudget resolves an open window as within_10m when the newest
// generation has converged inside the ten-minute budget. It locks the row FOR
// UPDATE and increments the durable counter in the same transaction, so the
// resolution and the counter increment are atomic and exactly-once even against
// the concurrent deadline sweep (OBS-SLI-02 / OBS-SLI-02.06). It returns false
// (and does nothing) when no window is open or the budget has already elapsed.
func (s *convergenceStore) resolveWithinBudget(ctx context.Context, gatewayID string, now time.Time) (bool, error) {
	g := s.g(ctx)
	var win openWindow
	res := g.Raw(`
		SELECT open_change_type AS change_type, clock_start, open_generation
		FROM gateway_convergence
		WHERE gateway_id = ? AND open_change_type IS NOT NULL
		FOR UPDATE`, gatewayID).Scan(&win)
	if res.Error != nil {
		return false, res.Error
	}
	if res.RowsAffected == 0 {
		return false, nil
	}
	if now.Sub(win.ClockStart) > convergenceBudget {
		// Past the deadline: the sweep owns this outcome, never within_10m.
		return false, nil
	}
	return true, s.resolveLocked(ctx, gatewayID, win.ChangeType, outcomeWithin10m, now)
}

// resolveLocked closes the window and increments the durable counter for the
// resolved (change_type, outcome). The caller must already hold the row lock.
func (s *convergenceStore) resolveLocked(ctx context.Context, gatewayID, changeType, outcome string, now time.Time) error {
	g := s.g(ctx)
	if err := g.Exec(`
		UPDATE gateway_convergence
		SET open_change_type = NULL, resolved_outcome = ?, resolved_at = ?, updated_at = ?
		WHERE gateway_id = ?`, outcome, now, now, gatewayID).Error; err != nil {
		return err
	}
	return s.incrementCounter(ctx, changeType, outcome)
}

// incrementCounter bumps the durable monotonic total for one (change_type,
// outcome) series backing hypershell_change_convergence_total (OBS-SLI-03).
func (s *convergenceStore) incrementCounter(ctx context.Context, changeType, outcome string) error {
	return s.g(ctx).Exec(`
		INSERT INTO gateway_convergence_counters (change_type, outcome, total)
		VALUES (?, ?, 1)
		ON CONFLICT (change_type, outcome) DO UPDATE SET total = gateway_convergence_counters.total + 1`,
		changeType, outcome).Error
}

// recordTransitions appends a durable timestamp for a phase change and for every
// provisioning-condition change, using the authoritative write time, so the
// convergence window and provision duration are computable from persisted data
// (OBS-SLI-01). It also keeps gateway_convergence.last_phase current.
func (s *convergenceStore) recordTransitions(ctx context.Context, current, next *Gateway, now time.Time) error {
	g := s.g(ctx)

	currentPhase := derefString(current.Phase)
	nextPhase := derefString(next.Phase)
	if nextPhase != "" && nextPhase != currentPhase {
		if err := g.Exec(`
			INSERT INTO gateway_status_transitions
				(gateway_id, generation, transition_type, name, value, transitioned_at)
			VALUES (?, ?, ?, ?, ?, ?)`,
			next.ID, next.Generation, transitionTypePhase, transitionTypePhase, nextPhase, now).Error; err != nil {
			return err
		}
		if err := g.Exec(`UPDATE gateway_convergence SET last_phase = ?, updated_at = ? WHERE gateway_id = ?`,
			nextPhase, now, next.ID).Error; err != nil {
			return err
		}
	}

	for _, change := range changedConditions(current.ProvisioningConditions, next.ProvisioningConditions) {
		if err := g.Exec(`
			INSERT INTO gateway_status_transitions
				(gateway_id, generation, transition_type, name, value, transitioned_at)
			VALUES (?, ?, ?, ?, ?, ?)`,
			next.ID, next.Generation, transitionTypeCondition, change.Type, change.ConditionStatus, now).Error; err != nil {
			return err
		}
	}
	return nil
}

// recordProvision records the provision metrics from the persisted transitions
// (OBS-SLI-04): one create-to-Running duration observation on the first Running,
// and one terminal outcome per gateway (success on the first Running, failure on
// the first Failed). COALESCE keeps the first write, so a Degraded-to-Running
// recovery adds neither another observation nor another outcome.
func (s *convergenceStore) recordProvision(ctx context.Context, current, next *Gateway, now time.Time) error {
	currentPhase := derefString(current.Phase)
	nextPhase := derefString(next.Phase)
	if nextPhase == currentPhase {
		return nil
	}
	g := s.g(ctx)
	switch nextPhase {
	case string(gatewayhealth.PhaseRunning):
		createdAt := next.GatewayCreatedAtOr(current)
		duration := now.Sub(createdAt).Seconds()
		if duration < 0 {
			duration = 0
		}
		return g.Exec(`
			INSERT INTO gateway_provisions (gateway_id, duration_seconds, outcome, recorded_at)
			VALUES (?, ?, ?, ?)
			ON CONFLICT (gateway_id) DO UPDATE SET
				duration_seconds = COALESCE(gateway_provisions.duration_seconds, EXCLUDED.duration_seconds),
				outcome = COALESCE(gateway_provisions.outcome, EXCLUDED.outcome)`,
			next.ID, duration, provisionOutcomeSuccess, now).Error
	case string(gatewayhealth.PhaseFailed):
		return g.Exec(`
			INSERT INTO gateway_provisions (gateway_id, outcome, recorded_at)
			VALUES (?, ?, ?)
			ON CONFLICT (gateway_id) DO UPDATE SET
				outcome = COALESCE(gateway_provisions.outcome, EXCLUDED.outcome)`,
			next.ID, provisionOutcomeFailure, now).Error
	default:
		return nil
	}
}

// sweep resolves every open window whose ten-minute deadline has elapsed without
// convergence, counting it bad at the deadline without waiting for eventual
// convergence (OBS-SLI-02.02 / 02.03). It is late when the newest generation is
// not in a terminal-failure state and failed when it is (phase == Failed). Each
// resolution and its counter increment are atomic in one transaction, and the
// rows are locked FOR UPDATE SKIP LOCKED, so the sweep never double-counts a
// window the status-write path is resolving concurrently (OBS-SLI-02.06).
// Callers serialize the sweep across replicas with the advisory lock
// (runDeadlineSweep); this method performs one pass.
func (s *convergenceStore) sweep(ctx context.Context, now time.Time) (int, error) {
	deadline := now.Add(-convergenceBudget)
	g := s.g(ctx)

	type expiredWindow struct {
		GatewayID  string
		ChangeType string
		Phase      string
	}
	var expired []expiredWindow
	if err := g.Raw(`
		SELECT c.gateway_id AS gateway_id, c.open_change_type AS change_type, COALESCE(g.phase, '') AS phase
		FROM gateway_convergence c
		LEFT JOIN gateways g ON g.id = c.gateway_id
		WHERE c.open_change_type IS NOT NULL AND c.clock_start <= ?
		FOR UPDATE OF c SKIP LOCKED`, deadline).Scan(&expired).Error; err != nil {
		return 0, err
	}

	resolved := 0
	for _, w := range expired {
		outcome := outcomeLate
		if w.Phase == string(gatewayhealth.PhaseFailed) {
			outcome = outcomeFailed
		}
		if err := s.resolveLocked(ctx, w.GatewayID, w.ChangeType, outcome, now); err != nil {
			return resolved, err
		}
		resolved++
	}
	return resolved, nil
}

// isConverged reports whether the newest generation has converged: the control
// plane has observed the current generation and the gateway is Running
// (OBS-SLI-01). Degraded, Failed, or observed_generation < generation is not
// converged.
func isConverged(g *Gateway) bool {
	return g.ObservedGeneration == g.Generation && derefString(g.Phase) == string(gatewayhealth.PhaseRunning)
}

// changedConditions returns the provisioning conditions whose status differs
// between the persisted and incoming documents (a new or changed condition),
// used to stamp a condition-transition timestamp (OBS-SLI-01).
func changedConditions(current, next *string) []provisioningConditionJSON {
	nextConds, ok := parseProvisioningConditions(next)
	if !ok {
		return nil
	}
	currentByType := map[string]string{}
	if currentConds, ok := parseProvisioningConditions(current); ok {
		for _, c := range currentConds {
			currentByType[c.Type] = c.ConditionStatus
		}
	}
	var changed []provisioningConditionJSON
	for _, c := range nextConds {
		if prev, found := currentByType[c.Type]; !found || prev != c.ConditionStatus {
			changed = append(changed, c)
		}
	}
	return changed
}

// GatewayCreatedAtOr returns this gateway's create time, falling back to the
// supplied gateway's create time when this one is zero (a partial status write
// may not carry the original Meta). It never returns a zero time when either is
// set.
func (g *Gateway) GatewayCreatedAtOr(fallback *Gateway) time.Time {
	if !g.CreatedAt.IsZero() {
		return g.CreatedAt
	}
	if fallback != nil && !fallback.CreatedAt.IsZero() {
		return fallback.CreatedAt
	}
	return g.CreatedAt
}

// convergenceCountKey identifies one durable counter series.
type convergenceCountKey struct {
	ChangeType string
	Outcome    string
}

// convergenceCounts returns the durable per-series totals backing
// hypershell_change_convergence_total (OBS-SLI-03). Missing series are absent
// from the map; the collector fills them with zero.
func (s *convergenceStore) convergenceCounts(ctx context.Context) (map[convergenceCountKey]int64, error) {
	type row struct {
		ChangeType string
		Outcome    string
		Total      int64
	}
	var rows []row
	if err := s.g(ctx).Raw(`SELECT change_type, outcome, total FROM gateway_convergence_counters`).Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[convergenceCountKey]int64, len(rows))
	for _, r := range rows {
		out[convergenceCountKey{ChangeType: r.ChangeType, Outcome: r.Outcome}] = r.Total
	}
	return out, nil
}

// provisionDurationSummary is the durable histogram state for
// gateway_provision_duration_seconds (OBS-SLI-04), reconstructed per scrape from
// the one-per-gateway persisted observations.
type provisionDurationSummary struct {
	Count   uint64
	Sum     float64
	Buckets map[float64]uint64
}

// provisionDurationBuckets are the explicit boundaries covering 1 second through
// 15 minutes (OBS-SLI-04). They match the series the operational dashboard
// already consumes so the provision-time contract is unchanged when the producer
// moves from the control plane to the API server.
var provisionDurationBuckets = []float64{1, 5, 10, 15, 30, 45, 60, 90, 120, 180, 300, 600, 900}

// provisionDurations reconstructs the durable histogram from the persisted
// one-per-gateway create-to-Running durations (OBS-SLI-04). Cumulative bucket
// counts and the overall count and sum are computed in SQL so the exported
// histogram is monotonic across restarts and never derived from process memory.
func (s *convergenceStore) provisionDurations(ctx context.Context) (provisionDurationSummary, error) {
	summary := provisionDurationSummary{Buckets: make(map[float64]uint64, len(provisionDurationBuckets))}

	var agg struct {
		Count int64
		Sum   *float64
	}
	if err := s.g(ctx).Raw(`
		SELECT COUNT(*) AS count, SUM(duration_seconds) AS sum
		FROM gateway_provisions WHERE duration_seconds IS NOT NULL`).Scan(&agg).Error; err != nil {
		return summary, err
	}
	summary.Count = uint64(agg.Count)
	if agg.Sum != nil {
		summary.Sum = *agg.Sum
	}

	for _, le := range provisionDurationBuckets {
		var c int64
		if err := s.g(ctx).Raw(`
			SELECT COUNT(*) FROM gateway_provisions
			WHERE duration_seconds IS NOT NULL AND duration_seconds <= ?`, le).Scan(&c).Error; err != nil {
			return summary, err
		}
		summary.Buckets[le] = uint64(c)
	}
	return summary, nil
}

// provisionOutcomeCounts returns the durable one-per-gateway terminal provision
// outcome totals backing gateway_provision_outcomes_total (OBS-SLI-04).
func (s *convergenceStore) provisionOutcomeCounts(ctx context.Context) (map[string]int64, error) {
	type row struct {
		Outcome string
		Count   int64
	}
	var rows []row
	if err := s.g(ctx).Raw(`
		SELECT outcome, COUNT(*) AS count FROM gateway_provisions
		WHERE outcome IS NOT NULL GROUP BY outcome`).Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[string]int64, len(rows))
	for _, r := range rows {
		out[r.Outcome] = r.Count
	}
	return out, nil
}

// convergenceSweepLockType / convergenceSweepLockID name the single advisory lock
// that serializes the deadline sweep across API-server replicas, so the sweep
// runs on one replica at a time (OBS-SLI-02).
const convergenceSweepLockType db.LockType = "convergence-sweep"
const convergenceSweepLockID string = "convergence-sweep"

// runDeadlineSweep runs one deadline-sweep pass under a non-blocking advisory
// lock in its own transaction, so exactly one replica resolves expired windows
// per tick and the resolution commits with the counter increment. A replica that
// cannot take the lock skips the tick (another replica owns it). It returns the
// number of windows resolved.
func (s *convergenceStore) runDeadlineSweep(baseCtx context.Context) (int, error) {
	ctx, err := db.NewContext(baseCtx, *s.sessionFactory)
	if err != nil {
		return 0, fmt.Errorf("open sweep transaction: %w", err)
	}
	defer db.Resolve(ctx)

	owner, acquired, err := s.lockFactory.NewNonBlockingLock(ctx, convergenceSweepLockID, convergenceSweepLockType)
	if err != nil {
		db.MarkForRollback(ctx, err)
		return 0, fmt.Errorf("acquire sweep lock: %w", err)
	}
	if !acquired {
		return 0, nil
	}
	defer s.lockFactory.Unlock(ctx, owner)

	resolved, err := s.sweep(ctx, s.now())
	if err != nil {
		db.MarkForRollback(ctx, err)
		return 0, err
	}
	return resolved, nil
}
