package watcher

import (
	"context"
	"testing"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
)

// fakeDefaultWorkspaceMembershipSink records every enqueued event so a test can
// assert the reconcile queue key without a real workqueue.
type fakeDefaultWorkspaceMembershipSink struct {
	events []Event[*pb.DefaultWorkspaceMembership]
}

func (f *fakeDefaultWorkspaceMembershipSink) enqueue(ev Event[*pb.DefaultWorkspaceMembership]) {
	f.events = append(f.events, ev)
}

// CTRL-CORE-16 reconciles a gateway's FULL live membership set as one unit, so
// the reconcile queue must be keyed by the membership's GATEWAY id, not its own
// row id: two different rows for the same gateway must coalesce onto the same
// key, and Handle must receive that gateway id as event.ResourceID.
func TestEnqueueDefaultWorkspaceMembershipEvent_KeyedByGatewayNotRowID(t *testing.T) {
	sink := &fakeDefaultWorkspaceMembershipSink{}
	m1 := membershipFixture("m-1", "gw-1", "sub-a", "pending")
	m2 := membershipFixture("m-2", "gw-1", "sub-b", "pending")

	enqueueDefaultWorkspaceMembershipEvent(context.Background(), "cluster-1", sink, EventCreated, "", m1)
	enqueueDefaultWorkspaceMembershipEvent(context.Background(), "cluster-1", sink, EventUpdated, "", m2)

	if len(sink.events) != 2 {
		t.Fatalf("enqueued events = %d, want 2", len(sink.events))
	}
	for _, ev := range sink.events {
		if ev.ResourceID != "gw-1" {
			t.Fatalf("ResourceID = %q, want the gateway id %q (not the row id)", ev.ResourceID, "gw-1")
		}
	}
}

// On delete, only the resource id (its gateway) is known reliably; the
// fallback gateway id must still be used to key the reconcile queue even when
// the deleted row carries it.
func TestEnqueueDefaultWorkspaceMembershipEvent_DeleteUsesGatewayKey(t *testing.T) {
	sink := &fakeDefaultWorkspaceMembershipSink{}
	m := membershipFixture("m-1", "gw-2", "sub-a", "ready")

	enqueueDefaultWorkspaceMembershipEvent(context.Background(), "cluster-1", sink, EventDeleted, "gw-2", m)

	if len(sink.events) != 1 {
		t.Fatalf("enqueued events = %d, want 1", len(sink.events))
	}
	if sink.events[0].ResourceID != "gw-2" {
		t.Fatalf("ResourceID = %q, want %q", sink.events[0].ResourceID, "gw-2")
	}
	if sink.events[0].Type != EventDeleted {
		t.Fatalf("Type = %v, want EventDeleted", sink.events[0].Type)
	}
}

// A membership with no gateway id is dropped rather than enqueued under an
// empty key, which would wrongly coalesce every such row onto one another.
func TestEnqueueDefaultWorkspaceMembershipEvent_NoGatewayIDIsDropped(t *testing.T) {
	sink := &fakeDefaultWorkspaceMembershipSink{}
	enqueueDefaultWorkspaceMembershipEvent(context.Background(), "cluster-1", sink, EventCreated, "", &pb.DefaultWorkspaceMembership{Subject: "sub-a"})
	if len(sink.events) != 0 {
		t.Fatalf("enqueued events = %#v, want none for a membership with no gateway id", sink.events)
	}
}

func membershipFixture(id, gatewayID, subject, status string) *pb.DefaultWorkspaceMembership {
	return &pb.DefaultWorkspaceMembership{
		Metadata:             &pb.ObjectReference{Id: id},
		GatewayId:            gatewayID,
		Subject:              subject,
		Role:                 "user",
		Source:               "gateway_access",
		ReconciliationStatus: status,
	}
}
