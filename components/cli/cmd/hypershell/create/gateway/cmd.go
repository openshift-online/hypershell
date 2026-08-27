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
	clusterId       string
	databaseId      string
	externalDns     string
	image           string
	name            string
	phase           string
	releaseId       string
	route           string
	serverDnsNames  string
	serviceType     string
	status          string
	supervisorImage string
	tlsMode         string
	bodyFile        string
}

var Cmd = &cobra.Command{
	Use:   "gateway [flags]",
	Short: "Create a gateway",
	Long: "Create a new gateway.\n\n" +
		"Examples:\n" +
		"  hsctl create gateway --cluster-id <value> --database-id <value> --external-dns <value> --image <value> --name <value> --phase <value> --release-id <value> --route <value> --server-dns-names <value> --service-type <value> --status <value> --supervisor-image <value> --tls-mode <value> \n" +
		"  hsctl create gateway --body request.json",
	Args: cobra.NoArgs,
	RunE: run,
}

func init() {
	fs := Cmd.Flags()
	fs.StringVar(&args.clusterId, "cluster-id", "", "cluster_id value.")
	fs.StringVar(&args.databaseId, "database-id", "", "database_id value.")
	fs.StringVar(&args.externalDns, "external-dns", "", "external_dns value.")
	fs.StringVar(&args.image, "image", "", "image value.")
	fs.StringVar(&args.name, "name", "", "name value.")
	fs.StringVar(&args.phase, "phase", "", "phase value.")
	fs.StringVar(&args.releaseId, "release-id", "", "release_id value.")
	fs.StringVar(&args.route, "route", "", "route value.")
	fs.StringVar(&args.serverDnsNames, "server-dns-names", "", "server_dns_names value.")
	fs.StringVar(&args.serviceType, "service-type", "", "service_type value.")
	fs.StringVar(&args.status, "status", "", "status value.")
	fs.StringVar(&args.supervisorImage, "supervisor-image", "", "supervisor_image value.")
	fs.StringVar(&args.tlsMode, "tls-mode", "", "tls_mode value.")
	fs.StringVar(&args.bodyFile, "body", "", "File containing the request body as JSON.")
}

func run(cmd *cobra.Command, argv []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	conn, err := connection.NewConnection().Config(cfg).Build()
	if err != nil {
		return err
	}
	defer conn.Close()

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
		if args.databaseId != "" {
			request["database_id"] = args.databaseId
		}
		if args.externalDns != "" {
			request["external_dns"] = args.externalDns
		}
		if args.image != "" {
			request["image"] = args.image
		}
		if args.name != "" {
			request["name"] = args.name
		}
		if args.phase != "" {
			request["phase"] = args.phase
		}
		if args.releaseId != "" {
			request["release_id"] = args.releaseId
		}
		route := args.route
		if route == "" {
			route = `{"enabled":true}`
		}
		request["route"] = route
		if args.serverDnsNames != "" {
			request["server_dns_names"] = args.serverDnsNames
		}
		if args.serviceType != "" {
			request["service_type"] = args.serviceType
		}
		if args.status != "" {
			request["status"] = args.status
		}
		if args.supervisorImage != "" {
			request["supervisor_image"] = args.supervisorImage
		}
		if args.tlsMode != "" {
			request["tls_mode"] = args.tlsMode
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
