# OpenShell Gateway Routing Specification

**Date:** 2026-08-05
**Status:** Draft
**Parent:** `openshell-gateway.spec.md` - core gateway provisioning
**Related:** `openshell-gateway-tls.spec.md` - TLS modes; `openshell-gateway-oidc.spec.md` - OIDC authentication

---

## Purpose

This specification defines how OpenShell gateways are exposed to external clients via the **Gateway API** (GRPCRoute + BackendTLSPolicy). The control plane auto-detects Gateway API availability at startup and provisions per-tenant GRPCRoutes that attach to a pre-existing shared Gateway. An administrator provisions the Gateway as cluster infrastructure (similar to a GatewayClass).

---

## Architecture

```
External Client (openshell CLI)
    │  TLS/HTTP2 (ALPN-negotiated)
    ▼
Shared Gateway (openshift-ingress namespace)
    │  Admin-provisioned with wildcard TLS cert (e.g. *.openshell.example.com)
    │  Terminates external TLS, negotiates HTTP/2 via ALPN
    │  GRPCRoute (in tenant namespace) matches on tenant-specific hostname
    │  BackendTLSPolicy: re-encrypts to pod, verifies cert via CA
    ▼
openshell-gateway Service (ClusterIP :8080, tenant namespace)
    ▼
openshell-gateway Pod (tenant namespace)
```

Requires:
- A pre-existing Gateway resource provisioned by an administrator (see `deploy/openshift/infrastructure/GATEWAY-SETUP.md`)
- `GATEWAY_API_GATEWAY_NAME` env var set on the controller
- Hostname: `gw-<tenant-namespace>.<base-domain>` (auto-derived from `GATEWAY_API_BASE_DOMAIN`)

---

## Requirements

### Requirement: OSG-ROUT-01 -- NetworkPolicy for Gateway API Proxy Ingress

**Superseded.** The control plane no longer creates NetworkPolicies for gateway
namespaces. On OVN-Kubernetes the default network posture is allow-all, so no
explicit policy is needed for Gateway API proxy ingress.

See [`openshell-gateway-helm-adoption.spec.md`](./openshell-gateway-helm-adoption.spec.md)
(NetworkPolicy Decision: Do Not Install) for rationale and empirical
verification.

---

### Requirement: OSG-ROUT-02 -- Gateway API Detection

The control plane SHALL detect at startup whether the cluster supports the Gateway API for route provisioning.

#### Scenario: OSG-ROUT-02.01 -- Gateway API available

- GIVEN the `gateway.networking.k8s.io` API group is available (Gateway, GRPCRoute, BackendTLSPolicy CRDs exist)
- AND `GATEWAY_API_GATEWAY_NAME` is set to the name of a pre-existing Gateway resource
- THEN the control plane SHALL enable Gateway API route provisioning

#### Scenario: OSG-ROUT-02.02 -- Gateway API not available

- GIVEN the `gateway.networking.k8s.io` API group is NOT available
- THEN the control plane SHALL disable route provisioning
- AND gateways with `route` configuration SHALL log a warning and skip route resource creation

---

### Requirement: OSG-ROUT-03 -- Gateway Route Configuration

The Gateway resource SHALL support an optional `route` field that declares external exposure via Gateway API resources.

#### Scenario: OSG-ROUT-03.01 -- Gateway with auto-assigned route host

- GIVEN a Gateway with `route: {}`
- THEN the control plane SHALL create a GRPCRoute in the tenant namespace with a cross-namespace parentRef to the pre-existing shared Gateway
- AND the hostname SHALL be `gw-<tenant-namespace>.<base-domain>`
- AND the derived hostname's first DNS label (e.g., `gw-openshell-a1b2c3d4e5f67890` at 29 chars) SHALL be well within the 63-character DNS label limit (RFC 1123), so no truncation is needed

#### Scenario: OSG-ROUT-03.02 -- Gateway with explicit route host

- GIVEN a Gateway with `route: { host: "custom.example.com" }`
- THEN the control plane SHALL use the provided hostname instead of auto-deriving it
- AND the hostname SHALL be validated as a valid RFC 1123 DNS name before use

#### Scenario: OSG-ROUT-03.03 -- Gateway without route configuration

- GIVEN a Gateway with no `route` field
- THEN no Gateway API resources (GRPCRoute, BackendTLSPolicy) SHALL be created
- AND the gateway SHALL be accessible only via cluster-internal DNS and `kubectl port-forward`

#### Scenario: OSG-ROUT-03.04 -- Route configuration removed from existing Gateway

- GIVEN a Gateway that previously had `route` configuration and associated route resources
- WHEN the `route` field is removed or set to null
- THEN the GatewayReconciler SHALL trigger removal of route-owned resources via Helm upgrade with routing disabled (or Helm uninstall if the gateway is being deleted). The chart manages: GRPCRoute, BackendTLSPolicy, `openshell-gateway-backend-ca` ConfigMap, and associated routing resources
- AND the reconciler SHALL clear the `routeAddress` field on the Gateway resource via the API server
- AND the gateway SHALL revert to cluster-internal-only access

---

### Requirement: OSG-ROUT-12 -- Per-Gateway Dedicated Route and Certificate

A Gateway MAY opt in to a dedicated route and external certificate at provision time by setting `route.certificate` to a bare object (`route: { certificate: {} }`). The opt-in is provision-time only: the API server SHALL reject any change to `route.certificate` after the Gateway exists (changing certificate mode requires deleting and re-provisioning the Gateway). `route.certificate` SHALL NOT be combined with `route.host`.

The opt-in exists to make OpenShell sandbox service exposure (ExposeService HTTP virtual-hosting) work: a shared spoke-level wildcard certificate covers only one DNS label, so `<sandbox>--<service>.gw-<ns>.<base-domain>` URLs are neither certificate-valid nor routable through the shared Gateway. An opted-in Gateway gets its own domain subtree, its own certificate, and its own load balancer.

The opt-in's shape is validated at the API boundary, not only during provisioning: a create or update whose route document combines `certificate` with `host`, or whose `certificate` value carries fields, SHALL be rejected with an immediate 400 naming the problem - an invalid shape must not be accepted with a 201 and discovered later as an asynchronously failing provisioning pass.

#### Scenario: Invalid opt-in shapes rejected at the API boundary

- GIVEN a Gateway create request with `route: {"host": "x.example.com", "certificate": {}}`
- WHEN the API server processes the request
- THEN it SHALL reject it with 400 and a reason naming that `route.host` is not supported with `route.certificate`
- GIVEN a Gateway create request with `route: {"certificate": {"issuer": "custom"}}`
- WHEN the API server processes the request
- THEN it SHALL reject it with 400 and a reason stating `route.certificate` accepts no fields (the issuer is cluster infrastructure)

For an opted-in Gateway the control plane SHALL create, in the tenant namespace:

1. A cert-manager `Certificate` `openshell-gateway-external`:
   - `secretName: openshell-gateway-external-tls`
   - `dnsNames`: `gw-<ns>.<base-domain>` and `*.gw-<ns>.<base-domain>` (one certificate, one ACME order)
   - `rotationPolicy: Always`
   - `issuerRef` resolved from the `GATEWAY_EXTERNAL_ISSUER` and `GATEWAY_EXTERNAL_ISSUER_KIND` controller env vars. Wildcard SANs require a DNS-01 issuer (e.g. the production `letsencrypt-dns01` ClusterIssuer); a cluster configured with an HTTP-01-only issuer cannot serve opted-in Gateways (documented prerequisite)
2. A per-tenant Gateway API `Gateway` `gw-<ns>` with `gatewayClassName` from `GATEWAY_API_GATEWAY_CLASS` (default `openshift-default`) and two listeners on port 443, both HTTPS/Terminate and both referencing the `openshell-gateway-external-tls` secret:
   - `grpc`: exact hostname `gw-<ns>.<base-domain>`, carries the Gateway's gRPC traffic
   - `sandbox`: wildcard hostname `*.gw-<ns>.<base-domain>`, carries sandbox-service traffic and the per-gateway console

   The per-tenant Gateway SHALL carry an `external-dns.kubernetes.io/hostname` infrastructure annotation listing both DNS records comma-separated (`gw-<ns>.<base-domain>,*.gw-<ns>.<base-domain>`), so external-dns publishes both. On clusters without external-dns, the operator adds the two records manually at provision time (documented fallback).
3. The tenant `GRPCRoute` `openshell-gateway` re-parents onto the per-tenant Gateway (`sectionName: grpc`). Its hostname, the BackendTLSPolicy, the backend-CA ConfigMap, and the `routeAddress` format (`grpcs://<hostname>:443`) are unchanged - the same hostname is now resolved through the per-tenant Gateway's load balancer.
4. An `HTTPRoute` `openshell-gateway-sandbox` with the wildcard hostname `*.gw-<ns>.<base-domain>`, parented on the per-tenant Gateway's `sandbox` listener, forwarding sandbox-service traffic to the `openshell-gateway` Service on port 8080. The existing BackendTLSPolicy re-encrypt remains in effect (proxy-to-pod leg is TLS with the pod's internal certificate), and the Host header SHALL be preserved through the proxy because the gateway's virtual-hosting keys on it.
5. The per-gateway console re-parents onto the per-tenant Gateway's `sandbox` listener with host `console-<ns>.gw-<ns>.<base-domain>`, which the wildcard SAN already covers (see [`openshell-gateway-console.spec.md`](./openshell-gateway-console.spec.md)).

The reconciler SHALL inject both `gw-<ns>.<base-domain>` and `*.gw-<ns>.<base-domain>` into the Gateway's `server_sans` for opted-in Gateways, so the gateway derives its per-gateway base domain for exposed-service URLs (see [`openshell-gateway-tls.spec.md`](./openshell-gateway-tls.spec.md) § Per-Gateway External Certificate).

Teardown: un-routing (removing the `route` field) or deleting an opted-in Gateway SHALL remove the `Certificate`, the per-tenant `Gateway`, and the `HTTPRoute` in addition to the shared-path route resources, so a removed Gateway does not leave a live listener serving its hostname or wildcard.

Readiness for an opted-in Gateway SHALL be observed from the per-tenant Gateway in the tenant namespace: the Gateway Exposure adapter SHALL prefer the per-tenant Gateway when present (see § Gateway Exposure Port).

#### Scenario: OSG-ROUT-12.01 -- Opted-in provisioning creates per-gateway resources

- GIVEN a Gateway created with `route: { certificate: {} }`
- AND `GATEWAY_EXTERNAL_ISSUER` is set on the controller to a DNS-01-capable issuer
- WHEN the GatewayReconciler reconciles the Gateway
- THEN it SHALL create the `Certificate` `openshell-gateway-external` in the tenant namespace with both the exact and the wildcard dnsNames
- AND it SHALL create the per-tenant `Gateway` `gw-<ns>` with the `grpc` and `sandbox` listeners on port 443, both referencing `openshell-gateway-external-tls`
- AND the `GRPCRoute` SHALL parent onto the per-tenant Gateway's `grpc` listener and NOT onto the shared Gateway
- AND it SHALL create the `HTTPRoute` `openshell-gateway-sandbox` with the wildcard hostname, parented on the `sandbox` listener and forwarding to the `openshell-gateway` Service on port 8080

#### Scenario: OSG-ROUT-12.02 -- SAN injection for an opted-in gateway

- GIVEN a Gateway created with `route: { certificate: {} }`
- WHEN the GatewayReconciler renders the gateway configuration
- THEN `server_sans` SHALL contain both `gw-<ns>.<base-domain>` and `*.gw-<ns>.<base-domain>`
- AND the gateway SHALL derive the base domain `gw-<ns>.<base-domain>` for exposed-service URLs of the form `<sandbox>--<service>.gw-<ns>.<base-domain>`

#### Scenario: OSG-ROUT-12.03 -- Non-opted-in gateways unchanged

- GIVEN a Gateway with `route: {}` (no `certificate` key)
- WHEN the GatewayReconciler reconciles the Gateway
- THEN it SHALL create only the shared-path route resources (GRPCRoute, BackendTLSPolicy, backend-CA ConfigMap) attaching to the shared Gateway
- AND it SHALL NOT create a per-tenant Gateway, a per-gateway Certificate, or the sandbox HTTPRoute

#### Scenario: OSG-ROUT-12.04 -- Opt-in without GATEWAY_EXTERNAL_ISSUER fails provisioning

- GIVEN a Gateway created with `route: { certificate: {} }`
- AND `GATEWAY_EXTERNAL_ISSUER` is not set on the controller
- WHEN the GatewayReconciler reconciles the Gateway
- THEN provisioning SHALL fail with a descriptive error naming the missing env var
- AND no per-gateway resources SHALL be created

#### Scenario: OSG-ROUT-12.05 -- Un-route removes per-gateway resources

- GIVEN an opted-in Gateway with the per-gateway resources present
- WHEN the `route` field is removed or set to null, or the Gateway is deleted
- THEN the control plane SHALL delete the `Certificate` `openshell-gateway-external`, the per-tenant `Gateway` `gw-<ns>`, and the `HTTPRoute` `openshell-gateway-sandbox`
- AND the `routeAddress` SHALL be cleared
- AND no listener SHALL remain serving the Gateway's hostname or wildcard

---

### Requirement: Shared Gateway (Admin Prerequisite)

---

### Requirement: OSG-ROUT-04 -- Shared Gateway (Admin Prerequisite)

The control plane does NOT create Gateway resources. An administrator provisions a shared Gateway as cluster infrastructure, similar to a GatewayClass. All tenant GRPCRoutes attach to this shared Gateway.

**Carve-out:** a Gateway that opts in to a dedicated route and certificate (`route.certificate`, see § Per-Gateway Dedicated Route and Certificate) does not attach to the shared Gateway; for that Gateway the control plane creates and manages a per-tenant Gateway instead. Every other Gateway keeps the shared-Gateway behavior described here.

The Gateway MUST:
- Use a wildcard hostname (e.g. `*.openshell.example.com`) with a cert-manager-issued wildcard TLS certificate
- Allow GRPCRoute attachments from all namespaces (or use a selector that includes tenant namespaces)
- Have a listener named `grpc` on port 443 with HTTPS/Terminate mode

See `deploy/openshift/infrastructure/GATEWAY-SETUP.md` for setup instructions including cert-manager configuration.

The `GATEWAY_API_GATEWAY_NAME` environment variable MUST be set on the controller to the name of this Gateway. The control plane will fail to reconcile routes if it is not set.

---

### Requirement: OSG-ROUT-05 -- GRPCRoute Resource Specification

The GRPCRoute SHALL be created in the tenant namespace with a cross-namespace parentRef pointing to the shared Gateway:

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: GRPCRoute
metadata:
  name: openshell-gateway
  namespace: <tenant-namespace>
  labels:
    app.kubernetes.io/name: openshell
    app.kubernetes.io/component: gateway
    app.kubernetes.io/managed-by: hypershell-control-plane
    hypershell.redhat.io/managed: "true"
spec:
  parentRefs:
  - name: <GATEWAY_API_GATEWAY_NAME>
    namespace: <GATEWAY_API_GATEWAY_NAMESPACE>
    sectionName: grpc
  hostnames:
  - gw-<tenant-namespace>.<base-domain>
  rules:
  - backendRefs:
    - name: openshell-gateway
      port: 8080
```

The explicit hostname ensures each tenant's GRPCRoute only matches its own subdomain under the wildcard Gateway listener. The backendRef targets the `openshell-gateway` Service in the tenant namespace (same namespace as the GRPCRoute). No `ReferenceGrant` is needed for same-namespace backends.

---

### Requirement: OSG-ROUT-06 -- BackendTLSPolicy for Re-encrypt

The control plane SHALL create a BackendTLSPolicy to enable TLS verification from the shared Gateway to the openshell gateway pod.

- Read `ca.crt` from `openshell-server-tls` Secret
- Create ConfigMap `openshell-gateway-backend-ca` with the CA certificate
- Create BackendTLSPolicy targeting `openshell-gateway` Service
- Validation hostname: `openshell-gateway.<namespace>.svc.cluster.local`

**Note:** BackendTLSPolicy only handles server certificate validation (the ingress proxy verifies the gateway pod's certificate). It does NOT support the proxy presenting a client certificate to the backend. Therefore, when routing is enabled the gateway's `client_ca_path` must be stripped from the config -- see `openshell-gateway-tls.spec.md` § Client Certificate Verification Conditional on Routing.

---

### Requirement: OSG-ROUT-07 -- Route Address Discovery

The GatewayReconciler SHALL obtain the external route address through the Gateway
Exposure port (see § Gateway Exposure Port) and PATCH it into the Gateway's
`routeAddress` field via the API server. For the Gateway API adapter the address
is deterministic (`grpcs://<hostname>:443`, where `<hostname>` is derived from
the namespace and `GATEWAY_API_BASE_DOMAIN`), so the adapter MAY resolve and
publish it as soon as the hostname is derived, before the shared Gateway
reports readiness.

- Format (Gateway API adapter): `grpcs://<hostname>:443`
- Published during the reconciliation that creates the exposure resources, using the same deterministic hostname as the GRPCRoute
- Stored in the Gateway resource for CLI and console consumption
- The `routeAddress` field records *where* the gateway will be reachable; it does **not** by itself assert readiness. Readiness to serve traffic is reflected by the Gateway `phase` reaching `Running`, which for a routed gateway now additionally requires the exposure to be observed Ready (see [`openshell-gateway-health.spec.md`](./openshell-gateway-health.spec.md) § Phase Reflects Workload and Route Readiness). The console and CLI SHALL NOT present the gateway as ready to connect until `phase` is `Running`, even though `routeAddress` may be populated earlier.
- `hsctl get gateways` displays the routeAddress
- If the address cannot be resolved (for example `GATEWAY_API_BASE_DOMAIN` is unset), the `routeAddress` SHALL remain empty

### Requirement: OSG-ROUT-08 -- Gateway Exposure Port

External exposure of a gateway - both **resolving the route address/URL** and
**observing readiness** - SHALL be accessed through an application-owned port so
the reconciler and health loop are decoupled from any single exposure backend.
The Gateway API implementation SHALL be one adapter behind this port; additional
adapters (e.g. OpenShift `Route`, passthrough `Route`) are expected and SHALL be
addable without changing the reconciler or health-reconciliation logic.

The port SHALL expose at least:

- **Address resolution** - given a gateway and its route configuration, return
  the external route address (e.g. `grpcs://<host>:443`), or empty if it cannot
  be resolved.
- **Readiness observation** - given a gateway, return whether its external
  exposure is currently Ready, along with a short human-readable reason when it
  is not. For the Gateway API adapter, Ready is defined as the per-tenant
  Gateway API `Gateway` reporting condition `Programmed=True` with a non-empty
  `.status.addresses`.

The reconciler and `GatewayHealthReconciler` SHALL depend only on the port's
address and readiness results, never on Gateway API types directly. The Gateway
API adapter SHALL read the per-tenant `Gateway` object's status using the typed
`sigs.k8s.io/gateway-api` client.

> **Scope note:** provisioning of the backend-specific resources (Gateway,
> GRPCRoute, BackendTLSPolicy for the Gateway API adapter) MAY
> also move behind this port as adapters are added; at minimum, address
> resolution and readiness observation SHALL be behind it.

#### Scenario: OSG-ROUT-08.01 -- Gateway API adapter reports not-ready before Programmed

- GIVEN a routed Gateway whose per-tenant Gateway API `Gateway` reports
  `Programmed=False` or has an empty `.status.addresses`
- WHEN the reconciler queries readiness through the Gateway Exposure port
- THEN the port SHALL report the exposure as not Ready
- AND SHALL provide a reason (e.g. "gateway not programmed")

#### Scenario: OSG-ROUT-08.02 -- Gateway API adapter reports ready when programmed

- GIVEN a routed Gateway whose per-tenant Gateway API `Gateway` reports
  `Programmed=True` with a non-empty `.status.addresses`
- WHEN the reconciler queries readiness through the Gateway Exposure port
- THEN the port SHALL report the exposure as Ready

#### Scenario: OSG-ROUT-08.03 -- New exposure backend added without touching the reconciler

- GIVEN a future OpenShift `Route`-based exposure adapter implementing the
  Gateway Exposure port
- WHEN it is registered as the active adapter
- THEN the reconciler and health-reconciliation logic SHALL consume its address
  and readiness results without code changes to their phase-decision logic

---

### Requirement: OSG-ROUT-09 -- Gateway API Configuration

| Variable | Default | Description |
|---|---|---|
| `GATEWAY_API_GATEWAY_NAME` | *(required)* | Name of the pre-existing Gateway resource that tenant GRPCRoutes attach to |
| `GATEWAY_API_GATEWAY_NAMESPACE` | `openshift-ingress` | Namespace where the pre-existing Gateway resource lives |
| `GATEWAY_API_BASE_DOMAIN` | auto-detected | Base domain for tenant hostname generation (e.g., `openshell.example.com` → `gw-<ns>.openshell.example.com`) |
| `GATEWAY_EXTERNAL_ISSUER` | *(required when any Gateway opts in via `route.certificate`)* | Name of the DNS-01-capable issuer (e.g. `letsencrypt-dns01`) that issues per-gateway wildcard certificates. Wildcard SANs require DNS-01; a cluster with only an HTTP-01 issuer cannot serve opted-in Gateways |
| `GATEWAY_EXTERNAL_ISSUER_KIND` | `ClusterIssuer` | Kind of the issuer named by `GATEWAY_EXTERNAL_ISSUER` |
| `GATEWAY_API_GATEWAY_CLASS` | `openshift-default` | GatewayClass used by controller-created per-tenant Gateways for opted-in Gateways |

### Requirement: OSG-ROUT-10 -- Gateway Exposure Configuration

| Variable | Default | Description |
|---|---|---|
| `GATEWAY_ROUTE_READY_TIMEOUT` | `10m` | Grace window a routed gateway's external exposure may remain not-Ready (e.g. Gateway API `Programmed=False` / no address, while a cloud load balancer provisions) after the Deployment is Ready, before the control plane transitions the `phase` to `Degraded`. Load balancer provisioning commonly exceeds the Deployment readiness window, so this is deliberately longer than the provisioning Deployment-readiness wait. |

The grace window governs only the `Provisioning → Degraded` transition for
exposure that never becomes Ready; it is not a hard deadline that stops
observation. The control plane SHALL keep observing the exposure after the window
elapses, so a gateway that eventually becomes programmed returns to `Running`.

---

### Requirement: OSG-ROUT-11 -- RBAC for Routing Resources

The control-plane ServiceAccount needs permissions in two namespaces:

**Tenant namespaces** (ClusterRole):
```yaml
- apiGroups: ["gateway.networking.k8s.io"]
  resources: ["grpcroutes", "backendtlspolicies"]
  verbs: ["get", "list", "create", "update", "patch", "delete"]

- apiGroups: ["networking.k8s.io"]
  resources: ["networkpolicies"]
  verbs: ["get", "list", "create", "update", "patch", "delete"]
```

**`openshift-ingress` namespace** (Role + RoleBinding):
```yaml
- apiGroups: ["gateway.networking.k8s.io"]
  resources: ["gateways"]
  verbs: ["get", "list"]
```

The `openshift-ingress` Role is deployed via `controller-gateway-rbac.yaml` in the deploy manifests. The controller only needs read access to the shared Gateway (it no longer creates or deletes Gateway resources).

---

## Reconciler Improvements (Planned)

1. **Gateway restart on ConfigMap change**: The gateway workload needs a hash annotation on the ConfigMap content so it automatically restarts when the TOML changes.

---

## Debugging Reference

| Symptom | Root Cause | Fix |
|---|---|---|
| TLS handshake: 0 bytes read, immediate EOF | Stale NetworkPolicy putting the gateway pod in deny-by-default | Check for and remove any leftover NetworkPolicies in the gateway namespace |
| Wrong TLS certificate served | DNS pointing to wrong LB (e.g. OpenShift default router instead of shared Gateway) | Verify DNS CNAME points to the shared Gateway's LB address |
| `hsctl apply` creates gateway but no external access | No `route` field on Gateway resource | Add `route: {}` to the Gateway resource |
| Gateway pods Ready but phase stuck `Provisioning`, then `Degraded` after the grace window | The shared Gateway API `Gateway` reports `Programmed=False` / no ADDRESS (cloud LB not provisioned, DNS/quota failure) | `oc get gateway -n $GATEWAY_API_GATEWAY_NAMESPACE $GATEWAY_API_GATEWAY_NAME -o wide`; inspect `.status.conditions` and events on the Gateway / its ELB |
| `GATEWAY_API_GATEWAY_NAME is required` error | Env var not set on controller | Set `GATEWAY_API_GATEWAY_NAME` to the pre-existing Gateway name |
| `peer sent no certificates` in gateway logs, client gets `upstream connect error or disconnect/reset before headers` | Gateway config has `client_ca_path` (mTLS) but ingress proxy cannot present client certs | Verify `route.enabled` is true -- the reconciler strips `client_ca_path` when routing is enabled (see TLS spec) |

---

## References

- [NVIDIA OpenShell Kubernetes Ingress Guide](https://docs.nvidia.com/openshell/kubernetes/ingress)
- [BackendTLSPolicy on OpenShift](https://www.redhat.com/en/blog/backendtlspolicy-expands-gateway-api-transport-security)
- [BackendTLSPolicy API Reference](https://gateway-api.sigs.k8s.io/reference/api-types/policy/backendtlspolicy/)
- [Gateway API TLS Guide](https://gateway-api.sigs.k8s.io/guides/tls/)
