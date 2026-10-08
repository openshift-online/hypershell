package apply

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
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

	conn, err := connection.NewConnectionBuilder().Config(cfg).Build()
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
		resources, err = loadFromKustomize(args.kustomize)
		if err != nil {
			return err
		}
	}

	results := []map[string]interface{}{}

	var hasError bool
	for _, resource := range resources {
		// Validate supported kind
		if !isSupportedKind(resource.Kind) {
			fmt.Fprintf(os.Stderr, "Skipping unsupported kind: %s/%s\n", resource.Kind, getName(resource))
			continue
		}

		if args.dryRun {
			result := map[string]interface{}{
				"kind":   resource.Kind,
				"name":   getName(resource),
				"status": "dry-run",
			}
			results = append(results, result)
			if args.outputFmt != "json" {
				fmt.Printf("%s/%s (dry run)\n", strings.ToLower(resource.Kind), getName(resource))
			}
			continue
		}

		result, err := applyResource(conn, resource)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error applying %s/%s: %v\n", resource.Kind, getName(resource), err)
			hasError = true
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

	if hasError {
		return fmt.Errorf("one or more resources failed to apply")
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

func loadFromKustomize(dir string) ([]Resource, error) {
	// Verify directory exists
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("kustomize directory not found: %s", dir)
		}
		return nil, fmt.Errorf("stat kustomize directory %s: %w", dir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("kustomize path is not a directory: %s", dir)
	}

	// Check for kustomization.yaml or kustomization.yml
	kustomizationPath := filepath.Join(dir, "kustomization.yaml")
	if _, err := os.Stat(kustomizationPath); os.IsNotExist(err) {
		kustomizationPath = filepath.Join(dir, "kustomization.yml")
		if _, err := os.Stat(kustomizationPath); os.IsNotExist(err) {
			return nil, fmt.Errorf("no kustomization.yaml or kustomization.yml found in %s", dir)
		}
	}

	// Find kustomize binary
	kustomizeBin, err := exec.LookPath("kustomize")
	if err != nil {
		return nil, fmt.Errorf("kustomize binary not found in PATH - install from https://kustomize.io/")
	}

	// Run kustomize build with security flags
	// --enable-alpha-plugins=false prevents arbitrary plugin execution
	// --load-restrictor=LoadRestrictionsRootOnly restricts file loading to root directory
	cmd := exec.Command(kustomizeBin, "build",
		"--enable-alpha-plugins=false",
		"--load-restrictor=LoadRestrictionsRootOnly",
		dir)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// Include stderr for better error messages
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg == "" {
			errMsg = err.Error()
		}
		return nil, fmt.Errorf("kustomize build failed: %s", errMsg)
	}

	// Parse the YAML stream from kustomize output
	resources, err := parseYAMLStream(&stdout)
	if err != nil {
		return nil, fmt.Errorf("failed to parse kustomize output: %w", err)
	}

	if len(resources) == 0 {
		return nil, fmt.Errorf("kustomize build produced no resources from %s", dir)
	}

	return resources, nil
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

func isSupportedKind(kind string) bool {
	supportedKinds := map[string]bool{
		"Gateway":         true,
		"ManagedCluster":  true,
		"Role":            true,
		"RoleBinding":     true,
		"AgentRuntime":    true,
		"SandboxTemplate": true,
		"ProviderSpec":    true,
		"ProviderBinding": true,
		"InferenceRoute":  true,
		"SecretSource":    true,
	}
	return supportedKinds[kind]
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
	case "ManagedCluster":
		basePath = urls.ManagedClustersPath
	case "Role":
		basePath = urls.RolesPath
	case "RoleBinding":
		basePath = urls.RoleBindingsPath
	case "AgentRuntime":
		basePath = urls.AgentRuntimesPath
	case "SandboxTemplate":
		basePath = urls.SandboxTemplatesPath
	case "ProviderSpec":
		basePath = urls.ProviderSpecsPath
	case "ProviderBinding":
		basePath = urls.ProviderBindingsPath
	case "InferenceRoute":
		basePath = urls.InferenceRoutesPath
	case "SecretSource":
		basePath = urls.SecretSourcesPath
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
	// TSL (Tree Search Language) requires values containing hyphens or other
	// special characters to be single-quoted; double an embedded single quote.
	quotedName := "'" + strings.ReplaceAll(name, "'", "''") + "'"
	listResp, err := conn.List(basePath, 1, 100, "name = "+quotedName, "")
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
