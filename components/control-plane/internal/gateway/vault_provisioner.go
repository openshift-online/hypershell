package gateway

import (
	"context"
	"fmt"
	"time"

	"k8s.io/client-go/kubernetes"
)

// vaultProvisionTimeout bounds each provisioner Vault call (login + one mount op).
const vaultProvisionTimeout = 15 * time.Second

// vaultProvisionerClient logs in to the per-cluster Vault as the control plane's own
// scoped provisioner identity and returns the client and its token. The control plane
// mints a vault-audience token for its OWN ServiceAccount (not a gateway's) via the
// Kubernetes TokenRequest API and exchanges it for the VaultProvisionerRole, whose policy
// grants only sys/mounts/openshell-gw/* - no secret read, no policy or role write. So this
// identity can manage per-tenant mounts but cannot read a provider secret or rewrite the
// isolation boundary. See openshell-gateway-credentials.spec.md (Provider Credential
// Isolation).
func vaultProvisionerClient(ctx context.Context, clientset kubernetes.Interface, opts ReconcileOpts) (*vaultClient, string, error) {
	v := resolveVaultValues("") // address, auth mount and CA source (conventions); the mount path is computed per call
	caPEM, _, _, err := readVaultCASource(ctx, clientset)
	if err != nil {
		return nil, "", err
	}
	jwt, err := mintGatewayVaultToken(ctx, clientset, opts.ControlPlaneNamespace, VaultProvisionerServiceAccount(), v.TokenAudience, vaultProvisionTimeout)
	if err != nil {
		return nil, "", fmt.Errorf("mint provisioner vault token: %w", err)
	}
	c, err := newVaultClient(v.Address, caPEM, vaultProvisionTimeout)
	if err != nil {
		return nil, "", err
	}
	token, err := c.login(ctx, v.KubernetesAuthMount, VaultProvisionerRole, jwt)
	if err != nil {
		return nil, "", fmt.Errorf("vault provisioner login: %w", err)
	}
	return c, token, nil
}

// EnsureVaultTenantMount creates the gateway's own per-tenant KV v2 mount
// (openshell-gw/<namespace>) if it does not already exist, so the gateway runtime has a
// mount to write its provider credentials to. Idempotent. Called before the workload is
// deployed (HYPERSHELL-369).
func EnsureVaultTenantMount(ctx context.Context, clientset kubernetes.Interface, opts ReconcileOpts, namespace string) error {
	c, token, err := vaultProvisionerClient(ctx, clientset, opts)
	if err != nil {
		return err
	}
	return c.enableKVv2Mount(ctx, token, VaultMountForNamespace(namespace))
}

// DeleteVaultTenantMount deletes the gateway's per-tenant KV mount on teardown, removing
// every provider-credential entry it held in one operation. Idempotent: an absent mount is
// treated as already-gone. Best-effort at the call site - the caller records a durable
// orphan and retries on failure (HYPERSHELL-369).
func DeleteVaultTenantMount(ctx context.Context, clientset kubernetes.Interface, opts ReconcileOpts, namespace string) error {
	c, token, err := vaultProvisionerClient(ctx, clientset, opts)
	if err != nil {
		return err
	}
	return c.disableMount(ctx, token, VaultMountForNamespace(namespace))
}
