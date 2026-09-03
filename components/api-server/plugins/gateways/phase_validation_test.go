package gateways_test

import (
	"net/http"
	"testing"

	. "github.com/onsi/gomega"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/hypershell/components/api-server/test"
)

// TestGRPCGatewayRejectsUnknownPhase proves the API server enforces the
// canonical phase vocabulary (see specs/platform/gateway-phase-vocabulary.spec.md):
// a create or update with a phase outside the canonical set is rejected with
// InvalidArgument and never persisted, while a canonical phase is accepted.
func TestGRPCGatewayRejectsUnknownPhase(t *testing.T) {
	h, _ := test.RegisterIntegration(t)
	h.StartControllersServer()

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)
	jwtToken := h.CreateJWTString(account)

	conn, err := grpc.NewClient(
		h.GRPCAddress(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithPerRPCCredentials(&bearerToken{token: jwtToken}),
	)
	Expect(err).NotTo(HaveOccurred())
	t.Cleanup(func() {
		Expect(conn.Close()).To(Succeed())
	})

	grpcClient := pb.NewGatewayServiceClient(conn)

	badPhase := "Booting"
	_, err = grpcClient.CreateGateway(ctx, &pb.CreateGatewayRequest{
		Name:       "reject-create",
		ClusterId:  "test-cluster_id",
		ReleaseId:  "test-release_id",
		DatabaseId: "test-database_id",
		Phase:      &badPhase,
	})
	Expect(err).To(HaveOccurred(), "create with an unknown phase must be rejected")
	Expect(status.Code(err)).To(Equal(codes.InvalidArgument))

	// A canonical phase on create is accepted.
	goodPhase := "Provisioning"
	created, err := grpcClient.CreateGateway(ctx, &pb.CreateGatewayRequest{
		Name:       "accept-create",
		ClusterId:  "test-cluster_id",
		ReleaseId:  "test-release_id",
		DatabaseId: "test-database_id",
		Phase:      &goodPhase,
	})
	Expect(err).NotTo(HaveOccurred())

	// An unknown phase on update is rejected...
	_, err = grpcClient.UpdateGateway(ctx, &pb.UpdateGatewayRequest{
		Id:    created.Gateway.Metadata.Id,
		Phase: &badPhase,
	})
	Expect(err).To(HaveOccurred(), "update with an unknown phase must be rejected")
	Expect(status.Code(err)).To(Equal(codes.InvalidArgument))

	// ...and the rejected value was not persisted.
	got, err := grpcClient.GetGateway(ctx, &pb.GetGatewayRequest{Id: created.Gateway.Metadata.Id})
	Expect(err).NotTo(HaveOccurred())
	Expect(got.Gateway.GetPhase()).To(Equal("Provisioning"))

	// A canonical phase on update is accepted.
	runningPhase := "Running"
	_, err = grpcClient.UpdateGateway(ctx, &pb.UpdateGatewayRequest{
		Id:    created.Gateway.Metadata.Id,
		Phase: &runningPhase,
	})
	Expect(err).NotTo(HaveOccurred())

	// An absent phase is accepted so the field stays optional.
	_, err = grpcClient.CreateGateway(ctx, &pb.CreateGatewayRequest{
		Name:       "accept-absent-phase",
		ClusterId:  "test-cluster_id",
		ReleaseId:  "test-release_id",
		DatabaseId: "test-database_id",
	})
	Expect(err).NotTo(HaveOccurred(), "create without a phase must be accepted")
}

// TestGatewayPatchNotTouchingPhaseAcceptsLegacyRecord proves the vocabulary
// tightening does not lock out pre-existing rows: a gateway whose stored phase
// predates validation (seeded through the service, which is not a write path)
// can still be patched on unrelated fields, because validation only fires when a
// write actually sets phase.
func TestGatewayPatchNotTouchingPhaseAcceptsLegacyRecord(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	// Seed a record with a non-canonical stored phase via the service layer,
	// which intentionally has no phase validation (only the REST/gRPC handlers do).
	legacy, err := newGateway("legacy-phase")
	Expect(err).NotTo(HaveOccurred())
	Expect(legacy.Phase).NotTo(BeNil())
	Expect(*legacy.Phase).To(Equal("test-phase"), "factory seeds a non-canonical phase")

	// A PATCH that does not touch phase must succeed despite the stored value.
	_, resp, err := client.DefaultAPI.UpdateGateway(ctx, legacy.ID).GatewayPatchRequest(openapi.GatewayPatchRequest{
		TlsMode: openapi.PtrString("updated-tls-mode"),
	}).Execute()
	Expect(err).NotTo(HaveOccurred(), "patching a non-phase field on a legacy record must be accepted")
	Expect(resp.StatusCode).To(Equal(http.StatusOK))
}

// TestRESTGatewayRejectsUnknownPhase proves the REST create path enforces the
// same canonical phase vocabulary with HTTP 400.
func TestRESTGatewayRejectsUnknownPhase(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	_, resp, err := client.DefaultAPI.CreateGateway(ctx).GatewayCreateRequest(openapi.GatewayCreateRequest{
		Name:       "reject-rest-create",
		ClusterId:  "test-cluster_id",
		ReleaseId:  "test-release_id",
		DatabaseId: "test-database_id",
		Phase:      openapi.PtrString("Booting"),
	}).Execute()
	Expect(err).To(HaveOccurred(), "REST create with an unknown phase must be rejected")
	Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
}
