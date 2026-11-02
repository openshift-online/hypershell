package roleBindings

import (
	"context"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

const metricsNamespace = "hypershell"
const metricsSubsystem = "role_bindings"

var roleBindingMetricsOnce sync.Once

// RegisterRoleBindingMetrics registers Prometheus gauges for aggregate role
// binding adoption metrics. Safe to call multiple times.
func RegisterRoleBindingMetrics(roleBindingDao RoleBindingDao) {
	roleBindingMetricsOnce.Do(func() {
		prometheus.MustRegister(newGatewayOwnerUsersCollector(roleBindingDao))
	})
}

type gatewayOwnerUsersCollector struct {
	roleBindingDao RoleBindingDao
	desc           *prometheus.Desc
}

func newGatewayOwnerUsersCollector(
	roleBindingDao RoleBindingDao,
) *gatewayOwnerUsersCollector {
	return &gatewayOwnerUsersCollector{
		roleBindingDao: roleBindingDao,
		desc: prometheus.NewDesc(
			prometheus.BuildFQName(metricsNamespace, metricsSubsystem, "gateway_owner_users_total"),
			"Distinct non-deleted users who have ever held the gateway-owner role.",
			nil,
			nil,
		),
	}
}

func (c *gatewayOwnerUsersCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.desc
}

func (c *gatewayOwnerUsersCollector) Collect(ch chan<- prometheus.Metric) {
	count, err := c.roleBindingDao.CountGatewayOwnerUsers(context.Background())
	if err != nil {
		ch <- prometheus.NewInvalidMetric(c.desc, err)
		return
	}
	ch <- prometheus.MustNewConstMetric(c.desc, prometheus.GaugeValue, float64(count))
}
