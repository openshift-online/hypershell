package sources

import (
	"context"
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/openshift-online/hypershell/components/fleet-dashboard/pkg/config"
)

// Topology reads per-instance topology from labelled ConfigMaps. The
// data-architecture spec (§3.2) treats topology.json/clone.json as data that
// exists for every instance but may be stale or missing; this source therefore
// surfaces what it finds without assuming any particular instance is present.
type Topology struct {
	cs       kubernetes.Interface
	ns       string // empty => all namespaces
	selector string
}

// NewTopology builds a Topology source from config.
func NewTopology(c *config.Config, cs kubernetes.Interface) *Topology {
	return &Topology{cs: cs, ns: c.TopologyNamespace, selector: c.TopologyLabelSelector}
}

// InstanceTopology is one instance's decoded topology plus provenance so the UI
// can flag staleness (§3.2, §5.2).
type InstanceTopology struct {
	Instance    string          `json:"instance"`
	Namespace   string          `json:"namespace"`
	ConfigMap   string          `json:"configMap"`
	ResourceVer string          `json:"resourceVersion,omitempty"`
	Topology    json.RawMessage `json:"topology,omitempty"`
	Clone       json.RawMessage `json:"clone,omitempty"`
	ParseError  string          `json:"parseError,omitempty"`
}

// Topology is the fetch function for the /api/topology data plane.
func (t *Topology) Topology(ctx context.Context) (any, error) {
	cms, err := t.cs.CoreV1().ConfigMaps(t.ns).List(ctx, metav1.ListOptions{LabelSelector: t.selector})
	if err != nil {
		return nil, fmt.Errorf("list topology configmaps: %w", err)
	}
	out := map[string]InstanceTopology{}
	for i := range cms.Items {
		cm := &cms.Items[i]
		it := decodeTopologyConfigMap(cm)
		out[it.Instance] = it
	}
	return out, nil
}

// decodeTopologyConfigMap pulls the instance key from the delivery label schema
// (product-level, already public) and decodes the two JSON documents, recording
// any parse error instead of failing the whole plane.
func decodeTopologyConfigMap(cm *corev1.ConfigMap) InstanceTopology {
	instance := cm.Labels[deliveryLabelPrefix+"instance"]
	if instance == "" {
		instance = cm.Name
	}
	it := InstanceTopology{
		Instance:    instance,
		Namespace:   cm.Namespace,
		ConfigMap:   cm.Name,
		ResourceVer: cm.ResourceVersion,
	}
	if raw, ok := cm.Data["topology.json"]; ok {
		if json.Valid([]byte(raw)) {
			it.Topology = json.RawMessage(raw)
		} else {
			it.ParseError = "topology.json is not valid JSON"
		}
	}
	if raw, ok := cm.Data["clone.json"]; ok {
		if json.Valid([]byte(raw)) {
			it.Clone = json.RawMessage(raw)
		} else if it.ParseError == "" {
			it.ParseError = "clone.json is not valid JSON"
		}
	}
	return it
}
