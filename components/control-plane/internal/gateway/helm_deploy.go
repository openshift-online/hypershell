package gateway

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/openshift-online/hypershell/components/control-plane/internal/helm"
)

// deployGatewayViaHelm installs or upgrades a gateway using the Helm chart.
func deployGatewayViaHelm(
	ctx context.Context,
	helmClient *helm.ShellClient,
	nsConfig NamespaceConfig,
	opts ReconcileOpts,
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
		Namespace:                  nsConfig.Name,
		HasCertManager:             opts.HasCertManager,
		IsOpenShift:                opts.IsOpenShift,
		HasGatewayAPI:              opts.HasGatewayAPI,
		GatewayAPIGatewayName:      getGatewayAPIGatewayName(),
		GatewayAPIGatewayNamespace: getGatewayAPIGatewayNamespace(),
		IngressBaseDomain:          opts.IngressBaseDomain,
		ExternalCAIssuerName:       opts.ExternalCAIssuerName,
		ExternalCAIssuerKind:       opts.ExternalCAIssuerKind,
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
	} else if status.Status == "failed" || status.Status == "pending-install" {
		// Failed install, retry via upgrade
		log.Printf("INFO retrying failed helm release in namespace %s (status: %s)", nsConfig.Name, status.Status)
		if err := helmClient.Upgrade(ctx, nsConfig.Name, values); err != nil {
			return fmt.Errorf("helm upgrade: %w", err)
		}
	} else if status.Status == "deployed" {
		log.Printf("INFO helm release already deployed in namespace %s, skipping", nsConfig.Name)
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
	if value := getEnvHelper(key); value != "" {
		return value
	}
	return fallback
}

// getEnvHelper retrieves environment variables (uses os package).
func getEnvHelper(key string) string {
	return os.Getenv(key)
}
