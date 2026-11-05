# CI Trust Extension API - self-service keyless CI on a gateway

**Date:** 2026-11-04
**Status:** Draft
**Related:** [`data-model.spec.md`](./data-model.spec.md) (CIProvider / CITrust kinds, PLAT-DATA-24/25), [`../security/rbac-enforcement.spec.md`](../security/rbac-enforcement.spec.md) (CIProvider `ci-federation:admin` authorization SEC-RBAC-21, CITrust per-gateway tenancy SEC-RBAC-22), gitops `specs/ci-oidc-federation.spec.md` (platform Keycloak federation design), [`control-plane.spec.md`](./control-plane.spec.md) (reconciler), `gateway-access-management.spec.md` (GTW-ACC Keycloak role bridge), [`agent-workspace-gateway-api.spec.md`](./agent-workspace-gateway-api.spec.md) (workspace scoping)

## Overview

A HyperShell user enables their own CI (GitHub Actions or GitLab CI/CD) to run
Fullsend agents against a gateway they manage, with **no stored secret**. The CI job
mints its native OIDC token, exchanges it at the cluster Keycloak, and receives a
short-lived OpenShell access token scoped to a workspace on that gateway.

This spec defines the reconcile contract and security model for the two extension
kinds whose data model lives in [`data-model.spec.md`](./data-model.spec.md):

- **CIProvider** (PLAT-DATA-24): registration of a CI OIDC issuer, authorized by
  the narrow `ci-federation:admin` role (not `platform:admin`; see
  [`../security/rbac-enforcement.spec.md`](../security/rbac-enforcement.spec.md)
  SEC-RBAC-21). Realized as a validation-only Keycloak identity provider.
- **CITrust** (PLAT-DATA-25): a gateway owner or admin's declaration that CI
  matching a subject claim predicate may federate into that gateway, authorized
  by the per-gateway `CanManageGatewayResources` tenancy check (SEC-RBAC-22).
  Realized as a scoped Keycloak claim-to-workspace mapping, a token-exchange
  permission, and a gateway RBAC binding.

The authorization model for both kinds (the `ci-federation:admin` role for
`CIProvider` and the per-gateway tenancy for `CITrust`) is specified canonically
in [`../security/rbac-enforcement.spec.md`](../security/rbac-enforcement.spec.md)
(SEC-RBAC-21, SEC-RBAC-22); this spec defers to it and restates only the
per-gateway tenancy gate in PLAT-CITR-03.

There is no secret in the repo, no OAuth app to register on the forge, and no
operator Terraform per tenant: a `CITrust` is the entire self-service act. This is
the declarative, user-owned face of the platform federation described in the
gitops `ci-oidc-federation.spec.md`; a `CIProvider` IS that spec's per-forge IdP.

## Architecture

```
user -> hsctl apply CITrust ──▶ API Server (PostgreSQL)
                                     │ gRPC watch (ext kinds)
                                     ▼
                          Control Plane reconciler (per cluster)
                                     │ reuses Keycloak admin client
                                     │ (components/control-plane/internal/keycloak)
             ┌───────────────────────┼───────────────────────────┐
             ▼                       ▼                             ▼
   validate-only OIDC IdP    scoped claim->workspace      gateway RBAC binding
   (from CIProvider)         mapper + token-exchange       (role in gh-<owner> /
                             permission onto gateway aud    gl-<namespace>)

later, at CI time (no control-plane involvement):
   CI run -> mint OIDC token (aud pinned) -> token-exchange at cluster Keycloak
          -> OpenShell token (workspace-scoped) -> gateway -> sandbox on cluster
```

The control plane already provisions Keycloak clients and protocol mappers
programmatically for gateways (`ProvisionGatewayClient`, `createProtocolMappers`
in `components/control-plane/internal/keycloak/client.go`). CIProvider/CITrust
reconciliation reuses that admin-REST client. The CI exchange authorization is
**config-driven** (PLAT-CITR-08): the control plane selects, by configuration and
not by code, how it realizes the token-exchange grant, because fleet Keycloaks run
different token-exchange generations. Two modes exist:

- **fgap-v1 mode** (the hyp0 fleet today): Keycloak runs legacy token exchange
  (`TOKEN_EXCHANGE`) with fine-grained admin authorization v1
  (`ADMIN_FINE_GRAINED_AUTHZ`) **enabled**. External-to-internal exchange is gated
  by two fine-grained grants the control plane SHALL attach (PLAT-CITR-10): an IdP
  token-exchange grant and a target-gateway-client token-exchange grant, each
  scoped to the shared federation requester client. The v2 external-internal path
  is not usable here because it rejects `subject_issuer` and does not wire IdP
  token-exchange permissions.
- **v2-attribute mode**: Keycloak runs external-to-internal token-exchange v2
  (`TOKEN_EXCHANGE_EXTERNAL_INTERNAL_V2`); the control plane authorizes the
  exchange by setting the `standard.token.exchange.enabled` attribute on the
  target gateway client, and the v1 permission API
  (`.../identity-provider/instances/<alias>/management/permissions`) is neither
  needed nor called.

Selecting the mode by configuration lets the fleet revert to v2 (RHBK >= 26.7,
once v2 wires IdP token-exchange permissions) with a configuration change and no
code change. The CI exchange requester is a **public** (keyless) client in every
mode; the CI run's OIDC token is the only credential. The fgap-v1 grants reuse the
same admin-REST flow as the e2e helper `EnsureE2ETokenExchange` /
`enableTokenExchangePermissions`, generalized from the fixed `hypershell-e2e`
requester to the CI federation requester (PLAT-CITR-10); that helper's own
impersonation use on the separate e2e subsystem
(`ephemeral-pr-environments.spec.md`) is unchanged.

## Requirements

### Requirement: PLAT-CITR-01 -- CIProvider Realization

The control plane SHALL realize each `CIProvider` as a **validation-only** OIDC
identity provider on the cluster Keycloak: signature validation against
`jwks_url`, issuer equal to `issuer`, and audience equal to `audience`. The IdP
SHALL NOT have an authorization-code flow enabled and SHALL hold no usable client
secret. The control plane SHALL write the realized alias to
`CIProvider.status.idp_alias` and set the resource Ready only when the IdP exists
and its JWKS is reachable.

A `CIProvider` SHALL be realized at most once per distinct `issuer` on a given
cluster Keycloak; multiple `CIProvider` records with the same issuer are a
conflict and SHALL be rejected with 409.

When the platform federation (the gitops realm configuration) already provisions
the identity provider and its claim mappers for a forge (for example the
`github-actions` IdP and its `repository` / `repository_owner` / `ref` claim
mappers), the control plane SHALL **adopt** that existing IdP: it SHALL record the
existing alias in `status.idp_alias` rather than create a second, parallel IdP,
and it SHALL NOT delete a platform-provisioned IdP on `CIProvider` delete. The
recorded alias SHALL be the alias Keycloak actually holds, so later reconcile
passes and the token exchange resolve the same IdP.

#### Scenario: PLAT-CITR-01.01 -- JWKS Unreachable Holds Not-Ready

- GIVEN a `CIProvider` whose `jwks_url` the cluster Keycloak cannot reach
- WHEN the control plane reconciles it
- THEN the `CIProvider` SHALL NOT be marked Ready
- AND its status SHALL name the unreachable JWKS endpoint

#### Scenario: PLAT-CITR-01.02 -- Platform-Provisioned IdP Is Adopted, Not Duplicated

- GIVEN the gitops realm configuration has already created the `github-actions` IdP
- WHEN the control plane reconciles a `github` `CIProvider` for that issuer
- THEN it SHALL record `status.idp_alias = github-actions`
- AND it SHALL NOT create a second IdP for the same issuer
- AND deleting the `CIProvider` SHALL NOT delete the platform-provisioned IdP

---

### Requirement: PLAT-CITR-02 -- CITrust Realization

For a Ready `CITrust`, the control plane SHALL realize three things and nothing
secret:

1. A **scoped claim-to-group mapping** on the referenced `CIProvider` IdP that
   accepts only tokens whose claims match the compiled `subject` predicate -- which
   SHALL include the repository/owner identity claim for GitHub/Forgejo
   (`repository`/`repository_owner`) or `project_path` for GitLab, since a
   ref/environment-only predicate is not a trust boundary -- and lands the matched
   identity in a **per-trust group** isolated to that one `CITrust` (its subject
   and its gateway). The group SHALL NOT be a shared per-owner group: aggregating
   distinct trusts' roles onto one `gh-<owner>` group would let a low-privilege
   repository inherit a sibling repository's higher role and leak roles across
   gateways.
2. A **token-exchange authorization** that allows the shared, **public** (keyless)
   federation client to exchange a token from the `CIProvider` IdP onto the target
   gateway's audience, so the exchange grant can mint a gateway-scoped token. It
   SHALL be realized through the configured authorization mode (PLAT-CITR-08): the
   two fine-grained v1 grants scoped to the federation requester client
   (PLAT-CITR-10) in fgap-v1 mode, or the external-to-internal v2 attribute on the
   gateway client in v2-attribute mode.
3. A **gateway RBAC binding** granting `role` to that per-trust group on the
   referenced `Gateway`, so a matched identity holds exactly the role its own
   `CITrust` grants on its own gateway and nothing more.

The derived workspace label (`gh-<owner>` for GitHub, `gl-<namespace>` for GitLab)
remains the forge-namespaced tenancy namespace reported in `CITrust` status and
used for quota; it is a display/quota namespace, not the RBAC group the role binds
to.

The control plane SHALL render no raw credential, PEM, or `Secret` for a
`CITrust`; the exchange requester holds no secret. Deleting a `CITrust` SHALL
remove the per-trust subject mapper, which is the authoritative revocation (the
deleted subject can no longer enter its group and so holds no role); the inert
per-trust group and the gateway's shared exchange opt-in MAY be left in place.

#### Scenario: PLAT-CITR-02.01 -- Forge-Namespaced Workspace Derivation

- GIVEN a `CITrust` referencing a `github` `CIProvider` with `subject.repository = acme/app`
- AND a second `CITrust` referencing a `gitlab` `CIProvider` with `subject.project_path = acme/app`
- WHEN both are reconciled
- THEN the first SHALL map to workspace `gh-acme` and the second to `gl-acme`
- AND the two SHALL be isolated workspaces with independent RBAC

#### Scenario: PLAT-CITR-02.02 -- Same-Owner Repositories Do Not Cross-Inherit

- GIVEN a `CITrust` for `subject.repository = acme/low` granting `openshell-user` on `gw-1`
- AND a second `CITrust` for `subject.repository = acme/high` granting `openshell-admin` on `gw-1`
- WHEN both are reconciled
- THEN each SHALL bind its role to its own per-trust group, not a shared `gh-acme` group
- AND a token from `acme/low` SHALL hold only `openshell-user` (it SHALL NOT inherit `openshell-admin`)
- AND a `CITrust` role granted on one gateway SHALL NOT bind onto another gateway's client

---

### Requirement: PLAT-CITR-03 -- Gateway Tenancy and Role Bound

The API server SHALL reject a `CITrust` create, update, or delete unless the
caller can manage the referenced `Gateway` -- `gateway:owner` or `gateway:admin`
on it, or `platform:admin` -- with 403; this is the shared
`CanManageGatewayResources` per-gateway tenancy check, authorized identically to
the other gateway-scoped extension kinds and specified canonically in
`security/rbac-enforcement.spec.md` (SEC-RBAC-22). The `gateway_id` SHALL be read
from the request body on create and from the stored record on update and delete.
The API server SHALL reject a `CITrust` whose `role` exceeds the caller's own
rights on that gateway, with 403. A `CITrust` therefore can only grant CI access
to a gateway the caller controls, and never more authority than the caller holds.
Deny-by-default holds: an unbound caller, a `gateway:creator` with no binding on
the referenced gateway, a `gateway:viewer`, and a caller bound only to a
different gateway SHALL all be rejected on write.

#### Scenario: PLAT-CITR-03.01 -- Cross-Tenant Grant Impossible

- GIVEN caller A manages gateway `gw-a` and caller B manages gateway `gw-b`
- WHEN caller A creates a `CITrust` referencing `gw-b`
- THEN the API server SHALL reject it with 403
- AND no Keycloak mapping or RBAC binding SHALL be created for `gw-b`

---

### Requirement: PLAT-CITR-04 -- Subject Predicate Is the Trust Boundary; Audience Pinning and Default-Deny

The real trust boundary SHALL be the per-subject claim predicate, not the
audience. The federated audience SHALL come from `CIProvider.audience` and SHALL
NOT be settable on a `CITrust`; a token whose `aud` does not equal the pinned
audience SHALL be rejected at exchange. The pinned audience is necessary but NOT
sufficient: because any repository on the forge can mint a token carrying that
audience, the exchange SHALL additionally succeed only for a token whose
`repository` and `repository_owner` claims (and any tighter claims the `subject`
sets) match a `CITrust` predicate for the requested gateway. A token that carries
the right audience but a non-matching `repository` or `repository_owner` SHALL be
rejected. Claims that do not match any `CITrust` subject predicate SHALL yield no
roles; the gateway SHALL reject a session that carries no workspace role
(default-deny). Once any `CITrust` exists for a gateway, the exchange SHALL remain
closed to every repository that no `CITrust` predicate admits.

#### Scenario: PLAT-CITR-04.01 -- Wrong Audience Rejected

- GIVEN a CI token minted with an audience other than the provider's pinned `audience`
- WHEN it is presented for token exchange
- THEN Keycloak SHALL reject the exchange
- AND no OpenShell token SHALL be issued

#### Scenario: PLAT-CITR-04.02 -- Correct Audience But Wrong Repository Rejected

- GIVEN a `CITrust` for `subject.repository = openshift-online/hypershell` on a gateway
- AND a CI token carrying the pinned `audience` but minted from a different repository
- WHEN that token is presented for token exchange onto the gateway
- THEN Keycloak SHALL reject the exchange
- AND no OpenShell token SHALL be issued

---

### Requirement: PLAT-CITR-05 -- Lifecycle and Referential Integrity

A `CIProvider` SHALL be deletable only when no `CITrust` references it (409
otherwise). Deleting a `CITrust` SHALL remove its claim mapping, token-exchange
permission, and gateway RBAC binding before the record is removed. Deleting the
referenced `Gateway` SHALL cascade-remove its `CITrust` records and their realized
artifacts.

#### Scenario: PLAT-CITR-05.01 -- CIProvider Delete Blocked While Referenced

- GIVEN a `CIProvider` with one `CITrust` referencing it
- WHEN a delete is requested on the `CIProvider`
- THEN the API server SHALL reject it with 409

#### Scenario: PLAT-CITR-05.02 -- Deleting a CITrust Revokes the Exchange

- GIVEN a `CITrust` whose subject can currently exchange its CI token for a gateway-scoped token
- WHEN the `CITrust` is deleted
- THEN the control plane SHALL remove the claim mapping, token-exchange authorization, and gateway RBAC binding
- AND a subsequent exchange for that subject SHALL fail

---

### Requirement: PLAT-CITR-06 -- Status and Workflow Hint

A Ready `CITrust` SHALL surface in status: `exchange_endpoint` (the cluster
Keycloak token endpoint), `workspace` (the derived forge-namespaced workspace),
and `workflow_hint` (a runnable CI snippet for the provider's forge that mints the
OIDC token with the pinned audience and exchanges it). The `workflow_hint` SHALL
NOT contain any secret.

---

### Requirement: PLAT-CITR-07 -- Multi-Forge Support

The control plane SHALL support `github`, `gitlab`, and `forgejo` providers
through the same reconcile path, differing only in the OIDC issuer, the subject
claim shape (per PLAT-DATA-25), and the `subject_issuer` used at exchange. Each
distinct GitLab or Forgejo instance SHALL be a separate `CIProvider` (issuers
differ). A provider whose issuer OIDC discovery or JWKS is not reachable from the
cluster Keycloak SHALL hold Not-Ready per PLAT-CITR-01; this is the expected state
for an internal forge (e.g. `gitlab.cee.redhat.com`) until a reachable path
exists.

---

### Requirement: PLAT-CITR-08 -- Config-Driven Token-Exchange Authorization Mode

The control plane SHALL select how it realizes the CI token-exchange authorization
by **configuration, not by code** (separate configuration from code): a configured
mode SHALL choose between the fine-grained-authorization v1 grant flow (fgap-v1
mode, PLAT-CITR-10) and the external-to-internal token-exchange v2 attribute flow
(v2-attribute mode). Moving the fleet from one mode to the other -- for example
reverting to v2 once a Keycloak release (RHBK >= 26.7) wires IdP token-exchange
permissions into v2 -- SHALL require only a configuration change and SHALL NOT
require a code change. In every mode the requester SHALL be the shared **public**
federation client and the CI run's OIDC token SHALL be the only credential; no
client secret SHALL be required anywhere in CI.

In **v2-attribute mode** the control plane SHALL authorize the exchange through the
Keycloak external-to-internal token-exchange v2 mechanism
(`TOKEN_EXCHANGE_EXTERNAL_INTERNAL_V2`), driven through the Keycloak admin REST
API, by setting the `standard.token.exchange.enabled` attribute on the target
gateway client; it SHALL NOT call the v1 token-exchange permission API
(`.../identity-provider/instances/<alias>/management/permissions`).

In **fgap-v1 mode** the control plane SHALL authorize the exchange through the
fine-grained admin authorization v1 permission API by attaching the two grants of
PLAT-CITR-10, and SHALL NOT rely on the v2 attribute as the authorization.

Each mode SHALL realize only the artifacts its own mechanism needs, and the
reconciler SHALL NOT fail because a mechanism the OTHER mode uses is unavailable on
the running Keycloak: in v2-attribute mode a 501 from the v1 permission API SHALL
NOT fail reconciliation, and in fgap-v1 mode the absence of v2 external-internal
support SHALL NOT fail reconciliation.

The exchange this authorizes is, in both modes, a token-exchange grant at the realm
token endpoint presenting the forge's OIDC JWT as the `subject_token`, the
`CIProvider` IdP alias as the `subject_issuer`, and the referenced gateway's client
as the `audience`; a successful exchange returns a workspace-scoped access token.

#### Scenario: PLAT-CITR-08.01 -- Mode Is Selected by Configuration, Not Code

- GIVEN a fleet Keycloak running fine-grained admin authorization v1 (fgap-v1)
- WHEN the control plane is configured for fgap-v1 mode
- THEN it SHALL authorize the exchange through the v1 fine-grained grant flow (PLAT-CITR-10)
- AND reconfiguring the same build to v2-attribute mode SHALL switch it to the v2 attribute flow with no code change

#### Scenario: PLAT-CITR-08.02 -- The Inactive Mode's API Is Not Required

- GIVEN the control plane configured for v2-attribute mode on a Keycloak whose v1 permission API returns 501
- WHEN it reconciles a `CITrust`
- THEN it SHALL authorize the exchange through the v2 attribute flow
- AND it SHALL NOT call the v1 token-exchange permission API
- AND reconciliation SHALL NOT fail because that v1 API returns 501

---

### Requirement: PLAT-CITR-09 -- Idempotent Reconcile That Survives Realm Reconcile

CITrust reconciliation SHALL be idempotent and re-entrant: repeated reconciles of
an unchanged `CITrust` SHALL converge to the same Keycloak state without creating
duplicate artifacts or erroring on already-present ones, per the reconciliation
contract (`specs/standards/control-plane/reconciliation-contract.spec.md`,
STD-RCON-02). This idempotency SHALL cover whichever token-exchange authorization
artifacts the configured mode (PLAT-CITR-08) realizes, including the fgap-v1 scope
permissions and client policies of PLAT-CITR-10 in fgap-v1 mode. Every realized artifact SHALL coexist with the periodic
realm-reconcile job that re-applies the gitops realm configuration: artifacts the
control plane creates SHALL NOT be pruned by that job, and artifacts the gitops
configuration owns (the IdP and its claim mappers) SHALL be adopted rather than
recreated or deleted (per PLAT-CITR-01). A failed read of Keycloak state SHALL be
treated as unknown, not absent, so the reconciler SHALL NOT delete or re-create an
artifact on a transient admin-API failure (STD-RCON-08).

#### Scenario: PLAT-CITR-09.01 -- Artifacts Survive the Realm-Reconcile Job

- GIVEN a Ready `CITrust` whose exchange authorization and RBAC binding are realized
- WHEN the periodic realm-reconcile job re-applies the gitops realm configuration
- THEN the `CITrust` exchange SHALL keep working without a new reconcile pass
- AND a subsequent reconcile of the unchanged `CITrust` SHALL make no duplicate artifact

---

### Requirement: PLAT-CITR-10 -- Fine-Grained v1 Token-Exchange Grants

In fgap-v1 mode (PLAT-CITR-08), external-to-internal token exchange is gated by two
independent fine-grained authorization v1 checks, and the control plane SHALL
attach BOTH for each `CITrust`, each scoped to the shared public federation
(requester) client:

1. **IdP token-exchange grant.** The control plane SHALL enable fine-grained
   authorization on the `CIProvider` IdP, SHALL create a client policy naming the
   federation requester client, and SHALL attach that policy to the IdP's
   `token-exchange` scope permission on the `realm-management` resource server.
   Without it the IdP-level exchange-authorization check denies the exchange.
2. **Target-client token-exchange grant.** The control plane SHALL enable
   fine-grained authorization on the target gateway client, SHALL create a client
   policy naming the federation requester client, and SHALL attach that policy to
   that client's `token-exchange` scope permission on the `realm-management`
   resource server. Without it the target-client-level exchange-authorization check
   denies the exchange.

Both grants SHALL generalize the existing fgap-v1 building blocks
(`EnsureE2ETokenExchange` / `enableTokenExchangePermissions`) from the fixed
`hypershell-e2e` requester to the CI federation requester client, rather than
duplicating that flow. The grants SHALL be scoped to the requester client: only
the named federation client -- not any client -- SHALL be authorized to exchange
onto the IdP and onto the gateway client. This fixes the per-requester
exchange-scoping behavior the reconciler previously left unresolved. Both grants
SHALL be idempotent: a repeated reconcile SHALL reuse an existing permission, scope
permission, and client policy rather than create a duplicate (PLAT-CITR-09).

#### Scenario: PLAT-CITR-10.01 -- Both Grants Attached for the Requester

- GIVEN the control plane in fgap-v1 mode reconciles a `CITrust` for a gateway
- WHEN reconciliation completes
- THEN fine-grained authorization SHALL be enabled on the `CIProvider` IdP and its `token-exchange` scope permission SHALL carry a client policy naming the federation requester client
- AND fine-grained authorization SHALL be enabled on the target gateway client and its `token-exchange` scope permission SHALL carry a client policy naming the federation requester client
- AND a CI token matching the trust SHALL exchange onto the gateway with no manual admin-API step

#### Scenario: PLAT-CITR-10.02 -- Grants Are Idempotent on Re-Run

- GIVEN a `CITrust` whose two fgap-v1 grants are already realized
- WHEN the control plane reconciles the unchanged `CITrust` again
- THEN it SHALL reuse the existing permissions and client policies
- AND it SHALL NOT create a duplicate permission or client policy

#### Scenario: PLAT-CITR-10.03 -- Missing Either Grant Denies the Exchange

- GIVEN only the IdP token-exchange grant is present and the target-client grant is absent
- WHEN a matching CI token is presented for exchange onto the gateway
- THEN Keycloak SHALL deny the exchange at the target-client exchange-authorization check
- AND a complete reconcile SHALL attach both grants so the exchange succeeds

---

### Requirement: PLAT-CITR-11 -- OIDC IdP Type-Claim Check Disabled for Forge Tokens

A GitHub Actions OIDC token carries `typ: "JWT"`, which is not in Keycloak's set of
supported token types, so the IdP's type-claim check SHALL be disabled
(`disableTypeClaimCheck = true`) on the `CIProvider` IdP or the forge token is
rejected before any exchange-authorization gate is reached. This attribute is owned
**declaratively by the gitops Keycloak IdP configuration** (the Terraform IdP
module), not minted per `CITrust` by the control plane, consistent with the
IdP-adoption rule (PLAT-CITR-01): the control plane SHALL adopt the gitops-owned
IdP and SHALL NOT strip or override this attribute. When the control plane itself
creates the IdP (no gitops-provisioned IdP exists to adopt), it SHALL set
`disableTypeClaimCheck = true` so forge tokens validate.

#### Scenario: PLAT-CITR-11.01 -- Forge Token Type Claim Does Not Block Exchange

- GIVEN a `CIProvider` IdP for GitHub Actions whose tokens carry `typ: "JWT"`
- WHEN a CI token is presented for token exchange
- THEN the IdP SHALL NOT reject the token for its type claim
- AND the exchange SHALL proceed to the token-exchange authorization gates

#### Scenario: PLAT-CITR-11.02 -- Adopted IdP Attribute Is Preserved

- GIVEN a gitops-provisioned `github-actions` IdP with `disableTypeClaimCheck = true`
- WHEN the control plane reconciles a `CIProvider` that adopts it
- THEN the control plane SHALL NOT strip or override `disableTypeClaimCheck`
- AND the attribute SHALL remain set after reconciliation

## Security analysis

- **Tenancy is the gate** (PLAT-CITR-03): a tenant can only enable CI on a
  gateway they manage (`gateway:owner` or `gateway:admin`, or `platform:admin`),
  and never escalate beyond their own rights. Naming another party's repository
  at most lets that repository act on the declarer's own gateway (self-harm),
  never a cross-tenant breach.
- **Subject predicate is the trust boundary** (PLAT-CITR-04): the
  `repository` / `repository_owner` claim match (plus any tighter claims) is the
  real gate. Audience pinning only blocks replay of tokens minted for other
  relying parties; it is necessary but not sufficient, because any repository on
  the forge can mint a token carrying the pinned audience.
- **Tenant isolation** is structural: the forge-namespaced workspace is derived
  from immutable owner/namespace claims and enforced by the gateway, not by
  policy a tenant can edit.
- **Least privilege and default-deny**: no platform or gateway-creator roles; a
  session with no matched workspace role is rejected.
- **No secret** is ever stored or rendered, and the exchange requester is a public
  (keyless) client (PLAT-CITR-08); the only credential is the per-run CI OIDC
  token, short-lived and audience-pinned.
- **Blast radius** of a compromised tenant CI is one gateway workspace.
- **Quotas**: the derived workspace SHALL carry an OpenShell resource quota
  (concurrent sandboxes, CPU/mem, wall-clock) so an untrusted CI cannot exhaust
  the cluster. See the gitops `ci-oidc-federation.spec.md` provisioning section.

## CI side

The exact per-forge snippets (GitHub `id-token: write` + `getIDToken`; GitLab
`id_tokens:`), the token-exchange request shape, and the dispatch asymmetry
(GitHub webhook-native vs GitLab poll/cron, Fullsend ADR 0067) are specified in
the gitops `ci-oidc-federation.spec.md`. A Ready `CITrust` emits the
forge-appropriate snippet in `status.workflow_hint`, so a tenant does not author
it by hand.
