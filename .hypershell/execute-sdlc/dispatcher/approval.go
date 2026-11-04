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
func (m Model) approvalView(lines []string, w, h int) string {
	r := *m.pendingApproval
	lines = append(lines, m.style("Merge queue / Approve PR", "3", true), "")
	body := m.wrapped(title(r), w-2, "7")
	body = append(body, "")
	body = append(body, m.wrapped(r.PRURL, w-2, "6")...)
	body = append(body, m.wrapped("PR commit: "+r.PRHeadSHA, w-2, "8")...)
	body = append(body, "")
	body = append(body, m.wrapped("Add agent/review-code-approved to this PR? This makes it eligible for automatic merging when repository queue rules pass. Issue approval is already recorded.", w-2, "7")...)
	body = append(body, m.wrapped("Authorization, linked PR, commits, CI and mergeability will be checked again. This approves only this PR; automatic merge approval stays unchanged in Configuration.", w-2, "8")...)
	body = append(body, "")
	for i, label := range []string{"Keep waiting", "Approve PR for merge"} {
		line := "  " + label
		if i == m.approvalChoice {
			line = m.selection("> "+label, w-2)
		}
		body = append(body, line)
	}
	count := max(1, h-len(lines)-4)
	start := min(m.confirmScroll, max(0, len(body)-count))
	lines = append(lines, m.withScrollbar(body[start:min(len(body), start+count)], start, min(len(body), start+count), len(body), count, w)...)
	lines = append(lines, m.scrollIndicator(start, min(len(body), start+count), len(body)), "", m.style("j/k choose   enter confirm   PgUp/PgDn scroll   esc back   q detach", "8", false))
	return m.frame(lines, w, h)
}
