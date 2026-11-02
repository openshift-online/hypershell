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
	// Scoped paths must be absolute, matching the server routes and the
	// non-scoped clients; the base HTTP clients add no prefix. Asserting the
	// full /api/hypershell prefix (not just the suffix) guards against a
	// relative-path regression like the one in #447.
	if !strings.Contains(resource.GoCollectionPath, "/api/hypershell/v1/gateways/%s/service_accounts") {
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
	// The in-place role action must project as a request-carrying action with
	// the shared item response type, distinct from the bodyless revoke action.
	if !resource.HasRoleUpdate || resource.RoleUpdateRequestType != "OpenShellGatewayServiceAccountRoleUpdateRequest" {
		t.Fatalf("role action was not projected: HasRoleUpdate=%v request=%q", resource.HasRoleUpdate, resource.RoleUpdateRequestType)
	}
	if len(resource.Actions) != 2 || resource.Actions[0] != "revoke" || resource.Actions[1] != "role" {
		t.Fatalf("actions = %#v, want [revoke role]", resource.Actions)
	}
	roleRequest, ok := modelByName(resource.Models, "OpenShellGatewayServiceAccountRoleUpdateRequest")
	if !ok {
		t.Fatal("role update request model was not projected")
	}
	for _, field := range roleRequest.Fields {
		if field.Name == "role" && !field.Required {
			t.Fatal("role update request role field must be required")
		}
	}
	foundImport := false
	for _, name := range resource.TSImports {
		if name == "OpenShellGatewayServiceAccountRoleUpdateRequest" {
			foundImport = true
		}
	}
	if !foundImport {
		t.Fatal("role update request type missing from TS imports")
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

func modelByName(models []Model, name string) (Model, bool) {
	for _, model := range models {
		if model.Name == name {
			return model, true
		}
	}
	return Model{}, false
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
		responseFields := make(map[string]Field, len(resource.Fields))
		for _, field := range resource.Fields {
			responseFields[field.Name] = field
		}
		if responseFields["generation"].Name == "" || responseFields["observed_generation"].Name == "" {
			t.Fatal("gateway response fields must retain generation and observed_generation")
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

func TestParseSpecProjectsGatewayProvisioningConditionsAsArray(t *testing.T) {
	specPath := filepath.Join("..", "..", "components", "api-server", "openapi", "openapi.yaml")
	spec, err := parseSpec(specPath, "/api/hypershell")
	if err != nil {
		t.Fatalf("parse spec: %v", err)
	}
	for _, resource := range spec.Resources {
		if resource.Name != "Gateway" {
			continue
		}
		for _, field := range resource.Fields {
			if field.Name != "provisioning_conditions" {
				continue
			}
			if field.Type != "array" || field.GoType != "[]map[string]any" || field.TSType != "Record<string, unknown>[]" {
				t.Fatalf("provisioning_conditions = type=%q go=%q ts=%q, want array []map[string]any Record<string, unknown>[]", field.Type, field.GoType, field.TSType)
			}
			return
		}
		t.Fatal("Gateway provisioning_conditions field is missing")
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

func TestParseSpecProjectsGatewayOwnershipFilterAndExistenceFlag(t *testing.T) {
	specPath := filepath.Join("..", "..", "components", "api-server", "openapi", "openapi.yaml")
	spec, err := parseSpec(specPath, "/api/hypershell")
	if err != nil {
		t.Fatalf("parse spec: %v", err)
	}

	// The custom `mine` list query parameter must project onto the shared
	// ListOptions union (deduped across primary collections) so it is generated
	// rather than added to the SDK by hand.
	var mine *Field
	for index := range spec.ExtraListParameters {
		if spec.ExtraListParameters[index].Name == "mine" {
			mine = &spec.ExtraListParameters[index]
			break
		}
	}
	if mine == nil {
		t.Fatalf("mine not projected as an extra list parameter: %#v", spec.ExtraListParameters)
	}
	if mine.TSType != "boolean" || mine.TSName != "mine" {
		t.Fatalf("mine parameter = TSType %q TSName %q, want boolean/mine", mine.TSType, mine.TSName)
	}
	// Scope-specific list filters (e.g. the access directory's sort/order/role)
	// must not leak into the shared ListOptions union.
	for _, parameter := range spec.ExtraListParameters {
		switch parameter.Name {
		case "sort", "order", "role", "status":
			t.Fatalf("scoped list parameter %q leaked into shared ListOptions", parameter.Name)
		}
	}

	// The browser-safe existence flag must project as an optional extra field on
	// GatewayList, outside the shared list envelope.
	var gateway *Resource
	for index := range spec.Resources {
		if spec.Resources[index].Name == "Gateway" {
			gateway = &spec.Resources[index]
			break
		}
	}
	if gateway == nil {
		t.Fatal("gateway resource is missing")
	}
	var flag *Field
	for index := range gateway.ListExtraFields {
		if gateway.ListExtraFields[index].Name == "has_other_owned_gateways" {
			flag = &gateway.ListExtraFields[index]
			break
		}
	}
	if flag == nil {
		t.Fatalf("has_other_owned_gateways not projected as a list extra field: %#v", gateway.ListExtraFields)
	}
	if flag.Required || flag.TSType != "boolean" {
		t.Fatalf("has_other_owned_gateways = required %v TSType %q, want optional boolean", flag.Required, flag.TSType)
	}

	outDir := t.TempDir()
	header := GeneratedHeader{SpecPath: specPath, SpecHash: "test"}
	tsOut := filepath.Join(outDir, "typescript")
	if err := generateTypeScript(spec, tsOut, header); err != nil {
		t.Fatalf("generate TypeScript SDK: %v", err)
	}
	base, err := os.ReadFile(filepath.Join(tsOut, "src", "base.ts"))
	if err != nil {
		t.Fatalf("read generated base.ts: %v", err)
	}
	if !strings.Contains(string(base), "mine?: boolean;") {
		t.Fatal("generated base.ts ListOptions is missing the mine filter")
	}
	if !strings.Contains(string(base), "if (opts.mine) params.set('mine', 'true');") {
		t.Fatal("generated base.ts buildQueryString does not serialize the mine filter")
	}
	tsGateway, err := os.ReadFile(filepath.Join(tsOut, "src", "gateway.ts"))
	if err != nil {
		t.Fatalf("read generated gateway.ts: %v", err)
	}
	if !strings.Contains(string(tsGateway), "has_other_owned_gateways?: boolean;") {
		t.Fatal("generated gateway.ts GatewayList is missing the has_other_owned_gateways flag")
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
