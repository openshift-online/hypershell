package sources

import (
	"context"
	"sort"
	"strings"
	"time"
)

// Release diagnostics (fleet-dashboard-delivery-pipeline.spec FLT-DLV-20). The
// Konflux managed release and the bundle publisher are not observable with the
// dashboard's credentials. When a change's Release phase is Overdue, the
// "Where to look" popup points to the places that can explain it. The links
// come from settings; the dashboard reads only the application repository.

// DeliveryReleaseDiagnostics is the content of the Release popup.
type DeliveryReleaseDiagnostics struct {
	// LastGood is the newest published bundle.
	LastGood *DeliveryBundle `json:"lastGood,omitempty"`
	// LogURL links to the Konflux release log (FD_DELIVERY_RELEASE_LOG_URL).
	LogURL string `json:"logUrl,omitempty"`
	// AppURL links to the release GitHub App's settings, where pending
	// permission requests show (FD_DELIVERY_RELEASE_APP_URL). The dashboard
	// cannot read them, so it does not say whether any are pending.
	AppURL string `json:"appUrl,omitempty"`
	// Commits touch the bundle publisher's paths since LastGood was published.
	Commits      []DeliveryCommitLink `json:"commits"`
	CommitsError string               `json:"commitsError,omitempty"`
	// RegistryTag is the newest bundle tag in the registry; GitHubReleaseTag the
	// newest published GitHub release. They differ when publication stopped
	// between the two.
	RegistryTag      string `json:"registryTag,omitempty"`
	GitHubReleaseTag string `json:"githubReleaseTag,omitempty"`
	ReleaseError     string `json:"releaseError,omitempty"`
}

// DeliveryCommitLink is one application commit with its links.
type DeliveryCommitLink struct {
	SHA      string `json:"sha"`
	ShortSHA string `json:"shortSha"`
	Title    string `json:"title"`
	PR       int    `json:"pr,omitempty"`
	URL      string `json:"url"`
	MergedAt string `json:"mergedAt"`
}

// releaseOverdue reports whether any listed change's Release stage is Overdue.
func releaseOverdue(p DeliveryPayload, now time.Time) bool {
	for _, c := range p.Changes {
		for _, s := range c.Stages {
			if s.Key == stageRelease && stageOverdue(s, now) {
				return true
			}
		}
	}
	return false
}

// releaseDiagnostics reads the release diagnostics: one path-filtered commits
// call per publisher path and one releases-list call (FLT-DLV-12, FLT-DLV-20).
// A failed read is reported in its own field; the others still show.
func (d *Delivery) releaseDiagnostics(ctx context.Context, in deliveryInput, tags []RegistryBundle) *DeliveryReleaseDiagnostics {
	diag := &DeliveryReleaseDiagnostics{LogURL: d.releaseLogURL, AppURL: d.releaseAppURL, Commits: []DeliveryCommitLink{}}
	var since time.Time
	if n := len(in.bundles); n > 0 {
		b := in.bundles[n-1]
		pub := bundlePublished(in, b)
		since = pub
		diag.LastGood = &DeliveryBundle{Tag: b.Tag, Version: b.Version, Digest: b.Digest, Locked: in.lockedSet[b.Digest]}
		if !pub.IsZero() {
			diag.LastGood.PublishedAt = pub.UTC().Format(time.RFC3339)
		}
	}
	for _, t := range tags {
		if t.Tag > diag.RegistryTag {
			diag.RegistryTag = t.Tag
		}
	}

	if !since.IsZero() {
		seen := map[string]bool{}
		var errs []string
		for _, path := range d.releasePaths {
			commits, err := d.gh.CommitsTouching(ctx, path, since)
			if err != nil {
				errs = append(errs, err.Error())
				continue
			}
			for _, c := range commits {
				if seen[c.SHA] {
					continue
				}
				seen[c.SHA] = true
				pr, title := splitSubject(c.Message)
				diag.Commits = append(diag.Commits, DeliveryCommitLink{
					SHA: c.SHA, ShortSHA: shortSHA7(c.SHA), Title: title, PR: pr,
					URL:      in.htmlBase + "/" + in.upstreamRepo + "/commit/" + c.SHA,
					MergedAt: c.MergedAt,
				})
			}
		}
		sort.Slice(diag.Commits, func(i, j int) bool { return diag.Commits[i].MergedAt > diag.Commits[j].MergedAt })
		diag.CommitsError = strings.Join(errs, "; ")
	}

	tag, err := d.gh.LatestReleaseTag(ctx)
	if err != nil {
		diag.ReleaseError = err.Error()
	}
	diag.GitHubReleaseTag = tag
	return diag
}
