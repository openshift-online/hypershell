package policyinterceptor

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/openshift-online/hypershell/components/api-server/pkg/cedar"
	"github.com/openshift-online/hypershell/components/control-plane/internal/policybundle"
)

// snapshot is one compiled, immutable bundle revision. Evaluate reads the
// current snapshot once per call, so a revision swap is atomic: every
// evaluation sees exactly one revision (PLAT-PGP-12).
type snapshot struct {
	bundle   policybundle.Bundle
	enforced *cedar.Evaluator
	audit    *cedar.Evaluator
	messages map[string]cedar.PolicyMessage
	// defaults are the fleet defaults carried by the bundle, nil when the
	// bundle carries none (PLAT-PGP-08.05).
	defaults *DefaultQuantities
	// mismatchLogged makes the env/bundle defaults mismatch log once per
	// revision; the metric counts every affected call.
	mismatchLogged atomic.Bool
}

// hasEnforced reports whether at least one policy is enforced on this gateway.
func (s *snapshot) hasEnforced() bool {
	return s != nil && s.bundle.ResourcePoliciesSupported && len(s.bundle.Enforced) > 0 && !s.enforced.Empty()
}

// Store holds the current snapshot and the last bundle load error.
type Store struct {
	gatewayID string
	current   atomic.Pointer[snapshot]

	mu      sync.Mutex
	lastErr error
}

// NewStore returns an empty store for one gateway. Until a bundle loads,
// Current returns nil and fail_closed bindings fail closed (PLAT-PGP-12.02).
func NewStore(gatewayID string) *Store {
	return &Store{gatewayID: gatewayID}
}

// Current returns the applied snapshot, or nil before the first bundle loads.
func (s *Store) Current() *snapshot { return s.current.Load() }

// AppliedRevision returns the applied bundle revision, or "" before the first
// bundle loads.
func (s *Store) AppliedRevision() string {
	if snap := s.Current(); snap != nil {
		return snap.bundle.Revision
	}
	return ""
}

// LastError returns the most recent bundle rejection, or nil after a successful
// apply.
func (s *Store) LastError() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastErr
}

func (s *Store) setErr(err error) {
	s.mu.Lock()
	s.lastErr = err
	s.mu.Unlock()
}

// Apply decodes, checks, and compiles raw bundle JSON and swaps it in. A bundle
// for another gateway, with an unknown schema version, or that does not compile
// is rejected and never evaluated (PLAT-PGP-03.02); the previously applied
// revision, if any, stays in effect. It returns whether the applied revision
// changed.
func (s *Store) Apply(raw []byte) (bool, error) {
	b, err := policybundle.Decode(raw)
	if err != nil {
		s.setErr(err)
		return false, err
	}
	if b.GatewayID != s.gatewayID {
		err := fmt.Errorf("policy bundle is for gateway %q, not this gateway %q", b.GatewayID, s.gatewayID)
		s.setErr(err)
		return false, err
	}
	if cur := s.Current(); cur != nil && cur.bundle.Revision == b.Revision {
		s.setErr(nil)
		return false, nil
	}
	enforced, audit, err := b.Evaluators()
	if err != nil {
		err = fmt.Errorf("compile policy bundle revision %s: %w", b.Revision, err)
		s.setErr(err)
		return false, err
	}
	snap := &snapshot{bundle: b, enforced: enforced, audit: audit, messages: b.Messages()}
	if b.Defaults != nil {
		parsed, err := b.Defaults.Parse()
		if err != nil {
			s.setErr(err)
			return false, err
		}
		snap.defaults = &DefaultQuantities{CPURequest: b.Defaults.CPURequest, CPULimit: b.Defaults.CPULimit,
			MemoryRequest: b.Defaults.MemoryRequest, MemoryLimit: b.Defaults.MemoryLimit, Parsed: parsed}
	}
	s.current.Store(snap)
	s.setErr(nil)
	return true, nil
}
