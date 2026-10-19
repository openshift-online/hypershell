package gateways

import (
	"encoding/json"
	"testing"

	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/hypershell/components/api-server/pkg/gatewayhealth"
)

func TestMarshalPlacementEncodesSelector(t *testing.T) {
	stored, err := marshalPlacement(openapi.GatewayPlacementIntent{Selector: []string{"network/public", "provider/ibm"}})
	if err != nil {
		t.Fatalf("marshalPlacement: %v", err)
	}
	if stored == nil {
		t.Fatal("expected a non-nil stored placement")
	}
	var decoded storedPlacement
	if err := json.Unmarshal([]byte(*stored), &decoded); err != nil {
		t.Fatalf("stored placement is not valid JSON: %v", err)
	}
	if len(decoded.Selector) != 2 || decoded.Selector[0] != "network/public" || decoded.Selector[1] != "provider/ibm" {
		t.Fatalf("selector did not round-trip: %+v", decoded.Selector)
	}
	if decoded.Mode != nil {
		t.Fatalf("expected no mode for a selector placement, got %q", *decoded.Mode)
	}
}

func TestMarshalPlacementEncodesLocalKind(t *testing.T) {
	mode := "local-kind"
	stored, err := marshalPlacement(openapi.GatewayPlacementIntent{Mode: &mode})
	if err != nil {
		t.Fatalf("marshalPlacement: %v", err)
	}
	var decoded storedPlacement
	if err := json.Unmarshal([]byte(*stored), &decoded); err != nil {
		t.Fatalf("stored placement is not valid JSON: %v", err)
	}
	if decoded.Mode == nil || *decoded.Mode != "local-kind" {
		t.Fatalf("local-kind mode did not round-trip: %+v", decoded)
	}
	if len(decoded.Selector) != 0 {
		t.Fatalf("expected no selector for local-kind, got %+v", decoded.Selector)
	}
}

func TestApplyAwaitingPlacementSetsPendingAndStatus(t *testing.T) {
	model := &Gateway{ClusterId: "should-be-cleared"}
	applyAwaitingPlacement(model)
	if model.ClusterId != "" {
		t.Fatalf("expected cluster_id cleared, got %q", model.ClusterId)
	}
	if model.Phase == nil || *model.Phase != string(gatewayhealth.PhasePending) {
		t.Fatalf("expected Pending phase, got %v", model.Phase)
	}
	if model.Status == nil || *model.Status != awaitingPlacementStatus {
		t.Fatalf("expected awaiting-placement status, got %v", model.Status)
	}
}

func TestDesiredStateChangedDetectsPlacement(t *testing.T) {
	a := &Gateway{Placement: strPtr(`{"selector":["network/public"]}`)}
	b := &Gateway{Placement: strPtr(`{"selector":["network/vpn"]}`)}
	if !desiredStateChanged(a, b) {
		t.Fatal("expected a placement change to be a desired-state change")
	}
	// jsonb key/space normalization must not count as a change.
	c := &Gateway{Placement: strPtr(`{ "selector": ["network/public"] }`)}
	if desiredStateChanged(a, c) {
		t.Fatal("expected semantically-equal placement to not be a desired-state change")
	}
}
