package main

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type approvalDone struct {
	key, pr string
	err     error
}

func (m Model) beginApproval(row Row) (tea.Model, tea.Cmd) {
	if m.approvalBusy {
		m.setNotice("Approval is already being checked.")
		return m, nil
	}
	if row.Status != "awaiting_approval" || m.historyIssue != nil {
		m.setNotice("Select a PR in Awaiting approval.")
		return m, nil
	}
	if m.state.Monitoring || m.engine.config().DryRun || m.engine.config().Status {
		m.setNotice("Approve from the active dispatcher terminal.")
		return m, nil
	}
	m.pendingApproval = &row
	m.approvalChoice, m.confirmScroll = 0, 0
	return m, nil
}
func (m Model) updateApproval(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc", "h":
		m.pendingApproval = nil
	case "j", "k", "up", "down", "left", "right", "tab":
		m.approvalChoice = 1 - m.approvalChoice
		m.confirmScroll = 1000000
	case "pgup":
		m.confirmScroll = max(0, m.confirmScroll-1)
	case "pgdown":
		m.confirmScroll++
	case "enter":
		row := *m.pendingApproval
		m.pendingApproval = nil
		if m.approvalChoice == 1 {
			m.approvalBusy = true
			m.approvalKey = row.Key
			m.setNotice("Checking live PR readiness before approval…")
			return m, func() tea.Msg {
				ctx, cancel := context.WithTimeout(m.ctx, 2*time.Minute)
				defer cancel()
				var result struct{}
				err := m.engine.adapter().call(ctx, "approve-merge", row.Job, &result)
				if err == nil {
					m.engine.note("Manual merge approval applied to PR #" + row.PRNumber)
				} else {
					m.engine.note("Manual merge approval failed: " + err.Error())
				}
				return approvalDone{row.Key, row.PRNumber, err}
			}
		}
	case "q", "ctrl+c":
		m.cancel()
		m.exit = 130
		return m, tea.Quit
	}
	return m, nil
}
func (m Model) approvalSummary(r Row, width int) []string {
	body := []string{m.style("PULL REQUEST", "8", true), m.style("PR #"+r.PRNumber, "7", true)}
	body = append(body, m.wrapped(r.PRURL, width, "6")...)
	body = append(body, "")
	body = append(body, m.style("ISSUE", "8", true))
	body = append(body, m.wrapped(title(r), width, "7")...)
	sha := r.PRHeadSHA
	if len(sha) > 12 {
		sha = sha[:12]
	}
	body = append(body, m.wrapped("Commit  "+sha, width, "8")...)
	return body
}
func (m Model) approvalView(lines []string, w, h int) string {
	r := *m.pendingApproval
	width := max(1, w-2)
	lines = append(lines, m.style("Approve PR for merge?", "approval-text", true), "")
	body := m.approvalSummary(r, width)
	body = append(body, "", m.style("WHAT HAPPENS NEXT", "8", true))
	body = append(body, m.wrapped("PR #"+r.PRNumber+" can merge automatically when the repository checks pass.", width, "7")...)
	body = append(body, m.wrapped("We will recheck the commits, CI and authorization before adding the PR approval label. Your automatic approval setting will not change.", width, "8")...)
	body = append(body, "")
	choices := []struct{ title, detail string }{
		{"Keep waiting", "Leave this PR out of the merge queue."},
		{"Approve PR for merge", "Allow this PR to enter the merge queue."},
	}
	for i, choice := range choices {
		if i > 0 {
			body = append(body, "")
		}
		line := "  " + choice.title
		if i == m.approvalChoice {
			line = "> " + choice.title
		}
		body = append(body, m.workRow(line, width, Row{Status: "awaiting_approval"}, i == m.approvalChoice))
		body = append(body, m.wrapped("  "+choice.detail, width, "8")...)
	}
	count := max(1, h-len(lines)-3)
	start := min(m.confirmScroll, max(0, len(body)-count))
	lines = append(lines, m.scrollPane(body, start, count, w)...)
	lines = append(lines, m.scrollIndicator(start, min(len(body), start+count), len(body)), "", m.style("j/k choose   enter confirm   PgUp/PgDn scroll   esc back   q detach", "8", false))
	return m.frame(lines, w, h)
}
