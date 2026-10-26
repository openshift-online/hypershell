# Investigation: Per-gateway wildcard certificates for sandbox service exposure

**Date:** 2026-09-29 (rev. 2, goal clarified)
**Status:** Investigation / design proposal - no code changed
**Goal:** Make OpenShell sandbox service exposure (ExposeService HTTP
virtual-hosting) work. Today the platform provisions one wildcard certificate at
the cluster (spoke) level, which means valid sandbox service URLs cannot be
generated, certified, or routed to the correct gateway. Hypothesis under test:
this works if each opted-in gateway gets its own wildcard certificate at
provision time, rotated by cert-manager.

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

The user's hypothesis is therefore correct but incomplete: a per-gateway wildcard
cert fixes Gap A; Gaps B and C must be fixed in the same feature or the URLs
still do not work end to end.

## 3. Recommended design: per-gateway wildcard cert + per-tenant Gateway

Opt-in at provision time only, on the Gateway resource. All new resources live in
the tenant namespace and are created by the control-plane reconciler (like the
GRPCRoute/BackendTLSPolicy today, not by the Helm chart).

### 3.1 Opt-in configuration

Extend the existing `route` JSONB field (already provision-time input, parsed by
`parseGatewayRouteConfig`):

```json
route: {
  "host": "special.example.com",
  "certificate": { "issuerRef": { "name": "letsencrypt-dns01", "kind": "ClusterIssuer" } }
}
```

Wildcards REQUIRE a DNS-01 issuer; the production ClusterIssuer
(`letsencrypt-dns01`, Route53 solver) already qualifies. HTTP-01 issuers must be
rejected at validation time.

### 3.2 What the control plane creates for an opted-in gateway

In the tenant namespace:

1. **`Certificate openshell-gateway-wildcard`** - `dnsNames:
   ["*.gw-<ns>.<spoke>.infra.hypershell.app"]` (or `*.<explicit host's parent
   domain>` when `route.host` is set), `issuerRef` from the opt-in config,
   `secretName: openshell-gateway-wildcard-tls`, `rotationPolicy: Always`,
   managed labels. cert-manager issues and rotates it from here on. DNS-01
   issuance needs no pre-existing DNS record (the issuer solves via the Route53
   API).
2. **`Gateway gw-<ns>`** (per-tenant Gateway API Gateway, class
   `openshift-default`) with one listener:
   - name `sandbox`, port 443, protocol HTTPS, TLS **Terminate**,
   - hostname `*.gw-<ns>.<spoke>.infra...`,
   - `certificateRefs` -> the per-gateway wildcard secret (same namespace, no
     ReferenceGrant),
   - `allowedRoutes.namespaces.from: Same`.
3. **`HTTPRoute openshell-gateway-sandbox`** - hostname `*.gw-<ns>.<spoke>...`,
   parentRef -> the per-tenant Gateway (sectionName `sandbox`), backendRef ->
   `openshell-gateway:8080`. The existing **BackendTLSPolicy** already targets
   that Service and re-encrypts with the internal CA, so the proxy->pod leg is
   TLS with the pod's internal cert (svc-FQDN SAN). The **Host header is
   preserved** through the proxy, which is what the gateway's virtual-hosting
   keys on. (Alternative: forward plaintext HTTP; the gateway accepts it when
   `enable_loopback_service_http=true` - default. Re-encrypt is recommended for
   consistency with the gRPC path.)
4. **DNS**: the per-tenant Gateway gets its own proxy Service/LB. Publish the
   wildcard record by setting `external-dns.kubernetes.io/hostname:
   *.gw-<ns>.<spoke>.infra...` in the Gateway's `infrastructure.annotations` -
   the exact mechanism hypershell-gitops already uses for the shared Gateway.
   On clusters without external-dns, the operator adds the record manually at
   provision time (acceptable: this is an explicit, special configuration).

### 3.3 Wiring the gateway's own knowledge (fixes Gap C)

- The reconciler injects `*.gw-<ns>.<spoke>.infra...` into `ServerDnsNames` for
  opted-in gateways, so it lands in `server_sans` and the gateway derives
  base domain `gw-<ns>.<spoke>.infra...` for exposed-service URLs.
- Independent fix (needed regardless, it is a bug today): the values builder must
  write the injected names to `certManager.serverDnsNames` (not
  `pkiInitJob.serverDnsNames`) when `HasCertManager` is true, or the chart
  should read one merged list. The internal Certificate gaining a wildcard SAN
  is harmless (it feeds URL generation and the pod's own cert; external clients
  trust the listener cert, not this one).

### 3.4 What does NOT change

- The gateway's own gRPC hostname `gw-<ns>.<spoke>...` keeps riding the shared
  Gateway + spoke wildcard (GRPCRoute, routeAddress, CLI connections,
  BackendTLSPolicy: all untouched). The single-label wildcard
  `*.gw-<ns>...` does NOT cover `gw-<ns>...` itself, so moving gRPC traffic is a
  separate decision (§4, variant "full independence").
- The console HTTPRoute stays on the shared Gateway listener.
- RBAC: `deploy/base/platform-resources/controller-rbac.yaml` already grants
  full CRUD on `gateways`, `httproutes`, and cert-manager `certificates` /
  `issuers`. No change.
- Rotation reaching the serving path: the wildcard cert is consumed by the
  Gateway **listener**, not by the gateway pod. The Gateway API implementation
  hot-reloads rotated listener secrets - production already depends on this for
  the rotating `hyp7-wildcard`. **No pod restart, no Reloader, no new reload
  requirement.** (The pod's internal cert renewal behavior is unchanged and
  orthogonal.)

### 3.5 Ownership and teardown

Certificate, Gateway, HTTPRoute, secret: all in the tenant namespace, reaped by
namespace deletion. Un-route/gateway-delete must explicitly delete the
per-tenant Gateway + Certificate + HTTPRoute (extend
`DeleteGatewayAPIResources` / health-loop teardown), so a removed gateway does
not leave a live wildcard listener.

### 3.6 Exposure/readiness

`routeAddress` for the gRPC path is unchanged. The per-tenant Gateway's
`Programmed` status should gate the feature's readiness signal (small extension
of `GatewayAPIExposure`, which already anticipates per-tenant Gateways via
`perTenantGatewayName`).

## 4. Option comparison

| | **B. Per-gateway wildcard + per-tenant Gateway (recommended, §3)** | **A. Add per-gateway listener to the shared Gateway** | **C. Pod-terminated wildcard (TLS passthrough / SNI external cert)** | **D. Upstream hashed hostnames under the one cluster wildcard (`service_routing_gateway_name`, issues #4840/#4631, hypershell #700)** |
|---|---|---|---|---|
| Fixes cert (Gap A) | Yes, per-gateway wildcard from DNS-01 issuer | Yes (listener cert) | Yes (pod presents wildcard via chart's `serverIssuerRef` SNI cert) | Yes, single cluster wildcard covers hashed labels |
| Fixes routing (Gap B) | Yes, wildcard HTTPRoute on per-tenant Gateway | Yes, but requires writing to the Argo CD-owned shared Gateway - ownership war, Argo reverts | Yes, TLSRoute passthrough on per-tenant Gateway (or Route) | Needs a NEW smart proxy that maps hash -> gateway (hypershell #700's sandbox-ingress) |
| Fixes gateway knowledge (Gap C) | Yes, wildcard in `server_sans` | Same | Same | Yes, but requires upstream fork support for the new setting |
| cert-manager rotates | Yes | Yes | Yes, but renewed cert must reach the POD - reload unverified, no restart mechanism exists | Yes |
| Argo CD conflict | None | **Fatal** | None | None |
| DNS | Per-gateway wildcard record (external-dns automatable) | Wildcard record still cluster-level | Same as B | Existing cluster wildcard only |
| Cost per opted-in gateway | One extra proxy Deployment + LB | None | Same as B | One shared proxy for all gateways |
| Upstream dependency | None | None | None (chart already supports the cert) but pod reload risk | **Requires upstream fork changes + new proxy component** |
| Per-gateway isolation | Strong (own domain, own cert, own LB) | Shared LB | Strong | Shared everything; hash collisions handled by route index |

**Recommendation: B.** It is the only option that fixes all three gaps with no
Argo CD conflict, no upstream dependency, no pod-side reload requirement, and it
directly realizes "the gateway has a specific wildcard cert for itself". Option D
is the long-term fleet-wide answer if per-gateway LBs become a cost problem, but
it needs upstream `service_routing_gateway_name` support in the
opendatahub-io/openshell fork plus a new hash-routing proxy - a much larger,
cross-repo effort. Option A is rejected outright. Option C is a viable fallback
if listener-level termination proves impossible on some platform, but it moves
cert rotation onto the unverified pod-reload path.

Variant within B ("full independence", deferred): also issue
`gw-<ns>.<spoke>...` in the per-gateway Certificate, add a second listener for
it, and move the GRPCRoute to the per-tenant Gateway. Costs one more DNS record
(`gw-<ns>...` -> per-tenant LB) and touches the main data path; not needed for
the stated goal, so recommend deferring.

## 5. Spec conflicts and required spec changes

Conflicts:

1. `openshell-gateway-routing.spec.md` / `openshell-gateway.spec.md`: "The
   control plane does NOT create Gateway resources" / "does not create or manage
   the Gateway-level TLS" - both need a carve-out for opted-in per-gateway
   wildcard exposure.
2. `openshell-gateway.spec.md` § Gateway Configuration documents `server_sans`
   as `<serverDnsNames from Gateway resource>` - must be updated for the
   wildcard injection and the `pkiInitJob`/`certManager` values-key fix.
3. `openshell-gateway.spec.md` line ~160: "This avoids per-tenant certificate
   issuance for the ingress listener" - now false for opted-in gateways; needs
   the exception documented.
4. No spec today covers sandbox service exposure at all (tracked as issues #234
   / #700) - the new spec section becomes the first.

Spec changes needed:

- `specs/platform/openshell-gateway-routing.spec.md`: new Requirement
  "Per-Gateway Sandbox Service Exposure" (opt-in `route.certificate`; per-tenant
  Gateway + wildcard Certificate + wildcard HTTPRoute; DNS/external-dns
  expectation; teardown; readiness). Amend the Shared Gateway requirement with
  the carve-out.
- `specs/platform/openshell-gateway-tls.spec.md`: new Requirement "Per-Gateway
  Wildcard Certificate" (cert-manager Certificate, DNS-01 issuer requirement,
  rotation served by the Gateway listener, wildcard SAN rules, `server_sans`
  injection).
- `specs/platform/openshell-gateway.spec.md`: `route` field table
  (`route.certificate.issuerRef.{name,kind}`), `server_sans` documentation,
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
     `RouteConfig.Certificate` (`IssuerRef{Name, Kind}`).
   - `components/control-plane/internal/gateway/validation.go` (+ tests) -
     issuerRef required/kind; reject known HTTP-01-only issuers is not possible
     statically, so instead document + validate the wildcard/domain shape.
   - `components/control-plane/internal/reconciler/reconciler.go` - no change
     needed (JSON passthrough) beyond reading the new key into `RouteConfig`.
3. **Reconciler** - `components/control-plane/internal/gateway/reconciler.go`:
   - In `reconcileGatewayAPIResources`, when `route.certificate` is set: create
     `Certificate` + per-tenant `Gateway` + wildcard `HTTPRoute` (dynamic
     client, `reconcileResource`, managed labels), wait for the wildcard secret
     (`waitForSecret`), inject the wildcard into `ServerDnsNames` before the
     Helm install (the injection currently happens pre-Helm in `Reconcile`, so
     the wildcard must be added there, keyed off the parsed route config).
   - `DeleteGatewayAPIResources` (+ health-loop teardown): delete the per-tenant
     Gateway, Certificate, and HTTPRoute.
4. **Exposure adapter** - `components/control-plane/internal/exposure/gatewayapi.go`
   (+ tests): readiness observation for the per-tenant Gateway (tenant
   namespace) when the feature is active.
5. **DNS** - `external-dns.kubernetes.io/hostname` infrastructure annotation on
   the per-tenant Gateway; document the manual fallback for non-external-dns
   clusters.
6. **Tests / e2e** - `reconciler_test.go`, `exposure/gatewayapi_test.go`,
   `helm/values_test.go`; e2e scenario in `tests/e2e/` (needs a DNS-01-capable
   issuer in the e2e cluster, or a self-signed wildcard Issuer for the test and
   a DNS-01 issuer only in production).
7. **Specs** - as listed in §5.

Explicitly not needed: RBAC changes, chart template changes (all new resources
are reconciler-owned), pod-side reload mechanisms, console changes.

## 7. Open questions for the user

1. **Issuer:** always an explicit `route.certificate.issuerRef`, or fall back to
   a cluster-configured default (new env var, e.g. `GATEWAY_SANDBOX_WILDCARD_ISSUER`)?
   Wildcards require DNS-01 - confirm `letsencrypt-dns01` is the intended issuer
   for opted-in gateways.
2. **Hostname scope:** opt-in with the standard derived host
   (`*.gw-<ns>.<spoke>...`) only, or also explicit `route.host` parents (custom
   vanity domains, e.g. `*.special.example.com`)?
3. **DNS:** is a per-gateway wildcard DNS record via external-dns acceptable on
   AWS spokes, and is a manual record acceptable elsewhere (IBM)? Without
   external-dns this needs an operator step at provision time.
4. **Cost:** one extra proxy Deployment + LB per opted-in gateway - acceptable
   for a special opt-in configuration? (If many gateways opt in, option D's
   shared hash-routing proxy becomes the better economics.)
5. **Full independence variant:** should the gateway's own gRPC hostname also
   move to the per-gateway cert eventually (deferred in this plan), or is
   sandbox-service traffic the only motivation?
6. **Update semantics:** "provision time only" - reject `route.certificate`
   changes after creation (API validation) or ignore them silently?
7. **Gap C bug:** confirm you want the `pkiInitJob`/`certManager`
   `serverDnsNames` fix landed as an independent bug fix ahead of the feature
   (recommended; it affects every cert-manager gateway's `server_sans` today).
