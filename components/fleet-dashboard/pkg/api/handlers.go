// Package api exposes the read-only BFF JSON surface described in
// data-architecture.spec §5: /api/{fleet,promotion,topology,instances,sdlc}, each
// wrapped in the cache.Snapshot envelope (data + generatedAt + stale/error).
package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/openshift-online/hypershell/components/fleet-dashboard/pkg/cache"
	"github.com/openshift-online/hypershell/components/fleet-dashboard/pkg/sources"
)

// E2ERunPlane is the e2e-run detail and live-stream surface the handlers need.
// It is an interface so the api package stays decoupled from the Loki source and
// so handler tests can supply a fake (e2e-run-log-visibility.spec E2E-LOG-17).
type E2ERunPlane interface {
	RunDetail(ctx context.Context, id string) (*sources.E2ERunDetail, error)
	RunLogs(ctx context.Context, id, before string, limit int) (*sources.E2ELogPage, error)
	Stream(w http.ResponseWriter, r *http.Request)
}

// Handlers serves the cached data planes.
type Handlers struct {
	Fleet     *cache.Source
	Promotion *cache.Source
	Topology  *cache.Source
	Instances *cache.Source
	// SdlcAgents is the SDLC agent runtime telemetry plane (SDLC-AGT spec).
	// It is unconditional since Prometheus is required config.
	SdlcAgents *cache.Source
	// SDLC is the Agent SDLC funnel-visibility plane
	// (agent-sdlc-visibility-dashboard.spec). It is optional: nil when no source
	// repository is configured, in which case the route is simply not mounted.
	SDLC *cache.Source
	// E2ERuns is the recent-runs snapshot plane; E2E serves run detail and the
	// live stream (e2e-run-log-visibility.spec). Both are nil when the feature is
	// unconfigured. Unlike the other optional planes, the e2e routes are ALWAYS
	// mounted so a disabled feature reports itself off rather than 404ing
	// (E2E-LOG-01/02).
	E2ERuns *cache.Source
	E2E     E2ERunPlane
}

// Register mounts the /api/* routes on the mux.
func (h *Handlers) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/fleet", h.serve(h.Fleet))
	mux.HandleFunc("GET /api/promotion", h.serve(h.Promotion))
	mux.HandleFunc("GET /api/topology", h.serve(h.Topology))
	mux.HandleFunc("GET /api/instances", h.serve(h.Instances))
	mux.HandleFunc("GET /api/sdlc-agents", h.serve(h.SdlcAgents))
	if h.SDLC != nil {
		mux.HandleFunc("GET /api/sdlc", h.serve(h.SDLC))
	}
	// The e2e-run routes are always mounted: when the feature is unconfigured they
	// report it off (E2E-LOG-01/02), so the SPA can render a feature-off state
	// rather than a failed request.
	mux.HandleFunc("GET /api/e2e/runs", h.serveE2ERuns())
	// The run id is a multi-segment address (tenant/namespace/runId), so it uses
	// the trailing wildcard rather than a single-segment {id}.
	mux.HandleFunc("GET /api/e2e/runs/{id...}", h.serveE2ERunDetail())
	// The paginated raw-log history (E2E-LOG-19). The run id is multi-segment, and a
	// ServeMux trailing wildcard ({id...}) must be the final path segment, so the
	// id cannot be followed by a literal /logs. The run id therefore rides a "run"
	// query value, exactly as the live stream endpoint already does.
	mux.HandleFunc("GET /api/e2e/logs", h.serveE2ERunLogs())
	mux.HandleFunc("GET /api/e2e/stream", h.serveE2EStream())
}

// e2eFeatureOffSnapshot is the body returned by the e2e endpoints when the
// feature is unconfigured: a healthy snapshot whose data flags the feature off.
func e2eFeatureOffSnapshot() cache.Snapshot {
	return cache.Snapshot{
		Data:        sources.E2ERunsPayload{Enabled: false},
		GeneratedAt: time.Now(),
	}
}

func writeJSON(w http.ResponseWriter, r *http.Request, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	body, err := json.Marshal(v)
	if err != nil {
		slog.Default().ErrorContext(r.Context(), "encode e2e response failed", "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"failed to encode response"}`))
		return
	}
	if status != http.StatusOK {
		w.WriteHeader(status)
	}
	_, _ = w.Write(body)
}

// serveE2ERuns serves the recent-runs snapshot, or a feature-off body when the
// plane is disabled.
func (h *Handlers) serveE2ERuns() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.E2ERuns == nil {
			writeJSON(w, r, http.StatusOK, e2eFeatureOffSnapshot())
			return
		}
		h.serve(h.E2ERuns)(w, r)
	}
}

// serveE2ERunDetail serves one run's reduced steps and bounded log history,
// addressed by its collision-free id, or 404 when the run is absent.
func (h *Handlers) serveE2ERunDetail() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.E2E == nil {
			writeJSON(w, r, http.StatusOK, e2eFeatureOffSnapshot())
			return
		}
		detail, err := h.E2E.RunDetail(r.Context(), r.PathValue("id"))
		if err != nil {
			if errors.Is(err, sources.ErrRunNotFound) {
				writeJSON(w, r, http.StatusNotFound, map[string]string{"error": "run not found"})
				return
			}
			slog.Default().ErrorContext(r.Context(), "e2e run detail failed", "err", err)
			// Serve a stale-style snapshot so the detail pane degrades rather than
			// blanking (E2E-LOG-10). The raw error can carry the internal Loki
			// identity (service DNS, LogQL, tenant), so only a generic message crosses
			// to the browser; the detail is logged above (E2E-LOG-08).
			writeJSON(w, r, http.StatusOK, cache.Snapshot{Stale: true, Error: "e2e log backend unavailable", GeneratedAt: time.Now()})
			return
		}
		writeJSON(w, r, http.StatusOK, cache.Snapshot{Data: detail, GeneratedAt: time.Now()})
	}
}

// serveE2ERunLogs serves one bounded page of a run's raw-log history addressed by
// its collision-free id (the "run" query value), with an optional `before`
// nanosecond cursor and `limit` (clamped by the source), or 404 when the run's
// tenant is not addressable. A failed query degrades to a stale-flagged snapshot
// rather than blanking, like the detail endpoint (E2E-LOG-19/20).
func (h *Handlers) serveE2ERunLogs() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.E2E == nil {
			writeJSON(w, r, http.StatusOK, e2eFeatureOffSnapshot())
			return
		}
		q := r.URL.Query()
		id := q.Get("run")
		before := q.Get("before")
		limit := 0
		if raw := q.Get("limit"); raw != "" {
			// An unparseable limit falls back to the source default (limit stays 0),
			// rather than failing the request on a stray query value.
			if n, err := strconv.Atoi(raw); err == nil {
				limit = n
			}
		}
		page, err := h.E2E.RunLogs(r.Context(), id, before, limit)
		if err != nil {
			if errors.Is(err, sources.ErrRunNotFound) {
				writeJSON(w, r, http.StatusNotFound, map[string]string{"error": "run not found"})
				return
			}
			slog.Default().ErrorContext(r.Context(), "e2e run logs failed", "err", err)
			// Degrade the log pane rather than blanking it; the raw error can carry the
			// internal Loki identity, so only a generic message crosses to the browser
			// (E2E-LOG-08).
			writeJSON(w, r, http.StatusOK, cache.Snapshot{Stale: true, Error: "e2e log backend unavailable", GeneratedAt: time.Now()})
			return
		}
		writeJSON(w, r, http.StatusOK, cache.Snapshot{Data: page, GeneratedAt: time.Now()})
	}
}

// serveE2EStream serves the live SSE stream, or a feature-off body when the plane
// is disabled.
func (h *Handlers) serveE2EStream() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.E2E == nil {
			writeJSON(w, r, http.StatusOK, e2eFeatureOffSnapshot())
			return
		}
		h.E2E.Stream(w, r)
	}
}

// serve returns the source's snapshot. A source that has never produced data
// (no successful refresh yet) returns 503 so callers can distinguish cold-start
// from stale-but-serving; a stale-but-serving source returns 200 with the
// snapshot flagged, honoring §5.2's "one dead source never blanks the board".
func (h *Handlers) serve(s *cache.Source) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		snap := s.Get()
		w.Header().Set("Content-Type", "application/json")
		// Marshal BEFORE writing the status: a json encode failure (e.g. a
		// NaN/Inf that slipped into the payload) must not leave the client with a
		// silent empty 200 body -- that blanked the dashboard panels. Surface it
		// as a 500 so the failure is visible in logs and to the UI.
		body, err := json.Marshal(snap)
		if err != nil {
			slog.Default().ErrorContext(r.Context(), "encode snapshot failed", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"failed to encode snapshot"}`))
			return
		}
		if snap.GeneratedAt.IsZero() {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		_, _ = w.Write(body)
	}
}
