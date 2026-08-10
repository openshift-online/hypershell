# OpenShell Gateway TLS Specification

**Date:** 2026-08-05
**Status:** Draft
**Parent:** `openshell-gateway.spec.md` - core gateway provisioning
**Related:** `openshell-gateway-oidc.spec.md` - OIDC authentication; `openshell-gateway-routing.spec.md` - external connectivity

---

## Purpose

This specification defines TLS certificate management for OpenShell gateways deployed by the HyperShell control plane. It covers certificate generation via cert-manager, SAN management, cert rotation, and trusted CA bundle injection. HyperShell gateways use OIDC for authentication; mTLS is not supported.

---

## Requirements

### Requirement: TLS Certificate Management via cert-manager

The GatewayReconciler SHALL use cert-manager for all TLS certificate lifecycle management. cert-manager is the sole certificate generation strategy.

#### Scenario: cert-manager installed

- GIVEN cert-manager is installed on the cluster (Certificate, Issuer CRDs are available)
- AND the GatewayReconciler detects cert-manager availability (via API discovery for `cert-manager.io` API group)
- WHEN the reconciler provisions a gateway
- THEN it SHALL create cert-manager resources for TLS certificate lifecycle:
  - A self-signed `Issuer` (`openshell-selfsigned`) in the project namespace
  - A `Certificate` for the CA (`openshell-ca`, ECDSA P256, creates `openshell-ca-tls` Secret)
  - A CA-backed `Issuer` (`openshell-ca-issuer`) that uses the CA certificate
  - A server `Certificate` (`openshell-server`, creates `openshell-server-tls` Secret with SANs from `serverDnsNames`)
  - A client `Certificate` (`openshell-client`, creates `openshell-client-tls` Secret)

#### Scenario: cert-manager not installed

- GIVEN cert-manager is NOT installed on the cluster
- WHEN the GatewayReconciler provisions a gateway
- THEN it SHALL log an error indicating cert-manager is a required cluster prerequisite
- AND it SHALL NOT deploy the gateway until cert-manager is available

**Cluster prerequisite:** cert-manager (v1.20+ recommended) must be installed cluster-wide by an administrator before gateways can use it.

#### Coexistence with certgen job

cert-manager handles TLS certificate lifecycle. The certgen job handles JWT key generation (`signing.pem`, `public.pem`, `kid` in `openshell-gateway-jwt-keys`). Both run: cert-manager creates TLS secrets, then certgen checks if they exist (skipping TLS) and only creates JWT keys.

---

### Requirement: SAN Management and Cert Rotation

The reconciler monitors `serverDnsNames` on the Gateway API resource and compares them against the `server_sans` key in the `openshell-gateway-config` ConfigMap. When they differ, the reconciler triggers certificate regeneration.

#### Scenario: DNS names changed

- GIVEN a Gateway's `serverDnsNames` differ from the ConfigMap's `server_sans`
- WHEN the GatewayReconciler reconciles
- THEN it SHALL update the cert-manager Certificate resources with the new SANs
- AND cert-manager SHALL regenerate the TLS secrets
- AND the gateway workload SHALL restart to pick up the new certificates

#### Operational warning: Cert rotation is destructive

When SANs change, TLS secrets are regenerated. Connected sandbox supervisors will experience `DecryptError` because their client certificates were signed by the old CA. Sandboxes must be recreated after cert rotation.

---

### Requirement: Trusted CA Bundle Injection

Gateways with OIDC enabled need to reach the identity provider's OIDC discovery endpoint over HTTPS. In environments where the IdP uses a non-public CA certificate (e.g., OpenShift CRC, private PKI), the gateway's default trust store will not include the required CA.

#### Scenario: Trusted CA ConfigMap present

- GIVEN a ConfigMap named `gateway-trusted-ca` exists in the HyperShell namespace
- AND the ConfigMap has a `ca-bundle.crt` key containing PEM-encoded CA certificates
- WHEN the GatewayReconciler reconciles a gateway in a tenant namespace
- THEN it SHALL copy the ConfigMap to the tenant namespace (create-or-update)
- AND it SHALL mount `ca-bundle.crt` at `/etc/pki/tls/certs/ca-bundle.crt` (read-only, subPath)
- AND it SHALL add `SSL_CERT_FILE=/etc/pki/tls/certs/ca-bundle.crt` to the gateway container

#### Scenario: Trusted CA ConfigMap absent

- GIVEN no ConfigMap named `gateway-trusted-ca` exists in the HyperShell namespace
- WHEN the GatewayReconciler reconciles a gateway
- THEN it SHALL NOT add any CA volume or `SSL_CERT_FILE` env var
- AND the gateway SHALL use its built-in trust store

---

### Requirement: RBAC for TLS Resources

The control plane ClusterRole SHALL include permissions for TLS-related resources:

```yaml
- apiGroups: [""]
  resources: ["secrets", "configmaps"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]

- apiGroups: ["cert-manager.io"]
  resources: ["issuers", "certificates"]
  verbs: ["get", "list", "create", "update", "patch", "delete"]
```

---

## Debugging Reference

| Symptom | Root Cause | Fix |
|---|---|---|
| `DecryptError` in gateway logs | Stale client cert from sandbox after cert rotation | Recreate affected sandboxes |
| Cert deletion loop (every 30s) | ConfigMap SANs don't match API SANs | Ensure exact match; manual certgen if needed |
| `invalid peer certificate: UnknownIssuer` | Self-signed CA - CLI doesn't trust gateway CA | Use `OPENSHELL_GATEWAY_INSECURE=true` or trust CA |
| OIDC discovery fails with TLS error | Gateway can't reach IdP (private CA) | Create `gateway-trusted-ca` ConfigMap |

---

## References

- [NVIDIA OpenShell Managing Certificates](https://docs.nvidia.com/openshell/kubernetes/managing-certificates)
- [cert-manager Installation](https://cert-manager.io/docs/installation/)
