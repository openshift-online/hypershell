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

func TestUsesGatewayMatchedCLIImage(t *testing.T) {
	for _, tt := range []struct {
		name, driverName, bin string
		want                  bool
	}{
		{name: "OpenShift default", driverName: "openshift", want: true},
		{name: "OpenShift generic binary", driverName: "openshift", bin: "openshell", want: true},
		{name: "OpenShift explicit override", driverName: "openshift", bin: "/usr/local/bin/openshell", want: false},
		{name: "Kind", driverName: "kind", want: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := usesGatewayMatchedCLIImage(tt.driverName, tt.bin); got != tt.want {
				t.Errorf("usesGatewayMatchedCLIImage(%q, %q) = %t, want %t", tt.driverName, tt.bin, got, tt.want)
			}
		})
	}
}
