package policyinterceptor

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics are the sidecar's dashboard counters (PLAT-PGP-15). They are never
// the source of the UI finding data; findings travel through ReportPolicyEvents.
// Labels are bounded: method is one of the static bindings, phase one of two
// phases, result one of allow, deny, or error.
type Metrics struct {
	registry *prometheus.Registry

	Evaluations      *prometheus.CounterVec
	WouldReject      prometheus.Counter
	WouldAddDefaults prometheus.Counter
	Denials          prometheus.Counter
	DroppedEvents    prometheus.Counter
	ReportFailures   prometheus.Counter
	BundleRejections prometheus.Counter
	AuthRejections   prometheus.Counter
	RecheckSkipped   *prometheus.CounterVec
	DefaultsMismatch prometheus.Counter
}

// NewMetrics builds the counters in a private registry.
func NewMetrics() *Metrics {
	m := &Metrics{
		registry: prometheus.NewRegistry(),
		Evaluations: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "hypershell_policy_interceptor_evaluations_total",
			Help: "Interceptor evaluations by bound method, phase, and result (allow, deny, error).",
		}, []string{"method", "phase", "result"}),
		WouldReject: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "hypershell_policy_interceptor_would_reject_total",
			Help: "Operations an audit-only policy would have rejected (PLAT-PGP-07).",
		}),
		WouldAddDefaults: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "hypershell_policy_interceptor_would_add_defaults_total",
			Help: "Operations that would have received fleet defaults while no policy is enforced (PLAT-PGP-08).",
		}),
		Denials: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "hypershell_policy_interceptor_denials_total",
			Help: "Operations denied by an enforced policy (PLAT-PGP-13).",
		}),
		DroppedEvents: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "hypershell_policy_interceptor_dropped_events_total",
			Help: "Audit events dropped because the bounded buffer was full (PLAT-PGP-15).",
		}),
		ReportFailures: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "hypershell_policy_interceptor_report_failures_total",
			Help: "Failed ReportPolicyEvents calls to the control plane.",
		}),
		BundleRejections: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "hypershell_policy_interceptor_bundle_rejections_total",
			Help: "Policy bundles rejected (wrong gateway, unknown schema version, or compile failure).",
		}),
		AuthRejections: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "hypershell_policy_interceptor_auth_rejections_total",
			Help: "Interceptor calls rejected because the gateway JWT did not verify (PLAT-PGP-09).",
		}),
		RecheckSkipped: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "hypershell_policy_interceptor_recheck_skipped_total",
			Help: "Start or resume re-checks skipped because the sandbox resources could not be read (PLAT-PGP-16).",
		}, []string{"method"}),
		DefaultsMismatch: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "hypershell_policy_interceptor_defaults_hash_mismatch_total",
			Help: "Calls that applied the policy bundle's fleet defaults because the sidecar environment defaults differ (PLAT-PGP-08.05).",
		}),
	}
	m.registry.MustRegister(m.Evaluations, m.WouldReject, m.WouldAddDefaults, m.Denials,
		m.DroppedEvents, m.ReportFailures, m.BundleRejections, m.AuthRejections, m.RecheckSkipped, m.DefaultsMismatch)
	return m
}

// Handler serves the private registry.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}
