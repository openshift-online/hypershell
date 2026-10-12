package gatewayAccess

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/openshift-online/hypershell/components/cli/pkg/config"
	"github.com/openshift-online/hypershell/components/cli/pkg/connection"
	"github.com/openshift-online/hypershell/components/cli/pkg/gatewayaccess"
)

var args struct {
	gatewayID string
	role      string
	search    string
	page      int
	size      int
	sort      string
	order     string
	output    string
}

var Cmd = &cobra.Command{
	Use:     "gatewayAccess [flags]",
	Aliases: []string{"gateway-access"},
	Short:   "List access grants on a gateway",
	Args:    cobra.NoArgs,
	RunE:    run,
}

func init() {
	flags := Cmd.Flags()
	flags.StringVar(&args.gatewayID, "gateway-id", "", "Gateway ID.")
	flags.StringVar(&args.role, "role", "", "Role filter: owner, admin, or user.")
	flags.StringVar(&args.search, "search", "", "Case-insensitive literal search over username and name.")
	flags.IntVar(&args.page, "page", 1, "Page number.")
	flags.IntVar(&args.size, "size", 20, "Page size (1-100).")
	flags.StringVar(&args.sort, "sort", "granted_at", "Sort field.")
	flags.StringVar(&args.order, "order", "desc", "Sort order: asc or desc.")
	flags.StringVarP(&args.output, "output", "o", "json", "Structured output format (json).")
}

func run(cmd *cobra.Command, _ []string) error {
	if args.gatewayID == "" {
		return fmt.Errorf("--gateway-id is required")
	}
	if args.role != "" {
		if err := gatewayaccess.ValidateRole(args.role); err != nil {
			return err
		}
	}
	if err := gatewayaccess.ValidateOutput(args.output); err != nil {
		return err
	}
	query := url.Values{"page": {strconv.Itoa(args.page)}, "size": {strconv.Itoa(args.size)}, "sort": {args.sort}, "order": {args.order}}
	if args.role != "" {
		query.Set("role", args.role)
	}
	if args.search != "" {
		query.Set("search", args.search)
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	conn, err := connection.NewConnection().Config(cfg).Build()
	if err != nil {
		return err
	}
	defer conn.Close()
	response, _, err := gatewayaccess.Request(conn, http.MethodGet, gatewayaccess.CollectionPath(args.gatewayID), query, nil, http.StatusOK)
	if err != nil {
		return fmt.Errorf("can't list gateway access: %w", err)
	}
	return gatewayaccess.WriteJSON(cmd.OutOrStdout(), response)
}
