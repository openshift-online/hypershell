package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v4"
)

func runWith(t *testing.T, refresh time.Duration) error {
	t.Helper()
	old := args.refresh
	args.refresh = refresh
	t.Cleanup(func() { args.refresh = old })
	return run(Cmd, nil)
}

func TestRefreshBelowMinimumIsRejected(t *testing.T) {
	err := runWith(t, time.Second)
	if err == nil || !strings.Contains(err.Error(), "at least 2s") {
		t.Errorf("err = %v, want the minimum named", err)
	}
}

func TestNotLoggedIn(t *testing.T) {
	t.Setenv("HYPERSHELL_CONFIG", filepath.Join(t.TempDir(), "missing.json"))
	err := runWith(t, 5*time.Second)
	if err == nil || err.Error() != "not logged in, server URL isn't set, run the 'login' command" {
		t.Errorf("err = %v", err)
	}
}

func TestRequiresInteractiveTerminal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"url":"https://api.example.com","access_token":"token"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HYPERSHELL_CONFIG", path)
	// Under `go test` stdout is a pipe, which is exactly the redirected case.
	err := runWith(t, 5*time.Second)
	if err == nil || err.Error() != "hsctl ui requires an interactive terminal" {
		t.Errorf("err = %v", err)
	}
}

func token(t *testing.T, claims jwt.MapClaims) string {
	t.Helper()
	s, err := jwt.NewWithClaims(jwt.SigningMethodNone, claims).SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestIdentity(t *testing.T) {
	cases := []struct {
		claims jwt.MapClaims
		want   string
	}{
		{jwt.MapClaims{"preferred_username": "alice", "sub": "123"}, "alice"},
		{jwt.MapClaims{"client_id": "ci-bot", "sub": "123"}, "ci-bot"},
		{jwt.MapClaims{"sub": "123"}, "123"},
	}
	for _, c := range cases {
		if got := identity(token(t, c.claims)); got != c.want {
			t.Errorf("identity(%v) = %q, want %q", c.claims, got, c.want)
		}
	}
	if got := identity("not-a-jwt"); got != "" {
		t.Errorf("identity of garbage = %q", got)
	}
}
