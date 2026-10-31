package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func press(m Model, key string) Model {
	keys := map[string]tea.KeyType{"enter": tea.KeyEnter, "esc": tea.KeyEsc, "ctrl+d": tea.KeyCtrlD, "ctrl+u": tea.KeyCtrlU, "ctrl+f": tea.KeyCtrlF, "ctrl+b": tea.KeyCtrlB, "backspace": tea.KeyBackspace}
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	if kind, ok := keys[key]; ok {
		msg = tea.KeyMsg{Type: kind}
	}
	updated, _ := m.Update(msg)
	return updated.(Model)
}
func TestLiveFilterEditingAndHistoryIsolation(t *testing.T) {
	m := sampleModel()
	defer m.cancel()
	m = press(m, "/")
	m = press(m, "42 code-review")
	if len(m.visibleRows()) != 1 || m.visibleRows()[0].Item != "42" {
		t.Fatal("filter did not match issue and agent")
	}
	m = press(m, "enter")
	m = press(m, "/")
	m = press(m, "q")
	if len(m.visibleRows()) != 0 || m.ctx.Err() != nil {
		t.Fatal("filter input invoked shortcut")
	}
	m = press(m, "esc")
	if m.filter != "42 code-review" {
		t.Fatal("cancel lost prior filter")
	}
	m = press(m, "esc")
	if m.filter != "" || len(m.visibleRows()) != 2 {
		t.Fatal("applied filter not cleared")
	}
	m = press(m, "/")
	m = press(m, "界")
	m = press(m, "backspace")
	if m.filter != "" {
		t.Fatal("backspace split unicode")
	}
	m = press(m, "esc")
	r := m.state.Rows[1]
	m.historyIssue = &r
	m.historyRows = []Row{r}
	m.historyFilter = "missing"
	if len(m.visibleRows()) != 0 || m.filter != "" {
		t.Fatal("history filter not isolated")
	}
}
func TestNavigationAcrossViewsAndTextEntry(t *testing.T) {
	m := sampleModel()
	defer m.cancel()
	m = press(m, "G")
	if m.index != 1 {
		t.Fatal("G did not select last")
	}
	m = press(m, "g")
	if m.screen == "signin" {
		t.Fatal("g opened sign-in")
	}
	m = press(m, "g")
	if m.index != 0 {
		t.Fatal("gg did not select first")
	}
	m = press(m, "ctrl+d")
	if m.index != 1 {
		t.Fatal("half page did not move")
	}
	m.screen = "logs"
	m.logText = strings.Repeat("log\n", 100)
	m = press(m, "g")
	m = press(m, "g")
	if m.scroll == 0 {
		t.Fatal("gg did not scroll logs to top")
	}
	m = press(m, "G")
	if m.scroll != 0 {
		t.Fatal("G did not scroll logs to bottom")
	}
	m = press(m, "h")
	if m.screen != "" {
		t.Fatal("h did not go back")
	}
	m.screen = "signin"
	m.signInActive = true
	m.signInPrompt = &gatewayPrompt{Choices: []gatewayChoice{{Title: "one"}, {Title: "two"}}}
	m = press(m, "j")
	if m.signInChoice != 1 {
		t.Fatal("j did not navigate menu")
	}
	m = press(m, "k")
	if m.signInChoice != 0 {
		t.Fatal("k did not navigate menu")
	}
	m.signInPrompt = &gatewayPrompt{Title: "Name"}
	for _, key := range []string{"j", "g", "?", "/"} {
		m = press(m, key)
	}
	if m.signInInput != "jg?/" || m.helpOpen {
		t.Fatal("navigation stole text input")
	}
}
func TestSortPreservesSelectionAndFilteringDoesNotMutateState(t *testing.T) {
	m := sampleModel()
	defer m.cancel()
	m.issueGrouping = true
	m.state.Rows = []Row{
		{Job: Job{Key: "100", Scope: m.engine.c.Scope, Item: "100", Title: "Alpha", Agent: "code-review"}, Status: "running", Started: 10},
		{Job: Job{Key: "2", Scope: m.engine.c.Scope, Item: "2", Title: "Zulu", Agent: "code-review"}, Status: "failed", Started: 20, Finished: 25},
	}
	m.selected = "100"
	m.sortMode = 2
	m = press(m, "o")
	if m.sortMode != 1 || !strings.Contains(m.View(), "Issue ↑") || m.visibleRows()[0].Item != "2" || m.visibleRows()[m.index].Key != "100" {
		t.Fatal("duration did not wrap to issue or selection was lost")
	}
	m = press(m, "O")
	if m.visibleRows()[0].Item != "100" || !strings.Contains(m.View(), "Issue ↓") {
		t.Fatal("descending issue sort failed")
	}
	m = press(m, "o")
	if m.sortMode != 0 || m.visibleRows()[0].Item != "100" || !strings.Contains(m.View(), "Status ↑") {
		t.Fatal("issue did not advance to status")
	}
	m = press(m, "O")
	if m.visibleRows()[0].Item != "2" || !strings.Contains(m.View(), "Status ↓") {
		t.Fatal("descending status sort failed")
	}
	m = press(m, "o")
	if m.sortMode != 2 || m.visibleRows()[0].Item != "2" || !strings.Contains(m.View(), "Duration ↑") {
		t.Fatal("status did not advance to duration")
	}
	if strings.Contains(m.View(), "o sort column") || !strings.Contains(m.View(), "O reverse") {
		t.Fatal("sort hints are not in the footer")
	}
	m.setFilter("failed")
	if len(m.visibleRows()) != 1 || len(m.state.Rows) != 2 || m.state.Rows[0].Item != "100" {
		t.Fatal("display controls mutated scheduler state")
	}
}
func TestHelpReturnsToSameView(t *testing.T) {
	m := sampleModel()
	defer m.cancel()
	m.screen = "details"
	m.scroll = 3
	m = press(m, "?")
	if !m.helpOpen || !strings.Contains(m.View(), "Keyboard shortcuts") {
		t.Fatal("help missing")
	}
	m = press(m, "ctrl+f")
	if m.helpScroll == 0 || m.scroll != 3 {
		t.Fatal("help scrolling changed details")
	}
	m = press(m, "h")
	if m.helpOpen || m.screen != "details" || m.scroll != 3 {
		t.Fatal("help lost view")
	}
}

func TestDurationSortAndMissingTimestamps(t *testing.T) {
	m := sampleModel()
	defer m.cancel()
	m.sortMode = 2
	m.issueGrouping = true
	m.state.Rows = []Row{
		{Job: Job{Key: "missing", Scope: m.engine.c.Scope}, Status: "failed"},
		{Job: Job{Key: "long", Scope: m.engine.c.Scope, Item: "1"}, Status: "failed", Started: 100, Finished: 160},
		{Job: Job{Key: "short", Scope: m.engine.c.Scope, Item: "2"}, Status: "failed", Started: 100, Finished: 110},
	}
	if m.visibleRows()[0].Key != "short" || m.visibleRows()[2].Key != "missing" {
		t.Fatal("ascending duration order incorrect")
	}
	m.sortDescending = true
	if m.visibleRows()[0].Key != "long" || m.visibleRows()[2].Key != "missing" {
		t.Fatal("descending duration order incorrect")
	}
	if got := runDurationAt(Row{Status: "running", Started: 100}, 125); got != 25 {
		t.Fatalf("elapsed duration = %v", got)
	}
}
