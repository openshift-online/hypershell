package tui

import (
	"os"
	"path/filepath"
	"testing"

	trextui "github.com/openshift-online/rh-trex-ai/components/api-server/pkg/tui"

	"github.com/openshift-online/hypershell/components/cli/data/generated/tui"
)

// The embedded descriptor must parse and drive a runtime model.
func TestEmbeddedDescriptorBuildsModel(t *testing.T) {
	data, err := tui.GetDescriptor()
	if err != nil {
		t.Fatal(err)
	}
	descriptor, err := trextui.ParseDescriptor(data)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := trextui.NewModel(descriptor, trextui.ClientConfig{BaseURL: "http://localhost:8000"}); err != nil {
		t.Fatal(err)
	}
	if len(descriptor.Views) == 0 {
		t.Fatal("descriptor has no views")
	}
}

func TestNotLoggedIn(t *testing.T) {
	t.Setenv("HYPERSHELL_CONFIG", filepath.Join(t.TempDir(), "missing.json"))
	_, err := session()
	if err == nil || err.Error() != "not logged in, server URL isn't set, run the 'login' command" {
		t.Errorf("err = %v", err)
	}
}

func TestRequiresInteractiveTerminal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"url":"https://api.example.com","access_token":"token"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HYPERSHELL_CONFIG", path)
	// Under `go test` stdout is a pipe, which is exactly the redirected case.
	_, err := session()
	if err == nil || err.Error() != "hsctl tui requires an interactive terminal" {
		t.Errorf("err = %v", err)
	}
}

// The command runs the session loader first and returns its error as is, so a
// missing login never reaches the descriptor, the model or the alternate screen.
func TestCommandReportsTheSessionErrorUnchanged(t *testing.T) {
	t.Setenv("HYPERSHELL_CONFIG", filepath.Join(t.TempDir(), "missing.json"))
	err := Cmd.RunE(Cmd, nil)
	if err == nil || err.Error() != "not logged in, server URL isn't set, run the 'login' command" {
		t.Errorf("err = %v", err)
	}
}
