package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

func TestLogRetainsWholeFileAndPausedAnchorAcrossAppendAndRotation(t *testing.T) {
	m := sampleModel()
	defer m.cancel()
	m.height = 32
	m.width = 100
	path := filepath.Join(t.TempDir(), "run.log")
	var source strings.Builder
	for i := 0; i < 5000; i++ {
		fmt.Fprintf(&source, "line %04d some substantial log content\n", i)
	}
	if err := os.WriteFile(path, []byte(source.String()), 0600); err != nil {
		t.Fatal(err)
	}
	m.openLog(path)
	if !strings.HasPrefix(m.logText, "line 0000") || len(m.logText) <= 65536 {
		t.Fatal("log was byte-truncated")
	}
	m = press(m, "g")
	m = press(m, "g")
	before := m.logContent().lines[len(m.logContent().lines)-m.scroll-m.logPageSize()]
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, err = file.WriteString("new line one\nnew line two\n")
	file.Close()
	if err != nil {
		t.Fatal(err)
	}
	m.refreshLog(path)
	after := m.logContent().lines[len(m.logContent().lines)-m.scroll-m.logPageSize()]
	if before != after || !strings.Contains(m.View(), "Paused") {
		t.Fatalf("paused view moved: %q -> %q", before, after)
	}
	m = press(m, "G")
	if m.scroll != 0 || !strings.Contains(m.View(), "new line two") {
		t.Fatal("bottom did not resume following")
	}
	if err := os.WriteFile(path, []byte("replacement\n"), 0600); err != nil {
		t.Fatal(err)
	}
	m.refreshLog(path)
	if m.logText != "replacement\n" || m.scroll != 0 {
		t.Fatal("truncation not handled")
	}
	next := filepath.Join(filepath.Dir(path), "replacement.log")
	if err := os.WriteFile(next, []byte("rotated log\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(next, path); err != nil {
		t.Fatal(err)
	}
	m.refreshLog(path)
	if m.logText != "rotated log\n" {
		t.Fatal("replacement not detected")
	}
}

func TestLogHorizontalPanWrapAndFrameDimensions(t *testing.T) {
	m := sampleModel()
	defer m.cancel()
	m.screen = "logs"
	m.height = 30
	m.width = 80
	m.logText = "START " + strings.Repeat("界🔐alpha ", 30) + " END_MARKER"
	m.clampLogScroll()
	if m.logs.wrap || m.logs.markdown || m.logs.noColor {
		t.Fatal("incorrect defaults")
	}
	if strings.Contains(m.View(), "END_MARKER") {
		t.Fatal("long line unexpectedly wrapped")
	}
	m = press(m, "$")
	if !strings.Contains(m.View(), "END_MARKER") || m.logs.x == 0 {
		t.Fatal("right edge inaccessible")
	}
	if !strings.Contains(m.View(), "Cols ") || !strings.Contains(m.View(), "┌") || !strings.Contains(m.View(), "└") {
		t.Fatal("missing pane/range")
	}
	for _, key := range []string{"w", "w", "left", "right", "0", "m", "m"} {
		m = press(m, key)
		if lipgloss.Height(m.View()) != m.height {
			t.Fatal("pane height changed")
		}
		for _, line := range strings.Split(m.View(), "\n") {
			if ansi.StringWidth(line) > m.width {
				t.Fatalf("overflow: %q", line)
			}
		}
	}
	m = press(m, "w")
	if m.logs.x != 0 || len(m.logContent().lines) < 2 {
		t.Fatal("wrap did not reflow")
	}
	m = press(m, "G")
	if !strings.Contains(m.View(), "END_MARKER") {
		t.Fatal("wrapped suffix missing")
	}
}

func TestLogFilterIsLocalLiveAndTextEntryWins(t *testing.T) {
	m := sampleModel()
	defer m.cancel()
	m.screen = "logs"
	m.width = 110
	m.height = 32
	m.filter = "work list filter"
	m.selected = "42"
	m.index = 0
	m.logText = "PASS alpha\nERROR queued work\nPASS beta\n"
	m = press(m, "/")
	m = press(m, "q")
	if m.ctx.Err() != nil || !m.logs.filtering || m.logs.filter != "q" {
		t.Fatal("filter typing triggered command")
	}
	m = press(m, "u")
	if m.logContent().matches != 1 {
		t.Fatal("live filter failed")
	}
	m = press(m, "enter")
	if m.logs.filtering || !strings.Contains(m.View(), "ERROR queued work") || strings.Contains(m.View(), "PASS alpha") {
		t.Fatal("filter not applied")
	}
	if m.filter != "work list filter" {
		t.Fatal("log filter changed work filter")
	}
	m = press(m, "/")
	m = press(m, "x")
	m = press(m, "esc")
	if m.logs.filter != "qu" {
		t.Fatal("cancel did not restore prior filter")
	}
	m = press(m, "esc")
	if m.screen != "logs" || m.logs.filter != "" {
		t.Fatal("escape did not clear filter before back")
	}
}

func TestMarkdownRenderingNeverWrapsOrTruncatesSourceUnlessEnabled(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(old)
	text := "# Heading\n\n**bold text** and `code`\n\n" + strings.Repeat("long phrase ", 30) + "UNIQUE_SUFFIX\n\n```go\n" + strings.Repeat("x", 180) + "CODE_SUFFIX\n```\n"
	key := logRenderKey{text: text, width: 45, markdown: true, color: true}
	got := renderLog(key)
	if got.err != nil {
		t.Fatal(got.err)
	}
	rendered := strings.Join(got.lines, "\n")
	if !strings.Contains(ansi.Strip(rendered), "UNIQUE_SUFFIX") || !strings.Contains(ansi.Strip(rendered), "CODE_SUFFIX") {
		t.Fatal("Markdown dropped content")
	}
	if got.width < 180 {
		t.Fatalf("Markdown wrapped despite wrap off: width=%d", got.width)
	}
	if strings.Contains(ansi.Strip(rendered), "**bold text**") || !strings.Contains(rendered, "\x1b[") {
		t.Fatal("Markdown was not styled")
	}
	key.wrap = true
	got = renderLog(key)
	for _, line := range got.lines {
		if ansi.StringWidth(line) > 45 {
			t.Fatalf("wrapped Markdown overflow: %d", ansi.StringWidth(line))
		}
	}
	key.color = false
	got = renderLog(key)
	if strings.Contains(strings.Join(got.lines, "\n"), "\x1b") {
		t.Fatal("color disabled still contains ANSI")
	}
}

func TestLogColorsRetainSGRButStripTerminalControlsAndHonorNoColor(t *testing.T) {
	raw := "\x1b[31mRED\x1b[0m\x1b[2J\x1b[5;5H\x1b]52;c;c2VjcmV0\a\x1b]0;malicious title\a\nvisible\tvalue\n"
	got := safeLog(raw, true)
	if !strings.Contains(got, "\x1b[31mRED") || strings.Contains(got, "\x1b[2J") || strings.Contains(got, "\x1b]") || strings.Contains(got, "malicious") {
		t.Fatalf("unsafe or missing SGR: %q", got)
	}
	if strings.Contains(safeLog(raw, false), "\x1b") {
		t.Fatal("monochrome retained escape")
	}
	m := sampleModel()
	defer m.cancel()
	m.screen = "logs"
	m.logText = raw
	m.plainColor = true
	if strings.Contains(m.View(), "\x1b") {
		t.Fatal("NO_COLOR not respected")
	}
	m.plainColor = false
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(old)
	if !strings.Contains(m.View(), "\x1b[31m") {
		t.Fatal("source colors lost")
	}
	m = press(m, "a")
	if strings.Contains(strings.Join(m.logContent().lines, "\n"), "\x1b") {
		t.Fatal("color toggle failed")
	}
}

func TestLogFollowPausesOnUpAndResumesByScrollingDown(t *testing.T) {
	m := sampleModel()
	defer m.cancel()
	m.screen = "logs"
	m.logText = strings.Repeat("line\n", 100)
	m = press(m, "k")
	if m.scroll != 1 {
		t.Fatal("scroll up did not pause")
	}
	m = press(m, "j")
	if m.scroll != 0 {
		t.Fatal("scroll down to bottom did not follow")
	}
	m = press(m, "k")
	m = press(m, "f")
	if m.scroll != 0 {
		t.Fatal("explicit follow failed")
	}
	m.logs.filtering = true
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil || m.ctx.Err() == nil {
		t.Fatal("cannot detach while filtering")
	}
}

func TestLogColorContinuesAcrossFilteredAndWrappedLines(t *testing.T) {
	key := logRenderKey{text: "\x1b[31mfirst line\nsecond " + strings.Repeat("word ", 30) + "\x1b[0m", color: true, width: 20, filter: "second"}
	got := renderLog(key)
	if len(got.lines) != 1 || !strings.Contains(got.lines[0], "\x1b[31m") {
		t.Fatal("filtered line lost inherited color")
	}
	key.wrap = true
	got = renderLog(key)
	for _, line := range got.lines {
		if strings.TrimSpace(ansi.Strip(line)) != "" && !strings.Contains(line, "\x1b[31m") {
			t.Fatalf("wrapped line lost color: %q", line)
		}
	}
}
