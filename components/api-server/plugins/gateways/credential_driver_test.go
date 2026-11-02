package gateways

import (
	"testing"
)

// TestEnforceGatewayCredentialDriverUpdateSemantics covers the effective-driver
// comparison behind OSG-CRED-13: the default spellings are equivalent, any change
// of type or sub-fields is rejected, and an equivalent resend keeps stored bytes.
// Verifies: OSG-CRED-13
func TestEnforceGatewayCredentialDriverUpdateSemantics(t *testing.T) {
	k8s := `{"type":"kubernetes-secrets","kubernetes_secrets":{"namespace":"a"}}`
	cases := []struct {
		name     string
		current  *string
		next     *string
		wantErr  bool
		wantNext *string
	}{
		{name: "nil next keeps stored value", current: strPtr(`{"type":"kek-db"}`), next: nil, wantNext: strPtr(`{"type":"kek-db"}`)},
		{name: "empty next keeps stored value", current: strPtr(`{"type":"kek-db"}`), next: strPtr(" "), wantNext: strPtr(`{"type":"kek-db"}`)},
		{name: "NULL and explicit vault are equivalent", current: nil, next: strPtr(`{"type":"vault"}`), wantNext: nil},
		{name: "empty object and vault are equivalent", current: strPtr(`{}`), next: strPtr(`{"type":"vault"}`), wantNext: strPtr(`{}`)},
		{name: "key order does not matter", current: strPtr(k8s), next: strPtr(`{"kubernetes_secrets":{"namespace":"a"},"type":"kubernetes-secrets"}`), wantNext: strPtr(k8s)},
		{name: "default vault to kek-db rejected", current: nil, next: strPtr(`{"type":"kek-db"}`), wantErr: true},
		{name: "vault to kubernetes-secrets rejected", current: strPtr(`{"type":"vault"}`), next: strPtr(`{"type":"kubernetes-secrets"}`), wantErr: true},
		{name: "kubernetes-secrets namespace change rejected", current: strPtr(k8s), next: strPtr(`{"type":"kubernetes-secrets","kubernetes_secrets":{"namespace":"b"}}`), wantErr: true},
		{name: "malformed next rejected", current: nil, next: strPtr(`{not json`), wantErr: true},
		{name: "malformed stored value accepts only identical resend", current: strPtr(`{bad`), next: strPtr(`{bad`), wantNext: strPtr(`{bad`)},
		{name: "malformed stored value rejects a change", current: strPtr(`{bad`), next: strPtr(`{"type":"vault"}`), wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			current := &Gateway{CredentialDriver: tc.current}
			next := &Gateway{CredentialDriver: tc.next}
			err := enforceGatewayCredentialDriverUpdateSemantics(current, next)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got none")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !strEq(next.CredentialDriver, tc.wantNext) {
				t.Fatalf("next.CredentialDriver = %v, want %v", derefString(next.CredentialDriver), derefString(tc.wantNext))
			}
		})
	}
}
