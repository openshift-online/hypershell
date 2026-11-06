package defaultWorkspaceMemberships

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

// memberLimitViolations backs hypershell_default_workspace_member_limit_violations_total
// (control-plane-reconciliation-metrics.spec.md, OBS-CRM-14). It is emitted by
// the API server, incremented inline by the gatewayAccess (GTW-ACC-17) and
// serviceAccounts (OSG-SACC-19) plugins at the moment they reject a request
// because completing it would exceed the 1,000-member default-workspace limit.
// It lives here, not in either caller plugin, so both share one registration
// and one counter instead of two competing registrations of the same metric
// name.
var memberLimitViolations = prometheus.NewCounter(prometheus.CounterOpts{
	Namespace: "hypershell",
	Name:      "default_workspace_member_limit_violations_total",
	Help:      "Count of gateway-access grants/changes and service-account creates/role-updates rejected because completing them would exceed the 1,000-member default-workspace limit.",
})

var metricsOnce sync.Once

// RegisterMetrics registers the member-limit-violations counter. Safe to call
// multiple times.
func RegisterMetrics() {
	metricsOnce.Do(func() {
		prometheus.MustRegister(memberLimitViolations)
	})
}

// RecordMemberLimitViolation increments hypershell_default_workspace_member_limit_violations_total.
func RecordMemberLimitViolation() {
	memberLimitViolations.Inc()
}
