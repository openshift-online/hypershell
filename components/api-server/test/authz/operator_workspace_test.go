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

// These tests enforce the body-dependent workspace_mode=operator entitlement
// (SEC-RBAC-28 / OSG-OPWS-01). The create handlers enforce it because path
// authorization middleware cannot see the body.

// operatorGatewayCreateRequest builds a pinned create request, optionally
// selecting operator workspace mode.
func operatorGatewayCreateRequest(name, clusterID string, operator bool) openapi.GatewayCreateRequest {
	request := openapi.GatewayCreateRequest{Name: name, ClusterId: openapi.PtrString(clusterID)}
	if operator {
		request.WorkspaceMode = openapi.PtrString("operator")
	}
	return request
}

// TestRESTOperatorWorkspaceRequiresEntitlement proves the REST create boundary: a
// caller holding only gateway:creator is rejected with a 403 naming
// gateway:operator-workspace-creator, while a caller granted that role or
// platform:admin passes and gets workspace_mode=operator. A create that does not
// select operator mode is unaffected.
func TestRESTOperatorWorkspaceRequiresEntitlement(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	spoke := registerSpoke(t, h, client)

	creator := newPrincipal(t, h, "user-"+strings.ToLower(api.NewID()), realmRoles(roles.RoleGatewayCreator))

	// A create without operator mode needs no entitlement (defaults to managed).
	plainName := uniqueName("plain-ws")
	plain, httpResp, err := client.DefaultAPI.CreateGateway(creator.rest).
		GatewayCreateRequest(operatorGatewayCreateRequest(plainName, spoke.clusterID, false)).Execute()
	Expect(err).NotTo(HaveOccurred(), "create without operator mode: HTTP %d %s", statusOf(httpResp), test.APIErrorReason(err))
	Expect(statusOf(httpResp)).To(Equal(http.StatusCreated))
	Expect(plain.GetWorkspaceMode()).To(Equal("managed"))

	// Selecting operator mode is rejected, naming the role.
	rejectedName := uniqueName("operator-rejected")
	_, httpResp, err = client.DefaultAPI.CreateGateway(creator.rest).
		GatewayCreateRequest(operatorGatewayCreateRequest(rejectedName, spoke.clusterID, true)).Execute()
	Expect(err).To(HaveOccurred(), "a gateway:creator without the entitlement must not select operator mode")
	Expect(statusOf(httpResp)).To(Equal(http.StatusForbidden))
	Expect(test.APIErrorReason(err)).To(ContainSubstring(roles.RoleGatewayOperatorWorkspaceCreator))
	expectNoGatewayNamedREST(t, client, creator, rejectedName)

	// A caller granted the entitlement passes.
	granted := newPrincipal(t, h, "user-"+strings.ToLower(api.NewID()), realmRoles(roles.RoleGatewayCreator, roles.RoleGatewayOperatorWorkspaceCreator))
	created, httpResp, err := client.DefaultAPI.CreateGateway(granted.rest).
		GatewayCreateRequest(operatorGatewayCreateRequest(uniqueName("operator-granted"), spoke.clusterID, true)).Execute()
	Expect(err).NotTo(HaveOccurred(), "granted caller: HTTP %d %s", statusOf(httpResp), test.APIErrorReason(err))
	Expect(statusOf(httpResp)).To(Equal(http.StatusCreated))
	Expect(created.GetWorkspaceMode()).To(Equal("operator"))

	// A creator with platform:admin also satisfies the entitlement.
	admin := newPrincipal(t, h, "admin-"+strings.ToLower(api.NewID()), realmRoles(roles.RoleGatewayCreator, roles.RolePlatformAdmin))
	adminCreated, httpResp, err := client.DefaultAPI.CreateGateway(admin.rest).
		GatewayCreateRequest(operatorGatewayCreateRequest(uniqueName("operator-admin"), spoke.clusterID, true)).Execute()
	Expect(err).NotTo(HaveOccurred(), "platform:admin caller: HTTP %d %s", statusOf(httpResp), test.APIErrorReason(err))
	Expect(statusOf(httpResp)).To(Equal(http.StatusCreated))
	Expect(adminCreated.GetWorkspaceMode()).To(Equal("operator"))
}

// TestGRPCOperatorWorkspaceRequiresEntitlement proves the gRPC create boundary
// holds the same body-dependent rule: PermissionDenied naming the role for a plain
// gateway:creator, and an accepted operator create for a granted caller.
func TestGRPCOperatorWorkspaceRequiresEntitlement(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	h.StartControllersServer()
	spoke := registerSpoke(t, h, client)
	ctx := context.Background()

	creator := newPrincipal(t, h, "user-"+strings.ToLower(api.NewID()), realmRoles(roles.RoleGatewayCreator))
	_, err := pb.NewGatewayServiceClient(creator.conn).CreateGateway(ctx, &pb.CreateGatewayRequest{
		Name:          uniqueName("grpc-operator-rejected"),
		ClusterId:     spoke.clusterID,
		WorkspaceMode: openapi.PtrString("operator"),
	})
	Expect(err).To(HaveOccurred(), "a gateway:creator without the entitlement must not select operator mode")
	Expect(grpcCode(err)).To(Equal(codes.PermissionDenied))
	Expect(err.Error()).To(ContainSubstring(roles.RoleGatewayOperatorWorkspaceCreator))

	granted := newPrincipal(t, h, "user-"+strings.ToLower(api.NewID()), realmRoles(roles.RoleGatewayCreator, roles.RoleGatewayOperatorWorkspaceCreator))
	created, err := pb.NewGatewayServiceClient(granted.conn).CreateGateway(ctx, &pb.CreateGatewayRequest{
		Name:          uniqueName("grpc-operator-granted"),
		ClusterId:     spoke.clusterID,
		WorkspaceMode: openapi.PtrString("operator"),
	})
	Expect(err).NotTo(HaveOccurred(), "granted caller over gRPC: %v", err)
	Expect(created.Gateway.GetWorkspaceMode()).To(Equal("operator"))
}
