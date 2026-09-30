// Package cache provides independent, per-source cached values with staleness
// tracking, matching data-architecture.spec §5.1 (cache/refresh) and §5.2
// (failure modes): each source refreshes on its own interval and, on an upstream
// error, serves its last-good value flagged stale so one dead source never
// blanks the dashboard.
package cache

import (
	"context"
	"sync"
	"time"
)

// Snapshot is the envelope every API payload is wrapped in.
type Snapshot struct {
	Data        any       `json:"data"`
	GeneratedAt time.Time `json:"generatedAt"`
	Stale       bool      `json:"stale"`
	Error       string    `json:"error,omitempty"`
}

// FetchFunc retrieves fresh data for a source.
type FetchFunc func(ctx context.Context) (any, error)

// Observer is notified after every refresh attempt (for /metrics).
type Observer func(source string, ok bool, dur time.Duration)

// Source is a single cached data plane with its own refresh loop.
type Source struct {
	name     string
	interval time.Duration
	fetch    FetchFunc
	observe  Observer

	mu   sync.RWMutex
	snap Snapshot
}

// NewSource creates a source. It has no data until the first successful refresh;
// until then it reports stale.
func NewSource(name string, interval time.Duration, fetch FetchFunc, observe Observer) *Source {
	return &Source{
		name:     name,
		interval: interval,
		fetch:    fetch,
		observe:  observe,
		snap:     Snapshot{Stale: true},
	}
}

// Name returns the source name.
func (s *Source) Name() string { return s.name }

// Get returns the current snapshot (last-good data, possibly flagged stale).
func (s *Source) Get() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.snap
}

// Age reports how long since the last successful refresh; zero if never.
func (s *Source) Age() time.Duration {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.snap.GeneratedAt.IsZero() {
		return 0
	}
	return time.Since(s.snap.GeneratedAt)
}

// refresh performs one fetch, updating the snapshot. On error it keeps the prior
// data and flags it stale.
func (s *Source) refresh(ctx context.Context) {
	start := time.Now()
	data, err := s.fetch(ctx)
	dur := time.Since(start)
	if s.observe != nil {
		s.observe(s.name, err == nil, dur)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		s.snap.Stale = true
		s.snap.Error = err.Error()
		return
	}
	s.snap = Snapshot{Data: data, GeneratedAt: time.Now(), Stale: false}
}

// Run refreshes once immediately, then on the source's interval until the
// context is cancelled.
func (s *Source) Run(ctx context.Context) {
	s.refresh(ctx)
	t := time.NewTicker(s.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.refresh(ctx)
		}
	}
}
