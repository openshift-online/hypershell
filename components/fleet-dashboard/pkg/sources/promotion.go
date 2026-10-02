package sources

import (
	"context"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"github.com/openshift-online/hypershell/components/fleet-dashboard/pkg/config"
)

const deliveryLabelPrefix = "delivery.hypershell.app/"

// Promotion reads GitOps Promoter CRDs and Argo Applications and derives the
// promotion payload. It is a faithful Go port of the prototype's
// dump-promotion.py. No fleet structure is assumed: the environment order, the
// env→instance mapping, and gate governance are all read from the live objects.
type Promotion struct {
	dyn       dynamic.Interface
	ns        string
	strategy  string
	argoNS    []string
	argoBase  string
	psGVR     schema.GroupVersionResource
	ctpGVR    schema.GroupVersionResource
	appGVR    schema.GroupVersionResource
	versioner VersionResolver
	analyzer  AnalysisResolver
	bundler   BundleEnricher
	history   ReleaseHistory
	// historyLimit caps the total number of release bundles in the payload (the
	// currently-deployed ones plus previously-deployed history cards). 0 disables
	// history entirely.
	historyLimit int
}

// ReleaseHistory lists the most recent DISTINCT release bundles the fleet has
// deployed, newest first, so previously-deployed bundles still render as history
// cards once no environment runs them. nil disables the feature.
type ReleaseHistory interface {
	Recent(ctx context.Context, limit int) []*Release
}

// VersionResolver maps a gitops commit SHA to a release version. Implementations
// may consult a local git mirror (spec §4/§5.1); the default degrades to the
// short SHA so the dashboard never blocks on git availability.
type VersionResolver interface {
	Resolve(ctx context.Context, sha string) *Release
}

// Release describes a resolved release bundle.
type Release struct {
	Version string `json:"version"`
	Tag     string `json:"tag,omitempty"`
	Date    string `json:"date,omitempty"`
	SHA     string `json:"sha,omitempty"`
	// Digest is the release bundle's stable identity (the versions lock's
	// reference.digest), when a lock resolver supplied it. It, not the gitops
	// SHA, is how environments on the same bundle are collapsed: two dry SHAs
	// that render the same bundle share one Release. Empty under the short-SHA
	// fallback, where SHA is the only identity.
	Digest string `json:"digest,omitempty"`
	// PRs is the set of pull requests this build introduced since the previous
	// build (Bundle tab). Populated by a BundleEnricher; nil when GitHub
	// enrichment is disabled or there is no prior build to diff against.
	PRs []PR `json:"prs,omitempty"`
}

// ShortSHAResolver is the default resolver: version == first 8 chars of the SHA.
type ShortSHAResolver struct{}

// Resolve implements VersionResolver.
func (ShortSHAResolver) Resolve(_ context.Context, sha string) *Release {
	if sha == "" {
		return nil
	}
	v := sha
	if len(v) > 8 {
		v = v[:8]
	}
	return &Release{Version: v, SHA: shortSHA(sha)}
}

// NewPromotion builds a Promotion source from config. A nil versioner defaults
// to ShortSHAResolver; a nil analyzer defaults to the no-op resolver (GitHub
// enrichment disabled), so analysisUrl is simply omitted; a nil bundler defaults
// to the no-op enricher, so release dates/PRs are simply omitted.
func NewPromotion(c *config.Config, dyn dynamic.Interface, versioner VersionResolver, analyzer AnalysisResolver, bundler BundleEnricher, history ReleaseHistory) *Promotion {
	if versioner == nil {
		versioner = ShortSHAResolver{}
	}
	if analyzer == nil {
		analyzer = noopAnalysis{}
	}
	if bundler == nil {
		bundler = noopBundles{}
	}
	return &Promotion{
		dyn:          dyn,
		ns:           c.PromoterNamespace,
		strategy:     c.PromotionStrategyName,
		argoNS:       c.ArgoNamespaces,
		argoBase:     c.ArgoBaseURL,
		psGVR:        schema.GroupVersionResource{Group: c.PromoterGroup, Version: c.PromoterVersion, Resource: "promotionstrategies"},
		ctpGVR:       schema.GroupVersionResource{Group: c.PromoterGroup, Version: c.PromoterVersion, Resource: "changetransferpolicies"},
		appGVR:       schema.GroupVersionResource{Group: c.ArgoGroup, Version: c.ArgoVersion, Resource: "applications"},
		versioner:    versioner,
		analyzer:     analyzer,
		bundler:      bundler,
		history:      history,
		historyLimit: c.ReleaseHistoryLimit,
	}
}

// PromotionPayload is the /api/promotion data (shape compatible with the
// prototype's promotion.json).
type PromotionPayload struct {
	Order        []string               `json:"order"`
	Environments map[string]Environment `json:"environments"`
	Releases     map[string]*Release    `json:"releases"`
	Frontier     *Release               `json:"frontier"`
}

// Environment is one promotion environment.
type Environment struct {
	Branch         string   `json:"branch"`
	Active         *Release `json:"active"`
	Proposed       *Release `json:"proposed"`
	ActiveHydrated string   `json:"activeHydrated"`
	// AnalysisURL links to the hypershell-analysis GitHub check-run for the
	// active hydrated commit (nullable -- spec §5). Empty/omitted when GitHub
	// enrichment is disabled or the run is absent/unreachable.
	AnalysisURL   string `json:"analysisUrl,omitempty"`
	UpToDate      bool   `json:"upToDate"`
	PRState       string `json:"prState,omitempty"`
	PRURL         string `json:"prUrl,omitempty"`
	ActiveGates   []Gate `json:"activeGates"`
	ProposedGates []Gate `json:"proposedGates"`
	// Argo + delivery labels (present when a matching Application is found).
	ArgoApp string `json:"argoApp,omitempty"`
	ArgoNS  string `json:"argoNs,omitempty"`
	// ArgoURL deep-links to the Application tree in the Argo CD UI, where the
	// env's analysis AnalysisRun and the Jobs/Pods it spawns surface (so their
	// logs are viewable without piping). Emitted only when a matching Application
	// is found AND an Argo base URL is configured (firewall - see config).
	ArgoURL    string `json:"argoUrl,omitempty"`
	ArgoHealth string `json:"argoHealth,omitempty"`
	ArgoSync   string `json:"argoSync,omitempty"`
	Role       string `json:"role,omitempty"`
	Provider   string `json:"provider,omitempty"`
	Env        string `json:"env,omitempty"`
	Cluster    string `json:"cluster,omitempty"`
	ConsoleURL string `json:"consoleUrl,omitempty"`
}

// Gate is a promoter commit status.
type Gate struct {
	Key   string `json:"key"`
	Phase string `json:"phase"`
}

// Promotion is the fetch function for the /api/promotion data plane.
func (p *Promotion) Promotion(ctx context.Context) (any, error) {
	ps, err := p.dyn.Resource(p.psGVR).Namespace(p.ns).Get(ctx, p.strategy, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get promotionstrategy %q: %w", p.strategy, err)
	}

	prByBranch := p.pullRequestsByBranch(ctx)
	argoByKey := p.argoByInstance(ctx)

	payload := PromotionPayload{
		Environments: map[string]Environment{},
		Releases:     map[string]*Release{},
	}

	envs, _, _ := nestedSlice(ps.Object, "status", "environments")
	for _, e := range envs {
		em, ok := e.(map[string]any)
		if !ok {
			continue
		}
		branch, _ := nestedString(em, "branch")
		key := strings.TrimPrefix(branch, "env/")
		payload.Order = append(payload.Order, key)

		active, _ := em["active"].(map[string]any)
		proposed, _ := em["proposed"].(map[string]any)
		activeSHA := drySHA(active)
		proposedSHA := drySHA(proposed)

		// Resolve releases through the map so every environment that shares a SHA
		// points at the SAME *Release. The bundler enriches the map in place
		// (dates + PRs); sharing the pointer means env.active/proposed see that
		// enrichment too, with no second pass.
		av := p.canonicalRelease(ctx, activeSHA, payload.Releases)
		pv := p.canonicalRelease(ctx, proposedSHA, payload.Releases)

		// analysisUrl is keyed on the active HYDRATED sha (the promoted,
		// rendered commit the analysis check-run actually ran against), matching
		// the prototype's analysis_checkrun_url(hydrated_of(act)).
		activeHydrated := hydratedSHA(active)
		env := Environment{
			Branch:         branch,
			Active:         av,
			Proposed:       pv,
			ActiveHydrated: shortSHA(activeHydrated),
			AnalysisURL:    p.analyzer.AnalysisURL(ctx, activeHydrated),
			// Up-to-date means active and proposed run the SAME release bundle, by
			// the digest identity the release cards dedup on -- NOT by raw gitops
			// dry SHA. Two dry commits that render the same bundle (e.g. a delivery
			// -plumbing commit) share a digest, so the env is up to date even though
			// the SHAs differ. Falls back to SHA identity when the lock is
			// unreadable (releaseKey), matching canonicalRelease.
			UpToDate:      sameRelease(av, pv),
			ActiveGates:   gatesOf(active),
			ProposedGates: gatesOf(proposed),
		}
		if pr, ok := prByBranch[branch]; ok {
			env.PRState = pr.state
			env.PRURL = pr.url
		}
		if a, ok := argoByKey[key]; ok {
			env.ArgoApp = a.app
			env.ArgoNS = a.ns
			env.ArgoURL = p.argoAppURL(a.ns, a.app)
			env.ArgoHealth = a.health
			env.ArgoSync = a.sync
			env.Role = a.role
			env.Provider = a.provider
			env.Env = a.env
			env.Cluster = a.cluster
			env.ConsoleURL = a.consoleURL
		}
		payload.Environments[key] = env
	}

	// Enrich release bundles (dates + PR lists) best-effort before frontier
	// selection, which depends on the date the enricher fills in.
	p.bundler.Enrich(ctx, payload.Releases)

	// Frontier = newest release by bundle date across the DEPLOYED envs (computed
	// before history is merged, so a dimmed previously-deployed card never becomes
	// the frontier).
	for _, r := range payload.Releases {
		if r.Date == "" {
			continue
		}
		if payload.Frontier == nil || r.Date > payload.Frontier.Date {
			payload.Frontier = r
		}
	}

	// Merge previously-deployed bundles so the freight bar shows release history,
	// not just what is live right now. These arrive with no environment pointing
	// at them, so the UI renders them as dimmed, zero-deployment cards.
	p.mergeHistory(ctx, &payload)
	return payload, nil
}

// mergeHistory adds previously-deployed release bundles to payload.Releases so the
// timeline shows history. It is a no-op when the history source is disabled.
//
// Only bundles no newer than the frontier are added: a lock commit on the default
// branch that is newer than every deployed env is a bundle not yet promoted
// anywhere (a pending/future release), which is not "previously deployed" and
// would otherwise appear as a stray dimmed card ahead of the fleet. Currently
// deployed bundles are already in the map (added by canonicalRelease) and keep
// their shared pointer. The total distinct bundle count is capped at historyLimit.
func (p *Promotion) mergeHistory(ctx context.Context, payload *PromotionPayload) {
	if p.history == nil || p.historyLimit <= 0 {
		return
	}
	// Ceiling for "previously deployed": the newest bundle any env actually runs.
	// Empty when nothing resolved to a dated bundle, in which case we add nothing
	// rather than guess which history entries were deployed.
	frontierDate := ""
	if payload.Frontier != nil {
		frontierDate = payload.Frontier.Date
	}
	if frontierDate == "" {
		return
	}
	for _, r := range p.history.Recent(ctx, p.historyLimit) {
		if len(payload.Releases) >= p.historyLimit {
			break
		}
		if r.Date == "" || r.Date > frontierDate {
			continue // undated or not-yet-deployed-anywhere (ahead of the fleet)
		}
		key := releaseKey(r)
		if _, ok := payload.Releases[key]; ok {
			continue // already present as a currently-deployed bundle
		}
		payload.Releases[key] = r
	}
}

// canonicalRelease resolves sha to a release, deduplicating into the shared map
// by release identity: the bundle digest when the resolver supplied one, else
// the SHA. Keying on the digest collapses two gitops SHAs that render the same
// bundle onto one shared *Release (and one map entry, which the UI consumes as
// releaseByDigest), so an env is only "behind" when it runs a genuinely older
// bundle. The first resolution for an identity wins; later callers get that same
// pointer. Returns nil for an empty/unresolvable SHA.
func (p *Promotion) canonicalRelease(ctx context.Context, sha string, releases map[string]*Release) *Release {
	r := p.versioner.Resolve(ctx, sha)
	if r == nil {
		return nil
	}
	key := releaseKey(r)
	if existing, ok := releases[key]; ok {
		return existing
	}
	releases[key] = r
	return r
}

// releaseKey is a release's identity for deduplication and for the wire
// `releases` map the UI keys by digest: the bundle digest when present, else the
// SHA (short-SHA fallback, where the gitops commit is the only identity).
func releaseKey(r *Release) string {
	if r.Digest != "" {
		return r.Digest
	}
	return r.SHA
}

// sameRelease reports whether active and proposed resolve to the same release
// bundle, by the same identity canonicalRelease dedups on (digest, else SHA).
// Two nil releases (both unresolvable) are not "the same" -- an env with no
// resolvable active bundle is not meaningfully up to date.
func sameRelease(active, proposed *Release) bool {
	if active == nil || proposed == nil {
		return false
	}
	return releaseKey(active) == releaseKey(proposed)
}

type prInfo struct{ state, url string }

func (p *Promotion) pullRequestsByBranch(ctx context.Context) map[string]prInfo {
	out := map[string]prInfo{}
	list, err := p.dyn.Resource(p.ctpGVR).Namespace(p.ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return out
	}
	for _, it := range list.Items {
		obj := it.Object
		branch, _ := nestedString(obj, "spec", "activeBranch")
		if branch == "" {
			branch, _ = nestedString(obj, "spec", "proposedBranch")
		}
		if branch == "" {
			continue
		}
		state, _ := nestedString(obj, "status", "pullRequest", "state")
		url, _ := nestedString(obj, "status", "pullRequest", "url")
		out[branch] = prInfo{state: state, url: url}
	}
	return out
}

// argoResourceFilter narrows the Application tree to the resources that carry
// the analysis e2e logs: the AnalysisRun and the Pods its Jobs spawn. The run is
// named "<instance>-release-<revision>", so its children match the name globs
// *hyp*-release* (the whole analysis family) and *e2e* (the e2e metric's Job and
// Pods specifically), filtering out the env's unrelated workload pods.
//
// These are generic Kubernetes/Argo Rollouts kinds plus wildcard name globs, not
// fleet-identifying values: the *hyp* glob carries no concrete instance number
// (the firewall bans hyp<N>, not the product prefix), so it is safe to compile in
// (data-architecture firewall). The value is the pre-encoded form of
// ?resource=kind:AnalysisRun,kind:Pod,name:*e2e*,name:*hyp*-release*.
const argoResourceFilter = "?resource=kind%3AAnalysisRun%2Ckind%3APod%2Cname%3A*e2e*%2Cname%3A*hyp*-release*"

// argoAppURL builds a deep link to an Argo CD Application tree. Modern Argo CD
// routes Applications as /applications/<appNamespace>/<appName>. A resource
// filter is appended so the tree lands pre-scoped to the analysis run and its
// pods. Returns "" when no base URL is configured (firewall) or the app is
// unknown, so the field is simply omitted.
func (p *Promotion) argoAppURL(ns, app string) string {
	if p.argoBase == "" || app == "" {
		return ""
	}
	if ns == "" {
		return fmt.Sprintf("%s/applications/%s%s", p.argoBase, app, argoResourceFilter)
	}
	return fmt.Sprintf("%s/applications/%s/%s%s", p.argoBase, ns, app, argoResourceFilter)
}

type argoInfo struct {
	app, ns, health, sync, role, provider, env, cluster, consoleURL string
}

// argoByInstance maps each instance key to its core Argo Application, using the
// prototype's rule: the Application is named "<namespace>-<instance>" and lives
// in "<namespace>", so name == namespace+"-"+key. This excludes per-instance
// keycloak/managed-db and cluster-scoped apps.
func (p *Promotion) argoByInstance(ctx context.Context) map[string]argoInfo {
	out := map[string]argoInfo{}
	var items []metav1Unstructured
	if len(p.argoNS) == 0 {
		list, err := p.dyn.Resource(p.appGVR).List(ctx, metav1.ListOptions{})
		if err != nil {
			return out
		}
		for i := range list.Items {
			items = append(items, metav1Unstructured{list.Items[i].Object})
		}
	} else {
		for _, ns := range p.argoNS {
			list, err := p.dyn.Resource(p.appGVR).Namespace(ns).List(ctx, metav1.ListOptions{})
			if err != nil {
				continue
			}
			for i := range list.Items {
				items = append(items, metav1Unstructured{list.Items[i].Object})
			}
		}
	}
	for _, it := range items {
		obj := it.obj
		name, _ := nestedString(obj, "metadata", "name")
		ns, _ := nestedString(obj, "metadata", "namespace")
		if ns == "" || !strings.HasPrefix(name, ns+"-") {
			continue
		}
		key := name[len(ns)+1:]
		labels := stringMap(obj, "metadata", "labels")
		ann := stringMap(obj, "metadata", "annotations")
		health, _ := nestedString(obj, "status", "health", "status")
		sync, _ := nestedString(obj, "status", "sync", "status")
		out[key] = argoInfo{
			app:        name,
			ns:         ns,
			health:     health,
			sync:       sync,
			role:       labels[deliveryLabelPrefix+"role"],
			provider:   labels[deliveryLabelPrefix+"provider"],
			env:        labels[deliveryLabelPrefix+"env"],
			cluster:    labels[deliveryLabelPrefix+"cluster"],
			consoleURL: ann["link.argocd.argoproj.io/external-link"],
		}
	}
	return out
}

type metav1Unstructured struct{ obj map[string]any }

func drySHA(node map[string]any) string {
	s, _ := nestedString(node, "dry", "sha")
	return s
}

func hydratedSHA(node map[string]any) string {
	s, _ := nestedString(node, "hydrated", "sha")
	return s
}

func gatesOf(node map[string]any) []Gate {
	var gates []Gate
	cs, ok, _ := nestedSlice(node, "commitStatuses")
	if !ok {
		return gates
	}
	for _, c := range cs {
		cm, ok := c.(map[string]any)
		if !ok {
			continue
		}
		key, _ := cm["key"].(string)
		phase, _ := cm["phase"].(string)
		gates = append(gates, Gate{Key: key, Phase: phase})
	}
	return gates
}

func shortSHA(s string) string {
	if len(s) > 10 {
		return s[:10]
	}
	return s
}
