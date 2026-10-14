package gateway

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/client-go/kubernetes"
)

// PurgeVaultCredentials removes every provider secret a gateway stored under its Vault
// KV path (secret/.../openshell/<namespace>/*), acting as the gateway's own identity
// (the templated policy scopes it to its own namespace). It is called on gateway
// deletion, before the gateway ServiceAccount is removed by teardown. Best-effort: a
// failure is reported to the caller, which records a durable VaultCredentialPath orphan.
func PurgeVaultCredentials(ctx context.Context, clientset kubernetes.Interface, namespace, credentialDriverJSON string) error {
	v := resolvedVault(ParseCredentialDriver(credentialDriverJSON))
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
		return fmt.Errorf("vault login for purge: %w", err)
	}
	return purgeKVRecursive(ctx, c, token, v.Mount, "openshell/"+namespace)
}

// purgeKVRecursive deletes every KV v2 secret under a metadata path, descending into
// nested folders (Vault LIST returns folder keys with a trailing slash).
func purgeKVRecursive(ctx context.Context, c *vaultClient, token, mount, path string) error {
	keys, err := c.listKeys(ctx, token, mount, path)
	if err != nil {
		return err
	}
	for _, k := range keys {
		child := strings.TrimRight(path, "/") + "/" + strings.TrimSuffix(k, "/")
		if strings.HasSuffix(k, "/") {
			if err := purgeKVRecursive(ctx, c, token, mount, child); err != nil {
				return err
			}
			continue
		}
		if err := c.deleteMetadata(ctx, token, mount, child); err != nil {
			return err
		}
	}
	return nil
}
