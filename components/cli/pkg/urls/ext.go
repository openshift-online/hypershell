package urls

// Extension kinds (AgentRuntime and friends) are served under /api/hypershell/ext/,
// not /api/hypershell/v1/. The rh-trex-ai CLI generator handles a single API
// prefix, so their paths are hand-maintained here; urls.go holds the generated
// core paths.
//
// The gateway placement-availability endpoint is also hand-maintained here:
// the generator emits only standard resource collection paths from the
// OpenAPI spec (urls.go), and this action-style sub-path is not one.
const (
	ExtAPIPrefix         = "/api/hypershell/ext"
	AgentRuntimesPath    = ExtAPIPrefix + "/agent_runtimes"
	SandboxTemplatesPath = ExtAPIPrefix + "/sandbox_templates"
	ProviderSpecsPath    = ExtAPIPrefix + "/provider_specs"
	ProviderBindingsPath = ExtAPIPrefix + "/provider_bindings"
	InferenceRoutesPath  = ExtAPIPrefix + "/inference_routes"
	SecretSourcesPath    = ExtAPIPrefix + "/secret_sources"
	RepositoriesPath     = ExtAPIPrefix + "/repositories"
	CIProvidersPath      = ExtAPIPrefix + "/ci_providers"
	CITrustsPath         = ExtAPIPrefix + "/ci_trusts"

	GatewaysPlacementAvailabilityPath = GatewaysPath + "/placement-availability"
)

func AgentRuntimePath(id string) string { return AgentRuntimesPath + "/" + id }

func SandboxTemplatePath(id string) string { return SandboxTemplatesPath + "/" + id }

func ProviderSpecPath(id string) string { return ProviderSpecsPath + "/" + id }

func ProviderBindingPath(id string) string { return ProviderBindingsPath + "/" + id }

func InferenceRoutePath(id string) string { return InferenceRoutesPath + "/" + id }

func SecretSourcePath(id string) string { return SecretSourcesPath + "/" + id }

func RepositoryPath(id string) string { return RepositoriesPath + "/" + id }

func CIProviderPath(id string) string { return CIProvidersPath + "/" + id }

func CITrustPath(id string) string { return CITrustsPath + "/" + id }
