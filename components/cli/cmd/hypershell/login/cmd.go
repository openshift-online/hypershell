package login

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/openshift-online/hypershell/components/cli/pkg/auth"
	"github.com/openshift-online/hypershell/components/cli/pkg/config"
)

const defaultClientID = "hypershell-cli"

var args struct {
	url       string
	issuerURL string
	clientID  string
	token     string
	tokenFile string
	noBrowser bool
	insecure  bool
}

var Cmd = &cobra.Command{
	Use:   "login",
	Short: "Log in to the API server",
	Long: "Log in using OIDC (browser or device flow), saving credentials to the config file.\n\n" +
		"Examples:\n" +
		"  hsctl login --url https://api.hypershell.localhost \\\n" +
		"    --issuer-url https://keycloak.hypershell.localhost/realms/hypershell --insecure\n\n" +
		"  # Device flow for headless/SSH environments\n" +
		"  hsctl login --no-browser --url https://api.hypershell.localhost \\\n" +
		"    --issuer-url https://keycloak.hypershell.localhost/realms/hypershell --insecure\n\n" +
		"  # Static token (service accounts / automation)\n" +
		"  hsctl login --token-file ~/.config/token --url https://api.hypershell.localhost --insecure",
	Args: cobra.NoArgs,
	RunE: run,
}

func init() {
	flags := Cmd.Flags()
	flags.StringVar(&args.url, "url", "", "URL of the API server.")
	flags.StringVar(&args.issuerURL, "issuer-url", "", "OIDC issuer URL (Keycloak realm).")
	flags.StringVar(&args.clientID, "client-id", defaultClientID, "OIDC client ID.")
	flags.BoolVar(&args.noBrowser, "no-browser", false, "Use device authorization flow instead of opening a browser.")
	flags.StringVar(&args.token, "token", "", "Bearer access token (JWT) -- use --token-file instead.")
	flags.StringVar(&args.tokenFile, "token-file", "", "File containing bearer access token (use /dev/stdin to read from stdin).")
	flags.BoolVar(&args.insecure, "insecure", false, "Disable TLS verification.")
}

func run(cmd *cobra.Command, argv []string) error {
	if args.url == "" {
		_ = cmd.Usage()
		return fmt.Errorf("required flag \"url\" not set")
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("can't load config: %w", err)
	}
	if cfg == nil {
		cfg = new(config.Config)
	}

	cfg.URL = args.url
	cfg.Insecure = args.insecure

	// Static token path (service accounts / scripts)
	if args.tokenFile != "" || args.token != "" {
		return staticTokenLogin(cfg)
	}

	if args.issuerURL == "" {
		_ = cmd.Usage()
		return fmt.Errorf("required flag \"issuer-url\" not set")
	}

	// OIDC path
	cfg.IssuerURL = args.issuerURL
	cfg.ClientID = args.clientID

	var tr auth.TokenResponse
	if args.noBrowser {
		tr, err = auth.DeviceFlow(args.issuerURL, args.clientID, args.insecure)
	} else {
		tr, err = auth.BrowserPKCE(args.issuerURL, args.clientID, args.insecure)
	}
	if err != nil {
		return err
	}

	cfg.AccessToken = tr.AccessToken
	cfg.RefreshToken = tr.RefreshToken

	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("can't save config: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Login successful.\n")
	return nil
}

func staticTokenLogin(cfg *config.Config) error {
	var token string

	if args.tokenFile != "" {
		var reader io.Reader
		if args.tokenFile == "/dev/stdin" {
			reader = os.Stdin
		} else {
			file, err := os.Open(args.tokenFile)
			if err != nil {
				return fmt.Errorf("can't open token file '%s': %w", args.tokenFile, err)
			}
			defer file.Close()
			reader = file
		}
		tokenBytes, err := io.ReadAll(reader)
		if err != nil {
			return fmt.Errorf("can't read token: %w", err)
		}
		token = strings.TrimSpace(string(tokenBytes))
	} else {
		fmt.Fprintf(os.Stderr, "Warning: --token exposes the token in shell history. Use --token-file instead.\n")
		token = args.token
	}

	if token == "" {
		return fmt.Errorf("token is empty")
	}

	cfg.AccessToken = token
	cfg.RefreshToken = ""
	cfg.IssuerURL = ""
	cfg.ClientID = ""

	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("can't save config: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Login successful.\n")
	return nil
}
