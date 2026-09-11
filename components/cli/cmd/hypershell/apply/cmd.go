package apply

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/openshift-online/hypershell/components/cli/pkg/config"
	"github.com/openshift-online/hypershell/components/cli/pkg/connection"
	"github.com/openshift-online/hypershell/components/cli/pkg/urls"
)

var args struct {
	filename  string
	kustomize string
	dryRun    bool
	outputFmt string
}

var Cmd = &cobra.Command{
	Use:   "apply",
	Short: "Apply resources from file or directory",
	Long: "Apply resources from YAML files or kustomize directories.\n\n" +
		"Examples:\n" +
		"  hsctl apply -f resource.yaml\n" +
		"  hsctl apply -f ./resources/\n" +
		"  hsctl apply -k ./overlays/prod/\n" +
		"  hsctl apply -f - < resource.yaml",
	Args: cobra.NoArgs,
	RunE: run,
}

func init() {
	fs := Cmd.Flags()
	fs.StringVarP(&args.filename, "filename", "f", "", "File or directory containing resources")
	fs.StringVarP(&args.kustomize, "kustomize", "k", "", "Kustomize directory")
	fs.BoolVar(&args.dryRun, "dry-run", false, "Print what would be applied without making changes")
	fs.StringVarP(&args.outputFmt, "output", "o", "", "Output format (json)")

	Cmd.MarkFlagsMutuallyExclusive("filename", "kustomize")
}

type Resource struct {
	APIVersion string                 `yaml:"apiVersion" json:"apiVersion"`
	Kind       string                 `yaml:"kind" json:"kind"`
	Metadata   map[string]interface{} `yaml:"metadata" json:"metadata"`
	Spec       map[string]interface{} `yaml:"spec" json:"spec,omitempty"`
}

func run(cmd *cobra.Command, argv []string) error {
	if args.filename == "" && args.kustomize == "" {
		return fmt.Errorf("must specify either -f or -k")
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

	var resources []Resource

	if args.filename != "" {
		resources, err = loadFromFile(args.filename)
		if err != nil {
			return err
		}
	} else if args.kustomize != "" {
		// For now, treat kustomize as a directory of YAML files
		// Full kustomize support would require running `kustomize build`
		return fmt.Errorf("kustomize support not yet implemented - use -f for now")
	}

	results := []map[string]interface{}{}

	for _, resource := range resources {
		result, err := applyResource(conn, resource)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error applying %s/%s: %v\n", resource.Kind, getName(resource), err)
			continue
		}
		results = append(results, result)

		if args.outputFmt != "json" {
			fmt.Printf("%s/%s %s\n",
				strings.ToLower(resource.Kind),
				getName(resource),
				result["status"])
		}
	}

	if args.outputFmt == "json" {
		output, err := json.MarshalIndent(results, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(output))
	}

	return nil
}

func loadFromFile(path string) ([]Resource, error) {
	if path == "-" {
		return parseYAMLStream(os.Stdin)
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	if info.IsDir() {
		return loadFromDirectory(path)
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	return parseYAMLStream(file)
}

func loadFromDirectory(dir string) ([]Resource, error) {
	var allResources []Resource

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".yaml") && !strings.HasSuffix(path, ".yml") {
			return nil
		}

		resources, err := loadFromFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to parse %s: %v\n", path, err)
			return nil
		}
		allResources = append(allResources, resources...)
		return nil
	})

	return allResources, err
}

func parseYAMLStream(reader io.Reader) ([]Resource, error) {
	var resources []Resource

	decoder := yaml.NewDecoder(reader)
	for {
		var resource Resource
		err := decoder.Decode(&resource)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		if resource.Kind == "" {
			continue
		}

		resources = append(resources, resource)
	}

	return resources, nil
}

func getName(resource Resource) string {
	if resource.Metadata == nil {
		return ""
	}
	name, ok := resource.Metadata["name"].(string)
	if !ok {
		return ""
	}
	return name
}

func applyResource(conn *connection.Connection, resource Resource) (map[string]interface{}, error) {
	kind := resource.Kind
	name := getName(resource)

	if name == "" {
		return nil, fmt.Errorf("resource missing metadata.name")
	}

	// Map kind to API path
	var basePath string
	switch kind {
	case "Gateway":
		basePath = urls.GatewaysPath
	case "GatewayNetwork":
		basePath = urls.GatewayNetworksPath
	case "GatewayRelease":
		basePath = urls.GatewayReleasesPath
	case "ManagedCluster":
		basePath = urls.ManagedClustersPath
	case "ManagedDatabase":
		basePath = urls.ManagedDatabasesPath
	case "Role":
		basePath = urls.RolesPath
	case "RoleBinding":
		basePath = urls.RoleBindingsPath
	default:
		return nil, fmt.Errorf("unsupported kind: %s", kind)
	}

	// Check if resource exists
	existing, err := getResourceByName(conn, basePath, name)
	if err != nil {
		return nil, err
	}

	// Merge spec into resource body
	body := make(map[string]interface{})
	if resource.Spec != nil {
		for k, v := range resource.Spec {
			body[k] = v
		}
	}
	body["name"] = name

	// Add metadata fields if present
	if resource.Metadata != nil {
		if desc, ok := resource.Metadata["description"].(string); ok {
			body["description"] = desc
		}
	}

	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	var status string
	if existing == nil {
		// Create
		resp, err := conn.Post(basePath, bytes.NewReader(bodyJSON))
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 && resp.StatusCode != 201 {
			respBody, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("API returned %d: %s", resp.StatusCode, string(respBody))
		}
		status = "created"
	} else {
		// Update
		existingID, ok := existing["id"].(string)
		if !ok {
			return nil, fmt.Errorf("existing resource has no id")
		}

		updatePath := basePath + "/" + existingID
		resp, err := conn.Patch(updatePath, bytes.NewReader(bodyJSON))
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			respBody, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("API returned %d: %s", resp.StatusCode, string(respBody))
		}
		status = "configured"
	}

	return map[string]interface{}{
		"kind":   kind,
		"name":   name,
		"status": status,
	}, nil
}

func getResourceByName(conn *connection.Connection, basePath, name string) (map[string]interface{}, error) {
	// List resources with name filter
	listResp, err := conn.List(basePath, 1, 100, fmt.Sprintf("name=%s", name), "")
	if err != nil {
		return nil, err
	}

	for _, item := range listResp.Items {
		var resource map[string]interface{}
		if err := json.Unmarshal(item, &resource); err != nil {
			continue
		}
		if resourceName, ok := resource["name"].(string); ok && resourceName == name {
			return resource, nil
		}
	}

	return nil, nil
}
