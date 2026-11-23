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

	PlatformGatewayPoliciesPath = ExtAPIPrefix + "/platform_gateway_policies"
	// PlatformPolicySettingsPath is the singleton global enforcement toggle
	// (PLAT-DATA-29): GET reads it, PATCH {"mode":"audit|enforce"} changes it.
	PlatformPolicySettingsPath = ExtAPIPrefix + "/platform_policy_settings"

	GatewaysPlacementAvailabilityPath = GatewaysPath + "/placement-availability"
)

func AgentRuntimePath(id string) string { return AgentRuntimesPath + "/" + id }

func SandboxTemplatePath(id string) string { return SandboxTemplatesPath + "/" + id }

func ProviderSpecPath(id string) string { return ProviderSpecsPath + "/" + id }

func ProviderBindingPath(id string) string { return ProviderBindingsPath + "/" + id }

func InferenceRoutePath(id string) string { return InferenceRoutesPath + "/" + id }

func SecretSourcePath(id string) string { return SecretSourcesPath + "/" + id }

// SecretSourceMaterialPath is the write-only material-upload sub-resource of a
// SecretSource (PLAT-DATA-27): POST writes the durable material through the
// api-server to the source's server-derived convention path.
func SecretSourceMaterialPath(id string) string { return SecretSourcesPath + "/" + id + "/material" }

func RepositoryPath(id string) string { return RepositoriesPath + "/" + id }

func CIProviderPath(id string) string { return CIProvidersPath + "/" + id }

func CITrustPath(id string) string { return CITrustsPath + "/" + id }

func PlatformGatewayPolicyPath(id string) string { return PlatformGatewayPoliciesPath + "/" + id }

// GatewayEffectivePoliciesPath is the read-only effective-policies view of one
// gateway (PLAT-PGP-19). It sits under the core /v1/ gateways collection, but
// the generator emits only standard resource collection paths, so it is
// hand-maintained here like the placement-availability path.
func GatewayEffectivePoliciesPath(gatewayID string) string {
	return GatewayPath(gatewayID) + "/effective_policies"
}
