package policyinterceptor

import (
	"net/http"
	"sync/atomic"
	"time"
)

// Health serves /healthz, /readyz, and /metrics. Readiness turns true once the
// interceptor socket is listening; it does not wait for a bundle, because the
// gateway must be able to call Describe before the first bundle loads
// (OSG-CORE-19.01) and fail_closed bindings already fail closed without one.
type Health struct {
	ready   atomic.Bool
	metrics *Metrics
}

// NewHealth returns a health handler serving metrics from m.
func NewHealth(m *Metrics) *Health { return &Health{metrics: m} }

// SetReady marks the interceptor socket as listening.
func (h *Health) SetReady(ready bool) { h.ready.Store(ready) }

// Handler returns the HTTP mux.
func (h *Health) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		if !h.ready.Load() {
			http.Error(w, "interceptor socket not listening\n", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	if h.metrics != nil {
		mux.Handle("/metrics", h.metrics.Handler())
	}
	return mux
}

// NewHTTPServer returns the health server with conservative timeouts.
func (h *Health) NewHTTPServer(addr string) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           h.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
	}
}
