package urls

const (
	APIPrefix            = "/api/hypershell/v1"
	GatewaysPath         = APIPrefix + "/gateways"
	GatewayNetworksPath  = APIPrefix + "/gateway_networks"
	GatewayProfilesPath  = APIPrefix + "/gateway_profiles"
	GatewayReleasesPath  = APIPrefix + "/gateway_releases"
	ManagedClustersPath  = APIPrefix + "/managed_clusters"
	ManagedDatabasesPath = APIPrefix + "/managed_databases"
	RolesPath            = APIPrefix + "/roles"
	RoleBindingsPath     = APIPrefix + "/role_bindings"
)

func GatewayPath(id string) string {
	return GatewaysPath + "/" + id
}

func GatewayNetworkPath(id string) string {
	return GatewayNetworksPath + "/" + id
}

func GatewayProfilePath(id string) string {
	return GatewayProfilesPath + "/" + id
}

func GatewayReleasePath(id string) string {
	return GatewayReleasesPath + "/" + id
}

func ManagedClusterPath(id string) string {
	return ManagedClustersPath + "/" + id
}

func ManagedDatabasePath(id string) string {
	return ManagedDatabasesPath + "/" + id
}

func RolePath(id string) string {
	return RolesPath + "/" + id
}

func RoleBindingPath(id string) string {
	return RoleBindingsPath + "/" + id
}
