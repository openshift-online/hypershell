package sources

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
)

// getGitHubJSON performs a GET against the GitHub REST API and decodes a 200 body
// into v, returning false (never an error) on any failure so callers degrade to
// their no-op/fallback path and never fail a cached source (spec §5.2). The
// installation token is read fresh from tokenFile on every call because ESO
// rotates it in place; an empty tokenFile sends an unauthenticated request.
func getGitHubJSON(ctx context.Context, client *http.Client, tokenFile string, logger *slog.Logger, url string, v any) bool {
	return getGitHubJSONConditional(ctx, client, tokenFile, logger, nil, url, v)
}

// githubETagCache remembers the ETag and body of earlier 200 answers per URL so a
// later GET can be sent as a conditional request (If-None-Match). GitHub does not
// count a 304 Not Modified answer against the REST rate limit, so an unchanged
// resource costs nothing. The cache is bounded; when full, an arbitrary entry is
// dropped (the next request for it is then a plain, counted GET).
type githubETagCache struct {
	mu      sync.Mutex
	max     int
	entries map[string]etagEntry
}

type etagEntry struct {
	etag string
	body []byte
}

func newGitHubETagCache(max int) *githubETagCache {
	return &githubETagCache{max: max, entries: map[string]etagEntry{}}
}

func (c *githubETagCache) get(url string) (etagEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[url]
	return e, ok
}

func (c *githubETagCache) put(url string, e etagEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.entries[url]; !ok && len(c.entries) >= c.max {
		for k := range c.entries {
			delete(c.entries, k)
			break
		}
	}
	c.entries[url] = e
}

// getGitHubJSONConditional is getGitHubJSON with an optional ETag cache. With a
// non-nil cache it sends If-None-Match for a URL it has seen, decodes the cached
// body on 304, and stores the ETag and body of every 200 that carries an ETag.
func getGitHubJSONConditional(ctx context.Context, client *http.Client, tokenFile string, logger *slog.Logger, etags *githubETagCache, url string, v any) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		logger.DebugContext(ctx, "github: build request", "err", err)
		return false
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if tokenFile != "" {
		tok, err := os.ReadFile(tokenFile)
		if err != nil {
			logger.DebugContext(ctx, "github: read token", "err", err)
			return false
		}
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(tok)))
	}
	var cached etagEntry
	var haveCached bool
	if etags != nil {
		if cached, haveCached = etags.get(url); haveCached {
			req.Header.Set("If-None-Match", cached.etag)
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		logger.DebugContext(ctx, "github: request failed", "err", err)
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotModified && haveCached {
		if err := json.Unmarshal(cached.body, v); err != nil {
			logger.DebugContext(ctx, "github: decode cached", "err", err)
			return false
		}
		return true
	}
	if resp.StatusCode != http.StatusOK {
		logger.DebugContext(ctx, "github: non-200", "status", resp.StatusCode, "url", url)
		return false
	}
	if etags == nil {
		if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
			logger.DebugContext(ctx, "github: decode", "err", err)
			return false
		}
		return true
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.DebugContext(ctx, "github: read body", "err", err)
		return false
	}
	if err := json.Unmarshal(body, v); err != nil {
		logger.DebugContext(ctx, "github: decode", "err", err)
		return false
	}
	if etag := resp.Header.Get("ETag"); etag != "" {
		etags.put(url, etagEntry{etag: etag, body: body})
	}
	return true
}

// postGitHubGraphQL performs an authenticated GraphQL POST and decodes the 200 body
// (the full `{"data":...}` envelope) into v, returning false (never an error) on any
// failure so callers degrade to their fallback path. GitHub's GraphQL API rejects
// unauthenticated requests, so an empty tokenFile short-circuits to false and the
// caller SHALL fall back to the REST path. The token is read fresh on every call
// because ESO rotates it in place.
func postGitHubGraphQL(ctx context.Context, client *http.Client, tokenFile, endpoint, query string, variables map[string]any, v any) bool {
	if tokenFile == "" {
		return false
	}
	tok, err := os.ReadFile(tokenFile)
	if err != nil {
		logger := slog.Default()
		logger.DebugContext(ctx, "github graphql: read token", "err", err)
		return false
	}
	body, err := json.Marshal(map[string]any{"query": query, "variables": variables})
	if err != nil {
		return false
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return false
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(tok)))

	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return false
	}
	return json.NewDecoder(resp.Body).Decode(v) == nil
}
