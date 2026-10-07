package sandboxTemplate

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/openshift-online/hypershell/components/cli/pkg/config"
	"github.com/openshift-online/hypershell/components/cli/pkg/connection"
	"github.com/openshift-online/hypershell/components/cli/pkg/dump"
	"github.com/openshift-online/hypershell/components/cli/pkg/urls"
)

var Cmd = &cobra.Command{
	Use:     "sandboxTemplate ID",
	Aliases: []string{"sandboxTemplates"},
	Short:   "Get a sandbox template by ID",
	Long:    "Get a sandbox template by ID and display its details",
	Args:    cobra.ExactArgs(1),
	RunE:    run,
}

func run(cmd *cobra.Command, argv []string) error {
	id := argv[0]

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	conn, err := connection.NewConnection().Config(cfg).Build()
	if err != nil {
		return err
	}
	defer conn.Close()

	resp, err := conn.Get(urls.SandboxTemplatePath(id), nil)
	if err != nil {
		return fmt.Errorf("can't retrieve sandbox template: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("can't read response: %w", err)
	}

	if resp.StatusCode != 200 {
		return fmt.Errorf("API returned %d: %s", resp.StatusCode, string(body))
	}

	return dump.Pretty(os.Stdout, body)
}
