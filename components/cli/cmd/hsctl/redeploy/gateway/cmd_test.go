package gateway

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, url string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"access_token":"tok","url":"`+url+`"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HYPERSHELL_CONFIG", path)
}

// Verifies: PLAT-DATA-33.01
func TestRedeployPostsTheActionAndPrintsTheGateway(t *testing.T) {
	var method, path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"gw-1","generation":3}`))
	}))
	defer server.Close()
	writeConfig(t, server.URL)

	var out bytes.Buffer
	Cmd.SetOut(&out)
	if err := run(Cmd, []string{"gw-1"}); err != nil {
		t.Fatal(err)
	}
	if method != http.MethodPost || path != "/api/hypershell/v1/gateways/gw-1/redeploy" {
		t.Fatalf("request = %s %s, want POST /api/hypershell/v1/gateways/gw-1/redeploy", method, path)
	}
	if !strings.Contains(out.String(), `"generation": 3`) {
		t.Fatalf("output = %q, want the returned gateway", out.String())
	}
}

// Verifies: SEC-RBAC-36.02
func TestRedeployReportsForbidden(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"code":"HYPERSHELL-4","reason":"forbidden"}`))
	}))
	defer server.Close()
	writeConfig(t, server.URL)

	err := run(Cmd, []string{"gw-1"})
	if err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("err = %v, want the 403 reported", err)
	}
}
