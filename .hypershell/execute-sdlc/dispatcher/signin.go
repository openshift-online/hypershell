package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/creack/pty"
	"golang.org/x/term"
)

type signInEvent struct {
	Session *signInSession
	Text    string
	Done    bool
	Err     error
}
type signInStarted struct {
	Session *signInSession
	Err     error
}
type signInSession struct {
	terminal *os.File
	events   chan signInEvent
	parent   context.Context
}

func (s *signInSession) next() tea.Cmd {
	return func() tea.Msg {
		select {
		case event := <-s.events:
			return event
		case <-s.parent.Done():
			return signInEvent{Session: s, Done: true, Err: s.parent.Err()}
		}
	}
}
func (s *signInSession) resize(width, height int) error {
	return pty.Setsize(s.terminal, &pty.Winsize{Rows: uint16(max(4, height)), Cols: uint16(max(12, width))})
}

// Give existing CLI prompts a real terminal while keeping all presentation in
// Bubble Tea. Browser login still runs normally; no credentials are synthesized.
func startSignIn(parent, ctx context.Context, e *Engine, width, height int) tea.Cmd {
	return func() tea.Msg {
		if err := e.beginGateway(ctx); err != nil {
			if ctx.Err() != nil {
				e.finishGateway(parent, e.c, err)
			}
			return signInStarted{Err: err}
		}
		e.authMu.Lock()
		finish := func(err error) error {
			c := e.c
			if err == nil {
				var loadErr error
				c, loadErr = loadConfig(ctx, e.a)
				if loadErr != nil {
					err = loadErr
				}
			}
			e.finishGateway(parent, c, err)
			return err
		}
		cmd := e.a.command(ctx, "gateway")
		cmd.Env = mergeEnv(os.Environ(), map[string]string{"NO_COLOR": "1", "TERM": "dumb", "PYTHONUNBUFFERED": "1", "FUNNEL_EMBEDDED_UI": "1"})
		cmd.Cancel = func() error {
			if cmd.Process == nil {
				return nil
			}
			err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			if err == syscall.ESRCH {
				return os.ErrProcessDone
			}
			return err
		}
		terminal, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: uint16(max(4, height)), Cols: uint16(max(12, width))})
		if err != nil {
			e.authMu.Unlock()
			finish(err)
			return signInStarted{Err: err}
		}
		// Bubble Tea edits the input line. Disable PTY echo so answers aren't drawn twice.
		if _, err = term.MakeRaw(int(terminal.Fd())); err != nil {
			_ = cmd.Cancel()
			_ = cmd.Wait()
			_ = terminal.Close()
			e.authMu.Unlock()
			finish(err)
			return signInStarted{Err: err}
		}
		s := &signInSession{terminal: terminal, events: make(chan signInEvent, 128), parent: parent}
		send := func(event signInEvent) {
			event.Session = s
			select {
			case s.events <- event:
			case <-parent.Done():
			}
		}
		readDone := make(chan struct{})
		go func() {
			defer close(readDone)
			buf := make([]byte, 4096)
			for {
				n, err := terminal.Read(buf)
				if n > 0 {
					send(signInEvent{Text: string(buf[:n])})
				}
				if err != nil {
					if err != io.EOF && err != syscall.EIO && ctx.Err() == nil && !strings.Contains(err.Error(), "input/output error") {
						e.note("Sign-in terminal closed: " + err.Error())
					}
					return
				}
			}
		}()
		go func() {
			err := cmd.Wait()
			select {
			case <-readDone:
			case <-time.After(500 * time.Millisecond):
				_ = terminal.Close()
				<-readDone
			}
			_ = terminal.Close()
			if ctx.Err() != nil {
				err = fmt.Errorf("Sign-in cancelled; the connection has not been restored")
			}
			diagnostics, readErr := tail(filepath.Join(e.c.LogDir, "gateway-signin.log"), 32768)
			if readErr == nil && diagnostics != "" {
				e.note("Gateway sign-in:\n" + diagnostics)
			}
			if err != nil && ctx.Err() == nil {
				err = fmt.Errorf("Sign-in failed: %w. See details for diagnostics", err)
			}
			e.authMu.Unlock()
			err = finish(err)
			send(signInEvent{Done: true, Err: err})
		}()
		return signInStarted{Session: s}
	}
}

// Prompt frames may span PTY reads. Keep incomplete frames separate from logs.
type gatewayChoice struct {
	Value       string
	Title       string
	Description string
}
type gatewayPrompt struct {
	Title       string
	Description string
	Default     string
	Choices     []gatewayChoice
}

func (m *Model) receiveSignIn(text string) {
	m.signInPending += text
	for len(m.signInPending) > 0 {
		start := strings.IndexByte(m.signInPending, 0x1e)
		if start < 0 {
			m.signInText += m.signInPending
			m.signInPending = ""
			return
		}
		m.signInText += m.signInPending[:start]
		m.signInPending = m.signInPending[start:]
		end := strings.IndexByte(m.signInPending, 0x1f)
		if end < 0 {
			return
		}
		var prompt gatewayPrompt
		if err := json.Unmarshal([]byte(m.signInPending[1:end]), &prompt); err != nil {
			m.signInText += "\nCannot read gateway prompt: " + err.Error()
		} else {
			m.signInPrompt = &prompt
			m.signInChoice = 0
			m.signInInput = ""
			m.signInDiagnostics = false
		}
		m.signInPending = m.signInPending[end+1:]
	}
}
func (m Model) signInBody(width int) []string {
	if m.signInDiagnostics {
		return strings.Split(clean(strings.ReplaceAll(m.signInText, "\r\n", "\n")), "\n")
	}
	p := m.signInPrompt
	if p == nil {
		if !m.signInActive {
			role := "2"
			if m.signInResult != "Gateway access verified." {
				role = "3"
			}
			return []string{m.style(m.signInResult, role, true), "", m.style("Press d to inspect connection diagnostics.", "8", false)}
		}
		// Keep browser URLs and device codes visible without showing setup chatter.
		out := []string{m.style("Connecting to your gateway…", "6", true), "", "Complete sign-in in your browser if prompted.", "", m.style("Connection output", "8", true)}
		text := strings.Split(clean(strings.ReplaceAll(m.signInText, "\r\n", "\n")), "\n")
		for _, line := range text {
			if strings.Contains(line, "Reusing saved gateway") || strings.TrimSpace(line) == "" {
				continue
			}
			out = append(out, line)
		}
		return out
	}
	out := []string{m.style(p.Title, "7", true), "", m.style(p.Description, "8", false), ""}
	for i, choice := range p.Choices {
		prefix := "  "
		if i == m.signInChoice {
			prefix = "> "
		}
		title := prefix + choice.Title
		if i == m.signInChoice {
			title = m.selection(title, width)
		} else {
			title = m.style(title, "7", true)
		}
		out = append(out, title, m.style("  "+choice.Description, "8", false), "")
	}
	if len(p.Choices) == 0 {
		out = append(out, m.style("Suggested name: "+p.Default, "8", false), "", m.style("> "+m.signInInput, "6", true))
	}
	return out
}
