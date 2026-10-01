package auth

import (
	"net/http"
	"testing"
)

// TestBearerToken covers both token sources the oauth-proxy can use: the
// Authorization header (direct bearer callers) and X-Forwarded-Access-Token
// (browser cookie sessions, via --pass-access-token). The browser path is the
// one that regressed: without it every cookie-authenticated user -- admins
// included -- reached the BFF tokenless and got a 401.
func TestBearerToken(t *testing.T) {
	tests := []struct {
		name    string
		headers map[string]string
		want    string
	}{
		{
			name:    "authorization bearer",
			headers: map[string]string{"Authorization": "Bearer abc123"},
			want:    "abc123",
		},
		{
			name:    "authorization bearer case-insensitive scheme",
			headers: map[string]string{"Authorization": "bearer abc123"},
			want:    "abc123",
		},
		{
			name:    "forwarded access token (browser cookie session)",
			headers: map[string]string{"X-Forwarded-Access-Token": "fwd-token"},
			want:    "fwd-token",
		},
		{
			name: "authorization takes precedence over forwarded",
			headers: map[string]string{
				"Authorization":            "Bearer direct",
				"X-Forwarded-Access-Token": "fwd-token",
			},
			want: "direct",
		},
		{
			name: "non-bearer authorization falls back to forwarded",
			headers: map[string]string{
				"Authorization":            "Basic dXNlcjpwYXNz",
				"X-Forwarded-Access-Token": "fwd-token",
			},
			want: "fwd-token",
		},
		{
			name:    "no token",
			headers: map[string]string{},
			want:    "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, _ := http.NewRequest(http.MethodGet, "/api/fleet", nil)
			for k, v := range tt.headers {
				r.Header.Set(k, v)
			}
			if got := bearerToken(r); got != tt.want {
				t.Errorf("bearerToken() = %q, want %q", got, tt.want)
			}
		})
	}
}
