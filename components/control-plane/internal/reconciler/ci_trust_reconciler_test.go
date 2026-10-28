package reconciler

import (
	"context"
	"testing"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/control-plane/internal/keycloak"
	"github.com/openshift-online/hypershell/components/control-plane/internal/watcher"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const testClusterID = "cluster-local"

// fakeCIKeycloak records the federation specs it is asked to realize/revoke and
// can be told to fail, so reconciler behavior is tested without a live Keycloak.
type fakeCIKeycloak struct {
	ensured   []keycloak.CIFederationSpec
	deleted   []keycloak.CIFederationSpec
	ensureErr error
	deleteErr error
}

func (f *fakeCIKeycloak) EnsureCIFederation(_ context.Context, spec keycloak.CIFederationSpec) error {
	f.ensured = append(f.ensured, spec)
	return f.ensureErr
}

func (f *fakeCIKeycloak) DeleteCIFederation(_ context.Context, spec keycloak.CIFederationSpec) error {
	f.deleted = append(f.deleted, spec)
	return f.deleteErr
}

func (f *fakeCIKeycloak) Issuer() string { return "https://keycloak.example.com/realms/hypershell" }

// Minimal gRPC service fakes: embed the generated client interface (nil method
// set) and override only the calls the reconciler makes.
type fakeCIProviderSvc struct {
	pb.CIProviderServiceClient
	provider *pb.CIProvider
	notFound bool
}

func (f *fakeCIProviderSvc) GetCIProvider(_ context.Context, _ *pb.GetCIProviderRequest, _ ...grpc.CallOption) (*pb.GetCIProviderResponse, error) {
	if f.notFound {
		return nil, status.Error(codes.NotFound, "ci provider not found")
	}
	return &pb.GetCIProviderResponse{CiProvider: f.provider}, nil
}

type fakeCITrustSvc struct {
	pb.CITrustServiceClient
	updated *pb.UpdateCITrustRequest
}

func (f *fakeCITrustSvc) UpdateCITrust(_ context.Context, req *pb.UpdateCITrustRequest, _ ...grpc.CallOption) (*pb.UpdateCITrustResponse, error) {
	f.updated = req
	return &pb.UpdateCITrustResponse{}, nil
}

type fakeGatewaySvc struct {
	pb.GatewayServiceClient
	gateway  *pb.Gateway
	notFound bool
}

func (f *fakeGatewaySvc) GetGateway(_ context.Context, _ *pb.GetGatewayRequest, _ ...grpc.CallOption) (*pb.GetGatewayResponse, error) {
	if f.notFound {
		return nil, status.Error(codes.NotFound, "gateway not found")
	}
	return &pb.GetGatewayResponse{Gateway: f.gateway}, nil
}

func strptr(s string) *string { return &s }

func newCITrustTestReconciler(kc ciFederationKeycloak, provider *pb.CIProvider, gw *fakeGatewaySvc, trustSvc *fakeCITrustSvc) *CITrustReconciler {
	return &CITrustReconciler{
		keycloakClient:     kc,
		ciTrustSvc:         trustSvc,
		ciProviderSvc:      &fakeCIProviderSvc{provider: provider},
		gatewaySvc:         gw,
		federationClientID: defaultCIFederationClientID,
		clusterID:          testClusterID,
	}
}

func githubProvider() *pb.CIProvider {
	return &pb.CIProvider{
		Metadata:       &pb.ObjectReference{Id: "prov-1"},
		Forge:          "github",
		Issuer:         "https://token.actions.githubusercontent.com",
		Audience:       "openshell-fleet",
		StatusIdpAlias: strptr("github-actions"),
	}
}

func githubTrust() *pb.CITrust {
	return &pb.CITrust{
		Metadata:          &pb.ObjectReference{Id: "trust-1"},
		CiProviderId:      "prov-1",
		GatewayId:         "gw-1",
		SubjectRepository: strptr("acme/app"),
		Role:              "openshell-user",
	}
}

func localGateway() *fakeGatewaySvc {
	return &fakeGatewaySvc{gateway: &pb.Gateway{
		Metadata:  &pb.ObjectReference{Id: "gw-1"},
		Name:      "prod",
		ClusterId: testClusterID,
	}}
}

func TestCITrustUpsert_RealizesFederationAndWritesStatus(t *testing.T) {
	kc := &fakeCIKeycloak{}
	trustSvc := &fakeCITrustSvc{}
	r := newCITrustTestReconciler(kc, githubProvider(), localGateway(), trustSvc)

	err := r.Handle(t.Context(), watcher.Event[*pb.CITrust]{
		Type:       watcher.EventCreated,
		ResourceID: "trust-1",
		Resource:   githubTrust(),
	})
	if err != nil {
		t.Fatalf("Handle upsert: %v", err)
	}

	if len(kc.ensured) != 1 {
		t.Fatalf("EnsureCIFederation calls = %d, want 1", len(kc.ensured))
	}
	spec := kc.ensured[0]
	if spec.IdpAlias != "github-actions" {
		t.Errorf("IdpAlias = %q, want the adopted github-actions alias", spec.IdpAlias)
	}
	if spec.GatewayClientID != "prod-gw-1" {
		t.Errorf("GatewayClientID = %q, want prod-gw-1 ({name}-{id})", spec.GatewayClientID)
	}
	if spec.Workspace != "gh-acme" {
		t.Errorf("Workspace = %q, want gh-acme", spec.Workspace)
	}
	if spec.Subject.RepositoryOwner != "acme" {
		t.Errorf("RepositoryOwner = %q, want acme", spec.Subject.RepositoryOwner)
	}

	if trustSvc.updated == nil {
		t.Fatal("status was not written")
	}
	if got := trustSvc.updated.GetStatusWorkspace(); got != "gh-acme" {
		t.Errorf("status.workspace = %q, want gh-acme", got)
	}
	if got := trustSvc.updated.GetStatusExchangeEndpoint(); got != "https://keycloak.example.com/realms/hypershell/protocol/openid-connect/token" {
		t.Errorf("status.exchange_endpoint = %q", got)
	}
	if trustSvc.updated.GetStatusWorkflowHint() == "" {
		t.Error("status.workflow_hint should be set")
	}
}

func TestCITrustUpsert_SkipsForeignCluster(t *testing.T) {
	kc := &fakeCIKeycloak{}
	trustSvc := &fakeCITrustSvc{}
	gw := &fakeGatewaySvc{gateway: &pb.Gateway{
		Metadata:  &pb.ObjectReference{Id: "gw-1"},
		Name:      "prod",
		ClusterId: "some-other-cluster",
	}}
	r := newCITrustTestReconciler(kc, githubProvider(), gw, trustSvc)

	err := r.Handle(t.Context(), watcher.Event[*pb.CITrust]{
		Type:       watcher.EventUpdated,
		ResourceID: "trust-1",
		Resource:   githubTrust(),
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(kc.ensured) != 0 {
		t.Errorf("foreign-cluster gateway must not be reconciled; got %d ensure calls", len(kc.ensured))
	}
	if trustSvc.updated != nil {
		t.Error("foreign-cluster gateway must not get a status write")
	}
}

func TestCITrustUpsert_GatewayNotFoundIsRetryable(t *testing.T) {
	kc := &fakeCIKeycloak{}
	gw := &fakeGatewaySvc{notFound: true}
	r := newCITrustTestReconciler(kc, githubProvider(), gw, &fakeCITrustSvc{})

	err := r.Handle(t.Context(), watcher.Event[*pb.CITrust]{
		Type:       watcher.EventCreated,
		ResourceID: "trust-1",
		Resource:   githubTrust(),
	})
	if err == nil {
		t.Fatal("expected an error so the resync retries when the gateway is absent")
	}
	if len(kc.ensured) != 0 {
		t.Error("no federation should be realized when the gateway is absent")
	}
}

func TestCITrustUpsert_KeycloakClientNotReadyIsRetryable(t *testing.T) {
	kc := &fakeCIKeycloak{ensureErr: &keycloak.ClientNotFoundError{ClientID: "prod-gw-1"}}
	r := newCITrustTestReconciler(kc, githubProvider(), localGateway(), &fakeCITrustSvc{})

	err := r.Handle(t.Context(), watcher.Event[*pb.CITrust]{
		Type:       watcher.EventCreated,
		ResourceID: "trust-1",
		Resource:   githubTrust(),
	})
	if err == nil {
		t.Fatal("expected a retryable error when the gateway keycloak client is not ready")
	}
}

func TestCITrustDelete_RevokesFederation(t *testing.T) {
	kc := &fakeCIKeycloak{}
	r := newCITrustTestReconciler(kc, githubProvider(), localGateway(), &fakeCITrustSvc{})

	err := r.Handle(t.Context(), watcher.Event[*pb.CITrust]{
		Type:       watcher.EventDeleted,
		ResourceID: "trust-1",
		Resource:   githubTrust(),
	})
	if err != nil {
		t.Fatalf("Handle delete: %v", err)
	}
	if len(kc.deleted) != 1 {
		t.Fatalf("DeleteCIFederation calls = %d, want 1", len(kc.deleted))
	}
	if kc.deleted[0].TrustID != "trust-1" || kc.deleted[0].IdpAlias != "github-actions" {
		t.Errorf("delete spec = %+v, want trust-1 on github-actions", kc.deleted[0])
	}
}

func TestCITrustDelete_GoneGatewayStillRevokes(t *testing.T) {
	kc := &fakeCIKeycloak{}
	gw := &fakeGatewaySvc{notFound: true}
	r := newCITrustTestReconciler(kc, githubProvider(), gw, &fakeCITrustSvc{})

	err := r.Handle(t.Context(), watcher.Event[*pb.CITrust]{
		Type:       watcher.EventDeleted,
		ResourceID: "trust-1",
		Resource:   githubTrust(),
	})
	if err != nil {
		t.Fatalf("Handle delete: %v", err)
	}
	if len(kc.deleted) != 1 {
		t.Errorf("a cascade-deleted gateway must still revoke the shared IdP mapper; got %d", len(kc.deleted))
	}
}

func TestCITrustDelete_MissingProviderIsNoOp(t *testing.T) {
	kc := &fakeCIKeycloak{}
	r := &CITrustReconciler{
		keycloakClient:     kc,
		ciTrustSvc:         &fakeCITrustSvc{},
		ciProviderSvc:      &fakeCIProviderSvc{notFound: true},
		gatewaySvc:         localGateway(),
		federationClientID: defaultCIFederationClientID,
		clusterID:          testClusterID,
	}

	err := r.Handle(t.Context(), watcher.Event[*pb.CITrust]{
		Type:       watcher.EventDeleted,
		ResourceID: "trust-1",
		Resource:   githubTrust(),
	})
	if err != nil {
		t.Fatalf("delete with a gone CIProvider should be a no-op, got %v", err)
	}
	if len(kc.deleted) != 0 {
		t.Error("no revoke should be attempted when the provider (and its IdP alias) is gone")
	}
}

func TestCITrustHandle_NilKeycloakSkips(t *testing.T) {
	r := NewCITrustReconciler(nil, nil, testClusterID)
	err := r.Handle(t.Context(), watcher.Event[*pb.CITrust]{
		Type:       watcher.EventCreated,
		ResourceID: "trust-1",
		Resource:   githubTrust(),
	})
	if err != nil {
		t.Fatalf("Handle with nil keycloak should be a no-op, got %v", err)
	}
}

// TestCITrustUpsert_CrossGatewayAndCrossRepoIsolation proves the end-to-end
// wiring preserves per-trust isolation (finding 1): two CITrusts under the same
// owner - one for a different repo, one on a different gateway - each resolve to a
// distinct Keycloak group, so neither can inherit the other's role. The shared
// per-owner workspace label is reported in status for legibility, but it is never
// the group the role binds to.
func TestCITrustUpsert_CrossGatewayAndCrossRepoIsolation(t *testing.T) {
	ensure := func(t *testing.T, trustID, repo, gatewayID, gatewayName, cluster, role string) keycloak.CIFederationSpec {
		t.Helper()
		kc := &fakeCIKeycloak{}
		gw := &fakeGatewaySvc{gateway: &pb.Gateway{
			Metadata:  &pb.ObjectReference{Id: gatewayID},
			Name:      gatewayName,
			ClusterId: cluster,
		}}
		r := newCITrustTestReconciler(kc, githubProvider(), gw, &fakeCITrustSvc{})
		trust := &pb.CITrust{
			Metadata:          &pb.ObjectReference{Id: trustID},
			CiProviderId:      "prov-1",
			GatewayId:         gatewayID,
			SubjectRepository: strptr(repo),
			Role:              role,
		}
		if err := r.Handle(t.Context(), watcher.Event[*pb.CITrust]{
			Type:       watcher.EventCreated,
			ResourceID: trustID,
			Resource:   trust,
		}); err != nil {
			t.Fatalf("Handle(%s): %v", trustID, err)
		}
		if len(kc.ensured) != 1 {
			t.Fatalf("ensure calls for %s = %d, want 1", trustID, len(kc.ensured))
		}
		return kc.ensured[0]
	}

	// Same owner, different repositories, same gateway.
	low := ensure(t, "trust-low", "acme/low", "gw-1", "prod", testClusterID, "openshell-user")
	high := ensure(t, "trust-high", "acme/high", "gw-1", "prod", testClusterID, "openshell-admin")
	// Same owner and repository, but a different gateway (distinct CITrust).
	gwA := ensure(t, "trust-gw-a", "acme/app", "gw-a", "edge-a", testClusterID, "openshell-user")
	gwB := ensure(t, "trust-gw-b", "acme/app", "gw-b", "edge-b", testClusterID, "openshell-user")

	// All share the per-owner workspace label (informational).
	for _, s := range []keycloak.CIFederationSpec{low, high, gwA, gwB} {
		if s.Workspace != "gh-acme" {
			t.Errorf("workspace label = %q, want gh-acme", s.Workspace)
		}
	}
	// But every trust binds its role onto a distinct group.
	groups := map[string]string{}
	for _, s := range []keycloak.CIFederationSpec{low, high, gwA, gwB} {
		if prev, dup := groups[s.GroupName()]; dup {
			t.Errorf("trust %s and %s share group %q; isolation lost", prev, s.TrustID, s.GroupName())
		}
		groups[s.GroupName()] = s.TrustID
	}
}

func TestDeriveWorkspace(t *testing.T) {
	cases := []struct {
		forge, repo, project, want string
	}{
		{"github", "Acme/App", "", "gh-acme"},
		{"gitlab", "", "Acme/group/app", "gl-acme"},
		{"forgejo", "Acme/App", "", "fj-acme"},
		{"other", "", "", "ci-other"},
	}
	for _, tc := range cases {
		if got := deriveWorkspace(tc.forge, tc.repo, tc.project); got != tc.want {
			t.Errorf("deriveWorkspace(%q,%q,%q) = %q, want %q", tc.forge, tc.repo, tc.project, got, tc.want)
		}
	}
}
