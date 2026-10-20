// cmd_connection.go is hand-authored. The generator owns cmd.go (basic get-by-ID);
// this file extends Cmd with the --show-connection feature without touching cmd.go.
package gateway

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/openshift-online/hypershell/components/cli/pkg/config"
	"github.com/openshift-online/hypershell/components/cli/pkg/connection"
	"github.com/openshift-online/hypershell/components/cli/pkg/gatewayconnect"
	"github.com/openshift-online/hypershell/components/cli/pkg/urls"
)

var showConnection bool

func init() {
	Cmd.Flags().BoolVar(&showConnection, "show-connection", false, "Print openshell connection instructions for the gateway")
	base := Cmd.RunE
	Cmd.RunE = func(cmd *cobra.Command, argv []string) error {
		if !showConnection {
			return base(cmd, argv)
		}
		id := argv[0]

		cfg, err := config.Load()
		if err != nil {
			return err
		}

		conn, err := connection.NewConnectionBuilder().Config(cfg).Build()
		if err != nil {
			return err
		}
		defer conn.Close()

		resp, err := conn.Get(urls.GatewayPath(id), nil)
		if err != nil {
			return fmt.Errorf("can't retrieve gateway: %w", err)
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("can't read response: %w", err)
		}

		if resp.StatusCode != 200 {
			return fmt.Errorf("API returned %d: %s", resp.StatusCode, string(body))
		}

		return gatewayconnect.WriteInstructions(os.Stdout, body)
	}
}
