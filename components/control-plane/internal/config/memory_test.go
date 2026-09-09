package config

import "testing"

func TestMemoryRequest(t *testing.T) {
	for _, name := range []string{"DATABASE_MEMORY_REQUEST", "GATEWAY_MEMORY_REQUEST"} {
		for _, test := range []struct {
			value, want string
			invalid     bool
		}{
			{"", "256Mi", false}, {"128Mi", "128Mi", false}, {"512Mi", "512Mi", false},
			{"invalid", "", true}, {"0", "", true}, {"-1Mi", "", true}, {"1Gi", "", true},
		} {
			t.Run(name+test.value, func(t *testing.T) {
				t.Setenv(name, test.value)
				got, err := MemoryRequest(name)
				if (err != nil) != test.invalid || got != test.want {
					t.Fatalf("got %q, %v", got, err)
				}
			})
		}
	}
}
