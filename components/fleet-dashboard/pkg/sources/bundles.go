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
type PR struct {
	Number   int    `json:"number"`
	Title    string `json:"title"`
	URL      string `json:"url"`
	Author   string `json:"author,omitempty"`
	MergedAt string `json:"mergedAt,omitempty"`
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

	mu    sync.Mutex
	dates map[string]string // sha -> commit author date (immutable)
	prs   map[string][]PR   // "repo:base...head" -> PRs (immutable)
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

// compareResponse is the subset of GET /repos/{repo}/compare/{base}...{head}
// we consume.
type compareResponse struct {
	Commits []struct {
		Commit struct {
			Message string `json:"message"`
			Author  struct {
				Date string `json:"date"`
			} `json:"author"`
		} `json:"commit"`
		Author *struct {
			Login string `json:"login"`
		} `json:"author"`
	} `json:"commits"`
}

// comparePRs returns the PRs introduced in repo between base and head (exclusive
// of base, inclusive of head), newest first. Non-PR commits (no "(#n)" trailer,
// e.g. merge commits) are skipped. tokenFile is empty for the public upstream
// repo (unauthenticated). Cached forever, keyed by repo+range: the diff between
// two fixed commits is immutable.
func (g *GitHubBundles) comparePRs(ctx context.Context, repo, tokenFile, base, head string) []PR {
	key := repo + ":" + base + "..." + head
	g.mu.Lock()
	if prs, ok := g.prs[key]; ok {
		g.mu.Unlock()
		return prs
	}
	g.mu.Unlock()

	var cmp compareResponse
	u := fmt.Sprintf("%s/repos/%s/compare/%s...%s", g.base, repo, base, head)
	if !getGitHubJSON(ctx, g.client, tokenFile, g.logger, u, &cmp) {
		return nil
	}

	var prs []PR
	for _, c := range cmp.Commits {
		subject := c.Commit.Message
		if nl := strings.IndexByte(subject, '\n'); nl >= 0 {
			subject = subject[:nl]
		}
		m := prSuffix.FindStringSubmatch(subject)
		if m == nil {
			continue
		}
		num, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}
		pr := PR{
			Number:   num,
			Title:    strings.TrimSpace(prSuffix.ReplaceAllString(subject, "")),
			URL:      fmt.Sprintf("%s/%s/pull/%d", g.htmlBase, repo, num),
			MergedAt: c.Commit.Author.Date,
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
	return prs
}

// getJSON performs an authenticated GET and decodes the body into v. It returns
// false (and logs at Debug) on any failure, so callers degrade gracefully.
func (g *GitHubBundles) getJSON(ctx context.Context, url string, v any) bool {
	return getGitHubJSON(ctx, g.client, g.tokenFile, g.logger, url, v)
}
