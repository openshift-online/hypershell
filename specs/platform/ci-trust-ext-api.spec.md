# CI Trust Extension API - self-service keyless CI on a gateway

**Date:** 2026-10-26
**Status:** Draft
**Related:** [`data-model.spec.md`](./data-model.spec.md) (CIProvider / CITrust kinds, PLAT-DATA-24/25), gitops `specs/ci-oidc-federation.spec.md` (platform Keycloak federation design), [`control-plane.spec.md`](./control-plane.spec.md) (reconciler), `gateway-access-management.spec.md` (GTW-ACC Keycloak role bridge), [`agent-workspace-gateway-api.spec.md`](./agent-workspace-gateway-api.spec.md) (workspace scoping)

## Overview

A HyperShell user enables their own CI (GitHub Actions or GitLab CI/CD) to run
Fullsend agents against a gateway they own, with **no stored secret**. The CI job
mints its native OIDC token, exchanges it at the cluster Keycloak, and receives a
short-lived OpenShell access token scoped to a workspace on that gateway.

This spec defines the reconcile contract and security model for the two extension
kinds whose data model lives in [`data-model.spec.md`](./data-model.spec.md):

- **CIProvider** (PLAT-DATA-24): platform-admin registration of a CI OIDC issuer.
  Realized as a validation-only Keycloak identity provider.
- **CITrust** (PLAT-DATA-25): a gateway owner's declaration that CI matching a
  subject claim predicate may federate into that gateway. Realized as a scoped
  Keycloak claim-to-workspace mapping, a token-exchange permission, and a gateway
  RBAC binding.

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

The control plane already provisions Keycloak clients, protocol mappers, and
token-exchange permissions programmatically for gateways and the e2e client
(`ProvisionGatewayClient`, `createProtocolMappers`, `EnsureE2ETokenExchange` /
`enableTokenExchangePermissions` in
`components/control-plane/internal/keycloak/client.go`). CIProvider/CITrust
reconciliation extends that proven path; it does not add a new subsystem.

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

#### Scenario: PLAT-CITR-01.01 -- JWKS Unreachable Holds Not-Ready

- GIVEN a `CIProvider` whose `jwks_url` the cluster Keycloak cannot reach
- WHEN the control plane reconciles it
- THEN the `CIProvider` SHALL NOT be marked Ready
- AND its status SHALL name the unreachable JWKS endpoint

---

### Requirement: PLAT-CITR-02 -- CITrust Realization

For a Ready `CITrust`, the control plane SHALL realize three things and nothing
secret:

1. A **scoped claim-to-workspace mapping** on the referenced `CIProvider` IdP that
   accepts only tokens whose claims match the compiled `subject` predicate and
   stamps the derived workspace (`gh-<owner>` for GitHub, `gl-<namespace>` for
   GitLab) into the issued token.
2. A **token-exchange permission** from the shared federation client onto the
   target gateway's audience, so the exchange grant can mint a gateway-scoped
   token.
3. A **gateway RBAC binding** granting `role` in the derived workspace on the
   referenced `Gateway`.

The control plane SHALL render no raw credential, PEM, or `Secret` for a
`CITrust`. Deleting a `CITrust` SHALL remove all three realized artifacts.

#### Scenario: PLAT-CITR-02.01 -- Forge-Namespaced Workspace Derivation

- GIVEN a `CITrust` referencing a `github` `CIProvider` with `subject.repository = acme/app`
- AND a second `CITrust` referencing a `gitlab` `CIProvider` with `subject.project_path = acme/app`
- WHEN both are reconciled
- THEN the first SHALL map to workspace `gh-acme` and the second to `gl-acme`
- AND the two SHALL be isolated workspaces with independent RBAC

---

### Requirement: PLAT-CITR-03 -- Gateway Ownership and Role Bound

The API server SHALL reject a `CITrust` create or update unless the caller owns
the referenced `Gateway`, with 403. The API server SHALL reject a `CITrust` whose
`role` exceeds the caller's own rights on that gateway, with 403. A `CITrust`
therefore can only grant CI access to a gateway the caller controls, and never
more authority than the caller holds.

#### Scenario: PLAT-CITR-03.01 -- Cross-Tenant Grant Impossible

- GIVEN caller A owns gateway `gw-a` and caller B owns gateway `gw-b`
- WHEN caller A creates a `CITrust` referencing `gw-b`
- THEN the API server SHALL reject it with 403
- AND no Keycloak mapping or RBAC binding SHALL be created for `gw-b`

---

### Requirement: PLAT-CITR-04 -- Audience Pinning and Default-Deny

The federated audience SHALL come from `CIProvider.audience` and SHALL NOT be
settable on a `CITrust`. A token whose `aud` does not equal the pinned audience
SHALL be rejected at exchange. Claims that do not match any `CITrust` subject
predicate SHALL yield no roles; the gateway SHALL reject a session that carries no
workspace role (default-deny).

#### Scenario: PLAT-CITR-04.01 -- Wrong Audience Rejected

- GIVEN a CI token minted with an audience other than the provider's pinned `audience`
- WHEN it is presented for token exchange
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

## Security analysis

- **Ownership is the gate** (PLAT-CITR-03): a tenant can only enable CI on a
  gateway they own, and never escalate beyond their own rights. Naming another
  party's repository at most lets that repository act on the declarer's own
  gateway (self-harm), never a cross-tenant breach.
- **Audience pinning** (PLAT-CITR-04) blocks replay of tokens minted for other
  relying parties.
- **Tenant isolation** is structural: the forge-namespaced workspace is derived
  from immutable owner/namespace claims and enforced by the gateway, not by
  policy a tenant can edit.
- **Least privilege and default-deny**: no platform or gateway-creator roles; a
  session with no matched workspace role is rejected.
- **No secret** is ever stored or rendered; the only credential is the per-run CI
  OIDC token, short-lived and audience-pinned.
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
