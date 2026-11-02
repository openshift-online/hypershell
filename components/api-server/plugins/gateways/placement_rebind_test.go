package gateways

import (
	"context"
	"testing"

	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	svcerrors "github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
)

// fakeRebindService implements only the two GatewayService methods
// resolvePendingPlacements uses; the embedded interface satisfies the rest (they
// are never called by the sweep).
type fakeRebindService struct {
	GatewayService
	unplaced GatewayList
	replaced []*Gateway
}

func (f *fakeRebindService) FindUnplaced(ctx context.Context) (GatewayList, *svcerrors.ServiceError) {
	return f.unplaced, nil
}

func (f *fakeRebindService) Replace(ctx context.Context, gw *Gateway) (*Gateway, *svcerrors.ServiceError) {
	f.replaced = append(f.replaced, gw)
	return gw, nil
}

func newUnplacedGateway(id, placementJSON string) *Gateway {
	gw := &Gateway{Placement: strPtr(placementJSON), Status: strPtr(awaitingPlacementStatus)}
	gw.ID = id
	return gw
}

func TestResolvePendingPlacementsAssignsMatchedGatewayOnly(t *testing.T) {
	matched := newUnplacedGateway("gw-match", `{"selector":["network/public","provider/ibm"]}`)
	waiting := newUnplacedGateway("gw-wait", `{"selector":["network/vpn"]}`)
	svc := &fakeRebindService{unplaced: GatewayList{matched, waiting}}

	resolve := func(ctx context.Context, intent openapi.GatewayPlacementIntent) (string, bool, *svcerrors.ServiceError) {
		if len(intent.Selector) == 2 && intent.Selector[0] == "network/public" {
			return "cluster-x", true, nil
		}
		return "", false, nil // no eligible cluster yet
	}

	if err := resolvePendingPlacements(context.Background(), svc, resolve); err != nil {
		t.Fatalf("resolvePendingPlacements: %v", err)
	}
	if len(svc.replaced) != 1 {
		t.Fatalf("expected exactly one gateway placed, got %d", len(svc.replaced))
	}
	placed := svc.replaced[0]
	if placed.ID != "gw-match" {
		t.Fatalf("expected gw-match placed, got %s", placed.ID)
	}
	if placed.ClusterId != "cluster-x" {
		t.Fatalf("expected cluster-x assigned, got %q", placed.ClusterId)
	}
	if placed.Status != nil {
		t.Fatalf("expected awaiting-placement status cleared on placement, got %q", *placed.Status)
	}
}

func TestResolvePendingPlacementsSkipsFailuresAndPlacesTheRest(t *testing.T) {
	broken := newUnplacedGateway("gw-broken", `{"selector":["network/public"]}`)
	good := newUnplacedGateway("gw-good", `{"selector":["provider/ibm"]}`)
	svc := &fakeRebindService{unplaced: GatewayList{broken, good}}

	resolve := func(ctx context.Context, intent openapi.GatewayPlacementIntent) (string, bool, *svcerrors.ServiceError) {
		if len(intent.Selector) == 1 && intent.Selector[0] == "network/public" {
			return "", false, svcerrors.GeneralError("boom")
		}
		return "cluster-y", true, nil
	}

	// A per-gateway failure is logged and skipped (not returned), so it does not gate
	// the ManagedCluster event that triggered the sweep.
	if err := resolvePendingPlacements(context.Background(), svc, resolve); err != nil {
		t.Fatalf("expected per-gateway failures to be swallowed, got %v", err)
	}
	// The failure of gw-broken must not stop gw-good from being placed.
	if len(svc.replaced) != 1 || svc.replaced[0].ID != "gw-good" {
		t.Fatalf("expected gw-good placed despite gw-broken error, got %+v", svc.replaced)
	}
}

func TestResolvePendingPlacementsSkipsLocalKind(t *testing.T) {
	localKind := newUnplacedGateway("gw-local", `{"mode":"local-kind"}`)
	svc := &fakeRebindService{unplaced: GatewayList{localKind}}

	called := false
	resolve := func(ctx context.Context, intent openapi.GatewayPlacementIntent) (string, bool, *svcerrors.ServiceError) {
		called = true
		return "", false, nil
	}

	if err := resolvePendingPlacements(context.Background(), svc, resolve); err != nil {
		t.Fatalf("resolvePendingPlacements: %v", err)
	}
	if called {
		t.Fatal("expected local-kind placement to be skipped, not re-resolved")
	}
	if len(svc.replaced) != 0 {
		t.Fatalf("expected no placement for local-kind, got %+v", svc.replaced)
	}
}
