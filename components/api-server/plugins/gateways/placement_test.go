package gateways

import (
	"errors"
	"testing"
	"time"
)

func TestPlacementControlPlaneConnectedUsesHeartbeatWindow(t *testing.T) {
	now := time.Now()
	healthy := now.Add(-time.Minute)
	offline := now.Add(-31 * time.Minute)
	if !PlacementControlPlaneConnected(&healthy, now) {
		t.Fatal("expected a recent heartbeat to be connected")
	}
	if PlacementControlPlaneConnected(&offline, now) || PlacementControlPlaneConnected(nil, now) {
		t.Fatal("expected an absent or stale heartbeat to be disconnected")
	}
}

func TestResolvePlacementSelectsRandomEligibleCandidate(t *testing.T) {
	selectedSize := 0
	selector := []string{"network/public", "provider/aws"}
	got, err := resolvePlacement(PlacementIntent{Selector: selector}, []PlacementCandidate{
		{ID: "disconnected", Capabilities: selector, Connected: false},
		{ID: "ibm", Capabilities: []string{"network/public", "provider/ibm"}, Connected: true},
		{ID: "aws-a", Capabilities: selector, Connected: true},
		{ID: "aws-b", Capabilities: append(selector, "runtime/kata"), Connected: true},
	}, func(size int) (int, error) {
		selectedSize = size
		return 1, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if selectedSize != 2 || got != "aws-b" {
		t.Fatalf("eligible size = %d, selected = %q; want 2 and aws-b", selectedSize, got)
	}
}

func TestResolvePlacementExcludesCandidatesCarryingExcludedLabels(t *testing.T) {
	got, err := ResolvePlacement(PlacementIntent{
		Selector:         []string{"network/public", "provider/ibm"},
		ExcludedSelector: []string{"runtime/kata"},
	}, []PlacementCandidate{
		{ID: "kata", Capabilities: []string{"network/public", "provider/ibm", "runtime/kata"}, Connected: true},
		{ID: "without-kata", Capabilities: []string{"network/public", "provider/ibm"}, Connected: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != "without-kata" {
		t.Fatalf("selected = %q; want without-kata", got)
	}
}

func TestResolvePlacementRejectsUnavailablePlacement(t *testing.T) {
	if _, err := ResolvePlacement(PlacementIntent{Selector: []string{"network/public", "provider/aws"}}, nil); err == nil {
		t.Fatal("expected unavailable placement error")
	}
}

func TestResolvePlacementReturnsRandomSourceFailure(t *testing.T) {
	want := errors.New("random unavailable")
	_, err := resolvePlacement(
		PlacementIntent{Selector: []string{"network/public", "provider/aws"}},
		[]PlacementCandidate{{ID: "aws", Capabilities: []string{"network/public", "provider/aws"}, Connected: true}},
		func(int) (int, error) { return 0, want },
	)
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
}

func TestManagedPlacementAvailableRequiresMatchAll(t *testing.T) {
	snapshot := placementSnapshot{candidates: []PlacementCandidate{{ID: "public-aws", Capabilities: []string{"network/public", "provider/aws"}, Connected: true}}}

	if !managedPlacementAvailable(snapshot, PlacementIntent{Selector: []string{"network/public", "provider/aws"}}) {
		t.Fatal("expected public AWS placement to be available")
	}
	if managedPlacementAvailable(snapshot, PlacementIntent{Selector: []string{"network/vpn", "provider/aws"}}) {
		t.Fatal("expected VPN AWS placement to be unavailable without a VPN cluster")
	}
}
