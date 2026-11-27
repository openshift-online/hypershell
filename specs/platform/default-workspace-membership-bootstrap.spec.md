# Default Workspace Membership Bootstrap Specification

**Date:** 2026-11-09
**Status:** Draft
**Tracks:** [Issue #1000](https://github.com/openshift-online/hypershell/issues/1000) - One-shot automatic default-workspace membership bootstrap
**Parent:** `gateway-access-management.spec.md` and `openshell-gateway-service-accounts.spec.md`
**Related:** [Issue #976](https://github.com/openshift-online/hypershell/issues/976) - default-workspace UI feedback and opt-out; `openshell-gateway-keycloak.spec.md`, `openshell-gateway-oidc.spec.md`, `security/rbac-enforcement.spec.md`, and `registered-users.spec.md`

---

## Purpose

This specification defines the one-time bootstrap that adds eligible identities to a gateway's reserved OpenShell `default` workspace.

HyperShell performs the bootstrap during the originating gateway-access grant or OpenShellGatewayServiceAccount creation request. After that request completes, HyperShell does not own, reconcile, repair, or remove OpenShell workspace membership.

This specification supersedes the desired-state and control-plane reconciliation approach previously proposed for default-workspace membership. It does not change how administrators manage other OpenShell workspaces.

## Terminology

- **Gateway bootstrap admin** is a persistent HyperShell-owned automation identity scoped to one gateway. The identity is not a user login.
- **Eligible human user** is a user granted the HyperShell gateway `user` tier, which maps to the OpenShell `openshell-user` role.
- **Eligible service account** is an OpenShellGatewayServiceAccount created with the `openshell-user` role.
- **Originating request** is the gateway-access grant or OpenShellGatewayServiceAccount creation request that triggers the bootstrap.
- **One-shot bootstrap** is the synchronous OpenShell `AddMember` call made during the originating request.

## Product Decisions

| Question                                                          | Decision                                                                                                                                                                                             |
| ----------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Is workspace membership desired state?                            | No. HyperShell makes one `AddMember` call and does not reconcile membership afterward.                                                                                                               |
| Which workspace receives the bootstrap?                           | Only the gateway's reserved OpenShell `default` workspace.                                                                                                                                           |
| Which identities are eligible?                                    | Human users granted the gateway `user` tier and OpenShellGatewayServiceAccounts created with `openshell-user`, unless the originating request explicitly opts out.                                   |
| Which identities are excluded?                                    | Gateway owners, gateway admins, and `openshell-admin` service accounts.                                                                                                                              |
| Which credentials call OpenShell?                                 | The gateway's persistent HyperShell-owned bootstrap admin identity.                                                                                                                                  |
| Can a caller opt out of provisioning the gateway bootstrap admin? | No. Every gateway receives a persistent bootstrap admin. The opt-out applies only to default-workspace membership for an eligible user or `openshell-user` service account.                          |
| What happens if the identity is already a member?                 | HyperShell treats the OpenShell response as success.                                                                                                                                                 |
| What happens if OpenShell rejects the add?                        | The originating request fails and does not report the access grant or service account as ready.                                                                                                      |
| Does HyperShell remove membership later?                          | No. Revoke, role change, expiration, service-account deletion, and manual OpenShell changes do not trigger HyperShell membership removal.                                                            |
| How are existing gateways handled?                                | The control plane may lazily provision a missing gateway bootstrap admin and publish readiness; unmarked gateways retain pre-bootstrap request behavior.                                             |
| How are mixed-version rollouts handled?                           | Only a gateway whose `DefaultWorkspaceBootstrapReady` provisioning condition is complete participates in bootstrap; unmarked gateways skip the one-shot add without failing the originating request. |
| Is gateway deletion cleanup blocking?                             | No. Gateway deletion attempts cleanup and records a failure if OpenShell or the identity provider is unavailable.                                                                                    |

## Requirements

### Requirement: GTW-BOOT-01 -- Gateway Bootstrap Admin Identity

HyperShell SHALL provision one persistent gateway bootstrap admin for each gateway. The gateway bootstrap admin SHALL be scoped to exactly one gateway and SHALL NOT be usable as a human login.

Gateway creation SHALL provision the gateway bootstrap admin and mark the gateway's `DefaultWorkspaceBootstrapReady` provisioning condition complete. If an existing gateway has no gateway bootstrap admin, the control plane MAY provision the missing identity and publish the same condition.

The API server SHALL NOT call OpenShell `AddMember` unless the gateway's `DefaultWorkspaceBootstrapReady` condition is complete. A gateway without that completed condition SHALL skip default-workspace bootstrap while still allowing eligible gateway-access grants and OpenShellGatewayServiceAccount creation to succeed. This compatibility rule prevents an API server rollout from failing requests for gateways that have not yet been marked ready by a control plane that supports the bootstrap contract.

HyperShell SHALL reuse the same gateway bootstrap admin for later default-workspace bootstrap calls on that gateway. HyperShell SHALL NOT create a new ephemeral admin identity for each membership add.

The gateway bootstrap admin SHALL be scoped to exactly one gateway. Version 1 uses the gateway-scoped OpenShell admin role required to call `AddMember` for the gateway's `default` workspace. Narrowing that role to a dedicated workspace-admin privilege is a follow-up OpenShell contract change; until then, HyperShell SHALL document and review this scoped-admin trade-off.

#### Scenario: GTW-BOOT-01.01 -- New gateway receives a bootstrap admin

- GIVEN a caller creates a gateway
- WHEN gateway creation succeeds
- THEN HyperShell SHALL provision exactly one gateway bootstrap admin scoped to that gateway
- AND the gateway bootstrap admin SHALL NOT be represented as a human User

#### Scenario: GTW-BOOT-01.02 -- Existing gateway receives a bootstrap admin lazily

- GIVEN a gateway existed before this specification
- AND the gateway has no gateway bootstrap admin
- WHEN the control plane reconciles that gateway
- THEN HyperShell MAY provision the missing gateway bootstrap admin
- AND HyperShell SHALL publish `DefaultWorkspaceBootstrapReady` only after that identity is available

#### Scenario: GTW-BOOT-01.04 -- Unmarked gateway remains compatible

- GIVEN a gateway does not have a completed `DefaultWorkspaceBootstrapReady` condition
- WHEN an eligible gateway-access grant or OpenShellGatewayServiceAccount creation is made for that gateway
- THEN HyperShell SHALL NOT call OpenShell `AddMember`
- AND the originating request SHALL retain its pre-bootstrap success behavior

#### Scenario: GTW-BOOT-01.03 -- Later grants reuse the bootstrap admin

- GIVEN a gateway has a gateway bootstrap admin
- WHEN two eligible originating requests occur for that gateway
- THEN both `AddMember` calls SHALL authenticate as the same gateway bootstrap admin

### Requirement: GTW-BOOT-02 -- Bootstrap Admin Security

HyperShell SHALL NOT expose the gateway bootstrap admin credential through the public HyperShell API, CLI, console, SDK, or logs.

HyperShell SHALL NOT use the caller's credentials or the target identity's credentials for the OpenShell `AddMember` call.

Audit records and error messages SHALL NOT contain the gateway bootstrap admin credential.

#### Scenario: GTW-BOOT-02.01 -- Public API does not expose the credential

- GIVEN a gateway has a gateway bootstrap admin
- WHEN any public HyperShell API response describes the gateway or its bootstrap result
- THEN the response SHALL NOT contain the gateway bootstrap admin credential

#### Scenario: GTW-BOOT-02.02 -- Logs do not expose the credential

- GIVEN an eligible originating request triggers the gateway bootstrap admin
- WHEN HyperShell records an audit event, log, metric label, or error
- THEN the recorded value SHALL NOT contain the gateway bootstrap admin credential

### Requirement: GTW-BOOT-03 -- Human User Grant Bootstrap

When a caller grants a user the HyperShell gateway `user` tier, HyperShell SHALL resolve the user's stable OIDC subject claim and call OpenShell `v1 AddMember` for the gateway's `default` workspace with role `user`.

The gateway-access grant API MAY set `skip_default_workspace_bootstrap` to `true`. When set, HyperShell SHALL NOT call `AddMember` for that grant.

HyperShell SHALL NOT use the user's mutable username as the OpenShell membership subject.

HyperShell SHALL NOT call `AddMember` when the granted tier maps to `openshell-admin`.

For a grant that has not opted out, the originating request SHALL NOT return success unless `AddMember` succeeds or OpenShell reports that the subject is already a member of the `default` workspace.

This requirement applies only when the gateway has a completed `DefaultWorkspaceBootstrapReady` provisioning condition. The grant SHALL succeed without an `AddMember` call when that condition is absent or incomplete.

#### Scenario: GTW-BOOT-03.01 -- Viewer grant bootstraps default workspace membership

- GIVEN a gateway has a gateway bootstrap admin
- WHEN an authorized caller grants user B the gateway `user` tier
- THEN HyperShell SHALL call OpenShell `AddMember` for gateway `default`
- AND the member subject SHALL be user B's stable OIDC `sub`
- AND the member role SHALL be `user`

#### Scenario: GTW-BOOT-03.02 -- Admin grant does not bootstrap membership

- GIVEN a gateway has a gateway bootstrap admin
- WHEN an authorized caller grants user C the gateway `admin` tier
- THEN HyperShell SHALL NOT call `AddMember` for user C

#### Scenario: GTW-BOOT-03.03 -- Existing member is success

- GIVEN user B already belongs to the gateway's `default` workspace with role `user`
- WHEN an authorized caller grants user B the gateway `user` tier
- THEN HyperShell SHALL treat the OpenShell response as a successful bootstrap
- AND the originating request SHALL return success

#### Scenario: GTW-BOOT-03.04 -- OpenShell rejection fails the grant

- GIVEN a gateway has a gateway bootstrap admin
- WHEN an authorized caller grants user B the gateway `user` tier
- AND OpenShell rejects the `AddMember` call
- THEN the originating request SHALL return an error
- AND the response SHALL NOT present user B's gateway access as ready

#### Scenario: GTW-BOOT-03.05 -- Grant opts out of default-workspace bootstrap

- GIVEN a gateway has a gateway bootstrap admin
- WHEN an authorized caller grants user D the gateway `user` tier with `skip_default_workspace_bootstrap` set to `true`
- THEN HyperShell SHALL NOT call `AddMember` for user D
- AND the gateway access grant SHALL succeed

### Requirement: GTW-BOOT-04 -- Service Account Creation Bootstrap

When a caller creates an OpenShellGatewayServiceAccount with role `openshell-user`, HyperShell SHALL call OpenShell `v1 AddMember` for the gateway's `default` workspace with role `user` and the service account's stable subject.

The service-account create API MAY set `skip_default_workspace_bootstrap` to `true`. When set, HyperShell SHALL NOT call `AddMember` for that service account.

HyperShell SHALL NOT call `AddMember` for an OpenShellGatewayServiceAccount created with role `openshell-admin`.

The originating creation request SHALL NOT return success unless `AddMember` succeeds or OpenShell reports that the service-account subject is already a member of the `default` workspace.

#### Scenario: GTW-BOOT-04.01 -- openshell-user service account is added to default

- GIVEN a gateway has a gateway bootstrap admin
- WHEN a caller creates an OpenShellGatewayServiceAccount with role `openshell-user`
- THEN HyperShell SHALL call OpenShell `AddMember` for gateway `default`
- AND the member subject SHALL be the service account's stable subject
- AND the member role SHALL be `user`

#### Scenario: GTW-BOOT-04.02 -- openshell-admin service account is not added

- GIVEN a gateway has a gateway bootstrap admin
- WHEN a caller creates an OpenShellGatewayServiceAccount with role `openshell-admin`
- THEN HyperShell SHALL NOT call `AddMember` for that service account

#### Scenario: GTW-BOOT-04.03 -- OpenShell rejection fails service-account creation

- GIVEN a gateway has a gateway bootstrap admin
- WHEN a caller creates an OpenShellGatewayServiceAccount with role `openshell-user`
- AND OpenShell rejects the `AddMember` call
- THEN the creation request SHALL return an error
- AND the response SHALL NOT present the service account as ready

#### Scenario: GTW-BOOT-04.04 -- Service-account creation opts out of default-workspace bootstrap

- GIVEN a gateway has a gateway bootstrap admin
- WHEN a caller creates an OpenShellGatewayServiceAccount with role `openshell-user` and `skip_default_workspace_bootstrap` set to `true`
- THEN HyperShell SHALL NOT call `AddMember` for that service account
- AND the creation request SHALL succeed

### Requirement: GTW-BOOT-05 -- One-Shot Semantics

HyperShell SHALL perform the default-workspace bootstrap as part of the originating request. HyperShell SHALL NOT create a `DefaultWorkspaceMembership` desired-state resource.

The API server SHALL use the gateway's completed `DefaultWorkspaceBootstrapReady` provisioning condition as the deployment compatibility boundary. A missing or incomplete condition means the gateway is not yet enrolled in default-workspace bootstrap; the originating request SHALL skip bootstrap rather than fail.

HyperShell SHALL NOT run a background reconciler for default-workspace membership. HyperShell SHALL NOT repair a manually removed member, and HyperShell SHALL NOT remove membership when gateway access is revoked, a role changes, a service account expires or is deleted, or a gateway bootstrap admin is deleted.

#### Scenario: GTW-BOOT-05.01 -- Manual removal is not repaired

- GIVEN user B was successfully added to the gateway's `default` workspace
- WHEN an OpenShell administrator removes user B from that workspace
- THEN HyperShell SHALL NOT add user B again

#### Scenario: GTW-BOOT-05.02 -- Revoke does not remove membership

- GIVEN user B was successfully added to the gateway's `default` workspace
- WHEN an authorized caller revokes user B's gateway access
- THEN HyperShell SHALL NOT remove user B from the `default` workspace

#### Scenario: GTW-BOOT-05.03 -- Service-account deletion does not remove membership

- GIVEN an `openshell-user` service account was successfully added to the gateway's `default` workspace
- WHEN an authorized caller deletes that service account
- THEN HyperShell SHALL NOT remove the service-account subject from the `default` workspace

### Requirement: GTW-BOOT-06 -- Gateway Lifecycle Cleanup

Gateway deletion SHALL trigger cleanup of the gateway bootstrap admin. If cleanup fails because OpenShell or the identity provider is unavailable, gateway deletion SHALL continue and HyperShell SHALL record a cleanup-failure audit event.

Cleanup failure SHALL NOT create a new gateway bootstrap admin or retry default-workspace membership operations.

#### Scenario: GTW-BOOT-06.01 -- Gateway deletion cleans up the bootstrap admin

- GIVEN a gateway has a gateway bootstrap admin
- WHEN an authorized caller deletes the gateway
- THEN HyperShell SHALL attempt to delete the gateway bootstrap admin

#### Scenario: GTW-BOOT-06.02 -- Cleanup failure is audited

- GIVEN a gateway has a gateway bootstrap admin
- AND OpenShell or the identity provider is unavailable
- WHEN an authorized caller deletes the gateway
- THEN gateway deletion SHALL continue
- AND HyperShell SHALL record an audit event identifying the cleanup failure and gateway

### Requirement: GTW-BOOT-07 -- Audit and Verification

HyperShell SHALL audit each default-workspace bootstrap attempt with:

- Gateway ID
- Subject
- Subject type, either human user or OpenShellGatewayServiceAccount
- OpenShell role `user`
- Outcome
- Correlation ID

HyperShell SHALL audit bootstrap-admin creation and gateway-deletion cleanup failures. Bootstrap-admin creation SHALL be recorded as a Kubernetes Event in the gateway namespace. Gateway-deletion cleanup failures SHALL continue to be recorded through the existing Keycloak-client cleanup orphan Event. Audit records SHALL NOT contain the gateway bootstrap admin credential.

Tests SHALL cover:

- Human `user` grant bootstrap
- Grant and service-account opt-out
- `openshell-user` service-account creation bootstrap
- `openshell-admin` exclusion
- Existing-member success
- OpenShell `AddMember` failure
- Stable OIDC subject use for human users
- Credential redaction
- Existing-gateway lazy bootstrap-admin provisioning
- Gateway-deletion cleanup and cleanup failure
- Gateway-scoped workspace visibility enrichment for the access and service-account list APIs

#### Scenario: GTW-BOOT-07.01 -- Successful bootstrap is audited

- GIVEN a gateway has a gateway bootstrap admin
- WHEN an eligible originating request successfully calls `AddMember`
- THEN HyperShell SHALL record an audit event with the gateway ID, subject, subject type, role `user`, success outcome, and correlation ID

#### Scenario: GTW-BOOT-07.02 -- Failed bootstrap is audited

- GIVEN a gateway has a gateway bootstrap admin
- WHEN OpenShell rejects an `AddMember` call
- THEN HyperShell SHALL record an audit event with the gateway ID, subject, subject type, role `user`, failure outcome, correlation ID, and a redacted error

### Requirement: GTW-BOOT-08 -- Gateway Workspace Visibility

The API server SHALL enrich the gateway access list and OpenShellGatewayServiceAccount list responses with a gateway-scoped `workspaces` array. For each user-scope item, the array SHALL contain the name of every workspace in the currently shown gateway to which that identity is attached. The array SHALL NOT filter memberships by the identity's OpenShell workspace-level role.

For owner, gateway-admin, and `openshell-admin` items, the API SHALL return an empty per-item `workspaces` array; the console SHALL display **All workspaces** for those rows. Admin-scope presentation SHALL take precedence over any user-scope membership.

The gateway access list response SHALL also return a gateway-level `workspaces` array containing every workspace name on the gateway, sorted alphabetically. The access list API SHALL accept a `workspace` query filter. When present, it SHALL return user rows attached to that workspace plus owner and gateway-admin rows, and the response total SHALL reflect that filtered set.

The API server SHALL use the gateway bootstrap admin to retrieve workspace memberships from OpenShell. The API server SHALL assemble the list response server-side and SHALL NOT require the web console to call OpenShell directly or issue one lookup per row. The bootstrap-admin credential SHALL NOT appear in a list response, event, log, or error.

If OpenShell cannot return the gateway-scoped membership data, the affected list request SHALL return a retryable error rather than an empty or incomplete `workspaces` array.

#### Scenario: GTW-BOOT-08.01 -- User-scope item lists all attached workspaces

- GIVEN user B has gateway role `user` on gw-1
- AND user B belongs to gw-1 workspaces `default` and `billing`
- WHEN an authorized caller lists gw-1 access
- THEN user B's item SHALL contain `workspaces` with `default` and `billing`

#### Scenario: GTW-BOOT-08.02 -- Admin-scope item returns no workspace list

- GIVEN user C has gateway role `admin` on gw-1 and belongs to gw-1 workspace `default`
- WHEN an authorized caller lists gw-1 access
- THEN user C's item SHALL contain an empty `workspaces` array
- AND the console SHALL display **All workspaces**

#### Scenario: GTW-BOOT-08.03 -- Workspace visibility failure is explicit

- GIVEN OpenShell is unavailable while the API server assembles the gw-1 access list
- WHEN an authorized caller requests that list
- THEN the API SHALL return a retryable error
- AND the response SHALL NOT present an empty `workspaces` array as successful data
