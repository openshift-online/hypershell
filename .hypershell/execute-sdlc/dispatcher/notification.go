package main

import (
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func (m *Model) setNotice(text string) {
	m.notice = text
	m.noticeUntil = time.Time{}
	if text == "Gateway access verified." || strings.HasPrefix(text, "Retry requested for ") || text == "Retry cancelled." {
		m.noticeUntil = time.Now().Add(8 * time.Second)
	}
}
func (m *Model) expireNotice(now time.Time) {
	if !m.noticeUntil.IsZero() && !now.Before(m.noticeUntil) {
		m.setNotice("")
	}
}

// Composite the card above the fixed action bar, without moving any content.
func (m Model) overlayNotice(lines []string, width, height int) []string {
	if m.notice == "" || width < 16 || height < 8 {
		return lines
	}
	role, title := "3", "Notice"
	if m.notice == "Gateway access verified." || strings.HasPrefix(m.notice, "Retry requested for ") {
		role, title = "2", "Success"
	}
	cardWidth := min(54, width)
	inner := cardWidth - 4
	closeHint := "x dismiss"
	if m.filtering || (m.screen == "signin" && m.signInActive && m.signInPrompt != nil && len(m.signInPrompt.Choices) == 0) {
		closeHint = "ctrl+x close"
	}
	heading := title + strings.Repeat(" ", max(1, inner-len(title)-len(closeHint))) + closeHint
	if inner < 20 {
		heading = closeHint
	}
	body := strings.Split(ansi.Hardwrap(clean(m.notice), inner, true), "\n")
	limit := max(1, min(6, height-7))
	if len(body) > limit {
		body = body[:limit]
		body[limit-1] = ansi.Truncate(body[limit-1], max(1, inner-1), "") + "…"
	}
	style := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1).Width(cardWidth - 2)
	if !m.plainColor {
		style = style.Background(themeColor("canvas")).Foreground(themeColor("7")).BorderForeground(themeColor(role))
	}
	card := strings.Split(style.Render(m.style(heading, role, true)+"\n\n"+strings.Join(body, "\n")), "\n")
	top := max(0, height-3-len(card))
	left := max(0, width-lipgloss.Width(card[0]))
	for i, line := range card {
		y := top + i
		if y >= len(lines) || y >= height-2 {
			break
		}
		base := lines[y] + strings.Repeat(" ", max(0, width-ansi.StringWidth(lines[y])))
		lines[y] = ansi.Cut(base, 0, left) + line
	}
	return lines
}
