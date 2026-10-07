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

Gateways with OIDC enabled need to reach the identity provider's OIDC discovery endpoint over HTTPS. In environments where the IdP uses a non-public CA certificate (e.g., OpenShift CRC, a Kind self-signed ingress CA, or a private PKI), the gateway's default trust store will not include the required CA. The platform SHALL supply the additional CA through the `gateway-trusted-ca` ConfigMap, which the upstream Helm chart injects into the gateway pod as the process-wide `SSL_CERT_FILE` (the control plane sets `server.oidc.caConfigMapName` to the copied ConfigMap; see [`openshell-gateway-helm-adoption.spec.md`](./openshell-gateway-helm-adoption.spec.md)).

**Trust store additivity.** The gateway's effective TLS trust store SHALL include BOTH the gateway image's complete default/system CA set AND every custom issuer CA, so the gateway can validate TLS for the private IdP endpoint and for publicly-trusted endpoints at the same time.

The gateway's TLS stack (Rust `rustls`/`reqwest`) treats `SSL_CERT_FILE` as a COMPLETE REPLACEMENT of the trust store, not an addition. Therefore the bundle that `SSL_CERT_FILE` points at SHALL already be the concatenation of the image's default system CA bundle and the custom issuer CA(s). A bundle that contains only the custom issuer CA is a defect: it silently removes trust for every publicly-trusted endpoint the gateway must also reach, including cloud inference provider token and API endpoints (e.g. `https://oauth2.googleapis.com/token`, Anthropic, AWS Bedrock/STS). See [`openshell-inference-routing.spec.md`](./openshell-inference-routing.spec.md) for the outbound provider path this protects.

Whichever component produces the `gateway-trusted-ca` content (the Kind `make kind-up` flow, or a production/GitOps source) SHALL publish the merged bundle; the control plane SHALL preserve the merged content when it copies the ConfigMap into the tenant namespace. The merge SHALL be idempotent so repeated runs do not duplicate or drop certificates.

#### Scenario: Trusted CA ConfigMap present

- GIVEN a ConfigMap named `gateway-trusted-ca` exists in the HyperShell namespace
- AND its CA material (key `ca-bundle.crt`) is the image's default system CA bundle concatenated with the custom issuer CA(s)
- WHEN the GatewayReconciler reconciles a gateway in a tenant namespace
- THEN it SHALL copy the ConfigMap to the tenant namespace (create-or-update), normalizing the key to `ca.crt` without altering the merged certificate content
- AND it SHALL set `server.oidc.caConfigMapName` so the chart mounts the ConfigMap at `/etc/openshell-tls/oidc-ca/` and sets `SSL_CERT_FILE=/etc/openshell-tls/oidc-ca/ca.crt` on the gateway container
- AND the gateway SHALL validate TLS for both the private OIDC issuer and publicly-trusted outbound endpoints using that single merged bundle

#### Scenario: Gateway reaches a public provider endpoint with a custom OIDC CA in use

- GIVEN a gateway whose `SSL_CERT_FILE` is driven by the `gateway-trusted-ca` ConfigMap (custom self-signed OIDC issuer CA)
- WHEN the gateway makes an outbound HTTPS call to a publicly-trusted provider endpoint (e.g. `https://oauth2.googleapis.com/token`)
- THEN the TLS handshake SHALL succeed using the system CA portion of the merged bundle
- AND `openshell provider create --type google-vertex-ai --from-gcloud-adc ...` SHALL complete end to end

#### Scenario: Trusted CA bundle containing only the custom CA is rejected as a defect

- GIVEN a candidate `gateway-trusted-ca` ConfigMap whose content is only the custom issuer CA (the system CA bundle is absent)
- WHEN the trusted-CA content is produced or verified
- THEN this SHALL be treated as a defect, because `SSL_CERT_FILE` would replace the system trust store and break all outbound HTTPS to publicly-trusted endpoints

#### Scenario: Trusted CA ConfigMap absent

- GIVEN no ConfigMap named `gateway-trusted-ca` exists in the HyperShell namespace
- WHEN the GatewayReconciler reconciles a gateway
- THEN it SHALL NOT set `server.oidc.caConfigMapName` and the chart SHALL NOT add any CA volume or `SSL_CERT_FILE` env var
- AND the gateway SHALL use its built-in (image default) trust store

#### Scenario: Regression guard for additive trust

- GIVEN the trusted-CA injection mechanism
- WHEN the test suite runs
- THEN a regression test (chart unit test and/or Kind e2e) SHALL assert that the bundle backing `SSL_CERT_FILE` contains the system CA bundle in addition to the custom issuer CA
- AND the e2e variant SHALL create a real-provider-type `openshell provider` and assert the gateway reaches the provider's real token/API endpoint with TLS verification intact (not merely that credentials were accepted locally)

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

### Requirement: Client Certificate Verification Conditional on Routing

The gateway config template includes a `client_ca_path` setting under `[openshell.gateway.tls]` that enables the server to require client certificates (mTLS). This setting SHALL be conditionally applied based on whether the gateway is exposed via Gateway API routing.

#### Scenario: Gateway with routing enabled

- GIVEN a Gateway with `route.enabled = true`
- WHEN the GatewayReconciler applies config overrides
- THEN it SHALL remove the `client_ca_path` setting from `gateway.toml`
- AND it SHALL remove the `tls-client-ca` volume and volume mount from the Deployment
- BECAUSE the Gateway API ingress proxy (Envoy) terminates external TLS and re-connects to the backend via BackendTLSPolicy, which only handles server certificate validation -- the proxy cannot present a client certificate, so requiring one causes "peer sent no certificates" connection resets

#### Scenario: Gateway without routing (direct access)

- GIVEN a Gateway with no `route` configuration or `route.enabled = false`
- WHEN the GatewayReconciler applies config overrides
- THEN it SHALL preserve the `client_ca_path` setting and `tls-client-ca` volume mount
- AND the gateway SHALL require client certificates for all incoming connections (used by supervisors connecting directly)

---

## Debugging Reference

| Symptom | Root Cause | Fix |
|---|---|---|
| `DecryptError` in gateway logs | Stale client cert from sandbox after cert rotation | Recreate affected sandboxes |
| Cert deletion loop (every 30s) | ConfigMap SANs don't match API SANs | Ensure exact match; manual certgen if needed |
| `invalid peer certificate: UnknownIssuer` | Self-signed CA - CLI doesn't trust gateway CA | Use `OPENSHELL_GATEWAY_INSECURE=true` or trust CA |
| OIDC discovery fails with TLS error | Gateway can't reach IdP (private CA) | Create `gateway-trusted-ca` ConfigMap (merged system CA + issuer CA) |
| Outbound HTTPS to a real provider fails (`unable to get local issuer certificate` for e.g. `https://oauth2.googleapis.com/token`) while OIDC still works | `SSL_CERT_FILE` bundle contains only the custom issuer CA, replacing the system trust store | Publish `gateway-trusted-ca` as the system CA bundle merged with the issuer CA (see Trust Store Additivity) |
| `peer sent no certificates` + `upstream connect error or disconnect/reset before headers` | Gateway requires client certs (mTLS) but ingress proxy cannot present one | Ensure `route.enabled` is true so `client_ca_path` is stripped from config |

---

## References

- [NVIDIA OpenShell Managing Certificates](https://docs.nvidia.com/openshell/kubernetes/managing-certificates)
- [cert-manager Installation](https://cert-manager.io/docs/installation/)
