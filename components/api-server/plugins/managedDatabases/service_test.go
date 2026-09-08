package managedDatabases

import (
	"strings"
	"testing"
)

// connection_secret names the NAMESPACE holding the fixed-name admin
// credentials Secret, not a Secret name. The reserved prefix plus the fixed
// Secret name are a security boundary: together they bound what the control
// plane can be made to read. See
// specs/platform/openshell-gateway-database-external.spec.md.
func TestValidateExternalConnectionSecret(t *testing.T) {
	ptr := func(s string) *string { return &s }

	cases := []struct {
		name    string
		input   *string
		wantErr bool
	}{
		{name: "nil", input: nil, wantErr: true},
		{name: "empty", input: ptr(""), wantErr: true},
		{name: "namespace/name form rejected", input: ptr("hypershell/hypershell-managed-db-credentials"), wantErr: true},
		{name: "slash anywhere rejected", input: ptr("hypershell-managed-db-a/b"), wantErr: true},
		{name: "missing reserved prefix", input: ptr("my-namespace"), wantErr: true},
		{name: "control plane namespace rejected", input: ptr("hypershell"), wantErr: true},
		{name: "platform db secret name rejected", input: ptr("hypershell-db-app"), wantErr: true},
		{name: "uppercase is not a DNS-1123 label", input: ptr("hypershell-managed-db-Prod"), wantErr: true},
		{name: "dots are not allowed in a namespace name", input: ptr("hypershell-managed-db-us.east.1"), wantErr: true},
		{name: "underscore is not a DNS-1123 label", input: ptr("hypershell-managed-db-us_east"), wantErr: true},
		{name: "trailing dash", input: ptr("hypershell-managed-db-"), wantErr: true},
		{name: "over 63 characters", input: ptr(externalCredentialsNamespacePrefix + strings.Repeat("a", 64)), wantErr: true},

		{name: "valid", input: ptr("hypershell-managed-db-us-east-1"), wantErr: false},
		{name: "valid with digits", input: ptr("hypershell-managed-db-123abc"), wantErr: false},
		{name: "exactly 63 characters", input: ptr(externalCredentialsNamespacePrefix + strings.Repeat("a", dns1123LabelMaxLength-len(externalCredentialsNamespacePrefix))), wantErr: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateExternalConnectionSecret(tc.input)
			if tc.wantErr && err == nil {
				t.Errorf("expected a validation error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected validation error: %v", err)
			}
		})
	}
}

// The prefix and the fixed Secret name are duplicated in the control plane
// (components/control-plane/internal/gateway/external_db.go). If either side
// changes, the reference stops resolving, so pin both here.
func TestExternalCredentialsConstantsArePinned(t *testing.T) {
	if externalCredentialsNamespacePrefix != "hypershell-managed-db-" {
		t.Errorf("namespace prefix = %q; the control plane expects %q", externalCredentialsNamespacePrefix, "hypershell-managed-db-")
	}
	if externalCredentialsSecretName != "hypershell-managed-db-credentials" {
		t.Errorf("secret name = %q; the control plane expects %q", externalCredentialsSecretName, "hypershell-managed-db-credentials")
	}
}

func TestIsSupportedProvider(t *testing.T) {
	for _, p := range []string{providerCNPG, providerDeployment, providerExternal} {
		if !isSupportedProvider(p) {
			t.Errorf("isSupportedProvider(%q) = false, want true", p)
		}
	}
	for _, p := range []string{"", "postgres", "CNPG", "rds"} {
		if isSupportedProvider(p) {
			t.Errorf("isSupportedProvider(%q) = true, want false", p)
		}
	}
}
