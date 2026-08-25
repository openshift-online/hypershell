package gateways

import "fmt"

// Database provider values for DATABASE_PROVIDER. ProviderDeployment is the
// default (unset or empty DATABASE_PROVIDER resolves to it): the API server
// auto-creates a dedicated deployment-backed ManagedDatabase per gateway
// (deploymentPlacement) and needs no CNPG APIs. ProviderCNPG selects
// CNPG-backed placement (cnpgPlacement), resolving database_id against
// existing ManagedDatabases exactly as before -- unchanged from the prior
// behavior of this package.
const (
	ProviderDeployment = "deployment"
	ProviderCNPG       = "cnpg"
)

// resolveDatabaseProvider validates a raw DATABASE_PROVIDER environment
// value read at gateway-service construction time (server startup). Unset or
// empty resolves to ProviderDeployment; any value other than "deployment" or
// "cnpg" is a startup configuration error, never an implicit fallback to
// "cnpg".
func resolveDatabaseProvider(raw string) (string, error) {
	switch raw {
	case "", ProviderDeployment:
		return ProviderDeployment, nil
	case ProviderCNPG:
		return ProviderCNPG, nil
	default:
		return "", fmt.Errorf("invalid DATABASE_PROVIDER %q: must be %q or %q (unset defaults to %q)",
			raw, ProviderCNPG, ProviderDeployment, ProviderDeployment)
	}
}
