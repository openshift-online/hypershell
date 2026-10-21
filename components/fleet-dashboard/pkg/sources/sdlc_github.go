package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
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
	Number   int     `json:"number"`
	Title    string  `json:"title"`
	HTMLURL  string  `json:"html_url"`
	State    string  `json:"state"`
	ClosedAt *string `json:"closed_at"`
	ClosedBy *struct {
		Login string `json:"login"`
	} `json:"closed_by"`
	PullRequest *struct {
		URL string `json:"url"`
	} `json:"pull_request"`
	Labels []struct {
		Name string `json:"name"`
	} `json:"labels"`
}

// ghTimelineEvent is the subset of an Issue Timeline event we read: labeled and
// unlabeled transitions with their timestamps (SDLC-VIS-14), plus cross-referenced
// events whose source is a pull request (the unauthenticated work-item -> PRs hop,
// SDLC-VIS-03).
type ghTimelineEvent struct {
	Event     string `json:"event"`
	CreatedAt string `json:"created_at"`
	Label     *struct {
		Name string `json:"name"`
	} `json:"label"`
	Source *struct {
		Issue *struct {
			Number      int              `json:"number"`
			PullRequest *json.RawMessage `json:"pull_request"`
		} `json:"issue"`
	} `json:"source"`
}

// Records implements SdlcStateProvider. It lists the in-scope work items
// (issues carrying the agent/allowed authorization gate, any state) and
// reconstructs each record's label history from the timeline. A failure to list
// the in-scope set is returned as an error so the cache can flag the plane stale
// (SDLC-VIS-01); a per-record timeline failure degrades only that record (empty
// history).
func (g *GitHubSDLC) Records(ctx context.Context) ([]SdlcRecord, error) {
	issues, ok := g.listIssues(ctx)
	if !ok {
		return nil, fmt.Errorf("github sdlc: list allowed work items from %q failed", g.repo)
	}
	out := make([]SdlcRecord, 0, len(issues))
	for _, iss := range issues {
		if iss.PullRequest != nil {
			continue // the issues endpoint also returns PRs; work items are issues
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
	if closed && iss.ClosedBy != nil {
		rec.ClosedBy = iss.ClosedBy.Login
	}
	rec.Phase = derivePhase(labels, closed)
	rec.Paused = derivePaused(labels, closed)
	rec.PhaseSince = phaseSince(history, rec.Phase)
	rec.ReworkCount = reworkCount(history, rec.Phase)
	rec.Scope = scopeLabel(iss)
	return rec
}

// scopeLabel returns the issue's agent/scope/* selector label (full label text), or
// "" when none is present. It is a data value, not compiled-in: the funnel's scope
// vocabulary lives on the issues, so we only match the shared prefix.
func scopeLabel(iss ghIssue) string {
	const prefix = "agent/scope/"
	for _, l := range iss.Labels {
		if strings.HasPrefix(l.Name, prefix) {
			return l.Name
		}
	}
	return ""
}

// issueURL prefers the issue's html_url; it falls back to a constructed URL so a
// link is always present even if the field is absent in a fixture.
func (g *GitHubSDLC) issueURL(iss ghIssue) string {
	if iss.HTMLURL != "" {
		return iss.HTMLURL
	}
	return fmt.Sprintf("%s/%s/issues/%d", g.htmlBase, g.repo, iss.Number)
}

// listIssues fetches every allowed work item (any state), following pages.
// Returns false only when the first page fails, so the plane can distinguish a
// hard failure from an empty funnel (SDLC-VIS-23).
func (g *GitHubSDLC) listIssues(ctx context.Context) ([]ghIssue, bool) {
	var all []ghIssue
	for page := 1; page <= g.maxPages; page++ {
		u := fmt.Sprintf("%s/repos/%s/issues?labels=%s&state=all&per_page=100&page=%d",
			g.base, g.repo, labelAllowed, page)
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
		return labelSet(labelAllowed)
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

// workItemPRMaxPages bounds timeline pagination for the work-item -> PRs lookup so a
// pathological issue cannot stall the refresh loop. Cross-references on a real work
// item fit well within this.
const workItemPRMaxPages = 10

// GitHubWorkItemPRs implements WorkItemPRResolver: the work-item -> PRs hop
// SDLC-VIS-03 adds on top of the shared bundle->PR enrichment. It resolves the pull
// requests ASSOCIATED WITH a work item (not a PR's self-declared closing reference),
// so an issue delivered by several PRs maps correctly and the attribution survives a
// PR body that omits the "Closes #N" keyword.
//
// Source of truth, by configuration:
//   - With an upstream token: GitHub GraphQL `closedByPullRequestsReferences`, the
//     authoritative set of linked/closing PRs.
//   - Unauthenticated (the default public-repo deployment): the REST issue timeline,
//     collecting `cross-referenced` events whose source is a pull request. GraphQL is
//     not usable unauthenticated, so this is the fallback (and the backstop when a
//     token is present but the GraphQL call yields nothing).
//
// Results are cached per work item only when non-empty, so an as-yet-unlinked record
// is retried on the next refresh rather than pinned to Unreleased forever.
type GitHubWorkItemPRs struct {
	base       string // REST base, e.g. https://api.github.com
	graphqlURL string // GraphQL endpoint, used only when tokenFile is set
	repo       string // owner/name
	tokenFile  string
	client     *http.Client
	logger     *slog.Logger

	mu    sync.Mutex
	cache map[int][]int // work-item number -> associated PR numbers (cached when non-empty)
}

// NewGitHubWorkItemPRs builds the work-item -> PRs resolver, or returns nil when no
// source repository is configured (FD_UPSTREAM_REPO empty).
func NewGitHubWorkItemPRs(c *config.Config) *GitHubWorkItemPRs {
	repo := strings.Trim(c.UpstreamRepo, "/")
	if repo == "" {
		return nil
	}
	return &GitHubWorkItemPRs{
		base:       strings.TrimRight(c.GitHubAPIBase, "/"),
		graphqlURL: strings.TrimRight(c.GitHubGraphQL, "/"),
		repo:       repo,
		tokenFile:  c.UpstreamTokenFile,
		client:     &http.Client{Timeout: 15 * time.Second},
		logger:     slog.Default(),
		cache:      map[int][]int{},
	}
}

// AssociatedPRs implements WorkItemPRResolver. It returns the PR numbers associated
// with the record's work item, preferring the authoritative GraphQL set when a token
// is configured and falling back to the unauthenticated REST timeline otherwise.
func (g *GitHubWorkItemPRs) AssociatedPRs(ctx context.Context, record SdlcRecord) []int {
	number := record.Number
	g.mu.Lock()
	if v, ok := g.cache[number]; ok {
		g.mu.Unlock()
		return v
	}
	g.mu.Unlock()

	prs := g.graphQLLinkedPRs(ctx, number)
	if len(prs) == 0 {
		prs = g.timelineLinkedPRs(ctx, number)
	}
	if len(prs) > 0 {
		g.mu.Lock()
		g.cache[number] = prs
		g.mu.Unlock()
	}
	return prs
}

// graphQLLinkedPRs returns the issue's closing/linked PRs via GraphQL, or nil when no
// token is configured or the call fails (caller then falls back to the timeline).
func (g *GitHubWorkItemPRs) graphQLLinkedPRs(ctx context.Context, number int) []int {
	if g.tokenFile == "" || g.graphqlURL == "" {
		return nil
	}
	owner, name, ok := strings.Cut(g.repo, "/")
	if !ok {
		return nil
	}
	const query = `query($owner:String!,$name:String!,$number:Int!){repository(owner:$owner,name:$name){issue(number:$number){closedByPullRequestsReferences(first:50,includeClosedPrs:true){nodes{number}}}}}`
	var out struct {
		Data struct {
			Repository struct {
				Issue struct {
					ClosedByPullRequestsReferences struct {
						Nodes []struct {
							Number int `json:"number"`
						} `json:"nodes"`
					} `json:"closedByPullRequestsReferences"`
				} `json:"issue"`
			} `json:"repository"`
		} `json:"data"`
	}
	vars := map[string]any{"owner": owner, "name": name, "number": number}
	if !postGitHubGraphQL(ctx, g.client, g.tokenFile, g.graphqlURL, query, vars, &out) {
		return nil
	}
	var prs []int
	for _, n := range out.Data.Repository.Issue.ClosedByPullRequestsReferences.Nodes {
		if n.Number != 0 {
			prs = append(prs, n.Number)
		}
	}
	return prs
}

// timelineLinkedPRs returns the PRs that reference the issue, read from the REST
// issue timeline (cross-referenced events whose source is a pull request). This works
// unauthenticated against the public product repo. Best-effort and deduped.
func (g *GitHubWorkItemPRs) timelineLinkedPRs(ctx context.Context, number int) []int {
	seen := map[int]bool{}
	var prs []int
	for page := 1; page <= workItemPRMaxPages; page++ {
		u := fmt.Sprintf("%s/repos/%s/issues/%d/timeline?per_page=100&page=%d", g.base, g.repo, number, page)
		var batch []ghTimelineEvent
		if !getGitHubJSON(ctx, g.client, g.tokenFile, g.logger, u, &batch) {
			break
		}
		for _, e := range batch {
			if e.Event != "cross-referenced" || e.Source == nil || e.Source.Issue == nil {
				continue
			}
			if e.Source.Issue.PullRequest == nil || e.Source.Issue.Number == 0 {
				continue
			}
			if n := e.Source.Issue.Number; !seen[n] {
				seen[n] = true
				prs = append(prs, n)
			}
		}
		if len(batch) < 100 {
			break
		}
	}
	return prs
}
