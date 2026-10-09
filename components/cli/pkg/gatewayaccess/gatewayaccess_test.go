package gatewayaccess

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/openshift-online/hypershell/components/cli/pkg/config"
	"github.com/openshift-online/hypershell/components/cli/pkg/connection"
)

func TestValidateRole(t *testing.T) {
	for _, ok := range []string{"owner", "admin", "user"} {
		if err := ValidateRole(ok); err != nil {
			t.Errorf("ValidateRole(%q) = %v, want nil", ok, err)
		}
	}
	for _, bad := range []string{"", "openshell-admin", "viewer", "Owner"} {
		if err := ValidateRole(bad); err == nil {
			t.Errorf("ValidateRole(%q) = nil, want error", bad)
		}
	}
}

func newConn(t *testing.T, serverURL string) *connection.Connection {
	t.Helper()
	conn, err := connection.NewConnection().Config(&config.Config{URL: serverURL, AccessToken: "management-token"}).Build()
	if err != nil {
		t.Fatal(err)
	}
	return conn
}

// A 409 (last-owner protection) must surface as a non-zero-exit error carrying
// the API's reason so the operator sees actionable guidance (GAM-13).
func TestRequest_LastOwnerConflictSurfacesReason(t *testing.T) {
	const reason = "this gateway must keep at least one owner"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"code":"conflict","reason":"` + reason + `"}`))
	}))
	defer server.Close()
	conn := newConn(t, server.URL)
	defer conn.Close()

	_, status, err := Request(conn, http.MethodDelete, "/access/u1", nil, nil, http.StatusNoContent)
	if err == nil {
		t.Fatal("expected a 409 to be an error")
	}
	if status != http.StatusConflict {
		t.Fatalf("status = %d, want 409", status)
	}
	if !strings.Contains(err.Error(), reason) {
		t.Fatalf("error %q does not carry the last-owner reason", err.Error())
	}
}

// A 403 (unauthorized management) must also surface as an error, not a silent
// success.
func TestRequest_ForbiddenSurfaces(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"code":"forbidden","reason":"only owners can assign the owner role"}`))
	}))
	defer server.Close()
	conn := newConn(t, server.URL)
	defer conn.Close()

	_, _, err := Request(conn, http.MethodPost, "/access", nil, nil, http.StatusCreated)
	if err == nil {
		t.Fatal("expected a 403 to be an error")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Fatalf("error %q should mention the status", err.Error())
	}
}

func TestRequest_AcceptedReturnsBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"role":"admin"}`))
	}))
	defer server.Close()
	conn := newConn(t, server.URL)
	defer conn.Close()

	body, status, err := Request(conn, http.MethodGet, "/access", nil, nil, http.StatusOK)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != http.StatusOK || !strings.Contains(string(body), "admin") {
		t.Fatalf("unexpected response: status=%d body=%s", status, body)
	}
}

func TestPaths(t *testing.T) {
	if got := CollectionPath("gw 1"); got != "/api/hypershell/v1/gateways/gw%201/access" {
		t.Errorf("CollectionPath escaping = %q", got)
	}
	if got := ItemPath("gw1", "u/1"); got != "/api/hypershell/v1/gateways/gw1/access/u%2F1" {
		t.Errorf("ItemPath escaping = %q", got)
	}
}
