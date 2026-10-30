package main

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// Terminal adaptations of Red Hat's restrained brand accent and PatternFly's
// semantic colors, neutral surfaces, selected rows, and section hierarchy.
// References: https://www.redhat.com/en/about/brand/standards/color
// https://www.patternfly.org/foundations-and-styles/colors/
// Font family and font size remain under the terminal user's control.
func themeColor(role string) lipgloss.TerminalColor {
	switch role {
	case "selection":
		return lipgloss.AdaptiveColor{Light: "#f0f0f0", Dark: "#303030"}
	case "canvas":
		return lipgloss.AdaptiveColor{Light: "#ffffff", Dark: "#151515"}
	case "scroll-track":
		return lipgloss.AdaptiveColor{Light: "#8a8d90", Dark: "#62666a"}
	case "brand":
		return lipgloss.Color("#ee0000")
	case "1":
		return lipgloss.AdaptiveColor{Light: "#a30000", Dark: "#f4b3b0"}
	case "2":
		return lipgloss.AdaptiveColor{Light: "#2e5c00", Dark: "#bde2b9"}
	case "3":
		return lipgloss.AdaptiveColor{Light: "#795600", Dark: "#f9e0a2"}
	case "6":
		return lipgloss.AdaptiveColor{Light: "#0066cc", Dark: "#92c5f9"}
	case "8":
		return lipgloss.AdaptiveColor{Light: "#4f5255", Dark: "#b8bbbe"}
	case "border":
		return lipgloss.AdaptiveColor{Light: "#c7c7c7", Dark: "#4f5255"}
	default:
		return lipgloss.AdaptiveColor{Light: "#151515", Dark: "#f4f4f4"}
	}
}
func (m Model) rule(width int) string {
	return m.style(strings.Repeat("─", max(0, width)), "border", false)
}
func (m Model) selection(line string, width int) string {
	if m.plainColor {
		return line
	}
	text := ansi.Truncate(ansi.Strip(line), width, "")
	text += strings.Repeat(" ", max(0, width-ansi.StringWidth(text)))
	base := lipgloss.NewStyle().Background(themeColor("selection")).Foreground(themeColor("7"))
	if !strings.HasPrefix(text, "> ") {
		return base.Render(text)
	}
	body := text[2:]
	marker := base.Foreground(themeColor("6")).Render("▎ ")
	// Give the issue identifier emphasis without bolding the whole row.
	issue := selectedIssue.FindString(body)
	if issue != "" {
		return marker + base.Bold(true).Render(issue) + base.Render(body[len(issue):])
	}
	return marker + base.Render(body)
}

var selectedIssue = regexp.MustCompile(`^#[0-9]+\b`)

func (m Model) groupHeading(agent, summary string, width int) string {
	summary = middle(clean(summary), max(1, width/3))
	agent = middle(clean(agent), max(1, width-lipgloss.Width(summary)-4))
	gap := max(0, width-lipgloss.Width(agent)-lipgloss.Width(summary))
	divider := strings.Repeat(" ", gap)
	if gap >= 3 {
		divider = " " + strings.Repeat("─", gap-2) + " "
	}
	if m.plainColor {
		return agent + divider + summary
	}
	render := func(text, role string, bold bool) string {
		return lipgloss.NewStyle().Foreground(themeColor(role)).Background(themeColor("canvas")).Bold(bold).Render(text)
	}
	return render(agent, "7", true) + render(divider, "border", false) + render(summary, "8", false)
}

func (m Model) viewTabs(issues bool, width int) []string {
	agentsColor, issuesColor := "6", "8"
	underline := "━━━━━━"
	if issues {
		agentsColor, issuesColor = "8", "6"
		underline = "          ━━━━━━"
	}
	left := m.style("Agents", agentsColor, !issues) + "    " + m.style("Issues", issuesColor, issues)
	hint := "tab switch view"
	if width > 55 {
		left += strings.Repeat(" ", max(2, width-lipgloss.Width(left)-len(hint))) + m.footerHints(hint)
	}
	return []string{left, m.style(underline, "6", true)}
}

// All views share the same footer treatment: accented keys, muted actions.
var shortcutSeparator = regexp.MustCompile(` {2,}`)

func (m Model) footerHints(text string) string {
	text = ansi.Strip(text)
	if m.plainColor {
		return text
	}
	render := func(hint string) string {
		if strings.HasPrefix(hint, "Type ") {
			return m.style(hint, "8", false)
		}
		end := strings.IndexByte(hint, ' ')
		if end < 0 {
			return m.style(hint, "8", false)
		}
		// Alternative keys are written as "h / esc / ? close".
		for strings.HasPrefix(hint[end:], " / ") {
			next := strings.IndexByte(hint[end+3:], ' ')
			if next < 0 {
				break
			}
			end += 3 + next
		}
		return m.style(hint[:end], "6", true) + m.style(hint[end:], "8", false)
	}
	var out strings.Builder
	start := 0
	for _, gap := range shortcutSeparator.FindAllStringIndex(text, -1) {
		out.WriteString(render(text[start:gap[0]]))
		out.WriteString(text[gap[0]:gap[1]])
		start = gap[1]
	}
	out.WriteString(render(text[start:]))
	return out.String()
}
