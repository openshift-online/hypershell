// Package api exposes the read-only BFF JSON surface described in
// data-architecture.spec §5: /api/{fleet,promotion,topology,instances}, each
// wrapped in the cache.Snapshot envelope (data + generatedAt + stale/error).
package api

import (
	"encoding/json"
	"net/http"

	"github.com/openshift-online/hypershell/components/fleet-dashboard/pkg/cache"
)

// Handlers serves the cached data planes.
type Handlers struct {
	Fleet     *cache.Source
	Promotion *cache.Source
	Topology  *cache.Source
	Instances *cache.Source
}

// Register mounts the /api/* routes on the mux.
func (h *Handlers) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/fleet", h.serve(h.Fleet))
	mux.HandleFunc("GET /api/promotion", h.serve(h.Promotion))
	mux.HandleFunc("GET /api/topology", h.serve(h.Topology))
	mux.HandleFunc("GET /api/instances", h.serve(h.Instances))
}

// serve returns the source's snapshot. A source that has never produced data
// (no successful refresh yet) returns 503 so callers can distinguish cold-start
// from stale-but-serving; a stale-but-serving source returns 200 with the
// snapshot flagged, honoring §5.2's "one dead source never blanks the board".
func (h *Handlers) serve(s *cache.Source) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		snap := s.Get()
		w.Header().Set("Content-Type", "application/json")
		if snap.GeneratedAt.IsZero() {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		_ = json.NewEncoder(w).Encode(snap)
	}
}
