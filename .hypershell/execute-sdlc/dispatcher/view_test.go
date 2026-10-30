package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestMiddleEllipsisAndUnicodeWidth(t *testing.T) {
	for _, title := range []string{"This is the very long title of the issue", "修复网关认证时的问题还有更多字符", "Fix gateway 🔐 login across restarts"} {
		got := middle(title, 20)
		if lipgloss.Width(got) > 20 {
			t.Fatalf("overflow: %q", got)
		}
		if !strings.Contains(got, "...") {
			t.Fatalf("missing middle ellipsis: %q", got)
		}
	}
}
func TestActivityDoesNotExposeCommandContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "run.log")
	os.WriteFile(path, []byte("Running agent\n⚙ Bash: SECRET=never-display\n"), 0600)
	if got := activity(path); got != "Working" {
		t.Fatal(got)
	}
	os.WriteFile(path, []byte("→ Running agent: funnel-ingest\nLoading harness: secret\n"), 0600)
	if got := activity(path); got != "Preparing agent" {
		t.Fatal(got)
	}
}
func sampleModel() Model {
	ctx, cancel := context.WithCancel(context.Background())
	return Model{ctx: ctx, cancel: cancel, width: 100, height: 24, plainColor: true, engine: &Engine{c: Config{Scope: "agent/scope/test", Concurrency: 10}}, state: Snapshot{Gateway: "test", Connection: "Connected", Active: 1, Rows: []Row{{Job: Job{Key: "scope", Scope: "agent/scope/test", Agent: "funnel-ingest", Item: "agent/scope/test"}, Status: "running", Activity: "Working", Started: seconds()}, {Job: Job{Key: "42", Scope: "agent/scope/test", Item: "42", Title: "The issue title", Agent: "code-review"}, Status: "failed", Next: seconds() + 60}}}}
}
func TestViewUsesPlainLabelsAndNoRepeatedCounters(t *testing.T) {
	m := sampleModel()
	defer m.cancel()
	view := m.View()
	for _, want := range []string{"Find and link issues", "#42", "The issue title", "Retry in", "enter logs"} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %q: %s", want, view)
		}
	}
	for _, bad := range []string{"DISPATCHING", "Scope intake", "Workers [", "RETRY "} {
		if strings.Contains(view, bad) {
			t.Fatalf("busy or unclear copy: %s", bad)
		}
	}
	if strings.Contains(view, "\x1b[") {
		t.Fatal("NO_COLOR emitted escapes")
	}
}
func TestNavigationLogViewAndDetach(t *testing.T) {
	m := sampleModel()
	defer m.cancel()
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(Model)
	if m.index != 1 {
		t.Fatal("selection did not move")
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.screen != "logs" {
		t.Fatal("log view did not open")
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	if m.screen != "" {
		t.Fatal("back failed")
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil || m.ctx.Err() == nil {
		t.Fatal("detach did not cancel dispatcher")
	}
}
func TestNarrowViewFitsAndRowsRemainSelectable(t *testing.T) {
	m := sampleModel()
	defer m.cancel()
	m.width = 42
	view := m.View()
	for _, line := range strings.Split(view, "\n") {
		if lipgloss.Width(line) > 42 {
			t.Fatalf("line overflows: %q", line)
		}
	}
	if !strings.Contains(view, "#42") {
		t.Fatal("narrow view hid issues")
	}
}

func TestDetailsAndLogsShareAgentAndStatusHeader(t *testing.T) {
	m := sampleModel()
	defer m.cancel()
	m.index = 1
	m.height = 32
	m.state.Rows[1].Log = "/tmp/worker.log"
	m.state.Rows[1].Error = "exit 1; see /tmp/worker.log"
	for _, screen := range []string{"details", "logs"} {
		m.screen = screen
		view := m.View()
		for _, want := range []string{"#42", "The issue title", "Agent", "code-review", "Failed · Retry in"} {
			if !strings.Contains(view, want) {
				t.Fatalf("%s missing %q: %s", screen, want, view)
			}
		}
		if strings.Contains(view, "Recent problems") || strings.Contains(view, "Gateway:") {
			t.Fatal("redundant or empty detail section")
		}
		if screen == "details" && strings.Count(view, "/tmp/worker.log") != 1 {
			t.Fatal("worker path repeated in failure message")
		}
	}
}

func TestDetailsPreserveStatusColorAndSanitizeExternalText(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(old)
	m := sampleModel()
	defer m.cancel()
	m.plainColor = false
	m.index = 1
	m.screen = "details"
	m.state.Rows[1].Title = "Issue\x1b[2J title"
	view := m.View()
	if !strings.Contains(view, m.style("Failed · "+rowStatus(m.state.Rows[1]), "1", true)) {
		t.Fatalf("failed status lost red styling: %q", view)
	}
	if strings.Contains(view, "\x1b[2J") {
		t.Fatal("untrusted terminal controls survived")
	}
	m.plainColor = true
	view = m.View()
	if strings.Contains(view, "\x1b[") {
		t.Fatal("NO_COLOR ignored")
	}
}

func TestDetailsScrollInsteadOfChangingSelectedAgent(t *testing.T) {
	m := sampleModel()
	defer m.cancel()
	m.screen = "details"
	m.index = 1
	m.state.Message = strings.Repeat("Detailed problem\n", 40)
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(Model)
	if m.index != 1 || m.scroll != 1 {
		t.Fatal("details scrolling changed selection")
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = next.(Model)
	if m.scroll != 0 {
		t.Fatal("details did not scroll back")
	}
}

func TestIssueGroupingAndManifestNames(t *testing.T) {
	m := sampleModel()
	m.issueGrouping = true
	defer m.cancel()
	m.width = 140
	older := m.state.Rows[1]
	older.Key = "42:spec"
	older.Agent = "spec-implementation"
	older.Status = "succeeded"
	m.state.Rows = append(m.state.Rows, older)
	if rows := m.visibleRows(); len(rows) != 2 || rows[1].Agent != "code-review" {
		t.Fatalf("grouping: %+v", rows)
	}
	view := m.View()
	if strings.Count(view, "#42") != 1 || !strings.Contains(view, "funnel-ingest") || !strings.Contains(view, "code-review") {
		t.Fatal(view)
	}
	if strings.Contains(view, "Intake") || strings.Contains(view, "Review code") {
		t.Fatal("invented agent aliases remain")
	}
}

func TestHistoryNavigationAndHistoricalLog(t *testing.T) {
	m := sampleModel()
	defer m.cancel()
	m.engine.store = testStore(t)
	m.index = 1
	issue := m.visibleRows()[1]
	path := filepath.Join(t.TempDir(), "old.log")
	os.WriteFile(path, []byte("previous attempt output"), 0600)
	id, err := m.engine.store.start(issue.Job, path)
	if err != nil {
		t.Fatal(err)
	}
	if err = m.engine.store.finish(issue.Key, id, 1, 30, 30); err != nil {
		t.Fatal(err)
	}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("H")})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("history load missing")
	}
	next, _ = m.Update(cmd())
	m = next.(Model)
	if len(m.visibleRows()) != 1 || !strings.Contains(m.View(), "Run history") {
		t.Fatal(m.View())
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.screen != "logs" || !strings.Contains(m.logText, "previous attempt output") {
		t.Fatal("wrong attempt log")
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	if m.historyIssue == nil || m.screen != "" {
		t.Fatal("logs should return to history")
	}
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	m = next.(Model)
	if cmd != nil || !strings.Contains(m.notice, "overview") {
		t.Fatal("historical attempt should not retry")
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	if m.historyIssue != nil || m.visibleRows()[m.index].Key != issue.Key {
		t.Fatal("issue selection was lost")
	}
}

func TestGroupedSelectionFollowsIssueAcrossAgentChanges(t *testing.T) {
	m := sampleModel()
	m.issueGrouping = true
	defer m.cancel()
	m.index = 1
	m.selected = "42"
	next := m.state.Rows[1]
	next.Agent = "code-implementation"
	next.Key = "42:code"
	next.Status = "running"
	m.engine.state = Snapshot{Rows: []Row{next, m.state.Rows[0]}}
	updated, _ := m.Update(tickMsg{})
	m = updated.(Model)
	if m.visibleRows()[m.index].Item != next.Item || m.selected != next.Key {
		t.Fatalf("selection moved to another issue: %d %s", m.index, m.selected)
	}
}

func TestAgentGroupsDefaultOrderAndCompletedToggle(t *testing.T) {
	m := sampleModel()
	defer m.cancel()
	m.width = 140
	m.height = 40
	base := m.state.Rows[1]
	done := base
	done.Key = "done"
	done.Agent = "aaa-completed"
	done.Status = "succeeded"
	running := base
	running.Key = "running-code"
	running.Agent = "code-implementation"
	running.Status = "running"
	other := running
	other.Key = "other-code"
	other.Item = "43"
	m.state.Rows = append(m.state.Rows, done, running, other)
	rows := m.visibleRows()
	if len(rows) != 4 || rows[0].Agent != "code-implementation" || rows[1].Agent != "code-implementation" || rows[3].Status != "failed" {
		t.Fatalf("wrong grouping/order: %+v", rows)
	}
	view := m.View()
	if strings.Count(view, "code-implementation") != 1 || !strings.Contains(view, "2 running") || strings.Contains(view, "aaa-completed") || strings.Contains(view, "FULLSEND AGENT") {
		t.Fatal(view)
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	m = next.(Model)
	if len(m.visibleRows()) != 5 || m.visibleRows()[4].Agent != "aaa-completed" {
		t.Fatal("completed group not revealed at end")
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(Model)
	if !m.issueGrouping || len(m.visibleRows()) != 3 {
		t.Fatalf("issue grouping: %+v", m.visibleRows())
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(Model)
	if m.issueGrouping {
		t.Fatal("did not return to agent grouping")
	}
}

func TestAgentViewportKeepsSelectedRowAndHeadingVisible(t *testing.T) {
	for _, width := range []int{45, 100, 140} {
		m := sampleModel()
		m.width = width
		m.height = 20
		m.state.Rows = nil
		for i := 0; i < 20; i++ {
			m.state.Rows = append(m.state.Rows, Row{Job: Job{Key: fmt.Sprint(i), Scope: m.engine.c.Scope, Agent: fmt.Sprintf("agent-%02d", i), Item: fmt.Sprint(i), Title: "Issue title"}, Status: "running"})
		}
		m.index = 18
		m.selected = "18"
		view := m.View()
		if !strings.Contains(view, "agent-18") || !strings.Contains(view, "> #18") {
			t.Fatalf("selection missing at width %d: %s", width, view)
		}
		for _, line := range strings.Split(view, "\n") {
			if lipgloss.Width(line) > width {
				t.Fatalf("overflow at %d: %q", width, line)
			}
		}
		m.cancel()
	}
}

func TestUnknownRetryRequiresExplicitConfirmation(t *testing.T) {
	m := sampleModel()
	defer m.cancel()
	m.index = 1
	m.state.Rows[1].Status = "uncertain"
	m.state.Rows[1].RunID = "original-run"
	m.engine.retries = make(chan retryRequest, 1)
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	m = next.(Model)
	if cmd != nil || m.pendingRetry == nil || !strings.Contains(m.View(), "I verified it stopped") {
		t.Fatal("missing confirmation")
	}
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	m = next.(Model)
	if cmd != nil || m.pendingRetry != nil {
		t.Fatal("cancel retried a job")
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	m = next.(Model)
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	m = next.(Model)
	if cmd == nil || m.pendingRetry != nil {
		t.Fatal("confirmation did not submit")
	}
	result := make(chan tea.Msg, 1)
	go func() { result <- cmd() }()
	request := <-m.engine.retries
	if request.Key != "42" || request.RunID != "original-run" {
		t.Fatalf("wrong retry target: %+v", request)
	}
	request.Done <- nil
	if got := (<-result).(retryDone); got.err != nil {
		t.Fatal(got.err)
	}
}

func TestCompletionAgeUsesFinishTime(t *testing.T) {
	for _, tc := range []struct {
		age  float64
		want string
	}{{0, "0s"}, {25, "25s"}, {60, "1min"}, {180, "3min"}, {3600, "1h"}, {86400, "1d"}, {-1, "0s"}} {
		if got := relativeAge(tc.age); got != tc.want {
			t.Fatalf("age %v: %s", tc.age, got)
		}
	}
	r := Row{Status: "succeeded", Started: seconds() - 3600, Finished: seconds() - 125}
	if got := rowStatus(r); got != "Run completed · 2min ago" {
		t.Fatal(got)
	}
	r.Activity = "history"
	if got := rowStatus(r); got != "Run completed · 2min ago" {
		t.Fatal(got)
	}
	r.Finished = 0
	if got := rowStatus(r); got != "Run completed" {
		t.Fatal("invented missing timestamp:", got)
	}
}

func TestScrollPanesKeepHeightAndClampAtBothEnds(t *testing.T) {
	for _, screen := range []string{"logs", "details"} {
		m := sampleModel()
		m.screen = screen
		m.height = 28
		m.width = 100
		m.index = 1
		m.logText = "first log line\n" + strings.Repeat("middle log line\n", 80) + "last log line"
		m.state.Message = strings.Repeat("Problem details\n", 80)
		for _, offset := range []int{0, 10, 10000} {
			m.scroll = offset
			m.clampScroll()
			view := m.View()
			if got := lipgloss.Height(view); got != m.height {
				t.Fatalf("%s height %d at offset %d", screen, got, offset)
			}
			if !strings.Contains(view, "Lines ") {
				t.Fatal("missing scroll indicator")
			}
			if offset == 10000 && screen == "logs" && !strings.Contains(view, "first log line") {
				t.Fatal("top line disappeared")
			}
		}
		old := m.scroll
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = next.(Model)
		if screen == "logs" && m.scroll != max(0, old-1) {
			t.Fatal("overscroll trapped navigation")
		}
		m.cancel()
	}
}

// Set FUNNEL_PREVIEW_DIR to render real terminal frames for visual inspection.
func TestTerminalPreviewFrames(t *testing.T) {
	dir := os.Getenv("FUNNEL_PREVIEW_DIR")
	if dir == "" {
		t.Skip("optional visual preview")
	}
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(old)
	dark := lipgloss.HasDarkBackground()
	lipgloss.SetHasDarkBackground(true)
	defer lipgloss.SetHasDarkBackground(dark)
	m := sampleModel()
	defer m.cancel()
	m.width = 120
	m.height = 32
	m.plainColor = false
	m.showCompleted = true
	m.state.Gateway = "gyre-fullsend"
	m.engine.c.Scope = "agent/scope/test"
	m.state.Rows = []Row{
		{Job: Job{Key: "840", Scope: m.engine.c.Scope, Item: "840", Title: "Glass: show bundle PRs separately for hypershell and hypershell-gitops", Agent: "code-implementation"}, Status: "running", Activity: "Working", Started: seconds() - 747},
		{Job: Job{Key: "856", Scope: m.engine.c.Scope, Item: "856", Title: "Fleet dashboard: improve navigation and gateway configuration", Agent: "code-implementation"}, Status: "running", Activity: "Creating sandbox", Started: seconds() - 36},
		{Job: Job{Key: "859", Scope: m.engine.c.Scope, Item: "859", Title: "Gateway configuration: validate route selection", Agent: "spec-review"}, Status: "failed", Next: seconds() + 45, Log: "/home/jsell/.local/state/hypershell/funnel/logs/spec-review-859.log", Error: "Agent exited with code 1 during validation."},
		{Job: Job{Key: "832", Scope: m.engine.c.Scope, Item: "832", Title: "Tighten RoleBinding write authorization for authenticated users", Agent: "funnel-reconcile"}, Status: "succeeded", Finished: seconds() - 125},
	}
	m.state.Active = 2
	for _, screen := range []string{"overview", "logs", "details", "signin", "filter", "help"} {
		m.screen = screen
		if screen == "overview" {
			m.screen = ""
			m.index = 0
		} else {
			m.index = 2
		}
		if screen == "signin" {
			m.signInActive = true
			m.signInPrompt = &gatewayPrompt{Title: "Connect your gateway", Description: "New work is paused. Choose how to reconnect.", Choices: []gatewayChoice{
				{Value: "1", Title: "Reconnect and sign in", Description: "Refresh this gateway connection, then sign in with your browser."},
				{Value: "2", Title: "Create a new gateway", Description: "Choose a cluster and name. Your existing gateway will be kept."},
				{Value: "3", Title: "Keep waiting", Description: "Return to the work list and retry the connection automatically."},
			}}
		}
		m.helpOpen = screen == "help"
		if screen == "filter" {
			m.screen = ""
			m.filter = "code-implementation"
			m.index = 0
		} else {
			m.filter = ""
		}
		m.logText = strings.Repeat("Checking repository and dependency configuration...\n", 40) + "Validation failed: required permission is missing."
		if err := os.WriteFile(filepath.Join(dir, screen+".ansi"), []byte(m.View()), 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestEveryScrollableListUsesScrollbar(t *testing.T) {
	for _, mode := range []string{"agents", "issues", "history", "details", "logs", "signin"} {
		m := sampleModel()
		m.width = 100
		m.height = 24
		m.state.Rows = nil
		for i := 0; i < 35; i++ {
			m.state.Rows = append(m.state.Rows, Row{Job: Job{Key: fmt.Sprint(i), Scope: m.engine.c.Scope, Item: fmt.Sprint(i), Agent: "code-implementation", Title: "Example work item"}, Status: "running"})
		}
		switch mode {
		case "issues":
			m.issueGrouping = true
		case "history":
			r := m.state.Rows[0]
			m.historyIssue = &r
			m.historyRows = m.state.Rows
		case "details":
			m.screen = "details"
			m.state.Message = strings.Repeat("Diagnostic line\n", 100)
		case "logs":
			m.screen = "logs"
			m.logText = strings.Repeat("Log line\n", 100)
		case "signin":
			m.screen = "signin"
			m.signInDiagnostics = true
			m.signInText = strings.Repeat("Authentication progress\n", 100)
		}
		view := m.View()
		if !strings.Contains(view, "█") || !strings.Contains(view, "│") {
			t.Fatalf("%s missing scrollbar: %s", mode, view)
		}
		if lipgloss.Height(view) != m.height {
			t.Fatalf("%s height mismatch", mode)
		}
		if strings.Contains(view, "\x1b[") {
			t.Fatalf("%s ignored no-color", mode)
		}
		m.cancel()
	}
}

func TestNativeGatewayMenu(t *testing.T) {
	m := sampleModel()
	defer m.cancel()
	m.screen = "signin"
	m.signInActive = true
	m.receiveSignIn("Reusing saved gateway secret-id\n\x1e{\"title\":\"Connect your gateway\",\"choices\":[{\"value\":\"1\",\"title\":\"Reconnect\"},")
	if m.signInPrompt != nil {
		t.Fatal("accepted incomplete prompt")
	}
	m.receiveSignIn("{\"value\":\"2\",\"title\":\"Create a gateway\"}]}\x1f")
	if m.signInPrompt == nil || len(m.signInPrompt.Choices) != 2 {
		t.Fatal("lost chunked prompt")
	}
	if strings.Contains(m.View(), "secret-id") || !strings.Contains(m.View(), "> Reconnect") {
		t.Fatal("menu exposed diagnostic text or lost selection")
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(Model)
	if m.signInChoice != 1 || !strings.Contains(m.View(), "> Create a gateway") {
		t.Fatal("arrow navigation failed")
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m = updated.(Model)
	if !strings.Contains(m.View(), "secret-id") {
		t.Fatal("diagnostics unavailable")
	}
}
