package helm

import (
	"fmt"
	"strings"
)

// GatewayConfig represents the configuration for a gateway deployment.
// This is a local copy to avoid import cycles with the gateway package.
type GatewayConfig struct {
	Image            string
	SupervisorImage  string
	ServerDnsNames   []string
	OIDC             OIDCConfig
	Route            RouteConfig
	CredentialDriver *CredentialDriverConfig
}

// OIDCConfig represents OIDC configuration.
type OIDCConfig struct {
	Issuer      string
	Audience    string
	RolesClaim  string
	AdminRole   string
	UserRole    string
	ScopesClaim string
}

// RouteConfig represents routing configuration.
type RouteConfig struct {
	Host    string
	Enabled bool
}

// CredentialDriverConfig represents credential driver configuration.
type CredentialDriverConfig struct {
	Type string
}

// ValuesBuilder builds Helm chart values from a Gateway configuration.
type ValuesBuilder struct {
	// Gateway configuration
	Gateway GatewayConfig
	// Namespace where the gateway is deployed
	Namespace string
	// HasCertManager indicates whether cert-manager is available on the cluster
	HasCertManager bool
	// IsOpenShift indicates whether the cluster is OpenShift
	IsOpenShift bool
	// HasGatewayAPI indicates whether Gateway API is available on the cluster
	HasGatewayAPI bool
	// GatewayAPIGatewayName is the name of the shared Gateway resource
	GatewayAPIGatewayName string
	// GatewayAPIGatewayNamespace is the namespace of the shared Gateway resource
	GatewayAPIGatewayNamespace string
	// IngressBaseDomain is the base domain for ingress routes
	IngressBaseDomain string
	// ExternalCAIssuerName is the name of the external CA issuer for Route passthrough mode
	ExternalCAIssuerName string
	// ExternalCAIssuerKind is the kind of the external CA issuer (ClusterIssuer or Issuer)
	ExternalCAIssuerKind string
}

// Build computes Helm chart values from the Gateway configuration.
// It returns a map[string]interface{} suitable for passing to Helm install/upgrade.
func (b *ValuesBuilder) Build() (map[string]interface{}, error) {
	values := make(map[string]interface{})

	// Core values
	if err := b.buildCoreValues(values); err != nil {
		return nil, fmt.Errorf("build core values: %w", err)
	}

	// OIDC values (conditional)
	if b.Gateway.OIDC.Issuer != "" {
		b.buildOIDCValues(values)
	}

	// Credential driver values (conditional)
	b.buildCredentialDriverValues(values)

	// Ingress values (conditional)
	if b.Gateway.Route.Enabled {
		if err := b.buildIngressValues(values); err != nil {
			return nil, fmt.Errorf("build ingress values: %w", err)
		}
	}

	// OpenShift values (conditional)
	if b.IsOpenShift {
		b.buildOpenShiftValues(values)
	}

	return values, nil
}

// buildCoreValues builds the core Helm chart values.
func (b *ValuesBuilder) buildCoreValues(values map[string]interface{}) error {
	// Image values
	if b.Gateway.Image != "" {
		repo, tag := splitImageRef(b.Gateway.Image)
		setNestedValue(values, repo, "image", "repository")
		setNestedValue(values, tag, "image", "tag")
	}

	if b.Gateway.SupervisorImage != "" {
		repo, tag := splitImageRef(b.Gateway.SupervisorImage)
		setNestedValue(values, repo, "supervisorImage", "repository")
		setNestedValue(values, tag, "supervisorImage", "tag")
	}

	// Workload configuration
	setNestedValue(values, "deployment", "workload", "kind")
	setNestedValue(values, 1, "replicaCount")

	// Sandbox configuration
	setNestedValue(values, b.Namespace, "server", "sandboxNamespace")

	// Server DNS names for TLS certificate SANs
	if len(b.Gateway.ServerDnsNames) > 0 {
		setNestedValue(values, b.Gateway.ServerDnsNames, "pkiInitJob", "serverDnsNames")
	}

	// ServiceAccount configuration
	setNestedValue(values, true, "serviceAccount", "create")
	setNestedValue(values, true, "sandboxServiceAccount", "create")

	// NetworkPolicy disabled (see spec decision)
	setNestedValue(values, false, "networkPolicy", "enabled")

	// GRPCRoute defaults -- the fork chart template accesses
	// grpcRoute.backendTLSPolicy.enabled unconditionally, so we must
	// always provide the key even when grpcRoute itself is disabled.
	setNestedValue(values, false, "grpcRoute", "enabled")
	setNestedValue(values, false, "grpcRoute", "backendTLSPolicy", "enabled")

	// cert-manager configuration
	setNestedValue(values, b.HasCertManager, "certManager", "enabled")

	// Database configuration
	setNestedValue(values, "openshell-gateway-db-credentials", "server", "externalDbSecret")

	// Trusted CA ConfigMap (optional - set when OIDC is configured)
	if b.Gateway.OIDC.Issuer != "" {
		setNestedValue(values, "gateway-trusted-ca", "server", "oidc", "caConfigMapName")
	}

	return nil
}

// buildOIDCValues builds OIDC-related Helm chart values.
func (b *ValuesBuilder) buildOIDCValues(values map[string]interface{}) {
	oidc := b.Gateway.OIDC
	if oidc.Issuer != "" {
		setNestedValue(values, oidc.Issuer, "server", "oidc", "issuer")
	}
	if oidc.Audience != "" {
		setNestedValue(values, oidc.Audience, "server", "oidc", "audience")
	}
	if oidc.RolesClaim != "" {
		setNestedValue(values, oidc.RolesClaim, "server", "oidc", "rolesClaim")
	}
	if oidc.AdminRole != "" {
		setNestedValue(values, oidc.AdminRole, "server", "oidc", "adminRole")
	}
	if oidc.UserRole != "" {
		setNestedValue(values, oidc.UserRole, "server", "oidc", "userRole")
	}
	if oidc.ScopesClaim != "" {
		setNestedValue(values, oidc.ScopesClaim, "server", "oidc", "scopesClaim")
	}
}

// buildCredentialDriverValues builds credential driver Helm chart values.
func (b *ValuesBuilder) buildCredentialDriverValues(values map[string]interface{}) {
	if b.Gateway.CredentialDriver == nil {
		// KEK mode (default)
		setNestedValue(values, false, "credentialDrivers", "kubernetesSecrets", "enabled")
		return
	}

	driver := b.Gateway.CredentialDriver
	switch driver.Type {
	case "kubernetes-secrets":
		setNestedValue(values, true, "credentialDrivers", "kubernetesSecrets", "enabled")
	case "vault":
		setNestedValue(values, true, "credentialDrivers", "vault", "enabled")
	}
}

// buildIngressValues builds ingress-related Helm chart values.
func (b *ValuesBuilder) buildIngressValues(values map[string]interface{}) error {
	route := b.Gateway.Route

	// Derive hostname
	hostname := fmt.Sprintf("gw-%s.%s", b.Namespace, b.IngressBaseDomain)
	if route.Host != "" {
		hostname = route.Host
	}

	if b.HasGatewayAPI {
		// GRPCRoute + BackendTLSPolicy mode (Gateway API)
		setNestedValue(values, true, "grpcRoute", "enabled")
		setNestedValue(values, []string{hostname}, "grpcRoute", "hostnames")
		setNestedValue(values, b.GatewayAPIGatewayName, "grpcRoute", "gateway", "name")
		setNestedValue(values, b.GatewayAPIGatewayNamespace, "grpcRoute", "gateway", "namespace")

		// BackendTLSPolicy (requires PR #2728)
		setNestedValue(values, true, "grpcRoute", "backendTLSPolicy", "enabled")
		// Disable mTLS when BackendTLSPolicy is used (per spec)
		setNestedValue(values, false, "server", "tls", "enableMtls")
	} else {
		// Route passthrough mode (OpenShift < 4.22)
		setNestedValue(values, true, "openshiftRoute", "enabled")
		setNestedValue(values, hostname, "openshiftRoute", "host")

		// HAProxy timeout annotation
		annotations := map[string]string{
			"haproxy.router.openshift.io/timeout": "3600s",
		}
		setNestedValue(values, annotations, "openshiftRoute", "annotations")

		// External CA issuer for Route passthrough mode
		if b.ExternalCAIssuerName == "" {
			return fmt.Errorf("EXTERNAL_CA_ISSUER_NAME is required for Route passthrough mode")
		}
		setNestedValue(values, b.ExternalCAIssuerName, "certManager", "serverIssuerRef", "name")
		setNestedValue(values, b.ExternalCAIssuerKind, "certManager", "serverIssuerRef", "kind")
		setNestedValue(values, []string{hostname}, "certManager", "serverDnsNames")
	}

	return nil
}

// buildOpenShiftValues builds OpenShift-specific Helm chart values.
func (b *ValuesBuilder) buildOpenShiftValues(values map[string]interface{}) {
	// Let SCC assign fsGroup and runAsUser
	setNestedValue(values, nil, "podSecurityContext", "fsGroup")
	setNestedValue(values, nil, "securityContext", "runAsUser")
}

// splitImageRef splits an image reference into repository and tag.
// If no tag is present, it defaults to "latest".
func splitImageRef(image string) (repo, tag string) {
	parts := strings.Split(image, ":")
	if len(parts) == 1 {
		return parts[0], "latest"
	}
	// Handle the case where the last ':' separates the tag
	lastColon := strings.LastIndex(image, ":")
	return image[:lastColon], image[lastColon+1:]
}

// setNestedValue sets a value in a nested map structure.
// It creates intermediate maps as needed.
func setNestedValue(m map[string]interface{}, value interface{}, path ...string) {
	if len(path) == 0 {
		return
	}

	current := m
	for i := 0; i < len(path)-1; i++ {
		key := path[i]
		if _, ok := current[key]; !ok {
			current[key] = make(map[string]interface{})
		}
		current = current[key].(map[string]interface{})
	}

	current[path[len(path)-1]] = value
}
