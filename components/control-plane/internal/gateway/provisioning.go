package gateway

const (
	ConditionEnvironmentReady      = "EnvironmentReady"
	ConditionDatabaseReady         = "DatabaseReady"
	ConditionIdentityProviderReady = "IdentityProviderReady"
	ConditionGatewayDeployed       = "GatewayDeployed"
	ConditionGatewayHealthy        = "GatewayHealthy"

	StatusPending    = "Pending"
	StatusInProgress = "InProgress"
	StatusComplete   = "Complete"
	StatusFailed     = "Failed"
)

// ProvisioningCondition represents a single user-facing provisioning step.
type ProvisioningCondition struct {
	Type            string `json:"type"`
	ConditionStatus string `json:"condition_status"`
	Message         string `json:"message"`
}

// ProgressReporter is called by ReconcileGateway at step boundaries to report
// provisioning condition transitions.
type ProgressReporter func(step, status, message string)

// InitConditions returns the initial ordered conditions list for a gateway.
// Keycloak is mandatory, so IdentityProviderReady is always one of them.
func InitConditions() []ProvisioningCondition {
	return []ProvisioningCondition{
		{Type: ConditionEnvironmentReady, ConditionStatus: StatusPending},
		{Type: ConditionDatabaseReady, ConditionStatus: StatusPending},
		{Type: ConditionIdentityProviderReady, ConditionStatus: StatusPending},
		{Type: ConditionGatewayDeployed, ConditionStatus: StatusPending},
		{Type: ConditionGatewayHealthy, ConditionStatus: StatusPending},
	}
}

// SetCondition updates the status and message for the named condition in place.
func SetCondition(conditions []ProvisioningCondition, step, status, message string) {
	for i := range conditions {
		if conditions[i].Type == step {
			conditions[i].ConditionStatus = status
			conditions[i].Message = message
			return
		}
	}
}
