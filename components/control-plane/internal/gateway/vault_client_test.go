package gateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func newTestVaultClient(t *testing.T, srv *httptest.Server) *vaultClient {
	t.Helper()
	c, err := newVaultClient(srv.URL, nil, 5*time.Second)
	if err != nil {
		t.Fatalf("newVaultClient: %v", err)
	}
	c.http = srv.Client() // trust the httptest TLS cert
	return c
}

func TestVaultClient_Login(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/auth/kubernetes/login" {
			t.Errorf("unexpected login request %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"auth":{"client_token":"hvs.TESTTOKEN"}}`))
	}))
	defer srv.Close()
	c := newTestVaultClient(t, srv)
	tok, err := c.login(context.Background(), "kubernetes", "openshell-gateway", "jwt-abc")
	if err != nil || tok != "hvs.TESTTOKEN" {
		t.Fatalf("login = %q, %v; want hvs.TESTTOKEN", tok, err)
	}
}

func TestVaultClient_LoginDenied(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"errors":["permission denied"]}`))
	}))
	defer srv.Close()
	c := newTestVaultClient(t, srv)
	if _, err := c.login(context.Background(), "kubernetes", "openshell-gateway", "jwt"); err == nil {
		t.Fatal("expected login denial error, got nil")
	}
}

func TestVaultClient_ListKeys(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("list") != "true" || r.Header.Get("X-Vault-Token") != "ct" {
			t.Errorf("unexpected list request: %s token=%q", r.URL.String(), r.Header.Get("X-Vault-Token"))
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{"keys":["prov-a","prov-b"]}}`))
	}))
	defer srv.Close()
	c := newTestVaultClient(t, srv)
	keys, err := c.listKeys(context.Background(), "ct", "secret", "openshell/ns-1")
	if err != nil || len(keys) != 2 || keys[0] != "prov-a" {
		t.Fatalf("listKeys = %v, %v", keys, err)
	}
}

func TestVaultClient_ListKeys_EmptyIs404(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	c := newTestVaultClient(t, srv)
	keys, err := c.listKeys(context.Background(), "ct", "secret", "openshell/empty")
	if err != nil || keys != nil {
		t.Fatalf("expected nil keys, nil err for 404; got %v, %v", keys, err)
	}
}

func TestVaultClient_DeleteMetadata(t *testing.T) {
	called := false
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == "/v1/secret/metadata/openshell/ns-1/prov-a" {
			called = true
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	c := newTestVaultClient(t, srv)
	if err := c.deleteMetadata(context.Background(), "ct", "secret", "openshell/ns-1/prov-a"); err != nil {
		t.Fatalf("deleteMetadata: %v", err)
	}
	if !called {
		t.Fatal("expected DELETE on the metadata path")
	}
}

func TestNewVaultClient_BadCA(t *testing.T) {
	if _, err := newVaultClient("https://vault:8200", []byte("not-a-cert"), time.Second); err == nil {
		t.Fatal("expected error for unusable CA bundle")
	}
}
