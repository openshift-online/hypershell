package reconciler

import (
	"context"
	"errors"
	"testing"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"google.golang.org/grpc"
)

// TestAdvanceObservedRelease pins the write-back contract: the observed release is
// advanced only to the release actually applied to the workload (appliedRelease),
// the write is skipped when it would be redundant (unchanged release or a
// direct-image gateway with no applied release), and a write-back failure is
// surfaced for retry rather than swallowed. Advancing to the *applied* release --
// not the desired one -- is what prevents falsely reporting a release the workload
// has not yet rolled out. See gateway-release-rollout.spec.md.
func TestAdvanceObservedRelease(t *testing.T) {
	t.Run("advances to the applied release", func(t *testing.T) {
		var got *pb.UpdateGatewayRequest
		client := &fakeGatewayClient{updateFn: func(_ context.Context, in *pb.UpdateGatewayRequest, _ ...grpc.CallOption) (*pb.UpdateGatewayResponse, error) {
			got = in
			return &pb.UpdateGatewayResponse{}, nil
		}}
		err := advanceObservedRelease(context.Background(), client, "gw-1", "rel-old", "rel-new")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got == nil {
			t.Fatal("expected an UpdateGateway call, got none")
		}
		if got.GetId() != "gw-1" {
			t.Errorf("Id = %q, want %q", got.GetId(), "gw-1")
		}
		if got.GetObservedReleaseId() != "rel-new" {
			t.Errorf("ObservedReleaseId = %q, want %q", got.GetObservedReleaseId(), "rel-new")
		}
		// Only the observed release is written back; phase/status stay untouched.
		if got.Phase != nil || got.Status != nil {
			t.Errorf("expected a narrow observed-release update, got phase=%v status=%v", got.Phase, got.Status)
		}
	})

	t.Run("no write when observed already matches the applied release", func(t *testing.T) {
		called := false
		client := &fakeGatewayClient{updateFn: func(_ context.Context, _ *pb.UpdateGatewayRequest, _ ...grpc.CallOption) (*pb.UpdateGatewayResponse, error) {
			called = true
			return &pb.UpdateGatewayResponse{}, nil
		}}
		if err := advanceObservedRelease(context.Background(), client, "gw-1", "rel-new", "rel-new"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if called {
			t.Error("expected no UpdateGateway call for an unchanged release")
		}
	})

	t.Run("no write for a direct-image gateway with no applied release", func(t *testing.T) {
		called := false
		client := &fakeGatewayClient{updateFn: func(_ context.Context, _ *pb.UpdateGatewayRequest, _ ...grpc.CallOption) (*pb.UpdateGatewayResponse, error) {
			called = true
			return &pb.UpdateGatewayResponse{}, nil
		}}
		if err := advanceObservedRelease(context.Background(), client, "gw-1", "", ""); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if called {
			t.Error("expected no UpdateGateway call when the workload has no applied release")
		}
	})

	t.Run("no write when the desired release is not yet applied", func(t *testing.T) {
		// The health loop must NOT advance observed to a desired release the workload
		// has not rolled out: it only ever passes the Deployment's applied release. An
		// empty applied release (the Deployment carries no annotation yet) is a no-op
		// even though a newer release may be desired.
		called := false
		client := &fakeGatewayClient{updateFn: func(_ context.Context, _ *pb.UpdateGatewayRequest, _ ...grpc.CallOption) (*pb.UpdateGatewayResponse, error) {
			called = true
			return &pb.UpdateGatewayResponse{}, nil
		}}
		if err := advanceObservedRelease(context.Background(), client, "gw-1", "rel-old", ""); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if called {
			t.Error("expected no UpdateGateway call when the desired release is not yet applied to the workload")
		}
	})

	t.Run("write-back failure is surfaced", func(t *testing.T) {
		client := &fakeGatewayClient{updateFn: func(_ context.Context, _ *pb.UpdateGatewayRequest, _ ...grpc.CallOption) (*pb.UpdateGatewayResponse, error) {
			return nil, errors.New("grpc down")
		}}
		err := advanceObservedRelease(context.Background(), client, "gw-1", "rel-old", "rel-new")
		if err == nil {
			t.Fatal("expected the write-back failure to be surfaced, got nil")
		}
	})
}
