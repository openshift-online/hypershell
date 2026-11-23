package policyinterceptor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	pev1 "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/policyevents/v1"
)

// Verifies: PLAT-PGP-15.02
func TestEventBufferBoundedAndDropsCounted(t *testing.T) {
	m := NewMetrics()
	b := NewEventBuffer(3, m.DroppedEvents.Inc)
	for i := 0; i < 5; i++ {
		b.Add(Event{ID: string(rune('a' + i)), Outcome: OutcomeWouldReject})
	}
	if b.Len() != 3 {
		t.Fatalf("buffer holds %d events, want 3", b.Len())
	}
	if got := metricValue(t, m, "hypershell_policy_interceptor_dropped_events_total"); got != 2 {
		t.Fatalf("dropped counter %v, want 2", got)
	}
	events := b.peek(10)
	if events[0].ID != "c" || events[2].ID != "e" {
		t.Fatalf("oldest events were not the ones dropped: %+v", events)
	}
}

type fakeEventClient struct {
	mu       sync.Mutex
	fail     bool
	requests []*pev1.ReportPolicyEventsRequest
	auth     []string
}

func (f *fakeEventClient) ReportPolicyEvents(ctx context.Context, in *pev1.ReportPolicyEventsRequest, _ ...grpc.CallOption) (*pev1.ReportPolicyEventsResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	md, _ := metadata.FromOutgoingContext(ctx)
	f.auth = append(f.auth, strings.Join(md.Get("authorization"), ","))
	if f.fail {
		return nil, errors.New("control plane unavailable")
	}
	f.requests = append(f.requests, in)
	return &pev1.ReportPolicyEventsResponse{Accepted: int32(len(in.GetEvents()))}, nil
}

// Verifies: PLAT-PGP-15.01
func TestReporterSendsBatchesHeartbeatsAndRetries(t *testing.T) {
	tokenFile := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(tokenFile, []byte("tok-1\n"), 0o600); err != nil {
		t.Fatalf("write token: %v", err)
	}
	client := &fakeEventClient{fail: true}
	buffer := NewEventBuffer(10, nil)
	revision := "rev-1"
	now := time.Unix(1700000000, 0)
	r := &Reporter{
		Client: client, Buffer: buffer, GatewayID: testGatewayID, TokenFile: tokenFile,
		Revision: func() string { return revision }, Metrics: NewMetrics(),
		HeartbeatInterval: 30 * time.Second,
	}
	r.now = func() time.Time { return now }
	ctx := context.Background()

	buffer.Add(Event{ID: eventID(testGatewayID, "CreateSandbox", OutcomeWouldReject, revision, "req-1"), Outcome: OutcomeWouldReject})
	// Same OpenShell request_id (mutation replay): same idempotency key.
	if a, b := eventID(testGatewayID, "CreateSandbox", OutcomeWouldReject, revision, "req-1"),
		eventID(testGatewayID, "CreateSandbox", OutcomeWouldReject, revision, "req-1"); a != b {
		t.Fatalf("replayed request ids differ: %s vs %s", a, b)
	}
	firstRandom := eventID(testGatewayID, "x", "y", "z", "")
	if firstRandom == eventID(testGatewayID, "x", "y", "z", "") || len(firstRandom) != 32 {
		t.Fatalf("random ids collided")
	}

	// Control plane down: the event stays buffered and the reporter backs off.
	r.tick(ctx)
	if buffer.Len() != 1 || metricValue(t, r.Metrics, "hypershell_policy_interceptor_report_failures_total") != 1 {
		t.Fatalf("failed send should keep the event and count a failure")
	}
	r.tick(ctx) // within backoff: no call
	if len(client.auth) != 1 {
		t.Fatalf("reporter did not back off: %d calls", len(client.auth))
	}

	// Control plane back after the backoff: the batch and revision are sent
	// with the fresh token.
	client.fail = false
	if err := os.WriteFile(tokenFile, []byte("tok-2"), 0o600); err != nil {
		t.Fatalf("rotate token: %v", err)
	}
	now = now.Add(2 * time.Second)
	r.tick(ctx)
	if buffer.Len() != 0 || len(client.requests) != 1 {
		t.Fatalf("batch not sent: buffered=%d requests=%d", buffer.Len(), len(client.requests))
	}
	got := client.requests[0]
	if got.GetGatewayId() != testGatewayID || got.GetAppliedRevision() != "rev-1" || len(got.GetEvents()) != 1 {
		t.Fatalf("unexpected request %+v", got)
	}
	if client.auth[len(client.auth)-1] != "Bearer tok-2" {
		t.Fatalf("token not re-read: %q", client.auth[len(client.auth)-1])
	}

	// No events, no revision change, heartbeat not due: nothing sent.
	now = now.Add(time.Second)
	r.tick(ctx)
	if len(client.requests) != 1 {
		t.Fatalf("unexpected heartbeat")
	}
	// A new applied revision is reported immediately even with no events.
	revision = "rev-2"
	r.tick(ctx)
	if len(client.requests) != 2 || client.requests[1].GetAppliedRevision() != "rev-2" || len(client.requests[1].GetEvents()) != 0 {
		t.Fatalf("revision heartbeat not sent: %+v", client.requests)
	}
	// Periodic heartbeat.
	now = now.Add(31 * time.Second)
	r.tick(ctx)
	if len(client.requests) != 3 {
		t.Fatalf("periodic heartbeat not sent")
	}
}
