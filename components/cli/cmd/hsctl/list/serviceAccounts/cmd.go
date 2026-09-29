package serviceAccounts

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/openshift-online/hypershell/components/cli/pkg/config"
	"github.com/openshift-online/hypershell/components/cli/pkg/connection"
	"github.com/openshift-online/hypershell/components/cli/pkg/serviceaccount"
)

var args struct {
	gatewayID string
	page      int
	size      int
	status    string
	search    string
	sort      string
	order     string
	output    string
}

var Cmd = &cobra.Command{
	Use:     "serviceAccounts [flags]",
	Aliases: []string{"serviceAccount", "service-accounts", "service-account"},
	Short:   "List OpenShell gateway service accounts",
	Args:    cobra.NoArgs,
	RunE:    run,
}

func init() {
	flags := Cmd.Flags()
	flags.StringVar(&args.gatewayID, "gateway-id", "", "Gateway ID.")
	flags.IntVar(&args.page, "page", 1, "Page number.")
	flags.IntVar(&args.size, "size", 20, "Page size (1-100).")
	flags.StringVar(&args.status, "status", "", "Status filter.")
	flags.StringVar(&args.search, "search", "", "Case-insensitive literal search.")
	flags.StringVar(&args.sort, "sort", "created_at", "Sort field.")
	flags.StringVar(&args.order, "order", "desc", "Sort order: asc or desc.")
	flags.StringVarP(&args.output, "output", "o", "json", "Structured output format (json).")
}

func run(cmd *cobra.Command, _ []string) error {
	if args.gatewayID == "" {
		return fmt.Errorf("--gateway-id is required")
	}
	if err := serviceaccount.ValidateOutput(args.output); err != nil {
		return err
	}
	query := url.Values{"page": {strconv.Itoa(args.page)}, "size": {strconv.Itoa(args.size)}, "sort": {args.sort}, "order": {args.order}}
	if args.status != "" {
		query.Set("status", args.status)
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
	response, _, err := serviceaccount.Request(conn, http.MethodGet, serviceaccount.CollectionPath(args.gatewayID), query, nil, http.StatusOK)
	if err != nil {
		return fmt.Errorf("can't list service accounts: %w", err)
	}
	return serviceaccount.WriteStructured(cmd.OutOrStdout(), "", response)
}
