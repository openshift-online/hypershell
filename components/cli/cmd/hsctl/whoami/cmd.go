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
	Short: "Show the logged-in identity",
	Long: "Show who the saved credentials belong to, from the claims of the saved access token.\n\n" +
		"The token is decoded without verifying its signature. The claims are for display only and\n" +
		"are never an authorization decision: the API server verifies the token on every request.\n" +
		"A token that is not a JWT is reported as opaque.",
	Args: cobra.NoArgs,
	RunE: run,
}

func init() {
	flags := Cmd.Flags()
	flags.BoolVar(&args.showToken, "show-token", false, "Also print the raw access token.")
	flags.BoolVar(&args.showTokenDecoded, "show-token-decoded", false, "Also print the decoded token claims as JSON.")
}

func run(cmd *cobra.Command, argv []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("can't load configuration file: %w", err)
	}
	if armed, reason := cfg.Armed(); !armed {
		return fmt.Errorf("not logged in, %s, run the 'login' command", reason)
	}

	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "API URL: %s\n", cfg.URL)

	parsed, err := config.ParseToken(cfg.AccessToken)
	claims, ok := jwt.MapClaims(nil), false
	if err == nil {
		claims, ok = parsed.Claims.(jwt.MapClaims)
	}
	if !ok {
		fmt.Fprintln(out, "Token: opaque (not a JWT), identity claims are not available")
		return printToken(cmd, cfg, nil)
	}

	user := stringClaim(claims, "preferred_username")
	if user == "" {
		user = stringClaim(claims, "sub")
	}
	printClaim(out, "User", user)
	printClaim(out, "Email", stringClaim(claims, "email"))
	printClaim(out, "Issuer", stringClaim(claims, "iss"))
	if exp, ok := claims["exp"].(float64); ok && exp > 0 {
		expiry := time.Unix(int64(exp), 0)
		state := "expires in " + time.Until(expiry).Round(time.Second).String()
		if time.Until(expiry) <= 0 {
			state = "expired"
		}
		fmt.Fprintf(out, "Expires: %s (%s)\n", expiry.Format(time.RFC3339), state)
	}
	if cfg.CanRefresh() {
		fmt.Fprintln(out, "Session: renewable with a saved refresh token")
	}
	return printToken(cmd, cfg, claims)
}

func printToken(cmd *cobra.Command, cfg *config.Config, claims jwt.MapClaims) error {
	out := cmd.OutOrStdout()
	if args.showToken {
		fmt.Fprintf(out, "Access token: %s\n", cfg.AccessToken)
	}
	if args.showTokenDecoded {
		if claims == nil {
			fmt.Fprintln(os.Stderr, "The access token is opaque, there are no claims to decode.")
			return nil
		}
		encoded, err := json.MarshalIndent(claims, "", "  ")
		if err != nil {
			return fmt.Errorf("can't encode the token claims: %w", err)
		}
		fmt.Fprintf(out, "Claims:\n%s\n", encoded)
	}
	return nil
}

func stringClaim(claims jwt.MapClaims, name string) string {
	value, _ := claims[name].(string)
	return value
}

func printClaim(out interface{ Write([]byte) (int, error) }, label, value string) {
	if value != "" {
		fmt.Fprintf(out, "%s: %s\n", label, value)
	}
}
