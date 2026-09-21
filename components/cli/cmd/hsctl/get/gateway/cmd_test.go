package gateway

import (
	"bytes"
	"strings"
	"testing"
)

func TestShellArg(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"simple", "simple"},
		{"with space", "'with space'"},
		{"it's", `'it'"'"'s'`},
		{"https://example.com/path", "https://example.com/path"},
		{"<PENDING>", "<PENDING>"},
		{"foo bar'baz", `'foo bar'"'"'baz'`},
		{`semi;colon`, `'semi;colon'`},
	}
	for _, tc := range cases {
		if got := shellArg(tc.input); got != tc.want {
			t.Errorf("shellArg(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestPrintConnectionInstructions_InvalidJSON(t *testing.T) {
	err := printConnectionInstructions(&bytes.Buffer{}, []byte("not-json"))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestBuildConnectionScript_IncludesDriverConfig(t *testing.T) {
	out := buildConnectionScript("gw", "https://gw.test:443", oidcConfig{
		Issuer:   "https://issuer.test",
		ClientID: "cli",
		Audience: "aud",
	})
	if !strings.Contains(out, "DRIVER_CONFIG='"+sandboxDriverConfig+"'") {
		t.Errorf("expected DRIVER_CONFIG variable with resource defaults, got:\n%s", out)
	}
	if !strings.Contains(out, `--driver-config-json "$DRIVER_CONFIG"`) {
		t.Errorf("expected --driver-config-json flag referencing $DRIVER_CONFIG, got:\n%s", out)
	}
	for _, want := range []string{`"cpu":"100m"`, `"cpu":"500m"`, `"memory":"512Mi"`} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in driver config, got:\n%s", want, out)
		}
	}
}

func TestPrintConnectionInstructions_PendingWhenEmpty(t *testing.T) {
	body := []byte(`{"name":"mygw","phase":"ready"}`)
	var buf bytes.Buffer
	if err := printConnectionInstructions(&buf, body); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "<PENDING>") {
		t.Errorf("expected <PENDING> for missing endpoint/oidc, got:\n%s", out)
	}
	if !strings.Contains(out, "mygw") {
		t.Errorf("expected gateway name in output, got:\n%s", out)
	}
}
