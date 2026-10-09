package e2e

import "testing"

func TestGatewayTLSInsecure(t *testing.T) {
	t.Setenv("OPENSHELL_GATEWAY_INSECURE", "")
	if gatewayTLSInsecure("kind") {
		t.Error("Kind must trust its installed local CA instead of disabling TLS verification")
	}
	if gatewayTLSInsecure("openshift") {
		t.Error("OpenShift Gateway API endpoints must verify their public TLS certificate")
	}

	t.Setenv("OPENSHELL_GATEWAY_INSECURE", "true")
	if !gatewayTLSInsecure("openshift") {
		t.Error("an explicit OPENSHELL_GATEWAY_INSECURE=true override must be honored")
	}
}

func TestValidE2EMode(t *testing.T) {
	for _, mode := range []string{modeShort, modeLong} {
		if !validE2EMode(mode) {
			t.Errorf("validE2EMode(%q) = false", mode)
		}
	}
	for _, mode := range []string{"", modePerf, "unexpected"} {
		if validE2EMode(mode) {
			t.Errorf("validE2EMode(%q) = true", mode)
		}
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
