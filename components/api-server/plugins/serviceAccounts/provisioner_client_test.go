package serviceAccounts

import (
	"context"
	"errors"
	"testing"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/provisioner/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestControlPlaneProvisionerMapsDesiredStateAndOneTimeSecret(t *testing.T) {
	client := &fakeProvisionerClient{provisionResponse: &pb.ProvisionResponse{
		ClientUuid: "client-uuid", ClientId: "client-id", ClientSecret: "one-time-secret", Subject: "subject-id",
	}}
	provisioner := &controlPlaneProvisioner{client: client}
	result, err := provisioner.ProvisionServiceAccount(t.Context(), ProvisioningSpec{
		ClientID: "client-id", DisplayName: "build bot", GatewayClientID: "gateway-client",
		GatewayID: "gateway-id", ServiceAccountID: "account-id", CreatorUserID: "user-id",
		Role: "openshell-user", ExpectedIssuer: "https://issuer.example/realms/hypershell",
		AccessTokenLifetimeSeconds: 300,
	})
	if err != nil {
		t.Fatalf("ProvisionServiceAccount() error = %v", err)
	}
	if result.ClientSecret != "one-time-secret" || result.ClientUUID != "client-uuid" || result.Subject != "subject-id" {
		t.Fatalf("ProvisionServiceAccount() = %#v", result)
	}
	if client.provisionRequest.GetSpec().GetGatewayId() != "gateway-id" ||
		client.provisionRequest.GetSpec().GetRole() != "openshell-user" ||
		client.provisionRequest.GetSpec().GetAccessTokenLifetimeSeconds() != 300 {
		t.Fatalf("provision request = %#v", client.provisionRequest)
	}
}

func TestControlPlaneProvisionerMapsNotFound(t *testing.T) {
	provisioner := &controlPlaneProvisioner{client: &fakeProvisionerClient{
		reconcileErr: status.Error(codes.NotFound, "managed client missing"),
	}}
	err := provisioner.ReconcileServiceAccount(t.Context(), ProvisioningSpec{}, "client-uuid", "subject-id", true)
	if !errors.Is(err, ErrProvisionerNotFound) {
		t.Fatalf("ReconcileServiceAccount() error = %v, want ErrProvisionerNotFound", err)
	}
}

type fakeProvisionerClient struct {
	provisionRequest  *pb.ProvisionRequest
	provisionResponse *pb.ProvisionResponse
	provisionErr      error
	reconcileErr      error
}

func (f *fakeProvisionerClient) Provision(_ context.Context, request *pb.ProvisionRequest, _ ...grpc.CallOption) (*pb.ProvisionResponse, error) {
	f.provisionRequest = request
	return f.provisionResponse, f.provisionErr
}

func (f *fakeProvisionerClient) Reconcile(context.Context, *pb.ReconcileRequest, ...grpc.CallOption) (*pb.ReconcileResponse, error) {
	return &pb.ReconcileResponse{}, f.reconcileErr
}

func (f *fakeProvisionerClient) Disable(context.Context, *pb.DisableRequest, ...grpc.CallOption) (*pb.DisableResponse, error) {
	return &pb.DisableResponse{}, nil
}

func (f *fakeProvisionerClient) Delete(context.Context, *pb.DeleteRequest, ...grpc.CallOption) (*pb.DeleteResponse, error) {
	return &pb.DeleteResponse{}, nil
}

func (f *fakeProvisionerClient) DeleteManaged(context.Context, *pb.DeleteManagedRequest, ...grpc.CallOption) (*pb.DeleteManagedResponse, error) {
	return &pb.DeleteManagedResponse{}, nil
}

func (f *fakeProvisionerClient) DeleteGateway(context.Context, *pb.DeleteGatewayRequest, ...grpc.CallOption) (*pb.DeleteGatewayResponse, error) {
	return &pb.DeleteGatewayResponse{}, nil
}

func (f *fakeProvisionerClient) ListManaged(context.Context, *pb.ListManagedRequest, ...grpc.CallOption) (*pb.ListManagedResponse, error) {
	return &pb.ListManagedResponse{}, nil
}
