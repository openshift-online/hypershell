package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// gatewayVaultSAName is the ServiceAccount every gateway runs as and that the Vault
// kubernetes-auth role (openshell-gateway) is bound to. The control plane mints this
// SA's token to probe as the gateway's own tenant identity.
const gatewayVaultSAName = "openshell-gateway"

const vaultProbeTimeout = 10 * time.Second

// readVaultCASource returns the per-cluster Vault serving CA (PEM) from its source
// Secret (VaultCASource), with the namespace/name for messages.
func readVaultCASource(ctx context.Context, clientset kubernetes.Interface) (pem []byte, namespace, name string, err error) {
	namespace, name = VaultCASource()
	secret, err := clientset.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, namespace, name, fmt.Errorf("read vault CA source %s/%s: %w", namespace, name, err)
	}
	pem = secret.Data[VaultCAKey]
	if len(pem) == 0 {
		return nil, namespace, name, fmt.Errorf("vault CA source %s/%s has no %q key", namespace, name, VaultCAKey)
	}
	return pem, namespace, name, nil
}

// probeVaultReachable is the pre-deploy, UNAUTHENTICATED reachability check: the
// per-cluster Vault is up and serving TLS trusted by its serving CA. It needs no
// ServiceAccount, so it runs during EnvironmentReady before the workload is deployed.
// Vault coordinates come from conventions (a black box; no per-gateway overrides), so
// the probe needs no credential_driver input.
func probeVaultReachable(ctx context.Context, clientset kubernetes.Interface) error {
	v := resolveVaultValues("")
	caPEM, _, _, err := readVaultCASource(ctx, clientset)
	if err != nil {
		return err
	}
	c, err := newVaultClient(v.Address, caPEM, vaultProbeTimeout)
	if err != nil {
		return err
	}
	return c.health(ctx)
}

// ParseCredentialDriver parses a gateway's stored credential_driver JSON into a config.
// An empty value returns (nil, nil), which effectiveCredentialDriver treats as the
// default (vault). A non-empty but malformed value returns a non-nil error instead of
// silently falling back to the default, so a caller can log or skip rather than Vault-probe
// (and mount-delete) a gateway whose stored driver it could not understand.
func ParseCredentialDriver(s string) (*CredentialDriverConfig, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	var c CredentialDriverConfig
	if err := json.Unmarshal([]byte(s), &c); err != nil {
		return nil, fmt.Errorf("parse credential_driver: %w", err)
	}
	return &c, nil
}

// IsVaultBacked reports whether a gateway with this stored credential_driver JSON
// resolves to the vault driver (including the flat default for an empty/absent value). It
// returns an error when the stored JSON is present but malformed, so callers do not treat
// an unparseable driver as the Vault default.
func IsVaultBacked(credentialDriverJSON string) (bool, error) {
	cfg, err := ParseCredentialDriver(credentialDriverJSON)
	if err != nil {
		return false, err
	}
	return isVaultDriver(effectiveCredentialDriver(cfg)), nil
}

// ProbeVaultReachable (exported) runs the unauthenticated reachability check against the
// per-cluster provider Vault. The health loop uses it to degrade a Running vault-backed
// gateway when its Vault goes away; the caller gates on IsVaultBacked first.
func ProbeVaultReachable(ctx context.Context, clientset kubernetes.Interface) error {
	return probeVaultReachable(ctx, clientset)
}

// probeVaultAuth is the post-deploy check that the gateway's own identity can
// authenticate to Vault (kubernetes auth, role openshell-gateway). The gateway
// ServiceAccount must already exist (created by the Helm deploy).
func probeVaultAuth(ctx context.Context, clientset kubernetes.Interface, namespace string) error {
	v := resolveVaultValues("")
	caPEM, _, _, err := readVaultCASource(ctx, clientset)
	if err != nil {
		return err
	}
	jwt, err := mintGatewayVaultToken(ctx, clientset, namespace, gatewayVaultSAName, v.TokenAudience, vaultProbeTimeout)
	if err != nil {
		return err
	}
	c, err := newVaultClient(v.Address, caPEM, vaultProbeTimeout)
	if err != nil {
		return err
	}
	token, err := c.login(ctx, v.KubernetesAuthMount, v.Role, jwt)
	if err != nil {
		return err
	}
	// Revoke the probe token immediately: it proved the gateway identity can authenticate,
	// and leaving it live would accumulate leases on every reconcile. Best-effort.
	revokeVaultToken(ctx, c, token)
	return nil
}
