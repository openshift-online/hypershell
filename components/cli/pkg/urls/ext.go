package urls

// Extension kinds (AgentRuntime and friends) are served under /api/hypershell/ext/,
// not /api/hypershell/v1/. The rh-trex-ai CLI generator handles a single API
// prefix, so their paths are hand-maintained here; urls.go holds the generated
// core paths.
const (
	ExtAPIPrefix         = "/api/hypershell/ext"
	AgentRuntimesPath    = ExtAPIPrefix + "/agent_runtimes"
	SandboxTemplatesPath = ExtAPIPrefix + "/sandbox_templates"
	ProviderSpecsPath    = ExtAPIPrefix + "/provider_specs"
	ProviderBindingsPath = ExtAPIPrefix + "/provider_bindings"
	InferenceRoutesPath  = ExtAPIPrefix + "/inference_routes"
	SecretSourcesPath    = ExtAPIPrefix + "/secret_sources"
	RepositoriesPath     = ExtAPIPrefix + "/repositories"
)

func AgentRuntimePath(id string) string { return AgentRuntimesPath + "/" + id }

func SandboxTemplatePath(id string) string { return SandboxTemplatesPath + "/" + id }

func ProviderSpecPath(id string) string { return ProviderSpecsPath + "/" + id }

func ProviderBindingPath(id string) string { return ProviderBindingsPath + "/" + id }

func InferenceRoutePath(id string) string { return InferenceRoutesPath + "/" + id }

func SecretSourcePath(id string) string { return SecretSourcesPath + "/" + id }

func RepositoryPath(id string) string { return RepositoriesPath + "/" + id }
