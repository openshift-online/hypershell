package reconciler

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"google.golang.org/grpc"
)

func TestParseGitHubOwnerRepo(t *testing.T) {
	cases := []struct {
		in         string
		owner      string
		name       string
		shouldFail bool
	}{
		{"https://github.com/acme/widgets", "acme", "widgets", false},
		{"https://github.com/acme/widgets.git", "acme", "widgets", false},
		{"git@github.com:acme/widgets.git", "acme", "widgets", false},
		{"https://github.com/acme", "", "", true},
		{"", "", "", true},
	}
	for _, c := range cases {
		owner, name, err := parseGitHubOwnerRepo(c.in)
		if c.shouldFail {
			if err == nil {
				t.Errorf("parseGitHubOwnerRepo(%q) expected error, got %s/%s", c.in, owner, name)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseGitHubOwnerRepo(%q) error = %v", c.in, err)
			continue
		}
		if owner != c.owner || name != c.name {
			t.Errorf("parseGitHubOwnerRepo(%q) = %s/%s, want %s/%s", c.in, owner, name, c.owner, c.name)
		}
	}
}

func TestGitHubSubtreeFetcher_FetchSubtree(t *testing.T) {
	// A GitHub API test server exposing the git-trees and git-blobs endpoints. Only
	// files under the import path (.hypershell) must be read.
	blobs := map[string]string{
		"sha-kust":  "apiVersion: kustomize.config.k8s.io/v1beta1\nkind: Kustomization\nresources:\n  - t.yaml\n",
		"sha-tmpl":  "apiVersion: hypershell.redhat.io/v1\nkind: SandboxTemplate\nmetadata:\n  name: t1\nspec:\n  image: img:v1\n",
		"sha-other": "should-not-be-read",
	}
	var authSeen string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authSeen = r.Header.Get("Authorization")
		switch {
		case strings.Contains(r.URL.Path, "/git/trees/"):
			_ = json.NewEncoder(w).Encode(gitTreeResponse{
				Tree: []gitTreeEntry{
					{Path: ".hypershell", Type: "tree", SHA: "sha-dir"},
					{Path: ".hypershell/kustomization.yaml", Type: "blob", SHA: "sha-kust"},
					{Path: ".hypershell/t.yaml", Type: "blob", SHA: "sha-tmpl"},
					{Path: "README.md", Type: "blob", SHA: "sha-other"},
				},
			})
		case strings.Contains(r.URL.Path, "/git/blobs/"):
			parts := strings.Split(r.URL.Path, "/")
			sha := parts[len(parts)-1]
			content, ok := blobs[sha]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_ = json.NewEncoder(w).Encode(gitBlobResponse{
				Content:  base64.StdEncoding.EncodeToString([]byte(content)),
				Encoding: "base64",
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	f := NewGitHubSubtreeFetcher(srv.Client(), srv.URL)
	r := &pb.Repository{
		Metadata: &pb.ObjectReference{Id: "r1"},
		Url:      "https://github.com/acme/widgets",
	}
	fSys, root, err := f.Fetch(context.Background(), r, "tok-123")
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if authSeen != "Bearer tok-123" {
		t.Errorf("Authorization header = %q, want Bearer tok-123", authSeen)
	}

	// The fetched subtree must render through the shared library, and the README
	// outside the import path must not have been pulled into the root.
	if fSys.Exists("/README.md") {
		t.Error("README.md outside the import path was fetched")
	}
	resources, err := renderImportables(fSys, root)
	if err != nil {
		t.Fatalf("renderImportables(fetched) error = %v", err)
	}
	if len(resources) != 1 || resources[0].Kind != "SandboxTemplate" || resources[0].Name != "t1" {
		t.Fatalf("fetched+rendered = %+v, want one SandboxTemplate t1", resources)
	}
}

func TestGitHubSubtreeFetcher_HeadRevision(t *testing.T) {
	var acceptSeen, pathSeen string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		acceptSeen = r.Header.Get("Accept")
		pathSeen = r.URL.Path
		if !strings.Contains(r.URL.Path, "/commits/") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		// The `.sha` media type returns the bare commit SHA, not JSON.
		_, _ = w.Write([]byte("abc123def456"))
	}))
	defer srv.Close()

	f := NewGitHubSubtreeFetcher(srv.Client(), srv.URL)
	mainBranch := "main"
	r := &pb.Repository{
		Metadata:      &pb.ObjectReference{Id: "r1"},
		Url:           "https://github.com/acme/widgets",
		DefaultBranch: &mainBranch,
	}
	rev, err := f.HeadRevision(context.Background(), r, "tok-123")
	if err != nil {
		t.Fatalf("HeadRevision() error = %v", err)
	}
	if rev != "abc123def456" {
		t.Errorf("revision = %q, want abc123def456", rev)
	}
	if acceptSeen != "application/vnd.github.sha" {
		t.Errorf("Accept header = %q, want application/vnd.github.sha (bare SHA, not the full commit)", acceptSeen)
	}
	if !strings.Contains(pathSeen, "/commits/main") {
		t.Errorf("request path = %q, want the commits endpoint for branch main", pathSeen)
	}
}

func TestGitHubSubtreeFetcher_TruncatedTreeFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(gitTreeResponse{Truncated: true})
	}))
	defer srv.Close()
	f := NewGitHubSubtreeFetcher(srv.Client(), srv.URL)
	_, _, err := f.Fetch(context.Background(), &pb.Repository{Url: "https://github.com/a/b"}, "")
	if err == nil || !strings.Contains(err.Error(), "truncated") {
		t.Fatalf("Fetch() error = %v, want a truncation error", err)
	}
}

// ---- credential precedence ----

type fakeSecretSourceClient struct {
	pb.SecretSourceServiceClient
	source *pb.SecretSource
	err    error
}

func (f *fakeSecretSourceClient) GetSecretSource(ctx context.Context, in *pb.GetSecretSourceRequest, opts ...grpc.CallOption) (*pb.GetSecretSourceResponse, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &pb.GetSecretSourceResponse{SecretSource: f.source}, nil
}

type fakeSecretReader struct {
	values map[string]string
}

func (f *fakeSecretReader) Get(ctx context.Context, location string) (string, error) {
	v, ok := f.values[location]
	if !ok {
		return "", errorsNotFound(location)
	}
	return v, nil
}

func errorsNotFound(loc string) error { return &notFoundError{loc} }

type notFoundError struct{ loc string }

func (e *notFoundError) Error() string { return "no secret at " + e.loc }

func TestCredentialResolver_DefaultPlatformToken(t *testing.T) {
	r := NewSecretStoreCredentialResolver(nil, nil, "platform-token")
	// No secret_source_id -> default platform token.
	got, err := r.Resolve(context.Background(), &pb.Repository{Metadata: &pb.ObjectReference{Id: "r1"}})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got != "platform-token" {
		t.Errorf("Resolve() = %q, want platform-token", got)
	}
}

func TestCredentialResolver_PerRepositoryPrecedence(t *testing.T) {
	sources := &fakeSecretSourceClient{source: &pb.SecretSource{Path: "secret/data/git", Metadata: &pb.ObjectReference{Id: "ss1"}}}
	reader := &fakeSecretReader{values: map[string]string{"secret/data/git": "repo-specific-token"}}
	r := NewSecretStoreCredentialResolver(reader, sources, "platform-token")

	repo := &pb.Repository{Metadata: &pb.ObjectReference{Id: "r1"}, SecretSourceId: "ss1"}
	got, err := r.Resolve(context.Background(), repo)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got != "repo-specific-token" {
		t.Errorf("Resolve() = %q, want the per-repository credential to take precedence over the platform token", got)
	}
}

func TestParseGitHubAppMaterial(t *testing.T) {
	cases := []struct {
		name  string
		value string
		ok    bool
	}{
		{"complete app material", `{"app_id":"1","installation_id":"2","private_key":"-----BEGIN RSA PRIVATE KEY-----\nx\n-----END RSA PRIVATE KEY-----"}`, true},
		{"plain token", "ghs_abc123", false},
		{"missing private_key", `{"app_id":"1","installation_id":"2"}`, false},
		{"missing app_id", `{"installation_id":"2","private_key":"pem"}`, false},
		{"missing installation_id", `{"app_id":"1","private_key":"pem"}`, false},
		{"not an object", `["a","b"]`, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, ok := parseGitHubAppMaterial(c.value)
			if ok != c.ok {
				t.Errorf("parseGitHubAppMaterial(%q) ok = %v, want %v", c.value, ok, c.ok)
			}
		})
	}
}

// newTestAppPrivateKeyPEM generates a throwaway RSA key and returns its PKCS1 PEM.
func newTestAppPrivateKeyPEM(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	block := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}
	return string(pem.EncodeToMemory(block))
}

// TestCredentialResolver_GitHubAppMintsInstallationToken verifies that App material
// is exchanged for an installation token (an RS256 App JWT is presented to the
// access-tokens endpoint) and that the minted token is cached across resolves.
func TestCredentialResolver_GitHubAppMintsInstallationToken(t *testing.T) {
	pemKey := newTestAppPrivateKeyPEM(t)
	calls := 0
	var authSeen string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, "/app/installations/inst-9/access_tokens") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		calls++
		authSeen = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"token":      "ghs_minted_installation_token",
			"expires_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		})
	}))
	defer srv.Close()

	material := githubAppMaterial{AppID: "42", InstallationID: "inst-9", PrivateKey: pemKey}
	matJSON, err := json.Marshal(material)
	if err != nil {
		t.Fatalf("marshal material: %v", err)
	}
	sources := &fakeSecretSourceClient{source: &pb.SecretSource{Path: "secret/data/github-app", Metadata: &pb.ObjectReference{Id: "ss1"}}}
	reader := &fakeSecretReader{values: map[string]string{"secret/data/github-app": string(matJSON)}}
	r := NewSecretStoreCredentialResolverWithGitHubApp(reader, sources, "platform-token", srv.Client(), srv.URL)

	repo := &pb.Repository{Metadata: &pb.ObjectReference{Id: "r1"}, SecretSourceId: "ss1", Provider: "github"}
	got, err := r.Resolve(context.Background(), repo)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got != "ghs_minted_installation_token" {
		t.Errorf("Resolve() = %q, want the minted installation token", got)
	}
	if !strings.HasPrefix(authSeen, "Bearer ") {
		t.Errorf("access-tokens request Authorization = %q, want a Bearer App JWT", authSeen)
	}

	// A second resolve must serve the cached token without re-minting.
	if _, err := r.Resolve(context.Background(), repo); err != nil {
		t.Fatalf("second Resolve() error = %v", err)
	}
	if calls != 1 {
		t.Errorf("installation-token endpoint called %d times, want 1 (token should be cached)", calls)
	}
}

// TestCredentialResolver_GitHubAppMintError surfaces a GitHub failure as an error
// that names the ids but never the key.
func TestCredentialResolver_GitHubAppMintError(t *testing.T) {
	pemKey := newTestAppPrivateKeyPEM(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"Bad credentials"}`))
	}))
	defer srv.Close()

	material := githubAppMaterial{AppID: "42", InstallationID: "inst-9", PrivateKey: pemKey}
	matJSON, _ := json.Marshal(material)
	sources := &fakeSecretSourceClient{source: &pb.SecretSource{Path: "secret/data/github-app", Metadata: &pb.ObjectReference{Id: "ss1"}}}
	reader := &fakeSecretReader{values: map[string]string{"secret/data/github-app": string(matJSON)}}
	r := NewSecretStoreCredentialResolverWithGitHubApp(reader, sources, "", srv.Client(), srv.URL)

	repo := &pb.Repository{Metadata: &pb.ObjectReference{Id: "r1"}, SecretSourceId: "ss1", Provider: "github"}
	_, err := r.Resolve(context.Background(), repo)
	if err == nil {
		t.Fatal("Resolve() expected an error from the token exchange")
	}
	if strings.Contains(err.Error(), pemKey) {
		t.Error("Resolve() error leaked the private key material")
	}
}
