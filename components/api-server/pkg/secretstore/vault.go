package secretstore

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// vaultValueKey is the field name the submitted value is stored under in the
// Vault KV v2 secret. The control plane's ExternalSecret rendering remaps keys by
// SecretSource purpose/key_mappings, so a single conventional key is sufficient
// here.
const vaultValueKey = "value"

// VaultStore writes secrets to a HashiCorp Vault KV v2 engine over its HTTP API.
// It is the implemented real backend for secret submission. Writes are idempotent:
// a KV v2 write at a fixed path overwrites the current version, so a retried
// submission to the same deterministic location does not fork the secret.
type VaultStore struct {
	addr       string
	token      string
	mount      string
	httpClient *http.Client
}

// NewVaultStore returns a VaultStore for the given address, token, and KV v2 mount
// (e.g. "secret").
func NewVaultStore(addr, token, mount string) *VaultStore {
	return &VaultStore{
		addr:       strings.TrimRight(addr, "/"),
		token:      token,
		mount:      strings.Trim(mount, "/"),
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// Put writes value at location in the KV v2 engine. The value is wrapped in the
// KV v2 {"data": {...}} envelope under vaultValueKey.
func (v *VaultStore) Put(ctx context.Context, location, value string) error {
	if location == "" {
		return fmt.Errorf("secret location is required")
	}
	url := fmt.Sprintf("%s/v1/%s/data/%s", v.addr, v.mount, strings.TrimLeft(location, "/"))

	payload, err := json.Marshal(map[string]any{
		"data": map[string]string{vaultValueKey: value},
	})
	if err != nil {
		return fmt.Errorf("marshal vault payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build vault request: %w", err)
	}
	req.Header.Set("X-Vault-Token", v.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := v.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("vault write: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		// Read a bounded amount of the body for context; it does not echo the
		// written value.
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("vault write to %s returned status %d: %s", url, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}
