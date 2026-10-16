// Package secretstore is the api-server's write-only client for the configured
// external secret manager. The secret-submission endpoint
// (data-model.spec.md "Secret Submission and Storage") writes a submitted secret
// value to the store at a deterministic location and then persists only a
// SecretSource reference; the raw value is never stored in the database, never
// resolved in-process, and never returned to a client. This package is therefore
// write-only by construction: it exposes Put and no read.
package secretstore

import (
	"context"
	"fmt"
	"os"
	"sync"
)

// Store is the write-only secret-manager client. Put writes value at location.
// It MUST be idempotent for a fixed (location, value): the submission endpoint
// derives location deterministically from the submission identity so a retried
// submission overwrites the same entry rather than creating a duplicate, which is
// what makes the write-then-record flow safe to retry after a database failure.
type Store interface {
	Put(ctx context.Context, location string, value string) error
}

// MemoryStore is an in-memory Store for tests and local development. It never
// leaves the process, so it is unsuitable for production but lets the submission
// flow be exercised end to end without a real secret manager.
type MemoryStore struct {
	mu   sync.Mutex
	data map[string]string
}

// NewMemoryStore returns an empty in-memory Store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{data: map[string]string{}}
}

// Put writes value at location, overwriting any prior value (idempotent).
func (m *MemoryStore) Put(_ context.Context, location, value string) error {
	if location == "" {
		return fmt.Errorf("secret location is required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[location] = value
	return nil
}

// Get reports the value written at location. It exists for tests only; the Store
// interface deliberately exposes no read.
func (m *MemoryStore) Get(location string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.data[location]
	return v, ok
}

// disabledStore is the default when no secret store is configured. It rejects
// every write so a secret submission in an unconfigured deployment fails loudly
// rather than silently dropping the value or persisting a reference to a secret
// that was never stored.
type disabledStore struct{}

func (disabledStore) Put(_ context.Context, _, _ string) error {
	return fmt.Errorf("no secret store configured: set HYPERSHELL_SECRET_STORE_BACKEND to accept secret submissions")
}

// errorStore returns a fixed configuration error on every write. It lets the
// server start with a misconfigured secret store (so unrelated routes stay up)
// while failing any secret submission loudly rather than silently.
type errorStore struct{ err error }

// NewErrorStore returns a Store whose Put always fails with err.
func NewErrorStore(err error) Store { return errorStore{err: err} }

func (s errorStore) Put(_ context.Context, _, _ string) error {
	return fmt.Errorf("secret store unavailable: %w", s.err)
}

// NewFromEnv builds the Store from the HYPERSHELL_SECRET_STORE_* environment
// (configuration is kept out of code per the repository conventions). Backends
// mirror the SecretSource backend vocabulary (vault, aws-secrets-manager,
// kubernetes); vault is the implemented real backend, memory is the in-process
// test/dev backend, and the remaining enum values are rejected explicitly rather
// than silently degraded. The default (unset) is a disabled store that rejects
// writes.
func NewFromEnv() (Store, error) {
	switch backend := os.Getenv("HYPERSHELL_SECRET_STORE_BACKEND"); backend {
	case "":
		return disabledStore{}, nil
	case "memory":
		return NewMemoryStore(), nil
	case "vault":
		addr := os.Getenv("HYPERSHELL_SECRET_STORE_VAULT_ADDR")
		token := os.Getenv("HYPERSHELL_SECRET_STORE_VAULT_TOKEN")
		if addr == "" || token == "" {
			return nil, fmt.Errorf("vault secret store requires HYPERSHELL_SECRET_STORE_VAULT_ADDR and HYPERSHELL_SECRET_STORE_VAULT_TOKEN")
		}
		mount := os.Getenv("HYPERSHELL_SECRET_STORE_VAULT_MOUNT")
		if mount == "" {
			mount = "secret"
		}
		return NewVaultStore(addr, token, mount), nil
	case "aws-secrets-manager", "kubernetes":
		return nil, fmt.Errorf("secret store backend %q is not yet implemented in the api-server; configure vault or memory", backend)
	default:
		return nil, fmt.Errorf("unknown secret store backend %q", backend)
	}
}
