package gateway

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
