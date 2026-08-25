package serviceaccountprovisioner

import (
	"context"
	"net"
	"testing"
	"time"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/provisioner/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func TestListenAndServeRequiresBindAddress(t *testing.T) {
	if err := ListenAndServe(t.Context(), TransportConfig{}, NewServer(&fakeProvider{configured: true})); err == nil {
		t.Fatal("ListenAndServe() error = nil, want error for empty bind address")
	}
}

func TestListenAndServeServesPlaintextProvisionerCalls(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for test: %v", err)
	}
	address := listener.Addr().String()
	// ListenAndServe binds its own listener from the address, so release this one.
	_ = listener.Close()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	served := make(chan error, 1)
	go func() {
		served <- ListenAndServe(ctx, TransportConfig{Address: address},
			NewServer(&fakeProvider{configured: true, secret: "one-time-secret"}))
	}()

	connection, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial provisioner: %v", err)
	}
	defer func() { _ = connection.Close() }()
	client := pb.NewOpenShellGatewayServiceAccountProvisionerServiceClient(connection)

	callCtx, callCancel := context.WithTimeout(ctx, 5*time.Second)
	defer callCancel()
	response, err := client.Provision(callCtx, &pb.ProvisionRequest{Spec: &pb.ServiceAccountSpec{GatewayId: "gateway-id"}})
	if err != nil {
		t.Fatalf("Provision() over plaintext gRPC error = %v", err)
	}
	if response.GetClientSecret() != "one-time-secret" {
		t.Fatalf("Provision() client secret = %q, want one-time-secret", response.GetClientSecret())
	}

	cancel()
	if serveErr := <-served; serveErr != nil {
		t.Fatalf("ListenAndServe() returned error = %v", serveErr)
	}
}
