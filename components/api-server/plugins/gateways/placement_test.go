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

func TestPlacementSupported(t *testing.T) {
	tests := []struct {
		network  string
		provider string
		want     bool
	}{
		{network: "public", provider: "aws", want: true},
		{network: "vpn", provider: "aws", want: true},
		{network: "public", provider: "ibm", want: true},
		{network: "vpn", provider: "ibm", want: false},
	}
	for _, test := range tests {
		if got := PlacementSupported(test.network, test.provider); got != test.want {
			t.Errorf("PlacementSupported(%q, %q) = %t, want %t", test.network, test.provider, got, test.want)
		}
	}
}

func TestResolvePlacementSelectsRandomEligibleCandidate(t *testing.T) {
	selectedSize := 0
	got, err := resolvePlacement(PlacementIntent{Network: "public", Provider: "aws"}, []PlacementCandidate{
		{ID: "disconnected", Provider: "aws", Connected: false},
		{ID: "ibm", Provider: "ibm", Connected: true},
		{ID: "aws-a", Provider: "aws", Visibility: "public", Connected: true},
		{ID: "aws-b", Provider: "aws", Visibility: "public", Connected: true},
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

func TestResolvePlacementRejectsUnsupportedAndUnavailablePlacement(t *testing.T) {
	if _, err := ResolvePlacement(PlacementIntent{Network: "vpn", Provider: "ibm"}, nil); err == nil {
		t.Fatal("expected unsupported placement error")
	}
	if _, err := ResolvePlacement(PlacementIntent{Network: "public", Provider: "aws"}, nil); err == nil {
		t.Fatal("expected unavailable placement error")
	}
}

func TestResolvePlacementReturnsRandomSourceFailure(t *testing.T) {
	want := errors.New("random unavailable")
	_, err := resolvePlacement(
		PlacementIntent{Network: "public", Provider: "aws"},
		[]PlacementCandidate{{ID: "aws", Provider: "aws", Visibility: "public", Connected: true}},
		func(int) (int, error) { return 0, want },
	)
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
}

func TestManagedPlacementAvailableRequiresMatchingVisibility(t *testing.T) {
	snapshot := placementSnapshot{providers: map[string]*providerPlacementState{
		"aws": {candidates: []PlacementCandidate{{ID: "public-aws", Provider: "aws", Visibility: "public", Connected: true}}},
	}}

	if !managedPlacementAvailable(snapshot, "public", "aws") {
		t.Fatal("expected public AWS placement to be available")
	}
	if managedPlacementAvailable(snapshot, "vpn", "aws") {
		t.Fatal("expected VPN AWS placement to be unavailable without a VPN cluster")
	}
}
