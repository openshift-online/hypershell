# OpenShell Gateway Routing Specification

**Date:** 2026-08-05
**Status:** Draft
**Parent:** `openshell-gateway.spec.md` — core gateway provisioning
**Related:** `openshell-gateway-tls.spec.md` — TLS modes; `openshell-gateway-oidc.spec.md` — OIDC authentication

---

## Purpose

This specification defines how OpenShell gateways are exposed to external clients via the **Gateway API** (GRPCRoute + BackendTLSPolicy). The control plane auto-detects Gateway API availability at startup and provisions per-tenant routing resources. A NetworkPolicy for external router ingress is required for connectivity.

---

## Architecture

```
External Client (openshell CLI)
    │  TLS/HTTP2 (ALPN-negotiated)
    ▼
Per-Tenant Gateway (OpenShift gateway controller / Envoy)
    │  Created by control plane in the tenant namespace
    │  Terminates external TLS, negotiates HTTP/2 via ALPN
    │  GRPCRoute matches on hostname, forwards to backendRef
    │  BackendTLSPolicy: re-encrypts to pod, verifies cert via CA
    ▼
openshell-gateway Service (ClusterIP :8080)
    ▼
openshell-gateway Pod
```

Requires:
- OpenShift 4.22+ (GatewayClass `openshift-default`)
- Hostname: `openshell-gateway-<tenant-namespace>.<base-domain>` (auto-derived)

---

## Requirements

### Requirement: NetworkPolicy for Gateway API Proxy Ingress

The GatewayReconciler creates `openshell-gateway-allow-sandbox` which allows ingress only from pods in the same namespace. The Gateway API controller spawns Envoy proxy pods that may run in the tenant namespace or in `openshift-ingress`. A separate NetworkPolicy SHALL be required for external route connectivity.

#### Scenario: Proxy NetworkPolicy required for GRPCRoute

- GIVEN an OpenShell gateway exposed via GRPCRoute
- AND the gateway namespace has NetworkPolicies applied
- WHEN an external client connects via the per-tenant Gateway
- THEN Gateway API proxy pods (Envoy) must reach the gateway pod on port 8080
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
    - podSelector:
        matchLabels:
          gateway.networking.k8s.io/gateway-name: openshell-gateway
    ports:
    - port: 8080
      protocol: TCP
    - port: 8081
      protocol: TCP
```

> The GatewayReconciler SHALL create this NetworkPolicy automatically when the Gateway has a `route` configuration. The ingress rule allows traffic from both the `openshift-ingress` namespace (where some controllers place proxy pods) and from Gateway-labeled proxy pods in the tenant namespace itself.

---

### Requirement: Gateway API Detection

The control plane SHALL detect at startup whether the cluster supports the Gateway API for route provisioning.

#### Scenario: Gateway API available

- GIVEN the `gateway.networking.k8s.io` API group is available (Gateway, GRPCRoute, BackendTLSPolicy CRDs exist)
- AND a GatewayClass named `openshift-default` exists
- THEN the control plane SHALL enable Gateway API route provisioning

#### Scenario: Gateway API not available

- GIVEN the `gateway.networking.k8s.io` API group is NOT available
- THEN the control plane SHALL disable route provisioning
- AND gateways with `route` configuration SHALL log a warning and skip route resource creation

---

### Requirement: Gateway Route Configuration

The Gateway resource SHALL support an optional `route` field that declares external exposure via Gateway API resources.

#### Scenario: Gateway with auto-assigned route host

- GIVEN a Gateway with `route: {}`
- THEN the control plane SHALL create a per-tenant Gateway API Gateway and GRPCRoute in the tenant namespace
- AND the hostname SHALL be `openshell-gateway-<tenant-namespace>.<base-domain>`

#### Scenario: Gateway without route configuration

- GIVEN a Gateway with no `route` field
- THEN no Gateway API resources (Gateway, GRPCRoute, BackendTLSPolicy) SHALL be created
- AND the gateway SHALL be accessible only via cluster-internal DNS and `kubectl port-forward`

---

### Requirement: Per-Tenant Gateway Resource Specification

The GatewayReconciler SHALL create a Gateway API Gateway resource in the tenant namespace for each openshell gateway with `route` configuration:

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: openshell-gateway
  namespace: <tenant-namespace>
  labels:
    app.kubernetes.io/name: openshell
    app.kubernetes.io/component: gateway
    app.kubernetes.io/managed-by: hypershell-control-plane
spec:
  gatewayClassName: openshift-default
  listeners:
  - name: grpc
    hostname: "openshell-gateway-<tenant-namespace>.<base-domain>"
    port: 443
    protocol: HTTPS
    tls:
      mode: Terminate
      certificateRefs:
      - name: grpc-gateway-certs
        kind: Secret
```

---

### Requirement: GRPCRoute Resource Specification

The GRPCRoute SHALL reference the per-tenant Gateway in the same namespace (no cross-namespace parentRef):

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
  ownerReferences:
  - apiVersion: apps/v1
    kind: Deployment
    name: openshell-gateway
    controller: true
    blockOwnerDeletion: true
spec:
  parentRefs:
  - name: openshell-gateway
  hostnames:
  - openshell-gateway-<tenant-namespace>.<base-domain>
  rules:
  - backendRefs:
    - name: openshell-gateway
      port: 8080
```

---

### Requirement: BackendTLSPolicy for Re-encrypt

The control plane SHALL create a BackendTLSPolicy to enable TLS verification from the per-tenant Gateway to the openshell gateway pod.

- Read `ca.crt` from `openshell-server-tls` Secret
- Create ConfigMap `openshell-backend-ca` with the CA certificate
- Create BackendTLSPolicy targeting `openshell-gateway` Service
- Validation hostname: `openshell-gateway.<namespace>.svc.cluster.local`

---

### Requirement: Route Address Discovery

The GatewayReconciler SHALL derive the external route address from the GRPCRoute hostname and PATCH it into the Gateway's `routeAddress` field via the API server.

- Format: `grpcs://<hostname>:443`
- Stored in the Gateway API resource for CLI consumption
- `hsctl get gateways` displays the routeAddress

---

### Requirement: Gateway API Configuration

| Variable | Default | Description |
|---|---|---|
| `GATEWAY_API_BASE_DOMAIN` | auto-detected | Cluster base domain for hostname generation (read from `ingresses.config.openshift.io/cluster` `.spec.domain`) |

---

### Requirement: RBAC for Routing Resources

```yaml
- apiGroups: ["gateway.networking.k8s.io"]
  resources: ["gateways", "grpcroutes", "backendtlspolicies"]
  verbs: ["get", "list", "create", "update", "patch", "delete"]

- apiGroups: ["gateway.networking.k8s.io"]
  resources: ["gatewayclasses"]
  verbs: ["get", "list"]

- apiGroups: ["networking.k8s.io"]
  resources: ["networkpolicies"]
  verbs: ["create", "get", "update", "patch", "delete"]
```

---

## Reconciler Improvements (Planned)

1. **Gateway restart on ConfigMap change**: The gateway workload needs a hash annotation on the ConfigMap content so it automatically restarts when the TOML changes.

---

## Debugging Reference

| Symptom | Root Cause | Fix |
|---|---|---|
| TLS handshake: 0 bytes read, immediate EOF | NetworkPolicy blocking Gateway API proxy → gateway | Create `openshell-gateway-allow-router` |
| grpcurl hangs but openssl s_client works | grpcurl blocked by NetworkPolicy | Check source namespace |
| `hsctl apply` creates gateway but no external access | No `route` field on Gateway resource | Add `route: {}` to the Gateway resource |

---

## References

- [NVIDIA OpenShell Kubernetes Ingress Guide](https://docs.nvidia.com/openshell/kubernetes/ingress)
- [BackendTLSPolicy on OpenShift](https://www.redhat.com/en/blog/backendtlspolicy-expands-gateway-api-transport-security)
- [BackendTLSPolicy API Reference](https://gateway-api.sigs.k8s.io/reference/api-types/policy/backendtlspolicy/)
- [Gateway API TLS Guide](https://gateway-api.sigs.k8s.io/guides/tls/)
