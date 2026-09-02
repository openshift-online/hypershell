package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestParseSpecProjectsScopedServiceAccountResource(t *testing.T) {
	specPath := filepath.Join("..", "..", "components", "api-server", "openapi", "openapi.yaml")
	spec, err := parseSpec(specPath, "/api/hypershell/v1")
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

func TestParseSpecExcludesSingletonMetadata(t *testing.T) {
	specPath := filepath.Join("..", "..", "components", "api-server", "openapi", "openapi.yaml")
	spec, err := parseSpec(specPath, "/api/hypershell/v1")
	if err != nil {
		t.Fatalf("parse spec: %v", err)
	}

	for _, resource := range spec.Resources {
		if resource.Name == "ServiceMetadata" {
			t.Fatal("singleton metadata must not become a CRUD SDK resource")
		}
	}
}
