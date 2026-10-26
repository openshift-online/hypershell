# Investigation: Per-gateway wildcard certificates for sandbox service exposure

**Date:** 2026-09-29 (rev. 3, plan confirmed with user)
**Status:** Investigation / design proposal - no code changed
**Goal:** Make OpenShell sandbox service exposure (ExposeService HTTP
virtual-hosting) work. Today the platform provisions one wildcard certificate at
the cluster (spoke) level, which means valid sandbox service URLs cannot be
generated, certified, or routed to the correct gateway.
**Plan (user's, decisions confirmed 2026-09-29):** a different provisioning path
where a gateway opts in to a dedicated route/cert. One cert-manager Certificate
with BOTH `gw-<ns>.<spoke>.infra.hypershell.app` and
`*.<gw>.<spoke>.infra.hypershell.app` (single cert, single DNS-01 order from the
cluster's one issuer); the gateway - gRPC traffic, sandbox services, AND console
- is routed with that cert instead of the global spoke-level wildcard; sandbox
services get the `gw-<ns>.<spoke>...` `server_sans` basename so the gateway
generates unambiguous, routable URLs. See §7 for the resolved decisions.

---

## 1. How sandbox service URLs are supposed to work (upstream model)

The OpenShell gateway serves sandbox services on its **own listener** (port 8080,
the same one as gRPC) using HTTP virtual-hosting: an exposed service for sandbox
`sbx` service `svc` answers at

```
https://<sandbox>--<service>.<base-domain>
```

(one DNS label containing the `--` separator, e.g.
`mysandbox--webapp.gw-openshell-a1b2c3.hyp0-spoke0.infra.hypershell.app`).

Three upstream facts drive the design (NVIDIA OpenShell docs / issues; the
gateway binary itself is not vendored here, so behavior is cited, not read):

1. **The gateway derives its base domains from `server_sans`.** Wildcard SANs in
   `server_sans` (gateway.toml) are stripped of `*.` and used as the base domain
   for exposed-service hostnames. Without a wildcard SAN naming its external
   domain, the gateway cannot even generate correct external service URLs - it
   falls back to internal base domains.
2. **A single wildcard cert only covers one label.** A SAN of
   `*.hyp0-spoke0.infra.hypershell.app` matches
   `gw-<ns>.hyp0-spoke0.infra...` but NOT
   `<sandbox>--<service>.gw-<ns>.hyp0-spoke0.infra...` (two labels below).
3. Upstream is aware of the multi-gateway/shared-ingress ambiguity: OpenShell
   issues #4840 / #4631 propose a `service_routing_gateway_name` setting that
   hashes (gateway, workspace, sandbox, service) into one label under a single
   shared wildcard, with a routing proxy that resolves the hash to a gateway.
   That is the "one cluster wildcard forever" alternative (compared in §4).

Related local tracking: openshift-online/hypershell issues #234 ("Neither route
nor gateway-api ingress mode provisions routing for OpenShell's per-sandbox
ExposeService HTTP virtual-hosting") and #700 (shared sandbox-ingress proxy).

## 2. Why it is broken today (three independent gaps)

Current production path (Gateway API mode; verified in this repo and the
read-only hypershell-gitops checkout):

```
client ──▶ DNS *.gwlb / *.<spoke>.infra... ──▶ shared Gateway
          openshift-ingress/openshell-grpc-gateway (Argo CD-owned)
          listener grpc-<spoke>, hostname *.<spoke>.infra.hypershell.app
          wildcard cert hyp7-wildcard / hyp0-spoke0-gateway-tls (letsencrypt-dns01)
          ──▶ GRPCRoute gw-<ns>.<spoke>... (tenant ns)
          ──▶ BackendTLSPolicy re-encrypt ──▶ gateway pod :8080
```

**Gap A - Certificate.** The only cert served for tenant traffic is the spoke
wildcard `*.<spoke>.infra.hypershell.app`. It covers `gw-<ns>.<spoke>...` but not
`<sandbox>--<service>.gw-<ns>.<spoke>...`. Any sandbox service URL fails TLS
validation.

**Gap B - Routing.** The shared Gateway listener hostname
`*.<spoke>.infra.hypershell.app` also matches only one label, so a request for
`<sandbox>--<service>.gw-<ns>...` does not match any listener hostname and is not
routable at all. Even the GRPCRoute's hostname is the exact
`gw-<ns>.<spoke>...`; nothing provisions a wildcard-hostname route.

**Gap C - The gateway does not know its external domain (latent config bug).**
The reconciler injects the derived hostname into `ServerDnsNames`
(`reconciler.go` `appendDNSNameIfMissing`), but the Helm values builder writes it
to `pkiInitJob.serverDnsNames` (`helm/values.go` buildCoreValues). When
`certManager.enabled=true` (production), the chart ignores that key entirely:
both the cert-manager server `Certificate` SANs and the `server_sans` entry in
gateway.toml come from `certManager.serverDnsNames`
(`charts/openshell/templates/cert-manager-pki.yaml`,
`charts/openshell/templates/_toml.tpl` lines 114-118, `certgen.yaml` lines
111-119). `certManager.serverDnsNames` is set by the builder only in Route mode
with an external issuer (`buildIngressValues`). So in Gateway API mode the
external hostname never reaches `server_sans`, and per upstream fact 1 the
gateway cannot derive an external base domain. This should be verified against a
live production gateway (`oc get cm openshell-gateway-config -o yaml` and inspect
`server_sans`), but the code path is unambiguous.

## 3. Recommended design: dedicated per-gateway route + wildcard cert

Opt-in at provision time only, on the Gateway resource. The opted-in gateway
stops using the shared spoke-level Gateway/wildcard entirely for its data plane
and gets its own resources, all in the tenant namespace, created by the
control-plane reconciler (like the GRPCRoute/BackendTLSPolicy today, not by the
Helm chart).

### 3.1 Opt-in configuration

Extend the existing `route` JSONB field (already provision-time input, parsed by
`parseGatewayRouteConfig`). The opt-in is a bare flag - **users are not aware of
how the certificate is provisioned**; the cluster has one issuer and it issues
every cert:

```json
route: { "certificate": {} }
```

- No per-gateway issuerRef. The controller resolves the issuer from a new
  deployment env var (e.g. `GATEWAY_EXTERNAL_ISSUER` +
  `GATEWAY_EXTERNAL_ISSUER_KIND`, defaulting to `ClusterIssuer`) - mirroring how
  `ExternalCAIssuerName` works for Route mode today.
- Validation: `route.certificate` may only be set at creation; the API server
  rejects changes to it after the gateway exists (provision-time only, no update
  support). The controller fails provisioning if the env var is unset.
- Wildcards REQUIRE a DNS-01 issuer; the production ClusterIssuer
  (`letsencrypt-dns01`, Route53 solver) qualifies. The env var names it; a
  cluster configured with an HTTP-01-only issuer cannot serve opted-in gateways
  (documented prerequisite).

### 3.2 One cert, two SANs (answer: yes, same cert)

A single cert-manager `Certificate` with both names is valid and preferable:

```yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: openshell-gateway-external
  namespace: <tenant-namespace>
spec:
  secretName: openshell-gateway-external-tls
  dnsNames:
  - gw-<ns>.<spoke>.infra.hypershell.app          # the gateway's own host
  - "*.gw-<ns>.<spoke>.infra.hypershell.app"      # sandbox service hosts
  issuerRef: { name: letsencrypt-dns01, kind: ClusterIssuer }
  rotationPolicy: Always
```

- A certificate may carry an exact name and a wildcard in the same SAN list; ACME
  CAs (Let's Encrypt) issue both in one order. The wildcard SAN mandates DNS-01,
  and the exact name is in the same zone, so one DNS-01 solver covers both. One
  Secret, one issuance, one rotation cycle.
- The wildcard `*.gw-<ns>.<spoke>...` covers exactly one label below
  `gw-<ns>.<spoke>...`, and `<sandbox>--<service>` is a single DNS label, so
  every sandbox service URL is covered. (The wildcard does NOT cover
  `gw-<ns>.<spoke>...` itself - hence the exact SAN.)
- Rate-limit note: Let's Encrypt counts the registered domain
  (`hypershell.app`); each opted-in gateway is one order with two SANs. The
  per-spoke wildcards already share this budget, so this scales linearly with
  opted-in gateways. Cert-manager retries/backoffs handle rejections.

### 3.3 What the control plane creates for an opted-in gateway

In the tenant namespace:

1. **`Certificate openshell-gateway-external`** - as in §3.2, with managed labels
   (`hypershell.redhat.io/managed=true`,
   `app.kubernetes.io/managed-by=hypershell-control-plane`). cert-manager issues
   and rotates it from here on. DNS-01 issuance needs no pre-existing DNS record
   (the issuer solves via the Route53 API).
2. **`Gateway gw-<ns>`** (per-tenant Gateway API Gateway, class
   `openshift-default`) with **two listeners on port 443**, both referencing the
   same secret (Gateway API allows multiple listeners per port with distinct,
   non-overlapping hostnames; the Kind dev Gateway already uses two
   hostname-scoped listeners):
   - `grpc`: hostname `gw-<ns>.<spoke>...`, HTTPS/Terminate - carries the
     gateway's own gRPC traffic.
   - `sandbox`: hostname `*.gw-<ns>.<spoke>...`, HTTPS/Terminate - carries
     sandbox service traffic AND the per-gateway console (its host is a single
     label under `gw-<ns>.<spoke>...`, see §3.5).
   - `allowedRoutes.namespaces.from: Same` on both; the `sandbox` listener
     allows both HTTPRoute kinds.
3. **`GRPCRoute openshell-gateway`** - unchanged shape (exact hostname
   `gw-<ns>.<spoke>...`), but its **parentRef switches** from the shared Gateway
   to the per-tenant Gateway (sectionName `grpc`). BackendTLSPolicy, backend-CA
   ConfigMap, and the `grpcs://<host>:443` routeAddress are unchanged - same
   hostname, now resolved through the per-tenant Gateway's LB.
4. **`HTTPRoute openshell-gateway-sandbox`** - hostname
   `*.gw-<ns>.<spoke>...`, parentRef -> per-tenant Gateway (sectionName
   `sandbox`), backendRef -> `openshell-gateway:8080`. The existing
   **BackendTLSPolicy** already targets that Service and re-encrypts with the
   internal CA, so the proxy->pod leg is TLS with the pod's internal cert
   (svc-FQDN SAN). The **Host header is preserved** through the proxy, which is
   what the gateway's virtual-hosting keys on. (Alternative: forward plaintext
   HTTP; the gateway accepts it when `enable_loopback_service_http=true` -
   default. Re-encrypt is recommended for consistency with the gRPC path.)
5. **DNS**: the per-tenant Gateway gets its own proxy Service/LB. Publish both
   records by setting `external-dns.kubernetes.io/hostname` in the Gateway's
   `infrastructure.annotations` - the same mechanism hypershell-gitops uses for
   the shared Gateway, with both names comma-separated
   (`gw-<ns>.<spoke>...,*.<gw>...`; external-dns splits the annotation on
   commas - verify against the deployed external-dns version). On clusters
   without external-dns, the operator adds the two records manually at provision
   time (acceptable: this is an explicit, special configuration).

### 3.4 Wiring the gateway's own knowledge (fixes Gap C)

- The reconciler injects **both** `gw-<ns>.<spoke>...` and
  `*.gw-<ns>.<spoke>...` into `ServerDnsNames` for opted-in gateways, so they
  land in `server_sans`. The gateway strips the `*.` from the wildcard entry and
  derives base domain `gw-<ns>.<spoke>...`; exposed-service URLs become
  `<sandbox>--<service>.gw-<ns>.<spoke>...` - **unambiguous (unique per
  gateway), cert-valid (wildcard SAN), and routable (wildcard listener +
  HTTPRoute + DNS)**.
- Independent fix (needed regardless, it is a bug today): the values builder must
  write the injected names to `certManager.serverDnsNames` (not
  `pkiInitJob.serverDnsNames`) when `HasCertManager` is true, or the chart
  should read one merged list. The internal Certificate gaining the external
  SANs is harmless (it feeds URL generation and the pod's own cert; external
  clients trust the listener cert, not this one).

### 3.5 The console moves too

The per-gateway console also re-parents onto the per-tenant Gateway - and needs
**no cert change**: its host becomes a single DNS label under the gateway's own
domain (e.g. `console.gw-<ns>.<spoke>...`, or keeping today's `console-<ns>`
label - implementation detail), which the `*.gw-<ns>.<spoke>...` wildcard SAN
already covers. The console HTTPRoute attaches to the per-tenant Gateway's
`sandbox` (wildcard) listener; `deriveConsoleHost`, the oauth2-proxy redirect
URI, and the Keycloak console client's web origins must follow the new host.

### 3.6 What does NOT change

- Non-opted-in gateways: byte-for-byte today's behavior (shared Gateway, spoke
  wildcard, no new resources; console stays at `console-<ns>.<spoke>...`).
- RBAC: `deploy/base/platform-resources/controller-rbac.yaml` already grants
  full CRUD on `gateways`, `httproutes`, `grpcroutes`, `backendtlspolicies`,
  and cert-manager `certificates` / `issuers`. No change.
- Rotation reaching the serving path: the per-gateway cert is consumed by the
  Gateway **listeners**, not by the gateway pod. The Gateway API implementation
  hot-reloads rotated listener secrets - production already depends on this for
  the rotating `hyp7-wildcard`. **No pod restart, no Reloader, no new reload
  requirement.** (The pod's internal cert renewal behavior is unchanged and
  orthogonal.)
- mTLS semantics: `client_ca_path` stays stripped for routed gateways (the proxy
  still cannot present client certificates); the sandbox HTTPRoute rides the
  same re-encrypt model as the GRPCRoute.

### 3.7 Ownership, teardown, readiness

- Certificate, Gateway, GRPCRoute, HTTPRoute, secret: all in the tenant
  namespace, reaped by namespace deletion. Un-route/gateway-delete must
  explicitly delete the per-tenant Gateway + Certificate + both routes (extend
  `DeleteGatewayAPIResources` and the health-loop teardown / route-absence
  checks), so a removed gateway does not leave a live listener serving its
  hostname or wildcard.
- `routeAddress` format is unchanged (`grpcs://<host>:443`).
  `GatewayAPIExposure.ObserveReadiness` must read the **per-tenant Gateway in
  the tenant namespace** for opted-in gateways (the adapter already anticipates
  per-tenant Gateways via `perTenantGatewayName`; today it prefers the shared
  name when `GATEWAY_API_GATEWAY_NAME` is set, so this is a scoped change).
- Hostname safety: the existing anti-hijack validation
  (`deriveGatewayHostname`: an explicit host under the spoke base domain must be
  the tenant's own `gw-<ns>` slot) extends naturally - the opted-in gateway owns
  its whole `gw-<ns>.<spoke>...` subtree, and no other gateway's wildcard can
  overlap it (distinct second-level labels).

## 4. Option comparison

| | **B. Dedicated per-gateway route + wildcard cert (recommended, §3)** | **A. Add per-gateway listeners to the shared Gateway** | **C. Pod-terminated wildcard (TLS passthrough / SNI external cert)** | **D. Upstream hashed hostnames under the one cluster wildcard (`service_routing_gateway_name`, issues #4840/#4631, hypershell #700)** |
|---|---|---|---|---|
| Fixes cert (Gap A) | Yes, one cert with exact + wildcard SANs | Yes (listener certs) | Yes (pod presents wildcard via chart's `serverIssuerRef` SNI cert) | Yes, single cluster wildcard covers hashed labels |
| Fixes routing (Gap B) | Yes, dedicated Gateway with exact + wildcard listeners | Yes, but requires writing to the Argo CD-owned shared Gateway - ownership war, Argo reverts | Yes, TLSRoute passthrough on per-tenant Gateway (or Route) | Needs a NEW smart proxy that maps hash -> gateway (hypershell #700's sandbox-ingress) |
| Fixes gateway knowledge (Gap C) | Yes, exact + wildcard in `server_sans` | Same | Same | Yes, but requires upstream fork support for the new setting |
| cert-manager rotates | Yes | Yes | Yes, but renewed cert must reach the POD - reload unverified, no restart mechanism exists | Yes |
| Argo CD conflict | None | **Fatal** | None | None |
| DNS | Two records per opted-in gateway -> its own LB (external-dns automatable) | Wildcard record still cluster-level | Same as B | Existing cluster wildcard only |
| Cost per opted-in gateway | One extra proxy Deployment + LB | None | Same as B | One shared proxy for all gateways |
| Upstream dependency | None | None | None (chart already supports the cert) but pod reload risk | **Requires upstream fork changes + new proxy component** |
| Per-gateway isolation | Strong (own domain subtree, own cert, own LB) | Shared LB | Strong | Shared everything; hash collisions handled by route index |

**Recommendation: B** (the user's plan). It is the only option that fixes all
three gaps with no Argo CD conflict, no upstream dependency, and no pod-side
reload requirement, and it makes the opted-in gateway fully self-contained: its
gRPC hostname, its sandbox wildcard, and its cert are all its own. Option D is
the long-term fleet-wide answer if per-gateway LBs become a cost problem, but it
needs upstream `service_routing_gateway_name` support in the
opendatahub-io/openshell fork plus a new hash-routing proxy - a much larger,
cross-repo effort. Option A is rejected outright. Option C is a viable fallback
if listener-level termination proves impossible on some platform, but it moves
cert rotation onto the unverified pod-reload path.

## 5. Spec conflicts and required spec changes

Conflicts:

1. `openshell-gateway-routing.spec.md` / `openshell-gateway.spec.md`: "The
   control plane does NOT create Gateway resources" / "does not create or manage
   the Gateway-level TLS" - both need a carve-out for opted-in gateways.
2. `openshell-gateway.spec.md` § Gateway Configuration documents `server_sans`
   as `<serverDnsNames from Gateway resource>` - must be updated for the
   wildcard injection and the `pkiInitJob`/`certManager` values-key fix.
3. `openshell-gateway.spec.md` line ~160: "This avoids per-tenant certificate
   issuance for the ingress listener" - now false for opted-in gateways; needs
   the exception documented.
4. `openshell-gateway-routing.spec.md` § Gateway Route Configuration: GRPCRoute
   parentRef is specified as the shared Gateway - needs the opted-in variant.
5. No spec today covers sandbox service exposure at all (tracked as issues #234
   / #700) - the new spec section becomes the first.

Spec changes needed:

- `specs/platform/openshell-gateway-routing.spec.md`: new Requirement
  "Per-Gateway Dedicated Route and Certificate" (opt-in `route.certificate`;
  per-tenant Gateway with exact + wildcard listeners; Certificate with both
  SANs; GRPCRoute re-parenting; wildcard HTTPRoute; DNS/external-dns
  expectation; teardown; readiness). Amend the Shared Gateway requirement with
  the carve-out.
- `specs/platform/openshell-gateway-tls.spec.md`: new Requirement "Per-Gateway
  External Certificate" (cert-manager Certificate with exact + wildcard SANs,
  DNS-01 issuer requirement, rotation served by the Gateway listeners,
  `server_sans` injection).
- `specs/platform/openshell-gateway.spec.md`: `route` field table
  (`route.certificate` opt-in flag + the `GATEWAY_EXTERNAL_ISSUER` env var),
  `server_sans` documentation,
  data-model `route` JSONB description.
- `specs/platform/data-model.spec.md`: `route` JSONB column comment.
- `docs/architecture/source/ingress.md`: diagram variant for opted-in gateways.

## 6. Recommended implementation plan (files to touch)

After plan approval - nothing below is implemented yet:

1. **Bug fix first (Gap C, independent of the feature):**
   `components/control-plane/internal/helm/values.go` - write injected
   `ServerDnsNames` to `certManager.serverDnsNames` when `HasCertManager` (keep
   `pkiInitJob.serverDnsNames` for the certgen path), + `values_test.go`.
   Verify against a live gateway that `server_sans` is missing the external
   hostname today.
2. **Config plumbing:**
   - `components/control-plane/internal/gateway/config.go` -
     `RouteConfig.Certificate` (bare opt-in struct).
   - `components/control-plane/internal/gateway/validation.go` (+ tests) -
     opt-in shape; controller env var (`GATEWAY_EXTERNAL_ISSUER`) must be set
     when any gateway opts in.
   - `components/control-plane/internal/reconciler/reconciler.go` - no change
     needed (JSON passthrough) beyond reading the new key into `RouteConfig`.
   - API server: reject `route.certificate` mutations after creation
     (`components/api-server/plugins/gateways/`).
3. **Reconciler** - `components/control-plane/internal/gateway/reconciler.go`:
   - In `Reconcile` (pre-Helm SAN injection): for opted-in gateways add both
     `gw-<ns>.<spoke>...` and `*.gw-<ns>.<spoke>...` to `ServerDnsNames`.
   - In `reconcileGatewayAPIResources`: when `route.certificate` is set, create
     the `Certificate` + per-tenant `Gateway` (two listeners) + wildcard
     `HTTPRoute` (dynamic client, `reconcileResource`, managed labels), point
     the GRPCRoute parentRef at the per-tenant Gateway, and wait for the
     external secret (`waitForSecret`) before proceeding.
   - `DeleteGatewayAPIResources` (+ health-loop teardown / route-absence
     checks): delete the per-tenant Gateway, Certificate, and HTTPRoute.
4. **Exposure adapter** - `components/control-plane/internal/exposure/gatewayapi.go`
   (+ tests): readiness observation reads the per-tenant Gateway (tenant
   namespace) when the feature is active.
5. **Console** - `components/control-plane/internal/gateway/console.go`:
   `deriveConsoleHost` returns the label under `gw-<ns>.<spoke>...` for opted-in
   gateways; console HTTPRoute parents onto the per-tenant Gateway's `sandbox`
   listener; redirect URI / Keycloak client follow.
6. **DNS** - comma-separated `external-dns.kubernetes.io/hostname`
   infrastructure annotation on the per-tenant Gateway; verify external-dns
   splits it; document the manual fallback for non-external-dns clusters.
7. **Tests / e2e** - `reconciler_test.go`, `exposure/gatewayapi_test.go`,
   `helm/values_test.go`; e2e scenario in `tests/e2e/` (needs a DNS-01-capable
   issuer in the e2e cluster, or a self-signed wildcard Issuer for the test and
   a DNS-01 issuer only in production).
8. **Specs** - as listed in §5.

Explicitly not needed: RBAC changes, chart template changes (all new resources
are reconciler-owned), pod-side reload mechanisms, console changes.

## 7. Resolved decisions (user, 2026-09-29)

1. **Hostname label:** keep the auto-derived `gw-<ns>` (the gateway's unique
   `openshell-<hex>` namespace). The namespace is hypershell's collision-proof
   per-gateway ID; the hostname is derived at provision time and is not known
   (nor needs to be known) in advance. One new Certificate per opted-in
   gateway.
2. **Issuer:** cluster-configured only. Users are not aware of how the cert is
   provisioned; one issuer per cluster issues every cert (new controller env
   var, e.g. `GATEWAY_EXTERNAL_ISSUER`). No issuerRef in the API.
3. **Hostname scope:** derived host only. Explicit `route.host` keeps today's
   shared-Gateway semantics; no vanity domains in this feature.
4. **DNS:** external-dns automation (both records via the per-tenant Gateway's
   infrastructure annotation); manual records are the documented fallback on
   clusters without external-dns.
5. **Cost:** one extra proxy Deployment + LB per opted-in gateway is
   acceptable. Revisit with the shared hash-routing proxy (option D) only if
   the opt-in becomes mainstream.
6. **Console:** moves onto the per-tenant Gateway too, under the existing
   `*.gw-<ns>...` wildcard SAN - no cert change, only host derivation,
   HTTPRoute parenting, redirect URI, and Keycloak client updates.
7. **Update policy:** the API rejects `route.certificate` changes after
   creation (provision-time only). Changing cert mode = delete and re-provision.
8. **Bug fix ordering:** the `pkiInitJob`/`certManager` `serverDnsNames` fix
   (Gap C) lands as an independent bug fix ahead of the feature.

Remaining implementation-time verifications (not user decisions):

- external-dns splits the comma-separated hostname annotation on the deployed
  version.
- Live-confirm Gap C on a production gateway's `openshell-gateway-config`.
- Gateway API implementation serves rotated listener secrets without restart
  (already the production assumption for the shared wildcard).
