package managedClusters

import (
	"testing"
	"time"

	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
)

// ValidateHealthVerdict enforces the bounded enums and a name per condition so a
// stored verdict is always well-formed, and rejects a malformed one with a 400
// (PLAT-CLUS-16.03).
func TestValidateHealthVerdict(t *testing.T) {
	now := time.Now()
	valid := &HealthVerdict{
		Overall:    HealthOverallFailing,
		ObservedAt: &now,
		DurationMs: 842,
		Conditions: []HealthCondition{
			{Name: "oidc-issuer-reachable", Status: HealthStatusFail, Message: "503"},
			{Name: "images-pullable", Status: HealthStatusPass},
		},
	}

	tests := []struct {
		name    string
		verdict *HealthVerdict
		wantErr bool
	}{
		{name: "nil verdict is accepted (health-less heartbeat)", verdict: nil},
		{name: "well-formed verdict", verdict: valid},
		{
			name:    "bad overall enum",
			verdict: &HealthVerdict{Overall: "bogus", Conditions: valid.Conditions},
			wantErr: true,
		},
		{
			name:    "bad condition status enum",
			verdict: &HealthVerdict{Overall: HealthOverallHealthy, Conditions: []HealthCondition{{Name: "images-pullable", Status: "ok"}}},
			wantErr: true,
		},
		{
			name:    "empty condition name",
			verdict: &HealthVerdict{Overall: HealthOverallHealthy, Conditions: []HealthCondition{{Name: "", Status: HealthStatusPass}}},
			wantErr: true,
		},
		{
			name:    "duplicate condition name",
			verdict: &HealthVerdict{Overall: HealthOverallHealthy, Conditions: []HealthCondition{{Name: "images-pullable", Status: HealthStatusPass}, {Name: "images-pullable", Status: HealthStatusPass}}},
			wantErr: true,
		},
		{
			name:    "negative duration",
			verdict: &HealthVerdict{Overall: HealthOverallHealthy, DurationMs: -1, Conditions: valid.Conditions},
			wantErr: true,
		},
		{
			name:    "no conditions is accepted",
			verdict: &HealthVerdict{Overall: HealthOverallHealthy},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateHealthVerdict(tt.verdict)
			if tt.wantErr && err == nil {
				t.Fatalf("expected a validation error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantErr && err != nil && err.HttpCode != 400 {
				t.Fatalf("validation error HTTP code = %d, want 400", err.HttpCode)
			}
		})
	}
}

// The OpenAPI <-> domain health conversion is lossless, and a nil verdict maps to
// nil in both directions (null `health`, PLAT-DATA-27.02 / PLAT-CLUS-16.02).
func TestHealthVerdictConversionRoundTrip(t *testing.T) {
	if got := ConvertHealthVerdict(nil); got != nil {
		t.Fatalf("ConvertHealthVerdict(nil) = %v, want nil", got)
	}
	if got := PresentHealthVerdict(nil); got != nil {
		t.Fatalf("PresentHealthVerdict(nil) = %v, want nil", got)
	}

	observed := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	transition := time.Date(2026, 11, 8, 9, 12, 0, 0, time.UTC)
	in := &openapi.ManagedClusterHealth{
		Overall:    HealthOverallDegraded,
		ObservedAt: &observed,
		DurationMs: openapi.PtrInt64(842),
		Conditions: []openapi.ManagedClusterHealthCondition{
			{Name: "oidc-issuer-reachable", Status: HealthStatusWarn, LastTransitionAt: &transition, Message: openapi.PtrString("degraded")},
			{Name: "images-pullable", Status: HealthStatusPass},
		},
	}

	domain := ConvertHealthVerdict(in)
	if domain.Overall != HealthOverallDegraded || domain.DurationMs != 842 || len(domain.Conditions) != 2 {
		t.Fatalf("ConvertHealthVerdict lost data: %+v", domain)
	}
	if domain.Conditions[0].Message != "degraded" || domain.Conditions[1].Message != "" {
		t.Fatalf("condition messages not mapped: %+v", domain.Conditions)
	}

	out := PresentHealthVerdict(domain)
	if out.Overall != in.Overall || out.GetDurationMs() != 842 || len(out.Conditions) != 2 {
		t.Fatalf("PresentHealthVerdict lost data: %+v", out)
	}
	if !out.ObservedAt.Equal(observed) {
		t.Fatalf("observed_at = %v, want %v", out.ObservedAt, observed)
	}
	if out.Conditions[0].GetMessage() != "degraded" || out.Conditions[1].Message != nil {
		t.Fatalf("round-trip lost condition message: %+v", out.Conditions)
	}
}
