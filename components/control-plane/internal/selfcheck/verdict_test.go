package selfcheck

import "testing"

func TestRollup(t *testing.T) {
	tests := []struct {
		name       string
		conditions []Condition
		want       Overall
	}{
		{
			// PLAT-HLTH-03.01
			name: "all pass rolls up to healthy",
			conditions: []Condition{
				{Name: CheckReconcilerHealthy, Status: StatusPass},
				{Name: CheckImagesPullable, Status: StatusPass},
			},
			want: OverallHealthy,
		},
		{
			// PLAT-HLTH-03.02
			name: "any fail rolls up to failing",
			conditions: []Condition{
				{Name: CheckReconcilerHealthy, Status: StatusPass},
				{Name: CheckImagesPullable, Status: StatusFail},
				{Name: CheckOIDCIssuerReachable, Status: StatusWarn},
			},
			want: OverallFailing,
		},
		{
			name: "a warn without a fail rolls up to degraded",
			conditions: []Condition{
				{Name: CheckReconcilerHealthy, Status: StatusPass},
				{Name: CheckIngressControllerHealthy, Status: StatusWarn},
			},
			want: OverallDegraded,
		},
		{
			name: "an unknown without a fail rolls up to degraded",
			conditions: []Condition{
				{Name: CheckReconcilerHealthy, Status: StatusPass},
				{Name: CheckOIDCIssuerReachable, Status: StatusUnknown},
			},
			want: OverallDegraded,
		},
		{
			name:       "no conditions rolls up to degraded",
			conditions: nil,
			want:       OverallDegraded,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Rollup(tt.conditions); got != tt.want {
				t.Fatalf("Rollup = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCheckNamesAreUnique(t *testing.T) {
	seen := map[CheckName]bool{}
	for _, n := range CheckNames() {
		if n == "" {
			t.Fatalf("empty check name in enumeration")
		}
		if seen[n] {
			t.Fatalf("duplicate check name %q", n)
		}
		seen[n] = true
	}
}
