# Keycloak readiness-ordering in the ephemeral OpenShift PR environment

**Date:** 2026-10-29
**Status:** Research / recommendations only (no fix shipped)
**Branch:** `research-keycloak-readiness-ordering`
**Scope:** `E2E / OpenShift` ephemeral PR environment flakiness
(`hypershell-ci-pr-<n>`), traced to dependents not robustly waiting for Keycloak.

This document confirms the root cause with exact code and manifest locations,
enumerates concrete fix options across three layers with tradeoffs, gives a
primary recommendation, and lists verification steps plus blast radius. It ships
no code change. Any prototype patch is a draft for review only.

---

## TL;DR

The ephemeral OpenShift PR environment has no durable ordering that holds
Keycloak-dependent workloads until Keycloak is serving JWKS. The api-server
enforces JWT by loading Keycloak's signing keys once at startup and, on failure,
calls `os.Exit(1)` (via the upstream rh-trex-ai framework). On a slow Keycloak
bring-up the api-server crash-loops; `CrashLoopBackOff` then grows its restart
backoff past the deploy's rollout-wait window, so the environment can fail to
settle in the allotted time even though it self-heals eventually.

The Kind overlay does not hit this because it guards the same ordering three
ways (an early Keycloak apply, a per-pod `wait-for-keycloak` initContainer on
the api-server and web-console, and an explicit post-Keycloak rollout restart).
The OpenShift overlay has none of these; it relies only on `oc rollout status`
ordering on a slower, image-re-pulling shared cluster.

**Primary recommendation:** port the already-proven Kind `wait-for-keycloak`
initContainer to the OpenShift overlay for the api-server and web-console. It is
the highest-leverage, lowest-risk change, scoped to `deploy/openshift/`, and it
eliminates the crash-loop at its source. Pair it with a bounded poll in the e2e
secret-read helper and, as a slower follow-up, an upstream change so the
api-server retries JWKS load instead of hard-exiting.

---

## Root cause, confirmed

### Layer A - the api-server hard-fails on JWKS load (no startup retry)

The api-server enables JWT in both the Kind and OpenShift overlays and loads
Keycloak's JWKS at startup. JWT handler construction is a hard gate:

- `deploy/openshift/kustomization.yaml:166` adds `--enable-jwt=true`;
  `:177` adds `--jwk-cert-url=$(JWK_CERT_URL)`;
  `:135-136` sets `API_ENV=development_oidc` (the environment that leaves JWT
  driven by flags, so JWKS loading actually takes effect).
- `JWK_CERT_URL` resolves to the in-cluster Keycloak JWKS endpoint. The driver
  writes it into the `hypershell-api-config` Secret:
  `scripts/cluster/drivers/openshift.sh:575`
  (`--from-literal=api-service.jwkCertUrl="${kc_svc}/realms/hypershell/protocol/openid-connect/certs"`,
  where `kc_svc=http://keycloak-service.${OPENSHIFT_KEYCLOAK_NAMESPACE}.svc.cluster.local:8080`,
  `openshift.sh:561`).

The hard-fail itself lives in the pinned upstream framework
`github.com/openshift-online/rh-trex-ai/components/api-server`
(pinned at commit `8f11b55b73f2` in `components/api-server/go.mod:14` and
`scripts/rh-trex-ai.ref`). The exact pinned commit was not in the local module
cache; the behavior below was read from the nearest cached versions and is
identical across every cached version from 2026-08 through 2026-10:

- `pkg/server/apiserver.go` builds the JWT handler and wraps the result in
  `Check(err, "Unable to create JWT authentication handler")`
  (line ~60 in `v0.0.0-20261021105531-fc88cf69871f`; line ~59 in the
  `20261007` builds).
- `pkg/server/server.go:32` - `func Check(err error, msg string)` logs the
  error and calls `os.Exit(1)`. There is no retry or backoff here.
- `pkg/auth/middleware.go` - `Build()` (line ~84) calls `loadKeys()` exactly
  once synchronously; `loadKeys()` (line ~212) fetches each configured URL
  (`loadKeysFromURLInto`) and, if nothing loads, logs
  `Failed to load JWT keys from URL ...` (line ~232) and returns an error, which
  propagates out of `Build()` into the `Check` above.

Crucially, the framework DOES have runtime resilience that only arms AFTER the
initial `Build()` succeeds: a `refreshKeysLoop()` goroutine (`middleware.go:268`)
refreshes hourly, and an on-demand refresh on an unknown `kid`
(`middleware.go:167-170`). The gap is purely at startup: the very first load is
a single attempt whose failure terminates the process.

Net effect: if Keycloak is not yet serving JWKS when the api-server container
starts, the process exits, the pod enters `CrashLoopBackOff`, and Kubernetes
applies an exponentially growing restart delay (10s, 20s, 40s, ... capped at
5 min). Even once Keycloak becomes Ready, the api-server may stay down for the
remainder of its current backoff interval.

### Layer A ordering on OpenShift (why the crash-loop outlasts the wait)

`make openshift-up` (`Makefile` -> `scripts/cluster/up.sh` ->
`scripts/cluster/drivers/openshift.sh` `cluster_up()`):

1. `apply_overlay()` (`openshift.sh:740-762`) applies Keycloak and then the
   HyperShell app overlay back-to-back, with **no readiness wait between them**.
   The api-server pod is therefore created at essentially the same time as
   Keycloak, and starts as soon as its `migrate` initContainer finishes.
2. `configure_oidc_from_routes()` (`openshift.sh:825`) then runs
   `wait_for_keycloak` (`:827`), patches Keycloak `KC_HOSTNAME` from the Route
   host (`:855-857`, which restarts Keycloak a second time), and sets
   `JWT_ISSUER` on the api-server via `oc set env` (`:892-894`, which triggers an
   api-server rollout).
3. `wait_for_deployments()` (`openshift.sh:899`) waits again for Keycloak
   (`:906`, `wait_for_keycloak` -> `oc rollout status`, 600s timeout at `:822`),
   then `wait_for_named_rollout hypershell-api-server` (`:912`) with the
   **default 180s timeout** (`:808`, `timeout="${3:-180s}"`).

So the only thing that pulls the api-server onto a healthy Keycloak is the
`JWT_ISSUER` `oc set env` rollout. But Keycloak is restarted by the
`KC_HOSTNAME` patch at roughly the same moment, and the shared cluster may be
autoscaling a node for Keycloak (the 600s timeout exists precisely for this,
`openshift.sh:819-822`). If Keycloak takes longer than ~180s to come back, the
api-server rollout started in step 2 crash-loops, and its growing backoff can
exceed the 180s `hypershell-api-server` rollout window in step 3. That makes
`wait_for_named_rollout` fail (`openshift.sh:810-813` -> `exit 1`), which fails
`make openshift-up`, which fails the Deploy job. For an unretained PR the env is
then torn down and the suite never runs; this is the "api-server
CrashLoopBackOff" failure mode (observed mode 1) turning into a hard deploy
failure.

A third Keycloak restart can follow in the Deploy action itself:
`.github/actions/deploy-pr-environment/action.yml:125-141` ("Recycle Keycloak
when GitHub OAuth secret changes") patches the Keycloak Deployment and runs
`oc rollout status deploy/keycloak --timeout=600s` after `make openshift-up`.
The api-server survives this one because it already holds cached keys, but it
widens the overall window in which Keycloak is in flux.

### Layer B - the e2e client-secret read is single-shot (fails fast)

The OpenShift suite reads the confidential `hypershell-e2e` client secret before
running:

- `.github/workflows/e2e.yml:1003` -
  `secret="$(bash scripts/ci/read-e2e-client-secret.sh)"`, then exports it as
  `E2E_OIDC_SA_CLIENT_SECRET`.
- `scripts/ci/read-e2e-client-secret.sh:35-45` - a single
  `oc get secret hypershell-e2e-client -n <ns>-keycloak -o jsonpath=... || true`;
  if the value is empty it prints
  `ERROR: could not read e2e-client-secret from Secret hypershell-e2e-client in <ns>-keycloak`
  and `exit 1`. There is no poll or retry.

In the normal open/synchronize flow this secret exists by read time:
`ensure-e2e-client-secret.sh` creates it (generated with `openssl`, idempotent,
`scripts/ci/ensure-e2e-client-secret.sh:35-40`) inside the Deploy job
(`deploy-pr-environment/action.yml:102-117`) before `make openshift-up`, and the
suite job `needs: deploy` (`e2e.yml:923`). The fast read fails when the job runs
against an environment whose secret is absent or not yet populated at read time.
The most concrete trigger is **re-running only the `E2E / OpenShift` job** (GitHub
"re-run failed jobs") after a prior unretained cycle already tore the namespace
group (and its secret) down. `/pr-extend` does not cause this: it redeploys
through the same Deploy action (which re-ensures the secret) and runs no suite
(`pr-environment-commands.yml` has `resolve`/`plan-images`/`deploy`/`destroy`
jobs and no e2e job), and the shared `pr-env-<n>` concurrency group with
`cancel-in-progress: false` keeps an extend redeploy from overlapping a running
suite. Either way, the ~43s abort (observed mode 2) is a single `oc get` against
a missing secret, not a wait that timed out.

### Layer C - gateway provisioning stalls are downstream of A

Gateway provisioning (observed mode 3, `P2.1-sandbox-lifecycle` timing out at
~5 min with gateway pods `0/1`) is a consequence, not a separate root cause: the
control plane provisions gateways by calling the api-server over gRPC with a JWT,
and every hypershell RPC requires auth (`--auth-bypass-methods` exempts only
health and reflection). While the api-server is crash-looping on JWKS, those
calls cannot succeed, so gateways cannot provision. Once the api-server settles,
provisioning proceeds. No control-plane or gateway code change is implicated.

### What is NOT the cause

- Not secret rotation: the rotator CronJobs are monthly and were idle; the
  `hypershell-e2e-client` secret is generated per environment, not rotated
  mid-run.
- Not product code: the stack self-heals to `1/1 Running` with no code change
  once Keycloak is up.
- Keycloak itself is correctly probed: `deploy/base/keycloak/keycloak.yaml`
  defines `startupProbe` `/health/started` (`:158-163`, 30 x 5s), `readinessProbe`
  `/health/ready` (`:164-168`), and `livenessProbe` `/health/live` (`:169-173`)
  on management port 9000. The problem is that dependents do not wait on that
  readiness, not that Keycloak lacks a readiness signal.

---

## Why `E2E / Kind` stays green

Kind guards the same Keycloak -> dependent ordering three times; OpenShift guards
it once (rollout-status ordering) on a slower cluster:

| Guard | Kind | OpenShift |
|-------|------|-----------|
| Keycloak applied early, boots during infra setup | Yes (`scripts/kind/up.sh:73-82`; `deploy/kind/keycloak/kustomization.yaml`) | No - applied back-to-back with the app overlay (`openshift.sh:740-762`) |
| Per-pod `wait-for-keycloak` initContainer on api-server and web-console | Yes (`deploy/kind/kustomization.yaml:244-251` -> `deploy/kind/wait-for-keycloak.patch.yaml`, polls JWKS 60 x 5s before the container starts) | **No** (only the `migrate` initContainer; `deploy/base/applications/api-server.yaml:25`) |
| Explicit post-Keycloak rollout restart of the api-server | Yes (`scripts/kind/up.sh:836-846`, comment: "If it started before Keycloak was serving keys it is stuck in CrashLoopBackoff; restart it now") | No equivalent; relies on the `JWT_ISSUER` `oc set env` rollout (`openshift.sh:892-894`) |
| Image pulls | Local single-node, images built/loaded (effectively instant) | `imagePullPolicy: Always` forced on the three components (`deploy/openshift/kustomization.yaml:205-234`), so every rollout re-pulls, widening the pre-JWKS window |
| Cluster scheduling | Single node, no autoscaler | Shared multi-node; Keycloak may wait minutes for the autoscaler (`openshift.sh:819-822`) |

Readiness-probe definitions and the use of `oc rollout status` (rather than
`wait --for=condition=available`) are the same on both. The difference is purely
the extra Kind-only ordering guards on a faster substrate. So the same latent
weakness exists on Kind, but Kind never lets the api-server container start
before Keycloak serves JWKS, and even if it did it would restart it explicitly;
OpenShift does neither.

---

## Fix options

### (a) api-server resilience

| Option | What | Tradeoffs |
|--------|------|-----------|
| **a1. Retry/backoff the initial JWKS load (upstream)** | In rh-trex-ai `middleware.go` `Build()`, wrap the first `loadKeys()` in a bounded retry-with-backoff instead of returning an error that `Check` turns into `os.Exit(1)`. The hourly refresh loop and on-unknown-kid refresh already exist; this extends the same resilience to startup. | Correct, root-level fix that would make every deployment robust (Kind, OpenShift, production). But it lives in a shared upstream repo pinned by commit; changing it means an upstream PR plus a pin bump, coordinated against other consumers. Slowest path, widest blast radius. Must keep fail-closed semantics (never serve with zero keys; retry then exit, do not start unauthenticated). |
| **a2. Treat Keycloak as a startup dependency at the pod level (initContainer)** | Do not touch the framework; gate the container start with an initContainer that waits for the JWKS endpoint. This is exactly what Kind already does. | No upstream change; declarative; reuses tested machinery. Does not fix the framework for other operators, but fully removes the crash-loop in this environment. See the primary recommendation. |
| **a3. Longer startup grace (startupProbe)** | Increase the api-server `startupProbe` budget. | Does **not** help: the process calls `os.Exit(1)`, so the container dies before the probe matters. A longer probe cannot keep a terminated process alive. Rejected. |

### (b) e2e harness secret read

| Option | What | Tradeoffs |
|--------|------|-----------|
| **b1. Poll in `read-e2e-client-secret.sh`** | Replace the single `oc get` (`:35-45`) with a bounded poll (reuse the `wait-for-secret-keys.sh` pattern: loop with a deadline, default ~120-300s), then the same fail-closed error on timeout. | Tiny, isolated, strictly safer than today. Turns a re-run/redeploy race into a short wait or a clear timeout. Does not mask a genuinely missing secret (still fails, just after a bounded wait). |
| **b2. Make the suite job re-ensure the secret** | Have the suite depend on an ensure step rather than only a read. | Larger change to the job graph; duplicates Deploy-job responsibility; b1 is simpler and sufficient. |

### (c) deploy-ordering primitives

| Option | What | Tradeoffs |
|--------|------|-----------|
| **c1. `wait-for-keycloak` initContainer in the OpenShift overlay** | Apply `deploy/kind/wait-for-keycloak.patch.yaml` (or a shared copy) to `hypershell-api-server` and `hypershell-web-console` in `deploy/openshift/kustomization.yaml`, with `KEYCLOAK_ENDPOINT` pointed at `keycloak-service.<ns>-keycloak.svc.cluster.local`. | Highest-leverage, lowest-risk; declarative; mirrors Kind. One adaptation needed: the Kind patch hardcodes the `keycloak` namespace, but the OpenShift Keycloak namespace is `${OPENSHIFT_NAMESPACE}-keycloak`, so the endpoint must be parameterized (inject via a kustomize replacement, or have the driver `oc set env` it the way it already sets `JWK_CERT_URL`/`JWT_ISSUER`). |
| **c2. Split the overlay apply: Keycloak first, wait, then dependents** | In `apply_overlay()` (`openshift.sh:740-762`), apply Keycloak, `wait_for_keycloak`, then apply the app overlay. | Removes the "applied together" race at the driver level. More invasive to the driver, imperative, and still leaves a window if Keycloak is later restarted (the `KC_HOSTNAME` patch, the oauth recycle). Weaker than c1, which gates per pod on every restart. |
| **c3. Raise the api-server rollout timeout** | Bump the `wait_for_named_rollout hypershell-api-server` default above 180s (`openshift.sh:808,912`). | Cheap, but only widens the window for self-heal; does not stop the crash-loop and still burns CI minutes. A band-aid, acceptable only alongside c1. |
| **c4. Order KC_HOSTNAME into first boot** | Avoid the post-apply `KC_HOSTNAME` restart by resolving the Route host before the Keycloak pod first starts. | Removes one of the two OpenShift-only Keycloak restarts, but the Route host is not known until the Route exists; non-trivial. Lower priority once c1 is in place. |

---

## Primary recommendation

Adopt a layered fix, in this order of leverage:

1. **(c1 / a2) Port the `wait-for-keycloak` initContainer to the OpenShift
   overlay for the api-server and web-console.** This is the highest-leverage,
   lowest-risk change. It stops the api-server (and web-console) from ever
   starting before Keycloak serves JWKS, which removes the `CrashLoopBackOff`
   at its source and therefore the backoff-vs-rollout-timeout failure, the
   downstream gateway-provisioning stall, and the dominant source of
   `E2E / OpenShift` flakiness. It reuses machinery already proven on Kind and
   is scoped to `deploy/openshift/`. The only new work is parameterizing the
   endpoint for the `<ns>-keycloak` namespace.
2. **(b1) Give `read-e2e-client-secret.sh` a bounded poll.** Independent, tiny,
   and strictly safer; it closes the fast-abort on re-run/redeploy races.
3. **(a1) File an upstream rh-trex-ai change to retry the initial JWKS load with
   backoff instead of `os.Exit(1)`**, then bump the pin. This is the correct
   root fix and benefits all deployments, but it is the slowest, widest-blast
   path, so it should follow the two deploy-time guards rather than block them.

Highest-leverage / lowest-risk single change: **c1**. If only one thing ships,
ship the OpenShift `wait-for-keycloak` initContainer.

---

## Verification

To prove the fix reduces flakiness:

1. **Deterministic race repro (pre-fix, to characterize).** In a scratch
   OpenShift namespace, `oc scale deploy/keycloak --replicas=0`, then apply the
   app overlay. Confirm the api-server enters `CrashLoopBackOff` with
   `Failed to load JWT keys from URL ... connection refused` followed by
   `Unable to create JWT authentication handler` and exit. Bring Keycloak back
   and observe self-heal only after the current backoff interval.
2. **Post-fix ordering.** With the `wait-for-keycloak` initContainer applied,
   repeat step 1. Confirm the api-server pod sits in `Init:` (initContainer
   polling) rather than `CrashLoopBackOff`, its app container never starts until
   JWKS is reachable, and it goes Ready promptly once Keycloak is up - with no
   crash events in `oc get events`.
3. **Render check.** `kustomize build deploy/openshift` shows the
   `wait-for-keycloak` initContainer on `hypershell-api-server` and
   `hypershell-web-console` with the correct `<ns>-keycloak` endpoint;
   `kustomize build deploy/kind` is unchanged.
4. **Flakiness measurement.** Run the OpenShift suite repeatedly (ideally on a
   cold cluster that must autoscale a node for Keycloak) before and after. Expect
   the crash-loop deploy failures and the 180s api-server rollout-timeout
   failures to disappear.
5. **Secret-read poll.** Re-run only the `E2E / OpenShift` job against a
   torn-down environment and confirm `read-e2e-client-secret.sh` waits up to its
   deadline rather than aborting at ~43s, and still fails closed with the same
   clear error if the secret never appears.

## Blast radius

- **c1 (OpenShift initContainer):** limited to `deploy/openshift/`. On the happy
  path it adds one JWKS `curl` that returns immediately (a few seconds). The
  init image (`registry.access.redhat.com/hi/curl`) is already used by the Kind
  patch. Risk: a misconfigured endpoint makes the api-server wait the full
  60 x 5s = 300s then fail the initContainer; mitigate by sourcing the endpoint
  from the same namespace value the driver already uses for `JWK_CERT_URL`. No
  effect on Kind, hub, gitops, or ibm overlays (hub/gitops/ibm bundle no
  Keycloak; they use external SSO).
- **b1 (secret-read poll):** limited to `scripts/ci/read-e2e-client-secret.sh`.
  Worst case adds up to the poll deadline on a genuinely missing secret, then
  fails with the existing error. No change to the happy path beyond at most one
  extra `oc get`.
- **a1 (upstream retry):** affects every rh-trex-ai consumer, not just
  HyperShell. Must preserve fail-closed semantics and be coordinated with a pin
  bump and a Kind smoke test. Largest blast radius; do it last.

---

## Exact locations (quick reference)

| Finding | Location |
|---------|----------|
| api-server hard-exit on JWKS load | rh-trex-ai `components/api-server/pkg/server/apiserver.go` `Check(err, "Unable to create JWT authentication handler")` (line ~60), -> `pkg/server/server.go:32` `os.Exit(1)`; initial single load in `pkg/auth/middleware.go` `Build()` (~`:84`) / `loadKeys()` (~`:212`, warn at ~`:232`). Pinned at `components/api-server/go.mod:14` / `scripts/rh-trex-ai.ref` commit `8f11b55b73f2`. |
| JWKS URL wired into api-server (OpenShift) | `deploy/openshift/kustomization.yaml:177` (`--jwk-cert-url=$(JWK_CERT_URL)`), value from `scripts/cluster/drivers/openshift.sh:561,575` |
| e2e client-secret fast read | `scripts/ci/read-e2e-client-secret.sh:35-45`, called from `.github/workflows/e2e.yml:1003` |
| e2e client-secret create (pre-up) | `scripts/ci/ensure-e2e-client-secret.sh:35-40`, called from `.github/actions/deploy-pr-environment/action.yml:117` |
| Keycloak probes | `deploy/base/keycloak/keycloak.yaml:158-173` |
| Kind wait-for-keycloak guard (the fix to port) | `deploy/kind/kustomization.yaml:244-251` -> `deploy/kind/wait-for-keycloak.patch.yaml` |
| Kind explicit post-Keycloak restart | `scripts/kind/up.sh:836-846` |
| OpenShift overlay apply + wait ordering | `scripts/cluster/drivers/openshift.sh:740-762` (apply), `:818-823` (`wait_for_keycloak`), `:899-915` (`wait_for_deployments`), `:805-816` (`wait_for_named_rollout`, 180s default) |
| api-server base (only `migrate` init, no Keycloak wait) | `deploy/base/applications/api-server.yaml:25-55` |
