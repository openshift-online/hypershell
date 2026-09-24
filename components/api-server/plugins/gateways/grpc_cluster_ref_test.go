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

// A gRPC create or reassignment to an unregistered cluster is INVALID_ARGUMENT.
func TestGRPCGatewayRejectsUnregisteredCluster(t *testing.T) {
	h, _ := test.RegisterIntegration(t)
	gw := pb.NewGatewayServiceClient(dialGRPC(t, h, h.CreateJWTString(h.NewRandAccount())))
	ctx := context.Background()

	for _, clusterID := range []string{"", createManualCluster(t)} {
		_, err := gw.CreateGateway(ctx, &pb.CreateGatewayRequest{Name: "grpc-unregistered", ClusterId: clusterID, ReleaseId: "test-release"})
		Expect(status.Code(err)).To(Equal(codes.InvalidArgument), "create with cluster_id %q: %v", clusterID, err)
	}

	created, err := gw.CreateGateway(ctx, &pb.CreateGatewayRequest{Name: "grpc-registered", ClusterId: registerTestCluster(t), ReleaseId: "test-release"})
	Expect(err).NotTo(HaveOccurred())
	_, err = gw.UpdateGateway(ctx, &pb.UpdateGatewayRequest{Id: created.Gateway.Metadata.Id, ClusterId: strPtr(createManualCluster(t))})
	Expect(status.Code(err)).To(Equal(codes.InvalidArgument), "reassign to unregistered cluster: %v", err)
	// Re-sending the stored cluster_id (as status write-backs do) is accepted.
	_, err = gw.UpdateGateway(ctx, &pb.UpdateGatewayRequest{Id: created.Gateway.Metadata.Id, ClusterId: strPtr(created.Gateway.ClusterId), Status: strPtr("ok")})
	Expect(err).NotTo(HaveOccurred())
}
