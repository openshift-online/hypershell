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
	psGVR     schema.GroupVersionResource
	ctpGVR    schema.GroupVersionResource
	appGVR    schema.GroupVersionResource
	versioner VersionResolver
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

// NewPromotion builds a Promotion source from config.
func NewPromotion(c *config.Config, dyn dynamic.Interface, versioner VersionResolver) *Promotion {
	if versioner == nil {
		versioner = ShortSHAResolver{}
	}
	return &Promotion{
		dyn:       dyn,
		ns:        c.PromoterNamespace,
		strategy:  c.PromotionStrategyName,
		argoNS:    c.ArgoNamespaces,
		psGVR:     schema.GroupVersionResource{Group: c.PromoterGroup, Version: c.PromoterVersion, Resource: "promotionstrategies"},
		ctpGVR:    schema.GroupVersionResource{Group: c.PromoterGroup, Version: c.PromoterVersion, Resource: "changetransferpolicies"},
		appGVR:    schema.GroupVersionResource{Group: c.ArgoGroup, Version: c.ArgoVersion, Resource: "applications"},
		versioner: versioner,
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
	UpToDate       bool     `json:"upToDate"`
	PRState        string   `json:"prState,omitempty"`
	PRURL          string   `json:"prUrl,omitempty"`
	ActiveGates    []Gate   `json:"activeGates"`
	ProposedGates  []Gate   `json:"proposedGates"`
	// Argo + delivery labels (present when a matching Application is found).
	ArgoApp    string `json:"argoApp,omitempty"`
	ArgoNS     string `json:"argoNs,omitempty"`
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

		av := p.versioner.Resolve(ctx, activeSHA)
		pv := p.versioner.Resolve(ctx, proposedSHA)
		if av != nil {
			payload.Releases[av.SHA] = av
		}
		if pv != nil {
			payload.Releases[pv.SHA] = pv
		}

		env := Environment{
			Branch:         branch,
			Active:         av,
			Proposed:       pv,
			ActiveHydrated: shortSHA(hydratedSHA(active)),
			UpToDate:       activeSHA == proposedSHA && activeSHA != "",
			ActiveGates:    gatesOf(active),
			ProposedGates:  gatesOf(proposed),
		}
		if pr, ok := prByBranch[branch]; ok {
			env.PRState = pr.state
			env.PRURL = pr.url
		}
		if a, ok := argoByKey[key]; ok {
			env.ArgoApp = a.app
			env.ArgoNS = a.ns
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

	// Frontier = newest release by bundle date across all envs.
	for _, r := range payload.Releases {
		if r.Date == "" {
			continue
		}
		if payload.Frontier == nil || r.Date > payload.Frontier.Date {
			payload.Frontier = r
		}
	}
	return payload, nil
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
