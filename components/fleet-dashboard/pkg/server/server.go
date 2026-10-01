// Package server wires the BFF's HTTP surface together: the authenticated
// /api/* JSON planes, unauthenticated /healthz + /readyz probes, and the
// embedded SPA with an index.html fallback (data-architecture.spec §3.3, §5).
// Readiness gates on the process and its own dependencies only - never on
// upstream data sources (§5.2), so a stale Prometheus never fails the pod.
//
// /metrics is served separately by Metrics() on its own listener
// (FD_METRICS_ADDR, default :9090), not fronted by the oauth-proxy, so
// Prometheus scrapes it directly (data-architecture.spec §9).
package server

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"strings"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/openshift-online/hypershell/components/fleet-dashboard/pkg/api"
	"github.com/openshift-online/hypershell/components/fleet-dashboard/pkg/auth"
	"github.com/openshift-online/hypershell/components/fleet-dashboard/pkg/version"
)

// New builds the top-level handler.
//
//   - /healthz, /readyz                 unauthenticated
//   - /api/*                            authenticated (auth.Middleware)
//   - everything else                   embedded SPA with index.html fallback
//
// /metrics is NOT on this handler; see Metrics() for the dedicated listener.
func New(h *api.Handlers, authn *auth.Authenticator, static fs.FS) http.Handler {
	apiMux := http.NewServeMux()
	h.Register(apiMux)

	root := http.NewServeMux()
	root.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":    "ok",
			"version":   version.Version,
			"buildTime": version.BuildTime,
		})
	})
	root.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready"))
	})
	root.Handle("/api/", authn.Middleware(apiMux))
	root.Handle("/", spaHandler(static))
	return root
}

// Metrics builds the handler for the dedicated Prometheus metrics listener. It
// is served on its own port (FD_METRICS_ADDR, default :9090) that is not fronted
// by the oauth-proxy, so Prometheus can scrape it directly (data-architecture.spec
// §9); access is restricted at the network layer (NetworkPolicy), not by auth.
func Metrics() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", promhttp.Handler())
	return mux
}

// spaHandler serves static assets, falling back to index.html for any path that
// isn't a real file so client-side routes (React Router) resolve on deep links.
func spaHandler(static fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(static))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p == "" {
			serveIndex(w, r, static)
			return
		}
		if _, err := fs.Stat(static, p); err != nil {
			serveIndex(w, r, static)
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}

func serveIndex(w http.ResponseWriter, r *http.Request, static fs.FS) {
	data, err := fs.ReadFile(static, "index.html")
	if err != nil {
		http.Error(w, "UI bundle not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
}
