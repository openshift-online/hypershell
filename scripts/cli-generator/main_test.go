package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHasGatewayServiceAccountsRequiresListAndCreate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "openapi.yaml")
	spec := `openapi: 3.0.3
info:
  title: test
  version: 1.0.0
paths:
  /api/hypershell/v1/gateways/{gateway_id}/service_accounts:
    parameters:
      - in: path
        name: gateway_id
        required: true
        schema:
          type: string
    get:
      operationId: listGatewayServiceAccounts
      responses:
        "200":
          description: ok
    post:
      operationId: createGatewayServiceAccount
      responses:
        "201":
          description: created
`
	if err := os.WriteFile(path, []byte(spec), 0600); err != nil {
		t.Fatal(err)
	}
	if !hasGatewayServiceAccounts(path, "/api/hypershell/v1") {
		t.Fatal("expected the nested GET/POST collection to be detected")
	}
}
