package reconciler

// These values are the shared Gateway health vocabulary for the reconciler and
// health paths. Keep phase comparisons and phase updates on this vocabulary.
const (
	gatewayPhaseProvisioning = "Provisioning"
	gatewayPhaseRunning      = "Running"
	gatewayPhaseDegraded     = "Degraded"
	gatewayPhaseFailed       = "Failed"
	gatewayStatusHealthy     = "Healthy"
)
