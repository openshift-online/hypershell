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
	DefaultSandboxImage() string
	DefaultConsoleImage() string
	DefaultOAuth2ProxyImage() string
}

const defaultSandboxImage = "ghcr.io/nvidia/openshell-community/sandboxes/base:latest"

// defaultConsoleImage is the OpenShell dashboard image (the per-gateway
// console). The upstream project publishes it to quay.io, so clusters pull it
// directly (imagePullPolicy IfNotPresent) rather than building from source.
// Pinned by digest to the sha-978bcb5 build for reproducibility; bump
// deliberately when adopting a new dashboard contract. Overridable via
// HYPERSHELL_CONSOLE_IMAGE (e.g. a platform-registry mirror in production).
const defaultConsoleImage = "quay.io/gkrumbach07/openshell-dashboard@sha256:c69c1f34c574556684710a7d2d2a3654f164b855efe0d273a098782447068fc5"

// defaultOAuth2ProxyImage is the oauth2-proxy sidecar image. Overridable via
// HYPERSHELL_OAUTH2_PROXY_IMAGE.
const defaultOAuth2ProxyImage = "quay.io/oauth2-proxy/oauth2-proxy:v7.7.1"

type StaticImageDefaults struct{}

// DefaultGatewayImage resolves the gateway server (and certgen) image used when
// a Gateway resource does not specify one. Must be set via GATEWAY_IMAGE environment
// variable; reconciliation will fail if not provided.
func (StaticImageDefaults) DefaultGatewayImage() string {
	return os.Getenv("GATEWAY_IMAGE")
}

// DefaultSupervisorImage resolves the supervisor sidecar image used when a
// Gateway resource does not specify one. Must be set via GATEWAY_SUPERVISOR_IMAGE environment
// variable; reconciliation will fail if not provided.
func (StaticImageDefaults) DefaultSupervisorImage() string {
	return os.Getenv("GATEWAY_SUPERVISOR_IMAGE")
}

// DefaultSandboxImage resolves the base image tenant sandbox pods launch from.
// It is overridable via GATEWAY_SANDBOX_IMAGE so clusters whose nodes cannot
// reach ghcr.io (e.g. IBM ROKS) can point it at an in-cluster registry mirror.
func (StaticImageDefaults) DefaultSandboxImage() string {
	if v := os.Getenv("GATEWAY_SANDBOX_IMAGE"); v != "" {
		return v
	}
	return defaultSandboxImage
}

func (StaticImageDefaults) DefaultConsoleImage() string {
	if v := os.Getenv("HYPERSHELL_CONSOLE_IMAGE"); v != "" {
		return v
	}
	return defaultConsoleImage
}

func (StaticImageDefaults) DefaultOAuth2ProxyImage() string {
	if v := os.Getenv("HYPERSHELL_OAUTH2_PROXY_IMAGE"); v != "" {
		return v
	}
	return defaultOAuth2ProxyImage
}

type NamespaceConfig struct {
	Name    string        `yaml:"name"`
	Gateway GatewayConfig `yaml:"gateway"`
}

type GatewayConfig struct {
	Image string `yaml:"image"`
	// ReleaseID is the GatewayRelease the Image was resolved from (empty for a
	// direct-image gateway). It is stamped onto the gateway Deployment as an
	// annotation at apply time so the health loop can advance observed_release_id
	// only to the release actually applied to the workload, never to a desired
	// release the provisioning path has not yet rolled out. See
	// gateway-release-rollout.spec.md.
	ReleaseID        string                  `yaml:"releaseID"`
	SupervisorImage  string                  `yaml:"supervisorImage"`
	ServerDnsNames   []string                `yaml:"serverDnsNames"`
	ExternalDns      string                  `yaml:"externalDns"`
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

// RouteAddressUpdater is called by the gateway reconciler to update the
// route_address field on the API-server Gateway resource.  The implementation
// is provided by the top-level reconciler which owns the gRPC connection.
type RouteAddressUpdater func(ctx context.Context, routeAddress string) error

// ConsoleAddressUpdater is called by the gateway reconciler to update the
// console_address field on the API-server Gateway resource. The implementation
// is provided by the top-level reconciler which owns the gRPC connection.
type ConsoleAddressUpdater func(ctx context.Context, consoleAddress string) error

// KeycloakConfig holds Keycloak Admin REST API connection parameters read
// from the hypershell-keycloak-admin Secret in the control-plane namespace.
type KeycloakConfig struct {
	ServerURL    string
	Realm        string
	ClientID     string
	ClientSecret string
}

type ReconcileOpts struct {
	IsOpenShift    bool
	HasCertManager bool
	HasGatewayAPI  bool
	// Database locates the mounted PostgreSQL admin credentials every gateway
	// database is provisioned with (see database.go).
	Database DatabaseConfig
	// databaseReconciler, when set, replaces the DatabaseReconciler built from
	// Database. It is a test seam for exercising provisioning and deletion
	// without a PostgreSQL server; production wiring leaves it nil.
	databaseReconciler    DatabaseReconciler
	ControlPlaneNamespace string
	Images                ImageDefaults
	// GatewayID is the API-server resource ID for the gateway being reconciled.
	// Used when updating fields (e.g. routeAddress) back to the API server.
	GatewayID string
	// UpdateRouteAddress is an optional callback that PATCHes the route_address
	// field on the API-server Gateway.  Nil means no update will be attempted.
	UpdateRouteAddress RouteAddressUpdater
	// UpdateConsoleAddress is an optional callback that PATCHes the console_address
	// field on the API-server Gateway. Nil means no update will be attempted.
	UpdateConsoleAddress ConsoleAddressUpdater
	// Keycloak holds the Keycloak Admin REST API configuration. Nil means
	// Keycloak integration is not configured.
	Keycloak *KeycloakConfig
	// UpdateOIDC is an optional callback that PATCHes the oidc field on the
	// API-server Gateway via gRPC.
	UpdateOIDC func(ctx context.Context, oidcJSON string) error
	// GatewayName is the user-visible name of the gateway being reconciled.
	GatewayName string
	// GatewayClientID is the validated identity recorded during provisioning.
	GatewayClientID string
	// KeycloakClient is a Keycloak Admin REST API client for cleanup operations.
	// Used during gateway deletion to remove the Keycloak OIDC client.
	KeycloakClient KeycloakClientAPI
	// Exposure is the Gateway Exposure port used to resolve the external route
	// address. Nil when no exposure backend is configured (e.g. clusters without
	// the Gateway API), in which case no route address is published.
	Exposure exposure.Port
	// RouteStillDesired, when set, reports whether the Gateway is still routed
	// according to its current API-server record (false if it has since been
	// un-routed or deleted). The provisioning path calls it after the potentially
	// long TLS-secret wait, before creating the remaining route- and console-owned
	// resources, so an in-flight pass does not recreate them behind a concurrent
	// health-loop teardown. Nil disables the re-check (the pass proceeds).
	RouteStillDesired func(ctx context.Context) (bool, error)
	// ReportProgress is called at provisioning step boundaries to report
	// condition transitions. Nil means no reporting (progress is silently
	// skipped).
	ReportProgress ProgressReporter
	// RecordOrphan, when set, records a durable, operator-visible signal that a
	// gateway-owned resource was left unreclaimed during deletion with no
	// automatic recovery path (e.g. a Keycloak client that could not be deleted
	// because Keycloak was unavailable). It exists so a best-effort cleanup
	// failure is never a silent orphan. Nil disables recording (the failure is
	// only logged, matching legacy behavior). Implementations must not include
	// secrets in any argument.
	RecordOrphan OrphanRecorder
	// ExternalCAIssuerName is the name of the cert-manager issuer for externally trusted certificates.
	// Required for Route passthrough mode (TLS termination at pod, client sees cert directly).
	ExternalCAIssuerName string
	// ExternalCAIssuerKind is the kind of the external CA issuer (ClusterIssuer or Issuer).
	ExternalCAIssuerKind string
	// IngressBaseDomain is the base domain for auto-derived ingress hostnames (e.g. apps.example.com).
	IngressBaseDomain string
}

// OrphanRecorder records a durable, operator-visible signal that a gateway-owned
// resource was left behind during deletion with no automatic recovery path.
// resourceKind and resourceName identify the leaked resource; reason explains
// why it was not reclaimed. Implementations must be best-effort and must never
// carry secrets.
type OrphanRecorder func(ctx context.Context, resourceKind, resourceName, reason string)

// KeycloakClientAPI is the subset of keycloak.Client needed by the gateway package.
type KeycloakClientAPI interface {
	DeleteGatewayServiceAccountClients(ctx context.Context, gatewayID string) error
	DeleteGatewayClient(ctx context.Context, gatewayName string) error
	DeleteConsoleClient(ctx context.Context, consoleClientID string) error
}
