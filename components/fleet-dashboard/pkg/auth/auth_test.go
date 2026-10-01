package auth

import (
	"context"
	"net/http"
	"testing"

	authnv1 "k8s.io/api/authentication/v1"
	authzv1 "k8s.io/api/authorization/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"

	"github.com/openshift-online/hypershell/components/fleet-dashboard/pkg/config"
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

// TestAuthorizeDropsTokenScopes guards the regression where every dashboard pane
// 403'd for every browser user. oauth-proxy mints a minimal-scope token
// (user:info) to identify the user; if the BFF forwards that scope into the SAR,
// OpenShift's scope authorizer rejects it ("scopes [user:info] prevent this
// action") regardless of the user's real RBAC. authorize() must strip the scopes
// extra so the SAR reflects the user's standing permissions. Other extra keys
// must still be forwarded.
func TestAuthorizeDropsTokenScopes(t *testing.T) {
	cs := fake.NewSimpleClientset()
	var captured *authzv1.SubjectAccessReview
	cs.PrependReactor("create", "subjectaccessreviews", func(action ktesting.Action) (bool, runtime.Object, error) {
		sar := action.(ktesting.CreateAction).GetObject().(*authzv1.SubjectAccessReview)
		captured = sar
		sar.Status.Allowed = true
		return true, sar, nil
	})

	a := &Authenticator{
		cs:      cs,
		sar:     config.SubjectAccessReview{Verb: "get", Group: "argoproj.io", Resource: "applications"},
		enabled: true,
	}
	user := authnv1.UserInfo{
		Username: "IAM#rh-ee-jsell",
		Groups:   []string{"ibm-admins", "system:authenticated"},
		Extra: map[string]authnv1.ExtraValue{
			scopesExtraKey:                          {"user:info"},
			"authentication.kubernetes.io/pod-name": {"somepod"},
		},
	}

	allowed, err := a.authorize(context.Background(), user)
	if err != nil {
		t.Fatalf("authorize() error = %v", err)
	}
	if !allowed {
		t.Fatalf("authorize() allowed = false, want true")
	}
	if captured == nil {
		t.Fatal("no SubjectAccessReview was created")
	}
	if _, ok := captured.Spec.Extra[scopesExtraKey]; ok {
		t.Errorf("SAR Extra still carries %q; token scopes must not be forwarded", scopesExtraKey)
	}
	if _, ok := captured.Spec.Extra["authentication.kubernetes.io/pod-name"]; !ok {
		t.Error("SAR Extra dropped a non-scope key; only token scopes should be stripped")
	}
	if captured.Spec.User != user.Username {
		t.Errorf("SAR User = %q, want %q", captured.Spec.User, user.Username)
	}
}
