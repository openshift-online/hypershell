package tui

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/openshift-online/hypershell/components/cli/pkg/config"
)

const (
	DefaultInterval = 5 * time.Second
	MinInterval     = 2 * time.Second
	MaxBackoff      = 60 * time.Second
	// LookupMaxAge bounds how often the Gateways view refreshes the cluster
	// and release names it displays.
	LookupMaxAge = 60 * time.Second
	// RequestTimeout bounds every API request the interface sends.
	RequestTimeout = 30 * time.Second
	MinWidth       = 80
	MinHeight      = 20
)

// SessionExpiredMessage is shown once the refresh token is rejected.
const SessionExpiredMessage = "session expired, run 'hsctl login'"

// Options configures a Model. Now and After exist so tests can control time;
// they default to the wall clock and tea.Tick.
type Options struct {
	Source   Source
	APIURL   string
	Identity string
	Interval time.Duration
	// Context is the parent for every API request. When cancelled (for example
	// on quit via tea.WithContext), in-flight requests abort. Nil means
	// context.Background.
	Context context.Context
	// Clipboard sends text to the terminal clipboard. Nil disables copying.
	Clipboard func(string) error
	Now       func() time.Time
	After     func(time.Duration, tea.Msg) tea.Cmd
}

type mode int

const (
	modeNormal mode = iota
	modeFilter
	modeCommand
	modeDetail
	modeCreate
	modeDelete
	modeHelp
)

// viewState is one resource collection and its polling state.
type viewState struct {
	kind        Kind
	items       []Resource
	loaded      bool
	inFlight    bool
	pending     bool
	lastSuccess time.Time
	failures    int
	lastErr     error
	truncated   bool
	// gen invalidates scheduled ticks: only a tick carrying the current gen
	// triggers a refresh, so at most one polling chain exists per view.
	gen        int
	cursor     int
	selectedID string
	offset     int
	filter     string
	// seq numbers list requests. awaitCreated is a gateway this session
	// created; it stays listed and selected until a list requested after the
	// create (seq > awaitAfter) settles whether the API returns it.
	seq          int
	awaitCreated *Resource
	awaitAfter   int
}

// lookupState caches a collection used to label gateway rows and fill the
// provisioning form.
type lookupState struct {
	items     []Resource
	byID      map[string]Resource
	loaded    bool
	inFlight  bool
	fetchedAt time.Time
	err       error
}

type detailState struct {
	kind     Kind
	id       string
	name     string
	gone     bool
	inFlight bool
	err      string
	viewport viewport.Model
}

type (
	tickMsg struct {
		kind Kind
		gen  int
	}
	clockMsg struct{}
	listMsg  struct {
		kind   Kind
		seq    int
		result ListResult
		err    error
	}
	lookupMsg struct {
		kind   Kind
		result ListResult
		err    error
	}
	detailMsg struct {
		kind Kind
		id   string
		res  Resource
		err  error
	}
	createdMsg struct {
		res Resource
		err error
	}
	deletedMsg struct {
		id   string
		name string
		err  error
	}
)

// Model is the root Bubble Tea model for `hsctl ui`.
type Model struct {
	opts      Options
	width     int
	height    int
	active    Kind
	views     map[Kind]*viewState
	lookups   map[Kind]*lookupState
	mode      mode
	input     textinput.Model
	detail    *detailState
	form      *createForm
	confirm   *deleteConfirm
	status    string
	statusErr bool
	expired   bool
}

func New(opts Options) *Model {
	if opts.Interval <= 0 {
		opts.Interval = DefaultInterval
	}
	if opts.Context == nil {
		opts.Context = context.Background()
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.After == nil {
		opts.After = func(d time.Duration, msg tea.Msg) tea.Cmd {
			return tea.Tick(d, func(time.Time) tea.Msg { return msg })
		}
	}
	m := &Model{
		opts:    opts,
		active:  KindGateways,
		views:   map[Kind]*viewState{},
		lookups: map[Kind]*lookupState{KindClusters: {}, KindReleases: {}},
		input:   newInput(),
	}
	for _, k := range kinds {
		m.views[k] = &viewState{kind: k}
	}
	return m
}

func newInput() textinput.Model {
	in := textinput.New()
	in.Cursor.SetMode(cursor.CursorStatic)
	return in
}

func (m *Model) now() time.Time { return m.opts.Now() }

// requestCtx returns a timeout context derived from the program context so
// in-flight API calls abort when the UI quits.
func (m *Model) requestCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(m.opts.Context, RequestTimeout)
}

func (m *Model) Init() tea.Cmd {
	return tea.Batch(
		m.fetchList(KindGateways, false),
		m.fetchLookups(false),
		m.opts.After(time.Second, clockMsg{}),
	)
}

// fetchList starts a refresh of kind unless one is already running. When
// queue is set, a refresh requested during a running one runs as soon as it
// finishes; scheduled ticks never queue, so a slow API is never sent
// overlapping requests.
func (m *Model) fetchList(kind Kind, queue bool) tea.Cmd {
	v := m.views[kind]
	if m.expired {
		return nil
	}
	if v.inFlight {
		v.pending = v.pending || queue
		return nil
	}
	v.inFlight = true
	v.pending = false
	v.seq++
	src, seq := m.opts.Source, v.seq
	return func() tea.Msg {
		ctx, cancel := m.requestCtx()
		defer cancel()
		res, err := src.List(ctx, kind)
		return listMsg{kind: kind, seq: seq, result: res, err: err}
	}
}

// fetchLookups refreshes the cluster and release name caches when they are
// older than LookupMaxAge, or always when force is set.
func (m *Model) fetchLookups(force bool) tea.Cmd {
	if m.expired {
		return nil
	}
	var cmds []tea.Cmd
	for _, kind := range []Kind{KindClusters, KindReleases} {
		l := m.lookups[kind]
		if l.inFlight {
			continue
		}
		if !force && !l.fetchedAt.IsZero() && m.now().Sub(l.fetchedAt) < LookupMaxAge {
			continue
		}
		l.inFlight = true
		src := m.opts.Source
		cmds = append(cmds, func() tea.Msg {
			ctx, cancel := m.requestCtx()
			defer cancel()
			res, err := src.List(ctx, kind)
			return lookupMsg{kind: kind, result: res, err: err}
		})
	}
	return tea.Batch(cmds...)
}

func (m *Model) fetchDetail() tea.Cmd {
	d := m.detail
	if d == nil || d.inFlight || m.expired {
		return nil
	}
	d.inFlight = true
	src, kind, id := m.opts.Source, d.kind, d.id
	return func() tea.Msg {
		ctx, cancel := m.requestCtx()
		defer cancel()
		res, err := src.Get(ctx, kind, id)
		return detailMsg{kind: kind, id: id, res: res, err: err}
	}
}

// schedule arms the next tick for kind, replacing any earlier chain.
func (m *Model) schedule(kind Kind, delay time.Duration) tea.Cmd {
	v := m.views[kind]
	v.gen++
	return m.opts.After(delay, tickMsg{kind: kind, gen: v.gen})
}

// nextDelay is the configured interval, doubled per consecutive failure up to
// MaxBackoff.
func (m *Model) nextDelay(failures int) time.Duration {
	d := m.opts.Interval
	for i := 0; i < failures && d < MaxBackoff; i++ {
		d *= 2
	}
	return min(d, MaxBackoff)
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		if m.detail != nil {
			m.detail.viewport.Width = m.width
			m.detail.viewport.Height = m.detailHeight()
		}
		return m, nil
	case clockMsg:
		return m, m.opts.After(time.Second, clockMsg{})
	case tickMsg:
		v := m.views[msg.kind]
		if msg.kind != m.active || msg.gen != v.gen || m.expired {
			return m, nil
		}
		return m, m.fetchList(msg.kind, false)
	case listMsg:
		return m, m.onList(msg)
	case lookupMsg:
		m.onLookup(msg)
		return m, nil
	case detailMsg:
		m.onDetail(msg)
		return m, nil
	case createdMsg:
		return m, m.onCreated(msg)
	case deletedMsg:
		return m, m.onDeleted(msg)
	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			return m, tea.Quit
		}
		return m, m.onKey(msg)
	}
	return m, nil
}

func (m *Model) sessionExpired(err error) bool {
	if err != nil && errors.Is(err, config.ErrSessionExpired) {
		m.expired = true
		m.setStatus(SessionExpiredMessage, true)
		return true
	}
	return false
}

func (m *Model) onList(msg listMsg) tea.Cmd {
	v := m.views[msg.kind]
	v.inFlight = false
	if m.sessionExpired(msg.err) {
		return nil
	}
	if msg.err != nil {
		v.failures++
		v.lastErr = msg.err
	} else {
		v.failures = 0
		v.lastErr = nil
		v.loaded = true
		v.lastSuccess = m.now()
		v.items = msg.result.Items
		v.truncated = msg.result.Truncated
		m.keepCreated(v, msg.seq)
		sortResources(v.items)
		m.reselect(v)
	}
	if msg.kind != m.active {
		return nil
	}
	var cmds []tea.Cmd
	if v.pending {
		cmds = append(cmds, m.fetchList(msg.kind, false))
	} else {
		cmds = append(cmds, m.schedule(msg.kind, m.nextDelay(v.failures)))
	}
	if msg.kind == KindGateways && msg.err == nil {
		cmds = append(cmds, m.fetchLookups(false))
	}
	if m.mode == modeDetail && m.detail != nil && m.detail.kind == msg.kind {
		cmds = append(cmds, m.fetchDetail())
	}
	return tea.Batch(cmds...)
}

// keepCreated holds a just-created gateway in the list until a response to a
// request sent after the create arrives, so a refresh already in flight when
// the create landed does not drop it or move the selection.
func (m *Model) keepCreated(v *viewState, seq int) {
	created := v.awaitCreated
	if created == nil {
		return
	}
	for _, r := range v.items {
		if r.ID == created.ID {
			v.awaitCreated = nil
			return
		}
	}
	if seq > v.awaitAfter {
		v.awaitCreated = nil
		return
	}
	v.items = append(v.items, *created)
}

func (m *Model) onLookup(msg lookupMsg) {
	l := m.lookups[msg.kind]
	l.inFlight = false
	l.fetchedAt = m.now()
	if m.sessionExpired(msg.err) {
		return
	}
	if msg.err != nil {
		l.err = msg.err
	} else {
		l.err = nil
		l.loaded = true
		l.items = msg.result.Items
		sortResources(l.items)
		l.byID = make(map[string]Resource, len(l.items))
		for _, r := range l.items {
			l.byID[r.ID] = r
		}
	}
	if m.form != nil {
		m.fillForm()
	}
}

func (m *Model) fillForm() {
	if c := m.lookups[KindClusters]; c.loaded || c.err != nil {
		m.form.setClusters(c.items, c.err)
	}
	if r := m.lookups[KindReleases]; r.loaded || r.err != nil {
		m.form.setReleases(r.items, r.err)
	}
}

func (m *Model) onDetail(msg detailMsg) {
	d := m.detail
	if d == nil || d.id != msg.id || d.kind != msg.kind {
		return
	}
	d.inFlight = false
	if m.sessionExpired(msg.err) {
		return
	}
	switch {
	case StatusOf(msg.err) == http.StatusNotFound:
		d.gone = true
	case msg.err != nil:
		d.err = ReasonOf(msg.err)
	default:
		d.gone = false
		d.err = ""
		d.name = msg.res.DisplayName()
		d.viewport.SetContent(detailContent(d.kind, msg.res))
	}
}

func (m *Model) onCreated(msg createdMsg) tea.Cmd {
	if m.sessionExpired(msg.err) {
		return nil
	}
	if msg.err != nil {
		reason := ReasonOf(msg.err)
		if StatusOf(msg.err) == http.StatusForbidden {
			reason = sentence(reason, creatorRoleHint)
		}
		if m.form != nil {
			m.form.submitting = false
			m.form.err = reason
		} else {
			m.setStatus("Gateway was not created: "+reason, true)
		}
		return nil
	}
	if m.mode == modeCreate {
		m.mode = modeNormal
	}
	m.form = nil
	v := m.views[KindGateways]
	found := false
	for _, r := range v.items {
		if r.ID == msg.res.ID {
			found = true
			break
		}
	}
	if !found {
		v.items = append(v.items, msg.res)
		sortResources(v.items)
	}
	created := msg.res
	v.awaitCreated = &created
	v.awaitAfter = v.seq
	v.filter = ""
	v.selectedID = msg.res.ID
	m.reselect(v)
	m.setStatus(fmt.Sprintf("Gateway %s created.", msg.res.DisplayName()), false)
	v.gen++
	return m.fetchList(KindGateways, true)
}

func (m *Model) onDeleted(msg deletedMsg) tea.Cmd {
	if m.sessionExpired(msg.err) {
		return nil
	}
	if msg.err != nil {
		if m.confirm != nil && m.confirm.id == msg.id {
			m.confirm.submitting = false
			m.confirm.err = ReasonOf(msg.err)
		} else {
			m.setStatus(fmt.Sprintf("Gateway %s was not deleted: %s", msg.name, ReasonOf(msg.err)), true)
		}
		return nil
	}
	if m.mode == modeDelete {
		m.mode = modeNormal
	}
	m.confirm = nil
	m.setStatus(fmt.Sprintf("Gateway %s deleted.", msg.name), false)
	m.views[KindGateways].gen++
	return m.fetchList(KindGateways, true)
}

func (m *Model) setStatus(text string, isErr bool) {
	if m.expired && text != SessionExpiredMessage {
		return
	}
	m.status = text
	m.statusErr = isErr
}

// visibleRows is the active view's items after the filter.
func (m *Model) visibleRows(v *viewState) []Resource {
	if v.filter == "" {
		return v.items
	}
	needle := strings.ToLower(v.filter)
	var out []Resource
	for _, r := range v.items {
		if strings.Contains(strings.ToLower(r.Name), needle) || strings.Contains(strings.ToLower(r.ID), needle) {
			out = append(out, r)
		}
	}
	return out
}

// reselect keeps the selection on the same resource ID; when it is gone the
// selection stays at the same row position.
func (m *Model) reselect(v *viewState) {
	rows := m.visibleRows(v)
	for i, r := range rows {
		if r.ID == v.selectedID {
			v.cursor = i
			return
		}
	}
	m.moveCursor(v, v.cursor)
}

func (m *Model) moveCursor(v *viewState, to int) {
	rows := m.visibleRows(v)
	if len(rows) == 0 {
		v.cursor = 0
		v.selectedID = ""
		return
	}
	v.cursor = max(0, min(to, len(rows)-1))
	v.selectedID = rows[v.cursor].ID
}

func (m *Model) selected() (Resource, bool) {
	v := m.views[m.active]
	rows := m.visibleRows(v)
	if len(rows) == 0 || v.cursor >= len(rows) {
		return Resource{}, false
	}
	return rows[v.cursor], true
}

func (m *Model) switchView(kind Kind) tea.Cmd {
	if kind == m.active {
		return nil
	}
	m.views[m.active].filter = ""
	m.active = kind
	v := m.views[kind]
	v.filter = ""
	m.reselect(v)
	v.gen++
	cmds := []tea.Cmd{m.fetchList(kind, false)}
	if kind == KindGateways {
		cmds = append(cmds, m.fetchLookups(false))
	}
	return tea.Batch(cmds...)
}

func (m *Model) clusterLabel(id string) string {
	return m.lookupLabel(KindClusters, id, hubClusterLabel)
}

func (m *Model) releaseLabel(id string) string {
	return m.lookupLabel(KindReleases, id, defaultReleaseLabel)
}

func (m *Model) lookupLabel(kind Kind, id, emptyLabel string) string {
	if id == "" {
		return emptyLabel
	}
	l := m.lookups[kind]
	if r, ok := l.byID[id]; ok {
		return r.DisplayName()
	}
	if !l.loaded && l.err == nil {
		return resolvingLabel
	}
	return id + unresolvedSuffix
}

func (m *Model) detailHeight() int {
	return max(m.height-headerLines-footerLines-1, 1)
}

func (m *Model) onKey(msg tea.KeyMsg) tea.Cmd {
	// Keys typed faster than the terminal is read arrive as one multi-rune
	// event. Normal-mode bindings are single keys, so replay them one at a
	// time; each goes to whatever mode the previous one left active.
	if m.mode == modeNormal && msg.Type == tea.KeyRunes && len(msg.Runes) > 1 && !msg.Paste {
		cmds := make([]tea.Cmd, 0, len(msg.Runes))
		for _, r := range msg.Runes {
			cmds = append(cmds, m.onKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}))
		}
		return tea.Batch(cmds...)
	}
	switch m.mode {
	case modeFilter:
		return m.onFilterKey(msg)
	case modeCommand:
		return m.onCommandKey(msg)
	case modeDetail:
		return m.onDetailKey(msg)
	case modeCreate:
		return m.onCreateKey(msg)
	case modeDelete:
		return m.onDeleteKey(msg)
	case modeHelp:
		m.mode = modeNormal
		return nil
	}
	return m.onNormalKey(msg)
}

func (m *Model) onNormalKey(msg tea.KeyMsg) tea.Cmd {
	v := m.views[m.active]
	switch msg.String() {
	case "q":
		return tea.Quit
	case "1", "2", "3", "4":
		return m.switchView(kinds[int(msg.Runes[0]-'1')])
	case "up", "k":
		m.moveCursor(v, v.cursor-1)
	case "down", "j":
		m.moveCursor(v, v.cursor+1)
	case "pgup":
		m.moveCursor(v, v.cursor-m.tableHeight())
	case "pgdown":
		m.moveCursor(v, v.cursor+m.tableHeight())
	case "home", "g":
		m.moveCursor(v, 0)
	case "end", "G":
		m.moveCursor(v, len(m.visibleRows(v))-1)
	case "r":
		v.failures = 0
		v.gen++
		return m.fetchList(m.active, true)
	case "/":
		m.mode = modeFilter
		m.input = newInput()
		m.input.Prompt = "/"
		m.input.SetValue(v.filter)
		m.input.Focus()
	case ":":
		m.mode = modeCommand
		m.input = newInput()
		m.input.Prompt = ":"
		m.input.Focus()
	case "?":
		m.mode = modeHelp
	case "esc":
		if v.filter != "" {
			v.filter = ""
			m.reselect(v)
		}
	case "enter":
		return m.openDetail()
	case "n":
		if m.active == KindGateways && !m.expired {
			return m.openCreate()
		}
	case "d":
		if r, ok := m.selected(); ok && m.active == KindGateways && !m.expired {
			m.confirm = newDeleteConfirm(r)
			m.mode = modeDelete
		}
	}
	return nil
}

func (m *Model) onFilterKey(msg tea.KeyMsg) tea.Cmd {
	v := m.views[m.active]
	switch msg.Type {
	case tea.KeyEsc:
		v.filter = ""
		m.mode = modeNormal
		m.reselect(v)
		return nil
	case tea.KeyEnter:
		m.mode = modeNormal
		return nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	v.filter = m.input.Value()
	m.reselect(v)
	return cmd
}

var viewCommands = map[string]Kind{
	"gateways": KindGateways, "gw": KindGateways,
	"clusters": KindClusters, "mc": KindClusters,
	"releases": KindReleases, "rel": KindReleases,
	"networks": KindNetworks, "net": KindNetworks,
}

func (m *Model) onCommandKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.Type {
	case tea.KeyEsc:
		m.mode = modeNormal
		return nil
	case tea.KeyEnter:
		m.mode = modeNormal
		command := strings.TrimSpace(m.input.Value())
		switch command {
		case "":
			return nil
		case "q", "quit":
			return tea.Quit
		}
		if kind, ok := viewCommands[command]; ok {
			return m.switchView(kind)
		}
		m.setStatus(fmt.Sprintf("Unknown command: %s", command), true)
		return nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return cmd
}

func (m *Model) openDetail() tea.Cmd {
	r, ok := m.selected()
	if !ok {
		return nil
	}
	vp := viewport.New(m.width, m.detailHeight())
	vp.SetContent(detailContent(m.active, r))
	m.detail = &detailState{kind: m.active, id: r.ID, name: r.DisplayName(), viewport: vp}
	m.mode = modeDetail
	return m.fetchDetail()
}

func (m *Model) onDetailKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "esc", "q":
		m.mode = modeNormal
		m.detail = nil
		return nil
	case "y":
		m.copyID(m.detail.id)
		return nil
	case "r":
		v := m.views[m.active]
		v.failures = 0
		v.gen++
		return tea.Batch(m.fetchList(m.active, true), m.fetchDetail())
	case "?":
		return nil
	}
	var cmd tea.Cmd
	m.detail.viewport, cmd = m.detail.viewport.Update(msg)
	return cmd
}

func (m *Model) copyID(id string) {
	if m.opts.Clipboard == nil {
		m.setStatus("Clipboard is not available in this terminal.", true)
		return
	}
	if err := m.opts.Clipboard(id); err != nil {
		m.setStatus("Could not send the ID to the clipboard: "+err.Error(), true)
		return
	}
	m.setStatus(fmt.Sprintf("Sent %s to the terminal clipboard.", id), false)
}

func (m *Model) openCreate() tea.Cmd {
	m.form = newCreateForm()
	m.fillForm()
	m.mode = modeCreate
	return m.fetchLookups(false)
}

func (m *Model) onCreateKey(msg tea.KeyMsg) tea.Cmd {
	f := m.form
	switch msg.Type {
	case tea.KeyEsc:
		m.mode = modeNormal
		m.form = nil
		return nil
	case tea.KeyTab:
		f.setFocus(f.focus + 1)
		return nil
	case tea.KeyShiftTab:
		f.setFocus(f.focus - 1)
		return nil
	case tea.KeyCtrlR:
		return m.fetchLookups(true)
	case tea.KeyEnter:
		if f.submitting {
			return nil
		}
		req, problem := f.request()
		if problem != "" {
			f.err = problem
			return nil
		}
		f.err = ""
		f.submitting = true
		src := m.opts.Source
		return func() tea.Msg {
			ctx, cancel := m.requestCtx()
			defer cancel()
			res, err := src.CreateGateway(ctx, req)
			return createdMsg{res: res, err: err}
		}
	}
	if f.submitting {
		return nil
	}
	switch f.focus {
	case formFieldPlacement:
		f.placement.update(msg)
	case formFieldRelease:
		f.release.update(msg)
	default:
		var cmd tea.Cmd
		f.name, cmd = f.name.Update(msg)
		return cmd
	}
	return nil
}

func (m *Model) onDeleteKey(msg tea.KeyMsg) tea.Cmd {
	c := m.confirm
	switch msg.Type {
	case tea.KeyEsc:
		m.mode = modeNormal
		m.confirm = nil
		return nil
	case tea.KeyEnter:
		if c.submitting || !c.confirmed() {
			return nil
		}
		c.submitting = true
		c.err = ""
		src, id, name := m.opts.Source, c.id, c.name
		return func() tea.Msg {
			ctx, cancel := m.requestCtx()
			defer cancel()
			return deletedMsg{id: id, name: name, err: src.DeleteGateway(ctx, id)}
		}
	}
	if c.submitting {
		return nil
	}
	var cmd tea.Cmd
	c.input, cmd = c.input.Update(msg)
	return cmd
}
