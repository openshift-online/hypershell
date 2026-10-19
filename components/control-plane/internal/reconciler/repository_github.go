package reconciler

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v4"
	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/api-server/pkg/secretstore"
	"sigs.k8s.io/kustomize/kyaml/filesys"
)

// defaultImportPath is the kustomize root read when a Repository does not set
// import_path, per data-model.spec.md ("Declarative `.hypershell` Import").
const defaultImportPath = ".hypershell"

// defaultBranch is the ref read when a Repository does not set default_branch.
const defaultBranch = "main"

// httpDoer is the subset of *http.Client the GitHub fetcher needs; a seam for
// tests.
type httpDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// GitHubSubtreeFetcher reads only the import_path subtree of a GitHub-hosted
// Repository over the GitHub REST API (the git-trees and blobs endpoints) and
// loads it into an in-memory filesystem. It performs NO git clone or checkout and
// writes nothing to disk: the control-plane runtime image ships no git binary and
// mounts no writable working copy (control-plane.spec.md "Import reads the
// import_path subtree over the GitHub API, not a git clone"). The authenticating
// token is resolved per Repository by a CredentialResolver and passed to Fetch.
type GitHubSubtreeFetcher struct {
	client  httpDoer
	apiBase string
}

// NewGitHubSubtreeFetcher builds a fetcher against apiBase (default
// https://api.github.com). A nil client uses a bounded-timeout http.Client.
func NewGitHubSubtreeFetcher(client httpDoer, apiBase string) *GitHubSubtreeFetcher {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	if apiBase == "" {
		apiBase = "https://api.github.com"
	}
	return &GitHubSubtreeFetcher{client: client, apiBase: strings.TrimRight(apiBase, "/")}
}

// gitTreeResponse is the subset of the GitHub git-trees response we read.
type gitTreeResponse struct {
	Tree      []gitTreeEntry `json:"tree"`
	Truncated bool           `json:"truncated"`
}

type gitTreeEntry struct {
	Path string `json:"path"`
	Type string `json:"type"` // "blob" or "tree"
	SHA  string `json:"sha"`
}

// gitBlobResponse is the subset of the GitHub git-blobs response we read.
type gitBlobResponse struct {
	Content  string `json:"content"`
	Encoding string `json:"encoding"`
}

// Fetch reads the Repository's import_path subtree at its branch and returns an
// in-memory filesystem rooted at "/" containing the subtree re-rooted to the
// import path, plus the kustomize root path within it ("/").
func (f *GitHubSubtreeFetcher) Fetch(ctx context.Context, repo *pb.Repository, token string) (filesys.FileSystem, string, error) {
	owner, name, err := parseGitHubOwnerRepo(repo.GetUrl())
	if err != nil {
		return nil, "", err
	}
	branch := repo.GetDefaultBranch()
	if branch == "" {
		branch = defaultBranch
	}
	importPath := strings.Trim(repo.GetImportPath(), "/")
	if importPath == "" {
		importPath = defaultImportPath
	}

	tree, err := f.getTree(ctx, owner, name, branch, token)
	if err != nil {
		return nil, "", err
	}
	if tree.Truncated {
		// A truncated tree means we would silently miss files; fail loudly rather
		// than render a partial kustomization.
		return nil, "", fmt.Errorf("github tree for %s/%s@%s is truncated; repository too large to read the full subtree", owner, name, branch)
	}

	prefix := importPath + "/"
	fSys := filesys.MakeFsInMemory()
	found := false
	for _, entry := range tree.Tree {
		if entry.Type != "blob" {
			continue
		}
		if entry.Path != importPath && !strings.HasPrefix(entry.Path, prefix) {
			continue
		}
		rel := strings.TrimPrefix(entry.Path, prefix)
		if rel == "" {
			// import_path pointed at a file, not a directory; skip - a kustomize root
			// is a directory.
			continue
		}
		content, err := f.getBlob(ctx, owner, name, entry.SHA, token)
		if err != nil {
			return nil, "", fmt.Errorf("read %s: %w", entry.Path, err)
		}
		dest := "/" + rel
		if dir := path.Dir(dest); dir != "/" && dir != "." {
			if err := fSys.MkdirAll(dir); err != nil {
				return nil, "", fmt.Errorf("prepare in-memory dir %s: %w", dir, err)
			}
		}
		if err := fSys.WriteFile(dest, content); err != nil {
			return nil, "", fmt.Errorf("write in-memory %s: %w", dest, err)
		}
		found = true
	}
	if !found {
		return nil, "", fmt.Errorf("no files found under import path %q in %s/%s@%s", importPath, owner, name, branch)
	}

	return fSys, "/", nil
}

func (f *GitHubSubtreeFetcher) getTree(ctx context.Context, owner, name, branch, token string) (*gitTreeResponse, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/git/trees/%s?recursive=1",
		f.apiBase, url.PathEscape(owner), url.PathEscape(name), url.PathEscape(branch))
	var out gitTreeResponse
	if err := f.getJSON(ctx, endpoint, token, &out); err != nil {
		return nil, fmt.Errorf("get git tree %s/%s@%s: %w", owner, name, branch, err)
	}
	return &out, nil
}

func (f *GitHubSubtreeFetcher) getBlob(ctx context.Context, owner, name, sha, token string) ([]byte, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/git/blobs/%s",
		f.apiBase, url.PathEscape(owner), url.PathEscape(name), url.PathEscape(sha))
	var out gitBlobResponse
	if err := f.getJSON(ctx, endpoint, token, &out); err != nil {
		return nil, err
	}
	switch out.Encoding {
	case "base64":
		// GitHub wraps base64 blob content at 60 columns.
		decoded, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(out.Content, "\n", ""))
		if err != nil {
			return nil, fmt.Errorf("decode base64 blob: %w", err)
		}
		return decoded, nil
	case "utf-8", "":
		return []byte(out.Content), nil
	default:
		return nil, fmt.Errorf("unsupported blob encoding %q", out.Encoding)
	}
}

func (f *GitHubSubtreeFetcher) getJSON(ctx context.Context, endpoint, token string, out interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return fmt.Errorf("request %s: %w", endpoint, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		// The GitHub error body does not contain the credential; it is safe to
		// include for diagnosis, bounded in length.
		snippet := strings.TrimSpace(string(body))
		if len(snippet) > 256 {
			snippet = snippet[:256]
		}
		return fmt.Errorf("github API returned %d: %s", resp.StatusCode, snippet)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

// parseGitHubOwnerRepo extracts the owner and repository name from a repository
// URL. It accepts https URLs (https://github.com/owner/repo[.git]) and scp-like
// git URLs (git@github.com:owner/repo.git).
func parseGitHubOwnerRepo(raw string) (owner, name string, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", fmt.Errorf("repository url is empty")
	}

	var repoPath string
	switch {
	case strings.HasPrefix(raw, "git@"):
		// git@github.com:owner/repo.git
		_, after, ok := strings.Cut(raw, ":")
		if !ok {
			return "", "", fmt.Errorf("unrecognized git url %q", raw)
		}
		repoPath = after
	default:
		u, perr := url.Parse(raw)
		if perr != nil {
			return "", "", fmt.Errorf("parse repository url %q: %w", raw, perr)
		}
		repoPath = u.Path
	}

	repoPath = strings.TrimPrefix(repoPath, "/")
	repoPath = strings.TrimSuffix(repoPath, ".git")
	parts := strings.Split(repoPath, "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("repository url %q does not contain owner/repo", raw)
	}
	return parts[0], parts[1], nil
}

// SecretStoreCredentialResolver resolves the bearer token used to read a
// Repository's subtree. The platform GitHub App credential (platformToken) is the
// default transport. When the Repository declares a secret_source_id, that
// per-Repository credential takes precedence: the resolver looks up the
// referenced SecretSource to find its backend path and reads the DURABLE MATERIAL
// through the read-only secret-store client (the same single-directional store
// access the control plane uses for agent provider material). It never writes to
// the store.
//
// For a GitHub repository the stored material is the GitHub App PEM (plus app_id /
// installation_id), NOT a static installation token. The resolver mints a
// short-lived (~1h) installation token from that material, signing an RS256 App JWT
// and exchanging it, and caches it until shortly before expiry. This realizes the
// app-installation-token refresh strategy and supersedes the static-token resolver
// path (agent-secret-late-binding.spec.md "GitHub App Credentials via
// app-installation-token"). A plain stored token (no App material) is still
// accepted as a transitional fallback.
type SecretStoreCredentialResolver struct {
	secrets       secretstore.Reader
	sources       pb.SecretSourceServiceClient
	platformToken string
	minter        *githubAppInstallationTokenMinter
}

// NewSecretStoreCredentialResolver builds a resolver with a default GitHub App
// token minter against the public GitHub API. platformToken is the default GitHub
// App credential (may be empty, e.g. for public repositories); secrets and sources
// resolve a per-Repository secret_source_id credential when one is set.
func NewSecretStoreCredentialResolver(secrets secretstore.Reader, sources pb.SecretSourceServiceClient, platformToken string) *SecretStoreCredentialResolver {
	return NewSecretStoreCredentialResolverWithGitHubApp(secrets, sources, platformToken, nil, "")
}

// NewSecretStoreCredentialResolverWithGitHubApp builds a resolver whose GitHub App
// token minter uses the given HTTP client and API base (both default when zero: a
// bounded-timeout client and https://api.github.com). The client and apiBase seam
// lets tests exercise minting against an httptest server.
func NewSecretStoreCredentialResolverWithGitHubApp(secrets secretstore.Reader, sources pb.SecretSourceServiceClient, platformToken string, client httpDoer, apiBase string) *SecretStoreCredentialResolver {
	return &SecretStoreCredentialResolver{
		secrets:       secrets,
		sources:       sources,
		platformToken: platformToken,
		minter:        newGitHubAppInstallationTokenMinter(client, apiBase),
	}
}

// Resolve returns the per-Repository credential when secret_source_id is set,
// otherwise the default platform token. When the per-Repository material is GitHub
// App material it returns a freshly minted (or cached) installation token.
func (r *SecretStoreCredentialResolver) Resolve(ctx context.Context, repo *pb.Repository) (string, error) {
	sourceID := repo.GetSecretSourceId()
	if sourceID == "" {
		return r.platformToken, nil
	}
	if r.secrets == nil || r.sources == nil {
		return "", fmt.Errorf("repository %s declares secret_source_id %s but no secret store client is configured", repo.GetMetadata().GetId(), sourceID)
	}
	resp, err := r.sources.GetSecretSource(ctx, &pb.GetSecretSourceRequest{Id: sourceID})
	if err != nil {
		return "", fmt.Errorf("get secret source %s: %w", sourceID, err)
	}
	source := resp.GetSecretSource()
	if source == nil || source.GetPath() == "" {
		return "", fmt.Errorf("secret source %s has no backend path", sourceID)
	}
	value, err := r.secrets.Get(ctx, source.GetPath())
	if err != nil {
		return "", fmt.Errorf("read git credential from store: %w", err)
	}
	// GitHub App material: mint (and cache) an installation token from the PEM.
	if material, ok := parseGitHubAppMaterial(value); ok {
		token, err := r.minter.mint(ctx, material)
		if err != nil {
			// The error carries only the app/installation ids (safe references),
			// never any bytes of the PEM.
			return "", fmt.Errorf("mint github installation token for secret source %s: %w", sourceID, err)
		}
		return token, nil
	}
	// Transitional fallback: a plain stored token used directly. Superseded by App
	// material; do not log or wrap the value.
	return value, nil
}

// githubAppMaterial is the durable GitHub App credential material stored in a
// SecretSource: the App id, the installation id, and the PEM private key. No usable
// installation token is stored; the resolver mints and rotates it. JSON field names
// mirror the ProviderBinding material/secret_material convention.
type githubAppMaterial struct {
	AppID          string `json:"app_id"`
	InstallationID string `json:"installation_id"`
	PrivateKey     string `json:"private_key"`
}

// parseGitHubAppMaterial reports whether the stored value is a GitHub App material
// object (a JSON object carrying at least a private_key, an app_id, and an
// installation_id) and returns it. A value that is not such an object (for example a
// plain token) returns ok=false so the caller can fall back.
func parseGitHubAppMaterial(value string) (githubAppMaterial, bool) {
	trimmed := strings.TrimSpace(value)
	if !strings.HasPrefix(trimmed, "{") {
		return githubAppMaterial{}, false
	}
	var m githubAppMaterial
	// A decode error here could echo bytes of the secret material, so it is
	// deliberately NOT surfaced: a non-App object is simply treated as "not App
	// material" (security.spec.md: raw material never appears in logs or errors).
	if err := json.Unmarshal([]byte(trimmed), &m); err != nil {
		return githubAppMaterial{}, false
	}
	if m.PrivateKey == "" || m.AppID == "" || m.InstallationID == "" {
		return githubAppMaterial{}, false
	}
	return m, true
}

// githubAppInstallationTokenMinter mints and caches short-lived GitHub App
// installation tokens from durable App material. OpenShell owns rotation for agent
// providers; this minter is the equivalent refresh for the control plane's own
// Repository import credential (agent-secret-late-binding.spec.md "GitHub App
// Credentials via app-installation-token"). It caches per installation id until
// shortly before expiry so a steady reconcile cadence does not re-mint every pass.
type githubAppInstallationTokenMinter struct {
	client  httpDoer
	apiBase string
	now     func() time.Time

	mu    sync.Mutex
	cache map[string]mintedInstallationToken
}

// mintedInstallationToken is a cached installation token and the instant it stops
// being usable (the real expiry minus a safety margin).
type mintedInstallationToken struct {
	token  string
	usable time.Time
}

// installationTokenRenewMargin is how long before the real expiry a cached token is
// treated as stale, so a token is never handed out moments before GitHub rejects it.
const installationTokenRenewMargin = 5 * time.Minute

// appJWTLifetime is the App JWT validity window. GitHub caps it at 10 minutes; 9
// keeps a margin against clock skew.
const appJWTLifetime = 9 * time.Minute

func newGitHubAppInstallationTokenMinter(client httpDoer, apiBase string) *githubAppInstallationTokenMinter {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	if apiBase == "" {
		apiBase = "https://api.github.com"
	}
	return &githubAppInstallationTokenMinter{
		client:  client,
		apiBase: strings.TrimRight(apiBase, "/"),
		now:     time.Now,
		cache:   map[string]mintedInstallationToken{},
	}
}

// mint returns a usable installation token for the material, from the cache when one
// is still fresh, otherwise newly minted and cached.
func (m *githubAppInstallationTokenMinter) mint(ctx context.Context, material githubAppMaterial) (string, error) {
	m.mu.Lock()
	if cached, ok := m.cache[material.InstallationID]; ok && m.now().Before(cached.usable) {
		token := cached.token
		m.mu.Unlock()
		return token, nil
	}
	m.mu.Unlock()

	token, expiry, err := m.exchange(ctx, material)
	if err != nil {
		return "", err
	}

	m.mu.Lock()
	m.cache[material.InstallationID] = mintedInstallationToken{token: token, usable: expiry.Add(-installationTokenRenewMargin)}
	m.mu.Unlock()
	return token, nil
}

// exchange signs an RS256 App JWT from the PEM and exchanges it for an installation
// token at the installations access-tokens endpoint, returning the token and its
// expiry.
func (m *githubAppInstallationTokenMinter) exchange(ctx context.Context, material githubAppMaterial) (string, time.Time, error) {
	key, err := jwt.ParseRSAPrivateKeyFromPEM([]byte(material.PrivateKey))
	if err != nil {
		// jwt's error does not echo the key bytes, only a structural reason.
		return "", time.Time{}, fmt.Errorf("parse app private key (app %s): %w", material.AppID, err)
	}
	now := m.now()
	claims := jwt.RegisteredClaims{
		Issuer: material.AppID,
		// Back-date iat by 60s per GitHub guidance so modest clock skew between this
		// host and GitHub does not trigger an "'iat' claim is in the future" rejection.
		IssuedAt:  jwt.NewNumericDate(now.Add(-60 * time.Second)),
		ExpiresAt: jwt.NewNumericDate(now.Add(appJWTLifetime)),
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(key)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign app jwt (app %s): %w", material.AppID, err)
	}

	endpoint := fmt.Sprintf("%s/app/installations/%s/access_tokens", m.apiBase, url.PathEscape(material.InstallationID))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("build installation token request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("Authorization", "Bearer "+signed)

	resp, err := m.client.Do(req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("request installation token (app %s, installation %s): %w", material.AppID, material.InstallationID, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("read installation token response: %w", err)
	}
	if resp.StatusCode != http.StatusCreated {
		// The GitHub error body does not carry the credential; include a bounded
		// snippet for diagnosis.
		snippet := strings.TrimSpace(string(body))
		if len(snippet) > 256 {
			snippet = snippet[:256]
		}
		return "", time.Time{}, fmt.Errorf("github returned %d minting installation token (app %s, installation %s): %s", resp.StatusCode, material.AppID, material.InstallationID, snippet)
	}
	var out struct {
		Token     string `json:"token"`
		ExpiresAt string `json:"expires_at"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", time.Time{}, fmt.Errorf("decode installation token response: %w", err)
	}
	if out.Token == "" {
		return "", time.Time{}, fmt.Errorf("github returned an empty installation token (app %s, installation %s)", material.AppID, material.InstallationID)
	}
	expiry, err := time.Parse(time.RFC3339, out.ExpiresAt)
	if err != nil {
		// A missing or unparseable expiry: treat the token as short-lived so the
		// next reconcile re-mints promptly rather than caching it indefinitely.
		expiry = now.Add(installationTokenRenewMargin + time.Minute)
	}
	return out.Token, expiry, nil
}
