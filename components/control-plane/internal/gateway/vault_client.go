package gateway

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	authnv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// vaultClient performs the few operations the control plane needs against a
// per-cluster provider Vault, acting ONLY as a gateway's own tenant identity (never
// as a Vault administrator): a reachability/auth probe (CRED-REACH). It speaks the
// Vault HTTP API directly to avoid pulling in the full Vault SDK. See
// openshell-gateway-credentials.spec.md.
type vaultClient struct {
	address string
	http    *http.Client
}

// newVaultClient builds a client for a Vault address, trusting caPEM (the per-cluster
// serving CA). A non-loopback address is expected to be https; an empty caPEM falls
// back to the system trust store.
func newVaultClient(address string, caPEM []byte, timeout time.Duration) (*vaultClient, error) {
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if len(caPEM) > 0 {
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caPEM) {
			return nil, fmt.Errorf("vault CA bundle contains no usable certificate")
		}
		tlsCfg.RootCAs = pool
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &vaultClient{
		address: strings.TrimRight(address, "/"),
		// DisableKeepAlives: this client is one-shot (a probe makes one or two requests
		// then is discarded), so no idle keep-alive connection should outlive it (CWE-404).
		http: &http.Client{Timeout: timeout, Transport: &http.Transport{TLSClientConfig: tlsCfg, DisableKeepAlives: true}},
	}, nil
}

// login exchanges a Kubernetes ServiceAccount JWT for a Vault client token via the
// kubernetes auth mount and the given role. A successful login proves the Vault is
// reachable, its TLS trusts caPEM, and the gateway identity is authorized.
func (c *vaultClient) login(ctx context.Context, authMount, role, saJWT string) (string, error) {
	payload, _ := json.Marshal(map[string]string{"role": role, "jwt": saJWT})
	url := fmt.Sprintf("%s/v1/auth/%s/login", c.address, strings.Trim(authMount, "/"))
	resp, body, err := c.do(ctx, http.MethodPost, url, "", payload)
	if err != nil {
		return "", fmt.Errorf("vault login request: %w", err)
	}
	if resp != http.StatusOK {
		return "", fmt.Errorf("vault login denied (HTTP %d)", resp)
	}
	var out struct {
		Auth struct {
			ClientToken string `json:"client_token"`
		} `json:"auth"`
	}
	if err := json.Unmarshal(body, &out); err != nil || out.Auth.ClientToken == "" {
		return "", fmt.Errorf("vault login returned no client token")
	}
	return out.Auth.ClientToken, nil
}

// health performs an unauthenticated reachability check (sys/health). A transport or
// TLS error means Vault is unreachable or its serving cert is not trusted; a non-200
// means Vault is reachable but not ready (sealed/uninitialized). No auth is needed, so
// this is usable pre-deploy, before the gateway ServiceAccount exists.
func (c *vaultClient) health(ctx context.Context) error {
	url := fmt.Sprintf("%s/v1/sys/health?standbyok=true&perfstandbyok=true", c.address)
	code, _, err := c.do(ctx, http.MethodGet, url, "", nil)
	if err != nil {
		return fmt.Errorf("vault unreachable at %s: %w", c.address, err)
	}
	if code != http.StatusOK {
		return fmt.Errorf("vault at %s not ready (sys/health HTTP %d)", c.address, code)
	}
	return nil
}

// enableKVv2Mount enables a KV version 2 secrets engine at mountPath, using the given
// (provisioner) token. Idempotent: an already-mounted path (Vault "path is already in
// use") is accepted ONLY after verifying the existing mount is actually KV v2, so a
// re-reconcile is a no-op but a path occupied by a different engine (KV v1, a leftover,
// or a manual setup) fails closed here instead of letting the gateway break at runtime
// when it writes KV v2 data paths. The token's policy is expected to allow only
// sys/mounts/<prefix>/*, so this cannot touch other mounts.
func (c *vaultClient) enableKVv2Mount(ctx context.Context, token, mountPath string) error {
	payload, _ := json.Marshal(map[string]interface{}{
		"type":    "kv",
		"options": map[string]string{"version": "2"},
	})
	url := fmt.Sprintf("%s/v1/sys/mounts/%s", c.address, strings.Trim(mountPath, "/"))
	code, body, err := c.do(ctx, http.MethodPost, url, token, payload)
	if err != nil {
		return fmt.Errorf("enable vault mount %s: %w", mountPath, err)
	}
	switch {
	case code >= 200 && code < 300:
		return nil
	case code == http.StatusBadRequest && bytes.Contains(body, []byte("already in use")):
		// Idempotent only if the existing engine is KV v2; otherwise fail closed.
		return c.verifyKVv2Mount(ctx, token, mountPath)
	default:
		return fmt.Errorf("enable vault mount %s: HTTP %d", mountPath, code)
	}
}

// verifyKVv2Mount confirms an existing mount at mountPath is a KV version 2 secrets
// engine. It is called when enableKVv2Mount finds the path already in use, so a path
// occupied by KV v1 or a different engine is rejected rather than silently accepted.
func (c *vaultClient) verifyKVv2Mount(ctx context.Context, token, mountPath string) error {
	url := fmt.Sprintf("%s/v1/sys/mounts/%s", c.address, strings.Trim(mountPath, "/"))
	code, body, err := c.do(ctx, http.MethodGet, url, token, nil)
	if err != nil {
		return fmt.Errorf("verify vault mount %s: %w", mountPath, err)
	}
	if code < 200 || code >= 300 {
		return fmt.Errorf("verify vault mount %s: HTTP %d", mountPath, code)
	}
	// GET sys/mounts/<path> returns the mount's config either at the top level (newer
	// Vault wraps it under "data"); accept either shape.
	var out struct {
		Type    string `json:"type"`
		Options struct {
			Version string `json:"version"`
		} `json:"options"`
		Data struct {
			Type    string `json:"type"`
			Options struct {
				Version string `json:"version"`
			} `json:"options"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return fmt.Errorf("verify vault mount %s: unparseable response: %w", mountPath, err)
	}
	engine, version := out.Type, out.Options.Version
	if engine == "" {
		engine, version = out.Data.Type, out.Data.Options.Version
	}
	if engine != "kv" || version != "2" {
		return fmt.Errorf("vault mount %s already exists but is not KV v2 (type=%q version=%q); refusing to reuse", mountPath, engine, version)
	}
	return nil
}

// revokeSelf revokes the given Vault token via auth/token/revoke-self so a short-lived
// provisioner/probe token does not linger as a live lease until its TTL ends. Best-effort:
// a revoke failure is returned for the caller to log but is not fatal to the operation the
// token already completed. The default policy grants every token revoke-self.
func (c *vaultClient) revokeSelf(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	url := fmt.Sprintf("%s/v1/auth/token/revoke-self", c.address)
	code, _, err := c.do(ctx, http.MethodPost, url, token, nil)
	if err != nil {
		return fmt.Errorf("revoke vault token: %w", err)
	}
	if code < 200 || code >= 300 {
		return fmt.Errorf("revoke vault token: HTTP %d", code)
	}
	return nil
}

// disableMount disables (deletes) the secrets engine at mountPath, using the given
// (provisioner) token. Deleting a KV mount removes every secret it held in one
// operation. Idempotent: Vault returns 204 whether or not the mount existed, and a 404
// is also treated as already-gone.
func (c *vaultClient) disableMount(ctx context.Context, token, mountPath string) error {
	url := fmt.Sprintf("%s/v1/sys/mounts/%s", c.address, strings.Trim(mountPath, "/"))
	code, _, err := c.do(ctx, http.MethodDelete, url, token, nil)
	if err != nil {
		return fmt.Errorf("disable vault mount %s: %w", mountPath, err)
	}
	if (code >= 200 && code < 300) || code == http.StatusNotFound {
		return nil
	}
	return fmt.Errorf("disable vault mount %s: HTTP %d", mountPath, code)
}

// do issues an HTTP request and returns the status code and a size-capped body. The
// Vault token header is set only when token is non-empty. Response bodies never carry
// a provider secret value for these operations (login auth, health).
func (c *vaultClient) do(ctx context.Context, method, url, token string, payload []byte) (int, []byte, error) {
	var rdr io.Reader
	if payload != nil {
		rdr = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, rdr)
	if err != nil {
		return 0, nil, err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("X-Vault-Token", token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return resp.StatusCode, nil, fmt.Errorf("read vault response body: %w", err)
	}
	return resp.StatusCode, body, nil
}

// mintGatewayVaultToken mints a short-lived ServiceAccount token for the gateway's SA,
// scoped to the Vault audience, via the Kubernetes TokenRequest API. This is the same
// identity the gateway pod presents to Vault, so the control plane probes as the
// tenant, never as an administrator.
func mintGatewayVaultToken(ctx context.Context, clientset kubernetes.Interface, namespace, saName, audience string, ttl time.Duration) (string, error) {
	seconds := int64(ttl.Seconds())
	if seconds < 600 {
		seconds = 600 // Kubernetes rejects a projected-token TTL under 10 minutes
	}
	tr := &authnv1.TokenRequest{
		Spec: authnv1.TokenRequestSpec{
			Audiences:         []string{audience},
			ExpirationSeconds: &seconds,
		},
	}
	out, err := clientset.CoreV1().ServiceAccounts(namespace).CreateToken(ctx, saName, tr, metav1.CreateOptions{})
	if err != nil {
		return "", fmt.Errorf("mint vault token for %s/%s: %w", namespace, saName, err)
	}
	return out.Status.Token, nil
}
