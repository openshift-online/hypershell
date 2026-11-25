package authz_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v4"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/hypershell/components/api-server/plugins/roleBindings"
	"github.com/openshift-online/hypershell/components/api-server/plugins/users"
	"github.com/openshift-online/hypershell/components/api-server/test"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/environments"
)

// These tests pin authorization for ManagedCluster records and cluster-scoped
// gRPC writes with RBAC enforced and one bootstrap-allowlisted account. Each
// scenario supplies its JWT roles explicitly. Creator attackers verify that
// gateway creation privilege never bypasses narrower tenant boundaries.

type bearer struct{ token string }

func (b bearer) GetRequestMetadata(context.Context, ...string) (map[string]string, error) {
	return map[string]string{"authorization": "Bearer " + b.token}, nil
}
func (bearer) RequireTransportSecurity() bool { return false }

type principal struct {
	name  string
	token string
	rest  context.Context
	conn  *grpc.ClientConn
}

func newPrincipal(t *testing.T, h *test.Helper, username string, extra jwt.MapClaims) principal {
	t.Helper()
	account := h.NewAccount(username, username, "")
	token := h.CreateJWTStringWithClaims(account, extra)
	conn, err := grpc.NewClient(h.GRPCAddress(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithPerRPCCredentials(bearer{token: token}))
	Expect(err).NotTo(HaveOccurred())
	t.Cleanup(func() { _ = conn.Close() })
	return principal{
		name:  username,
		token: token,
		rest:  context.WithValue(context.Background(), openapi.ContextAccessToken, token),
		conn:  conn,
	}
}

func realmRoles(roles ...string) jwt.MapClaims {
	values := make([]interface{}, len(roles))
	for i, r := range roles {
		values[i] = r
	}
	return jwt.MapClaims{"realm_access": map[string]interface{}{"roles": values}}
}

// spoke is a registered control plane: a client_credentials principal holding
// managed-cluster-registrar that registered its ManagedCluster over REST.
type spoke struct {
	principal
	subject   string
	name      string
	clusterID string
}

func registerSpoke(t *testing.T, h *test.Helper, client *openapi.APIClient) spoke {
	t.Helper()
	suffix := strings.ToLower(api.NewID())
	subject := "cp-" + suffix
	p := newPrincipal(t, h, "service-account-cp-"+suffix, test.ControlPlaneClaims(subject))
	s := spoke{principal: p, subject: subject, name: "mc-" + suffix}
	s.clusterID = registerAs(t, client, s)
	return s
}

func registerAs(t *testing.T, client *openapi.APIClient, s spoke) string {
	t.Helper()
	resp, httpResp, err := client.DefaultAPI.RegisterManagedCluster(s.rest).
		ManagedClusterRegistrationRequest(openapi.ManagedClusterRegistrationRequest{Name: s.name, Capabilities: spokeCapabilities(s)}).Execute()
	Expect(err).NotTo(HaveOccurred(), "register %s: HTTP %d %s", s.name, statusOf(httpResp), test.APIErrorReason(err))
	spokeRegionByCluster.Store(resp.ClusterId, spokeRegion(s.name))
	return resp.ClusterId
}

// spokeRegion is the per-spoke region capability that makes a spoke the ONLY
// cluster a placement selector matches. Pinning by cluster_id needs platform:admin
// (SEC-RBAC-34), so non-admin creates in this suite target their spoke this way.
func spokeRegion(name string) string { return "region/" + name }

func spokeCapabilities(s spoke) []string {
	return []string{"network/public", "provider/aws", spokeRegion(s.name)}
}

// spokeRegionByCluster maps a registered cluster id to its unique region label so
// helpers that take a cluster id can build a placement selector for it.
var spokeRegionByCluster sync.Map

func regionForCluster(clusterID string) string {
	label, ok := spokeRegionByCluster.Load(clusterID)
	Expect(ok).To(BeTrue(), "no spoke region recorded for cluster %s", clusterID)
	return label.(string)
}

// grpcPlacementFor returns a gRPC placement intent that resolves to exactly the
// given test spoke.
func grpcPlacementFor(clusterID string) *pb.GatewayPlacementIntent {
	return &pb.GatewayPlacementIntent{Placement: &pb.GatewayPlacementIntent_CapabilitySelector{
		CapabilitySelector: &pb.CapabilitySelectorPlacement{Selector: []string{regionForCluster(clusterID)}},
	}}
}

func statusOf(resp *http.Response) int {
	if resp == nil {
		return 0
	}
	return resp.StatusCode
}

func grpcCode(err error) codes.Code { return status.Code(err) }

// Verifies: SEC-RBAC-13.02
func TestRESTGatewayCreateRequiresExplicitCreatorRole(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	cluster := registerSpoke(t, h, client)

	for _, tc := range []struct {
		name   string
		claims jwt.MapClaims
		status int
	}{
		{name: "no-realm-roles", status: http.StatusForbidden},
		{name: "platform-admin-only", claims: realmRoles("platform:admin"), status: http.StatusForbidden},
		{name: "gateway-creator", claims: realmRoles("gateway:creator"), status: http.StatusCreated},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			caller := newPrincipal(t, h, "user-"+strings.ToLower(api.NewID()), tc.claims)
			created, response, err := client.DefaultAPI.CreateGateway(caller.rest).
				GatewayCreateRequest(gatewayCreateRequest(uniqueName("explicit-role"), cluster.clusterID, false)).Execute()
			g.Expect(statusOf(response)).To(Equal(tc.status), "gateway create: %s", test.APIErrorReason(err))
			if tc.status == http.StatusCreated {
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(created.GetId()).NotTo(BeEmpty())
			} else {
				g.Expect(err).To(HaveOccurred())
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ManagedCluster deletion, REST
// ---------------------------------------------------------------------------

// Verifies: PLAT-CLUS-03.01, PLAT-CLUS-03.02, PLAT-CLUS-03.03
func TestRESTDeleteManagedClusterIsAdminOrSelf(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	victim := registerSpoke(t, h, client)
	other := registerSpoke(t, h, client)
	user := newPrincipal(t, h, "user-"+strings.ToLower(api.NewID()), realmRoles("gateway:creator"))
	admin := newPrincipal(t, h, "admin-"+strings.ToLower(api.NewID()), realmRoles("platform:admin"))

	// A caller explicitly granted gateway:creator must not be able to
	// deregister somebody else's control plane.
	httpResp, err := client.DefaultAPI.DeleteManagedCluster(user.rest, victim.clusterID).Execute()
	Expect(err).To(HaveOccurred(), "an ordinary user deleted ManagedCluster %s", victim.clusterID)
	Expect(statusOf(httpResp)).To(Equal(http.StatusForbidden))

	// Another registered control plane must not either.
	httpResp, err = client.DefaultAPI.DeleteManagedCluster(other.rest, victim.clusterID).Execute()
	Expect(err).To(HaveOccurred(), "control plane %s deleted another cluster's record", other.name)
	Expect(statusOf(httpResp)).To(Equal(http.StatusForbidden))

	_, httpResp, err = client.DefaultAPI.GetManagedCluster(admin.rest, victim.clusterID).Execute()
	Expect(err).NotTo(HaveOccurred(), "the record must survive the refused deletes (HTTP %d)", statusOf(httpResp))

	// A control plane may deregister itself (bin/teardown-cluster relies on it).
	httpResp, err = client.DefaultAPI.DeleteManagedCluster(victim.rest, victim.clusterID).Execute()
	Expect(err).NotTo(HaveOccurred(), "self-deregistration refused: HTTP %d", statusOf(httpResp))

	// platform:admin may delete any record.
	httpResp, err = client.DefaultAPI.DeleteManagedCluster(admin.rest, other.clusterID).Execute()
	Expect(err).NotTo(HaveOccurred(), "platform:admin delete refused: HTTP %d", statusOf(httpResp))
}

func TestRESTManagedClusterWritesRequirePlatformAdmin(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	victim := registerSpoke(t, h, client)
	user := newPrincipal(t, h, "user-"+strings.ToLower(api.NewID()), realmRoles("gateway:creator"))
	admin := newPrincipal(t, h, "admin-"+strings.ToLower(api.NewID()), realmRoles("platform:admin"))

	// Renaming a registered record makes its control plane's next registration
	// a 409 ("registered under a different name").
	_, httpResp, err := client.DefaultAPI.UpdateManagedCluster(user.rest, victim.clusterID).
		ManagedClusterPatchRequest(openapi.ManagedClusterPatchRequest{Name: openapi.PtrString("hijacked")}).Execute()
	Expect(err).To(HaveOccurred(), "an ordinary user renamed ManagedCluster %s", victim.clusterID)
	Expect(statusOf(httpResp)).To(Equal(http.StatusForbidden))

	// A manual placeholder squats a name, so the spoke that later registers
	// under it gets a permanent 409.
	squat := "mc-" + strings.ToLower(api.NewID())
	_, httpResp, err = client.DefaultAPI.CreateManagedCluster(user.rest).
		ManagedCluster(*openapi.NewManagedCluster(squat, "")).Execute()
	Expect(err).To(HaveOccurred(), "an ordinary user created placeholder %s", squat)
	Expect(statusOf(httpResp)).To(Equal(http.StatusForbidden))

	// Operators keep both.
	_, httpResp, err = client.DefaultAPI.CreateManagedCluster(admin.rest).
		ManagedCluster(*openapi.NewManagedCluster(squat, "")).Execute()
	Expect(err).NotTo(HaveOccurred(), "platform:admin create refused: HTTP %d", statusOf(httpResp))

	// Reads stay open to gateway:creator: the console's placement picker and
	// the GitOps teardown list clusters with a creator token.
	_, httpResp, err = client.DefaultAPI.ListManagedClusters(user.rest).Execute()
	Expect(err).NotTo(HaveOccurred(), "list refused: HTTP %d", statusOf(httpResp))
}

// ---------------------------------------------------------------------------
// ManagedCluster deletion, gRPC
// ---------------------------------------------------------------------------

// Verifies: PLAT-CLUS-03.01, PLAT-CLUS-03.02, PLAT-CLUS-03.03
func TestGRPCDeleteManagedClusterIsAdminOrSelf(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	victim := registerSpoke(t, h, client)
	other := registerSpoke(t, h, client)
	user := newPrincipal(t, h, "user-"+strings.ToLower(api.NewID()), realmRoles("gateway:creator"))
	allowlisted := newPrincipal(t, h, allowlistedAccount, nil)
	admin := newPrincipal(t, h, "admin-"+strings.ToLower(api.NewID()), realmRoles("platform:admin"))

	del := func(p principal, id string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, err := pb.NewManagedClusterServiceClient(p.conn).DeleteManagedCluster(ctx, &pb.DeleteManagedClusterRequest{Id: id})
		return err
	}

	Expect(grpcCode(del(user, victim.clusterID))).To(Equal(codes.PermissionDenied), "ordinary user")
	Expect(grpcCode(del(other.principal, victim.clusterID))).To(Equal(codes.PermissionDenied), "another control plane")
	Expect(grpcCode(del(allowlisted, victim.clusterID))).To(Equal(codes.PermissionDenied), "bootstrap allowlist")

	Expect(del(victim.principal, victim.clusterID)).To(Succeed(), "self-deregistration")
	Expect(del(admin, other.clusterID)).To(Succeed(), "platform:admin")
}

// ---------------------------------------------------------------------------
// Re-registration after deletion
// ---------------------------------------------------------------------------

// A deleted record must not lock its control plane out: the record is
// soft-deleted and its (oidc_subject, name) index entry survives, which used to
// turn the next registration into a permanent 409. It must not come back under
// a new id either: the control plane would restart under that id, see none of
// its gateways as live, and its namespace GC would reap them. Registration
// restores the same record.
// Verifies: PLAT-CLUS-01.03
func TestReRegistrationAfterDeleteRestoresTheRecord(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	s := registerSpoke(t, h, client)
	owner := newPrincipal(t, h, "owner-"+strings.ToLower(api.NewID()), realmRoles("gateway:creator"))
	gw := createGateway(t, client, owner, s.clusterID)

	httpResp, err := client.DefaultAPI.DeleteManagedCluster(s.rest, s.clusterID).Execute()
	Expect(err).NotTo(HaveOccurred(), "self-deregistration: HTTP %d", statusOf(httpResp))

	// While deleted, the spoke is not a control-plane identity for its gateways.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err = pb.NewGatewayServiceClient(s.conn).SetGatewayVersion(ctx, &pb.SetGatewayVersionRequest{Id: gw.GetId(), GatewayVersion: "0.0.1"})
	Expect(grpcCode(err)).To(Equal(codes.PermissionDenied), "a deregistered spoke wrote a gateway: %v", err)

	resp, httpResp, err := client.DefaultAPI.RegisterManagedCluster(s.rest).
		ManagedClusterRegistrationRequest(openapi.ManagedClusterRegistrationRequest{Name: s.name, Capabilities: spokeCapabilities(s)}).Execute()
	Expect(err).NotTo(HaveOccurred(), "re-registration after delete: HTTP %d %s", statusOf(httpResp), test.APIErrorReason(err))
	Expect(resp.ClusterId).To(Equal(s.clusterID), "the record must come back under its original id")

	_, err = pb.NewGatewayServiceClient(s.conn).SetGatewayVersion(ctx, &pb.SetGatewayVersionRequest{Id: gw.GetId(), GatewayVersion: "0.0.1"})
	Expect(err).NotTo(HaveOccurred(), "the restored control plane must serve its gateways again")
}

// A name another control plane registered while the record was deleted is not
// taken back: that is a name collision (409), as for any held name.
func TestReRegistrationAfterDeleteDoesNotReclaimATakenName(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	s := registerSpoke(t, h, client)
	admin := newPrincipal(t, h, "admin-"+strings.ToLower(api.NewID()), realmRoles("platform:admin"))

	httpResp, err := client.DefaultAPI.DeleteManagedCluster(admin.rest, s.clusterID).Execute()
	Expect(err).NotTo(HaveOccurred(), "platform:admin delete: HTTP %d", statusOf(httpResp))

	suffix := strings.ToLower(api.NewID())
	other := spoke{
		principal: newPrincipal(t, h, "service-account-cp-"+suffix, test.ControlPlaneClaims("cp-"+suffix)),
		subject:   "cp-" + suffix,
		name:      s.name,
	}
	other.clusterID = registerAs(t, client, other)

	_, httpResp, err = client.DefaultAPI.RegisterManagedCluster(s.rest).
		ManagedClusterRegistrationRequest(openapi.ManagedClusterRegistrationRequest{Name: s.name, Capabilities: spokeCapabilities(s)}).Execute()
	Expect(err).To(HaveOccurred(), "a deleted control plane took back a name another one registered")
	Expect(statusOf(httpResp)).To(Equal(http.StatusConflict))
}

// ---------------------------------------------------------------------------
// Control-plane writes are scoped to the caller's own cluster
// ---------------------------------------------------------------------------

func createGateway(t *testing.T, client *openapi.APIClient, owner principal, clusterID string) *openapi.Gateway {
	t.Helper()
	_ = client
	created, err := pb.NewGatewayServiceClient(owner.conn).CreateGateway(context.Background(), &pb.CreateGatewayRequest{
		Name:      fmt.Sprintf("gw-%s", strings.ToLower(api.NewID())[:10]),
		Placement: grpcPlacementFor(clusterID),
	})
	Expect(err).NotTo(HaveOccurred(), "create gateway")
	id := created.GetGateway().GetMetadata().GetId()
	user, svcErr := users.Service(&environments.Environment().Services).GetByUsername(context.Background(), owner.name)
	Expect(svcErr).NotTo(HaveOccurred(), "find gateway owner")
	Expect(roleBindings.Service(&environments.Environment().Services).CreateGatewayOwnerBinding(
		context.Background(), user.ID, id,
	)).To(Succeed(), "bind gateway owner")
	return &openapi.Gateway{Id: &id, Namespace: created.GetGateway().GetNamespace()}
}

// Verifies: PLAT-CLUS-11.02, PLAT-CLUS-11.06
func TestGRPCControlPlaneWritesAreScopedToItsCluster(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	a := registerSpoke(t, h, client)
	b := registerSpoke(t, h, client)
	owner := newPrincipal(t, h, "owner-"+strings.ToLower(api.NewID()), realmRoles("gateway:creator"))
	gwA := createGateway(t, client, owner, a.clusterID)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	gwB := pb.NewGatewayServiceClient(b.conn)
	gwAClient := pb.NewGatewayServiceClient(a.conn)

	_, err := gwB.SetGatewayVersion(ctx, &pb.SetGatewayVersionRequest{Id: gwA.GetId(), GatewayVersion: "evil"})
	Expect(grpcCode(err)).To(Equal(codes.PermissionDenied), "SetGatewayVersion on another cluster's gateway: %v", err)
	_, err = gwB.AdjustActiveSandboxCount(ctx, &pb.AdjustActiveSandboxCountRequest{Namespace: gwA.GetNamespace(), Delta: 5})
	Expect(grpcCode(err)).To(Equal(codes.PermissionDenied), "AdjustActiveSandboxCount on another cluster's gateway: %v", err)
	_, err = gwB.SetActiveSandboxCount(ctx, &pb.SetActiveSandboxCountRequest{Namespace: gwA.GetNamespace(), Count: 5})
	Expect(grpcCode(err)).To(Equal(codes.PermissionDenied), "SetActiveSandboxCount on another cluster's gateway: %v", err)
	_, err = gwB.UpdateGateway(ctx, &pb.UpdateGatewayRequest{Id: gwA.GetId(), Image: strPtr("quay.io/evil/gateway:latest")})
	Expect(grpcCode(err)).To(Equal(codes.PermissionDenied), "UpdateGateway on another cluster's gateway: %v", err)
	// PLAT-CLUS-11.06 / SEC-RBAC-09: GetGateway is a fleet-wide read for a
	// control-plane identity. Control plane B resolves A's gateway by id (so the
	// hub can read a spoke-placed gateway while reconciling), NOT NOT_FOUND. The
	// writes above stay own-cluster only.
	readForeign, err := gwB.GetGateway(ctx, &pb.GetGatewayRequest{Id: gwA.GetId()})
	Expect(err).NotTo(HaveOccurred(), "GetGateway on another cluster's gateway should be a fleet-wide read: %v", err)
	Expect(readForeign.GetGateway().GetMetadata().GetId()).To(Equal(gwA.GetId()))
	_, err = gwB.DeleteGateway(ctx, &pb.DeleteGatewayRequest{Id: gwA.GetId()})
	Expect(grpcCode(err)).To(Equal(codes.PermissionDenied), "DeleteGateway on another cluster's gateway: %v", err)

	// The owning control plane keeps every write it performs today...
	_, err = gwAClient.SetGatewayVersion(ctx, &pb.SetGatewayVersionRequest{Id: gwA.GetId(), GatewayVersion: "0.0.1"})
	Expect(err).NotTo(HaveOccurred())
	_, err = gwAClient.AdjustActiveSandboxCount(ctx, &pb.AdjustActiveSandboxCountRequest{Namespace: gwA.GetNamespace(), Delta: 1})
	Expect(err).NotTo(HaveOccurred())
	_, err = gwAClient.UpdateGateway(ctx, &pb.UpdateGatewayRequest{Id: gwA.GetId(), Status: strPtr("Ready")})
	Expect(err).NotTo(HaveOccurred())
	_, err = gwAClient.GetGateway(ctx, &pb.GetGatewayRequest{Id: gwA.GetId()})
	Expect(err).NotTo(HaveOccurred())

	// ...but cannot hand its gateway to (or pull one from) another cluster.
	_, err = gwAClient.UpdateGateway(ctx, &pb.UpdateGatewayRequest{Id: gwA.GetId(), ClusterId: strPtr(b.clusterID)})
	Expect(grpcCode(err)).To(Equal(codes.PermissionDenied), "re-pointing cluster_id: %v", err)
}

// deleteConvergenceWithin10m reads the durable count of delete windows that
// converged inside budget, the metric SLO 2 tracks (OBS-SLI-01).
func deleteConvergenceWithin10m() int64 {
	var n int64
	sf := environments.Environment().Database.SessionFactory
	Expect(sf.New(context.Background()).Raw(
		`SELECT COALESCE((SELECT total FROM gateway_convergence_counters WHERE change_type = 'delete' AND outcome = 'within_10m'), 0)`,
	).Row().Scan(&n)).NotTo(HaveOccurred())
	return n
}

// TestGRPCControlPlaneFinalizesOwnTombstoneConvergesDelete is the end-to-end
// regression for GTW-DEL-10 through the real gRPC interceptor chain: after a
// gateway is soft-deleted, its own control plane reports finalization with
// FinalizeGatewayDeletion and the delete window converges within_10m. A foreign
// control plane is refused, and DeleteGateway stays denied to the control-plane
// identity even for its own tombstone (SEC-RBAC-09.10, PLAT-CLUS-11.07). On
// current main the control plane can only re-call DeleteGateway, which the
// interceptor denies, so the window never converges here: this test fails.
// Verifies: GTW-DEL-10.01, GTW-DEL-10.02, SEC-RBAC-09.10, PLAT-CLUS-11.07, OBS-SLI-01.03
func TestGRPCControlPlaneFinalizesOwnTombstoneConvergesDelete(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	a := registerSpoke(t, h, client)
	b := registerSpoke(t, h, client)
	owner := newPrincipal(t, h, "owner-"+strings.ToLower(api.NewID()), realmRoles("gateway:creator"))
	gw := createGateway(t, client, owner, a.clusterID)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	gwA := pb.NewGatewayServiceClient(a.conn)
	gwB := pb.NewGatewayServiceClient(b.conn)

	// Converge the create, then the owner soft-deletes the gateway, opening a
	// delete window.
	running := "Running"
	var observed int64 = 1
	_, err := gwA.UpdateGateway(ctx, &pb.UpdateGatewayRequest{Id: gw.GetId(), Phase: &running, ObservedGeneration: &observed})
	Expect(err).NotTo(HaveOccurred())
	_, err = pb.NewGatewayServiceClient(owner.conn).DeleteGateway(ctx, &pb.DeleteGatewayRequest{Id: gw.GetId()})
	Expect(err).NotTo(HaveOccurred(), "owner soft-deletes the gateway: %v", err)

	before := deleteConvergenceWithin10m()

	// A foreign control plane cannot finalize this cluster's tombstone.
	_, err = gwB.FinalizeGatewayDeletion(ctx, &pb.FinalizeGatewayDeletionRequest{Id: gw.GetId()})
	Expect(grpcCode(err)).To(Equal(codes.PermissionDenied), "foreign control plane finalizing the tombstone: %v", err)

	// DeleteGateway stays denied to the control-plane identity, even for its own
	// tombstone: finalization is the dedicated RPC's job, not a DeleteGateway re-call.
	_, err = gwA.DeleteGateway(ctx, &pb.DeleteGatewayRequest{Id: gw.GetId()})
	Expect(grpcCode(err)).To(Equal(codes.PermissionDenied), "control plane re-calling DeleteGateway on its own tombstone: %v", err)

	// The owning control plane finalizes its own tombstone: the delete window
	// converges within budget.
	_, err = gwA.FinalizeGatewayDeletion(ctx, &pb.FinalizeGatewayDeletionRequest{Id: gw.GetId()})
	Expect(err).NotTo(HaveOccurred(), "own control plane finalizing its tombstone: %v", err)
	Expect(deleteConvergenceWithin10m()).To(Equal(before+1), "delete did not converge within_10m after finalization")
}

// ---------------------------------------------------------------------------
// Users on gRPC get the HTTP rules, not "creator may call anything"
// ---------------------------------------------------------------------------

// Verifies: SEC-RBAC-09.03, SEC-RBAC-09.04
func TestGRPCUserCallsFollowHTTPGatewayRules(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	a := registerSpoke(t, h, client)
	owner := newPrincipal(t, h, "owner-"+strings.ToLower(api.NewID()), realmRoles("gateway:creator"))
	attacker := newPrincipal(t, h, "attacker-"+strings.ToLower(api.NewID()), realmRoles("gateway:creator"))
	admin := newPrincipal(t, h, "admin-"+strings.ToLower(api.NewID()), realmRoles("platform:admin"))
	gw := createGateway(t, client, owner, a.clusterID)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	evil := pb.NewGatewayServiceClient(attacker.conn)

	_, err := evil.UpdateGateway(ctx, &pb.UpdateGatewayRequest{Id: gw.GetId(), Image: strPtr("quay.io/evil/gateway:latest")})
	Expect(grpcCode(err)).To(Equal(codes.PermissionDenied), "a non-owner rewrote a gateway image over gRPC: %v", err)
	_, err = evil.DeleteGateway(ctx, &pb.DeleteGatewayRequest{Id: gw.GetId()})
	Expect(grpcCode(err)).To(Equal(codes.PermissionDenied), "a non-owner deleted a gateway over gRPC: %v", err)
	_, err = evil.GetGateway(ctx, &pb.GetGatewayRequest{Id: gw.GetId()})
	Expect(grpcCode(err)).To(Equal(codes.NotFound), "a non-owner read a gateway over gRPC: %v", err)
	_, err = evil.ListGateways(ctx, &pb.ListGatewaysRequest{})
	Expect(grpcCode(err)).To(Equal(codes.PermissionDenied), "a non-admin listed every gateway over gRPC: %v", err)
	_, err = pb.NewRoleBindingServiceClient(attacker.conn).ListRoleBindings(ctx, &pb.ListRoleBindingsRequest{UserId: strPtr("anyone")})
	Expect(grpcCode(err)).To(Equal(codes.PermissionDenied), "a user listed another user's bindings over gRPC: %v", err)

	// The owner and platform:admin keep what HTTP gives them.
	_, err = pb.NewGatewayServiceClient(owner.conn).GetGateway(ctx, &pb.GetGatewayRequest{Id: gw.GetId()})
	Expect(err).NotTo(HaveOccurred())
	_, err = pb.NewGatewayServiceClient(admin.conn).ListGateways(ctx, &pb.ListGatewaysRequest{})
	Expect(err).NotTo(HaveOccurred())
	_, err = pb.NewGatewayServiceClient(owner.conn).DeleteGateway(ctx, &pb.DeleteGatewayRequest{Id: gw.GetId()})
	Expect(err).NotTo(HaveOccurred())
}

func strPtr(s string) *string { return &s }

// TestGatewayClusterTargetingRequiresRole runs with RBAC on. A gateway:creator is
// denied a cluster_id pin and a required purpose/ label over REST and gRPC, with a
// message naming gateway:cluster-targeting, but may place by an ordinary selector
// and may exclude a purpose/ label. gateway:cluster-targeting and platform:admin
// may pin. An import may not pin, whatever roles its Repository owner holds.
// Verifies: SEC-RBAC-34.01, SEC-RBAC-34.02, SEC-RBAC-34.03, SEC-RBAC-34.04, SEC-RBAC-35.01, SEC-RBAC-35.02, SEC-RBAC-35.03
func TestGatewayClusterTargetingRequiresRole(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	cluster := registerSpoke(t, h, client)
	creator := newPrincipal(t, h, "user-"+strings.ToLower(api.NewID()), realmRoles("gateway:creator"))
	targeter := newPrincipal(t, h, "user-"+strings.ToLower(api.NewID()), realmRoles("gateway:creator", "gateway:cluster-targeting"))
	admin := newPrincipal(t, h, "user-"+strings.ToLower(api.NewID()), realmRoles("gateway:creator", "platform:admin"))

	pinned := func(prefix string) openapi.GatewayCreateRequest {
		return openapi.GatewayCreateRequest{Name: uniqueName(prefix), ClusterId: openapi.PtrString(cluster.clusterID)}
	}

	// A gateway:creator may not pin, over REST or gRPC.
	_, resp, err := client.DefaultAPI.CreateGateway(creator.rest).GatewayCreateRequest(pinned("pin-rest")).Execute()
	Expect(statusOf(resp)).To(Equal(http.StatusForbidden), "creator REST pin: %s", test.APIErrorReason(err))
	Expect(test.APIErrorReason(err)).To(ContainSubstring("gateway:cluster-targeting"))
	_, err = pb.NewGatewayServiceClient(creator.conn).CreateGateway(context.Background(), &pb.CreateGatewayRequest{Name: uniqueName("pin-grpc"), ClusterId: cluster.clusterID})
	Expect(grpcCode(err)).To(Equal(codes.PermissionDenied), "creator gRPC pin: %v", err)
	Expect(err.Error()).To(ContainSubstring("placement selector"))

	// A gateway:creator may not require a purpose/ label, but may exclude one.
	required := openapi.GatewayCreateRequest{Name: uniqueName("placement-required")}
	required.SetPlacement(openapi.GatewayPlacementIntent{Selector: []string{"purpose/release-check"}})
	_, resp, err = client.DefaultAPI.CreateGateway(creator.rest).GatewayCreateRequest(required).Execute()
	Expect(statusOf(resp)).To(Equal(http.StatusForbidden), "creator requiring purpose/: %s", test.APIErrorReason(err))
	Expect(test.APIErrorReason(err)).To(ContainSubstring("purpose/"))
	excluded := openapi.GatewayCreateRequest{Name: uniqueName("placement-excluded")}
	excluded.SetPlacement(openapi.GatewayPlacementIntent{Selector: []string{regionForCluster(cluster.clusterID)}, ExcludedSelector: []string{"purpose/release-check"}})
	_, resp, err = client.DefaultAPI.CreateGateway(creator.rest).GatewayCreateRequest(excluded).Execute()
	Expect(statusOf(resp)).To(Equal(http.StatusCreated), "creator excluding purpose/: %s", test.APIErrorReason(err))

	// A gateway:creator still places by an ordinary capability selector.
	placed, resp, err := client.DefaultAPI.CreateGateway(creator.rest).GatewayCreateRequest(gatewayCreateRequest(uniqueName("placed"), cluster.clusterID, false)).Execute()
	Expect(statusOf(resp)).To(Equal(http.StatusCreated), "creator placement create: %s", test.APIErrorReason(err))
	Expect(placed.GetClusterId()).To(Equal(cluster.clusterID))

	// gateway:cluster-targeting and platform:admin may pin and require purpose/.
	for name, p := range map[string]principal{"cluster-targeting": targeter, "platform:admin": admin} {
		created, resp, err := client.DefaultAPI.CreateGateway(p.rest).GatewayCreateRequest(pinned("pin-ok")).Execute()
		Expect(statusOf(resp)).To(Equal(http.StatusCreated), "%s REST pin: %s", name, test.APIErrorReason(err))
		Expect(created.GetClusterId()).To(Equal(cluster.clusterID))
		label := openapi.GatewayCreateRequest{Name: uniqueName("placement-ok")}
		label.SetPlacement(openapi.GatewayPlacementIntent{Selector: []string{"purpose/unmatched-" + strings.ToLower(api.NewID())}})
		_, resp, err = client.DefaultAPI.CreateGateway(p.rest).GatewayCreateRequest(label).Execute()
		Expect(statusOf(resp)).To(Equal(http.StatusCreated), "%s requiring purpose/: %s", name, test.APIErrorReason(err))
	}

	// An import may not pin. The control plane carries the Repository binding.
	repo := registerImportRepository(t, client, admin)
	_, err = pb.NewGatewayServiceClient(cluster.conn).CreateGateway(importMD(repo), &pb.CreateGatewayRequest{Name: uniqueName("import-pin"), ClusterId: cluster.clusterID})
	Expect(grpcCode(err)).To(Equal(codes.PermissionDenied), "import pin: %v", err)
	Expect(err.Error()).To(ContainSubstring("import"))
}
