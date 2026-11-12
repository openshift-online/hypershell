package gateway

import "testing"

func TestInitConditionsIncludesDefaultWorkspaceBootstrapReadiness(t *testing.T) {
	found := false
	for _, condition := range InitConditions() {
		if condition.Type == ConditionDefaultWorkspaceBootstrapReady {
			found = true
			if condition.ConditionStatus != StatusPending {
				t.Fatalf("initial condition status = %q, want %q", condition.ConditionStatus, StatusPending)
			}
		}
	}
	if !found {
		t.Fatal("DefaultWorkspaceBootstrapReady condition is missing")
	}
}
