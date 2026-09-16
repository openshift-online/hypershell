package gateways

import (
	"encoding/json"
	"testing"
)

func condsJSON(t *testing.T, conds ...provisioningConditionJSON) *string {
	t.Helper()
	data, err := json.Marshal(conds)
	if err != nil {
		t.Fatalf("marshal conditions: %v", err)
	}
	s := string(data)
	return &s
}

func parseOrFail(t *testing.T, raw *string) []provisioningConditionJSON {
	t.Helper()
	conds, ok := parseProvisioningConditions(raw)
	if !ok {
		t.Fatalf("expected parseable conditions, got %v", raw)
	}
	return conds
}

func statusByType(conds []provisioningConditionJSON) map[string]string {
	out := make(map[string]string, len(conds))
	for _, c := range conds {
		out[c.Type] = c.ConditionStatus
	}
	return out
}

func TestMergeMonotonicProvisioningConditions(t *testing.T) {
	tests := []struct {
		name     string
		current  *string
		incoming *string
		want     map[string]string // nil means expect result == current pointer
		wantNil  bool
	}{
		{
			name:     "nil incoming preserves current",
			current:  condsJSON(t, provisioningConditionJSON{Type: "GatewayHealthy", ConditionStatus: conditionStatusComplete}),
			incoming: nil,
			want:     map[string]string{"GatewayHealthy": conditionStatusComplete},
		},
		{
			name:     "completed step does not regress to InProgress",
			current:  condsJSON(t, provisioningConditionJSON{Type: "GatewayHealthy", ConditionStatus: conditionStatusComplete}),
			incoming: condsJSON(t, provisioningConditionJSON{Type: "GatewayHealthy", ConditionStatus: conditionStatusInProgress}),
			want:     map[string]string{"GatewayHealthy": conditionStatusComplete},
		},
		{
			name:     "in-progress step does not regress to Pending",
			current:  condsJSON(t, provisioningConditionJSON{Type: "GatewayDeployed", ConditionStatus: conditionStatusInProgress}),
			incoming: condsJSON(t, provisioningConditionJSON{Type: "GatewayDeployed", ConditionStatus: conditionStatusPending}),
			want:     map[string]string{"GatewayDeployed": conditionStatusInProgress},
		},
		{
			name:     "forward progress is accepted",
			current:  condsJSON(t, provisioningConditionJSON{Type: "DatabaseReady", ConditionStatus: conditionStatusInProgress}),
			incoming: condsJSON(t, provisioningConditionJSON{Type: "DatabaseReady", ConditionStatus: conditionStatusComplete}),
			want:     map[string]string{"DatabaseReady": conditionStatusComplete},
		},
		{
			name:     "failure surfaces over completed",
			current:  condsJSON(t, provisioningConditionJSON{Type: "GatewayHealthy", ConditionStatus: conditionStatusComplete}),
			incoming: condsJSON(t, provisioningConditionJSON{Type: "GatewayHealthy", ConditionStatus: conditionStatusFailed}),
			want:     map[string]string{"GatewayHealthy": conditionStatusFailed},
		},
		{
			name:     "recovery from failed is accepted",
			current:  condsJSON(t, provisioningConditionJSON{Type: "GatewayHealthy", ConditionStatus: conditionStatusFailed}),
			incoming: condsJSON(t, provisioningConditionJSON{Type: "GatewayHealthy", ConditionStatus: conditionStatusInProgress}),
			want:     map[string]string{"GatewayHealthy": conditionStatusInProgress},
		},
		{
			name: "incoming full reset keeps completed steps",
			current: condsJSON(t,
				provisioningConditionJSON{Type: "EnvironmentReady", ConditionStatus: conditionStatusComplete},
				provisioningConditionJSON{Type: "DatabaseReady", ConditionStatus: conditionStatusComplete},
				provisioningConditionJSON{Type: "GatewayDeployed", ConditionStatus: conditionStatusInProgress},
				provisioningConditionJSON{Type: "GatewayHealthy", ConditionStatus: conditionStatusPending},
			),
			incoming: condsJSON(t,
				provisioningConditionJSON{Type: "EnvironmentReady", ConditionStatus: conditionStatusPending},
				provisioningConditionJSON{Type: "DatabaseReady", ConditionStatus: conditionStatusPending},
				provisioningConditionJSON{Type: "GatewayDeployed", ConditionStatus: conditionStatusPending},
				provisioningConditionJSON{Type: "GatewayHealthy", ConditionStatus: conditionStatusPending},
			),
			want: map[string]string{
				"EnvironmentReady": conditionStatusComplete,
				"DatabaseReady":    conditionStatusComplete,
				"GatewayDeployed":  conditionStatusInProgress,
				"GatewayHealthy":   conditionStatusPending,
			},
		},
		{
			name:     "empty current accepts incoming",
			current:  nil,
			incoming: condsJSON(t, provisioningConditionJSON{Type: "EnvironmentReady", ConditionStatus: conditionStatusPending}),
			want:     map[string]string{"EnvironmentReady": conditionStatusPending},
		},
		{
			name:     "incoming omitting a completed step retains it",
			current:  condsJSON(t, provisioningConditionJSON{Type: "IdentityProviderReady", ConditionStatus: conditionStatusComplete}, provisioningConditionJSON{Type: "GatewayDeployed", ConditionStatus: conditionStatusInProgress}),
			incoming: condsJSON(t, provisioningConditionJSON{Type: "GatewayDeployed", ConditionStatus: conditionStatusComplete}),
			want:     map[string]string{"IdentityProviderReady": conditionStatusComplete, "GatewayDeployed": conditionStatusComplete},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := mergeMonotonicProvisioningConditions(tc.current, tc.incoming)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			gotStatuses := statusByType(parseOrFail(t, got))
			for typ, wantStatus := range tc.want {
				if gotStatuses[typ] != wantStatus {
					t.Errorf("%s: got status %q, want %q (full: %v)", typ, gotStatuses[typ], wantStatus, gotStatuses)
				}
			}
			if len(gotStatuses) != len(tc.want) {
				t.Errorf("got %d conditions, want %d (full: %v)", len(gotStatuses), len(tc.want), gotStatuses)
			}
		})
	}
}

func TestParseProvisioningConditions(t *testing.T) {
	if _, ok := parseProvisioningConditions(nil); ok {
		t.Error("nil should not parse")
	}
	empty := ""
	if _, ok := parseProvisioningConditions(&empty); ok {
		t.Error("empty string should not parse")
	}
	emptyList := "[]"
	if _, ok := parseProvisioningConditions(&emptyList); ok {
		t.Error("empty list should not parse")
	}
	bad := "{not json"
	if _, ok := parseProvisioningConditions(&bad); ok {
		t.Error("invalid json should not parse")
	}
	good := condsJSON(t, provisioningConditionJSON{Type: "GatewayHealthy", ConditionStatus: conditionStatusComplete})
	conds, ok := parseProvisioningConditions(good)
	if !ok || len(conds) != 1 {
		t.Errorf("expected 1 parsed condition, got ok=%v conds=%v", ok, conds)
	}
}
