package login

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/openshift-online/hypershell/components/cli/pkg/config"
	"github.com/openshift-online/hypershell/components/cli/pkg/oidc"
)

var args struct {
	url       string
	token     string
	tokenFile string
	insecure  bool

	issuerURL string
	clientID  string
	scopes    []string
	noBrowser bool
}

var Cmd = &cobra.Command{
	Use:   "login",
	Short: "Log in to the API server",
	Long: "Log in, saving the credentials to the configuration file.\n\n" +
		"With --issuer-url the login uses OpenID Connect: the browser flow with PKCE by default,\n" +
		"or the device flow with --no-browser. The session is renewed automatically with the\n" +
		"saved refresh token. Without --issuer-url a bearer token you already have is saved.\n\n" +
		"Examples:\n" +
		"  hsctl login --token-file ~/.config/token --url http://localhost:8000\n" +
		"  echo \"$JWT_TOKEN\" | hsctl login --token-file /dev/stdin --url local\n" +
		"  hsctl login --issuer-url https://sso.example.com/realms/example --url https://api.example.com\n" +
		"  hsctl login --issuer-url https://sso.example.com/realms/example --no-browser",
	Args: cobra.NoArgs,
	RunE: run,
}

func init() {
	flags := Cmd.Flags()
	flags.StringVar(&args.url, "url", "http://localhost:8000", "URL of the API server.")
	flags.StringVar(&args.token, "token", "", "Bearer access token (JWT) - DEPRECATED: use --token-file instead.")
	flags.StringVar(&args.tokenFile, "token-file", "", "File containing bearer access token (use /dev/stdin to read from stdin).")
	flags.BoolVar(&args.insecure, "insecure", false, "Enables insecure communication with the server.")
	flags.StringVar(&args.issuerURL, "issuer-url", "", "OpenID Connect issuer URL; selects OIDC login instead of a saved token.")
	flags.StringVar(&args.clientID, "client-id", "hypershell-cli", "OpenID Connect public client ID (with --issuer-url).")
	flags.StringSliceVar(&args.scopes, "scope", []string{"openid"}, "OpenID Connect scopes to request (with --issuer-url).")
	flags.BoolVar(&args.noBrowser, "no-browser", false, "Use the device flow instead of opening a browser (with --issuer-url).")
}

func run(cmd *cobra.Command, argv []string) error {
	if args.issuerURL != "" {
		if args.token != "" || args.tokenFile != "" {
			return fmt.Errorf("--issuer-url can't be combined with --token or --token-file")
		}
		return runOIDC(cmd)
	}
	return runToken()
}

func runToken() error {
	var token string

	// Handle token input (prefer --token-file over --token for security)
	if args.tokenFile != "" {
		var reader io.Reader
		if args.tokenFile == "/dev/stdin" {
			reader = os.Stdin
		} else {
			file, err := os.Open(args.tokenFile)
			if err != nil {
				return fmt.Errorf("can't open token file '%s': %v", args.tokenFile, err)
			}
			defer file.Close()
			reader = file
		}

		tokenBytes, err := io.ReadAll(reader)
		if err != nil {
			return fmt.Errorf("can't read token: %v", err)
		}
		token = strings.TrimSpace(string(tokenBytes))
	} else if args.token != "" {
		fmt.Fprintf(os.Stderr, "Warning: Using --token flag exposes token in shell history. Use --token-file instead.\n")
		token = args.token
	} else {
		return fmt.Errorf("a token is required, use '--token-file <path>' or '--token-file /dev/stdin', or '--issuer-url' for OpenID Connect")
	}

	if token == "" {
		return fmt.Errorf("token cannot be empty")
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("can't load config file: %v", err)
	}
	if cfg == nil {
		cfg = new(config.Config)
	}

	cfg.AccessToken = token
	cfg.URL = args.url
	cfg.Insecure = args.insecure
	// A saved token has no refresh capability: drop any earlier OIDC session.
	cfg.ClearOIDC()

	err = config.Save(cfg)
	if err != nil {
		return fmt.Errorf("can't save config file: %v", err)
	}

	fmt.Fprintf(os.Stderr, "Login successful. Configuration saved.\n")
	return nil
}

func runOIDC(cmd *cobra.Command) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	client := oidc.NewClient(args.issuerURL, args.clientID, args.insecure)
	var tokens *oidc.Tokens
	var err error
	if args.noBrowser {
		tokens, err = client.LoginDevice(ctx, args.scopes, os.Stderr)
	} else {
		tokens, err = client.LoginBrowser(ctx, oidc.BrowserOptions{Scopes: args.scopes, Out: os.Stderr, Timeout: 5 * time.Minute})
	}
	if err != nil {
		return fmt.Errorf("login failed: %w", err)
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("can't load config file: %v", err)
	}
	if cfg == nil {
		cfg = new(config.Config)
	}
	cfg.AccessToken = tokens.AccessToken
	cfg.RefreshToken = tokens.RefreshToken
	cfg.IssuerURL = client.Issuer
	cfg.ClientID = args.clientID
	cfg.ExpiresAt = tokens.ExpiresAt(time.Now())
	cfg.URL = args.url
	cfg.Insecure = args.insecure
	if tokens.RefreshToken == "" {
		fmt.Fprintf(os.Stderr, "Warning: the issuer returned no refresh token, so the session can't be renewed automatically.\n")
	}

	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("can't save config file: %v", err)
	}
	fmt.Fprintf(os.Stderr, "Login successful. Configuration saved.\n")
	return nil
}
