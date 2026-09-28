package ui

import (
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/golang-jwt/jwt/v4"
	"github.com/muesli/termenv"
	"github.com/spf13/cobra"

	"github.com/openshift-online/hypershell/components/cli/pkg/config"
	"github.com/openshift-online/hypershell/components/cli/pkg/connection"
	"github.com/openshift-online/hypershell/components/cli/pkg/output"
	"github.com/openshift-online/hypershell/components/cli/pkg/tui"
)

var args struct {
	refresh time.Duration
}

var Cmd = &cobra.Command{
	Use:     "ui",
	Aliases: []string{"tui"},
	Short:   "Open the interactive terminal interface",
	Long: "Open a full-screen terminal interface that shows gateways, managed clusters,\n" +
		"gateway releases, and gateway networks, refreshed in place. Gateways can be\n" +
		"provisioned and deleted from it. Press ? inside the interface for key bindings.",
	Args: cobra.NoArgs,
	RunE: run,
}

func init() {
	Cmd.Flags().DurationVar(&args.refresh, "refresh", tui.DefaultInterval,
		fmt.Sprintf("Polling interval for the active view (minimum %s).", tui.MinInterval))
}

func run(cmd *cobra.Command, argv []string) error {
	if args.refresh < tui.MinInterval {
		return fmt.Errorf("--refresh must be at least %s", tui.MinInterval)
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	// Fail before Build so redirected stdin/stdout never triggers a token refresh.
	if !output.IsTerminal(os.Stdin) || !output.IsTerminal(os.Stdout) {
		return fmt.Errorf("hsctl ui requires an interactive terminal")
	}

	conn, err := connection.NewConnection().Config(cfg).RefreshPerRequest(true).Build()
	if err != nil {
		return err
	}
	defer conn.Close()

	if os.Getenv("NO_COLOR") != "" {
		lipgloss.SetColorProfile(termenv.Ascii)
	}

	ctx := cmd.Context()
	model := tui.New(tui.Options{
		Source:    tui.NewRESTSource(conn),
		APIURL:    cfg.URL,
		Identity:  identity(cfg.AccessToken),
		Interval:  args.refresh,
		Context:   ctx,
		Clipboard: osc52(os.Stdout),
	})
	_, err = tea.NewProgram(model, tea.WithAltScreen(), tea.WithContext(ctx)).Run()
	if err != nil {
		return fmt.Errorf("terminal interface failed: %w", err)
	}
	return nil
}

// identity names the caller from the access token: the username, else the
// service-account client ID, else the subject. It never returns the token.
func identity(accessToken string) string {
	token, err := config.ParseToken(accessToken)
	if err != nil {
		return ""
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return ""
	}
	for _, key := range []string{"preferred_username", "client_id", "azp", "sub"} {
		if v, ok := claims[key].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

// osc52 sends text to the terminal clipboard with the OSC 52 escape sequence.
// Terminals that do not support it ignore the sequence.
func osc52(w io.Writer) func(string) error {
	return func(text string) error {
		_, err := fmt.Fprintf(w, "\x1b]52;c;%s\a", base64.StdEncoding.EncodeToString([]byte(text)))
		return err
	}
}
