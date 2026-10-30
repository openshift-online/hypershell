# Investigation: dedicated-route console certificate mismatch (hyp2-spoke1)

**Date:** 2026-10-30
**Status:** Root cause identified from external evidence + code review; control-plane
fixes committed on `fix/dedicated-route-console-cert`. Live-gateway remediation and
cluster-side confirmation still need read access to the spoke (see §6).
**Reported by:** user -- a gateway created with `--dedicated-route` does not serve its
console with the per-gateway certificate.
**Affected gateway:** `jhjaggars-dedicated-route-test`
(`3LPrPHFcZByVi1WGTjpqA2X92s8`), namespace `openshell-e0ccb89a08d0598f`, cluster
`hyp2-spoke1` (hypc-ibm-03), created 2026-10-30T13:21:57Z, `route` =
`{"enabled": true, "certificate": {}}`, `dedicated_route: true`, phase Running.

## 1. What is served where (all observed read-only, 2026-10-30 ~14:00Z)

| Host | Resolves to (DNS) | Certificate presented | HTTP |
|---|---|---|---|
| `console-openshell-e0ccb89a08d0598f.gw-openshell-e0ccb89a08d0598f.hyp2-spoke1.infra.hypershell.app` (the published `console_address`) | `e10672c3-us-east.lb.appdomain.cloud` (150.239.84.166, 150.239.83.1) | `CN=*.hyp2-spoke1.infra.hypershell.app` (Let's Encrypt YR1, issued Oct 7) -- **name mismatch**, curl: `SSL: no alternative certificate subject name matches target hostname` | -- |
| `gw-openshell-e0ccb89a08d0598f.hyp2-spoke1.infra.hypershell.app` (the published `route_address`) | same LB | same shared spoke wildcard | HTTP/2 404 (no route claims the host on the shared Gateway) |
| `xyz123.gw-openshell-e0ccb89a08d0598f...` (any label under the per-gateway wildcard) | same LB | same shared spoke wildcard | -- |
| `console-openshell-e0ccb89a08d0598f.hyp2-spoke1.infra.hypershell.app` (**shared-form** console host) | same LB | shared spoke wildcard (valid for this host) | **302 → Keycloak**; the console (oauth2-proxy) is alive here, with `redirect_uri` = the shared host |
| `a.b.hyp2-spoke1.infra.hypershell.app` (unknown SNI) | same LB | shared spoke wildcard | -- |

Key DNS facts (Route53 zone for `hyp2-spoke1.infra.hypershell.app`):

- The **only** record covering any of this is the shared wildcard
  `*.hyp2-spoke1.infra.hypershell.app` → `e10672c3-us-east.lb.appdomain.cloud`,
  which is external-dns-owned from the **shared Gateway's** generated Service
  (`service/openshift-ingress/openshell-grpc-gateway-openshell-default`, TXT
  heritage record).
- **No per-gateway records exist.** external-dns's TXT registry has no
  `_external-dns.*` ownership records for `gw-<ns>...` or `*.gw-<ns>...`, so the
  per-tenant Gateway's infrastructure annotation never became DNS records (or the
  per-tenant Gateway/its LoadBalancer Service does not exist).
- Route53 wildcard records match at **any depth**, so
  `console-<ns>.gw-<ns>.hyp2-spoke1...` resolves through the *shared* wildcard to
  the **shared Gateway's** envoy. That envoy has a single listener
  `*.hyp2-spoke1.infra.hypershell.app`; the console host (two labels below the
  spoke domain) matches no listener, so the envoy serves its fallback certificate --
  the shared spoke wildcard. Hence the browser's name-mismatch error.

Certificate Transparency (crt.sh, identity search -- validated by finding the shared
wildcard): **no certificate has ever been issued** for
`gw-openshell-e0ccb89a08d0598f.hyp2-spoke1.infra.hypershell.app` or
`*.gw-openshell-e0ccb89a08d0598f...`. The spoke's `GATEWAY_EXTERNAL_ISSUER`
(`letsencrypt-dns01`, gitops #1706) is the production Let's Encrypt ACME server,
which always logs to CT -- so the per-gateway Certificate either was never created
or never issued.

## 2. Root cause

The gateway is in a **half-migrated state**: its *published addresses* follow the
dedicated-route opt-in, but its *actual exposure* is on the shared path.

1. **Provisioning built the shared path.** The console is demonstrably exposed at
   the shared host on the shared Gateway (the 302/Keycloak redirect above proves
   the oauth2-proxy, its `OAUTH2_PROXY_REDIRECT_URL`, and the Keycloak console
   client all use `console-<ns>.<spoke>...`), and no per-gateway Certificate /
   per-tenant Gateway / DNS records exist. A controller that parses
   `route.certificate` (#790, merged Oct 27; image revs ≥ `0231ad5b` promoted to
   `env/hyp2-spoke1` by Oct 29-30) would have taken the dedicated path and failed
   provisioning if the certificate did not issue -- the gateway instead reached
   Running with every condition Complete. The exact in-cluster reason (controller
   version at provision time, a stuck Certificate, or an unconverged
   `observed_generation(0) != generation(2)` that no event ever re-drove) needs
   spoke access; see §6.
2. **The health loop then published the dedicated URL anyway.**
   `syncConsoleAddress` (reconciler.go) derives the console URL from the parsed
   `route.certificate` -- correctly -- but its readiness gate
   (`ConsoleExposureReady`) only checked that the console HTTPRoute named
   `openshell-console` was *Accepted by some parent*. The shared-host exposure is
   Accepted (by the shared Gateway), so the health loop published
   `https://console-<ns>.gw-<ns>...` -- a URL that resolves (via Route53
   multi-depth wildcard matching) to the shared Gateway, which has no listener
   for it and presents the shared spoke wildcard: **the user's cert error**.
3. **The one path that could have converged the console makes it worse.** The
   health loop's console self-heal (`selfHealConsole`, health.go) called
   `ReconcileConsole` with `NamespaceConfig{Name: namespace}` -- dropping the
   gateway's parsed route config. For a dedicated gateway whose console is
   observed not servable, self-heal would re-derive the *shared* host, re-parent
   the console HTTPRoute onto the *shared* Gateway, and re-point the Keycloak
   client redirect and oauth2-proxy env at the shared URL -- actively deforming a
   correctly-provisioned dedicated console while `console_address` keeps
   advertising the dedicated host.

So even a *correctly* provisioned dedicated gateway is one console-readiness blip
away from the same broken state: self-heal re-parents the console onto the shared
ingress, the exposure stays Accepted (shared host), and the address publisher
keeps advertising the dedicated host.

## 3. Fix (this branch)

Three changes, all in the control plane:

1. **`internal/gateway/console.go` -- `ConsoleExposureReady` now verifies the
   exposure serves the derived console host.** It takes the gateway's
   `NamespaceConfig` (route config included), derives the expected host with
   `deriveConsoleHost` (the single source of truth the address publisher uses),
   and requires: the HTTPRoute's `spec.hostnames` contain that host; its
   `parentRefs` target the serving Gateway (per-tenant `gw-<ns>` / `sandbox`
   listener for dedicated gateways, the shared Gateway / HTTP listener
   otherwise); or, in Route mode, `spec.host` equals the derived host. A mismatch
   is reported not-ready with an actionable reason.
2. **`internal/reconciler/reconciler.go` -- `syncConsoleAddress` publishes only a
   served URL.** It builds one `nsConfig` (with the new shared
   `consoleRouteConfig` helper) for both the URL derivation and the exposure
   check, so the published `console_address` can never point at a host the
   exposure does not serve. A mismatch retracts the address (hiding the console
   button) *and* drives the self-heal that converges the exposure.
3. **`internal/reconciler/health.go` -- `selfHealConsole` keeps the route
   config.** It passes the parsed route document through
   `consoleRouteConfig(gw)`, so a dedicated gateway's console self-heal
   re-parents onto the per-tenant Gateway's `sandbox` listener with the
   dedicated host and re-points the Keycloak client / oauth2-proxy redirect
   accordingly -- never onto the shared ingress.

Spec: `OSG-CON-09` (openshell-gateway-console.spec.md) now requires the exposure
to serve the derived host, with new scenario `OSG-CON-09.02`.

With these, after deploy: the affected gateway's `console_address` is retracted
(honest: no dead link, no cert error), and its console self-heal converges the
exposure onto the dedicated host. Because the per-tenant Gateway / Certificate /
DNS records for this gateway were never created, full service on the dedicated
route additionally requires the gateway to be (re-)provisioned on the dedicated
path -- see §6.

## 4. What is NOT fixed here (needs decisions / cluster access)

- The provisioning-time skew itself: nothing in this branch forces an
  unconverged Running gateway (`observed_generation < generation`) to re-provision
  on controller upgrade. The convergence gate would re-provision it on any
  gateway event, but no event has evidently fired since 13:52 (row `updated_at`
  is frozen at 13:52:18Z). Whether to force re-provision of unconverged Running
  gateways at seed time (`forceSeedRecovery`) is a behavioral change worth its
  own discussion.
- external-dns scope on the spokes publishes only LoadBalancer *Services* under
  the controller domain. Whether the per-tenant Gateway's generated Service (the
  OpenShift Gateway API implementation propagates `spec.infrastructure`
  annotations to it, as proven by the shared Gateway's TXT record) is annotated
  and LoadBalancer-typed in the tenant namespace needs spoke access to confirm;
  if it is, records appear automatically once the per-tenant Gateway exists.
- `waitForSecret` allows only 60s for DNS-01 issuance; first issuance of a
  two-SAN order can exceed that, which would fail provisioning (by design, but
  worth a longer window if dedicated gateways flap at provision time).

## 5. Verification after deploy

1. Deploy the controller to a spoke; for the affected gateway expect
   `console_address` to become empty (retracted) while the exposure serves the
   shared host, then the console HTTPRoute to be re-parented to
   `gw-<ns>`/`sandbox` with host `console-<ns>.gw-<ns>...`.
2. Once the gateway is re-provisioned on the dedicated path (delete/re-create,
   or a forced reconcile), verify externally:
   - `dig +short gw-<ns>.<spoke>.infra.hypershell.app` and
     `dig +short '*.gw-<ns>...'` point at the per-tenant Gateway's own LB (a
     **different** `*.lb.appdomain.cloud` hostname than the shared one), and
     `_external-dns.cname.gw-<ns>...` / `_external-dns.cname.wildcard.gw-<ns>...`
     TXT records exist;
   - `openssl s_client -connect <console-host>:443 -servername <console-host>`
     shows the per-gateway certificate (SANs: `gw-<ns>...` and `*.gw-<ns>...`);
   - crt.sh finds the new certificate.
3. Negative check: a non-dedicated gateway keeps
   `console-<ns>.<spoke>...` and the shared wildcard (valid for that host).

## 6. Open questions for the user / follow-ups

- **Spoke read access**: the IBM contexts on this host are expired. To confirm
  the in-cluster state (does `gw-openshell-e0ccb89a08d0598f` exist? does the
  `openshell-gateway-external` Certificate exist and what is its status? what
  does the console HTTPRoute actually reference? what does the controller log
  say happened at 13:21–13:52?), run `ibmcloud login --sso` +
  `ibmcloud ks cluster config` for hypc-ibm-03, or run these read-only commands:
  ```
  oc -n openshell-e0ccb89a08d0598f get gateway,gw,httproute,grpcroute,certificate,deploy,svc
  oc -n openshell-e0ccb89a08d0598f get certificate openshell-gateway-external -o yaml
  oc -n openshell-e0ccb89a08d0598f get httproute openshell-console -o yaml
  oc -n openshell-e0ccb89a08d0598f get svc | grep gw-
  ```
- **Live gateway remediation** (needs a user decision; production, and
  `route.certificate` is create-only): delete and re-create the gateway with
  `--dedicated-route` on a controller that includes #790/#815 and
  `GATEWAY_EXTERNAL_ISSUER` (all confirmed present in `env/hyp2-spoke1` as of
  Oct 30 11:20), and verify per §5. Alternatively force a reconcile
  (any spec change bumps generation; the convergence gate re-provisions).
- DNS TTL / Route53 multi-depth wildcard behavior is what turns "no record"
  into "silently wrong LB": consider a guard or documentation note
  that per-gateway names must never resolve through the spoke wildcard.
