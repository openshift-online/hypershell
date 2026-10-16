package secretstore

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMemoryStore_PutThenGet(t *testing.T) {
	s := NewMemoryStore()
	if err := s.Put(context.Background(), "hypershell/secret-sources/ar-1/gw", "topsecret"); err != nil {
		t.Fatalf("Put error = %v", err)
	}
	got, ok := s.Get("hypershell/secret-sources/ar-1/gw")
	if !ok || got != "topsecret" {
		t.Fatalf("Get = (%q,%v), want (topsecret,true)", got, ok)
	}
}

func TestMemoryStore_PutIsIdempotentOverwrite(t *testing.T) {
	s := NewMemoryStore()
	loc := "hypershell/secret-sources/ar-1/gw"
	_ = s.Put(context.Background(), loc, "v1")
	_ = s.Put(context.Background(), loc, "v2")
	if got, _ := s.Get(loc); got != "v2" {
		t.Fatalf("after overwrite Get = %q, want v2", got)
	}
}

func TestMemoryStore_PutRequiresLocation(t *testing.T) {
	if err := NewMemoryStore().Put(context.Background(), "", "x"); err == nil {
		t.Fatal("expected error for empty location")
	}
}

func TestDisabledStore_RejectsWrite(t *testing.T) {
	store, err := NewFromEnv() // unset backend -> disabled
	if err != nil {
		t.Fatalf("NewFromEnv error = %v", err)
	}
	if err := store.Put(context.Background(), "loc", "v"); err == nil {
		t.Fatal("disabled store must reject writes")
	}
}

func TestNewFromEnv_Backends(t *testing.T) {
	t.Setenv("HYPERSHELL_SECRET_STORE_BACKEND", "memory")
	if s, err := NewFromEnv(); err != nil || s == nil {
		t.Fatalf("memory backend: (%v,%v)", s, err)
	}

	t.Setenv("HYPERSHELL_SECRET_STORE_BACKEND", "vault")
	if _, err := NewFromEnv(); err == nil {
		t.Fatal("vault without addr/token must error")
	}
	t.Setenv("HYPERSHELL_SECRET_STORE_VAULT_ADDR", "https://vault.example.com")
	t.Setenv("HYPERSHELL_SECRET_STORE_VAULT_TOKEN", "t")
	if s, err := NewFromEnv(); err != nil || s == nil {
		t.Fatalf("vault backend: (%v,%v)", s, err)
	}

	t.Setenv("HYPERSHELL_SECRET_STORE_BACKEND", "aws-secrets-manager")
	if _, err := NewFromEnv(); err == nil {
		t.Fatal("unsupported backend must error, not silently degrade")
	}

	t.Setenv("HYPERSHELL_SECRET_STORE_BACKEND", "bogus")
	if _, err := NewFromEnv(); err == nil {
		t.Fatal("unknown backend must error")
	}
}

func TestVaultStore_PutWritesKVv2(t *testing.T) {
	var gotPath, gotToken string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotToken = r.Header.Get("X-Vault-Token")
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{"version":1}}`))
	}))
	defer srv.Close()

	store := NewVaultStore(srv.URL, "my-token", "secret")
	if err := store.Put(context.Background(), "hypershell/secret-sources/ar-1/gw", "topsecret"); err != nil {
		t.Fatalf("Put error = %v", err)
	}
	if gotPath != "/v1/secret/data/hypershell/secret-sources/ar-1/gw" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotToken != "my-token" {
		t.Fatalf("token = %q", gotToken)
	}
	data, _ := gotBody["data"].(map[string]any)
	if data["value"] != "topsecret" {
		t.Fatalf("stored value = %v, want topsecret", data["value"])
	}
}

func TestVaultStore_PutSurfacesHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"errors":["permission denied"]}`))
	}))
	defer srv.Close()

	err := NewVaultStore(srv.URL, "t", "secret").Put(context.Background(), "loc", "v")
	if err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("expected 403 error, got %v", err)
	}
}
