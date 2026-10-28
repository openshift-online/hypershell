package sources

import (
	"context"
	"errors"
	"fmt"
	"sort"
)

// Control-plane info gauges (OBS-CPL-09). Each control plane pushes them over
// OTLP; the collector tags every series with the controller's namespace
// (k8s_namespace_name) and remote-write adds the physical cluster label.
const (
	capabilityInfoMetric   = "hypershell_managed_cluster_capability_info"
	controlPlaneInfoMetric = "hypershell_control_plane_info"
	// identityNamespaceLabel is the controller's own namespace. On the dashboard
	// side it equals the Argo Application's destination namespace, which is how a
	// metric series finds its card.
	identityNamespaceLabel = "k8s_namespace_name"
)

// ClusterIdentity is what one control plane reports about itself.
type ClusterIdentity struct {
	// Cluster is the physical cluster label on the series ("" when absent).
	Cluster string
	// Capabilities is the normalized capability set (normalizeCapabilities).
	Capabilities []string
	// GatewayImage is the default gateway image ("" when not reported).
	GatewayImage string
}

// ClusterIdentities reads both info gauges and groups them by controller
// namespace. max by collapses duplicate series from multiple controller
// replicas. Each query is best-effort: one failing still returns the other's
// data; the error is returned only when both fail.
func (p *Prometheus) ClusterIdentities(ctx context.Context) (map[string][]ClusterIdentity, error) {
	type key struct{ ns, cluster string }
	caps := map[key][]string{}
	images := map[key][]string{}

	capRes, capErr := p.instant(ctx, fmt.Sprintf("max by (%s,%s,capability) (%s)",
		identityNamespaceLabel, p.clusterLabel, capabilityInfoMetric))
	for _, r := range capRes {
		k := key{r.Metric[identityNamespaceLabel], r.Metric[p.clusterLabel]}
		if k.ns != "" {
			caps[k] = append(caps[k], r.Metric["capability"])
		}
	}
	imgRes, imgErr := p.instant(ctx, fmt.Sprintf("max by (%s,%s,gateway_image) (%s)",
		identityNamespaceLabel, p.clusterLabel, controlPlaneInfoMetric))
	for _, r := range imgRes {
		k := key{r.Metric[identityNamespaceLabel], r.Metric[p.clusterLabel]}
		if img := r.Metric["gateway_image"]; k.ns != "" && img != "" {
			images[k] = append(images[k], img)
		}
	}
	if capErr != nil && imgErr != nil {
		return nil, fmt.Errorf("cluster identity metrics: %w", errors.Join(capErr, imgErr))
	}
	if err := errors.Join(capErr, imgErr); err != nil {
		p.logger.WarnContext(ctx, "cluster identity metrics partially unavailable", "error", err.Error())
	}

	keys := map[key]bool{}
	for k := range caps {
		keys[k] = true
	}
	for k := range images {
		keys[k] = true
	}
	out := map[string][]ClusterIdentity{}
	for k := range keys {
		id := ClusterIdentity{Cluster: k.cluster, Capabilities: normalizeCapabilities(caps[k])}
		// Two images for one controller only happen briefly during a rollout,
		// while the old pod's series has not expired yet. Pick deterministically.
		if imgs := images[k]; len(imgs) > 0 {
			sort.Strings(imgs)
			id.GatewayImage = imgs[0]
		}
		out[k.ns] = append(out[k.ns], id)
	}
	for ns := range out {
		sort.Slice(out[ns], func(i, j int) bool { return out[ns][i].Cluster < out[ns][j].Cluster })
	}
	return out, nil
}

// identityFor picks the identity for a card. The namespace must match. When
// only one cluster reports that namespace it is used; when several do, the one
// whose cluster label matches the card's delivery cluster wins, and no match
// means no identity (never guess between clusters).
func identityFor(ids map[string][]ClusterIdentity, namespace, cluster string) (ClusterIdentity, bool) {
	cands := ids[namespace]
	if namespace == "" || len(cands) == 0 {
		return ClusterIdentity{}, false
	}
	if len(cands) == 1 {
		return cands[0], true
	}
	for _, c := range cands {
		if cluster != "" && c.Cluster == cluster {
			return c, true
		}
	}
	return ClusterIdentity{}, false
}
