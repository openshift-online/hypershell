package gateway

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"log"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/yaml"
)

func LoadGatewayManifests(manifestsDir string) (map[string][]*unstructured.Unstructured, error) {
	manifests := make(map[string][]*unstructured.Unstructured)

	entries, err := os.ReadDir(manifestsDir)
	if err != nil {
		return nil, fmt.Errorf("read manifests directory: %w", err)
	}

	requiredFiles := []string{"serviceaccount.yaml", "configmap.yaml", "service.yaml", "rbac.yaml", "deployment.yaml", "database.yaml"}
	foundFiles := make(map[string]bool)

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}

		filePath := filepath.Join(manifestsDir, entry.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("read manifest file %s: %w", entry.Name(), err)
		}

		decoder := yaml.NewYAMLOrJSONDecoder(strings.NewReader(string(data)), 4096)
		var resources []*unstructured.Unstructured

		for {
			obj := &unstructured.Unstructured{}
			if err := decoder.Decode(obj); err != nil {
				if err.Error() == "EOF" {
					break
				}
				return nil, fmt.Errorf("decode manifest %s: %w", entry.Name(), err)
			}

			if len(obj.Object) == 0 {
				continue
			}

			resources = append(resources, obj)
		}

		manifests[entry.Name()] = resources
		foundFiles[entry.Name()] = true

		log.Printf("DEBUG loaded gateway manifest %s (%d resources)", entry.Name(), len(resources))
	}

	for _, required := range requiredFiles {
		if !foundFiles[required] {
			return nil, fmt.Errorf("required manifest file not found: %s", required)
		}
	}

	totalResources := 0
	for _, resources := range manifests {
		totalResources += len(resources)
	}

	log.Printf("INFO loaded %d gateway manifest files with %d resources", len(manifests), totalResources)

	return manifests, nil
}

func ApplyManifestToNamespace(manifest *unstructured.Unstructured, namespace string, config GatewayConfig, images ImageDefaults) (*unstructured.Unstructured, error) {
	obj := manifest.DeepCopy()

	jsonBytes, err := obj.MarshalJSON()
	if err != nil {
		return nil, fmt.Errorf("marshal manifest: %w", err)
	}

	manifestJSON := string(jsonBytes)
	manifestJSON = strings.ReplaceAll(manifestJSON, "NAMESPACE_PLACEHOLDER", namespace)

	supervisorImage := images.DefaultSupervisorImage()
	if config.SupervisorImage != "" {
		supervisorImage = config.SupervisorImage
	}
	manifestJSON = strings.ReplaceAll(manifestJSON, "SUPERVISOR_IMAGE_PLACEHOLDER", supervisorImage)

	image := images.DefaultGatewayImage()
	if config.Image != "" {
		image = config.Image
	}
	dbImage := config.Database.Image
	if dbImage == "" {
		dbImage = "postgres:18"
	}
	dbStorage := config.Database.StorageSize
	if dbStorage == "" {
		dbStorage = "5Gi"
	}
	userKey, passKey, dbKey := postgresEnvKeys(dbImage)
	dataPath := postgresDataPath(dbImage)

	// Replace DB_IMAGE_PLACEHOLDER before IMAGE_PLACEHOLDER because
	// the shorter string is a substring of the longer one.
	manifestJSON = strings.ReplaceAll(manifestJSON, "DB_IMAGE_PLACEHOLDER", dbImage)
	manifestJSON = strings.ReplaceAll(manifestJSON, "DB_STORAGE_PLACEHOLDER", dbStorage)
	manifestJSON = strings.ReplaceAll(manifestJSON, "DB_USER_KEY_PLACEHOLDER", userKey)
	manifestJSON = strings.ReplaceAll(manifestJSON, "DB_PASS_KEY_PLACEHOLDER", passKey)
	manifestJSON = strings.ReplaceAll(manifestJSON, "DB_NAME_KEY_PLACEHOLDER", dbKey)
	manifestJSON = strings.ReplaceAll(manifestJSON, "DB_DATA_PATH_PLACEHOLDER", dataPath)
	manifestJSON = strings.ReplaceAll(manifestJSON, "IMAGE_PLACEHOLDER", image)

	result := &unstructured.Unstructured{}
	if err := result.UnmarshalJSON([]byte(manifestJSON)); err != nil {
		return nil, fmt.Errorf("unmarshal manifest: %w", err)
	}

	return result, nil
}

func ApplyDatabaseOverrides(obj *unstructured.Unstructured, dbConfig DatabaseConfig) error {
	jsonBytes, err := obj.MarshalJSON()
	if err != nil {
		return fmt.Errorf("marshal for database overrides: %w", err)
	}
	manifestJSON := string(jsonBytes)

	storageSize := dbConfig.StorageSize
	if storageSize == "" {
		storageSize = "5Gi"
	}
	dbImage := dbConfig.Image
	if dbImage == "" {
		dbImage = "postgres:18"
	}

	userKey, passKey, dbKey := postgresEnvKeys(dbImage)
	dataPath := postgresDataPath(dbImage)

	manifestJSON = strings.ReplaceAll(manifestJSON, "DB_STORAGE_PLACEHOLDER", storageSize)
	manifestJSON = strings.ReplaceAll(manifestJSON, "DB_IMAGE_PLACEHOLDER", dbImage)
	manifestJSON = strings.ReplaceAll(manifestJSON, "DB_USER_KEY_PLACEHOLDER", userKey)
	manifestJSON = strings.ReplaceAll(manifestJSON, "DB_PASS_KEY_PLACEHOLDER", passKey)
	manifestJSON = strings.ReplaceAll(manifestJSON, "DB_NAME_KEY_PLACEHOLDER", dbKey)
	manifestJSON = strings.ReplaceAll(manifestJSON, "DB_DATA_PATH_PLACEHOLDER", dataPath)

	if err := obj.UnmarshalJSON([]byte(manifestJSON)); err != nil {
		return fmt.Errorf("unmarshal after database overrides: %w", err)
	}

	return nil
}

func ApplyConfigOverrides(obj *unstructured.Unstructured, config GatewayConfig) error {
	kind := obj.GetKind()

	if kind == "ConfigMap" && obj.GetName() == "openshell-gateway-config" && len(config.ServerDnsNames) > 0 {
		data, found, err := unstructured.NestedMap(obj.Object, "data")
		if err != nil || !found {
			return fmt.Errorf("configmap data not found")
		}

		toml, ok := data["gateway.toml"].(string)
		if !ok {
			return fmt.Errorf("gateway.toml not found in configmap")
		}

		serverSans := "["
		for i, dns := range config.ServerDnsNames {
			if i > 0 {
				serverSans += ", "
			}
			serverSans += fmt.Sprintf("\"%s\"", dns)
		}
		serverSans += "]"

		lines := strings.Split(toml, "\n")
		for i, line := range lines {
			if strings.Contains(line, "server_sans =") {
				lines[i] = fmt.Sprintf("    server_sans = %s", serverSans)
				break
			}
		}

		if config.OIDC.Issuer != "" {
			for i, line := range lines {
				if strings.Contains(line, "allow_unauthenticated_users") {
					lines[i] = "    allow_unauthenticated_users = false"
					break
				}
			}

			oidcSection := "\n    [openshell.gateway.oidc]\n"
			oidcSection += fmt.Sprintf("    issuer      = \"%s\"\n", config.OIDC.Issuer)
			audience := config.OIDC.Audience
			if audience == "" {
				audience = "openshell-cli"
			}
			oidcSection += fmt.Sprintf("    audience    = \"%s\"\n", audience)
			jwksTTL := config.OIDC.JwksTTL
			if jwksTTL == 0 {
				jwksTTL = 3600
			}
			oidcSection += fmt.Sprintf("    jwks_ttl    = %d\n", jwksTTL)
			if config.OIDC.RolesClaim != "" {
				oidcSection += fmt.Sprintf("    roles_claim = \"%s\"\n", config.OIDC.RolesClaim)
			}
			if config.OIDC.AdminRole != "" {
				oidcSection += fmt.Sprintf("    admin_role  = \"%s\"\n", config.OIDC.AdminRole)
			}
			if config.OIDC.UserRole != "" {
				oidcSection += fmt.Sprintf("    user_role   = \"%s\"\n", config.OIDC.UserRole)
			}
			if config.OIDC.ScopesClaim != "" {
				oidcSection += fmt.Sprintf("    scopes_claim = \"%s\"\n", config.OIDC.ScopesClaim)
			}

			lines = append(lines, oidcSection)
		}

		data["gateway.toml"] = strings.Join(lines, "\n")

		if err := unstructured.SetNestedMap(obj.Object, data, "data"); err != nil {
			return fmt.Errorf("set configmap data: %w", err)
		}
	}

	if kind == "Job" && strings.Contains(obj.GetName(), "certgen") && len(config.ServerDnsNames) > 0 {
		containers, found, err := unstructured.NestedSlice(obj.Object, "spec", "template", "spec", "containers")
		if err != nil || !found {
			return nil
		}

		for i, container := range containers {
			containerMap, ok := container.(map[string]interface{})
			if !ok {
				continue
			}

			args, found, _ := unstructured.NestedStringSlice(containerMap, "args")
			if !found {
				continue
			}

			newArgs := []string{}
			for _, arg := range args {
				if !strings.HasPrefix(arg, "--server-san=") {
					newArgs = append(newArgs, arg)
				}
			}

			newArgs = append(newArgs, "--server-san=localhost")
			for _, dns := range config.ServerDnsNames {
				if dns != "localhost" {
					newArgs = append(newArgs, fmt.Sprintf("--server-san=%s", dns))
				}
			}

			if err := unstructured.SetNestedStringSlice(containerMap, newArgs, "args"); err != nil {
				return fmt.Errorf("set job args: %w", err)
			}

			containers[i] = containerMap
		}

		if err := unstructured.SetNestedSlice(obj.Object, containers, "spec", "template", "spec", "containers"); err != nil {
			return fmt.Errorf("set job containers: %w", err)
		}
	}

	return nil
}
