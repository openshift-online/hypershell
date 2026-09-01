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
