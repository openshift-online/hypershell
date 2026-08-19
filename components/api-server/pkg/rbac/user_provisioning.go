package rbac

import (
	"context"
	"net/http"

	"github.com/golang-jwt/jwt/v4"
	"github.com/golang/glog"

	"github.com/openshift-online/rh-trex-ai/pkg/auth"
)

type JWTRoleSyncer interface {
	SyncJWTRoles(ctx context.Context, userID string, jwtRoles []string) error
}

type contextKey string

const ContextUserIDKey contextKey = "rbac_user_id"
const ContextJWTRolesKey contextKey = "rbac_jwt_roles"

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
				if syncer != nil {
					if syncErr := syncer.SyncJWTRoles(ctx, userID, jwtRoles); syncErr != nil {
						glog.Warningf("JWT role sync failed for %q: %v", payload.Username, syncErr)
					}
				}
			}

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func extractJWTRoles(r *http.Request) []string {
	token, err := auth.TokenFromContext(r.Context())
	if err != nil {
		return nil
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil
	}

	realmAccess, ok := claims["realm_access"]
	if !ok {
		return nil
	}

	raMap, ok := realmAccess.(map[string]interface{})
	if !ok {
		return nil
	}

	rolesRaw, ok := raMap["roles"]
	if !ok {
		return nil
	}

	rolesSlice, ok := rolesRaw.([]interface{})
	if !ok {
		return nil
	}

	result := make([]string, 0, len(rolesSlice))
	for _, r := range rolesSlice {
		if s, ok := r.(string); ok {
			result = append(result, s)
		}
	}
	return result
}

func GetUserIDFromContext(ctx context.Context) string {
	v := ctx.Value(ContextUserIDKey)
	if v == nil {
		return ""
	}
	return v.(string)
}

func HasPlatformAdminRole(ctx context.Context, userID string) bool {
	if userID == "" {
		return false
	}
	v := ctx.Value(ContextJWTRolesKey)
	if v == nil {
		return false
	}
	jwtRoles, ok := v.([]string)
	if !ok {
		return false
	}
	for _, role := range jwtRoles {
		if role == "platform:admin" {
			return true
		}
	}
	return false
}
