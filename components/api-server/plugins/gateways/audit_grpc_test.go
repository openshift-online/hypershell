package gateways_test

import (
	"context"
	"strings"
	"testing"

	"github.com/golang-jwt/jwt/v4"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/api-server/test"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/audit"
)

// TestGRPCGatewayDeleteEmitsAuditEvent verifies the framework audit trail
// (SEC-AUD-01, SEC-AUD-07): a platform administrator deleting a gateway over
// gRPC produces one audit event with action "delete" for kind "gateways" with
// the administrator as actor. The integration environment records events in an
// in-memory recorder (OverrideAudit) so the test can assert on them.
func TestGRPCGatewayDeleteEmitsAuditEvent(t *testing.T) {
	h, _ := test.RegisterIntegration(t)

	rec, ok := h.Env().Audit.Recorder().(*audit.MemoryRecorder)
	Expect(ok).To(BeTrue(), "the integration environment must use audit.MemoryRecorder")

	// Create the gateway as its control plane, the identity allowed to set the
	// cluster placement (PLAT-DATA-23), mirroring TestGRPCGatewayCRUD.
	cpClient, clusterID := newControlPlaneGatewayClient(t, h)
	created, err := cpClient.CreateGateway(context.Background(), &pb.CreateGatewayRequest{
		Name:      "audit-delete-test",
		ClusterId: clusterID,
	})
	Expect(err).NotTo(HaveOccurred())
	gatewayID := created.Gateway.Metadata.Id

	// Act as a platform administrator over gRPC. The gRPC interceptor syncs the
	// JWT-carried platform:admin realm role into a binding, so the admin may
	// delete any gateway.
	adminUsername := "audit-admin-" + strings.ToLower(api.NewID())
	adminAccount := h.NewAccount(adminUsername, adminUsername, "")
	adminToken := h.CreateJWTStringWithClaims(adminAccount, jwt.MapClaims{
		"realm_access": map[string]interface{}{"roles": []interface{}{"platform:admin"}},
	})
	adminConn, err := grpc.NewClient(
		h.GRPCAddress(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithPerRPCCredentials(&bearerToken{token: adminToken}),
	)
	Expect(err).NotTo(HaveOccurred())
	t.Cleanup(func() { Expect(adminConn.Close()).To(Succeed()) })
	adminClient := pb.NewGatewayServiceClient(adminConn)

	// Isolate the delete from the create and registration traffic above.
	rec.Reset()

	_, err = adminClient.DeleteGateway(context.Background(), &pb.DeleteGatewayRequest{Id: gatewayID})
	Expect(err).NotTo(HaveOccurred())

	var deleteEvent *audit.Event
	for _, e := range rec.Events() {
		e := e
		if e.Action == "delete" && e.Resource.Kind == "gateways" && e.Resource.ID == gatewayID {
			deleteEvent = &e
			break
		}
	}
	Expect(deleteEvent).NotTo(BeNil(), "expected a delete audit event for the gateway; events: %+v", rec.Events())
	Expect(deleteEvent.Transport).To(Equal(audit.TransportGRPC))
	Expect(deleteEvent.Outcome).To(Equal(audit.OutcomeSuccess))
	Expect(deleteEvent.Actor.Type).To(Equal(audit.ActorUser))
	Expect(deleteEvent.Actor.Username).To(Equal(strings.ToLower(adminUsername)))
}
