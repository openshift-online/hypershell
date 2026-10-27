package gateways_test

import (
	"testing"

	. "github.com/onsi/gomega"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/api-server/test"
)

// controlPlaneGatewayClient dials the gRPC API as a REGISTERED control plane:
// after the default-deny gateway field policy (PLAT-DATA-23), the control
// plane is the only writer class that may author the platform-managed route
// field, so it is the path through which the route.certificate opt-in is
// provisioned and the path these tests exercise.
func controlPlaneGatewayClient(t *testing.T, h *test.Helper) (pb.GatewayServiceClient, string) {
	t.Helper()
	clusterID, subject := registerTestClusterWithSubject(t)
	cpAccount := h.NewAccount("service-account-cp-"+subject, "cp", "")
	cpToken := h.CreateJWTStringWithClaims(cpAccount, test.ControlPlaneClaims(subject))
	return pb.NewGatewayServiceClient(dialGRPC(t, h, cpToken)), clusterID
}

// routePtr is a test shorthand for the optional route field.
func routePtr(s string) *string { return &s }

// TestGatewayRouteCertificateIsProvisionTimeOnly proves the API server enforces
// the provision-time-only contract on route.certificate (docs/investigations/
// per-gateway-certificates.md §7.7): the opt-in can be chosen when the gateway
// is created, but any later add, removal, or alteration is rejected - even for
// the privileged control-plane writer that authors the route field - while
// re-sending the certificate unchanged (other route keys differing) stays
// accepted. The gRPC code is FailedPrecondition, not the framework's blanket
// 409-to-AlreadyExists mapping: the error means the stored gateway's state
// forbids the change, not a duplicate creation (API UX review finding 4).
func TestGatewayRouteCertificateIsProvisionTimeOnly(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	h.StartControllersServer()

	cp, clusterID := controlPlaneGatewayClient(t, h)
	ctx := h.NewAuthenticatedContext(h.NewRandAccount())

	// Provision with the opt-in, the way the platform authors a route.
	created, err := cp.CreateGateway(ctx, &pb.CreateGatewayRequest{
		Name:      "route-cert-optin",
		ClusterId: clusterID,
		Route:     routePtr(`{"certificate":{}}`),
	})
	Expect(err).NotTo(HaveOccurred(), "control-plane create with route.certificate must be accepted: %v", err)
	gatewayID := created.Gateway.Metadata.Id

	// Update re-sending the certificate unchanged (only unrelated keys differ)
	// must succeed: the opt-in itself did not change.
	keptRoute := `{"certificate":{},"enabled":true}`
	_, err = cp.UpdateGateway(ctx, &pb.UpdateGatewayRequest{
		Id:    gatewayID,
		Route: routePtr(keptRoute),
	})
	Expect(err).NotTo(HaveOccurred(), "update keeping route.certificate must be accepted: %v", err)

	// Removing the opt-in must be rejected with FailedPrecondition and a reason
	// that explains the provision-time-only contract and the PATCH round-trip.
	_, err = cp.UpdateGateway(ctx, &pb.UpdateGatewayRequest{
		Id:    gatewayID,
		Route: routePtr(`{"enabled":true}`),
	})
	Expect(err).To(HaveOccurred(), "removing route.certificate must be rejected")
	Expect(status.Convert(err).Code()).To(Equal(codes.FailedPrecondition))
	Expect(err.Error()).To(ContainSubstring("route.certificate is provision-time only"))
	Expect(err.Error()).To(ContainSubstring("delete and re-create"))
	Expect(err.Error()).To(ContainSubstring("keep certificate: {} unchanged"))

	// Altering the opt-in value must be rejected the same way (the immutability
	// check reports before the shape validation).
	_, err = cp.UpdateGateway(ctx, &pb.UpdateGatewayRequest{
		Id:    gatewayID,
		Route: routePtr(`{"enabled":true,"certificate":{"issuer":"custom"}}`),
	})
	Expect(err).To(HaveOccurred(), "altering route.certificate must be rejected")
	Expect(status.Convert(err).Code()).To(Equal(codes.FailedPrecondition))

	// Adding a host to an opted-in gateway is rejected at the boundary with the
	// control plane's validation message: the combination would fail
	// provisioning asynchronously otherwise.
	_, err = cp.UpdateGateway(ctx, &pb.UpdateGatewayRequest{
		Id:    gatewayID,
		Route: routePtr(`{"certificate":{},"host":"renamed.example.com"}`),
	})
	Expect(err).To(HaveOccurred(), "adding route.host to an opted-in gateway must be rejected")
	Expect(status.Convert(err).Code()).To(Equal(codes.InvalidArgument))
	Expect(err.Error()).To(ContainSubstring("route.host is not supported with route.certificate"))

	// The rejected updates must not have persisted: the stored route still
	// carries the original opt-in.
	retrieved, resp, err := client.DefaultAPI.GetGateway(ctx, gatewayID).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(200))
	Expect(retrieved.GetRoute()).To(MatchJSON(keptRoute))
}

// TestGatewayRouteCertificateCannotBeAddedAfterCreation proves the reverse
// direction: a gateway created without the opt-in cannot gain it later, while
// authoring route content that does not touch the certificate key keeps
// working for the control-plane writer.
func TestGatewayRouteCertificateCannotBeAddedAfterCreation(t *testing.T) {
	h, _ := test.RegisterIntegration(t)
	h.StartControllersServer()

	cp, clusterID := controlPlaneGatewayClient(t, h)
	ctx := h.NewAuthenticatedContext(h.NewRandAccount())

	created, err := cp.CreateGateway(ctx, &pb.CreateGatewayRequest{
		Name:      "route-cert-absent",
		ClusterId: clusterID,
	})
	Expect(err).NotTo(HaveOccurred(), "control-plane create without a route must be accepted: %v", err)
	gatewayID := created.Gateway.Metadata.Id

	_, err = cp.UpdateGateway(ctx, &pb.UpdateGatewayRequest{
		Id:    gatewayID,
		Route: routePtr(`{"certificate":{}}`),
	})
	Expect(err).To(HaveOccurred(), "adding route.certificate after creation must be rejected")
	Expect(status.Convert(err).Code()).To(Equal(codes.FailedPrecondition))
	Expect(err.Error()).To(ContainSubstring("route.certificate is provision-time only"))

	// Route authoring that does not touch the certificate key stays valid for
	// the privileged writer.
	_, err = cp.UpdateGateway(ctx, &pb.UpdateGatewayRequest{
		Id:    gatewayID,
		Route: routePtr(`{"host":"moved.example.com"}`),
	})
	Expect(err).NotTo(HaveOccurred(), "authoring route keys other than certificate must stay accepted: %v", err)
}

// TestGatewayRouteCertificateShapeRejectedAtCreation proves the shape
// validation runs at the API boundary (immediate 400 / InvalidArgument), not
// only during provisioning: host + certificate, and a non-empty certificate
// object, are both rejected at create time with actionable messages, even for
// the privileged control-plane author.
func TestGatewayRouteCertificateShapeRejectedAtCreation(t *testing.T) {
	h, _ := test.RegisterIntegration(t)
	h.StartControllersServer()

	cp, clusterID := controlPlaneGatewayClient(t, h)
	ctx := h.NewAuthenticatedContext(h.NewRandAccount())

	// host + certificate: the control plane's ValidateGatewayConfig rejects
	// this combination at provisioning; rejecting it here turns an
	// asynchronously failing gateway into an immediate, explained error.
	_, err := cp.CreateGateway(ctx, &pb.CreateGatewayRequest{
		Name:      "route-cert-bad-combo",
		ClusterId: clusterID,
		Route:     routePtr(`{"host":"gw.example.com","certificate":{}}`),
	})
	Expect(err).To(HaveOccurred(), "create with host + route.certificate must be rejected")
	Expect(status.Convert(err).Code()).To(Equal(codes.InvalidArgument))
	Expect(err.Error()).To(ContainSubstring("route.host is not supported with route.certificate"))

	// A non-empty certificate object: its fields would be silently discarded
	// (the issuer is cluster infrastructure), so the API rejects the natural
	// cert-manager idiom instead of swallowing it.
	_, err = cp.CreateGateway(ctx, &pb.CreateGatewayRequest{
		Name:      "route-cert-bad-shape",
		ClusterId: clusterID,
		Route:     routePtr(`{"certificate":{"issuer":"my-issuer"}}`),
	})
	Expect(err).To(HaveOccurred(), "create with a non-empty route.certificate must be rejected")
	Expect(status.Convert(err).Code()).To(Equal(codes.InvalidArgument))
	Expect(err.Error()).To(ContainSubstring("route.certificate accepts no fields"))
}
