package sources

import (
	"context"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/openshift-online/hypershell/components/fleet-dashboard/pkg/cache"
	"github.com/openshift-online/hypershell/components/fleet-dashboard/pkg/config"
)

// Delivery pipeline plane (fleet-dashboard-delivery-pipeline.spec). It follows
// each application change from merge to the last promotion environment as one row
// of stages, from sources the dashboard already reads.

// Stage states (FLT-DLV-04).
const (
	StageNotStarted = "not-started"
	StageWaiting    = "waiting"
	StageRunning    = "running"
	StagePassed     = "passed"
	StageFailed     = "failed"
	StageNotNeeded  = "not-needed"
	StageUnknown    = "unknown"
)

// Fixed pipeline stage keys, in order (FLT-DLV-03). Environment-group stages
// follow, keyed "env:<group>".
const (
	stageMerged  = "merged"
	stageBuild   = "build"
	stagePolicy  = "policy"
	stageRelease = "release"
	stageGitOps  = "gitops"
)

// Reason codes. The UI owns the wording; the BFF only names the condition.
const (
	reasonNoComponentChanged = "no-component-changed"
	reasonBuildReplaced      = "build-replaced"
	reasonWaitingPolicy      = "waiting-policy"
	reasonInferredRelease    = "inferred-release"
	reasonNextBundle         = "next-bundle"
	reasonWaitingBundlePR    = "waiting-bundle-pr"
	reasonWaitingHydration   = "waiting-hydration"
	reasonWaitingGroups      = "waiting-groups"
	reasonGateRunning        = "gate-running"
	reasonGateFailed         = "gate-failed"
	reasonPartial            = "partial"
)

// Source keys for per-source degradation (FLT-DLV-13).
const (
	srcUpstream  = "upstream"
	srcRegistry  = "registry"
	srcGitOps    = "gitops"
	srcPromotion = "promotion"
)

// typicalMinSamples is the fewest completed samples a typical duration needs
// (FLT-DLV-09).
const typicalMinSamples = 3

// commitsReadLimit bounds the one commits-list call per refresh (FLT-DLV-12).
const commitsReadLimit = 100

// DeliveryPayload is the /api/delivery data (FLT-DLV-12).
type DeliveryPayload struct {
	Enabled bool               `json:"enabled"`
	Stages  []DeliveryStageDef `json:"stages"`
	Changes []DeliveryChange   `json:"changes"`
	// TypicalSeconds maps a stage key to its median duration. A stage with fewer
	// than three samples has no entry (FLT-DLV-09).
	TypicalSeconds map[string]int64 `json:"typicalSeconds"`
	// Omitted counts undelivered changes dropped by the window (FLT-DLV-02).
	Omitted int                             `json:"omitted"`
	Sources map[string]DeliverySourceStatus `json:"sources"`
}

// DeliveryStageDef is one column of the change table.
type DeliveryStageDef struct {
	Key string `json:"key"`
	// Kind is "pipeline" for the fixed stages and "group" for an environment group.
	Kind string `json:"kind"`
	// Group is the environment group's label value (group stages only).
	Group string `json:"group,omitempty"`
}

// DeliverySourceStatus reports one source's health (FLT-DLV-13).
type DeliverySourceStatus struct {
	Stale bool `json:"stale"`
	// Never is true when the source has never succeeded.
	Never       bool   `json:"never"`
	Error       string `json:"error,omitempty"`
	LastSuccess string `json:"lastSuccess,omitempty"`
}

// DeliveryChange is one change row.
type DeliveryChange struct {
	SHA         string             `json:"sha"`
	ShortSHA    string             `json:"shortSha"`
	MergedAt    string             `json:"mergedAt"`
	Title       string             `json:"title"`
	Author      string             `json:"author,omitempty"`
	PR          int                `json:"pr,omitempty"`
	PRURL       string             `json:"prUrl,omitempty"`
	CommitURL   string             `json:"commitUrl"`
	Delivered   bool               `json:"delivered"`
	DeliveredAt string             `json:"deliveredAt,omitempty"`
	Stages      []DeliveryStage    `json:"stages"`
	Builds      []DeliveryCheck    `json:"builds"`
	Policies    []DeliveryCheck    `json:"policies"`
	Bundle      *DeliveryBundle    `json:"bundle,omitempty"`
	GitOpsPR    *DeliveryPRLink    `json:"gitopsPr,omitempty"`
	Envs        []DeliveryEnvState `json:"environments"`
}

// DeliveryStage is one stage cell.
type DeliveryStage struct {
	Key        string `json:"key"`
	State      string `json:"state"`
	StartedAt  string `json:"startedAt,omitempty"`
	FinishedAt string `json:"finishedAt,omitempty"`
	Reason     string `json:"reason,omitempty"`
	// Components names the builds or policy checks the cell is about.
	Components []string `json:"components,omitempty"`
	// Groups names the upstream environment groups a waiting group stage waits for.
	Groups []string `json:"groups,omitempty"`
	// Gate is the promotion gate key a running or failed group stage is about.
	Gate string `json:"gate,omitempty"`
	// Done/Total count environments for a partially promoted group.
	Done  int `json:"done,omitempty"`
	Total int `json:"total,omitempty"`
	// PR and Checks describe the GitOps update pull request (gitops stage).
	PR     int    `json:"pr,omitempty"`
	Checks string `json:"checks,omitempty"`
	// FailedAttempts counts failed promotions in a Passed group (FLT-DLV-18).
	FailedAttempts int `json:"failedAttempts,omitempty"`
}

// DeliveryCheck is one component build or policy check.
type DeliveryCheck struct {
	Component   string `json:"component"`
	State       string `json:"state"`
	StartedAt   string `json:"startedAt,omitempty"`
	CompletedAt string `json:"completedAt,omitempty"`
	URL         string `json:"url,omitempty"`
}

// DeliveryBundle is a change's first bundle (FLT-DLV-06/11).
type DeliveryBundle struct {
	Tag         string `json:"tag"`
	Version     string `json:"version"`
	Digest      string `json:"digest"`
	PublishedAt string `json:"publishedAt,omitempty"`
	Locked      bool   `json:"locked"`
	// ReleaseURL is the application repository's published GitHub release; empty
	// means "Downloads not published yet".
	ReleaseURL string `json:"releaseUrl,omitempty"`
}

// DeliveryPRLink is a pull request reference.
type DeliveryPRLink struct {
	Number int    `json:"number"`
	URL    string `json:"url,omitempty"`
}

// DeliveryEnvState is one environment's state for a change (drawer, FLT-DLV-11).
type DeliveryEnvState struct {
	Env      string `json:"env"`
	Group    string `json:"group"`
	State    string `json:"state"`
	Contains bool   `json:"contains"`
	// History lists the promotions of bundles containing the change, oldest
	// first (FLT-DLV-18).
	History []DeliveryPromotion `json:"history,omitempty"`
}

// LockTimeline is the shared lock resolver's view the delivery plane reads
// (FLT-DLV-06). Implemented by GitHubLockResolver.
type LockTimeline interface {
	LastLockTimeline() ([]LockCommit, time.Time, bool)
	ResolveCached(sha string) (*Release, bool)
	Resolve(ctx context.Context, sha string) *Release
	// ResolveHydrated follows a rendered commit's hydration metadata to its bundle.
	ResolveHydrated(ctx context.Context, sha string) *Release
}

// BundleWindower is the shared bundle-membership rule (FLT-DLV-06). Implemented
// by GitHubBundles.
type BundleWindower interface {
	BundleWindows(ctx context.Context, releases []*Release) []BundleWindow
}

// SnapshotReader reads a cached plane (the promotion source).
type SnapshotReader interface {
	Get() cache.Snapshot
}

// Delivery is the delivery-plane source.
type Delivery struct {
	window       int
	checkApp     string
	upstreamRepo string
	gitopsRepo   string
	htmlBase     string
	gh           DeliveryGitHub
	registry     BundleRegistry
	lock         LockTimeline
	windower     BundleWindower
	promo        SnapshotReader
	now          func() time.Time
	logger       *slog.Logger

	mu sync.Mutex
	// Last good data per source, so a failing source serves stale (FLT-DLV-13).
	commits   []UpstreamCommit
	commitsAt time.Time
	commitErr string
	tags      []RegistryBundle
	tagsAt    time.Time
	tagErr    string
	tagsEver  bool
	pr        *BundlePR
	prErr     string
	checks    map[string][]CheckRun
	// revisions caches each pending bundle's manifests revision by digest; bundle
	// content is immutable (FLT-DLV-12).
	revisions map[string]string
	// hist caches each environment's promotion history (FLT-DLV-18). Only the
	// refresh goroutine touches it.
	hist    map[string]*envHistory
	histAt  time.Time
	histErr string
	// appName is the Konflux application name, learned from policy check names,
	// used to shorten component labels.
	appName string
}

// NewDelivery builds the delivery source, or returns nil when
// FD_DELIVERY_BUNDLE_REPOSITORY is unset (FLT-DLV-12). lock and windower may be
// nil when the GitOps repository is not configured; the GitOps-fed stages then
// read Unknown.
func NewDelivery(c *config.Config, lock LockTimeline, windower BundleWindower, promo SnapshotReader) *Delivery {
	if c.DeliveryBundleRepository == "" {
		return nil
	}
	reg := NewQuayBundleRegistry(c.DeliveryBundleRepository)
	if reg == nil {
		slog.Default().Warn("delivery plane disabled: FD_DELIVERY_BUNDLE_REPOSITORY is not host/namespace/repo")
		return nil
	}
	gh := NewGitHubDelivery(c.GitHubAPIBase, c.UpstreamRepo, c.UpstreamTokenFile, c.GitHubRepo, c.GitHubTokenFile, c.DeliveryReleasePRBranch)
	return newDelivery(c, gh, reg, lock, windower, promo)
}

func newDelivery(c *config.Config, gh DeliveryGitHub, reg BundleRegistry, lock LockTimeline, windower BundleWindower, promo SnapshotReader) *Delivery {
	window := c.DeliveryWindow
	if window <= 0 {
		window = 30
	}
	return &Delivery{
		window:       window,
		checkApp:     c.DeliveryCheckApp,
		upstreamRepo: strings.Trim(c.UpstreamRepo, "/"),
		gitopsRepo:   strings.Trim(c.GitHubRepo, "/"),
		htmlBase:     htmlBaseFrom(strings.TrimRight(c.GitHubAPIBase, "/")),
		gh:           gh,
		registry:     reg,
		lock:         lock,
		windower:     windower,
		promo:        promo,
		now:          time.Now,
		logger:       slog.Default(),
		checks:       map[string][]CheckRun{},
		revisions:    map[string]string{},
		hist:         map[string]*envHistory{},
	}
}

// DisabledDeliveryPayload is served when the plane is not configured.
func DisabledDeliveryPayload() DeliveryPayload {
	return DeliveryPayload{Enabled: false, Stages: []DeliveryStageDef{}, Changes: []DeliveryChange{}, TypicalSeconds: map[string]int64{}, Sources: map[string]DeliverySourceStatus{}}
}

// Delivery is the cache fetch function for /api/delivery. It never fails the
// plane for one source: each source degrades on its own (FLT-DLV-13).
func (d *Delivery) Delivery(ctx context.Context) (any, error) {
	now := d.now()
	sources := map[string]DeliverySourceStatus{}

	// 1. Application repository commits (one call).
	commits, err := d.gh.Commits(ctx, commitsReadLimit)
	d.mu.Lock()
	if err == nil {
		d.commits, d.commitsAt, d.commitErr = commits, now, ""
	} else {
		d.commitErr = err.Error()
	}
	commits = d.commits
	sources[srcUpstream] = status(d.commitsAt, d.commitErr)
	d.mu.Unlock()

	// 2. Locked bundles from the shared lock timeline (no call).
	locked, lockCommits, gitopsStatus := d.lockedBundles()
	sources[srcGitOps] = gitopsStatus

	// 3. Registry tags (one call) and pending bundle contents (one read per new
	// pending bundle, cached forever).
	tags, terr := d.registry.ListBundles(ctx)
	d.mu.Lock()
	if terr == nil {
		d.tags, d.tagsAt, d.tagErr, d.tagsEver = tags, now, "", true
	} else {
		d.tagErr = terr.Error()
	}
	tags = d.tags
	sources[srcRegistry] = status(d.tagsAt, d.tagErr)
	d.mu.Unlock()
	pending := d.pendingBundles(ctx, tags, locked)

	// 4. The open bundle-update pull request (one call), resolved to the bundle
	// it proposes through the shared lock resolver (cached per head SHA).
	pr, perr := d.gh.OpenBundlePR(ctx)
	d.mu.Lock()
	if perr == nil {
		d.pr, d.prErr = pr, ""
	} else {
		d.prErr = perr.Error()
	}
	pr = d.pr
	d.mu.Unlock()
	var prDigest string
	if pr != nil && d.lock != nil {
		if r := d.lock.Resolve(ctx, pr.HeadSHA); r != nil {
			prDigest = r.Digest
		}
	}

	// 5. Promotion snapshot (shared cache, no call).
	promo, promoStatus := d.promotion()
	sources[srcPromotion] = promoStatus

	// 6. Bundle placement through the shared rule.
	all := append(append([]*Release{}, locked...), pending...)
	var windows []BundleWindow
	if d.windower != nil {
		windows = d.windower.BundleWindows(ctx, all)
	}

	published := map[string]time.Time{}
	for _, t := range tags {
		if !t.PublishedAt.IsZero() {
			published[t.Tag] = t.PublishedAt
		}
	}

	in := deliveryInput{
		now:          now,
		window:       d.window,
		checkApp:     d.checkApp,
		upstreamRepo: d.upstreamRepo,
		gitopsRepo:   d.gitopsRepo,
		htmlBase:     d.htmlBase,
		commits:      commits,
		bundles:      orderBundles(all),
		windows:      windows,
		lockedSet:    digestSet(locked),
		lockCommits:  lockCommits,
		gitopsKnown:  !sources[srcGitOps].Never,
		pendingKnown: d.tagsEver,
		published:    published,
		bundlePR:     pr,
		bundlePRDig:  prDigest,
		promo:        promo,
		sources:      sources,
	}

	// 7. Select the window, then read check runs only for listed changes.
	listed, omitted := selectWindow(in)
	in.checks = d.readChecks(ctx, listed, in)
	in.history, sources[srcHistory] = d.readHistory(ctx, promo, oldestMerge(listed, now), now)
	d.learnAppName(in.checks)
	in.appName = d.appName

	payload := buildDelivery(in, listed, omitted)
	d.attachReleaseLinks(ctx, &payload)
	return payload, nil
}

func status(lastOK time.Time, errText string) DeliverySourceStatus {
	s := DeliverySourceStatus{Error: errText}
	if lastOK.IsZero() {
		s.Never = errText != ""
		s.Stale = errText != ""
		return s
	}
	s.LastSuccess = lastOK.UTC().Format(time.RFC3339)
	s.Stale = errText != ""
	return s
}

// lockedBundles reads the distinct locked bundles from the shared lock timeline.
func (d *Delivery) lockedBundles() ([]*Release, []LockCommit, DeliverySourceStatus) {
	if d.lock == nil {
		return nil, nil, DeliverySourceStatus{Never: true, Stale: true, Error: "GitOps repository not configured"}
	}
	tl, at, ok := d.lock.LastLockTimeline()
	if !ok {
		return nil, nil, DeliverySourceStatus{Never: true, Stale: true, Error: "release lock history not read yet"}
	}
	seen := map[string]bool{}
	var out []*Release
	for _, c := range tl {
		if seen[c.Digest] {
			continue
		}
		r, ok := d.lock.ResolveCached(c.SHA)
		if !ok || r.Digest == "" {
			continue
		}
		seen[c.Digest] = true
		out = append(out, r)
	}
	st := DeliverySourceStatus{LastSuccess: at.UTC().Format(time.RFC3339)}
	return out, tl, st
}

// pendingBundles returns the registry bundles newer than the newest locked
// bundle, with their manifests revision (FLT-DLV-06).
func (d *Delivery) pendingBundles(ctx context.Context, tags []RegistryBundle, locked []*Release) []*Release {
	newest := ""
	for _, r := range locked {
		if r.Tag > newest {
			newest = r.Tag
		}
	}
	if newest == "" {
		// Without a locked baseline a pending bundle has no compare base.
		return nil
	}
	var out []*Release
	for _, t := range tags {
		if t.Tag <= newest {
			continue
		}
		d.mu.Lock()
		rev := d.revisions[t.Digest]
		d.mu.Unlock()
		if rev == "" {
			var err error
			rev, err = d.registry.ManifestsRevision(ctx, t.Digest)
			if err != nil || rev == "" {
				d.logger.DebugContext(ctx, "delivery: read pending bundle", "tag", t.Tag, "err", err)
				continue
			}
			d.mu.Lock()
			d.revisions[t.Digest] = rev
			d.mu.Unlock()
		}
		out = append(out, &Release{
			Version:      versionFromTag(t.Tag),
			Tag:          t.Tag,
			Digest:       t.Digest,
			Date:         dateFromTag(t.Tag),
			ManifestsRev: rev,
		})
	}
	return out
}

func (d *Delivery) promotion() (*PromotionPayload, DeliverySourceStatus) {
	if d.promo == nil {
		return nil, DeliverySourceStatus{Never: true, Stale: true, Error: "promotion plane unavailable"}
	}
	snap := d.promo.Get()
	p, ok := snap.Data.(PromotionPayload)
	if !ok || snap.GeneratedAt.IsZero() {
		return nil, DeliverySourceStatus{Never: true, Stale: true, Error: snap.Error}
	}
	return &p, DeliverySourceStatus{Stale: snap.Stale, Error: snap.Error, LastSuccess: snap.GeneratedAt.UTC().Format(time.RFC3339)}
}

// readChecks returns the check runs for the listed changes. A change whose
// runs have settled is not read again (FLT-DLV-12).
func (d *Delivery) readChecks(ctx context.Context, listed []listedChange, in deliveryInput) map[string][]CheckRun {
	out := map[string][]CheckRun{}
	for _, l := range listed {
		sha := l.commit.SHA
		d.mu.Lock()
		prev, had := d.checks[sha]
		d.mu.Unlock()
		if had && checksSettled(prev, l.commit.MergedAt, l.delivered, in.now) {
			out[sha] = prev
			continue
		}
		runs, err := d.gh.CheckRuns(ctx, sha)
		if err != nil {
			if had {
				out[sha] = prev
			}
			continue
		}
		runs = filterApp(runs, in.checkApp)
		d.mu.Lock()
		d.checks[sha] = runs
		d.mu.Unlock()
		out[sha] = runs
	}
	// Forget commits that left the read window.
	d.mu.Lock()
	keep := map[string]bool{}
	for _, c := range d.commits {
		keep[c.SHA] = true
	}
	for sha := range d.checks {
		if !keep[sha] {
			delete(d.checks, sha)
		}
	}
	d.mu.Unlock()
	return out
}

// buildStartGrace is how long after a merge a change with no build check run
// may still get one. Konflux creates build check runs within minutes.
const buildStartGrace = time.Hour

// checksSettled reports whether a change's check runs (already filtered to the
// check app) can no longer change what the Delivery tab shows, so they need not
// be read again. It looks only at the runs the stages use: component builds and
// the policy checks of built components. A failed run is never settled, because
// Konflux can finish the same check run later with another conclusion. A
// delivered change settles once nothing failed, even with a stuck policy check.
func checksSettled(runs []CheckRun, mergedAt string, delivered bool, now time.Time) bool {
	builds, policies := classifyChecks(runs)
	built := map[string]bool{}
	for _, b := range builds {
		if checkState(b.run) == StagePassed {
			built[b.component] = true
		}
	}
	relevant := make([]CheckRun, 0, len(builds)+len(policies))
	for _, b := range builds {
		relevant = append(relevant, b.run)
	}
	for _, p := range policies {
		if built[p.component] {
			relevant = append(relevant, p.run)
		}
	}
	for _, r := range relevant {
		if checkState(r) == StageFailed {
			return false
		}
	}
	if delivered {
		return true
	}
	if len(builds) == 0 {
		// No build yet: settled only once a build would have started.
		t, err := time.Parse(time.RFC3339, mergedAt)
		return err == nil && now.Sub(t) > buildStartGrace
	}
	for _, r := range relevant {
		if r.Status != "completed" {
			return false
		}
	}
	return true
}

func filterApp(runs []CheckRun, app string) []CheckRun {
	out := make([]CheckRun, 0, len(runs))
	for _, r := range runs {
		if app == "" || r.App == app {
			out = append(out, r)
		}
	}
	return out
}

// learnAppName remembers the Konflux application name from a policy check name
// ("<prefix> / <application>-enterprise-contract / <component>").
func (d *Delivery) learnAppName(checks map[string][]CheckRun) {
	for _, runs := range checks {
		for _, r := range runs {
			parts := splitCheckName(r.Name)
			for _, p := range parts {
				if app, ok := strings.CutSuffix(p, "-enterprise-contract"); ok && app != "" {
					d.mu.Lock()
					d.appName = app
					d.mu.Unlock()
					return
				}
			}
		}
	}
}

// attachReleaseLinks fills each listed change's first bundle release link. One
// call per distinct unpublished bundle; a published release is cached forever.
func (d *Delivery) attachReleaseLinks(ctx context.Context, p *DeliveryPayload) {
	links := map[string]string{}
	for i := range p.Changes {
		b := p.Changes[i].Bundle
		if b == nil || b.Tag == "" {
			continue
		}
		u, ok := links[b.Tag]
		if !ok {
			published, url, err := d.gh.ReleasePublished(ctx, b.Tag)
			if err == nil && published {
				u = url
			}
			links[b.Tag] = u
		}
		b.ReleaseURL = u
	}
}

func digestSet(rs []*Release) map[string]bool {
	out := map[string]bool{}
	for _, r := range rs {
		out[r.Digest] = true
	}
	return out
}

// --- Pure assembly ---------------------------------------------------------

// deliveryInput is everything the pure assembly needs. Tests build it directly.
type deliveryInput struct {
	now          time.Time
	window       int
	checkApp     string
	appName      string
	upstreamRepo string
	gitopsRepo   string
	htmlBase     string
	commits      []UpstreamCommit // newest first
	// bundles is every known bundle (locked and pending), oldest first.
	bundles     []*Release
	windows     []BundleWindow
	lockedSet   map[string]bool
	lockCommits []LockCommit // newest first
	// gitopsKnown and pendingKnown are false when that source never succeeded.
	gitopsKnown  bool
	pendingKnown bool
	published    map[string]time.Time
	bundlePR     *BundlePR
	bundlePRDig  string
	promo        *PromotionPayload
	// history is each environment's promotions, oldest first (FLT-DLV-18).
	history map[string][]histPromotion
	checks  map[string][]CheckRun
	sources map[string]DeliverySourceStatus
}

// listedChange is a change chosen for the payload, with its placement.
type listedChange struct {
	commit    UpstreamCommit
	pr        int
	title     string
	first     int // index into bundles of the first bundle; -1 when none
	delivered bool
}

// group is one environment group, in promotion order.
type envGroup struct {
	name string
	envs []string
}

// groupsOf derives environment groups from the promotion payload: envs sharing a
// delivery env label value, ordered by their earliest position in the promotion
// order (FLT-DLV-03).
func groupsOf(p *PromotionPayload) []envGroup {
	if p == nil {
		return nil
	}
	idx := map[string]int{}
	var out []envGroup
	for _, key := range p.Order {
		env := p.Environments[key]
		name := env.Env
		if name == "" {
			name = key
		}
		i, ok := idx[name]
		if !ok {
			i = len(out)
			idx[name] = i
			out = append(out, envGroup{name: name})
		}
		out[i].envs = append(out[i].envs, key)
	}
	return out
}

// placement maps each change to its first bundle index.
type placement struct {
	byPR    map[int]int    // PR number -> first bundle index
	byRev   map[string]int // manifests revision -> bundle index
	digests map[string]int // digest -> bundle index
}

func placeBundles(in deliveryInput) placement {
	pl := placement{byPR: map[int]int{}, byRev: map[string]int{}, digests: map[string]int{}}
	for i, b := range in.bundles {
		pl.byRev[b.ManifestsRev] = i
		pl.digests[b.Digest] = i
	}
	for _, w := range in.windows {
		if !w.OK || w.Release == nil {
			continue
		}
		bi, ok := pl.digests[w.Release.Digest]
		if !ok {
			continue
		}
		for _, pr := range w.PRs {
			if pr.Repository != "" && pr.Repository != in.upstreamRepo {
				continue
			}
			if cur, seen := pl.byPR[pr.Number]; !seen || bi < cur {
				pl.byPR[pr.Number] = bi
			}
		}
	}
	return pl
}

// firstBundle returns the change's first bundle index, or -1. Membership comes
// from the shared compare windows by pull request number. Two cases have no
// window: a commit with no pull request trailer, and a change older than the
// oldest known bundle (which has no predecessor to compare against). Those are
// placed by position: on the linear default branch a bundle contains every
// commit at or before its manifests revision.
func firstBundle(pl placement, commits []UpstreamCommit, i int, pr int) int {
	if pr > 0 {
		if bi, ok := pl.byPR[pr]; ok {
			return bi
		}
	}
	// commits is newest first: a bundle contains commit i when its revision is at
	// index <= i.
	best := -1
	for j := i; j >= 0; j-- {
		if bi, ok := pl.byRev[commits[j].SHA]; ok && (best == -1 || bi < best) {
			best = bi
		}
	}
	return best
}

func containsIdx(pl placement, digest string, first int) bool {
	if first < 0 || digest == "" {
		return false
	}
	bi, ok := pl.digests[digest]
	return ok && bi >= first
}

// selectWindow chooses the listed changes (FLT-DLV-02): every undelivered change,
// then the newest delivered ones, up to the window. Undelivered changes beyond
// the window are dropped oldest first and counted.
func selectWindow(in deliveryInput) ([]listedChange, int) {
	pl := placeBundles(in)
	groups := groupsOf(in.promo)
	var undelivered, delivered []listedChange
	for i, c := range in.commits {
		pr, title := splitSubject(c.Message)
		lc := listedChange{commit: c, pr: pr, title: title, first: firstBundle(pl, in.commits, i, pr)}
		lc.delivered = isDelivered(in, pl, groups, lc.first)
		if lc.delivered {
			delivered = append(delivered, lc)
		} else {
			undelivered = append(undelivered, lc)
		}
	}
	omitted := 0
	if len(undelivered) > in.window {
		// Without GitOps and promotion data no change can be known delivered, so
		// the overflow count would only measure the read window: do not report it.
		if in.gitopsKnown && in.promo != nil {
			omitted = len(undelivered) - in.window
		}
		undelivered = undelivered[:in.window]
	}
	room := in.window - len(undelivered)
	if room > len(delivered) {
		room = len(delivered)
	}
	return append(undelivered, delivered[:room]...), omitted
}

func isDelivered(in deliveryInput, pl placement, groups []envGroup, first int) bool {
	if in.promo == nil || first < 0 || len(groups) == 0 {
		return false
	}
	for _, g := range groups {
		for _, key := range g.envs {
			env := in.promo.Environments[key]
			if env.Active == nil || !containsIdx(pl, env.Active.Digest, first) {
				return false
			}
		}
	}
	return true
}

// splitSubject returns the pull request number and the title without the trailer.
func splitSubject(message string) (int, string) {
	subject, _, _ := strings.Cut(message, "\n")
	if n, title, ok := extractPR(message); ok && prSuffix.MatchString(subject) {
		return n, title
	}
	return 0, strings.TrimSpace(subject)
}

func splitCheckName(name string) []string {
	parts := strings.Split(name, "/")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

// componentCheck is a classified check run.
type componentCheck struct {
	component string
	run       CheckRun
}

// classifyChecks splits check runs into component builds and policy checks
// (FLT-DLV-05). A build's name ends in "-on-push"; a policy check's name holds
// "-enterprise-contract /".
func classifyChecks(runs []CheckRun) (builds, policies []componentCheck) {
	for _, r := range runs {
		parts := splitCheckName(r.Name)
		last := parts[len(parts)-1]
		switch {
		case strings.HasSuffix(last, "-on-push"):
			builds = append(builds, componentCheck{component: strings.TrimSuffix(last, "-on-push"), run: r})
		case strings.Contains(r.Name, "-enterprise-contract /"):
			policies = append(policies, componentCheck{component: last, run: r})
		}
	}
	return builds, policies
}

// shortComponent drops the Konflux application's name prefix and suffix from a
// component name ("<a>-<x>-<b>" for application "<a>-<b>"), when it has them.
func shortComponent(component, app string) string {
	// The application name's last dash splits prefix and suffix.
	i := strings.LastIndex(app, "-")
	if i <= 0 {
		return component
	}
	pre, suf := app[:i], app[i+1:]
	s, okp := strings.CutPrefix(component, pre+"-")
	s, oks := strings.CutSuffix(s, "-"+suf)
	if okp && oks && s != "" {
		return s
	}
	return component
}

func checkState(r CheckRun) string {
	switch {
	case r.Status != "completed":
		return StageRunning
	case checkSucceeded(r.Conclusion):
		return StagePassed
	case checkIgnorable(r.Conclusion):
		return StageNotNeeded
	default:
		return StageFailed
	}
}

// aggregate folds component checks into one stage state.
func aggregate(checks []componentCheck, app string) (state string, components []string, started, finished string) {
	var running, failed []string
	passed := 0
	for _, c := range checks {
		switch checkState(c.run) {
		case StageRunning:
			running = append(running, shortComponent(c.component, app))
		case StageFailed:
			failed = append(failed, shortComponent(c.component, app))
		case StagePassed:
			passed++
		}
		if c.run.StartedAt != "" && (started == "" || c.run.StartedAt < started) {
			started = c.run.StartedAt
		}
		if c.run.CompletedAt != "" && c.run.CompletedAt > finished {
			finished = c.run.CompletedAt
		}
	}
	sort.Strings(running)
	sort.Strings(failed)
	switch {
	case len(failed) > 0:
		return StageFailed, failed, started, finished
	case len(running) > 0:
		return StageRunning, running, started, ""
	case passed > 0:
		all := make([]string, 0, len(checks))
		for _, c := range checks {
			if checkState(c.run) == StagePassed {
				all = append(all, shortComponent(c.component, app))
			}
		}
		sort.Strings(all)
		return StagePassed, all, started, finished
	default:
		return StageNotNeeded, nil, started, finished
	}
}

func toChecks(cs []componentCheck, app string) []DeliveryCheck {
	out := make([]DeliveryCheck, 0, len(cs))
	for _, c := range cs {
		out = append(out, DeliveryCheck{
			Component: shortComponent(c.component, app), State: checkState(c.run),
			StartedAt: c.run.StartedAt, CompletedAt: c.run.CompletedAt, URL: c.run.URL,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Component < out[j].Component })
	return out
}

// buildDelivery assembles the payload from resolved inputs. It is pure: the same
// input always yields the same payload.
func buildDelivery(in deliveryInput, listed []listedChange, omitted int) DeliveryPayload {
	pl := placeBundles(in)
	groups := groupsOf(in.promo)
	p := DeliveryPayload{
		Enabled:        true,
		Stages:         stageDefs(groups),
		Changes:        make([]DeliveryChange, 0, len(listed)),
		TypicalSeconds: map[string]int64{},
		Omitted:        omitted,
		Sources:        in.sources,
	}
	for _, l := range listed {
		p.Changes = append(p.Changes, buildChange(in, pl, groups, l))
	}
	p.TypicalSeconds = typicalDurations(p.Changes)
	return p
}

func stageDefs(groups []envGroup) []DeliveryStageDef {
	defs := []DeliveryStageDef{
		{Key: stageMerged, Kind: "pipeline"},
		{Key: stageBuild, Kind: "pipeline"},
		{Key: stagePolicy, Kind: "pipeline"},
		{Key: stageRelease, Kind: "pipeline"},
		{Key: stageGitOps, Kind: "pipeline"},
	}
	for _, g := range groups {
		defs = append(defs, DeliveryStageDef{Key: "env:" + g.name, Kind: "group", Group: g.name})
	}
	return defs
}

func buildChange(in deliveryInput, pl placement, groups []envGroup, l listedChange) DeliveryChange {
	c := l.commit
	ch := DeliveryChange{
		SHA:       c.SHA,
		ShortSHA:  shortSHA7(c.SHA),
		MergedAt:  c.MergedAt,
		Title:     l.title,
		Author:    c.Author,
		PR:        l.pr,
		CommitURL: in.htmlBase + "/" + in.upstreamRepo + "/commit/" + c.SHA,
		Delivered: l.delivered,
		Builds:    []DeliveryCheck{},
		Policies:  []DeliveryCheck{},
		Envs:      []DeliveryEnvState{},
	}
	if l.pr > 0 {
		ch.PRURL = in.htmlBase + "/" + in.upstreamRepo + "/pull/" + itoa(l.pr)
	}

	merged := DeliveryStage{Key: stageMerged, State: StagePassed, FinishedAt: c.MergedAt}

	// Build and policy check (FLT-DLV-05).
	runs, known := in.checks[c.SHA]
	builds, policies := classifyChecks(runs)
	var build, policy DeliveryStage
	build.Key, policy.Key = stageBuild, stagePolicy
	switch {
	case !known:
		build.State, policy.State = StageUnknown, StageUnknown
	case len(builds) == 0:
		build.State, build.Reason = StageNotNeeded, reasonNoComponentChanged
		policy.State, policy.Reason = StageNotNeeded, reasonNoComponentChanged
	default:
		st, comps, _, finished := aggregate(builds, in.appName)
		build.State, build.Components, build.FinishedAt = st, comps, finished
		build.StartedAt = c.MergedAt
		if st == StageNotNeeded {
			build.Reason = reasonBuildReplaced
		}
		// Policy checks apply only to the components built for this change.
		built := map[string]bool{}
		for _, b := range builds {
			if checkState(b.run) == StagePassed {
				built[b.component] = true
			}
		}
		var mine []componentCheck
		for _, pc := range policies {
			if built[pc.component] {
				mine = append(mine, pc)
			}
		}
		policies = mine
		switch st {
		case StagePassed:
			if len(mine) == 0 {
				policy.State, policy.Reason = StageWaiting, reasonWaitingPolicy
			} else {
				pst, pcomps, _, pfin := aggregate(mine, in.appName)
				policy.State, policy.Components, policy.FinishedAt = pst, pcomps, pfin
				if pst == StageNotNeeded {
					policy.Reason = reasonBuildReplaced
				}
			}
		case StageNotNeeded:
			policy.State, policy.Reason = StageNotNeeded, reasonBuildReplaced
		default:
			policy.State = StageNotStarted
		}
	}
	ch.Builds = toChecks(builds, in.appName)
	ch.Policies = toChecks(policies, in.appName)

	// Release (FLT-DLV-06).
	release := DeliveryStage{Key: stageRelease}
	if l.first >= 0 {
		b := in.bundles[l.first]
		pub := in.published[b.Tag]
		finished := b.Date
		if !pub.IsZero() {
			finished = pub.UTC().Format(time.RFC3339)
		}
		release.State, release.FinishedAt = StagePassed, finished
		ch.Bundle = &DeliveryBundle{Tag: b.Tag, Version: b.Version, Digest: b.Digest, PublishedAt: finished, Locked: in.lockedSet[b.Digest]}
	} else {
		switch {
		case build.State == StageFailed:
			release.State = StageNotStarted
		case !in.gitopsKnown:
			// Without the locked baseline no bundle placement is possible.
			release.State = StageUnknown
		case policy.State == StagePassed:
			if !in.pendingKnown {
				release.State = StageUnknown
			} else {
				release.State, release.Reason = StageRunning, reasonInferredRelease
			}
		case policy.State == StageNotNeeded:
			release.State, release.Reason = StageWaiting, reasonNextBundle
		case build.State == StageUnknown:
			release.State = StageUnknown
		default:
			release.State = StageNotStarted
		}
	}

	// GitOps update (FLT-DLV-07).
	gitops := DeliveryStage{Key: stageGitOps}
	switch {
	case !in.gitopsKnown:
		gitops.State = StageUnknown
	case l.first < 0:
		gitops.State = StageNotStarted
		if release.State == StageUnknown {
			gitops.State = StageUnknown
		}
	default:
		if lc, ok := firstLockCommit(in, pl, l.first); ok {
			gitops.State, gitops.FinishedAt = StagePassed, lc.Date
			if n, _, ok := extractPR(lc.Message); ok {
				gitops.PR = n
				ch.GitOpsPR = &DeliveryPRLink{Number: n, URL: in.htmlBase + "/" + in.gitopsRepo + "/pull/" + itoa(n)}
			}
		} else if in.bundlePR != nil && containsIdx(pl, in.bundlePRDig, l.first) {
			gitops.State, gitops.PR, gitops.Checks = StageRunning, in.bundlePR.Number, in.bundlePR.Checks
			ch.GitOpsPR = &DeliveryPRLink{Number: in.bundlePR.Number, URL: in.bundlePR.URL}
		} else {
			gitops.State, gitops.Reason = StageWaiting, reasonWaitingBundlePR
		}
	}

	stages := []DeliveryStage{merged, build, policy, release, gitops}

	// Environment groups (FLT-DLV-08).
	var upstreamPending []string
	for _, g := range groups {
		st, envs := groupStage(in, pl, g, l.first, gitops.State, upstreamPending)
		stages = append(stages, st)
		ch.Envs = append(ch.Envs, envs...)
		if st.State != StagePassed {
			upstreamPending = append(upstreamPending, g.name)
		}
	}
	// Each stage starts when the closest earlier stage with an end finished.
	prevEnd := ""
	for i := range stages {
		s := &stages[i]
		if s.StartedAt == "" && prevEnd != "" && (s.State == StageRunning || s.State == StageWaiting || s.State == StagePassed || s.State == StageFailed) {
			s.StartedAt = prevEnd
		}
		if s.FinishedAt != "" {
			prevEnd = s.FinishedAt
		}
	}
	ch.Stages = stages

	if ch.Delivered {
		last := ""
		for _, s := range stages[5:] {
			if s.FinishedAt > last {
				last = s.FinishedAt
			}
		}
		ch.DeliveredAt = last
	}
	return ch
}

// firstLockCommit returns the oldest lock commit on GitOps main whose bundle
// contains the change (FLT-DLV-07).
func firstLockCommit(in deliveryInput, pl placement, first int) (LockCommit, bool) {
	for i := len(in.lockCommits) - 1; i >= 0; i-- {
		lc := in.lockCommits[i]
		if containsIdx(pl, lc.Digest, first) {
			return lc, true
		}
	}
	return LockCommit{}, false
}

// groupStage computes one environment group's stage and per-environment states.
func groupStage(in deliveryInput, pl placement, g envGroup, first int, gitopsState string, upstreamPending []string) (DeliveryStage, []DeliveryEnvState) {
	st := DeliveryStage{Key: "env:" + g.name}
	envs := make([]DeliveryEnvState, 0, len(g.envs))
	// Without promotion data, or without the GitOps lock timeline that places
	// the change in a bundle, no environment state can be known (FLT-DLV-13).
	if in.promo == nil || (first < 0 && gitopsState == StageUnknown) {
		st.State = StageUnknown
		for _, key := range g.envs {
			envs = append(envs, DeliveryEnvState{Env: key, Group: g.name, State: StageUnknown})
		}
		return st, envs
	}
	done := 0
	var gateRunning, gateFailed, latest string
	for _, key := range g.envs {
		env := in.promo.Environments[key]
		contains := env.Active != nil && containsIdx(pl, env.Active.Digest, first)
		es := DeliveryEnvState{Env: key, Group: g.name, Contains: contains, History: envAttempts(in, pl, key, first)}
		if contains {
			done++
			es.State = StagePassed
			// Once a promotion containing the change passed here, the change has
			// passed this environment. A newer bundle's checks belong to the
			// newer changes' rows (FLT-DLV-08).
			gs, gk := StagePassed, ""
			if !passedBefore(es.History) {
				gs, gk = gatePhase(env.ActiveGates)
			}
			switch gs {
			case StageFailed:
				es.State, gateFailed = StageFailed, gk
			case StageRunning:
				es.State = StageRunning
				if gateRunning == "" {
					gateRunning = gk
				}
			}
			// The group's time is when its last environment first got the change.
			if len(es.History) > 0 && es.History[0].PromotedAt > latest {
				latest = es.History[0].PromotedAt
			}
		} else {
			es.State = StageWaiting
			if env.Proposed != nil && containsIdx(pl, env.Proposed.Digest, first) {
				if gs, gk := gatePhase(env.ProposedGates); gs == StageFailed {
					es.State, gateFailed = StageFailed, gk
				} else if gs == StageRunning && gateRunning == "" {
					gateRunning = gk
				}
			}
		}
		envs = append(envs, es)
	}
	total := len(g.envs)
	switch {
	case gitopsState != StagePassed && done == 0:
		st.State = StageNotStarted
		if gitopsState == StageUnknown {
			st.State = StageUnknown
		}
	case gateFailed != "":
		st.State, st.Reason, st.Gate = StageFailed, reasonGateFailed, gateFailed
	case done == total && gateRunning != "":
		st.State, st.Reason, st.Gate = StageRunning, reasonGateRunning, gateRunning
	case done == total:
		st.State, st.FinishedAt = StagePassed, latest
		st.FailedAttempts = failedAttempts(envs)
	case done > 0:
		st.State, st.Reason, st.Done, st.Total = StageRunning, reasonPartial, done, total
	default:
		st.State = StageWaiting
		hydrated := true
		for _, key := range g.envs {
			env := in.promo.Environments[key]
			if env.Proposed == nil || !containsIdx(pl, env.Proposed.Digest, first) {
				hydrated = false
			}
		}
		switch {
		case !hydrated:
			st.Reason = reasonWaitingHydration
		case len(upstreamPending) > 0:
			st.Reason, st.Groups = reasonWaitingGroups, append([]string(nil), upstreamPending...)
		case gateRunning != "":
			st.Reason, st.Gate = reasonGateRunning, gateRunning
		}
	}
	return st, envs
}

// passedBefore reports whether an environment's history for the change ends
// in a passing promotion (FLT-DLV-08, FLT-DLV-18).
func passedBefore(history []DeliveryPromotion) bool {
	return len(history) > 0 && history[len(history)-1].Outcome == OutcomePassed
}

// gatePhase folds promoter gates: failed wins over running; the gate key names
// the first gate in that state.
func gatePhase(gates []Gate) (string, string) {
	running := ""
	for _, g := range gates {
		switch strings.ToLower(g.Phase) {
		case "failure", "failed", "error":
			return StageFailed, g.Key
		case "pending", "", "running", "in_progress":
			if running == "" {
				running = g.Key
			}
		}
	}
	if running != "" {
		return StageRunning, running
	}
	return StagePassed, ""
}

// oldestMerge is the merge time of the oldest listed change: the history read
// reaches back that far (FLT-DLV-18).
func oldestMerge(listed []listedChange, now time.Time) time.Time {
	oldest := now
	for _, l := range listed {
		if t, err := time.Parse(time.RFC3339, l.commit.MergedAt); err == nil && t.Before(oldest) {
			oldest = t
		}
	}
	return oldest
}

// typicalDurations is the median duration per stage over completed stages
// (FLT-DLV-09). A stage's duration runs from its start (the end of the previous
// stage) to its end.
func typicalDurations(changes []DeliveryChange) map[string]int64 {
	samples := map[string][]int64{}
	for _, c := range changes {
		for _, s := range c.Stages {
			if s.State != StagePassed || s.StartedAt == "" || s.FinishedAt == "" || s.Key == stageMerged {
				continue
			}
			a, err1 := time.Parse(time.RFC3339, s.StartedAt)
			b, err2 := time.Parse(time.RFC3339, s.FinishedAt)
			if err1 != nil || err2 != nil || b.Before(a) {
				continue
			}
			samples[s.Key] = append(samples[s.Key], int64(b.Sub(a).Seconds()))
		}
	}
	out := map[string]int64{}
	for k, v := range samples {
		if len(v) < typicalMinSamples {
			continue
		}
		out[k] = median(v)
	}
	return out
}

func median(v []int64) int64 {
	s := append([]int64(nil), v...)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	n := len(s)
	if n%2 == 1 {
		return s[n/2]
	}
	return (s[n/2-1] + s[n/2]) / 2
}

func shortSHA7(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

func itoa(n int) string { return strconv.Itoa(n) }
