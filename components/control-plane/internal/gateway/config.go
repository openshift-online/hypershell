package gateway

// TODO: Move default image versions into the hypershell database so they can be
// updated dynamically without downtime and vary by region/group. Use a versions
// table for each OpenShell release and a gatewayVersion join table to associate
// gateways with specific version sets.
const (
	DefaultGatewayImage    = "ghcr.io/nvidia/openshell/gateway:0.0.101"
	DefaultSupervisorImage = "ghcr.io/nvidia/openshell/supervisor:0.0.101"
)

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
}
