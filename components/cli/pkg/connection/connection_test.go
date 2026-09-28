package connection

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v4"

	"github.com/openshift-online/hypershell/components/cli/pkg/config"
)

const proxyHelperEnv = "HSCTL_TEST_PROXY_HELPER"

// http.ProxyFromEnvironment reads the environment once per process, so the
// assertion runs in a re-executed test binary whose environment is fixed up
// front instead of relying on t.Setenv winning that race.
func TestNewTransportHonorsProxyEnvironment(t *testing.T) {
	if os.Getenv(proxyHelperEnv) == "1" {
		for _, insecure := range []bool{false, true} {
			transport := newTransport(insecure)
			if transport.Proxy == nil {
				t.Fatalf("insecure=%v: transport.Proxy is nil", insecure)
			}
			req, err := http.NewRequest(http.MethodGet, "https://api.example.com/api/hypershell/v1/gateways", nil)
			if err != nil {
				t.Fatalf("NewRequest: %v", err)
			}
			got, err := transport.Proxy(req)
			if err != nil {
				t.Fatalf("insecure=%v: Proxy: %v", insecure, err)
			}
			if got == nil || got.String() != "http://proxy.example.com:3128" {
				t.Errorf("insecure=%v: proxy = %v, want http://proxy.example.com:3128", insecure, got)
			}
		}
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestNewTransportHonorsProxyEnvironment$", "-test.v")
	cmd.Env = append(os.Environ(),
		proxyHelperEnv+"=1",
		"HTTPS_PROXY=http://proxy.example.com:3128",
		"https_proxy=http://proxy.example.com:3128",
		"NO_PROXY=",
		"no_proxy=",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("helper process failed: %v\n%s", err, out)
	}
}

func TestNewTransportInsecure(t *testing.T) {
	if cfg := newTransport(false).TLSClientConfig; cfg != nil && cfg.InsecureSkipVerify {
		t.Error("TLS verification disabled without insecure")
	}
	cfg := newTransport(true).TLSClientConfig
	if cfg == nil || !cfg.InsecureSkipVerify {
		t.Error("expected InsecureSkipVerify with insecure")
	}
}

func expiredToken(t *testing.T) string {
	t.Helper()
	claims := jwt.MapClaims{"exp": float64(time.Now().Add(-time.Minute).Unix())}
	s, err := jwt.NewWithClaims(jwt.SigningMethodNone, claims).SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// refreshingConnection builds a per-request-refresh connection whose access
// token has expired after Build, as happens during a long session.
func refreshingConnection(t *testing.T, apiURL, issuerURL string) *Connection {
	t.Helper()
	// A successful refresh saves the config; keep it away from the real one.
	t.Setenv("HYPERSHELL_CONFIG", filepath.Join(t.TempDir(), "config.json"))
	cfg := &config.Config{URL: apiURL, AccessToken: "initial"}
	conn, err := NewConnection().Config(cfg).RefreshPerRequest(true).Build()
	if err != nil {
		t.Fatal(err)
	}
	cfg.AccessToken = expiredToken(t)
	cfg.RefreshToken = "refresh"
	cfg.IssuerURL = issuerURL
	cfg.ClientID = "hsctl"
	return conn
}

func TestRefreshPerRequestRenewsExpiredToken(t *testing.T) {
	issuer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"access_token":"renewed","refresh_token":"refresh-2"}`))
	}))
	defer issuer.Close()
	var got string
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Authorization")
	}))
	defer api.Close()

	conn := refreshingConnection(t, api.URL, issuer.URL)
	resp, err := conn.DoContext(context.Background(), http.MethodGet, "/", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if got != "Bearer renewed" {
		t.Errorf("Authorization = %q, want the renewed token", got)
	}
}

func TestRefreshPerRequestReportsExpiredSession(t *testing.T) {
	issuer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"invalid_grant"}`))
	}))
	defer issuer.Close()
	called := false
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))
	defer api.Close()

	conn := refreshingConnection(t, api.URL, issuer.URL)
	_, err := conn.DoContext(context.Background(), http.MethodGet, "/", nil, nil)
	if !errors.Is(err, config.ErrSessionExpired) {
		t.Errorf("err = %v, want ErrSessionExpired", err)
	}
	if called {
		t.Error("request was sent with an expired token")
	}
}
