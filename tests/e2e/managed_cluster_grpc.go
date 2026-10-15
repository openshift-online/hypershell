package e2e

import (
	"context"
	"testing"
	"time"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (s *E2ESuite) assertManagedClusterGRPCIdentity(t *testing.T) {
	if s.driver.Name() != "kind" {
		t.Log("gRPC identity checks use the Kind API-server pod boundary; OpenShift external gRPC discovery is not configured")
		return
	}
	pods, err := s.clients.Kube.CoreV1().Pods(s.driver.PlatformNamespace()).List(t.Context(), metav1.ListOptions{LabelSelector: "app=hypershell-api-server"})
	s.Require().NoError(err, "list API server pods for gRPC identity checks")
	s.Require().NotEmpty(pods.Items, "API server pod for gRPC identity checks")

	forwardCtx, cancel := context.WithCancel(t.Context())
	defer cancel()
	address, closeForward, err := s.clients.PortForwardPod(forwardCtx, s.driver.PlatformNamespace(), pods.Items[0].Name, 9000)
	s.Require().NoError(err, "port-forward API server gRPC listener")
	defer closeForward()

	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	s.Require().NoError(err, "dial API server gRPC listener")
	defer func() { _ = conn.Close() }()
	client := pb.NewGatewayServiceClient(conn)

	s.Assert().Equal(codes.Unauthenticated, watchGatewaysCode(t.Context(), client, "", &s.clusterID), "watch without token")
	registrar, err := s.driver.AcquireClientCredentialsToken(t.Context(),
		envOrDefault("E2E_REGISTRAR_CLIENT_ID", "hypershell-control-plane"),
		envOrDefault("E2E_REGISTRAR_CLIENT_SECRET", "control-plane-secret"))
	s.Require().NoError(err, "acquire registrar token for gRPC identity checks")
	s.Assert().Equal(codes.PermissionDenied, watchGatewaysCode(t.Context(), client, registrar.AccessToken, stringPointer("e2e-not-my-cluster")), "watch for foreign cluster")
	s.Assert().Equal(codes.InvalidArgument, watchGatewaysCode(t.Context(), client, registrar.AccessToken, nil), "watch without cluster filter")
}

func watchGatewaysCode(parent context.Context, client pb.GatewayServiceClient, token string, clusterID *string) codes.Code {
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	if token != "" {
		ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token)
	}
	stream, err := client.WatchGateways(ctx, &pb.WatchGatewaysRequest{ClusterId: clusterID})
	if err == nil {
		_, err = stream.Recv()
	}
	if err == nil {
		return codes.OK
	}
	return status.Code(err)
}

func stringPointer(value string) *string { return &value }
