package registration

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type staticTokenSource struct {
	token string
	err   error
}

func (s staticTokenSource) Token() (string, error) { return s.token, s.err }

// TestRegisterNoTokenOmitsAuthHeader verifies the auth-disabled path: when the
// client is built with a nil TokenSource (local development against an API server
// with authentication disabled), the request carries no Authorization header and
// still resolves a cluster_id.
func TestRegisterNoTokenOmitsAuthHeader(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"cluster_id":"abc123"}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "local", nil)
	clusterID, err := client.Register(context.Background())
	if err != nil {
		t.Fatalf("Register() error = %v, want nil", err)
	}
	if clusterID != "abc123" {
		t.Fatalf("Register() clusterID = %q, want %q", clusterID, "abc123")
	}
	if gotAuth != "" {
		t.Fatalf("Authorization header = %q, want empty (no token)", gotAuth)
	}
}

// TestRegisterWithTokenSetsAuthHeader verifies the auth-enabled path still sends
// the bearer token.
func TestRegisterWithTokenSetsAuthHeader(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"cluster_id":"xyz789"}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "hyp0-mc1", staticTokenSource{token: "tok"})
	clusterID, err := client.Register(context.Background())
	if err != nil {
		t.Fatalf("Register() error = %v, want nil", err)
	}
	if clusterID != "xyz789" {
		t.Fatalf("Register() clusterID = %q, want %q", clusterID, "xyz789")
	}
	if gotAuth != "Bearer tok" {
		t.Fatalf("Authorization header = %q, want %q", gotAuth, "Bearer tok")
	}
}

// TestRegisterForbiddenIsNonRetryable verifies a 403 maps to ErrForbidden so the
// caller exits instead of retrying (only reachable when auth is enabled).
func TestRegisterForbiddenIsNonRetryable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "hyp0-mc1", staticTokenSource{token: "tok"})
	if _, err := client.Register(context.Background()); !errors.Is(err, ErrForbidden) {
		t.Fatalf("Register() error = %v, want ErrForbidden", err)
	}
}
