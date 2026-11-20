package cedar

import "testing"

func TestParseMillis(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want int64
	}{
		{"milli suffix", "500m", 500},
		{"whole cores", "2", 2000},
		{"fractional cores", "0.5", 500},
		{"one and a half cores", "1.5", 1500},
		{"one milli", "1m", 1},
		{"zero", "0", 0},
		{"whitespace trimmed", "  250m  ", 250},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseMillis(tt.in)
			if err != nil {
				t.Fatalf("ParseMillis(%q) error: %v", tt.in, err)
			}
			if got != tt.want {
				t.Fatalf("ParseMillis(%q) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

func TestParseMillisErrors(t *testing.T) {
	for _, in := range []string{"", "abc", "-1", "1Gi5", "0.0005"} {
		t.Run(in, func(t *testing.T) {
			if _, err := ParseMillis(in); err == nil {
				t.Fatalf("ParseMillis(%q) expected error", in)
			}
		})
	}
}

func TestParseBytes(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want int64
	}{
		// PLAT-PGP-03.01: 16Gi normalizes to 17179869184 bytes.
		{"gibibytes", "16Gi", 17179869184},
		{"one gibibyte", "1Gi", 1073741824},
		{"mebibytes", "512Mi", 536870912},
		{"kibibytes", "1Ki", 1024},
		{"decimal giga", "1G", 1000000000},
		{"plain bytes", "1000", 1000},
		{"zero", "0", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseBytes(tt.in)
			if err != nil {
				t.Fatalf("ParseBytes(%q) error: %v", tt.in, err)
			}
			if got != tt.want {
				t.Fatalf("ParseBytes(%q) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

func TestParseBytesErrors(t *testing.T) {
	for _, in := range []string{"", "xyz", "-5", "0.5"} {
		t.Run(in, func(t *testing.T) {
			if _, err := ParseBytes(in); err == nil {
				t.Fatalf("ParseBytes(%q) expected error", in)
			}
		})
	}
}
