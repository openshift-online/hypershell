package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const (
	// headerLines is the title, tabs, and view summary; footerLines is the
	// status line and key hints.
	headerLines = 3
	footerLines = 2
)

func (m *Model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}
	if m.width < MinWidth || m.height < MinHeight {
		return fmt.Sprintf("Terminal is %dx%d. hsctl ui needs at least %dx%d.\nEnlarge the terminal or press q to quit.",
			m.width, m.height, MinWidth, MinHeight)
	}
	sections := []string{m.titleLine(), m.tabsLine(), m.summaryLine()}
	bodyHeight := m.height - headerLines - footerLines
	body := m.body(bodyHeight)
	lines := strings.Split(body, "\n")
	for len(lines) < bodyHeight {
		lines = append(lines, "")
	}
	sections = append(sections, strings.Join(lines[:bodyHeight], "\n"))
	sections = append(sections, m.statusLine(), m.footerLine())
	return strings.Join(sections, "\n")
}

func (m *Model) titleLine() string {
	parts := []string{titleStyle.Render("hsctl ui"), m.opts.APIURL}
	if m.opts.Identity != "" {
		parts = append(parts, "user: "+m.opts.Identity)
	}
	return fit(strings.Join(parts, "  "), m.width)
}

func (m *Model) tabsLine() string {
	tabs := make([]string, 0, len(kinds))
	for i, k := range kinds {
		label := fmt.Sprintf("%d %s", i+1, k.Title())
		if k == m.active {
			tabs = append(tabs, activeTab.Render(label))
		} else {
			tabs = append(tabs, tabStyle.Render(label))
		}
	}
	return fit(strings.Join(tabs, " "), m.width)
}

func (m *Model) summaryLine() string {
	v := m.views[m.active]
	var parts []string
	title := m.active.Title()
	if m.mode == modeDetail && m.detail != nil {
		title += " / " + m.detail.name
	}
	parts = append(parts, titleStyle.Render(title))
	rows := m.visibleRows(v)
	if v.filter != "" {
		parts = append(parts, fmt.Sprintf("%d of %d match %q", len(rows), len(v.items), v.filter))
	} else if v.loaded {
		parts = append(parts, plural(len(v.items), "item"))
	}
	if v.truncated {
		parts = append(parts, warnStyle.Render(fmt.Sprintf("truncated at %d", PageSize*MaxPages)))
	}
	switch {
	case m.expired:
		parts = append(parts, errorStyle.Render(SessionExpiredMessage))
	case v.failures > 0 && v.loaded:
		parts = append(parts, warnStyle.Render(fmt.Sprintf("STALE, last updated %s ago: %s", age(m.now(), v.lastSuccess), ReasonOf(v.lastErr))))
	case v.failures > 0:
		parts = append(parts, errorStyle.Render("unavailable: "+ReasonOf(v.lastErr)))
	case !v.loaded:
		parts = append(parts, mutedStyle.Render("loading..."))
	default:
		parts = append(parts, mutedStyle.Render(fmt.Sprintf("updated %s ago", age(m.now(), v.lastSuccess))))
	}
	return fit(strings.Join(parts, "  "), m.width)
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

func (m *Model) tableHeight() int {
	return max(m.height-headerLines-footerLines-1, 1)
}

func (m *Model) body(height int) string {
	switch m.mode {
	case modeDetail:
		return m.detailView(height)
	case modeCreate:
		return panelStyle.Width(min(m.width-2, 100)).Render(m.form.view(min(m.width-6, 96)))
	case modeDelete:
		return panelStyle.Width(min(m.width-2, 80)).Render(m.confirm.view())
	case modeHelp:
		return helpText
	}
	return m.tableView(height)
}

func (m *Model) tableView(height int) string {
	v := m.views[m.active]
	rows := m.visibleRows(v)
	if len(rows) == 0 {
		switch {
		case !v.loaded:
			return ""
		case v.filter != "":
			return mutedStyle.Render("No rows match the filter. Press esc to clear it.")
		case m.active == KindGateways:
			return mutedStyle.Render("No gateways. Press n to provision one.")
		}
		return mutedStyle.Render("No " + strings.ToLower(m.active.Title()) + ".")
	}
	visible := max(height-1, 1)
	if v.cursor < v.offset {
		v.offset = v.cursor
	}
	if v.cursor >= v.offset+visible {
		v.offset = v.cursor - visible + 1
	}
	v.offset = max(0, min(v.offset, len(rows)-visible))
	end := min(len(rows), v.offset+visible)

	cols := columnsFor(m.active)
	cells := make([][]string, 0, end-v.offset)
	for _, r := range rows[v.offset:end] {
		row := make([]string, len(cols))
		for i, c := range cols {
			row[i] = c.cell(m, r)
		}
		cells = append(cells, row)
	}
	return strings.Join(renderTable(cols, cells, v.cursor-v.offset, m.width), "\n")
}

func (m *Model) detailView(height int) string {
	d := m.detail
	if d.gone {
		return errorStyle.Render(fmt.Sprintf("%s no longer exists.", d.name)) + "\n\n" + mutedStyle.Render("Press esc to return.")
	}
	d.viewport.Width = m.width
	d.viewport.Height = height - 1
	note := ""
	if d.err != "" {
		note = warnStyle.Render("Showing last known state: " + d.err)
	}
	return note + "\n" + d.viewport.View()
}

func (m *Model) statusLine() string {
	switch m.mode {
	case modeFilter, modeCommand:
		return fit(m.input.View(), m.width)
	}
	if m.status == "" {
		return ""
	}
	style := okStyle
	if m.statusErr {
		style = errorStyle
	}
	return style.Render(fit(m.status, m.width))
}

func (m *Model) footerLine() string {
	var hints string
	switch m.mode {
	case modeFilter:
		hints = "type to filter  enter keep  esc clear"
	case modeCommand:
		hints = ":gw :mc :rel :net :q  enter run  esc cancel"
	case modeDetail:
		hints = "up/down scroll  y copy ID  r refresh  esc back"
	case modeCreate:
		hints = "tab next field  up/down choose  type to filter  enter provision  ctrl+r retry lists  esc cancel"
	case modeDelete:
		hints = "type the name  enter delete  esc cancel"
	case modeHelp:
		hints = "any key to close"
	default:
		hints = "1-4 view  enter details  / filter  : command  r refresh  ? help  q quit"
		if m.active == KindGateways {
			hints = "1-4 view  enter details  n new  d delete  / filter  : command  r refresh  ? help  q quit"
		}
	}
	return mutedStyle.Render(fit(hints, m.width))
}

var helpText = lipgloss.JoinVertical(lipgloss.Left,
	titleStyle.Render("Keys"),
	"",
	"  1 2 3 4          Gateways, Managed Clusters, Gateway Releases, Gateway Networks",
	"  :gw :mc :rel :net  same, from the command bar; :q quits",
	"  up/down j/k      move the selection; pgup/pgdown, g/G jump",
	"  enter            open the selected resource",
	"  /                filter by name or ID (literal, case-insensitive); esc clears",
	"  r                refresh now",
	"  n                provision a gateway (Gateways view)",
	"  d                delete the selected gateway (Gateways view)",
	"  y                copy the resource ID (detail view)",
	"  esc              back",
	"  q, ctrl+c        quit",
	"",
	"Data refreshes automatically; after a failure the last data stays on screen",
	"marked STALE while retries back off to at most one per minute.",
)
