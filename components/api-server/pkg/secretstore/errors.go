package secretstore

import "errors"

// ErrNotFound reports that a store was reached successfully but holds no value at
// the requested location. It is distinct from a store that is unreachable,
// misconfigured, or errors: callers use errors.Is(err, ErrNotFound) to tell a
// value that has not been uploaded yet (a benign, self-healing SecretPending state)
// from a genuine read failure that should degrade the dependent resource
// (agent-secret-late-binding.spec.md "SecretPending Condition and Self-Healing
// Retry"; control-plane.spec.md "AgentRuntime Reconciliation"). Readers wrap this
// sentinel for the reachable-but-missing case only.
var ErrNotFound = errors.New("secret not found")
