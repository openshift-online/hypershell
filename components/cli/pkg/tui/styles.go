package tui

import "github.com/charmbracelet/lipgloss"

var (
	titleStyle    = lipgloss.NewStyle().Bold(true)
	mutedStyle    = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "243", Dark: "245"})
	tabStyle      = lipgloss.NewStyle().Padding(0, 1)
	activeTab     = tabStyle.Bold(true).Reverse(true)
	headerCell    = lipgloss.NewStyle().Bold(true).Underline(true)
	selectedRow   = lipgloss.NewStyle().Reverse(true)
	errorStyle    = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "160", Dark: "203"})
	warnStyle     = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "136", Dark: "221"})
	okStyle       = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "28", Dark: "114"})
	progressStyle = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "25", Dark: "111"})
	panelStyle    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
	focusedLabel  = lipgloss.NewStyle().Bold(true).Underline(true)

	// Detail-view YAML syntax styles (TUI-06). Punctuation stays neutral.
	yamlKeyStyle    = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "25", Dark: "117"})
	yamlStringStyle = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "65", Dark: "151"})
	yamlNumberStyle = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "91", Dark: "177"})
	yamlBoolStyle   = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "130", Dark: "215"})
	yamlNullStyle   = mutedStyle
)

// phaseStyle styles the canonical gateway phases
// (specs/platform/gateway-phase-vocabulary.spec.md). Any other value is
// neutral. The phase text is always shown, so meaning never rests on color.
func phaseStyle(phase string) lipgloss.Style {
	switch phase {
	case "Running":
		return okStyle
	case "Pending", "Provisioning":
		return progressStyle
	case "Degraded":
		return warnStyle
	case "Failed":
		return errorStyle
	}
	return lipgloss.NewStyle()
}
