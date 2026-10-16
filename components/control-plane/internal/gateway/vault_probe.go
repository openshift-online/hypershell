package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/openshift-online/hypershell/components/control-plane/internal/helm"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// gatewayVaultSAName is the ServiceAccount every gateway runs as and that the Vault
// kubernetes-auth role (openshell-gateway) is bound to. The control plane mints this
// SA's token to probe/purge as the gateway's own tenant identity.
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

// resolvedVault returns the resolved Vault coordinates for a vault-resolved driver.
func resolvedVault(driver *CredentialDriverConfig) *helm.VaultValues {
	return resolveVaultValues(effectiveCredentialDriver(driver).Vault)
}

// probeVaultReachable is the pre-deploy, UNAUTHENTICATED reachability check: the
// per-cluster Vault is up and serving TLS trusted by its serving CA. It needs no
// ServiceAccount, so it runs during EnvironmentReady before the workload is deployed.
func probeVaultReachable(ctx context.Context, clientset kubernetes.Interface, driver *CredentialDriverConfig) error {
	v := resolvedVault(driver)
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

// ParseCredentialDriver parses a gateway's stored credential_driver JSON (which may be
// empty) into a config. An empty or unparseable value returns nil, which
// effectiveCredentialDriver treats as the default (vault).
func ParseCredentialDriver(s string) *CredentialDriverConfig {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	var c CredentialDriverConfig
	if err := json.Unmarshal([]byte(s), &c); err != nil {
		return nil
	}
	return &c
}

// IsVaultBacked reports whether a gateway with this stored credential_driver JSON
// resolves to the vault driver (including the flat default for an empty/absent value).
func IsVaultBacked(credentialDriverJSON string) bool {
	return isVaultDriver(effectiveCredentialDriver(ParseCredentialDriver(credentialDriverJSON)))
}

// ProbeVaultReachable (exported) runs the unauthenticated reachability check for a
// gateway described by its stored credential_driver JSON. Used by the health loop to
// degrade a Running vault-backed gateway when its Vault goes away.
func ProbeVaultReachable(ctx context.Context, clientset kubernetes.Interface, credentialDriverJSON string) error {
	return probeVaultReachable(ctx, clientset, ParseCredentialDriver(credentialDriverJSON))
}

// probeVaultAuth is the post-deploy check that the gateway's own identity can
// authenticate to Vault (kubernetes auth, role openshell-gateway). The gateway
// ServiceAccount must already exist (created by the Helm deploy).
func probeVaultAuth(ctx context.Context, clientset kubernetes.Interface, namespace string, driver *CredentialDriverConfig) error {
	v := resolvedVault(driver)
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
	if _, err := c.login(ctx, v.KubernetesAuthMount, v.Role, jwt); err != nil {
		return err
	}
	return nil
}
