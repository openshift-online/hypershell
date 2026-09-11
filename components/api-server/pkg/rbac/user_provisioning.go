package rbac

import (
	"context"
	"net/http"

	"github.com/golang/glog"

	"github.com/openshift-online/rh-trex-ai/pkg/auth"
)

type JWTRoleSyncer interface {
	SyncJWTRoles(ctx context.Context, userID string, jwtRoles []string) error
}

type contextKey string

const ContextUserIDKey contextKey = "rbac_user_id"
const ContextJWTRolesKey contextKey = "rbac_jwt_roles"

// HypershellAdminRole is the Keycloak realm role that grants dashboard-operator access.
const HypershellAdminRole = "hypershell-admins"

type UserProvisioner interface {
	UpsertFromJWT(ctx context.Context, payload *auth.Payload) (userID string, err error)
}

func UserProvisioningMiddleware(provisioner UserProvisioner, syncer JWTRoleSyncer) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			payload, err := auth.GetAuthPayload(r)
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}

			if payload.Username == "" {
				next.ServeHTTP(w, r)
				return
			}

			userID, upsertErr := provisioner.UpsertFromJWT(r.Context(), payload)
			if upsertErr != nil {
				glog.Warningf("user provisioning failed for %q: %v", payload.Username, upsertErr)
				next.ServeHTTP(w, r)
				return
			}

			ctx := context.WithValue(r.Context(), ContextUserIDKey, userID)

			jwtRoles := extractJWTRoles(r)
			if len(jwtRoles) > 0 {
				ctx = context.WithValue(ctx, ContextJWTRolesKey, jwtRoles)
			}
			// Always sync even when jwtRoles is empty: SyncJWTRoles applies
			// configured default roles (e.g. gateway:creator) so that users with
			// no Keycloak realm roles still receive their initial bindings.
			if syncer != nil {
				if syncErr := syncer.SyncJWTRoles(ctx, userID, jwtRoles); syncErr != nil {
					glog.Warningf("JWT role sync failed for %q: %v", payload.Username, syncErr)
				}
			}

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func GetUserIDFromContext(ctx context.Context) string {
	v := ctx.Value(ContextUserIDKey)
	if v == nil {
		return ""
	}
	return v.(string)
}

func GetJWTRolesFromContext(ctx context.Context) []string {
	v := ctx.Value(ContextJWTRolesKey)
	if v == nil {
		return nil
	}
	roles, ok := v.([]string)
	if !ok {
		return nil
	}
	return roles
}

func HasHypershellAdminRole(jwtRoles []string) bool {
	for _, role := range jwtRoles {
		if role == HypershellAdminRole {
			return true
		}
	}
	return false
}

func HasPlatformAdminRole(ctx context.Context, userID string) bool {
	if userID == "" {
		return false
	}
	for _, role := range GetJWTRolesFromContext(ctx) {
		if role == "platform:admin" {
			return true
		}
	}
	return false
}
