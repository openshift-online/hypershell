package rbac

import (
	"context"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v4"

	"github.com/openshift-online/rh-trex-ai/pkg/auth"
)

// extractRealmRolesFromClaims reads realm roles from OIDC claims emitted by HyperShell Keycloak.
//
// The hypershell-frontend client maps realm roles into the top-level groups claim on
// tokens via oidc-usermodel-realm-role-mapper. Access tokens may also carry
// realm_access.roles. The roles claim is honored when present. Group-path values such
// as /hypershell-admins are normalized by stripping a leading slash.
//
// This mirrors components/web-console/bff/src/auth.ts extractRealmRoles so the API
// server and BFF agree on dashboard-operator access for the same bearer token.
func extractRealmRolesFromClaims(claims jwt.MapClaims) []string {
	if roles, ok := readStringClaim(claims, "roles"); ok {
		return roles
	}

	if groups, ok := readStringClaim(claims, "groups"); ok {
		return groups
	}

	return readRealmAccessRoles(claims)
}

func readStringClaim(claims jwt.MapClaims, claimName string) ([]string, bool) {
	raw, ok := claims[claimName]
	if !ok {
		return nil, false
	}

	rolesSlice, ok := raw.([]interface{})
	if !ok {
		return nil, false
	}

	result := make([]string, 0, len(rolesSlice))
	for _, role := range rolesSlice {
		if s, ok := role.(string); ok {
			result = append(result, normalizeRoleName(s))
		}
	}
	return result, true
}

func readRealmAccessRoles(claims jwt.MapClaims) []string {
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
	for _, role := range rolesSlice {
		if s, ok := role.(string); ok {
			result = append(result, normalizeRoleName(s))
		}
	}
	return result
}

func normalizeRoleName(role string) string {
	if strings.HasPrefix(role, "/") {
		return role[1:]
	}
	return role
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

	return extractRealmRolesFromClaims(claims)
}

func extractJWTRolesFromContext(ctx context.Context) []string {
	token, err := auth.TokenFromContext(ctx)
	if err != nil {
		return nil
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil
	}

	return extractRealmRolesFromClaims(claims)
}
