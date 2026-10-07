package urls

const (
	APIPrefix           = "/api/hypershell/v1"
	GatewaysPath        = APIPrefix + "/gateways"
	ManagedClustersPath = APIPrefix + "/managed_clusters"
	RolesPath           = APIPrefix + "/roles"
	RoleBindingsPath    = APIPrefix + "/role_bindings"
	UsersPath           = APIPrefix + "/users"

	ExtAPIPrefix         = "/api/hypershell/ext"
	AgentRuntimesPath    = ExtAPIPrefix + "/agent_runtimes"
	SandboxTemplatesPath = ExtAPIPrefix + "/sandbox_templates"
	ProviderSpecsPath    = ExtAPIPrefix + "/provider_specs"
	ProviderBindingsPath = ExtAPIPrefix + "/provider_bindings"
	InferenceRoutesPath  = ExtAPIPrefix + "/inference_routes"
	SecretSourcesPath    = ExtAPIPrefix + "/secret_sources"
)

func GatewayPath(id string) string { return GatewaysPath + "/" + id }

func ManagedClusterPath(id string) string { return ManagedClustersPath + "/" + id }

func RolePath(id string) string { return RolesPath + "/" + id }

func RoleBindingPath(id string) string { return RoleBindingsPath + "/" + id }

func UserPath(id string) string { return UsersPath + "/" + id }

func AgentRuntimePath(id string) string { return AgentRuntimesPath + "/" + id }

func SandboxTemplatePath(id string) string { return SandboxTemplatesPath + "/" + id }

func ProviderSpecPath(id string) string { return ProviderSpecsPath + "/" + id }

func ProviderBindingPath(id string) string { return ProviderBindingsPath + "/" + id }

func InferenceRoutePath(id string) string { return InferenceRoutesPath + "/" + id }

func SecretSourcePath(id string) string { return SecretSourcesPath + "/" + id }
