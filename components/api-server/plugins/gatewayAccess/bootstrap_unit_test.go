package gatewayAccess

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/openshift-online/hypershell/components/api-server/pkg/api"
	"github.com/openshift-online/hypershell/components/api-server/pkg/rbac"
	"github.com/openshift-online/hypershell/components/api-server/plugins/defaultWorkspaceBootstrap"
	"github.com/openshift-online/hypershell/components/api-server/plugins/gateways"
	"github.com/openshift-online/hypershell/components/api-server/plugins/roleBindings"
	"github.com/openshift-online/hypershell/components/api-server/plugins/roles"
	"github.com/openshift-online/hypershell/components/api-server/plugins/users"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/db"
	trexerrors "github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
)

func TestGrantBootstrapsDefaultWorkspaceOnlyForUserTier(t *testing.T) {
	for _, test := range []struct {
		name string
		role string
		want bool
	}{
		{name: "user", role: TierUser, want: true},
		{name: "admin", role: TierAdmin, want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			deps := newBootstrapTestDeps()
			deps.dao.roleName = tierToRoleName(test.role)
			item, problem := deps.service.Grant(context.Background(), "gateway-id", "owner", GrantInput{
				Username: "target", Role: test.role,
			})
			if problem != nil {
				t.Fatalf("Grant() problem = %#v", problem)
			}
			if called := deps.bootstrap.input != (defaultWorkspaceBootstrap.Input{}); called != test.want {
				t.Fatalf("bootstrap called = %v, want %v; input = %#v", called, test.want, deps.bootstrap.input)
			}
			if item.Role != test.role {
				t.Fatalf("grant role = %q, want %q", item.Role, test.role)
			}
		})
	}
}

func TestGrantDoesNotWriteBindingWhenDefaultWorkspaceBootstrapFails(t *testing.T) {
	deps := newBootstrapTestDeps()
	deps.bootstrap.problem = &defaultWorkspaceBootstrap.APIError{
		Status: http.StatusServiceUnavailable, Code: "default_workspace_bootstrap_failed", Message: "failed",
	}
	_, problem := deps.service.Grant(context.Background(), "gateway-id", "owner", GrantInput{
		Username: "target", Role: TierUser,
	})
	if problem == nil || problem.Status != http.StatusServiceUnavailable || problem.Code != "default_workspace_bootstrap_failed" {
		t.Fatalf("Grant() problem = %#v", problem)
	}
	if deps.bindings.currentRole("target", "gateway-id") != "" {
		t.Fatal("binding was written after bootstrap failure")
	}
}

func TestGrantCanSkipDefaultWorkspaceBootstrap(t *testing.T) {
	deps := newBootstrapTestDeps()
	deps.dao.roleName = roles.RoleGatewayViewer
	deps.bootstrap.problem = &defaultWorkspaceBootstrap.APIError{
		Status: http.StatusServiceUnavailable, Code: "default_workspace_bootstrap_failed", Message: "failed",
	}
	item, problem := deps.service.Grant(context.Background(), "gateway-id", "owner", GrantInput{
		Username: "target", Role: TierUser, SkipDefaultWorkspaceBootstrap: true,
	})
	if problem != nil {
		t.Fatalf("Grant() problem = %#v", problem)
	}
	if deps.bootstrap.input != (defaultWorkspaceBootstrap.Input{}) {
		t.Fatalf("bootstrap input = %#v, want no call", deps.bootstrap.input)
	}
	if deps.bindings.currentRole("target", "gateway-id") != roles.RoleGatewayViewer {
		t.Fatal("binding was not written when bootstrap was skipped")
	}
	if item.Role != TierUser {
		t.Fatalf("grant role = %q, want %q", item.Role, TierUser)
	}
}

func TestGrantHandlerAcceptsDefaultWorkspaceBootstrapOptOut(t *testing.T) {
	service := &recordingGrantService{}
	request := httptest.NewRequest(http.MethodPost, "/gateways/gateway-id/access", strings.NewReader(`{
		"username": "target", "role": "user", "skip_default_workspace_bootstrap": true
	}`))
	request = mux.SetURLVars(request, map[string]string{"gateway_id": "gateway-id"})
	response := httptest.NewRecorder()

	NewHandler(service).Grant(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusCreated)
	}
	if !service.input.SkipDefaultWorkspaceBootstrap {
		t.Fatal("handler did not pass the default-workspace bootstrap opt-out to the service")
	}
}

func TestChangeRoleToUserBootstrapsDefaultWorkspace(t *testing.T) {
	deps := newBootstrapTestDeps()
	deps.dao.roleName = roles.RoleGatewayViewer
	deps.bindings.setRole("target", "gateway-id", roles.RoleGatewayAdmin)
	if _, problem := deps.service.ChangeRole(context.Background(), "gateway-id", "owner", "target", TierUser); problem != nil {
		t.Fatalf("ChangeRole() problem = %#v", problem)
	}
	if deps.bootstrap.input != expectedBootstrapInput("gateway-id") {
		t.Fatalf("bootstrap input = %#v", deps.bootstrap.input)
	}
}

func TestGrantSkipsDefaultWorkspaceBootstrapForLegacyGateway(t *testing.T) {
	deps := newBootstrapTestDeps()
	deps.dao.roleName = roles.RoleGatewayViewer
	deps.bootstrap.problem = &defaultWorkspaceBootstrap.APIError{
		Status: http.StatusServiceUnavailable, Code: "default_workspace_bootstrap_failed", Message: "failed",
	}
	service := deps.service.(*service)
	service.gateways = unmarkedGatewayLookup{}

	item, problem := service.Grant(context.Background(), "gateway-id", "owner", GrantInput{
		Username: "target", Role: TierUser,
	})
	if problem != nil {
		t.Fatalf("Grant() problem = %#v", problem)
	}
	if deps.bootstrap.input != (defaultWorkspaceBootstrap.Input{}) {
		t.Fatalf("bootstrap input = %#v, want no call", deps.bootstrap.input)
	}
	if deps.bindings.currentRole("target", "gateway-id") != roles.RoleGatewayViewer || item.Role != TierUser {
		t.Fatalf("grant did not succeed for legacy gateway: item = %#v", item)
	}
}

func TestGrantMapsMissingGatewayToNotFound(t *testing.T) {
	deps := newBootstrapTestDeps()
	service := deps.service.(*service)
	service.gateways = missingGatewayLookup{}

	_, problem := service.Grant(context.Background(), "gateway-id", "owner", GrantInput{
		Username: "target", Role: TierUser,
	})
	if problem == nil || problem.Status != http.StatusNotFound {
		t.Fatalf("Grant() problem = %#v, want not found", problem)
	}
}

type bootstrapTestDeps struct {
	service   Service
	dao       *fakeAccessDao
	bindings  *fakeBindingService
	bootstrap *fakeBootstrapService
}

func newBootstrapTestDeps() *bootstrapTestDeps {
	dao := &fakeAccessDao{}
	bindings := &fakeBindingService{byUserGateway: map[string]string{
		"owner:gateway-id": roles.RoleGatewayOwner,
	}}
	bootstrap := &fakeBootstrapService{}
	service := NewService(
		dao, bindings, &fakeUserDirectory{}, fakeDirectoryResolver{},
		db.NewNoOpLockFactory(), fakeGatewayLookup{}, bootstrap,
	)
	return &bootstrapTestDeps{service: service, dao: dao, bindings: bindings, bootstrap: bootstrap}
}

func expectedBootstrapInput(gatewayID string) defaultWorkspaceBootstrap.Input {
	return defaultWorkspaceBootstrap.Input{
		GatewayID: gatewayID, GatewayNamespace: "gateway-namespace",
		GatewayEndpoint: "https://gateway.example:443", Issuer: "https://issuer.example/realms/hypershell",
		GatewayClientID: "gateway-client", Subject: "target-subject",
		SubjectType: defaultWorkspaceBootstrap.SubjectTypeUser, Role: "user",
	}
}

type fakeAccessDao struct {
	roleName string
}

func (f *fakeAccessDao) list(_ context.Context, _ string, _ ListOptions) ([]accessRow, int64, error) {
	return nil, 0, nil
}

func (f *fakeAccessDao) getForUser(_ context.Context, _, _ string) (accessRow, bool, error) {
	return accessRow{UserID: "target", Username: "target", RoleName: f.roleName}, true, nil
}

func (f *fakeAccessDao) ownerUserIDs(_ context.Context, _ string) ([]string, error) {
	return []string{"owner"}, nil
}

type fakeBindingService struct {
	byUserGateway map[string]string
}

func (f *fakeBindingService) currentRole(userID, gatewayID string) string {
	return f.byUserGateway[userID+":"+gatewayID]
}

func (f *fakeBindingService) setRole(userID, gatewayID, roleName string) {
	f.byUserGateway[userID+":"+gatewayID] = roleName
}

func (f *fakeBindingService) FindBindingsByUserID(_ context.Context, userID string) ([]rbac.BindingSummary, error) {
	rolesByName := map[string]string{}
	for _, roleName := range f.byUserGateway {
		rolesByName[roleName] = roleName
	}
	summaries := make([]rbac.BindingSummary, 0, len(rolesByName))
	for roleName := range rolesByName {
		summaries = append(summaries, rbac.BindingSummary{
			RoleName: roleName, Scope: roleBindings.ScopeGateway, GatewayID: strPtr("gateway-id"),
		})
	}
	_ = userID
	return summaries, nil
}

func (f *fakeBindingService) SetGatewayRole(_ context.Context, userID, gatewayID, roleName string) (*roleBindings.RoleBinding, *trexerrors.ServiceError) {
	f.setRole(userID, gatewayID, roleName)
	return nil, nil
}

func (f *fakeBindingService) RevokeGatewayAccess(_ context.Context, userID, gatewayID string) (int, *trexerrors.ServiceError) {
	role := f.currentRole(userID, gatewayID)
	delete(f.byUserGateway, userID+":"+gatewayID)
	if role == "" {
		return 0, nil
	}
	return 1, nil
}

type fakeUserDirectory struct{}

func (f *fakeUserDirectory) Get(_ context.Context, userID string) (*users.User, *trexerrors.ServiceError) {
	return &users.User{Meta: api.Meta{ID: userID}, Username: "target"}, nil
}

func (f *fakeUserDirectory) UpsertByUsername(_ context.Context, username string, _, _ *string) (string, error) {
	_ = username
	return "target", nil
}

type fakeBootstrapService struct {
	input   defaultWorkspaceBootstrap.Input
	problem *defaultWorkspaceBootstrap.APIError
}

type recordingGrantService struct {
	Service
	input GrantInput
}

func (f *recordingGrantService) Grant(_ context.Context, _ string, _ string, input GrantInput) (GrantItem, *APIError) {
	f.input = input
	return GrantItem{Role: input.Role}, nil
}

func (f *fakeBootstrapService) Bootstrap(_ context.Context, input defaultWorkspaceBootstrap.Input) *defaultWorkspaceBootstrap.APIError {
	f.input = input
	return f.problem
}

type fakeDirectoryResolver struct{}

func (f fakeDirectoryResolver) Resolve(_ context.Context, _ string) (DirectoryUser, bool, error) {
	return DirectoryUser{
		Username: "target", Name: "Directory User", Email: "target@example.com",
		Subject: "target-subject",
	}, true, nil
}

func (f fakeDirectoryResolver) Search(_ context.Context, _ string, _ int) ([]DirectoryUser, error) {
	return nil, nil
}

type fakeGatewayLookup struct{}

func (f fakeGatewayLookup) Get(context.Context, string) (*gateways.Gateway, *trexerrors.ServiceError) {
	oidc := `{"issuer":"https://issuer.example/realms/hypershell","client_id":"gateway-client","audience":"gateway-client"}`
	route := "grpcs://gateway.example:443"
	conditions := `[{"type":"DefaultWorkspaceBootstrapReady","condition_status":"Complete"}]`
	return &gateways.Gateway{
		Namespace: "gateway-namespace", Oidc: &oidc, RouteAddress: &route,
		ProvisioningConditions: &conditions,
	}, nil
}

type unmarkedGatewayLookup struct{}

func (f unmarkedGatewayLookup) Get(context.Context, string) (*gateways.Gateway, *trexerrors.ServiceError) {
	oidc := `{"issuer":"https://issuer.example/realms/hypershell","client_id":"gateway-client","audience":"gateway-client"}`
	route := "grpcs://gateway.example:443"
	return &gateways.Gateway{Namespace: "gateway-namespace", Oidc: &oidc, RouteAddress: &route}, nil
}

type missingGatewayLookup struct{}

func (f missingGatewayLookup) Get(context.Context, string) (*gateways.Gateway, *trexerrors.ServiceError) {
	return nil, trexerrors.New(trexerrors.ErrorNotFound, "gateway not found")
}

func strPtr(value string) *string { return &value }
