// Package auth implements the BFF's backend defense-in-depth gate
// (data-architecture.spec §3.4): even though an oauth-proxy sidecar
// authenticates the request at the edge, the backend independently validates the
// forwarded bearer token (TokenReview) and authorizes it (SubjectAccessReview).
// The SAR attributes are supplied by config, so no admin group/resource identity
// is compiled into the binary (§3.5).
package auth

import (
	"context"
	"net/http"
	"strings"
	"time"

	authnv1 "k8s.io/api/authentication/v1"
	authzv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/openshift-online/hypershell/components/fleet-dashboard/pkg/config"
)

// Authenticator validates and authorizes forwarded identities.
type Authenticator struct {
	cs      kubernetes.Interface
	sar     config.SubjectAccessReview
	enabled bool
}

// New builds an Authenticator. When disabled (local dev), Middleware is a no-op.
func New(c *config.Config, cs kubernetes.Interface) *Authenticator {
	return &Authenticator{cs: cs, sar: c.SAR, enabled: c.AuthEnabled}
}

// Middleware wraps a handler, rejecting requests whose forwarded token fails
// TokenReview or SubjectAccessReview.
func (a *Authenticator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !a.enabled {
			next.ServeHTTP(w, r)
			return
		}
		token := bearerToken(r)
		if token == "" {
			http.Error(w, "missing bearer token", http.StatusUnauthorized)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		user, ok, err := a.review(ctx, token)
		if err != nil {
			http.Error(w, "authentication backend error", http.StatusBadGateway)
			return
		}
		if !ok {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}
		allowed, err := a.authorize(ctx, user)
		if err != nil {
			http.Error(w, "authorization backend error", http.StatusBadGateway)
			return
		}
		if !allowed {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *Authenticator) review(ctx context.Context, token string) (authnv1.UserInfo, bool, error) {
	tr := &authnv1.TokenReview{Spec: authnv1.TokenReviewSpec{Token: token}}
	res, err := a.cs.AuthenticationV1().TokenReviews().Create(ctx, tr, metav1.CreateOptions{})
	if err != nil {
		return authnv1.UserInfo{}, false, err
	}
	return res.Status.User, res.Status.Authenticated, nil
}

func (a *Authenticator) authorize(ctx context.Context, user authnv1.UserInfo) (bool, error) {
	extra := map[string]authzv1.ExtraValue{}
	for k, v := range user.Extra {
		extra[k] = authzv1.ExtraValue(v)
	}
	sar := &authzv1.SubjectAccessReview{
		Spec: authzv1.SubjectAccessReviewSpec{
			User:   user.Username,
			UID:    user.UID,
			Groups: user.Groups,
			Extra:  extra,
			ResourceAttributes: &authzv1.ResourceAttributes{
				Verb:      a.sar.Verb,
				Group:     a.sar.Group,
				Resource:  a.sar.Resource,
				Namespace: a.sar.Namespace,
				Name:      a.sar.Name,
			},
		},
	}
	res, err := a.cs.AuthorizationV1().SubjectAccessReviews().Create(ctx, sar, metav1.CreateOptions{})
	if err != nil {
		return false, err
	}
	return res.Status.Allowed, nil
}

// forwardedAccessTokenHeader is where the OpenShift oauth-proxy sidecar places
// the user's OAuth access token when started with --pass-access-token. This is
// the ONLY header that carries the token for browser (cookie-session) requests:
// --pass-user-bearer-token re-emits an Authorization: Bearer header only when the
// INCOMING request already carried one (i.e. programmatic bearer callers), which a
// browser session never does. Reading Authorization alone therefore 401s every
// browser user -- including cluster-admins -- which surfaced as every panel
// showing "Could not load this view". Accept both: Authorization: Bearer for
// direct bearer callers, X-Forwarded-Access-Token for proxied browser sessions.
const forwardedAccessTokenHeader = "X-Forwarded-Access-Token"

func bearerToken(r *http.Request) string {
	if h := r.Header.Get("Authorization"); h != "" {
		const prefix = "Bearer "
		if len(h) > len(prefix) && strings.EqualFold(h[:len(prefix)], prefix) {
			return strings.TrimSpace(h[len(prefix):])
		}
	}
	return strings.TrimSpace(r.Header.Get(forwardedAccessTokenHeader))
}
