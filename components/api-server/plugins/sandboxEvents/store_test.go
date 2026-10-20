package sandboxEvents

import (
	"testing"
	"time"
)

func TestMemoryStoreIsolationCapExpiryAndEviction(t *testing.T) {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	store := NewMemoryStore(time.Minute, 5, 2)
	defer store.Close()
	store.now = func() time.Time { return now }
	events := make([]Event, 7)
	if !store.Replace("ns-a", "writer", events) {
		t.Fatal("replace failed")
	}
	store.Replace("ns-b", "writer", []Event{{Reason: "other"}})
	got, ok := store.Read("ns-a", "writer")
	if !ok || len(got.Events) != 5 {
		t.Fatalf("key isolation or cap failed: %#v, %v", got, ok)
	}
	if listed := store.List("ns-b"); len(listed) != 1 || listed[0].Events[0].Reason != "other" {
		t.Fatalf("namespace list = %#v", listed)
	}
	store.Replace("ns-c", "third", nil)
	if _, ok := store.Read("ns-a", "writer"); ok {
		t.Fatal("least recently refreshed entry should be evicted")
	}
	now = now.Add(2 * time.Minute)
	store.SweepExpired()
	if listed := store.List("ns-b"); len(listed) != 0 {
		t.Fatalf("expired snapshots = %#v", listed)
	}
}

func TestMemoryStoreReplaceClearsSnapshot(t *testing.T) {
	store := NewMemoryStore(time.Minute, 10, 10)
	defer store.Close()
	store.Replace("ns", "sandbox", []Event{{Reason: "old"}})
	store.Replace("ns", "sandbox", nil)
	got, ok := store.Read("ns", "sandbox")
	if !ok || len(got.Events) != 0 {
		t.Fatalf("empty replacement = %#v, %v", got, ok)
	}
}
