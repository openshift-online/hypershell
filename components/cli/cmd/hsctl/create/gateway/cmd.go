package gateway

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/openshift-online/hypershell/components/cli/pkg/config"
	"github.com/openshift-online/hypershell/components/cli/pkg/connection"
	"github.com/openshift-online/hypershell/components/cli/pkg/dump"
	"github.com/openshift-online/hypershell/components/cli/pkg/placement"
	"github.com/openshift-online/hypershell/components/cli/pkg/urls"
)

const longHelp = "Create a new gateway.\n\n" +
	"The gateway is created with exactly one placement shape: pinned to a managed\n" +
	"cluster (--cluster-id), or late-bound by capability-label selectors\n" +
	"(repeatable --placement-selector, optionally with\n" +
	"--placement-exclude-selector), or the local-kind development mode\n" +
	"(--local-kind). When a selector matches no connected cluster, the gateway is\n" +
	"created Pending with an empty cluster_id and binds when a matching cluster\n" +
	"connects; an empty cluster_id in the output means late binding, not failure.\n\n" +
	"Examples:\n" +
	"  hsctl create gateway --name my-gw --cluster-id <id>\n" +
	"  hsctl create gateway --name my-gw --placement-selector network/public --placement-selector provider/ibm\n" +
	"  hsctl create gateway --name my-gw --placement-selector region/us-east-1 --placement-exclude-selector runtime/kata\n" +
	"  hsctl create gateway --name my-gw --local-kind --dedicated-route\n" +
	"  hsctl create gateway --name my-gw --dry-run --placement-selector network/public\n" +
	"  hsctl create gateway --body request.json"

type createGatewayArgs struct {
	clusterId                string
	dedicatedRoute           bool
	name                     string
	bodyFile                 string
	placementSelector        []string
	placementExcludeSelector []string
	localKind                bool
	dryRun                   bool
}

// newCreateGatewayCommand builds the command with its own flag set and args
// struct so tests can execute fresh instances without cobra's parsed-flag
// state leaking between runs.
func newCreateGatewayCommand() *cobra.Command {
	args := &createGatewayArgs{}
	cmd := &cobra.Command{
		Use:   "gateway [flags]",
		Short: "Create a gateway",
		Long:  longHelp,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, argv []string) error {
			return run(args, cmd, argv)
		},
	}

	fs := cmd.Flags()
	fs.StringVar(&args.clusterId, "cluster-id", "", "Pin the gateway to this managed cluster (discover it via 'hsctl list managedClusters'). Mutually exclusive with the placement flags.")
	fs.BoolVar(&args.dedicatedRoute, "dedicated-route", false, "Request the dedicated per-gateway route and wildcard certificate at creation. Requires the gateway:dedicated-route-creator role.")
	fs.StringVar(&args.name, "name", "", "Gateway name.")
	fs.StringVar(&args.bodyFile, "body", "", "File containing the request body as JSON.")
	fs.StringArrayVar(&args.placementSelector, "placement-selector", nil, "Capability label the gateway's cluster must advertise (e.g. network/public, provider/ibm, region/us-east-1). Repeat the flag to require multiple labels. Mutually exclusive with --cluster-id and --local-kind.")
	fs.StringArrayVar(&args.placementExcludeSelector, "placement-exclude-selector", nil, "Capability label the gateway's cluster must not advertise. Requires at least one --placement-selector.")
	fs.BoolVar(&args.localKind, "local-kind", false, "Place the gateway on the local Kind development cluster. Mutually exclusive with the selector flags and --cluster-id.")
	fs.BoolVar(&args.dryRun, "dry-run", false, "Check placement availability without creating the gateway.")

	// The server accepts exactly one placement shape (data-model.spec.md
	// PLAT-DATA-02); mirror that here so conflicting flags fail before any
	// request. The exclusion-requires-selector dependency is enforced in
	// placement.BuildIntent, not by cobra.
	cmd.MarkFlagsMutuallyExclusive("cluster-id", "placement-selector")
	cmd.MarkFlagsMutuallyExclusive("cluster-id", "placement-exclude-selector")
	cmd.MarkFlagsMutuallyExclusive("cluster-id", "local-kind")
	cmd.MarkFlagsMutuallyExclusive("local-kind", "placement-selector")
	cmd.MarkFlagsMutuallyExclusive("local-kind", "placement-exclude-selector")
	cmd.MarkFlagsMutuallyExclusive("dry-run", "body")

	return cmd
}

var Cmd = newCreateGatewayCommand()

func run(args *createGatewayArgs, cmd *cobra.Command, argv []string) error {
	// Fail fast on invalid placement flags before loading config or dialing.
	// The --body escape hatch bypasses flag building entirely, so it skips
	// validation too (CLI-PLC-01.09).
	var intent map[string]interface{}
	if args.bodyFile == "" {
		var err error
		intent, err = placement.BuildIntent(args.placementSelector, args.placementExcludeSelector, args.localKind)
		if err != nil {
			return err
		}
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	conn, err := connection.NewConnectionBuilder().Config(cfg).Build()
	if err != nil {
		return err
	}
	defer conn.Close()

	if args.dryRun {
		return runDryRun(args, conn)
	}

	var body []byte

	if args.bodyFile != "" {
		body, err = os.ReadFile(args.bodyFile)
		if err != nil {
			return fmt.Errorf("can't read body file: %v", err)
		}
	} else {
		request := map[string]interface{}{}
		if args.clusterId != "" {
			request["cluster_id"] = args.clusterId
		}
		if args.dedicatedRoute {
			request["dedicated_route"] = args.dedicatedRoute
		}
		if args.name != "" {
			request["name"] = args.name
		}
		if intent != nil {
			request["placement"] = intent
		}
		body, err = json.Marshal(request)
		if err != nil {
			return fmt.Errorf("can't marshal request: %v", err)
		}
	}

	resp, err := conn.Post(urls.GatewaysPath, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("can't create gateway: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("can't read response: %v", err)
	}

	if resp.StatusCode != 201 && resp.StatusCode != 200 {
		return fmt.Errorf("API returned %d: %s", resp.StatusCode, string(respBody))
	}

	return dump.Pretty(os.Stdout, respBody)
}

// runDryRun checks placement availability without creating a gateway
// (CLI-PLC-03). The local-kind and pinned shapes make no availability
// request: the availability endpoint's contract is capability-selector
// based, and those shapes are validated server-side at create time.
func runDryRun(args *createGatewayArgs, conn *connection.Connection) error {
	switch {
	case args.localKind:
		fmt.Println("local-kind placement resolves only in the Kind development environment; the server validates it at create time.")
		fmt.Println("No gateway created (dry run).")
		return nil
	case args.clusterId != "":
		fmt.Printf("Would create the gateway pinned to cluster %s; the server validates the cluster reference at create time.\n", args.clusterId)
		fmt.Println("No gateway created (dry run).")
		return nil
	case len(args.placementSelector) > 0:
	default:
		return fmt.Errorf("nothing to check: a gateway must be created with either a cluster_id (pinned) or a placement selector (late-bound)")
	}

	requestBody, err := json.Marshal(placement.AvailabilityRequest(args.placementSelector))
	if err != nil {
		return fmt.Errorf("can't marshal availability request: %v", err)
	}
	resp, err := conn.Post(urls.GatewaysPlacementAvailabilityPath, bytes.NewReader(requestBody))
	if err != nil {
		return fmt.Errorf("can't check placement availability: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("can't read response: %v", err)
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("API returned %d: %s", resp.StatusCode, string(respBody))
	}

	var availability struct {
		Placements []struct {
			Selector  []string `json:"selector"`
			Available bool     `json:"available"`
		} `json:"placements"`
	}
	if err := json.Unmarshal(respBody, &availability); err != nil {
		return fmt.Errorf("can't parse availability response: %v", err)
	}

	labels := strings.Join(args.placementSelector, ", ")
	// Unavailability is a normal late-binding outcome, not an error: the
	// create would succeed and hold the gateway Pending until a matching
	// cluster connects (CLI-PLC-03.02).
	if len(availability.Placements) == 1 && availability.Placements[0].Available {
		fmt.Printf("Placement available: %s\n", labels)
	} else {
		fmt.Printf("No connected managed cluster currently matches: %s\n", labels)
		fmt.Println("The create would succeed and hold the gateway in Pending (awaiting placement) until a matching cluster connects.")
	}
	if len(args.placementExcludeSelector) > 0 {
		fmt.Println("Note: the availability check evaluates required labels only; excluded labels are enforced at create time.")
	}
	fmt.Println("No gateway created (dry run).")
	return nil
}
