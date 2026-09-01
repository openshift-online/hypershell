package managedClusters

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/openshift-online/hypershell/components/cli/pkg/arguments"
	"github.com/openshift-online/hypershell/components/cli/pkg/config"
	"github.com/openshift-online/hypershell/components/cli/pkg/connection"
	"github.com/openshift-online/hypershell/components/cli/pkg/dump"
	"github.com/openshift-online/hypershell/components/cli/pkg/output"
	"github.com/openshift-online/hypershell/components/cli/pkg/urls"
)

var args struct {
	parameter []string
	noHeaders bool
	columns   string
	outputFmt string
	search    string
	orderBy   string
}

var Cmd = &cobra.Command{
	Use:     "managedClusters [flags]",
	Aliases: []string{"managedCluster"},
	Short:   "List managedClusters",
	Long:    "List managedClusters, optionally filtering by search query",
	Args:    cobra.NoArgs,
	RunE:    run,
}

func init() {
	fs := Cmd.Flags()
	arguments.AddParameterFlag(fs, &args.parameter)
	arguments.AddNoHeadersFlag(fs, &args.noHeaders)
	arguments.AddColumnsFlag(fs, &args.columns, "id, api_server_url, database_id, kubeconfig_secret, name, created_at")
	arguments.AddOutputFlag(fs, &args.outputFmt)
	fs.StringVar(&args.search, "search", "", "Search filter expression.")
	fs.StringVar(&args.orderBy, "order-by", "", "Order by expression.")
}

func run(cmd *cobra.Command, argv []string) error {
	ctx := context.Background()

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	conn, err := connection.NewConnection().Config(cfg).Build()
	if err != nil {
		return err
	}
	defer conn.Close()

	searchQuery := args.search
	for _, param := range args.parameter {
		name, value := arguments.ParseNameValuePair(param)
		if name == "search" {
			if searchQuery != "" {
				searchQuery += " and "
			}
			searchQuery += value
		}
	}

	if args.outputFmt == "json" {
		return listJSON(conn, searchQuery)
	}

	printer, err := output.NewPrinter().
		Writer(os.Stdout).
		Pager(cfg.Pager).
		Build(ctx)
	if err != nil {
		return err
	}
	defer printer.Close()

	table, err := printer.NewTable().
		Name("managedClusters").
		Columns(args.columns).
		Build(ctx)
	if err != nil {
		return err
	}
	defer table.Close()

	if !args.noHeaders {
		table.WriteHeaders()
	}

	size := 100
	page := 1
	for {
		resp, err := conn.List(urls.ManagedClustersPath, page, size, searchQuery, args.orderBy)
		if err != nil {
			return fmt.Errorf("can't retrieve managedClusters: %v", err)
		}

		for _, item := range resp.Items {
			err = table.WriteRawObject(item)
			if err != nil {
				return err
			}
		}

		if resp.Size < size {
			break
		}
		page++
	}

	return nil
}

func listJSON(conn *connection.Connection, search string) error {
	size := 100
	page := 1
	var allItems []json.RawMessage

	for {
		resp, err := conn.List(urls.ManagedClustersPath, page, size, search, args.orderBy)
		if err != nil {
			return fmt.Errorf("can't retrieve managedClusters: %v", err)
		}
		allItems = append(allItems, resp.Items...)
		if resp.Size < size {
			break
		}
		page++
	}

	result := map[string]interface{}{
		"kind":  "ManagedClusterList",
		"total": len(allItems),
		"items": allItems,
	}
	body, err := json.Marshal(result)
	if err != nil {
		return err
	}
	return dump.Pretty(os.Stdout, body)
}
