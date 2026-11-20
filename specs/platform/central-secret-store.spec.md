# Central Secret Store (OIDC-secured, ExternalSecrets-style consumption)

**ID prefix:** PLAT-SSS

**Status:** Draft
**Related:** [`agent-secret-late-binding.spec.md`](./agent-secret-late-binding.spec.md) (how the control plane consumes this store to realize OpenShell providers; SecretSource reference model), [`openshell-gateway-credentials.spec.md`](./openshell-gateway-credentials.spec.md) (OSG-CRED, the per-gateway OpenShell runtime store that holds the derived provider copy), [`ci-trust-ext-api.spec.md`](./ci-trust-ext-api.spec.md) (PLAT-CITR, the keyless OIDC token-exchange this reuses for machine and CI auth), [`../security/rbac-enforcement.spec.md`](../security/rbac-enforcement.spec.md) (SEC-RBAC, api-server passthrough authorization), [`../standards/security/security.spec.md`](../standards/security/security.spec.md) (SEC-STD, secret-plane standards), [`data-model.spec.md`](./data-model.spec.md) (SecretSource reference). GitOps counterpart: `hypershell-gitops` stands up the central Vault and its Red Hat SSO OIDC auth, path policies, and audit device.

## Purpose

End users need one place to store their own secrets and have HyperShell reconcile
them onto the gateways and agents that consume them, without binding every other
project to HyperShell as its secret broker. This spec defines a **central,
OIDC-secured secret store** that is a standalone shared service (not HyperShell-owned
per tenant), into which users write their own material under identity-scoped paths,
and which HyperShell consumes as a least-privilege reader in the **External Secrets
Operator (ESO) pattern**: read from a source store, deliver to a destination.

The store is the single **source of truth** for user-provided durable material. It is
distinct from, and upstream of, the per-gateway OpenShell runtime store
([`openshell-gateway-credentials.spec.md`](./openshell-gateway-credentials.spec.md)),
which holds a **derived copy** once a secret is realized as an OpenShell provider on
the cluster running the gateway. These are two different stores in a source-to-copy
relationship, and SHALL NOT be conflated.

This supersedes the per-cluster, late-bound, "material never transits the hub,
every read is cluster-local" model that
[`agent-secret-late-binding.spec.md`](./agent-secret-late-binding.spec.md) previously
defined: there is now one central source store that consumers read, including across
clusters, exactly as ESO reads a central store from any cluster.

## Requirements

### Requirement: PLAT-SSS-01 -- Central OIDC-Secured Secret Store Service

The platform SHALL provide a **central** secret store (a single logical Vault per hub
fleet, not a per-tenant or per-cluster instance) secured by OIDC through Red Hat SSO.
The store SHALL be a standalone shared service: HyperShell is one consumer among
others (for example literal ExternalSecrets Operator for ordinary Kubernetes
consumers, and other projects), and the store SHALL NOT require HyperShell to operate
or own it. Its address, OIDC issuer, and auth mounts are deployment configuration
owned by the GitOps counterpart, not HyperShell code (separate configuration from
code).

#### Scenario: PLAT-SSS-01.01 -- One central store, multiple independent consumers

- GIVEN a hub fleet with a central secret store secured by Red Hat SSO OIDC
- WHEN HyperShell, a literal ExternalSecrets Operator, and another project each authenticate to it
- THEN each SHALL authenticate as its own distinct identity and resolve only the paths its policy allows
- AND no consumer SHALL depend on HyperShell to broker its access

### Requirement: PLAT-SSS-02 -- Human Upload via OIDC to Identity-Scoped Paths

A human SHALL upload secret material by authenticating to the store with their own Red
Hat SSO/OIDC identity and writing only under identity- or team-scoped paths authorized
to them. The write grant SHALL be **upload-only** by default (`create`/`update`
capability, without `read` or `list`) so an uploader cannot read back or enumerate
other material. Authorization SHALL map OIDC claims (for example SSO group membership)
to store policies, so path scope follows identity and requires no store-administrator
action per user. No human SHALL need a store-root token, a HyperShell
platform-administrator identity, or a shared writer identity to upload their own
secret.

#### Scenario: PLAT-SSS-02.01 -- User writes only to their own path

- GIVEN a user authenticated to the store with their Red Hat SSO identity mapped to team `foo`
- WHEN the user writes a secret under team `foo`'s path prefix
- THEN the write SHALL succeed
- AND a write under another team's prefix SHALL be denied
- AND the user SHALL NOT be able to read or list secrets they did not create (upload-only grant)

### Requirement: PLAT-SSS-03 -- Least-Privilege Machine Consumption

A machine consumer (the HyperShell control plane, literal ESO, or another project)
SHALL authenticate to the store with its OWN non-human identity (for example Kubernetes
auth for an in-cluster workload, or an OIDC/JWT identity reusing the keyless exchange of
[`ci-trust-ext-api.spec.md`](./ci-trust-ext-api.spec.md)), never a human user's token.
Each machine identity SHALL be granted **read-only** access scoped to only the paths it
must resolve, and SHALL NOT be granted write access to user material or administrative
access to the store. A machine consumer SHALL be able to read the central store across
clusters (a consumer on one cluster MAY read the central store served from the hub),
which is the ESO source-store read pattern.

#### Scenario: PLAT-SSS-03.01 -- Control plane reads the central store cross-cluster, read-only and scoped

- GIVEN a control plane on a spoke cluster reconciling a gateway whose user secret lives in the central hub store
- WHEN it resolves the referenced material
- THEN it SHALL authenticate to the central store with its own machine identity and read only the referenced path
- AND the cross-cluster read SHALL be permitted (there is no requirement that the read be cluster-local)
- AND the control-plane identity SHALL NOT be able to write user material or administer the store

### Requirement: PLAT-SSS-04 -- HyperShell Consumes the Store as an ExternalSecrets-Style Syncer

The HyperShell control plane SHALL consume the central store in the ExternalSecrets
pattern: it reads a referenced secret from the store (the source) with its
least-privilege reader identity and delivers it to the destination that actually
consumes it. For an agent or gateway credential the destination is an **OpenShell
provider** realized over the gateway API
([`agent-secret-late-binding.spec.md`](./agent-secret-late-binding.spec.md),
[`agent-workspace-gateway-api.spec.md`](./agent-workspace-gateway-api.spec.md)), NOT a
rendered Kubernetes `Secret`. The reconcile SHALL be idempotent and self-healing: a
referenced secret not yet present in the store SHALL hold the dependent resource in a
pending state (SecretPending, AGT-SEC-06) rather than fail, and SHALL converge the
moment the material appears.

The same central store MAY also serve literal ExternalSecrets Operator `ExternalSecret`
resources for ordinary Kubernetes consumers and other projects; that delivery path is
ESO's own and is out of scope for HyperShell's syncer.

#### Scenario: PLAT-SSS-04.01 -- Control plane syncs a central-store secret into an OpenShell provider

- GIVEN a user has uploaded material for a referenced secret into the central store
- WHEN the control plane reconciles the owning resource
- THEN it SHALL read the material from the central store and realize it as an OpenShell provider on the gateway/workspace
- AND it SHALL NOT render the credential as a raw Kubernetes `Secret`

#### Scenario: PLAT-SSS-04.02 -- Missing material holds pending, converges on upload

- GIVEN a referenced secret whose material has not yet been uploaded to the central store
- WHEN the control plane reconciles the owning resource
- THEN it SHALL set SecretPending and SHALL NOT realize the provider with empty material
- AND when the user later uploads the material, the next reconcile SHALL realize the provider and clear SecretPending with no manual trigger

### Requirement: PLAT-SSS-05 -- Source Store and Derived Provider Copy Are Distinct

The central store (the user source of truth, Store 1) and the per-gateway OpenShell
runtime store that holds the realized provider credential (Store 2,
[`openshell-gateway-credentials.spec.md`](./openshell-gateway-credentials.spec.md))
SHALL be distinct stores in a source-to-copy relationship. When the control plane
realizes a provider, OpenShell persists the derived credential into Store 2 on the
cluster running the gateway; that derived copy SHALL NOT be treated as the user's
upload location, and the user SHALL never be directed to write into Store 2. Deleting
or rotating the source material in Store 1 SHALL be reconciled forward into the derived
copy through the normal provider realization and rotation path, not by editing Store 2
directly.

#### Scenario: PLAT-SSS-05.01 -- Derived copy lives in the per-gateway store, not the user path

- GIVEN a user secret uploaded to the central store and realized as an OpenShell provider
- WHEN the provider is created on the gateway
- THEN OpenShell SHALL hold the derived credential in its per-gateway runtime store (Store 2) on the gateway's cluster
- AND the user's upload location SHALL remain the central store path (Store 1), never the per-gateway store

### Requirement: PLAT-SSS-06 -- Control-Plane Exposure Minimization

The control plane's handling of material read from the central store SHALL minimize
plaintext exposure. The baseline SHALL be a read-only identity scoped to only the
reconciled paths (PLAT-SSS-03), with the material never written to a log, error
string, telemetry, Kubernetes Event, or API response. The store SHOULD additionally
support reducing or removing control-plane plaintext handling where the backend allows
it, for example signing a GitHub App assertion through a store transit/sign capability
so the control plane never holds the App private key, or issuing a downscoped,
short-lived delegated read for a user-bound secret. These stronger controls are
permitted enhancements over the read-only baseline and SHALL NOT be required for the
baseline to be conformant.

#### Scenario: PLAT-SSS-06.01 -- Material never leaks through logs or responses

- GIVEN the control plane reads material from the central store to realize a provider
- WHEN it logs progress, sets status, or returns an API response
- THEN none of these SHALL contain the material
- AND the control-plane reader identity SHALL be scoped to only the reconciled paths

### Requirement: PLAT-SSS-07 -- Audit and Path Isolation

The central store SHALL enable an audit device so every access (human upload, machine
read, administrative action) is recorded with the acting identity, path, and action.
Path isolation SHALL be structural: one tenant's or user's policy SHALL NOT grant
access to another's path prefix, and a leaked reference to one path SHALL NOT be usable
to read or enumerate another. The audit trail SHALL tie a machine read performed on a
user's behalf back to the originating user where a delegated identity is used
(PLAT-SSS-06).

#### Scenario: PLAT-SSS-07.01 -- Cross-tenant path access is denied and audited

- GIVEN two teams with distinct path prefixes in the central store
- WHEN a principal scoped to team A attempts to read team B's path
- THEN the store SHALL deny it
- AND the denied attempt SHALL appear in the audit device with the acting identity, path, and action

### Requirement: PLAT-SSS-08 -- Secret Plane Map

The platform SHALL maintain three non-conflated secret planes, and this spec SHALL own
the first:

1. **Central user-secret store (this spec, Store 1).** The OIDC-secured shared source
   of truth for user-uploaded material.
2. **Per-gateway OpenShell runtime store (Store 2,
   [`openshell-gateway-credentials.spec.md`](./openshell-gateway-credentials.spec.md)).**
   The derived provider-credential copy on the cluster running the gateway.
3. **Application-secret plane.** Platform/application secrets managed by Secrets Manager
   plus ExternalSecrets Operator.

The planes SHALL NOT share identities, policies, or paths such that one plane's
compromise crosses into another. The central store (plane 1) MAY be read by literal ESO
to deliver plane-3-style Kubernetes secrets for other consumers; that does not merge
the planes' HyperShell-facing contracts.

#### Scenario: PLAT-SSS-08.01 -- The three planes stay isolated

- GIVEN the central user-secret store, the per-gateway OpenShell runtime store, and the application-secret plane
- WHEN an identity is granted access to one plane
- THEN that grant SHALL NOT confer access to another plane's paths or identities

## Design Decisions

| Decision | Rationale |
|----------|-----------|
| One central OIDC store, not per-cluster HyperShell Vaults | A single shared, standalone store lets users self-serve their secrets and lets other projects (and literal ESO) consume the same source, decoupling "store my secret" from "run HyperShell." It reverses the prior per-cluster, cluster-local-only model whose purpose was blast-radius isolation; that isolation is preserved instead by OIDC path-scoping, least-privilege consumers, and an audit device. |
| Users authenticate with their own OIDC identity, upload-only | The uploader is a person; binding the write grant to their SSO identity (not a shared HyperShell writer) means no admin is in the per-user upload path, and upload-only (no read/list) keeps an uploader from reading or enumerating others' material. |
| HyperShell consumes as an ESO-style syncer, machine identity, read-only | The reader is a machine with a different lifetime and trust than the human uploader; a scoped read-only machine identity (never the user's token) is least privilege and matches how ESO reads a central source store from any cluster. |
| Source store distinct from the derived provider copy | The user's source of truth (Store 1) and OpenShell's runtime provider copy (Store 2) are different instances with different owners; modeling them as source-to-copy prevents directing users at the wrong store and keeps rotation flowing forward through provider realization. |
| Exposure minimization as a roadmap, not a gate | Read-only scoped access is the conformant baseline; transit-sign (never hold the GitHub App PEM) and delegated downscoped reads are permitted enhancements so the baseline can ship while exposure shrinks over time. |
