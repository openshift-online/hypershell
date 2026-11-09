package reconciler

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/control-plane/internal/keycloak"
	"github.com/openshift-online/hypershell/components/control-plane/internal/watcher"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/test/bufconn"
)

// syncCIKeycloak is a thread-safe ciFederationKeycloak fake. The reconcile queue
// runs Handle on a worker goroutine while the test reads the recorded specs, so the
// unlocked fakeCIKeycloak would race under -race.
type syncCIKeycloak struct {
	mu      sync.Mutex
	ensured []keycloak.CIFederationSpec
}

func (f *syncCIKeycloak) EnsureCIFederation(_ context.Context, spec keycloak.CIFederationSpec) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ensured = append(f.ensured, spec)
	return nil
}

func (f *syncCIKeycloak) DeleteCIFederation(_ context.Context, _ keycloak.CIFederationSpec) error {
	return nil
}

func (f *syncCIKeycloak) Issuer() string {
	return "https://keycloak.example.com/realms/hypershell"
}

func (f *syncCIKeycloak) ensuredSpecs() []keycloak.CIFederationSpec {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]keycloak.CIFederationSpec(nil), f.ensured...)
}

// silentCITrustServer serves the CITrust, CIProvider, and Gateway RPCs the CITrust
// reconciler calls from a fixed inventory and, in WatchCITrusts, sends the
// subscription header and then blocks without ever sending an event. A pre-existing
// trust can therefore reach the reconciler only through the startup seed or the
// periodic resync, never through a live watch event -- exactly the gap CTRL-CORE-20
// closes.
type silentCITrustServer struct {
	pb.UnimplementedCITrustServiceServer
	pb.UnimplementedCIProviderServiceServer
	pb.UnimplementedGatewayServiceServer
	trusts   []*pb.CITrust
	provider *pb.CIProvider
	gateway  *pb.Gateway
}

func (s *silentCITrustServer) ListCITrusts(_ context.Context, req *pb.ListCITrustsRequest) (*pb.ListCITrustsResponse, error) {
	return &pb.ListCITrustsResponse{
		Items:    s.trusts,
		Metadata: &pb.ListMeta{Page: req.GetPage(), Size: req.GetSize(), Total: int32(len(s.trusts))},
	}, nil
}

func (s *silentCITrustServer) WatchCITrusts(_ *pb.WatchCITrustsRequest, stream pb.CITrustService_WatchCITrustsServer) error {
	if err := stream.SendHeader(metadata.MD{}); err != nil {
		return err
	}
	<-stream.Context().Done()
	return nil
}

func (s *silentCITrustServer) UpdateCITrust(_ context.Context, _ *pb.UpdateCITrustRequest) (*pb.UpdateCITrustResponse, error) {
	return &pb.UpdateCITrustResponse{}, nil
}

func (s *silentCITrustServer) GetCIProvider(_ context.Context, _ *pb.GetCIProviderRequest) (*pb.GetCIProviderResponse, error) {
	return &pb.GetCIProviderResponse{CiProvider: s.provider}, nil
}

func (s *silentCITrustServer) GetGateway(_ context.Context, _ *pb.GetGatewayRequest) (*pb.GetGatewayResponse, error) {
	return &pb.GetGatewayResponse{Gateway: s.gateway}, nil
}

func newCITrustSeedConn(t *testing.T, srv *silentCITrustServer) *grpc.ClientConn {
	t.Helper()
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	pb.RegisterCITrustServiceServer(server, srv)
	pb.RegisterCIProviderServiceServer(server, srv)
	pb.RegisterGatewayServiceServer(server, srv)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial CI trust seed server: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// A CITrust that already exists when the control plane starts (its create event
// predates the reconciler or was missed by the almost-always-idle stream) must have
// its Keycloak federation realized through the startup seed, and again on the
// periodic resync, without any live watch event. The resync is idempotent: every
// pass realizes the same CIFederationSpec, and EnsureCIFederation is itself
// idempotent (internal/keycloak/ci_federation.go). This runs the real WatchCITrusts
// entry point so it fails if WatchCITrusts stops seeding or resyncing.
// Verifies: CTRL-CORE-20.01, CTRL-CORE-20.02
func TestWatchCITrusts_StartupSeedsPreExistingTrust(t *testing.T) {
	srv := &silentCITrustServer{
		trusts:   []*pb.CITrust{githubTrust()},
		provider: githubProvider(),
		gateway: &pb.Gateway{
			Metadata:  &pb.ObjectReference{Id: "gw-1"},
			Name:      "prod",
			ClusterId: testClusterID,
		},
	}
	conn := newCITrustSeedConn(t, srv)

	kc := &syncCIKeycloak{}
	r := &CITrustReconciler{
		keycloakClient:     kc,
		ciTrustSvc:         pb.NewCITrustServiceClient(conn),
		ciProviderSvc:      pb.NewCIProviderServiceClient(conn),
		gatewaySvc:         pb.NewGatewayServiceClient(conn),
		federationClientID: defaultCIFederationClientID,
		clusterID:          testClusterID,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	// A short resync interval so the periodic pass runs within the test window; the
	// idle watchdog is disabled (0).
	go func() { done <- watcher.WatchCITrusts(ctx, conn, r, 0, 25*time.Millisecond) }()

	// Wait for the seed pass AND at least one resync pass to realize the trust.
	deadline := time.After(10 * time.Second)
	var specs []keycloak.CIFederationSpec
	for {
		specs = kc.ensuredSpecs()
		if len(specs) >= 2 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("EnsureCIFederation calls = %d, want >= 2 (seed + resync)", len(specs))
		case <-time.After(10 * time.Millisecond):
		}
	}

	// Every pass realizes the same desired state -- the resync is idempotent.
	want := specs[0]
	if want.TrustID != "trust-1" {
		t.Errorf("TrustID = %q, want trust-1", want.TrustID)
	}
	if want.IdpAlias != "github-actions" {
		t.Errorf("IdpAlias = %q, want the adopted github-actions alias", want.IdpAlias)
	}
	if want.GatewayClientID != "gw-1" {
		t.Errorf("GatewayClientID = %q, want gw-1", want.GatewayClientID)
	}
	if want.Workspace != "gh-acme" {
		t.Errorf("Workspace = %q, want gh-acme", want.Workspace)
	}
	if want.Role != "openshell-user" {
		t.Errorf("Role = %q, want openshell-user", want.Role)
	}
	for i, got := range specs {
		if got.GroupName() != want.GroupName() || got.GatewayClientID != want.GatewayClientID ||
			got.IdpAlias != want.IdpAlias || got.Workspace != want.Workspace || got.Role != want.Role {
			t.Errorf("pass %d realized a different spec %+v, want idempotent %+v", i, got, want)
		}
	}

	cancel()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("WatchCITrusts did not return after cancel")
	}
}

// A trust whose referenced gateway belongs to another cluster must be skipped even
// though the unfiltered platform-scoped list still returns it: the per-trust
// reconcile, not the list, enforces ownership (CITrust carries no cluster filter).
// Verifies: CTRL-CORE-20.03
func TestWatchCITrusts_SeedSkipsForeignClusterTrust(t *testing.T) {
	srv := &silentCITrustServer{
		trusts:   []*pb.CITrust{githubTrust()},
		provider: githubProvider(),
		gateway: &pb.Gateway{
			Metadata:  &pb.ObjectReference{Id: "gw-1"},
			Name:      "prod",
			ClusterId: "some-other-cluster",
		},
	}
	conn := newCITrustSeedConn(t, srv)

	kc := &syncCIKeycloak{}
	r := &CITrustReconciler{
		keycloakClient:     kc,
		ciTrustSvc:         pb.NewCITrustServiceClient(conn),
		ciProviderSvc:      pb.NewCIProviderServiceClient(conn),
		gatewaySvc:         pb.NewGatewayServiceClient(conn),
		federationClientID: defaultCIFederationClientID,
		clusterID:          testClusterID,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- watcher.WatchCITrusts(ctx, conn, r, 0, 25*time.Millisecond) }()

	// Give the seed and a couple of resync passes time to run; none must realize a
	// foreign-cluster trust.
	time.Sleep(200 * time.Millisecond)
	if specs := kc.ensuredSpecs(); len(specs) != 0 {
		t.Fatalf("foreign-cluster trust must not be realized; got %d ensure calls", len(specs))
	}

	cancel()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("WatchCITrusts did not return after cancel")
	}
}
