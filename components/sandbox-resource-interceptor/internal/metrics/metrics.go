// Package metrics exposes the sandbox-resource-interceptor Prometheus
// metrics: evaluation counts by outcome and denial reason, and the current
// enforcement mode. With no audit stage in the policy, these metrics are the
// only visibility into what enforcement changes.
package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/openshift-online/hypershell/components/sandbox-resource-interceptor/internal/policy"
)

// Registry holds the interceptor's metrics on a private Prometheus registry.
// A private registry keeps repeated construction in tests safe and keeps the
// process's default registry untouched.
type Registry struct {
	registry    *prometheus.Registry
	evaluations *prometheus.CounterVec
	mode        *prometheus.GaugeVec
}

// New registers the metrics on a fresh registry and pins the mode gauge.
// Both mode series are initialised so an alert can compare
// sandbox_resource_interceptor_mode{mode="enforce"} against 0 when the
// interceptor runs with enforcement off.
func New(mode policy.Mode) *Registry {
	reg := &Registry{
		registry: prometheus.NewRegistry(),
		evaluations: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "sandbox_resource_interceptor_evaluations_total",
			Help: "CreateSandbox evaluations by outcome (allowed, patched, denied) and machine-readable denial reason.",
		}, []string{"outcome", "reason"}),
		mode: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "sandbox_resource_interceptor_mode",
			Help: "Current enforcement mode: 1 for the active mode, 0 for the inactive one.",
		}, []string{"mode"}),
	}
	reg.registry.MustRegister(reg.evaluations, reg.mode)
	for _, known := range []policy.Mode{policy.ModeOff, policy.ModeEnforce} {
		value := 0.0
		if known == mode {
			value = 1.0
		}
		reg.mode.WithLabelValues(string(known)).Set(value)
	}
	return reg
}

// RecordEvaluation counts one evaluation outcome. reason is the
// machine-readable denial reason and is empty for allowed and patched
// evaluations.
func (r *Registry) RecordEvaluation(outcome policy.Outcome, reason policy.DenialReason) {
	r.evaluations.WithLabelValues(string(outcome), string(reason)).Inc()
}

// Handler serves the /metrics endpoint.
func (r *Registry) Handler() http.Handler {
	return promhttp.HandlerFor(r.registry, promhttp.HandlerOpts{})
}
