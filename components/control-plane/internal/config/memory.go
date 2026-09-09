package config

import (
	"fmt"
	"k8s.io/apimachinery/pkg/api/resource"
	"os"
	"strings"
)

// MemoryRequest reads a gateway or deployment database memory request.
// Reject requests above the existing 512Mi container limit.
func MemoryRequest(name string) (string, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return "256Mi", nil
	}
	value, err := resource.ParseQuantity(raw)
	maximum := resource.MustParse("512Mi")
	if err != nil || value.Sign() <= 0 || value.Cmp(maximum) > 0 {
		return "", fmt.Errorf("%s must be a positive Kubernetes memory quantity at most 512Mi", name)
	}
	return value.String(), nil
}
