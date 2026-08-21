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

	requiredFiles := []string{"serviceaccount.yaml", "configmap.yaml", "service.yaml", "rbac.yaml", "deployment.yaml"}
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

	// Replace SANDBOX_IMAGE_PLACEHOLDER before IMAGE_PLACEHOLDER because the
	// shorter string is a substring of the longer one.
	manifestJSON = strings.ReplaceAll(manifestJSON, "SANDBOX_IMAGE_PLACEHOLDER", images.DefaultSandboxImage())

	image := images.DefaultGatewayImage()
	if config.Image != "" {
		image = config.Image
	}
	manifestJSON = strings.ReplaceAll(manifestJSON, "IMAGE_PLACEHOLDER", image)

	result := &unstructured.Unstructured{}
	if err := result.UnmarshalJSON([]byte(manifestJSON)); err != nil {
		return nil, fmt.Errorf("unmarshal manifest: %w", err)
	}

	return result, nil
}

func ApplyConfigOverrides(obj *unstructured.Unstructured, config GatewayConfig, tenantNamespace ...string) error {
	kind := obj.GetKind()

	if kind == "ConfigMap" && obj.GetName() == "openshell-gateway-config" && (len(config.ServerDnsNames) > 0 || config.CredentialDriver != nil) {
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
			oidcSection += fmt.Sprintf("    jwks_ttl_secs = %d\n", jwksTTL)
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

		if config.CredentialDriver != nil {
			ns := ""
			if len(tenantNamespace) > 0 {
				ns = tenantNamespace[0]
			}
			lines = applyCredentialDriverToml(lines, config.CredentialDriver, ns)
		}

		data["gateway.toml"] = strings.Join(lines, "\n")

		if err := unstructured.SetNestedMap(obj.Object, data, "data"); err != nil {
			return fmt.Errorf("set configmap data: %w", err)
		}
	}

	// The certgen job runs with --jwt-only and provisions only the sandbox-JWT
	// signing keys; cert-manager owns the server TLS Secret and its SANs, so no
	// --server-san injection is needed here.

	if kind == "Deployment" && obj.GetName() == "openshell-gateway" && config.CredentialDriver != nil {
		if err := applyCredentialDriverDeploymentOverrides(obj, config.CredentialDriver); err != nil {
			return fmt.Errorf("apply credential driver deployment overrides: %w", err)
		}
	}

	return nil
}

func tomlEscapeString(s string) string {
	r := strings.NewReplacer(
		`\`, `\\`,
		`"`, `\"`,
		"\n", `\n`,
		"\r", `\r`,
		"\t", `\t`,
	)
	return r.Replace(s)
}

func applyCredentialDriverToml(lines []string, driver *CredentialDriverConfig, tenantNamespace string) []string {
	var result []string
	skip := false
	for _, line := range lines {
		if strings.Contains(line, "[openshell.gateway.credential_storage]") {
			skip = true
			continue
		}
		if skip {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" || strings.HasPrefix(trimmed, "[") {
				skip = false
			} else {
				continue
			}
		}
		if !skip {
			result = append(result, line)
		}
	}

	switch driver.Type {
	case "kubernetes-secrets":
		ns := tenantNamespace
		if driver.KubernetesSecrets != nil && driver.KubernetesSecrets.Namespace != "" {
			ns = driver.KubernetesSecrets.Namespace
		}
		result = append(result, "")
		result = append(result, "    credential_drivers = [\"kubernetes-secrets\"]")
		result = append(result, "")
		result = append(result, "    [openshell.credential_drivers.kubernetes-secrets]")
		if ns != "" {
			result = append(result, fmt.Sprintf("    namespace = \"%s\"", tomlEscapeString(ns)))
		}
	case "vault":
		v := driver.Vault
		mount := v.Mount
		if mount == "" {
			mount = "secret"
		}
		authMethod := v.AuthMethod
		if authMethod == "" {
			authMethod = "kubernetes"
		}
		kubeAuthMount := v.KubernetesAuthMount
		if kubeAuthMount == "" {
			kubeAuthMount = "kubernetes"
		}
		timeoutSecs := v.TimeoutSecs
		if timeoutSecs == 0 {
			timeoutSecs = 30
		}
		result = append(result, "")
		result = append(result, "    credential_drivers = [\"vault\"]")
		result = append(result, "")
		result = append(result, "    [openshell.credential_drivers.vault]")
		result = append(result, fmt.Sprintf("    address = \"%s\"", tomlEscapeString(v.Address)))
		result = append(result, fmt.Sprintf("    mount = \"%s\"", tomlEscapeString(mount)))
		result = append(result, fmt.Sprintf("    auth_method = \"%s\"", tomlEscapeString(authMethod)))
		result = append(result, fmt.Sprintf("    role = \"%s\"", tomlEscapeString(v.Role)))
		result = append(result, fmt.Sprintf("    kubernetes_auth_mount = \"%s\"", tomlEscapeString(kubeAuthMount)))
		result = append(result, fmt.Sprintf("    timeout_secs = %d", timeoutSecs))
		if authMethod == "kubernetes" {
			result = append(result, "    service_account_token_path = \"/var/run/secrets/vault/token\"")
		}
	}

	return result
}

func applyCredentialDriverDeploymentOverrides(obj *unstructured.Unstructured, driver *CredentialDriverConfig) error {
	containers, found, err := unstructured.NestedSlice(obj.Object, "spec", "template", "spec", "containers")
	if err != nil || !found {
		return nil
	}

	for i, container := range containers {
		containerMap, ok := container.(map[string]interface{})
		if !ok {
			continue
		}
		name, _, _ := unstructured.NestedString(containerMap, "name")
		if name != "openshell-gateway" {
			continue
		}

		envList, _, _ := unstructured.NestedSlice(containerMap, "env")
		var filteredEnv []interface{}
		for _, e := range envList {
			em, ok := e.(map[string]interface{})
			if !ok {
				filteredEnv = append(filteredEnv, e)
				continue
			}
			if em["name"] == "OPENSHELL_GATEWAY_CREDENTIAL_KEY_ENCRYPTION_KEY" {
				continue
			}
			filteredEnv = append(filteredEnv, e)
		}
		if err := unstructured.SetNestedSlice(containerMap, filteredEnv, "env"); err != nil {
			return fmt.Errorf("set env: %w", err)
		}

		if driver.Type == "vault" && (driver.Vault == nil || driver.Vault.AuthMethod == "" || driver.Vault.AuthMethod == "kubernetes") {
			mounts, _, _ := unstructured.NestedSlice(containerMap, "volumeMounts")
			mounts = append(mounts, map[string]interface{}{
				"name":      "vault-sa-token",
				"mountPath": "/var/run/secrets/vault",
				"readOnly":  true,
			})
			if err := unstructured.SetNestedSlice(containerMap, mounts, "volumeMounts"); err != nil {
				return fmt.Errorf("set volumeMounts: %w", err)
			}
		}

		containers[i] = containerMap
	}

	if err := unstructured.SetNestedSlice(obj.Object, containers, "spec", "template", "spec", "containers"); err != nil {
		return fmt.Errorf("set containers: %w", err)
	}

	if driver.Type == "vault" && (driver.Vault == nil || driver.Vault.AuthMethod == "" || driver.Vault.AuthMethod == "kubernetes") {
		volumes, _, _ := unstructured.NestedSlice(obj.Object, "spec", "template", "spec", "volumes")
		volumes = append(volumes, map[string]interface{}{
			"name": "vault-sa-token",
			"projected": map[string]interface{}{
				"sources": []interface{}{
					map[string]interface{}{
						"serviceAccountToken": map[string]interface{}{
							"path":              "token",
							"expirationSeconds": int64(3600),
							"audience":          "vault",
						},
					},
				},
			},
		})
		if err := unstructured.SetNestedSlice(obj.Object, volumes, "spec", "template", "spec", "volumes"); err != nil {
			return fmt.Errorf("set volumes: %w", err)
		}
	}

	return nil
}
