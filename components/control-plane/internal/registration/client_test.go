package registration

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type staticToken string

func (s staticToken) Token() (string, error) { return string(s), nil }

func TestRegisterStatusHandling(t *testing.T) {
	cases := []struct {
		name        string
		status      int
		body        string
		wantID      string
		wantErrIs   error
		wantErrText string
	}{
		{name: "created", status: http.StatusCreated, body: `{"cluster_id":"2abc"}`, wantID: "2abc"},
		{name: "ok", status: http.StatusOK, body: `{"cluster_id":"2abc"}`, wantID: "2abc"},
		{name: "forbidden", status: http.StatusForbidden, body: `{"kind":"Error"}`, wantErrIs: ErrForbidden},
		{
			name:        "conflict carries API reason",
			status:      http.StatusConflict,
			body:        `{"kind":"Error","code":"HYPERSHELL-MGMT-6","reason":"managed cluster name \"local-kind\" is held by record 2xyz"}`,
			wantErrIs:   ErrConflict,
			wantErrText: "held by record 2xyz",
		},
		{name: "server error is transient", status: http.StatusInternalServerError, body: `boom`, wantErrText: "registration returned 500"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/hypershell/v1/managed_clusters/registration" || r.Method != http.MethodPost {
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
				}
				if got := r.Header.Get("Authorization"); got != "Bearer tok" {
					t.Errorf("Authorization = %q", got)
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			id, err := NewClient(srv.URL, "local-kind", staticToken("tok")).Register(context.Background())
			if tc.wantID != "" {
				if err != nil || id != tc.wantID {
					t.Fatalf("Register() = (%q, %v), want (%q, nil)", id, err, tc.wantID)
				}
				return
			}
			if err == nil {
				t.Fatalf("Register() = %q, want error", id)
			}
			if tc.wantErrIs != nil && !errors.Is(err, tc.wantErrIs) {
				t.Fatalf("Register() error %v, want errors.Is %v", err, tc.wantErrIs)
			}
			if tc.wantErrIs == nil && (errors.Is(err, ErrForbidden) || errors.Is(err, ErrConflict)) {
				t.Fatalf("transient error %v must not be classified as non-retryable", err)
			}
			if tc.wantErrText != "" && !strings.Contains(err.Error(), tc.wantErrText) {
				t.Fatalf("Register() error %q does not contain %q", err, tc.wantErrText)
			}
		})
	}
}
