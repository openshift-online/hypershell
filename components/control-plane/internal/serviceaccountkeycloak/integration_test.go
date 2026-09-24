package serviceaccountkeycloak

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"slices"
	"testing"
	"time"

	jwt "github.com/golang-jwt/jwt/v4"
	"github.com/openshift-online/hypershell/components/control-plane/internal/keycloak"
)

// Run only against a disposable Keycloak realm. The provisioner needs its usual
// client/user management permissions; the test creates and removes its own
// gateway clients and service accounts, leaving existing identities untouched.
func TestIntegrationGatewayServiceAccountIsolation(t *testing.T) {
	base := os.Getenv("KEYCLOAK_INTEGRATION_URL")
	if base == "" {
		t.Skip("set KEYCLOAK_INTEGRATION_URL and provisioner credentials for a disposable realm")
	}
	realm := os.Getenv("KEYCLOAK_INTEGRATION_REALM")
	clientID := os.Getenv("KEYCLOAK_INTEGRATION_CLIENT_ID")
	secret := os.Getenv("KEYCLOAK_INTEGRATION_CLIENT_SECRET")
	if realm == "" || clientID == "" || secret == "" {
		t.Fatal("integration realm and provisioner credentials are required")
	}
	management := keycloak.NewClient(base, realm, clientID, secret)
	client := NewClient(base, realm, clientID, secret)
	prefix := fmt.Sprintf("integration-%d", time.Now().UnixNano())
	for _, suffix := range []string{"a", "b"} {
		gateway := prefix + "-" + suffix
		if _, err := management.ProvisionGatewayClient(t.Context(), gateway); err != nil {
			t.Fatalf("provision gateway: %v", err)
		}
		t.Cleanup(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := management.DeleteGatewayClient(ctx, gateway); err != nil {
				t.Errorf("delete integration gateway: %v", err)
			}
		})
	}
	for _, tc := range []struct {
		suffix string
		role   string
	}{
		{suffix: "a", role: RoleAdmin},
		{suffix: "b", role: RoleUser},
	} {
		t.Run(tc.suffix, func(t *testing.T) {
			gateway := prefix + "-" + tc.suffix
			spec := ServiceAccountSpec{
				ClientID: gateway + "-machine", DisplayName: "Integration machine", GatewayClientID: gateway,
				GatewayID: gateway, ServiceAccountID: gateway + "-account", CreatorUserID: "integration-creator",
				Role: tc.role, ExpectedIssuer: base + "/realms/" + realm, AccessTokenLifetimeSeconds: 300,
			}
			account, err := client.ProvisionServiceAccount(t.Context(), spec)
			if err != nil {
				t.Fatalf("provision machine: %v", err)
			}
			t.Cleanup(func() {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				if err := client.DeleteServiceAccount(ctx, account.ClientUUID, spec.GatewayID, spec.ServiceAccountID); err != nil {
					t.Errorf("delete integration machine: %v", err)
				}
			})
			other := prefix + "-a"
			if tc.suffix == "a" {
				other = prefix + "-b"
			}
			// Even an accidental role assignment on another gateway must not
			// escape this client's role scope and gateway-specific mapper.
			otherUUID, otherRoles, err := client.resolveGatewayRoles(t.Context(), other, RoleAdmin)
			if err != nil {
				t.Fatal("resolve other gateway roles")
			}
			body, _ := json.Marshal(otherRoles)
			_, status, err := client.admin(t.Context(), http.MethodPost,
				fmt.Sprintf("/admin/realms/%s/users/%s/role-mappings/clients/%s", realm, account.Subject, otherUUID), body)
			if err != nil || status != http.StatusNoContent {
				t.Fatal("assign other gateway roles")
			}
			grant := func(audience string) (int, jwt.MapClaims) {
				t.Helper()
				httpClient := &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
				response, err := httpClient.PostForm(base+"/realms/"+realm+"/protocol/openid-connect/token", url.Values{
					"grant_type": {"client_credentials"}, "client_id": {account.ClientID},
					"client_secret": {account.ClientSecret}, "audience": {audience},
				})
				if err != nil {
					t.Fatal("token request failed")
				}
				defer response.Body.Close()
				if response.StatusCode != http.StatusOK {
					return response.StatusCode, nil
				}
				var result struct {
					AccessToken string `json:"access_token"`
				}
				if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
					t.Fatal("invalid token response")
				}
				claims := jwt.MapClaims{}
				if _, _, err := new(jwt.Parser).ParseUnverified(result.AccessToken, claims); err != nil {
					t.Fatal("invalid token payload")
				}
				return response.StatusCode, claims
			}
			// A client_credentials caller cannot select another gateway's audience
			// or inherit an admin role from a different client. Keycloak may ignore
			// this audience parameter, but the issued token must remain constrained.
			for _, requested := range []string{gateway, other, "hypershell-frontend"} {
				status, claims := grant(requested)
				audience, _ := json.Marshal(claims["aud"])
				singleAudience := string(audience) == fmt.Sprintf("%q", gateway) || string(audience) == fmt.Sprintf("[%q]", gateway)
				if status != http.StatusOK || !singleAudience || !claims.VerifyAudience(gateway, true) ||
					claims.VerifyAudience(other, true) || claims.VerifyAudience("hypershell-frontend", true) ||
					claims["sub"] != account.Subject {
					t.Fatal("gateway token audience or subject isolation failed")
				}
				roles, ok := claims["hypershell"].(map[string]any)
				if !ok {
					t.Fatal("gateway token has no role claim")
				}
				encoded, _ := json.Marshal(roles["roles"])
				var actualRoles []string
				if err := json.Unmarshal(encoded, &actualRoles); err != nil {
					t.Fatal("invalid gateway roles")
				}
				slices.Sort(actualRoles)
				wantRoles := []string{RoleUser}
				if tc.role == RoleAdmin {
					wantRoles = []string{RoleAdmin, RoleUser}
				}
				if !slices.Equal(actualRoles, wantRoles) {
					t.Fatal("gateway token role isolation failed")
				}
			}
			if err := client.DisableServiceAccount(t.Context(), account.ClientUUID, spec.GatewayID, spec.ServiceAccountID); err != nil {
				t.Fatalf("revoke machine: %v", err)
			}
			if status, _ := grant(gateway); status != http.StatusUnauthorized && status != http.StatusBadRequest {
				t.Fatal("revoked machine can still obtain a token")
			}
		})
	}
}
