package main

import (
	"context"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) updateConfiguration(v tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := v.String()
	if key == "ctrl+c" || (!m.configEditing && key == "q") {
		m.cancel()
		m.exit = 130
		return m, tea.Quit
	}
	rows := m.engine.config().Settings
	if len(rows) == 0 {
		switch key {
		case "tab":
			m.screen = ""
			m.issueGrouping = false
		case "shift+tab":
			m.screen = ""
			m.issueGrouping = true
		case "esc":
			m.screen = m.configReturn
		}
		return m, nil
	}
	m.configIndex = min(max(0, m.configIndex), len(rows)-1)
	row := rows[m.configIndex]
	if key == "ctrl+x" {
		m.setNotice("")
		return m, nil
	}
	if m.configSaving {
		return m, nil
	}
	if !m.configEditing {
		switch key {
		case "tab":
			m.screen = ""
			m.issueGrouping = false
			m.historyIssue = nil
			m.historyRows = nil
		case "shift+tab":
			m.screen = ""
			m.issueGrouping = true
			m.historyIssue = nil
			m.historyRows = nil
		case "esc", "h":
			m.screen = m.configReturn
		case "?":
			m.helpOpen = true
			m.helpScroll = 0
		case "x", "ctrl+x":
			m.setNotice("")
		case "j", "down":
			m.configIndex = min(len(rows)-1, m.configIndex+1)
		case "k", "up":
			m.configIndex = max(0, m.configIndex-1)
		case "G", "end":
			m.configIndex = len(rows) - 1
		case "home":
			m.configIndex = 0
		case "g":
			if !m.gPending.IsZero() && time.Since(m.gPending) < time.Second {
				m.configIndex = 0
				m.gPending = time.Time{}
			} else {
				m.gPending = time.Now()
			}
		case "pgdown", "ctrl+f", "ctrl+d":
			m.configIndex = min(len(rows)-1, m.configIndex+max(1, (m.height-17)/2))
		case "pgup", "ctrl+b", "ctrl+u":
			m.configIndex = max(0, m.configIndex-max(1, (m.height-17)/2))
		case "enter", "l":
			if m.state.Monitoring {
				m.setNotice("Edit configuration from the dispatcher that owns scheduling.")
			} else {
				m.setNotice("")
				m.configEditing = true
				m.configInput = row.Value
				if row.Saved != nil {
					m.configInput = *row.Saved
				}
				m.configCursor = len([]rune(m.configInput))
				m.configError = ""
				m.configChoice = 0
				for i, value := range row.Choices {
					if value == m.configInput {
						m.configChoice = i
					}
				}
			}
		}
		return m, nil
	}
	if key == "esc" {
		m.configEditing = false
		m.configError = ""
		return m, nil
	}
	if row.Kind == "readonly" {
		if key == "q" {
			m.cancel()
			m.exit = 130
			return m, tea.Quit
		}
		if key == "enter" || key == "h" {
			m.configEditing = false
		}
		return m, nil
	}
	if key == "enter" {
		value := m.configInput
		if row.Kind == "choice" && len(row.Choices) > 0 {
			value = row.Choices[m.configChoice]
		}
		m.configSaving = true
		m.configError = ""
		return m, func() tea.Msg {
			ctx, cancel := context.WithTimeout(m.ctx, 10*time.Second)
			defer cancel()
			return settingsDone(m.engine.requestSetting(ctx, row.Key, value))
		}
	}
	if row.Kind == "choice" && len(row.Choices) > 0 {
		if key == "j" || key == "down" || key == "right" || key == "tab" {
			m.configChoice = (m.configChoice + 1) % len(row.Choices)
		}
		if key == "k" || key == "up" || key == "left" {
			m.configChoice = (m.configChoice + len(row.Choices) - 1) % len(row.Choices)
		}
		return m, nil
	}
	text := []rune(m.configInput)
	cursor := min(m.configCursor, len(text))
	switch key {
	case "left":
		cursor = max(0, cursor-1)
	case "right":
		cursor = min(len(text), cursor+1)
	case "home", "ctrl+a":
		cursor = 0
	case "end", "ctrl+e":
		cursor = len(text)
	case "ctrl+u":
		text = text[cursor:]
		cursor = 0
	case "ctrl+k":
		text = text[:cursor]
	case "backspace", "ctrl+h":
		if cursor > 0 {
			text = append(text[:cursor-1], text[cursor:]...)
			cursor--
		}
	case "delete":
		if cursor < len(text) {
			text = append(text[:cursor], text[cursor+1:]...)
		}
	default:
		if v.Type == tea.KeyRunes && len(text)+len(v.Runes) <= 4096 {
			insert := []rune(strings.ReplaceAll(clean(string(v.Runes)), "\n", ""))
			tail := append([]rune(nil), text[cursor:]...)
			text = append(append(text[:cursor], insert...), tail...)
			cursor += len(insert)
		}
	}
	m.configInput = string(text)
	m.configCursor = cursor
	return m, nil
}
