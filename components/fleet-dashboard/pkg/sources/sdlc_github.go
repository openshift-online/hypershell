package sources

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/openshift-online/hypershell/components/fleet-dashboard/pkg/config"
)

// GitHubSDLC is the GitHub-label adapter implementing SdlcStateProvider
// (SDLC-VIS-02). It reads the in-scope execution records by label from the PUBLIC
// product repository and reconstructs each record's label-event history from the
// GitHub Issue Events / Timeline API (SDLC-VIS-14), deriving the current phase,
// pause state, time-in-phase start, and rework count in domain terms. All data is
// public product-repo data, read on the shared refresh loop and served to every
// viewer from cache (SDLC-VIS-04); a viewer load never triggers a GitHub request.
//
// The source repository slug arrives from FD_UPSTREAM_REPO, never compiled in
// (SDLC-VIS-05 / fleet-identity firewall). Issue titles and labels are public
// product-repo data and are permitted at runtime.
type GitHubSDLC struct {
	base      string // API base, e.g. https://api.github.com (no trailing slash)
	htmlBase  string // HTML base for issue links
	repo      string // owner/repo of the public product repo (execution records)
	tokenFile string // optional; empty => unauthenticated public reads
	client    *http.Client
	logger    *slog.Logger
	// maxPages bounds pagination per listing so one runaway response can never make
	// an unbounded number of GitHub calls (SDLC-VIS-04).
	maxPages int
}

// NewGitHubSDLC builds the GitHub-label adapter, or returns nil when no source
// repository is configured (FD_UPSTREAM_REPO empty), signalling the SDLC plane off.
func NewGitHubSDLC(c *config.Config) *GitHubSDLC {
	repo := strings.Trim(c.UpstreamRepo, "/")
	if repo == "" {
		return nil
	}
	base := strings.TrimRight(c.GitHubAPIBase, "/")
	return &GitHubSDLC{
		base:      base,
		htmlBase:  htmlBaseFrom(base),
		repo:      repo,
		tokenFile: c.UpstreamTokenFile,
		client:    &http.Client{Timeout: 20 * time.Second},
		logger:    slog.Default(),
		maxPages:  50,
	}
}

// ghIssue is the subset of a GitHub issue we read. The issues endpoint also returns
// pull requests; a non-nil PullRequest marks those so we can skip them.
type ghIssue struct {
	Number      int     `json:"number"`
	Title       string  `json:"title"`
	HTMLURL     string  `json:"html_url"`
	State       string  `json:"state"`
	ClosedAt    *string `json:"closed_at"`
	PullRequest *struct {
		URL string `json:"url"`
	} `json:"pull_request"`
	Labels []struct {
		Name string `json:"name"`
	} `json:"labels"`
}

// ghTimelineEvent is the subset of an Issue Timeline event we read: labeled and
// unlabeled transitions with their timestamps (SDLC-VIS-14).
type ghTimelineEvent struct {
	Event     string `json:"event"`
	CreatedAt string `json:"created_at"`
	Label     *struct {
		Name string `json:"name"`
	} `json:"label"`
}

// Records implements SdlcStateProvider. It lists the in-scope execution records
// (issues carrying the execution-record marker, any state) and reconstructs each
// record's label history from the timeline. A failure to list the in-scope set is
// returned as an error so the cache can flag the plane stale (SDLC-VIS-01); a
// per-record timeline failure degrades only that record (empty history).
func (g *GitHubSDLC) Records(ctx context.Context) ([]SdlcRecord, error) {
	issues, ok := g.listIssues(ctx)
	if !ok {
		return nil, fmt.Errorf("github sdlc: list execution records from %q failed", g.repo)
	}
	out := make([]SdlcRecord, 0, len(issues))
	for _, iss := range issues {
		if iss.PullRequest != nil {
			continue // the issues endpoint also returns PRs; execution records are issues
		}
		out = append(out, g.record(ctx, iss))
	}
	return out, nil
}

// record builds one execution record: current labels drive the phase and pause
// state; the reconstructed timeline drives time-in-phase and the rework count.
func (g *GitHubSDLC) record(ctx context.Context, iss ghIssue) SdlcRecord {
	labels := map[string]bool{}
	for _, l := range iss.Labels {
		labels[l.Name] = true
	}
	closed := iss.State == "closed"
	history := g.history(ctx, iss.Number)

	rec := SdlcRecord{
		Number:  iss.Number,
		Title:   iss.Title,
		URL:     g.issueURL(iss),
		Closed:  closed,
		History: history,
	}
	if closed && iss.ClosedAt != nil {
		rec.ClosedAt = *iss.ClosedAt
	}
	rec.Phase = derivePhase(labels, closed)
	rec.Paused = derivePaused(labels, closed)
	rec.PhaseSince = phaseSince(history, rec.Phase)
	rec.ReworkCount = reworkCount(history, rec.Phase)
	return rec
}

// issueURL prefers the issue's html_url; it falls back to a constructed URL so a
// link is always present even if the field is absent in a fixture.
func (g *GitHubSDLC) issueURL(iss ghIssue) string {
	if iss.HTMLURL != "" {
		return iss.HTMLURL
	}
	return fmt.Sprintf("%s/%s/issues/%d", g.htmlBase, g.repo, iss.Number)
}

// listIssues fetches every execution-record issue (any state), following pages.
// Returns false only when the first page fails, so the plane can distinguish a
// hard failure from an empty funnel.
func (g *GitHubSDLC) listIssues(ctx context.Context) ([]ghIssue, bool) {
	var all []ghIssue
	for page := 1; page <= g.maxPages; page++ {
		u := fmt.Sprintf("%s/repos/%s/issues?labels=%s&state=all&per_page=100&page=%d",
			g.base, g.repo, labelExecutionRecord, page)
		var batch []ghIssue
		if !getGitHubJSON(ctx, g.client, g.tokenFile, g.logger, u, &batch) {
			if page == 1 {
				return nil, false
			}
			break // a later page failed; serve what we have rather than fail the plane
		}
		all = append(all, batch...)
		if len(batch) < 100 {
			break
		}
	}
	return all, true
}

// history reconstructs an issue's label-event history from the timeline, following
// pages. Best-effort: a failure yields whatever was collected (possibly empty).
func (g *GitHubSDLC) history(ctx context.Context, number int) []SdlcLabelEvent {
	var events []SdlcLabelEvent
	for page := 1; page <= g.maxPages; page++ {
		u := fmt.Sprintf("%s/repos/%s/issues/%d/timeline?per_page=100&page=%d",
			g.base, g.repo, number, page)
		var batch []ghTimelineEvent
		if !getGitHubJSON(ctx, g.client, g.tokenFile, g.logger, u, &batch) {
			break
		}
		for _, e := range batch {
			if e.Event != "labeled" && e.Event != "unlabeled" {
				continue
			}
			if e.Label == nil {
				continue
			}
			events = append(events, SdlcLabelEvent{Label: e.Label.Name, Action: e.Event, At: e.CreatedAt})
		}
		if len(batch) < 100 {
			break
		}
	}
	return events
}

// derivePhase maps the current label set to a funnel phase. A closed, non-duplicate
// record is Shipped (merged); a closed duplicate keeps its last active phase and is
// routed to the lane by its pause state. Open records resolve to their active phase.
func derivePhase(labels map[string]bool, closed bool) string {
	if closed && !labels[labelDuplicate] {
		return PhaseShipped
	}
	return activePhase(labels)
}

// activePhase resolves the single active funnel phase from the current labels,
// highest-progress first (the funnel normalizes to one phase label at a time, but
// this is defensive if more than one is present).
func activePhase(labels map[string]bool) string {
	switch {
	case labels[labelReleaseVerified]:
		return PhaseShipped
	case labels[labelCodeApproved]:
		return PhaseCodeApproved
	case labels[labelReviewableCode] || labels[labelCodeRejected]:
		return PhaseCode
	case labels[labelSpecApproved]:
		return PhaseSpecApproved
	case labels[labelReviewableSpec] || labels[labelSpecRejected]:
		return PhaseSpec
	case labels[labelWorkable]:
		return PhaseWorkable
	default:
		return PhaseIntake
	}
}

// derivePaused returns the pause/escalation label for the record, or "". A closed,
// non-duplicate record (shipped) is never paused: blocked/needs-input on it would
// be stale, and it belongs in the matrix.
func derivePaused(labels map[string]bool, closed bool) string {
	if closed && !labels[labelDuplicate] {
		return ""
	}
	switch {
	case labels[labelBlocked]:
		return EscalationBlocked
	case labels[labelNeedsInput]:
		return EscalationNeedsInput
	case labels[labelDuplicate]:
		return EscalationDuplicate
	default:
		return ""
	}
}

// phaseSince returns the RFC3339 timestamp of the latest labeled event for the
// current phase's defining labels, i.e. when the record entered its current phase.
// Empty when no such event exists (time-in-phase then reads as unknown/zero).
func phaseSince(history []SdlcLabelEvent, phase string) string {
	targets := phaseLabels(phase)
	best := ""
	for _, e := range history {
		if e.Action != "labeled" || !targets[e.Label] {
			continue
		}
		if e.At > best { // RFC3339 Z timestamps sort chronologically
			best = e.At
		}
	}
	return best
}

// reworkCount counts the labeled events for the current review phase's reject label.
// This is the ONLY faithful source of the count because the funnel removes the
// reject label on retry, so the live label set never retains it (SDLC-VIS-09/14).
func reworkCount(history []SdlcLabelEvent, phase string) int {
	var target string
	switch phase {
	case PhaseSpec:
		target = labelSpecRejected
	case PhaseCode:
		target = labelCodeRejected
	default:
		return 0
	}
	n := 0
	for _, e := range history {
		if e.Action == "labeled" && e.Label == target {
			n++
		}
	}
	return n
}

// phaseLabels is the set of labels whose most recent application marks entry into a
// phase. Review phases include their reject label, since a retry (reject label
// re-applied) restarts the clock for that phase.
func phaseLabels(phase string) map[string]bool {
	switch phase {
	case PhaseShipped:
		return labelSet(labelReleaseVerified, labelCodeApproved)
	case PhaseCodeApproved:
		return labelSet(labelCodeApproved)
	case PhaseCode:
		return labelSet(labelReviewableCode, labelCodeRejected)
	case PhaseSpecApproved:
		return labelSet(labelSpecApproved)
	case PhaseSpec:
		return labelSet(labelReviewableSpec, labelSpecRejected)
	case PhaseWorkable:
		return labelSet(labelWorkable)
	case PhaseIntake:
		return labelSet(labelExecutionRecord)
	default:
		return map[string]bool{}
	}
}

func labelSet(names ...string) map[string]bool {
	out := make(map[string]bool, len(names))
	for _, n := range names {
		out[n] = true
	}
	return out
}

// closesRef matches a pull-request body's closing reference ("Closes #123",
// "Fixes #45", "Resolves #9", case-insensitive) so a merged PR resolves to the
// execution record it closed (SDLC-VIS-03).
var closesRef = regexp.MustCompile(`(?i)\b(?:close[sd]?|fix(?:e[sd]?)?|resolve[sd]?)\b[:\s]+#(\d+)`)

// GitHubIssueBundles implements IssueBundleResolver: the one PR->closing-issue hop
// SDLC-VIS-03 adds on top of the shared bundle->PR enrichment. It reads the merged
// PR's body from the PUBLIC product repo and parses its closing reference. A merged
// PR's body is effectively immutable, so successful lookups are cached forever and
// the refresh loop re-hits GitHub only for PRs it has not yet resolved.
type GitHubIssueBundles struct {
	base      string
	repo      string
	tokenFile string
	client    *http.Client
	logger    *slog.Logger

	mu      sync.Mutex
	closing map[int]int // pr number -> closing issue number (cached on success)
}

// NewGitHubIssueBundles builds the PR->closing-issue resolver, or returns nil when
// no source repository is configured (FD_UPSTREAM_REPO empty).
func NewGitHubIssueBundles(c *config.Config) *GitHubIssueBundles {
	repo := strings.Trim(c.UpstreamRepo, "/")
	if repo == "" {
		return nil
	}
	return &GitHubIssueBundles{
		base:      strings.TrimRight(c.GitHubAPIBase, "/"),
		repo:      repo,
		tokenFile: c.UpstreamTokenFile,
		client:    &http.Client{Timeout: 15 * time.Second},
		logger:    slog.Default(),
		closing:   map[int]int{},
	}
}

// ClosingIssue implements IssueBundleResolver. It returns the issue a merged PR
// closed, or 0. Only successful lookups are cached, so a transient GitHub failure
// is retried on the next refresh rather than pinned to 0 forever.
func (g *GitHubIssueBundles) ClosingIssue(ctx context.Context, prNumber int) int {
	g.mu.Lock()
	if v, ok := g.closing[prNumber]; ok {
		g.mu.Unlock()
		return v
	}
	g.mu.Unlock()

	var pr struct {
		Body string `json:"body"`
	}
	u := fmt.Sprintf("%s/repos/%s/pulls/%d", g.base, g.repo, prNumber)
	if !getGitHubJSON(ctx, g.client, g.tokenFile, g.logger, u, &pr) {
		return 0
	}
	issue := 0
	if m := closesRef.FindStringSubmatch(pr.Body); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil {
			issue = n
		}
	}
	g.mu.Lock()
	g.closing[prNumber] = issue
	g.mu.Unlock()
	return issue
}
