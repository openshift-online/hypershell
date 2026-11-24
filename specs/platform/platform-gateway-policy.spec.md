# Platform Gateway Policy - Cedar admission rules enforced by gateway interceptors

**ID prefix:** PLAT-PGP

**Date:** 2026-11-20
**Status:** Draft
**Related:** [`data-model.spec.md`](./data-model.spec.md) (PlatformGatewayPolicy / PlatformPolicySettings / policy audit findings data model, PLAT-DATA-28/29/30), [`../security/rbac-enforcement.spec.md`](../security/rbac-enforcement.spec.md) (`platform-policy:admin` role SEC-RBAC-30, effective-policies and findings authorization SEC-RBAC-31, sidecar event authorization SEC-RBAC-32), [`control-plane.spec.md`](./control-plane.spec.md) (policy bundle reconcile and `ReportPolicyEvents` endpoint CTRL-CORE-22), [`openshell-gateway.spec.md`](./openshell-gateway.spec.md) (interceptor sidecar in the gateway pod OSG-CORE-19), [`openshell-gateway-helm-adoption.spec.md`](./openshell-gateway-helm-adoption.spec.md) (vendored-chart sidecar support OSG-HELM-06), [`generated-gateway-config-validation.spec.md`](./generated-gateway-config-validation.spec.md) (interceptor registration GTW-CFG-05), [`extension-api-openshell-api-mapping.spec.md`](./extension-api-openshell-api-mapping.spec.md) (AGT-EXT-06), [`hsctl-extension-commands.spec.md`](./hsctl-extension-commands.spec.md) (CLI-EXT-06), [`e2e-testing.spec.md`](./e2e-testing.spec.md) (enforcement canary short-mode and long-mode coverage E2E-CORE-49/50), [`../web-console/platform-policy-admin.spec.md`](../web-console/platform-policy-admin.spec.md) (admin control panel and Cedar editor WEB-PADM), [`../web-console/gateway-list-detail.spec.md`](../web-console/gateway-list-detail.spec.md) (read-only gateway Policies tab WEB-GLIS-26), [`../standards/control-plane/reconciliation-contract.spec.md`](../standards/control-plane/reconciliation-contract.spec.md) (STD-RCON idempotent reconcile)

## Overview

A platform administrator sets rules for the sandboxes that users run on HyperShell
gateways: for example, require CPU and memory requests and limits and bound their
range. HyperShell expresses every such rule in one policy kind,
**PlatformGatewayPolicy**. A policy is a [Cedar](https://www.cedarpolicy.com/)
policy (one or more `forbid` statements) plus user-facing metadata: a message, a
documentation link, the target gateways, and an enforcement mode. Only a platform
administrator writes policies. A policy applies to all gateways or to a selected
set.

A HyperShell **policy interceptor** runs as a sidecar in each gateway pod and
enforces the applicable policies through the OpenShell gateway interceptor
contract. Policy changes reach every gateway in seconds and never restart a
gateway. A platform-wide toggle sets the minimum enforcement mode (`audit` or
`enforce`); a single policy MAY be enforced while the toggle is `audit`. Sandbox
resource defaults are fleet-wide controller configuration.

This spec defines the behavior and security model of PlatformGatewayPolicy: the
policy language and its validation, the enforcement mode and defaults, the
enforcement architecture (interceptor packaging, static bindings, bundle
distribution, failure policy, evaluation, and denial text), the audit-finding
pipeline, and the effective-policy read surface. The data model lives in
[`data-model.spec.md`](./data-model.spec.md) (PLAT-DATA-28/29/30); the
authorization model lives in
[`../security/rbac-enforcement.spec.md`](../security/rbac-enforcement.spec.md)
(SEC-RBAC-30/31/32); this spec defers to both and restates only what a reader
needs in context.

Gateway owners cannot author policies, set defaults, or change enforcement in this
feature; they read the policies that apply to their gateway and that gateway's
audit findings. Cedar's forbid-overrides-permit model lets owner-authored policies
be added later without weakening a platform policy.

## Architecture

```
platform-policy:admin
  -> hsctl apply / web console -> API Server (PostgreSQL)
       - PlatformGatewayPolicy (Cedar + metadata)      write-time validation (shared cedar-go package)
       - PlatformPolicySettings.mode (audit|enforce)
                               │ gRPC watch (ext kinds + settings), periodic resync
                               ▼
                  Control Plane reconciler (per cluster)
                   builds a per-gateway policy bundle (revision hash, @id attribution,
                   enforced + audit-only sets, effective modes resolved)
                               │ update-or-create ConfigMap in the gateway namespace
                               ▼
                  hypershell-policy-bundle (ConfigMap, labeled with gateway id)
                               │ Kubernetes watch
                               ▼
   gateway pod ── gateway (OpenShell) ──unix socket──▶ policy interceptor sidecar
                   bound write RPCs                     (control-plane image, 2nd binary)
                   (modify_operation / validate /       - Kubernetes driver adapter + defaults
                    post_commit)                        - cedar-go evaluation of the bundle
                                                        - denial text, audit events
                               │ ReportPolicyEvents (projected SA token)
                               ▼
                  Control Plane ─ TokenReview + namespace check ─▶ API Server
                   (registered identity)                            policy_audit_findings,
                                                                    per-gateway status write-back
```

The interceptor is a second binary built from `components/control-plane/cmd/` and
shipped in the existing control-plane image; no new image and no release-bundle
component are added (`bin/expand-hypershell-release` accepts only the api-server,
control-plane, and web-console components). The controller runs the sidecar with
its own image digest, read from its own Pod status, so the controller and the
sidecar always run the same build.

The policy bundle is a versioned contract between the control plane and the
sidecar. It travels as a labeled ConfigMap (not a CRD: the `deploy/gitops`
packages install no CRDs, and a CRD would need a separate install on every hub and
spoke). Because distribution is a ConfigMap written directly by the controller and
not a Helm value, the gateway reconciler's Helm generation gate
(`observed_generation == generation`) does not apply, and a policy change does not
restart the gateway.

## Requirements

### Requirement: PLAT-PGP-01 -- PlatformGatewayPolicy Kind

The platform SHALL provide one platform-scoped extension kind,
`PlatformGatewayPolicy`, whose data model is defined in
[`data-model.spec.md`](./data-model.spec.md) (PLAT-DATA-28). A policy SHALL carry:
`name` and `description` for identification; a `target` that is either `all`
gateways or a selector over gateway ids, with an explicit `exclude`
list; an `enforcementMode` of `audit` or `enforce`; a `userMessage` and a
`documentationUrl` shown to users; and `cedar` policy text of one or more `forbid`
statements. The API server SHALL authorize every create, update, and delete with
the `platform-policy:admin` role (or `platform:admin`), per
[`../security/rbac-enforcement.spec.md`](../security/rbac-enforcement.spec.md)
(SEC-RBAC-30); `gateway:creator`, `gateway:owner`, `gateway:admin`, and
`gateway:viewer` SHALL NOT author, edit, or change the mode of any policy.

A new policy SHALL start in `audit` regardless of the request, so that a policy
cannot reject any request until an administrator enforces it or the global toggle
is raised. The policy `status` SHALL carry the write-time validation result, the
per-gateway applied bundle revision and condition (`Applied`, `Pending`, or
`Unsupported`), and audit-finding counts for the last 24 hours.

#### Scenario: PLAT-PGP-01.01 -- Only the Policy Admin Role May Author

- GIVEN a caller without `platform-policy:admin` or `platform:admin`
- WHEN the caller creates, updates, or deletes a `PlatformGatewayPolicy`
- THEN the API server SHALL reject the write with 403
- AND a `gateway:owner` of a targeted gateway SHALL still be rejected with 403

#### Scenario: PLAT-PGP-01.02 -- New Policy Starts in Audit

- GIVEN a create request whose body sets `enforcementMode: enforce`
- WHEN the policy is created
- THEN the stored policy SHALL be in `audit`
- AND raising it to `enforce` SHALL require a later authorized mode change (PLAT-PGP-06)

---

### Requirement: PLAT-PGP-02 -- Allow-By-Default, Forbid-Only Semantics

Policy admission SHALL be allow-by-default and restrict-only. The interceptor SHALL
evaluate one built-in base `permit` statement together with the `forbid` statements
of every applicable policy. A request SHALL be allowed only when no `forbid`
statement matches it; adding a policy SHALL be able to reject more requests and
never to allow a request that another policy rejects. Policy text SHALL contain
only `forbid` statements; a `permit` written by an author SHALL be rejected at
write time (PLAT-PGP-04).

#### Scenario: PLAT-PGP-02.01 -- A Policy Can Only Narrow Admission

- GIVEN a gateway with no enforced policy, on which a request R is allowed
- WHEN an administrator enforces a new policy whose `forbid` does not match R
- THEN R SHALL still be allowed
- AND a request that the new policy's `forbid` matches SHALL now be rejected

#### Scenario: PLAT-PGP-02.02 -- One Policy Cannot Re-Allow What Another Forbids

- GIVEN two enforced policies, one of which forbids request R
- WHEN both are evaluated for R
- THEN R SHALL be rejected regardless of the other policy
- AND no policy text SHALL be able to express a `permit` that overrides the forbid

---

### Requirement: PLAT-PGP-03 -- Cedar Schema and Quantity Normalization

The platform SHALL define a versioned, driver-neutral Cedar schema. For v1 the
schema SHALL model: a principal `OpenShell::User` with `subject: String` and
`roles: Set<String>`; an action `OpenShell::Action::"CreateSandbox"`; a resource
`OpenShell::Gateway` with `id: String` and `name: String`; and a context with
`cpuRequestMillis`, `cpuLimitMillis`, `memoryRequestBytes`, `memoryLimitBytes`,
`gpuCount` (all `Long`), and `image` and `sandboxName` (`String`). A **driver
adapter** in the sidecar SHALL map an OpenShell request to this context after
defaults are applied, converting Kubernetes quantities (for example `500m`,
`16Gi`) to the integer units the schema names (millicores, bytes), because Cedar
has no quantity type. The schema version SHALL be carried in the policy bundle
(PLAT-PGP-12); a change to the OpenShell request shape SHALL require a new schema
version and a documented compatibility note.

The Cedar schema, the quantity normalization, the write-time default-request
check, and the display renderer SHALL live in one shared Go package used by the
api-server (validation), the control plane (bundle build), and the interceptor
(evaluation), so the three agree on one schema and one normalization.

#### Scenario: PLAT-PGP-03.01 -- Quantities Normalize to Schema Units

- GIVEN a `CreateSandbox` request with a CPU request of `500m` and a memory limit of `16Gi`
- WHEN the driver adapter builds the Cedar context
- THEN `context.cpuRequestMillis` SHALL be `500` and `context.memoryLimitBytes` SHALL be `17179869184`

#### Scenario: PLAT-PGP-03.02 -- One Shared Schema Package

- GIVEN the api-server validates a policy and the interceptor evaluates it
- WHEN both load the Cedar schema and normalize quantities
- THEN both SHALL use the same shared Go package and schema version
- AND a schema version mismatch between a bundle and a sidecar SHALL be reported, not silently evaluated

---

### Requirement: PLAT-PGP-04 -- Write-Time Validation

On create and update the API server SHALL validate `cedar` policy text, and the
control plane SHALL re-validate before it publishes a bundle. Validation SHALL
require all of the following, and SHALL reject the write with the Cedar
diagnostics otherwise:

1. The text parses and validates against the current Cedar schema (PLAT-PGP-03);
   unknown attributes and wrong types are rejected.
2. Every statement is a `forbid`; a `permit` is rejected.
3. The **default request** - the request that results when every fleet default
   (PLAT-PGP-08) is applied and no user value is set - is not forbidden by the
   policy; a policy that forbids the defaults would reject every sandbox that omits
   a value and SHALL be rejected.
4. Author-written statement annotations are rejected or overwritten; the control
   plane owns the `@id` annotation (PLAT-PGP-14).

#### Scenario: PLAT-PGP-04.01 -- Permit and Unknown Attribute Rejected

- GIVEN a policy whose text contains a `permit` statement or references an attribute the schema does not declare
- WHEN the API server validates the write
- THEN it SHALL reject the write with the Cedar diagnostic naming the offending statement or attribute

#### Scenario: PLAT-PGP-04.02 -- A Policy That Forbids the Defaults Is Rejected

- GIVEN fleet defaults of CPU request `500m` and the policy `forbid ... when { context.cpuRequestMillis < 600 }`
- WHEN the policy is validated
- THEN the write SHALL be rejected because the default request would be forbidden

---

### Requirement: PLAT-PGP-05 -- Cedar Library Capability and Fallback

The interceptor, the control plane, and the api-server SHALL evaluate and build
Cedar with [`cedar-go`](https://github.com/cedar-policy/cedar-go). The first task
of the enforcement work SHALL confirm that the pinned `cedar-go` release validates
policies against a schema (rejecting unknown attributes and type errors) with
usable diagnostics. When `cedar-go` schema validation is sufficient, the api-server
SHALL validate with `cedar-go`. When it is not, the api-server SHALL validate with
a Rust Cedar validator (for example a small validation binary added to the
api-server image), while interceptor evaluation stays on `cedar-go`; the decision
and its evidence SHALL be recorded in this spec. Whichever path is chosen,
write-time validation SHALL meet the contract of PLAT-PGP-04.

#### Scenario: PLAT-PGP-05.01 -- Validation Contract Holds Regardless of Library Path

- GIVEN either the `cedar-go` or the Rust validator path is selected
- WHEN a policy with an unknown attribute is submitted
- THEN the API server SHALL reject it with a diagnostic (PLAT-PGP-04)
- AND the interceptor SHALL evaluate accepted policies with `cedar-go`

---

### Requirement: PLAT-PGP-06 -- Enforcement Mode and Global Toggle

The platform SHALL hold a DB-backed, platform-wide singleton
`PlatformPolicySettings.mode` with value `audit` or `enforce`, default `audit`,
defined in [`data-model.spec.md`](./data-model.spec.md) (PLAT-DATA-29). The toggle
SHALL set the **minimum** enforcement mode. The **effective mode** of a policy
SHALL be the stricter of the global toggle and the policy's own `enforcementMode`:
when the toggle is `enforce`, every policy is enforced; when the toggle is `audit`,
only policies whose `enforcementMode` is `enforce` are enforced. Only
`platform-policy:admin` (or `platform:admin`) SHALL change the toggle or a policy
mode (SEC-RBAC-30). Each mode change SHALL be audit-logged (SEC-RBAC-12) and SHALL
require a confirmation that shows the last 24 hours of findings for the affected
scope.

#### Scenario: PLAT-PGP-06.01 -- Effective Mode Is the Stricter of the Two

- GIVEN the global toggle is `audit`
- WHEN a policy has `enforcementMode: enforce`
- THEN that policy SHALL be enforced and every `audit` policy SHALL remain audit-only
- AND raising the toggle to `enforce` SHALL make every policy enforced

#### Scenario: PLAT-PGP-06.02 -- Mode Change Is Authorized and Audited

- GIVEN a caller without `platform-policy:admin`
- WHEN the caller changes the toggle or a policy mode
- THEN the API server SHALL reject it with 403
- AND an authorized change SHALL be recorded in the audit log

---

### Requirement: PLAT-PGP-07 -- Strict Audit Changes Nothing

For a policy whose effective mode is audit, the interceptor SHALL change nothing
about the request: it SHALL add no denial, SHALL apply no patch on behalf of that
policy, and SHALL instead record what the policy would reject as an audit finding
(PLAT-PGP-15). Audit-only statements SHALL never contribute to denial text.

#### Scenario: PLAT-PGP-07.01 -- Audit-Only Policy Allows but Records

- GIVEN an audit-only policy whose `forbid` matches a `CreateSandbox` request
- WHEN the request is evaluated
- THEN the request SHALL be allowed
- AND a `would_reject` finding SHALL be recorded naming that policy

---

### Requirement: PLAT-PGP-08 -- Fleet Default Sandbox Resources

Sandbox resource defaults SHALL be fleet-wide controller configuration, for example
`HYPERSHELL_POLICY_DEFAULT_CPU_REQUEST=500m`, `..._CPU_LIMIT=2`,
`..._MEMORY_REQUEST=1Gi`, `..._MEMORY_LIMIT=4Gi`. The controller SHALL pass the
defaults to the sidecar as environment variables. Defaults SHALL NOT be stored in a
policy and SHALL NOT be settable by a gateway owner. The sidecar SHALL add a
default in `modify_operation` only when at least one policy is enforced on that
gateway; otherwise it SHALL change nothing and record a `would_add_defaults`
finding. A change to a fleet default SHALL be checked against every existing policy
before rollout (the default-request check of PLAT-PGP-04); the controller SHALL
refuse a defaults change that an enforced policy would forbid. A defaults change
MAY roll the gateway pods, which is acceptable for a rare operator change.

#### Scenario: PLAT-PGP-08.01 -- Defaults Injected Only Under Enforcement

- GIVEN a gateway with at least one enforced policy and a `CreateSandbox` request that omits CPU request
- WHEN the interceptor runs `modify_operation`
- THEN it SHALL add the fleet default CPU request to the request
- AND on a gateway with no enforced policy it SHALL add nothing and record a `would_add_defaults` finding

#### Scenario: PLAT-PGP-08.02 -- A Defaults Change Inconsistent With a Policy Is Refused

- GIVEN an enforced policy that forbids `context.cpuRequestMillis < 600`
- WHEN an operator changes the default CPU request to `500m`
- THEN the controller SHALL refuse the defaults change because the new default request would be forbidden

---

### Requirement: PLAT-PGP-09 -- Interceptor Packaging and Sidecar Identity

The policy interceptor SHALL be a second binary, `hypershell-policy-interceptor`,
built from `components/control-plane/cmd/` and shipped in the existing
control-plane image; no new container image and no release-bundle component SHALL
be added. The controller SHALL run the sidecar using its **own** image digest,
read from its own Pod status, so the controller and the interceptor always run the
same build and no image reference needs configuration. The sidecar SHALL run as a
native sidecar (an init container with `restartPolicy: Always`) in each gateway
pod, so it is ready before the gateway calls `Describe`. The gateway SHALL reach
the sidecar over a `unix://` socket on a pod-local `emptyDir`. The sidecar SHALL
verify the gateway JWT (`iss = openshell-gateway:<gateway_id>`) against the local
gateway JWKS and SHALL hold no HyperShell credentials for request evaluation.

#### Scenario: PLAT-PGP-09.01 -- Sidecar Shares the Controller Build

- GIVEN a control-plane release with a fixed image digest
- WHEN the controller renders the gateway pod
- THEN the policy interceptor sidecar SHALL use the controller's own image digest
- AND no separate interceptor image reference SHALL be configured

#### Scenario: PLAT-PGP-09.02 -- Gateway JWT Verified Over the Local Socket

- GIVEN the gateway calls the interceptor over the pod-local unix socket
- WHEN the request carries a gateway JWT
- THEN the sidecar SHALL verify it against the local gateway JWKS before evaluating
- AND SHALL reject a call whose JWT does not verify

---

### Requirement: PLAT-PGP-10 -- Static Bindings, Dynamic Policies

The generated `gatewayConfig` SHALL register the interceptor once, with
`binding_policy = "exact"`, binding the fixed set of interceptable write RPCs with
fixed phases (GTW-CFG-05). Interceptor registration SHALL change only on an
OpenShell version bump, which restarts the gateway anyway; a policy change SHALL
never modify `gateway.toml`. The sidecar SHALL obtain the current policies from the
bundle (PLAT-PGP-12), not from registration.

#### Scenario: PLAT-PGP-10.01 -- Policy Change Does Not Touch gateway.toml

- GIVEN a gateway with the interceptor registered and a running sidecar
- WHEN an administrator edits a policy
- THEN `gateway.toml` SHALL be unchanged and the gateway SHALL NOT restart
- AND the new policy SHALL take effect through a new bundle revision (PLAT-PGP-12)

---

### Requirement: PLAT-PGP-11 -- Per-RPC Failure Policy

Each interceptor binding SHALL declare a failure policy. The create, start, resume,
attach, and policy-write RPCs SHALL bind `fail_closed`; the delete, stop, suspend,
revoke, reject, undo, and clear RPCs SHALL bind `fail_open`, so that an interceptor
outage never blocks cleanup. Supervisor-originated calls (for example `UpdateConfig`
and the policy-analysis RPC) SHALL stay fast and SHALL NOT break running sandboxes.
Every patch the interceptor returns SHALL be deterministic, because OpenShell
mutation replay re-runs interceptors on retries.

#### Scenario: PLAT-PGP-11.01 -- Fail Closed on Create, Fail Open on Delete

- GIVEN the interceptor sidecar is unavailable
- WHEN a `CreateSandbox` call is made
- THEN the gateway SHALL fail the create closed
- AND a `DeleteSandbox` call SHALL succeed (fail open)

#### Scenario: PLAT-PGP-11.02 -- Patches Are Deterministic Under Replay

- GIVEN the same `CreateSandbox` request evaluated twice by `modify_operation`
- WHEN OpenShell replays the mutation on retry
- THEN the interceptor SHALL return the same patch both times

---

### Requirement: PLAT-PGP-12 -- Policy Bundle Distribution

The api-server SHALL stream `PlatformGatewayPolicy` records and the
`PlatformPolicySettings` toggle to the control plane over the existing gRPC watch,
with a periodic resync. The control plane SHALL build a per-gateway **policy
bundle** and write it as a ConfigMap (for example `hypershell-policy-bundle`) into
the gateway namespace with update-or-create, labeled with the gateway id and the
bundle schema version (CTRL-CORE-22). The bundle SHALL be a versioned contract
carrying: `schemaVersion`, `gatewayId`, `revision` (a hash of the resolved
content), the Cedar schema version, two Cedar policy sets with `@id` annotations
(an **enforced** set and an **audit-only** set, with effective modes already
resolved per PLAT-PGP-06), and per policy the `name`, `userMessage`, and
`documentationUrl`. The sidecar SHALL watch the ConfigMap through the Kubernetes
API and apply a new revision within seconds without a gateway restart. When the
sidecar has no bundle, it SHALL fail closed on `fail_closed` RPCs (PLAT-PGP-11).

#### Scenario: PLAT-PGP-12.01 -- A New Revision Applies in Seconds Without Restart

- GIVEN a running sidecar with an applied bundle revision
- WHEN the control plane writes a new bundle revision to the ConfigMap
- THEN the sidecar SHALL apply the new revision within seconds
- AND the gateway SHALL NOT restart

#### Scenario: PLAT-PGP-12.02 -- No Bundle Fails Closed on Protected RPCs

- GIVEN a sidecar that has not yet loaded any bundle
- WHEN a `fail_closed` RPC such as `CreateSandbox` is evaluated
- THEN the gateway SHALL fail the call closed

---

### Requirement: PLAT-PGP-13 -- Evaluation on Bound Write RPCs

On a bound `CreateSandbox` call the interceptor SHALL, in `modify_operation`, add
fleet defaults per PLAT-PGP-08, and in `validate` build the Cedar request with the
driver adapter, evaluate the enforced policy set, and deny with `PERMISSION_DENIED`
when any enforced statement forbids the request. It SHALL then evaluate the
audit-only set and record what it would reject (PLAT-PGP-15). Bindings on
`StartSandbox` and `ResumeSandbox` SHALL re-check the sandbox when it starts or
resumes.

#### Scenario: PLAT-PGP-13.01 -- Enforced Forbid Denies the Create

- GIVEN an enforced policy that forbids `context.cpuRequestMillis > 1000` and a request with a CPU request of `2`
- WHEN the interceptor evaluates `validate`
- THEN it SHALL deny the call with `PERMISSION_DENIED`
- AND the audit-only set SHALL additionally be evaluated and any would-reject recorded

---

### Requirement: PLAT-PGP-14 -- Denial Text and Attribution

The control plane SHALL add an `@id("<policy-name>")` annotation to every statement
in a bundle, so Cedar reports which policy decided a result. The interceptor SHALL
build the denial `reason` from **every** enforced statement that forbids the
request, grouped by policy: each line SHALL name the policy (`@id`) and show its
`userMessage`, and SHALL additionally state the offending value and the bound when
the display renderer (PLAT-PGP-17) recognizes the matching statement. The reason
SHALL end with a link: the policy `documentationUrl` when exactly one policy
matches, otherwise the gateway Policies tab. OpenShell passes
`InterceptorResult.reason` to the client verbatim as the gRPC status message, so it
is the only explanation that reaches the OpenShell CLI, SDKs, and console. The
interceptor SHALL put only normalized resource values into the reason, never a
request field marked secret, and the policy editor SHALL warn authors not to put
secrets, credentials, or internal hostnames in `userMessage` (because every user
who hits the rule can read it).

#### Scenario: PLAT-PGP-14.01 -- Denial Names Every Matching Policy

- GIVEN two enforced policies that both forbid a request
- WHEN the interceptor denies the call
- THEN the `reason` SHALL list both policies, each with its `userMessage`
- AND when the renderer recognizes a matching statement the line SHALL also state the value and the bound

#### Scenario: PLAT-PGP-14.02 -- The Reason Carries No Secret Request Fields

- GIVEN a request that includes a field marked secret
- WHEN the interceptor builds the denial reason
- THEN the reason SHALL contain only normalized resource values, never the secret field

---

### Requirement: PLAT-PGP-15 -- Audit Findings Pipeline

For each operation that an audit-only statement would reject, or that would receive
defaults while no policy is enforced, the sidecar SHALL record a structured event
carrying: time, gateway, principal subject, operation, outcome (`would_reject` or
`would_add_defaults`), the matching policy names, the bundle revision, and the
normalized resource values; it SHALL store no denial text. The sidecar SHALL send
events and its applied bundle revision to a control-plane gRPC endpoint
(`ReportPolicyEvents`, CTRL-CORE-22), authenticating with a projected,
audience-bound, short-lived service-account token; the control plane SHALL verify
the token with `TokenReview`, check that the token's namespace belongs to the
reporting gateway, and forward the events to the api-server under its registered
identity (SEC-RBAC-32). Each event SHALL carry a sidecar-generated id so retries
are idempotent. The sidecar SHALL buffer a bounded number of events in memory while
the control plane is unavailable and count dropped events in a metric. Findings
SHALL be stored in the `policy_audit_findings` table (PLAT-DATA-30) and pruned
after a configurable retention (30 days by default); policy status SHALL show the
last-24-hours counts computed from that table. The sidecar SHALL also export
counters (evaluations, would-reject, would-add-defaults, dropped events) for
dashboards; metrics SHALL NOT be the source of the UI finding data.

#### Scenario: PLAT-PGP-15.01 -- Events Are Idempotent and Namespace-Checked

- GIVEN the sidecar reports an event twice with the same sidecar-generated id
- WHEN the control plane forwards them
- THEN the api-server SHALL store one finding row (idempotent)
- AND the control plane SHALL reject an event whose service-account token namespace does not belong to the reporting gateway

#### Scenario: PLAT-PGP-15.02 -- Buffer Bounded and Drops Counted

- GIVEN the control plane is unavailable
- WHEN the sidecar produces more events than its bounded buffer holds
- THEN it SHALL drop events rather than grow without bound
- AND it SHALL increment a dropped-events metric

---

### Requirement: PLAT-PGP-16 -- Existing Sandboxes

Interceptors SHALL act only on writes; a policy change SHALL NOT retroactively stop
a running sandbox. Bindings on `StartSandbox` and `ResumeSandbox` SHALL re-check a
sandbox when it starts or resumes (PLAT-PGP-13). The control plane SHALL report, in
policy status, the running sandboxes on a targeted gateway that do not comply with
an enforced policy, so an administrator sees existing drift without a forced
restart.

#### Scenario: PLAT-PGP-16.01 -- Running Sandbox Re-Checked on Resume

- GIVEN a suspended sandbox that would violate a now-enforced policy
- WHEN it is resumed
- THEN the `ResumeSandbox` binding SHALL re-evaluate it against the enforced set
- AND a still-running non-compliant sandbox SHALL be reported in policy status

---

### Requirement: PLAT-PGP-17 -- Best-Effort Cedar Display Renderer

The shared Go package SHALL provide a best-effort renderer that recognizes common
statement shapes and renders them in plain words - for v1, comparisons on the
resource context fields joined by `||` or `&&` against a literal (for example
`forbid ... when { context.cpuRequestMillis > 4000 }` renders as "CPU request: at
most 4"). Any statement the renderer does not recognize SHALL be shown as its Cedar
source together with the policy `userMessage`. The renderer SHALL affect display
only; enforcement, the default-request check, and the denial value-and-bound text
SHALL always use the Cedar evaluation, not the renderer.

#### Scenario: PLAT-PGP-17.01 -- Recognized Shape Renders as Plain Words

- GIVEN a statement `forbid ... when { context.cpuRequestMillis > 4000 }`
- WHEN the renderer displays it
- THEN it SHALL render "CPU request: at most 4"
- AND a statement the renderer does not recognize SHALL be shown as Cedar source with the policy message

---

### Requirement: PLAT-PGP-18 -- Driver Neutrality and Unsupported Gateways

The Cedar schema SHALL be driver-neutral; a driver adapter in the sidecar SHALL map
an OpenShell request to the Cedar context and apply defaults. v1 SHALL ship only the
Kubernetes adapter (reading `spec.template.resources.{requests,limits}`, the
effective request when only a limit is set, and `driver_config` resources). The
control plane SHALL read driver capabilities from `GetGatewayInfo`; a gateway whose
driver has no adapter or cannot honor resource values (for example the VM driver)
SHALL show `Unsupported` in policy status and SHALL never enforce a resource policy.

#### Scenario: PLAT-PGP-18.01 -- Unsupported Driver Never Enforces

- GIVEN a gateway whose driver cannot honor sandbox resource values
- WHEN a resource policy targets that gateway
- THEN policy status for that gateway SHALL be `Unsupported`
- AND the interceptor SHALL not reject a request on that gateway for a resource rule

---

### Requirement: PLAT-PGP-19 -- Effective-Policies Read Surface

The API server SHALL expose a read endpoint, `GET
/gateways/{id}/effective_policies`, returning the policies that apply to the
gateway (name, `userMessage`, `documentationUrl`, effective mode and the reason it
is enforced or audit-only, Cedar text, and the rendered summary), the fleet
defaults, the global mode, and the gateway's audit findings. Every access level of
the gateway (owner, admin, viewer) SHALL read it; `platform-policy:admin` and
`fleet:reader` SHALL additionally see policy targets and exclusions, which SHALL be
hidden from gateway users so the endpoint never reveals another gateway
(SEC-RBAC-31, following the 403/404 masking rule SEC-RBAC-10). A gateway user SHALL
read the findings of their own gateway only, including other users' subjects on
that gateway.

#### Scenario: PLAT-PGP-19.01 -- Gateway User Sees Rules but Not Targets

- GIVEN a `gateway:viewer` of gateway G
- WHEN the viewer reads `GET /gateways/G/effective_policies`
- THEN the response SHALL list the applicable policies, defaults, mode, and G's findings
- AND it SHALL NOT include the policies' target or exclude sets

#### Scenario: PLAT-PGP-19.02 -- Findings Scoped to the Gateway

- GIVEN a `gateway:viewer` of gateway G
- WHEN the viewer reads the effective policies
- THEN the findings SHALL be G's findings only (including other subjects on G)
- AND SHALL NOT include another gateway's findings

---

### Requirement: PLAT-PGP-20 -- E2E Enforcement Canary Policy

The platform SHALL seed, with an additive idempotent migration, a built-in platform
policy `e2e-enforcement-canary` with `target: all` and `enforcementMode: enforce`
(enforced whatever the global toggle is). Its Cedar SHALL apply only to sandboxes
whose name starts with the reserved prefix `e2epol-`, so it never
affects a real user:

```cedar
forbid (principal, action == OpenShell::Action::"CreateSandbox", resource)
when { context.sandboxName like "e2epol-*" && context.cpuRequestMillis > 1000 };
```

Its `userMessage` SHALL state that it is used by HyperShell release checks and
applies only to sandboxes named `e2epol-*`. It SHALL be visible like
any other policy. A `platform-policy:admin` SHALL NOT be able to delete it; the
canary MAY be disabled per environment with a controller setting, and the e2e short
mode SHALL then report the policy check as skipped, never as passed (E2E-CORE-49).

#### Scenario: PLAT-PGP-20.01 -- Canary Enforced but Scoped to the Reserved Prefix

- GIVEN the seeded `e2e-enforcement-canary` policy
- WHEN a sandbox named `e2epol-<run>-deny` is created with a CPU request of `2`
- THEN the create SHALL be rejected with `PERMISSION_DENIED` naming the canary
- AND a sandbox whose name lacks the reserved prefix SHALL NOT be rejected by the canary

#### Scenario: PLAT-PGP-20.02 -- Canary Is Undeletable and Disable Is Reported as Skipped

- GIVEN the seeded canary policy
- WHEN a `platform-policy:admin` attempts to delete it
- THEN the delete SHALL be refused
- AND disabling it per environment SHALL make the short-mode policy check report skipped, never passed

---

### Requirement: PLAT-PGP-21 -- Editor Dry Run and Per-Policy Findings

The API server SHALL expose two endpoints that support the policy editor and the
policy detail view (WEB-PADM-04, WEB-PADM-05, WEB-PADM-08):

- `POST /api/hypershell/v1/ext/platform_gateway_policies/evaluate` SHALL take draft
  Cedar text and an optional sample request, and SHALL return the write-time
  validation result (PLAT-PGP-04, with line-numbered diagnostics), the rendered
  summary (PLAT-PGP-17), the fleet-default check, and, when a sample request is
  given, the decision and the denial text the interceptor would produce
  (PLAT-PGP-14). It SHALL persist nothing.
- `GET /api/hypershell/v1/ext/platform_gateway_policies/{id}/findings` SHALL return
  the audit findings of one policy across its target gateways, paginated, with an
  optional `outcome` filter (`would_reject` or `would_add_defaults`).

The evaluate endpoint SHALL be authorized for `platform-policy:admin` and
`platform:admin` only (SEC-RBAC-30). The findings endpoint SHALL additionally be
readable by `fleet:reader` (SEC-RBAC-31). Gateway users read findings through
PLAT-PGP-19 instead.

#### Scenario: PLAT-PGP-21.01 -- Dry Run Validates and Evaluates Without Saving

- GIVEN a `platform-policy:admin`
- WHEN they post draft Cedar with a sample request to the evaluate endpoint
- THEN the response SHALL contain the validation diagnostics, the rendered summary, the defaults check, and the decision with its denial text
- AND no policy SHALL be created or changed

#### Scenario: PLAT-PGP-21.02 -- Per-Policy Findings Are Not Readable by Gateway Users

- GIVEN a policy P with findings on two gateways
- WHEN a `platform-policy:admin` reads `GET .../platform_gateway_policies/P/findings?outcome=would_reject`
- THEN the response SHALL list P's would-reject findings from both gateways, paginated
- AND a `gateway:owner` reading the same path SHALL be refused

## Security analysis

- **Restrict-only by construction** (PLAT-PGP-02, PLAT-PGP-04): forbid-only Cedar
  with a base permit means a policy can only narrow admission. A gateway owner
  cannot weaken a platform policy, and a future owner-authored policy cannot either.
- **Authority is the platform policy role** (PLAT-PGP-01, SEC-RBAC-30): only
  `platform-policy:admin` (DB-backed, grantable to an IT or security group without
  Keycloak changes) and `platform:admin` author policies, change modes, or move the
  toggle. Gateway users are read-only.
- **The sidecar holds no HyperShell credential** (PLAT-PGP-09, PLAT-PGP-15): it
  verifies the gateway JWT locally and reports findings with a projected,
  audience-bound service-account token the control plane validates by `TokenReview`
  and namespace check, so a compromised sidecar cannot write arbitrary findings for
  another gateway.
- **No cross-gateway disclosure** (PLAT-PGP-19, SEC-RBAC-31): targets and exclusions
  are visible only to platform policy admins and fleet readers; a gateway user sees
  the rules and findings of their own gateway only.
- **Denials leak no secrets** (PLAT-PGP-14): the reason carries only normalized
  resource values; the editor warns authors against secrets in `userMessage`.
- **Safe rollout** (PLAT-PGP-06, PLAT-PGP-07): audit mode changes nothing and
  records what it would reject, and each mode change is confirmed against recent
  findings, so an administrator sees impact before enforcing.
- **Availability** (PLAT-PGP-11): an interceptor outage fails cleanup RPCs open and
  only the protected write RPCs closed, so an outage cannot strand sandboxes.

## Decisions

- **Policy language: Cedar**, for its built-in forbid-overrides-permit semantics
  (the restrict-only guarantee), statement-level attribution via `@id`, and policy
  analysis. CEL (`cel-go`) was considered and not chosen.
- **Bundle object: ConfigMap**, not a CRD, because `deploy/gitops` installs no CRDs.
- **Interceptor packaging: a second binary in the control-plane image**, run by the
  controller with the controller's own image digest; no new image.
- **Role name:** `platform-policy:admin`.
- **Gateway owners:** read-only in this feature.

## Out of scope

- Gateway-owner-authored policies, owner-set defaults, and owner-changed
  enforcement. Owners read platform policies and findings only.
- Workload-template resources: `CreateSandboxTemplate` is not interceptable, so a
  sandbox created from a stored template takes the template's resources.
- Per-workspace or per-user quotas (sandbox counts, total CPU).
- Egress and image rules; the Cedar schema is designed to extend to them later
  (the `ip()` extension for egress, string and set operations for images) without a
  new API type.
