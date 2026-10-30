package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/openshift-online/hypershell/components/fleet-dashboard/pkg/api"
	"github.com/openshift-online/hypershell/components/fleet-dashboard/pkg/auth"
	"github.com/openshift-online/hypershell/components/fleet-dashboard/pkg/config"
)

// staticFS is a representative embedded UI bundle: an index document plus a
// fingerprinted asset, mirroring a production `vite build` output.
func staticFS() fstest.MapFS {
	return fstest.MapFS{
		"index.html":        {Data: []byte("<!doctype html><title>glass</title>")},
		"assets/app-abc.js": {Data: []byte("console.log('app')")},
	}
}

func get(t *testing.T, h http.Handler, path string) (int, string, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	res := rec.Result()
	defer func() { _ = res.Body.Close() }()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return res.StatusCode, res.Header.Get("Content-Type"), string(body)
}

// FLT-NAV-19: direct navigation to and reload of a Glass application route must
// return the SPA document, while API and asset paths keep their own behavior and
// are never served the SPA document.
// Verifies: FLT-NAV-19.01
func TestSPAFallbackForGlassRoutes(t *testing.T) {
	// Auth disabled (dev mode) so the /api path exercises the handler, not a 401.
	authn := auth.New(&config.Config{AuthEnabled: false}, nil)
	h := New(&api.Handlers{}, authn, staticFS())

	const indexBody = "<!doctype html><title>glass</title>"

	// Every client-side route (root, each tab, a base-path-prefixed deep link)
	// resolves to the index document on direct navigation and reload.
	for _, path := range []string{
		"/",
		"/map",
		"/sdlc",
		"/e2e",
		"/e2e?run=tenant%2Fns%2Frun-1&rtab=logs",
		"/glass/e2e",
	} {
		status, ctype, body := get(t, h, path)
		if status != http.StatusOK {
			t.Errorf("GET %s: status = %d, want 200", path, status)
		}
		if ctype != "text/html; charset=utf-8" {
			t.Errorf("GET %s: content-type = %q, want text/html", path, ctype)
		}
		if body != indexBody {
			t.Errorf("GET %s: body = %q, want the SPA index document", path, body)
		}
	}

	// A real asset is served as itself, never the SPA document.
	assetStatus, _, assetBody := get(t, h, "/assets/app-abc.js")
	if assetStatus != http.StatusOK || assetBody == indexBody {
		t.Errorf("asset served the SPA document: status=%d body=%q", assetStatus, assetBody)
	}
	if assetBody != "console.log('app')" {
		t.Errorf("asset body = %q, want the real asset", assetBody)
	}

	// An /api route is handled by the API mux, not the SPA fallback. With the e2e
	// plane unconfigured it reports the feature off as JSON (E2E-LOG-01), which must
	// not be the SPA index document.
	apiStatus, apiCtype, apiBody := get(t, h, "/api/e2e/runs")
	if apiStatus != http.StatusOK {
		t.Errorf("GET /api/e2e/runs: status = %d, want 200", apiStatus)
	}
	if apiBody == indexBody {
		t.Error("GET /api/e2e/runs was served the SPA document")
	}
	if apiCtype != "application/json" {
		t.Errorf("GET /api/e2e/runs: content-type = %q, want application/json", apiCtype)
	}
}

// Extra APIs such as the spec viewer's (/specs/api/) sit behind the same auth
// gate as /api/, get the prefix stripped, and never fall back to the SPA.
func TestMountsAreAuthenticatedAndPrefixStripped(t *testing.T) {
	echo := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "mounted "+r.URL.Path)
	})
	mount := Mount{Prefix: "/specs/api/", Handler: echo}

	open := New(&api.Handlers{}, auth.New(&config.Config{AuthEnabled: false}, nil), staticFS(), mount)
	if status, _, body := get(t, open, "/specs/api/markers"); status != http.StatusOK || body != "mounted /markers" {
		t.Errorf("GET /specs/api/markers = %d %q, want 200 \"mounted /markers\"", status, body)
	}
	// The viewer's page is a static folder with its own index document.
	static := staticFS()
	static["specs/index.html"] = &fstest.MapFile{Data: []byte("<!doctype html><title>specs</title>")}
	static["specs/assets/viewer.js"] = &fstest.MapFile{Data: []byte("viewer")}
	withViewer := New(&api.Handlers{}, auth.New(&config.Config{AuthEnabled: false}, nil), static, mount)
	for path, want := range map[string]string{
		"/specs/":                 "<!doctype html><title>specs</title>",
		"/specs/assets/viewer.js": "viewer",
		"/sdlc/":                  "<!doctype html><title>glass</title>",
		"/":                       "<!doctype html><title>glass</title>",
	} {
		if status, _, body := get(t, withViewer, path); status != http.StatusOK || body != want {
			t.Errorf("GET %s = %d %q, want %q", path, status, body, want)
		}
	}
	if status, _, _ := get(t, withViewer, "/specs"); status != http.StatusMovedPermanently {
		t.Errorf("GET /specs = %d, want a redirect to /specs/", status)
	}

	gated := New(&api.Handlers{}, auth.New(&config.Config{AuthEnabled: true}, nil), staticFS(), mount)
	if status, _, _ := get(t, gated, "/specs/api/markers"); status != http.StatusUnauthorized {
		t.Errorf("GET /specs/api/markers without a token = %d, want 401", status)
	}
}
