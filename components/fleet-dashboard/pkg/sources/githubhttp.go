package sources

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"strings"
)

// getGitHubJSON performs a GET against the GitHub REST API and decodes a 200 body
// into v, returning false (never an error) on any failure so callers degrade to
// their no-op/fallback path and never fail a cached source (spec §5.2). The
// installation token is read fresh from tokenFile on every call because ESO
// rotates it in place; an empty tokenFile sends an unauthenticated request.
func getGitHubJSON(ctx context.Context, client *http.Client, tokenFile string, logger *slog.Logger, url string, v any) bool {
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

	resp, err := client.Do(req)
	if err != nil {
		logger.DebugContext(ctx, "github: request failed", "err", err)
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		logger.DebugContext(ctx, "github: non-200", "status", resp.StatusCode, "url", url)
		return false
	}
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		logger.DebugContext(ctx, "github: decode", "err", err)
		return false
	}
	return true
}
