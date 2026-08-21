package serviceAccounts

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/openshift-online/hypershell/components/api-server/pkg/keycloak"
	"github.com/openshift-online/hypershell/components/api-server/pkg/rbac"
	"github.com/openshift-online/hypershell/components/api-server/plugins/gateways"
	"github.com/openshift-online/rh-trex-ai/pkg/db"
	trexerrors "github.com/openshift-online/rh-trex-ai/pkg/errors"
	"gorm.io/gorm"
)

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
	configured     bool
	secret         string
	lastSpec       keycloak.ServiceAccountSpec
	provisionCalls int
	disableCalls   int
}

func (f *fakeKeycloak) Configured() bool { return f.configured }

func (f *fakeKeycloak) ProvisionServiceAccount(_ context.Context, spec keycloak.ServiceAccountSpec) (*keycloak.ProvisionedServiceAccount, error) {
	f.provisionCalls++
	f.lastSpec = spec
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
	return nil
}

func (f *fakeKeycloak) DeleteServiceAccount(context.Context, string) error { return nil }
func (f *fakeKeycloak) DeleteManagedServiceAccount(context.Context, string, string) error {
	return nil
}
func (f *fakeKeycloak) DeleteGatewayServiceAccounts(context.Context, string) error { return nil }

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
