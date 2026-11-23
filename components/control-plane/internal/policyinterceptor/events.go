package policyinterceptor

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/timestamppb"

	pev1 "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/policyevents/v1"
)

// Audit finding outcomes (PLAT-PGP-15, PLAT-DATA-30).
const (
	OutcomeWouldReject      = "would_reject"
	OutcomeWouldAddDefaults = "would_add_defaults"
)

// Event is one audit finding. It carries normalized resource values only and
// never denial text (PLAT-PGP-15).
type Event struct {
	ID               string
	OccurredAt       time.Time
	PrincipalSubject string
	Operation        string
	Outcome          string
	PolicyNames      []string
	BundleRevision   string
	RequestSummary   map[string]int64

	seq uint64
}

func (e Event) toProto() *pev1.PolicyEvent {
	return &pev1.PolicyEvent{
		Id:               e.ID,
		OccurredAt:       timestamppb.New(e.OccurredAt),
		PrincipalSubject: e.PrincipalSubject,
		Operation:        e.Operation,
		Outcome:          e.Outcome,
		PolicyNames:      append([]string(nil), e.PolicyNames...),
		BundleRevision:   e.BundleRevision,
		RequestSummary:   e.RequestSummary,
	}
}

// eventID returns a sidecar-generated idempotency key. When the operation
// carries an OpenShell request_id, the id is derived from it so a mutation
// replay of the same request reports the same finding id; otherwise it is a
// random 128-bit value.
func eventID(gatewayID, operation, outcome, revision, requestID string) string {
	if requestID != "" {
		sum := sha256.Sum256([]byte(strings.Join([]string{gatewayID, operation, outcome, revision, requestID}, "\x00")))
		return hex.EncodeToString(sum[:16])
	}
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand.Read does not fail on supported platforms; fall back to a
		// time-derived id rather than dropping the finding.
		sum := sha256.Sum256(fmt.Appendf(nil, "%s\x00%d", gatewayID, time.Now().UnixNano()))
		return hex.EncodeToString(sum[:16])
	}
	return hex.EncodeToString(b[:])
}

// EventBuffer is a bounded in-memory FIFO. When full, Add drops the oldest
// event and counts it (PLAT-PGP-15.02). Add never blocks on the network.
type EventBuffer struct {
	mu        sync.Mutex
	events    []Event
	capacity  int
	threshold int
	nextSeq   uint64
	dropped   func()
	flush     chan struct{}
}

// NewEventBuffer returns a buffer holding at most capacity events. onDrop is
// called once per dropped event.
func NewEventBuffer(capacity int, onDrop func()) *EventBuffer {
	if capacity <= 0 {
		capacity = DefaultEventBuffer
	}
	threshold := capacity / 2
	if threshold < 1 {
		threshold = 1
	}
	if onDrop == nil {
		onDrop = func() {}
	}
	return &EventBuffer{capacity: capacity, threshold: threshold, dropped: onDrop, flush: make(chan struct{}, 1)}
}

// Add appends an event, dropping the oldest when the buffer is full, and
// signals the reporter when the buffer passes the flush threshold.
func (b *EventBuffer) Add(e Event) {
	b.mu.Lock()
	b.nextSeq++
	e.seq = b.nextSeq
	if len(b.events) >= b.capacity {
		b.events = b.events[1:]
		b.dropped()
	}
	b.events = append(b.events, e)
	over := len(b.events) >= b.threshold
	b.mu.Unlock()
	if over {
		select {
		case b.flush <- struct{}{}:
		default:
		}
	}
}

// Len returns the number of buffered events.
func (b *EventBuffer) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.events)
}

// peek returns up to limit of the oldest events without removing them.
func (b *EventBuffer) peek(limit int) []Event {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := len(b.events)
	if n > limit {
		n = limit
	}
	return append([]Event(nil), b.events[:n]...)
}

// ack removes every event with a sequence number up to and including seq.
// Events dropped meanwhile are already gone, so acknowledging by sequence (not
// by count) never removes an unsent event.
func (b *EventBuffer) ack(seq uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	i := 0
	for i < len(b.events) && b.events[i].seq <= seq {
		i++
	}
	b.events = b.events[i:]
}

// PolicyEventClient is the subset of the generated client the reporter uses.
type PolicyEventClient interface {
	ReportPolicyEvents(ctx context.Context, in *pev1.ReportPolicyEventsRequest, opts ...grpc.CallOption) (*pev1.ReportPolicyEventsResponse, error)
}

// Reporter sends buffered events and the applied bundle revision to the
// control plane's ReportPolicyEvents endpoint (CTRL-CORE-22). It sends a batch
// every Interval, immediately when the buffer passes its threshold, and a
// heartbeat carrying the applied revision when the revision changed or
// HeartbeatInterval elapsed, so the controller can mark the bundle Applied with
// no findings. Failures back off exponentially; Evaluate is never blocked.
type Reporter struct {
	Client            PolicyEventClient
	Buffer            *EventBuffer
	GatewayID         string
	TokenFile         string
	Revision          func() string
	Metrics           *Metrics
	Log               *slog.Logger
	Interval          time.Duration
	HeartbeatInterval time.Duration
	MaxBackoff        time.Duration
	MaxBatch          int
	CallTimeout       time.Duration

	lastSentRevision string
	lastSent         time.Time
	sentOnce         bool
	failures         int
	nextAttempt      time.Time
	now              func() time.Time
}

func (r *Reporter) defaults() {
	if r.Interval <= 0 {
		r.Interval = 5 * time.Second
	}
	if r.HeartbeatInterval <= 0 {
		r.HeartbeatInterval = 30 * time.Second
	}
	if r.MaxBackoff <= 0 {
		r.MaxBackoff = time.Minute
	}
	if r.MaxBatch <= 0 {
		r.MaxBatch = 500
	}
	if r.CallTimeout <= 0 {
		r.CallTimeout = 10 * time.Second
	}
	if r.Log == nil {
		r.Log = slog.Default()
	}
	if r.now == nil {
		r.now = time.Now
	}
}

// Run reports until ctx is done, then makes one best-effort final flush.
func (r *Reporter) Run(ctx context.Context) {
	r.defaults()
	ticker := time.NewTicker(r.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			final, cancel := context.WithTimeout(context.Background(), r.CallTimeout)
			r.nextAttempt = time.Time{}
			r.tick(final)
			cancel()
			return
		case <-ticker.C:
			r.tick(ctx)
		case <-r.Buffer.flush:
			r.tick(ctx)
		}
	}
}

// tick sends at most one batch.
func (r *Reporter) tick(ctx context.Context) {
	r.defaults()
	now := r.now()
	if now.Before(r.nextAttempt) {
		return
	}
	revision := r.Revision()
	batch := r.Buffer.peek(r.MaxBatch)
	heartbeatDue := !r.sentOnce || revision != r.lastSentRevision || now.Sub(r.lastSent) >= r.HeartbeatInterval
	if len(batch) == 0 && !heartbeatDue {
		return
	}
	if err := r.send(ctx, revision, batch); err != nil {
		r.failures++
		backoff := time.Second << min(r.failures-1, 6)
		if backoff > r.MaxBackoff {
			backoff = r.MaxBackoff
		}
		r.nextAttempt = now.Add(backoff)
		if r.Metrics != nil {
			r.Metrics.ReportFailures.Inc()
		}
		r.Log.WarnContext(ctx, "report policy events failed; retrying with backoff",
			"error", err, "buffered", r.Buffer.Len(), "backoff", backoff.String())
		return
	}
	r.failures = 0
	r.nextAttempt = time.Time{}
	r.sentOnce = true
	r.lastSent = now
	r.lastSentRevision = revision
	if len(batch) > 0 {
		r.Buffer.ack(batch[len(batch)-1].seq)
	}
}

func (r *Reporter) send(ctx context.Context, revision string, batch []Event) error {
	token, err := readToken(r.TokenFile)
	if err != nil {
		return err
	}
	req := &pev1.ReportPolicyEventsRequest{GatewayId: r.GatewayID, AppliedRevision: revision}
	for _, e := range batch {
		req.Events = append(req.Events, e.toProto())
	}
	callCtx, cancel := context.WithTimeout(ctx, r.CallTimeout)
	defer cancel()
	callCtx = metadata.AppendToOutgoingContext(callCtx, "authorization", "Bearer "+token)
	if _, err := r.Client.ReportPolicyEvents(callCtx, req); err != nil {
		return fmt.Errorf("ReportPolicyEvents: %w", err)
	}
	return nil
}

// readToken reads the projected service-account token fresh on every call
// because the kubelet rotates it.
func readToken(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read events token: %w", err)
	}
	token := strings.TrimSpace(string(raw))
	if token == "" {
		return "", errors.New("events token file is empty")
	}
	return token, nil
}

// Kick asks the reporter to report now, for example after a new bundle
// revision is applied, without waiting for the next interval.
func (b *EventBuffer) Kick() {
	select {
	case b.flush <- struct{}{}:
	default:
	}
}
