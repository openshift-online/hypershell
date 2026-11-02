# Investigation: Can service-account claims be changed after creation?

**Date:** 2026-09-24
**Branch:** `investigate/service-account-claims`
**Question:** Is there a practical reason the claims assigned to an
OpenShellGatewayServiceAccount cannot be changed after creation? If not, scope
the change.
**Read-only investigation -- no product code was changed.**

---

## Verdict

**No practical technical reason. Claims immutability is unimplemented v1 scope,
not an enforced constraint.** The platform itself already changes a service
account's token claims after creation: the reconciler rewrites the
`hypershell.roles` claim (admin → user) whenever the creator's gateway binding
is downgraded, using exactly the machinery an update endpoint would need. There
is no schema constraint, no validation rule, and no git-recorded technical
rationale -- only a one-line v1 scope statement in the spec ("Version 1 has no
public endpoint to patch…", present unchanged since the original HYPERSHELL-49
commit).

There are three genuine *design interactions* to resolve (creator-cap
auto-revert, downgrade-permanence policy, and a seconds-long issuance blip
during repair), covered below. None is a blocker; two are policy choices and
one is an accepted, already-shipped behavior.

### What "claims" means here

The claims on a service-account access token are derived, not free-form
(`specs/platform/openshell-gateway-service-accounts.spec.md:18` -- "Callers
cannot set audiences, claims, or access-token lifetimes"):

| Claim | Source | Mutable in principle? |
| --- | --- | --- |
| `aud` (gateway audience) | Audience mapper pinned to the gateway client | **No -- must stay immutable.** Single-gateway isolation is a security invariant (spec §Single-Gateway Isolation). |
| `hypershell.roles` | Client-role mapper + role/scope mappings from the account's `role` column | **Yes.** This is the only user-meaningful "claim", and the platform already rewrites it post-creation (see evidence). |
| `sub`, `azp`, `iss` | Keycloak-generated identity | No -- identity of the credential. |
| `exp` / token lifetime | Pinned attribute, 300 s default, ≤900 s | No -- operator-configured only. |

So "changing a service account's claims" reduces to **changing its `role`**
(`openshell-user` ↔ `openshell-admin`), plus optionally non-claim metadata
(name/description).

---

## Evidence

### 1. Where claims are set, stored, and consumed

**Set (Keycloak, owned by the control plane).** The claims are produced by two
protocol mappers and two role-mapping layers that HyperShell pins on each
managed client:

- `components/control-plane/internal/serviceaccountkeycloak/client.go:389-431`
  (`managedProtocolMappers`): the `gateway-audience` mapper (`aud`) and the
  `gateway-client-roles` mapper (`hypershell.roles`).
- `components/control-plane/internal/serviceaccountkeycloak/client.go:837`
  (`replaceRoleMappings`): removes *all* existing user role-mappings and client
  scope-mappings, then assigns exactly the desired gateway roles
  (`desiredRoleNames`, client.go:607 -- admin = `[openshell-admin,
  openshell-user]`, user = `[openshell-user]`).

**Stored.** The durable record is the `role` column, a plain `varchar not null`
with no immutability constraint
(`components/api-server/plugins/serviceAccounts/model.go:38`). The DAO already
has general-purpose write paths that persist it
(`components/api-server/plugins/serviceAccounts/dao.go:63` `Update`, `:76`
`ConditionalUpdate`). Keycloak is the system of record for the live mappings;
the DB row is the desired state the reconciler converges to.

**Consumed (token issuance).** Keycloak mints a fresh token on every
`client_credentials` grant; HyperShell never stores or caches tokens (spec
§Data Model: "The database SHALL NOT contain … an access token"). A role change
therefore applies to the *next* grant. HyperShell verifies claims only once, at
create time (`verifyClientCredentials`, serviceaccountkeycloak/client.go:~1000).

**Consumed (gateway authz).** The OpenShell gateway validates tokens locally
from `hypershell.roles` (configured via `roles_claim`,
`components/control-plane/internal/gateway/reconciler.go:1168`). It is
stateless w.r.t. roles -- no role cache exists to invalidate. The only gateway
cache is JWKS (signing keys), which a role change does not touch.

**Consumed (management plane).** Nowhere. SA tokens are rejected by the
management API (no management audience, no RoleBinding for the SA subject --
spec scenario "Gateway access token is presented to the management API";
management authz is DB-binding-based,
`components/api-server/pkg/rbac/authorization.go:110-120`). Changing SA claims
cannot affect management-plane authorization.

**Already-issued tokens.** Bounded by the 5-minute default (hard max 15 min)
access-token lifetime: a token minted before a role change keeps its old
`hypershell.roles` until `exp`. This is identical to today's semantics for
reconciler-driven downgrades (spec: "an already-issued admin JWT MAY remain
usable only until its short `exp`").

### 2. The claim-change machinery already exists and runs in production

The reconciler changes SA claims after creation today, on two paths:

- **Creator downgrade:** `components/api-server/plugins/serviceAccounts/service.go:784`
  detects `account.Role == RoleAdmin` while the creator's current binding no
  longer permits admin, `service.go:820` rewrites the spec to `openshell-user`,
  `service.go:822` calls the provisioner's `ReconcileServiceAccount`, and
  `service.go:832-833` persists the new role with a `role_downgrade` audit
  event.
- **Drift repair:** `components/control-plane/internal/serviceaccountkeycloak/client.go:196`
  (`ReconcileServiceAccount`) diffs desired vs. live state, and on divergence
  disables the client, calls `updateRepresentation`, `replaceRoleMappings`, and
  `replaceProtocolMappers`, then re-enables -- never touching the client secret.

Both paths write the `role` column post-creation through the normal DAO. An
update endpoint is the same operation with a user-supplied desired role instead
of a derived one.

### 3. Immutability is not enforced anywhere

- **Routes:** only GET/POST/GET/DELETE/revoke are registered
  (`components/api-server/plugins/serviceAccounts/plugin.go:71-75`). No
  PATCH/PUT route exists -- the gap is *absence*, not rejection.
- **OpenAPI:** `components/api-server/openapi/openapi.serviceAccounts.yaml`
  defines only the five v1 paths; no patch operation or request schema.
- **Schema:** `role` is a mutable varchar (model.go:38); the migration
  (`migration.go`) adds no constraint; the DAO update paths are unrestricted.
- **Validation:** `validateCreateInput` (service.go:~940) validates roles at
  create only; nothing rejects a role change because nothing accepts one.
- **Spec:** the sole statement is the v1 scope line,
  `specs/platform/openshell-gateway-service-accounts.spec.md:199`: "Version 1
  has no public endpoint to patch an OpenShellGatewayServiceAccount, read a
  secret, issue an access token, re-enable an account, or transfer ownership."
  Role updates are **not** in the spec's "Explicitly Deferred" table (that table
  defers secret rotation, WIF, multi-gateway credentials, etc.) -- the omission
  was never a recorded decision.
- **Git history:** the no-patch line is unchanged since the original
  HYPERSHELL-49 commit `85115573` (initial spec, Aug 2026). No subsequent
  commit (`git log -S "patch"`, `-S "update"`, `-S "role"` on the spec/plugin)
  discusses adding or rejecting an update endpoint. No technical rationale was
  ever recorded.

### 4. The three real design interactions (not blockers)

1. **Creator-cap auto-revert.** Every drift sweep re-derives the allowed ceiling
   from the *creator's* current binding
   (`service.go:755` -- `s.access(ctx, account.GatewayID, account.CreatedByUserID)`),
   so an account whose role exceeds the creator's current binding is
   auto-downgraded within one reconcile interval (default 30 s,
   `plugin.go:100-110`). Consequently, if an *owner other than the creator*
   upgraded a viewer-created account to `openshell-admin`, the next sweep would
   revert it. An update feature must either (a) cap the request at the
   creator's current binding (recommended -- preserves the spec invariant "Each
   active OpenShellGatewayServiceAccount SHALL remain subject to the creator's
   current gateway role", spec:595), or (b) introduce a separate role-ceiling
   concept (larger change, not recommended for v1).
2. **Downgrade-permanence policy.** Spec:626 makes a reconciler downgrade
   permanent ("even if user-a is later promoted"). A bidirectional update
   endpoint relaxes this: re-upgrading requires the creator to *currently* hold
   an owner-equivalent binding, which is exactly the power the creator already
   has to create a new `openshell-admin` account. No escalation results. The
   spec line must be amended either way (or updates restricted to
   downgrade-only, which conflicts with nothing).
3. **Issuance blip during change.** `ReconcileServiceAccount` disables the
   client while replacing role mappings
   (serviceaccountkeycloak/client.go:239-253) so no token is minted mid-repair.
   An in-place role change therefore causes a seconds-long window where
   `client_credentials` grants fail -- the same behavior as today's drift
   repair. CI jobs fetch tokens on demand and retry; the replacement flow
   (create-new → switch → revoke-old) remains the zero-blip alternative and
   stays supported.

---

## Scoped proposal: role update endpoint

**Scope:** add a user-initiated role change (and optionally description edit)
for ready service accounts. Audience, subject, credential type, expiration, and
token lifetime remain immutable. No ownership transfer, no re-enable, no secret
rotation.

### API

- `PATCH /api/hypershell/v1/gateways/{gateway_id}/service_accounts/{service_account_id}`
  with body `{"role": "openshell-user" | "openshell-admin", "description"?: string}`.
  (Alternative: `POST .../role` action endpoint, mirroring the existing
  `.../revoke` style -- either fits the existing contract; PATCH matches the
  rh-trex-ai generator conventions.)
- Authorization: same visibility rules as Revoke (creator, or any
  `gateway:owner`/`gateway:admin` via `can_manage_all`). The requested role is
  capped at the **creator's current** gateway binding using the existing
  `roleAllowed`/`Access.AllowedRoles` logic -- identical rule to create, which
  resolves interaction #1 without touching the reconciler.
- Validation: `role` ∈ {`openshell-user`, `openshell-admin`}; request rejected
  with `403 role_not_allowed` if it exceeds the creator's ceiling (matching
  create semantics); `409` if the account is not `ready`/`degraded` (terminal
  and transitional states reject); `description` ≤1024 chars.
- Response: `202 Accepted` with the account in a transitional state (reuse
  `degraded`, or add `updating`), or `200` after the synchronous Keycloak
  repair completes -- mirroring the revoke/delete persistence-first pattern.
- OpenAPI: new path + request/response schemas in
  `openapi.serviceAccounts.yaml`; regenerate Go and TS SDKs (`make
  generate-sdk`).

### Service / provisioner changes

- New `Service.UpdateRole` (~120 lines in `service.go`): acquire the per-gateway
  advisory lock, re-read the account (`lockedAccount`), validate, write the new
  desired role + transitional status, call the **existing**
  `provisioner.ReconcileServiceAccount` with the updated spec, audit, commit.
  On failure, follow the existing `failReconcile`/`degraded` pattern -- the
  background sweep already converges a half-applied role change, so the
  endpoint can be best-effort after the intent row is persisted.
- **No provisioner change.** `Reconcile` gRPC already accepts a full
  `ServiceAccountSpec` including `role`
  (`components/api-server/proto/hypershell/provisioner/v1/service_accounts.proto`,
  `ServiceAccountSpec.role = 7`).
- Handler + route: one handler method and one `router.HandleFunc` (PATCH) in
  `plugin.go`.

### Migration

**None required.** `role` is already a stored, reconciler-written column; the
audit table already records role values per event. If a distinct `updating`
status is chosen instead of reusing `degraded`, add the enum value to the
OpenAPI schema, SDKs, UI status union, and the handler's `validStatus` map
(handler.go:~125) -- no SQL migration (status is a varchar).

### Existing tokens and caches

- Already-issued access tokens keep the prior `hypershell.roles` until `exp`
  (≤5 min default, ≤15 min absolute) -- same bound the spec already accepts for
  downgrades and revocations. Document it in the endpoint's UI copy ("may take
  up to the access-token lifetime to take effect on in-flight jobs").
- No HyperShell-side cache holds SA claims: the gateway reads roles statelessly
  from each token; the management plane never consumes SA claims; JWKS caching
  is unaffected (signature keys unchanged).
- The repair's disable window (seconds) may fail one in-flight grant; CI
  retry-on-401 is already the documented pattern (spec §Client Credentials
  Token Issuance).

### Authorization, audit, and spec

- Add an `update_role` audit event (actor, creator, old role captured via the
  existing `AuditEvent.Role` field on a preceding `update_role`/`started`
  event, or extend `AuditEvent` with `previous_role` -- a nullable column and
  one-line migration, recommended for the audit trail).
- Spec changes (`openshell-gateway-service-accounts.spec.md`): amend the v1
  no-patch line (keep "no secret read, no token issuance, no re-enable, no
  ownership transfer"); add an "In-Place Role Update" requirement with the
  creator-cap rule, the token-lifetime lag scenario, and the downgrade-flow
  interaction; relax or refine the downgrade-permanence sentence at spec:626;
  add update coverage to the Verification Coverage list.

### Tests

- Service unit tests: creator-capped upgrade/downdate, `403` on ceiling
  exceeded, `409` on non-ready states, lock serialization against a concurrent
  revoke, persistence-first `202` when Keycloak is unavailable, auto-converge
  after a failed repair (degraded → ready with new role), drift sweep does
  *not* revert a legitimate update (creator still holds the ceiling).
- Handler tests: route wiring, strict JSON, status codes.
- Provisioner: covered by existing `ReconcileServiceAccount` drift tests
  (role-set change is already exercised); add one convergence test asserting a
  role-only change performs the disable→replace→enable sequence and zero writes
  when the role is unchanged.
- UI: row-action render gating on capabilities; edit dialog; locale keys.
- E2E (optional): create admin SA → update to user → assert new token lacks
  `openshell-admin`.

### UI / CLI

- UI (`packages/gateway-management-ui/src/service-accounts/`): add an edit
  action to `service-account-row-actions.tsx`, a small role-select dialog
  reusing the create form's role control, list refetch after mutation. Note:
  any changed console page follows the standard ui-build-gate/mockup process.
- CLI (`components/cli/cmd/hsctl/`): `hsctl update service-account --role …`
  following the existing create/revoke command structure.

### Size estimate

| Area | Files | ~Lines |
| --- | --- | --- |
| Spec | 1 | 60–90 |
| OpenAPI + regenerated SDKs | 3–8 | 150 (mostly generated) |
| API server (handler, route, service, tests) | 5 | 350–450 |
| Audit `previous_role` (model + migration) | 2 | 25 |
| UI + tests | 4–6 | 250–350 |
| CLI + tests | 3–4 | 150–200 |

**Total: ~1,000–1,300 lines across ~20 files; roughly 4–6 engineering days**
including spec review, SDK regeneration, and the repository check gates. The
control plane and provisioner need **zero** changes -- the entire mechanism
already exists.

### Alternatives (if update is deferred again)

The replacement flow (create → verify → switch consumer → revoke → delete) is
already the documented, zero-blip path (spec §Replacement-Based Credential
Rotation) and remains available; it costs a quota slot during overlap and a
consumer-side credential update. If product wants *no* new surface at all,
documenting the replacement flow as the intentional answer to this question --
and adding role updates to the Explicitly Deferred table with a reason -- is the
minimal honest fix for the current spec gap (the deferral is currently
implicit only).
