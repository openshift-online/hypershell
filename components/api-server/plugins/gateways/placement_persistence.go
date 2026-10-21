package gateways

import (
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
)

// awaitingPlacementStatus is the human-readable status a late-bound gateway holds
// while its placement selector matches no eligible managed cluster. The gateway
// stays in the canonical Pending phase (gatewayhealth.PhasePending); this status
// explains why it has not been acted on yet. See data-model.spec.md, "Requirement:
// Gateway Placement and Late Binding".
const awaitingPlacementStatus = "Awaiting placement: no connected managed cluster matches the placement selector"

// storedPlacement is the on-disk JSON shape of a persisted placement selector. It
// is kept deliberately minimal (independent of the generated openapi type's
// marshalling) so the stored document is stable: {"selector":[...]} for a
// capability-label selector or {"mode":"local-kind"} for local-kind placement. The
// API server re-evaluates it to assign a cluster_id when a matching ManagedCluster
// becomes available.
type storedPlacement struct {
	Selector []string `json:"selector,omitempty"`
	Mode     *string  `json:"mode,omitempty"`
}

// marshalPlacement encodes a placement intent for persistence on the Gateway row.
func marshalPlacement(intent openapi.GatewayPlacementIntent) (*string, error) {
	stored := storedPlacement{Selector: intent.Selector, Mode: intent.Mode}
	data, err := json.Marshal(stored)
	if err != nil {
		return nil, fmt.Errorf("marshal gateway placement: %w", err)
	}
	s := string(data)
	return &s, nil
}

// storedPlacementToIntent decodes a persisted placement document back into a
// placement intent so the API server can re-resolve a late-bound gateway. It
// returns ok=false for an absent, empty, unparseable, or contentless document
// (logging the parse failure without failing the caller).
func storedPlacementToIntent(stored *string) (openapi.GatewayPlacementIntent, bool) {
	if stored == nil || *stored == "" {
		return openapi.GatewayPlacementIntent{}, false
	}
	var parsed storedPlacement
	if err := json.Unmarshal([]byte(*stored), &parsed); err != nil {
		slog.Warn("failed to unmarshal gateway placement", "placement", *stored, "error", err)
		return openapi.GatewayPlacementIntent{}, false
	}
	if len(parsed.Selector) == 0 && parsed.Mode == nil {
		return openapi.GatewayPlacementIntent{}, false
	}
	return openapi.GatewayPlacementIntent{Selector: parsed.Selector, Mode: parsed.Mode}, true
}
