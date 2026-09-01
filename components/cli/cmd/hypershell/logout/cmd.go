package logout

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/openshift-online/hypershell/components/cli/pkg/auth"
	"github.com/openshift-online/hypershell/components/cli/pkg/config"
)

var Cmd = &cobra.Command{
	Use:   "logout",
	Short: "Log out",
	Long:  "Log out, revoking the token at the identity provider and removing credentials from the config file.",
	Args:  cobra.NoArgs,
	RunE:  run,
}

func run(cmd *cobra.Command, argv []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("can't load configuration file: %w", err)
	}

	// Revoke the refresh token at Keycloak (best-effort; continue even if it fails)
	if cfg.RefreshToken != "" && cfg.IssuerURL != "" && cfg.ClientID != "" {
		if revokeErr := auth.Revoke(cfg.IssuerURL, cfg.ClientID, cfg.RefreshToken, cfg.Insecure); revokeErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not revoke token: %v\n", revokeErr)
		}
	}

	cfg.Disarm()

	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("can't save configuration file: %w", err)
	}

	return nil
}
