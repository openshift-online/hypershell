package helm

import (
	"strings"
	"testing"
)

func TestOperatorWorkspaceConfiguration(t *testing.T) {
	valid := `{"namespaceLabelKey":"example.org/workspace","allowedPodLabels":["example.org/*","app.kubernetes.io/name"]}`
	for _, raw := range []string{
		`{}`, `null`, valid + `{}`, strings.Replace(valid, "example.org/workspace", "*", 1),
		strings.Replace(valid, "example.org/*", "*", 1),
		strings.Replace(valid, "namespaceLabelKey", "namespaceLabels", 1),
	} {
		if _, err := ParseOperatorWorkspace(raw); err == nil {
			t.Errorf("accepted invalid operator configuration %s", raw)
		}
	}
	config, err := ParseOperatorWorkspace(valid)
	if err != nil {
		t.Fatal(err)
	}
	builder := ValuesBuilder{
		Gateway: GatewayConfig{
			WorkspaceMode: "operator",
			OIDC:          OIDCConfig{Issuer: "https://identity.example.org"},
			Route:         RouteConfig{Enabled: true},
		},
		Namespace:         "openshell-deadbeefdeadbeef",
		OperatorWorkspace: config,
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
	if driver["operatorNamespaceLabel"] != config.NamespaceLabelKey+"="+builder.Namespace || driver["allowDriverConfig"] != true {
		t.Fatal("trusted operator namespace or driver configuration was not propagated")
	}
	builder.OperatorWorkspace = nil
	if _, err := builder.Build(); err == nil {
		t.Fatal("operator mode without an explicit trust configuration must fail closed")
	}
}
