package secretstore

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMemoryReader_Get(t *testing.T) {
	r := NewMemoryReader(map[string]string{"a/b": "val"})
	got, err := r.Get(context.Background(), "a/b")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != "val" {
		t.Fatalf("Get = %q, want val", got)
	}

	if _, err := r.Get(context.Background(), "missing"); err == nil {
		t.Fatalf("expected an error reading a missing location")
	}
	if _, err := r.Get(context.Background(), ""); err == nil {
		t.Fatalf("expected an error for an empty location")
	}

	// Set lets a test seed after construction.
	r.Set("c/d", "later")
	if got, _ := r.Get(context.Background(), "c/d"); got != "later" {
		t.Fatalf("Get after Set = %q, want later", got)
	}
}

func TestDisabledReader_RejectsReads(t *testing.T) {
	t.Setenv("HYPERSHELL_SECRET_STORE_BACKEND", "")
	r, err := NewReaderFromEnv()
	if err != nil {
		t.Fatalf("NewReaderFromEnv: %v", err)
	}
	if _, err := r.Get(context.Background(), "any"); err == nil {
		t.Fatalf("disabled reader must reject reads")
	}
}

func TestErrorReader_FailsLoudly(t *testing.T) {
	sentinel := errors.New("boom")
	r := NewErrorReader(sentinel)
	_, err := r.Get(context.Background(), "x")
	if err == nil || !errors.Is(err, sentinel) {
		t.Fatalf("error reader must wrap the configuration error, got %v", err)
	}
}

func TestNewReaderFromEnv_BackendSelection(t *testing.T) {
	t.Run("memory", func(t *testing.T) {
		t.Setenv("HYPERSHELL_SECRET_STORE_BACKEND", "memory")
		r, err := NewReaderFromEnv()
		if err != nil {
			t.Fatalf("NewReaderFromEnv: %v", err)
		}
		if _, ok := r.(*MemoryReader); !ok {
			t.Fatalf("memory backend must yield a *MemoryReader, got %T", r)
		}
	})
	t.Run("vault requires addr and token", func(t *testing.T) {
		t.Setenv("HYPERSHELL_SECRET_STORE_BACKEND", "vault")
		t.Setenv("HYPERSHELL_SECRET_STORE_VAULT_ADDR", "")
		t.Setenv("HYPERSHELL_SECRET_STORE_VAULT_TOKEN", "")
		if _, err := NewReaderFromEnv(); err == nil {
			t.Fatalf("vault backend without addr/token must error")
		}
	})
	t.Run("vault", func(t *testing.T) {
		t.Setenv("HYPERSHELL_SECRET_STORE_BACKEND", "vault")
		t.Setenv("HYPERSHELL_SECRET_STORE_VAULT_ADDR", "https://vault.example.com")
		t.Setenv("HYPERSHELL_SECRET_STORE_VAULT_TOKEN", "tok")
		r, err := NewReaderFromEnv()
		if err != nil {
			t.Fatalf("NewReaderFromEnv: %v", err)
		}
		if _, ok := r.(*VaultReader); !ok {
			t.Fatalf("vault backend must yield a *VaultReader, got %T", r)
		}
	})
	t.Run("unimplemented backend", func(t *testing.T) {
		t.Setenv("HYPERSHELL_SECRET_STORE_BACKEND", "aws-secrets-manager")
		if _, err := NewReaderFromEnv(); err == nil {
			t.Fatalf("unimplemented backend must error")
		}
	})
	t.Run("unknown backend", func(t *testing.T) {
		t.Setenv("HYPERSHELL_SECRET_STORE_BACKEND", "nonsense")
		if _, err := NewReaderFromEnv(); err == nil {
			t.Fatalf("unknown backend must error")
		}
	})
}

func TestVaultReader_Get(t *testing.T) {
	var gotPath, gotToken, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		gotPath = req.URL.Path
		gotToken = req.Header.Get("X-Vault-Token")
		gotMethod = req.Method
		_, _ = w.Write([]byte(`{"data":{"data":{"value":"the-secret"}}}`))
	}))
	defer srv.Close()

	r := NewVaultReader(srv.URL, "tok", "secret")
	got, err := r.Get(context.Background(), "hypershell/secret-sources/ar/github")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != "the-secret" {
		t.Fatalf("Get = %q, want the-secret", got)
	}
	if gotMethod != http.MethodGet {
		t.Fatalf("method = %q, want GET", gotMethod)
	}
	if gotPath != "/v1/secret/data/hypershell/secret-sources/ar/github" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotToken != "tok" {
		t.Fatalf("token header = %q, want tok", gotToken)
	}
}

func TestVaultReader_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	r := NewVaultReader(srv.URL, "tok", "secret")
	_, err := r.Get(context.Background(), "missing")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected a not-found error, got %v", err)
	}
}

func TestVaultReader_MissingValueField(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"data":{"other":"x"}}}`))
	}))
	defer srv.Close()

	r := NewVaultReader(srv.URL, "tok", "secret")
	if _, err := r.Get(context.Background(), "x"); err == nil {
		t.Fatalf("expected an error when the value field is absent")
	}
}

func TestVaultReader_ErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("permission denied"))
	}))
	defer srv.Close()

	r := NewVaultReader(srv.URL, "tok", "secret")
	_, err := r.Get(context.Background(), "x")
	if err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("expected an error mentioning 403, got %v", err)
	}
}
