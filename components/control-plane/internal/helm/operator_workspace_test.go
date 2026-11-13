package helm

import (
	"strings"
	"testing"
)

// Verifies: OSG-OPWS-08.01, OSG-OPWS-08.02
func TestOperatorWorkspaceConfiguration(t *testing.T) {
	valid := `{"namespaceLabelKey":"example.org/workspace","allowedPodLabels":["example.org/*","app.kubernetes.io/name"]}`
	for _, raw := range []string{
		`{}`, `null`, valid + `{}`, strings.Replace(valid, "example.org/workspace", "*", 1),
		strings.Replace(valid, "example.org/*", "*", 1),
		strings.Replace(valid, "namespaceLabelKey", "namespaceLabels", 1),
		strings.Replace(valid, "{", `{"allowDriverConfig":"true",`, 1),
	} {
		if _, err := ParseOperatorWorkspace(raw); err == nil {
			t.Errorf("accepted invalid operator configuration %s", raw)
		}
	}
	config, err := ParseOperatorWorkspace(valid)
	if err != nil {
		t.Fatal(err)
	}
	optIn, err := ParseOperatorWorkspace(strings.Replace(valid, "{", `{"allowDriverConfig":true,"disableResourceAdmission":true,"mtlsAuth":true,`, 1))
	if err != nil {
		t.Fatal(err)
	}
	if !optIn.AllowDriverConfig || !optIn.DisableResourceAdmission || !optIn.MTLSAuth {
		t.Fatal("explicit administrator permissions were not parsed")
	}
	builder := ValuesBuilder{
		Gateway: GatewayConfig{
			WorkspaceMode: "operator",
			OIDC:          OIDCConfig{Issuer: "https://identity.example.org"},
			Route:         RouteConfig{},
		},
		Namespace:                 "openshell-deadbeefdeadbeef",
		OperatorWorkspace:         config,
		OperatorNamespaceLabelKey: config.NamespaceLabelKey,
	}
	values, err := builder.Build()
	if err != nil {
		t.Fatal(err)
	}
	// Gateway API must retain bearer-only OIDC while direct operator RPCs can
	// authenticate with a client certificate, without losing the configured CA.
	server := values["server"].(map[string]interface{})
	if server["tls"].(map[string]interface{})["enableMtls"] != true {
		t.Fatal("operator client CA was disabled by Gateway API routing")
	}
	driver := server["drivers"].(map[string]interface{})["kubernetes"].(map[string]interface{})
	if driver["operatorNamespaceLabel"] != config.NamespaceLabelKey+"="+builder.Namespace || driver["allowDriverConfig"] != false {
		t.Fatal("trusted operator namespace or driver configuration was not propagated")
	}
	if driver["resourceAdmission"].(map[string]interface{})["enabled"] != true ||
		values["gatewayConfig"].(map[string]interface{})["openshell.gateway.mtls_auth"].(map[string]interface{})["enabled"] != false {
		t.Fatal("operator defaults must retain resource approval and disable client-certificate authentication")
	}
	builder.OperatorWorkspace = nil
	if _, err := builder.Build(); err == nil {
		t.Fatal("operator mode without an explicit trust configuration must fail closed")
	}
}
