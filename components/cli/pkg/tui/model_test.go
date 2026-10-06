package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/openshift-online/hypershell/components/cli/pkg/config"
)

// fakeSource is an in-memory Source. Responses are set per kind; every call is
// recorded.
type fakeSource struct {
	mu        sync.Mutex
	lists     map[Kind]ListResult
	listErrs  map[Kind]error
	listCalls map[Kind]int
	getErr    error
	createRes Resource
	createErr error
	creates   []GatewayCreate
	deleteErr error
	deletes   []string
}

func newFakeSource() *fakeSource {
	return &fakeSource{
		lists:     map[Kind]ListResult{},
		listErrs:  map[Kind]error{},
		listCalls: map[Kind]int{},
	}
}

func (f *fakeSource) List(_ context.Context, kind Kind) (ListResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.listCalls[kind]++
	if err := f.listErrs[kind]; err != nil {
		return ListResult{}, err
	}
	res := f.lists[kind]
	res.Items = append([]Resource(nil), res.Items...)
	return res, nil
}

func (f *fakeSource) Get(_ context.Context, kind Kind, id string) (Resource, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.getErr != nil {
		return Resource{}, f.getErr
	}
	for _, r := range f.lists[kind].Items {
		if r.ID == id {
			return r, nil
		}
	}
	return Resource{}, &APIError{Status: 404, Reason: "not found"}
}

func (f *fakeSource) CreateGateway(_ context.Context, req GatewayCreate) (Resource, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.creates = append(f.creates, req)
	if f.createErr == nil {
		list := f.lists[KindGateways]
		list.Items = append(list.Items, f.createRes)
		f.lists[KindGateways] = list
	}
	return f.createRes, f.createErr
}

func (f *fakeSource) DeleteGateway(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deletes = append(f.deletes, id)
	return f.deleteErr
}

func res(t *testing.T, fields map[string]any) Resource {
	t.Helper()
	raw, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	r, err := DecodeResource(raw)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func gateway(t *testing.T, id, name string, extra ...any) Resource {
	fields := map[string]any{"id": id, "name": name, "cluster_id": "", "phase": "Running"}
	for i := 0; i+1 < len(extra); i += 2 {
		fields[extra[i].(string)] = extra[i+1]
	}
	return res(t, fields)
}

type scheduled struct {
	delay time.Duration
	msg   tea.Msg
}

// harness drives a Model synchronously: commands run inline and scheduled
// ticks are recorded instead of sleeping.
type harness struct {
	t         *testing.T
	m         *Model
	src       *fakeSource
	now       time.Time
	scheduled []scheduled
	quit      bool
}

func newHarness(t *testing.T, src *fakeSource) *harness {
	t.Helper()
	h := &harness{t: t, src: src, now: time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)}
	h.m = New(Options{
		Source:   src,
		APIURL:   "https://api.example.com",
		Identity: "alice",
		Now:      func() time.Time { return h.now },
		After: func(d time.Duration, msg tea.Msg) tea.Cmd {
			h.scheduled = append(h.scheduled, scheduled{d, msg})
			return nil
		},
	})
	h.dispatch(tea.WindowSizeMsg{Width: 140, Height: 30})
	h.run(h.m.Init())
	return h
}

func (h *harness) run(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	h.dispatch(cmd())
}

func (h *harness) dispatch(msg tea.Msg) {
	switch msg := msg.(type) {
	case nil:
	case tea.BatchMsg:
		for _, c := range msg {
			h.run(c)
		}
	case tea.QuitMsg:
		h.quit = true
	default:
		_, cmd := h.m.Update(msg)
		h.run(cmd)
	}
}

var namedKeys = map[string]tea.KeyType{
	"enter": tea.KeyEnter, "esc": tea.KeyEsc, "tab": tea.KeyTab, "shift+tab": tea.KeyShiftTab,
	"up": tea.KeyUp, "down": tea.KeyDown, "backspace": tea.KeyBackspace, "ctrl+r": tea.KeyCtrlR,
	"ctrl+c": tea.KeyCtrlC,
}

func keyMsg(k string) tea.KeyMsg {
	if t, ok := namedKeys[k]; ok {
		return tea.KeyMsg{Type: t}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
}

func (h *harness) key(keys ...string) {
	for _, k := range keys {
		h.dispatch(keyMsg(k))
	}
}

func (h *harness) typeText(s string) {
	for _, r := range s {
		if r == ' ' {
			h.dispatch(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}})
			continue
		}
		h.dispatch(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
}

// lastTick is the most recently scheduled refresh tick for kind.
func (h *harness) lastTick(kind Kind) (scheduled, bool) {
	for i := len(h.scheduled) - 1; i >= 0; i-- {
		if tick, ok := h.scheduled[i].msg.(tickMsg); ok && tick.kind == kind {
			return h.scheduled[i], true
		}
	}
	return scheduled{}, false
}

func (h *harness) fireTick(kind Kind) {
	h.t.Helper()
	s, ok := h.lastTick(kind)
	if !ok {
		h.t.Fatalf("no tick scheduled for %s", kind.Title())
	}
	h.now = h.now.Add(s.delay)
	h.dispatch(s.msg)
}

func (h *harness) view() string {
	return h.m.View()
}

func assertContains(t *testing.T, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Errorf("expected output to contain %q, got:\n%s", needle, haystack)
	}
}

func assertNotContains(t *testing.T, haystack, needle string) {
	t.Helper()
	if strings.Contains(haystack, needle) {
		t.Errorf("expected output not to contain %q, got:\n%s", needle, haystack)
	}
}

func threeGateways(t *testing.T, src *fakeSource) {
	src.lists[KindGateways] = ListResult{Items: []Resource{
		gateway(t, "g1", "gw-a"), gateway(t, "g2", "gw-b"), gateway(t, "g3", "gw-c"),
	}}
}

func TestPollingKeepsDataStaleAndBacksOff(t *testing.T) {
	src := newFakeSource()
	threeGateways(t, src)
	h := newHarness(t, src)

	if s, _ := h.lastTick(KindGateways); s.delay != DefaultInterval {
		t.Fatalf("first delay = %s, want %s", s.delay, DefaultInterval)
	}

	src.listErrs[KindGateways] = fmt.Errorf("connection refused")
	h.fireTick(KindGateways)
	if s, _ := h.lastTick(KindGateways); s.delay != 10*time.Second {
		t.Errorf("delay after one failure = %s, want 10s", s.delay)
	}
	h.fireTick(KindGateways)
	if s, _ := h.lastTick(KindGateways); s.delay != 20*time.Second {
		t.Errorf("delay after two failures = %s, want 20s", s.delay)
	}
	view := h.view()
	for _, name := range []string{"gw-a", "gw-b", "gw-c"} {
		assertContains(t, view, name)
	}
	assertContains(t, view, "STALE")

	delete(src.listErrs, KindGateways)
	h.fireTick(KindGateways)
	if s, _ := h.lastTick(KindGateways); s.delay != DefaultInterval {
		t.Errorf("delay after recovery = %s, want %s", s.delay, DefaultInterval)
	}
	assertNotContains(t, h.view(), "STALE")
}

func TestBackoffIsCapped(t *testing.T) {
	m := New(Options{Source: newFakeSource()})
	if got := m.nextDelay(10); got != MaxBackoff {
		t.Errorf("nextDelay(10) = %s, want %s", got, MaxBackoff)
	}
}

func TestSlowAPIDoesNotStackRequests(t *testing.T) {
	src := newFakeSource()
	threeGateways(t, src)
	h := newHarness(t, src)
	s, _ := h.lastTick(KindGateways)

	_, inFlight := h.m.Update(s.msg)
	if inFlight == nil {
		t.Fatal("first tick did not start a refresh")
	}
	gen := h.m.views[KindGateways].gen
	for range 2 {
		if _, cmd := h.m.Update(tickMsg{kind: KindGateways, gen: gen}); cmd != nil {
			t.Fatal("a tick during a running refresh started another request")
		}
	}
	before := src.listCalls[KindGateways]
	h.run(inFlight)
	if got := src.listCalls[KindGateways] - before; got != 1 {
		t.Errorf("requests while slow = %d, want 1", got)
	}
}

func TestSelectionFollowsIDAcrossRefresh(t *testing.T) {
	src := newFakeSource()
	threeGateways(t, src)
	h := newHarness(t, src)
	h.key("down")
	if r, _ := h.m.selected(); r.Name != "gw-b" {
		t.Fatalf("selected %q, want gw-b", r.Name)
	}

	src.lists[KindGateways] = ListResult{Items: append(src.lists[KindGateways].Items, gateway(t, "g0", "gw-0"))}
	h.fireTick(KindGateways)
	v := h.m.views[KindGateways]
	if r, _ := h.m.selected(); r.Name != "gw-b" || v.cursor != 2 {
		t.Errorf("after reorder selected %q at row %d, want gw-b at row 2", r.Name, v.cursor)
	}

	src.lists[KindGateways] = ListResult{Items: []Resource{gateway(t, "g0", "gw-0"), gateway(t, "g1", "gw-a"), gateway(t, "g3", "gw-c")}}
	h.fireTick(KindGateways)
	if r, _ := h.m.selected(); r.Name != "gw-c" {
		t.Errorf("after removal selected %q, want gw-c at the same position", r.Name)
	}
}

func TestGatewayColumnsResolveNames(t *testing.T) {
	src := newFakeSource()
	src.lists[KindGateways] = ListResult{Items: []Resource{
		gateway(t, "g1", "on-hub"),
		gateway(t, "g2", "on-east", "cluster_id", "c1"),
		gateway(t, "g3", "orphan", "cluster_id", "gone", "phase", "Upgrading"),
		gateway(t, "g4", "fresh", "phase", ""),
	}}
	src.lists[KindClusters] = ListResult{Items: []Resource{res(t, map[string]any{"id": "c1", "name": "mc-east"})}}
	h := newHarness(t, src)

	view := h.view()
	assertContains(t, view, hubClusterLabel)
	assertContains(t, view, "mc-east")
	assertContains(t, view, "gone (unresolved)")
	assertContains(t, view, "Upgrading")
	assertNotContains(t, view, "c1")
}

func TestPhaseStyles(t *testing.T) {
	for _, phase := range []string{"Running", "Pending", "Provisioning", "Degraded", "Failed"} {
		if _, neutral := phaseStyle(phase).GetForeground().(lipgloss.NoColor); neutral {
			t.Errorf("canonical phase %q has no distinct style", phase)
		}
	}
	for _, phase := range []string{"Upgrading", ""} {
		if _, neutral := phaseStyle(phase).GetForeground().(lipgloss.NoColor); !neutral {
			t.Errorf("non-canonical phase %q is not neutral", phase)
		}
	}
}

func TestLookupsRefreshAtMostEveryMinute(t *testing.T) {
	src := newFakeSource()
	threeGateways(t, src)
	h := newHarness(t, src)
	if src.listCalls[KindClusters] != 1 {
		t.Fatalf("cluster lookups at start = %d, want 1", src.listCalls[KindClusters])
	}
	h.fireTick(KindGateways)
	h.fireTick(KindGateways)
	if src.listCalls[KindClusters] != 1 {
		t.Errorf("cluster lookups after 10s = %d, want 1", src.listCalls[KindClusters])
	}
	h.now = h.now.Add(LookupMaxAge)
	h.fireTick(KindGateways)
	if src.listCalls[KindClusters] != 2 {
		t.Errorf("cluster lookups after a minute = %d, want 2", src.listCalls[KindClusters])
	}
}

func TestFilterIsLiteral(t *testing.T) {
	src := newFakeSource()
	src.lists[KindGateways] = ListResult{Items: []Resource{gateway(t, "g1", "prod_a"), gateway(t, "g2", "prodXa")}}
	h := newHarness(t, src)
	before := src.listCalls[KindGateways]

	h.key("/")
	h.typeText("PROD_")
	rows := h.m.visibleRows(h.m.views[KindGateways])
	if len(rows) != 1 || rows[0].Name != "prod_a" {
		t.Fatalf("filter rows = %v, want only prod_a", rows)
	}
	assertContains(t, h.view(), `1 of 2 match "PROD_"`)
	if src.listCalls[KindGateways] != before {
		t.Error("filtering sent an API request")
	}
	h.key("enter")
	h.fireTick(KindGateways)
	if got := len(h.m.visibleRows(h.m.views[KindGateways])); got != 1 {
		t.Errorf("filter did not persist across refresh: %d rows", got)
	}
	h.key("esc")
	if got := len(h.m.visibleRows(h.m.views[KindGateways])); got != 2 {
		t.Errorf("esc did not clear the filter: %d rows", got)
	}
}

func TestSwitchViewMovesPolling(t *testing.T) {
	src := newFakeSource()
	threeGateways(t, src)
	h := newHarness(t, src)
	h.key("/")
	h.typeText("gw-a")
	h.key("enter")
	gatewayTick, _ := h.lastTick(KindGateways)

	h.key("2")
	if h.m.active != KindClusters {
		t.Fatalf("active view = %s, want clusters", h.m.active.Title())
	}
	if src.listCalls[KindClusters] != 2 {
		t.Errorf("cluster list calls = %d, want lookup plus view refresh", src.listCalls[KindClusters])
	}
	before := src.listCalls[KindGateways]
	h.dispatch(gatewayTick.msg)
	if src.listCalls[KindGateways] != before {
		t.Error("an inactive view was polled")
	}
	h.key("1")
	if h.m.views[KindGateways].filter != "" {
		t.Error("filter survived a view change")
	}
}

func TestCommandBar(t *testing.T) {
	h := newHarness(t, newFakeSource())
	h.key(":")
	h.typeText("mc")
	h.key("enter")
	if h.m.active != KindClusters {
		t.Fatalf("active = %s, want clusters", h.m.active.Title())
	}
	h.key(":")
	h.typeText("nope")
	h.key("enter")
	if h.m.active != KindClusters {
		t.Error("unknown command changed the view")
	}
	assertContains(t, h.view(), "Unknown command: nope")
	h.key(":")
	h.typeText("q")
	h.key("enter")
	if !h.quit {
		t.Error(":q did not quit")
	}
}

func TestCreateOnHubTrimsName(t *testing.T) {
	src := newFakeSource()
	threeGateways(t, src)
	src.createRes = gateway(t, "g9", "demo", "phase", "Pending")
	h := newHarness(t, src)

	h.key("n")
	h.typeText("  demo  ")
	h.key("enter")

	if len(src.creates) != 1 {
		t.Fatalf("create requests = %d, want 1", len(src.creates))
	}
	body, _ := json.Marshal(src.creates[0])
	if string(body) != `{"name":"demo","cluster_id":""}` {
		t.Errorf("request body = %s", body)
	}
	if h.m.mode != modeNormal {
		t.Errorf("form still open after 201")
	}
	if r, _ := h.m.selected(); r.ID != "g9" {
		t.Errorf("selected %q, want the new gateway g9", r.ID)
	}
	assertContains(t, h.view(), "Gateway demo created.")
}

func TestCreatedGatewayStaysSelectedThroughInFlightRefresh(t *testing.T) {
	src := newFakeSource()
	threeGateways(t, src)
	src.createRes = gateway(t, "g9", "demo")
	h := newHarness(t, src)

	// A refresh starts, and its response without the new gateway is held
	// until after the create lands.
	s, _ := h.lastTick(KindGateways)
	_, inFlight := h.m.Update(s.msg)
	stale := inFlight().(listMsg)

	h.key("n")
	h.typeText("demo")
	h.key("enter")
	if r, _ := h.m.selected(); r.ID != "g9" {
		t.Fatalf("selected %q after create, want g9", r.ID)
	}

	h.dispatch(stale)
	if r, _ := h.m.selected(); r.ID != "g9" {
		t.Errorf("stale refresh moved the selection to %q", r.ID)
	}
	if src.listCalls[KindGateways] < 3 {
		t.Errorf("no refresh was queued after the create")
	}
}

func TestCreateOnManagedCluster(t *testing.T) {
	src := newFakeSource()
	src.lists[KindClusters] = ListResult{Items: []Resource{
		res(t, map[string]any{"id": "c1", "name": "mc-east", "provider": "aws", "region": "us-east-1"}),
		res(t, map[string]any{"id": "c2", "name": "mc-west"}),
	}}
	src.createRes = gateway(t, "g9", "demo")
	h := newHarness(t, src)

	h.key("n")
	h.typeText("demo")
	h.key("tab")
	h.typeText("mc-e")
	h.key("enter")

	if len(src.creates) != 1 {
		t.Fatalf("create requests = %d, want 1", len(src.creates))
	}
	if got, want := src.creates[0], (GatewayCreate{Name: "demo", ClusterID: "c1"}); got != want {
		t.Errorf("request = %+v, want %+v", got, want)
	}
}

func TestCreateFilterWithNoMatchBlocksSubmit(t *testing.T) {
	src := newFakeSource()
	h := newHarness(t, src)
	h.key("n")
	h.typeText("demo")
	h.key("tab")
	h.typeText("zzz")
	h.key("enter")
	if len(src.creates) != 0 {
		t.Fatal("submitted with no placement selected")
	}
	assertContains(t, h.view(), "Select a placement.")
}

func TestCreateForbiddenKeepsInput(t *testing.T) {
	src := newFakeSource()
	src.createErr = &APIError{Status: 403, Reason: "caller lacks permission"}
	h := newHarness(t, src)
	h.key("n")
	h.typeText("demo")
	h.key("enter")

	if h.m.mode != modeCreate {
		t.Fatal("form closed after 403")
	}
	if h.m.form.name.Value() != "demo" {
		t.Errorf("name input = %q, want it unchanged", h.m.form.name.Value())
	}
	view := h.view()
	assertContains(t, view, "caller lacks permission")
	assertContains(t, view, "gateway:creator")
}

func TestCreateIgnoresDoubleSubmit(t *testing.T) {
	src := newFakeSource()
	src.createRes = gateway(t, "g9", "demo")
	h := newHarness(t, src)
	h.key("n")
	h.typeText("demo")

	_, first := h.m.Update(keyMsg("enter"))
	if first == nil {
		t.Fatal("submit sent nothing")
	}
	if _, second := h.m.Update(keyMsg("enter")); second != nil {
		t.Fatal("second submit while in flight produced a request")
	}
	h.run(first)
	if len(src.creates) != 1 {
		t.Errorf("create requests = %d, want 1", len(src.creates))
	}
}

func TestCreateWithClusterListUnavailable(t *testing.T) {
	src := newFakeSource()
	src.listErrs[KindClusters] = fmt.Errorf("503")
	h := newHarness(t, src)
	h.key("n")

	if o, ok := h.m.form.placement.selected(); !ok || o.id != "" {
		t.Fatalf("placement = %+v, want the hub selected", o)
	}
	view := h.view()
	assertContains(t, view, hubPlacementLabel)
	assertContains(t, view, "Managed clusters could not be loaded. Press ctrl+r to retry.")

	h.typeText("demo")
	delete(src.listErrs, KindClusters)
	src.lists[KindClusters] = ListResult{Items: []Resource{res(t, map[string]any{"id": "c1", "name": "mc-east"})}}
	before := src.listCalls[KindClusters]
	h.key("ctrl+r")
	if src.listCalls[KindClusters] != before+1 {
		t.Error("ctrl+r did not retry the cluster list")
	}
	if h.m.form.name.Value() != "demo" {
		t.Error("retry cleared the name input")
	}
	assertContains(t, h.view(), "mc-east")
}

func TestCreateEscSendsNothing(t *testing.T) {
	src := newFakeSource()
	h := newHarness(t, src)
	h.key("n")
	h.typeText("demo")
	h.key("esc")
	if h.m.mode != modeNormal || len(src.creates) != 0 {
		t.Error("esc did not cancel without a request")
	}
}

func TestDeleteRequiresExactName(t *testing.T) {
	src := newFakeSource()
	src.lists[KindGateways] = ListResult{Items: []Resource{gateway(t, "g1", "demo")}}
	h := newHarness(t, src)

	h.key("d")
	h.typeText("dem")
	h.key("enter")
	if len(src.deletes) != 0 {
		t.Fatal("deleted with a mistyped name")
	}
	h.typeText("o")
	h.key("enter")
	if len(src.deletes) != 1 || src.deletes[0] != "g1" {
		t.Fatalf("deletes = %v, want [g1]", src.deletes)
	}
	assertContains(t, h.view(), "Gateway demo deleted.")
}

func TestDeleteFailureKeepsGateway(t *testing.T) {
	src := newFakeSource()
	src.lists[KindGateways] = ListResult{Items: []Resource{gateway(t, "g1", "demo")}}
	src.deleteErr = &APIError{Status: 403, Reason: "not an owner"}
	h := newHarness(t, src)
	h.key("d")
	h.typeText("demo")
	h.key("enter")
	view := h.view()
	assertContains(t, view, "not an owner")
	h.key("esc")
	assertContains(t, h.view(), "demo")
}

func TestDeleteOnlyInGatewaysView(t *testing.T) {
	src := newFakeSource()
	src.lists[KindClusters] = ListResult{Items: []Resource{res(t, map[string]any{"id": "c1", "name": "mc-east"})}}
	h := newHarness(t, src)
	h.key("2", "d")
	if h.m.mode != modeNormal {
		t.Error("delete offered outside the Gateways view")
	}
}

func TestDetailShowsYAMLAndHandlesDeletion(t *testing.T) {
	src := newFakeSource()
	src.lists[KindGateways] = ListResult{Items: []Resource{gateway(t, "g1", "gw-a", "phase", "Pending")}}
	h := newHarness(t, src)
	h.key("enter")
	view := ansi.Strip(h.view())
	assertContains(t, view, "name: gw-a")
	assertContains(t, view, "available once the gateway is Running")

	src.getErr = &APIError{Status: 404}
	h.fireTick(KindGateways)
	assertContains(t, h.view(), "gw-a no longer exists.")
	h.key("esc")
	if h.m.mode != modeNormal {
		t.Error("esc did not leave the detail view")
	}
}

func TestDetailCopiesID(t *testing.T) {
	src := newFakeSource()
	threeGateways(t, src)
	h := newHarness(t, src)
	var copied string
	h.m.opts.Clipboard = func(s string) error { copied = s; return nil }
	h.key("enter", "y")
	if copied != "g1" {
		t.Errorf("copied %q, want g1", copied)
	}
	assertContains(t, h.view(), "Sent g1 to the terminal clipboard.")
}

func TestSmallTerminal(t *testing.T) {
	src := newFakeSource()
	threeGateways(t, src)
	h := newHarness(t, src)
	h.dispatch(tea.WindowSizeMsg{Width: 70, Height: 30})
	assertContains(t, h.view(), "needs at least 80x20")
	h.dispatch(tea.WindowSizeMsg{Width: 100, Height: 30})
	assertContains(t, h.view(), "gw-a")
}

func TestViewFitsTerminal(t *testing.T) {
	src := newFakeSource()
	threeGateways(t, src)
	h := newHarness(t, src)
	h.dispatch(tea.WindowSizeMsg{Width: 80, Height: 20})
	lines := strings.Split(h.view(), "\n")
	if len(lines) != 20 {
		t.Errorf("view has %d lines, want 20", len(lines))
	}
}

func TestSessionExpiredStopsPolling(t *testing.T) {
	src := newFakeSource()
	threeGateways(t, src)
	h := newHarness(t, src)
	src.listErrs[KindGateways] = fmt.Errorf("%w and token refresh failed", config.ErrSessionExpired)
	h.fireTick(KindGateways)
	assertContains(t, h.view(), SessionExpiredMessage)

	before := src.listCalls[KindGateways]
	h.key("r")
	h.dispatch(tickMsg{kind: KindGateways, gen: h.m.views[KindGateways].gen})
	if src.listCalls[KindGateways] != before {
		t.Error("polling continued after the session expired")
	}
}

func TestQuit(t *testing.T) {
	h := newHarness(t, newFakeSource())
	h.key("q")
	if !h.quit {
		t.Error("q did not quit")
	}
}

func TestBurstOfKeysIsReplayed(t *testing.T) {
	src := newFakeSource()
	threeGateways(t, src)
	h := newHarness(t, src)
	h.dispatch(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2?")})
	if h.m.active != KindClusters || h.m.mode != modeHelp {
		t.Errorf("active = %s mode = %d, want clusters with help open", h.m.active.Title(), h.m.mode)
	}
	h.key("esc", "1")
	h.dispatch(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/gw-b")})
	if got := h.m.views[KindGateways].filter; got != "gw-b" {
		t.Errorf("filter = %q, want gw-b", got)
	}
}

func TestSummaryCount(t *testing.T) {
	src := newFakeSource()
	src.lists[KindGateways] = ListResult{Items: []Resource{gateway(t, "g1", "only")}}
	h := newHarness(t, src)
	assertContains(t, h.view(), "1 item ")
}
