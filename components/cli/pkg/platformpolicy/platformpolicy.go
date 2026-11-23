// Package platformpolicy holds what the hand-written PlatformGatewayPolicy
// commands share (CLI-EXT-06): the request helper that surfaces a 403 as a
// denied operation, and the flags that build a policy create or update body.
package platformpolicy

import (
	"fmt"
	"io"
	"net/url"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/openshift-online/hypershell/components/cli/pkg/apierror"
	"github.com/openshift-online/hypershell/components/cli/pkg/connection"
)

// WriteRoles names the roles that authorize policy writes and toggle changes
// (SEC-RBAC-30).
const WriteRoles = "platform-policy:admin or platform:admin"

// Request performs one API call and returns the response body. A status not in
// accepted becomes an error; a 403 becomes an apierror.DeniedError naming
// operation, so an unauthorized write is reported as denied, not retryable.
func Request(conn *connection.Connection, method, path string, query url.Values, body io.Reader, operation, requires string, accepted ...int) ([]byte, error) {
	resp, err := conn.Do(method, path, query, body)
	if err != nil {
		return nil, fmt.Errorf("can't %s: %w", operation, err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, fmt.Errorf("can't read API response: %w", err)
	}
	if err := apierror.Check(operation, requires, resp.StatusCode, respBody, accepted...); err != nil {
		return nil, err
	}
	return respBody, nil
}

// Fields are the policy flags shared by create and update.
type Fields struct {
	Description      string
	Cedar            string
	CedarFile        string
	UserMessage      string
	DocumentationURL string
	All              bool
	GatewayIDs       []string
	Exclude          []string
}

// AddFlags registers the shared policy flags on fs.
func (f *Fields) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&f.Description, "description", "", "Policy description.")
	fs.StringVar(&f.Cedar, "cedar", "", "Cedar forbid statements, inline.")
	fs.StringVar(&f.CedarFile, "cedar-file", "", "File containing the Cedar forbid statements ('-' reads standard input).")
	fs.StringVar(&f.UserMessage, "user-message", "", "Text shown to users in the denial reason. Must not contain secrets.")
	fs.StringVar(&f.DocumentationURL, "documentation-url", "", "Documentation link shown with the denial reason.")
	fs.BoolVar(&f.All, "all", false, "Target every gateway (minus --exclude).")
	fs.StringSliceVar(&f.GatewayIDs, "gateway-id", nil, "Target only this gateway ID (repeatable, comma-separated).")
	fs.StringSliceVar(&f.Exclude, "exclude", nil, "Gateway ID the policy never applies to (repeatable, comma-separated).")
}

// Body returns the request fields whose flags were set on cmd, so an update
// sends only what changed. The target is sent whole when any target flag is set.
func (f *Fields) Body(cmd *cobra.Command) (map[string]interface{}, error) {
	fs := cmd.Flags()
	if fs.Changed("cedar") && fs.Changed("cedar-file") {
		return nil, fmt.Errorf("--cedar and --cedar-file are mutually exclusive")
	}
	if f.All && len(f.GatewayIDs) > 0 {
		return nil, fmt.Errorf("--all and --gateway-id are mutually exclusive")
	}

	body := map[string]interface{}{}
	if fs.Changed("description") {
		body["description"] = f.Description
	}
	switch {
	case fs.Changed("cedar"):
		body["cedar"] = f.Cedar
	case fs.Changed("cedar-file"):
		cedar, err := readCedar(cmd, f.CedarFile)
		if err != nil {
			return nil, err
		}
		body["cedar"] = cedar
	}
	if fs.Changed("user-message") {
		body["user_message"] = f.UserMessage
	}
	if fs.Changed("documentation-url") {
		body["documentation_url"] = f.DocumentationURL
	}
	if fs.Changed("all") || fs.Changed("gateway-id") || fs.Changed("exclude") {
		target := map[string]interface{}{"all": f.All || len(f.GatewayIDs) == 0}
		if len(f.GatewayIDs) > 0 {
			target["gateway_ids"] = f.GatewayIDs
		}
		if fs.Changed("exclude") {
			target["exclude"] = nonNil(f.Exclude)
		}
		body["target"] = target
	}
	return body, nil
}

func readCedar(cmd *cobra.Command, path string) (string, error) {
	var (
		data []byte
		err  error
	)
	if path == "-" {
		data, err = io.ReadAll(cmd.InOrStdin())
	} else {
		data, err = os.ReadFile(path)
	}
	if err != nil {
		return "", fmt.Errorf("can't read Cedar file: %w", err)
	}
	if len(data) == 0 {
		return "", fmt.Errorf("cedar file %q is empty", path)
	}
	return string(data), nil
}

func nonNil(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
