package sources

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/openshift-online/hypershell/components/fleet-dashboard/pkg/cache"
	"github.com/openshift-online/hypershell/components/fleet-dashboard/pkg/config"
)

// The Agent SDLC visibility plane (agent-sdlc-visibility-dashboard.spec) makes the
// label-driven agent funnel observable: an aging work-in-progress board, a rework
// meter per in-review card, an escalation lane, and a release-by-hub-cluster
// matrix. This file holds the framework-free domain: the provider port the plane
// depends on (SDLC-VIS-02), the serialized payload, and the derivations that turn
// provider records into board cards, lane cards, and the matrix. The GitHub-label
// adapter that implements the port lives in sdlc_github.go.

// SDLC phase keys in funnel order. The board columns are the timed phases
// (config.SDLCPhaseKeys); Shipped is terminal and rendered as the matrix, not a
// timed column.
const (
	PhaseIntake       = "intake"
	PhaseWorkable     = "workable"
	PhaseSpec         = "spec"
	PhaseSpecApproved = "spec-approved"
	PhaseCode         = "code"
	PhaseCodeApproved = "code-approved"
	PhaseShipped      = "shipped"
)

// Escalation reasons. A card leaves the board for the lane when it is paused
// (blocked/needs-input/duplicate) or when its time-in-phase crosses its column SLE
// (SDLC-VIS-10).
const (
	EscalationBlocked    = "blocked"
	EscalationNeedsInput = "needs-input"
	EscalationDuplicate  = "duplicate"
	EscalationPastSLE    = "past-sle"
)

// Funnel label names (the GitHub label state machine - .hypershell/README.md,
// specs/standards/sdlc/jira-github-sync.spec.md). The GitHub-label adapter reads
// these; a future Jira adapter maps Jira status/labels to the same domain phases
// behind the same port, with no change here (SDLC-VIS-02).
const (
	labelAllowed         = "agent/allowed"
	labelWorkable        = "agent/workable"
	labelReviewableSpec  = "agent/reviewable-spec"
	labelSpecRejected    = "agent/review-spec-rejected"
	labelSpecApproved    = "agent/review-spec-approved"
	labelReviewableCode  = "agent/reviewable-code"
	labelCodeRejected    = "agent/review-code-rejected"
	labelCodeApproved    = "agent/review-code-approved"
	labelReleaseVerified = "agent/release-verified"
	labelBlocked         = "agent/blocked"
	labelNeedsInput      = "agent/needs-input"
	labelDuplicate       = "agent/duplicate"
)

// Rework block thresholds (jira-github-sync.spec / .hypershell): spec review blocks
// at 3 rejections, code review at 10. The rework meter encodes progress toward
// these (SDLC-VIS-09).
const (
	specReworkThreshold = 3
	codeReworkThreshold = 10
)

// SdlcLabelEvent is one label transition in an execution record's reconstructed
// history ("state over time", SDLC-VIS-14). At is RFC3339 (UTC/Z), so lexical
// comparison is chronological.
type SdlcLabelEvent struct {
	Label  string `json:"label"`
	Action string `json:"action"` // "labeled" | "unlabeled"
	At     string `json:"at"`     // RFC3339
}

// SdlcRecord is one in-scope execution record's funnel state, expressed by the
// SdlcStateProvider port in domain terms independent of the source system. It
// carries the current phase, the per-record label-event history, and the rework
// count (SDLC-VIS-02). Bundle membership is deliberately NOT here: it is the
// separate shared GitHub bundle concern (SDLC-VIS-03).
type SdlcRecord struct {
	Number      int
	Title       string
	URL         string
	Closed      bool
	ClosedAt    string // RFC3339 when Closed (used as the "merged" timestamp)
	ClosedBy    string // GitHub login that closed/merged the record, if known
	Phase       string // current funnel phase key
	Paused      string // "" or blocked/needs-input/duplicate (label-derived pause)
	PhaseSince  string // RFC3339 of the latest labeled event for the current phase
	ReworkCount int    // count of labeled events for the current review phase's reject label
	Scope       string // the agent/scope/* selector label, if any (for filtering)
	History     []SdlcLabelEvent
}

// SdlcStateProvider is the application-owned port the SDLC plane depends on for
// funnel state. It expresses, in domain terms independent of any source system,
// the in-scope execution records with their current phase, per-record label-event
// history, and rework count (SDLC-VIS-02). A GitHub-label adapter is the only
// implementation today; a Jira adapter is a drop-in behind this same interface
// with no domain or UI change (jira-github-sync.spec). Issue-to-bundle membership
// is intentionally absent: that stays the shared GitHub bundle concern of
// SDLC-VIS-03, whichever state adapter is active.
type SdlcStateProvider interface {
	Records(ctx context.Context) ([]SdlcRecord, error)
}

// WorkItemPRResolver supplies the grouping hop SDLC-VIS-03 uses on top of the
// shared bundle->PR enrichment: a unit of work (today a GitHub issue; tomorrow a
// Jira item, per jira-github-sync.spec) to the pull requests associated with it.
// It is deliberately shaped work-item -> PRs, not PR -> issue: a single work item
// can be delivered by several PRs, and the authoritative association lives on the
// work item (GitHub's linked pull requests / closing references), not scattered
// across PR bodies. The matrix INVERTS these results into a PR -> work-item map and
// uses it only to GROUP each bundle's own pull requests under the tracked issue they
// resolve to; it does NOT attribute issues to bundles (bundle contents are driven by
// the bundle's PR set). nil disables grouping; every bundle PR then lists as a
// one-off pull request, and shipped records with no bundle PR collect in Unreleased.
//
// This is the Jira seam: a Jira-backed adapter satisfies the same interface by
// returning the PRs linked to a Jira item, with no domain or matrix change.
type WorkItemPRResolver interface {
	// AssociatedPRs returns the numbers of the pull requests associated with a work
	// item (its linked / closing PRs), or nil when none are found or the lookup is
	// unavailable this cycle. Order is not significant.
	AssociatedPRs(ctx context.Context, record SdlcRecord) []int
}

// promotionReader returns the latest cached promotion snapshot so the SDLC matrix
// reuses the SAME bundle and hub-cluster truth the promotion plane already resolved
// (SDLC-VIS-03), rather than re-deriving bundles or re-calling the compare API.
// *cache.Source satisfies this.
type promotionReader interface {
	Get() cache.Snapshot
}

// SDLCPayload is the /api/sdlc data plane. Cards are the aging-board cards (not
// escalated), Lane the escalated cards in action-queue order, and Matrix the
// release-by-hub-cluster matrix (the Shipped column).
type SDLCPayload struct {
	Phases []SDLCPhase `json:"phases"`
	Cards  []SDLCCard  `json:"cards"`
	Lane   []SDLCCard  `json:"lane"`
	Matrix SDLCMatrix  `json:"matrix"`
}

// SDLCPhase is one funnel column. SLESeconds is the per-phase service-level
// expectation (0 for Shipped, which has no timed column). The UI draws the dashed
// SLE line at this value (SDLC-VIS-08) and positions cards by time-in-phase.
type SDLCPhase struct {
	Key        string `json:"key"`
	SLESeconds int    `json:"sleSeconds"`
}

// SDLCRework encodes a card's progress toward its phase block threshold. One
// proportional encoding for all thresholds: the UI fills Count/Threshold whether
// the limit is 3 or 10 (SDLC-VIS-09).
type SDLCRework struct {
	Count     int `json:"count"`
	Threshold int `json:"threshold"`
}

// SDLCCard is one execution record on the board or in the lane.
type SDLCCard struct {
	Number             int              `json:"number"`
	Title              string           `json:"title"`
	URL                string           `json:"url"`
	Phase              string           `json:"phase"`
	TimeInPhaseSeconds int              `json:"timeInPhaseSeconds"`
	Scope              string           `json:"scope,omitempty"`
	Escalation         string           `json:"escalation,omitempty"`
	Rework             *SDLCRework      `json:"rework,omitempty"`
	History            []SdlcLabelEvent `json:"history"`
}

// SDLCHubCluster is one matrix column: a hub cluster and the bundle it currently
// runs, so deployment and drift read at a glance (SDLC-VIS-11).
type SDLCHubCluster struct {
	Key            string `json:"key"`
	Cluster        string `json:"cluster,omitempty"`
	Env            string `json:"env,omitempty"`
	CurrentVersion string `json:"currentVersion,omitempty"`
	CurrentDigest  string `json:"currentDigest,omitempty"`
}

// SDLCPullRequest is one pull request shipped in a bundle. Bundle contents are
// PR-driven (SDLC-VIS-03): these mirror the shared bundles.PR the Fleet Map freight
// bar shows, so a pull request is never hidden because its issue link is
// unrecoverable.
type SDLCPullRequest struct {
	Number   int    `json:"number"`
	Title    string `json:"title"`
	URL      string `json:"url"`
	Author   string `json:"author,omitempty"`
	MergedAt string `json:"mergedAt,omitempty"`
}

// SDLCShippedIssue is one tracked execution record: an issue that shipped in a bundle
// (with the bundle's pull requests that resolve to it grouped beneath, PRs), or a
// shipped issue awaiting a bundle in the Unreleased row (PRs empty). A single issue
// MAY be delivered by several pull requests in one bundle; it appears once with those
// pull requests grouped under it (SDLC-VIS-03).
type SDLCShippedIssue struct {
	Number   int               `json:"number"`
	Title    string            `json:"title"`
	URL      string            `json:"url"`
	MergedAt string            `json:"mergedAt,omitempty"`
	MergedBy string            `json:"mergedBy,omitempty"`
	PRs      []SDLCPullRequest `json:"prs,omitempty"`
}

// SDLCBundleRow is one matrix row: a release bundle (newest first) or the single
// top Unreleased row, which carries no version and no hub-cluster markers
// (SDLC-VIS-11). HubClusters lists the instance keys currently running this bundle.
// A bundle row's contents are PR-driven and split into two groups: Issues holds the
// bundle's pull requests grouped under the tracked execution record they resolve to,
// and OtherPRs holds the one-off pull requests with no tracked issue. The Unreleased
// row carries only tracked issues (shipped records whose pull requests are absent
// from every bundle) and no OtherPRs.
type SDLCBundleRow struct {
	Version     string             `json:"version,omitempty"`
	Digest      string             `json:"digest,omitempty"`
	Date        string             `json:"date,omitempty"`
	Unreleased  bool               `json:"unreleased"`
	HubClusters []string           `json:"hubClusters,omitempty"`
	Issues      []SDLCShippedIssue `json:"issues"`
	OtherPRs    []SDLCPullRequest  `json:"otherPRs"`
}

// SDLCMatrix is the release-by-hub-cluster matrix (the Shipped column).
type SDLCMatrix struct {
	HubClusters []SDLCHubCluster `json:"hubClusters"`
	Rows        []SDLCBundleRow  `json:"rows"`
}

// SDLC is the fetch source for /api/sdlc. It reads funnel state through the
// provider port, reuses the cached promotion bundle truth for the matrix, and adds
// the work-item -> PRs hop for issue-to-bundle membership.
type SDLC struct {
	provider SdlcStateProvider
	promo    promotionReader
	issues   WorkItemPRResolver
	sle      map[string]time.Duration
	now      func() time.Time
	logger   *slog.Logger
}

// NewSDLC builds the SDLC source. provider supplies funnel state; promo is the
// promotion cache.Source whose snapshot the matrix reuses (SDLC-VIS-03); issues is
// the optional work-item -> PRs resolver (nil disables matrix issue mapping).
func NewSDLC(c *config.Config, provider SdlcStateProvider, promo promotionReader, issues WorkItemPRResolver) *SDLC {
	return &SDLC{
		provider: provider,
		promo:    promo,
		issues:   issues,
		sle:      c.SDLCPhaseSLE,
		now:      time.Now,
		logger:   slog.Default(),
	}
}

// SDLC is the FetchFunc for the /api/sdlc plane. A provider failure to load the
// in-scope record set returns an error so the cache serves last-good flagged stale
// (or 503 before the first success), per SDLC-VIS-01. Per-record timeline gaps
// degrade that record gracefully inside the adapter.
func (s *SDLC) SDLC(ctx context.Context) (any, error) {
	records, err := s.provider.Records(ctx)
	if err != nil {
		return nil, fmt.Errorf("sdlc: load records: %w", err)
	}
	now := s.now()

	payload := SDLCPayload{
		Phases: s.phases(),
		Cards:  []SDLCCard{},
		Lane:   []SDLCCard{},
	}

	var shipped []SdlcRecord
	for _, rec := range records {
		esc := s.escalation(rec, now)
		if esc == "" && rec.Phase == PhaseShipped {
			shipped = append(shipped, rec)
			continue
		}
		card := s.card(rec, now, esc)
		if esc != "" {
			payload.Lane = append(payload.Lane, card)
		} else {
			payload.Cards = append(payload.Cards, card)
		}
	}
	sortLane(payload.Lane)
	payload.Matrix = s.matrix(ctx, shipped)
	return payload, nil
}

// phases lists the board columns (timed, with their SLE) followed by the terminal
// Shipped column (no SLE).
func (s *SDLC) phases() []SDLCPhase {
	out := make([]SDLCPhase, 0, len(config.SDLCPhaseKeys)+1)
	for _, k := range config.SDLCPhaseKeys {
		out = append(out, SDLCPhase{Key: k, SLESeconds: int(s.sle[k].Seconds())})
	}
	out = append(out, SDLCPhase{Key: PhaseShipped})
	return out
}

// escalation classifies a record into a lane bucket, or "" when it stays on the
// board. A paused label always escalates; otherwise a time-in-phase beyond the
// column SLE escalates as past-SLE (SDLC-VIS-10).
func (s *SDLC) escalation(rec SdlcRecord, now time.Time) string {
	switch rec.Paused {
	case EscalationBlocked, EscalationNeedsInput, EscalationDuplicate:
		return rec.Paused
	}
	sle, ok := s.sle[rec.Phase]
	if !ok || sle <= 0 {
		return ""
	}
	if timeInPhase(rec, now) > sle {
		return EscalationPastSLE
	}
	return ""
}

// card builds a board/lane card, attaching a rework meter for in-review phases.
func (s *SDLC) card(rec SdlcRecord, now time.Time, esc string) SDLCCard {
	c := SDLCCard{
		Number:             rec.Number,
		Title:              rec.Title,
		URL:                rec.URL,
		Phase:              rec.Phase,
		TimeInPhaseSeconds: int(timeInPhase(rec, now).Seconds()),
		Scope:              rec.Scope,
		Escalation:         esc,
		History:            rec.History,
	}
	if c.History == nil {
		c.History = []SdlcLabelEvent{}
	}
	if th := reworkThreshold(rec.Phase); th > 0 {
		c.Rework = &SDLCRework{Count: rec.ReworkCount, Threshold: th}
	}
	return c
}

// matrix builds the release-by-hub-cluster matrix from the cached promotion
// snapshot (SDLC-VIS-03). Each bundle row's contents are PR-driven: they come from
// that bundle's own merged pull requests (the shared bundle->PR enrichment), split
// into tracked issues (pull requests grouped under the execution record they resolve
// to) and one-off pull requests (no tracked issue). The work-item -> PRs resolver is
// inverted into a PR -> issue map and used only to group, never to attribute issues
// to bundles. The released-vs-unreleased decision does not depend on the compare-window
// horizon: a record whose pull requests fall in no resolved window but which merged at
// or before the oldest resolved bundle's build date is contained in that bundle (and
// every newer one), so it is grouped under that oldest bundle row rather than reported
// in-flight (SDLC-VIS-22). Only records genuinely merged after the newest resolved
// bundle collect in the single top Unreleased row, which holds only genuinely in-flight
// tracked work (SDLC-VIS-11).
func (s *SDLC) matrix(ctx context.Context, shipped []SdlcRecord) SDLCMatrix {
	m := SDLCMatrix{HubClusters: []SDLCHubCluster{}, Rows: []SDLCBundleRow{}}

	var promo PromotionPayload
	if s.promo != nil {
		if p, ok := s.promo.Get().Data.(PromotionPayload); ok {
			promo = p
		}
	}

	// Columns: each environment is a hub cluster, in the promotion plane's order.
	activeKeyByEnv := map[string]string{}
	for _, key := range promo.Order {
		env := promo.Environments[key]
		hc := SDLCHubCluster{Key: key, Cluster: env.Cluster, Env: env.Env}
		if env.Active != nil {
			hc.CurrentVersion = env.Active.Version
			hc.CurrentDigest = releaseKey(env.Active)
			activeKeyByEnv[key] = releaseKey(env.Active)
		}
		m.HubClusters = append(m.HubClusters, hc)
	}

	// Invert the work-item -> PRs resolver into a PR -> tracked-issue map, over the
	// in-scope shipped records only. This REUSES the resolver solely to GROUP a
	// bundle's own pull requests under the issue they resolve to; it does not attribute
	// issues to bundles (bundle contents are the bundle's PR set, below) and adds no
	// GitHub calls beyond the resolver's own cached lookups.
	recByNumber := map[int]SdlcRecord{}
	issueByPR := map[int]int{} // PR number -> tracked work-item issue number
	for _, rec := range shipped {
		recByNumber[rec.Number] = rec
		if s.issues == nil {
			continue
		}
		// A PR authoritatively resolves to a single work item, so last-write-wins here
		// is benign; if two in-scope records ever claimed the same PR, the later record
		// would win that PR's grouping and the earlier record would rely on its other
		// bundle PRs (or fall to Unreleased).
		for _, pr := range s.issues.AssociatedPRs(ctx, rec) {
			issueByPR[pr] = rec.Number
		}
	}

	// Which hub clusters currently run each bundle (for the row markers).
	hubsByKey := map[string][]string{}
	for _, key := range promo.Order {
		if rk, ok := activeKeyByEnv[key]; ok {
			hubsByKey[rk] = append(hubsByKey[rk], key)
		}
	}

	// Released rows newest-first by build date.
	type relRow struct {
		key string
		rel *Release
	}
	var ordered []relRow
	for key, rel := range promo.Releases {
		if rel == nil {
			continue
		}
		ordered = append(ordered, relRow{key, rel})
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].rel.Date != ordered[j].rel.Date {
			return ordered[i].rel.Date > ordered[j].rel.Date
		}
		return ordered[i].key < ordered[j].key
	})

	// Build each bundle row from its own pull requests, grouping tracked issues and
	// collecting one-off PRs. placed records which tracked issues landed in at least
	// one bundle, so the Unreleased row can exclude them. A tracked issue whose PRs
	// span several bundles appears in each of those bundles, grouped under the PRs that
	// landed there (bundle contents are PR-driven, SDLC-VIS-03).
	placed := map[int]bool{}
	relRows := make([]SDLCBundleRow, 0, len(ordered))
	for _, rr := range ordered {
		grouped := map[int][]SDLCPullRequest{}
		var issueOrder []int
		var others []SDLCPullRequest
		for _, pr := range rr.rel.PRs {
			sp := SDLCPullRequest(pr)
			if issNum, ok := issueByPR[pr.Number]; ok {
				if _, seen := grouped[issNum]; !seen {
					issueOrder = append(issueOrder, issNum)
				}
				grouped[issNum] = append(grouped[issNum], sp)
				placed[issNum] = true
			} else {
				others = append(others, sp)
			}
		}
		tracked := make([]SDLCShippedIssue, 0, len(issueOrder))
		for _, issNum := range issueOrder {
			rec := recByNumber[issNum]
			tracked = append(tracked, SDLCShippedIssue{
				Number:   rec.Number,
				Title:    rec.Title,
				URL:      rec.URL,
				MergedAt: rec.ClosedAt,
				MergedBy: rec.ClosedBy,
				PRs:      sortedPRs(grouped[issNum]),
			})
		}
		relRows = append(relRows, SDLCBundleRow{
			Version:     rr.rel.Version,
			Digest:      rr.rel.Digest,
			Date:        rr.rel.Date,
			HubClusters: hubsByKey[rr.key],
			Issues:      sortedShipped(tracked),
			OtherPRs:    sortedPRs(others),
		})
	}

	// SDLC-VIS-22: decouple the released-vs-unreleased decision from the compare-window
	// horizon. The resolved bundle set is bounded, so its SHA compare windows span only
	// from the oldest to the newest resolved bundle; a pull request merged before the
	// oldest resolved bundle falls inside no window even though it is contained in that
	// bundle and every newer one. Find the oldest resolved bundle row and its build date
	// (relRows mirrors the newest-first `ordered`; the minimum parseable Date is the
	// oldest). This is purely in-memory over Release.Date fields already in hand: no
	// additional GitHub call (SDLC-VIS-04).
	oldestIdx := -1
	var oldestDate time.Time
	for i := range relRows {
		d, err := time.Parse(time.RFC3339, relRows[i].Date)
		if err != nil {
			continue
		}
		if oldestIdx == -1 || d.Before(oldestDate) {
			oldestIdx = i
			oldestDate = d
		}
	}

	// Unreleased holds only genuinely in-flight tracked work: shipped records whose pull
	// requests are absent from every resolved bundle's compare window AND which merged
	// after the oldest resolved bundle's build date. One-off PRs never collect here (they
	// appear only under the bundle that shipped them). A window-unplaced record that
	// merged at or before the oldest resolved bundle's date is pre-horizon: it is already
	// contained in that bundle, so it is grouped under the oldest resolved bundle row
	// (SDLC-VIS-22) rather than dropped or reported in-flight.
	var unreleased []SDLCShippedIssue
	var preHorizon []SDLCShippedIssue
	for _, rec := range shipped {
		if placed[rec.Number] {
			continue
		}
		si := SDLCShippedIssue{Number: rec.Number, Title: rec.Title, URL: rec.URL, MergedAt: rec.ClosedAt, MergedBy: rec.ClosedBy}
		// A record whose merge time is missing or unparseable cannot be proven
		// pre-horizon, so it falls through to Unreleased: that keeps it visible rather
		// than dropped, which is the safe direction when the boundary is undecidable.
		if oldestIdx >= 0 {
			if mt, err := time.Parse(time.RFC3339, rec.ClosedAt); err == nil && !mt.After(oldestDate) {
				preHorizon = append(preHorizon, si)
				continue
			}
		}
		unreleased = append(unreleased, si)
	}

	// Keep pre-horizon items visible in a released position: group them under the oldest
	// resolved bundle row (SDLC-VIS-22). They carry no bundle PRs of their own (no window
	// placed them), so they simply join that row's tracked issues.
	if len(preHorizon) > 0 && oldestIdx >= 0 {
		relRows[oldestIdx].Issues = sortedShipped(append(relRows[oldestIdx].Issues, preHorizon...))
	}

	// Unreleased always sits at the top (no version, no hub-cluster markers, no PRs).
	m.Rows = append(m.Rows, SDLCBundleRow{Unreleased: true, Issues: sortedShipped(unreleased), OtherPRs: []SDLCPullRequest{}})
	m.Rows = append(m.Rows, relRows...)
	return m
}

// timeInPhase returns how long a record has sat in its current phase, from the
// latest phase label event (SDLC-VIS-08/14). Zero when the phase start is unknown.
func timeInPhase(rec SdlcRecord, now time.Time) time.Duration {
	if rec.PhaseSince == "" {
		return 0
	}
	t, err := time.Parse(time.RFC3339, rec.PhaseSince)
	if err != nil {
		return 0
	}
	d := now.Sub(t)
	if d < 0 {
		return 0
	}
	return d
}

// reworkThreshold returns the block threshold for a review phase, or 0 for phases
// that carry no rework meter.
func reworkThreshold(phase string) int {
	switch phase {
	case PhaseSpec:
		return specReworkThreshold
	case PhaseCode:
		return codeReworkThreshold
	default:
		return 0
	}
}

// laneRank orders the escalation lane as an action queue: blocked first, then
// needs-input and duplicate, then past-SLE (SDLC-VIS-10).
func laneRank(escalation string) int {
	switch escalation {
	case EscalationBlocked:
		return 0
	case EscalationNeedsInput:
		return 1
	case EscalationDuplicate:
		return 2
	case EscalationPastSLE:
		return 3
	default:
		return 4
	}
}

// sortLane orders lane cards by bucket, then stalest-first within a bucket, then by
// issue number for a stable order.
func sortLane(cards []SDLCCard) {
	sort.SliceStable(cards, func(i, j int) bool {
		ri, rj := laneRank(cards[i].Escalation), laneRank(cards[j].Escalation)
		if ri != rj {
			return ri < rj
		}
		if cards[i].TimeInPhaseSeconds != cards[j].TimeInPhaseSeconds {
			return cards[i].TimeInPhaseSeconds > cards[j].TimeInPhaseSeconds
		}
		return cards[i].Number < cards[j].Number
	})
}

// sortedShipped returns the issues newest-merged first (stable), never nil.
func sortedShipped(in []SDLCShippedIssue) []SDLCShippedIssue {
	out := in
	if out == nil {
		out = []SDLCShippedIssue{}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].MergedAt != out[j].MergedAt {
			return out[i].MergedAt > out[j].MergedAt
		}
		return out[i].Number > out[j].Number
	})
	return out
}

// sortedPRs returns the pull requests newest-merged first (stable), never nil. The
// order mirrors the shared bundle enrichment (MergedAt desc, then number desc), so a
// bundle's one-off PRs and a tracked issue's grouped PRs read most-recent-first.
func sortedPRs(in []SDLCPullRequest) []SDLCPullRequest {
	out := in
	if out == nil {
		out = []SDLCPullRequest{}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].MergedAt != out[j].MergedAt {
			return out[i].MergedAt > out[j].MergedAt
		}
		return out[i].Number > out[j].Number
	})
	return out
}
