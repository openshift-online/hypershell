package gateways_test

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/api-server/test"
)

func dialGRPC(t *testing.T, h *test.Helper, token string) *grpc.ClientConn {
	t.Helper()
	opts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	if token != "" {
		opts = append(opts, grpc.WithPerRPCCredentials(&bearerToken{token: token}))
	}
	conn, err := grpc.NewClient(h.GRPCAddress(), opts...)
	Expect(err).NotTo(HaveOccurred())
	t.Cleanup(func() { Expect(conn.Close()).To(Succeed()) })
	return conn
}

func strPtr(s string) *string { return &s }

// newControlPlaneGatewayClient returns a gateway gRPC client authenticated as a
// registered control-plane identity, plus the id of the cluster it registered. Its writes
// are marked control-plane, so they bypass the default-deny gateway field policy and may
// set platform-managed / observed fields that a user or import caller may not
// (PLAT-DATA-23 / SEC-RBAC-18). Integration tests run with RBAC off, so this marker, not
// method authorization, is what distinguishes a control-plane write from a user write.
func newControlPlaneGatewayClient(t *testing.T, h *test.Helper) (pb.GatewayServiceClient, string) {
	t.Helper()
	clusterID, subject := registerTestClusterWithSubject(t)
	cpAccount := h.NewAccount("service-account-cp-"+subject, "cp", "")
	cpToken := h.CreateJWTStringWithClaims(cpAccount, test.ControlPlaneClaims(subject))
	return pb.NewGatewayServiceClient(dialGRPC(t, h, cpToken)), clusterID
}

// A gRPC create or reassignment to an unregistered cluster is INVALID_ARGUMENT.
func TestGRPCGatewayRejectsUnregisteredCluster(t *testing.T) {
	h, _ := test.RegisterIntegration(t)
	gw := pb.NewGatewayServiceClient(dialGRPC(t, h, h.CreateJWTString(h.NewRandAccount())))
	ctx := context.Background()

	for _, clusterID := range []string{"", createManualCluster(t)} {
		_, err := gw.CreateGateway(ctx, &pb.CreateGatewayRequest{Name: "grpc-unregistered", ClusterId: clusterID})
		Expect(status.Code(err)).To(Equal(codes.InvalidArgument), "create with cluster_id %q: %v", clusterID, err)
	}

	created, err := gw.CreateGateway(ctx, &pb.CreateGatewayRequest{Name: "grpc-registered", ClusterId: registerTestCluster(t)})
	Expect(err).NotTo(HaveOccurred())
	// cluster_id is not user-writable on update (PLAT-DATA-23); an attempt to move the
	// gateway to an unregistered cluster is rejected by the handler's cluster validation
	// before the field policy, so it stays INVALID_ARGUMENT.
	_, err = gw.UpdateGateway(ctx, &pb.UpdateGatewayRequest{Id: created.Gateway.Metadata.Id, ClusterId: strPtr(createManualCluster(t))})
	Expect(status.Code(err)).To(Equal(codes.InvalidArgument), "reassign to unregistered cluster: %v", err)
	// Re-sending the stored cluster_id (unchanged) is not a reassignment, so a user may
	// still rename the gateway in the same request.
	_, err = gw.UpdateGateway(ctx, &pb.UpdateGatewayRequest{Id: created.Gateway.Metadata.Id, ClusterId: strPtr(created.Gateway.ClusterId), Name: strPtr("grpc-renamed")})
	Expect(err).NotTo(HaveOccurred())
}
