package api

import (
	"testing"

	"gopkg.in/yaml.v3"
)

type openAPIOperation struct {
	OperationID string `yaml:"operationId"`
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
		OpenAPI string                     `yaml:"openapi"`
		Paths   map[string]openAPIPathItem `yaml:"paths"`
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
	if operationCount != 31 {
		t.Fatalf("embedded operation count = %d, want 31", operationCount)
	}

	expectedDeletes := map[string]string{
		"/api/hypershell/v1/fleets/{id}":            "deleteFleet",
		"/api/hypershell/v1/gateway_networks/{id}":  "deleteGatewayNetwork",
		"/api/hypershell/v1/gateway_releases/{id}":  "deleteGatewayRelease",
		"/api/hypershell/v1/gateways/{id}":          "deleteGateway",
		"/api/hypershell/v1/managed_clusters/{id}":  "deleteManagedCluster",
		"/api/hypershell/v1/managed_databases/{id}": "deleteManagedDatabase",
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
}
