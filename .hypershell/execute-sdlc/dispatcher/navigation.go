package main

import (
	"sort"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

const shortcutHelp = `NAVIGATION
j / k or arrows    Move down / up
gg / G            First / last item or top / bottom
Ctrl-d / Ctrl-u    Half page down / up
Ctrl-f / Ctrl-b    Full page down / up (also PgDn / PgUp)
Enter / l         Open selected run
Esc / h           Back

WORK LISTS AND HISTORY
/                 Filter by issue, title, agent or status
Enter             Keep filter
Esc               Cancel editing; clear an applied filter
n / N             Next / previous filtered result
o                 Cycle sort column left to right: issue, status, duration
O                 Reverse sort direction (arrow in column heading)
Tab               Switch Agents / Issues
c                 Show / hide completed runs
H                 Open or refresh history

RUN ACTIONS
d                 Details (diagnostics during sign-in)
r                 Retry selected run
s                 Gateway sign-in
x                 Dismiss notification (Ctrl-x in text fields)
?                 Show / close this help
q / Ctrl-c       Detach; workers keep running

Text fields accept normal typing. Esc cancels editing.
Agent groups stay together; sorting applies within each group.
Filtering does not reveal hidden completed runs: press c to include them.
Logs and details scroll with the navigation keys; / filters work lists only.`

func (m Model) currentFilter() string {
	if m.historyIssue != nil {
		return m.historyFilter
	}
	return m.filter
}
func (m *Model) setFilter(value string) {
	if m.historyIssue != nil {
		m.historyFilter = value
	} else {
		m.filter = value
	}
	m.index = 0
	m.selected = ""
	m.restoreSelection("", nil)
}
func (m Model) filterRows(rows []Row) []Row {
	terms := strings.Fields(strings.ToLower(m.currentFilter()))
	out := make([]Row, 0, len(rows))
	for _, r := range rows {
		haystack := strings.ToLower(title(r) + " " + r.Item + " " + r.Agent + " " + r.Status + " " + rowStatus(r))
		match := true
		for _, term := range terms {
			if !strings.Contains(haystack, term) {
				match = false
				break
			}
		}
		if match || ((m.screen == "logs" || m.screen == "details") && r.Key == m.selected) {
			out = append(out, r)
		}
	}
	return out
}
func (m Model) rowLess(a, b Row) bool {
	cmp := 0
	switch m.sortMode {
	case 1:
		ai, ae := strconv.Atoi(a.Item)
		bi, be := strconv.Atoi(b.Item)
		if (ae == nil) != (be == nil) {
			return ae == nil
		}
		if ai < bi {
			cmp = -1
		} else if ai > bi {
			cmp = 1
		}
	case 2:
		now := seconds()
		ad, bd := runDurationAt(a, now), runDurationAt(b, now)
		if (ad >= 0) != (bd >= 0) {
			return ad >= 0
		}
		if ad < bd {
			cmp = -1
		} else if ad > bd {
			cmp = 1
		}
	default:
		if rank(a.Status) < rank(b.Status) {
			cmp = -1
		} else if rank(a.Status) > rank(b.Status) {
			cmp = 1
		}
	}
	if cmp == 0 {
		return a.Started > b.Started
	}
	if m.sortDescending {
		return cmp > 0
	}
	return cmp < 0
}
func (m Model) orderRows(rows []Row) []Row {
	sort.SliceStable(rows, func(i, j int) bool { return m.rowLess(rows[i], rows[j]) })
	return rows
}
func (m Model) sortHeading(label string, mode int) string {
	if m.sortMode != mode {
		return label
	}
	arrow := " ↑"
	if m.sortDescending {
		arrow = " ↓"
	}
	return label + arrow
}
func runDuration(r Row) float64 { return runDurationAt(r, seconds()) }
func runDurationAt(r Row, now float64) float64 {
	if r.Started <= 0 {
		return -1
	}
	if r.Status == "running" {
		return max(0, now-r.Started)
	}
	if r.Finished <= 0 {
		return -1
	}
	return max(0, r.Finished-r.Started)
}
func durationLabel(r Row) string {
	if runDuration(r) < 0 {
		return "-"
	}
	return duration(runDuration(r))
}
func tableStatus(r Row) string {
	if r.Status == "running" {
		if r.Activity != "" && r.Activity != "history" {
			return r.Activity
		}
		return "Running"
	}
	return rowStatus(r)
}
func (m Model) listControls() string {
	text := ""
	if m.filtering {
		return "/ " + m.currentFilter() + "▏  Enter keep · Esc cancel"
	}
	if m.currentFilter() != "" {
		text = "Filter: " + m.currentFilter() + " · Esc clear"
	}
	return text
}

// Handle shared navigation before view-specific actions. Text entry always wins.
func (m *Model) navigation(v tea.KeyMsg) (bool, tea.Cmd) {
	key := v.String()
	if m.filtering {
		switch key {
		case "ctrl+c":
			m.cancel()
			m.exit = 130
			return true, tea.Quit
		case "ctrl+x":
			m.setNotice("")
		case "enter":
			m.filtering = false
		case "esc":
			m.setFilter(m.filterBefore)
			m.filtering = false
		case "backspace", "ctrl+h":
			r := []rune(m.currentFilter())
			if len(r) > 0 {
				m.setFilter(string(r[:len(r)-1]))
			}
		case "ctrl+u":
			m.setFilter("")
		default:
			if v.Type == tea.KeyRunes {
				m.setFilter(m.currentFilter() + strings.ReplaceAll(clean(string(v.Runes)), "\n", ""))
			}
		}
		return true, nil
	}
	textInput := m.screen == "signin" && m.signInActive && m.signInPrompt != nil && len(m.signInPrompt.Choices) == 0
	if textInput {
		return false, nil
	}
	if key == "?" {
		m.helpOpen = !m.helpOpen
		m.helpScroll = 0
		return true, nil
	}
	if m.helpOpen && (key == "x" || key == "ctrl+x") {
		m.setNotice("")
		return true, nil
	}
	if m.helpOpen && (key == "esc" || key == "h") {
		m.helpOpen = false
		return true, nil
	}
	if m.helpOpen && (key == "q" || key == "ctrl+c") {
		m.cancel()
		m.exit = 130
		return true, tea.Quit
	}
	list := m.screen == "" && m.pendingRetry == nil && !m.helpOpen
	if list {
		switch key {
		case "/":
			m.filterBefore = m.currentFilter()
			m.filtering = true
			return true, nil
		case "esc":
			if m.currentFilter() != "" {
				m.setFilter("")
				return true, nil
			}
		case "o":
			// Columns are Issue (1), Status (0), Duration (2).
			m.sortMode = [3]int{2, 0, 1}[m.sortMode]
			m.sortDescending = false
			m.restoreSelection(m.selected, nil)
			return true, nil
		case "O":
			m.sortDescending = !m.sortDescending
			m.restoreSelection(m.selected, nil)
			return true, nil
		case "n":
			m.moveNavigation(1)
			return true, nil
		case "N":
			m.moveNavigation(-1)
			return true, nil
		}
	}
	delta := 0
	switch key {
	case "g":
		if !m.gPending.IsZero() && time.Since(m.gPending) < time.Second {
			delta = -1000000
			m.gPending = time.Time{}
		} else {
			m.gPending = time.Now()
			return true, nil
		}
	case "G", "end":
		delta = 1000000
	case "home":
		delta = -1000000
	case "j", "down":
		delta = 1
	case "k", "up":
		delta = -1
	case "ctrl+d":
		delta = max(1, (m.height-12)/2)
	case "ctrl+u":
		delta = -max(1, (m.height-12)/2)
	case "ctrl+f", "pgdown":
		delta = max(1, m.height-12)
	case "ctrl+b", "pgup":
		delta = -max(1, m.height-12)
	default:
		m.gPending = time.Time{}
	}
	if delta != 0 {
		m.moveNavigation(delta)
		return true, nil
	}
	if m.helpOpen {
		return true, nil
	}
	// Reuse existing handlers for back and opening native sign-in choices.
	if key == "h" {
		_, cmd := m.updateAlias(tea.KeyEsc)
		return true, cmd
	}
	if key == "l" && m.screen == "signin" {
		_, cmd := m.updateAlias(tea.KeyEnter)
		return true, cmd
	}
	return false, nil
}
func (m *Model) updateAlias(key tea.KeyType) (bool, tea.Cmd) {
	updated, cmd := m.Update(tea.KeyMsg{Type: key})
	*m = updated.(Model)
	return true, cmd
}
func (m *Model) moveNavigation(delta int) {
	switch {
	case m.helpOpen:
		total := len(strings.Split(ansi.Hardwrap(shortcutHelp, max(10, m.width-6), true), "\n"))
		m.helpScroll = max(0, min(max(0, total-max(1, m.height-11)), m.helpScroll+delta))
		return
	case m.pendingRetry != nil:
		m.confirmScroll = max(0, m.confirmScroll+delta)
	case m.screen == "signin":
		if m.signInPrompt != nil && len(m.signInPrompt.Choices) > 0 && !m.signInDiagnostics {
			m.signInChoice = max(0, min(len(m.signInPrompt.Choices)-1, m.signInChoice+delta))
		} else {
			m.signInScroll = max(0, m.signInScroll+delta)
		}
	case m.screen == "logs":
		m.scroll = max(0, m.scroll-delta)
	case m.screen == "details":
		m.scroll = max(0, m.scroll+delta)
	default:
		rows := m.visibleRows()
		m.index = max(0, min(len(rows)-1, m.index+delta))
		if len(rows) > 0 {
			m.selected = rows[m.index].Key
		}
	}
	m.clampScroll()
}

func (m Model) tableHeading(width int, withAgent bool) string {
	if (!withAgent && width < 70) || (withAgent && width < 98) {
		return m.style(m.sortHeading("Issue", 1)+" · "+m.sortHeading("Status", 0)+" · "+m.sortHeading("Duration", 2), "8", true)
	}
	issueWidth := width - 38
	agent := ""
	if withAgent {
		issueWidth -= 25
		agent = cell("Agent", 24) + " "
	}
	return m.style("  "+cell(m.sortHeading("Issue", 1), issueWidth)+" "+agent+cell(m.sortHeading("Status", 0), 24)+" "+cell(m.sortHeading("Duration", 2), 10), "8", true)
}
func cell(text string, width int) string {
	text = middle(text, width)
	return text + strings.Repeat(" ", max(0, width-ansi.StringWidth(text)))
}
func (m Model) tableRow(r Row, prefix string, width int, withAgent bool) string {
	issueWidth := width - 38
	agent := ""
	if withAgent {
		issueWidth -= 25
		agent = cell(r.Agent, 24) + " "
	}
	return prefix + cell(m.listTitle(r), issueWidth) + " " + agent + m.style(cell(tableStatus(r), 24), statusColor(r.Status), false) + " " + m.style(cell(durationLabel(r), 10), "8", false)
}
