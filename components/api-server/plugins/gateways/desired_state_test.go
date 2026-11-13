package gateways

import "testing"

func strPtr(s string) *string { return &s }

// jsonb round-trips reorder keys and add spacing; that must not count as a
// desired-state change or generation is bumped on every control-plane write.
func TestDesiredStateChangedIgnoresJSONNormalization(t *testing.T) {
	stored := &Gateway{
		Oidc:  strPtr(`{"issuer": "https://kc/realms/x", "audience": "a", "jwks_ttl": 3600}`),
		Route: strPtr(`{"host": "a.example.com"}`),
	}
	written := &Gateway{
		Oidc:  strPtr(`{"audience":"a","issuer":"https://kc/realms/x","jwks_ttl":3600}`),
		Route: strPtr(`{"host":"a.example.com"}`),
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

// A workspace_mode change is a desired-spec change: it flips the namespace
// topology the control plane provisions, so it must advance generation and
// re-open the convergence gate. (The service funnel rejects such a change
// before it is written; this guards the comparison itself.)
func TestDesiredStateChangedWorkspaceMode(t *testing.T) {
	stored := &Gateway{WorkspaceMode: strPtr("shared")}
	written := &Gateway{WorkspaceMode: strPtr("managed")}
	if !desiredStateChanged(stored, written) {
		t.Fatal("changed workspace_mode not reported as a desired-state change")
	}
	if desiredStateChanged(written, &Gateway{WorkspaceMode: strPtr("managed")}) {
		t.Fatal("unchanged workspace_mode reported as a desired-state change")
	}
}
