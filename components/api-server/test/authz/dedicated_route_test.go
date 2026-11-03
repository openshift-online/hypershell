package authz_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	"google.golang.org/grpc/codes"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/hypershell/components/api-server/plugins/roles"
	"github.com/openshift-online/hypershell/components/api-server/test"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
)

// These tests enforce the body-dependent dedicated_route entitlement. Each
// caller receives explicit JWT realm roles. The create handlers enforce this
// entitlement because path authorization middleware cannot see the body.

// uniqueName returns a name safe to reuse across principals and runs.
func uniqueName(prefix string) string {
	return prefix + "-" + strings.ToLower(api.NewID())
}

// gatewayCreateRequest builds a pinned create request against the spoke's cluster.
func gatewayCreateRequest(name, clusterID string, dedicatedRoute bool) openapi.GatewayCreateRequest {
	request := openapi.GatewayCreateRequest{Name: name, ClusterId: openapi.PtrString(clusterID)}
	if dedicatedRoute {
		request.DedicatedRoute = openapi.PtrBool(true)
	}
	return request
}

// TestRESTDedicatedRouteRequiresEntitlement proves the REST create boundary: a
// caller holding only the gateway:creator role is rejected with a 403 that
// names gateway:dedicated-route-creator and the rate-limit rationale, while a
// caller granted that role and a platform:admin pass and get the dedicated route
// opt-in applied. A create that does not request the flag is unaffected, so the
// entitlement is not required to create gateways at all.
func TestRESTDedicatedRouteRequiresEntitlement(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	spoke := registerSpoke(t, h, client)

	creator := newPrincipal(t, h, "user-"+strings.ToLower(api.NewID()), realmRoles(roles.RoleGatewayCreator))

	// A create without the flag needs no entitlement.
	plainName := uniqueName("plain-route")
	_, httpResp, err := client.DefaultAPI.CreateGateway(creator.rest).
		GatewayCreateRequest(gatewayCreateRequest(plainName, spoke.clusterID, false)).Execute()
	Expect(err).NotTo(HaveOccurred(), "create without dedicated_route: HTTP %d %s", statusOf(httpResp), test.APIErrorReason(err))
	Expect(statusOf(httpResp)).To(Equal(http.StatusCreated))

	// Requesting it is rejected, naming the role and saying why it is separate.
	rejectedName := uniqueName("dedicated-rejected")
	_, httpResp, err = client.DefaultAPI.CreateGateway(creator.rest).
		GatewayCreateRequest(gatewayCreateRequest(rejectedName, spoke.clusterID, true)).Execute()
	Expect(err).To(HaveOccurred(), "a gateway:creator without the entitlement must not request dedicated_route")
	Expect(statusOf(httpResp)).To(Equal(http.StatusForbidden))
	reason := test.APIErrorReason(err)
	Expect(reason).To(ContainSubstring(roles.RoleGatewayDedicatedRouteCreator))
	Expect(reason).To(ContainSubstring("rate-limited"))
	expectNoGatewayNamedREST(t, client, creator, rejectedName)

	// A caller granted the entitlement passes.
	granted := newPrincipal(t, h, "user-"+strings.ToLower(api.NewID()), realmRoles(roles.RoleGatewayCreator, roles.RoleGatewayDedicatedRouteCreator))
	created, httpResp, err := client.DefaultAPI.CreateGateway(granted.rest).
		GatewayCreateRequest(gatewayCreateRequest(uniqueName("dedicated-granted"), spoke.clusterID, true)).Execute()
	Expect(err).NotTo(HaveOccurred(), "granted caller: HTTP %d %s", statusOf(httpResp), test.APIErrorReason(err))
	Expect(statusOf(httpResp)).To(Equal(http.StatusCreated))
	Expect(created.GetDedicatedRoute()).To(BeTrue())
	Expect(created.GetRoute()).To(ContainSubstring("certificate"))

	// A creator with platform:admin also satisfies the dedicated-route entitlement.
	admin := newPrincipal(t, h, "admin-"+strings.ToLower(api.NewID()), realmRoles(roles.RoleGatewayCreator, roles.RolePlatformAdmin))
	adminCreated, httpResp, err := client.DefaultAPI.CreateGateway(admin.rest).
		GatewayCreateRequest(gatewayCreateRequest(uniqueName("dedicated-admin"), spoke.clusterID, true)).Execute()
	Expect(err).NotTo(HaveOccurred(), "platform:admin caller: HTTP %d %s", statusOf(httpResp), test.APIErrorReason(err))
	Expect(statusOf(httpResp)).To(Equal(http.StatusCreated))
	Expect(adminCreated.GetDedicatedRoute()).To(BeTrue())
	Expect(adminCreated.GetRoute()).To(ContainSubstring("certificate"))
}

// TestGRPCDedicatedRouteRequiresEntitlement proves the gRPC create boundary holds
// the same body-dependent rule: PermissionDenied naming the role for a plain
// gateway:creator, and an accepted create for a caller granted the entitlement.
func TestGRPCDedicatedRouteRequiresEntitlement(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	h.StartControllersServer()
	spoke := registerSpoke(t, h, client)
	ctx := context.Background()

	creator := newPrincipal(t, h, "user-"+strings.ToLower(api.NewID()), realmRoles(roles.RoleGatewayCreator))
	_, err := pb.NewGatewayServiceClient(creator.conn).CreateGateway(ctx, &pb.CreateGatewayRequest{
		Name:           uniqueName("grpc-dedicated-rejected"),
		ClusterId:      spoke.clusterID,
		DedicatedRoute: openapi.PtrBool(true),
	})
	Expect(err).To(HaveOccurred(), "a gateway:creator without the entitlement must not request dedicated_route")
	Expect(grpcCode(err)).To(Equal(codes.PermissionDenied))
	Expect(err.Error()).To(ContainSubstring(roles.RoleGatewayDedicatedRouteCreator))
	Expect(err.Error()).To(ContainSubstring("rate-limited"))

	granted := newPrincipal(t, h, "user-"+strings.ToLower(api.NewID()), realmRoles(roles.RoleGatewayCreator, roles.RoleGatewayDedicatedRouteCreator))
	created, err := pb.NewGatewayServiceClient(granted.conn).CreateGateway(ctx, &pb.CreateGatewayRequest{
		Name:           uniqueName("grpc-dedicated-granted"),
		ClusterId:      spoke.clusterID,
		DedicatedRoute: openapi.PtrBool(true),
	})
	Expect(err).NotTo(HaveOccurred(), "granted caller over gRPC: %v", err)
	Expect(created.Gateway.GetDedicatedRoute()).To(BeTrue())
	Expect(created.Gateway.GetRoute()).To(ContainSubstring("certificate"))
}

// expectNoGatewayNamedREST asserts the rejected create persisted nothing. The
// search runs as the same principal so the list is not filtered away by RBAC.
func expectNoGatewayNamedREST(t *testing.T, client *openapi.APIClient, p principal, name string) {
	t.Helper()
	list, httpResp, err := client.DefaultAPI.ListGateways(p.rest).
		Search("name = '" + name + "'").Execute()
	Expect(err).NotTo(HaveOccurred(), "list gateways: HTTP %d %s", statusOf(httpResp), test.APIErrorReason(err))
	Expect(list.Items).To(BeEmpty(), "no gateway named %s may be persisted", name)
}
