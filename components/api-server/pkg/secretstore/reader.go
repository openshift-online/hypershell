package secretstore

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// Reader is the control plane's read-only secret-store client. It mirrors the
// write-only Store (see store.go): where the api-server is write-only to the store
// (it only Puts a submitted value and persists a reference), the control plane is
// read-only from it (it only Gets a referenced value to place it directly into a
// Kubernetes Secret). Keeping the two directions as distinct interfaces enforces
// the single-direction-per-component rule from security.spec.md "Secret Store
// Access Is Single-Directional per Component" by construction: api-server code
// cannot read and control-plane code cannot write.
type Reader interface {
	// Get returns the value previously written at location. It returns an error
	// if the location is empty, does not exist, or cannot be read.
	Get(ctx context.Context, location string) (string, error)
}

// MemoryReader is an in-memory Reader for tests and local development. It is
// seeded with a fixed location->value map and never leaves the process, so it is
// unsuitable for production but lets the control-plane placement flow be exercised
// end to end without a real secret store.
type MemoryReader struct {
	mu   sync.Mutex
	data map[string]string
}

// NewMemoryReader returns a Reader seeded with the given location->value pairs.
// A nil seed yields an empty reader whose Get always reports a missing value.
func NewMemoryReader(seed map[string]string) *MemoryReader {
	data := map[string]string{}
	for k, v := range seed {
		data[k] = v
	}
	return &MemoryReader{data: data}
}

// Set writes value at location. It exists for tests only; the Reader interface
// deliberately exposes no write.
func (m *MemoryReader) Set(location, value string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[location] = value
}

// Get returns the value seeded at location, or an error if absent.
func (m *MemoryReader) Get(_ context.Context, location string) (string, error) {
	if location == "" {
		return "", fmt.Errorf("secret location is required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.data[location]
	if !ok {
		return "", fmt.Errorf("secret not found at location %q", location)
	}
	return v, nil
}

// disabledReader is the default when no secret store is configured. It rejects
// every read so a secret placement in an unconfigured deployment fails loudly
// rather than silently placing an empty credential.
type disabledReader struct{}

func (disabledReader) Get(_ context.Context, _ string) (string, error) {
	return "", fmt.Errorf("no secret store configured: set HYPERSHELL_SECRET_STORE_BACKEND to read stored secrets")
}

// errorReader returns a fixed configuration error on every read. It lets the
// control plane start with a misconfigured secret store (so unrelated reconcilers
// stay up) while failing any SecretSource placement loudly rather than silently
// placing an empty credential.
type errorReader struct{ err error }

// NewErrorReader returns a Reader whose Get always fails with err.
func NewErrorReader(err error) Reader { return errorReader{err: err} }

func (r errorReader) Get(_ context.Context, _ string) (string, error) {
	return "", fmt.Errorf("secret store unavailable: %w", r.err)
}

// NewReaderFromEnv builds the Reader from the same HYPERSHELL_SECRET_STORE_*
// environment NewFromEnv (store.go) uses for the write client, so a single
// deployment configuration drives both directions. The default (unset) is a
// disabled reader that rejects reads.
func NewReaderFromEnv() (Reader, error) {
	switch backend := os.Getenv("HYPERSHELL_SECRET_STORE_BACKEND"); backend {
	case "":
		return disabledReader{}, nil
	case "memory":
		return NewMemoryReader(nil), nil
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
		return NewVaultReader(addr, token, mount), nil
	case "aws-secrets-manager", "kubernetes":
		return nil, fmt.Errorf("secret store backend %q is not yet implemented in the control plane; configure vault or memory", backend)
	default:
		return nil, fmt.Errorf("unknown secret store backend %q", backend)
	}
}

// VaultReader reads secrets from a HashiCorp Vault KV v2 engine over its HTTP API.
// It is the read mirror of VaultStore (vault.go) and reads the value written under
// vaultValueKey.
type VaultReader struct {
	addr       string
	token      string
	mount      string
	httpClient *http.Client
}

// NewVaultReader returns a VaultReader for the given address, token, and KV v2
// mount (e.g. "secret").
func NewVaultReader(addr, token, mount string) *VaultReader {
	return &VaultReader{
		addr:       strings.TrimRight(addr, "/"),
		token:      strings.TrimSpace(token),
		mount:      strings.Trim(mount, "/"),
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// vaultReadResponse is the KV v2 read envelope: the stored fields live under
// .data.data (the outer data is the KV v2 wrapper, the inner data is the secret).
type vaultReadResponse struct {
	Data struct {
		Data map[string]string `json:"data"`
	} `json:"data"`
}

// Get reads value at location from the KV v2 engine and returns the field stored
// under vaultValueKey (the key VaultStore.Put writes).
func (v *VaultReader) Get(ctx context.Context, location string) (string, error) {
	if location == "" {
		return "", fmt.Errorf("secret location is required")
	}
	url := fmt.Sprintf("%s/v1/%s/data/%s", v.addr, v.mount, strings.TrimLeft(location, "/"))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("build vault request: %w", err)
	}
	req.Header.Set("X-Vault-Token", v.token)

	resp, err := v.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("vault read: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return "", fmt.Errorf("secret not found at location %q", location)
	}
	if resp.StatusCode != http.StatusOK {
		// Read a bounded amount of the body for context; it does not echo the
		// stored value on an error status.
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return "", fmt.Errorf("vault read from %s returned status %d: %s", url, resp.StatusCode, strings.TrimSpace(string(body)))
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("read vault response: %w", err)
	}
	var parsed vaultReadResponse
	if err := json.Unmarshal(bytes.TrimSpace(body), &parsed); err != nil {
		return "", fmt.Errorf("decode vault response: %w", err)
	}
	value, ok := parsed.Data.Data[vaultValueKey]
	if !ok {
		return "", fmt.Errorf("vault secret at location %q has no %q field", location, vaultValueKey)
	}
	return value, nil
}
