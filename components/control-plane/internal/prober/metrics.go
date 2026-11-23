package prober

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Result is the classification of a single probe (OBS-PRB-07): every probe is
// classified exactly once, as success or failure.
type Result string

const (
	// ResultSuccess labels a good probe.
	ResultSuccess Result = "success"
	// ResultFailure labels a failed probe.
	ResultFailure Result = "failure"
)

// Metrics holds the three Prometheus counters the prober exports (OBS-PRB-07).
// Each counter is incremented exactly once per completed probe and a success is
// never synthesized when a probe did not complete.
type Metrics struct {
	registry *prometheus.Registry

	gatewayHealth *prometheus.CounterVec
	apiRead       *prometheus.CounterVec
	apiAuth       *prometheus.CounterVec
}

// NewMetrics builds the prober counters in a private registry so the metrics
// endpoint serves only the prober's own series (OBS-PRB-07).
//
// The gateway health counter carries one series per gateway, keyed by gateway_id
// and managed_cluster, so the downstream worst-gateways panel can rank
// individual gateways. The gateway_id label is the bounded exception to
// API-OBS-06 that OBS-PRB-07 permits: its cardinality is bounded by the
// eligible-gateway count, and no per-probe identifier or route address is added.
func NewMetrics() *Metrics {
	m := &Metrics{
		registry: prometheus.NewRegistry(),
		gatewayHealth: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "hypershell_probe_gateway_health_total",
			Help: "Count of gateway health probes (OBS-PRB-03), by gateway, managed cluster, and result.",
		}, []string{"gateway_id", "managed_cluster", "result"}),
		apiRead: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "hypershell_probe_api_read_total",
			Help: "Count of API availability probes (OBS-PRB-04), by production instance and result.",
		}, []string{"instance", "result"}),
		apiAuth: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "hypershell_probe_api_auth_total",
			Help: "Count of API authentication probes (OBS-PRB-05), by production instance and result.",
		}, []string{"instance", "result"}),
	}
	m.registry.MustRegister(m.gatewayHealth, m.apiRead, m.apiAuth)
	return m
}

// RecordGatewayHealth increments the per-gateway health counter once (OBS-PRB-03,
// OBS-PRB-07.01).
func (m *Metrics) RecordGatewayHealth(gatewayID, managedCluster string, result Result) {
	m.gatewayHealth.WithLabelValues(gatewayID, managedCluster, string(result)).Inc()
}

// DeleteGatewaySeries removes every gateway health series for a gateway that is no
// longer eligible, so the gateway_id cardinality stays bounded by the current
// eligible-gateway count (fleet size) rather than growing with cumulative churn
// over the prober's uptime (OBS-PRB-07, the bounded-cardinality exception to
// API-OBS-06). Counter resets from this deletion are handled correctly by rate().
func (m *Metrics) DeleteGatewaySeries(gatewayID string) {
	m.gatewayHealth.DeletePartialMatch(prometheus.Labels{"gateway_id": gatewayID})
}

// RecordAPIRead increments the API availability counter once (OBS-PRB-04).
func (m *Metrics) RecordAPIRead(instance string, result Result) {
	m.apiRead.WithLabelValues(instance, string(result)).Inc()
}

// RecordAPIAuth increments the API authentication counter once (OBS-PRB-05).
func (m *Metrics) RecordAPIAuth(instance string, result Result) {
	m.apiAuth.WithLabelValues(instance, string(result)).Inc()
}

// Handler serves the prober counters in Prometheus exposition format
// (OBS-PRB-07.02).
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

// Serve runs the metrics HTTP server until ctx is cancelled (OBS-PRB-07). The
// endpoint is the prober's readiness and liveness surface too (OBS-PRB-11): it
// answers as soon as the process is up, independent of the clusters it measures
// (OBS-PRB-01).
func (m *Metrics) Serve(ctx context.Context, addr, path string) error {
	mux := http.NewServeMux()
	mux.Handle(path, m.Handler())
	// A liveness/readiness endpoint that does not depend on any measured target,
	// so a total outage of a measured cluster never fails the prober's own probes
	// (OBS-PRB-01). Skip it when the operator pointed the metrics path at the same
	// route, since a duplicate ServeMux registration would panic the prober; the
	// metrics handler then doubles as the health surface.
	if path != "/healthz" {
		mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		})
	}

	server := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		slog.InfoContext(ctx, "Prober metrics endpoint listening", "addr", addr, "path", path)
		errCh <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return err
		}
		return nil
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
