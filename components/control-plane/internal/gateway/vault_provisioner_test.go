package gateway

import (
	"context"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	authnv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

// TestResolveVaultValues_PerTenantMount pins the per-tenant mount derivation: the mount is
// always openshell-gw/<namespace> (Vault is a black box - no per-gateway override), and an
// empty namespace (probe callers that never use the mount) leaves it unset.
func TestResolveVaultValues_PerTenantMount(t *testing.T) {
	t.Run("derived from namespace", func(t *testing.T) {
		v := resolveVaultValues("openshell-acme")
		if v.Mount != "openshell-gw/openshell-acme" {
			t.Errorf("mount = %q; want openshell-gw/openshell-acme", v.Mount)
		}
	})
	t.Run("empty namespace leaves mount unset", func(t *testing.T) {
		v := resolveVaultValues("")
		if v.Mount != "" {
			t.Errorf("mount = %q; want empty", v.Mount)
		}
	})
}

func TestVaultClient_EnableKVv2Mount(t *testing.T) {
	t.Run("enable succeeds", func(t *testing.T) {
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost || r.URL.Path != "/v1/sys/mounts/openshell-gw/openshell-ns" {
				t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			}
			w.WriteHeader(http.StatusNoContent)
		}))
		defer srv.Close()
		c := newTestVaultClient(t, srv)
		if err := c.enableKVv2Mount(context.Background(), "tok", "openshell-gw/openshell-ns"); err != nil {
			t.Fatalf("enableKVv2Mount: %v", err)
		}
	})

	t.Run("already in use is idempotent", func(t *testing.T) {
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"errors":["path is already in use at openshell-gw/openshell-ns/"]}`))
		}))
		defer srv.Close()
		c := newTestVaultClient(t, srv)
		if err := c.enableKVv2Mount(context.Background(), "tok", "openshell-gw/openshell-ns"); err != nil {
			t.Fatalf("already-in-use should be a no-op, got %v", err)
		}
	})

	t.Run("other error propagates", func(t *testing.T) {
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"errors":["permission denied"]}`))
		}))
		defer srv.Close()
		c := newTestVaultClient(t, srv)
		if err := c.enableKVv2Mount(context.Background(), "tok", "openshell-gw/openshell-ns"); err == nil {
			t.Fatal("expected error on 403, got nil")
		}
	})
}

func TestVaultClient_DisableMount(t *testing.T) {
	t.Run("disable succeeds", func(t *testing.T) {
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodDelete || r.URL.Path != "/v1/sys/mounts/openshell-gw/openshell-ns" {
				t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			}
			w.WriteHeader(http.StatusNoContent)
		}))
		defer srv.Close()
		c := newTestVaultClient(t, srv)
		if err := c.disableMount(context.Background(), "tok", "openshell-gw/openshell-ns"); err != nil {
			t.Fatalf("disableMount: %v", err)
		}
	})

	t.Run("not found is idempotent", func(t *testing.T) {
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer srv.Close()
		c := newTestVaultClient(t, srv)
		if err := c.disableMount(context.Background(), "tok", "openshell-gw/openshell-ns"); err != nil {
			t.Fatalf("404 should be a no-op, got %v", err)
		}
	})
}

// vaultTLSCASecret PEM-encodes an httptest TLS server's certificate into the Vault
// serving-CA source Secret the provisioner reads, so the control plane's own vaultClient
// trusts the test server over real TLS.
func vaultTLSCASecret(t *testing.T, srv *httptest.Server) *corev1.Secret {
	t.Helper()
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	if len(caPEM) == 0 {
		t.Fatal("failed to PEM-encode test server certificate")
	}
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: VaultCADefaultSourceNamespace, Name: VaultCADefaultSourceSecret},
		Data:       map[string][]byte{VaultCAKey: caPEM},
	}
}

// fakeClientsetWithToken returns a fake clientset that serves a non-empty token for every
// serviceaccounts/token (TokenRequest) create, recording the namespace/SA it was asked for.
func fakeClientsetWithToken(t *testing.T, objs ...runtime.Object) (*k8sfake.Clientset, *atomic.Pointer[[2]string]) {
	t.Helper()
	cs := k8sfake.NewSimpleClientset(objs...)
	last := &atomic.Pointer[[2]string]{}
	cs.PrependReactor("create", "serviceaccounts", func(action k8stesting.Action) (bool, runtime.Object, error) {
		if action.GetSubresource() != "token" {
			return false, nil, nil
		}
		ns := action.GetNamespace()
		name := action.(k8stesting.CreateAction).GetObject().(*authnv1.TokenRequest).Name
		// CreateAction for a subresource carries the SA name in the action, not the object;
		// fall back to the object name when present, else leave it to the handler.
		rec := [2]string{ns, name}
		last.Store(&rec)
		return true, &authnv1.TokenRequest{Status: authnv1.TokenRequestStatus{Token: "sa-jwt"}}, nil
	})
	return cs, last
}

// TestEnsureAndDeleteVaultTenantMount drives the provisioner end to end against an httptest
// Vault: the control plane mints its own SA token, logs in to the provisioner role, and
// enables (then deletes) the per-tenant mount at openshell-gw/<namespace>.
func TestEnsureAndDeleteVaultTenantMount(t *testing.T) {
	var enabled, disabled, loggedIn atomic.Bool
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/auth/kubernetes/login":
			loggedIn.Store(true)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"auth":{"client_token":"hvs.PROVISIONER"}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/sys/mounts/openshell-gw/openshell-acme":
			if got := r.Header.Get("X-Vault-Token"); got != "hvs.PROVISIONER" {
				t.Errorf("mount enable used token %q; want hvs.PROVISIONER", got)
			}
			enabled.Store(true)
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/sys/mounts/openshell-gw/openshell-acme":
			disabled.Store(true)
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusTeapot)
		}
	}))
	defer srv.Close()

	cs, _ := fakeClientsetWithToken(t, vaultTLSCASecret(t, srv))
	opts := ReconcileOpts{ControlPlaneNamespace: "hypershell-system"}
	// Vault is a black box with no per-gateway overrides; point the provisioner at the test
	// server via the control-plane deployment env (the only supported address override).
	t.Setenv("GATEWAY_VAULT_ADDRESS", srv.URL)

	if err := EnsureVaultTenantMount(context.Background(), cs, opts, "openshell-acme"); err != nil {
		t.Fatalf("EnsureVaultTenantMount: %v", err)
	}
	if !loggedIn.Load() || !enabled.Load() {
		t.Fatalf("expected provisioner login and mount enable; loggedIn=%v enabled=%v", loggedIn.Load(), enabled.Load())
	}

	if err := DeleteVaultTenantMount(context.Background(), cs, opts, "openshell-acme"); err != nil {
		t.Fatalf("DeleteVaultTenantMount: %v", err)
	}
	if !disabled.Load() {
		t.Fatal("expected mount disable to be called")
	}
}

// TestIsVaultBacked locks the contract the delete path depends on: a gateway stored with NO
// credential_driver (NULL -> empty string via the getter) still resolves to vault, so its
// per-tenant mount must be reclaimed on delete. Regression guard for the leak where a nil
// CredentialDriver skipped mount deletion (amarin review).
func TestIsVaultBacked(t *testing.T) {
	cases := []struct {
		name string
		json string
		want bool
	}{
		{"empty (NULL default) is vault-backed", "", true},
		{"empty-type object is vault-backed", `{}`, true},
		{"explicit vault is vault-backed", `{"type":"vault"}`, true},
		{"kek-db is not vault-backed", `{"type":"kek-db"}`, false},
		{"kubernetes-secrets is not vault-backed", `{"type":"kubernetes-secrets"}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsVaultBacked(tc.json); got != tc.want {
				t.Errorf("IsVaultBacked(%q) = %v; want %v", tc.json, got, tc.want)
			}
		})
	}
}
