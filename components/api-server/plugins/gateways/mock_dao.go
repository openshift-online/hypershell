package gateways

import (
	"context"

	"gorm.io/gorm"

	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
)

var _ GatewayDao = &gatewayDaoMock{}

type gatewayDaoMock struct {
	gateways GatewayList
}

func NewMockGatewayDao() *gatewayDaoMock {
	return &gatewayDaoMock{}
}

func (d *gatewayDaoMock) Get(ctx context.Context, id string) (*Gateway, error) {
	for _, gateway := range d.gateways {
		if gateway.ID == id {
			return gateway, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (d *gatewayDaoMock) GetUnscoped(ctx context.Context, id string) (*Gateway, error) {
	return d.Get(ctx, id)
}

func (d *gatewayDaoMock) Create(ctx context.Context, gateway *Gateway) (*Gateway, error) {
	d.gateways = append(d.gateways, gateway)
	return gateway, nil
}

func (d *gatewayDaoMock) Replace(ctx context.Context, gateway *Gateway) (*Gateway, error) {
	return nil, errors.NotImplemented("Gateway").AsError()
}

func (d *gatewayDaoMock) Delete(ctx context.Context, id string) error {
	return errors.NotImplemented("Gateway").AsError()
}

func (d *gatewayDaoMock) ClusterIDByNamespace(ctx context.Context, namespace string) (string, error) {
	for _, gateway := range d.gateways {
		if gateway.Namespace == namespace {
			return gateway.ClusterId, nil
		}
	}
	return "", gorm.ErrRecordNotFound
}

func (d *gatewayDaoMock) FindByIDs(ctx context.Context, ids []string) (GatewayList, error) {
	return nil, errors.NotImplemented("Gateway").AsError()
}

func (d *gatewayDaoMock) All(ctx context.Context) (GatewayList, error) {
	return d.gateways, nil
}

func (d *gatewayDaoMock) AdjustActiveSandboxCount(ctx context.Context, namespace string, delta int) (int, error) {
	gw := d.findByNamespace(namespace)
	if gw == nil {
		return 0, nil
	}
	next := derefCount(gw.ActiveSandboxCount) + delta
	if next < 0 {
		next = 0
	}
	gw.ActiveSandboxCount = &next
	return next, nil
}

func (d *gatewayDaoMock) SetActiveSandboxCount(ctx context.Context, namespace string, count int) (int, error) {
	gw := d.findByNamespace(namespace)
	if gw == nil {
		return 0, nil
	}
	if count < 0 {
		count = 0
	}
	gw.ActiveSandboxCount = &count
	return count, nil
}

func (d *gatewayDaoMock) SetGatewayVersion(ctx context.Context, id, version string) (string, error) {
	gw, err := d.Get(ctx, id)
	if err != nil {
		return "", err
	}
	gw.GatewayVersion = &version
	return version, nil
}

func (d *gatewayDaoMock) CountByPhase(ctx context.Context) (map[string]int64, error) {
	counts := make(map[string]int64)
	for _, gw := range d.gateways {
		if gw.Phase != nil {
			counts[*gw.Phase]++
		}
	}
	return counts, nil
}

func (d *gatewayDaoMock) SumActiveSandboxCount(ctx context.Context) (int64, error) {
	var total int64
	for _, gw := range d.gateways {
		total += int64(derefCount(gw.ActiveSandboxCount))
	}
	return total, nil
}

// CountByClusterAndPhase groups by the gateway's cluster_id as a stand-in for the
// resolved ManagedCluster name (the mock has no registry to join against); a blank
// cluster_id buckets to managedClusterUnknown.
func (d *gatewayDaoMock) CountByClusterAndPhase(ctx context.Context) ([]ClusterPhaseCount, error) {
	byCluster := map[string]map[string]int64{}
	for _, gw := range d.gateways {
		cluster := gw.ClusterId
		if cluster == "" {
			cluster = managedClusterUnknown
		}
		phase := ""
		if gw.Phase != nil {
			phase = *gw.Phase
		}
		phases, ok := byCluster[cluster]
		if !ok {
			phases = map[string]int64{}
			byCluster[cluster] = phases
		}
		phases[phase]++
	}
	var out []ClusterPhaseCount
	for cluster, phases := range byCluster {
		for phase, count := range phases {
			out = append(out, ClusterPhaseCount{ClusterName: cluster, Phase: phase, Count: count})
		}
	}
	return out, nil
}

func (d *gatewayDaoMock) SumActiveSandboxCountByCluster(ctx context.Context) ([]ClusterSandboxCount, error) {
	byCluster := map[string]int64{}
	for _, gw := range d.gateways {
		cluster := gw.ClusterId
		if cluster == "" {
			cluster = managedClusterUnknown
		}
		byCluster[cluster] += int64(derefCount(gw.ActiveSandboxCount))
	}
	var out []ClusterSandboxCount
	for cluster, count := range byCluster {
		out = append(out, ClusterSandboxCount{ClusterName: cluster, Count: count})
	}
	return out, nil
}

func (d *gatewayDaoMock) findByNamespace(namespace string) *Gateway {
	for _, gateway := range d.gateways {
		if gateway.Namespace == namespace {
			return gateway
		}
	}
	return nil
}
