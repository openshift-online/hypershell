package e2e

import "testing"

func TestGatewayTLSInsecure(t *testing.T) {
	t.Setenv("OPENSHELL_GATEWAY_INSECURE", "")
	if !gatewayTLSInsecure("kind") {
		t.Error("Kind gateway TLS must remain insecure for its local self-signed CA")
	}
	if gatewayTLSInsecure("openshift") {
		t.Error("OpenShift Gateway API endpoints must verify their public TLS certificate")
	}

	t.Setenv("OPENSHELL_GATEWAY_INSECURE", "true")
	if !gatewayTLSInsecure("openshift") {
		t.Error("an explicit OPENSHELL_GATEWAY_INSECURE=true override must be honored")
	}
}

func TestUsesSingleServiceAccountIdentity(t *testing.T) {
	t.Setenv("E2E_OIDC_GRANT", "")
	if usesSingleServiceAccountIdentity() {
		t.Error("password grant must use the distinct seeded test identities")
	}

	t.Setenv("E2E_OIDC_GRANT", "client_credentials")
	if !usesSingleServiceAccountIdentity() {
		t.Error("client_credentials must be recognized as one service-account identity")
	}
}
