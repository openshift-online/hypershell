package logout

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/openshift-online/hypershell/components/cli/pkg/config"
	"github.com/openshift-online/hypershell/components/cli/pkg/oidc"
)

var Cmd = &cobra.Command{
	Use:   "logout",
	Short: "Log out",
	Long: "Log out, removing credentials from the config file. When the login used OpenID Connect,\n" +
		"the refresh token is first revoked at the issuer; a revocation failure only prints a warning.",
	Args: cobra.NoArgs,
	RunE: run,
}

func run(cmd *cobra.Command, argv []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("can't load configuration file: %w", err)
	}

	if cfg.CanRefresh() {
		revoke(cfg)
	}

	cfg.Disarm()

	err = config.Save(cfg)
	if err != nil {
		return fmt.Errorf("can't save configuration file: %w", err)
	}

	return nil
}

// revoke is best effort: the local credentials are cleared either way.
func revoke(cfg *config.Config) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	client := oidc.NewClient(cfg.IssuerURL, cfg.ClientID, cfg.Insecure)
	err := client.Revoke(ctx, cfg.RefreshToken)
	switch {
	case err == nil, errors.Is(err, oidc.ErrNoRevocationEndpoint):
	default:
		fmt.Fprintf(os.Stderr, "Warning: couldn't revoke the refresh token at the issuer: %v\n", err)
	}
}
