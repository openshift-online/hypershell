# Investigation: dedicated-route console certificate mismatch (hyp2-spoke1)

**Date:** 2026-10-30
**Status:** Root cause fully confirmed (external evidence + code review + spoke read
access, 2026-10-30 ~17:00Z); control-plane fixes committed on
`fix/dedicated-route-console-cert`; gitops IAM fix committed on
`fix/acme-per-gateway-dedicated-route` in hypershell-gitops. Remediation is
apply-and-wait: see §6.
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
| `gw-openshell-e0ccb89a08d0598f.hyp2-spoke1.infra.hypershell.app` (the published `route_address`) | same LB | same shared spoke wildcard | plain HTTP: 404; gRPC (`openshell sandbox list`): trailers-only `grpc-status: 12` UNIMPLEMENTED -- the shared Gateway has no route for the host after the (aborted) re-parent | 
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

1. **Provisioning took the dedicated path and is stuck on the external
certificate.** Confirmed on the spoke (read-only, 2026-10-30 ~17:00Z): the
`openshell-gateway-external` Certificate **exists** with `Ready=False`, its
CertificateRequest `openshell-gateway-external-1` created, the ACME Order and
both DNS-01 Challenges **pending** for 96 minutes with reason:

   ```
   failed to change Route 53 record set: operation error Route 53:
   ChangeResourceRecordSets ... StatusCode: 403 ... AccessDenied: User:
   arn:aws:iam::451971655738:user/hypershell/hypc-ibm-03-cert-manager-dns01 is
   not authorized to perform: route53:ChangeResourceRecordSets on resource:
   arn:aws:route53:::hostedzone/Z03431841SAX65X5TEJTB because no identity-based
   policy allows the route53:ChangeResourceRecordSets action
   ```

   (cert-manager logs, ns `cert-manager`). The gitops IAM policy for the
   per-cluster DNS-01 user (`clusters/hypc-ibm-03/1-terraform/route53-acme.tf`)
allow-lists TXT records at `_acme-challenge.<hub label>` and
   `_acme-challenge.<gateway domain>` -- the shared-wildcard challenge names it
   was built for. The dedicated route's challenges live at
   `_acme-challenge.gw-<ns>.<gateway domain>`, two labels deeper, and are denied.
   The controller is in its 30s reconcile-retry loop over exactly this
   (`wait for external TLS secret ... the dedicated gateway certificate was not
   issued in time`), so the per-tenant Gateway, the re-parented GRPCRoute (the
   GRPCRoute does not exist at all -- the dedicated path aborts before creating
   it), and the sandbox HTTPRoute were never created. The console exposure on
   the shared Gateway is the health loop's doing -- point 3 below -- not a
   shared-path provisioning decision.
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

**Gitops fix (separate repo, branch `fix/acme-per-gateway-dedicated-route` in
hypershell-gitops, commit "fix(dns): allow per-gateway dedicated-route ACME
challenges in Route53 DNS-01 IAM"):** the cert-manager DNS-01 IAM identities
gain one StringLike wildcard per gateway domain,
`_acme-challenge.gw-*.<gateway domain>`, on every IBM ROKS cluster
(`route53-acme.tf`; hypc-ibm-infra already allows `_acme-challenge.*`) and the
AWS clusters (`identities.tf`); the hysh-aws-01 IRSA role is zone-scoped
without name conditions and needs nothing; the external-dns identities already
cover per-gateway names (`*.<domain>` StringLike matches across labels). The
`topology.tftest.hcl` assertions pin the exact lists and gain the same entries.

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
  the controller domain. The OpenShift Gateway API implementation propagates
  `spec.infrastructure` annotations to the generated Service (proven by the
  shared Gateway's TXT record naming
  `service/openshift-ingress/openshell-grpc-gateway-openshift-default`), and
  external-dns watches Services cluster-wide, so records should appear
  automatically once the per-tenant Gateway exists and its Envoy Service gets
  an address. Verify after the IAM fix applies (§5).
- `waitForSecret` allows only 60s for DNS-01 issuance; first issuance of a
  two-SAN order can exceed that. This only *delays* convergence -- the
  controller's 30s retry loop re-enters the pass and finds the Secret already
  present once cert-manager finishes -- but a longer window would avoid a
  guaranteed first-pass failure (and the Failed-ish phase flapping) for every
  new dedicated gateway.

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

## 6. Resolution and remediation (spoke access obtained 2026-10-30 ~17:00Z)

All §6 open questions from the first pass are answered:

- **Spoke read access**: obtained. `kubectl get gateway -A` shows only the
  shared `openshell-grpc-gateway` (Programmed, address
  `e10672c3-us-east.lb.appdomain.cloud`); the per-tenant
  `gw-openshell-e0ccb89a08d0598f` does not exist. The tenant namespace has the
  gateway and console Deployments/Services, the four gateway chart
  Certificates (all Ready except `openshell-gateway-external`), the console
  HTTPRoute (shared host, shared-Gateway parent, Accepted), and **no
  GRPCRoute**. The controller (ns `hyp2-spoke1`) logs the 30s retry loop over
  the external-TLS wait. The Certificate/Order/Challenge status and the
  cert-manager AccessDenied are quoted in §2.1.
- **The "no per-gateway DNS records" observation stands**: every lookup that
  appeared to resolve `gw-<ns>...` (and even `foo--bar.gw-<ns>...`) is the
  shared `*.hyp2-spoke1` wildcard synthesizing answers at any depth; the
  external-dns TXT probes are shadowed the same way, so their absence is not
  evidence of manual records. No cleanup of stray per-gateway DNS records is
  needed.

**Remediation is apply-and-wait; no gateway delete/re-create is needed** (the
earlier suggestion is superseded):

1. Merge and apply the gitops IAM fix
   (`fix/acme-per-gateway-dedicated-route`). Terraform updates the inline IAM
   user policy in place; no key rotation, no Secret churn.
2. cert-manager retries the pending challenge with backoff. If the Challenge
   does not present within a few minutes of the policy applying, delete the
   stuck ACME Order (or the CertificateRequest) to force a fresh one -- a
   one-line live action for the operator.
3. The controller's existing 30s retry loop then completes the dedicated path
   on its own: Secret appears (possibly on the second pass -- see §4's 60s
   window note), per-tenant Gateway is created, Istio's automated deployment
   brings up its Envoy + LoadBalancer Service carrying the external-dns
   annotation, external-dns publishes `gw-<ns>` and `*.gw-<ns>` pointing at
   the per-tenant LB (a different `*.lb.appdomain.cloud` name than the shared
   one), and the GRPCRoute re-parents.
4. Deploy this branch's controller build: the console self-heal re-parents the
   console onto the per-tenant Gateway's `sandbox` listener at
   `console-<ns>.gw-<ns>...`, and `console_address` is only published once the
   exposure actually serves that host. Verify per §5.
- DNS TTL / Route53 multi-depth wildcard behavior is what turns "no record"
  into "silently wrong LB": consider a guard or documentation note
  that per-gateway names must never resolve through the spoke wildcard.
