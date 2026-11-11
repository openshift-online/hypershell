# OpenShell Gateway TLS Specification

**Date:** 2026-08-05
**Status:** Draft
**Parent:** `openshell-gateway.spec.md` - core gateway provisioning
**Related:** `openshell-gateway-oidc.spec.md` - OIDC authentication; `openshell-gateway-routing.spec.md` - external connectivity

---

## Purpose

This specification defines TLS certificate management for OpenShell gateways deployed by the HyperShell control plane. It covers certificate generation via cert-manager, SAN management, cert rotation, and trusted CA bundle injection. HyperShell gateways use OIDC for authentication. Client certificate verification (mTLS) is turned off only in Gateway API routing mode (OSG-TLS-05).

---

## Requirements

### Requirement: OSG-TLS-01 -- TLS Certificate Management via cert-manager

The GatewayReconciler SHALL use cert-manager for all TLS certificate lifecycle management. cert-manager is the sole certificate generation strategy.

cert-manager availability is a feature prerequisite determined through the shared,
refreshable dependency discovery ([`control-plane.spec.md`](./control-plane.spec.md),
CTRL-CORE-18), distinguishing present, confirmed absent, and unknown/unavailable. The
reconciler SHALL use this prerequisite to gate TLS-dependent work (CTRL-CORE-19)
while the controller continues startup and independent reconciliation.

#### Scenario: OSG-TLS-01.01 -- cert-manager present

- GIVEN discovery confirms cert-manager is present on the cluster (Certificate, Issuer CRDs are available in the `cert-manager.io` API group)
- WHEN the reconciler provisions a gateway
- THEN it SHALL create cert-manager resources for TLS certificate lifecycle:
  - A self-signed `Issuer` (`openshell-selfsigned`) in the project namespace
  - A `Certificate` for the CA (`openshell-ca`, ECDSA P256, creates `openshell-ca-tls` Secret)
  - A CA-backed `Issuer` (`openshell-ca-issuer`) that uses the CA certificate
  - A server `Certificate` (`openshell-server`, creates `openshell-server-tls` Secret with SANs from `serverDnsNames`)
  - A client `Certificate` (`openshell-client`, creates `openshell-client-tls` Secret)

#### Scenario: OSG-TLS-01.02 -- cert-manager confirmed absent

- GIVEN a successful discovery confirms cert-manager is NOT installed on the cluster
- WHEN the GatewayReconciler provisions a gateway
- THEN it SHALL report a prerequisite condition identifying cert-manager
- AND it SHALL keep independent reconciliation work running
- AND after cert-manager is installed, the running controller SHALL reconcile the gateway's required certificates and workload before completing provisioning

#### Scenario: OSG-TLS-01.03 -- cert-manager unavailable

- GIVEN cert-manager discovery fails with a transport, authorization, or stale-group error
- WHEN the GatewayReconciler provisions a gateway that requires cert-manager
- THEN it SHALL mark the affected gateway as waiting/retryable and retry with bounded backoff
- AND the controller SHALL continue reporting process health and reconciling independent work
- AND gateway provisioning SHALL complete after its required certificates and workload are reconciled

**Cluster prerequisite:** cert-manager (v1.20+ recommended) must be installed cluster-wide by an administrator before gateways can use it.

#### Coexistence with certgen job

cert-manager handles TLS certificate lifecycle. The certgen job handles JWT key generation (`signing.pem`, `public.pem`, `kid` in `openshell-gateway-jwt-keys`). Both run: cert-manager creates TLS secrets, then certgen checks if they exist (skipping TLS) and only creates JWT keys.

---

### Requirement: OSG-TLS-02 -- SAN Management and Cert Rotation

The reconciler passes the Gateway's `serverDnsNames` to the gateway Helm release as the `certManager.serverDnsNames` value. The chart uses this value for both the `server_sans` setting in `gateway.toml` and the SANs of the internal server `Certificate`. A change to `serverDnsNames` bumps the Gateway generation, and the reconciler runs a Helm upgrade with the new values.

#### Scenario: OSG-TLS-02.01 -- DNS names changed

- GIVEN a Gateway whose `serverDnsNames` changed since the last applied generation
- WHEN the GatewayReconciler reconciles
- THEN it SHALL upgrade the gateway Helm release with the new SANs in `certManager.serverDnsNames`
- AND the chart SHALL update the cert-manager server Certificate with the new SANs
- AND cert-manager SHALL regenerate the TLS secrets
- AND the gateway workload SHALL restart to pick up the new certificates

#### Operational warning: Cert rotation is destructive

When SANs change, TLS secrets are regenerated. Connected sandbox supervisors will experience `DecryptError` because their client certificates were signed by the old CA. Sandboxes must be recreated after cert rotation.

---

### Requirement: OSG-TLS-06 -- Per-Gateway External Certificate

A Gateway that opts in to a dedicated route via `route.certificate` (see [`openshell-gateway-routing.spec.md`](./openshell-gateway-routing.spec.md) § Per-Gateway Dedicated Route and Certificate) gets a single external cert-manager `Certificate` (`openshell-gateway-external`) in the tenant namespace, with `secretName: openshell-gateway-external-tls` and both the exact name `gw-<ns>.<base-domain>` and the wildcard `*.gw-<ns>.<base-domain>` as dnsNames.

One certificate, one Secret, one ACME order. A certificate may carry an exact name and a wildcard in the same SAN list; because both names are in the same DNS zone, one DNS-01 solver covers both. The wildcard covers exactly one label below `gw-<ns>.<base-domain>` - exposed-service hosts (`<sandbox>--<service>`) are a single DNS label, so every sandbox service URL is covered. The exact SAN covers the Gateway's own hostname, which the wildcard does not.

The issuer is cluster-configured (`GATEWAY_EXTERNAL_ISSUER` / `GATEWAY_EXTERNAL_ISSUER_KIND`); the API carries no issuerRef. Wildcard SANs REQUIRE a DNS-01 issuer (e.g. `letsencrypt-dns01`); a cluster configured with an HTTP-01-only issuer cannot serve opted-in Gateways.

#### Scenario: OSG-TLS-06.01 -- Single certificate with exact and wildcard SANs

- GIVEN a Gateway created with `route: { certificate: {} }`
- AND `GATEWAY_EXTERNAL_ISSUER` names a DNS-01-capable ClusterIssuer
- WHEN the control plane reconciles the Gateway's external certificate
- THEN it SHALL create exactly one `Certificate` `openshell-gateway-external` in the tenant namespace
- AND its dnsNames SHALL be `gw-<ns>.<base-domain>` and `*.gw-<ns>.<base-domain>`
- AND the resulting Secret `openshell-gateway-external-tls` SHALL serve both the Gateway's own hostname and every `<sandbox>--<service>.gw-<ns>.<base-domain>` host

#### Scenario: OSG-TLS-06.02 -- Rotation reaches the serving path without pod involvement

- GIVEN an opted-in Gateway whose external certificate is served by the per-tenant Gateway's listeners
- WHEN cert-manager renews the `Certificate` (`rotationPolicy: Always`)
- THEN the renewed Secret SHALL be picked up by the Gateway API implementation's listener hot-reload
- AND the gateway pod SHALL NOT restart, and no reloader mechanism SHALL be required
- AND the pod's internal certificate renewal (SAN Management and Cert Rotation) SHALL remain unchanged and orthogonal

#### Scenario: OSG-TLS-06.03 -- server_sans injection for an opted-in gateway

- GIVEN a Gateway created with `route: { certificate: {} }`
- WHEN the GatewayReconciler renders the gateway configuration
- THEN `server_sans` SHALL contain both `gw-<ns>.<base-domain>` and `*.gw-<ns>.<base-domain>`
- AND the gateway SHALL strip the `*.` from the wildcard entry and derive the base domain `gw-<ns>.<base-domain>` for exposed-service URLs, making them unambiguous (unique per Gateway), certificate-valid (wildcard SAN), and routable (wildcard listener, HTTPRoute, and DNS)

---

### Requirement: OSG-TLS-03 -- Trusted CA Bundle Injection

Every gateway uses OIDC, so it needs to reach the identity provider's OIDC discovery endpoint over HTTPS. In environments where the IdP uses a non-public CA certificate (e.g., OpenShift CRC, private PKI), the gateway's default trust store will not include the required CA.

#### Scenario: OSG-TLS-03.01 -- Trusted CA ConfigMap present

- GIVEN a ConfigMap named `gateway-trusted-ca` exists in the HyperShell namespace
- AND the ConfigMap has a `ca-bundle.crt` key containing PEM-encoded CA certificates
- WHEN the GatewayReconciler reconciles a gateway in a tenant namespace
- THEN it SHALL copy the ConfigMap to the tenant namespace (create-or-update)
- AND it SHALL mount `ca-bundle.crt` at `/etc/pki/tls/certs/ca-bundle.crt` (read-only, subPath)
- AND it SHALL add `SSL_CERT_FILE=/etc/pki/tls/certs/ca-bundle.crt` to the gateway container

#### Scenario: OSG-TLS-03.02 -- Trusted CA ConfigMap absent

- GIVEN no ConfigMap named `gateway-trusted-ca` exists in the HyperShell namespace
- WHEN the GatewayReconciler reconciles a gateway
- THEN it SHALL NOT add any CA volume or `SSL_CERT_FILE` env var
- AND the gateway SHALL use its built-in trust store

---

### Requirement: OSG-TLS-04 -- RBAC for TLS Resources

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

### Requirement: OSG-TLS-05 -- Client Certificate Verification Conditional on Routing

The OpenShell Helm chart value `server.tls.enableMtls` controls whether the gateway server requires client certificates (mTLS). The control plane SHALL set it based on how the gateway is exposed.

#### Scenario: OSG-TLS-05.01 -- Gateway API routing mode

- GIVEN a Gateway with `route.enabled = true`
- AND the control plane uses Gateway API routing
- WHEN the GatewayReconciler builds the Helm values
- THEN it SHALL set `server.tls.enableMtls` to `false`
- BECAUSE the Gateway API ingress proxy (Envoy) terminates external TLS and re-connects to the backend via BackendTLSPolicy, which only handles server certificate validation -- the proxy cannot present a client certificate, so requiring one causes "peer sent no certificates" connection resets

#### Scenario: OSG-TLS-05.02 -- Direct access without a route

- GIVEN a Gateway with no `route` configuration or `route.enabled = false`
- WHEN the GatewayReconciler builds the Helm values
- THEN it SHALL NOT set `server.tls.enableMtls`, so the chart default (mTLS on) applies
- AND the gateway SHALL require client certificates for incoming connections

---

### Requirement: OSG-TLS-07 -- Sandbox Destination CA Trust

Sandbox supervisors verify destination TLS independently of the workload. The
control plane SHALL support operator-provided public CA roots without disabling
upstream certificate verification. This trust SHALL remain separate from gateway
mTLS, OIDC issuer trust, and Vault serving-CA trust. It SHALL NOT authorize network
destinations or bypass private-address restrictions on advisor-proposed policies.

#### Scenario: OSG-TLS-07.01 -- Capability-selected corporate trust

- GIVEN `GATEWAY_ADDITIONAL_CA_CONFIGMAP_NAME` names a ConfigMap in the controller namespace
- AND `GATEWAY_ADDITIONAL_CA_REQUIRED_CAPABILITY` is `network/vpn`
- WHEN `HYPERSHELL_MANAGED_CLUSTER_CAPABILITIES` contains that exact comma-separated capability (ignoring surrounding whitespace)
- THEN the controller SHALL apply the public destination CA bundle to every gateway it manages
- AND instances without that capability SHALL NOT read or propagate the bundle
- AND an unset required capability SHALL allow an explicitly configured bundle on any instance
- AND an unset ConfigMap name SHALL leave destination trust unconfigured
- AND GitOps SHALL configure the corporate bundle for every instance declaring `network/vpn`, including future instances

#### Scenario: OSG-TLS-07.02 -- Bundle validation and propagation

- GIVEN an eligible instance with a configured source ConfigMap
- WHEN provisioning or reconciling a gateway
- THEN the controller SHALL read PEM certificates from `ca.crt`, accepting `ca-bundle.crt` as a fallback
- AND a missing, unreadable, or certificate-free source SHALL fail reconciliation with the source identified
- AND the controller SHALL copy public certificates into a content-addressed ConfigMap in the gateway namespace
- AND it SHALL set `supervisor.network.additionalCaConfigMapName` in the gateway Helm values
- AND the gateway SHALL stage this trust for its sandbox supervisors through OpenShell's additional-CA support
- AND no private key SHALL be required in the source bundle

#### Scenario: OSG-TLS-07.03 -- Existing gateways and rotation

- GIVEN an eligible instance whose source bundle or capability configuration has changed
- WHEN the controller starts and seeds existing gateways, or receives a gateway reconciliation event
- THEN it SHALL check destination trust even when the Gateway API generation has already converged
- AND a changed content-addressed volume reference SHALL cause a Helm upgrade and gateway rollout
- AND old ConfigMap generations SHALL remain mountable until gateway namespace cleanup
- AND running sandbox supervisors SHALL acquire the new trust on sandbox stop/start, preserving durable workspace data
- AND changing trust SHALL NOT require deleting sandboxes or regenerating gateway mTLS certificates

Source ConfigMap changes are not watched directly. Operators SHALL restart the
controller after a bundle update to ensure all existing gateways are rechecked.
Sandbox stop/start interrupts running processes and SHALL be coordinated with owners.

#### Scenario: OSG-TLS-07.04 -- Removing configured trust

- GIVEN the bundle setting is removed or its capability selector no longer matches
- WHEN a Gateway spec update advances its generation and triggers provisioning
- THEN the controller SHALL omit the additional-CA Helm value
- AND affected sandboxes SHALL require stop/start to discard their previous trust generation

Removing a setting alone SHALL NOT bypass the normal convergence gate on an
ineligible instance. Gateway spec reconciliation is required to remove previously
applied trust from already-converged gateways.

**Verification:** `internal/gateway/destination_ca_test.go` covers eligibility,
validation, propagation, and trust rotation; `internal/helm/values_test.go` renders
the vendored chart to verify the supervisor trust path.

---

## Debugging Reference

| Symptom | Root Cause | Fix |
|---|---|---|
| `DecryptError` in gateway logs | Stale client cert from sandbox after cert rotation | Recreate affected sandboxes |
| `invalid peer certificate: UnknownIssuer` | Self-signed CA - CLI doesn't trust gateway CA | Use `OPENSHELL_GATEWAY_INSECURE=true` or trust CA |
| OIDC discovery fails with TLS error | Gateway can't reach IdP (private CA) | Create `gateway-trusted-ca` ConfigMap |
| `peer sent no certificates` + `upstream connect error or disconnect/reset before headers` | Gateway requires client certs (mTLS) but ingress proxy cannot present one | Ensure `route.enabled` is true so the control plane sets `server.tls.enableMtls=false` in Gateway API routing mode |

---

## References

- [NVIDIA OpenShell Managing Certificates](https://docs.nvidia.com/openshell/kubernetes/managing-certificates)
- [cert-manager Installation](https://cert-manager.io/docs/installation/)
