package gatewayProfiles

import (
	"net/http"
	"testing"
)

func strPtr(s string) *string { return &s }
func int32Ptr(i int32) *int32 { return &i }

// TestValidateProfileFields exercises the real k8s.io/apimachinery quantity
// parser wired into validate.go. Empty/unset values are "not set" and always
// pass; non-empty values must parse as valid, non-negative Kubernetes resource
// quantities; counts must be non-negative.
func TestValidateProfileFields(t *testing.T) {
	t.Run("nil and empty fields are accepted", func(t *testing.T) {
		if svcErr := validateProfileFields(&GatewayProfile{Name: "empty"}); svcErr != nil {
			t.Fatalf("expected no error for empty profile, got %v", svcErr)
		}
		if svcErr := validateProfileFields(&GatewayProfile{CpuRequestTotal: strPtr("")}); svcErr != nil {
			t.Fatalf("expected no error for empty-string quantity, got %v", svcErr)
		}
	})

	t.Run("valid quantities and counts are accepted", func(t *testing.T) {
		p := &GatewayProfile{
			CpuRequestTotal:               strPtr("2"),
			CpuLimitTotal:                 strPtr("4"),
			MemoryRequestTotal:            strPtr("4Gi"),
			MemoryLimitTotal:              strPtr("8Gi"),
			EphemeralStorageTotal:         strPtr("10Gi"),
			ContainerCpuRequestDefault:    strPtr("100m"),
			ContainerCpuLimitMax:          strPtr("1500m"),
			ContainerMemoryRequestDefault: strPtr("128Mi"),
			ContainerMemoryLimitMax:       strPtr("1G"),
			PodCount:                      int32Ptr(10),
			PvcCount:                      int32Ptr(0), // zero is a valid "not set" count
		}
		if svcErr := validateProfileFields(p); svcErr != nil {
			t.Fatalf("expected valid profile to pass, got %v", svcErr)
		}
	})

	badQuantities := map[string]string{
		"garbage":       "tow",
		"bad unit":      "2GB",
		"trailing text": "5Gib",
		"space":         "5 Gi",
		"double suffix": "5GiGi",
	}
	for name, val := range badQuantities {
		val := val
		t.Run("rejects unparseable quantity: "+name, func(t *testing.T) {
			svcErr := validateProfileFields(&GatewayProfile{MemoryLimitTotal: strPtr(val)})
			if svcErr == nil {
				t.Fatalf("expected validation error for %q, got nil", val)
			}
			if svcErr.HttpCode != http.StatusBadRequest {
				t.Fatalf("expected HTTP 400 for %q, got %d", val, svcErr.HttpCode)
			}
		})
	}

	t.Run("rejects a negative quantity", func(t *testing.T) {
		svcErr := validateProfileFields(&GatewayProfile{CpuRequestTotal: strPtr("-1")})
		if svcErr == nil {
			t.Fatal("expected validation error for negative quantity, got nil")
		}
		if svcErr.HttpCode != http.StatusBadRequest {
			t.Fatalf("expected HTTP 400, got %d", svcErr.HttpCode)
		}
	})

	t.Run("rejects negative counts", func(t *testing.T) {
		cases := []struct {
			name    string
			profile *GatewayProfile
		}{
			{"pod_count", &GatewayProfile{PodCount: int32Ptr(-1)}},
			{"pvc_count", &GatewayProfile{PvcCount: int32Ptr(-5)}},
		}
		for _, tc := range cases {
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				svcErr := validateProfileFields(tc.profile)
				if svcErr == nil {
					t.Fatalf("expected error for negative %s, got nil", tc.name)
				}
				if svcErr.HttpCode != http.StatusBadRequest {
					t.Fatalf("expected HTTP 400 for negative %s, got %d", tc.name, svcErr.HttpCode)
				}
			})
		}
	})

	t.Run("accepts nil and zero counts", func(t *testing.T) {
		if svcErr := validateProfileFields(&GatewayProfile{}); svcErr != nil {
			t.Fatalf("expected nil counts to pass, got %v", svcErr)
		}
		if svcErr := validateProfileFields(&GatewayProfile{PodCount: int32Ptr(0), PvcCount: int32Ptr(0)}); svcErr != nil {
			t.Fatalf("expected zero counts to pass, got %v", svcErr)
		}
	})

	t.Run("cross-field: request exceeding limit is rejected", func(t *testing.T) {
		cases := []struct {
			name    string
			profile *GatewayProfile
		}{
			{"cpu_request_total > cpu_limit_total", &GatewayProfile{
				CpuRequestTotal: strPtr("8"), CpuLimitTotal: strPtr("4"),
			}},
			{"memory_request_total > memory_limit_total", &GatewayProfile{
				MemoryRequestTotal: strPtr("16Gi"), MemoryLimitTotal: strPtr("8Gi"),
			}},
			{"container_cpu_request_default > container_cpu_limit_max", &GatewayProfile{
				ContainerCpuRequestDefault: strPtr("2"), ContainerCpuLimitMax: strPtr("500m"),
			}},
			{"container_memory_request_default > container_memory_limit_max", &GatewayProfile{
				ContainerMemoryRequestDefault: strPtr("512Mi"), ContainerMemoryLimitMax: strPtr("256Mi"),
			}},
		}
		for _, tc := range cases {
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				svcErr := validateProfileFields(tc.profile)
				if svcErr == nil {
					t.Fatalf("expected validation error for %s, got nil", tc.name)
				}
				if svcErr.HttpCode != http.StatusBadRequest {
					t.Fatalf("expected HTTP 400 for %s, got %d", tc.name, svcErr.HttpCode)
				}
			})
		}
	})

	t.Run("cross-field: request equal to limit is accepted", func(t *testing.T) {
		p := &GatewayProfile{
			CpuRequestTotal:               strPtr("4"),
			CpuLimitTotal:                 strPtr("4"),
			MemoryRequestTotal:            strPtr("8Gi"),
			MemoryLimitTotal:              strPtr("8Gi"),
			ContainerCpuRequestDefault:    strPtr("500m"),
			ContainerCpuLimitMax:          strPtr("500m"),
			ContainerMemoryRequestDefault: strPtr("256Mi"),
			ContainerMemoryLimitMax:       strPtr("256Mi"),
		}
		if svcErr := validateProfileFields(p); svcErr != nil {
			t.Fatalf("expected request==limit to pass, got %v", svcErr)
		}
	})

	t.Run("cross-field: unset limit skips request check", func(t *testing.T) {
		// Only cpu_request_total is set; cpu_limit_total is nil → no cross-check.
		if svcErr := validateProfileFields(&GatewayProfile{CpuRequestTotal: strPtr("100")}); svcErr != nil {
			t.Fatalf("expected unset limit to skip cross-check, got %v", svcErr)
		}
	})
}
