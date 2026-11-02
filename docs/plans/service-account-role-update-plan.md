# Implementation Plan: In-Place Service-Account Role Update

**Date:** 2026-09-24
**Branch:** `investigate/service-account-claims`
**Companion research:** `docs/investigations/service-account-claims-immutability.md`
**Tracking:** extends [HYPERSHELL-49](https://redhat.atlassian.net/browse/HYPERSHELL-49) (OpenShellGatewayServiceAccount)

---

## Objective

Allow the OpenShell role (the `hypershell.roles` token claim source) and the
description of an existing OpenShellGatewayServiceAccount to be changed after
creation, without replacing the credential. Audience, subject, credential type,
expiration, and token lifetime remain immutable.

## Decisions locked (product)

| Decision | Choice |
| --- | --- |
| Update direction | **Both directions** (upgrade and downgrade), capped at the **creator's current** gateway binding -- the same rule as create. Requires relaxing the spec's downgrade-permanence sentence. |
| Endpoint shape | **POST action**: `POST /api/hypershell/v1/gateways/{gateway_id}/service_accounts/{service_account_id}/role`, mirroring the existing `/revoke` action style. |
| Field scope | **Role + optional description** in one request. |
| Stack breadth | **Full stack in one slice**: spec, OpenAPI + SDK regen, API server + audit, hsctl, console UI. |

**Endpoint body note.** `description` rides on the `/role` action because a
second metadata endpoint is surface bloat for v1; the action is named for the
claim it changes. `role` is required, `description` optional. A caller who only
wants a description edit re-sends the current role (the UI does this). Spec
review may rename the action (`/update`) -- flag during Phase 0.

## Why this is safe (from the investigation)

- The claim-change machinery already exists and runs in production: the
  reconciler rewrites `hypershell.roles` post-creation on creator downgrade
  (`service.go:784–833`) and on drift repair
  (`serviceaccountkeycloak/client.go:196`, `:837`).
- Keycloak mints fresh claims per `client_credentials` grant; HyperShell stores
  no tokens and no role caches. The gateway reads roles statelessly from each
  token; the management plane never consumes SA claims.
- Already-issued tokens keep the prior role until `exp` (≤5 min default, ≤15 min
  absolute) -- the same lag the spec already accepts for downgrades/revocation.
- `role` is a plain, already-reconciler-written varchar (`model.go:38`); the
  `Reconcile` gRPC already carries `spec.role`
  (`service_accounts.proto`, field 7). **Zero control-plane or provisioner
  changes.**

## Design

### Request/response contract

```text
POST /api/hypershell/v1/gateways/{gateway_id}/service_accounts/{service_account_id}/role
Content-Type: application/json

{ "role": "openshell-admin", "description": "optional, ≤1024 chars" }
```

| Condition | Response |
| --- | --- |
| Success (Keycloak converged synchronously) | `200` with `OpenShellGatewayServiceAccountListItem` (existing schema; already carries `role`, `description`, `status`) |
| Intent persisted, Keycloak unavailable or repair failed | `202` with the item in `degraded` status (`last_error` = stable code) |
| Requested role exceeds the creator's current ceiling | `403` `role_not_allowed` |
| Role value not in enum / description >1024 chars / unknown field | `400` `invalid_request` |
| Account not in `ready`/`degraded` (e.g. `provisioning`, `revoking`, `revoked`, `expired`, `error`, `deleting`) | `409` `service_account_not_mutable` |
| Not visible (no binding, or viewer reading another creator's account) | `404` `not_found` |
| Gateway OIDC not ready | `409` `gateway_not_ready` (repair path needs the gateway client) |

Authorization follows the existing visibility model: the creator, or any
`gateway:owner`/`gateway:admin` (`can_manage_all`). **The role ceiling is
always computed from the creator's current binding** (`Access.AllowedRoles`
for `account.CreatedByUserID`) -- never the actor's -- because the drift sweep
enforces the creator cap (`service.go:755`) and would otherwise revert an
owner-granted upgrade within one reconcile interval (default 30 s). An owner
managing another user's viewer-created account therefore sees `403` on an
`openshell-admin` request.

### Mutation flow (api-server, `service.UpdateRole`)

1. Visibility check (reuse `visibleAccount`), then acquire the per-gateway
   advisory lock and re-read (`lockedAccount`) -- identical to Revoke.
2. Validate: status ∈ {`ready`, `degraded`}; role enum; creator-cap
   (`roleAllowed(requestedRole, creatorAccess.AllowedRoles)`); description
   length.
3. **Persist intent first** (persistence-first, like revoke/delete): write the
   new `role`/`description` with status unchanged. A crash after this write is
   self-healing: the account is `ready`, so `ListDrift` returns it
   (`dao.go:201–212`) and the next sweep converges Keycloak to the persisted
   role through the existing `reconcileOne` path.
4. Emit `update_role` / `started` audit event (with `previous_role`).
5. Attempt the synchronous repair inline:
   `provisioner.ReconcileServiceAccount(serviceAccountSpec(account, gateway, oidc), uuid, subject, true)`.
6. Success → audit `update_role` / `succeeded`, `200`.
   Failure → follow the `failReconcile` pattern: set `degraded` +
   `last_error` (`keycloak_role_update_pending`), audit `update_role` /
   `failed`, `202`. The due/transitional drain includes `degraded`
   (`dao.go:181`) and re-converges within one interval, restoring `ready`.
7. No-op (unchanged role and description): skip the Keycloak round trip, still
   audit, return `200`. Idempotent retries are safe.

No new status enum value is introduced: `degraded` already means "reconciler
will re-converge; health signal, not lifecycle change" and is already exposed
in OpenAPI, SDKs, UI, and the handler's status filter map.

### Interaction with existing reconciliation

- Creator downgrade sweep (admin→user) and a user-initiated downgrade compose:
  both write the same desired state.
- Creator upgrade after a sweep downgrade: allowed once the creator currently
  holds the ceiling -- identical authority to creating a new admin SA. The
  sweep never *upgrades*, so a persisted upgrade is stable.
- The repair's client-disable window (seconds) may fail one in-flight grant;
  CI retry is already the documented pattern. The replacement flow remains
  available and unchanged.

---

## Work breakdown

### Phase 0 -- Spec amendment (`specs/platform/openshell-gateway-service-accounts.spec.md`)

Follow `skills/plan/spec/SKILL.md`. Amend (not just append):

1. **Product Decisions table**: add "Can the OpenShell role change after
   creation?" → "Yes, through the role action endpoint, capped at the creator's
   current gateway role."
2. **REST API Contract table** (near line 190): add the `POST .../role` row.
   Amend line 199: keep "no endpoint to read a secret, issue an access token,
   re-enable an account, or transfer ownership"; replace the blanket no-patch
   sentence with the role/description action. Add the error-semantics rows from
   the table above.
3. **New Requirement: In-Place Role Update** with scenarios:
   - Owner upgrades a user SA → next client-credentials grant carries both
     roles; already-issued tokens keep the prior role until `exp`.
   - Downgrade to `openshell-user` → new tokens omit `openshell-admin`.
   - Request exceeds the creator's ceiling (viewer-created account, owner
     actor) → `403`, no record change, no Keycloak write.
   - Creator-downgrade sweep and explicit update compose (same desired state).
   - Keycloak unavailable → `202` + `degraded`, sweep converges, `ready`
     restored.
   - Non-mutable status → `409`.
   - No-op request → `200`, zero Keycloak writes.
4. **Amend the downgrade-permanence scenario** (line ~626): a sweep downgrade
   stands until an authorized role update changes it while the creator holds
   the ceiling. Align the "Creator is downgraded" scenario wording.
5. **Auditability requirement**: add `update_role` (with previous role) to the
   audited actions; keep the no-secrets rules.
6. **CLI requirement**: add the update command and options.
7. **Secret-Safe Management UI requirement**: add the edit action, dialog, and
   confirmation-copy requirements (token-lag explanation, creator-cap error
   mapping).
8. **Verification Coverage**: add role-update cases.
9. Run the critic pass: cross-check `openshell-gateway-keycloak.spec.md`,
   `security/rbac-enforcement.spec.md`, and `gateway-access-management.spec.md`
   for statements that assume role immutability.

**Visual deliverable check (per skill Phase 1):** the service-accounts tab
gains a row action + dialog. If product supplies an approved visual reference,
a `specs/web-console/mockups/` mockup + Storybook story becomes part of this
deliverable and `check_mockup_patternfly.py` must pass. Without a reference,
proceed without a mockup (warn-and-continue path).

### Phase 1 -- API contract

- `components/api-server/openapi/openapi.serviceAccounts.yaml`: add the
  `/role` path (operationId `updateGatewayServiceAccountRole`) with request
  schema `OpenShellGatewayServiceAccountRoleUpdateRequest`
  (`role`: enum, required; `description`: string, optional), responses reusing
  `OpenShellGatewayServiceAccountListItem`; register in `openapi.yaml` path
  refs; add error responses per the table.
- Regenerate: `make generate` and `make generate-sdk` (Go + TS SDKs,
  `DefaultAPI.md`).

### Phase 2 -- API server (`components/api-server/plugins/serviceAccounts/`)

- **`model.go`**: add `PreviousRole *string` to `AuditEvent` (column
  `previous_role`).
- **`migration.go`**: new gormigrate migration (next ID after
  `2026082115000001`) adding the `previous_role` column. No other schema
  change.
- **`service.go`**: add `UpdateRole` to the `Service` interface and implement
  per the mutation flow above (~130 lines incl. validation helper
  `validateRoleUpdateInput`). Reuse: `visibleAccount`, `acquireGatewayLock`,
  `lockedAccount`, `serviceAccountSpec`, `readyGateway`, `roleAllowed`,
  `failReconcile`-style degraded write, `audit` (extend signature or add an
  `auditWithPrevious` helper).
- **`handler.go`**: `UpdateRole` handler -- strict JSON body
  (`updateRoleRequest`), route vars, map `bool complete` to 200/202.
- **`plugin.go`**: register
  `router.HandleFunc("/{service_account_id}/role", handler.UpdateRole).Methods(http.MethodPost)`.
- **`presenter.go`**: no change (item response already carries role/description).

### Phase 3 -- CLI (`components/cli/`)

- `pkg/serviceaccount/serviceaccount.go`: add `RolePath(gatewayID, id)` helper
  (`.../role`).
- `cmd/hsctl/update/serviceAccount/cmd.go`: new command (the `update` group
  already exists) -- `hsctl update serviceAccount ID --gateway-id --role
  [--description]`, structured output via existing
  `serviceaccount.Request`/`WriteStructured`; validate `--role` enum
  client-side; accept 200 and 202.

### Phase 4 -- Console UI (`packages/gateway-management-ui/src/`)

- `application/gateway-types.ts` + `gateway-operations.ts`: wrap the generated
  SDK method (`updateOpenShellGatewayServiceAccountRole`) following the
  `revokeOpenShellGatewayServiceAccount` pattern (name, execute label).
- `service-accounts/service-account-row-actions.tsx`: add an `Edit role`
  kebab action for `ready`/`degraded` rows, gated on the same visibility logic
  as revoke/delete.
- New `service-accounts/service-account-edit-dialog.tsx`: role `Select`
  reusing the create form's option content (role + access description) and a
  description textarea; submit sends both fields. Confirmation copy explains:
  (a) new tokens reflect the change on the next grant; (b) already-issued
  tokens keep the old role until they expire (≤5 min); (c) a viewer-created
  account cannot be raised above its creator's current role (map
  `role_not_allowed` to localized guidance; never display raw API reasons).
- `service-accounts/service-accounts-page.tsx`: invalidate/refetch the list on
  mutation success (existing pattern); degraded rows already poll every 5 s.
- Locale messages (`messages` module): dialog title, field labels,
  confirm/cancel, token-lag explanation, error mappings.
- Follow the ui-build-gate (`skills/build/ui-build-gate/SKILL.md`) for the
  changed page -- mockup story required only if Phase 0 produced one.

### Phase 5 -- Verification

- `make check` (repo policy gates), `make generate-sdk` drift check.
- Go: `go test ./components/api-server/plugins/serviceAccounts/... ./components/cli/...`.
- UI: package test suite (vitest) + lint.

## Test matrix

**Service (`service_test.go`)** -- reuse the in-memory DAO/provisioner fakes:

| Case | Asserts |
| --- | --- |
| Creator upgrades own account (owner) | 200; provisioner called with new role; audit `update_role`/`succeeded` with previous role; row persisted |
| Owner (manage-all) upgrades a viewer-created account | 403 `role_not_allowed`; no persist; no provisioner call |
| Viewer updates own account to user | 200 |
| Viewer requests admin | 403 |
| Creator with no remaining binding | 403 (cap empty) |
| Non-mutable status (revoked, provisioning, deleting, error) | 409, no writes |
| Invalid role / long description / unknown JSON field | 400 |
| No-op (same role + description) | 200; zero provisioner calls |
| Keycloak failure | 202; row `degraded` + `last_error`; audit `failed` |
| Persisted role converged by sweep after simulated crash post-persist | drift path sets ready; provisioner saw new role |
| Sweep does not revert a legitimate upgrade (creator still owner) | converged, role stays admin |
| Concurrent revoke during update | serialized by the advisory lock; one wins |
| Description-only change | persisted, provisioner skipped when role unchanged |
| Audit failure | error surfaced, no false 200 (follow create's pattern) |

**Handler**: route wiring, strict JSON, 200/202 mapping.

**Provisioner** (`serviceaccountkeycloak`): already-covered role replacement;
add one convergence test asserting a role-only diff triggers the
disable→replace→enable sequence and zero writes when unchanged.

**CLI**: command wiring, enum validation, output shape.

**UI**: row-action gating; dialog open/submit; `role_not_allowed` guidance;
list refetch; locale keys present.

**E2E (optional, follow-up)**: create admin SA → update to user → new token
lacks `openshell-admin`; old token still works until `exp`.

## Risks and mitigations

| Risk | Mitigation |
| --- | --- |
| Drift sweep races a foreground update | Both serialize on the per-gateway advisory lock; update re-reads under the lock (same pattern as Revoke) |
| Owner confusion when upgrading another user's account 403s | UI copy + stable error code explain the creator-cap invariant; server is authoritative |
| Spec inconsistency (downgrade permanence, "no patch" line) | Phase 0 amends both explicitly; critic pass greps related specs for immutability assumptions |
| Token-lag misunderstanding | Confirmation copy in UI + spec scenario state the ≤5-min lag explicitly |
| SDK drift gate failure | Run `make generate-sdk` in the same change; commit generated artifacts |

## Size estimate

| Area | Files | ~Lines |
| --- | --- | --- |
| Spec | 1 | 80–110 |
| OpenAPI + generated SDKs/docs | 4–8 | 150–200 (mostly generated) |
| API server + tests | 6 | 450–550 |
| Migration (previous_role) | 2 | 25 |
| CLI + tests | 3 | 150–200 |
| UI + tests + locale | 6–8 | 300–400 |

**Total: ~1,200–1,500 lines across ~22 files; 5–7 engineering days** including
spec review, SDK regeneration, and repo check gates. Control plane and
provisioner: **zero changes**.

## Sequencing

Phase 0 → 1 → 2 can land as one PR (contract + server). Phase 3 and 4 are
independent of each other and can follow in the same PR or two follow-ups;
Phase 5 gates apply to every PR. If the slice must shrink, the first cut is
Phase 0–2 + 5 (API only, CLI/UI follow-up) -- the endpoint is fully usable via
the generated SDK.
