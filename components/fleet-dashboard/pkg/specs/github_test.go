package specs

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitHubListsBranchesAcrossPagesWithTheToken(t *testing.T) {
	var auth []string
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = append(auth, r.Header.Get("Authorization"))
		switch {
		case r.URL.Path == "/repos/owner/repo":
			_, _ = io.WriteString(w, `{"default_branch":"main"}`)
		case r.URL.Path == "/repos/owner/repo/branches" && r.URL.Query().Get("page") == "1":
			w.Header().Set("Link", `<x?page=2>; rel="next"`)
			_, _ = io.WriteString(w, `[{"name":"main"},{"name":"feature/x"}]`)
		case r.URL.Path == "/repos/owner/repo/branches" && r.URL.Query().Get("page") == "2":
			_, _ = io.WriteString(w, `[{"name":"release"}]`)
		case r.URL.EscapedPath() == "/repos/owner/repo/tarball/feature/a%20b":
			_, _ = io.WriteString(w, "archive")
		default:
			http.NotFound(w, r)
		}
	}))
	defer api.Close()
	tokenFile := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(tokenFile, []byte("secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	gh := NewGitHub(api.URL+"/", "/owner/repo/", tokenFile)
	list, err := gh.Branches(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if list.Default != "main" || strings.Join(list.Refs, ",") != "main,feature/x,release" {
		t.Errorf("branches = %+v", list)
	}
	body, err := gh.Archive(context.Background(), "feature/a b")
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(body)
	_ = body.Close()
	if string(data) != "archive" {
		t.Errorf("archive = %q", data)
	}
	for _, header := range auth {
		if header != "Bearer secret" {
			t.Errorf("Authorization = %q, want the token from the file", header)
		}
	}

	if _, err := gh.Archive(context.Background(), "missing"); err == nil || !strings.Contains(err.Error(), "404") {
		t.Errorf("missing ref error = %v, want the GitHub status", err)
	}
}

func TestGitHubWithoutARepoIsDisabled(t *testing.T) {
	if NewGitHub("https://api.github.com", "", "") != nil {
		t.Error("an empty repo should disable the source")
	}
}

func TestGitHubReportsAnUnreadableToken(t *testing.T) {
	gh := NewGitHub("http://127.0.0.1:1", "owner/repo", filepath.Join(t.TempDir(), "missing"))
	if _, err := gh.Branches(context.Background()); err == nil || !strings.Contains(err.Error(), "token") {
		t.Errorf("err = %v, want a token error", err)
	}
}
