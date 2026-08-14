package gateway

import (
	"context"
	"os"
)

// ImageDefaults resolves the default container images for gateway deployments.
// TODO: Replace StaticImageDefaults with a database-backed implementation that
// reads from a versions table + gatewayVersion join table, so defaults can be
// updated dynamically without downtime and vary by region/group/fleet.
type ImageDefaults interface {
	DefaultGatewayImage() string
	DefaultSupervisorImage() string
	DefaultDatabaseImage() string
}

const defaultDatabaseImage = "postgres:18"

type StaticImageDefaults struct{}

func (StaticImageDefaults) DefaultGatewayImage() string {
	return "ghcr.io/nvidia/openshell/gateway:0.0.101"
}

func (StaticImageDefaults) DefaultSupervisorImage() string {
	return "ghcr.io/nvidia/openshell/supervisor:0.0.101"
}

func (StaticImageDefaults) DefaultDatabaseImage() string {
	if v := os.Getenv("HYPERSHELL_DATABASE_IMAGE"); v != "" {
		return v
	}
	return defaultDatabaseImage
}

type NamespaceConfig struct {
	Name    string        `yaml:"name"`
	Gateway GatewayConfig `yaml:"gateway"`
}

type GatewayConfig struct {
	Image           string         `yaml:"image"`
	SupervisorImage string         `yaml:"supervisorImage"`
	ServerDnsNames  []string       `yaml:"serverDnsNames"`
	ExternalDns     string         `yaml:"externalDns"`
	Database        DatabaseConfig `yaml:"database"`
	OIDC            OIDCConfig     `yaml:"oidc"`
	Route           RouteConfig    `yaml:"route"`
}

type RouteConfig struct {
	Host    string `yaml:"host" json:"host,omitempty"`
	Enabled bool   `yaml:"enabled" json:"enabled,omitempty"`
}

type OIDCConfig struct {
	Issuer      string `yaml:"issuer" json:"issuer,omitempty"`
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
}

// KeycloakClientAPI is the subset of keycloak.Client needed by the gateway package.
type KeycloakClientAPI interface {
	DeleteGatewayClient(ctx context.Context, gatewayName string) error
}
