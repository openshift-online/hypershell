package gateway

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// Fake-Vault coverage for the fail-closed Vault probes (HYPERSHELL-369). The e2e step P2.7
// covers only the "CA is not a certificate" case end to end; the deeper failure modes
// (wrong authority, sealed Vault, denied login) are covered here against httptest Vaults.

// caSecretWith returns the Vault serving-CA source Secret at the default location holding
// caPEM.
func caSecretWith(caPEM []byte) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: VaultCADefaultSourceNamespace, Name: VaultCADefaultSourceSecret},
		Data:       map[string][]byte{VaultCAKey: caPEM},
	}
}

func healthyVault(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/sys/health":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"initialized":true,"sealed":false}`))
		case "/v1/auth/kubernetes/login":
			_, _ = w.Write([]byte(`{"auth":{"client_token":"hvs.PROBE"}}`))
		case "/v1/auth/token/revoke-self":
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// otherAuthorityPEM returns a valid, freshly minted self-signed CA certificate that did NOT
// sign the httptest server certificate.
func otherAuthorityPEM(t *testing.T) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "unrelated-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

// Verifies: OSG-CRED-07.01
func TestProbeVaultReachable_FailsClosed(t *testing.T) {
	srv := healthyVault(t)
	t.Setenv("GATEWAY_VAULT_ADDRESS", srv.URL)
	goodCA := vaultTLSCASecret(t, srv).Data[VaultCAKey]

	sealed := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable) // sys/health: sealed
		_, _ = w.Write([]byte(`{"initialized":true,"sealed":true}`))
	}))
	t.Cleanup(sealed.Close)

	cases := []struct {
		name    string
		address string
		ca      []byte
		wantErr string
	}{
		{"healthy", srv.URL, goodCA, ""},
		{"non-PEM CA (P2.7 placeholder)", srv.URL, []byte("fail-closed-e2e: not a certificate"), "no usable certificate"},
		{"CA from a different authority", srv.URL, otherAuthorityPEM(t), "vault unreachable"},
		{"sealed Vault (503)", sealed.URL, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: sealed.Certificate().Raw}), "not ready"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GATEWAY_VAULT_ADDRESS", tc.address)
			cs, _ := fakeClientsetWithToken(t, caSecretWith(tc.ca))
			err := probeVaultReachable(context.Background(), cs)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("probeVaultReachable: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("probeVaultReachable err = %v; want it to contain %q", err, tc.wantErr)
			}
		})
	}

	t.Run("missing CA Secret", func(t *testing.T) {
		cs, _ := fakeClientsetWithToken(t)
		if err := probeVaultReachable(context.Background(), cs); err == nil {
			t.Fatal("expected an error when the CA source Secret is missing")
		}
	})

	t.Run("CA source override", func(t *testing.T) {
		t.Setenv("GATEWAY_VAULT_ADDRESS", srv.URL)
		t.Setenv("GATEWAY_VAULT_CA_SECRET", "hypershell-system/hypershell-vault-ca")
		copySecret := caSecretWith(goodCA)
		copySecret.Namespace, copySecret.Name = "hypershell-system", "hypershell-vault-ca"
		// The default-location Secret is broken; the probe must read only the override.
		cs, _ := fakeClientsetWithToken(t, copySecret, caSecretWith([]byte("broken")))
		if err := probeVaultReachable(context.Background(), cs); err != nil {
			t.Fatalf("probeVaultReachable with override: %v", err)
		}
	})
}

// Verifies: OSG-CRED-07.02
func TestProbeVaultAuth(t *testing.T) {
	var mu sync.Mutex
	var loginRole string
	denied := false
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/auth/kubernetes/login":
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			mu.Lock()
			loginRole = body["role"]
			d := denied
			mu.Unlock()
			if d {
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"errors":["permission denied"]}`))
				return
			}
			_, _ = w.Write([]byte(`{"auth":{"client_token":"hvs.PROBE"}}`))
		case "/v1/auth/token/revoke-self":
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("GATEWAY_VAULT_ADDRESS", srv.URL)

	cs, last := fakeClientsetWithToken(t, vaultTLSCASecret(t, srv))
	if err := probeVaultAuth(context.Background(), cs, "openshell-acme"); err != nil {
		t.Fatalf("probeVaultAuth: %v", err)
	}
	if got := last.Load(); got == nil || got[0] != "openshell-acme" {
		t.Errorf("token minted for %v; want namespace openshell-acme", got)
	}
	if loginRole != VaultRoleDefault() {
		t.Errorf("login role = %q; want %q", loginRole, VaultRoleDefault())
	}

	mu.Lock()
	denied = true
	mu.Unlock()
	if err := probeVaultAuth(context.Background(), cs, "openshell-acme"); err == nil || !strings.Contains(err.Error(), "denied") {
		t.Fatalf("probeVaultAuth err = %v; want a login denial", err)
	}
}

// fakeAPIServer is a minimal Kubernetes API that serves the calls ReconcileGateway makes
// before the Vault reachability probe (namespace get/create, CA Secret get) and records
// every request, so a test can prove nothing past EnvironmentReady ran.
type fakeAPIServer struct {
	mu       sync.Mutex
	requests []string
	caPEM    []byte
}

func (f *fakeAPIServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.requests = append(f.requests, r.Method+" "+r.URL.Path)
	f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	switch {
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/namespaces/") && strings.Count(r.URL.Path, "/") == 4:
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"kind":"Status","apiVersion":"v1","status":"Failure","reason":"NotFound","code":404}`))
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/namespaces":
		var ns corev1.Namespace
		_ = json.NewDecoder(r.Body).Decode(&ns)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(&ns)
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/namespaces/"+VaultCADefaultSourceNamespace+"/secrets/"+VaultCADefaultSourceSecret:
		s := caSecretWith(f.caPEM)
		s.APIVersion, s.Kind = "v1", "Secret"
		_ = json.NewEncoder(w).Encode(s)
	default:
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"kind":"Status","apiVersion":"v1","status":"Failure","reason":"NotFound","code":404}`))
	}
}

// When the Vault reachability probe fails, the reconcile marks EnvironmentReady Failed with
// the Vault message and stops: no DatabaseReady or later step runs, and nothing past the
// namespace is written to the cluster.
//
// Verifies: OSG-CRED-07.01
func TestReconcileGateway_VaultUnreachableFailsClosed(t *testing.T) {
	vault := healthyVault(t)
	t.Setenv("GATEWAY_VAULT_ADDRESS", vault.URL)

	api := &fakeAPIServer{caPEM: []byte("fail-closed-e2e: not a certificate")}
	apiSrv := httptest.NewServer(api)
	t.Cleanup(apiSrv.Close)
	clientset, err := kubernetes.NewForConfig(&rest.Config{Host: apiSrv.URL})
	if err != nil {
		t.Fatalf("clientset: %v", err)
	}

	disc := &fakeDiscovery{byGV: map[string]*metav1.APIResourceList{
		"gateway.networking.k8s.io/v1": presentList("gateway.networking.k8s.io/v1", "GRPCRoute"),
		"cert-manager.io/v1":           presentList("cert-manager.io/v1", "Certificate"),
	}}
	var reports []reconcileReport
	opts := ReconcileOpts{
		Dependencies:          NewDependencyDiscovery(disc),
		ReportProgress:        collectReports(&reports),
		ControlPlaneNamespace: "hypershell-system",
	}
	nsConfig := NamespaceConfig{Name: "openshell-abc", Gateway: GatewayConfig{
		WorkspaceMode: "shared",
		OIDC:          OIDCConfig{Issuer: "https://issuer.example", Audience: "openshell"},
	}}

	err = ReconcileGateway(context.Background(), nil, clientset, nil, nsConfig, opts)
	if err == nil || !strings.Contains(err.Error(), "vault reachability probe failed") {
		t.Fatalf("ReconcileGateway err = %v; want a vault reachability failure", err)
	}
	assertLastReport(t, reports, reconcileReport{ConditionEnvironmentReady, StatusFailed, "Vault credential backend is unreachable"})
	for _, r := range reports {
		if r.condition != ConditionEnvironmentReady {
			t.Errorf("unexpected report past EnvironmentReady: %+v", r)
		}
	}
	api.mu.Lock()
	defer api.mu.Unlock()
	for _, req := range api.requests {
		if strings.HasPrefix(req, "GET ") || req == "POST /api/v1/namespaces" {
			continue
		}
		t.Errorf("unexpected cluster write before a failed Vault probe: %s", req)
	}
}
