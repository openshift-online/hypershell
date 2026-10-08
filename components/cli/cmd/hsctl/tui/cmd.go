// Package tui is hsctl tui, the terminal interface generated from the API's
// OpenAPI description (specs/platform/hsctl-terminal-ui.spec.md). The command,
// its flags and the screens come from the rh-trex-ai runtime; this package
// only supplies the embedded descriptor and the saved login.
package tui

import (
	"fmt"
	"os"

	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/tuicmd"

	"github.com/openshift-online/hypershell/components/cli/data/generated/tui"
	"github.com/openshift-online/hypershell/components/cli/pkg/config"
	"github.com/openshift-online/hypershell/components/cli/pkg/connection"
	"github.com/openshift-online/hypershell/components/cli/pkg/output"
)

// Cmd is hsctl tui.
var Cmd = tuicmd.NewTUICommand(tui.GetDescriptor, tuicmd.WithSession(session))

// session loads the saved login. It runs before the terminal is taken over, so
// every failure here is an ordinary command error.
func session() (tuicmd.Session, error) {
	cfg, err := config.Load()
	if err != nil {
		return tuicmd.Session{}, err
	}
	if cfg == nil {
		return tuicmd.Session{}, fmt.Errorf("not logged in, run the 'login' command")
	}
	if armed, reason := cfg.Armed(); !armed {
		return tuicmd.Session{}, fmt.Errorf("not logged in, %s, run the 'login' command", reason)
	}
	if !output.IsTerminal(os.Stdin) || !output.IsTerminal(os.Stdout) {
		return tuicmd.Session{}, fmt.Errorf("hsctl tui requires an interactive terminal")
	}

	// Renew the session now so an expired login is reported here. The provider
	// keeps renewing it during the session.
	tokens := connection.NewConfigTokenProvider(cfg)
	if _, err := tokens.GetToken(); err != nil {
		return tuicmd.Session{}, err
	}
	return tuicmd.Session{Server: cfg.URL, Insecure: cfg.Insecure, TokenProvider: tokens}, nil
}
