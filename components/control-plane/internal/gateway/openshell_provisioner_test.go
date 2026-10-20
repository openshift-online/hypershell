package gateway

import (
	"context"
	"errors"
	"testing"
)

// fakeOpenShellAPI records the OpenShell provisioning operations it receives and can be
// told to return a specific error for each, so tests assert the provisioner's call
// sequence, idempotency, and error classification.
type fakeOpenShellAPI struct {
	ensureWorkspace    []string
	reconcileMembers   [][]MembershipSpec
	ensureProvider     []ProviderRealization
	configureRefresh   []RefreshRealization
	setRoute           []InferenceRouteConfig
	deleteWorkspace    []string
	ensureWorkspaceErr error
	ensureProviderErr  error
	deleteWorkspaceErr error
	closed             bool
}

func (s *fakeOpenShellAPI) EnsureWorkspace(_ context.Context, _, workspaceName string) error {
	s.ensureWorkspace = append(s.ensureWorkspace, workspaceName)
	return s.ensureWorkspaceErr
}

func (s *fakeOpenShellAPI) ReconcileMemberships(_ context.Context, _ string, memberships []MembershipSpec) error {
	s.reconcileMembers = append(s.reconcileMembers, memberships)
	return nil
}

func (s *fakeOpenShellAPI) EnsureProvider(_ context.Context, _ string, provider ProviderRealization) error {
	s.ensureProvider = append(s.ensureProvider, provider)
	return s.ensureProviderErr
}

func (s *fakeOpenShellAPI) ConfigureProviderRefresh(_ context.Context, _ string, refresh RefreshRealization) error {
	s.configureRefresh = append(s.configureRefresh, refresh)
	return nil
}

func (s *fakeOpenShellAPI) SetInferenceRoute(_ context.Context, _, providerName, modelAlias string) error {
	s.setRoute = append(s.setRoute, InferenceRouteConfig{ProviderName: providerName, ModelAlias: modelAlias})
	return nil
}

func (s *fakeOpenShellAPI) DeleteWorkspace(_ context.Context, workspaceName string) error {
	s.deleteWorkspace = append(s.deleteWorkspace, workspaceName)
	return s.deleteWorkspaceErr
}

// newFakeProvisioner wires an OpenShellProvisioner to an in-process fake OpenShell API. The
// session factory re-opens per operation, mirroring production.
func newFakeProvisioner(t *testing.T, api *fakeOpenShellAPI) *OpenShellProvisioner {
	t.Helper()
	session := func(context.Context, string) (OpenShellAPI, func() error, error) {
		return api, func() error { api.closed = true; return nil }, nil
	}
	p, err := NewOpenShellProvisioner(session)
	if err != nil {
		t.Fatalf("NewOpenShellProvisioner: %v", err)
	}
	return p
}

// nativeRefreshSpec is a workspace whose provider uses an OpenShell-owned refresh strategy.
func nativeRefreshSpec() WorkspaceSpec {
	return WorkspaceSpec{
		GatewayID:      "gw-1",
		AgentRuntimeID: "ar-1",
		WorkspaceName:  "amber-reviews",
		Memberships:    []MembershipSpec{{Subject: "subject-1", Admin: true}},
		Providers: []ProviderConfig{{
			Name:          "vertex",
			ProviderType:  "google-vertex-ai",
			CredentialKey: "GOOGLE_VERTEX_AI_SERVICE_ACCOUNT_TOKEN",
			Refresh: &RefreshConfig{
				CredentialKey:  "GOOGLE_VERTEX_AI_SERVICE_ACCOUNT_TOKEN",
				Strategy:       "google-service-account-jwt",
				SecretMaterial: map[string][]byte{"service-account-key": []byte("SA-KEY")},
			},
		}},
		Route: &InferenceRouteConfig{ProviderName: "vertex", ModelAlias: "claude-sonnet-4-5"},
	}
}

// githubRotatedSpec is a workspace whose GitHub provider carries a control-plane-minted,
// rotated credential and configures NO OpenShell refresh strategy.
func githubRotatedSpec() WorkspaceSpec {
	return WorkspaceSpec{
		GatewayID:      "gw-1",
		AgentRuntimeID: "ar-1",
		WorkspaceName:  "amber-reviews",
		Memberships:    []MembershipSpec{{Subject: "subject-1", Admin: true}},
		Providers: []ProviderConfig{{
			Name:                   "github",
			ProviderType:           "github",
			CredentialKey:          "GH_TOKEN",
			ControlPlaneCredential: "ghs-token",
			Rotated:                true,
		}},
	}
}

func TestOpenShellProvisioner_RealizeNativeRefreshSequence(t *testing.T) {
	api := &fakeOpenShellAPI{}
	p := newFakeProvisioner(t, api)

	if err := p.Realize(context.Background(), nativeRefreshSpec()); err != nil {
		t.Fatalf("Realize: %v", err)
	}
	if len(api.ensureWorkspace) != 1 {
		t.Fatalf("EnsureWorkspace calls = %d, want 1", len(api.ensureWorkspace))
	}
	if len(api.reconcileMembers) != 1 || len(api.reconcileMembers[0]) != 1 || !api.reconcileMembers[0][0].Admin {
		t.Fatalf("ReconcileMemberships not called with the full admin membership set: %+v", api.reconcileMembers)
	}
	if len(api.ensureProvider) != 1 || api.ensureProvider[0].ProviderType != "google-vertex-ai" {
		t.Fatalf("EnsureProvider not called for the vertex provider: %+v", api.ensureProvider)
	}
	if len(api.configureRefresh) != 1 || api.configureRefresh[0].Strategy != "google-service-account-jwt" {
		t.Fatalf("OpenShell refresh strategy must be configured for a native provider: %+v", api.configureRefresh)
	}
	if len(api.setRoute) != 1 {
		t.Fatalf("SetInferenceRoute calls = %d, want 1", len(api.setRoute))
	}
	if !api.closed {
		t.Fatalf("session must be closed after the pass")
	}
}

func TestOpenShellProvisioner_GitHubProviderConfiguresNoRefresh(t *testing.T) {
	api := &fakeOpenShellAPI{}
	p := newFakeProvisioner(t, api)

	if err := p.Realize(context.Background(), githubRotatedSpec()); err != nil {
		t.Fatalf("Realize: %v", err)
	}
	if len(api.ensureProvider) != 1 || api.ensureProvider[0].CredentialValue != "ghs-token" {
		t.Fatalf("github provider credential must be set through EnsureProvider: %+v", api.ensureProvider)
	}
	if len(api.configureRefresh) != 0 {
		t.Fatalf("github (control-plane-rotated) provider must NOT configure an OpenShell refresh: %+v", api.configureRefresh)
	}
}

func TestOpenShellProvisioner_RealizeIsIdempotent(t *testing.T) {
	api := &fakeOpenShellAPI{}
	p := newFakeProvisioner(t, api)

	for i := 0; i < 2; i++ {
		if err := p.Realize(context.Background(), nativeRefreshSpec()); err != nil {
			t.Fatalf("Realize pass %d: %v", i, err)
		}
	}
	// The fake accepts a repeat ensure as a no-op success, so two passes make two
	// successful calls without error - the idempotent contract from the client's view.
	if len(api.ensureWorkspace) != 2 {
		t.Fatalf("EnsureWorkspace calls = %d, want 2 (idempotent re-ensure)", len(api.ensureWorkspace))
	}
}

func TestOpenShellProvisioner_NameCollisionIsPermanent(t *testing.T) {
	api := &fakeOpenShellAPI{ensureWorkspaceErr: ErrWorkspaceNameCollision}
	p := newFakeProvisioner(t, api)

	err := p.Realize(context.Background(), nativeRefreshSpec())
	if !errors.Is(err, ErrWorkspaceNameCollision) {
		t.Fatalf("error = %v, want ErrWorkspaceNameCollision", err)
	}
	if len(api.ensureProvider) != 0 {
		t.Fatalf("provider must not be ensured after a workspace collision")
	}
}

func TestOpenShellProvisioner_AdminRequiredIsPermanent(t *testing.T) {
	api := &fakeOpenShellAPI{ensureProviderErr: ErrAdminMembershipRequired}
	p := newFakeProvisioner(t, api)

	err := p.Realize(context.Background(), nativeRefreshSpec())
	if !errors.Is(err, ErrAdminMembershipRequired) {
		t.Fatalf("error = %v, want ErrAdminMembershipRequired", err)
	}
}

func TestOpenShellProvisioner_UnexpectedSentinelFromWrongOperationIsRetryable(t *testing.T) {
	// A collision sentinel from EnsureProvider (not EnsureWorkspace) is a convergeable
	// condition, not the permanent name collision; it must stay retryable.
	api := &fakeOpenShellAPI{ensureProviderErr: ErrWorkspaceNameCollision}
	p := newFakeProvisioner(t, api)
	err := p.Realize(context.Background(), nativeRefreshSpec())
	if err == nil {
		t.Fatalf("expected an error")
	}
	if errors.Is(err, ErrWorkspaceNameCollision) || errors.Is(err, ErrAdminMembershipRequired) {
		t.Fatalf("a collision from ensure provider must be retryable, got permanent: %v", err)
	}

	// An admin-required sentinel from EnsureWorkspace (not a provider op) is likewise
	// convergeable and must stay retryable.
	api2 := &fakeOpenShellAPI{ensureWorkspaceErr: ErrAdminMembershipRequired}
	p2 := newFakeProvisioner(t, api2)
	err2 := p2.Realize(context.Background(), nativeRefreshSpec())
	if err2 == nil {
		t.Fatalf("expected an error")
	}
	if errors.Is(err2, ErrAdminMembershipRequired) || errors.Is(err2, ErrWorkspaceNameCollision) {
		t.Fatalf("an admin-required from ensure workspace must be retryable, got permanent: %v", err2)
	}
}

func TestOpenShellProvisioner_TransientErrorIsRetryable(t *testing.T) {
	api := &fakeOpenShellAPI{ensureWorkspaceErr: errors.New("gateway down")}
	p := newFakeProvisioner(t, api)

	err := p.Realize(context.Background(), nativeRefreshSpec())
	if err == nil {
		t.Fatalf("expected a retryable error")
	}
	if errors.Is(err, ErrWorkspaceNameCollision) || errors.Is(err, ErrAdminMembershipRequired) {
		t.Fatalf("transient error must not be classified as a permanent rejection: %v", err)
	}
}

func TestOpenShellProvisioner_DeleteCascades(t *testing.T) {
	api := &fakeOpenShellAPI{}
	p := newFakeProvisioner(t, api)

	if err := p.Delete(context.Background(), "gw-1", "ar-1", "amber-reviews"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if len(api.deleteWorkspace) != 1 {
		t.Fatalf("DeleteWorkspace calls = %d, want 1", len(api.deleteWorkspace))
	}
}

func TestOpenShellProvisioner_DeleteErrorPropagates(t *testing.T) {
	api := &fakeOpenShellAPI{deleteWorkspaceErr: errors.New("gateway down")}
	p := newFakeProvisioner(t, api)

	if err := p.Delete(context.Background(), "gw-1", "ar-1", "amber-reviews"); err == nil {
		t.Fatalf("a delete failure must propagate so the delete is retried")
	}
}

func TestNewOpenShellProvisioner_RequiresSessionFactory(t *testing.T) {
	if _, err := NewOpenShellProvisioner(nil); err == nil {
		t.Fatalf("expected an error for a nil session factory")
	}
}
