package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestNotificationLifetimeAndDismiss(t *testing.T) {
	m := sampleModel()
	defer m.cancel()
	m.setNotice("Gateway access verified.")
	deadline := m.noticeUntil
	if deadline.IsZero() {
		t.Fatal("success needs an expiry")
	}
	m.expireNotice(deadline.Add(-time.Second))
	if m.notice == "" {
		t.Fatal("expired early")
	}
	m.expireNotice(deadline)
	if m.notice != "" {
		t.Fatal("success did not expire")
	}
	m.setNotice("Cannot connect to gateway")
	m.expireNotice(time.Now().Add(time.Hour))
	if m.notice == "" {
		t.Fatal("warning disappeared without dismissal")
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	m = updated.(Model)
	if m.notice != "" {
		t.Fatal("x did not dismiss")
	}
	m.screen = "signin"
	m.signInActive = true
	m.signInPrompt = &gatewayPrompt{Title: "New gateway name"}
	m.setNotice("Check connection")
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	m = updated.(Model)
	if m.signInInput != "x" || m.notice == "" {
		t.Fatal("dismiss stole text input")
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlX})
	m = updated.(Model)
	if m.notice != "" {
		t.Fatal("ctrl+x did not dismiss during text entry")
	}
}

func TestNotificationOverlayEveryScreen(t *testing.T) {
	for _, screen := range []string{"", "logs", "details", "signin", "history"} {
		for _, width := range []int{32, 80, 120} {
			m := sampleModel()
			m.width = width
			m.height = 28
			m.screen = screen
			if screen == "history" {
				r := m.state.Rows[0]
				m.historyIssue = &r
				m.historyRows = m.state.Rows
			}
			before := strings.Split(m.View(), "\n")
			m.setNotice("Gateway access verified.")
			view := m.View()
			after := strings.Split(view, "\n")
			if lipgloss.Height(view) != m.height || lipgloss.Width(view) > width {
				t.Fatalf("%s/%d changed bounds", screen, width)
			}
			if !strings.Contains(view, "x dismiss") || strings.Count(view, "Gateway access verified.") != 1 {
				t.Fatalf("%s/%d missing or duplicated notification", screen, width)
			}
			if before[len(before)-2] != after[len(after)-2] {
				t.Fatal("notification obscured action bar")
			}
			if strings.Contains(view, "\x1b[") {
				t.Fatal("notification ignored NO_COLOR")
			}
			m.cancel()
		}
	}
}

func TestNotificationPreview(t *testing.T) {
	dir := os.Getenv("FUNNEL_PREVIEW_DIR")
	if dir == "" {
		t.Skip("set FUNNEL_PREVIEW_DIR to render")
	}
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(old)
	dark := lipgloss.HasDarkBackground()
	lipgloss.SetHasDarkBackground(true)
	defer lipgloss.SetHasDarkBackground(dark)
	m := sampleModel()
	defer m.cancel()
	m.width = 120
	m.height = 32
	m.plainColor = false
	m.setNotice("Gateway access verified.")
	if err := os.WriteFile(filepath.Join(dir, "notification.ansi"), []byte(m.View()), 0600); err != nil {
		t.Fatal(err)
	}
}
