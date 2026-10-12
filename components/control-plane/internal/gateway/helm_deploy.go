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
			CredentialDriver: convertCredentialDriver(nsConfig.Gateway.CredentialDriver),
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

// defaultCredentialDriverType returns the driver an omitted credential_driver
// resolves to, selected per-deployment via GATEWAY_DEFAULT_CREDENTIAL_DRIVER.
//
// It defaults to "kek-db", which preserves today's behavior on every cluster that
// has not opted in - so dev, kind, CI and clusters without a provider Vault keep
// working unchanged. hyp0 sets "vault" on its control-plane (in hypershell-gitops)
// during the HYPERSHELL-369 rollout, so it is the first cluster to default new
// gateways to Vault. This selector is TRANSITIONAL: once every cluster has a
// provider Vault, Vault becomes the flat default and this knob is removed (see
// openshell-gateway-credentials.spec.md).
func defaultCredentialDriverType() string {
	if os.Getenv("GATEWAY_DEFAULT_CREDENTIAL_DRIVER") == "vault" {
		return "vault"
	}
	return "kek-db"
}

// effectiveCredentialDriver applies the deployment default to an omitted
// credential_driver. A non-nil (explicitly requested) driver is returned
// unchanged, so a user who asks for a specific backend always gets it.
func effectiveCredentialDriver(driver *CredentialDriverConfig) *CredentialDriverConfig {
	if driver != nil {
		return driver
	}
	if defaultCredentialDriverType() == "vault" {
		return &CredentialDriverConfig{Type: "vault"}
	}
	return nil // kek-db: the chart provisions the KEK store when no driver is set
}

// convertCredentialDriver converts gateway.CredentialDriverConfig to
// helm.CredentialDriverConfig, applying the deployment default for an omitted
// driver and resolving the Vault conventions for the vault driver.
func convertCredentialDriver(driver *CredentialDriverConfig) *helm.CredentialDriverConfig {
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
		return &helm.CredentialDriverConfig{Type: "vault", Vault: resolveVaultValues(driver.Vault)}
	default:
		return &helm.CredentialDriverConfig{Type: driver.Type}
	}
}

// resolveVaultValues fills the Vault coordinates from the per-cluster conventions
// (config.go), applying any per-gateway overrides in credential_driver.vault.
func resolveVaultValues(o *VaultCredentialConfig) *helm.VaultValues {
	v := &helm.VaultValues{
		Address:                 VaultAddressDefault(),
		Mount:                   VaultDefaultMount,
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
	if o != nil {
		if o.Address != "" {
			v.Address = o.Address
		}
		if o.Mount != "" {
			v.Mount = o.Mount
		}
		if o.AuthMethod != "" {
			v.AuthMethod = o.AuthMethod
		}
		if o.Role != "" {
			v.Role = o.Role
		}
		if o.KubernetesAuthMount != "" {
			v.KubernetesAuthMount = o.KubernetesAuthMount
		}
		if o.TimeoutSecs != 0 {
			v.TimeoutSecs = o.TimeoutSecs
		}
	}
	return v
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
