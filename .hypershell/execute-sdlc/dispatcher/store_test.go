package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	s, err := openStore(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.db.Close() })
	return s
}
func testJob() Job {
	return Job{Key: "1:triage", Scope: "agent/scope/test", Agent: "triage", Item: "1", Title: "Test issue", Fingerprint: "first"}
}
func TestPersistentBackoffAndSuccess(t *testing.T) {
	s := testStore(t)
	j := testJob()
	id, err := s.start(j, "/tmp/test.log")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.finish(j.Key, id, 7, 60, 3600); err != nil {
		t.Fatal(err)
	}
	rows, err := s.rows()
	if err != nil {
		t.Fatal(err)
	}
	if eligible(j, rows, seconds()) || rows[0].Failures != 1 {
		t.Fatal("failed job did not back off")
	}
	if !eligible(j, rows, seconds()+61) {
		t.Fatal("failed job never becomes eligible")
	}
	id, err = s.start(j, "/tmp/retry.log")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.finish(j.Key, id, 0, 60, 3600); err != nil {
		t.Fatal(err)
	}
	rows, _ = s.rows()
	if eligible(j, rows, seconds()+3600) {
		t.Fatal("successful unchanged job repeated")
	}
	j.Fingerprint = "changed"
	if !eligible(j, rows, seconds()+3600) {
		t.Fatal("changed issue not eligible")
	}
}
func TestWorkerLockPreservesAdoptionAndUnknownBlocksItem(t *testing.T) {
	s := testStore(t)
	j := testJob()
	id, err := s.start(j, "/tmp/worker.log")
	if err != nil {
		t.Fatal(err)
	}
	lock, err := s.runLock(id)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.recover(); err != nil {
		t.Fatal(err)
	}
	rows, _ := s.rows()
	if rows[0].Status != "running" {
		t.Fatal("surviving worker marked unknown")
	}
	lock.Close()
	if err = s.recover(); err != nil {
		t.Fatal(err)
	}
	rows, _ = s.rows()
	if rows[0].Status != "uncertain" {
		t.Fatal("missing worker not marked unknown")
	}
	j.Key = "1:other"
	if eligible(j, rows, seconds()) {
		t.Fatal("unknown outcome allowed another phase")
	}
}
func TestOldRunCannotOverwriteNewOwner(t *testing.T) {
	s := testStore(t)
	j := testJob()
	first, err := s.start(j, "first")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.start(j, "second"); err != nil {
		t.Fatal(err)
	}
	if err = s.finish(j.Key, first, 0, 60, 3600); err == nil {
		t.Fatal("stale completion accepted")
	}
}
func TestLegacyPythonHash(t *testing.T) {
	if got := hashString("openshift-online/hypershell")[:16]; got != "82fed93ce80fb065" {
		t.Fatalf("state directory changed: %s", got)
	}
}
func TestAuthTimerIndependentOfSlowDiscovery(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "openshell")
	count := filepath.Join(dir, "calls")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\necho check >> '"+count+"'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	s := testStore(t)
	e, err := newEngine(Config{LogDir: dir, AuthRetry: 1, AuthCheck: 1}, Adapter{}, s)
	if err != nil {
		t.Fatal(err)
	}
	defer e.diagnostic.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := make(chan authResult, 10)
	go e.authLoop(ctx, ch)
	for i := 0; i < 2; i++ {
		select {
		case result := <-ch:
			if result.Err != nil {
				t.Fatal(result.Err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("auth refresh blocked")
		}
	}
}
func TestJobPayloadKeepsLegacyNames(t *testing.T) {
	data, _ := json.Marshal(Job{PRNumber: "12", PreviousPhase: "agent/workable", SourceFingerprint: "stable-inputs"})
	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	if fields["pr_number"] != "12" || fields["previous_phase"] != "agent/workable" || fields["source_fingerprint"] != "stable-inputs" {
		t.Fatal("legacy payload contract changed")
	}
}

func TestManualRetryOnlyClearsBackoffForFailedJobsInScope(t *testing.T) {
	s := testStore(t)
	j := testJob()
	id, err := s.start(j, "log")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.retryFailed(j.Key, j.Scope); err == nil {
		t.Fatal("retried running job")
	}
	if err = s.finish(j.Key, id, 1, 3600, 3600); err != nil {
		t.Fatal(err)
	}
	if err = s.retryFailed(j.Key, "agent/scope/other"); err == nil {
		t.Fatal("retried another scope")
	}
	if err = s.retryFailed(j.Key, j.Scope); err != nil {
		t.Fatal(err)
	}
	rows, _ := s.rows()
	if rows[0].Next != 0 || rows[0].Failures != 1 || !eligible(j, rows, seconds()) {
		t.Fatal("retry did not preserve failure history and clear only delay")
	}
}

func TestIssueHistoryKeepsAttemptsAndOutcomes(t *testing.T) {
	s := testStore(t)
	j := testJob()
	first, err := s.start(j, "/tmp/first.log")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.finish(j.Key, first, 7, 1, 1); err != nil {
		t.Fatal(err)
	}
	second, err := s.start(j, "/tmp/second.log")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.finish(j.Key, second, 0, 1, 1); err != nil {
		t.Fatal(err)
	}
	other := j
	other.Key = "other"
	other.Scope = "another-scope"
	if _, err = s.start(other, "/tmp/not-this-issue.log"); err != nil {
		t.Fatal(err)
	}
	rows, err := s.history(Row{Job: j})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].Log != "/tmp/second.log" || rows[1].Log != "/tmp/first.log" {
		t.Fatalf("history: %+v", rows)
	}
	if !strings.HasPrefix(rowStatus(rows[0]), "Run completed · ") || rowStatus(rows[1]) != "Failed" {
		t.Fatalf("outcomes: %+v", rows)
	}
	if rows[0].Key == rows[1].Key {
		t.Fatal("attempt selection keys must differ")
	}
}

func TestConcurrentWorkerResultsAreRecorded(t *testing.T) {
	s := testStore(t)
	const workers = 12
	stores := make([]*Store, workers)
	jobs := make([]Job, workers)
	ids := make([]string, workers)
	for i := range workers {
		var err error
		stores[i], err = openStore(s.path)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { stores[i].db.Close() })
		jobs[i] = testJob()
		jobs[i].Key = fmt.Sprintf("%d:triage", i)
		jobs[i].Item = fmt.Sprint(i)
		ids[i], err = s.start(jobs[i], "/tmp/test.log")
		if err != nil {
			t.Fatal(err)
		}
	}
	start := make(chan struct{})
	results := make(chan error, workers)
	for i := range workers {
		go func() { <-start; results <- stores[i].finish(jobs[i].Key, ids[i], 1, 60, 60) }()
	}
	close(start)
	for range workers {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	rows, err := s.rows()
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.Status != "failed" || r.Failures != 1 {
			t.Fatalf("lost result: %+v", r)
		}
	}
	var count int
	if err = s.db.QueryRow(`SELECT count(*) FROM runs WHERE exit_code=1 AND finished IS NOT NULL`).Scan(&count); err != nil || count != workers {
		t.Fatalf("runs saved %d: %v", count, err)
	}
}

func TestBusyTimeoutSurvivesReplacementConnection(t *testing.T) {
	s := testStore(t)
	s.db.SetMaxIdleConns(0)
	var timeout int
	if err := s.db.QueryRow(`PRAGMA busy_timeout`).Scan(&timeout); err != nil || timeout != 30000 {
		t.Fatalf("timeout %d: %v", timeout, err)
	}
}

func TestResultSaveRetriesBusyWithoutDuplicatingAttempt(t *testing.T) {
	s := testStore(t)
	j := testJob()
	id, err := s.start(j, "/tmp/result.log")
	if err != nil {
		t.Fatal(err)
	}
	blocker, err := openStore(s.path)
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.db.Close()
	tx, err := blocker.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err = s.db.Exec(`PRAGMA busy_timeout=1`); err != nil {
		t.Fatal(err)
	}
	// Verify that the held writer produces the exact retryable driver error.
	if err = s.finish(j.Key, id, 7, 30, 30); !databaseBusy(err) {
		t.Fatalf("expected SQLite busy: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- s.finishReliably(j.Key, id, 7, 30, 30) }()
	time.Sleep(50 * time.Millisecond)
	if err = tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("save did not recover")
	}
	rows, err := s.rows()
	if err != nil {
		t.Fatal(err)
	}
	if rows[0].Status != "failed" || rows[0].Failures != 1 {
		t.Fatalf("incorrect saved outcome: %+v", rows[0])
	}
	var count int
	if err = s.db.QueryRow(`SELECT count(*) FROM runs`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("duplicated attempt: %d %v", count, err)
	}
}

func TestConfirmedUnknownRetryRejectsStaleAndLiveRuns(t *testing.T) {
	s := testStore(t)
	j := testJob()
	id, err := s.start(j, "/tmp/unknown.log")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec(`UPDATE jobs SET status='uncertain' WHERE key=?`, j.Key); err != nil {
		t.Fatal(err)
	}
	if err = s.retryConfirmedUnknown(j.Key, "another-scope", id); err == nil {
		t.Fatal("scope bypass")
	}
	if err = s.retryConfirmedUnknown(j.Key, j.Scope, "old-run"); err == nil {
		t.Fatal("stale confirmation accepted")
	}
	lock, err := s.runLock(id)
	if err != nil {
		t.Fatal(err)
	}
	err = s.retryConfirmedUnknown(j.Key, j.Scope, id)
	lock.Close()
	if err == nil {
		t.Fatal("live worker retried")
	}
	if err = s.retryConfirmedUnknown(j.Key, j.Scope, id); err != nil {
		t.Fatal(err)
	}
	rows, err := s.rows()
	if err != nil {
		t.Fatal(err)
	}
	if rows[0].Status != "failed" || rows[0].Next != 0 {
		t.Fatal(rows)
	}
	if err = s.retryConfirmedUnknown(j.Key, j.Scope, id); err == nil {
		t.Fatal("duplicate confirmation accepted")
	}
}

func TestLegacyLifecycleNamesPreserveOwnershipAndHistory(t *testing.T) {
	s := testStore(t)
	j := testJob()
	j.Key, j.Agent = "1:funnel-reconcile", "funnel-reconcile"
	id, err := s.start(j, "/tmp/legacy.log")
	if err != nil {
		t.Fatal(err)
	}
	rows, err := s.rows()
	if err != nil {
		t.Fatal(err)
	}
	current := j
	current.Agent = "reconcile"
	if rows[0].Agent != "reconcile" || rows[0].Key != j.Key || eligible(current, rows, seconds()) {
		t.Fatal("rename changed ownership or display", rows)
	}
	if err := s.finish(j.Key, id, 0, 60, 3600); err != nil {
		t.Fatal(err)
	}
	rows, _ = s.rows()
	if eligible(current, rows, seconds()) {
		t.Fatal("rename replayed successful job")
	}
	history, err := s.history(rows[0])
	if err != nil || len(history) != 1 || history[0].Agent != "reconcile" {
		t.Fatal(history, err)
	}
}
