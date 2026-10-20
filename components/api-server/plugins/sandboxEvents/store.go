package sandboxEvents

import (
	"sort"
	"sync"
	"time"
)

const (
	DefaultEventLimit = 10
	DefaultTTL        = 10 * time.Minute
	DefaultMaxEntries = 1000
)

// Event is the bounded diagnostic payload shared by the gRPC writer and REST reader.
type Event struct {
	Type            string
	Reason          string
	Message         string
	FirstObservedAt *time.Time
	LastObservedAt  *time.Time
	Count           int32
	InvolvedPodName string
	InvolvedPodUID  string
}

// Snapshot is one sandbox's complete event set at a single observation time.
type Snapshot struct {
	Namespace  string
	Sandbox    string
	Events     []Event
	ObservedAt time.Time
}

// SandboxEventStore keeps snapshots keyed by gateway namespace and sandbox name.
// Implementations must treat Replace as an atomic full-snapshot replacement.
type SandboxEventStore interface {
	Replace(namespace, sandbox string, events []Event) bool
	Read(namespace, sandbox string) (Snapshot, bool)
	List(namespace string) []Snapshot
}

type key struct{ namespace, sandbox string }
type entry struct {
	snapshot    Snapshot
	expiresAt   time.Time
	refreshedAt time.Time
}

// MemoryStore is the default process-local, TTL-bound store.
type MemoryStore struct {
	mu         sync.Mutex
	ttl        time.Duration
	limit      int
	maxEntries int
	now        func() time.Time
	entries    map[key]entry
	stopSweep  chan struct{}
	closeOnce  sync.Once
}

func NewMemoryStore(ttl time.Duration, limit, maxEntries int) *MemoryStore {
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	if limit < 5 || limit > 10 {
		limit = DefaultEventLimit
	}
	if maxEntries <= 0 {
		maxEntries = DefaultMaxEntries
	}
	store := &MemoryStore{ttl: ttl, limit: limit, maxEntries: maxEntries, now: time.Now, entries: make(map[key]entry), stopSweep: make(chan struct{})}
	interval := ttl / 2
	if interval > time.Minute {
		interval = time.Minute
	}
	if interval < time.Second {
		interval = time.Second
	}
	go store.sweepPeriodically(interval)
	return store
}

func (s *MemoryStore) sweepPeriodically(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.SweepExpired()
		case <-s.stopSweep:
			return
		}
	}
}

// Close stops the store's background expiry sweeper. It is useful for callers
// that explicitly own a short-lived store, such as focused tests.
func (s *MemoryStore) Close() { s.closeOnce.Do(func() { close(s.stopSweep) }) }

func (s *MemoryStore) Replace(namespace, sandbox string, events []Event) bool {
	if namespace == "" || sandbox == "" {
		return false
	}
	now := s.now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweepLocked(now)
	k := key{namespace, sandbox}
	if _, exists := s.entries[k]; !exists && len(s.entries) >= s.maxEntries {
		s.evictOneLocked()
	}
	if len(events) > s.limit {
		events = events[:s.limit]
	}
	s.entries[k] = entry{snapshot: Snapshot{Namespace: namespace, Sandbox: sandbox, Events: cloneEvents(events), ObservedAt: now}, expiresAt: now.Add(s.ttl), refreshedAt: now}
	return true
}

func (s *MemoryStore) Read(namespace, sandbox string) (Snapshot, bool) {
	now := s.now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweepLocked(now)
	value, ok := s.entries[key{namespace, sandbox}]
	if !ok {
		return Snapshot{}, false
	}
	return cloneSnapshot(value.snapshot), true
}

func (s *MemoryStore) List(namespace string) []Snapshot {
	now := s.now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweepLocked(now)
	out := make([]Snapshot, 0)
	for k, value := range s.entries {
		if k.namespace == namespace {
			out = append(out, cloneSnapshot(value.snapshot))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Sandbox < out[j].Sandbox })
	return out
}

// SweepExpired permits a periodic maintenance caller and is safe to call at any time.
func (s *MemoryStore) SweepExpired() { s.mu.Lock(); defer s.mu.Unlock(); s.sweepLocked(s.now().UTC()) }
func (s *MemoryStore) sweepLocked(now time.Time) {
	for k, value := range s.entries {
		if !value.expiresAt.After(now) {
			delete(s.entries, k)
		}
	}
}
func (s *MemoryStore) evictOneLocked() {
	var victim key
	var oldest time.Time
	for k, value := range s.entries {
		if oldest.IsZero() || value.refreshedAt.Before(oldest) {
			victim, oldest = k, value.refreshedAt
		}
	}
	if !oldest.IsZero() {
		delete(s.entries, victim)
	}
}
func cloneSnapshot(s Snapshot) Snapshot { s.Events = cloneEvents(s.Events); return s }
func cloneEvents(in []Event) []Event {
	out := make([]Event, len(in))
	copy(out, in)
	for i := range out {
		if in[i].FirstObservedAt != nil {
			t := *in[i].FirstObservedAt
			out[i].FirstObservedAt = &t
		}
		if in[i].LastObservedAt != nil {
			t := *in[i].LastObservedAt
			out[i].LastObservedAt = &t
		}
	}
	return out
}
