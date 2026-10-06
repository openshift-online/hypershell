package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

const (
	hubPlacementLabel     = "Hub cluster (default)"
	creatorRoleHint       = "Provisioning requires the gateway:creator role."
	selectVisibleOptions  = 6
	formFieldName         = 0
	formFieldPlacement    = 1
	formFieldCount        = 2
	formNameCharLimit     = 253
	formSelectFilterLimit = 64
)

// option is one choice in a selectList. An empty id is the default choice.
type option struct {
	id    string
	label string
	// match is the text the filter searches: the resource name, or the
	// label for the default choice.
	match string
}

// selectList is a filterable single-select. The selection is the highlighted
// option among those matching the filter; when nothing matches there is no
// selection, so a filter never silently keeps a hidden choice.
type selectList struct {
	title   string
	options []option
	filter  string
	cursor  int
	// unavailable names the list when it failed to load; the default option
	// stays selectable.
	unavailable string
}

func (s *selectList) visible() []option {
	if s.filter == "" {
		return s.options
	}
	needle := strings.ToLower(s.filter)
	var out []option
	for _, o := range s.options {
		if strings.Contains(strings.ToLower(o.match), needle) {
			out = append(out, o)
		}
	}
	return out
}

func (s *selectList) selected() (option, bool) {
	vis := s.visible()
	if len(vis) == 0 {
		return option{}, false
	}
	return vis[min(s.cursor, len(vis)-1)], true
}

// setOptions replaces the non-default options while keeping the current
// choice selected when it is still offered.
func (s *selectList) setOptions(def option, items []option) {
	current, had := s.selected()
	s.options = append([]option{def}, items...)
	s.cursor = 0
	if !had {
		return
	}
	for i, o := range s.visible() {
		if o.id == current.id {
			s.cursor = i
			return
		}
	}
}

func (s *selectList) update(msg tea.KeyMsg) {
	switch msg.Type {
	case tea.KeyUp:
		if s.cursor > 0 {
			s.cursor--
		}
	case tea.KeyDown:
		if s.cursor < len(s.visible())-1 {
			s.cursor++
		}
	case tea.KeyBackspace:
		if r := []rune(s.filter); len(r) > 0 {
			s.filter = string(r[:len(r)-1])
			s.cursor = 0
		}
	case tea.KeyRunes, tea.KeySpace:
		if len([]rune(s.filter)) < formSelectFilterLimit {
			s.filter += string(msg.Runes)
			s.cursor = 0
		}
	}
}

func (s *selectList) view(focused bool, width int) string {
	var sb strings.Builder
	label := s.title
	if focused {
		label = focusedLabel.Render(label)
	}
	sb.WriteString(label)
	if s.filter != "" {
		sb.WriteString(mutedStyle.Render("  filter: " + s.filter))
	}
	sb.WriteString("\n")
	vis := s.visible()
	if len(vis) == 0 {
		sb.WriteString(errorStyle.Render("  No match. Backspace to widen the filter."))
		sb.WriteString("\n")
	}
	cursor := min(s.cursor, max(len(vis)-1, 0))
	start := max(0, min(cursor-selectVisibleOptions/2, len(vis)-selectVisibleOptions))
	end := min(len(vis), start+selectVisibleOptions)
	for i := start; i < end; i++ {
		line := fit(vis[i].label, max(width-4, 10))
		if i == cursor {
			line = "> " + line
			if focused {
				line = selectedRow.Render(line)
			}
		} else {
			line = "  " + line
		}
		sb.WriteString(line)
		sb.WriteString("\n")
	}
	if end < len(vis) {
		sb.WriteString(mutedStyle.Render(fmt.Sprintf("  %d more; type to filter", len(vis)-end)))
		sb.WriteString("\n")
	}
	if s.unavailable != "" {
		sb.WriteString(warnStyle.Render(fmt.Sprintf("  %s could not be loaded. Press ctrl+r to retry.", s.unavailable)))
		sb.WriteString("\n")
	}
	return sb.String()
}

// createForm is the gateway provisioning form (TUI-07). It collects a name and
// a placement; the platform default image applies.
type createForm struct {
	name       textinput.Model
	placement  selectList
	focus      int
	submitting bool
	err        string
}

func newCreateForm() *createForm {
	name := textinput.New()
	name.Placeholder = "gateway name"
	name.CharLimit = formNameCharLimit
	name.Prompt = ""
	name.Cursor.SetMode(cursor.CursorStatic)
	name.Focus()
	f := &createForm{
		name:      name,
		placement: selectList{title: "Placement"},
	}
	f.placement.setOptions(option{label: hubPlacementLabel, match: hubPlacementLabel}, nil)
	return f
}

func (f *createForm) setClusters(items []Resource, err error) {
	opts := make([]option, 0, len(items))
	for _, r := range items {
		detail := strings.Trim(strings.Join(nonEmpty(r.Field("provider"), r.Field("region")), " / "), " ")
		label := r.DisplayName()
		if detail != "" {
			label += "  " + mutedStyle.Render(detail)
		}
		opts = append(opts, option{id: r.ID, label: label, match: r.DisplayName()})
	}
	f.placement.setOptions(option{label: hubPlacementLabel, match: hubPlacementLabel}, opts)
	f.placement.unavailable = ""
	if err != nil {
		f.placement.unavailable = "Managed clusters"
	}
}

func nonEmpty(values ...string) []string {
	var out []string
	for _, v := range values {
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

// request validates the form and builds the create body. A non-empty problem
// is the inline message that blocks submission.
func (f *createForm) request() (req GatewayCreate, problem string) {
	name := strings.TrimSpace(f.name.Value())
	if name == "" {
		return GatewayCreate{}, "Name is required."
	}
	placement, ok := f.placement.selected()
	if !ok {
		return GatewayCreate{}, "Select a placement."
	}
	return GatewayCreate{Name: name, ClusterID: placement.id}, ""
}

func (f *createForm) setFocus(i int) {
	f.focus = (i + formFieldCount) % formFieldCount
	if f.focus == formFieldName {
		f.name.Focus()
	} else {
		f.name.Blur()
	}
}

func (f *createForm) view(width int) string {
	var sb strings.Builder
	sb.WriteString(titleStyle.Render("Provision a gateway"))
	sb.WriteString("\n\n")
	label := "Name"
	if f.focus == formFieldName {
		label = focusedLabel.Render(label)
	}
	sb.WriteString(label + "\n  " + f.name.View() + "\n\n")
	sb.WriteString(f.placement.view(f.focus == formFieldPlacement, width))
	if f.submitting {
		sb.WriteString("\n" + progressStyle.Render("Provisioning..."))
	}
	if f.err != "" {
		sb.WriteString("\n" + errorStyle.Render(f.err))
	}
	return sb.String()
}

// deleteConfirm asks for the gateway name before deleting it (TUI-08).
type deleteConfirm struct {
	id         string
	name       string
	input      textinput.Model
	submitting bool
	err        string
}

func newDeleteConfirm(r Resource) *deleteConfirm {
	in := textinput.New()
	in.Prompt = ""
	in.CharLimit = formNameCharLimit
	in.Cursor.SetMode(cursor.CursorStatic)
	in.Focus()
	return &deleteConfirm{id: r.ID, name: r.DisplayName(), input: in}
}

func (d *deleteConfirm) confirmed() bool {
	return d.input.Value() == d.name
}

func (d *deleteConfirm) view() string {
	var sb strings.Builder
	sb.WriteString(titleStyle.Render("Delete gateway " + d.name))
	sb.WriteString("\n\n")
	sb.WriteString("This deletes the gateway and its workloads. Type the gateway name to confirm.\n\n  ")
	sb.WriteString(d.input.View())
	sb.WriteString("\n\n")
	switch {
	case d.submitting:
		sb.WriteString(progressStyle.Render("Deleting..."))
	case d.confirmed():
		sb.WriteString("Press enter to delete, esc to cancel.")
	default:
		sb.WriteString(mutedStyle.Render("enter is disabled until the name matches; esc to cancel."))
	}
	if d.err != "" {
		sb.WriteString("\n" + errorStyle.Render(d.err))
	}
	return sb.String()
}
