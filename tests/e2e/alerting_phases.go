package e2e

import "testing"

// p2_10AlertingSignal validates that an operational fault surfaces an observable
// alerting signal (E2E-COV-04). It is qualification-gated.
//
// The metric-fault path the suite used to provoke - a Vault-backed gateway
// failing closed on an absent serving-CA - is no longer reachable through the
// API after PLAT-DATA-23, which makes credential_driver (and every gateway field
// except name) control-plane-only and rejects user-supplied values, so a user
// can no longer create a gateway that deterministically faults. Main's serial
// P2.7 (vault-fail-closed) already exercises that fault path itself. The firing
// tier additionally needs an authored PrometheusRule, which the platform does
// not ship yet. Until a qualification harness supplies both a reachable fault and
// an alert rule, this phase skips rather than false-passing.
func (s *E2ESuite) p2_10AlertingSignal(t *testing.T) {
	if envOrDefault("E2E_QUALIFY_ALERTING", "0") != "1" {
		t.Skip("alerting-signal qualification requires E2E_QUALIFY_ALERTING=1, an authored PrometheusRule, and a reachable Prometheus/Alertmanager")
	}
	t.Skip("no PrometheusRule is authored for the platform yet, and the reconcile-fault provocation is no longer reachable through the API after PLAT-DATA-23 (gateway fields are control-plane-only); alerting-signal has no rule to provoke")
}
