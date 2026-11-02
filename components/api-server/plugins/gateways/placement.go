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
	Selector         []string `json:"selector,omitempty"`
	ExcludedSelector []string `json:"excludedSelector,omitempty"`
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
	ID           string
	Capabilities []string
	Connected    bool
}

func eligiblePlacementCandidates(intent PlacementIntent, candidates []PlacementCandidate) []PlacementCandidate {
	eligible := make([]PlacementCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.Connected &&
			capabilitySetContains(candidate.Capabilities, intent.Selector) &&
			capabilitySetExcludes(candidate.Capabilities, intent.ExcludedSelector) {
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

func capabilitySetContains(capabilities, selector []string) bool {
	set := make(map[string]struct{}, len(capabilities))
	for _, capability := range capabilities {
		set[capability] = struct{}{}
	}
	for _, label := range selector {
		if _, ok := set[label]; !ok {
			return false
		}
	}
	return true
}

func capabilitySetExcludes(capabilities, excludedSelector []string) bool {
	set := make(map[string]struct{}, len(capabilities))
	for _, capability := range capabilities {
		set[capability] = struct{}{}
	}
	for _, label := range excludedSelector {
		if _, ok := set[label]; ok {
			return false
		}
	}
	return true
}

func ResolvePlacement(intent PlacementIntent, candidates []PlacementCandidate) (string, error) {
	return resolvePlacement(intent, candidates, randomPlacementIndex)
}
