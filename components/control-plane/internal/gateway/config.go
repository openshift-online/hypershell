package gateway

import (
	"context"
	"os"

	"github.com/openshift-online/hypershell/components/control-plane/internal/exposure"
)

// ImageDefaults resolves the default container images for gateway deployments.
// TODO: Replace StaticImageDefaults with a database-backed implementation that
// reads from a versions table + gatewayVersion join table, so defaults can be
// updated dynamically without downtime and vary by region/group/fleet.
type ImageDefaults interface {
	DefaultGatewayImage() string
	DefaultSupervisorImage() string
	DefaultDatabaseImage() string
	DefaultSandboxImage() string
}

const defaultDatabaseImage = "postgres:18"
const defaultSandboxImage = "ghcr.io/nvidia/openshell-community/sandboxes/base:latest"

type StaticImageDefaults struct{}

const defaultGatewayImage = "ghcr.io/nvidia/openshell/gateway:0.0.106"
const defaultSupervisorImage = "ghcr.io/nvidia/openshell/supervisor:0.0.106"

// DefaultGatewayImage resolves the gateway server (and certgen) image used when
// a Gateway resource does not specify one. Overridable via GATEWAY_IMAGE so
// clusters whose nodes cannot reach ghcr.io (e.g. IBM ROKS) can point it at an
// in-cluster registry mirror, mirroring the GATEWAY_SANDBOX_IMAGE override.
func (StaticImageDefaults) DefaultGatewayImage() string {
	if v := os.Getenv("GATEWAY_IMAGE"); v != "" {
		return v
	}
	return defaultGatewayImage
}

// DefaultSupervisorImage resolves the supervisor sidecar image used when a
// Gateway resource does not specify one. Overridable via GATEWAY_SUPERVISOR_IMAGE
// for the same ghcr.io-unreachable clusters as DefaultGatewayImage.
func (StaticImageDefaults) DefaultSupervisorImage() string {
	if v := os.Getenv("GATEWAY_SUPERVISOR_IMAGE"); v != "" {
		return v
	}
	return defaultSupervisorImage
}

func (StaticImageDefaults) DefaultDatabaseImage() string {
	if v := os.Getenv("HYPERSHELL_DATABASE_IMAGE"); v != "" {
		return v
	}
	return defaultDatabaseImage
}

// DefaultSandboxImage resolves the base image tenant sandbox pods launch from.
// It is overridable via GATEWAY_SANDBOX_IMAGE so clusters whose nodes cannot
// reach ghcr.io (e.g. IBM ROKS) can point it at an in-cluster registry mirror,
// mirroring the HYPERSHELL_DATABASE_IMAGE override for the gateway database.
func (StaticImageDefaults) DefaultSandboxImage() string {
	if v := os.Getenv("GATEWAY_SANDBOX_IMAGE"); v != "" {
		return v
	}
	return defaultSandboxImage
}

type NamespaceConfig struct {
	Name    string        `yaml:"name"`
	Gateway GatewayConfig `yaml:"gateway"`
}

type GatewayConfig struct {
	Image            string                  `yaml:"image"`
	SupervisorImage  string                  `yaml:"supervisorImage"`
	ServerDnsNames   []string                `yaml:"serverDnsNames"`
	ExternalDns      string                  `yaml:"externalDns"`
	Database         DatabaseConfig          `yaml:"database"`
	OIDC             OIDCConfig              `yaml:"oidc"`
	Route            RouteConfig             `yaml:"route"`
	CredentialDriver *CredentialDriverConfig `yaml:"credentialDriver"`
}

type CredentialDriverConfig struct {
	Type              string                   `yaml:"type" json:"type"`
	KubernetesSecrets *KubernetesSecretsConfig `yaml:"kubernetes_secrets,omitempty" json:"kubernetes_secrets,omitempty"`
	Vault             *VaultCredentialConfig   `yaml:"vault,omitempty" json:"vault,omitempty"`
}

type KubernetesSecretsConfig struct {
	Namespace string `yaml:"namespace,omitempty" json:"namespace,omitempty"`
}

type VaultCredentialConfig struct {
	Address             string `yaml:"address" json:"address"`
	Mount               string `yaml:"mount,omitempty" json:"mount,omitempty"`
	AuthMethod          string `yaml:"auth_method,omitempty" json:"auth_method,omitempty"`
	Role                string `yaml:"role" json:"role"`
	KubernetesAuthMount string `yaml:"kubernetes_auth_mount,omitempty" json:"kubernetes_auth_mount,omitempty"`
	TimeoutSecs         int    `yaml:"timeout_secs,omitempty" json:"timeout_secs,omitempty"`
}

type RouteConfig struct {
	Host    string `yaml:"host" json:"host,omitempty"`
	Enabled bool   `yaml:"enabled" json:"enabled,omitempty"`
}

type OIDCConfig struct {
	Issuer string `yaml:"issuer" json:"issuer,omitempty"`
	// ClientID is client-facing metadata (the per-gateway Keycloak clientId,
	// formatted as {name}-{id}) that the console and CLI need for `openshell gateway
	// add`. It is surfaced on the Gateway resource but not written to gateway.toml,
	// since the gateway server validates issuer and audience, not client id.
	ClientID    string `yaml:"client_id" json:"client_id,omitempty"`
	Audience    string `yaml:"audience" json:"audience,omitempty"`
	JwksTTL     int    `yaml:"jwks_ttl" json:"jwks_ttl,omitempty"`
	RolesClaim  string `yaml:"roles_claim" json:"roles_claim,omitempty"`
	AdminRole   string `yaml:"admin_role" json:"admin_role,omitempty"`
	UserRole    string `yaml:"user_role" json:"user_role,omitempty"`
	ScopesClaim string `yaml:"scopes_claim" json:"scopes_claim,omitempty"`
}

type DatabaseConfig struct {
	StorageSize       string `yaml:"storageSize" json:"storage_size,omitempty"`
	Image             string `yaml:"image" json:"image,omitempty"`
	ExternalSecretRef string `yaml:"externalSecretRef" json:"external_secret_ref,omitempty"`
}

// RouteAddressUpdater is called by the gateway reconciler to update the
// route_address field on the API-server Gateway resource.  The implementation
// is provided by the top-level reconciler which owns the gRPC connection.
type RouteAddressUpdater func(ctx context.Context, routeAddress string) error

// KeycloakConfig holds Keycloak Admin REST API connection parameters read
// from the hypershell-keycloak-admin Secret in the control-plane namespace.
type KeycloakConfig struct {
	ServerURL    string
	Realm        string
	ClientID     string
	ClientSecret string
}

type ReconcileOpts struct {
	IsOpenShift           bool
	HasCertManager        bool
	HasGatewayAPI         bool
	ControlPlaneNamespace string
	Images                ImageDefaults
	// SkipNetworkPolicies disables creation of the per-tenant gateway
	// NetworkPolicies. On distributions where the shared Gateway data plane
	// runs out-of-cluster (e.g. cloud-provider-kind's Envoy container in local
	// dev), its source IP cannot be matched by the namespace/pod selectors
	// these policies use, so the policies would blackhole all ingress to the
	// gateway. Dev clusters set this true to skip them; production/OpenShift
	// leaves it false (the zero value) to keep tenant isolation enforced.
	SkipNetworkPolicies bool
	// GatewayID is the API-server resource ID for the gateway being reconciled.
	// Used when updating fields (e.g. routeAddress) back to the API server.
	GatewayID string
	// UpdateRouteAddress is an optional callback that PATCHes the route_address
	// field on the API-server Gateway.  Nil means no update will be attempted.
	UpdateRouteAddress RouteAddressUpdater
	// RotateDBCredentials is the value of the hypershell.redhat.io/rotate-db-credentials
	// annotation on the Gateway resource. Empty means no rotation requested.
	RotateDBCredentials string
	// Keycloak holds the Keycloak Admin REST API configuration. Nil means
	// Keycloak integration is not configured.
	Keycloak *KeycloakConfig
	// UpdateOIDC is an optional callback that PATCHes the oidc field on the
	// API-server Gateway via gRPC.
	UpdateOIDC func(ctx context.Context, oidcJSON string) error
	// GatewayName is the user-visible name of the gateway being reconciled.
	GatewayName string
	// KeycloakClient is a Keycloak Admin REST API client for cleanup operations.
	// Used during gateway deletion to remove the Keycloak OIDC client.
	KeycloakClient KeycloakClientAPI
	// Exposure is the Gateway Exposure port used to resolve the external route
	// address. Nil when no exposure backend is configured (e.g. clusters without
	// the Gateway API), in which case no route address is published.
	Exposure exposure.Port
}

// KeycloakClientAPI is the subset of keycloak.Client needed by the gateway package.
type KeycloakClientAPI interface {
	DeleteGatewayClient(ctx context.Context, gatewayName string) error
}
