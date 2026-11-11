package secretSource

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// resetUploadArgs clears the package-global flag state between table cases.
func resetUploadArgs() {
	uploadArgs.agentRuntimeID = ""
	uploadArgs.valueFile = ""
	uploadArgs.valueStdin = false
}

// Material never comes from an argv flag; exactly one of --value-file/--value-stdin
// is required and the two are mutually exclusive.
// Verifies: CLI-EXT-05.03
func TestUpload_FlagGroupValidation(t *testing.T) {
	// The command must expose no flag that accepts the material value directly.
	if f := uploadCmd.Flags().Lookup("value"); f != nil {
		t.Fatalf("upload must not expose a --value flag that takes the material on argv")
	}

	cases := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{"neither source", []string{"upload", "my-secret"}, "at least one"},
		{"both sources", []string{"upload", "my-secret", "--value-file", "x", "--value-stdin"}, "none of the others"},
		{"missing name", []string{"upload", "--value-stdin"}, "arg"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			resetUploadArgs()
			parent := &cobra.Command{Use: "secret-source"}
			fresh := *uploadCmd
			parent.AddCommand(&fresh)
			parent.SetArgs(c.args)
			parent.SetOut(&bytes.Buffer{})
			parent.SetErr(&bytes.Buffer{})
			// Feed empty stdin so a (wrongly) accepted invocation cannot block.
			fresh.SetIn(strings.NewReader(""))
			err := parent.Execute()
			if err == nil {
				t.Fatalf("expected an error for args %v", c.args)
			}
			if !strings.Contains(err.Error(), c.wantErr) {
				t.Fatalf("error = %q, want it to contain %q", err.Error(), c.wantErr)
			}
		})
	}
}

func TestReadMaterial_FromFile(t *testing.T) {
	resetUploadArgs()
	dir := t.TempDir()
	file := filepath.Join(dir, "key.pem")
	if err := os.WriteFile(file, []byte("topsecret-material"), 0o600); err != nil {
		t.Fatal(err)
	}
	uploadArgs.valueFile = file

	got, err := readMaterial(&cobra.Command{})
	if err != nil {
		t.Fatalf("readMaterial: %v", err)
	}
	if got != "topsecret-material" {
		t.Fatalf("material = %q, want topsecret-material", got)
	}
}

func TestReadMaterial_FromStdin(t *testing.T) {
	resetUploadArgs()
	uploadArgs.valueStdin = true
	cmd := &cobra.Command{}
	cmd.SetIn(strings.NewReader("piped-material"))

	got, err := readMaterial(cmd)
	if err != nil {
		t.Fatalf("readMaterial: %v", err)
	}
	if got != "piped-material" {
		t.Fatalf("material = %q, want piped-material", got)
	}
}

func TestReadMaterial_EmptyFileRejected(t *testing.T) {
	resetUploadArgs()
	dir := t.TempDir()
	file := filepath.Join(dir, "empty.pem")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	uploadArgs.valueFile = file

	if _, err := readMaterial(&cobra.Command{}); err == nil {
		t.Fatal("expected an error for an empty material file")
	}
}
