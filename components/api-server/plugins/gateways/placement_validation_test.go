package gateways

import (
	"context"
	"testing"

	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	rherrors "github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
)

func TestValidatePlacementIntentNormalizesSelectorAndExcludedSelector(t *testing.T) {
	intent := openapi.GatewayPlacementIntent{
		Selector:         []string{"provider/ibm", "network/public", "provider/ibm"},
		ExcludedSelector: []string{"runtime/kata", "runtime/kata"},
	}

	if svcErr := validatePlacementIntent(&intent); svcErr != nil {
		t.Fatalf("validatePlacementIntent: %v", svcErr)
	}
	if len(intent.Selector) != 2 || intent.Selector[0] != "network/public" || intent.Selector[1] != "provider/ibm" {
		t.Fatalf("selector was not normalized: %+v", intent.Selector)
	}
	if len(intent.ExcludedSelector) != 1 || intent.ExcludedSelector[0] != "runtime/kata" {
		t.Fatalf("excluded selector was not normalized: %+v", intent.ExcludedSelector)
	}
}

func TestValidatePlacementIntentRejectsExcludedSelectorForLocalKind(t *testing.T) {
	mode := "local-kind"
	intent := openapi.GatewayPlacementIntent{
		Mode:             &mode,
		ExcludedSelector: []string{"runtime/kata"},
	}

	if svcErr := validatePlacementIntent(&intent); svcErr == nil {
		t.Fatal("expected local-kind placement with excludedSelector to be rejected")
	}
}

func TestResolveGatewayPlacementPersistsNormalizedSelectors(t *testing.T) {
	var resolvedIntent openapi.GatewayPlacementIntent
	handler := gatewayHandler{
		placement: func(_ context.Context, intent openapi.GatewayPlacementIntent) (string, bool, *rherrors.ServiceError) {
			resolvedIntent = intent
			return "cluster-1", true, nil
		},
	}
	intent := openapi.GatewayPlacementIntent{
		Selector:         []string{"provider/ibm", "network/public", "provider/ibm"},
		ExcludedSelector: []string{"runtime/kata", "runtime/kata"},
	}
	model := &Gateway{}

	if svcErr := handler.resolveGatewayPlacement(context.Background(), intent, model); svcErr != nil {
		t.Fatalf("resolveGatewayPlacement: %v", svcErr)
	}
	if len(resolvedIntent.Selector) != 2 || resolvedIntent.Selector[0] != "network/public" || resolvedIntent.Selector[1] != "provider/ibm" {
		t.Fatalf("resolver selector was not normalized: %+v", resolvedIntent.Selector)
	}
	if len(resolvedIntent.ExcludedSelector) != 1 || resolvedIntent.ExcludedSelector[0] != "runtime/kata" {
		t.Fatalf("resolver excluded selector was not normalized: %+v", resolvedIntent.ExcludedSelector)
	}
	stored, ok := storedPlacementToIntent(model.Placement)
	if !ok {
		t.Fatal("expected placement to be persisted")
	}
	if len(stored.Selector) != 2 || stored.Selector[0] != "network/public" || stored.Selector[1] != "provider/ibm" {
		t.Fatalf("persisted selector was not normalized: %+v", stored.Selector)
	}
	if len(stored.ExcludedSelector) != 1 || stored.ExcludedSelector[0] != "runtime/kata" {
		t.Fatalf("persisted excluded selector was not normalized: %+v", stored.ExcludedSelector)
	}
}
