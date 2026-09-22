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
// When hasKeycloak is false, the IdentityProviderReady condition is omitted.
func InitConditions(hasKeycloak bool) []ProvisioningCondition {
	conditions := []ProvisioningCondition{
		{Type: ConditionEnvironmentReady, ConditionStatus: StatusPending},
		{Type: ConditionDatabaseReady, ConditionStatus: StatusPending},
	}
	if hasKeycloak {
		conditions = append(conditions, ProvisioningCondition{
			Type:            ConditionIdentityProviderReady,
			ConditionStatus: StatusPending,
		})
	}
	conditions = append(conditions,
		ProvisioningCondition{Type: ConditionGatewayDeployed, ConditionStatus: StatusPending},
		ProvisioningCondition{Type: ConditionGatewayHealthy, ConditionStatus: StatusPending},
	)
	return conditions
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
