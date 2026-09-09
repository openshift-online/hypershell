package rbac

import (
	"testing"

	"github.com/golang-jwt/jwt/v4"
)

func TestExtractRealmRolesFromClaims(t *testing.T) {
	tests := []struct {
		name   string
		claims jwt.MapClaims
		want   []string
	}{
		{
			name: "reads roles from the roles claim",
			claims: jwt.MapClaims{
				"roles": []interface{}{"hypershell-admins", "hypershell-users"},
			},
			want: []string{"hypershell-admins", "hypershell-users"},
		},
		{
			name: "reads realm roles from the groups claim used by HyperShell Keycloak",
			claims: jwt.MapClaims{
				"groups": []interface{}{"hypershell-admins", "hypershell-users"},
			},
			want: []string{"hypershell-admins", "hypershell-users"},
		},
		{
			name: "normalizes leading slashes on groups claim values",
			claims: jwt.MapClaims{
				"groups": []interface{}{"/hypershell-admins", "/hypershell-users"},
			},
			want: []string{"hypershell-admins", "hypershell-users"},
		},
		{
			name: "prefers the roles claim over groups and realm_access",
			claims: jwt.MapClaims{
				"groups":       []interface{}{"hypershell-users"},
				"realm_access": map[string]interface{}{"roles": []interface{}{"platform:admin"}},
				"roles":        []interface{}{"hypershell-admins"},
			},
			want: []string{"hypershell-admins"},
		},
		{
			name: "falls back to realm_access.roles when roles and groups are absent",
			claims: jwt.MapClaims{
				"realm_access": map[string]interface{}{
					"roles": []interface{}{"platform:admin", "hypershell-users"},
				},
			},
			want: []string{"platform:admin", "hypershell-users"},
		},
		{
			name: "normalizes leading slashes on realm_access.roles values",
			claims: jwt.MapClaims{
				"realm_access": map[string]interface{}{
					"roles": []interface{}{"/hypershell-admins"},
				},
			},
			want: []string{"hypershell-admins"},
		},
		{
			name: "ignores non-string role entries",
			claims: jwt.MapClaims{
				"roles": []interface{}{"hypershell-admins", 42, nil},
			},
			want: []string{"hypershell-admins"},
		},
		{
			name:   "returns nil when no role claims are present",
			claims: jwt.MapClaims{"sub": "user-1"},
			want:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractRealmRolesFromClaims(tt.claims)
			if !stringSlicesEqual(got, tt.want) {
				t.Fatalf("extractRealmRolesFromClaims() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHasHypershellAdminRole_FromGroupsClaim(t *testing.T) {
	roles := extractRealmRolesFromClaims(jwt.MapClaims{
		"groups": []interface{}{"/hypershell-admins"},
	})
	if !HasHypershellAdminRole(roles) {
		t.Fatal("hypershell-admins in groups claim should grant dashboard-operator access")
	}
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
