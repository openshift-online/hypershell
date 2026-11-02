package gateways_test

import (
	"testing"

	. "github.com/onsi/gomega"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/api-server/plugins/gateways"
	"github.com/openshift-online/hypershell/components/api-server/test"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/environments"
)

// TestGatewayCredentialDriverServiceImmutability proves a control-plane write
// cannot change a gateway's effective credential driver after creation, and that
// an equivalent resend is accepted without bumping generation.
// Verifies: OSG-CRED-13
func TestGatewayCredentialDriverServiceImmutability(t *testing.T) {
	test.RegisterIntegration(t)

	gatewayService := gateways.Service(&environments.Environment().Services)

	created, err := gatewayService.Create(controlPlaneWriteContext(), &gateways.Gateway{
		Name:      "cred-driver-immutable",
		ClusterId: "test-cluster_id",
	})
	Expect(err).NotTo(HaveOccurred())
	Expect(created.CredentialDriver).To(BeNil(), "no driver stored means the vault default")

	replacement := func(driver string) *gateways.Gateway {
		// Start from the stored row so only credential_driver differs.
		gw := *created
		gw.CredentialDriver = &driver
		return &gw
	}

	// Changing the default vault driver to kek-db is rejected.
	_, err = gatewayService.Replace(controlPlaneWriteContext(), replacement(`{"type":"kek-db"}`))
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("credential driver cannot be changed"))

	// Changing it to kubernetes-secrets is rejected too.
	_, err = gatewayService.Replace(controlPlaneWriteContext(), replacement(`{"type":"kubernetes-secrets"}`))
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("credential driver cannot be changed"))

	// An explicit vault resend is equivalent to the stored default: accepted, the
	// stored value is untouched and generation does not move.
	updated, err := gatewayService.Replace(controlPlaneWriteContext(), replacement(`{"type":"vault"}`))
	Expect(err).NotTo(HaveOccurred(), "an equivalent driver resend must be accepted")
	Expect(updated.CredentialDriver).To(BeNil())
	Expect(updated.Generation).To(Equal(created.Generation))
}

// TestGRPCGatewayUpdateRejectsUserCredentialDriver proves a user gRPC update that
// carries credential_driver is rejected naming the field, rather than the field
// being silently dropped.
// Verifies: OSG-CRED-13, PLAT-DATA-23
func TestGRPCGatewayUpdateRejectsUserCredentialDriver(t *testing.T) {
	h, _ := test.RegisterIntegration(t)
	h.StartControllersServer()

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)
	grpcClient := newGRPCGatewayClient(t, h)

	created, err := grpcClient.CreateGateway(ctx, &pb.CreateGatewayRequest{
		Name:      "grpc-cred-driver",
		ClusterId: registerTestCluster(t),
	})
	Expect(err).NotTo(HaveOccurred())

	kek := `{"type":"kek-db"}`
	_, err = grpcClient.UpdateGateway(ctx, &pb.UpdateGatewayRequest{
		Id:               created.Gateway.Metadata.Id,
		CredentialDriver: &kek,
	})
	Expect(err).To(HaveOccurred(), "user update with credential_driver must be rejected")
	Expect(status.Code(err)).To(Equal(codes.PermissionDenied))
	Expect(err.Error()).To(ContainSubstring("credential_driver"))
}
