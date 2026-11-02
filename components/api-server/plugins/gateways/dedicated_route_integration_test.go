package gateways_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	. "github.com/onsi/gomega"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"gopkg.in/resty.v1"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/hypershell/components/api-server/pkg/rbac"
	"github.com/openshift-online/hypershell/components/api-server/plugins/gateways"
	"github.com/openshift-online/hypershell/components/api-server/test"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/environments"
)

// serverAppliedDedicatedRoute is the route document the API server applies for a
// create that requests dedicated_route and authors no route: the platform-managed
// certificate opt-in the control plane keys on.
const serverAppliedDedicatedRoute = `{"enabled":true,"certificate":{}}`

// routeHasCertificate reports whether a route JSON document carries a non-null
// "certificate" sub-key, which is how the control plane selects the dedicated
// per-gateway route and wildcard certificate path.
func routeHasCertificate(t *testing.T, route string) bool {
	t.Helper()
	if route == "" {
		return false
	}
	var doc map[string]any
	Expect(json.Unmarshal([]byte(route), &doc)).To(Succeed(), "route %q is not a JSON object", route)
	cert, present := doc["certificate"]
	return present && cert != nil
}

// TestGatewayPostDedicatedRouteAppliesCertificateOptIn proves the create
// translation (design point 2): a user create carrying dedicated_route with no
// route is persisted with the platform-managed route opt-in, the flag is readable
// back, and a create without it stays unrouted.
// Verifies: PLAT-DATA-26, OSG-ROUT-12
func TestGatewayPostDedicatedRouteAppliesCertificateOptIn(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	clusterID := registerTestCluster(t)
	gatewayInput := openapi.GatewayCreateRequest{
		Name:           "dedicated-route-create",
		ClusterId:      openapi.PtrString(clusterID),
		DedicatedRoute: openapi.PtrBool(true),
	}

	created, resp, err := client.DefaultAPI.CreateGateway(ctx).GatewayCreateRequest(gatewayInput).Execute()
	Expect(err).NotTo(HaveOccurred(), "create with dedicated_route: %v", err)
	Expect(resp.StatusCode).To(Equal(http.StatusCreated))
	Expect(created.GetDedicatedRoute()).To(BeTrue(), "dedicated_route must be exposed read-only on the response")
	Expect(routeHasCertificate(t, created.GetRoute())).To(BeTrue(),
		"server must apply the certificate opt-in route %s, got %q", serverAppliedDedicatedRoute, created.GetRoute())

	// The applied route is persisted, not just presented.
	retrieved, resp, err := client.DefaultAPI.GetGateway(ctx, *created.Id).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusOK))
	Expect(retrieved.GetDedicatedRoute()).To(BeTrue())
	Expect(routeHasCertificate(t, retrieved.GetRoute())).To(BeTrue(), "persisted route = %q", retrieved.GetRoute())

	// A create that does not request it is left untranslated.
	plain, resp, err := client.DefaultAPI.CreateGateway(ctx).GatewayCreateRequest(openapi.GatewayCreateRequest{
		Name:      "dedicated-route-absent",
		ClusterId: openapi.PtrString(clusterID),
	}).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusCreated))
	Expect(plain.GetDedicatedRoute()).To(BeFalse())
	Expect(plain.GetRoute()).To(BeEmpty(), "a create without dedicated_route must stay unrouted, got %q", plain.GetRoute())
}

// TestGatewayPatchRejectsDedicatedRoute proves dedicated_route is create-only
// (design point 1): an update cannot add it, and the PATCH contract (name only)
// rejects it at the boundary naming the field.
func TestGatewayPatchRejectsDedicatedRoute(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	registerTestCluster(t)
	created, resp, err := client.DefaultAPI.CreateGateway(ctx).GatewayCreateRequest(managedGatewayCreateRequest("dedicated-route-patch")).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusCreated))

	jwtToken := ctx.Value(openapi.ContextAccessToken)
	patchResp, err := resty.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", fmt.Sprintf("Bearer %s", jwtToken)).
		SetBody(`{"dedicated_route":true}`).
		Patch(h.RestURL(fmt.Sprintf("/gateways/%s", *created.Id)))
	Expect(err).NotTo(HaveOccurred())
	Expect(patchResp.StatusCode()).To(Equal(http.StatusBadRequest))
	Expect(string(patchResp.Body())).To(ContainSubstring("dedicated_route"))

	// The stored gateway is unchanged.
	after, _, err := client.DefaultAPI.GetGateway(ctx, *created.Id).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(after.GetDedicatedRoute()).To(BeFalse())
	Expect(after.GetRoute()).To(BeEmpty())
}

// TestGRPCGatewayDedicatedRoute proves the gRPC create boundary honors the same
// contract: a user request is translated into the certificate opt-in, a
// control-plane-authored route wins over the request, and an import-marked create
// that requests the flag is rejected with an actionable message.
func TestGRPCGatewayDedicatedRoute(t *testing.T) {
	h, _ := test.RegisterIntegration(t)
	h.StartControllersServer()

	user := newGRPCGatewayClient(t, h)
	ctx := context.Background()

	created, err := user.CreateGateway(ctx, &pb.CreateGatewayRequest{
		Name:           "grpc-dedicated-route",
		ClusterId:      registerTestCluster(t),
		DedicatedRoute: openapi.PtrBool(true),
	})
	Expect(err).NotTo(HaveOccurred(), "gRPC create with dedicated_route: %v", err)
	Expect(created.Gateway.GetDedicatedRoute()).To(BeTrue())
	Expect(routeHasCertificate(t, created.Gateway.GetRoute())).To(BeTrue(), "route = %q", created.Gateway.GetRoute())

	// An explicit control-plane-authored route wins: a request flag never rewrites
	// the platform-managed route document.
	cp, clusterID := controlPlaneGatewayClient(t, h)
	explicit := `{"enabled":true}`
	cpCreated, err := cp.CreateGateway(ctx, &pb.CreateGatewayRequest{
		Name:           "grpc-dedicated-route-explicit",
		ClusterId:      clusterID,
		Route:          &explicit,
		DedicatedRoute: openapi.PtrBool(true),
	})
	Expect(err).NotTo(HaveOccurred(), "control-plane create with an explicit route: %v", err)
	Expect(cpCreated.Gateway.GetDedicatedRoute()).To(BeTrue())
	Expect(cpCreated.Gateway.GetRoute()).To(Equal(explicit), "the explicit route must be kept verbatim")

	// A GitOps import may not request it (design point 4). The import caller kind
	// is identified by the reconciled-Repository metadata.
	importCtx := metadata.AppendToOutgoingContext(context.Background(), rbac.ImportRepositoryMetadataKey, "repo-dedicated-route")
	_, err = user.CreateGateway(importCtx, &pb.CreateGatewayRequest{
		Name:           "grpc-dedicated-route-import",
		ClusterId:      registerTestCluster(t),
		DedicatedRoute: openapi.PtrBool(true),
	})
	Expect(err).To(HaveOccurred(), "import create with dedicated_route must be rejected")
	Expect(status.Code(err)).To(Equal(codes.PermissionDenied), "import create: %v", err)
	Expect(err.Error()).To(ContainSubstring("cannot be requested by an import caller"))
}

// TestGatewayDedicatedRouteServiceSemantics proves the service funnel contract for
// every writer class: the request is translated into the certificate opt-in at
// create, an import create is rejected, the flag is immutable afterwards with
// delete-and-recreate guidance, and resending the stored value is a no-op.
func TestGatewayDedicatedRouteServiceSemantics(t *testing.T) {
	test.RegisterIntegration(t)

	gatewayService := gateways.Service(&environments.Environment().Services)

	// A plain user create applies the certificate opt-in.
	created, svcErr := gatewayService.Create(context.Background(), &gateways.Gateway{
		Name:           "dr-semantics",
		ClusterId:      "test-cluster_id",
		DedicatedRoute: openapi.PtrBool(true),
	})
	Expect(svcErr).NotTo(HaveOccurred())
	Expect(created.DedicatedRoute).NotTo(BeNil())
	Expect(*created.DedicatedRoute).To(BeTrue())
	Expect(created.Route).NotTo(BeNil())
	Expect(routeHasCertificate(t, *created.Route)).To(BeTrue(), "route = %q", *created.Route)

	// An import create may not request it.
	importCtx := rbac.WithGatewayWriterKind(context.Background(), rbac.GatewayWriterImport)
	_, svcErr = gatewayService.Create(importCtx, &gateways.Gateway{
		Name:           "dr-semantics-import",
		ClusterId:      "test-cluster_id",
		DedicatedRoute: openapi.PtrBool(true),
	})
	Expect(svcErr).NotTo(BeNil(), "an import create with dedicated_route must be rejected")
	Expect(svcErr.IsForbidden()).To(BeTrue())
	Expect(svcErr.Reason).To(ContainSubstring("cannot be requested by an import caller"))

	// An explicit route wins for a privileged create.
	explicit := `{"enabled":true}`
	explicitCreated, svcErr := gatewayService.Create(controlPlaneWriteContext(), &gateways.Gateway{
		Name:           "dr-semantics-explicit",
		ClusterId:      "test-cluster_id",
		Route:          &explicit,
		DedicatedRoute: openapi.PtrBool(true),
	})
	Expect(svcErr).NotTo(HaveOccurred())
	Expect(explicitCreated.Route).NotTo(BeNil())
	Expect(*explicitCreated.Route).To(Equal(explicit))

	// An update can neither add the request to a gateway created without it ...
	plain, svcErr := gatewayService.Create(context.Background(), &gateways.Gateway{
		Name:      "dr-semantics-plain",
		ClusterId: "test-cluster_id",
	})
	Expect(svcErr).NotTo(HaveOccurred())
	storedPlain, svcErr := gatewayService.Get(context.Background(), plain.ID)
	Expect(svcErr).NotTo(HaveOccurred())
	added := *storedPlain
	added.DedicatedRoute = openapi.PtrBool(true)
	_, svcErr = gatewayService.Replace(controlPlaneWriteContext(), &added)
	Expect(svcErr).NotTo(BeNil(), "adding dedicated_route after creation must be rejected")
	Expect(svcErr.IsConflict()).To(BeTrue())
	Expect(svcErr.Reason).To(ContainSubstring("create-only"))
	Expect(svcErr.Reason).To(ContainSubstring("delete and re-create"))

	// ... nor remove it afterwards, even for the privileged control-plane writer.
	stored, svcErr := gatewayService.Get(context.Background(), created.ID)
	Expect(svcErr).NotTo(HaveOccurred())
	removal := *stored
	removal.DedicatedRoute = openapi.PtrBool(false)
	_, svcErr = gatewayService.Replace(controlPlaneWriteContext(), &removal)
	Expect(svcErr).NotTo(BeNil(), "removing dedicated_route after creation must be rejected")
	Expect(svcErr.IsConflict()).To(BeTrue())
	Expect(svcErr.Reason).To(ContainSubstring("delete and re-create"))

	// Re-sending the stored value is accepted (idempotent): every other field is
	// resent unchanged, including the route the server applied.
	resend := *stored
	updated, svcErr := gatewayService.Replace(controlPlaneWriteContext(), &resend)
	Expect(svcErr).NotTo(HaveOccurred(), "re-sending the stored dedicated_route must be accepted")
	Expect(updated.DedicatedRoute).NotTo(BeNil())
	Expect(*updated.DedicatedRoute).To(BeTrue())
	Expect(updated.Route).NotTo(BeNil())
	Expect(routeHasCertificate(t, *updated.Route)).To(BeTrue())
}
