package gateway

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	dnsLabelRegex = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`)
	imageRefRegex = regexp.MustCompile(`^([a-z0-9.-]+/)?[a-z0-9._-]+(/[a-z0-9._-]+)*(:[a-z0-9._-]+)?(@sha256:[a-f0-9]{64})?$`)
)

func ValidateDNSName(name string) error {
	if len(name) == 0 {
		return fmt.Errorf("DNS name cannot be empty")
	}
	if len(name) > 253 {
		return fmt.Errorf("DNS name too long (max 253 characters): %d", len(name))
	}
	if !dnsLabelRegex.MatchString(name) {
		return fmt.Errorf("invalid DNS name format: %q", name)
	}
	return nil
}

func ValidateImageReference(ref string) error {
	if len(ref) == 0 {
		return fmt.Errorf("image reference cannot be empty")
	}

	normalized := strings.ToLower(ref)
	if !imageRefRegex.MatchString(normalized) {
		return fmt.Errorf("invalid image reference format: %q", ref)
	}

	if strings.Contains(ref, ";") || strings.Contains(ref, "&") ||
		strings.Contains(ref, "|") || strings.Contains(ref, "`") ||
		strings.Contains(ref, "$") || strings.Contains(ref, "\n") {
		return fmt.Errorf("image reference contains invalid characters: %q", ref)
	}

	return nil
}

func ValidateGatewayConfig(config GatewayConfig) error {
	if config.Image != "" {
		if err := ValidateImageReference(config.Image); err != nil {
			return fmt.Errorf("invalid image: %w", err)
		}
	}

	for i, dns := range config.ServerDnsNames {
		if err := ValidateDNSName(dns); err != nil {
			return fmt.Errorf("invalid serverDnsNames[%d]: %w", i, err)
		}
	}

	if err := ValidateOIDCConfig(config.OIDC); err != nil {
		return fmt.Errorf("invalid OIDC config: %w", err)
	}

	return nil
}

func ValidateOIDCConfig(oidc OIDCConfig) error {
	if oidc.Issuer == "" {
		return nil
	}

	if (oidc.AdminRole != "") != (oidc.UserRole != "") {
		return fmt.Errorf("both admin_role and user_role must be set, or both must be empty")
	}

	return nil
}
