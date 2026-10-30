package sources

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/openshift-online/hypershell/components/fleet-dashboard/pkg/config"
)

// PR is one pull request included in a release bundle (ui-architecture Bundle
// tab: "In this bundle"). Number/title/url/author/mergedAt are all public
// repository data read at runtime from the configured repo -- no fleet structure
// is implied, so none of it is compiled in (config.go firewall note).
//
// Repository carries the owner/repo the PR belongs to (FLT-BPR-02) so the UI can
// group the application repository's pull requests apart from the GitOps delivery
// repository's, and never confuse two PRs that share a number in different
// repositories. The repo slugs arrive at runtime (never compiled in). Deduplication
// keys on Repository plus Number, never on Number alone.
type PR struct {
	Number     int    `json:"number"`
	Title      string `json:"title"`
	URL        string `json:"url"`
	Author     string `json:"author,omitempty"`
	MergedAt   string `json:"mergedAt,omitempty"`
	Repository string `json:"repository,omitempty"`
	// Environments are the deployment environments whose current source-commit
	// range includes this pull request (FLT-BPR-06). It is populated only for GitOps
	// (deployment) pull requests when environments on one bundle digest run
	// different GitOps revisions, so the UI can attribute a change to the
	// environments that actually received it and never imply it reached an
	// environment that has not. Empty for application pull requests and for
	// historical cards built without live environment context.
	Environments []string `json:"environments,omitempty"`
}

// BundleEnricher fills in each release bundle's date and the pull requests it
// introduced "since the previous build". It is best-effort: any failure leaves
// the releases untouched (empty date / nil PRs) and never fails the promotion
// plane (data-architecture spec §5.2, mirroring AnalysisResolver).
type BundleEnricher interface {
	// ResolveDates fills each release's build Date (best-effort) so the caller can
	// order bundles and select the frontier before PRs are diffed.
	ResolveDates(ctx context.Context, releases map[string]*Release)
	Enrich(ctx context.Context, releases map[string]*Release)
}

// noopBundles is the enricher used when GitHub enrichment is disabled
// (FD_GITHUB_REPO unset). It leaves releases untouched.
type noopBundles struct{}

// Enrich implements BundleEnricher.
func (noopBundles) Enrich(context.Context, map[string]*Release) {}

// ResolveDates implements BundleEnricher.
func (noopBundles) ResolveDates(context.Context, map[string]*Release) {}

// prSuffix matches the "(#1234)" trailer GitHub appends to a squash-merge commit
// subject. We derive the PR number/title from the commit subject rather than the
// pulls API because the dashboard App is scoped to contents:read only (no
// pull_requests scope) -- the compare API is sufficient.
var prSuffix = regexp.MustCompile(`\s*\(#(\d+)\)\s*$`)

// prMergeCommit matches the subject GitHub writes for a NON-squash merge ("Merge
// pull request #123 from owner/branch"). Repositories differ in merge style
// (FLT-BPR-09.04): the application repo squash-merges (prSuffix) while the GitOps
// repo may use merge commits, so PR extraction recognizes both formats.
var prMergeCommit = regexp.MustCompile(`^Merge pull request #(\d+) from `)

// GitOps range status values (serialized on Release.GitOpsStatus) let the UI tell
// an unavailable comparison (no baseline / compare error -- render an error state)
// from a successful comparison that simply found no pull requests (empty state),
// per FLT-BPR-10. The empty "" status means GitOps grouping was not attempted
// (feature disabled or no lock-history provenance), so the UI hides the section.
const (
	gitOpsAvailable   = "available"
	gitOpsUnavailable = "unavailable"
)

// gitopsLockHistoryDepth bounds how many recent gitops lock commits we read to
// place a bundle on the deployment-revision timeline. One commits-API page (max
// 100) of lock changes spans far more release bundles than the freight bar shows,
// so this never truncates a visible card's provenance.
const gitopsLockHistoryDepth = 100

// LockCommit is one gitops lock change: the source (dry) commit that rewrote the
// release lock and the bundle digest it selected. Ordered newest-first by the
// provider, it is the deployment-revision timeline the GitOps PR ranges are built
// from (FLT-BPR-04/05/07/08).
type LockCommit struct {
	SHA    string
	Digest string
}

// LockHistoryProvider yields the gitops lock-commit timeline (newest-first). The
// release-lock resolver implements it, reusing its per-SHA bundle cache so the
// walk costs no extra GitHub calls after the first refresh. A GitOps range is
// built only from these recorded commits -- never by comparing a historical
// bundle against the current branch (FLT-BPR-08).
type LockHistoryProvider interface {
	LockHistory(ctx context.Context, limit int) []LockCommit
}

// GitHubBundles enriches release bundles via the GitHub compare API using the
// same scoped installation token as GitHubAnalysis (contents read on the
// configured repo, delivered by ESO -- spec §6/§7). It caches per-SHA commit
// dates and per-(base,head) PR lists; both are immutable once computed, so a 30s
// promotion refresh never re-hits GitHub for a pair it already resolved, keeping
// us far under the rate limit (§5.1).
type GitHubBundles struct {
	base     string // API base, e.g. https://api.github.com (no trailing slash)
	htmlBase string // HTML base for PR links, e.g. https://github.com
	repo     string // gitops owner/repo (fleet-identifying: supplied by config, never compiled in)

	// upstreamRepo is the PUBLIC product source repo (owner/repo) whose merged PRs
	// compose a bundle ("In this bundle"). Not fleet-identifying, so it carries a
	// compiled-in default. Read with upstreamTokenFile, which is empty by default
	// (public repo, unauthenticated) -- the gitops installation token is scoped to
	// repo and would not authorize a different repo anyway.
	upstreamRepo      string
	upstreamTokenFile string

	tokenFile string
	client    *http.Client
	logger    *slog.Logger

	// lockHistory supplies the gitops lock-commit timeline used to bound each
	// bundle's GitOps PR range (FLT-BPR-04). nil leaves GitOps grouping disabled
	// (GitOpsStatus stays ""), so the UI simply omits the GitOps section and the
	// existing application group is unaffected.
	lockHistory LockHistoryProvider

	mu    sync.Mutex
	dates map[string]string // sha -> commit author date (immutable)
	prs   map[string][]PR   // "repo:base...head" -> PRs (immutable)
}

// WithLockHistory wires the gitops lock-commit timeline provider (the release-lock
// resolver) so Enrich can build the GitOps deployment-revision PR group. Returns
// the receiver for chaining. A nil provider leaves GitOps grouping disabled.
func (g *GitHubBundles) WithLockHistory(p LockHistoryProvider) *GitHubBundles {
	if g != nil {
		g.lockHistory = p
	}
	return g
}

// NewGitHubBundles builds a GitHubBundles enricher from config, or returns nil
// when FD_GITHUB_REPO is unset -- the feature is opt-in and the repo slug is a
// fleet-identifying value that must arrive from config (config.go firewall).
// A nil return signals callers to fall back to the no-op enricher.
func NewGitHubBundles(c *config.Config) *GitHubBundles {
	if c.GitHubRepo == "" {
		return nil
	}
	base := strings.TrimRight(c.GitHubAPIBase, "/")
	// openshift-online/hypershell is the public product source and is NOT
	// fleet-identifying, so it is safe to default in code (firewall) when config
	// (which Load() already defaults) leaves it blank, e.g. in tests.
	upstream := strings.Trim(c.UpstreamRepo, "/")
	if upstream == "" {
		upstream = "openshift-online/hypershell"
	}
	return &GitHubBundles{
		base:              base,
		htmlBase:          htmlBaseFrom(base),
		repo:              strings.Trim(c.GitHubRepo, "/"),
		upstreamRepo:      upstream,
		upstreamTokenFile: c.UpstreamTokenFile,
		tokenFile:         c.GitHubTokenFile,
		client:            &http.Client{Timeout: 15 * time.Second},
		logger:            slog.Default(),
		dates:             map[string]string{},
		prs:               map[string][]PR{},
	}
}

// htmlBaseFrom derives the web base (for PR permalinks) from the API base. The
// compare API returns commits but not the PR html_url (that needs the pulls
// association we are not scoped for), so we construct "/pull/<n>" ourselves.
func htmlBaseFrom(apiBase string) string {
	switch apiBase {
	case "", "https://api.github.com":
		return "https://github.com"
	default:
		// GitHub Enterprise: https://host/api/v3 -> https://host.
		return strings.TrimSuffix(strings.TrimRight(apiBase, "/"), "/api/v3")
	}
}

// ResolveDates implements BundleEnricher. It fills each release's build Date
// (best-effort) from its gitops commit so the caller can order bundles and
// select the frontier before PRs are diffed. A lock resolver already fills Date
// from the bundle's build timestamp; this only fills the short-SHA fallback case,
// where the gitops commit date is the only signal. Cached forever (immutable).
func (g *GitHubBundles) ResolveDates(ctx context.Context, releases map[string]*Release) {
	for _, r := range releases {
		if r == nil || r.SHA == "" || r.Date != "" {
			continue
		}
		r.Date = g.commitDate(ctx, r.SHA)
	}
}

// Enrich implements BundleEnricher. It orders the releases oldest-first and
// attaches to each the UPSTREAM product-source PRs the bundle introduced since
// the previous build -- the changes "in this bundle". These come from comparing
// consecutive bundles' manifests revisions (ManifestsRev) over the public
// upstream repo, NOT from the gitops lock-bump commits. The oldest release keeps
// nil PRs (there is no prior build to diff against); a release without upstream
// provenance (fallback/older lock) is skipped.
//
// Callers run this over the FULL release set (currently-deployed bundles plus
// merged-in history), AFTER history is merged, so a fleet fully converged on a
// single bundle still diffs the frontier against the immediately-previous bundle
// rather than finding only one deployed release and bailing out.
func (g *GitHubBundles) Enrich(ctx context.Context, releases map[string]*Release) {
	if len(releases) == 0 {
		return
	}
	g.ResolveDates(ctx, releases)
	// The GitOps deployment-change group is built from the recorded lock-commit
	// timeline, independent of the application diff below, so it runs even when there
	// is only one application bundle to show and a GitOps failure never clears the
	// application PRs (FLT-BPR-10.01).
	g.enrichGitOps(ctx, releases)

	// Keep only releases we could place on the timeline (dated) AND diff (an
	// upstream manifests revision). The map is keyed by release identity (bundle
	// digest when resolved, else SHA); we order by build date and diff on each
	// release's own upstream revision (r.ManifestsRev), not the key.
	type dated struct {
		rev  string // upstream product-source revision (bundle.manifests.git.revision)
		date string
		rel  *Release
	}
	var ordered []dated
	for _, r := range releases {
		if r == nil || r.Date == "" || r.ManifestsRev == "" {
			continue
		}
		ordered = append(ordered, dated{rev: r.ManifestsRev, date: r.Date, rel: r})
	}
	if len(ordered) < 2 {
		return
	}

	// Oldest first; tie-break on the revision for a stable order.
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].date != ordered[j].date {
			return ordered[i].date < ordered[j].date
		}
		return ordered[i].rev < ordered[j].rev
	})

	for i := 1; i < len(ordered); i++ {
		prev, cur := ordered[i-1].rev, ordered[i].rev
		ordered[i].rel.PRs = g.comparePRs(ctx, g.upstreamRepo, g.upstreamTokenFile, prev, cur)
	}
}

// enrichGitOps attaches the GitOps (deployment) pull-request group to each release
// bundle from the gitops repository (FLT-BPR-01/04). The range for a bundle is
// bounded by the recorded lock-commit timeline (FLT-BPR-08) -- never by comparing
// against the current branch -- and follows the lock commits, which are the source
// (dry) commits the lock resolver resolved, not hydrated artifacts (FLT-BPR-07).
// It is a no-op when no lock-history provenance is wired, leaving GitOpsStatus ""
// so the UI omits the section. Each release's status distinguishes an unavailable
// range from a successful empty one (FLT-BPR-10).
func (g *GitHubBundles) enrichGitOps(ctx context.Context, releases map[string]*Release) {
	if g.lockHistory == nil {
		return
	}
	history := g.lockHistory.LockHistory(ctx, gitopsLockHistoryDepth)
	for _, r := range releases {
		if r == nil || r.Digest == "" {
			continue
		}
		base, intro, ok := gitopsBaseline(history, r.Digest)
		if !ok {
			// No recorded baseline (or no place on the timeline): report the range
			// as unavailable rather than invent one (FLT-BPR-08.02).
			r.GitOpsStatus = gitOpsUnavailable
			continue
		}
		prs, ok := g.gitopsPRsFor(ctx, base, intro, r.DeploymentRevisions)
		if !ok {
			r.GitOpsStatus = gitOpsUnavailable
			continue
		}
		r.GitOpsStatus = gitOpsAvailable
		r.GitOpsPRs = prs
	}
}

// gitopsBaseline finds a bundle digest's bounded deployment range on the recorded
// lock timeline (FLT-BPR-08). history is newest-first, so intro is the MOST RECENT
// lock commit that selected this digest -- the commit the current deployment is on
// -- and base is the lock commit immediately older than it. Anchoring to the most
// recent selection (not the first introduction) keeps the range bounded when a
// digest is introduced, replaced, and later rolled back to: without it, base would
// point before the first introduction and the environment extension in gitopsPRsFor
// (base...currentEnvDrySHA) would span the whole excursion and surface the lock-
// update and configuration pull requests of the intervening digests on this card.
// Using the recorded most-recent occurrence handles rollbacks and divergent
// histories explicitly, never by ordering commits by application build date
// (FLT-BPR-08.03). ok is false when the digest is absent from the recorded timeline
// or has no older baseline commit, so the caller reports the range unavailable
// instead of inventing one.
func gitopsBaseline(history []LockCommit, digest string) (base, intro string, ok bool) {
	i := -1
	for j := 0; j < len(history); j++ {
		if history[j].Digest == digest {
			i = j
			break
		}
	}
	if i < 0 || i+1 >= len(history) {
		return "", "", false
	}
	return history[i+1].SHA, history[i].SHA, true
}

// gitopsPRsFor builds a bundle's GitOps PR group from the baseline to each
// environment's current source commit, attributing every pull request to the
// environments whose range includes it (FLT-BPR-06). The introduction commit is
// always compared (so the lock-update and introduction PRs every environment shares
// are present even before any environment advances), and each deployment revision's
// source (dry) commit extends the range so configuration-only PRs appear without a
// new digest (FLT-BPR-04.02). A pull request that only one environment's range
// contains is attributed to that environment alone, so the display never implies a
// change reached an environment that lacks it. ok is false when ANY compare target
// failed: a partially-built group would omit some environment's configuration pull
// requests and leave that environment unattributed while still reporting
// "available", which overstates completeness, so an incomplete result is surfaced as
// unavailable rather than a misleadingly-partial success (FLT-BPR-10). A set in which
// every compare succeeded but found no pull requests returns (nil, true) (empty, not
// unavailable).
func (g *GitHubBundles) gitopsPRsFor(ctx context.Context, base, intro string, revisions []DeploymentRevision) ([]PR, bool) {
	// One compare target per distinct head: the introduction commit plus each
	// environment's source commit. The intro target carries no environment (its PRs
	// are common to the digest); each revision target carries its environment so
	// per-environment-only PRs are attributed honestly.
	type target struct {
		env  string
		head string
	}
	targets := []target{{head: intro}}
	for _, dr := range revisions {
		if dr.DrySHA == "" {
			continue
		}
		targets = append(targets, target{env: dr.Env, head: dr.DrySHA})
	}

	byNum := map[int]*PR{}
	var order []int // first-seen order, refined by the sort below
	envs := map[int]map[string]bool{}
	for _, t := range targets {
		prs, ok := g.comparePRsStatus(ctx, g.repo, g.tokenFile, base, t.head)
		if !ok {
			// A failed target leaves the group incomplete (a missing environment's
			// configuration PRs and attribution). Reporting "available" on an
			// incomplete group would overstate completeness, so surface the whole
			// group as unavailable rather than a misleadingly-partial success.
			return nil, false
		}
		for i := range prs {
			pr := prs[i]
			if _, seen := byNum[pr.Number]; !seen {
				clone := pr
				clone.Environments = nil
				byNum[pr.Number] = &clone
				order = append(order, pr.Number)
			}
			if t.env != "" {
				if envs[pr.Number] == nil {
					envs[pr.Number] = map[string]bool{}
				}
				envs[pr.Number][t.env] = true
			}
		}
	}

	out := make([]PR, 0, len(order))
	for _, num := range order {
		pr := byNum[num]
		if set := envs[num]; len(set) > 0 {
			attributed := make([]string, 0, len(set))
			for env := range set {
				attributed = append(attributed, env)
			}
			sort.Strings(attributed)
			pr.Environments = attributed
		}
		out = append(out, *pr)
	}
	// Present newest-first by merge time, tie-broken on PR number desc, matching the
	// application group's ordering.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].MergedAt != out[j].MergedAt {
			return out[i].MergedAt > out[j].MergedAt
		}
		return out[i].Number > out[j].Number
	})
	if len(out) == 0 {
		return nil, true
	}
	return out, true
}

// commitDate returns sha's author date (RFC3339), or "" on any failure. Results
// are cached forever: a commit's date is immutable.
func (g *GitHubBundles) commitDate(ctx context.Context, sha string) string {
	g.mu.Lock()
	if d, ok := g.dates[sha]; ok {
		g.mu.Unlock()
		return d
	}
	g.mu.Unlock()

	var out struct {
		Commit struct {
			Author struct {
				Date string `json:"date"`
			} `json:"author"`
		} `json:"commit"`
	}
	u := fmt.Sprintf("%s/repos/%s/commits/%s", g.base, g.repo, sha)
	if !g.getJSON(ctx, u, &out) {
		return ""
	}
	date := out.Commit.Author.Date
	if date != "" {
		g.mu.Lock()
		g.dates[sha] = date
		g.mu.Unlock()
	}
	return date
}

// compareCommit is one commit in a compare response.
type compareCommit struct {
	Commit struct {
		Message string `json:"message"`
		Author  struct {
			Date string `json:"date"`
		} `json:"author"`
	} `json:"commit"`
	Author *struct {
		Login string `json:"login"`
	} `json:"author"`
}

// compareResponse is the subset of GET /repos/{repo}/compare/{base}...{head}
// we consume. TotalCommits is the grand total across pages, used to drive
// pagination so a long range does not silently lose commits (FLT-BPR-09.03).
type compareResponse struct {
	TotalCommits int             `json:"total_commits"`
	Commits      []compareCommit `json:"commits"`
}

// extractPR derives a pull request's number and title from a merged commit's
// message, covering both merge formats a repository may use (FLT-BPR-09.04): the
// squash-merge subject trailer "(#123)" and the merge-commit subject "Merge pull
// request #123 from ...". Returns ok=false for a commit that is neither (a direct
// push or an unrecognized merge), so the caller skips it.
func extractPR(message string) (num int, title string, ok bool) {
	lines := strings.Split(message, "\n")
	subject := lines[0]
	if m := prSuffix.FindStringSubmatch(subject); m != nil {
		n, err := strconv.Atoi(m[1])
		if err != nil {
			return 0, "", false
		}
		return n, strings.TrimSpace(prSuffix.ReplaceAllString(subject, "")), true
	}
	if m := prMergeCommit.FindStringSubmatch(subject); m != nil {
		n, err := strconv.Atoi(m[1])
		if err != nil {
			return 0, "", false
		}
		// A merge commit's PR title is the first non-empty body line; fall back to
		// the subject when the body is empty.
		title = strings.TrimSpace(subject)
		for _, line := range lines[1:] {
			if t := strings.TrimSpace(line); t != "" {
				title = t
				break
			}
		}
		return n, title, true
	}
	return 0, "", false
}

// comparePRs returns the PRs introduced in repo between base and head, newest
// first, discarding the available/empty distinction. Callers that need to tell an
// unavailable comparison from a genuinely empty one use comparePRsStatus.
func (g *GitHubBundles) comparePRs(ctx context.Context, repo, tokenFile, base, head string) []PR {
	prs, _ := g.comparePRsStatus(ctx, repo, tokenFile, base, head)
	return prs
}

// comparePRsStatus returns the PRs introduced in repo between base and head
// (exclusive of base, inclusive of head), newest first, plus whether the
// comparison succeeded. Non-PR commits (direct pushes, unrecognized merges) are
// skipped; the remaining commits are deduplicated by PR number (FLT-BPR-02.02,
// within one repository) and tagged with the repository (FLT-BPR-02). tokenFile is
// empty for the public upstream repo (unauthenticated). ok is false only when the
// GitHub compare call itself failed (so the caller can distinguish an error state
// from an empty-but-successful range -- FLT-BPR-10); an empty slice with ok=true
// means "no pull requests in this range". Successful results are cached forever,
// keyed by repo+range (FLT-BPR-09.02): the diff between two fixed commits is
// immutable.
func (g *GitHubBundles) comparePRsStatus(ctx context.Context, repo, tokenFile, base, head string) ([]PR, bool) {
	key := repo + ":" + base + "..." + head
	g.mu.Lock()
	if prs, ok := g.prs[key]; ok {
		g.mu.Unlock()
		return prs, true
	}
	g.mu.Unlock()

	commits, ok := g.fetchCompareCommits(ctx, repo, tokenFile, base, head)
	if !ok {
		return nil, false
	}

	var prs []PR
	seen := map[int]bool{}
	for _, c := range commits {
		num, title, ok := extractPR(c.Commit.Message)
		if !ok || seen[num] {
			continue
		}
		seen[num] = true
		pr := PR{
			Number:     num,
			Title:      title,
			URL:        fmt.Sprintf("%s/%s/pull/%d", g.htmlBase, repo, num),
			MergedAt:   c.Commit.Author.Date,
			Repository: repo,
		}
		if c.Author != nil {
			pr.Author = c.Author.Login
		}
		prs = append(prs, pr)
	}
	// Present newest-first (most recently merged PR on top). Sort explicitly by
	// merge time rather than trusting the compare API's commit order: MergedAt is the
	// squash-merge commit date (RFC3339, same Z offset, so lexical sort == chronological).
	// Tie-break on PR number desc so same-timestamp merges stay in a stable order.
	sort.SliceStable(prs, func(i, j int) bool {
		if prs[i].MergedAt != prs[j].MergedAt {
			return prs[i].MergedAt > prs[j].MergedAt
		}
		return prs[i].Number > prs[j].Number
	})

	g.mu.Lock()
	g.prs[key] = prs
	g.mu.Unlock()
	return prs, true
}

// fetchCompareCommits reads every commit in repo's base...head range, following
// the compare API's pagination to completion so a long range does not silently
// lose commits (FLT-BPR-09.03). It returns ok=false on any page failure so the
// caller can surface an unavailable state rather than a misleadingly short list.
func (g *GitHubBundles) fetchCompareCommits(ctx context.Context, repo, tokenFile, base, head string) ([]compareCommit, bool) {
	const perPage = 100
	// Safety cap: 100 pages * 100 = 10k commits, far beyond any real bundle range,
	// so a pathological (or cyclic) response can never spin forever.
	const maxPages = 100
	var all []compareCommit
	for page := 1; page <= maxPages; page++ {
		var cmp compareResponse
		u := fmt.Sprintf("%s/repos/%s/compare/%s...%s?per_page=%d&page=%d", g.base, repo, base, head, perPage, page)
		if !getGitHubJSON(ctx, g.client, tokenFile, g.logger, u, &cmp) {
			return nil, false
		}
		all = append(all, cmp.Commits...)
		// A short page (fewer than a full per_page of commits) is the last page --
		// the standard GitHub pagination terminal signal. Also stop once we have
		// reached the grand total GitHub reports for the range.
		if len(cmp.Commits) < perPage || (cmp.TotalCommits > 0 && len(all) >= cmp.TotalCommits) {
			break
		}
	}
	return all, true
}

// getJSON performs an authenticated GET and decodes the body into v. It returns
// false (and logs at Debug) on any failure, so callers degrade gracefully.
func (g *GitHubBundles) getJSON(ctx context.Context, url string, v any) bool {
	return getGitHubJSON(ctx, g.client, g.tokenFile, g.logger, url, v)
}
