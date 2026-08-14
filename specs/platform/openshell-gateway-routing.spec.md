# OpenShell Gateway Routing Specification

**Date:** 2026-08-05
**Status:** Draft
**Parent:** `openshell-gateway.spec.md` - core gateway provisioning
**Related:** `openshell-gateway-tls.spec.md` - TLS modes; `openshell-gateway-oidc.spec.md` - OIDC authentication

---

## Purpose

This specification defines how OpenShell gateways are exposed to external clients via the **Gateway API** (GRPCRoute + BackendTLSPolicy). The control plane auto-detects Gateway API availability at startup and provisions per-tenant routing resources. A NetworkPolicy for external router ingress is required for connectivity.

---

## Architecture

```
External Client (openshell CLI)
    │  TLS/HTTP2 (ALPN-negotiated)
    ▼
Per-Tenant Gateway (openshift-ingress namespace, Envoy)
    │  Created by control plane in openshift-ingress (NOT the tenant namespace)
    │  Ingress operator auto-creates DNSRecord → Route 53 CNAME → ELB
    │  Terminates external TLS, negotiates HTTP/2 via ALPN
    │  GRPCRoute (in tenant namespace) matches on hostname, forwards to backendRef
    │  BackendTLSPolicy: re-encrypts to pod, verifies cert via CA
    ▼
openshell-gateway Service (ClusterIP :8080, tenant namespace)
    ▼
openshell-gateway Pod (tenant namespace)
```

Requires:
- OpenShift 4.22+ (GatewayClass `openshift-default`)
- Hostname: `gw-<tenant-namespace>.<base-domain>` (auto-derived)

### Why openshift-ingress?

The OpenShift ingress operator's Gateway Service DNS controller only auto-creates `DNSRecord` CRs (which publish CNAME records to Route 53) for Gateway resources in the `openshift-ingress` namespace. Gateways in other namespaces get their own ELB but no DNS record, so the `*.apps` wildcard resolves to the default router instead of the Gateway's ELB. Placing the Gateway in `openshift-ingress` gives automatic DNS management without needing the ExternalDNS Operator.

---

## Requirements

### Requirement: NetworkPolicy for Gateway API Proxy Ingress

The GatewayReconciler creates `openshell-gateway-allow-sandbox-v2` which allows ingress only from pods in the same namespace. The Gateway API controller spawns Envoy proxy pods in the tenant namespace (co-located with the gateway). A separate NetworkPolicy SHALL be required for external route connectivity.

> **Future consideration:** This same-namespace policy assumes gateways and sandboxes are co-located on the same cluster. When separating GatewayClusters from SandboxClusters, or when supporting non-Kubernetes sandbox backends (e.g., VMs), the NetworkPolicy model will need to be extended to allow cross-namespace or cross-cluster ingress. For now, the smallest blast radius is achieved by keeping a dedicated gateway per tenant namespace.

#### Scenario: Proxy NetworkPolicy required for GRPCRoute

- GIVEN an OpenShell gateway exposed via GRPCRoute
- AND the gateway namespace has NetworkPolicies applied
- WHEN an external client connects via the per-tenant Gateway
- THEN Gateway API proxy pods (Envoy, running in `openshift-ingress`) must reach the gateway pod on port 8080
- AND without the proxy NetworkPolicy, the TLS handshake hangs with zero bytes read

#### NetworkPolicy Definition

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: openshell-gateway-allow-router
  namespace: <tenant-namespace>
spec:
  podSelector:
    matchLabels:
      app.kubernetes.io/instance: openshell-gateway
      app.kubernetes.io/name: openshell
  ingress:
  - from:
    - namespaceSelector:
        matchLabels:
          kubernetes.io/metadata.name: openshift-ingress
      podSelector:
        matchLabels:
          gateway.networking.k8s.io/gateway-name: gw-<tenant-namespace>
    ports:
    - port: 8080
      protocol: TCP
    - port: 8081
      protocol: TCP
```

> The GatewayReconciler SHALL create this NetworkPolicy automatically when the Gateway has a `route` configuration. The ingress rule allows traffic from Gateway-labeled Envoy proxy pods in `openshift-ingress` (cross-namespace, since the Gateway lives there).

---

### Requirement: Gateway API Detection

The control plane SHALL detect at startup whether the cluster supports the Gateway API for route provisioning.

#### Scenario: Gateway API available

- GIVEN the `gateway.networking.k8s.io` API group is available (Gateway, GRPCRoute, BackendTLSPolicy CRDs exist)
- AND a supported GatewayClass exists (e.g., `openshift-default` on OpenShift, `cloud-provider-kind` on Kind)
- THEN the control plane SHALL enable Gateway API route provisioning
- AND the GatewayClass name SHALL be configurable via the `GATEWAY_API_GATEWAY_CLASS` environment variable (default: `openshift-default`)

#### Scenario: Gateway API not available

- GIVEN the `gateway.networking.k8s.io` API group is NOT available
- THEN the control plane SHALL disable route provisioning
- AND gateways with `route` configuration SHALL log a warning and skip route resource creation

---

### Requirement: Gateway Route Configuration

The Gateway resource SHALL support an optional `route` field that declares external exposure via Gateway API resources.

#### Scenario: Gateway with auto-assigned route host

- GIVEN a Gateway with `route: {}`
- THEN the control plane SHALL create a per-tenant Gateway API Gateway in the `openshift-ingress` namespace (named `gw-<tenant-namespace>`)
- AND the control plane SHALL create a GRPCRoute in the tenant namespace with a cross-namespace parentRef to the Gateway
- AND the hostname SHALL be `gw-<tenant-namespace>.<base-domain>`
- AND the derived hostname's first DNS label (e.g., `gw-openshell-a1b2c3d4e5f67890` at 29 chars) SHALL be well within the 63-character DNS label limit (RFC 1123), so no truncation is needed

#### Scenario: Gateway with explicit route host

- GIVEN a Gateway with `route: { host: "custom.example.com" }`
- THEN the control plane SHALL use the provided hostname instead of auto-deriving it
- AND the hostname SHALL be validated as a valid RFC 1123 DNS name before use

#### Scenario: Gateway without route configuration

- GIVEN a Gateway with no `route` field
- THEN no Gateway API resources (Gateway, GRPCRoute, BackendTLSPolicy) SHALL be created
- AND the gateway SHALL be accessible only via cluster-internal DNS and `kubectl port-forward`

#### Scenario: Route configuration removed from existing Gateway

- GIVEN a Gateway that previously had `route` configuration and associated route resources
- WHEN the `route` field is removed or set to null
- THEN the GatewayReconciler SHALL delete all route-owned resources: the per-tenant Gateway API Gateway (from `openshift-ingress`), GRPCRoute, BackendTLSPolicy, `openshell-backend-ca` ConfigMap, and `openshell-gateway-allow-router` NetworkPolicy
- AND it SHALL clear the `routeAddress` field on the Gateway resource via the API server
- AND the gateway SHALL revert to cluster-internal-only access

---

### Requirement: Per-Tenant Gateway Resource Specification

The GatewayReconciler SHALL create a Gateway API Gateway resource in the `openshift-ingress` namespace for each openshell gateway with `route` configuration. The Gateway is placed in `openshift-ingress` so the ingress operator auto-creates a `DNSRecord` CR pointing the listener hostname to the Gateway's load balancer.

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: gw-<tenant-namespace>
  namespace: openshift-ingress
  labels:
    app.kubernetes.io/name: openshell
    app.kubernetes.io/component: gateway
    app.kubernetes.io/managed-by: hypershell-control-plane
    hypershell.redhat.io/tenant: <tenant-namespace>
spec:
  gatewayClassName: <GATEWAY_API_GATEWAY_CLASS>   # default: openshift-default
  listeners:
  - name: grpc
    hostname: "gw-<tenant-namespace>.<base-domain>"
    port: 443
    protocol: HTTPS
    tls:
      mode: Terminate
      certificateRefs:
      - name: grpc-gateway-certs
        kind: Secret
    allowedRoutes:
      namespaces:
        from: Selector
        selector:
          matchLabels:
            kubernetes.io/metadata.name: <tenant-namespace>
```

The `grpc-gateway-certs` Secret must exist in `openshift-ingress` (cluster prerequisite -- see README). The `allowedRoutes` selector restricts which namespaces can attach GRPCRoutes to this Gateway.

---

### Requirement: GRPCRoute Resource Specification

The GRPCRoute SHALL be created in the tenant namespace with a cross-namespace parentRef pointing to the Gateway in `openshift-ingress`:

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
  - name: gw-<tenant-namespace>
    namespace: openshift-ingress
  hostnames:
  - gw-<tenant-namespace>.<base-domain>
  rules:
  - backendRefs:
    - name: openshell-gateway
      port: 8080
```

The backendRef targets the `openshell-gateway` Service in the tenant namespace (same namespace as the GRPCRoute). No `ReferenceGrant` is needed for same-namespace backends.

---

### Requirement: BackendTLSPolicy for Re-encrypt

The control plane SHALL create a BackendTLSPolicy to enable TLS verification from the per-tenant Gateway to the openshell gateway pod.

- Read `ca.crt` from `openshell-server-tls` Secret
- Create ConfigMap `openshell-backend-ca` with the CA certificate
- Create BackendTLSPolicy targeting `openshell-gateway` Service
- Validation hostname: `openshell-gateway.<namespace>.svc.cluster.local`

**Note:** BackendTLSPolicy only handles server certificate validation (the ingress proxy verifies the gateway pod's certificate). It does NOT support the proxy presenting a client certificate to the backend. Therefore, when routing is enabled the gateway's `client_ca_path` must be stripped from the config — see `openshell-gateway-tls.spec.md` § Client Certificate Verification Conditional on Routing.

---

### Requirement: Route Address Discovery

The GatewayReconciler SHALL derive the external route address from the GRPCRoute hostname and PATCH it into the Gateway's `routeAddress` field via the API server as soon as the hostname is derived during reconciliation. The route address is deterministic (`grpcs://<hostname>:443`, where `<hostname>` is derived from the namespace and `GATEWAY_API_BASE_DOMAIN`), so it is known before the per-tenant Gateway reports readiness. The reconciler SHALL NOT wait for `Accepted`/`Programmed` conditions before publishing it.

- Format: `grpcs://<hostname>:443`
- Published during the reconciliation that creates the Gateway API resources, using the same deterministic hostname as the GRPCRoute
- Stored in the Gateway API resource for CLI and console consumption so the connection command is available while the gateway finishes provisioning
- Readiness to serve traffic is reflected separately by the Gateway `phase` (`Provisioning` → `Running`), not by the presence of `routeAddress`
- `hsctl get gateways` displays the routeAddress
- If the hostname cannot be derived (for example `GATEWAY_API_BASE_DOMAIN` is unset), the `routeAddress` SHALL remain empty

---

### Requirement: Gateway API Configuration

| Variable | Default | Description |
|---|---|---|
| `GATEWAY_API_BASE_DOMAIN` | auto-detected | Cluster base domain for hostname generation (read from `ingresses.config.openshift.io/cluster` `.spec.domain`) |
| `GATEWAY_API_GATEWAY_CLASS` | `openshift-default` | GatewayClass name for per-tenant Gateway resources (e.g., `cloud-provider-kind` for Kind clusters) |
| `GATEWAY_API_GATEWAY_NAMESPACE` | `openshift-ingress` | Namespace where per-tenant Gateway API Gateway resources are created. On OpenShift, the ingress operator only auto-creates DNSRecord CRs for Gateways in `openshift-ingress` |

---

### Requirement: RBAC for Routing Resources

The control-plane ServiceAccount needs permissions in two namespaces:

**Tenant namespaces** (ClusterRole):
```yaml
- apiGroups: ["gateway.networking.k8s.io"]
  resources: ["grpcroutes", "backendtlspolicies"]
  verbs: ["get", "list", "create", "update", "patch", "delete"]

- apiGroups: ["gateway.networking.k8s.io"]
  resources: ["gatewayclasses"]
  verbs: ["get", "list"]

- apiGroups: ["networking.k8s.io"]
  resources: ["networkpolicies"]
  verbs: ["get", "list", "create", "update", "patch", "delete"]
```

**`openshift-ingress` namespace** (Role + RoleBinding):
```yaml
- apiGroups: ["gateway.networking.k8s.io"]
  resources: ["gateways"]
  verbs: ["get", "list", "create", "update", "patch", "delete"]

- apiGroups: [""]
  resources: ["secrets"]
  verbs: ["get", "list"]
```

The `openshift-ingress` Role is deployed via `controller-gateway-rbac.yaml` in the deploy manifests.

---

## Reconciler Improvements (Planned)

1. **Gateway restart on ConfigMap change**: The gateway workload needs a hash annotation on the ConfigMap content so it automatically restarts when the TOML changes.

---

## Debugging Reference

| Symptom | Root Cause | Fix |
|---|---|---|
| TLS handshake: 0 bytes read, immediate EOF | NetworkPolicy blocking Gateway API proxy → gateway | Create `openshell-gateway-allow-router` |
| grpcurl hangs but openssl s_client works | grpcurl blocked by NetworkPolicy | Check pod labels match `gateway.networking.k8s.io/gateway-name` selector |
| `hsctl apply` creates gateway but no external access | No `route` field on Gateway resource | Add `route: {}` to the Gateway resource |
| `peer sent no certificates` in gateway logs, client gets `upstream connect error or disconnect/reset before headers` | Gateway config has `client_ca_path` (mTLS) but ingress proxy cannot present client certs | Verify `route.enabled` is true — the reconciler strips `client_ca_path` when routing is enabled (see TLS spec) |

---

## References

- [NVIDIA OpenShell Kubernetes Ingress Guide](https://docs.nvidia.com/openshell/kubernetes/ingress)
- [BackendTLSPolicy on OpenShift](https://www.redhat.com/en/blog/backendtlspolicy-expands-gateway-api-transport-security)
- [BackendTLSPolicy API Reference](https://gateway-api.sigs.k8s.io/reference/api-types/policy/backendtlspolicy/)
- [Gateway API TLS Guide](https://gateway-api.sigs.k8s.io/guides/tls/)
