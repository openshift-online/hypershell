package gateways

import "fmt"

// Database provider values for DATABASE_PROVIDER. ProviderDeployment is the
// default (unset or empty DATABASE_PROVIDER resolves to it): the API server
// auto-creates a dedicated deployment-backed ManagedDatabase per gateway
// (deploymentPlacement) and needs no CNPG APIs. ProviderCNPG selects
// CNPG-backed placement (cnpgPlacement) and resolves database_id against the
// sole existing ManagedDatabase with provider "cnpg". ProviderExternal selects
// external-server placement (externalPlacement): database_id is resolved
// against the sole existing ManagedDatabase with provider "external". Exactly
// one external ManagedDatabase must exist; create-gateway is rejected when zero
// or more than one are registered.
const (
	ProviderDeployment = "deployment"
	ProviderCNPG       = "cnpg"
	ProviderExternal   = "external"
)

// resolveDatabaseProvider validates a raw DATABASE_PROVIDER environment
// value read at gateway-service construction time (server startup). Unset or
// empty resolves to ProviderDeployment; any value other than "deployment",
// "cnpg", or "external" is a startup configuration error, never an implicit
// fallback.
func resolveDatabaseProvider(raw string) (string, error) {
	switch raw {
	case "", ProviderDeployment:
		return ProviderDeployment, nil
	case ProviderCNPG:
		return ProviderCNPG, nil
	case ProviderExternal:
		return ProviderExternal, nil
	default:
		return "", fmt.Errorf("invalid DATABASE_PROVIDER %q: must be %q, %q, or %q (unset defaults to %q)",
			raw, ProviderCNPG, ProviderDeployment, ProviderExternal, ProviderDeployment)
	}
}
