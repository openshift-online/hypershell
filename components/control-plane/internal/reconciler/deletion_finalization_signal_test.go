package reconciler

import (
	"context"
	"testing"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// recordingFinalizeClient records which finalization RPC the reconciler calls so
// the test can assert the control plane reports completion through the dedicated
// FinalizeGatewayDeletion method (GTW-DEL-10) and never re-calls DeleteGateway,
// which the control-plane identity is denied.
type recordingFinalizeClient struct {
	pb.GatewayServiceClient
	finalizeIDs []string
	deleteIDs   []string
	finalizeErr error
}

func (c *recordingFinalizeClient) FinalizeGatewayDeletion(_ context.Context, in *pb.FinalizeGatewayDeletionRequest, _ ...grpc.CallOption) (*pb.FinalizeGatewayDeletionResponse, error) {
	c.finalizeIDs = append(c.finalizeIDs, in.GetId())
	if c.finalizeErr != nil {
		return nil, c.finalizeErr
	}
	return &pb.FinalizeGatewayDeletionResponse{}, nil
}

func (c *recordingFinalizeClient) DeleteGateway(_ context.Context, in *pb.DeleteGatewayRequest, _ ...grpc.CallOption) (*pb.DeleteGatewayResponse, error) {
	c.deleteIDs = append(c.deleteIDs, in.GetId())
	return &pb.DeleteGatewayResponse{}, nil
}

// TestReportDeletionFinalizedUsesDedicatedRPC verifies GTW-DEL-10.01: once a
// delete's teardown is complete, the control plane reports finalization through
// FinalizeGatewayDeletion and does not re-call DeleteGateway as the signal.
func TestReportDeletionFinalizedUsesDedicatedRPC(t *testing.T) {
	fake := &recordingFinalizeClient{}
	r := &GatewayReconciler{gatewayClientOverride: fake}

	r.reportDeletionFinalized(context.Background(), "gw-1")

	if got := fake.finalizeIDs; len(got) != 1 || got[0] != "gw-1" {
		t.Fatalf("FinalizeGatewayDeletion calls = %v, want exactly [gw-1]", got)
	}
	if len(fake.deleteIDs) != 0 {
		t.Fatalf("DeleteGateway was called as the finalization signal: %v", fake.deleteIDs)
	}
}

// TestReportDeletionFinalizedTreatsNotFoundAsSuccess verifies GTW-DEL-10.04: a
// NotFound response (another replica finalized the delete first) is not surfaced
// as a failure.
func TestReportDeletionFinalizedTreatsNotFoundAsSuccess(t *testing.T) {
	fake := &recordingFinalizeClient{finalizeErr: status.Error(codes.NotFound, "gateway not found")}
	r := &GatewayReconciler{gatewayClientOverride: fake}

	// Must not panic or re-call DeleteGateway; NotFound is treated as success.
	r.reportDeletionFinalized(context.Background(), "gw-1")

	if len(fake.finalizeIDs) != 1 {
		t.Fatalf("FinalizeGatewayDeletion calls = %v, want exactly one", fake.finalizeIDs)
	}
	if len(fake.deleteIDs) != 0 {
		t.Fatalf("DeleteGateway must never be used as the finalization signal: %v", fake.deleteIDs)
	}
}
