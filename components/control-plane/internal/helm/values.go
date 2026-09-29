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
	// UseOpenShiftRoute selects the OpenShift Route (HAProxy passthrough)
	// exposure path instead of the default Kubernetes Gateway API path. It is
	// derived from the resolved ingress mode (GATEWAY_INGRESS_MODE), NOT from raw
	// Gateway API capability: IBM Cloud ROKS ships the Gateway API CRDs but runs
	// no functional controller, so the emitted exposure resource must follow the
	// operator-selected mode, not whichever CRDs happen to be installed.
	UseOpenShiftRoute bool
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
	// HasTrustedCA indicates whether the gateway-trusted-ca ConfigMap exists
	HasTrustedCA bool
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
	// The upstream chart name is "helm-chart", not "openshell", so the default
	// fullname (release-name + chart-name) would produce "openshell-gateway-helm-chart".
	// Pin fullnameOverride so every chart resource uses the expected name.
	setNestedValue(values, ReleaseName, "fullnameOverride")

	// Image values
	if b.Gateway.Image != "" {
		repo, tag := splitImageRef(b.Gateway.Image)
		setNestedValue(values, repo, "image", "repository")
		setNestedValue(values, tag, "image", "tag")
	}

	if b.Gateway.SupervisorImage != "" {
		repo, tag := splitImageRef(b.Gateway.SupervisorImage)
		setNestedValue(values, repo, "supervisor", "image", "repository")
		setNestedValue(values, tag, "supervisor", "image", "tag")
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

	// NetworkPolicy disabled; see openshell-gateway-helm-adoption.spec.md.
	setNestedValue(values, false, "networkPolicy", "enabled")
	setNestedValue(values, true, "supervisor", "sandboxRuntime", "networkPolicyEnforced")

	// GRPCRoute defaults -- the chart template accesses
	// grpcRoute.backendTLSPolicy.enabled unconditionally, so we must
	// always provide the key even when grpcRoute itself is disabled.
	setNestedValue(values, false, "grpcRoute", "enabled")
	setNestedValue(values, false, "grpcRoute", "backendTLSPolicy", "enabled")

	// cert-manager configuration
	setNestedValue(values, b.HasCertManager, "certManager", "enabled")

	// Database configuration
	setNestedValue(values, "openshell-gateway-db-credentials", "server", "externalDbSecret")

	// Trusted CA ConfigMap - only set when OIDC is configured and the ConfigMap was actually copied
	if b.HasTrustedCA && b.Gateway.OIDC.Issuer != "" {
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

	if !b.UseOpenShiftRoute {
		// Default: Kubernetes Gateway API mode. The reconciler creates the
		// GRPCRoute, BackendTLSPolicy, and backend-ca ConfigMap after the helm
		// release is installed, so the chart's own grpcRoute and backendTLSPolicy
		// templates stay disabled (defaults from buildCoreValues). Only mTLS must
		// be disabled because the Gateway proxy cannot present client certificates.
		setNestedValue(values, false, "server", "tls", "enableMtls")
		return nil
	}

	// Route passthrough mode: opt-in via GATEWAY_INGRESS_MODE=route (e.g. IBM
	// Cloud ROKS, whose Ingress Operator owns the Gateway API CRDs but runs no
	// functional controller). The chart owns the Route resource via
	// openshiftRoute.enabled; the reconciler still handles publishRouteAddress
	// and the router NetworkPolicy.
	setNestedValue(values, true, "openshiftRoute", "enabled")
	setNestedValue(values, hostname, "openshiftRoute", "host")

	annotations := map[string]string{
		"haproxy.router.openshift.io/timeout": "3600s",
	}
	setNestedValue(values, annotations, "openshiftRoute", "annotations")

	// The external CA issuer is optional. When set, cert-manager issues the
	// gateway server certificate from it, covering the Route host (the chart's
	// route template validates that coverage). When unset, the gateway keeps its
	// self-signed per-tenant CA (pkiInitJob), whose SANs already include the
	// Route host (the reconciler injects it into ServerDnsNames before install);
	// passthrough forwards the encrypted stream by SNI, so no router-trusted
	// certificate is required. Only wire cert-manager when an issuer was actually
	// provided, so the self-signed passthrough path is not forced to fail.
	if b.ExternalCAIssuerName != "" {
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
// The chart template joins repo:tag with a colon, so @sha256:... digest
// suffixes are stripped. When both tag and digest are present
// (image:tag@sha256:...), the tag is retained. Handles registry ports
// (registry:5000/image) by comparing colon position to last slash.
func splitImageRef(image string) (repo, tag string) {
	if at := strings.LastIndex(image, "@"); at != -1 {
		image = image[:at]
	}
	lastSlash := strings.LastIndex(image, "/")
	lastColon := strings.LastIndex(image, ":")
	if lastColon <= lastSlash {
		return image, "latest"
	}
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
		next, ok := current[key].(map[string]interface{})
		if !ok {
			next = make(map[string]interface{})
			current[key] = next
		}
		current = next
	}

	current[path[len(path)-1]] = value
}
