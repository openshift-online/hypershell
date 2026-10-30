package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func clean(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' || !unicode.IsControl(r) {
			return r
		}
		return -1
	}, ansi.Strip(s))
}
func middle(s string, width int) string {
	s = strings.ReplaceAll(clean(s), "\n", " ")
	s = strings.ReplaceAll(s, "\t", " ")
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= width {
		return s
	}
	if width <= 3 {
		return strings.Repeat(".", width)
	}
	left := (width - 3 + 1) / 2
	right := width - 3 - left
	runes := []rune(s)
	prefix := ""
	suffix := ""
	for _, r := range runes {
		if lipgloss.Width(prefix+string(r)) > left {
			break
		}
		prefix += string(r)
	}
	for i := len(runes) - 1; i >= 0; i-- {
		if lipgloss.Width(string(runes[i])+suffix) > right {
			break
		}
		suffix = string(runes[i]) + suffix
	}
	return prefix + "..." + suffix
}
func duration(seconds float64) string {
	n := max(0, int(seconds))
	if n < 60 {
		return fmt.Sprintf("%ds", n)
	}
	if n < 3600 {
		return fmt.Sprintf("%dm %02ds", n/60, n%60)
	}
	return fmt.Sprintf("%dh %02dm", n/3600, n/60%60)
}
func tail(path string, limit int64) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	size, err := f.Stat()
	if err != nil {
		return "", err
	}
	if _, err = f.Seek(max(0, size.Size()-limit), io.SeekStart); err != nil {
		return "", err
	}
	data, err := io.ReadAll(io.LimitReader(f, limit))
	return clean(string(data)), err
}
func activity(path string) string {
	text, err := tail(path, 32768)
	if err != nil {
		return "Log unavailable"
	}
	markers := [][2]string{{"Cleaning up sandbox", "Cleaning up"}, {"Collecting OpenShell logs", "Collecting logs"}, {"Post-loop validation", "Checking results"}, {"Extracting target repo", "Saving changes"}, {"Extracting transcripts", "Saving transcript"}, {"Extracting output files", "Saving results"}, {"Agent running (", "Working"}, {"Running agent", "Working"}, {"Checking GitHub API", "Checking GitHub"}, {"pre-agent security scan", "Security check"}, {"Copying project code", "Copying project"}, {"Bootstrapping sandbox", "Preparing sandbox"}, {"Sandbox bootstrapped", "Sandbox ready"}, {"Creating sandbox", "Creating sandbox"}, {"Sandbox created", "Sandbox ready"}, {"Ensuring provider", "Setting up access"}, {"Checking gateway", "Checking gateway"}, {"Loading harness", "Preparing agent"}}
	lines := strings.Split(text, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, "⚙") || strings.HasPrefix(line, "💬") {
			return "Working"
		}
		for _, m := range markers {
			if m[0] == "Running agent" && strings.Contains(line, "Running agent:") {
				continue
			}
			if strings.Contains(line, m[0]) {
				return m[1]
			}
		}
	}
	return "Starting"
}
func title(r Row) string {
	if r.Agent == "funnel-ingest" {
		return "Find and link issues"
	}
	if r.Title == "" {
		return "#" + r.Item + "  Loading title"
	}
	return "#" + r.Item + "  " + r.Title
}
func agentName(a string) string { return a }
func completionStatus(finished float64) string {
	if finished <= 0 {
		return "Run completed"
	}
	return "Run completed · " + relativeAge(seconds()-finished) + " ago"
}
func relativeAge(age float64) string {
	n := max(0, int(age))
	switch {
	case n < 60:
		return fmt.Sprintf("%ds", n)
	case n < 3600:
		return fmt.Sprintf("%dmin", n/60)
	case n < 86400:
		return fmt.Sprintf("%dh", n/3600)
	default:
		return fmt.Sprintf("%dd", n/86400)
	}
}
func rowStatus(r Row) string {
	if r.Activity == "history" {
		switch r.Status {
		case "failed":
			return "Failed"
		case "succeeded":
			return completionStatus(r.Finished)
		case "running":
			return "Running"
		default:
			return "Outcome unknown"
		}
	}
	switch r.Status {
	case "running":
		phase := r.Activity
		if phase == "" {
			phase = "Running"
		}
		return phase + " · " + duration(seconds()-r.Started)
	case "failed":
		if r.Next > seconds() {
			return "Retry in " + duration(r.Next-seconds())
		}
		return "Ready to retry"
	case "uncertain":
		return "Outcome unknown"
	case "succeeded":
		return completionStatus(r.Finished)
	default:
		return r.Status
	}
}

type tickMsg time.Time
type retryDone struct {
	key string
	err error
}
type historyDone struct {
	issue Row
	rows  []Row
	err   error
}
type Model struct {
	sortDescending                      bool
	filter, historyFilter, filterBefore string
	filtering, helpOpen                 bool
	helpScroll, sortMode                int
	gPending                            time.Time
	confirmScroll                       int
	signIn                              *signInSession
	signInActive                        bool
	signInCancel                        context.CancelFunc
	signInText, signInInput             string
	signInScroll                        int
	signInPending                       string
	signInPrompt                        *gatewayPrompt
	signInChoice                        int
	signInResult                        string
	signInDiagnostics                   bool
	pendingRetry                        *Row
	issueGrouping                       bool
	showCompleted                       bool
	historyIssue                        *Row
	historyRows                         []Row
	historyLoading                      bool
	engine                              *Engine
	ctx                                 context.Context
	cancel                              context.CancelFunc
	state                               Snapshot
	width, height, index, scroll        int
	selected, screen, notice, logText   string
	noticeUntil                         time.Time
	plainColor                          bool
	exit                                int
}

func (m Model) Init() tea.Cmd { return tick() }
func tick() tea.Cmd {
	return tea.Tick(400*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) })
}
func (m Model) scopedRows() []Row {
	rows := []Row{}
	for _, r := range m.state.Rows {
		if r.Scope == m.engine.c.Scope || r.Status == "running" {
			rows = append(rows, r)
		}
	}
	return rows
}
func (m Model) visibleRows() []Row {
	if m.historyIssue != nil {
		return m.orderRows(m.filterRows(append([]Row(nil), m.historyRows...)))
	}
	rows := []Row{}
	seen := map[string]bool{}
	for _, r := range m.filterRows(m.scopedRows()) {
		if r.Status == "succeeded" && !m.showCompleted && !(m.screen != "" && r.Key == m.selected) {
			continue
		}
		group := r.Scope + "\x00" + r.Repo + "\x00" + r.Item
		if !m.issueGrouping || !seen[group] {
			rows = append(rows, r)
			seen[group] = true
		}
	}
	if !m.issueGrouping {
		ranks := map[string]int{}
		for _, r := range rows {
			v, ok := ranks[r.Agent]
			if !ok || rank(r.Status) < v {
				ranks[r.Agent] = rank(r.Status)
			}
		}
		sort.SliceStable(rows, func(i, j int) bool {
			a, b := rows[i], rows[j]
			if ranks[a.Agent] != ranks[b.Agent] {
				return ranks[a.Agent] < ranks[b.Agent]
			}
			if a.Agent != b.Agent {
				return a.Agent < b.Agent
			}
			return m.rowLess(a, b)
		})
	}
	if m.issueGrouping {
		rows = m.orderRows(rows)
	}
	return rows
}
func (m *Model) restoreSelection(key string, issue *Row) {
	rows := m.visibleRows()
	m.index = max(0, min(m.index, len(rows)-1))
	for i, r := range rows {
		if r.Key == key {
			m.index = i
			m.selected = r.Key
			return
		}
	}
	if issue != nil && m.issueGrouping {
		for i, r := range rows {
			if sameIssue(r, *issue) {
				m.index = i
				break
			}
		}
	}
	if len(rows) > 0 {
		m.selected = rows[m.index].Key
	} else {
		m.selected = ""
	}
}
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = v.Width
		m.height = v.Height
		if m.signIn != nil && m.signInActive {
			if err := m.signIn.resize(v.Width-8, v.Height-12); err != nil {
				m.setNotice("Cannot resize sign-in terminal: " + err.Error())
			}
		}
	case tickMsg:
		m.expireNotice(time.Time(v))
		var previous *Row
		before := m.visibleRows()
		if m.historyIssue == nil && m.index >= 0 && m.index < len(before) {
			r := before[m.index]
			previous = &r
		}
		m.state = m.engine.snapshot()
		if m.state.Connection == "Connection lost" && m.notice == "Gateway access verified." {
			m.setNotice("Connection check failed. Press d for the error.")
		}
		rows := m.visibleRows()
		m.restoreSelection(m.selected, previous)
		if m.screen == "logs" && len(rows) > 0 {
			value, err := tail(rows[m.index].Log, 65536)
			if err != nil {
				m.logText = "Log is not available yet: " + err.Error()
			} else {
				m.logText = value
			}
		}
		if m.state.Done {
			m.exit = m.state.Exit
			return m, tea.Quit
		}
		m.clampScroll()
		return m, tick()
	case signInStarted:
		if v.Err != nil {
			m.signInActive = false
			m.setNotice(v.Err.Error())
			m.signInResult = v.Err.Error()
			m.signInText += "\n" + v.Err.Error()
			return m, nil
		}
		m.signIn = v.Session
		return m, m.signIn.next()
	case signInEvent:
		if v.Session != m.signIn {
			return m, nil
		}
		m.receiveSignIn(v.Text)
		if len(m.signInText) > 65536 {
			m.signInText = m.signInText[len(m.signInText)-65536:]
		}
		if v.Done {
			m.signInPrompt = nil
			m.signInActive = false
			if m.signInCancel != nil {
				m.signInCancel()
			}
			if v.Err != nil {
				m.setNotice(v.Err.Error())
				m.signInResult = v.Err.Error()
				m.signInText += "\n" + v.Err.Error()
			} else {
				m.setNotice("Gateway access verified.")
				m.signInResult = "Gateway access verified."
				m.signInText += "\nGateway access verified."
			}
			return m, nil
		}
		return m, m.signIn.next()
	case historyDone:
		if m.historyIssue == nil || m.historyIssue.Key != v.issue.Key {
			return m, nil
		}
		m.historyLoading = false
		if v.err != nil {
			m.setNotice("Cannot load history: " + v.err.Error())
		} else {
			m.historyRows = v.rows
			m.index = 0
			m.selected = ""
		}
	case retryDone:
		if v.err != nil {
			m.setNotice(v.err.Error())
		} else {
			m.setNotice("Retry requested for " + v.key + ". Starts when eligible and a worker slot is free.")
		}
	case tea.KeyMsg:
		if handled, cmd := m.navigation(v); handled {
			return m, cmd
		}
		if (v.String() == "x" && !(m.screen == "signin" && m.signInActive && m.signInPrompt != nil && len(m.signInPrompt.Choices) == 0)) || v.String() == "ctrl+x" {
			m.setNotice("")
			return m, nil
		}
		if m.screen == "signin" {
			switch v.String() {
			case "esc":
				if m.signInActive {
					if m.signInCancel != nil {
						m.signInCancel()
					}
					m.setNotice("Cancelling sign-in…")
				} else {
					m.screen = ""
				}
			case "ctrl+c":
				if m.signInCancel != nil {
					m.signInCancel()
				}
				m.cancel()
				m.exit = 130
				return m, tea.Quit
			case "up", "down":
				if m.signInPrompt != nil && len(m.signInPrompt.Choices) > 0 {
					delta := 1
					if v.String() == "up" {
						delta = -1
					}
					m.signInChoice = (m.signInChoice + delta + len(m.signInPrompt.Choices)) % len(m.signInPrompt.Choices)
				}
			case "pgup":
				m.signInScroll = max(0, m.signInScroll-max(1, m.height-12))
			case "pgdown":
				m.signInScroll += max(1, m.height-12)
			case "enter", "ctrl+j":
				if m.signIn != nil && m.signInActive {
					answer := m.signInInput
					if p := m.signInPrompt; p != nil {
						if len(p.Choices) > 0 {
							answer = p.Choices[m.signInChoice].Value
						} else if answer == "" {
							answer = p.Default
						}
					}
					if _, err := m.signIn.terminal.Write([]byte(answer + "\n")); err != nil {
						m.setNotice("Cannot send sign-in answer: " + err.Error())
					}
					m.signInInput = ""
					m.signInPrompt = nil
					m.signInScroll = 0
				} else if !m.signInActive {
					m.screen = ""
				}
			case "backspace", "ctrl+h":
				r := []rune(m.signInInput)
				if len(r) > 0 {
					m.signInInput = string(r[:len(r)-1])
				}
			default:
				if !m.signInActive && v.String() == "q" {
					m.cancel()
					m.exit = 130
					return m, tea.Quit
				}
				if v.String() == "d" && (m.signInPrompt == nil || len(m.signInPrompt.Choices) > 0) {
					m.signInDiagnostics = !m.signInDiagnostics
					m.signInScroll = 0
					return m, nil
				}
				if p := m.signInPrompt; p != nil && len(p.Choices) > 0 {
					for i, choice := range p.Choices {
						if v.String() == choice.Value {
							m.signInChoice = i
						}
					}
					return m, nil
				}
				if m.signInActive && v.Type == tea.KeyRunes && len(m.signInInput) < 1024 {
					m.signInInput += strings.ReplaceAll(clean(string(v.Runes)), "\n", "")
				}
			}
			m.clampScroll()
			return m, nil
		}
		if m.pendingRetry != nil {
			switch v.String() {
			case "up", "pgup":
				m.confirmScroll = max(0, m.confirmScroll-1)
				return m, nil
			case "down", "pgdown":
				m.confirmScroll++
				m.clampScroll()
				return m, nil
			case "y":
				row := *m.pendingRetry
				m.pendingRetry = nil
				m.setNotice("Requesting retry…")
				return m, func() tea.Msg { return retryDone{row.Key, m.engine.requestRetry(m.ctx, row.Key, row.RunID)} }
			case "n", "esc":
				m.pendingRetry = nil
				m.setNotice("Retry cancelled.")
				return m, nil
			case "q", "ctrl+c": // Preserve detach while the prompt is open.
			default:
				return m, nil
			}
		}
		rows := m.visibleRows()
		switch v.String() {
		case "ctrl+c", "q":
			m.cancel()
			m.exit = 130
			return m, tea.Quit
		case "tab", "c":
			if m.screen == "" && m.historyIssue == nil {
				var prior *Row
				if len(rows) > 0 {
					r := rows[m.index]
					prior = &r
				}
				if v.String() == "tab" {
					m.issueGrouping = !m.issueGrouping
				} else {
					m.showCompleted = !m.showCompleted
				}
				m.restoreSelection(m.selected, prior)
			}
		case "esc":
			if m.screen == "" && m.historyIssue != nil {
				m.selected = m.historyIssue.Key
				m.historyIssue = nil
				m.historyRows = nil
				m.index = 0
				for i, r := range m.visibleRows() {
					if r.Key == m.selected {
						m.index = i
						break
					}
				}
			}
			m.screen = ""
			m.scroll = 0
		case "enter", "l":
			if len(rows) > 0 {
				m.screen = "logs"
				m.scroll = 0
				value, err := tail(rows[m.index].Log, 65536)
				if err != nil {
					m.logText = err.Error()
				} else {
					m.logText = value
				}
			}
		case "H":
			if len(rows) > 0 || m.historyIssue != nil {
				var issue Row
				if m.historyIssue != nil {
					issue = *m.historyIssue
				} else {
					issue = rows[m.index]
				}
				m.historyFilter = ""
				m.historyIssue = &issue
				m.historyLoading = true
				m.historyRows = nil
				m.index = 0
				m.selected = ""
				m.screen = ""
				m.setNotice("")
				return m, func() tea.Msg { rows, err := m.engine.store.history(issue); return historyDone{issue, rows, err} }
			}
		case "r":
			if m.historyIssue != nil {
				m.setNotice("Return to the issue overview to retry its current failed job.")
				break
			}
			if m.state.Monitoring {
				m.setNotice("Request retries from the dispatcher that owns this run.")
			} else if len(rows) > 0 {
				row := rows[m.index]
				if row.Status == "uncertain" {
					if row.RunID == "" {
						m.setNotice("This record has no run ID. Inspect its details before recovery.")
					} else {
						m.pendingRetry = &row
						m.confirmScroll = 0
						m.setNotice("")
					}
				} else if row.Status != "failed" {
					m.setNotice("Only failed jobs can be retried here.")
				} else {
					return m, func() tea.Msg { return retryDone{row.Key, m.engine.requestRetry(m.ctx, row.Key)} }
				}
			}
		case "d":
			m.scroll = 0
			m.screen = "details"
		case "s":
			if m.state.Monitoring {
				m.setNotice("Another dispatcher owns this connection. Sign in from that terminal.")
			} else if m.state.Active > 0 {
				m.setNotice("Workers are running. Gateway changes are available when they finish.")
			} else {
				ctx, cancel := context.WithCancel(m.ctx)
				m.signInCancel = cancel
				m.signInActive = true
				m.signIn = nil
				m.signInText = "Preparing gateway sign-in…\n"
				m.signInPrompt = nil
				m.signInPending = ""
				m.signInResult = ""
				m.signInDiagnostics = false
				m.signInInput = ""
				m.signInScroll = 0
				m.screen = "signin"
				m.setNotice("")
				return m, startSignIn(m.ctx, ctx, m.engine, m.width-8, m.height-12)
			}
		}
	}
	m.restoreSelection(m.selected, nil)
	m.clampScroll()
	return m, nil
}
func (m Model) style(text, color string, bold bool) string {
	if m.plainColor {
		return clean(text)
	}
	st := lipgloss.NewStyle().Bold(bold)
	if !m.plainColor {
		st = st.Foreground(themeColor(color))
	}
	return st.Render(clean(text))
}
func statusColor(status string) string {
	switch status {
	case "failed":
		return "1"
	case "uncertain":
		return "3"
	case "succeeded":
		return "2"
	default:
		return "6"
	}
}

func (m Model) workHeader(row Row, width int, section string) []string {
	status := rowStatus(row)
	if row.Status == "failed" && row.Activity != "history" {
		status = "Failed · " + status
	}
	return []string{
		m.style("Runs  /  "+section, "8", false),
		m.style(middle(title(row), width), "7", true),
		m.style("Agent  ", "8", false) + m.style(middle(row.Agent, max(1, width-7)), "7", false),
		m.style(middle(status, width), statusColor(row.Status), true),
		"",
	}
}

func (m Model) wrapped(text string, width int, color string) []string {
	// Sanitize external text before styling; never strip ANSI from styled lines.
	wrapped := ansi.Hardwrap(clean(text), width, true)
	lines := strings.Split(wrapped, "\n")
	for i := range lines {
		lines[i] = m.style(lines[i], color, false)
	}
	return lines
}

func sameIssue(a, b Row) bool { return a.Scope == b.Scope && a.Repo == b.Repo && a.Item == b.Item }
func (m Model) listTitle(r Row) string {
	if m.historyIssue != nil {
		return "#" + r.Item + "  " + time.Unix(int64(r.Started), 0).Local().Format("Jan 02 15:04:05")
	}
	return title(r)
}
func (m Model) firstColumn() string {
	if m.historyIssue != nil {
		return "STARTED (LOCAL)"
	}
	return "ISSUE"
}

func (m Model) agentGroups(rows []Row, w, budget int) []string {
	if len(rows) == 0 {
		if m.currentFilter() != "" {
			return []string{"No matching runs. Press / to edit the filter; Esc clears it."}
		}
		for _, r := range m.scopedRows() {
			if r.Status == "succeeded" {
				return []string{"No active work. Press c to show completed runs."}
			}
		}
		if m.state.Busy {
			return []string{"Finding issues in Jira and GitHub…"}
		}
		return []string{"Waiting for eligible issues"}
	}
	outerWidth := w
	w -= 2
	budget = max(1, budget-1) // Reserve space for the visible range.
	lines := []string{}
	start := max(0, m.index-max(1, budget/4)+1)
	last := ""
	end := start
	for i := start; i < len(rows); i++ {
		r := rows[i]
		entry := []string{}
		if r.Agent != last {
			if last != "" {
				entry = append(entry, "")
			}
			running, failed, done := 0, 0, 0
			for _, item := range rows {
				if item.Agent == r.Agent {
					switch item.Status {
					case "running":
						running++
					case "failed", "uncertain":
						failed++
					case "succeeded":
						done++
					}
				}
			}
			counts := []string{}
			if running > 0 {
				counts = append(counts, fmt.Sprintf("%d running", running))
			}
			if failed > 0 {
				counts = append(counts, fmt.Sprintf("%d need attention", failed))
			}
			if done > 0 {
				counts = append(counts, fmt.Sprintf("%d completed", done))
			}
			entry = append(entry, m.groupHeading(r.Agent, strings.Join(counts, " · "), w))
		}
		prefix := "  "
		if i == m.index {
			prefix = "> "
		}
		if w >= 70 {
			entry = append(entry, m.tableRow(r, prefix, w, false))
		} else {
			entry = append(entry, prefix+middle(title(r), w-2), "  "+m.style(middle(tableStatus(r)+" · "+durationLabel(r), w-2), statusColor(r.Status), false))
		}

		if i == m.index {
			first := len(entry) - 1
			if w < 70 {
				first--
			}
			for n := first; n < len(entry); n++ {
				entry[n] = m.selection(entry[n], w)
			}
		}
		if len(lines)+len(entry) > budget && len(lines) > 0 {
			break
		}
		lines = append(lines, entry...)
		last = r.Agent
		end = i + 1
	}
	lines = m.withScrollbar(lines, start, end, len(rows), budget, outerWidth)
	if start > 0 || end < len(rows) {
		lines = append(lines, m.style(fmt.Sprintf("Runs %d-%d of %d · ↑↓ to browse", start+1, end, len(rows)), "8", false))
	}
	return lines
}

func (m Model) detailContent(rows []Row, w int) ([]string, []string) {
	header := []string{}
	body := []string{}
	if len(rows) > 0 {
		r := rows[m.index]
		header = append(header, m.workHeader(r, w, "Details")...)
		if r.Status == "uncertain" {
			body = append(body, m.style("OUTCOME NOT RECORDED", "3", true))
			body = append(body, m.wrapped("The local worker stopped without saving its result. Check this log and confirm the remote sandbox has stopped before retrying.", w, "7")...)
			body = append(body, m.wrapped("Press r to confirm and retry here, or restart with --retry-uncertain "+r.Item+":"+r.Agent, w, "6")...)
			body = append(body, "")
		}
		if r.Error != "" {
			body = append(body, m.style("Last attempt", "8", true))
			message := strings.TrimSuffix(r.Error, "; see "+r.Log)
			body = append(body, m.wrapped(message, w, "1")...)
			body = append(body, "")
		}
		body = append(body, m.style("Log files", "8", true))
		body = append(body, m.style("Agent log", "6", false))
		body = append(body, m.wrapped(r.Log, w, "7")...)
	} else {
		header = append(header, m.style("Work / Details", "8", false), "")
	}
	body = append(body, "", m.style("Dispatcher log", "6", false))
	body = append(body, m.wrapped(filepath.Join(m.engine.c.LogDir, "dispatcher.log"), w, "7")...)
	if m.state.RecoveryError != "" {
		body = append(body, "", m.style("Last sign-in", "3", true))
		body = append(body, m.wrapped(m.state.RecoveryError, w, "1")...)
	}
	if len(m.state.Errors) > 0 || m.state.Message != "" || m.state.ConnectionError != "" {
		body = append(body, "", m.style("Needs attention", "3", true))
		if m.state.Message != "" {
			body = append(body, m.wrapped(m.state.Message, w, "3")...)
		}
		if m.state.ConnectionError != "" {
			body = append(body, m.wrapped(m.state.ConnectionError, w, "1")...)
		}
		for _, problem := range m.state.Errors {
			body = append(body, m.wrapped(problem, w, "3")...)
		}
	}
	return header, body
}
func (m Model) logPageSize() int {
	return max(1, m.height-2-11-3)
}
func (m *Model) clampScroll() {
	if m.screen == "signin" {
		text := strings.Join(m.signInBody(max(10, m.width-6)), "\n")
		total := len(strings.Split(ansi.Hardwrap(text, max(10, m.width-6), true), "\n"))
		m.signInScroll = min(m.signInScroll, max(0, total-max(1, m.height-13)))
	}
	if m.pendingRetry != nil {
		total := len(m.retryPrompt(max(12, m.width-4)))
		m.confirmScroll = min(m.confirmScroll, max(0, total-max(1, m.height-11)))
	}
	if m.screen == "logs" {
		m.scroll = min(m.scroll, max(0, len(strings.Split(m.logText, "\n"))-m.logPageSize()))
	}
	if m.screen == "details" {
		header, body := m.detailContent(m.visibleRows(), max(12, m.width-4))
		count := max(1, m.height-2-4-len(header)-3)
		m.scroll = min(m.scroll, max(0, len(body)-count))
	}
}
func (m Model) withScrollbar(content []string, start, end, total, height, width int) []string {
	lines := make([]string, 0, height)
	visible := max(1, end-start)
	thumbSize := min(height, max(1, height*visible/max(1, total)))
	thumbStart := 0
	if total > visible {
		thumbStart = start * (height - thumbSize) / (total - visible)
	}
	for i := 0; i < height; i++ {
		line := ""
		if i < len(content) {
			line = content[i]
		}
		marker := " "
		if total > visible {
			marker = m.style("│", "scroll-track", false)
			if i >= thumbStart && i < thumbStart+thumbSize {
				marker = m.style("█", "7", true)
			}
		}
		lines = append(lines, lipgloss.NewStyle().Width(width-2).Render(ansi.Truncate(line, width-2, "…"))+" "+marker)
	}
	return lines
}
func (m Model) scrollPane(content []string, start, count, width int) []string {
	end := min(len(content), start+count)
	return m.withScrollbar(content[start:end], start, end, len(content), count, width)
}

func (m Model) scrollIndicator(start, end, total int) string {
	above, below := "Top", "End"
	if start > 0 {
		above = "↑ more"
	}
	if end < total {
		below = "↓ more"
	}
	return m.style(fmt.Sprintf("%s   Lines %d-%d of %d   %s", above, min(start+1, total), end, total, below), "8", false)
}

func (m Model) retryPrompt(width int) []string {
	if m.pendingRetry == nil {
		return nil
	}
	r := *m.pendingRetry
	body := m.wrapped(title(r), width, "7")
	body = append(body, m.style("Agent  "+r.Agent, "6", false), "")
	body = append(body, m.wrapped("Its result was not saved. The remote sandbox could still be running. Check the log and verify that sandbox has stopped before confirming.", width, "7")...)
	body = append(body, "", m.style("y  I verified it stopped; retry", "3", true))
	return body
}

func (m Model) View() string {
	w := max(12, m.width-4)
	h := max(3, m.height-2)
	header := m.style("▌ ", "brand", true) + m.style("FULLSEND", "7", true) + "  " + middle(strings.TrimPrefix(m.engine.c.Scope, "agent/scope/"), max(4, w-30))
	counter := fmt.Sprintf("%d/%d running", m.state.Active, m.engine.c.Concurrency)
	gap := max(1, w-lipgloss.Width(header)-len(counter))
	header += strings.Repeat(" ", gap) + counter
	connection := m.state.Connection
	if m.state.Monitoring {
		connection = "Watching another run"
	}
	connectionColor := "2"
	if connection != "Connected" {
		connectionColor = "3"
	}
	lines := []string{header, m.style(middle(m.state.Gateway, w-24), "8", false) + " · " + m.style(connection, connectionColor, false), m.rule(w), ""}
	if m.helpOpen {
		lines = append(lines, m.style("Keyboard shortcuts", "7", true), "")
		content := strings.Split(ansi.Hardwrap(shortcutHelp, w-2, true), "\n")
		for i, line := range content {
			if line != "" && line == strings.ToUpper(line) {
				content[i] = m.style(line, "6", true)
			}
		}
		count := max(1, h-len(lines)-3)
		start := min(m.helpScroll, max(0, len(content)-count))
		lines = append(lines, m.scrollPane(content, start, count, w)...)
		lines = append(lines, m.scrollIndicator(start, min(len(content), start+count), len(content)), "", m.style("j/k scroll   h / esc / ? close   q detach", "8", false))
		return m.frame(lines, w, h)
	}
	if m.screen == "signin" {
		lines = append(lines, m.style("Gateway  /  Sign in", "7", true), "")

		body := strings.Join(m.signInBody(w-2), "\n")
		content := strings.Split(ansi.Hardwrap(body, w-2, true), "\n")
		count := max(1, h-len(lines)-3)
		start := min(m.signInScroll, max(0, len(content)-count))
		// Keep the selected menu choice visible on short terminals.
		if p := m.signInPrompt; p != nil && !m.signInDiagnostics && len(p.Choices) > 0 {
			target := 0
			for i, line := range content {
				if strings.HasPrefix(clean(line), "> ") || strings.HasPrefix(clean(line), "▎ ") {
					target = i
					break
				}
			}
			start = max(0, min(len(content)-count, target+3-count))
		}
		lines = append(lines, m.scrollPane(content, start, count, w)...)
		lines = append(lines, m.scrollIndicator(start, min(len(content), start+count), len(content)), "")
		footer := "j/k choose   Enter continue   d diagnostics   ? help   esc cancel"
		if m.signInPrompt == nil {
			footer = "d diagnostics   PgUp/PgDn scroll   esc cancel"
		}
		if m.signInPrompt != nil && len(m.signInPrompt.Choices) == 0 {
			footer = "Type a name   Enter continue   esc cancel"
		}
		if !m.signInActive {
			footer = "Enter / esc back   d diagnostics   PgUp/PgDn scroll   q detach"
		}
		lines = append(lines, m.style(footer, "8", false))
		return m.frame(lines, w, h)
	}
	if m.pendingRetry != nil {
		lines = append(lines, m.style("Retry an unknown outcome?", "3", true), "")
		body := m.retryPrompt(w)
		count := max(1, h-len(lines)-3)
		start := min(m.confirmScroll, max(0, len(body)-count))
		lines = append(lines, m.scrollPane(body, start, count, w)...)
		lines = append(lines, m.scrollIndicator(start, min(len(body), start+count), len(body)), "", m.style("y retry  n cancel  ↑↓ scroll  q detach", "8", false))
		return m.frame(lines, w, h)
	}

	rows := m.visibleRows()
	if m.screen == "logs" && len(rows) > 0 {
		row := rows[m.index]
		lines = append(lines, m.workHeader(row, w, "Logs")...)
		lines = append(lines, m.style(middle(row.Log, w), "8", false), "")
		content := strings.Split(m.logText, "\n")
		count := max(1, h-len(lines)-3)
		end := max(min(len(content), count), len(content)-m.scroll)
		start := max(0, end-count)
		lines = append(lines, m.scrollPane(content, start, count, w)...)
		lines = append(lines, m.scrollIndicator(start, end, len(content)))
		lines = append(lines, "", m.style("j/k scroll   d details   r retry   h back   ? help   q detach", "8", false))
	} else if m.screen == "details" {
		header, body := m.detailContent(rows, w)
		lines = append(lines, header...)
		count := max(1, h-len(lines)-3)
		start := min(m.scroll, max(0, len(body)-count))
		lines = append(lines, m.scrollPane(body, start, count, w)...)
		lines = append(lines, m.scrollIndicator(start, min(len(body), start+count), len(body)))
		lines = append(lines, "", m.style("j/k scroll   enter logs   r retry   h back   ? help   q detach", "8", false))
	} else {
		if m.state.Monitoring {
			lines = append(lines, "Existing workers are finishing. Scheduling resumes automatically.", "")
		} else if connection == "Connection lost" {
			lines = append(lines, m.style("New work paused. Retrying the connection; s to sign in.", "3", false), "")
		}
		if m.historyIssue != nil {
			lines = append(lines, m.style(middle(title(*m.historyIssue), w), "7", true), m.style("Run history · up to 200 attempts · H refresh · esc back", "8", false), "")
		}
		if m.historyIssue == nil && !m.issueGrouping {
			lines = append(lines, m.viewTabs(false, w)...)
			if controls := m.listControls(); controls != "" {
				lines = append(lines, m.style(controls, "8", false))
			}
			if m.height > 24 {
				lines = append(lines, "")
			}
			lines = append(lines, m.tableHeading(w-2, false))
			lines = append(lines, m.agentGroups(rows, w, max(1, h-len(lines)-6))...)
		} else {
			if m.historyIssue == nil {
				lines = append(lines, m.viewTabs(true, w)...)
			}
			if controls := m.listControls(); controls != "" {
				lines = append(lines, m.style(controls, "8", false))
			}
			if m.height > 24 {
				lines = append(lines, "")
			}
			lines = append(lines, m.tableHeading(w-2, true))
			paneHeight := max(1, h-len(lines)-6)
			count := paneHeight
			if w < 100 {
				count = max(1, count/2)
			}
			start := max(0, m.index-count+1)
			end := min(len(rows), start+count)
			listStart := len(lines)
			for i := start; i < end; i++ {
				r := rows[i]
				prefix := "  "
				if i == m.index {
					prefix = "> "
				}
				color := statusColor(r.Status)
				if w >= 100 {
					line := m.tableRow(r, prefix, w-2, true)
					if i == m.index {
						line = m.selection(line, w)
					}
					lines = append(lines, line)
				} else {
					lines = append(lines, prefix+middle(m.listTitle(r), w-2), "  "+m.style(middle(agentName(r.Agent)+" · "+tableStatus(r)+" · "+durationLabel(r), w-2), color, false))
					if i == m.index {
						lines[len(lines)-2] = m.selection(lines[len(lines)-2], w-2)
						lines[len(lines)-1] = m.selection(lines[len(lines)-1], w-2)
					}
				}
			}
			if len(rows) > 0 {
				lines = append(lines[:listStart], m.withScrollbar(lines[listStart:], start, end, len(rows), paneHeight, w)...)
			}
			if len(rows) == 0 {
				message := "Waiting for eligible issues"
				if m.currentFilter() != "" {
					message = "No matching runs. Press / to edit the filter; Esc clears it."
				}
				if m.historyIssue != nil {
					message = "No recorded attempts"
					if m.historyLoading {
						message = "Loading history…"
					}
				}
				if m.state.Busy {
					message = "Finding issues in Jira and GitHub…"
				}
				if m.currentFilter() != "" {
					message = "No matching runs. Press / to edit the filter; Esc clears it."
				}
				lines = append(lines, message)
			}
			if len(rows) > count {
				lines = append(lines, m.style(fmt.Sprintf("%d-%d of %d", start+1, end, len(rows)), "8", false))
			}
		}
		summary := ""
		if m.historyIssue == nil {
			completed := 0
			for _, r := range m.scopedRows() {
				if r.Status == "succeeded" {
					completed++
				}
			}
			action := "show"
			if m.showCompleted {
				action = "hide"
			}
			summary = m.style(fmt.Sprintf("%d completed runs · c to %s", completed, action), "8", false)
		}
		if len(m.state.Errors) > 0 {
			lines = append(lines, m.style(fmt.Sprintf("%d problem(s) need attention. Press d for details.", len(m.state.Errors)), "3", false))
		}
		// The list summary belongs to the bottom of the viewport, not the
		// last rendered row. Reserve it before the shared frame pads the body.
		if summary != "" {
			target := max(0, h-3)
			if len(lines) > target {
				lines = lines[:target]
			}
			for len(lines) < target {
				lines = append(lines, "")
			}
			lines = append(lines, summary)
		}
		footer := "j/k select  enter logs  / filter  o sort  O reverse  H history  ? help  q detach"
		if m.historyIssue != nil {
			footer = "j/k select  enter logs  / filter  o sort  O reverse  H refresh  h back  ? help"
		}
		lines = append(lines, "", m.style(footer, "8", false))
	}
	return m.frame(lines, w, h)
}

func (m Model) frame(lines []string, w, h int) string {
	if len(lines) >= 2 {
		footer := m.footerHints(lines[len(lines)-1])
		body := lines[:len(lines)-2]
		if len(body) > h-2 {
			body = body[:max(0, h-2)]
		}
		for len(body) < h-2 {
			body = append(body, "")
		}
		lines = append(body, m.rule(w), footer)
	}
	// Every rendered line stays inside the terminal, including narrow windows.
	for i, line := range lines {
		lines[i] = ansi.Truncate(line, w, "")
	}
	lines = m.overlayNotice(lines, w, h)
	frame := lipgloss.NewStyle().Padding(1, 2)
	if !m.plainColor {
		frame = frame.Foreground(themeColor("7")).Background(themeColor("canvas"))
	}
	return frame.Render(strings.Join(lines, "\n"))
}
