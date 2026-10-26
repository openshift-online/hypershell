# Investigation: Per-gateway certificates (opt-in, provision time)

**Date:** 2026-09-29
**Status:** Investigation / design proposal - no code changed
**Scope:** Client-facing TLS for tenant gateways; the shared wildcard certificate on
the shared Gateway API `Gateway` is the status quo. This document traces today's TLS
provisioning, identifies what a per-gateway certificate changes, compares options,
and proposes a recommended plan.

---

## 1. How gateway TLS and hostname are provisioned today

### 1.1 The client-facing (external) TLS leg

```
openshell CLI ──TLS──▶ shared Gateway `openshell-grpc-gateway`
                       (openshift-ingress, class openshift-default,
                        Argo CD-owned via hypershell-gitops)
                       listener grpc-<spoke>, hostname *.<spoke>.infra.hypershell.app
                       wildcard cert-manager Certificate (e.g. hyp7-wildcard
                       → secret hyp7-instance-tls, ClusterIssuer letsencrypt-dns01)
        ──GRPCRoute (tenant ns, host gw-<ns>.<spoke>.infra...)──▶
        ──BackendTLSPolicy re-encrypt──▶ openshell-gateway Service :8080 ──▶ pod
```

- The **shared cluster certificate** is a per-spoke wildcard Certificate
  (`*.<spoke>.infra.hypershell.app`) living in `openshift-ingress`, owned by
  hypershell-gitops (`clusters/hshellp01ue1/gateway/certificates.yaml`,
  `clusters/hypc-aws-00/3-bootstrap-apps/03-ingress-certificate/gateways/`). It is
  issued and rotated by cert-manager from the `letsencrypt-dns01` ClusterIssuer.
  Every tenant gateway on a spoke rides this one wildcard; the control plane never
  touches it (`openshell-gateway.spec.md` § Gateway Ingress TLS Certificate: "The
  control plane does not create or manage the Gateway-level TLS").
- The control plane creates, per tenant gateway (reconciler.go
  `reconcileGatewayAPIResources`): the **GRPCRoute** in the tenant namespace
  (parentRef → shared Gateway listener, hostname `gw-<ns>.<base-domain>` derived by
  `exposure.DeriveGatewayAPIHost`, single source of truth), the
  **BackendTLSPolicy** + **`openshell-gateway-backend-ca` ConfigMap** (contents of
  the tenant's internal `openshell-server-tls` `ca.crt`), and publishes
  `routeAddress = grpcs://<host>:443` via the Gateway Exposure port.
- Hostname opt-in today: the Gateway API resource's `route` JSONB field
  (`{host: ..., enabled: true}`), parsed by `parseGatewayRouteConfig` in
  `components/control-plane/internal/reconciler/reconciler.go`. An explicit host
  under the base domain must be the tenant's own `gw-<ns>` slot (anti-hijack
  validation in `deriveGatewayHostname`).

### 1.2 The internal (backend) TLS leg

- The Helm chart (`charts/openshell/templates/cert-manager-pki.yaml`) creates, in
  the tenant namespace: self-signed `Issuer` → CA `Certificate` (`openshell-ca` →
  secret `openshell-ca-tls`) → CA-backed `Issuer` (`openshell-ca-issuer`) → server
  `Certificate` (`openshell-server` → secret `openshell-server-tls`, SANs from
  `certManager.serverDnsNames`) and a client `Certificate`. cert-manager rotates
  all of them.
- The reconciler injects the external hostname into `ServerDnsNames` before Helm
  install so the backend cert covers `gw-<ns>...` (BackendTLSPolicy validation
  hostname is `openshell-gateway.<ns>.svc.cluster.local`, covered by defaults).
- `client_ca_path` in `gateway.toml` is stripped when routing is enabled (the
  Gateway proxy cannot present a client cert; BackendTLSPolicy only validates the
  server side) - `openshell-gateway-tls.spec.md` § Client Certificate Verification
  Conditional on Routing.
- The chart already supports an **external server certificate**:
  `certManager.serverIssuerRef` creates a second `Certificate`
  (`openshell-gateway-server-external` → secret
  `openshell-gateway-server-external-tls`) from an operator-configured issuer with
  only externally-resolvable SANs, mounted at `/etc/openshell-tls/server-external`
  and presented via SNI by the gateway pod. Today this is wired only for Route
  passthrough mode (`GATEWAY_INGRESS_MODE=route`, e.g. ROKS), where the pod itself
  terminates client TLS.

### 1.3 Certificate renewal reaching running components (verified)

- **Gateway listener (shared wildcard):** no reloader or rollout exists anywhere in
  hypershell or hypershell-gitops for the shared cert. Production already depends
  on the Gateway API implementation (OpenShift `openshift-default`, Istio-based)
  hot-reloading the rotated Secret via its SDS/watch path - `hyp7-wildcard` has
  been rotating in production with no restart mechanism.
- **Gateway pod (internal/external certs):** no config-hash / Reloader / rollout
  exists (`applyConfigHashAnnotation` from `openshell-gateway-secret-rotation.spec.md`
  is aspirational - it is not in the code). Pods keep their loaded keypairs until
  restarted. Whether the OpenShell gateway binary re-reads the mounted cert at
  handshake (as the rh-trex-ai framework does for the hub api-server,
  `hub-grpc-tls.spec.md`) is **unverified** - the gateway is the upstream NVIDIA
  OpenShell binary, and rh-trex-ai is not vendored here.

---

## 2. What changes for a per-gateway certificate

Requirements recap: opt-in, configured on the gateway **at provision time only**;
no add/switch after provisioning; cert-manager issues **and rotates** it.

### 2.1 Where the opt-in config lives

The `route` JSONB field on the Gateway API resource is the natural home - a
dedicated cert only makes sense for a routed (externally exposed) gateway, and the
field is already provision-time input parsed by the control plane:

```json
route: {
  "host": "special.example.com",          // optional, as today
  "certificate": {
    "issuerRef": { "name": "letsencrypt-dns01", "kind": "ClusterIssuer" }
  }
}
```

- `RouteConfig` (`components/control-plane/internal/gateway/config.go`) gains
  `Certificate *RouteCertificateConfig { IssuerRef {Name, Kind} }`.
- `parseGatewayRouteConfig` unmarshals it transparently (JSON passthrough).
- Validation (`validation.go`): issuer name required, kind ∈ {Issuer,
  ClusterIssuer}; host must be externally resolvable (the chart's own guard
  rejects `.svc`/`.localhost` names for external issuers - mirror it).
- No update support: the API server's update path may reject changes to
  `route.certificate` after creation (provision-time only), or simply leave it
  un-enforced (the reconciler only reads it during initial provisioning).

### 2.2 What the control plane creates (recommended option, see §3)

For an opted-in gateway, in the **tenant namespace** (all names illustrative):

1. **`Certificate openshell-gateway-external`** -
   `dnsNames: [<the gateway's route hostname>]`, `issuerRef` from the opt-in
   config, `secretName: openshell-gateway-external-tls`,
   `rotationPolicy: Always`, standard labels
   (`hypershell.redhat.io/managed=true`, `app.kubernetes.io/managed-by=hypershell-control-plane`).
   cert-manager owns issuance and renewal from here on.
2. **`Gateway gw-<ns>`** (per-tenant Gateway API Gateway) - class
   `openshift-default`, one `grpc` listener, port 443, HTTPS/Terminate, hostname =
   the gateway's route hostname, `certificateRefs` → the per-gateway secret
   (same namespace ⇒ no ReferenceGrant needed), `allowedRoutes.namespaces.from:
   Same`. Same managed labels.
3. The **GRPCRoute's parentRef** points at this per-tenant Gateway (sectionName
   `grpc`) instead of the shared Gateway. Everything else (BackendTLSPolicy,
   backend-CA ConfigMap, routeAddress publishing) is unchanged.
4. **DNS**: the per-tenant Gateway gets its own proxy Service/LB. Publish it by
   setting the `external-dns.kubernetes.io/hostname` infrastructure annotation on
   the per-tenant Gateway (the same mechanism hypershell-gitops uses on the shared
   Gateway; external-dns is deployed on the AWS spokes). On clusters without
   external-dns, the operator adds the DNS record as a provision-time manual step.

### 2.3 Rotation (the key advantage)

The per-gateway cert is consumed by the **Gateway listener**, not by the gateway
pod. The Gateway API implementation watches the TLS Secret and serves renewed
certificates on new handshakes - exactly the mechanism production already relies
on for the rotating shared wildcard (`hyp7-wildcard`). **No pod restart, no
Reloader, and no new hot-reload requirement is introduced.** The internal
per-tenant CA/backend cert keeps today's behavior (renewed by cert-manager, picked
up on next pod restart), unchanged by this feature.

### 2.4 Ownership and garbage collection

- Certificate, Gateway, Secret, GRPCRoute, BackendTLSPolicy all live in the tenant
  namespace → reclaimed by namespace deletion (same as today's tenant resources;
  `DeleteGatewayResources` comment already documents namespace-deletion GC).
- Route removal (un-route) / gateway delete: extend `DeleteGatewayAPIResources`
  to explicitly delete the per-tenant Gateway and Certificate (mirroring today's
  GRPCRoute/BackendTLSPolicy deletion), so un-routing doesn't leave a live
  listener serving the tenant's hostname.

### 2.5 RBAC

Already sufficient: `deploy/base/platform-resources/controller-rbac.yaml` grants
the controller full CRUD on `gateways`, `grpcroutes`, `backendtlspolicies`
(gateway.networking.k8s.io) and `issuers`, `certificates` (cert-manager.io)
cluster-wide. No RBAC change needed.

### 2.6 Exposure adapter (readiness observation)

`GatewayAPIExposure.ObserveReadiness` reads `Gateways(gatewayIngressNamespace())`
with `perTenantGatewayName(namespace)` - which prefers the shared name when
`GATEWAY_API_GATEWAY_NAME` is set. For an opted-in gateway it must instead read
the **per-tenant Gateway in the tenant namespace** and report its
`Programmed`/addresses (a ~20-line change in `gatewayapi.go`; the port interface
is unchanged, satisfying the routing spec's adapter-isolation requirement).

### 2.7 Console

The per-gateway console's HTTPRoute keeps attaching to the **shared** Gateway
listener - `console-<ns>.<spoke>...` remains covered by the spoke wildcard. The
per-gateway cert applies only to the gateway gRPC data plane. (If a per-gateway
console cert is ever wanted, the same per-tenant Gateway can gain an `https`
listener later; explicitly out of scope now.)

---

## 3. Option comparison

| | **A. Per-gateway listener on the shared Gateway** | **B. Per-tenant Gateway + controller-created Certificate (recommended)** | **C. Route passthrough + pod-terminated external cert** | **D. User-supplied Secret only** |
|---|---|---|---|---|
| How | Control plane patches a new listener + cert ref onto `openshift-ingress/openshell-grpc-gateway` | Control plane creates per-tenant `Gateway` + `Certificate` in tenant ns; GRPCRoute re-parents | Per-gateway ingress-mode override to `route`; reuse chart's `certManager.serverIssuerRef` + SNI cert mounted in the pod | User pre-creates a secret/issuer; control plane only wires the reference into a per-tenant Gateway |
| cert-manager rotates | Yes | **Yes** | Yes | Only if user also creates the Certificate (else manual) |
| Renewal reaches serving path | Yes (listener) | **Yes (listener, proven by shared wildcard)** | Only if gateway pod hot-reloads - **unverified**, and no restart mechanism exists today | Same as B if user uses cert-manager |
| Argo CD conflict | **Fatal**: shared Gateway is Argo-owned in gitops; Argo reverts control-plane writes | None (tenant-ns resources) | None | None |
| DNS | Wildcard keeps working | Needs per-gateway DNS record → per-tenant LB (external-dns automatable; manual on some clusters) | Custom host must point at the OpenShift router, or host must move to cluster apps domain | Same as B |
| Cost per opted-in gateway | None | One extra proxy Deployment + LB per opted-in gateway (Istio data plane) | None extra (uses cluster router) | Same as B |
| Architecture fit | Breaks "control plane does not touch the shared Gateway" | Fits Gateway API mode; exposure adapter already anticipates per-tenant Gateways | **Requires per-gateway ingress mode** - today `GATEWAY_INGRESS_MODE` is cluster-level, threaded through reconciler, chart values, console, and health loop; a big refactor | Fits, but pushes cert lifecycle correctness onto the user |
| Risks | Ownership war, drift | New LB/IP per gateway; external-dns dependency; listener count none (separate Gateways) | Mode mixing; mTLS/client_ca semantics change; unverified pod cert reload | Orphaned/expired secrets; violates "cert-manager must rotate" unless enforced |

**Recommendation: Option B**, with Option D as a later sub-variant (allow
`route.certificate.secretRef` to reference a user-managed Certificate instead of
an issuerRef) if users ever want to bring their own PKI. Option A is rejected
outright (Argo CD ownership). Option C is rejected because it needs a per-gateway
ingress mode, which cuts against the environment-adaptive single-mode design, and
because pod cert reload is unverified.

---

## 4. Spec conflicts and required spec changes

Conflicts found:

1. `openshell-gateway-routing.spec.md` § Shared Gateway (Admin Prerequisite):
   "The control plane does NOT create Gateway resources." - Option B violates
   this; needs an explicit exception for per-gateway-cert gateways.
2. `openshell-gateway.spec.md` § Gateway Ingress TLS Certificate: "The control
   plane does not create or manage the Gateway-level TLS." - same carve-out
   needed.
3. `openshell-gateway-routing.spec.md` § Gateway Exposure Port already says the
   adapter reads "the **per-tenant** Gateway API `Gateway`" - existing spec/code
   drift (code prefers the shared name when the env var is set). The new work
   should reconcile this text.
4. `openshell-gateway-tls.spec.md` covers only the per-tenant internal PKI; a new
   requirement section is needed for per-gateway external certificates.
5. `openshell-gateway-secret-rotation.spec.md` claims a config-hash restart
   mechanism for TLS renewal that does not exist in code - orthogonal, but worth
   fixing or annotating while touching TLS specs.

Spec changes needed:

- `specs/platform/openshell-gateway-routing.spec.md`: new Requirement
  "Per-Gateway Certificate Exposure" (opt-in `route.certificate` at provision
  time; per-tenant Gateway + Certificate in tenant ns; GRPCRoute re-parenting;
  DNS/external-dns expectation; adapter readiness from the per-tenant Gateway;
  un-route/delete teardown). Amend the Shared Gateway requirement with the
  carve-out.
- `specs/platform/openshell-gateway-tls.spec.md`: new Requirement "Per-Gateway
  External Certificate" (cert-manager Certificate + issuerRef from route config;
  rotation served by the Gateway listener without pod involvement; SAN rules).
- `specs/platform/openshell-gateway.spec.md`: extend the `route` field table
  (`route.certificate.issuerRef.{name,kind}`), the example Gateway config, and
  the data-model `route` JSONB description.
- `specs/platform/data-model.spec.md`: update the `route` JSONB column comment.

---

## 5. Recommended implementation plan (files to touch)

After plan approval - nothing below is implemented yet:

1. **API / config plumbing**
   - `components/control-plane/internal/gateway/config.go` - `RouteConfig.Certificate`.
   - `components/control-plane/internal/gateway/validation.go` (+ tests) -
     issuerRef validation, external-host SAN rules.
   - `components/api-server/openapi/` - route field schema documentation if the
     route object is schema'd there (route is a JSON string passthrough today;
     likely doc-only).
2. **Reconciler** - `components/control-plane/internal/gateway/reconciler.go`
   - In `reconcileGatewayAPIResources`: when `route.certificate` is set, create
     the `Certificate` + per-tenant `Gateway` (labeled, dynamic client,
     `reconcileResource`), point the GRPCRoute parentRef at it, and wait for the
     per-gateway secret before proceeding (reuse `waitForSecret`).
   - `DeleteGatewayAPIResources` (+ health-loop teardown): delete the per-tenant
     Gateway + Certificate on un-route.
3. **Exposure adapter** - `components/control-plane/internal/exposure/gatewayapi.go`
   (+ tests): `ObserveReadiness` reads the per-tenant Gateway in the tenant
   namespace when the gateway is on a per-gateway cert.
4. **DNS** - set `external-dns.kubernetes.io/hostname` on the per-tenant
   Gateway's `infrastructure.annotations`; document the manual fallback.
5. **Tests** - `reconciler_test.go`, `exposure/gatewayapi_test.go`,
   `charts/openshell/tests/` unaffected (chart not changed). E2E:
   `tests/e2e/` add a per-gateway-cert scenario (Kind/dev-cluster first; the
   OpenShift e2e would need an issuer that can issue for the derived host).
6. **Specs** - as listed in §4.
7. **Docs** - `deploy/openshift/infrastructure/GATEWAY-SETUP.md` note;
   `docs/architecture/source/ingress.md` diagram variant.

Explicitly **not** needed: RBAC changes, chart changes (the per-tenant Gateway and
Certificate are reconciler-owned like the GRPCRoute today, not chart-owned), and
any pod-side reload mechanism.

---

## 6. Open questions for the user

1. **Issuer choice:** should the opt-in always name an explicit issuer
   (`route.certificate.issuerRef`), or should an omitted issuerRef fall back to a
   cluster-configured default external issuer (new env var, e.g.
   `GATEWAY_EXTERNAL_CA_ISSUER`, mirroring the Route-mode
   `ExternalCAIssuerName`)?
2. **Hostname:** is the per-gateway cert expected for the standard derived host
   `gw-<ns>.<domain>` (likely - SAN = that host), or primarily for explicit
   custom hosts (`route.host`)? Both work identically in the design.
3. **DNS:** on spokes without external-dns (IBM?), is a manual provision-time DNS
   record acceptable, or must per-gateway certs be limited to external-dns
   clusters?
4. **Cost:** is one extra Istio proxy Deployment + LB per opted-in gateway
   acceptable, given this is a special opt-in configuration?
5. **Update semantics:** "no support for updates" - should the API actively
   reject `route.certificate` changes after creation (validation error), or is
   ignoring later changes acceptable (documented best-effort)?
6. **Console:** confirm the per-gateway console may stay on the shared wildcard
   (only the gRPC data plane gets the dedicated cert).
