package gateway

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var (
	dnsLabelRegex = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`)
	// Optional registry host may carry a port (e.g. the in-cluster registry
	// service address "image-registry.openshift-image-registry.svc:5000/..."),
	// which standard Docker image references permit as host[:port]/path[:tag].
	imageRefRegex = regexp.MustCompile(`^([a-z0-9.-]+(:[0-9]+)?/)?[a-z0-9._-]+(/[a-z0-9._-]+)*(:[a-z0-9._-]+)?(@sha256:[a-f0-9]{64})?$`)
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

	if config.SupervisorImage != "" {
		if err := ValidateImageReference(config.SupervisorImage); err != nil {
			return fmt.Errorf("invalid supervisor image: %w", err)
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

	if err := ValidateCredentialDriverConfig(config.CredentialDriver); err != nil {
		return fmt.Errorf("invalid credential driver config: %w", err)
	}

	return nil
}

func ValidateCredentialDriverConfig(config *CredentialDriverConfig) error {
	if config == nil {
		return nil
	}

	switch config.Type {
	case "kubernetes-secrets":
		if config.KubernetesSecrets != nil && config.KubernetesSecrets.Namespace != "" {
			if err := ValidateDNSLabel(config.KubernetesSecrets.Namespace); err != nil {
				return fmt.Errorf("invalid kubernetes_secrets.namespace: %w", err)
			}
		}
	case "vault":
		if config.Vault == nil || config.Vault.Address == "" || config.Vault.Role == "" {
			return fmt.Errorf("vault credential driver requires \"address\" and \"role\"")
		}
		if _, err := url.ParseRequestURI(config.Vault.Address); err != nil {
			return fmt.Errorf("invalid vault address URL: %w", err)
		}
		if err := validateIdentifier(config.Vault.Role); err != nil {
			return fmt.Errorf("invalid vault role: %w", err)
		}
		if config.Vault.Mount != "" {
			if err := validateIdentifier(config.Vault.Mount); err != nil {
				return fmt.Errorf("invalid vault mount: %w", err)
			}
		}
		if config.Vault.AuthMethod != "" {
			if err := validateIdentifier(config.Vault.AuthMethod); err != nil {
				return fmt.Errorf("invalid vault auth_method: %w", err)
			}
		}
		if config.Vault.KubernetesAuthMount != "" {
			if err := validateIdentifier(config.Vault.KubernetesAuthMount); err != nil {
				return fmt.Errorf("invalid vault kubernetes_auth_mount: %w", err)
			}
		}
	default:
		return fmt.Errorf("unsupported credential driver type %q; supported: kubernetes-secrets, vault", config.Type)
	}

	return nil
}

func ValidateDNSLabel(label string) error {
	if len(label) == 0 || len(label) > 63 {
		return fmt.Errorf("must be 1-63 characters: %q", label)
	}
	if !dnsLabelRegex.MatchString(label) {
		return fmt.Errorf("invalid DNS label: %q", label)
	}
	return nil
}

var identifierRegex = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._/-]*$`)

func validateIdentifier(value string) error {
	if len(value) == 0 || len(value) > 253 {
		return fmt.Errorf("must be 1-253 characters: %q", value)
	}
	if strings.ContainsAny(value, "\"\n\r\t\\") {
		return fmt.Errorf("contains invalid characters: %q", value)
	}
	if !identifierRegex.MatchString(value) {
		return fmt.Errorf("invalid identifier format: %q", value)
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
