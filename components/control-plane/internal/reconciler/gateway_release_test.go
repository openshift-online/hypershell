package reconciler

import (
	"context"
	"fmt"
	"strings"
	"testing"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/control-plane/internal/watcher"
	"google.golang.org/grpc"
)

// fakeReleaseClient records UpdateGatewayRelease calls and can inject an error.
type fakeReleaseClient struct {
	pb.GatewayReleaseServiceClient
	updates   []*pb.UpdateGatewayReleaseRequest
	updateErr error
}

func (f *fakeReleaseClient) UpdateGatewayRelease(ctx context.Context, in *pb.UpdateGatewayReleaseRequest, opts ...grpc.CallOption) (*pb.UpdateGatewayReleaseResponse, error) {
	f.updates = append(f.updates, in)
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	return &pb.UpdateGatewayReleaseResponse{}, nil
}

// fakeReleaseGatewayClient serves a fixed gateway inventory to ListGateways.
type fakeReleaseGatewayClient struct {
	pb.GatewayServiceClient
	gateways []*pb.Gateway
	listErr  error
}

func (f *fakeReleaseGatewayClient) ListGateways(ctx context.Context, in *pb.ListGatewaysRequest, opts ...grpc.CallOption) (*pb.ListGatewaysResponse, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return &pb.ListGatewaysResponse{
		Items:    f.gateways,
		Metadata: &pb.ListMeta{Page: in.Page, Size: in.Size, Total: int32(len(f.gateways))},
	}, nil
}

// recordingEnqueuer captures the gateways enqueued for reconciliation.
type recordingEnqueuer struct {
	enqueued []string
}

func (r *recordingEnqueuer) EnqueueForced(ev watcher.Event[*pb.Gateway]) {
	r.enqueued = append(r.enqueued, ev.ResourceID)
}

func newTestReleaseReconciler(gw pb.GatewayServiceClient, rel pb.GatewayReleaseServiceClient, q gatewayEnqueuer) *GatewayReleaseReconciler {
	return &GatewayReleaseReconciler{
		active:    make(map[string]struct{}),
		lastImage: make(map[string]string),
		gateways:  gw,
		releases:  rel,
		gwQueue:   q,
	}
}

func releaseEvent(t watcher.EventType, id, image, status string) watcher.Event[*pb.GatewayRelease] {
	rel := &pb.GatewayRelease{
		Metadata: &pb.ObjectReference{Id: id},
		Name:     "rel-" + id,
		Image:    image,
	}
	if status != "" {
		rel.Status = &status
	}
	return watcher.Event[*pb.GatewayRelease]{Type: t, ResourceID: id, Resource: rel}
}

func gatewayWithRelease(id, releaseID string) *pb.Gateway {
	return &pb.Gateway{
		Metadata:  &pb.ObjectReference{Id: id},
		Name:      "gw-" + id,
		ReleaseId: releaseID,
	}
}

func TestGatewayRelease_ValidImageSetsAvailableWithoutFanOut(t *testing.T) {
	rel := &fakeReleaseClient{}
	gw := &fakeReleaseGatewayClient{gateways: []*pb.Gateway{gatewayWithRelease("g1", "r1")}}
	q := &recordingEnqueuer{}
	r := newTestReleaseReconciler(gw, rel, q)

	err := r.Handle(context.Background(), releaseEvent(watcher.EventCreated, "r1", "registry.redhat.io/openshell/gateway:v1", ""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rel.updates) != 1 || rel.updates[0].GetStatus() != releaseStatusAvailable {
		t.Fatalf("expected status Available, got updates=%v", rel.updates)
	}
	// First observation is a baseline: no fan-out even though g1 references r1.
	if len(q.enqueued) != 0 {
		t.Fatalf("expected no fan-out on first observation, got %v", q.enqueued)
	}
}

func TestGatewayRelease_MalformedImageSetsInvalidAndSkipsFanOut(t *testing.T) {
	rel := &fakeReleaseClient{}
	gw := &fakeReleaseGatewayClient{gateways: []*pb.Gateway{gatewayWithRelease("g1", "r1")}}
	q := &recordingEnqueuer{}
	r := newTestReleaseReconciler(gw, rel, q)

	// Seed a baseline with a valid image so a subsequent invalid update would
	// otherwise be a "change".
	if err := r.Handle(context.Background(), releaseEvent(watcher.EventCreated, "r1", "registry.redhat.io/openshell/gateway:v1", "")); err != nil {
		t.Fatalf("seed: %v", err)
	}
	rel.updates = nil
	q.enqueued = nil

	err := r.Handle(context.Background(), releaseEvent(watcher.EventUpdated, "r1", "gateway:v1; rm -rf /", ""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rel.updates) != 1 || !strings.HasPrefix(rel.updates[0].GetStatus(), releaseStatusInvalid) {
		t.Fatalf("expected Invalid status with reason, got %v", rel.updates)
	}
	if len(q.enqueued) != 0 {
		t.Fatalf("invalid release must not fan out, got %v", q.enqueued)
	}
}

func TestGatewayRelease_CorrectionAfterInvalidFansOut(t *testing.T) {
	rel := &fakeReleaseClient{}
	gw := &fakeReleaseGatewayClient{gateways: []*pb.Gateway{gatewayWithRelease("g1", "r1")}}
	q := &recordingEnqueuer{}
	r := newTestReleaseReconciler(gw, rel, q)

	// Baseline established at v1.
	if err := r.Handle(context.Background(), releaseEvent(watcher.EventCreated, "r1", "registry.redhat.io/openshell/gateway:v1", releaseStatusAvailable)); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// An invalid update must not fan out and must not drop the v1 baseline.
	if err := r.Handle(context.Background(), releaseEvent(watcher.EventUpdated, "r1", "gateway:v1; rm -rf /", releaseStatusAvailable)); err != nil {
		t.Fatalf("invalid update: %v", err)
	}
	q.enqueued = nil

	// Correcting to a different valid image (v3) is a genuine change from the
	// retained v1 baseline and must fan out to referencing gateways.
	if err := r.Handle(context.Background(), releaseEvent(watcher.EventUpdated, "r1", "registry.redhat.io/openshell/gateway:v3", releaseStatusAvailable)); err != nil {
		t.Fatalf("correction: %v", err)
	}
	if len(q.enqueued) != 1 || q.enqueued[0] != "g1" {
		t.Fatalf("expected g1 to fan out after correction, got %v", q.enqueued)
	}
}

func TestGatewayRelease_NoRedundantStatusWrite(t *testing.T) {
	rel := &fakeReleaseClient{}
	r := newTestReleaseReconciler(&fakeReleaseGatewayClient{}, rel, &recordingEnqueuer{})

	// Persisted status already Available and image valid: no update expected.
	err := r.Handle(context.Background(), releaseEvent(watcher.EventUpdated, "r1", "registry.redhat.io/openshell/gateway:v1", releaseStatusAvailable))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rel.updates) != 0 {
		t.Fatalf("expected no status write, got %v", rel.updates)
	}
}

func TestGatewayRelease_ImageChangeFansOutToReferencingGatewaysOnly(t *testing.T) {
	rel := &fakeReleaseClient{}
	gw := &fakeReleaseGatewayClient{gateways: []*pb.Gateway{
		gatewayWithRelease("g1", "r1"),
		gatewayWithRelease("g2", "r1"),
		gatewayWithRelease("g3", "other"),
	}}
	q := &recordingEnqueuer{}
	r := newTestReleaseReconciler(gw, rel, q)

	// Establish baseline.
	if err := r.Handle(context.Background(), releaseEvent(watcher.EventCreated, "r1", "registry.redhat.io/openshell/gateway:v1", releaseStatusAvailable)); err != nil {
		t.Fatalf("seed: %v", err)
	}
	q.enqueued = nil

	// Image changes -> fan out to g1 and g2 only.
	err := r.Handle(context.Background(), releaseEvent(watcher.EventUpdated, "r1", "registry.redhat.io/openshell/gateway:v2", releaseStatusAvailable))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(q.enqueued) != 2 {
		t.Fatalf("expected 2 gateways enqueued, got %v", q.enqueued)
	}
	got := map[string]bool{}
	for _, id := range q.enqueued {
		got[id] = true
	}
	if !got["g1"] || !got["g2"] || got["g3"] {
		t.Fatalf("fan-out targeted wrong gateways: %v", q.enqueued)
	}
}

func TestGatewayRelease_RenameDoesNotFanOut(t *testing.T) {
	rel := &fakeReleaseClient{}
	gw := &fakeReleaseGatewayClient{gateways: []*pb.Gateway{gatewayWithRelease("g1", "r1")}}
	q := &recordingEnqueuer{}
	r := newTestReleaseReconciler(gw, rel, q)

	if err := r.Handle(context.Background(), releaseEvent(watcher.EventCreated, "r1", "registry.redhat.io/openshell/gateway:v1", releaseStatusAvailable)); err != nil {
		t.Fatalf("seed: %v", err)
	}
	q.enqueued = nil

	// Same image, unchanged: no fan-out.
	err := r.Handle(context.Background(), releaseEvent(watcher.EventUpdated, "r1", "registry.redhat.io/openshell/gateway:v1", releaseStatusAvailable))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(q.enqueued) != 0 {
		t.Fatalf("expected no fan-out on unchanged image, got %v", q.enqueued)
	}
}

func TestGatewayRelease_DeleteIsNoOp(t *testing.T) {
	rel := &fakeReleaseClient{}
	q := &recordingEnqueuer{}
	r := newTestReleaseReconciler(&fakeReleaseGatewayClient{}, rel, q)

	err := r.Handle(context.Background(), releaseEvent(watcher.EventDeleted, "r1", "registry.redhat.io/openshell/gateway:v1", ""))
	if err != nil {
		t.Fatalf("expected delete to succeed, got %v", err)
	}
	if len(rel.updates) != 0 || len(q.enqueued) != 0 {
		t.Fatalf("delete must not write status or fan out: updates=%v enqueued=%v", rel.updates, q.enqueued)
	}
}

func TestGatewayRelease_StatusWriteFailureIsRetried(t *testing.T) {
	rel := &fakeReleaseClient{updateErr: fmt.Errorf("api server unavailable")}
	r := newTestReleaseReconciler(&fakeReleaseGatewayClient{}, rel, &recordingEnqueuer{})

	err := r.Handle(context.Background(), releaseEvent(watcher.EventCreated, "r1", "registry.redhat.io/openshell/gateway:v1", ""))
	if err == nil {
		t.Fatalf("expected error to be returned so the reconcile is requeued")
	}
}

func TestGatewayRelease_FanOutFailureIsRetriedAndReDetected(t *testing.T) {
	rel := &fakeReleaseClient{}
	gw := &fakeReleaseGatewayClient{listErr: fmt.Errorf("list unavailable")}
	q := &recordingEnqueuer{}
	r := newTestReleaseReconciler(gw, rel, q)

	// Baseline with a valid image.
	if err := r.Handle(context.Background(), releaseEvent(watcher.EventCreated, "r1", "v1.example/img:1", releaseStatusAvailable)); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Image change but listing fails -> error returned, baseline NOT advanced.
	if err := r.Handle(context.Background(), releaseEvent(watcher.EventUpdated, "r1", "v1.example/img:2", releaseStatusAvailable)); err == nil {
		t.Fatalf("expected fan-out error to be returned")
	}

	// Listing recovers; the change must still be detected on retry.
	gw.listErr = nil
	gw.gateways = []*pb.Gateway{gatewayWithRelease("g1", "r1")}
	if err := r.Handle(context.Background(), releaseEvent(watcher.EventUpdated, "r1", "v1.example/img:2", releaseStatusAvailable)); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if len(q.enqueued) != 1 || q.enqueued[0] != "g1" {
		t.Fatalf("expected g1 enqueued on retry, got %v", q.enqueued)
	}
}
