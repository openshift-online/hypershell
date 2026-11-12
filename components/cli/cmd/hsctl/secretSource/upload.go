package secretSource

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/openshift-online/hypershell/components/cli/pkg/config"
	"github.com/openshift-online/hypershell/components/cli/pkg/connection"
	"github.com/openshift-online/hypershell/components/cli/pkg/urls"
)

var uploadArgs struct {
	agentRuntimeID string
	valueFile      string
	valueStdin     bool
}

var uploadCmd = &cobra.Command{
	Use:   "upload <name> (--value-file <path> | --value-stdin) [--agent-runtime <id>]",
	Short: "Upload the durable material for an existing secret source",
	Long: "Upload the durable material for an already-created, reference-only secret source\n" +
		"through the api-server write-only material-upload operation. The material is read\n" +
		"ONLY from a file or from standard input - never from a command-line flag - so it\n" +
		"never appears in shell history or the process table. The upload is idempotent:\n" +
		"re-running it overwrites the same material at the server-derived convention path.\n\n" +
		"Examples:\n" +
		"  hsctl secret-source upload my-secret --value-file ./key.pem\n" +
		"  cat ./key.pem | hsctl secret-source upload my-secret --value-stdin\n" +
		"  hsctl secret-source upload my-secret --agent-runtime <id> --value-file ./key.pem",
	Args: cobra.ExactArgs(1),
	RunE: runUpload,
}

func init() {
	fs := uploadCmd.Flags()
	fs.StringVar(&uploadArgs.agentRuntimeID, "agent-runtime", "", "Agent runtime ID that owns the secret source (disambiguates a name shared across runtimes).")
	fs.StringVar(&uploadArgs.valueFile, "value-file", "", "Path to a file containing the material to upload.")
	fs.BoolVar(&uploadArgs.valueStdin, "value-stdin", false, "Read the material to upload from standard input.")
	// Enforce exactly one material source: the material is never accepted as a
	// value flag, so it cannot leak into shell history or the process table.
	uploadCmd.MarkFlagsMutuallyExclusive("value-file", "value-stdin")
	uploadCmd.MarkFlagsOneRequired("value-file", "value-stdin")
}

func runUpload(cmd *cobra.Command, argv []string) error {
	name := argv[0]

	material, err := readMaterial(cmd)
	if err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	conn, err := connection.NewConnection().Config(cfg).Build()
	if err != nil {
		return err
	}
	defer conn.Close()

	id, err := resolveSecretSourceID(conn, name, uploadArgs.agentRuntimeID)
	if err != nil {
		return err
	}

	// The body carries ONLY the material; the path is derived server-side.
	body, err := json.Marshal(map[string]string{"value": material})
	if err != nil {
		return fmt.Errorf("can't marshal request: %w", err)
	}

	resp, err := conn.Post(urls.SecretSourceMaterialPath(id), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("can't upload secret source material: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("can't read response: %w", err)
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("API returned %d: %s", resp.StatusCode, string(respBody))
	}

	// Do not print the material. Confirm the upload and identify the source.
	fmt.Fprintf(cmd.OutOrStdout(), "Uploaded material for secret source %q (id %s).\n", name, id)
	return nil
}

// readMaterial reads the material from exactly one of the file or stdin. The flag
// framework guarantees exactly one is set; this reads whichever it is.
func readMaterial(cmd *cobra.Command) (string, error) {
	if uploadArgs.valueStdin {
		data, err := io.ReadAll(cmd.InOrStdin())
		if err != nil {
			return "", fmt.Errorf("can't read material from stdin: %w", err)
		}
		if len(data) == 0 {
			return "", fmt.Errorf("no material read from stdin")
		}
		return string(data), nil
	}
	data, err := os.ReadFile(uploadArgs.valueFile)
	if err != nil {
		return "", fmt.Errorf("can't read material file: %w", err)
	}
	if len(data) == 0 {
		return "", fmt.Errorf("material file %q is empty", uploadArgs.valueFile)
	}
	return string(data), nil
}

// resolveSecretSourceID resolves a SecretSource name to its id, optionally
// disambiguated by the owning agent runtime when a name is shared across runtimes.
func resolveSecretSourceID(conn *connection.Connection, name, agentRuntimeID string) (string, error) {
	// TSL requires values with hyphens or other special characters to be
	// single-quoted; double an embedded single quote.
	quotedName := "'" + strings.ReplaceAll(name, "'", "''") + "'"
	listResp, err := conn.List(urls.SecretSourcesPath, 1, 100, "name = "+quotedName, "")
	if err != nil {
		return "", fmt.Errorf("can't look up secret source %q: %w", name, err)
	}

	var matches []struct {
		id      string
		runtime string
	}
	for _, item := range listResp.Items {
		var resource struct {
			ID             string `json:"id"`
			Name           string `json:"name"`
			AgentRuntimeID string `json:"agent_runtime_id"`
		}
		if err := json.Unmarshal(item, &resource); err != nil {
			continue
		}
		if resource.Name != name {
			continue
		}
		if agentRuntimeID != "" && resource.AgentRuntimeID != agentRuntimeID {
			continue
		}
		matches = append(matches, struct {
			id      string
			runtime string
		}{id: resource.ID, runtime: resource.AgentRuntimeID})
	}

	switch len(matches) {
	case 0:
		if agentRuntimeID != "" {
			return "", fmt.Errorf("no secret source named %q owned by agent runtime %q", name, agentRuntimeID)
		}
		return "", fmt.Errorf("no secret source named %q", name)
	case 1:
		return matches[0].id, nil
	default:
		return "", fmt.Errorf("secret source name %q is owned by more than one agent runtime; disambiguate with --agent-runtime <id>", name)
	}
}
