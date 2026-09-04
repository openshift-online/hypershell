package reconciler

import (
	"context"
	"strings"
	"testing"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/control-plane/internal/watcher"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// fakeNetworkClient records UpdateGatewayNetwork calls and can inject an error.
type fakeNetworkClient struct {
	pb.GatewayNetworkServiceClient
	updates   []*pb.UpdateGatewayNetworkRequest
	updateErr error
}

func (f *fakeNetworkClient) UpdateGatewayNetwork(ctx context.Context, in *pb.UpdateGatewayNetworkRequest, opts ...grpc.CallOption) (*pb.UpdateGatewayNetworkResponse, error) {
	f.updates = append(f.updates, in)
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	return &pb.UpdateGatewayNetworkResponse{}, nil
}

// fakeNetworkGatewayClient resolves GetGateway against a fixed hub inventory and
// can inject a lookup error to simulate not-found or transient failures.
type fakeNetworkGatewayClient struct {
	pb.GatewayServiceClient
	existing map[string]bool
	getErr   error
}

func (f *fakeNetworkGatewayClient) GetGateway(ctx context.Context, in *pb.GetGatewayRequest, opts ...grpc.CallOption) (*pb.GetGatewayResponse, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	if f.existing[in.Id] {
		return &pb.GetGatewayResponse{Gateway: &pb.Gateway{Metadata: &pb.ObjectReference{Id: in.Id}}}, nil
	}
	return nil, status.Error(codes.NotFound, "gateway not found")
}

func newTestNetworkReconciler(gw pb.GatewayServiceClient, net pb.GatewayNetworkServiceClient) *GatewayNetworkReconciler {
	return &GatewayNetworkReconciler{
		active:   make(map[string]struct{}),
		gateways: gw,
		networks: net,
	}
}

func networkEvent(t watcher.EventType, id, topology, hubID, status string) watcher.Event[*pb.GatewayNetwork] {
	net := &pb.GatewayNetwork{
		Metadata: &pb.ObjectReference{Id: id},
		Name:     "net-" + id,
	}
	if topology != "" {
		net.Topology = &topology
	}
	if hubID != "" {
		net.HubGatewayId = &hubID
	}
	if status != "" {
		net.Status = &status
	}
	return watcher.Event[*pb.GatewayNetwork]{Type: t, ResourceID: id, Resource: net}
}

func TestGatewayNetwork_ValidHubSpokeSetsValid(t *testing.T) {
	net := &fakeNetworkClient{}
	gw := &fakeNetworkGatewayClient{existing: map[string]bool{"hub1": true}}
	r := newTestNetworkReconciler(gw, net)

	if err := r.Handle(context.Background(), networkEvent(watcher.EventCreated, "n1", "hub-spoke", "hub1", "")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(net.updates) != 1 || net.updates[0].GetStatus() != networkStatusValid {
		t.Fatalf("expected status Valid, got updates=%v", net.updates)
	}
}

func TestGatewayNetwork_ValidMeshSetsValid(t *testing.T) {
	net := &fakeNetworkClient{}
	r := newTestNetworkReconciler(&fakeNetworkGatewayClient{}, net)

	if err := r.Handle(context.Background(), networkEvent(watcher.EventCreated, "n1", "mesh", "", "")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(net.updates) != 1 || net.updates[0].GetStatus() != networkStatusValid {
		t.Fatalf("expected status Valid, got updates=%v", net.updates)
	}
}

func TestGatewayNetwork_UnrecognizedTopologyIsInvalid(t *testing.T) {
	net := &fakeNetworkClient{}
	r := newTestNetworkReconciler(&fakeNetworkGatewayClient{}, net)

	if err := r.Handle(context.Background(), networkEvent(watcher.EventCreated, "n1", "ring", "", "")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(net.updates) != 1 || !strings.HasPrefix(net.updates[0].GetStatus(), networkStatusInvalid) {
		t.Fatalf("expected Invalid status, got updates=%v", net.updates)
	}
	if !strings.Contains(net.updates[0].GetStatus(), "ring") {
		t.Fatalf("expected reason to mention the unrecognized topology, got %q", net.updates[0].GetStatus())
	}
}

func TestGatewayNetwork_HubSpokeWithoutHubIsInvalid(t *testing.T) {
	net := &fakeNetworkClient{}
	r := newTestNetworkReconciler(&fakeNetworkGatewayClient{}, net)

	if err := r.Handle(context.Background(), networkEvent(watcher.EventCreated, "n1", "hub-spoke", "", "")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(net.updates) != 1 || !strings.HasPrefix(net.updates[0].GetStatus(), networkStatusInvalid) {
		t.Fatalf("expected Invalid status, got updates=%v", net.updates)
	}
}

func TestGatewayNetwork_DanglingHubIsInvalid(t *testing.T) {
	net := &fakeNetworkClient{}
	// hub inventory is empty, so GetGateway returns NotFound for hub1.
	gw := &fakeNetworkGatewayClient{existing: map[string]bool{}}
	r := newTestNetworkReconciler(gw, net)

	if err := r.Handle(context.Background(), networkEvent(watcher.EventCreated, "n1", "hub-spoke", "hub1", "")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(net.updates) != 1 || !strings.HasPrefix(net.updates[0].GetStatus(), networkStatusInvalid) {
		t.Fatalf("expected Invalid status, got updates=%v", net.updates)
	}
	if !strings.Contains(net.updates[0].GetStatus(), "hub1") {
		t.Fatalf("expected reason to mention the missing hub gateway, got %q", net.updates[0].GetStatus())
	}
}

func TestGatewayNetwork_NoRedundantStatusWrite(t *testing.T) {
	net := &fakeNetworkClient{}
	gw := &fakeNetworkGatewayClient{existing: map[string]bool{"hub1": true}}
	r := newTestNetworkReconciler(gw, net)

	// Persisted status already equals the reconciled outcome.
	if err := r.Handle(context.Background(), networkEvent(watcher.EventUpdated, "n1", "hub-spoke", "hub1", networkStatusValid)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(net.updates) != 0 {
		t.Fatalf("expected no status write when unchanged, got %v", net.updates)
	}
}

func TestGatewayNetwork_NoRedundantInvalidStatusWrite(t *testing.T) {
	net := &fakeNetworkClient{}
	r := newTestNetworkReconciler(&fakeNetworkGatewayClient{}, net)

	// Persisted status already equals the recomputed Invalid outcome (same reason).
	persisted := networkStatusInvalid + ": unrecognized topology \"ring\""
	if err := r.Handle(context.Background(), networkEvent(watcher.EventUpdated, "n1", "ring", "", persisted)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(net.updates) != 0 {
		t.Fatalf("expected no status write when Invalid status unchanged, got %v", net.updates)
	}
}

func TestGatewayNetwork_DeleteIsNoOp(t *testing.T) {
	net := &fakeNetworkClient{}
	r := newTestNetworkReconciler(&fakeNetworkGatewayClient{}, net)

	if err := r.Handle(context.Background(), networkEvent(watcher.EventDeleted, "n1", "hub-spoke", "hub1", "")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(net.updates) != 0 {
		t.Fatalf("expected no status write on delete, got %v", net.updates)
	}
}

func TestGatewayNetwork_NilResourceIsNoOp(t *testing.T) {
	net := &fakeNetworkClient{}
	r := newTestNetworkReconciler(&fakeNetworkGatewayClient{}, net)

	ev := watcher.Event[*pb.GatewayNetwork]{Type: watcher.EventCreated, ResourceID: "n1", Resource: nil}
	if err := r.Handle(context.Background(), ev); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(net.updates) != 0 {
		t.Fatalf("expected no status write for nil resource, got %v", net.updates)
	}
}

func TestGatewayNetwork_TransientHubLookupSurfacesAsError(t *testing.T) {
	net := &fakeNetworkClient{}
	gw := &fakeNetworkGatewayClient{getErr: status.Error(codes.Unavailable, "hub lookup down")}
	r := newTestNetworkReconciler(gw, net)

	err := r.Handle(context.Background(), networkEvent(watcher.EventCreated, "n1", "hub-spoke", "hub1", ""))
	if err == nil {
		t.Fatalf("expected transient hub lookup failure to return an error")
	}
	// The network must not be settled to Invalid on account of a transient failure.
	if len(net.updates) != 0 {
		t.Fatalf("expected no status write on transient failure, got %v", net.updates)
	}
}

func TestGatewayNetwork_StatusWriteFailureSurfacesAsError(t *testing.T) {
	net := &fakeNetworkClient{updateErr: status.Error(codes.Unavailable, "api down")}
	gw := &fakeNetworkGatewayClient{existing: map[string]bool{"hub1": true}}
	r := newTestNetworkReconciler(gw, net)

	err := r.Handle(context.Background(), networkEvent(watcher.EventCreated, "n1", "hub-spoke", "hub1", ""))
	if err == nil {
		t.Fatalf("expected status write failure to return an error")
	}
}
