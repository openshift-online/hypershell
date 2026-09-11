package whoami

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/spf13/cobra"

	"github.com/openshift-online/hypershell/components/cli/pkg/config"
)

var args struct {
	showToken        bool
	showTokenDecoded bool
}

var Cmd = &cobra.Command{
	Use:   "whoami",
	Short: "Show current login information",
	Long:  "Display the user identity, token expiry, and API server from the saved configuration.",
	Args:  cobra.NoArgs,
	RunE:  run,
}

func init() {
	Cmd.Flags().BoolVarP(&args.showToken, "show-token", "t", false, "Print only the raw access token.")
	Cmd.Flags().BoolVar(&args.showTokenDecoded, "show-token-decoded", false, "Print only the decoded token claims as JSON.")
}

func run(cmd *cobra.Command, argv []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("can't load config: %w", err)
	}

	armed, reason := cfg.Armed()
	if !armed {
		return fmt.Errorf("not logged in: %s", reason)
	}

	if err := config.EnsureFreshToken(cfg); err != nil {
		return err
	}

	if args.showToken {
		fmt.Fprintln(os.Stdout, cfg.AccessToken)
		return nil
	}

	token, err := config.ParseToken(cfg.AccessToken)
	if err != nil {
		return fmt.Errorf("can't parse token: %w", err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return fmt.Errorf("unexpected token claims type")
	}

	if args.showTokenDecoded {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(map[string]any(claims))
	}

	username := stringClaim(claims, "preferred_username")
	if username == "" {
		username = stringClaim(claims, "sub")
	}
	email := stringClaim(claims, "email")
	issuer := stringClaim(claims, "iss")

	fmt.Fprintf(os.Stdout, "User:    %s\n", username)
	if email != "" {
		fmt.Fprintf(os.Stdout, "Email:   %s\n", email)
	}
	fmt.Fprintf(os.Stdout, "Issuer:  %s\n", issuer)
	fmt.Fprintf(os.Stdout, "API URL: %s\n", cfg.URL)

	if exp, ok := claims["exp"].(float64); ok && exp > 0 {
		expTime := time.Unix(int64(exp), 0)
		remaining := time.Until(expTime)
		if remaining > 0 {
			fmt.Fprintf(os.Stdout, "Expires: %s (in %s)\n", expTime.Local().Format(time.RFC3339), remaining.Truncate(time.Second))
		} else {
			fmt.Fprintf(os.Stdout, "Expires: %s (expired)\n", expTime.Local().Format(time.RFC3339))
		}
	}

	return nil
}

func stringClaim(claims jwt.MapClaims, key string) string {
	v, _ := claims[key].(string)
	return v
}
