package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

func TestColorizeYAMLPreservesText(t *testing.T) {
	plain := strings.Join([]string{
		"name: demo",
		"phase: Running",
		"count: 3",
		"ready: true",
		"extra: null",
		"server_dns_names:",
		"  - a.example.com",
		"labels:",
		`  z: "1"`,
	}, "\n")
	got := colorizeYAML(plain)
	if stripped := ansi.Strip(got); stripped != plain {
		t.Errorf("stripped =\n%s\nwant\n%s", stripped, plain)
	}
}

func TestColorizeYAMLAppliesDistinctStyles(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	got := colorizeYAML(strings.Join([]string{
		"name: demo",
		"phase: Running",
		"count: 3",
		"ready: true",
		"extra: null",
		"phase: Upgrading",
	}, "\n"))
	if !strings.Contains(got, "\x1b[") {
		t.Fatal("expected ANSI color codes in colored YAML")
	}

	keyDemo := yamlKeyStyle.Render("name") + ": " + yamlStringStyle.Render("demo")
	phaseRunning := yamlKeyStyle.Render("phase") + ": " + okStyle.Render("Running")
	count := yamlKeyStyle.Render("count") + ": " + yamlNumberStyle.Render("3")
	ready := yamlKeyStyle.Render("ready") + ": " + yamlBoolStyle.Render("true")
	extra := yamlKeyStyle.Render("extra") + ": " + yamlNullStyle.Render("null")
	phaseOther := yamlKeyStyle.Render("phase") + ": " + yamlStringStyle.Render("Upgrading")

	for _, want := range []string{keyDemo, phaseRunning, count, ready, extra, phaseOther} {
		if !strings.Contains(got, want) {
			t.Errorf("colored YAML missing styled segment %q\ngot:\n%s", want, got)
		}
	}
}

func TestColorizeYAMLListScalar(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	got := colorizeYAML("  - a.example.com")
	want := "  - " + yamlStringStyle.Render("a.example.com")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestDetailContentColorsYAML(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	r := gateway(t, "g1", "demo", "phase", "Running", "active_sandbox_count", 2)
	got := detailContent(KindGateways, r)
	if ansi.Strip(got) == got {
		t.Fatal("detail YAML was not colored")
	}
	if !strings.Contains(got, yamlKeyStyle.Render("name")) {
		t.Errorf("missing styled key, got:\n%s", got)
	}
	if !strings.Contains(got, okStyle.Render("Running")) {
		t.Errorf("phase Running missing healthy style, got:\n%s", got)
	}
}
