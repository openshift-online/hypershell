package gateway

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/openshift-online/hypershell/components/cli/pkg/config"
	"github.com/openshift-online/hypershell/components/cli/pkg/confirm"
	"github.com/openshift-online/hypershell/components/cli/pkg/connection"
	"github.com/openshift-online/hypershell/components/cli/pkg/urls"
)

// ConfirmPrompt returns the confirmation question for one resource. A project
// can replace it, for example from its hand-maintained main.go, to customize the
// wording without replacing the command.
var ConfirmPrompt = func(id string) string {
	return "Delete gateway " + id + "?"
}

var args struct {
	yes bool
}

var Cmd = &cobra.Command{
	Use:     "gateway ID [flags]",
	Aliases: []string{"gateways"},
	Short:   "Delete a gateway by ID",
	Long: "Delete a gateway by ID after asking for confirmation.\n\n" +
		"Without a terminal on standard input the command refuses to proceed unless --yes is given.",
	Args: cobra.ExactArgs(1),
	RunE: run,
}

func init() {
	Cmd.Flags().BoolVar(&args.yes, "yes", false, "Skip the confirmation prompt.")
}

func run(cmd *cobra.Command, argv []string) error {
	id := argv[0]

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	if err := confirm.Ask(ConfirmPrompt(id), args.yes); err != nil {
		return err
	}

	conn, err := connection.NewConnectionBuilder().Config(cfg).Build()
	if err != nil {
		return err
	}
	defer conn.Close()

	resp, err := conn.Delete(urls.GatewayPath(id))
	if err != nil {
		return fmt.Errorf("can't delete gateway: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("can't read response: %v", err)
	}

	switch resp.StatusCode {
	case 200, 202, 204:
	default:
		return fmt.Errorf("API returned %d: %s", resp.StatusCode, string(body))
	}

	fmt.Fprintf(os.Stdout, "Gateway %s deleted.\n", id)
	return nil
}
