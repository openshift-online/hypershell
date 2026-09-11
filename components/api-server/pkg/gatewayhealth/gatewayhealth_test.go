package gatewayhealth

import "testing"

func TestIsValidPhase(t *testing.T) {
	cases := []struct {
		name  string
		phase string
		want  bool
	}{
		{"pending", "Pending", true},
		{"provisioning", "Provisioning", true},
		{"running", "Running", true},
		{"degraded", "Degraded", true},
		{"failed", "Failed", true},
		{"empty is not valid", "", false},
		{"unknown value", "Booting", false},
		{"wrong case is rejected", "running", false},
		{"trailing space is rejected", "Running ", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsValidPhase(tc.phase); got != tc.want {
				t.Fatalf("IsValidPhase(%q) = %v, want %v", tc.phase, got, tc.want)
			}
		})
	}
}

func TestPhaseStringsCoversEveryConstant(t *testing.T) {
	got := PhaseStrings()
	want := []string{"Pending", "Provisioning", "Running", "Degraded", "Failed"}
	if len(got) != len(want) {
		t.Fatalf("PhaseStrings() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("PhaseStrings()[%d] = %q, want %q (order must be canonical)", i, got[i], want[i])
		}
	}
}

func TestPhasesReturnsCopy(t *testing.T) {
	first := Phases()
	first[0] = "Mutated"
	if second := Phases(); second[0] != PhasePending {
		t.Fatalf("Phases() returned a mutable view: got %q after mutation, want %q", second[0], PhasePending)
	}
}
