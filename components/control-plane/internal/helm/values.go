package helm

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/distribution/reference"
	corev1 "k8s.io/api/core/v1"
)

// gatewayNamespacePattern matches API-assigned gateway namespaces
// (openshell-{16-hex}, the hex encoding of the first eight bytes of the
// gateway ksuid payload, assigned at create time).
var gatewayNamespacePattern = regexp.MustCompile(`^openshell-([0-9a-f]{16})$`)

// ManagedGatewayID derives the OpenShell gateway_id for a managed-mode gateway
// provisioned into the given namespace. The chart's default gateway_id is the
// release-pinned fullname, which is identical for every gateway, so managed
// per-workspace namespaces (openshell-{gateway_id}-{workspace}) would collide
// across gateways on one cluster. Using the hexadecimal suffix of the
// API-assigned namespace makes the id unique per gateway, keeps the derived
// namespace within DNS-1123 bounds, and lets the control plane attribute
// managed namespaces back to their gateway (see
// gateway.SandboxGatewayNamespace).
func ManagedGatewayID(namespace string) (string, error) {
	m := gatewayNamespacePattern.FindStringSubmatch(namespace)
	if m == nil {
		return "", fmt.Errorf("managed workspace mode requires an API-assigned gateway namespace of the form openshell-{16-hex}, got %q", namespace)
	}
	return m[1], nil
}

// GatewayConfig represents the configuration for a gateway deployment.
// This is a local copy to avoid import cycles with the gateway package.
type GatewayConfig struct {
	Image               string
	SupervisorImage     string
	SandboxRuntimeImage string
	ServerDnsNames      []string
	OIDC                OIDCConfig
	Route               RouteConfig
	CredentialDriver    *CredentialDriverConfig
	// WorkspaceMode selects how the OpenShell Kubernetes driver maps workspaces
	// to namespaces (shared/managed/operator). Empty omits the Helm value so the
	// chart default (shared) applies (legacy gateways). "managed" also derives a
	// per-gateway unique gateway_id from the namespace.
	WorkspaceMode string
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
	Host string
}

// CredentialDriverConfig represents credential driver configuration. Type is the
// resolved driver (`vault`, `kubernetes-secrets`, or `kek-db`); the matching
// sub-struct carries the resolved coordinates when set.
type CredentialDriverConfig struct {
	Type              string
	KubernetesSecrets *KubernetesSecretsValues
	Vault             *VaultValues
}

// KubernetesSecretsValues holds resolved Kubernetes Secrets driver values.
type KubernetesSecretsValues struct {
	Namespace string
}

// VaultValues holds the resolved Vault credential-driver coordinates the control
// plane fills from conventions (HYPERSHELL-369). They map to the chart's
// server.credentialDrivers.vault.* values and the projected vault-audience token.
type VaultValues struct {
	Address                 string
	Mount                   string
	KVVersion               string
	AuthMethod              string
	Role                    string
	KubernetesAuthMount     string
	ServiceAccountTokenPath string
	TokenAudience           string
	TokenDir                string
	TokenExpirySeconds      int
	CAConfigMapName         string
	TimeoutSecs             int
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
	// GatewayAPIGatewayName is the name of the shared Gateway resource
	GatewayAPIGatewayName string
	// GatewayAPIGatewayNamespace is the namespace of the shared Gateway resource
	GatewayAPIGatewayNamespace string
	// IngressBaseDomain is the base domain for ingress routes
	IngressBaseDomain string
	// HasTrustedCA indicates whether the gateway-trusted-ca ConfigMap exists
	HasTrustedCA bool
	// AdditionalCAConfigMapName supplies destination TLS roots to sandbox supervisors.
	AdditionalCAConfigMapName string
	// HasVaultCA indicates whether the per-tenant openshell-gateway-vault-ca
	// ConfigMap (the mirrored Vault serving CA) exists, so the vault driver's
	// caConfigMapName is set only when the gateway can actually mount it.
	HasVaultCA bool
	// Resources overrides the gateway container requests and limits
	// (GATEWAY_RESOURCES). Nil uses DefaultGatewayResources.
	Resources *corev1.ResourceRequirements
	// SandboxRuntimeClass sets server.defaultRuntimeClassName in the Helm chart,
	// applying the named Kubernetes RuntimeClass to all sandbox pods provisioned
	// by this gateway. Empty string omits the field (cluster default, typically runc).
	// Set to "kata" or "kata-remote" to run sandboxes inside lightweight VMs.
	// For an operator-mode gateway it is overridden by OperatorSandboxRuntimeClass.
	SandboxRuntimeClass string
	// OperatorNamespaceLabelKey is the administrator-approved label KEY the external
	// operator stamps on an operator-mode gateway's tenant namespaces. The driver's
	// operatorNamespaceLabel selector is built as <key>=<namespace> so a gateway
	// discovers only its own tenant namespaces (OSG-OPWS-02). Operator mode only.
	OperatorWorkspace         *OperatorWorkspaceConfig
	OperatorNamespaceLabelKey string
	// OperatorSandboxRuntimeClass is the administrator-approved sandbox RuntimeClass
	// applied to server.defaultRuntimeClassName for operator-mode gateways ONLY
	// (OSG-OPWS-03), overriding any cluster-wide SandboxRuntimeClass.
	OperatorSandboxRuntimeClass string
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
	if err := b.buildIngressValues(values); err != nil {
		return nil, fmt.Errorf("build ingress values: %w", err)
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

	if b.AdditionalCAConfigMapName != "" {
		setNestedValue(values, b.AdditionalCAConfigMapName, "supervisor", "network", "additionalCaConfigMapName")
	}

	// Image values. The chart uses <image>.image.{registry,repository,tag,digest}
	// and prepends global.image.registry (ghcr.io/nvidia) when the per-image
	// registry is unset. We always pass fully-qualified image references from the
	// gateway's resolved image, so we split registry out and set it explicitly
	// per-image to prevent the global prefix from being applied. A configured
	// digest is preserved so the chart renders the exact pinned content
	// (OSG-HELM-05); a malformed reference is a validation error rather than a
	// silently dropped or replaced reference.
	if b.Gateway.Image != "" {
		if err := setImageValues(values, b.Gateway.Image, "gateway", "image"); err != nil {
			return fmt.Errorf("gateway image: %w", err)
		}
	}

	if b.Gateway.SupervisorImage != "" {
		if err := setImageValues(values, b.Gateway.SupervisorImage, "supervisor", "image"); err != nil {
			return fmt.Errorf("supervisor image: %w", err)
		}
	}

	// The sandbox runtime and supervisor speak a versioned boundary protocol, so
	// the runtime must be pinned to the same OpenShell build as the supervisor.
	// Left unset, the chart defaults to a moving upstream tag.
	if b.Gateway.SandboxRuntimeImage != "" {
		if err := setImageValues(values, b.Gateway.SandboxRuntimeImage, "sandboxRuntime", "image"); err != nil {
			return fmt.Errorf("sandbox runtime image: %w", err)
		}
	}

	// Workload configuration
	setNestedValue(values, "deployment", "workload", "kind")
	setNestedValue(values, 1, "replicaCount")
	// Keep the gateway workload requests and limits explicit.
	setNestedValue(values, map[string]interface{}{
		"requests": map[string]interface{}{"cpu": "100m", "memory": "256Mi"},
		"limits":   map[string]interface{}{"cpu": "500m", "memory": "512Mi"},
	}, "resources")

	// Gateway container resources. Always set: the upstream chart defaults to
	// `resources: {}`, which would run the gateway BestEffort.
	resources := DefaultGatewayResources()
	if b.Resources != nil {
		resources = *b.Resources
	}
	setNestedValue(values, resourcesValue(resources), "resources")

	// Sandbox configuration
	setNestedValue(values, b.Namespace, "server", "sandboxNamespace")
	if b.SandboxRuntimeClass != "" {
		setNestedValue(values, b.SandboxRuntimeClass, "server", "defaultRuntimeClassName")
	}

	// Workspace mode is always passed: every stored gateway carries one, so the
	// chart default never decides it. Managed mode also pins a per-gateway unique
	// gateway_id so per-workspace namespaces (openshell-{gateway_id}-{workspace})
	// never collide across gateways on one cluster (openshell-gateway.spec.md,
	// "Gateway Workspace Mode").
	if b.Gateway.WorkspaceMode == "" {
		return fmt.Errorf("gateway workspace mode is required")
	}
	setNestedValue(values, b.Gateway.WorkspaceMode, "server", "drivers", "kubernetes", "workspaceMode")
	switch b.Gateway.WorkspaceMode {
	case "managed":
		gatewayID, err := ManagedGatewayID(b.Namespace)
		if err != nil {
			return err
		}
		setNestedValue(values, gatewayID, "server", "sandboxJwt", "gatewayId")
	case "operator":
		// Operator mode: the external operator labels each tenant namespace with the
		// administrator-approved key set to this gateway's own (unique) namespace
		// name, so the driver discovers only this gateway's namespaces and two
		// gateways never resolve to the same selector (OSG-OPWS-02). Both the label
		// key and the runtime class are administrator-owned, never caller input. The
		// operator sandbox RuntimeClass applies to operator gateways ONLY, overriding
		// any cluster-wide SandboxRuntimeClass set above (OSG-OPWS-03). An operator
		// gateway deliberately gets NO managed sandboxJwt.gatewayId block.
		setNestedValue(values, fmt.Sprintf("%s=%s", b.OperatorNamespaceLabelKey, b.Namespace),
			"server", "drivers", "kubernetes", "operatorNamespaceLabel")
		setNestedValue(values, b.OperatorSandboxRuntimeClass, "server", "defaultRuntimeClassName")
	}

	if b.Gateway.WorkspaceMode == "operator" {
		if b.OperatorWorkspace == nil {
			return fmt.Errorf("operator workspace mode requires GATEWAY_OPERATOR_WORKSPACE")
		}
		setNestedValue(values, false, "workspaceResources", "enabled")
		// Wider operator access requires explicit administrator opt-in.
		setNestedValue(values, b.OperatorWorkspace.AllowDriverConfig, "server", "drivers", "kubernetes", "allowDriverConfig")
		setNestedValue(values, !b.OperatorWorkspace.DisableResourceAdmission, "server", "drivers", "kubernetes", "resourceAdmission", "enabled")
		setNestedValue(values, b.OperatorWorkspace.AllowedPodLabels, "gatewayConfig", "openshell.drivers.kubernetes", "allowed_pod_labels")
		setNestedValue(values, b.OperatorWorkspace.MTLSAuth, "gatewayConfig", "openshell.gateway.mtls_auth", "enabled")
	}

	// Server DNS names for TLS certificate SANs
	dnsNames := b.Gateway.ServerDnsNames
	// Operator clients reach sandbox services through OpenShell's internal routing domain.
	if b.Gateway.WorkspaceMode == "operator" && !slices.Contains(dnsNames, "*.openshell.localhost") {
		dnsNames = append(slices.Clone(dnsNames), "*.openshell.localhost")
	}
	if len(dnsNames) > 0 {
		setNestedValue(values, dnsNames, "pkiInitJob", "serverDnsNames")
		if b.HasCertManager {
			// When cert-manager is enabled the chart ignores pkiInitJob.serverDnsNames:
			// server_sans in gateway.toml and the internal server Certificate both
			// come from certManager.serverDnsNames instead. Set both keys from the
			// same source.
			setNestedValue(values, dnsNames, "certManager", "serverDnsNames")
		}
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

// buildCredentialDriverValues builds credential driver Helm chart values. The
// chart reads these under server.credentialDrivers.* (not top-level). When neither
// external driver is enabled the chart provisions the KEK-encrypted database store.
func (b *ValuesBuilder) buildCredentialDriverValues(values map[string]interface{}) {
	driver := b.Gateway.CredentialDriver
	if driver == nil {
		// No driver: the chart provisions the KEK store. The gateway package resolves an
		// omitted credential_driver to vault before it builds values, so only direct
		// callers reach this branch.
		setNestedValue(values, false, "server", "credentialDrivers", "kubernetesSecrets", "enabled")
		setNestedValue(values, false, "server", "credentialDrivers", "vault", "enabled")
		return
	}

	switch driver.Type {
	case "kubernetes-secrets":
		setNestedValue(values, true, "server", "credentialDrivers", "kubernetesSecrets", "enabled")
		if driver.KubernetesSecrets != nil && driver.KubernetesSecrets.Namespace != "" {
			setNestedValue(values, driver.KubernetesSecrets.Namespace, "server", "credentialDrivers", "kubernetesSecrets", "namespace")
		}
	case "kek-db":
		// Explicit KEK-encrypted database store: neither external driver enabled.
		setNestedValue(values, false, "server", "credentialDrivers", "kubernetesSecrets", "enabled")
		setNestedValue(values, false, "server", "credentialDrivers", "vault", "enabled")
	case "vault", "":
		b.buildVaultValues(values, driver.Vault)
	}
}

// buildVaultValues wires the resolved Vault coordinates into the chart and injects
// a projected ServiceAccount token scoped to the `vault` audience (least-privilege;
// the chart's default is the generic SA token) via the extraVolumes hooks.
func (b *ValuesBuilder) buildVaultValues(values map[string]interface{}, v *VaultValues) {
	if v == nil {
		return
	}
	setNestedValue(values, true, "server", "credentialDrivers", "vault", "enabled")
	setNestedValue(values, v.Address, "server", "credentialDrivers", "vault", "address")
	setNestedValue(values, v.Mount, "server", "credentialDrivers", "vault", "mount")
	setNestedValue(values, v.KVVersion, "server", "credentialDrivers", "vault", "kvVersion")
	setNestedValue(values, v.AuthMethod, "server", "credentialDrivers", "vault", "authMethod")
	setNestedValue(values, v.Role, "server", "credentialDrivers", "vault", "role")
	setNestedValue(values, v.KubernetesAuthMount, "server", "credentialDrivers", "vault", "kubernetesAuthMount")
	setNestedValue(values, v.ServiceAccountTokenPath, "server", "credentialDrivers", "vault", "serviceAccountTokenPath")
	if v.TimeoutSecs > 0 {
		setNestedValue(values, v.TimeoutSecs, "server", "credentialDrivers", "vault", "timeoutSecs")
	}
	// Point the chart at the mirrored CA ConfigMap only when the mirror succeeded,
	// mirroring the trusted-CA-bundle pattern. When absent the gateway falls back to
	// platform trust roots (Wave 3 adds a fail-closed reachability check).
	if b.HasVaultCA && v.CAConfigMapName != "" {
		setNestedValue(values, v.CAConfigMapName, "server", "credentialDrivers", "vault", "caConfigMapName")
	}

	tokenVolume := map[string]interface{}{
		"name": "vault-sa-token",
		"projected": map[string]interface{}{
			"sources": []interface{}{
				map[string]interface{}{
					"serviceAccountToken": map[string]interface{}{
						"path":              "token",
						"audience":          v.TokenAudience,
						"expirationSeconds": int64(v.TokenExpirySeconds),
					},
				},
			},
		},
	}
	tokenMount := map[string]interface{}{
		"name":      "vault-sa-token",
		"mountPath": v.TokenDir,
		"readOnly":  true,
	}
	appendNestedSlice(values, []interface{}{tokenVolume}, "server", "extraVolumes")
	appendNestedSlice(values, []interface{}{tokenMount}, "server", "extraVolumeMounts")
}

// appendNestedSlice appends items to the []interface{} at path, creating the path
// if absent and preserving anything already there. server.extraVolumes /
// extraVolumeMounts are a shared extension point (Helm replaces arrays rather than
// merging), so writers must accumulate rather than assign, or the last writer wins
// and silently drops the others.
func appendNestedSlice(m map[string]interface{}, items []interface{}, path ...string) {
	if len(path) == 0 || len(items) == 0 {
		return
	}
	current := m
	for i := 0; i < len(path)-1; i++ {
		next, ok := current[path[i]].(map[string]interface{})
		if !ok {
			next = make(map[string]interface{})
			current[path[i]] = next
		}
		current = next
	}
	key := path[len(path)-1]
	existing, _ := current[key].([]interface{})
	current[key] = append(existing, items...)
}

// buildIngressValues builds ingress-related Helm chart values.
func (b *ValuesBuilder) buildIngressValues(values map[string]interface{}) error {
	// Kubernetes Gateway API mode. The reconciler creates the GRPCRoute,
	// BackendTLSPolicy, and backend-ca ConfigMap after the helm release is
	// installed, so the chart's own grpcRoute and backendTLSPolicy templates stay
	// disabled (defaults from buildCoreValues). Operator clients use mTLS directly;
	// OpenShell also accepts bearer-only OIDC through ingress.
	setNestedValue(values, b.Gateway.WorkspaceMode == "operator", "server", "tls", "enableMtls")
	return nil
}

// buildOpenShiftValues builds OpenShift-specific Helm chart values.
func (b *ValuesBuilder) buildOpenShiftValues(values map[string]interface{}) {
	// Let SCC assign fsGroup and runAsUser
	setNestedValue(values, nil, "podSecurityContext", "fsGroup")
	setNestedValue(values, nil, "securityContext", "runAsUser")
}

// parsedImage holds a container image reference split into the parts the chart
// consumes under <image>.image.{registry,repository,tag,digest}.
type parsedImage struct {
	registry   string
	repository string
	tag        string
	digest     string
}

// parseImageRef parses a container image reference into its registry,
// repository, optional tag and optional digest, preserving a configured digest
// (OSG-HELM-05). It uses the canonical distribution/reference parser so registry
// ports (registry.example:5000/repo) and tag-plus-digest references are handled
// correctly. The registry is the reference domain; when the reference carries no
// domain the registry is empty and the chart applies global.image.registry. A
// malformed reference is returned as an error so the caller reports a validation
// error instead of silently dropping or replacing parts of the reference.
func parseImageRef(image string) (parsedImage, error) {
	ref, err := reference.Parse(strings.TrimSpace(image))
	if err != nil {
		return parsedImage{}, fmt.Errorf("invalid image reference %q: %w", image, err)
	}
	named, ok := ref.(reference.Named)
	if !ok {
		return parsedImage{}, fmt.Errorf("invalid image reference %q: missing repository name", image)
	}

	parsed := parsedImage{
		registry:   reference.Domain(named),
		repository: reference.Path(named),
	}
	if tagged, ok := ref.(reference.Tagged); ok {
		parsed.tag = tagged.Tag()
	}
	if digested, ok := ref.(reference.Digested); ok {
		parsed.digest = digested.Digest().String()
	}
	return parsed, nil
}

// setImageValues parses an image reference and writes its parts under the given
// value path prefix (for example "gateway", "image"). A configured digest is
// preserved so the chart renders the exact pinned content; the chart image
// helpers give image.digest priority over image.tag (see the openshell.image
// helper in charts/openshell/templates/_helpers.tpl), so the rendered reference
// uses the digest when both a tag and a digest are present. A digest-only
// reference sets only the digest and never falls back to a tag or "latest"; a
// reference with neither a tag nor a digest defaults to the "latest" tag.
// A malformed reference is returned as an error (OSG-HELM-05).
func setImageValues(values map[string]interface{}, image string, prefix ...string) error {
	parsed, err := parseImageRef(image)
	if err != nil {
		return err
	}

	path := func(leaf string) []string {
		p := make([]string, 0, len(prefix)+1)
		p = append(p, prefix...)
		return append(p, leaf)
	}

	setNestedValue(values, parsed.registry, path("registry")...)
	setNestedValue(values, parsed.repository, path("repository")...)
	if parsed.digest != "" {
		setNestedValue(values, parsed.digest, path("digest")...)
	}
	switch {
	case parsed.tag != "":
		setNestedValue(values, parsed.tag, path("tag")...)
	case parsed.digest == "":
		// Neither tag nor digest: default to "latest" (Docker semantics) rather
		// than letting the chart fall back to its app version.
		setNestedValue(values, "latest", path("tag")...)
	}
	return nil
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
