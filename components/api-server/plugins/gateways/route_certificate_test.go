package gateways

import (
	"strings"
	"testing"
)

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

// TestValidateRouteCertificateShape guards the API-boundary validation of the
// certificate opt-in (API UX review finding 1 and 3): an invalid shape must be
// rejected with an immediate 400 at create/update, not accepted with a 201 and
// failed asynchronously during provisioning. Non-empty certificate objects
// would be silently discarded (the issuer is cluster infrastructure), and
// certificate+host is a combination the control plane's ValidateGatewayConfig
// rejects at provisioning time.
func TestValidateRouteCertificateShape(t *testing.T) {
	cases := []struct {
		name       string
		route      *string
		wantErr    bool
		wantSubstr string
	}{
		{name: "nil route", route: nil, wantErr: false},
		{name: "no certificate key", route: strPtr(`{"host":"a.example.com"}`), wantErr: false},
		{name: "certificate null counts as absent", route: strPtr(`{"certificate":null}`), wantErr: false},
		{name: "empty certificate object accepted", route: strPtr(`{"certificate":{},"enabled":true}`), wantErr: false},
		{name: "non-empty certificate object rejected", route: strPtr(`{"certificate":{"issuer":"custom"}}`),
			wantErr: true, wantSubstr: "accepts no fields"},
		{name: "non-object certificate value rejected", route: strPtr(`{"certificate":true}`),
			wantErr: true, wantSubstr: "accepts no fields"},
		{name: "certificate with host rejected", route: strPtr(`{"host":"a.example.com","certificate":{}}`),
			wantErr: true, wantSubstr: "route.host is not supported with route.certificate"},
		{name: "empty-string host counts as absent", route: strPtr(`{"host":"","certificate":{}}`), wantErr: false},
		{name: "null host counts as absent", route: strPtr(`{"host":null,"certificate":{}}`), wantErr: false},
		{name: "unparseable route left to the jsonb constraint", route: strPtr(`{"certificate":`), wantErr: false},
		{name: "non-object route document left alone", route: strPtr(`["certificate"]`), wantErr: false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			svcErr := validateRouteCertificateShape(c.route)
			if c.wantErr {
				if svcErr == nil {
					t.Fatalf("validateRouteCertificateShape(%v) = nil, want error", c.route)
				}
				if c.wantSubstr != "" && !strings.Contains(svcErr.Error(), c.wantSubstr) {
					t.Errorf("error %q does not contain %q", svcErr.Error(), c.wantSubstr)
				}
			} else if svcErr != nil {
				t.Errorf("validateRouteCertificateShape(%v) = %v, want nil", c.route, svcErr)
			}
		})
	}
}
