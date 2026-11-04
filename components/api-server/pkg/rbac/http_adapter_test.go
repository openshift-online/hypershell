package rbac

import (
	"net/http"

	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/authz"
)

// AuthorizeApi adapts the Authorizer to an http middleware for the tests that
// drive the REST rules through a router. Status codes follow TRex's
// authz.HTTPMiddleware: deny 403, hidden deny 404, error 503.
func (a *Authorizer) AuthorizeApi(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		d, err := a.Authorize(r.Context(), authz.Request{Subject: authz.SubjectFromContext(r.Context()), HTTP: r})
		switch {
		case err != nil:
			w.WriteHeader(http.StatusServiceUnavailable)
		case !d.Allowed && d.HideExistence:
			w.WriteHeader(http.StatusNotFound)
		case !d.Allowed:
			w.WriteHeader(http.StatusForbidden)
		default:
			next.ServeHTTP(w, r)
		}
	})
}
