package rbac

import (
	"context"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc/metadata"

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
	if token, err := auth.TokenFromContext(ctx); err == nil && token != nil {
		if claims, ok := token.Claims.(jwt.MapClaims); ok {
			return extractRealmRolesFromClaims(claims)
		}
		return nil
	}
	// gRPC: the framework's auth interceptor verifies the bearer token but
	// stores only the username, so without this fallback role sync saw no
	// realm roles on gRPC and revoked a caller's platform:admin binding on every
	// gRPC call. The claims are read from the exact metadata value the
	// interceptor verified (the first authorization value, "Bearer " trimmed);
	// callers reach here only after it set a username (provisionUserForGRPC).
	if claims := verifiedGRPCBearerClaims(ctx); claims != nil {
		return extractRealmRolesFromClaims(claims)
	}
	return nil
}

// verifiedGRPCBearerClaims returns the claims of the bearer token in the gRPC
// authorization metadata, parsed the way the framework's
// authenticateGRPCRequest parses it. It does not verify the signature: it is
// only meaningful after that interceptor accepted the call.
func verifiedGRPCBearerClaims(ctx context.Context) jwt.MapClaims {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil
	}
	values := md.Get("authorization")
	if len(values) == 0 {
		return nil
	}
	tokenStr := strings.TrimPrefix(strings.TrimPrefix(values[0], "Bearer "), "bearer ")
	if tokenStr == "" {
		return nil
	}
	parsed, _, err := jwt.NewParser().ParseUnverified(tokenStr, jwt.MapClaims{})
	if err != nil {
		return nil
	}
	claims, _ := parsed.Claims.(jwt.MapClaims)
	return claims
}
