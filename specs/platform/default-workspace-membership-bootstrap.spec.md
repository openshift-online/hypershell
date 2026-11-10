# Default Workspace Membership Bootstrap Specification

**Date:** 2026-11-09
**Status:** Draft
**Tracks:** [Issue #1000](https://github.com/openshift-online/hypershell/issues/1000) - One-shot automatic default-workspace membership bootstrap
**Parent:** `gateway-access-management.spec.md` and `openshell-gateway-service-accounts.spec.md`
**Related:** `openshell-gateway-keycloak.spec.md`, `openshell-gateway-oidc.spec.md`, `security/rbac-enforcement.spec.md`, and `registered-users.spec.md`

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

| Question | Decision |
| --- | --- |
| Is workspace membership desired state? | No. HyperShell makes one `AddMember` call and does not reconcile membership afterward. |
| Which workspace receives the bootstrap? | Only the gateway's reserved OpenShell `default` workspace. |
| Which identities are eligible? | Human users granted the gateway `user` tier and OpenShellGatewayServiceAccounts created with `openshell-user`, unless the gateway-access request explicitly opts out. |
| Which identities are excluded? | Gateway owners, gateway admins, and `openshell-admin` service accounts. |
| Which credentials call OpenShell? | The gateway's persistent HyperShell-owned bootstrap admin identity. |
| What happens if the identity is already a member? | HyperShell treats the OpenShell response as success. |
| What happens if OpenShell rejects the add? | The originating request fails and does not report the access grant or service account as ready. |
| Does HyperShell remove membership later? | No. Revoke, role change, expiration, service-account deletion, and manual OpenShell changes do not trigger HyperShell membership removal. |
| How are existing gateways handled? | The first eligible originating request ensures a missing gateway bootstrap admin before calling `AddMember`. |
| Is gateway deletion cleanup blocking? | No. Gateway deletion attempts cleanup and records a failure if OpenShell or the identity provider is unavailable. |

## Requirements

### Requirement: GTW-BOOT-01 -- Gateway Bootstrap Admin Identity

HyperShell SHALL provision one persistent gateway bootstrap admin for each gateway. The gateway bootstrap admin SHALL be scoped to exactly one gateway and SHALL NOT be usable as a human login.

Gateway creation SHALL provision the gateway bootstrap admin. If an existing gateway has no gateway bootstrap admin, the first eligible originating request SHALL provision the missing identity before calling OpenShell `AddMember`.

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
- WHEN an eligible originating request is made for that gateway
- THEN HyperShell SHALL provision the missing gateway bootstrap admin before calling `AddMember`

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

### Requirement: GTW-BOOT-05 -- One-Shot Semantics

HyperShell SHALL perform the default-workspace bootstrap as part of the originating request. HyperShell SHALL NOT create a `DefaultWorkspaceMembership` desired-state resource.

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
- OpenShell role, either `user` or `admin`
- Outcome
- Correlation ID

HyperShell SHALL audit bootstrap-admin creation and gateway-deletion cleanup failures. Bootstrap-admin creation SHALL be recorded as a Kubernetes Event in the gateway namespace. Gateway-deletion cleanup failures SHALL continue to be recorded through the existing Keycloak-client cleanup orphan Event. Audit records SHALL NOT contain the gateway bootstrap admin credential.

Tests SHALL cover:

- Human `user` grant bootstrap
- `openshell-user` service-account creation bootstrap
- `openshell-admin` exclusion
- Existing-member success
- OpenShell `AddMember` failure
- Stable OIDC subject use for human users
- Credential redaction
- Existing-gateway lazy bootstrap-admin provisioning
- Gateway-deletion cleanup and cleanup failure

#### Scenario: GTW-BOOT-07.01 -- Successful bootstrap is audited

- GIVEN a gateway has a gateway bootstrap admin
- WHEN an eligible originating request successfully calls `AddMember`
- THEN HyperShell SHALL record an audit event with the gateway ID, subject, subject type, role `user`, success outcome, and correlation ID

#### Scenario: GTW-BOOT-07.02 -- Failed bootstrap is audited

- GIVEN a gateway has a gateway bootstrap admin
- WHEN OpenShell rejects an `AddMember` call
- THEN HyperShell SHALL record an audit event with the gateway ID, subject, subject type, role `user`, failure outcome, correlation ID, and a redacted error
