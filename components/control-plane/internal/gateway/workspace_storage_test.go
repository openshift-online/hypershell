package gateway

import (
	"os"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/yaml"
)

func TestWorkspaceStorageRendering(t *testing.T) {
	for _, test := range []struct {
		name, class, size string
		wantError         bool
	}{
		{name: "defaults"},
		{name: "class only", class: "sandbox-gp3"},
		{name: "size only", size: "5Gi"},
		{name: "class and size", class: "sandbox-gp3", size: "5Gi"},
		{name: "reject invalid class", class: "bad/class", wantError: true},
		{name: "reject invalid size", size: "0Gi", wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("GATEWAY_WORKSPACE_STORAGE_CLASS", test.class)
			t.Setenv("GATEWAY_WORKSPACE_DEFAULT_STORAGE_SIZE", test.size)
			data, err := os.ReadFile("../../manifests/gateway/configmap.yaml")
			if err != nil {
				t.Fatal(err)
			}
			obj := &unstructured.Unstructured{}
			if err := yaml.NewYAMLOrJSONDecoder(strings.NewReader(string(data)), 4096).Decode(obj); err != nil {
				t.Fatal(err)
			}
			config := GatewayConfig{CredentialDriver: &CredentialDriverConfig{Type: "kubernetes-secrets"}}
			config.OIDC.Issuer = "https://issuer.example"
			if err := ApplyConfigOverrides(obj, config, "tenant"); err != nil {
				if test.wantError {
					return
				}
				t.Fatal(err)
			} else if test.wantError {
				t.Fatal("expected invalid storage settings to fail rendering")
			}
			toml, _, err := unstructured.NestedString(obj.Object, "data", "gateway.toml")
			if err != nil {
				t.Fatal(err)
			}
			table := ""
			values := map[string]string{}
			for _, line := range strings.Split(toml, "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "[") {
					table = line
				}
				key, value, ok := strings.Cut(line, " = ")
				if ok && (key == "workspace_storage_class" || key == "workspace_default_storage_size") {
					if table != "[openshell.drivers.kubernetes]" {
						t.Fatalf("%s rendered in wrong table %s", key, table)
					}
					if _, duplicate := values[key]; duplicate {
						t.Fatalf("duplicate storage key %s", key)
					}
					values[key] = strings.Trim(value, "\"")
				}
			}
			if values["workspace_storage_class"] != test.class || values["workspace_default_storage_size"] != test.size {
				t.Fatalf("unexpected storage configuration: %v", values)
			}
			if test.class == "" && test.size == "" && len(values) != 0 {
				t.Fatalf("default settings must omit storage keys: %v", values)
			}
		})
	}
}
