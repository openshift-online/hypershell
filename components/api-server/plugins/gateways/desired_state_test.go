package gateways

import "testing"

func strPtr(s string) *string { return &s }

// jsonb round-trips reorder keys and add spacing; that must not count as a
// desired-state change or generation is bumped on every control-plane write.
func TestDesiredStateChangedIgnoresJSONNormalization(t *testing.T) {
	stored := &Gateway{
		Oidc:  strPtr(`{"issuer": "https://kc/realms/x", "audience": "a", "jwks_ttl": 3600}`),
		Route: strPtr(`{"enabled": true}`),
	}
	written := &Gateway{
		Oidc:  strPtr(`{"audience":"a","issuer":"https://kc/realms/x","jwks_ttl":3600}`),
		Route: strPtr(`{"enabled":true}`),
	}
	if desiredStateChanged(stored, written) {
		t.Fatal("semantically equal oidc/route reported as a desired-state change")
	}

	written.Oidc = strPtr(`{"audience":"b","issuer":"https://kc/realms/x","jwks_ttl":3600}`)
	if !desiredStateChanged(stored, written) {
		t.Fatal("changed oidc audience not reported as a desired-state change")
	}

	written.Oidc = nil
	if !desiredStateChanged(stored, written) {
		t.Fatal("cleared oidc not reported as a desired-state change")
	}
}
