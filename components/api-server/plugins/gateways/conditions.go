package gateways

import "encoding/json"

// Provisioning condition statuses, mirrored from the control plane's
// gateway.Status* constants. The API server treats them as an ordered progress
// ladder so a completed step cannot silently regress within a generation.
const (
	conditionStatusPending    = "Pending"
	conditionStatusInProgress = "InProgress"
	conditionStatusComplete   = "Complete"
	conditionStatusFailed     = "Failed"
)

// provisioningConditionJSON is the persisted shape of a single provisioning
// condition inside the gateways.provisioning_conditions jsonb column. It matches
// the JSON the control plane writes and the gRPC handler unmarshals.
type provisioningConditionJSON struct {
	Type            string `json:"type"`
	ConditionStatus string `json:"condition_status"`
	Message         string `json:"message"`
}

// conditionRank orders the active progress states. Higher means further along.
// Failed is handled separately (see pickProvisioningCondition) because it is a
// visibility signal, not a point on the forward ladder.
func conditionRank(status string) int {
	switch status {
	case conditionStatusInProgress:
		return 1
	case conditionStatusComplete:
		return 2
	default: // Pending and any unknown status
		return 0
	}
}

// pickProvisioningCondition chooses between the persisted (current) condition and
// an incoming one so that progress only moves forward within a generation:
//   - an incoming Failed always surfaces (an operator must see a real failure);
//   - recovery from a persisted Failed accepts the incoming state;
//   - otherwise the higher-ranked state wins, so a completed step never regresses
//     to InProgress or Pending when a redundant reconcile pass replays an earlier
//     stage.
func pickProvisioningCondition(current, incoming provisioningConditionJSON) provisioningConditionJSON {
	if incoming.ConditionStatus == conditionStatusFailed {
		return incoming
	}
	if current.ConditionStatus == conditionStatusFailed {
		return incoming
	}
	if conditionRank(incoming.ConditionStatus) >= conditionRank(current.ConditionStatus) {
		return incoming
	}
	return current
}

// mergeMonotonicProvisioningConditions merges an incoming provisioning-conditions
// document onto the persisted one so that, within a single generation, no step
// regresses. It is the API server's authoritative enforcement point: redundant
// control-plane reconcile passes (watch re-seed on reconnect, or two controller
// pods overlapping during a rollout, neither of which is serialized end to end)
// each replay earlier-stage conditions, and last-writer-wins would otherwise let
// a completed step flip back to InProgress. Because Replace holds the gateway's
// advisory lock, this merge sees a consistent current value regardless of writer.
//
// Both arguments are the raw jsonb strings (possibly nil). Incoming ordering is
// preserved; any condition present only in the persisted document is retained so
// a narrower incoming write cannot drop a completed step. A nil/empty or
// unparseable incoming document leaves the persisted value unchanged.
func mergeMonotonicProvisioningConditions(current, incoming *string) (*string, error) {
	incomingConds, ok := parseProvisioningConditions(incoming)
	if !ok {
		// Nothing meaningful to merge in: preserve the persisted value verbatim.
		return current, nil
	}
	currentConds, _ := parseProvisioningConditions(current)
	currentByType := make(map[string]provisioningConditionJSON, len(currentConds))
	for _, c := range currentConds {
		currentByType[c.Type] = c
	}

	merged := make([]provisioningConditionJSON, 0, len(incomingConds)+len(currentConds))
	seen := make(map[string]bool, len(incomingConds))
	for _, inc := range incomingConds {
		seen[inc.Type] = true
		if cur, found := currentByType[inc.Type]; found {
			merged = append(merged, pickProvisioningCondition(cur, inc))
			continue
		}
		merged = append(merged, inc)
	}
	// Retain persisted conditions the incoming write omitted so a partial update
	// cannot erase a step that already completed.
	for _, cur := range currentConds {
		if !seen[cur.Type] {
			merged = append(merged, cur)
		}
	}

	data, err := json.Marshal(merged)
	if err != nil {
		return nil, err
	}
	s := string(data)
	return &s, nil
}

// parseProvisioningConditions unmarshals a jsonb conditions document. It returns
// ok=false when the pointer is nil, empty, or does not decode to a non-empty
// list, so callers can distinguish "no conditions supplied" from "conditions
// present".
func parseProvisioningConditions(raw *string) ([]provisioningConditionJSON, bool) {
	if raw == nil || *raw == "" {
		return nil, false
	}
	var conds []provisioningConditionJSON
	if err := json.Unmarshal([]byte(*raw), &conds); err != nil {
		return nil, false
	}
	if len(conds) == 0 {
		return nil, false
	}
	return conds, true
}
