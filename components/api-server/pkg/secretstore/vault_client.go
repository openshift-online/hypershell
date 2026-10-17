package secretstore

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// defaultServiceAccountTokenPath is where a Kubernetes projects the pod's
// ServiceAccount JWT. The Vault Kubernetes auth method exchanges this JWT for a
// short-lived Vault token.
const defaultServiceAccountTokenPath = "/var/run/secrets/kubernetes.io/serviceaccount/token"

// vaultConfig is the resolved connection configuration shared by VaultStore (write)
// and VaultReader (read). It is built from the HYPERSHELL_SECRET_STORE_* environment
// (configuration, not code) so a single deployment setting drives both directions.
// The agent-secret Vault plane is served with a private CA and authenticated with
// Kubernetes auth on a real cluster; a static token remains supported for the
// co-located and development cases (agent-secret-late-binding.spec.md "Agent-Secret
// Vault Client Capabilities").
type vaultConfig struct {
	addr       string
	mount      string
	httpClient *http.Client
	tokens     tokenSource
}

// tokenSource provides the X-Vault-Token value for a request. Separating it lets the
// client support both a static token and the Kubernetes auth login/renew flow without
// the read and write paths knowing which is in use.
type tokenSource interface {
	token(ctx context.Context) (string, error)
}

// staticTokenSource returns a fixed token (the X-Vault-Token auth method).
type staticTokenSource struct{ value string }

func (s staticTokenSource) token(context.Context) (string, error) {
	if s.value == "" {
		return "", fmt.Errorf("vault token is empty")
	}
	return s.value, nil
}

// kubernetesTokenSource logs in to Vault's Kubernetes auth method with the pod's
// ServiceAccount JWT and caches the returned Vault token until shortly before it
// expires, re-logging in as needed. It never writes the ServiceAccount JWT or the
// Vault token to logs.
type kubernetesTokenSource struct {
	addr        string
	authMount   string
	role        string
	saTokenPath string
	httpClient  *http.Client

	mu       sync.Mutex
	cached   string
	expiry   time.Time
	nowFunc  func() time.Time
	readFile func(string) ([]byte, error)
}

func (k *kubernetesTokenSource) now() time.Time {
	if k.nowFunc != nil {
		return k.nowFunc()
	}
	return time.Now()
}

func (k *kubernetesTokenSource) read(path string) ([]byte, error) {
	if k.readFile != nil {
		return k.readFile(path)
	}
	return os.ReadFile(path)
}

// token returns a cached Vault token when it is still valid, otherwise it performs a
// fresh Kubernetes auth login. A small skew before expiry forces renewal so a token
// is not used in the instant it expires.
func (k *kubernetesTokenSource) token(ctx context.Context) (string, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.cached != "" && k.now().Before(k.expiry) {
		return k.cached, nil
	}
	tok, ttl, err := k.login(ctx)
	if err != nil {
		return "", err
	}
	k.cached = tok
	// Renew at half the lease, bounded below so a very short lease still leaves time
	// to use the token.
	renewIn := ttl / 2
	if renewIn < 30*time.Second {
		renewIn = 30 * time.Second
	}
	k.expiry = k.now().Add(renewIn)
	return tok, nil
}

type kubernetesLoginResponse struct {
	Auth struct {
		ClientToken   string `json:"client_token"`
		LeaseDuration int    `json:"lease_duration"`
	} `json:"auth"`
}

// login exchanges the ServiceAccount JWT for a Vault token via
// POST {addr}/v1/auth/{mount}/login. Errors never include the JWT or the token.
func (k *kubernetesTokenSource) login(ctx context.Context) (string, time.Duration, error) {
	jwtBytes, err := k.read(k.saTokenPath)
	if err != nil {
		return "", 0, fmt.Errorf("read service account token %s: %w", k.saTokenPath, err)
	}
	jwt := strings.TrimSpace(string(jwtBytes))
	if jwt == "" {
		return "", 0, fmt.Errorf("service account token at %s is empty", k.saTokenPath)
	}
	payload, err := json.Marshal(map[string]string{"role": k.role, "jwt": jwt})
	if err != nil {
		return "", 0, fmt.Errorf("marshal vault kubernetes login: %w", err)
	}
	url := fmt.Sprintf("%s/v1/auth/%s/login", k.addr, k.authMount)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return "", 0, fmt.Errorf("build vault kubernetes login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := k.httpClient.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("vault kubernetes login: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return "", 0, fmt.Errorf("vault kubernetes login to %s returned status %d: %s", url, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", 0, fmt.Errorf("read vault kubernetes login response: %w", err)
	}
	var parsed kubernetesLoginResponse
	if err := json.Unmarshal(bytes.TrimSpace(body), &parsed); err != nil {
		return "", 0, fmt.Errorf("decode vault kubernetes login response: %w", err)
	}
	if parsed.Auth.ClientToken == "" {
		return "", 0, fmt.Errorf("vault kubernetes login returned no client token")
	}
	return parsed.Auth.ClientToken, time.Duration(parsed.Auth.LeaseDuration) * time.Second, nil
}

// newVaultHTTPClient builds the HTTP client, trusting the private CA bundle at
// caCertPath in addition to the system roots when a path is configured. A Vault
// served with a private CA (for example the cluster Vault's
// openshell-gateway-vault-tls serving CA) therefore completes its TLS handshake
// instead of failing for an untrusted authority.
func newVaultHTTPClient(caCertPath string) (*http.Client, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	if caCertPath == "" {
		return client, nil
	}
	pem, err := os.ReadFile(caCertPath)
	if err != nil {
		return nil, fmt.Errorf("read vault CA bundle %s: %w", caCertPath, err)
	}
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		// SystemCertPool failed (or is empty on this platform): fall back to a pool
		// holding only the configured private CA. This is intended for a private-CA Vault
		// where the system roots are not required; a store reachable only via a
		// public CA would then fail its handshake and surface loudly rather than silently.
		pool = x509.NewCertPool()
	}
	if !pool.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("vault CA bundle %s contains no valid certificate", caCertPath)
	}
	client.Transport = &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}}
	return client, nil
}

// vaultConfigFromEnv resolves the shared Vault configuration from the
// HYPERSHELL_SECRET_STORE_VAULT_* environment. It supports a static token
// (default) and the Kubernetes auth method, and an optional private-CA bundle.
func vaultConfigFromEnv() (vaultConfig, error) {
	addr := os.Getenv("HYPERSHELL_SECRET_STORE_VAULT_ADDR")
	if addr == "" {
		return vaultConfig{}, fmt.Errorf("vault secret store requires HYPERSHELL_SECRET_STORE_VAULT_ADDR")
	}
	mount := os.Getenv("HYPERSHELL_SECRET_STORE_VAULT_MOUNT")
	if mount == "" {
		mount = "secret"
	}
	httpClient, err := newVaultHTTPClient(os.Getenv("HYPERSHELL_SECRET_STORE_VAULT_CACERT"))
	if err != nil {
		return vaultConfig{}, err
	}

	cfg := vaultConfig{
		addr:       strings.TrimRight(addr, "/"),
		mount:      strings.Trim(mount, "/"),
		httpClient: httpClient,
	}

	switch method := os.Getenv("HYPERSHELL_SECRET_STORE_VAULT_AUTH_METHOD"); method {
	case "", "token":
		token := os.Getenv("HYPERSHELL_SECRET_STORE_VAULT_TOKEN")
		if token == "" {
			return vaultConfig{}, fmt.Errorf("vault token auth requires HYPERSHELL_SECRET_STORE_VAULT_TOKEN")
		}
		cfg.tokens = staticTokenSource{value: strings.TrimSpace(token)}
	case "kubernetes":
		role := os.Getenv("HYPERSHELL_SECRET_STORE_VAULT_ROLE")
		if role == "" {
			return vaultConfig{}, fmt.Errorf("vault kubernetes auth requires HYPERSHELL_SECRET_STORE_VAULT_ROLE")
		}
		authMount := os.Getenv("HYPERSHELL_SECRET_STORE_VAULT_KUBERNETES_AUTH_MOUNT")
		if authMount == "" {
			authMount = "kubernetes"
		}
		saTokenPath := os.Getenv("HYPERSHELL_SECRET_STORE_VAULT_KUBERNETES_TOKEN_PATH")
		if saTokenPath == "" {
			saTokenPath = defaultServiceAccountTokenPath
		}
		cfg.tokens = &kubernetesTokenSource{
			addr:        cfg.addr,
			authMount:   strings.Trim(authMount, "/"),
			role:        role,
			saTokenPath: saTokenPath,
			httpClient:  httpClient,
		}
	default:
		return vaultConfig{}, fmt.Errorf("unknown vault auth method %q (want token or kubernetes)", method)
	}
	return cfg, nil
}
