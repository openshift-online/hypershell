package authz_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
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
	"github.com/openshift-online/hypershell/components/api-server/test"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
)

// These tests pin the authorization of ManagedCluster records and of the
// cluster-scoped gRPC writes under the GitOps hub configuration (see
// TestMain): every authenticated principal holds gateway:creator, spokes are
// registered control planes, and one account is on the bootstrap allowlist.

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
		ManagedClusterRegistrationRequest(openapi.ManagedClusterRegistrationRequest{Name: s.name}).Execute()
	Expect(err).NotTo(HaveOccurred(), "register %s: HTTP %d %s", s.name, statusOf(httpResp), test.APIErrorReason(err))
	return resp.ClusterId
}

func statusOf(resp *http.Response) int {
	if resp == nil {
		return 0
	}
	return resp.StatusCode
}

func grpcCode(err error) codes.Code { return status.Code(err) }

// ---------------------------------------------------------------------------
// ManagedCluster deletion, REST
// ---------------------------------------------------------------------------

func TestRESTDeleteManagedClusterIsAdminOrSelf(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	victim := registerSpoke(t, h, client)
	other := registerSpoke(t, h, client)
	user := newPrincipal(t, h, "user-"+strings.ToLower(api.NewID()), nil)
	admin := newPrincipal(t, h, "admin-"+strings.ToLower(api.NewID()), realmRoles("platform:admin"))

	// An ordinary signed-in user holds gateway:creator (default role) and must
	// not be able to deregister somebody else's control plane.
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
	user := newPrincipal(t, h, "user-"+strings.ToLower(api.NewID()), nil)
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
		ManagedCluster(*openapi.NewManagedCluster(squat, "aws", "")).Execute()
	Expect(err).To(HaveOccurred(), "an ordinary user created placeholder %s", squat)
	Expect(statusOf(httpResp)).To(Equal(http.StatusForbidden))

	// Operators keep both.
	_, httpResp, err = client.DefaultAPI.CreateManagedCluster(admin.rest).
		ManagedCluster(*openapi.NewManagedCluster(squat, "aws", "")).Execute()
	Expect(err).NotTo(HaveOccurred(), "platform:admin create refused: HTTP %d", statusOf(httpResp))

	// Reads stay open to gateway:creator: the console's placement picker and
	// the GitOps teardown list clusters with a creator token.
	_, httpResp, err = client.DefaultAPI.ListManagedClusters(user.rest).Execute()
	Expect(err).NotTo(HaveOccurred(), "list refused: HTTP %d", statusOf(httpResp))
}

// ---------------------------------------------------------------------------
// ManagedCluster deletion, gRPC
// ---------------------------------------------------------------------------

func TestGRPCDeleteManagedClusterIsAdminOrSelf(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	victim := registerSpoke(t, h, client)
	other := registerSpoke(t, h, client)
	user := newPrincipal(t, h, "user-"+strings.ToLower(api.NewID()), nil)
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
func TestReRegistrationAfterDeleteRestoresTheRecord(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	s := registerSpoke(t, h, client)
	owner := newPrincipal(t, h, "owner-"+strings.ToLower(api.NewID()), nil)
	gw := createGateway(t, client, owner, s.clusterID)

	httpResp, err := client.DefaultAPI.DeleteManagedCluster(s.rest, s.clusterID).Execute()
	Expect(err).NotTo(HaveOccurred(), "self-deregistration: HTTP %d", statusOf(httpResp))

	// While deleted, the spoke is not a control-plane identity for its gateways.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err = pb.NewGatewayServiceClient(s.conn).SetGatewayVersion(ctx, &pb.SetGatewayVersionRequest{Id: gw.GetId(), GatewayVersion: "0.0.1"})
	Expect(grpcCode(err)).To(Equal(codes.PermissionDenied), "a deregistered spoke wrote a gateway: %v", err)

	resp, httpResp, err := client.DefaultAPI.RegisterManagedCluster(s.rest).
		ManagedClusterRegistrationRequest(openapi.ManagedClusterRegistrationRequest{Name: s.name}).Execute()
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
		ManagedClusterRegistrationRequest(openapi.ManagedClusterRegistrationRequest{Name: s.name}).Execute()
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
		ClusterId: clusterID,
	})
	Expect(err).NotTo(HaveOccurred(), "create gateway")
	id := created.GetGateway().GetMetadata().GetId()
	return &openapi.Gateway{Id: &id, Namespace: created.GetGateway().GetNamespace()}
}

func TestGRPCControlPlaneWritesAreScopedToItsCluster(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	a := registerSpoke(t, h, client)
	b := registerSpoke(t, h, client)
	owner := newPrincipal(t, h, "owner-"+strings.ToLower(api.NewID()), nil)
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
	_, err = gwB.GetGateway(ctx, &pb.GetGatewayRequest{Id: gwA.GetId()})
	Expect(grpcCode(err)).To(Equal(codes.NotFound), "GetGateway on another cluster's gateway: %v", err)
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

func TestGRPCControlPlaneCannotRewriteFleetWideReleases(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	a := registerSpoke(t, h, client)
	admin := newPrincipal(t, h, "creator-"+strings.ToLower(api.NewID()), nil)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	release, err := pb.NewGatewayReleaseServiceClient(admin.conn).CreateGatewayRelease(ctx, &pb.CreateGatewayReleaseRequest{
		Name: "rel-" + strings.ToLower(api.NewID())[:8], Image: "quay.io/good/gateway:1.0",
	})
	Expect(err).NotTo(HaveOccurred())

	releases := pb.NewGatewayReleaseServiceClient(a.conn)
	_, err = releases.UpdateGatewayRelease(ctx, &pb.UpdateGatewayReleaseRequest{
		Id: release.GetGatewayRelease().GetMetadata().GetId(), Image: strPtr("quay.io/evil/gateway:1.0"),
	})
	Expect(grpcCode(err)).To(Equal(codes.PermissionDenied), "a control plane rewrote a release image: %v", err)
	_, err = releases.CreateGatewayRelease(ctx, &pb.CreateGatewayReleaseRequest{Name: "rel-evil", Image: "quay.io/evil/gateway:1.0"})
	Expect(grpcCode(err)).To(Equal(codes.PermissionDenied), "a control plane created a release: %v", err)

	// The status write the release reconciler performs stays allowed.
	_, err = releases.UpdateGatewayRelease(ctx, &pb.UpdateGatewayReleaseRequest{
		Id: release.GetGatewayRelease().GetMetadata().GetId(), Status: strPtr("Active"),
	})
	Expect(err).NotTo(HaveOccurred())
}

// ---------------------------------------------------------------------------
// Users on gRPC get the HTTP rules, not "creator may call anything"
// ---------------------------------------------------------------------------

func TestGRPCUserCallsFollowHTTPGatewayRules(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	a := registerSpoke(t, h, client)
	owner := newPrincipal(t, h, "owner-"+strings.ToLower(api.NewID()), nil)
	attacker := newPrincipal(t, h, "attacker-"+strings.ToLower(api.NewID()), nil)
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
