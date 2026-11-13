package gateway

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/distribution/reference"
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

	// Reject at input anything the Helm values builder's canonical parser would
	// reject at reconcile time (helm.parseImageRef uses the same
	// distribution/reference parser). The regex above is lowercased before
	// matching, so it accepts uppercase repository paths that the parser rejects
	// ("repository name must be lowercase"); without this check such a reference
	// passes input validation but then hard-fails Build() on every reconcile,
	// turning a cleanly rejectable input into a permanent reconcile block.
	if _, err := reference.Parse(strings.TrimSpace(ref)); err != nil {
		return fmt.Errorf("invalid image reference %q: %w", ref, err)
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

	if config.SandboxRuntimeImage != "" {
		if err := ValidateImageReference(config.SandboxRuntimeImage); err != nil {
			return fmt.Errorf("invalid sandbox runtime image: %w", err)
		}
	}

	// The API server already enforces the workspace-mode vocabulary on write
	// and stores a mode for every gateway; this is the reconcile-time defense in
	// depth so a missing or bad value can never reach the Helm chart and silently
	// fall back to the chart default (openshell-gateway.spec.md, "Gateway
	// Workspace Mode").
	switch config.WorkspaceMode {
	case "shared", "managed", "operator":
	default:
		return fmt.Errorf("invalid workspaceMode %q: must be one of shared, managed, operator", config.WorkspaceMode)
	}

	for i, dns := range config.ServerDnsNames {
		if err := ValidateDNSName(dns); err != nil {
			return fmt.Errorf("invalid serverDnsNames[%d]: %w", i, err)
		}
	}

	// A dedicated-certificate gateway (route.certificate) always uses the
	// derived gw-<namespace> hostname: its certificate, listeners, and DNS
	// records are all provisioned from that derivation, so an explicit
	// route.host has no meaning on that path and is rejected rather than
	// silently ignored. Explicit hosts keep today's shared-Gateway semantics.
	if config.Route.Certificate != nil && config.Route.Host != "" {
		return fmt.Errorf("route.host is not supported with route.certificate: a dedicated-certificate gateway uses the derived gw-<namespace> hostname")
	}

	// An explicit route host lands verbatim in the Route spec.host and the
	// gateway certificate SANs, so it must be a well-formed DNS name just like
	// ServerDnsNames. The per-tenant slot constraint (it must be the tenant's
	// own gw-<namespace>.<base-domain> when under the shared base domain) is
	// enforced in deriveGatewayHostname, where the namespace and base domain
	// are both available.
	if config.Route.Host != "" {
		if err := ValidateDNSName(config.Route.Host); err != nil {
			return fmt.Errorf("invalid route host: %w", err)
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

// ConfigValidationError marks a generated gateway configuration that must not be
// rolled out (for example one with no auth mechanism). The reconciler uses it to
// settle the gateway to Failed with a human-readable status reason, distinct
// from other reconcile errors. See
// specs/platform/generated-gateway-config-validation.spec.md.
type ConfigValidationError struct {
	Err error
}

func (e *ConfigValidationError) Error() string { return e.Err.Error() }

func (e *ConfigValidationError) Unwrap() error { return e.Err }

// ValidateGatewayAuth checks the generated OIDC configuration before rollout.
// The chart renders the [openshell.gateway.oidc] section from these values, so
// an empty issuer or audience would ship a gateway with no auth mechanism, and
// the gateway refuses to start without one (issue #809). The error is a
// ConfigValidationError so the gateway settles to Failed with a reason
// instead of crash-looping (GTW-CFG-01.04).
func ValidateGatewayAuth(config GatewayConfig) error {
	var missing []string
	if strings.TrimSpace(config.OIDC.Issuer) == "" {
		missing = append(missing, "issuer")
	}
	if strings.TrimSpace(config.OIDC.Audience) == "" {
		missing = append(missing, "audience")
	}
	if len(missing) > 0 {
		return &ConfigValidationError{Err: fmt.Errorf("gateway has no OIDC configuration (missing %s); every gateway requires OIDC", strings.Join(missing, ", "))}
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
	case "kek-db":
		// Explicit opt-in to the KEK-encrypted database store (the former default).
		// It carries no sub-fields; the control plane provisions the KEK Secret.
	case "vault", "":
		// Vault is the default backend (HYPERSHELL-369) and a BLACK BOX to users: the
		// control plane fills every coordinate from the per-cluster conventions + deployment
		// config (config.go). There are deliberately no per-gateway Vault overrides to
		// validate - a gateway cannot set its address/role/mount/auth - so nothing to check
		// here beyond the type itself.
	default:
		return fmt.Errorf("unsupported credential driver type %q; supported: vault, kubernetes-secrets, kek-db", config.Type)
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

// claimPathRegex matches a JWT claim reference: one or more dot-separated
// segments of letters, digits, and underscores. It accepts both top-level
// claims ("roles", "groups") and nested paths ("realm_access.roles",
// "hypershell.roles") that realms emit for role mapping.
var claimPathRegex = regexp.MustCompile(`^[a-zA-Z0-9_]+(\.[a-zA-Z0-9_]+)*$`)

func ValidateOIDCConfig(oidc OIDCConfig) error {
	if oidc.Issuer == "" {
		return nil
	}

	if (oidc.AdminRole != "") != (oidc.UserRole != "") {
		return fmt.Errorf("both admin_role and user_role must be set, or both must be empty")
	}

	// Format-validate roles_claim only when it is set. A blank roles_claim is left
	// to the gateway's own default (groups), so a BYO OIDC config that sets
	// admin_role/user_role and omits roles_claim keeps delegating to that default
	// rather than being rejected. Validation runs on the reconcile path
	// (ReconcileGateway), so hard-failing a pre-existing config here would break
	// its reconciliation with no migration. A claim the realm never emits still
	// can't be caught here without the token (P3-1); this only rejects a
	// syntactically invalid claim path.
	if oidc.RolesClaim != "" && !claimPathRegex.MatchString(oidc.RolesClaim) {
		return fmt.Errorf("invalid roles_claim %q: must be a dot-separated JWT claim path (e.g. \"roles\" or \"realm_access.roles\")", oidc.RolesClaim)
	}

	return nil
}
