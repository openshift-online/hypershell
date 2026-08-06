package gateway

type NamespaceConfig struct {
	Name    string        `yaml:"name"`
	Gateway GatewayConfig `yaml:"gateway"`
}

type GatewayConfig struct {
	Image          string         `yaml:"image"`
	ServerDnsNames []string       `yaml:"serverDnsNames"`
	ExternalDns    string         `yaml:"externalDns"`
	Database       DatabaseConfig `yaml:"database"`
	OIDC           OIDCConfig     `yaml:"oidc"`
	Route          RouteConfig    `yaml:"route"`
}

type RouteConfig struct {
	Host    string `yaml:"host"`
	Enabled bool   `yaml:"enabled"`
}

type OIDCConfig struct {
	Issuer      string `yaml:"issuer"`
	Audience    string `yaml:"audience"`
	JwksTTL     int    `yaml:"jwks_ttl"`
	RolesClaim  string `yaml:"roles_claim"`
	AdminRole   string `yaml:"admin_role"`
	UserRole    string `yaml:"user_role"`
	ScopesClaim string `yaml:"scopes_claim"`
}

type DatabaseConfig struct {
	StorageSize string `yaml:"storageSize"`
	Image       string `yaml:"image"`
}

type ReconcileOpts struct {
	IsOpenShift           bool
	HasCertManager        bool
	HasGatewayAPI         bool
	ControlPlaneNamespace string
}
