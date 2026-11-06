package defaultWorkspaceMemberships

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
)

// Verifies: a registered control plane cannot use its own (legitimate)
// gateway_id to overwrite the status of a membership row that actually belongs
// to a different gateway. The gRPC authorization layer only checks that the
// REQUEST's gateway_id belongs to the caller's cluster; this handler is the one
// place that cross-checks the request's gateway_id against the row's real
// gateway_id.
func TestUpdateDefaultWorkspaceMembershipStatus_RejectsGatewayIDMismatch(t *testing.T) {
	svc := NewService(newFakeDao(), noopEvents{})
	item, err := svc.Create(context.Background(), "gw-real-owner", "dana", SourceGatewayAccess)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	handler := NewGRPCHandler(svc, nil, nil)

	_, grpcErr := handler.UpdateDefaultWorkspaceMembershipStatus(context.Background(), &pb.UpdateDefaultWorkspaceMembershipStatusRequest{
		Id:                   item.ID,
		GatewayId:            "gw-attacker-claimed",
		ReconciliationStatus: StatusReady,
	})
	if grpcErr == nil {
		t.Fatal("expected an error for a gateway_id that does not own the membership")
	}
	if st, ok := status.FromError(grpcErr); !ok || st.Code() != codes.PermissionDenied {
		t.Fatalf("error = %v, want codes.PermissionDenied", grpcErr)
	}

	// The status must be unchanged.
	unchanged, err := svc.Get(context.Background(), item.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if unchanged.ReconciliationStatus != StatusPending {
		t.Fatalf("status = %q, want unchanged %q", unchanged.ReconciliationStatus, StatusPending)
	}
}

func TestUpdateDefaultWorkspaceMembershipStatus_SucceedsForTheOwningGateway(t *testing.T) {
	svc := NewService(newFakeDao(), noopEvents{})
	item, err := svc.Create(context.Background(), "gw-real-owner", "dana", SourceGatewayAccess)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	handler := NewGRPCHandler(svc, nil, nil)

	resp, grpcErr := handler.UpdateDefaultWorkspaceMembershipStatus(context.Background(), &pb.UpdateDefaultWorkspaceMembershipStatusRequest{
		Id:                   item.ID,
		GatewayId:            "gw-real-owner",
		ReconciliationStatus: StatusReady,
	})
	if grpcErr != nil {
		t.Fatalf("unexpected error: %v", grpcErr)
	}
	if resp.GetItem().GetReconciliationStatus() != StatusReady {
		t.Fatalf("status = %q, want %q", resp.GetItem().GetReconciliationStatus(), StatusReady)
	}
}

func TestUpdateDefaultWorkspaceMembershipStatus_UnknownIDIsNotFound(t *testing.T) {
	svc := NewService(newFakeDao(), noopEvents{})
	handler := NewGRPCHandler(svc, nil, nil)

	_, grpcErr := handler.UpdateDefaultWorkspaceMembershipStatus(context.Background(), &pb.UpdateDefaultWorkspaceMembershipStatusRequest{
		Id:                   "does-not-exist",
		GatewayId:            "gw-1",
		ReconciliationStatus: StatusReady,
	})
	if grpcErr == nil {
		t.Fatal("expected a not-found error")
	}
	if st, ok := status.FromError(grpcErr); !ok || st.Code() != codes.NotFound {
		t.Fatalf("error = %v, want codes.NotFound", grpcErr)
	}
}
