package reconciler

import (
	"context"
	"errors"
	"testing"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/control-plane/internal/openshellclient"
	"github.com/openshift-online/hypershell/components/control-plane/internal/watcher"
)

type fakeGatewaySession struct {
	ensuredWorkspace string
	members          map[string]openshellclient.Role
	closed           bool
	ensureErr        error
}

func (s *fakeGatewaySession) EnsureWorkspace(_ context.Context, name string) error {
	s.ensuredWorkspace = name
	return s.ensureErr
}

func (s *fakeGatewaySession) EnsureMembers(_ context.Context, _ string, want map[string]openshellclient.Role) error {
	s.members = want
	return nil
}

func (s *fakeGatewaySession) Close() error { s.closed = true; return nil }

// gatewayWithNamespace is a local gateway fixture carrying a namespace so the
// provisioning path can derive the OpenShell endpoint.
func gatewayWithNamespace() *fakeGatewaySvc {
	return &fakeGatewaySvc{gateway: &pb.Gateway{
		Metadata:  &pb.ObjectReference{Id: "gw-1"},
		Name:      "prod",
		Namespace: "openshell-abc",
		ClusterId: testClusterID,
	}}
}

func TestProvisionGatewayWorkspace_EnsuresWorkspaceAndMembers(t *testing.T) {
	kc := &fakeCIKeycloak{groupMembers: map[string][]string{
		"gh-acme-citrust-trust-1": {"sub-a", "sub-b"},
	}}
	sess := &fakeGatewaySession{}
	var dialedEndpoint, dialedToken string
	var dialedInsecure bool

	r := &CITrustReconciler{
		keycloakClient:        kc,
		ciTrustSvc:            &fakeCITrustSvc{},
		ciProviderSvc:         &fakeCIProviderSvc{provider: githubProvider()},
		gatewaySvc:            gatewayWithNamespace(),
		federationClientID:    defaultCIFederationClientID,
		clusterID:             testClusterID,
		workspaceProvisioning: true,
		gatewayInsecure:       true,
		gatewayBaseDomain:     "example.com",
		dialGateway: func(endpoint, token string, insecure bool) (gatewayWorkspaceSession, error) {
			dialedEndpoint, dialedToken, dialedInsecure = endpoint, token, insecure
			return sess, nil
		},
	}

	if err := r.Handle(context.Background(), watcher.Event[*pb.CITrust]{Type: watcher.EventCreated, ResourceID: "trust-1", Resource: githubTrust()}); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	if dialedEndpoint != "gw-openshell-abc.example.com:443" {
		t.Errorf("endpoint = %q, want gw-openshell-abc.example.com:443", dialedEndpoint)
	}
	if dialedToken != "admin-token" {
		t.Errorf("token = %q, want admin-token", dialedToken)
	}
	if !dialedInsecure {
		t.Error("expected insecure dial")
	}
	if len(kc.mintedFor) != 1 || kc.mintedFor[0] != "gw-1" {
		t.Errorf("minted admin token for %v, want [gw-1]", kc.mintedFor)
	}
	if sess.ensuredWorkspace != "gh-acme" {
		t.Errorf("ensured workspace %q, want gh-acme", sess.ensuredWorkspace)
	}
	want := map[string]openshellclient.Role{"sub-a": openshellclient.RoleUser, "sub-b": openshellclient.RoleUser}
	if len(sess.members) != len(want) {
		t.Fatalf("members = %v, want %v", sess.members, want)
	}
	for s, role := range want {
		if sess.members[s] != role {
			t.Errorf("member %q role = %v, want %v", s, sess.members[s], role)
		}
	}
	if !sess.closed {
		t.Error("session not closed")
	}
}

func TestProvisionGatewayWorkspace_DisabledIsNoop(t *testing.T) {
	kc := &fakeCIKeycloak{}
	dialed := false
	r := &CITrustReconciler{
		keycloakClient:        kc,
		ciTrustSvc:            &fakeCITrustSvc{},
		ciProviderSvc:         &fakeCIProviderSvc{provider: githubProvider()},
		gatewaySvc:            gatewayWithNamespace(),
		federationClientID:    defaultCIFederationClientID,
		clusterID:             testClusterID,
		workspaceProvisioning: false,
		dialGateway: func(string, string, bool) (gatewayWorkspaceSession, error) {
			dialed = true
			return &fakeGatewaySession{}, nil
		},
	}
	if err := r.Handle(context.Background(), watcher.Event[*pb.CITrust]{Type: watcher.EventCreated, ResourceID: "trust-1", Resource: githubTrust()}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if dialed {
		t.Error("gateway should not be dialed when provisioning is disabled")
	}
	if len(kc.ensured) != 1 {
		t.Errorf("keycloak federation should still be realized, got %d", len(kc.ensured))
	}
}

func TestProvisionGatewayWorkspace_ErrorPropagates(t *testing.T) {
	kc := &fakeCIKeycloak{groupMembers: map[string][]string{"gh-acme-citrust-trust-1": {"sub-a"}}}
	r := &CITrustReconciler{
		keycloakClient:        kc,
		ciTrustSvc:            &fakeCITrustSvc{},
		ciProviderSvc:         &fakeCIProviderSvc{provider: githubProvider()},
		gatewaySvc:            gatewayWithNamespace(),
		federationClientID:    defaultCIFederationClientID,
		clusterID:             testClusterID,
		workspaceProvisioning: true,
		gatewayBaseDomain:     "example.com",
		dialGateway: func(string, string, bool) (gatewayWorkspaceSession, error) {
			return &fakeGatewaySession{ensureErr: errors.New("boom")}, nil
		},
	}
	err := r.Handle(context.Background(), watcher.Event[*pb.CITrust]{Type: watcher.EventCreated, ResourceID: "trust-1", Resource: githubTrust()})
	if err == nil {
		t.Fatal("expected provisioning error to propagate")
	}
}

func TestProvisionGatewayWorkspace_MissingBaseDomainFails(t *testing.T) {
	kc := &fakeCIKeycloak{}
	r := &CITrustReconciler{
		keycloakClient:        kc,
		ciTrustSvc:            &fakeCITrustSvc{},
		ciProviderSvc:         &fakeCIProviderSvc{provider: githubProvider()},
		gatewaySvc:            gatewayWithNamespace(),
		federationClientID:    defaultCIFederationClientID,
		clusterID:             testClusterID,
		workspaceProvisioning: true,
		gatewayBaseDomain:     "",
		dialGateway: func(string, string, bool) (gatewayWorkspaceSession, error) {
			return &fakeGatewaySession{}, nil
		},
	}
	if err := r.Handle(context.Background(), watcher.Event[*pb.CITrust]{Type: watcher.EventCreated, ResourceID: "trust-1", Resource: githubTrust()}); err == nil {
		t.Fatal("expected error when GATEWAY_API_BASE_DOMAIN is unset")
	}
}
