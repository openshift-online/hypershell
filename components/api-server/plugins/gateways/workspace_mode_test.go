package gateways_test

import (
	"fmt"
	"net/http"
	"testing"

	. "github.com/onsi/gomega"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gopkg.in/resty.v1"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/hypershell/components/api-server/plugins/gateways"
	"github.com/openshift-online/hypershell/components/api-server/test"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/environments"
)

// TestGatewayPostDefaultsWorkspaceModeToManaged proves the create path applies
// the managed default (openshell-gateway.spec.md, "OSG-CORE-18 -- Gateway
// Workspace Mode"): a gateway created through the user allowlist (name +
// placement) is persisted with workspace_mode "managed" and the value is
// readable back through REST, even though no caller can supply it.
func TestGatewayPostDefaultsWorkspaceModeToManaged(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	registerTestCluster(t)
	gatewayInput := managedGatewayCreateRequest("default-managed")

	gatewayOutput, resp, err := client.DefaultAPI.CreateGateway(ctx).GatewayCreateRequest(gatewayInput).Execute()
	Expect(err).NotTo(HaveOccurred(), "Error posting gateway without workspace_mode: %v", err)
	Expect(resp.StatusCode).To(Equal(http.StatusCreated))
	Expect(gatewayOutput.WorkspaceMode).NotTo(BeNil())
	Expect(*gatewayOutput.WorkspaceMode).To(Equal("managed"))

	retrieved, resp, err := client.DefaultAPI.GetGateway(ctx, *gatewayOutput.Id).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusOK))
	Expect(retrieved.WorkspaceMode).NotTo(BeNil())
	Expect(*retrieved.WorkspaceMode).To(Equal("managed"))
}

// TestGatewayPostRejectsWorkspaceMode proves workspace_mode is platform-managed
// (PLAT-DATA-23): the strict create contract rejects a body that declares it,
// naming the field, and nothing is persisted.
func TestGatewayPostRejectsWorkspaceMode(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)
	registerTestCluster(t)
	jwToken := ctx.Value(openapi.ContextAccessToken)
	resp, err := resty.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", fmt.Sprintf("Bearer %s", jwToken)).
		SetBody(`{"name":"ws-mode-create","placement":{"selector":["network/public","provider/aws"]},"workspace_mode":"shared"}`).
		Post(h.RestURL("/gateways"))
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode()).To(Equal(http.StatusBadRequest))
	Expect(string(resp.Body())).To(ContainSubstring("workspace_mode"))
	expectNoGatewayNamed(ctx, client, "ws-mode-create")
}

// TestGatewayPatchRejectsWorkspaceMode proves the PATCH contract (name only)
// rejects a workspace_mode write with a 400 naming the field.
func TestGatewayPatchRejectsWorkspaceMode(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	registerTestCluster(t)
	created, resp, err := client.DefaultAPI.CreateGateway(ctx).GatewayCreateRequest(managedGatewayCreateRequest("ws-mode-patch")).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusCreated))

	jwToken := ctx.Value(openapi.ContextAccessToken)
	patchResp, err := resty.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", fmt.Sprintf("Bearer %s", jwToken)).
		SetBody(`{"workspace_mode":"shared"}`).
		Patch(h.RestURL(fmt.Sprintf("/gateways/%s", *created.Id)))
	Expect(err).NotTo(HaveOccurred())
	Expect(patchResp.StatusCode()).To(Equal(http.StatusBadRequest))
	Expect(string(patchResp.Body())).To(ContainSubstring("workspace_mode"))
}

// TestGRPCGatewayWorkspaceMode proves the gRPC boundary holds the same policy:
// a user create or update carrying workspace_mode is rejected naming the field
// (PermissionDenied), and a create without it gets the managed default.
// Verifies: SEC-RBAC-18, PLAT-DATA-23, OSG-CORE-18
func TestGRPCGatewayWorkspaceMode(t *testing.T) {
	h, _ := test.RegisterIntegration(t)
	h.StartControllersServer()

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)
	grpcClient := newGRPCGatewayClient(t, h)

	// A user create with workspace_mode is rejected naming the field.
	shared := "shared"
	_, err := grpcClient.CreateGateway(ctx, &pb.CreateGatewayRequest{
		Name:          "grpc-ws-reject",
		ClusterId:     registerTestCluster(t),
		WorkspaceMode: &shared,
	})
	Expect(err).To(HaveOccurred(), "user create with workspace_mode must be rejected")
	Expect(status.Code(err)).To(Equal(codes.PermissionDenied))
	Expect(err.Error()).To(ContainSubstring("workspace_mode"))

	// A user create without it gets the managed default.
	created, err := grpcClient.CreateGateway(ctx, &pb.CreateGatewayRequest{
		Name:      "grpc-ws-default",
		ClusterId: registerTestCluster(t),
	})
	Expect(err).NotTo(HaveOccurred())
	Expect(created.Gateway.GetWorkspaceMode()).To(Equal("managed"))

	// A user update carrying workspace_mode is rejected naming the field.
	_, err = grpcClient.UpdateGateway(ctx, &pb.UpdateGatewayRequest{
		Id:            created.Gateway.Metadata.Id,
		WorkspaceMode: &shared,
	})
	Expect(err).To(HaveOccurred(), "user update with workspace_mode must be rejected")
	Expect(status.Code(err)).To(Equal(codes.PermissionDenied))
	Expect(err.Error()).To(ContainSubstring("workspace_mode"))
}

// TestGatewayWorkspaceModeServiceSemantics proves the value contract the
// service funnel enforces for every writer class (the control plane included,
// since only a control-plane internal write can carry the field at all): the
// managed default is applied on create, the vocabulary is enforced, a stored
// value is immutable, and re-sending the stored value is a no-op.
// Verifies: OSG-CORE-18
func TestGatewayWorkspaceModeServiceSemantics(t *testing.T) {
	test.RegisterIntegration(t)

	gatewayService := gateways.Service(&environments.Environment().Services)

	// Create applies the managed default.
	created, err := gatewayService.Create(controlPlaneWriteContext(), &gateways.Gateway{
		Name:      "ws-semantics",
		ClusterId: "test-cluster_id",
	})
	Expect(err).NotTo(HaveOccurred())
	Expect(created.WorkspaceMode).NotTo(BeNil())
	Expect(*created.WorkspaceMode).To(Equal("managed"))

	// An invalid vocabulary value is rejected even for a control-plane write.
	bad := "combined"
	_, err = gatewayService.Create(controlPlaneWriteContext(), &gateways.Gateway{
		Name: "ws-semantics-bad", ClusterId: "test-cluster_id", WorkspaceMode: &bad,
	})
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("workspace_mode"))

	// A control-plane update cannot change the stored value.
	changed := "shared"
	replacement := &gateways.Gateway{Name: created.Name, ClusterId: created.ClusterId, WorkspaceMode: &changed}
	replacement.ID = created.ID
	replacement.Namespace = created.Namespace
	_, err = gatewayService.Replace(controlPlaneWriteContext(), replacement)
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("workspace mode cannot be changed"))

	// Re-sending the stored value is accepted (idempotent).
	same := "managed"
	resend := &gateways.Gateway{Name: created.Name, ClusterId: created.ClusterId, WorkspaceMode: &same}
	resend.ID = created.ID
	resend.Namespace = created.Namespace
	updated, err := gatewayService.Replace(controlPlaneWriteContext(), resend)
	Expect(err).NotTo(HaveOccurred(), "re-sending the stored workspace_mode must be accepted")
	Expect(updated.WorkspaceMode).NotTo(BeNil())
	Expect(*updated.WorkspaceMode).To(Equal("managed"))
}
