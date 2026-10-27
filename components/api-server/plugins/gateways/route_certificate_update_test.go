package gateways_test

import (
	"testing"

	. "github.com/onsi/gomega"

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

// TestGatewayRouteCertificateIsProvisionTimeOnly proves the API server enforces
// the provision-time-only contract on route.certificate (docs/investigations/
// per-gateway-certificates.md §7.7): the opt-in can be chosen when the gateway
// is created, but any later add, removal, or alteration is rejected - even for
// the privileged control-plane writer that authors the route field - while
// re-sending the certificate unchanged (other route keys differing) stays
// accepted.
func TestGatewayRouteCertificateIsProvisionTimeOnly(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	h.StartControllersServer()

	cp, clusterID := controlPlaneGatewayClient(t, h)
	ctx := h.NewAuthenticatedContext(h.NewRandAccount())

	// Provision with the opt-in, the way the platform authors a route.
	created, err := cp.CreateGateway(ctx, &pb.CreateGatewayRequest{
		Name:      "route-cert-optin",
		ClusterId: clusterID,
		Route:     func() *string { s := `{"certificate":{}}`; return &s }(),
	})
	Expect(err).NotTo(HaveOccurred(), "control-plane create with route.certificate must be accepted: %v", err)
	gatewayID := created.Gateway.Metadata.Id

	// Update re-sending the certificate unchanged (only unrelated keys differ)
	// must succeed: the opt-in itself did not change.
	keptRoute := `{"certificate":{},"enabled":true}`
	_, err = cp.UpdateGateway(ctx, &pb.UpdateGatewayRequest{
		Id:    gatewayID,
		Route: func() *string { s := keptRoute; return &s }(),
	})
	Expect(err).NotTo(HaveOccurred(), "update keeping route.certificate must be accepted: %v", err)

	// Removing the opt-in must be rejected with a reason that explains the
	// provision-time-only contract.
	_, err = cp.UpdateGateway(ctx, &pb.UpdateGatewayRequest{
		Id:    gatewayID,
		Route: func() *string { s := `{"enabled":true}`; return &s }(),
	})
	Expect(err).To(HaveOccurred(), "removing route.certificate must be rejected")
	Expect(err.Error()).To(ContainSubstring("route.certificate is provision-time only"))
	Expect(err.Error()).To(ContainSubstring("delete and re-create"))

	// Altering the opt-in value must be rejected the same way.
	_, err = cp.UpdateGateway(ctx, &pb.UpdateGatewayRequest{
		Id:    gatewayID,
		Route: func() *string { s := `{"enabled":true,"certificate":{"issuer":"custom"}}`; return &s }(),
	})
	Expect(err).To(HaveOccurred(), "altering route.certificate must be rejected")

	// The rejected updates must not have persisted: the stored route still
	// carries the opt-in.
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
		Route: func() *string { s := `{"certificate":{}}`; return &s }(),
	})
	Expect(err).To(HaveOccurred(), "adding route.certificate after creation must be rejected")
	Expect(err.Error()).To(ContainSubstring("route.certificate is provision-time only"))

	// Route authoring that does not touch the certificate key stays valid for
	// the privileged writer.
	_, err = cp.UpdateGateway(ctx, &pb.UpdateGatewayRequest{
		Id:    gatewayID,
		Route: func() *string { s := `{"host":"moved.example.com"}`; return &s }(),
	})
	Expect(err).NotTo(HaveOccurred(), "authoring route keys other than certificate must stay accepted: %v", err)
}
