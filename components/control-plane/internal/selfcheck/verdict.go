// Package selfcheck implements the on-cluster managed-cluster self-healthcheck
// (specs/platform/managed-cluster-self-healthcheck.spec.md, PLAT-HLTH-01..07).
//
// Each managed cluster proves it can host a gateway continuously from inside the
// cluster: a continuous reconciler runs a bounded catalog of checks, rolls their
// per-check conditions up into a single verdict, publishes that verdict for the
// registration/heartbeat path to carry to the hub (PLAT-HLTH-05), and exports it
// as low-cardinality OpenTelemetry gauges (PLAT-HLTH-07, OBS-CPL-12).
//
// The verdict shape (overall rollup + per-check conditions with name, status,
// last-transition timestamp, and message, plus a run timestamp and duration) is
// stored verbatim on the owning ManagedCluster (PLAT-DATA-27). The types here
// carry the JSON tags the hub stores and surfaces, so the registration client
// can serialize a verdict straight onto the heartbeat body.
package selfcheck

import "time"

// Overall is the verdict rollup enum (PLAT-HLTH-03).
type Overall string

const (
	// OverallHealthy means every condition reports pass.
	OverallHealthy Overall = "healthy"
	// OverallDegraded means no condition fails but at least one is not pass
	// (for example a warn condition, a degraded-but-serving dependency).
	OverallDegraded Overall = "degraded"
	// OverallFailing means at least one condition reports fail.
	OverallFailing Overall = "failing"
)

// Status is the per-check condition status enum (PLAT-HLTH-03).
type Status string

const (
	// StatusPass means the check succeeded.
	StatusPass Status = "pass"
	// StatusWarn means the check detected a degraded-but-serving condition; it
	// rolls up to degraded rather than failing.
	StatusWarn Status = "warn"
	// StatusFail means the check failed.
	StatusFail Status = "fail"
	// StatusUnknown means the check could not be evaluated (for example a
	// dependency the control plane cannot reach or is not wired to observe).
	StatusUnknown Status = "unknown"
)

// CheckName is a value from the bounded, stable check-name enumeration
// (PLAT-HLTH-02). The names are safe as low-cardinality observability attributes
// (PLAT-HLTH-07, carried in the `check` attribute).
type CheckName string

const (
	// CheckReconcilerHealthy -- the reconciler/operator is healthy and can reach
	// the in-cluster Kubernetes API.
	CheckReconcilerHealthy CheckName = "reconciler-healthy"
	// CheckGatewayNamespacePrerequisites -- the gateway namespace prerequisites
	// and RBAC are present.
	CheckGatewayNamespacePrerequisites CheckName = "gateway-namespace-prerequisites"
	// CheckImagesPullable -- the required gateway, supervisor, and sandbox images
	// are pullable.
	CheckImagesPullable CheckName = "images-pullable"
	// CheckOIDCIssuerReachable -- the OIDC issuer is reachable.
	CheckOIDCIssuerReachable CheckName = "oidc-issuer-reachable"
	// CheckGatewayDatabaseAdminSecret -- the gateway-database admin secret is
	// present and complete.
	CheckGatewayDatabaseAdminSecret CheckName = "gateway-database-admin-secret"
	// CheckGatewayDatabaseReachable -- the gateway-database server is reachable.
	CheckGatewayDatabaseReachable CheckName = "gateway-database-reachable"
	// CheckIngressControllerHealthy -- the ingress/route controller is healthy.
	CheckIngressControllerHealthy CheckName = "ingress-controller-healthy"
)

// CheckNames returns the bounded check-name enumeration in a stable order. It is
// the single source of truth for which names are valid observability attributes.
func CheckNames() []CheckName {
	return []CheckName{
		CheckReconcilerHealthy,
		CheckGatewayNamespacePrerequisites,
		CheckImagesPullable,
		CheckOIDCIssuerReachable,
		CheckGatewayDatabaseAdminSecret,
		CheckGatewayDatabaseReachable,
		CheckIngressControllerHealthy,
	}
}

// Condition is a single per-check result in a verdict (PLAT-HLTH-03). Its JSON
// shape matches what the hub stores verbatim and surfaces (PLAT-DATA-27).
type Condition struct {
	Name             CheckName `json:"name"`
	Status           Status    `json:"status"`
	LastTransitionAt time.Time `json:"last_transition_at"`
	Message          string    `json:"message"`
}

// Verdict is the rich health verdict (PLAT-HLTH-03). Its JSON shape matches the
// `health` object the hub stores and surfaces (PLAT-DATA-27) and the control
// plane reports on the heartbeat (PLAT-CLUS-16).
type Verdict struct {
	Overall    Overall     `json:"overall"`
	ObservedAt time.Time   `json:"observed_at"`
	DurationMs int64       `json:"duration_ms"`
	Conditions []Condition `json:"conditions"`
}

// Rollup derives the overall verdict from its conditions (PLAT-HLTH-03):
// healthy when every condition is pass; failing when any condition fails;
// degraded otherwise (for example a warn or unknown condition). An empty
// condition set rolls up to degraded: there is nothing proving readiness.
func Rollup(conditions []Condition) Overall {
	if len(conditions) == 0 {
		return OverallDegraded
	}
	allPass := true
	for _, c := range conditions {
		if c.Status == StatusFail {
			return OverallFailing
		}
		if c.Status != StatusPass {
			allPass = false
		}
	}
	if allPass {
		return OverallHealthy
	}
	return OverallDegraded
}
