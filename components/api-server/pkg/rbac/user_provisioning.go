package rbac

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/auth"
)

type JWTRoleSyncer interface {
	SyncJWTRoles(ctx context.Context, userID string, jwtRoles []string) error
}

type contextKey string

const ContextUserIDKey contextKey = "rbac_user_id"
const ContextJWTRolesKey contextKey = "rbac_jwt_roles"

// HypershellAdminRole is the legacy Keycloak realm group name. Dashboard-operator
// access requires platform:admin (JWT-synced binding), not this role alone.
const HypershellAdminRole = "hypershell-admins"

type UserProvisioner interface {
	UpsertFromJWT(ctx context.Context, payload *auth.Payload) (userID string, err error)
}

type DailyActivityRecorder interface {
	RecordDailyActivity(ctx context.Context, userID string, at time.Time)
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
				slog.WarnContext(r.Context(), "user provisioning failed", "username", payload.Username, "error", upsertErr)
				next.ServeHTTP(w, r)
				return
			}

			ctx := context.WithValue(r.Context(), ContextUserIDKey, userID)

			jwtRoles := extractJWTRoles(r)
			if len(jwtRoles) > 0 {
				ctx = context.WithValue(ctx, ContextJWTRolesKey, jwtRoles)
			}
			// Always sync even when jwtRoles is empty, so that a role removed
			// from the caller's Keycloak realm roles is also removed from its
			// RoleBindings on the next request.
			if syncer != nil {
				if syncErr := syncer.SyncJWTRoles(ctx, userID, jwtRoles); syncErr != nil {
					slog.WarnContext(ctx, "JWT role sync failed", "username", payload.Username, "error", syncErr)
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
