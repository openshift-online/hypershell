package gateways_test

import (
	"context"
	"net/http"
	"testing"

	. "github.com/onsi/gomega"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/hypershell/components/api-server/plugins/gateways"
	"github.com/openshift-online/hypershell/components/api-server/test"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/environments"
)

// TestGRPCControlPlaneRejectsUnknownPhaseOnUpdate proves that phase is a
// control-plane-managed field (PLAT-DATA-23): the control plane writes it, and when it
// does the API server enforces the canonical phase vocabulary
// (specs/platform/gateway-phase-vocabulary.spec.md) -- an unknown phase is rejected with
// InvalidArgument and never persisted, a canonical phase is accepted. A user or import
// write of phase is rejected earlier by the field policy (policy_test.go), so this test
// exercises the remaining writer, the control plane.
// Verifies: GTW-PHS-02.01, GTW-PHS-02.02, GTW-PHS-02.03
func TestGRPCControlPlaneRejectsUnknownPhaseOnUpdate(t *testing.T) {
	h, _ := test.RegisterIntegration(t)
	h.StartControllersServer()

	clusterID, subject := registerTestClusterWithSubject(t)

	// A user creates the gateway with only the user-writable allowlist (name + cluster);
	// the control plane owns phase.
	userCtx := h.NewAuthenticatedContext(h.NewRandAccount())
	service := gateways.Service(&environments.Environment().Services)
	gw, svcErr := service.Create(userCtx, &gateways.Gateway{Name: "cp-phase", ClusterId: clusterID})
	Expect(svcErr).NotTo(HaveOccurred())

	cpAccount := h.NewAccount("service-account-cp-"+subject, "cp", "")
	cpToken := h.CreateJWTStringWithClaims(cpAccount, test.ControlPlaneClaims(subject))
	cp := pb.NewGatewayServiceClient(dialGRPC(t, h, cpToken))
	ctx := context.Background()

	badPhase := "Booting"
	_, err := cp.UpdateGateway(ctx, &pb.UpdateGatewayRequest{Id: gw.ID, Phase: &badPhase})
	Expect(err).To(HaveOccurred(), "control-plane update with an unknown phase must be rejected")
	Expect(status.Code(err)).To(Equal(codes.InvalidArgument))

	// The rejected value was not persisted.
	got, err := cp.GetGateway(ctx, &pb.GetGatewayRequest{Id: gw.ID})
	Expect(err).NotTo(HaveOccurred())
	Expect(got.Gateway.GetPhase()).To(BeEmpty())

	// A canonical phase on update is accepted.
	runningPhase := "Running"
	_, err = cp.UpdateGateway(ctx, &pb.UpdateGatewayRequest{Id: gw.ID, Phase: &runningPhase})
	Expect(err).NotTo(HaveOccurred())

	got, err = cp.GetGateway(ctx, &pb.GetGatewayRequest{Id: gw.ID})
	Expect(err).NotTo(HaveOccurred())
	Expect(got.Gateway.GetPhase()).To(Equal("Running"))
}

// TestGatewayPatchRenameAcceptsLegacyPhaseRecord proves the vocabulary tightening does
// not lock out pre-existing rows: a gateway whose stored phase predates validation
// (seeded through the service, a non-user write path) can still be renamed, because phase
// validation fires only when a write sets phase and name is the only user-writable update
// field (PLAT-DATA-23).
func TestGatewayPatchRenameAcceptsLegacyPhaseRecord(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	legacy, err := newGateway("legacy-phase")
	Expect(err).NotTo(HaveOccurred())
	Expect(legacy.Phase).NotTo(BeNil())
	Expect(*legacy.Phase).To(Equal("test-phase"), "factory seeds a non-canonical phase")

	_, resp, err := client.DefaultAPI.UpdateGateway(ctx, legacy.ID).GatewayPatchRequest(openapi.GatewayPatchRequest{
		Name: openapi.PtrString("legacy-renamed"),
	}).Execute()
	Expect(err).NotTo(HaveOccurred(), "renaming a legacy record must be accepted")
	Expect(resp.StatusCode).To(Equal(http.StatusOK))
}
