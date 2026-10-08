package gateway

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/openshift-online/hypershell/components/cli/pkg/config"
	"github.com/openshift-online/hypershell/components/cli/pkg/connection"
	"github.com/openshift-online/hypershell/components/cli/pkg/dump"
	"github.com/openshift-online/hypershell/components/cli/pkg/urls"
)

var args struct {
	clusterId        string
	credentialDriver string
	externalDns      string
	image            string
	name             string
	oidc             string
	phase            string
	route            string
	routeAddress     string
	serverDnsNames   string
	serviceType      string
	status           string
	supervisorImage  string
	tlsMode          string
	bodyFile         string
}

var Cmd = &cobra.Command{
	Use:     "gateway ID [flags]",
	Aliases: []string{"gateways"},
	Short:   "Update a gateway by ID",
	Long: "Update a gateway by ID using PATCH. Only the flags that are set are sent.\n\n" +
		"Examples:\n" +
		"  hsctl update gateway ID --cluster-id <value> --credential-driver <value> --external-dns <value> --image <value> --name <value> --oidc <value> --phase <value> --route <value> --route-address <value> --server-dns-names <value> --service-type <value> --status <value> --supervisor-image <value> --tls-mode <value>\n" +
		"  hsctl update gateway ID --body request.json",
	Args: cobra.ExactArgs(1),
	RunE: run,
}

func init() {
	fs := Cmd.Flags()
	fs.StringVar(&args.clusterId, "cluster-id", "", "cluster_id value.")
	fs.StringVar(&args.credentialDriver, "credential-driver", "", "credential_driver value.")
	fs.StringVar(&args.externalDns, "external-dns", "", "external_dns value.")
	fs.StringVar(&args.image, "image", "", "image value.")
	fs.StringVar(&args.name, "name", "", "name value.")
	fs.StringVar(&args.oidc, "oidc", "", "oidc value.")
	fs.StringVar(&args.phase, "phase", "", "phase value.")
	fs.StringVar(&args.route, "route", "", "route value.")
	fs.StringVar(&args.routeAddress, "route-address", "", "route_address value.")
	fs.StringVar(&args.serverDnsNames, "server-dns-names", "", "server_dns_names value.")
	fs.StringVar(&args.serviceType, "service-type", "", "service_type value.")
	fs.StringVar(&args.status, "status", "", "status value.")
	fs.StringVar(&args.supervisorImage, "supervisor-image", "", "supervisor_image value.")
	fs.StringVar(&args.tlsMode, "tls-mode", "", "tls_mode value.")
	fs.StringVar(&args.bodyFile, "body", "", "File containing the request body as JSON.")
}

func run(cmd *cobra.Command, argv []string) error {
	id := argv[0]

	var body []byte
	var err error
	if args.bodyFile != "" {
		body, err = os.ReadFile(args.bodyFile)
		if err != nil {
			return fmt.Errorf("can't read body file: %v", err)
		}
	} else {
		request := map[string]interface{}{}
		if cmd.Flags().Changed("cluster-id") {
			request["cluster_id"] = args.clusterId
		}
		if cmd.Flags().Changed("credential-driver") {
			request["credential_driver"] = args.credentialDriver
		}
		if cmd.Flags().Changed("external-dns") {
			request["external_dns"] = args.externalDns
		}
		if cmd.Flags().Changed("image") {
			request["image"] = args.image
		}
		if cmd.Flags().Changed("name") {
			request["name"] = args.name
		}
		if cmd.Flags().Changed("oidc") {
			request["oidc"] = args.oidc
		}
		if cmd.Flags().Changed("phase") {
			request["phase"] = args.phase
		}
		if cmd.Flags().Changed("route") {
			request["route"] = args.route
		}
		if cmd.Flags().Changed("route-address") {
			request["route_address"] = args.routeAddress
		}
		if cmd.Flags().Changed("server-dns-names") {
			request["server_dns_names"] = args.serverDnsNames
		}
		if cmd.Flags().Changed("service-type") {
			request["service_type"] = args.serviceType
		}
		if cmd.Flags().Changed("status") {
			request["status"] = args.status
		}
		if cmd.Flags().Changed("supervisor-image") {
			request["supervisor_image"] = args.supervisorImage
		}
		if cmd.Flags().Changed("tls-mode") {
			request["tls_mode"] = args.tlsMode
		}
		if len(request) == 0 {
			return fmt.Errorf("nothing to update: set at least one field flag or pass --body")
		}
		body, err = json.Marshal(request)
		if err != nil {
			return fmt.Errorf("can't marshal request: %v", err)
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

	resp, err := conn.Patch(urls.GatewayPath(id), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("can't update gateway: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("can't read response: %v", err)
	}

	switch resp.StatusCode {
	case 200, 202:
	case 204:
		fmt.Fprintf(os.Stdout, "Gateway %s updated.\n", id)
		return nil
	default:
		return fmt.Errorf("API returned %d: %s", resp.StatusCode, string(respBody))
	}

	return dump.Pretty(os.Stdout, respBody)
}
