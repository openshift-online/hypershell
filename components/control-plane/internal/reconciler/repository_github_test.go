package reconciler

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
