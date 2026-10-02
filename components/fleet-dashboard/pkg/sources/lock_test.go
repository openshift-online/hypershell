package sources

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/openshift-online/hypershell/components/fleet-dashboard/pkg/config"
)

func TestVersionAndDateFromTag(t *testing.T) {
	const tag = "release-bundle-20260930T165529000000Z-2d94439429ed22de"
	if got := versionFromTag(tag); got != "v20260930" {
		t.Errorf("versionFromTag = %q, want v20260930", got)
	}
	if got := dateFromTag(tag); got != "2026-09-30T16:55:29Z" {
		t.Errorf("dateFromTag = %q, want 2026-09-30T16:55:29Z", got)
	}
	// A tag with no recognizable timestamp falls back to the whole tag / empty date.
	if got := versionFromTag("weird-tag"); got != "weird-tag" {
		t.Errorf("versionFromTag(weird) = %q, want weird-tag", got)
	}
	if got := dateFromTag("weird-tag"); got != "" {
		t.Errorf("dateFromTag(weird) = %q, want empty", got)
	}
}

// TestNewGitHubLockResolverDisabled confirms the feature is opt-in: no repo => nil.
func TestNewGitHubLockResolverDisabled(t *testing.T) {
	if r := NewGitHubLockResolver(&config.Config{}); r != nil {
		t.Errorf("NewGitHubLockResolver should return nil when FD_GITHUB_REPO is unset")
	}
}

// contentsBody wraps JSON as the GitHub contents API does: base64 with the
// "base64" encoding tag.
func contentsBody(t *testing.T, lockJSON string) string {
	t.Helper()
	enc := base64.StdEncoding.EncodeToString([]byte(lockJSON))
	b, err := json.Marshal(contentsResponse{Content: enc, Encoding: "base64"})
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestGitHubLockResolverResolve resolves a dry SHA to the release bundle its lock
// references, forwards the installation token, and caches the (immutable) result.
func TestGitHubLockResolverResolve(t *testing.T) {
	const lockJSON = `{"reference":{"tag":"release-bundle-20260930T165529000000Z-2d94439429ed22de","digest":"sha256:6145e7f19d28d502b57f21aac8a60d4b07049390810264078e9bbc3e7aebd608"}}`
	var calls int32
	tokenSeen := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		tokenSeen = r.Header.Get("Authorization")
		if r.URL.Path == "/repos/acme/gitops/contents/versions/hypershell-release-lock.json" &&
			r.URL.Query().Get("ref") == "c2bdf68dc5b5f347a2282213fb092eb60767f87e" {
			_, _ = w.Write([]byte(contentsBody(t, lockJSON)))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	tokFile := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(tokFile, []byte("inst-token-123\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	lr := NewGitHubLockResolver(&config.Config{
		GitHubRepo:      "acme/gitops",
		GitHubAPIBase:   srv.URL,
		GitHubTokenFile: tokFile,
	})
	if lr == nil {
		t.Fatal("NewGitHubLockResolver returned nil for a configured repo")
	}

	const sha = "c2bdf68dc5b5f347a2282213fb092eb60767f87e"
	rel := lr.Resolve(context.Background(), sha)
	if rel == nil {
		t.Fatal("Resolve returned nil")
	}
	if tokenSeen != "Bearer inst-token-123" {
		t.Errorf("installation token not forwarded: %q", tokenSeen)
	}
	if rel.Version != "v20260930" {
		t.Errorf("Version = %q, want v20260930", rel.Version)
	}
	if rel.Tag != "release-bundle-20260930T165529000000Z-2d94439429ed22de" {
		t.Errorf("Tag = %q", rel.Tag)
	}
	if rel.Digest != "sha256:6145e7f19d28d502b57f21aac8a60d4b07049390810264078e9bbc3e7aebd608" {
		t.Errorf("Digest = %q", rel.Digest)
	}
	if rel.Date != "2026-09-30T16:55:29Z" {
		t.Errorf("Date = %q", rel.Date)
	}
	if rel.SHA != shortSHA(sha) {
		t.Errorf("SHA = %q, want %q", rel.SHA, shortSHA(sha))
	}

	// Second resolution is served from cache: no new HTTP call...
	rel2 := lr.Resolve(context.Background(), sha)
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("expected 1 HTTP call (cached), got %d", got)
	}
	// ...but each call returns a *distinct* pointer (a fresh copy), so a later
	// in-place Enrich on one snapshot cannot race a concurrently-served one.
	if rel2 == rel {
		t.Error("Resolve must return a fresh copy per call, not the shared cached pointer")
	}
	if rel2 == nil || rel2.Digest != rel.Digest || rel2.Version != rel.Version || rel2.SHA != rel.SHA {
		t.Errorf("cached copy content diverged: %+v vs %+v", rel2, rel)
	}
	// Mutating one copy (as Enrich would) must not touch the other or the cache.
	rel.PRs = []PR{{Number: 1}}
	if len(rel2.PRs) != 0 {
		t.Error("mutating one copy leaked into another copy")
	}
	if rel3 := lr.Resolve(context.Background(), sha); len(rel3.PRs) != 0 {
		t.Error("mutating a copy leaked into the cached original")
	}
}

// TestGitHubLockResolverFallback confirms an unreadable lock degrades to the
// short-SHA identity (no digest) rather than failing, so the env still renders.
func TestGitHubLockResolverFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	lr := NewGitHubLockResolver(&config.Config{
		GitHubRepo:    "acme/gitops",
		GitHubAPIBase: srv.URL,
	})
	const sha = "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"
	rel := lr.Resolve(context.Background(), sha)
	if rel == nil {
		t.Fatal("fallback should still return a release")
	}
	if rel.Digest != "" {
		t.Errorf("fallback must carry no digest, got %q", rel.Digest)
	}
	// Fallback mirrors ShortSHAResolver exactly: 8-char Version, 10-char SHA.
	if want := (ShortSHAResolver{}).Resolve(context.Background(), sha); rel.Version != want.Version || rel.SHA != want.SHA {
		t.Errorf("fallback identity = %q/%q, want %q/%q (ShortSHAResolver)", rel.Version, rel.SHA, want.Version, want.SHA)
	}

	// Empty SHA resolves to nil (no environment).
	if lr.Resolve(context.Background(), "") != nil {
		t.Error("Resolve(\"\") should be nil")
	}
}
