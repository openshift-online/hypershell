package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Verifies: SDLC-LOC-12
func TestSettingsPersistAndApplyWithoutChangingActiveJob(t *testing.T) {
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	prefs := filepath.Join(t.TempDir(), "preferences.json")
	t.Setenv("FULLSEND_CONFIG_FILE", prefs)
	t.Setenv("FULLSEND_RUNTIME_SETTINGS", "")
	t.Setenv("PATH", "/usr/bin:/bin:"+os.Getenv("PATH"))
	a := Adapter{Path: filepath.Join(root, ".hypershell/execute-sdlc/dispatch_adapter.py"), Args: []string{"agent/scope/test", "--dry-run", "--fullsend-dir", filepath.Join(root, ".fullsend"), "--target-repo", root, "--state-dir", t.TempDir()}}
	c, err := loadConfig(context.Background(), a)
	if err != nil {
		t.Fatal(err)
	}
	s := testStore(t)
	e, err := newEngine(c, a, s)
	if err != nil {
		t.Fatal(err)
	}
	defer e.diagnostic.Close()
	j := testJob()
	j.Command = []string{"fullsend", "run", "triage", "--model", "old-model"}
	if _, err = s.start(j, "log"); err != nil {
		t.Fatal(err)
	}
	result := e.saveSetting(context.Background(), "concurrency", "1")
	if result.err != nil {
		t.Fatal(result.err)
	}
	if e.config().Concurrency != 1 {
		t.Fatal("worker limit not applied live")
	}
	rows, err := s.rows()
	if err != nil || rows[0].Status != "running" {
		t.Fatalf("existing job changed: %v", err)
	}
	result = e.saveSetting(context.Background(), "model", "next-model")
	if result.err != nil {
		t.Fatal(result.err)
	}
	if e.config().Model != "next-model" || !strings.Contains(strings.Join(rows[0].Command, " "), "old-model") {
		t.Fatal("model timing is incorrect")
	}
	result = e.saveSetting(context.Background(), "poll_interval", "10000000")
	if result.err != nil || e.config().Poll != 10000000 {
		t.Fatalf("large integer changed representation: %v", result.err)
	}
	result = e.saveSetting(context.Background(), "auto_merge", "disabled")
	if result.err != nil {
		t.Fatal(result.err)
	}
	if value := e.config().Environment["FULLSEND_AUTO_MERGE"]; value == nil || *value != "disabled" {
		t.Fatal("merge policy not applied to new workers")
	}
	old := e.config().Repo
	result = e.saveSetting(context.Background(), "repo", "other/repository")
	if result.err != nil {
		t.Fatal(result.err)
	}
	if e.config().Repo != old {
		t.Fatal("restart-only change altered current repository")
	}
	// Helpers stay pinned to the current repository despite the saved restart value.
	active, err := loadConfig(context.Background(), e.adapter())
	if err != nil {
		t.Fatal(err)
	}
	if active.Repo != old || active.Concurrency != 1 {
		t.Fatalf("helper pins lost: %+v", active)
	}
	data, err := os.ReadFile(prefs)
	if err != nil {
		t.Fatal(err)
	}
	var saved struct {
		Values map[string]any `json:"values"`
	}
	if err = json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Values["repo"] != "other/repository" || saved.Values["concurrency"] != float64(1) {
		t.Fatal(string(data))
	}
	result = e.saveSetting(context.Background(), "concurrency", "0")
	if result.err == nil || e.config().Concurrency != 1 {
		t.Fatal("invalid update changed active configuration")
	}
	after, _ := os.ReadFile(prefs)
	if string(after) != string(data) {
		t.Fatal("invalid update changed disk")
	}
}

// Verifies: SDLC-LOC-09, SDLC-LOC-12
func TestConfigurationTabEditingDisclosureAndTerminalFrame(t *testing.T) {
	m := sampleModel()
	defer m.cancel()
	m.engine.c.SettingsFile = "/home/test/.config/hypershell/fullsend.json"
	m.engine.c.Settings = []Setting{{Key: "concurrency", Label: "Worker limit", Value: "10", Kind: "number", Effect: "Immediate"},
		{Key: "repo", Label: "Repository", Value: "test/repo", Kind: "repo", Effect: "Restart required"},
		{Key: "GH_TOKEN", Label: "GitHub credentials", Value: "Configured (hidden)", Kind: "readonly", Effect: "Read only"}}
	update := func(key tea.KeyMsg) { next, _ := m.Update(key); m = next.(Model) }
	update(tea.KeyMsg{Type: tea.KeyTab})
	update(tea.KeyMsg{Type: tea.KeyTab})
	if m.screen != "configuration" {
		t.Fatal("third tab is missing")
	}
	for _, size := range [][2]int{{100, 28}, {42, 24}} {
		m.width, m.height = size[0], size[1]
		view := m.View()
		if !strings.Contains(view, "SETTINGS FILE") || !strings.Contains(view, "disk") {
			t.Fatal(view)
		}
		if lipgloss.Height(view) > m.height {
			t.Fatal("configuration frame overflows")
		}
		for _, line := range strings.Split(view, "\n") {
			if lipgloss.Width(line) > m.width {
				t.Fatal("configuration line overflows")
			}
		}
	}
	update(tea.KeyMsg{Type: tea.KeyEnter})
	if !m.configEditing {
		t.Fatal("editor did not open")
	}
	update(tea.KeyMsg{Type: tea.KeyCtrlU})
	update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q?")})
	if m.ctx.Err() != nil || m.helpOpen || m.configInput != "q?" {
		t.Fatal("work shortcuts intercepted text input")
	}
	update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.configEditing {
		t.Fatal("discard did not close editor")
	}
	update(tea.KeyMsg{Type: tea.KeyTab})
	if m.screen != "" || m.issueGrouping {
		t.Fatal("tab did not return to agents")
	}
}

// Verifies: SDLC-LOC-12
func TestConfigurationReturnClampsHistorySelection(t *testing.T) {
	m := sampleModel()
	defer m.cancel()
	m.engine.c.Settings = []Setting{{Key: "concurrency", Value: "2", Kind: "number", Effect: "Immediate"}}
	issue := m.state.Rows[0]
	m.historyIssue = &issue
	for i := 0; i < 20; i++ {
		m.historyRows = append(m.historyRows, issue)
	}
	m.index = 19
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(",")})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = updated.(Model)
	if m.historyIssue != nil || m.index >= len(m.visibleRows()) {
		t.Fatal("configuration retained stale history selection")
	}
	_ = m.View()
}
