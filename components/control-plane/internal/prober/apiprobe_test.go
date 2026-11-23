package prober

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClassifyAvailability(t *testing.T) {
	cases := []struct {
		name   string
		status int
		err    error
		want   Result
	}{
		{"503 is failure", 503, nil, ResultFailure},
		{"500 is failure", 500, nil, ResultFailure},
		{"401 is good", 401, nil, ResultSuccess},
		{"200 is good", 200, nil, ResultSuccess},
		{"transport error is failure", 0, errors.New("timeout"), ResultFailure},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyAvailability(tc.status, tc.err); got != tc.want {
				t.Errorf("classifyAvailability(%d,%v) = %q, want %q", tc.status, tc.err, got, tc.want)
			}
		})
	}
}

func TestClassifyAuth(t *testing.T) {
	cases := []struct {
		name   string
		status int
		err    error
		want   Result
	}{
		{"200 is good", 200, nil, ResultSuccess},
		{"401 is failure", 401, nil, ResultFailure},
		{"403 is failure", 403, nil, ResultFailure},
		{"500 is failure", 500, nil, ResultFailure},
		{"transport error is failure", 0, errors.New("timeout"), ResultFailure},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyAuth(tc.status, tc.err); got != tc.want {
				t.Errorf("classifyAuth(%d,%v) = %q, want %q", tc.status, tc.err, got, tc.want)
			}
		})
	}
}

// TestAPIAvailability5xxIsFailed covers OBS-PRB-04.01.
func TestAPIAvailability5xxIsFailed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	if got := probeAPIAvailability(context.Background(), srv.Client(), srv.URL, "/read", ""); got != ResultFailure {
		t.Fatalf("availability probe = %q, want failure for 503", got)
	}
}

// TestAPIAvailability401IsGood covers OBS-PRB-04.02.
func TestAPIAvailability401IsGood(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	if got := probeAPIAvailability(context.Background(), srv.Client(), srv.URL, "/read", ""); got != ResultSuccess {
		t.Fatalf("availability probe = %q, want success for 401", got)
	}
}

// TestAPIAvailabilityUnreachableIsFailed covers OBS-PRB-04.03.
func TestAPIAvailabilityUnreachableIsFailed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(100 * time.Millisecond)
	}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if got := probeAPIAvailability(ctx, srv.Client(), srv.URL, "/read", ""); got != ResultFailure {
		t.Fatalf("availability probe = %q, want failure on timeout", got)
	}
}

// TestAPIAuthFullReadIsGood covers OBS-PRB-05.01.
func TestAPIAuthFullReadIsGood(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer good-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	exchange := func(context.Context) (string, error) { return "good-token", nil }
	if got := probeAPIAuth(context.Background(), srv.Client(), srv.URL, "/read", exchange); got != ResultSuccess {
		t.Fatalf("auth probe = %q, want success", got)
	}
}

// TestAPIAuthTokenFailureDoesNotCallRead covers OBS-PRB-05.02: a token
// acquisition failure is a failed probe and the authenticated read is not called.
func TestAPIAuthTokenFailureDoesNotCallRead(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	exchange := func(context.Context) (string, error) { return "", errors.New("keycloak down") }
	if got := probeAPIAuth(context.Background(), srv.Client(), srv.URL, "/read", exchange); got != ResultFailure {
		t.Fatalf("auth probe = %q, want failure on token acquisition failure", got)
	}
	if called {
		t.Fatal("authenticated read was called despite token acquisition failure (OBS-PRB-05.02)")
	}
}

// TestAPIAuthRejectedReadIsFailed covers OBS-PRB-05.03.
func TestAPIAuthRejectedReadIsFailed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	exchange := func(context.Context) (string, error) { return "token", nil }
	if got := probeAPIAuth(context.Background(), srv.Client(), srv.URL, "/read", exchange); got != ResultFailure {
		t.Fatalf("auth probe = %q, want failure for 401 read", got)
	}
}
