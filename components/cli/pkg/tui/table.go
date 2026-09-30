package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const (
	hubClusterLabel     = "Hub cluster"
	defaultReleaseLabel = "default"
	unresolvedSuffix    = " (unresolved)"
	resolvingLabel      = "..."
)

// column describes one table column: its header and how a cell is derived
// from a resource. phase marks the column whose cells get phase styling.
type column struct {
	title string
	cell  func(m *Model, r Resource) string
	phase bool
}

func field(name string) func(*Model, Resource) string {
	return func(_ *Model, r Resource) string { return r.Field(name) }
}

func columnsFor(kind Kind) []column {
	name := column{title: "NAME", cell: func(_ *Model, r Resource) string { return r.DisplayName() }}
	age := column{title: "AGE", cell: func(m *Model, r Resource) string { return age(m.now(), r.CreatedAt) }}
	status := column{title: "STATUS", cell: field("status")}
	switch kind {
	case KindClusters:
		return []column{name,
			{title: "PROVIDER", cell: field("provider")},
			{title: "REGION", cell: field("region")},
			status,
			{title: "LAST SEEN", cell: func(m *Model, r Resource) string { return sinceField(m.now(), r.Field("last_seen_at")) }},
			age}
	case KindReleases:
		return []column{name,
			{title: "IMAGE", cell: field("image")},
			{title: "ROLLOUT STRATEGY", cell: field("rollout_strategy")},
			status, age}
	case KindNetworks:
		return []column{name,
			{title: "TOPOLOGY", cell: field("topology")},
			{title: "TUNNEL MODE", cell: field("tunnel_mode")},
			{title: "HUB GATEWAY", cell: field("hub_gateway_id")},
			status, age}
	}
	return []column{name,
		{title: "CLUSTER", cell: func(m *Model, r Resource) string { return m.clusterLabel(r.Field("cluster_id")) }},
		{title: "PHASE", cell: func(_ *Model, r Resource) string { return orDash(r.Field("phase")) }, phase: true},
		status,
		{title: "SANDBOXES", cell: field("active_sandbox_count")},
		{title: "RELEASE", cell: func(m *Model, r Resource) string { return m.releaseLabel(r.Field("release_id")) }},
		age}
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// age renders the time since t in the largest whole unit.
func age(now, t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	d := now.Sub(t)
	switch {
	case d < 0:
		return "0s"
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

func sinceField(now time.Time, ts string) string {
	if ts == "" {
		return "-"
	}
	t, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return ts
	}
	return age(now, t) + " ago"
}

const (
	columnGap      = 2
	minColumnWidth = 4
	maxColumnWidth = 48
)

// renderTable lays out rows to exactly width columns. Cells wider than their
// column are truncated with an ellipsis; the widest columns shrink first when
// the natural widths do not fit.
func renderTable(cols []column, rows [][]string, selected int, width int) []string {
	widths := make([]int, len(cols))
	for i, c := range cols {
		widths[i] = lipgloss.Width(c.title)
	}
	for _, row := range rows {
		for i, cell := range row {
			widths[i] = max(widths[i], min(lipgloss.Width(cell), maxColumnWidth))
		}
	}
	for total(widths) > width {
		widest := 0
		for i := range widths {
			if widths[i] > widths[widest] {
				widest = i
			}
		}
		if widths[widest] <= minColumnWidth {
			break
		}
		widths[widest]--
	}

	lines := make([]string, 0, len(rows)+1)
	titles := make([]string, len(cols))
	for i, c := range cols {
		titles[i] = headerCell.Render(fit(c.title, widths[i]))
	}
	lines = append(lines, strings.Join(titles, strings.Repeat(" ", columnGap)))
	for r, row := range rows {
		cells := make([]string, len(cols))
		for i, cell := range row {
			text := fit(cell, widths[i])
			if cols[i].phase && r != selected {
				text = phaseStyle(cell).Render(text)
			}
			cells[i] = text
		}
		line := strings.Join(cells, strings.Repeat(" ", columnGap))
		if r == selected {
			line = selectedRow.Render(fit(line, width))
		}
		lines = append(lines, line)
	}
	return lines
}

func total(widths []int) int {
	sum := 0
	for _, w := range widths {
		sum += w
	}
	return sum + columnGap*max(len(widths)-1, 0)
}

// fit truncates or right-pads s to exactly width display cells.
func fit(s string, width int) string {
	if lipgloss.Width(s) > width {
		s = ansi.Truncate(s, width, "…")
	}
	return s + strings.Repeat(" ", max(width-lipgloss.Width(s), 0))
}
