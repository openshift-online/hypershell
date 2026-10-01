package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"log/slog"

	"github.com/openshift-online/hypershell/components/fleet-dashboard/pkg/config"
)

// analysisCheckName is the GitHub check-run that records the hypershell-analysis
// run for a commit. It is a product-level name (not fleet-identifying), so it is
// safe as a compiled-in default (config.go firewall note). We match it exactly
// first, then fall back to any check whose name starts with it (the promoter
// mirror variant), mirroring the prototype's dump-promotion.py behaviour.
const analysisCheckName = "hypershell-analysis"

// AnalysisResolver maps a gitops commit SHA to the GitHub check-run page URL for
// its hypershell-analysis run (data-architecture.spec §5: the nullable
// environments[].analysisUrl). It is best-effort: callers treat "" as "no link".
type AnalysisResolver interface {
	// AnalysisURL returns the html_url of the hypershell-analysis check-run on
	// sha, or "" when there is no such run, GitHub is unreachable, or the
	// feature is disabled. It never returns an error: a GitHub hiccup must only
	// drop the link, never fail the /api/promotion plane (§5.2).
	AnalysisURL(ctx context.Context, sha string) string
}

// noopAnalysis is the resolver used when GitHub enrichment is disabled
// (FD_GITHUB_REPO unset). It always returns "".
type noopAnalysis struct{}

// AnalysisURL implements AnalysisResolver.
func (noopAnalysis) AnalysisURL(context.Context, string) string { return "" }

// GitHubAnalysis resolves analysisUrl via the GitHub check-runs API, using a
// scoped GitHub App installation token (contents+checks read on the gitops repo
// only, delivered by ESO -- spec §6/§7). It holds a per-SHA cache so a 30s
// promotion refresh does not re-hit GitHub for SHAs it has already resolved,
// keeping us far under the rate limit (§5.1, the prototype's _cr_cache).
type GitHubAnalysis struct {
	base      string // API base, e.g. https://api.github.com (no trailing slash)
	repo      string // owner/repo (fleet-identifying: supplied by config, never compiled in)
	tokenFile string // path to the installation-token file (ESO secret key `token`)
	client    *http.Client
	logger    *slog.Logger

	mu    sync.Mutex
	cache map[string]string // sha -> resolved html_url (positive results only)
}

// NewGitHubAnalysis builds a GitHubAnalysis resolver from config, or returns nil
// when FD_GITHUB_REPO is unset -- the feature is opt-in and the repo slug is a
// fleet-identifying value that must arrive from config (config.go firewall).
// A nil return signals callers to fall back to the no-op resolver.
func NewGitHubAnalysis(c *config.Config) *GitHubAnalysis {
	if c.GitHubRepo == "" {
		return nil
	}
	return &GitHubAnalysis{
		base:      strings.TrimRight(c.GitHubAPIBase, "/"),
		repo:      strings.Trim(c.GitHubRepo, "/"),
		tokenFile: c.GitHubTokenFile,
		client:    &http.Client{Timeout: 15 * time.Second},
		logger:    slog.Default(),
		cache:     map[string]string{},
	}
}

// checkRunsResponse is the subset of GET /repos/{repo}/commits/{sha}/check-runs
// we consume.
type checkRunsResponse struct {
	CheckRuns []struct {
		Name    string `json:"name"`
		HTMLURL string `json:"html_url"`
	} `json:"check_runs"`
}

// AnalysisURL implements AnalysisResolver.
//
// Caching policy (deliberately stricter than the one-shot prototype): we cache
// only POSITIVE results. A check-run's html_url for a SHA is immutable once it
// exists, so a hit is cached forever. A miss is NOT cached, because the analysis
// run commonly lands after the commit is first observed -- caching "" would
// pin the link to null permanently. Misses only recur for the handful of SHAs
// still awaiting analysis (bounded by the environment count), so this stays well
// within rate limits while letting a late-arriving link appear.
func (g *GitHubAnalysis) AnalysisURL(ctx context.Context, sha string) string {
	if sha == "" {
		return ""
	}
	g.mu.Lock()
	if url, ok := g.cache[sha]; ok {
		g.mu.Unlock()
		return url
	}
	g.mu.Unlock()

	url := g.fetch(ctx, sha)
	if url != "" {
		g.mu.Lock()
		g.cache[sha] = url
		g.mu.Unlock()
	}
	return url
}

// fetch queries the check-runs API for sha and returns the matching html_url, or
// "" on any failure. Failures are logged at Debug (best-effort, expected under
// rate limits/token refresh) so they are observable without alarming.
func (g *GitHubAnalysis) fetch(ctx context.Context, sha string) string {
	u := fmt.Sprintf("%s/repos/%s/commits/%s/check-runs", g.base, g.repo, sha)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		g.logger.DebugContext(ctx, "github analysis: build request", "err", err)
		return ""
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if g.tokenFile != "" {
		tok, err := os.ReadFile(g.tokenFile)
		if err != nil {
			g.logger.DebugContext(ctx, "github analysis: read token", "err", err)
			return ""
		}
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(tok)))
	}

	resp, err := g.client.Do(req)
	if err != nil {
		g.logger.DebugContext(ctx, "github analysis: request failed", "err", err)
		return ""
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		g.logger.DebugContext(ctx, "github analysis: non-200", "status", resp.StatusCode, "sha", shortSHA(sha))
		return ""
	}
	var cr checkRunsResponse
	if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil {
		g.logger.DebugContext(ctx, "github analysis: decode", "err", err)
		return ""
	}
	return matchAnalysisURL(cr)
}

// matchAnalysisURL picks the analysis check-run's html_url: an exact
// analysisCheckName match wins; otherwise the first check whose name has that
// prefix (the promoter mirror variant). Returns "" if neither is present.
func matchAnalysisURL(cr checkRunsResponse) string {
	var prefixHit string
	for _, run := range cr.CheckRuns {
		if run.Name == analysisCheckName {
			return run.HTMLURL
		}
		if prefixHit == "" && strings.HasPrefix(run.Name, analysisCheckName) {
			prefixHit = run.HTMLURL
		}
	}
	return prefixHit
}
