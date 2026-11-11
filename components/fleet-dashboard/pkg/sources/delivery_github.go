package sources

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// UpstreamCommit is one first-parent change on the application repository's
// default branch (FLT-DLV-02).
type UpstreamCommit struct {
	SHA      string
	Message  string
	MergedAt string // committer date, RFC3339
	Author   string // GitHub login, when GitHub matched one
}

// CheckRun is the subset of a GitHub check run the delivery plane classifies
// (FLT-DLV-05).
type CheckRun struct {
	Name        string
	App         string
	Status      string // queued | in_progress | completed
	Conclusion  string
	StartedAt   string
	CompletedAt string
	URL         string
}

// BundlePR is the open automated bundle-update pull request in the GitOps
// repository (FLT-DLV-07).
type BundlePR struct {
	Number  int
	URL     string
	HeadSHA string
	// Checks summarizes the head commit's check runs: running, passed, or failed.
	Checks string
}

// EnvPromotion is one promotion on a GitOps environment branch: its merge
// commit, merge time, and pull request (FLT-DLV-18).
type EnvPromotion struct {
	SHA      string
	MergedAt string // RFC3339
	PR       int    // 0 when the merge message names no pull request
	URL      string
}

// DeliveryGitHub is the GitHub port of the delivery plane.
type DeliveryGitHub interface {
	// Commits lists the newest changes on the application repository's default
	// branch (one call per refresh).
	Commits(ctx context.Context, limit int) ([]UpstreamCommit, error)
	// CheckRuns lists sha's check runs on the application repository. The
	// delivery source decides when a commit's runs have settled (FLT-DLV-12).
	CheckRuns(ctx context.Context, sha string) ([]CheckRun, error)
	// EnvPromotions lists the promotions on a GitOps environment branch merged
	// since a time, newest first: one merge commit per promotion (FLT-DLV-18).
	EnvPromotions(ctx context.Context, branch string, since time.Time) ([]EnvPromotion, error)
	// GateRuns lists the check runs on a GitOps commit, the latest run per name.
	GateRuns(ctx context.Context, sha string) ([]CheckRun, error)
	// OpenBundlePR returns the open pull request from the configured head branch
	// in the GitOps repository, or nil when there is none (one call per refresh).
	OpenBundlePR(ctx context.Context) (*BundlePR, error)
	// ReleasePublished reports whether the application repository has a published
	// (non-draft) GitHub release for tag. A published release is cached forever.
	ReleasePublished(ctx context.Context, tag string) (published bool, url string, err error)
}

// GitHubDelivery implements DeliveryGitHub over the GitHub REST API with the
// tokens the dashboard already holds: the upstream token for the application
// repository and the GitOps token for the GitOps repository (FLT-DLV-12).
type GitHubDelivery struct {
	base              string
	htmlBase          string
	upstreamRepo      string
	upstreamTokenFile string
	gitopsRepo        string
	gitopsTokenFile   string
	prBranch          string
	client            *http.Client
	logger            *slog.Logger

	mu        sync.Mutex
	published map[string]string // tag -> release URL (published releases only)
	prChecks  map[string]string // head sha -> settled check state (passed/failed)
}

// NewGitHubDelivery builds the GitHub port. gitopsRepo or prBranch may be empty;
// OpenBundlePR then reports no pull request.
func NewGitHubDelivery(apiBase, upstreamRepo, upstreamTokenFile, gitopsRepo, gitopsTokenFile, prBranch string) *GitHubDelivery {
	base := strings.TrimRight(apiBase, "/")
	return &GitHubDelivery{
		base:              base,
		htmlBase:          htmlBaseFrom(base),
		upstreamRepo:      strings.Trim(upstreamRepo, "/"),
		upstreamTokenFile: upstreamTokenFile,
		gitopsRepo:        strings.Trim(gitopsRepo, "/"),
		gitopsTokenFile:   gitopsTokenFile,
		prBranch:          prBranch,
		client:            &http.Client{Timeout: 15 * time.Second},
		logger:            slog.Default(),
		published:         map[string]string{},
		prChecks:          map[string]string{},
	}
}

// Commits implements DeliveryGitHub.
func (g *GitHubDelivery) Commits(ctx context.Context, limit int) ([]UpstreamCommit, error) {
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	var raw []struct {
		SHA    string `json:"sha"`
		Commit struct {
			Message   string `json:"message"`
			Committer struct {
				Date string `json:"date"`
			} `json:"committer"`
		} `json:"commit"`
		Author *struct {
			Login string `json:"login"`
		} `json:"author"`
	}
	u := fmt.Sprintf("%s/repos/%s/commits?per_page=%d", g.base, g.upstreamRepo, limit)
	if !getGitHubJSON(ctx, g.client, g.upstreamTokenFile, g.logger, u, &raw) {
		return nil, fmt.Errorf("list application repository commits failed")
	}
	out := make([]UpstreamCommit, 0, len(raw))
	for _, c := range raw {
		uc := UpstreamCommit{SHA: c.SHA, Message: c.Commit.Message, MergedAt: c.Commit.Committer.Date}
		if c.Author != nil {
			uc.Author = c.Author.Login
		}
		out = append(out, uc)
	}
	return out, nil
}

// mergePR matches GitHub's merge commit subject, "Merge pull request #<n> from".
var mergePR = regexp.MustCompile(`^Merge pull request #(\d+) `)

// EnvPromotions implements DeliveryGitHub.
func (g *GitHubDelivery) EnvPromotions(ctx context.Context, branch string, since time.Time) ([]EnvPromotion, error) {
	if g.gitopsRepo == "" {
		return nil, fmt.Errorf("GitOps repository not configured")
	}
	var raw []struct {
		SHA     string `json:"sha"`
		HTMLURL string `json:"html_url"`
		Commit  struct {
			Message   string `json:"message"`
			Committer struct {
				Date string `json:"date"`
			} `json:"committer"`
		} `json:"commit"`
		Parents []struct {
			SHA string `json:"sha"`
		} `json:"parents"`
	}
	u := fmt.Sprintf("%s/repos/%s/commits?sha=%s&since=%s&per_page=100", g.base, g.gitopsRepo,
		url.QueryEscape(branch), url.QueryEscape(since.UTC().Format(time.RFC3339)))
	if !getGitHubJSON(ctx, g.client, g.gitopsTokenFile, g.logger, u, &raw) {
		return nil, fmt.Errorf("list commits on %s failed", branch)
	}
	out := make([]EnvPromotion, 0, len(raw)/2)
	for _, c := range raw {
		if len(c.Parents) < 2 {
			continue // the hydrated commit itself; its merge is the promotion
		}
		p := EnvPromotion{SHA: c.SHA, MergedAt: c.Commit.Committer.Date, URL: c.HTMLURL}
		if m := mergePR.FindStringSubmatch(c.Commit.Message); m != nil {
			p.PR, _ = strconv.Atoi(m[1])
			p.URL = fmt.Sprintf("%s/%s/pull/%d", g.htmlBase, g.gitopsRepo, p.PR)
		}
		out = append(out, p)
	}
	return out, nil
}

// GateRuns implements DeliveryGitHub.
func (g *GitHubDelivery) GateRuns(ctx context.Context, sha string) ([]CheckRun, error) {
	if g.gitopsRepo == "" {
		return nil, fmt.Errorf("GitOps repository not configured")
	}
	return g.readCheckRuns(ctx, g.gitopsRepo, g.gitopsTokenFile, sha)
}

type deliveryCheckRuns struct {
	CheckRuns []struct {
		Name        string `json:"name"`
		Status      string `json:"status"`
		Conclusion  string `json:"conclusion"`
		StartedAt   string `json:"started_at"`
		CompletedAt string `json:"completed_at"`
		HTMLURL     string `json:"html_url"`
		App         struct {
			Slug string `json:"slug"`
		} `json:"app"`
	} `json:"check_runs"`
}

// CheckRuns implements DeliveryGitHub.
func (g *GitHubDelivery) CheckRuns(ctx context.Context, sha string) ([]CheckRun, error) {
	// No cache here: the delivery source decides when a commit's runs have
	// settled (checksSettled), because a completed run can still change.
	return g.readCheckRuns(ctx, g.upstreamRepo, g.upstreamTokenFile, sha)
}

func (g *GitHubDelivery) readCheckRuns(ctx context.Context, repo, tokenFile, sha string) ([]CheckRun, error) {
	var resp deliveryCheckRuns
	u := fmt.Sprintf("%s/repos/%s/commits/%s/check-runs?per_page=100", g.base, repo, sha)
	if !getGitHubJSON(ctx, g.client, tokenFile, g.logger, u, &resp) {
		return nil, fmt.Errorf("list check runs failed")
	}
	out := make([]CheckRun, 0, len(resp.CheckRuns))
	for _, r := range resp.CheckRuns {
		out = append(out, CheckRun{
			Name: r.Name, App: r.App.Slug, Status: r.Status, Conclusion: r.Conclusion,
			StartedAt: r.StartedAt, CompletedAt: r.CompletedAt, URL: r.HTMLURL,
		})
	}
	return out, nil
}

// OpenBundlePR implements DeliveryGitHub.
func (g *GitHubDelivery) OpenBundlePR(ctx context.Context) (*BundlePR, error) {
	if g.gitopsRepo == "" || g.prBranch == "" {
		return nil, nil
	}
	owner, _, _ := strings.Cut(g.gitopsRepo, "/")
	var prs []struct {
		Number  int    `json:"number"`
		HTMLURL string `json:"html_url"`
		Head    struct {
			SHA string `json:"sha"`
		} `json:"head"`
	}
	u := fmt.Sprintf("%s/repos/%s/pulls?state=open&per_page=5&head=%s", g.base, g.gitopsRepo, url.QueryEscape(owner+":"+g.prBranch))
	if !getGitHubJSON(ctx, g.client, g.gitopsTokenFile, g.logger, u, &prs) {
		return nil, fmt.Errorf("list bundle update pull requests failed")
	}
	if len(prs) == 0 {
		return nil, nil
	}
	pr := &BundlePR{Number: prs[0].Number, URL: prs[0].HTMLURL, HeadSHA: prs[0].Head.SHA}
	pr.Checks = g.prCheckState(ctx, pr.HeadSHA)
	return pr, nil
}

// prCheckState summarizes the bundle pull request's head checks. A settled state
// is cached per head SHA; a new push changes the SHA.
func (g *GitHubDelivery) prCheckState(ctx context.Context, sha string) string {
	g.mu.Lock()
	if s, ok := g.prChecks[sha]; ok {
		g.mu.Unlock()
		return s
	}
	g.mu.Unlock()
	runs, err := g.readCheckRuns(ctx, g.gitopsRepo, g.gitopsTokenFile, sha)
	if err != nil {
		return ""
	}
	state := "passed"
	for _, r := range runs {
		switch {
		case r.Status != "completed":
			state = "running"
		case !checkSucceeded(r.Conclusion) && !checkIgnorable(r.Conclusion):
			g.mu.Lock()
			g.prChecks[sha] = "failed"
			g.mu.Unlock()
			return "failed"
		}
	}
	if state == "passed" && len(runs) > 0 {
		g.mu.Lock()
		g.prChecks[sha] = state
		g.mu.Unlock()
	}
	if len(runs) == 0 {
		return "running"
	}
	return state
}

// ReleasePublished implements DeliveryGitHub. The releases/tags endpoint only
// returns published releases, so a draft or a missing release reads as false.
func (g *GitHubDelivery) ReleasePublished(ctx context.Context, tag string) (bool, string, error) {
	g.mu.Lock()
	if u, ok := g.published[tag]; ok {
		g.mu.Unlock()
		return true, u, nil
	}
	g.mu.Unlock()
	var rel struct {
		HTMLURL string `json:"html_url"`
		Draft   bool   `json:"draft"`
	}
	u := fmt.Sprintf("%s/repos/%s/releases/tags/%s", g.base, g.upstreamRepo, url.PathEscape(tag))
	if !getGitHubJSON(ctx, g.client, g.upstreamTokenFile, g.logger, u, &rel) || rel.Draft {
		return false, "", nil
	}
	g.mu.Lock()
	g.published[tag] = rel.HTMLURL
	g.mu.Unlock()
	return true, rel.HTMLURL, nil
}

// checkSucceeded reports a passing conclusion. Konflux reports a successful
// policy evaluation as neutral (FLT-DLV-05).
func checkSucceeded(conclusion string) bool {
	return conclusion == "success" || conclusion == "neutral"
}

// checkIgnorable reports a conclusion that is neither a pass nor a failure: the
// run was cancelled or skipped, usually because a newer push replaced it.
func checkIgnorable(conclusion string) bool {
	return conclusion == "cancelled" || conclusion == "skipped" || conclusion == "stale"
}
