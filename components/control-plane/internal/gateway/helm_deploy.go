package gateway

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/openshift-online/hypershell/components/control-plane/internal/helm"
)

// deployGatewayViaHelm installs or upgrades a gateway using the Helm chart.
func deployGatewayViaHelm(
	ctx context.Context,
	helmClient *helm.ShellClient,
	nsConfig NamespaceConfig,
	opts ReconcileOpts,
	hasTrustedCA bool,
	hasVaultCA bool,
) error {
	// Build Helm values from gateway configuration
	valuesBuilder := &helm.ValuesBuilder{
		Gateway: helm.GatewayConfig{
			Image:               nsConfig.Gateway.Image,
			SupervisorImage:     nsConfig.Gateway.SupervisorImage,
			SandboxRuntimeImage: nsConfig.Gateway.SandboxRuntimeImage,
			ServerDnsNames:      nsConfig.Gateway.ServerDnsNames,
			OIDC: helm.OIDCConfig{
				Issuer:      nsConfig.Gateway.OIDC.Issuer,
				Audience:    nsConfig.Gateway.OIDC.Audience,
				RolesClaim:  nsConfig.Gateway.OIDC.RolesClaim,
				AdminRole:   nsConfig.Gateway.OIDC.AdminRole,
				UserRole:    nsConfig.Gateway.OIDC.UserRole,
				ScopesClaim: nsConfig.Gateway.OIDC.ScopesClaim,
			},
			Route: helm.RouteConfig{
				Host:    nsConfig.Gateway.Route.Host,
				Enabled: nsConfig.Gateway.Route.Enabled,
			},
			CredentialDriver: convertCredentialDriver(nsConfig.Gateway.CredentialDriver, nsConfig.Name),
		},
		Namespace:      nsConfig.Name,
		HasCertManager: opts.HasCertManager,
		IsOpenShift:    opts.IsOpenShift,
		// Exposure follows the resolved ingress mode (the single source of truth
		// shared with the reconciler and health checker), NOT raw Gateway API
		// capability -- otherwise ROKS, which has the Gateway API CRDs but no
		// controller, would emit a dead GRPCRoute while the operator asked for a
		// Route. Route is opt-in (GATEWAY_INGRESS_MODE=route); every other mode
		// defaults to the Gateway API path.
		UseOpenShiftRoute:          gatewayIngressMode(opts) == IngressModeRoute,
		GatewayAPIGatewayName:      getGatewayAPIGatewayName(),
		GatewayAPIGatewayNamespace: getGatewayAPIGatewayNamespace(),
		IngressBaseDomain:          opts.IngressBaseDomain,
		ExternalCAIssuerName:       opts.ExternalCAIssuerName,
		ExternalCAIssuerKind:       opts.ExternalCAIssuerKind,
		HasTrustedCA:               hasTrustedCA,
		HasVaultCA:                 hasVaultCA,
		Resources:                  opts.GatewayResources,
		SandboxRuntimeClass:        opts.SandboxRuntimeClass,
		AllowDriverConfig:          opts.AllowDriverConfig,
	}

	values, err := valuesBuilder.Build()
	if err != nil {
		return fmt.Errorf("build helm values: %w", err)
	}

	// Check if Helm release exists
	status, err := helmClient.GetReleaseStatus(ctx, nsConfig.Name)
	if err != nil {
		return fmt.Errorf("check helm release status: %w", err)
	}

	// Install or upgrade the release
	if status == nil {
		// No release exists, install it
		log.Printf("INFO installing helm release in namespace %s", nsConfig.Name)
		if err := helmClient.Install(ctx, nsConfig.Name, values); err != nil {
			return fmt.Errorf("helm install: %w", err)
		}
	} else if status.Status == "failed" {
		// A previous install/upgrade ran to completion but with an error. Helm
		// allows retrying this via a normal upgrade.
		log.Printf("INFO retrying failed helm release in namespace %s (status: %s)", nsConfig.Name, status.Status)
		if err := helmClient.Upgrade(ctx, nsConfig.Name, values); err != nil {
			return fmt.Errorf("helm upgrade: %w", err)
		}
	} else if strings.HasPrefix(status.Status, "pending-") {
		// An operation (install/upgrade/rollback) was interrupted before reaching
		// a terminal state -- for example the controller restarted mid-install.
		// Helm refuses any new operation against a release in this state
		// ("another operation ... is in progress"), even though nothing is
		// actually running, so retrying via upgrade can never succeed and would
		// retry forever. The only clean recovery is to discard the stuck release
		// and reinstall; nothing the chart renders can have been left in a
		// working state while the release was still pending.
		log.Printf("INFO helm release in namespace %s is stuck (status: %s); reinstalling", nsConfig.Name, status.Status)
		if err := helmClient.Uninstall(ctx, nsConfig.Name); err != nil {
			return fmt.Errorf("uninstall stuck helm release: %w", err)
		}
		if err := helmClient.Install(ctx, nsConfig.Name, values); err != nil {
			return fmt.Errorf("reinstall helm release: %w", err)
		}
	} else if status.Status == "deployed" {
		log.Printf("INFO upgrading deployed helm release in namespace %s", nsConfig.Name)
		if err := helmClient.Upgrade(ctx, nsConfig.Name, values); err != nil {
			return fmt.Errorf("helm upgrade: %w", err)
		}
	} else {
		log.Printf("INFO upgrading helm release in namespace %s (current status: %s)", nsConfig.Name, status.Status)
		if err := helmClient.Upgrade(ctx, nsConfig.Name, values); err != nil {
			return fmt.Errorf("helm upgrade: %w", err)
		}
	}

	log.Printf("INFO helm release deployed in namespace %s", nsConfig.Name)
	return nil
}

// effectiveCredentialDriver resolves an unspecified credential_driver to the flat
// default, vault (HYPERSHELL-369). An explicit driver (non-empty Type) is returned
// unchanged, so a user who asks for a specific backend (kek-db / kubernetes-secrets)
// always gets it. Both a nil driver and an empty-type object (`credential_driver:
// {}`) count as unspecified and resolve to vault, preserving any supplied overrides.
//
// Prerequisite (see openshell-gateway-credentials.spec.md): every gateway-hosting
// cluster must run its per-cluster Vault before this default takes effect; a cluster
// without its Vault should pin new gateways to kek-db explicitly.
func effectiveCredentialDriver(driver *CredentialDriverConfig) *CredentialDriverConfig {
	if driver != nil && driver.Type != "" {
		return driver
	}
	return &CredentialDriverConfig{Type: "vault"}
}

// convertCredentialDriver converts gateway.CredentialDriverConfig to
// helm.CredentialDriverConfig, applying the deployment default for an omitted
// driver and resolving the Vault conventions for the vault driver. namespace is the
// gateway's tenant namespace, from which the per-tenant Vault mount is derived.
func convertCredentialDriver(driver *CredentialDriverConfig, namespace string) *helm.CredentialDriverConfig {
	driver = effectiveCredentialDriver(driver)
	if driver == nil {
		return nil
	}
	switch driver.Type {
	case "kubernetes-secrets":
		hc := &helm.CredentialDriverConfig{Type: "kubernetes-secrets"}
		if driver.KubernetesSecrets != nil {
			hc.KubernetesSecrets = &helm.KubernetesSecretsValues{Namespace: driver.KubernetesSecrets.Namespace}
		}
		return hc
	case "kek-db":
		return &helm.CredentialDriverConfig{Type: "kek-db"}
	case "vault", "":
		return &helm.CredentialDriverConfig{Type: "vault", Vault: resolveVaultValues(namespace)}
	default:
		return &helm.CredentialDriverConfig{Type: driver.Type}
	}
}

// resolveVaultValues fills the Vault coordinates purely from the per-cluster conventions
// (config.go) plus control-plane deployment config (GATEWAY_VAULT_* env). Vault is a black
// box to users: there are NO per-gateway overrides, so address/role/auth are fixed and the
// mount is always the gateway's own per-tenant mount openshell-gw/<namespace>. An empty
// namespace (probe callers that do not use the mount) leaves the mount unset.
func resolveVaultValues(namespace string) *helm.VaultValues {
	mount := ""
	if namespace != "" {
		mount = VaultMountForNamespace(namespace)
	}
	return &helm.VaultValues{
		Address:                 VaultAddressDefault(),
		Mount:                   mount,
		KVVersion:               VaultDefaultKVVersion,
		AuthMethod:              VaultDefaultAuthMethod,
		Role:                    VaultRoleDefault(),
		KubernetesAuthMount:     VaultDefaultKubernetesAuthMount,
		ServiceAccountTokenPath: VaultServiceAccountTokenPath,
		TokenAudience:           VaultServiceAccountTokenAudience,
		TokenDir:                VaultServiceAccountTokenDir,
		TokenExpirySeconds:      VaultServiceAccountTokenExpiry,
		CAConfigMapName:         VaultCATargetConfigMapName,
		TimeoutSecs:             VaultDefaultTimeoutSecs,
	}
}

// isVaultDriver reports whether the resolved credential driver is vault (an
// explicit `vault` type, or an empty-typed driver object which defaults to vault).
func isVaultDriver(driver *CredentialDriverConfig) bool {
	return driver != nil && (driver.Type == "vault" || driver.Type == "")
}

// getGatewayAPIGatewayName returns the name of the shared Gateway API Gateway resource.
func getGatewayAPIGatewayName() string {
	// Read from environment variable (required when Gateway API is available)
	// See specs/platform/openshell-gateway-routing.spec.md
	return getEnv("GATEWAY_API_GATEWAY_NAME", "")
}

// getGatewayAPIGatewayNamespace returns the namespace of the shared Gateway API Gateway resource.
func getGatewayAPIGatewayNamespace() string {
	// Read from environment variable
	return getEnv("GATEWAY_API_GATEWAY_NAMESPACE", "")
}

// getEnv retrieves an environment variable with a fallback default.
func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
