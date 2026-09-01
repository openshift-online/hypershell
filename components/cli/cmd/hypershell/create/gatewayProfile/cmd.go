package gatewayProfile

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
	containerCpuLimitMax          string
	containerCpuRequestDefault    string
	containerMemoryLimitMax       string
	containerMemoryRequestDefault string
	cpuLimitTotal                 string
	cpuRequestTotal               string
	description                   string
	ephemeralStorageTotal         string
	memoryLimitTotal              string
	memoryRequestTotal            string
	name                          string
	podCount                      int
	pvcCount                      int
	bodyFile                      string
}

var Cmd = &cobra.Command{
	Use:   "gatewayProfile [flags]",
	Short: "Create a gatewayProfile",
	Long: "Create a new gatewayProfile.\n\n" +
		"Examples:\n" +
		"  hypershell create gatewayProfile --container-cpu-limit-max <value> --container-cpu-request-default <value> --container-memory-limit-max <value> --container-memory-request-default <value> --cpu-limit-total <value> --cpu-request-total <value> --description <value> --ephemeral-storage-total <value> --memory-limit-total <value> --memory-request-total <value> --name <value> --pod-count <value> --pvc-count <value> \n" +
		"  hypershell create gatewayProfile --body request.json",
	Args: cobra.NoArgs,
	RunE: run,
}

func init() {
	fs := Cmd.Flags()
	fs.StringVar(&args.containerCpuLimitMax, "container-cpu-limit-max", "", "container_cpu_limit_max value.")
	fs.StringVar(&args.containerCpuRequestDefault, "container-cpu-request-default", "", "container_cpu_request_default value.")
	fs.StringVar(&args.containerMemoryLimitMax, "container-memory-limit-max", "", "container_memory_limit_max value.")
	fs.StringVar(&args.containerMemoryRequestDefault, "container-memory-request-default", "", "container_memory_request_default value.")
	fs.StringVar(&args.cpuLimitTotal, "cpu-limit-total", "", "cpu_limit_total value.")
	fs.StringVar(&args.cpuRequestTotal, "cpu-request-total", "", "cpu_request_total value.")
	fs.StringVar(&args.description, "description", "", "description value.")
	fs.StringVar(&args.ephemeralStorageTotal, "ephemeral-storage-total", "", "ephemeral_storage_total value.")
	fs.StringVar(&args.memoryLimitTotal, "memory-limit-total", "", "memory_limit_total value.")
	fs.StringVar(&args.memoryRequestTotal, "memory-request-total", "", "memory_request_total value.")
	fs.StringVar(&args.name, "name", "", "name value.")
	fs.IntVar(&args.podCount, "pod-count", 0, "pod_count value.")
	fs.IntVar(&args.pvcCount, "pvc-count", 0, "pvc_count value.")
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
		if args.containerCpuLimitMax != "" {
			request["container_cpu_limit_max"] = args.containerCpuLimitMax
		}
		if args.containerCpuRequestDefault != "" {
			request["container_cpu_request_default"] = args.containerCpuRequestDefault
		}
		if args.containerMemoryLimitMax != "" {
			request["container_memory_limit_max"] = args.containerMemoryLimitMax
		}
		if args.containerMemoryRequestDefault != "" {
			request["container_memory_request_default"] = args.containerMemoryRequestDefault
		}
		if args.cpuLimitTotal != "" {
			request["cpu_limit_total"] = args.cpuLimitTotal
		}
		if args.cpuRequestTotal != "" {
			request["cpu_request_total"] = args.cpuRequestTotal
		}
		if args.description != "" {
			request["description"] = args.description
		}
		if args.ephemeralStorageTotal != "" {
			request["ephemeral_storage_total"] = args.ephemeralStorageTotal
		}
		if args.memoryLimitTotal != "" {
			request["memory_limit_total"] = args.memoryLimitTotal
		}
		if args.memoryRequestTotal != "" {
			request["memory_request_total"] = args.memoryRequestTotal
		}
		if args.name != "" {
			request["name"] = args.name
		}
		if args.podCount != 0 {
			request["pod_count"] = args.podCount
		}
		if args.pvcCount != 0 {
			request["pvc_count"] = args.pvcCount
		}
		body, err = json.Marshal(request)
		if err != nil {
			return fmt.Errorf("can't marshal request: %v", err)
		}
	}

	resp, err := conn.Post(urls.GatewayProfilesPath, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("can't create gatewayProfile: %v", err)
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
