package serviceaccountkeycloak

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	jwt "github.com/golang-jwt/jwt/v4"
)

func TestProvisionServiceAccountCreatesLeastPrivilegeClientAndVerifiesToken(t *testing.T) {
	for _, test := range []struct {
		name      string
		role      string
		wantRoles []string
	}{
		{name: "user", role: RoleUser, wantRoles: []string{RoleUser}},
		{name: "admin", role: RoleAdmin, wantRoles: []string{RoleAdmin, RoleUser}},
	} {
		t.Run(test.name, func(t *testing.T) {
			fake := newServiceAccountKeycloak(t, test.wantRoles, false)
			client := NewClient(fake.server.URL, "realm", "provisioner", "admin-secret")
			spec := fake.spec(test.role)

			result, err := client.ProvisionServiceAccount(t.Context(), spec)
			if err != nil {
				t.Fatalf("ProvisionServiceAccount() error = %v", err)
			}
			if result.ClientUUID != fake.serviceUUID || result.Subject != fake.subject || result.ClientSecret != fake.secret {
				t.Fatalf("result = %#v", result)
			}

			fake.mu.Lock()
			defer fake.mu.Unlock()
			if enabled, _ := fake.created["enabled"].(bool); enabled {
				t.Error("client must be created disabled")
			}
			for _, field := range []string{"standardFlowEnabled", "implicitFlowEnabled", "directAccessGrantsEnabled", "authorizationServicesEnabled", "fullScopeAllowed"} {
				if enabled, _ := fake.created[field].(bool); enabled {
					t.Errorf("%s must be false", field)
				}
			}
			if enabled, _ := fake.created["serviceAccountsEnabled"].(bool); !enabled {
				t.Error("serviceAccountsEnabled must be true")
			}
			if scopes, ok := fake.created["defaultClientScopes"].([]any); !ok || len(scopes) != 0 {
				t.Errorf("defaultClientScopes = %#v, want empty", fake.created["defaultClientScopes"])
			}
			attrs, _ := fake.created["attributes"].(map[string]any)
			if attrs[managedAttribute] != "true" || attrs[gatewayIDAttribute] != "gateway-id" || attrs[serviceAccountIDAttribute] != "resource-id" {
				t.Errorf("managed attributes = %#v", attrs)
			}
			if !reflect.DeepEqual(roleNames(fake.userRoles), test.wantRoles) {
				t.Errorf("user roles = %v, want %v", roleNames(fake.userRoles), test.wantRoles)
			}
			if !reflect.DeepEqual(roleNames(fake.scopeRoles), test.wantRoles) {
				t.Errorf("scope roles = %v, want %v", roleNames(fake.scopeRoles), test.wantRoles)
			}
			if !reflect.DeepEqual(roleNames(fake.removedUserRealmRoles), []string{"unexpected-realm-role"}) {
				t.Errorf("removed user realm roles = %v", roleNames(fake.removedUserRealmRoles))
			}
			if !reflect.DeepEqual(roleNames(fake.removedScopeRealmRoles), []string{"unexpected-realm-role"}) {
				t.Errorf("removed scope realm roles = %v", roleNames(fake.removedScopeRealmRoles))
			}
			if len(fake.mappers) != 2 {
				t.Fatalf("mappers = %d, want 2", len(fake.mappers))
			}
			mapperByName := map[string]map[string]any{}
			for _, mapper := range fake.mappers {
				name, _ := mapper["name"].(string)
				mapperByName[name] = mapper
			}
			audienceConfig, _ := mapperByName["gateway-audience"]["config"].(map[string]any)
			if audienceConfig["included.client.audience"] != "gateway-client" {
				t.Errorf("audience mapper = %#v", audienceConfig)
			}
			rolesConfig, _ := mapperByName["gateway-client-roles"]["config"].(map[string]any)
			if rolesConfig["claim.name"] != "hypershell.roles" || rolesConfig["usermodel.clientRoleMapping.clientId"] != "gateway-client" {
				t.Errorf("roles mapper = %#v", rolesConfig)
			}
			if !fake.enabled {
				t.Error("client was not enabled after verification")
			}
		})
	}
}

func TestProvisionServiceAccountDoesNotLeakSecretOrProviderBody(t *testing.T) {
	fake := newServiceAccountKeycloak(t, []string{RoleUser}, true)
	client := NewClient(fake.server.URL, "realm", "provisioner", "admin-secret")
	_, err := client.ProvisionServiceAccount(t.Context(), fake.spec(RoleUser))
	if err == nil {
		t.Fatal("expected token verification to fail")
	}
	message := err.Error()
	for _, forbidden := range []string{fake.secret, "provider-sensitive-body", "access_token"} {
		if strings.Contains(message, forbidden) {
			t.Errorf("error leaked %q: %s", forbidden, message)
		}
	}
	if !fake.deleted {
		t.Error("failed provisioning must delete the partial client")
	}
}

func TestProvisionServiceAccountRejectsAnUnexpectedTokenLifetime(t *testing.T) {
	fake := newServiceAccountKeycloak(t, []string{RoleUser}, false)
	fake.tokenLifetime = 60
	client := NewClient(fake.server.URL, "realm", "provisioner", "admin-secret")

	if _, err := client.ProvisionServiceAccount(t.Context(), fake.spec(RoleUser)); err == nil {
		t.Fatal("ProvisionServiceAccount() error = nil, want lifetime verification failure")
	}
	if !fake.deleted {
		t.Error("failed lifetime verification must delete the partial client")
	}
}

func TestProvisionServiceAccountRejectsACrossGatewayAudience(t *testing.T) {
	fake := newServiceAccountKeycloak(t, []string{RoleUser}, false)
	fake.tokenAudience = "another-gateway-client"
	client := NewClient(fake.server.URL, "realm", "provisioner", "admin-secret")

	if _, err := client.ProvisionServiceAccount(t.Context(), fake.spec(RoleUser)); err == nil {
		t.Fatal("ProvisionServiceAccount() error = nil, want audience verification failure")
	}
	if !fake.deleted {
		t.Error("failed audience verification must delete the partial client")
	}
}

func TestProvisionedCredentialsCanRenewShortLivedAccessTokens(t *testing.T) {
	fake := newServiceAccountKeycloak(t, []string{RoleUser}, false)
	client := NewClient(fake.server.URL, "realm", "provisioner", "admin-secret")
	result, err := client.ProvisionServiceAccount(t.Context(), fake.spec(RoleUser))
	if err != nil {
		t.Fatalf("ProvisionServiceAccount() error = %v", err)
	}

	var previous string
	for range 2 {
		form := url.Values{
			"client_id":     {result.ClientID},
			"client_secret": {result.ClientSecret},
			"grant_type":    {"client_credentials"},
		}
		response, postErr := http.PostForm(fake.server.URL+"/realms/realm/protocol/openid-connect/token", form)
		if postErr != nil {
			t.Fatalf("Client Credentials grant failed: %v", postErr)
		}
		var tokenResponse struct {
			AccessToken string `json:"access_token"`
			ExpiresIn   int64  `json:"expires_in"`
		}
		decodeErr := json.NewDecoder(response.Body).Decode(&tokenResponse)
		_ = response.Body.Close()
		if response.StatusCode != http.StatusOK || decodeErr != nil || tokenResponse.AccessToken == "" || tokenResponse.ExpiresIn != 300 {
			t.Fatalf("renewal response status=%d decodeErr=%v expiresIn=%d", response.StatusCode, decodeErr, tokenResponse.ExpiresIn)
		}
		if tokenResponse.AccessToken == previous {
			t.Error("renewal returned the prior access token")
		}
		previous = tokenResponse.AccessToken
	}

	fake.mu.Lock()
	grants := fake.serviceTokenGrants
	fake.mu.Unlock()
	if grants != 3 {
		t.Fatalf("service-account token grants = %d, want initial verification plus two renewals", grants)
	}
}

func TestDeleteGatewayServiceAccountsDisablesEveryClientBeforeDeletion(t *testing.T) {
	clientIDs := []string{"service-1", "service-2"}
	events := make([]string, 0, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/realms/realm/protocol/openid-connect/token":
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "admin-token", "expires_in": 300})
		case r.URL.Path == "/admin/realms/realm/clients" && r.Method == http.MethodGet:
			page := make([]kcClient, 0, len(clientIDs))
			for _, id := range clientIDs {
				page = append(page, kcClient{ID: id, ClientID: id})
			}
			_ = json.NewEncoder(w).Encode(page)
		case strings.HasPrefix(r.URL.Path, "/admin/realms/realm/clients/"):
			id := strings.TrimPrefix(r.URL.Path, "/admin/realms/realm/clients/")
			switch r.Method {
			case http.MethodGet:
				_ = json.NewEncoder(w).Encode(map[string]any{
					"id": id, "clientId": id, "enabled": true,
					"attributes": map[string]string{
						managedAttribute: "true", gatewayIDAttribute: "gateway-id",
						serviceAccountIDAttribute: "resource-" + id,
					},
				})
			case http.MethodPut:
				var representation map[string]any
				if err := json.NewDecoder(r.Body).Decode(&representation); err != nil {
					t.Errorf("decode disable request: %v", err)
				}
				if enabled, _ := representation["enabled"].(bool); enabled {
					t.Errorf("client %s was not disabled", id)
				}
				events = append(events, "disable:"+id)
				w.WriteHeader(http.StatusNoContent)
			case http.MethodDelete:
				if len(events) < len(clientIDs) ||
					!strings.HasPrefix(events[0], "disable:") ||
					!strings.HasPrefix(events[1], "disable:") {
					t.Errorf("deleted %s before every client was disabled: %v", id, events)
				}
				events = append(events, "delete:"+id)
				w.WriteHeader(http.StatusNoContent)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	client := NewClient(server.URL, "realm", "provisioner", "admin-secret")
	if err := client.DeleteGatewayServiceAccounts(t.Context(), "gateway-id"); err != nil {
		t.Fatalf("DeleteGatewayServiceAccounts() error = %v", err)
	}
	want := []string{
		"disable:service-1", "disable:service-2",
		"delete:service-1", "delete:service-2",
	}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("lifecycle events = %v, want %v", events, want)
	}
}

type serviceAccountKeycloak struct {
	t                      *testing.T
	server                 *httptest.Server
	serviceUUID            string
	gatewayUUID            string
	subject                string
	secret                 string
	wantRoles              []string
	failGrant              bool
	tokenLifetime          int64
	tokenAudience          string
	serviceTokenGrants     int
	mu                     sync.Mutex
	created                map[string]any
	userRoles              []kcRole
	scopeRoles             []kcRole
	removedUserRealmRoles  []kcRole
	removedScopeRealmRoles []kcRole
	mappers                []map[string]any
	enabled                bool
	deleted                bool
}

func newServiceAccountKeycloak(t *testing.T, wantRoles []string, failGrant bool) *serviceAccountKeycloak {
	fake := &serviceAccountKeycloak{
		t: t, serviceUUID: "service-uuid", gatewayUUID: "gateway-uuid", subject: "service-subject",
		secret: "one-time-client-secret", wantRoles: wantRoles, failGrant: failGrant,
		tokenLifetime: 300, tokenAudience: "gateway-client",
	}
	fake.server = httptest.NewServer(http.HandlerFunc(fake.serveHTTP))
	t.Cleanup(fake.server.Close)
	return fake
}

func (f *serviceAccountKeycloak) spec(role string) ServiceAccountSpec {
	return ServiceAccountSpec{
		ClientID: "hs-sa-gateway-id-resource-id", DisplayName: "deploy bot",
		GatewayClientID: "gateway-client", GatewayID: "gateway-id", ServiceAccountID: "resource-id",
		CreatorUserID: "creator-id", Role: role, ExpectedIssuer: f.server.URL + "/realms/realm",
		AccessTokenLifetimeSeconds: 300,
	}
}

func (f *serviceAccountKeycloak) serveHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/realms/realm/protocol/openid-connect/token" {
		f.token(w, r)
		return
	}
	switch {
	case r.URL.Path == "/admin/realms/realm/clients" && r.Method == http.MethodGet:
		_ = json.NewEncoder(w).Encode([]kcClient{{ID: f.gatewayUUID, ClientID: "gateway-client"}})
	case r.URL.Path == "/admin/realms/realm/clients" && r.Method == http.MethodPost:
		f.decode(r, &f.created)
		w.Header().Set("Location", "/admin/realms/realm/clients/"+f.serviceUUID)
		w.WriteHeader(http.StatusCreated)
	case r.URL.Path == "/admin/realms/realm/clients/"+f.gatewayUUID+"/roles":
		_ = json.NewEncoder(w).Encode([]kcRole{{ID: "admin-id", Name: RoleAdmin}, {ID: "user-id", Name: RoleUser}})
	case r.URL.Path == "/admin/realms/realm/clients/"+f.serviceUUID+"/service-account-user":
		_ = json.NewEncoder(w).Encode(kcUser{ID: f.subject})
	case r.URL.Path == "/admin/realms/realm/users/"+f.subject+"/role-mappings" && r.Method == http.MethodGet:
		_ = json.NewEncoder(w).Encode(map[string]any{
			"realmMappings":  []kcRole{{ID: "unexpected-realm-id", Name: "unexpected-realm-role"}},
			"clientMappings": map[string]any{},
		})
	case r.URL.Path == "/admin/realms/realm/users/"+f.subject+"/role-mappings/realm" && r.Method == http.MethodDelete:
		f.mu.Lock()
		f.decode(r, &f.removedUserRealmRoles)
		f.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	case r.URL.Path == "/admin/realms/realm/users/"+f.subject+"/role-mappings/clients/"+f.gatewayUUID && r.Method == http.MethodPost:
		f.decode(r, &f.userRoles)
		w.WriteHeader(http.StatusNoContent)
	case r.URL.Path == "/admin/realms/realm/clients/"+f.serviceUUID+"/scope-mappings" && r.Method == http.MethodGet:
		_ = json.NewEncoder(w).Encode(map[string]any{
			"realmMappings":  []kcRole{{ID: "unexpected-realm-id", Name: "unexpected-realm-role"}},
			"clientMappings": map[string]any{},
		})
	case r.URL.Path == "/admin/realms/realm/clients/"+f.serviceUUID+"/scope-mappings/realm" && r.Method == http.MethodDelete:
		f.mu.Lock()
		f.decode(r, &f.removedScopeRealmRoles)
		f.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	case r.URL.Path == "/admin/realms/realm/clients/"+f.serviceUUID+"/scope-mappings/clients/"+f.gatewayUUID && r.Method == http.MethodPost:
		f.decode(r, &f.scopeRoles)
		w.WriteHeader(http.StatusNoContent)
	case r.URL.Path == "/admin/realms/realm/clients/"+f.serviceUUID+"/protocol-mappers/models" && r.Method == http.MethodGet:
		_, _ = io.WriteString(w, `[]`)
	case r.URL.Path == "/admin/realms/realm/clients/"+f.serviceUUID+"/protocol-mappers/models" && r.Method == http.MethodPost:
		var mapper map[string]any
		f.decode(r, &mapper)
		f.mu.Lock()
		f.mappers = append(f.mappers, mapper)
		f.mu.Unlock()
		w.WriteHeader(http.StatusCreated)
	case r.URL.Path == "/admin/realms/realm/clients/"+f.serviceUUID+"/client-secret":
		_ = json.NewEncoder(w).Encode(map[string]string{"value": f.secret})
	case r.URL.Path == "/admin/realms/realm/clients/"+f.serviceUUID && r.Method == http.MethodGet:
		f.mu.Lock()
		created := f.created
		f.mu.Unlock()
		created["id"] = f.serviceUUID
		_ = json.NewEncoder(w).Encode(created)
	case r.URL.Path == "/admin/realms/realm/clients/"+f.serviceUUID && r.Method == http.MethodPut:
		var rep map[string]any
		f.decode(r, &rep)
		f.mu.Lock()
		f.enabled, _ = rep["enabled"].(bool)
		f.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	case r.URL.Path == "/admin/realms/realm/clients/"+f.serviceUUID && r.Method == http.MethodDelete:
		f.mu.Lock()
		f.deleted = true
		f.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, fmt.Sprintf("unexpected %s %s", r.Method, r.URL.Path), http.StatusNotFound)
	}
}

func (f *serviceAccountKeycloak) token(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	form, _ := url.ParseQuery(string(body))
	if form.Get("client_id") == "provisioner" {
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "admin-token", "expires_in": 300})
		return
	}
	if f.failGrant {
		http.Error(w, "provider-sensitive-body "+f.secret, http.StatusBadRequest)
		return
	}
	if form.Get("client_id") != "hs-sa-gateway-id-resource-id" || form.Get("client_secret") != f.secret {
		http.Error(w, "invalid client credentials", http.StatusUnauthorized)
		return
	}
	f.mu.Lock()
	f.serviceTokenGrants++
	grant := f.serviceTokenGrants
	audience := f.tokenAudience
	wantRoles := append([]string(nil), f.wantRoles...)
	f.mu.Unlock()
	now := time.Now().Unix() + int64(grant)
	claims := jwt.MapClaims{
		"iss": f.server.URL + "/realms/realm", "sub": f.subject,
		"azp": "hs-sa-gateway-id-resource-id", "aud": []string{audience},
		"hypershell": map[string]any{"roles": wantRoles}, "iat": now, "exp": now + f.tokenLifetime,
	}
	token, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("test-key"))
	_ = json.NewEncoder(w).Encode(map[string]any{"access_token": token, "expires_in": f.tokenLifetime})
}

func (f *serviceAccountKeycloak) decode(r *http.Request, target any) {
	f.t.Helper()
	if err := json.NewDecoder(r.Body).Decode(target); err != nil {
		f.t.Errorf("decode %s %s: %v", r.Method, r.URL.Path, err)
	}
}

func roleNames(roles []kcRole) []string {
	result := make([]string, 0, len(roles))
	for _, role := range roles {
		result = append(result, role.Name)
	}
	return result
}
