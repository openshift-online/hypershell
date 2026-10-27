package gateways

import "testing"

// routeCertificateChanged guards the provision-time-only certificate opt-in
// (docs/investigations/per-gateway-certificates.md §7.7): adding, removing, or
// altering route.certificate after creation must be detected, while
// normalization noise (whitespace, key order) and unrelated sub-key edits must
// not be.
func TestRouteCertificateChanged(t *testing.T) {
	cases := []struct {
		name    string
		current *string
		next    *string
		want    bool
	}{
		{
			name:    "both absent",
			current: strPtr(`{"host":"a.example.com"}`),
			next:    strPtr(`{"host":"b.example.com"}`),
			want:    false,
		},
		{
			name:    "opt-in added",
			current: strPtr(`{"host":"a.example.com"}`),
			next:    strPtr(`{"host":"a.example.com","certificate":{}}`),
			want:    true,
		},
		{
			name:    "opt-in removed",
			current: strPtr(`{"host":"a.example.com","certificate":{}}`),
			next:    strPtr(`{"host":"a.example.com"}`),
			want:    true,
		},
		{
			name:    "opt-in kept, unrelated keys changed",
			current: strPtr(`{"host":"a.example.com","certificate":{}}`),
			next:    strPtr(`{"certificate":{},"host":"b.example.com"}`),
			want:    false,
		},
		{
			name:    "opt-in kept, jsonb normalization",
			current: strPtr(`{"certificate": { }, "host": "a.example.com", "enabled": true}`),
			next:    strPtr(`{"enabled":true,"host":"a.example.com","certificate":{}}`),
			want:    false,
		},
		{
			name:    "opt-in value altered",
			current: strPtr(`{"certificate":{}}`),
			next:    strPtr(`{"certificate":{"issuer":"custom"}}`),
			want:    true,
		},
		{
			name:    "opt-in value equal after normalization",
			current: strPtr(`{"certificate": {"issuer": "custom"}}`),
			next:    strPtr(`{"certificate":{"issuer":"custom"}}`),
			want:    false,
		},
		{
			name:    "json null counts as absent on both sides",
			current: strPtr(`{"host":"a.example.com","certificate":null}`),
			next:    strPtr(`{"host":"a.example.com"}`),
			want:    false,
		},
		{
			name:    "json null to present is an opt-in",
			current: strPtr(`{"certificate":null}`),
			next:    strPtr(`{"certificate":{}}`),
			want:    true,
		},
		{
			name:    "nil route vs absent certificate",
			current: nil,
			next:    strPtr(`{"host":"a.example.com"}`),
			want:    false,
		},
		{
			name:    "nil route vs opted-in route",
			current: nil,
			next:    strPtr(`{"certificate":{}}`),
			want:    true,
		},
		{
			name:    "cleared route vs opted-in route",
			current: strPtr(`{"certificate":{}}`),
			next:    nil,
			want:    true,
		},
		{
			name:    "non-object route cannot carry the key",
			current: strPtr(`["certificate"]`),
			next:    strPtr(`{"certificate":{}}`),
			want:    true,
		},
		{
			name:    "unparseable incoming route is left to the jsonb constraint",
			current: strPtr(`{"certificate":{}}`),
			next:    strPtr(`{"certificate":`),
			want:    false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := routeCertificateChanged(c.current, c.next); got != c.want {
				t.Errorf("routeCertificateChanged(%v, %v) = %v, want %v", c.current, c.next, got, c.want)
			}
		})
	}
}
