package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Verifies: SDLC-LOC-11
func TestCancellationWorkerProcess(t *testing.T) {
	if os.Getenv("FUNNEL_TEST_CANCEL_WORKER") != "1" {
		return
	}
	for i, arg := range os.Args {
		if arg == "--" {
			os.Exit(workerMain(os.Args[i+1:]))
		}
	}
	os.Exit(127)
}

func awaitCancelTest(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("timed out waiting for cancellation worker")
}

// Verifies: SDLC-LOC-04, SDLC-LOC-11
func TestCancelDetachedWorkerWaitsForCleanupAndSurvivesReconnect(t *testing.T) {
	s := testStore(t)
	dir := t.TempDir()
	ready, stopping, release := filepath.Join(dir, "ready"), filepath.Join(dir, "stopping"), filepath.Join(dir, "release")
	script := filepath.Join(dir, "worker.sh")
	// The fake Fullsend exits successfully after handling TERM, just as a tool can
	// finish cleanup successfully without producing a successful agent verdict.
	body := "#!/bin/sh\ntrap 'touch \"$2\"; while [ ! -f \"$3\" ]; do sleep 0.02; done; exit 0' TERM\ntouch \"$1\"\nwhile :; do sleep 0.02; done\n"
	if err := os.WriteFile(script, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	j := testJob()
	j.Command = []string{"sh", script, ready, stopping, release}
	id, err := s.start(j, filepath.Join(dir, "run.log"))
	if err != nil {
		t.Fatal(err)
	}
	lock, err := s.runLock(id)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestCancellationWorkerProcess$", "--", s.path, j.Key, id, "1", "10", "unused", dir)
	cmd.Env = append(os.Environ(), "FUNNEL_TEST_CANCEL_WORKER=1")
	cmd.ExtraFiles = []*os.File{lock}
	if err = cmd.Start(); err != nil {
		lock.Close()
		t.Fatal(err)
	}
	lock.Close()
	finished := make(chan error, 1)
	go func() { finished <- cmd.Wait() }()
	t.Cleanup(func() { _ = os.WriteFile(release, nil, 0600); _ = cmd.Process.Kill() })
	awaitCancelTest(t, func() bool { _, err := os.Stat(ready); return err == nil })
	// A different connection stands in for a restarted or second dashboard.
	resumed, err := openStore(s.path)
	if err != nil {
		t.Fatal(err)
	}
	defer resumed.db.Close()
	if err = resumed.requestCancel(j.Key, "other-scope", id); err == nil {
		t.Fatal("cross-scope request accepted")
	}
	if err = resumed.requestCancel(j.Key, j.Scope, "stale-id"); err == nil {
		t.Fatal("stale request accepted")
	}
	if err = resumed.requestCancel(j.Key, j.Scope, id); err != nil {
		t.Fatal(err)
	}
	awaitCancelTest(t, func() bool { _, err := os.Stat(stopping); return err == nil })
	rows, err := resumed.rows()
	if err != nil {
		t.Fatal(err)
	}
	if rows[0].Status != "running" || !rows[0].CancelRequested {
		t.Fatalf("released while cleaning up: %+v", rows[0])
	}
	if alive, err := resumed.alive(id); err != nil || !alive {
		t.Fatalf("cleanup lost ownership: %v %v", alive, err)
	}
	if err = resumed.retryFailed(j.Key, j.Scope); err == nil {
		t.Fatal("retry accepted during cleanup")
	}
	if eligible(j, rows, seconds()) {
		t.Fatal("duplicate work eligible during cleanup")
	}
	if err = os.WriteFile(release, nil, 0600); err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-finished:
		if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 130 {
			t.Fatalf("expected graceful cancellation exit 130, got %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("worker never finished cleanup")
	}
	rows, err = resumed.rows()
	if err != nil {
		t.Fatal(err)
	}
	if rows[0].Status != "canceled" || rows[0].Finished == 0 || rows[0].Failures != 0 {
		t.Fatalf("incorrect saved outcome: %+v", rows[0])
	}
	changed := j
	changed.Fingerprint = "new input"
	if eligible(changed, rows, seconds()+100000) {
		t.Fatal("canceled issue auto-retried")
	}
	history, err := resumed.history(rows[0])
	if err != nil || len(history) != 1 || history[0].Status != "canceled" {
		t.Fatalf("history lost cancellation: %+v %v", history, err)
	}
	if err = resumed.requestCancel(j.Key, j.Scope, id); err == nil {
		t.Fatal("completed run accepted cancel")
	}
	if err = resumed.retryFailed(j.Key, j.Scope); err != nil {
		t.Fatal(err)
	}
	rows, err = resumed.rows()
	if err != nil || !eligible(j, rows, seconds()) {
		t.Fatalf("manual retry unavailable: %v", err)
	}
}

// Verifies: SDLC-LOC-11
func TestLegacyWorkerDoesNotAcceptCancel(t *testing.T) {
	s := testStore(t)
	j := testJob()
	id, err := s.start(j, "log")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.requestCancel(j.Key, j.Scope, id); err == nil || !strings.Contains(err.Error(), "predates") {
		t.Fatalf("legacy cancellation: %v", err)
	}
	rows, err := s.rows()
	if err != nil || rows[0].Status != "running" {
		t.Fatalf("legacy outcome changed: %v", err)
	}
}

// Verifies: SDLC-LOC-09, SDLC-LOC-11
func TestCancelConfirmationDefaultsToKeepRunningAndPinsAttempt(t *testing.T) {
	m := sampleModel()
	defer m.cancel()
	s := testStore(t)
	m.engine.store = s
	j := m.state.Rows[0].Job
	id, err := s.start(j, "log")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec(`INSERT INTO worker_control(run_id) VALUES(?)`, id); err != nil {
		t.Fatal(err)
	}
	m.state.Rows[0].RunID = id
	key := func(k string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)} }
	next, cmd := m.Update(key("C"))
	m = next.(Model)
	if cmd != nil || m.pendingCancel == nil || m.cancelChoice != 0 {
		t.Fatal("cancel did not require confirmation")
	}
	view := m.View()
	if !strings.Contains(view, "Keep running") || !strings.Contains(view, "Cancel run?") {
		t.Fatal(view)
	}
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if cmd != nil || m.pendingCancel != nil {
		t.Fatal("default submitted cancellation")
	}
	next, _ = m.Update(key("C"))
	m = next.(Model)
	next, _ = m.Update(key("j"))
	m = next.(Model)
	if m.cancelChoice != 1 {
		t.Fatal("arrow/vim selection unavailable")
	}
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("confirmed cancel missing")
	}
	// A replacement attempt cannot be interrupted by an old confirmation.
	if _, err = s.start(j, "replacement"); err != nil {
		t.Fatal(err)
	}
	result := cmd().(cancelDone)
	if result.err == nil {
		t.Fatal("stale confirmation canceled replacement")
	}
}

// Verifies: SDLC-LOC-09, SDLC-LOC-11
func TestCancelFrameFitsTerminal(t *testing.T) {
	for _, size := range [][2]int{{42, 16}, {100, 24}, {120, 32}} {
		m := sampleModel()
		m.width, m.height = size[0], size[1]
		row := m.state.Rows[0]
		m.pendingCancel = &row
		view := m.View()
		for _, line := range strings.Split(view, "\n") {
			if lipgloss.Width(line) > m.width {
				t.Fatalf("confirmation overflows %dx%d", m.width, m.height)
			}
		}
		if lipgloss.Height(view) > m.height {
			t.Fatalf("confirmation height overflows %dx%d", m.width, m.height)
		}
		m.cancel()
	}
}
