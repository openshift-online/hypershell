package serviceAccounts

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/openshift-online/hypershell/components/api-server/pkg/keycloak"
	"github.com/openshift-online/hypershell/components/api-server/pkg/rbac"
	"github.com/openshift-online/hypershell/components/api-server/plugins/gateways"
	"github.com/openshift-online/rh-trex-ai/pkg/db"
	trexerrors "github.com/openshift-online/rh-trex-ai/pkg/errors"
	"gorm.io/gorm"
)

func TestCreateHandlerReturnsTheCredentialOnceWithCacheProtection(t *testing.T) {
	dao := newMemoryDAO()
	svc := newTestService(dao, &fakeKeycloak{configured: true}, testBindings("creator", "gateway:owner"), time.Now().UTC())
	handler := NewHandler(svc)
	body := bytes.NewBufferString(`{"name":"deploy-bot","role":"openshell-user"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/hypershell/v1/gateways/gateway-id/service_accounts", body)
	request = mux.SetURLVars(request, map[string]string{"gateway_id": "gateway-id"})
	request = request.WithContext(context.WithValue(request.Context(), rbac.ContextUserIDKey, "creator"))
	recorder := httptest.NewRecorder()

	handler.Create(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if recorder.Header().Get("Cache-Control") != "no-store" || recorder.Header().Get("Pragma") != "no-cache" {
		t.Fatalf("cache headers = %#v", recorder.Header())
	}
	var created map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	credential, ok := created["credential"].(map[string]any)
	if !ok || credential["client_secret"] != "one-time-secret" {
		t.Fatalf("create credential = %#v", created["credential"])
	}
	if credential["gateway_endpoint"] != "https://gateway.example:443" {
		t.Fatalf("gateway endpoint = %q, want HTTPS endpoint", credential["gateway_endpoint"])
	}

	getRequest := httptest.NewRequest(http.MethodGet, "/api/hypershell/v1/gateways/gateway-id/service_accounts/"+created["id"].(string), nil)
	getRequest = mux.SetURLVars(getRequest, map[string]string{
		"gateway_id": "gateway-id", "service_account_id": created["id"].(string),
	})
	getRequest = getRequest.WithContext(context.WithValue(getRequest.Context(), rbac.ContextUserIDKey, "creator"))
	getRecorder := httptest.NewRecorder()
	handler.Get(getRecorder, getRequest)
	if getRecorder.Code != http.StatusOK {
		t.Fatalf("get status = %d, body = %s", getRecorder.Code, getRecorder.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(getRecorder.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode get response: %v", err)
	}
	if _, exists := got["credential"]; exists {
		t.Fatalf("get response leaked credential: %#v", got["credential"])
	}
}

func TestGatewayEndpointNormalizesGRPCSchemesForOpenShell(t *testing.T) {
	for _, test := range []struct {
		name string
		in   string
		want string
	}{
		{name: "TLS", in: "grpcs://gateway.example:443", want: "https://gateway.example:443"},
		{name: "plaintext", in: "grpc://gateway.example:80", want: "http://gateway.example:80"},
		{name: "already HTTP", in: "https://gateway.example:443", want: "https://gateway.example:443"},
		{name: "trimmed", in: "  grpcs://gateway.example:443  ", want: "https://gateway.example:443"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := gatewayEndpoint(test.in); got != test.want {
				t.Fatalf("gatewayEndpoint(%q) = %q, want %q", test.in, got, test.want)
			}
		})
	}
}

func TestCreateEnforcesRoleCapAndKeepsCredentialOutOfPersistence(t *testing.T) {
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name       string
		binding    string
		requested  string
		wantStatus int
		wantRole   string
	}{
		{name: "owner may select admin", binding: "gateway:owner", requested: RoleAdmin, wantRole: RoleAdmin},
		{name: "owner may select user", binding: "gateway:owner", requested: RoleUser, wantRole: RoleUser},
		{name: "viewer defaults to user", binding: "gateway:viewer", requested: "", wantRole: RoleUser},
		{name: "viewer cannot select admin", binding: "gateway:viewer", requested: RoleAdmin, wantStatus: 403},
	} {
		t.Run(test.name, func(t *testing.T) {
			dao := newMemoryDAO()
			kc := &fakeKeycloak{configured: true}
			service := newTestService(dao, kc, testBindings("creator", test.binding), now)
			result, problem := service.Create(t.Context(), "gateway-id", "creator", CreateInput{Name: "deploy-bot", Role: test.requested})
			if test.wantStatus != 0 {
				if problem == nil || problem.Status != test.wantStatus {
					t.Fatalf("problem = %#v, want status %d", problem, test.wantStatus)
				}
				if kc.provisionCalls != 0 {
					t.Error("forbidden request reached Keycloak")
				}
				return
			}
			if problem != nil {
				t.Fatalf("Create() problem = %v", problem)
			}
			if result.Account.Role != test.wantRole || kc.lastSpec.Role != test.wantRole {
				t.Errorf("role = %q, Keycloak role = %q, want %q", result.Account.Role, kc.lastSpec.Role, test.wantRole)
			}
			if result.Credential.ClientSecret != kc.secret {
				t.Error("one-time response omitted credential")
			}
			if result.Account.Status != StatusReady || result.Account.Subject != "subject-id" {
				t.Errorf("account = %#v", result.Account)
			}
			if got := result.Account.ExpiresAt.Sub(now); got != DefaultExpiration {
				t.Errorf("default expiration = %s, want %s", got, DefaultExpiration)
			}
			persistedJSON, _ := json.Marshal(dao.items[result.Account.ID])
			if strings.Contains(string(persistedJSON), kc.secret) || strings.Contains(string(persistedJSON), "client_secret\":\""+kc.secret) {
				t.Fatalf("persisted model leaked credential: %s", persistedJSON)
			}
			for _, audit := range dao.audits {
				auditJSON, _ := json.Marshal(audit)
				if strings.Contains(string(auditJSON), kc.secret) {
					t.Fatalf("audit leaked credential: %s", auditJSON)
				}
			}
		})
	}
}

func TestViewerVisibilityIsCreatorOnlyWhileOwnerCanManageAll(t *testing.T) {
	dao := newMemoryDAO()
	now := time.Now().UTC()
	dao.seed(
		&OpenShellGatewayServiceAccount{GatewayID: "gateway-id", Name: "mine", CreatedByUserID: "viewer-a", Status: StatusReady, Role: RoleUser, CredentialType: CredentialTypeClientSecret, KeycloakClientID: "client-a", Subject: "sub-a", ExpiresAt: now.Add(time.Hour)},
		&OpenShellGatewayServiceAccount{GatewayID: "gateway-id", Name: "theirs", CreatedByUserID: "viewer-b", Status: StatusReady, Role: RoleUser, CredentialType: CredentialTypeClientSecret, KeycloakClientID: "client-b", Subject: "sub-b", ExpiresAt: now.Add(time.Hour)},
	)
	bindings := fakeBindings{byUser: map[string][]rbac.BindingSummary{
		"viewer-a": binding("gateway:viewer"), "owner": binding("gateway:owner"),
	}}
	service := newTestService(dao, &fakeKeycloak{configured: true}, bindings, now)

	items, total, access, problem := service.List(t.Context(), "gateway-id", "viewer-a", ListOptions{Page: 1, Size: 20, Sort: "created_at", Order: "asc"})
	if problem != nil || len(items) != 1 || total != 1 || items[0].CreatedByUserID != "viewer-a" || access.CanManageAll {
		t.Fatalf("viewer list = %#v total=%d access=%#v problem=%v", items, total, access, problem)
	}
	if _, _, problem := service.Get(t.Context(), "gateway-id", itemsIDByCreator(dao, "viewer-b"), "viewer-a"); problem == nil || problem.Status != 404 {
		t.Fatalf("viewer cross-account Get() problem = %#v, want 404", problem)
	}
	ownerItems, ownerTotal, ownerAccess, problem := service.List(t.Context(), "gateway-id", "owner", ListOptions{Page: 1, Size: 20, Sort: "created_at", Order: "asc"})
	if problem != nil || len(ownerItems) != 2 || ownerTotal != 2 || !ownerAccess.CanManageAll {
		t.Fatalf("owner list = %#v total=%d access=%#v problem=%v", ownerItems, ownerTotal, ownerAccess, problem)
	}
}

func TestCreateHidesGatewayReadinessWithoutAnExactBinding(t *testing.T) {
	dao := newMemoryDAO()
	bindings := fakeBindings{byUser: map[string][]rbac.BindingSummary{
		"platform-admin": {{RoleName: "platform:admin", Scope: "global"}},
	}}
	svc := newTestService(dao, &fakeKeycloak{configured: true}, bindings, time.Now().UTC())
	internal := svc.(*service)
	gateway := internal.gateways.(fakeGateway).gateway
	phase := "Provisioning"
	gateway.Phase = &phase
	gateway.Oidc = nil

	for _, userID := range []string{"unbound-user", "platform-admin"} {
		if _, problem := svc.Create(t.Context(), gateway.ID, userID, CreateInput{Name: "hidden"}); problem == nil || problem.Status != 404 {
			t.Fatalf("Create() for %s problem = %#v, want 404", userID, problem)
		}
	}
	dao.seed(&OpenShellGatewayServiceAccount{
		GatewayID: gateway.ID, Name: "hidden", CreatedByUserID: "someone-else", Status: StatusReady,
		Role: RoleUser, CredentialType: CredentialTypeClientSecret, KeycloakClientID: "hidden-client",
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	})
	if _, _, problem := svc.Get(t.Context(), gateway.ID, itemsIDByCreator(dao, "someone-else"), "unbound-user"); problem == nil || problem.Status != 404 {
		t.Fatalf("Get() problem = %#v, want 404", problem)
	}
}

func TestCreateFailureRemovesAProvenAbsentCredentialReservation(t *testing.T) {
	dao := newMemoryDAO()
	kc := &fakeKeycloak{configured: true, provisionErr: errors.New("provider unavailable")}
	service := newTestService(dao, kc, testBindings("creator", "gateway:owner"), time.Now().UTC())

	if _, problem := service.Create(t.Context(), "gateway-id", "creator", CreateInput{Name: "deploy-bot"}); problem == nil || problem.Status != 503 {
		t.Fatalf("Create() problem = %#v, want 503", problem)
	}
	if len(dao.items) != 0 {
		t.Fatalf("failed reservation was not removed: %#v", dao.items)
	}
	if kc.deleteManagedCalls != 1 {
		t.Fatalf("DeleteManagedServiceAccount() calls = %d, want 1", kc.deleteManagedCalls)
	}
	if len(dao.audits) < 2 || dao.audits[len(dao.audits)-1].Outcome != "failed" {
		t.Fatalf("creation failure audit = %#v", dao.audits)
	}
}

func TestReconcileDowngradesAdminAndRevokesWhenBindingIsRemoved(t *testing.T) {
	dao := newMemoryDAO()
	now := time.Now().UTC()
	account := &OpenShellGatewayServiceAccount{
		GatewayID: "gateway-id", Name: "bot", CreatedByUserID: "creator", Status: StatusReady,
		Role: RoleAdmin, CredentialType: CredentialTypeClientSecret, KeycloakClientID: "client-id",
		KeycloakClientUUID: "client-uuid", Subject: "subject-id", ExpiresAt: now.Add(time.Hour),
	}
	dao.seed(account)
	bindings := testBindings("creator", "gateway:viewer")
	kc := &fakeKeycloak{configured: true}
	service := newTestService(dao, kc, bindings, now)

	if err := service.ReconcileOnce(t.Context()); err != nil {
		t.Fatalf("ReconcileOnce() error = %v", err)
	}
	stored := dao.items[account.ID]
	if stored.Role != RoleUser || kc.lastSpec.Role != RoleUser || stored.Status != StatusReady {
		t.Fatalf("downgraded account = %#v, Keycloak role = %q", stored, kc.lastSpec.Role)
	}

	bindings.byUser["creator"] = nil
	if err := service.ReconcileOnce(t.Context()); err != nil {
		t.Fatalf("ReconcileOnce() after removal error = %v", err)
	}
	stored = dao.items[account.ID]
	if stored.Status != StatusRevoked || stored.RevokedAt == nil || kc.disableCalls == 0 {
		t.Fatalf("revoked account = %#v, disableCalls=%d", stored, kc.disableCalls)
	}
}

func TestReconcileExpiresAndContinuesCheckingTerminalAccounts(t *testing.T) {
	dao := newMemoryDAO()
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	reason := "previous_failure"
	account := &OpenShellGatewayServiceAccount{
		GatewayID: "gateway-id", Name: "bot", CreatedByUserID: "creator", Status: StatusReady,
		Role: RoleUser, CredentialType: CredentialTypeClientSecret, KeycloakClientID: "client-id",
		KeycloakClientUUID: "client-uuid", Subject: "subject-id", ExpiresAt: now, LastError: &reason,
	}
	dao.seed(account)
	kc := &fakeKeycloak{configured: true}
	service := newTestService(dao, kc, testBindings("creator", "gateway:viewer"), now)

	if err := service.ReconcileOnce(t.Context()); err != nil {
		t.Fatalf("ReconcileOnce() error = %v", err)
	}
	stored := dao.items[account.ID]
	if stored.Status != StatusExpired || stored.RevokedAt == nil || stored.LastError != nil || kc.disableCalls != 1 {
		t.Fatalf("expired account = %#v, disableCalls=%d", stored, kc.disableCalls)
	}

	if err := service.ReconcileOnce(t.Context()); err != nil {
		t.Fatalf("ReconcileOnce() terminal drift check error = %v", err)
	}
	if kc.disableCalls != 2 {
		t.Fatalf("terminal drift disable calls = %d, want 2", kc.disableCalls)
	}
}

func TestRevokePersistsForRetryAndDeletePerformsFinalCleanup(t *testing.T) {
	dao := newMemoryDAO()
	now := time.Now().UTC()
	account := &OpenShellGatewayServiceAccount{
		GatewayID: "gateway-id", Name: "bot", CreatedByUserID: "creator", Status: StatusReady,
		Role: RoleUser, CredentialType: CredentialTypeClientSecret, KeycloakClientID: "client-id",
		KeycloakClientUUID: "client-uuid", Subject: "subject-id", ExpiresAt: now.Add(time.Hour),
	}
	dao.seed(account)
	kc := &fakeKeycloak{configured: true, disableErr: errors.New("provider unavailable")}
	service := newTestService(dao, kc, testBindings("creator", "gateway:owner"), now)

	pending, complete, problem := service.Revoke(t.Context(), "gateway-id", account.ID, "creator")
	if problem != nil || complete || pending.Status != StatusRevoking {
		t.Fatalf("Revoke() account=%#v complete=%t problem=%v", pending, complete, problem)
	}
	if stored := dao.items[account.ID]; stored.Status != StatusRevoking || stored.LastError == nil {
		t.Fatalf("persisted revocation = %#v", stored)
	}

	kc.disableErr = nil
	if err := service.ReconcileOnce(t.Context()); err != nil {
		t.Fatalf("ReconcileOnce() retry error = %v", err)
	}
	if stored := dao.items[account.ID]; stored.Status != StatusRevoked || stored.RevokedAt == nil {
		t.Fatalf("retried revocation = %#v", stored)
	}

	_, deleted, problem := service.Delete(t.Context(), "gateway-id", account.ID, "creator")
	if problem != nil || !deleted {
		t.Fatalf("Delete() complete=%t problem=%v", deleted, problem)
	}
	if _, exists := dao.items[account.ID]; exists {
		t.Fatal("deleted account remains visible")
	}
	if len(kc.deletedUUIDs) != 1 || kc.deletedUUIDs[0] != "client-uuid" {
		t.Fatalf("deleted Keycloak UUIDs = %v", kc.deletedUUIDs)
	}
}

func TestCleanupGatewayDeletesIdentitiesBeforeRecords(t *testing.T) {
	dao := newMemoryDAO()
	now := time.Now().UTC()
	dao.seed(
		&OpenShellGatewayServiceAccount{GatewayID: "gateway-id", Name: "first", CreatedByUserID: "creator", Status: StatusReady, Role: RoleUser, CredentialType: CredentialTypeClientSecret, KeycloakClientID: "client-1", ExpiresAt: now.Add(time.Hour)},
		&OpenShellGatewayServiceAccount{GatewayID: "gateway-id", Name: "second", CreatedByUserID: "creator", Status: StatusRevoked, Role: RoleUser, CredentialType: CredentialTypeClientSecret, KeycloakClientID: "client-2", ExpiresAt: now.Add(time.Hour)},
	)
	kc := &fakeKeycloak{configured: true}
	service := newTestService(dao, kc, testBindings("creator", "gateway:owner"), now)

	if err := service.CleanupGateway(t.Context(), "gateway-id"); err != nil {
		t.Fatalf("CleanupGateway() error = %v", err)
	}
	if kc.deleteGatewayCalls != 1 {
		t.Fatalf("DeleteGatewayServiceAccounts() calls = %d, want 1", kc.deleteGatewayCalls)
	}
	if len(dao.items) != 0 {
		t.Fatalf("gateway cleanup retained records: %#v", dao.items)
	}
	if len(dao.audits) != 2 {
		t.Fatalf("gateway cleanup audits = %d, want 2", len(dao.audits))
	}
}

func TestReconcileReportsAndRetriesUndeliveredCredentialCleanup(t *testing.T) {
	dao := newMemoryDAO()
	now := time.Now().UTC()
	account := &OpenShellGatewayServiceAccount{
		GatewayID: "gateway-id", Name: "bot", CreatedByUserID: "creator", Status: StatusError,
		Role: RoleUser, CredentialType: CredentialTypeClientSecret, KeycloakClientID: "client-id",
		KeycloakClientUUID: "client-uuid", ExpiresAt: now.Add(time.Hour),
	}
	dao.seed(account)
	kc := &fakeKeycloak{configured: true, deleteErr: errors.New("provider unavailable")}
	service := newTestService(dao, kc, testBindings("creator", "gateway:viewer"), now)

	if err := service.ReconcileOnce(t.Context()); err == nil {
		t.Fatal("ReconcileOnce() error = nil, want cleanup failure")
	}
	if _, exists := dao.items[account.ID]; !exists {
		t.Fatal("failed cleanup removed the reservation")
	}
	kc.deleteErr = nil
	if err := service.ReconcileOnce(t.Context()); err != nil {
		t.Fatalf("ReconcileOnce() retry error = %v", err)
	}
	if _, exists := dao.items[account.ID]; exists {
		t.Fatal("successful cleanup retained the reservation")
	}
}

func TestReconcileDeletesKeycloakClientsWithoutAResource(t *testing.T) {
	dao := newMemoryDAO()
	kc := &fakeKeycloak{
		configured: true,
		managedClients: []keycloak.ManagedClient{{
			UUID: "orphan-uuid", GatewayID: "gateway-id", ServiceAccountID: "missing-account",
		}},
	}
	service := newTestService(dao, kc, testBindings("creator", "gateway:viewer"), time.Now().UTC())

	if err := service.ReconcileOnce(t.Context()); err != nil {
		t.Fatalf("ReconcileOnce() error = %v", err)
	}
	if len(kc.deletedUUIDs) != 1 || kc.deletedUUIDs[0] != "orphan-uuid" {
		t.Fatalf("deleted UUIDs = %v", kc.deletedUUIDs)
	}
}

func newTestService(dao *memoryDAO, kc *fakeKeycloak, bindings fakeBindings, now time.Time) Service {
	oidc, _ := json.Marshal(GatewayOIDC{Issuer: "https://issuer.example/realms/hypershell", ClientID: "gateway-client", Audience: "gateway-client"})
	phase, status, route := "Running", "Healthy", "grpcs://gateway.example:443"
	gateway := &gateways.Gateway{Name: "gateway", Phase: &phase, Status: &status, RouteAddress: &route, Oidc: stringPointer(string(oidc))}
	gateway.ID = "gateway-id"
	result := NewService(dao, fakeGateway{gateway: gateway}, bindings, kc, db.NewNoOpLockFactory())
	result.(*service).now = func() time.Time { return now }
	return result
}

type fakeGateway struct{ gateway *gateways.Gateway }

func (f fakeGateway) Get(_ context.Context, id string) (*gateways.Gateway, *trexerrors.ServiceError) {
	if f.gateway == nil || f.gateway.ID != id {
		return nil, trexerrors.NotFound("not found")
	}
	return f.gateway, nil
}

type fakeBindings struct {
	byUser map[string][]rbac.BindingSummary
}

func (f fakeBindings) FindBindingsByUserID(_ context.Context, userID string) ([]rbac.BindingSummary, error) {
	return f.byUser[userID], nil
}

func testBindings(userID, role string) fakeBindings {
	return fakeBindings{byUser: map[string][]rbac.BindingSummary{userID: binding(role)}}
}

func binding(role string) []rbac.BindingSummary {
	gatewayID := "gateway-id"
	return []rbac.BindingSummary{{RoleName: role, Scope: "gateway", GatewayID: &gatewayID}}
}

type fakeKeycloak struct {
	configured         bool
	disableErr         error
	deleteErr          error
	provisionErr       error
	secret             string
	lastSpec           keycloak.ServiceAccountSpec
	managedClients     []keycloak.ManagedClient
	provisionCalls     int
	disableCalls       int
	deleteManagedCalls int
	deleteGatewayCalls int
	deletedUUIDs       []string
}

func (f *fakeKeycloak) Configured() bool { return f.configured }

func (f *fakeKeycloak) ProvisionServiceAccount(_ context.Context, spec keycloak.ServiceAccountSpec) (*keycloak.ProvisionedServiceAccount, error) {
	f.provisionCalls++
	f.lastSpec = spec
	if f.provisionErr != nil {
		return nil, f.provisionErr
	}
	if f.secret == "" {
		f.secret = "one-time-secret"
	}
	return &keycloak.ProvisionedServiceAccount{ClientUUID: "client-uuid", ClientID: spec.ClientID, ClientSecret: f.secret, Subject: "subject-id"}, nil
}

func (f *fakeKeycloak) ReconcileServiceAccount(_ context.Context, spec keycloak.ServiceAccountSpec, _, _ string, _ bool) error {
	f.lastSpec = spec
	return nil
}

func (f *fakeKeycloak) DisableServiceAccount(context.Context, string) error {
	f.disableCalls++
	return f.disableErr
}

func (f *fakeKeycloak) DeleteServiceAccount(_ context.Context, uuid string) error {
	if f.deleteErr == nil {
		f.deletedUUIDs = append(f.deletedUUIDs, uuid)
	}
	return f.deleteErr
}
func (f *fakeKeycloak) DeleteManagedServiceAccount(context.Context, string, string) error {
	f.deleteManagedCalls++
	return f.deleteErr
}
func (f *fakeKeycloak) DeleteGatewayServiceAccounts(context.Context, string) error {
	f.deleteGatewayCalls++
	return f.deleteErr
}
func (f *fakeKeycloak) ListManagedClients(context.Context, string) ([]keycloak.ManagedClient, error) {
	return f.managedClients, nil
}

type memoryDAO struct {
	items  map[string]*OpenShellGatewayServiceAccount
	audits []*AuditEvent
	next   int
}

func newMemoryDAO() *memoryDAO {
	return &memoryDAO{items: map[string]*OpenShellGatewayServiceAccount{}}
}

func (d *memoryDAO) seed(accounts ...*OpenShellGatewayServiceAccount) {
	for _, account := range accounts {
		if account.ID == "" {
			d.next++
			account.ID = "account-" + string(rune('0'+d.next))
		}
		if account.CreatedAt.IsZero() {
			account.CreatedAt = time.Now().UTC()
			account.UpdatedAt = account.CreatedAt
		}
		copy := *account
		d.items[account.ID] = &copy
	}
}

func (d *memoryDAO) ActiveNameExists(_ context.Context, gatewayID, name string) (bool, error) {
	for _, account := range d.items {
		if account.GatewayID == gatewayID && strings.EqualFold(account.Name, name) && account.Status != StatusExpired && account.Status != StatusRevoked {
			return true, nil
		}
	}
	return false, nil
}

func (d *memoryDAO) Create(_ context.Context, account *OpenShellGatewayServiceAccount) error {
	if _, exists := d.items[account.ID]; exists {
		return errors.New("duplicate")
	}
	account.CreatedAt = time.Now().UTC()
	account.UpdatedAt = account.CreatedAt
	copy := *account
	d.items[account.ID] = &copy
	return nil
}

func (d *memoryDAO) Update(_ context.Context, account *OpenShellGatewayServiceAccount) error {
	account.UpdatedAt = time.Now().UTC()
	copy := *account
	d.items[account.ID] = &copy
	return nil
}

func (d *memoryDAO) Get(_ context.Context, gatewayID, id string) (*OpenShellGatewayServiceAccount, error) {
	account, ok := d.items[id]
	if !ok || account.GatewayID != gatewayID {
		return nil, gorm.ErrRecordNotFound
	}
	copy := *account
	return &copy, nil
}

func (d *memoryDAO) List(_ context.Context, gatewayID string, options ListOptions) ([]OpenShellGatewayServiceAccount, int64, error) {
	items := make([]OpenShellGatewayServiceAccount, 0)
	for _, account := range d.items {
		if account.GatewayID != gatewayID || (options.CreatorUserID != "" && account.CreatedByUserID != options.CreatorUserID) || (options.Status != "" && account.Status != options.Status) {
			continue
		}
		if options.Search != "" && !strings.Contains(strings.ToLower(account.Name), strings.ToLower(options.Search)) {
			continue
		}
		items = append(items, *account)
	}
	return items, int64(len(items)), nil
}

func (d *memoryDAO) CountActive(_ context.Context, gatewayID, creator string) (int64, int64, error) {
	var gatewayCount, creatorCount int64
	for _, account := range d.items {
		if account.GatewayID == gatewayID && account.Status != StatusExpired && account.Status != StatusRevoked {
			gatewayCount++
			if account.CreatedByUserID == creator {
				creatorCount++
			}
		}
	}
	return gatewayCount, creatorCount, nil
}

func (d *memoryDAO) ListReconcilable(context.Context, int) ([]OpenShellGatewayServiceAccount, error) {
	items := make([]OpenShellGatewayServiceAccount, 0, len(d.items))
	for _, account := range d.items {
		items = append(items, *account)
	}
	return items, nil
}

func (d *memoryDAO) SoftDelete(_ context.Context, account *OpenShellGatewayServiceAccount) error {
	delete(d.items, account.ID)
	return nil
}

func (d *memoryDAO) CreateAudit(_ context.Context, event *AuditEvent) error {
	copy := *event
	d.audits = append(d.audits, &copy)
	return nil
}

func itemsIDByCreator(dao *memoryDAO, creator string) string {
	for id, account := range dao.items {
		if account.CreatedByUserID == creator {
			return id
		}
	}
	return ""
}

func stringPointer(value string) *string { return &value }
