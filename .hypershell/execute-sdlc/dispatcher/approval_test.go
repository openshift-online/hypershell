package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// Verifies: SDLC-LOC-12
func heldApproval() Row {
	return Row{Job: Job{Key: "856:merge-approval", Scope: "agent/scope/test", Repo: "o/r", Item: "856", Title: "Gateway list: newest first and ownership filters", Agent: "code-review", PRNumber: "900", PRURL: "https://github.com/o/r/pull/900", PRHeadSHA: "abc123", PRBaseSHA: "base123"}, Status: "awaiting_approval"}
}
func TestHeldApprovalsStayFirstInBothViewsAndFilters(t *testing.T) {
	m := sampleModel()
	defer m.cancel()
	m.state.AwaitingApproval = []Row{heldApproval()}
	for _, issues := range []bool{false, true} {
		m.issueGrouping = issues
		for _, descending := range []bool{false, true} {
			m.sortDescending = descending
			rows := m.visibleRows()
			if len(rows) < 2 || rows[0].Status != "awaiting_approval" {
				t.Fatalf("approval lost priority: %+v", rows)
			}
			view := m.View()
			if !strings.Contains(view, "Awaiting approval") || strings.Index(view, "Awaiting approval") > strings.Index(view, "856") {
				t.Fatal(view)
			}
		}
	}
	m.filter = "856"
	if rows := m.visibleRows(); len(rows) != 1 || rows[0].Status != "awaiting_approval" {
		t.Fatalf("filter: %+v", rows)
	}
}
func TestApprovalRequiresExplicitConfirmationAndBlocksMonitoring(t *testing.T) {
	m := sampleModel()
	defer m.cancel()
	row := heldApproval()
	next, cmd := m.beginApproval(row)
	m = next.(Model)
	if cmd != nil || m.pendingApproval == nil || m.approvalChoice != 0 {
		t.Fatal("unsafe initial confirmation")
	}
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if cmd != nil || m.pendingApproval != nil || m.approvalBusy {
		t.Fatal("default confirmation wrote approval")
	}
	next, _ = m.beginApproval(row)
	m = next.(Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = next.(Model)
	if m.approvalChoice != 1 {
		t.Fatal("choice is not keyboard navigable")
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	if m.pendingApproval != nil {
		t.Fatal("escape did not cancel")
	}
	m.state.Monitoring = true
	next, cmd = m.beginApproval(row)
	m = next.(Model)
	if cmd != nil || m.pendingApproval != nil {
		t.Fatal("monitor was allowed to approve")
	}
}
func TestApprovalConfirmationUsesSharedFixedFrame(t *testing.T) {
	m := sampleModel()
	defer m.cancel()
	row := heldApproval()
	m.pendingApproval = &row
	for _, size := range [][2]int{{60, 18}, {100, 30}, {140, 40}} {
		m.width, m.height = size[0], size[1]
		view := m.View()
		if lipgloss.Width(view) > m.width || lipgloss.Height(view) != m.height {
			t.Fatalf("overflow %v: %dx%d", size, lipgloss.Width(view), lipgloss.Height(view))
		}
		if !strings.Contains(view, "Merge queue / Approve PR") {
			t.Fatal(view)
		}
	}
}
func TestApprovalPreview(t *testing.T) {
	dir := os.Getenv("FUNNEL_PREVIEW_DIR")
	if dir == "" {
		t.Skip("optional visual preview")
	}
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(old)
	m := sampleModel()
	defer m.cancel()
	m.width, m.height = 120, 32
	m.plainColor = false
	m.state.AwaitingApproval = []Row{heldApproval()}
	for _, screen := range []string{"approval-agents", "approval-issues", "approval-confirm"} {
		m.issueGrouping = screen == "approval-issues"
		if screen == "approval-confirm" {
			row := heldApproval()
			m.pendingApproval = &row
		}
		if err := os.WriteFile(filepath.Join(dir, screen+".ansi"), []byte(m.View()), 0600); err != nil {
			t.Fatal(err)
		}
	}
}
