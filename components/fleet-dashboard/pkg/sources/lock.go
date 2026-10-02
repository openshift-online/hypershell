package sources

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/openshift-online/hypershell/components/fleet-dashboard/pkg/config"
)

// lockPath is the release-bundle lock the gitops repo renders every environment
// from. Its reference (tag + digest) is the TRUE identity of the HyperShell
// release an environment runs - unlike the gitops dry-commit SHA, which also
// advances on delivery-plumbing commits that do not change the product bundle
// (e.g. a per-cluster PostSync-hook fix). Keying releases on this collapses
// every env on the same bundle onto one freight card so an env is only ever
// "behind" when it genuinely runs an older bundle.
const lockPath = "versions/hypershell-release-lock.json"

// tagTimestamp pulls the YYYYMMDDThhmmss stamp out of a release-bundle tag like
// "release-bundle-20260930T165529000000Z-2d94439429ed22de".
var tagTimestamp = regexp.MustCompile(`(\d{8}T\d{6})`)

// GitHubLockResolver resolves a gitops dry-commit SHA to the release bundle that
// commit deploys, by reading lockPath at that SHA through the GitHub contents
// API (same contents:read installation token as GitHubBundles/GitHubAnalysis --
// spec §6/§7). The lock at a fixed commit is immutable, so results are cached
// forever and a 30s promotion refresh never re-hits GitHub for a SHA it already
// resolved. Best-effort: any failure falls back to the short-SHA identity so the
// dashboard still shows the environment (just not collapsed) and never blocks.
type GitHubLockResolver struct {
	base      string // API base, e.g. https://api.github.com (no trailing slash)
	repo      string // owner/repo (fleet-identifying: from config, never compiled in)
	tokenFile string
	client    *http.Client
	logger    *slog.Logger

	mu       sync.Mutex
	resolved map[string]*Release // sha -> release (immutable)
}

// NewGitHubLockResolver builds a resolver from config, or returns nil when
// FD_GITHUB_REPO is unset -- the feature is opt-in and the repo slug is a
// fleet-identifying value that must arrive from config (config.go firewall). A
// nil return signals NewPromotion to fall back to the short-SHA resolver.
func NewGitHubLockResolver(c *config.Config) *GitHubLockResolver {
	if c.GitHubRepo == "" {
		return nil
	}
	return &GitHubLockResolver{
		base:      strings.TrimRight(c.GitHubAPIBase, "/"),
		repo:      strings.Trim(c.GitHubRepo, "/"),
		tokenFile: c.GitHubTokenFile,
		client:    &http.Client{Timeout: 15 * time.Second},
		logger:    slog.Default(),
		resolved:  map[string]*Release{},
	}
}

// contentsResponse is the subset of GET /repos/{repo}/contents/{path} we read.
type contentsResponse struct {
	Content  string `json:"content"`
	Encoding string `json:"encoding"`
}

// releaseLock is the subset of versions/hypershell-release-lock.json we read:
// the bundle reference's tag (human version + timestamp) and digest (the stable
// bundle identity).
type releaseLock struct {
	Reference struct {
		Tag    string `json:"tag"`
		Digest string `json:"digest"`
	} `json:"reference"`
}

// Resolve implements VersionResolver. It returns the release bundle sha deploys,
// or the short-SHA fallback when the lock cannot be read/parsed.
//
// The cached entry is immutable and shared, so every return value is a fresh
// shallow copy: downstream (canonicalRelease -> payload.Releases -> the cached
// snapshot the HTTP handler serializes) and GitHubBundles.Enrich mutate the
// returned *Release in place each refresh. Handing out the cached pointer itself
// would let refresh N+1's Enrich write a struct that a concurrently-served
// snapshot from refresh N still references -- a data race. A per-call copy gives
// each refresh its own mutable instance while the cached original stays pristine.
func (g *GitHubLockResolver) Resolve(ctx context.Context, sha string) *Release {
	if sha == "" {
		return nil
	}
	g.mu.Lock()
	cached, ok := g.resolved[sha]
	g.mu.Unlock()
	if ok {
		return copyRelease(cached)
	}

	r := g.resolveUncached(ctx, sha)

	// Cache only a genuine bundle resolution (digest present). A fallback means
	// GitHub was unreachable or the token was not yet mounted; caching it would
	// pin the env to the uncollapsed short-SHA identity forever, so we let the
	// next refresh retry. Misses recur only for the handful of unresolved envs
	// (bounded by the environment count), staying well within rate limits --
	// same policy as GitHubAnalysis.
	if r != nil && r.Digest != "" {
		g.mu.Lock()
		g.resolved[sha] = r
		g.mu.Unlock()
		return copyRelease(r)
	}
	// Fallback releases are already fresh per call (resolveUncached allocates a
	// new one each time) and never cached, so they are safe to return directly.
	return r
}

// copyRelease returns a shallow copy of r. The only slice field, PRs, is
// reassigned wholesale by GitHubBundles.Enrich (never appended to in place), so
// a shallow copy is sufficient to give each caller an independently-mutable
// Release without sharing backing storage.
func copyRelease(r *Release) *Release {
	if r == nil {
		return nil
	}
	c := *r
	return &c
}

// fallback is the short-SHA identity used when the lock is unavailable so an
// unresolvable env still renders (uncollapsed). It delegates to ShortSHAResolver
// so the two can never drift (8-char Version, 10-char SHA).
func fallback(sha string) *Release {
	return ShortSHAResolver{}.Resolve(context.Background(), sha)
}

func (g *GitHubLockResolver) resolveUncached(ctx context.Context, sha string) *Release {
	var body contentsResponse
	u := fmt.Sprintf("%s/repos/%s/contents/%s?ref=%s", g.base, g.repo, lockPath, sha)
	if !getGitHubJSON(ctx, g.client, g.tokenFile, g.logger, u, &body) {
		return fallback(sha)
	}
	if strings.ToLower(strings.TrimSpace(body.Encoding)) != "base64" {
		g.logger.DebugContext(ctx, "lock resolver: unexpected content encoding", "encoding", body.Encoding, "sha", sha)
		return fallback(sha)
	}
	// The contents API wraps base64 in newlines; StdEncoding rejects those.
	raw, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(body.Content, "\n", ""))
	if err != nil {
		g.logger.DebugContext(ctx, "lock resolver: base64 decode", "err", err, "sha", sha)
		return fallback(sha)
	}
	var lock releaseLock
	if err := json.Unmarshal(raw, &lock); err != nil {
		g.logger.DebugContext(ctx, "lock resolver: json decode", "err", err, "sha", sha)
		return fallback(sha)
	}
	tag := strings.TrimSpace(lock.Reference.Tag)
	digest := strings.TrimSpace(lock.Reference.Digest)
	if tag == "" || digest == "" {
		g.logger.DebugContext(ctx, "lock resolver: lock missing reference tag/digest", "sha", sha)
		return fallback(sha)
	}
	return &Release{
		Version: versionFromTag(tag),
		Tag:     tag,
		Digest:  digest,
		Date:    dateFromTag(tag),
		SHA:     shortSHA(sha),
	}
}

// commitRef is the subset of GET /repos/{repo}/commits we read: just the SHA of
// each commit that touched the lock file.
type commitRef struct {
	SHA string `json:"sha"`
}

// Recent implements ReleaseHistory. It walks the default branch's commit history
// for lockPath (newest first) and resolves each commit to the bundle it rendered,
// returning the most recent `limit` DISTINCT release bundles. This surfaces
// previously-deployed bundles (every lock change was promoted through the fleet)
// as history cards even once no environment still runs them.
//
// Each distinct commit SHA is resolved through the same per-SHA cache Resolve
// uses, so after the first refresh warms it the history costs no GitHub calls.
// We over-fetch commits (2x limit) so that lock commits which do not change the
// bundle digest (rare delivery-plumbing touches) still leave `limit` distinct
// bundles after dedup. Best-effort: a GitHub failure returns nil and the caller
// simply omits history.
func (g *GitHubLockResolver) Recent(ctx context.Context, limit int) []*Release {
	if limit <= 0 {
		return nil
	}
	var commits []commitRef
	u := fmt.Sprintf("%s/repos/%s/commits?path=%s&per_page=%d", g.base, g.repo, lockPath, limit*2)
	if !getGitHubJSON(ctx, g.client, g.tokenFile, g.logger, u, &commits) {
		return nil
	}
	seen := map[string]bool{}
	out := make([]*Release, 0, limit)
	for _, c := range commits {
		r := g.Resolve(ctx, c.SHA)
		// Only genuine bundle resolutions (digest present) count as history; a
		// fallback would key on the commit SHA and pollute the timeline with a
		// pseudo-bundle. Resolve already declined to cache those.
		if r == nil || r.Digest == "" {
			continue
		}
		k := releaseKey(r)
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, r)
		if len(out) >= limit {
			break
		}
	}
	return out
}

// versionFromTag derives a human release version ("v20260930") from a bundle tag
// like "release-bundle-20260930T165529000000Z-2d94439429ed22de", matching the
// "Update HyperShell release bundle to vYYYYMMDD" convention. Falls back to the
// full tag when it carries no recognizable timestamp.
func versionFromTag(tag string) string {
	if m := tagTimestamp.FindString(tag); m != "" {
		return "v" + m[:8] // YYYYMMDD
	}
	return tag
}

// dateFromTag parses the bundle build time from the tag's YYYYMMDDThhmmss stamp
// and returns it as RFC3339 (UTC), so frontier selection orders bundles by build
// time (lexically, as the other enrichers do). Returns "" when absent.
func dateFromTag(tag string) string {
	m := tagTimestamp.FindString(tag)
	if m == "" {
		return ""
	}
	t, err := time.Parse("20060102T150405", m)
	if err != nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
