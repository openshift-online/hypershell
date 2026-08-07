package gateway

// ImageDefaults resolves the default container images for gateway deployments.
// TODO: Replace StaticImageDefaults with a database-backed implementation that
// reads from a versions table + gatewayVersion join table, so defaults can be
// updated dynamically without downtime and vary by region/group/fleet.
type ImageDefaults interface {
	DefaultGatewayImage() string
	DefaultSupervisorImage() string
}

type StaticImageDefaults struct{}

func (StaticImageDefaults) DefaultGatewayImage() string {
	return "ghcr.io/nvidia/openshell/gateway:0.0.101"
}

func (StaticImageDefaults) DefaultSupervisorImage() string {
	return "ghcr.io/nvidia/openshell/supervisor:0.0.101"
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

type ReconcileOpts struct {
	IsOpenShift           bool
	HasCertManager        bool
	HasGatewayAPI         bool
	ControlPlaneNamespace string
	Images                ImageDefaults
}
