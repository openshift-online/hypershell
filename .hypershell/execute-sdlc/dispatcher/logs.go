package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	glamansi "github.com/charmbracelet/glamour/ansi"
	"github.com/charmbracelet/glamour/styles"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/cellbuf"
	"github.com/muesli/termenv"
)

type logViewState struct {
	path                    string
	info                    os.FileInfo
	offset                  int64
	x                       int
	markdown, wrap, noColor bool
	filter, filterBefore    string
	filtering               bool
	readError               string
	cache                   *renderedLog
}
type logRenderKey struct {
	text, filter                string
	width                       int
	markdown, wrap, color, dark bool
}
type renderedLog struct {
	key                   logRenderKey
	lines                 []string
	width, matches, total int
	err                   error
}

// Keep printable graphemes and SGR colors/styles only. Logs cannot move the
// cursor, erase the screen, set titles, open links or write to the clipboard.
func safeLog(text string, color bool) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	var out strings.Builder
	var state byte
	parser := ansi.NewParser()
	parser.SetDataSize(1024)
	var pen cellbuf.Style
	for len(text) > 0 {
		seq, width, n, next := ansi.DecodeSequence(text, state, parser)
		if n <= 0 {
			break
		}
		state = next
		text = text[n:]
		switch {
		case width > 0:
			out.WriteString(seq)
		case seq == "\n" || seq == "\r":
			out.WriteByte('\n')
			if color && !pen.Empty() {
				out.WriteString(pen.Sequence())
			}
		case seq == "\t":
			out.WriteString("    ")
		case color && strings.HasPrefix(seq, "\x1b[") && strings.HasSuffix(seq, "m"):
			params := strings.TrimSuffix(strings.TrimPrefix(seq, "\x1b["), "m")
			if strings.IndexFunc(params, func(r rune) bool { return !unicode.IsDigit(r) && r != ';' && r != ':' }) < 0 {
				cellbuf.ReadStyle(parser.Params(), &pen)
				pen.SlowBlink(false).RapidBlink(false).Conceal(false)
				out.WriteString(ansi.ResetStyle + pen.Sequence())
			}
		}
	}
	return out.String()
}

func markdownLog(text string, width int, color bool) (string, error) {
	// Start with a compact, neutral style, then apply the terminal's semantic
	// palette. No background detection queries, environment styles, or file loads.
	style := styles.ASCIIStyleConfig
	zero := uint(0)
	style.Document.Margin = &zero
	style.Document.BlockPrefix = ""
	style.Document.BlockSuffix = ""
	bold := true
	style.Strong = glamansi.StylePrimitive{Bold: &bold}
	italic, crossed := true, true
	style.Emph = glamansi.StylePrimitive{Italic: &italic}
	style.Strikethrough = glamansi.StylePrimitive{CrossedOut: &crossed}
	style.H1.Prefix = ""
	style.H2.Prefix = ""
	style.H3.Prefix = ""
	style.H4.Prefix = ""
	style.H5.Prefix = ""
	style.H6.Prefix = ""
	style.Heading.Bold = &bold
	if color {
		fg, accent, muted := "#f4f4f4", "#92c5f9", "#b8bbbe"
		if !lipgloss.HasDarkBackground() {
			fg, accent, muted = "#151515", "#0066cc", "#4f5255"
		}
		style.Document.Color = &fg
		style.Heading.Color = &accent
		style.Code.Color = &accent
		style.Link.Color = &accent
		style.LinkText.Color = &accent
		style.BlockQuote.Color = &muted
		style.CodeBlock.Theme = "github"
		if lipgloss.HasDarkBackground() {
			style.CodeBlock.Theme = "github-dark"
		}
	}
	profile := termenv.Ascii
	if color {
		profile = termenv.Profile(lipgloss.ColorProfile())
	}
	renderer, err := glamour.NewTermRenderer(glamour.WithStyles(style), glamour.WithColorProfile(profile), glamour.WithWordWrap(width), glamour.WithPreservedNewLines())
	if err != nil {
		return "", err
	}
	return renderer.Render(text)
}

func (m Model) logWidth() int { return max(1, m.width-8) } // frame, pane border, scrollbar
func (m Model) logKey() logRenderKey {
	return logRenderKey{m.logText, m.logs.filter, m.logWidth(), m.logs.markdown, m.logs.wrap, !m.plainColor && !m.logs.noColor, lipgloss.HasDarkBackground()}
}

type logSourceLine struct {
	raw, prose string
	message    int // zero is a CLI line; positive IDs group agent prose
}

// Classification happens before filtering so matching continuation lines retain
// their message context even if the message marker itself was filtered out.
func classifyLogLines(lines []string) []logSourceLine {
	result := make([]logSourceLine, 0, len(lines))
	active, next := 0, 0
	for _, line := range lines {
		plain := ansi.Strip(line)
		trimmed := strings.TrimSpace(plain)
		if strings.HasPrefix(trimmed, "💬") {
			next++
			active = next
			prose := strings.TrimLeft(strings.TrimPrefix(trimmed, "💬"), " \ufe0e\ufe0f")
			result = append(result, logSourceLine{line, prose, active})
			continue
		}
		cli := false
		for _, prefix := range []string{"⚡", "→", "✓", "✗", "• ", "! ", "⚙", "Error:", "WARNING:", "runtime:", "model:", "post-lifecycle:", "pre-lifecycle:"} {
			if strings.HasPrefix(trimmed, prefix) {
				cli = true
				break
			}
		}
		if strings.HasPrefix(trimmed, "⏳") {
			// A heartbeat can interleave a message; preserve its CLI formatting
			// without claiming the next unmarked prose line is CLI output.
			result = append(result, logSourceLine{raw: line})
			continue
		}
		if cli {
			active = 0
		}
		result = append(result, logSourceLine{line, plain, active})
	}
	return result
}
func colorCLILog(line string, color bool) string {
	if !color || strings.Contains(line, "\x1b[") {
		return line
	}
	trimmed := strings.TrimSpace(line)
	role := ""
	switch {
	case strings.HasPrefix(trimmed, "Error:"), strings.HasPrefix(trimmed, "✗"), strings.HasPrefix(trimmed, "FAIL"):
		role = "1"
	case strings.HasPrefix(trimmed, "WARNING:"), strings.HasPrefix(trimmed, "!"), strings.HasPrefix(trimmed, "WARN"):
		role = "3"
	case strings.HasPrefix(trimmed, "✓"), strings.HasPrefix(trimmed, "PASS"):
		role = "2"
	case strings.HasPrefix(trimmed, "→"), strings.HasPrefix(trimmed, "•"):
		role = "6"
	}
	if role != "" {
		return lipgloss.NewStyle().Foreground(themeColor(role)).Render(line)
	}
	return line
}
func renderLog(key logRenderKey) *renderedLog {
	result := &renderedLog{key: key}
	source := safeLog(key.text, key.color)
	sourceLines := strings.Split(strings.TrimSuffix(source, "\n"), "\n")
	if source == "" {
		sourceLines = nil
	}
	result.total = len(sourceLines)
	selected := make([]logSourceLine, 0, len(sourceLines))
	query := strings.ToLower(key.filter)
	for _, line := range classifyLogLines(sourceLines) {
		if query == "" || strings.Contains(strings.ToLower(ansi.Strip(line.raw)), query) {
			selected = append(selected, line)
		}
	}
	result.matches = len(selected)
	output := make([]string, 0, len(selected))
	for i := 0; i < len(selected); {
		line := selected[i]
		if !key.markdown || line.message == 0 {
			output = append(output, colorCLILog(line.raw, key.color))
			i++
			continue
		}
		end := i + 1
		for end < len(selected) && selected[end].message == line.message {
			end++
		}
		prose := make([]string, 0, end-i)
		for _, part := range selected[i:end] {
			prose = append(prose, part.prose)
		}
		width := 0
		if key.wrap {
			width = max(1, key.width-3)
		}
		rendered, err := markdownLog(strings.Join(prose, "\n"), width, key.color)
		if err != nil {
			result.err = err
			for _, part := range selected[i:end] {
				output = append(output, part.raw)
			}
		} else {
			icon := "💬 "
			if key.color {
				icon = lipgloss.NewStyle().Foreground(themeColor("6")).Render("💬 ")
			}
			renderedLines := strings.Split(strings.Trim(safeLog(rendered, key.color), "\n"), "\n")
			for n, text := range renderedLines {
				prefix := "   "
				if n == 0 {
					prefix = icon
				}
				output = append(output, prefix+text)
			}
		}
		i = end
	}
	text := strings.Join(output, "\n")
	if key.wrap {
		text = safeLog(ansi.Wrap(text, key.width, ""), key.color)
	}
	result.lines = strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	if len(selected) == 0 {
		result.lines = nil
	}
	for _, line := range result.lines {
		result.width = max(result.width, ansi.StringWidth(line))
	}
	return result
}
func (m Model) logContent() *renderedLog {
	key := m.logKey()
	if m.logs.cache != nil && m.logs.cache.key == key {
		return m.logs.cache
	}
	return renderLog(key)
}
func (m *Model) prepareLogs() { m.logs.cache = m.logContent() }

// Read appended bytes, retaining the full log instead of a trailing 64 KiB.
// File identity and size also detect replacement/truncation. Nothing is written.
func (m *Model) refreshLog(path string) {
	before := len(m.logContent().lines)
	paused := m.scroll > 0
	info, err := os.Stat(path)
	if err != nil {
		m.logs.readError = "Cannot read log: " + err.Error()
		return
	}
	if !info.Mode().IsRegular() {
		m.logs.readError = "Log path is not a regular file"
		return
	}
	reset := m.logs.path != path || m.logs.info == nil || !os.SameFile(info, m.logs.info) || info.Size() < m.logs.offset
	if !reset && info.Size() == m.logs.offset && info.ModTime().Equal(m.logs.info.ModTime()) {
		m.logs.readError = ""
		return
	}
	if !reset && info.Size() == m.logs.offset {
		reset = true
	}
	file, err := os.Open(path)
	if err != nil {
		m.logs.readError = "Cannot open log: " + err.Error()
		return
	}
	offset := m.logs.offset
	if reset {
		offset = 0
	}
	if _, err = file.Seek(offset, io.SeekStart); err != nil {
		file.Close()
		m.logs.readError = "Cannot seek log: " + err.Error()
		return
	}
	data, err := io.ReadAll(file)
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		m.logs.readError = "Cannot load log: " + err.Error()
		return
	}
	if reset {
		m.logText = string(data)
		m.scroll = 0
	} else {
		m.logText += string(data)
	}
	m.logs.path = path
	m.logs.info = info
	m.logs.offset = offset + int64(len(data))
	m.logs.readError = ""
	m.prepareLogs()
	if paused && !reset {
		m.scroll += len(m.logContent().lines) - before
	}
	m.clampLogScroll()
}
func (m *Model) openLog(path string) {
	m.screen = "logs"
	m.scroll = 0
	m.logs.x = 0
	if m.logs.path != path {
		m.logText = ""
		m.logs.path = ""
		m.logs.info = nil
		m.logs.offset = 0
		m.logs.cache = nil
	}
	m.refreshLog(path)
}
func (m Model) logPageSize() int {
	fixed := 16
	if m.height-2 < 22 {
		fixed -= 2
	}
	if m.height-2 < 18 {
		fixed--
	}
	if m.logs.filtering {
		fixed++
	}
	return max(1, m.height-fixed)
}
func (m *Model) clampLogScroll() {
	m.prepareLogs()
	content := m.logContent()
	m.scroll = max(0, min(m.scroll, max(0, len(content.lines)-m.logPageSize())))
	if m.logs.wrap {
		m.logs.x = 0
	} else {
		m.logs.x = max(0, min(m.logs.x, max(0, content.width-m.logWidth())))
	}
}
func (m *Model) setLogFilter(value string) { m.logs.filter = value; m.scroll = 0; m.clampLogScroll() }

func (m *Model) logNavigation(v tea.KeyMsg) (bool, tea.Cmd) {
	key := v.String()
	if m.logs.filtering {
		switch key {
		case "ctrl+c":
			m.cancel()
			m.exit = 130
			return true, tea.Quit
		case "ctrl+x":
			m.setNotice("")
		case "enter":
			m.logs.filtering = false
		case "esc":
			m.setLogFilter(m.logs.filterBefore)
			m.logs.filtering = false
		case "backspace", "ctrl+h":
			r := []rune(m.logs.filter)
			if len(r) > 0 {
				m.setLogFilter(string(r[:len(r)-1]))
			}
		case "ctrl+u":
			m.setLogFilter("")
		default:
			if v.Type == tea.KeyRunes {
				m.setLogFilter(m.logs.filter + strings.ReplaceAll(clean(string(v.Runes)), "\n", ""))
			}
		}
		return true, nil
	}
	switch key {
	case "/":
		m.logs.filterBefore = m.logs.filter
		m.logs.filtering = true
	case "esc":
		if m.logs.filter == "" {
			return false, nil
		}
		m.setLogFilter("")
	case "m":
		m.logs.markdown = !m.logs.markdown
		m.clampLogScroll()
	case "w":
		m.logs.wrap = !m.logs.wrap
		m.clampLogScroll()
	case "a":
		m.logs.noColor = !m.logs.noColor
		m.clampLogScroll()
	case "f":
		m.scroll = 0
	case "left", "H":
		m.logs.x -= 4
		m.clampLogScroll()
	case "right", "L":
		m.logs.x += 4
		m.clampLogScroll()
	case "shift+left":
		m.logs.x -= max(1, m.logWidth()/2)
		m.clampLogScroll()
	case "shift+right":
		m.logs.x += max(1, m.logWidth()/2)
		m.clampLogScroll()
	case "0":
		m.logs.x = 0
	case "$":
		m.logs.x = m.logContent().width
		m.clampLogScroll()
	default:
		return false, nil
	}
	return true, nil
}
func onOff(value bool) string {
	if value {
		return "on"
	}
	return "off"
}

// Metadata belongs to the pane outline; it consumes no log rows.
func (m Model) logBorder(width int, leftEdge, rightEdge, left, right, rightColor string) string {
	room := max(0, width-6)
	right = middle(right, max(0, room/2))
	left = middle(left, max(0, room-ansi.StringWidth(right)-2))
	gap := max(0, width-6-ansi.StringWidth(left)-ansi.StringWidth(right))
	edge := func(text string) string { return m.style(text, "log-border", false) }
	if right == "" {
		return edge(leftEdge+"─ ") + m.style(left, "8", false) + edge(" "+strings.Repeat("─", max(0, width-5-ansi.StringWidth(left)))+rightEdge)
	}
	return edge(leftEdge+"─ ") + m.style(left, "8", false) + edge(" "+strings.Repeat("─", max(0, gap-2))+" ") + m.style(right, rightColor, false) + edge(" ─"+rightEdge)
}

func (m Model) logsView(lines []string, row Row, w, h int) []string {
	section := "Logs"
	if awaitingNewReview(row) {
		section = "Previous review logs"
	}
	header := m.workHeader(row, w, section)
	if h < 22 {
		header = []string{m.style(middle(title(row), w), "7", true), m.style(middle(row.Agent+" · "+rowStatus(row), w), rowColor(row), false), ""}
	}
	lines = append(lines, header...)
	if h >= 18 {
		lines = append(lines, m.style(middle(row.Log, w), "8", false))
	}
	content := m.logContent()
	if m.logs.filtering {
		lines = append(lines, m.style(middle("/ "+m.logs.filter+"▏  Enter keep · Esc cancel", w), "6", false))
	}
	count := max(1, h-len(lines)-4) // pane border, blank, footer
	end := max(min(len(content.lines), count), len(content.lines)-m.scroll)
	end = min(end, len(content.lines))
	start := max(0, end-count)
	border := func(text string) string { return m.style(text, "log-border", false) }
	following, followColor := "Following", "2"
	if m.scroll > 0 {
		following, followColor = "Paused · G resumes", "3"
	}
	label := "Markdown"
	if w < 80 {
		label = "MD"
	}
	modes := fmt.Sprintf("%s %s · Wrap %s · Color %s", label, onOff(m.logs.markdown), onOff(m.logs.wrap), onOff(!m.logs.noColor && !m.plainColor))
	if m.logs.filter != "" && !m.logs.filtering {
		modes = "/" + m.logs.filter + " · " + modes
	}
	lines = append(lines, m.logBorder(w, "┌", "┐", modes, following, followColor))
	pane := make([]string, count)
	for i := range pane {
		line := ""
		if start+i < end {
			line = ansi.Cut(content.lines[start+i], m.logs.x, m.logs.x+m.logWidth())
		}
		if !m.plainColor && !m.logs.noColor {
			line += "\x1b[0m"
		}
		pane[i] = line + strings.Repeat(" ", max(0, m.logWidth()-ansi.StringWidth(line)))
	}
	if len(content.lines) == 0 {
		empty := "Waiting for log output..."
		if m.logs.filter != "" {
			empty = "No log lines match this filter."
		}
		pane[0] = m.style(middle(empty, m.logWidth()), "8", false)
	}
	if m.logs.readError != "" {
		pane[0] = m.style(middle(m.logs.readError, m.logWidth()), "1", false)
	}
	if content.err != nil {
		pane[0] = m.style(middle("Markdown rendering failed; showing source: "+content.err.Error(), m.logWidth()), "1", false)
	}
	track := m.withScrollbar(pane, start, end, len(content.lines), count, w-2)
	for _, line := range track {
		lines = append(lines, border("│")+line+border("│"))
	}
	rangeText := ansi.Strip(m.scrollIndicator(start, end, len(content.lines)))
	if w < 90 {
		rangeText = fmt.Sprintf("Lines %d-%d/%d", min(start+1, end), end, len(content.lines))
	}
	if len(content.lines) == 0 {
		rangeText = "No lines to display"
	}
	columns := ""
	if !m.logs.wrap && content.width > m.logWidth() {
		columns = fmt.Sprintf("Cols %d-%d/%d", m.logs.x+1, min(content.width, m.logs.x+m.logWidth()), content.width)
	}
	if m.logs.filter != "" {
		if columns != "" {
			columns += " · "
		}
		columns += fmt.Sprintf("Matches %d/%d", content.matches, content.total)
	}
	lines = append(lines, m.logBorder(w, "└", "┘", rangeText, columns, "8"), "")
	pan := ""
	if !m.logs.wrap {
		pan = "  ←/→ pan"
	}
	footer := "j/k scroll" + pan + "  / filter  m markdown  w wrap  a color  f follow  d details  h back  ? help  q detach"
	if w < 110 {
		footer = "j/k scroll" + pan + "  / filter  m/w/a modes  h back  ? help"
	}
	if w < 70 {
		footer = "j/k scroll  / filter  m/w/a modes  h back  ? help"
	}
	if w < 48 {
		footer = "↑↓ scroll  / filter  h back  ? help"
	}

	if m.logs.filtering {
		footer = "Type filter  Enter keep  Esc cancel  Ctrl-u clear  Ctrl-c detach"
	}
	lines = append(lines, m.style(footer, "8", false))
	return lines
}
