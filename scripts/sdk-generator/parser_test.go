package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseSpecProjectsScopedServiceAccountResource(t *testing.T) {
	specPath := filepath.Join("..", "..", "components", "api-server", "openapi", "openapi.yaml")
	spec, err := parseSpec(specPath, "/api/hypershell")
	if err != nil {
		t.Fatalf("parse spec: %v", err)
	}

	var resource *Resource
	for index := range spec.Resources {
		if spec.Resources[index].Name == "OpenShellGatewayServiceAccount" {
			resource = &spec.Resources[index]
			break
		}
	}
	if resource == nil {
		t.Fatal("scoped service-account resource was not projected")
	}
	if !resource.Scoped {
		t.Fatal("service-account resource must be marked scoped")
	}
	if len(resource.ScopeParameters) != 1 || resource.ScopeParameters[0].Name != "gateway_id" {
		t.Fatalf("scope parameters = %#v, want gateway_id", resource.ScopeParameters)
	}
	if !strings.Contains(resource.GoCollectionPath, "/gateways/%s/service_accounts") {
		t.Fatalf("Go collection path = %q", resource.GoCollectionPath)
	}
	if !strings.Contains(resource.TSItemPath, "encodeURIComponent(serviceAccountId)") {
		t.Fatalf("TypeScript item path = %q", resource.TSItemPath)
	}
	if resource.CreateRequestType != "OpenShellGatewayServiceAccountCreateRequest" ||
		resource.CreateResponseType != "OpenShellGatewayServiceAccountCreateResponse" ||
		resource.GetResponseType != "OpenShellGatewayServiceAccountGetResponse" {
		t.Fatalf("operation-specific schemas were not preserved: %#v", resource)
	}

	models := make(map[string]Model, len(resource.Models))
	for _, model := range resource.Models {
		models[model.Name] = model
	}
	credential := models["OpenShellGatewayServiceAccountCredential"]
	if !credential.ContainsSensitive {
		t.Fatal("credential model must be marked sensitive")
	}
	foundSecret := false
	for _, field := range credential.Fields {
		if field.Name == "client_secret" {
			foundSecret = field.Sensitive && field.GoType == "string" && field.TSType == "string"
		}
	}
	if !foundSecret {
		t.Fatal("client_secret must remain a sensitive string field")
	}
	if !models["OpenShellGatewayServiceAccountCreateResponse"].ContainsSensitive {
		t.Fatal("create response must inherit sensitive-model redaction")
	}
	for _, modelName := range []string{"OpenShellGatewayServiceAccountListItem", "OpenShellGatewayServiceAccountGetResponse"} {
		for _, field := range models[modelName].Fields {
			if field.Name == "client_secret" || field.Name == "credential" {
				t.Fatalf("%s unexpectedly exposes %s", modelName, field.Name)
			}
		}
	}
}

func TestParseSpecProjectsGatewayCreateRequestFromOperationSchema(t *testing.T) {
	specPath := filepath.Join("..", "..", "components", "api-server", "openapi", "openapi.yaml")
	spec, err := parseSpec(specPath, "/api/hypershell")
	if err != nil {
		t.Fatalf("parse spec: %v", err)
	}
	for _, resource := range spec.Resources {
		if resource.Name != "Gateway" {
			continue
		}
		if len(resource.CreateFields) == 0 {
			t.Fatal("gateway create fields are missing")
		}
		fields := make(map[string]Field, len(resource.CreateFields))
		for _, field := range resource.CreateFields {
			fields[field.Name] = field
		}
		if fields["cluster_id"].Name != "" {
			t.Fatal("cluster_id must not be in the create request")
		}
		if !fields["name"].Required || !fields["placement"].Required {
			t.Fatal("name and placement must be required")
		}
		if fields["placement"].TSType != "GatewayPlacementIntent" {
			t.Fatalf("placement type = %q", fields["placement"].TSType)
		}
		return
	}
	t.Fatal("gateway resource is missing")
}

func TestGeneratedGatewayBuildersValidateRequiredPlacement(t *testing.T) {
	specPath := filepath.Join("..", "..", "components", "api-server", "openapi", "openapi.yaml")
	spec, err := parseSpec(specPath, "/api/hypershell")
	if err != nil {
		t.Fatalf("parse spec: %v", err)
	}

	outDir := t.TempDir()
	header := GeneratedHeader{SpecPath: specPath, SpecHash: "test"}
	goOut := filepath.Join(outDir, "go")
	if err := generateGo(spec, goOut, header); err != nil {
		t.Fatalf("generate Go SDK: %v", err)
	}
	goGateway, err := os.ReadFile(filepath.Join(goOut, "types", "gateway.go"))
	if err != nil {
		t.Fatalf("read generated Go gateway: %v", err)
	}
	if !strings.Contains(string(goGateway), "if !b.placementSet") {
		t.Fatal("generated Go gateway builder does not validate required placement")
	}

	tsOut := filepath.Join(outDir, "typescript")
	if err := generateTypeScript(spec, tsOut, header); err != nil {
		t.Fatalf("generate TypeScript SDK: %v", err)
	}
	tsGateway, err := os.ReadFile(filepath.Join(tsOut, "src", "gateway.ts"))
	if err != nil {
		t.Fatalf("read generated TypeScript gateway: %v", err)
	}
	if !strings.Contains(string(tsGateway), "this.data['placement'] === undefined") {
		t.Fatal("generated TypeScript gateway builder does not validate required placement")
	}
}

func TestParseSpecPreservesWritableCreateFieldsWithoutPostOperation(t *testing.T) {
	specPath := filepath.Join("..", "..", "components", "api-server", "openapi", "openapi.yaml")
	spec, err := parseSpec(specPath, "/api/hypershell")
	if err != nil {
		t.Fatalf("parse spec: %v", err)
	}
	for _, resource := range spec.Resources {
		if resource.Name != "Role" {
			continue
		}
		fields := make(map[string]Field, len(resource.CreateFields))
		for _, field := range resource.CreateFields {
			fields[field.Name] = field
		}
		if !fields["name"].Required || fields["description"].Name == "" {
			t.Fatalf("role create fields were not preserved: %#v", fields)
		}
		return
	}
	t.Fatal("role resource is missing")
}
