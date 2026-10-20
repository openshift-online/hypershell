package secretstore

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// A reachable Vault that has no value at the location must surface ErrNotFound so the
// control plane can treat it as SecretPending, not a read failure.
func TestVaultReader_NotFoundWrapsErrNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := NewVaultReader(srv.URL, "tok", "secret").Get(context.Background(), "loc")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get err = %v, want wrapped ErrNotFound", err)
	}
}

// A connection error (unreachable store) must NOT be ErrNotFound, so it degrades the
// dependent resource rather than being mistaken for a pending upload.
func TestVaultReader_UnreachableIsNotErrNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	srv.Close() // closed: the dial fails

	_, err := NewVaultReader(srv.URL, "tok", "secret").Get(context.Background(), "loc")
	if err == nil {
		t.Fatal("Get err = nil, want a read error")
	}
	if errors.Is(err, ErrNotFound) {
		t.Fatalf("Get err = %v, want NOT ErrNotFound", err)
	}
}

func TestMemoryReader_NotFoundWrapsErrNotFound(t *testing.T) {
	_, err := NewMemoryReader(nil).Get(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get err = %v, want wrapped ErrNotFound", err)
	}
}

// A private-CA Vault handshake succeeds when the CA bundle is trusted and fails
// against the default system trust store.
func TestNewVaultHTTPClient_PrivateCATrust(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// Default client does not trust the server's private CA.
	if _, err := (&http.Client{Timeout: 5 * time.Second}).Get(srv.URL); err == nil {
		t.Fatal("expected untrusted-CA failure with the default client")
	}

	caPath := writeServerCAPEM(t, srv)
	client, err := newVaultHTTPClient(caPath)
	if err != nil {
		t.Fatalf("newVaultHTTPClient: %v", err)
	}
	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("request with private-CA trust: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}

func TestNewVaultHTTPClient_RejectsInvalidCABundle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(path, []byte("not a certificate"), 0o600); err != nil {
		t.Fatalf("write temp ca: %v", err)
	}
	if _, err := newVaultHTTPClient(path); err == nil {
		t.Fatal("expected an error for an invalid CA bundle")
	}
}

// Kubernetes auth exchanges the ServiceAccount JWT for a Vault token and caches it,
// so a second call does not log in again.
func TestKubernetesTokenSource_LoginAndCache(t *testing.T) {
	var logins int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/auth/kubernetes/login" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		atomic.AddInt32(&logins, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"auth":{"client_token":"vault-tok","lease_duration":3600}}`))
	}))
	defer srv.Close()

	ks := &kubernetesTokenSource{
		addr:        srv.URL,
		authMount:   "kubernetes",
		role:        "hypershell-secret-sources",
		saTokenPath: "/unused",
		httpClient:  srv.Client(),
		readFile:    func(string) ([]byte, error) { return []byte("sa-jwt"), nil },
	}

	for i := 0; i < 2; i++ {
		tok, err := ks.token(context.Background())
		if err != nil {
			t.Fatalf("token call %d: %v", i, err)
		}
		if tok != "vault-tok" {
			t.Fatalf("token = %q, want vault-tok", tok)
		}
	}
	if got := atomic.LoadInt32(&logins); got != 1 {
		t.Fatalf("logins = %d, want 1 (second call must use the cache)", got)
	}
}

func TestKubernetesTokenSource_MissingTokenFile(t *testing.T) {
	ks := &kubernetesTokenSource{
		addr:        "http://vault.invalid",
		authMount:   "kubernetes",
		role:        "r",
		saTokenPath: filepath.Join(t.TempDir(), "absent"),
		httpClient:  &http.Client{},
	}
	if _, err := ks.token(context.Background()); err == nil {
		t.Fatal("expected an error when the service account token file is absent")
	}
}

// writeServerCAPEM writes the httptest TLS server's certificate to a PEM file and
// returns its path, so it can be used as a private-CA trust bundle.
func writeServerCAPEM(t *testing.T, srv *httptest.Server) string {
	t.Helper()
	cert := srv.Certificate()
	if cert == nil {
		t.Fatal("test server has no certificate")
	}
	block := &pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}
	path := filepath.Join(t.TempDir(), "server-ca.pem")
	if err := os.WriteFile(path, pem.EncodeToMemory(block), 0o600); err != nil {
		t.Fatalf("write server ca pem: %v", err)
	}
	// Sanity: the PEM parses back to the same certificate.
	if _, err := x509.ParseCertificate(cert.Raw); err != nil {
		t.Fatalf("parse server cert: %v", err)
	}
	return path
}
