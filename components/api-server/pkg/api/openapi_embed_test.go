package api

import (
	"testing"

	"gopkg.in/yaml.v3"
)

type openAPIOperation struct {
	OperationID string `yaml:"operationId"`
	Responses   map[string]openAPIResponse
	Security    *[]map[string][]string `yaml:"security"`
}

type openAPIResponse struct {
	Content map[string]openAPIContent
}

type openAPIContent struct {
	Schema openAPISchemaReference
}

type openAPISchemaReference struct {
	Reference string `yaml:"$ref"`
}

type openAPISchema struct {
	Required   []string
	Properties map[string]struct {
		Type string
	}
}

type openAPIPathItem struct {
	Get    *openAPIOperation `yaml:"get"`
	Post   *openAPIOperation `yaml:"post"`
	Patch  *openAPIOperation `yaml:"patch"`
	Delete *openAPIOperation `yaml:"delete"`
}

func TestGetOpenAPISpecReturnsCompleteContract(t *testing.T) {
	data, err := GetOpenAPISpec()
	if err != nil {
		t.Fatalf("read embedded OpenAPI document: %v", err)
	}

	var spec struct {
		OpenAPI    string                     `yaml:"openapi"`
		Paths      map[string]openAPIPathItem `yaml:"paths"`
		Components struct {
			Schemas map[string]openAPISchema
		}
	}
	if err := yaml.Unmarshal(data, &spec); err != nil {
		t.Fatalf("parse embedded OpenAPI document: %v", err)
	}
	if spec.OpenAPI != "3.0.3" {
		t.Fatalf("OpenAPI version = %q, want 3.0.3", spec.OpenAPI)
	}

	operationIDs := make(map[string]string)
	operationCount := 0
	for path, item := range spec.Paths {
		operations := map[string]*openAPIOperation{
			"GET": item.Get, "POST": item.Post, "PATCH": item.Patch, "DELETE": item.Delete,
		}
		for method, operation := range operations {
			if operation == nil {
				continue
			}
			operationCount++
			if operation.OperationID == "" {
				t.Errorf("%s %s has no operationId", method, path)
				continue
			}
			if previous, exists := operationIDs[operation.OperationID]; exists {
				t.Errorf("operationId %q is used by both %s and %s %s", operation.OperationID, previous, method, path)
			}
			operationIDs[operation.OperationID] = method + " " + path
		}
	}
	if operationCount != 37 {
		t.Fatalf("embedded operation count = %d, want 37", operationCount)
	}

	expectedDeletes := map[string]string{
		"/api/hypershell/v1/gateway_networks/{id}":                                       "deleteGatewayNetwork",
		"/api/hypershell/v1/gateway_releases/{id}":                                       "deleteGatewayRelease",
		"/api/hypershell/v1/gateways/{id}":                                               "deleteGateway",
		"/api/hypershell/v1/managed_clusters/{id}":                                       "deleteManagedCluster",
		"/api/hypershell/v1/managed_databases/{id}":                                      "deleteManagedDatabase",
		"/api/hypershell/v1/role_bindings/{id}":                                          "deleteRoleBinding",
		"/api/hypershell/v1/gateways/{gateway_id}/service_accounts/{service_account_id}": "deleteGatewayServiceAccount",
	}
	for path, expectedOperationID := range expectedDeletes {
		operation := spec.Paths[path].Delete
		if operation == nil {
			t.Errorf("DELETE %s is missing", path)
			continue
		}
		if operation.OperationID != expectedOperationID {
			t.Errorf("DELETE %s operationId = %q, want %q", path, operation.OperationID, expectedOperationID)
		}
	}

	metadataOperation := spec.Paths["/api/hypershell/v1/metadata"].Get
	if metadataOperation == nil {
		t.Fatal("GET metadata operation is missing")
	}
	if metadataOperation.Security == nil || len(*metadataOperation.Security) != 0 {
		t.Error("GET metadata must have an explicit empty security requirement")
	}
	response, exists := metadataOperation.Responses["200"]
	if !exists {
		t.Fatal("GET metadata has no 200 response")
	}
	if got := response.Content["application/json"].Schema.Reference; got != "#/components/schemas/ServiceMetadata" {
		t.Errorf("GET metadata response schema = %q", got)
	}

	metadataSchema, exists := spec.Components.Schemas["ServiceMetadata"]
	if !exists {
		t.Fatal("ServiceMetadata schema is missing")
	}
	required := make(map[string]bool, len(metadataSchema.Required))
	for _, name := range metadataSchema.Required {
		required[name] = true
	}
	for _, name := range []string{"id", "href", "kind", "version", "build_time"} {
		if !required[name] {
			t.Errorf("ServiceMetadata does not require %q", name)
		}
		if property, exists := metadataSchema.Properties[name]; !exists || property.Type != "string" {
			t.Errorf("ServiceMetadata property %q must be a string", name)
		}
	}
}
