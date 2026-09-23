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
) error {
	// Build Helm values from gateway configuration
	valuesBuilder := &helm.ValuesBuilder{
		Gateway: helm.GatewayConfig{
			Image:           nsConfig.Gateway.Image,
			SupervisorImage: nsConfig.Gateway.SupervisorImage,
			ServerDnsNames:  nsConfig.Gateway.ServerDnsNames,
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

// convertCredentialDriver converts gateway.CredentialDriverConfig to helm.CredentialDriverConfig.
func convertCredentialDriver(driver *CredentialDriverConfig) *helm.CredentialDriverConfig {
	if driver == nil {
		return nil
	}
	return &helm.CredentialDriverConfig{
		Type: driver.Type,
	}
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
