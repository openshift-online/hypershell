package gateways

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"time"
)

var errNoEligiblePlacement = errors.New("no eligible placement is available")

// PlacementIntent is the client-facing placement request. Cluster IDs are
// deliberately absent: they are selected by the API server at create time.
type PlacementIntent struct {
	Mode     string `json:"mode,omitempty"`
	Network  string `json:"network,omitempty"`
	Provider string `json:"provider,omitempty"`
}

const managedClusterConnectedMaxAge = 5 * time.Minute

func PlacementControlPlaneConnected(lastSeen *time.Time, now time.Time) bool {
	if lastSeen == nil {
		return false
	}
	age := now.Sub(*lastSeen)
	return age >= 0 && age < managedClusterConnectedMaxAge
}

type PlacementCandidate struct {
	ID         string
	Provider   string
	Visibility string
	Connected  bool
}

func PlacementSupported(network, provider string) bool {
	switch provider {
	case "aws":
		return network == "public" || network == "vpn"
	case "ibm":
		return network == "public"
	default:
		return false
	}
}

func eligiblePlacementCandidates(intent PlacementIntent, candidates []PlacementCandidate) []PlacementCandidate {
	eligible := make([]PlacementCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.Connected && candidate.Provider == intent.Provider && candidate.Visibility == intent.Network {
			eligible = append(eligible, candidate)
		}
	}
	return eligible
}

func randomPlacementIndex(size int) (int, error) {
	index, err := rand.Int(rand.Reader, big.NewInt(int64(size)))
	if err != nil {
		return 0, fmt.Errorf("select random placement: %w", err)
	}
	return int(index.Int64()), nil
}

func resolvePlacement(intent PlacementIntent, candidates []PlacementCandidate, selectIndex func(int) (int, error)) (string, error) {
	if !PlacementSupported(intent.Network, intent.Provider) {
		return "", fmt.Errorf("placement %s/%s is unsupported", intent.Provider, intent.Network)
	}
	eligible := eligiblePlacementCandidates(intent, candidates)
	if len(eligible) == 0 {
		return "", errNoEligiblePlacement
	}
	index, err := selectIndex(len(eligible))
	if err != nil {
		return "", err
	}
	if index < 0 || index >= len(eligible) {
		return "", fmt.Errorf("placement selector returned invalid index %d", index)
	}
	return eligible[index].ID, nil
}

func ResolvePlacement(intent PlacementIntent, candidates []PlacementCandidate) (string, error) {
	return resolvePlacement(intent, candidates, randomPlacementIndex)
}
