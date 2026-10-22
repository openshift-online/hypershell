package gatewayconnect

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
	err := WriteInstructions(&bytes.Buffer{}, []byte("not-json"))
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

func TestBuildConnectionScript_ProfileImportPrecedesProviderCreate(t *testing.T) {
	out := buildConnectionScript("gw", "https://gw.test:443", oidcConfig{
		Issuer:   "https://issuer.test",
		ClientID: "cli",
		Audience: "aud",
	})

	const importCmd = "openshell provider profile import -f google-vertex-ai.yaml --global"
	if !strings.Contains(out, importCmd) {
		t.Errorf("expected profile import command %q, got:\n%s", importCmd, out)
	}

	importAt := strings.Index(out, "openshell provider profile import")
	createAt := strings.Index(out, "openshell provider create")
	if importAt < 0 {
		t.Fatalf("expected profile import step in output, got:\n%s", out)
	}
	if createAt < 0 {
		t.Fatalf("expected provider create step in output, got:\n%s", out)
	}
	if importAt >= createAt {
		t.Errorf("expected profile import (%d) to precede provider create (%d), got:\n%s", importAt, createAt, out)
	}
}

func TestBuildConnectionScript_ProfileAndProviderScopesMatch(t *testing.T) {
	out := buildConnectionScript("gw", "https://gw.test:443", oidcConfig{
		Issuer:   "https://issuer.test",
		ClientID: "cli",
		Audience: "aud",
	})

	if !strings.Contains(out, "import -f google-vertex-ai.yaml --global") {
		t.Errorf("expected --global scope on the profile import command, got:\n%s", out)
	}
	if !strings.Contains(out, "--global-profile") {
		t.Errorf("expected --global-profile on the provider create command to match the import scope, got:\n%s", out)
	}
	if !strings.Contains(out, "must match the scope used") {
		t.Errorf("expected a comment noting the import and create scopes must match, got:\n%s", out)
	}
}

func TestPrintConnectionInstructions_PendingWhenEmpty(t *testing.T) {
	body := []byte(`{"name":"mygw","phase":"ready"}`)
	var buf bytes.Buffer
	if err := WriteInstructions(&buf, body); err != nil {
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
