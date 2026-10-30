package specs

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// Source lists a repository's branches and downloads one as a gzipped tar.
type Source interface {
	Branches(ctx context.Context) (RefList, error)
	Archive(ctx context.Context, ref string) (io.ReadCloser, error)
}

// RefList is the answer of the refs endpoint.
type RefList struct {
	Default string   `json:"default"`
	Refs    []string `json:"refs"`
}

// maxBranchPages caps the branch listing at 1000 branches.
const maxBranchPages = 10

// GitHub reads a repository through the GitHub REST API.
type GitHub struct {
	base      string
	repo      string
	tokenFile string
	client    *http.Client
}

// NewGitHub returns a source for repo ("owner/name") at the REST API base.
// The token is read from tokenFile on every call, because it is rotated in
// place; an empty tokenFile sends unauthenticated requests. An empty repo
// returns nil, which disables the viewer's data API.
func NewGitHub(base, repo, tokenFile string) *GitHub {
	repo = strings.Trim(repo, "/")
	if repo == "" {
		return nil
	}
	return &GitHub{
		base:      strings.TrimRight(base, "/"),
		repo:      repo,
		tokenFile: tokenFile,
		client:    &http.Client{Timeout: 2 * time.Minute},
	}
}

func (g *GitHub) get(ctx context.Context, path string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, g.base+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if g.tokenFile != "" {
		token, err := os.ReadFile(g.tokenFile)
		if err != nil {
			return nil, fmt.Errorf("reading the GitHub token: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(token)))
	}
	resp, err := g.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("GitHub answered %s for %s", resp.Status, path)
	}
	return resp, nil
}

func (g *GitHub) getJSON(ctx context.Context, path string, v any) (http.Header, error) {
	resp, err := g.get(ctx, path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		return nil, fmt.Errorf("decoding %s: %w", path, err)
	}
	return resp.Header, nil
}

// Branches lists every branch, with the repository's default branch.
func (g *GitHub) Branches(ctx context.Context) (RefList, error) {
	var repo struct {
		DefaultBranch string `json:"default_branch"`
	}
	if _, err := g.getJSON(ctx, "/repos/"+g.repo, &repo); err != nil {
		return RefList{}, err
	}
	list := RefList{Default: repo.DefaultBranch, Refs: []string{}}
	for page := 1; page <= maxBranchPages; page++ {
		var branches []struct {
			Name string `json:"name"`
		}
		header, err := g.getJSON(ctx, fmt.Sprintf("/repos/%s/branches?per_page=100&page=%d", g.repo, page), &branches)
		if err != nil {
			return RefList{}, err
		}
		for _, branch := range branches {
			list.Refs = append(list.Refs, branch.Name)
		}
		if !strings.Contains(header.Get("Link"), `rel="next"`) {
			break
		}
	}
	return list, nil
}

// Archive downloads the gzipped tar of ref; the caller closes it.
func (g *GitHub) Archive(ctx context.Context, ref string) (io.ReadCloser, error) {
	segments := strings.Split(ref, "/")
	for i, segment := range segments {
		segments[i] = url.PathEscape(segment)
	}
	resp, err := g.get(ctx, "/repos/"+g.repo+"/tarball/"+strings.Join(segments, "/"))
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}
