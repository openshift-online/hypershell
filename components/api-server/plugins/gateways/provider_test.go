package gateways

import "testing"

// TestResolveDatabaseProvider pins the DATABASE_PROVIDER startup contract:
// unset/empty and "deployment" both resolve to ProviderDeployment (the
// default, requiring no CNPG APIs), "cnpg" resolves to ProviderCNPG, and any
// other value is a startup configuration error rather than an implicit
// fallback to CNPG.
func TestResolveDatabaseProvider(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    string
		wantErr bool
	}{
		{name: "unset/empty defaults to deployment", raw: "", want: ProviderDeployment},
		{name: "deployment stays deployment", raw: "deployment", want: ProviderDeployment},
		{name: "cnpg selects cnpg", raw: "cnpg", want: ProviderCNPG},
		{name: "unsupported value is an error, not a cnpg fallback", raw: "postgres", wantErr: true},
		{name: "case-sensitive: CNPG is an error", raw: "CNPG", wantErr: true},
		{name: "whitespace is an error, not trimmed", raw: " cnpg", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveDatabaseProvider(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("resolveDatabaseProvider(%q) = %q, want an error", tt.raw, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveDatabaseProvider(%q): unexpected error: %v", tt.raw, err)
			}
			if got != tt.want {
				t.Fatalf("resolveDatabaseProvider(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}
