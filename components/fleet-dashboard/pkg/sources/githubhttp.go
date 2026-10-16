package sources

import (
	"bytes"
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
