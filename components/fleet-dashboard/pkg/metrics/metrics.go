// Package metrics exposes the BFF's own operational metrics (data-architecture
// spec §5, /metrics): per-source refresh counts and durations, so the dashboard's
// own freshness is observable in the same Prometheus it reads from.
package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	refreshTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "fleet_dashboard_source_refresh_total",
		Help: "Total source refresh attempts, partitioned by source and result.",
	}, []string{"source", "result"})

	refreshDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "fleet_dashboard_source_refresh_duration_seconds",
		Help:    "Source refresh duration in seconds, partitioned by source.",
		Buckets: prometheus.DefBuckets,
	}, []string{"source"})
)

// Observe records one refresh outcome. It matches cache.Observer's signature.
func Observe(source string, ok bool, dur time.Duration) {
	result := "success"
	if !ok {
		result = "error"
	}
	refreshTotal.WithLabelValues(source, result).Inc()
	refreshDuration.WithLabelValues(source).Observe(dur.Seconds())
}
