# E2E Coverage for Credential Drivers, Gateway Sharing, Kata, Alerting, and CLI/UI Parity

**Date:** 2026-10-06
**Status:** Draft
**Jira:** HYPERSHELL-368
**Related:** `e2e-testing.spec.md` (the Go `testify/suite` framework, the named phase model P0.x-P3.x, `E2E_MODE` short/long, the `E2EInfraDriver` contract, the `harness` helpers, the parallel scheduler, and the TLS-trust rule);
             `e2e-console-browser-testing.spec.md` (browser suite; install-docs link);
             `openshell-gateway-credentials.spec.md` (credential driver fields, TOML, RBAC, Vault SA token, KEK);
             `managed-cluster-registration.spec.md` (registration, orphan-on-delete, user-id discovery via `/users`);
             `openshell-gateway-oidc.spec.md` and `specs/security/rbac-enforcement.spec.md` (`gateway:owner`/`gateway:viewer` -> `openshell-admin`/`openshell-user`);
             `control-plane-reconciliation-metrics.spec.md` (reconcile outcome/error metrics the alerting phase asserts on)

---

## Purpose

Five platform features are implemented and shipped but carry no (or only
indirect) end-to-end coverage in the Go functional suite (`E2ESuite`,
`tests/e2e/`) or the browser suite (`tests/e2e/e2e-console.sh`): per-gateway
provider credential storage drivers (Vault and Kubernetes Secrets), sharing a
gateway between users through the `role_bindings` API, Kata-runtime sandboxes,
automated alerting / incident signal, and parity between the CLI commands the web
console renders and the commands the e2e suite actually executes. This
specification defines the e2e coverage that closes those gaps.

The e2e suite was refactored from Bash to a Go `testify/suite` (`E2ESuite`,
`tests/e2e/suite.go`). Work is organized into **named phase steps** with dotted
identifiers (P0.x preflight, P1.x the provisioning gate, P2.x parallel feature
waves, P3.x admin/validation), registered in `TestPhases` (`suite.go`) via
`s.gate(...)` (serial) or `s.parallelStep(...)` (parallel wave) and marked
`tagShort` or `tagLong`. There is no longer a numbered "area" model.

The five feature gaps become **new long-tagged phase steps** appended to the P2
parallel wave (`p2ParallelSteps()`), each a `func (s *E2ESuite) p2_N<name>(t *testing.T)`.
This spec additionally defines one **cross-component coverage addition**: an
extension of the existing `managedcluster-lifecycle` phase (P2.4) (see
[Cross-Component Route Coverage Additions](#cross-component-route-coverage-additions)).
Proposed identifiers below (P2.8-P2.11) are assigned when the steps are appended;
the slug is the stable name. P2.7 is the separately-owned `vault-fail-closed`
phase; credential-driver selection is control-plane-only under PLAT-DATA-23 and is
not exercised as a dedicated per-gateway phase (see Requirement E2E-COV-01).

| Proposed step | Slug | Feature |
|---|---|---|
| P2.8 | `gateway-sharing-rolebindings` | Gateway sharing via `role_bindings` |
| P2.9 | `kata-runtime` | Kata-runtime sandboxes |
| P2.10 | `alerting-signal` | Alerting / incident signal |
| P2.11 | `cli-ui-parity` | CLI/UI command parity |
| P2.4 (ext) | `managedcluster-lifecycle` | ManagedCluster delete + re-registration |

Every step here is **long-tagged**: it runs only in `long` mode and is skipped
(`t.Skipf`) in `short` mode and in the matrix's default short runs
(`modeAllows`). `perf` is no longer an `E2E_MODE` (performance is a separate Go
benchmark). Steps that act as a second identity use the dev / platform-admin
credentials the suite seeds only in long mode (`s.devCreds()`,
`apiClientForCreds`). Each step reuses the `E2EInfraDriver` contract
(`AcquireOIDCToken`, `AcquireGatewayTokenWithRole`, `AssignGatewayClientRole`,
`APIClient`), the typed `apiclient.Client` (no raw curl), client-go via
`harness.Clients`, `harness.Poll`/`Retry`, and testify assertions, and adds no new
`E2EInfraDriver` methods.

Backends the Kind CI gate cannot host (a running Vault server; a
`kata-containers` RuntimeClass) are opt-in: the step reads an enable flag and
`t.Skipf`s with a reason when the flag or backing infrastructure is absent - the
suite's three-outcome skip idiom (the same `t.Skipf`-with-reason pattern a step uses
when an `E2E_QUALIFY_*` backend is absent), never a false pass. The halves Kind can host (the Kubernetes Secrets credential
driver, the full `role_bindings` sharing flow, CLI/UI parity, and the
metric-signal half of alerting) run in long mode on every target, including the
Kind CI's long runs.

### Scope

In scope: the five new feature phase steps, the cross-component coverage addition,
their mode placement, and the environment variables they add. Out of
scope: the feature implementations themselves (owned by the specs cross-referenced
above), the browser console suite's own areas (owned by
`e2e-console-browser-testing.spec.md`, except where this spec notes a browser
counterpart), and authoring a `PrometheusRule` for the alerting feature (see
[Alerting and Incident Signal Coverage](#requirement-alerting-and-incident-signal-coverage)).

---

## Requirements

### Requirement: E2E-COV-01 -- Provider Credential Driver Coverage

A gateway's `credential_driver` is a control-plane-only field. Per PLAT-DATA-23
(`data-model.spec.md`) and `openshell-gateway-credentials.spec.md`, `name` is the
only user-writable gateway field and the API rejects a user-supplied
`credential_driver` on both create and update; the control plane resolves the
driver from the deployment's `GATEWAY_DEFAULT_CREDENTIAL_DRIVER`
(`defaultCredentialDriverType()` in
`components/control-plane/internal/gateway/helm_deploy.go`), not from a per-gateway
create request. There is therefore no API path for the suite to provision a gateway
with an explicit per-gateway driver, so the earlier dedicated `credential-drivers`
phase and its per-type create assertions are NOT part of this coverage. Driver
behavior is exercised instead by the deployment default applied to every gateway
and by the separately-owned serial `vault-fail-closed` phase (P2.7). This
requirement records that explicit per-gateway driver e2e coverage is intentionally
dropped under PLAT-DATA-23; it defines no scenarios.

### Requirement: E2E-COV-02 -- Gateway Sharing Coverage

The e2e test suite SHALL validate a complete "owner shares a gateway with a
second user" flow through the `role_bindings` API
(`/api/hypershell/v1/role_bindings`). This is the `gateway-sharing-rolebindings`
phase (P2.8), long-tagged (it acts as a second identity and mutates bindings).
The `role_bindings` API is a distinct, active public surface; the existing
`gateway-access-management` phase (P2.5) covers the parallel
`/gateways/{id}/access` sharing surface (whose grants write the same underlying
RoleBinding records through the gatewayAccess handler), so this phase covers the
`role_bindings` REST contract without overlap: binding by `role_id` with
`scope`/`user_id`/`gateway_id` fields, the non-owner authorization guard, the
Keycloak-only role refusals, and binding create/delete issued against
`/role_bindings` itself.

The phase SHALL provision and own a **dedicated gateway** (not the P1 primary
gateway), because P2 steps run concurrently and the `gateway-access-management`
phase (P2.5) already grants, promotes, and revokes the developer identity on the
primary gateway; sharing the primary would let either phase tear down Keycloak/API
state the other is still asserting. The admin is the owner of this dedicated
gateway (the `gateway:owner` RoleBinding the API server assigns to the gateway
creator at create time, which the control plane's RoleBinding reconciler then
propagates into Keycloak).

The suite SHALL discover the second user's id through the admin `/users` inventory
(ensuring that user record exists first by acquiring a token for `E2E_DEV_USERNAME`
via `AcquireOIDCToken`), and SHALL discover the `gateway:viewer` role's id through
`Roles().List` (the `role_bindings` API binds by `role_id`, not by role name; the
SDK `RoleBinding` carries `RoleID`). It SHALL then, as the owner, create a
`role_bindings` entry through `apiclient` (`RoleBindings().Create`, or `RawJSON`
where the typed op is absent) with `scope = gateway`, the resolved
`role_id` for `gateway:viewer`, `user_id = <second user>`, and
`gateway_id = <dedicated gateway>`. It SHALL then prove the RoleBinding reconciler
propagated the grant into Keycloak by acquiring a per-gateway token for the second
user with `E2EInfraDriver.AcquireGatewayTokenWithRole` and asserting the
`openshell-user` role lands in it, and by confirming the second user MAY now
create a sandbox on that gateway (via the openshell CLI as that identity).

The suite SHALL then revoke access (`RoleBindings().Delete`) as the owner and
assert the second user loses the grant (a freshly minted per-gateway token no
longer carries `openshell-user`, and a sandbox create is denied), confirming the
reconciler's delete-side role recomputation. It SHALL also assert the
authorization guard: a caller who is not `gateway:owner` on the target gateway
SHALL be rejected when creating a `gateway:viewer`/`gateway:owner` binding, and a
`role_bindings` create for `gateway:creator` or `platform:admin` SHALL be refused
(those are Keycloak-only).

The suite SHALL clean up any binding and gateway it creates on every exit path.

#### Scenario: E2E-COV-02.01 -- Owner shares viewer access and the reconciler propagates it

- GIVEN the admin owns the phase's dedicated `Running` gateway, the second user's id is resolved from `/users`, and the `gateway:viewer` role id is resolved from `Roles().List`
- WHEN the owner creates a `role_bindings` entry with `scope = gateway`, the resolved `role_id`, the second user's `user_id`, and the dedicated `gateway_id`
- THEN `E2EInfraDriver.AcquireGatewayTokenWithRole` for the second user SHALL observe the `openshell-user` role on the per-gateway client within the role-reconcile timeout
- AND the second user SHALL be able to create a sandbox on that gateway

#### Scenario: E2E-COV-02.02 -- Revoking the binding removes access

- GIVEN a `gateway:viewer` binding the owner created for the second user
- WHEN the owner deletes that binding
- THEN a newly minted per-gateway token for the second user SHALL NOT carry `openshell-user`
- AND a sandbox create by the second user SHALL be denied

#### Scenario: E2E-COV-02.03 -- Non-owner cannot share and privileged roles are refused

- GIVEN a caller who does not hold `gateway:owner` on the target gateway
- WHEN that caller creates a `gateway:viewer` binding for the gateway
- THEN the API SHALL reject the request
- AND a `role_bindings` create requesting `gateway:creator` or `platform:admin` SHALL be refused as Keycloak-only, regardless of caller

### Requirement: E2E-COV-03 -- Kata Runtime Sandbox Coverage

The e2e test suite SHALL validate that a sandbox honors a configured Kata
RuntimeClass, closing the gap that the suite exercises only the cluster-default
runtime. This is the `kata-runtime` phase (P2.9), long-tagged. Because the Kind
gate hosts no `kata-containers` RuntimeClass, the phase is gated on
`E2E_QUALIFY_KATA=1`; when unset it SHALL `t.Skipf`, not pass.

The runtime class is a gateway-level configuration value
(`server.defaultRuntimeClassName`, rendered as `default_runtime_class_name` in the
gateway configuration). When enabled, the suite SHALL provision a gateway
configured with the Kata RuntimeClass (name from `E2E_KATA_RUNTIME_CLASS`, default
`kata-containers`),
create a sandbox, and assert the resulting Kubernetes pod (read via
`harness.Clients`) carries `spec.runtimeClassName` equal to that RuntimeClass. A
run against a cluster where that RuntimeClass admits the pod SHALL additionally
wait for the pod to reach `Running`; where the qualification cluster provides the
RuntimeClass object but no node handler, asserting the pod's `runtimeClassName`
field is set SHALL suffice and the readiness wait SHALL `t.Skipf`, not fail.

#### Scenario: E2E-COV-03.01 -- Sandbox lands on the configured Kata runtime

- GIVEN `E2E_QUALIFY_KATA=1` and a gateway configured with `default_runtime_class_name = <E2E_KATA_RUNTIME_CLASS>`
- WHEN the suite creates a sandbox on that gateway
- THEN the sandbox pod's `spec.runtimeClassName` SHALL equal `E2E_KATA_RUNTIME_CLASS`
- AND where the cluster can schedule that runtime, the pod SHALL reach `Running` within `E2E_SANDBOX_TIMEOUT`

#### Scenario: E2E-COV-03.02 -- Kata qualification skipped by default

- GIVEN `E2E_QUALIFY_KATA` is unset
- WHEN the suite reaches the `kata-runtime` phase
- THEN it SHALL `t.Skipf` with a reason and continue, and SHALL NOT report a false pass

### Requirement: E2E-COV-04 -- Alerting and Incident Signal Coverage

The e2e test suite SHALL validate that an operational fault surfaces an observable
signal. This is the `alerting-signal` phase (P2.10), long-tagged and gated on
`E2E_QUALIFY_ALERTING=1`; it is deferred for two reasons and SHALL `t.Skipf` (never
false-pass) until both are resolved:

1. **No alert rule.** The repository defines metrics but no alert rules: there is
   no `PrometheusRule` and no alert definition in any spec. Asserting an alert
   fires requires a rule to exist first.

2. **No API-reachable reconcile fault.** The suite previously provoked a metric
   fault by creating a Vault-backed gateway whose serving-CA source Secret is
   absent. Under PLAT-DATA-23 `credential_driver` (and every gateway field except
   `name`) is control-plane-only and rejected on create, so a user can no longer
   provision a gateway that deterministically faults; the serial `vault-fail-closed`
   phase (P2.7) exercises that fault path itself.

When a qualification harness supplies both a reachable fault and an authored alert
rule (plus a reachable Alertmanager/Prometheus), the phase SHALL provoke the rule's
condition, assert the alert reaches `firing`, and assert it clears when the
condition resolves, restoring any state it perturbed on every exit path.

#### Scenario: E2E-COV-04.01 -- Alerting signal deferred by default

- GIVEN `E2E_QUALIFY_ALERTING` is unset (the default while no `PrometheusRule` exists and no API-reachable reconcile fault is available)
- WHEN the suite reaches the `alerting-signal` phase
- THEN it SHALL `t.Skipf` with a reason and SHALL NOT report a false pass

#### Scenario: E2E-COV-04.02 -- Alert firing qualification (deferred)

- GIVEN `E2E_QUALIFY_ALERTING=1`, an authored alert rule, and a reachable Alertmanager/Prometheus
- WHEN the suite provokes the rule's condition
- THEN the corresponding alert SHALL reach `firing` and SHALL clear when the condition resolves

#### Scenario: E2E-COV-04.03 -- Alert firing skipped until the feature exists

- GIVEN `E2E_QUALIFY_ALERTING` is unset (the default while no `PrometheusRule` exists)
- WHEN the suite reaches the alert-firing tier of the `alerting-signal` phase
- THEN it SHALL `t.Skipf` with a reason and continue, and SHALL NOT report a false pass

### Requirement: E2E-COV-05 -- CLI and UI Command Parity Coverage

The e2e test suite SHALL assert that the CLI commands the web console renders are
the same commands the e2e suite executes, and that they stay valid across
OpenShell upgrades. This is the `cli-ui-parity` phase (P2.11), long-tagged. Today
the UI's `buildOpenShellInstallCommand` / `buildProviderCreateCommand`
(`packages/gateway-management-ui/src/gateways/gateway-connections.ts`) and the
suite's version-derivation logic (`openshell_cli_image_tag` in
`tests/e2e/lib.sh`, still Bash and used by the console suite; the Go suite has no
port of it) share version-derivation logic coupled only by the UI's "keep in
sync" comment (which points at the Bash helper) and separate unit tests; nothing
asserts identical command strings end to end.

The suite SHALL derive the install command the UI would render for the live,
reconciled `gateway_version` (invoking the same `buildOpenShellInstallCommand`
logic, or scraping the rendered command from the console's connection tab) and
assert it string-equals the command the suite executes for that same version,
including the version-suffix handling (for example `v0.0.116-rhaiv.6` preserved,
not truncated) and the correct branch of `scripts/install-openshell.sh` (the quay
image path for suffixed versions, the GitHub-release-with-checksum path for plain
semver). It SHALL then execute the UI-rendered install command through
`scripts/install-openshell.sh` with the UI's `OPENSHELL_VERSION` value and assert
the installed `openshell --version` matches the deployed gateway (a Go port of
`openshell_cli_matches_version` from `tests/e2e/lib.sh`, which this phase
introduces), proving the command the console shows a user
actually produces a working, version-matched CLI. The phase SHALL install into an
isolated location (its own temp dir / PATH) so it does not perturb the shared CLI
binary other phases resolve through `resolveOpenshellBin`.

The suite SHALL additionally cover the provider-add command: it SHALL derive the
UI's `buildProviderCreateCommand` (`openshell provider create ... --from-gcloud-adc`)
and execute it (or a credential-free equivalent) end to end against the connected
gateway, asserting the provider is created, so the `--from-gcloud-adc` path has
coverage it lacks today.

Validity across upgrades SHALL be exercised by driving the parity assertion from
the live reconciled `gateway_version` rather than a pinned constant, so a version
bump that breaks suffix handling or branch selection fails this phase.

#### Scenario: E2E-COV-05.01 -- UI install command equals the executed command

- GIVEN the live reconciled `gateway_version` of a `Running` gateway
- WHEN the suite derives the UI-rendered install command and the command it executes for that version
- THEN the two command strings SHALL be equal, including version-suffix handling and the install-script branch selected

#### Scenario: E2E-COV-05.02 -- UI-rendered install command produces a matching CLI

- GIVEN the UI-rendered install command and its `OPENSHELL_VERSION` value
- WHEN the suite runs it through `scripts/install-openshell.sh` into an isolated location
- THEN the installed `openshell --version` SHALL match the deployed gateway version
- AND the checksum-verified GitHub-release branch SHALL be used for plain-semver versions and the quay-image branch for suffixed versions

#### Scenario: E2E-COV-05.03 -- Provider-add command runs end to end

- GIVEN a connected gateway and the UI's `buildProviderCreateCommand` output
- WHEN the suite executes that `openshell provider create ... --from-gcloud-adc` command (or a credential-free equivalent)
- THEN the provider SHALL be created on the gateway, giving the `--from-gcloud-adc` path e2e coverage

#### Scenario: E2E-COV-05.04 -- Parity is driven by the live version, not a constant

- GIVEN an OpenShell upgrade that changes `gateway_version`
- WHEN the `cli-ui-parity` phase runs against the upgraded gateway
- THEN the parity assertions SHALL use the reconciled `gateway_version`, so a regression in suffix handling or branch selection fails the phase rather than passing against a stale pinned value

## Cross-Component Route Coverage Additions

A review of every published REST route against the suite found that route-level
validation (does a route accept input, enforce auth, return the right status, and
persist a record) is already owned by the trex integration tests - each resource
plugin's `integration_test.go` drives List, Get, Create, Patch, Delete, paging,
and search over real HTTP against a testcontainers Postgres, including 401 and 404
cases. e2e adds nothing by re-issuing those routes. e2e adds value only where a
route triggers **cross-component** behavior (an API write the control plane must
reconcile across the api-server, the control plane, and the cluster) that no
phase asserts.

Of the routes no phase exercises today, exactly one carries untested
cross-component behavior: `DELETE /managed_clusters/{id}` (orphan-and-restore),
which extends the existing `managedcluster-lifecycle` phase (P2.4) below. (The
GatewayRelease and GatewayNetwork kinds were removed from the API on 2026-10-06 -
their plugins are gone and registered migrations drop the `gateway_releases` and
`gateway_networks` tables - so `PATCH /gateway_releases/{id}` and the
`/gateway_networks` routes no longer exist and are not coverage targets;
`gateway-release-rollout.spec.md` and `gateway-release-reconciliation.spec.md`
still describe the removed API and need their own reconciliation.) The remaining
uncovered routes are pure CRUD or reads (`GET /roles`, `GET /role_bindings/{id}`,
`GET /metrics/gateways`, metadata/openapi/healthcheck): their
only observable effects are covered by the trex integration tests, by a prior
phase (sandbox-count gRPC write-back at the `sandbox-lifecycle` phase, P2.1), or
by a dedicated unit test (`plugins/gateways/metrics_test.go` for the
`managed_cluster` metric attribution, which
`gateway-managed-cluster-attribution.spec.md` GTW-ATTR-06 assigns to unit tests).
Those SHALL NOT be re-tested in e2e.

### Requirement: E2E-COV-06 -- ManagedCluster Delete and Re-Registration Coverage

The e2e test suite SHALL validate the orphan-on-delete and
restore-on-re-registration semantics in `managed-cluster-registration.spec.md`:
deleting a registered ManagedCluster's record (`DELETE /managed_clusters/{id}`)
detaches the gateways placed on it until the control plane registers again, and
the control plane's next `/registration` restores the **same** `cluster_id` so the
gateways re-attach. This extends the `managedcluster-lifecycle` phase (P2.4),
which today covers idempotent re-registration, the non-registrar 403, the
name-collision 409, and gRPC identity, but never deletes a registered cluster with
a gateway on it. It is long-tagged.

Because deleting a registered cluster's record disrupts every gateway on that
cluster, the suite SHALL run this against a **second** real cluster, gated on
`E2E_MULTICLUSTER=1`, so the primary cluster the rest of the suite depends on is
never disrupted; on a single-cluster run it SHALL `t.Skipf` (a single real control
plane cannot be deleted without breaking the run, and a synthetic record cannot
hold a gateway). It SHALL place a throwaway gateway on the second cluster, delete
that cluster's record, assert the gateway is detached (no longer reconciled)
within a bounded window, then wait for the second control plane's next
`/registration` to restore the same `cluster_id` and the gateway to re-attach,
leaving the fleet as it found it. The suite SHALL delete the throwaway gateway on
all exit paths.

#### Scenario: E2E-COV-06.01 -- Deleting a registered cluster orphans its gateways

- GIVEN `E2E_MULTICLUSTER=1` and a throwaway gateway on the second registered cluster
- WHEN the suite deletes that cluster's record
- THEN the gateway SHALL be detached and no longer reconciled within a bounded window

#### Scenario: E2E-COV-06.02 -- Re-registration restores the cluster and re-attaches gateways

- GIVEN a deleted second-cluster record whose control plane is still running
- WHEN that control plane issues its next `/registration`
- THEN the record SHALL be restored with the same `cluster_id` and the gateway SHALL re-attach and reconcile again

#### Scenario: E2E-COV-06.03 -- Single-cluster run skips the delete

- GIVEN a single-cluster run (no `E2E_MULTICLUSTER`)
- WHEN the suite reaches the cluster-delete sub-check of `managedcluster-lifecycle`
- THEN it SHALL `t.Skipf`, because deleting the only registered cluster would break the run and a synthetic record cannot hold a gateway

### Requirement: E2E-COV-07 -- Phase Placement and Mode Tags

The five feature phases SHALL register as new long-tagged steps of `E2ESuite`
in `p2ParallelSteps()` (and mirrored in the scheduler golden order), and the
ManagedCluster delete coverage as a sub-check of the existing P2.4 phase. All
SHALL be long-tagged: they run only in `long` mode and are skipped
(`t.Skipf`/`modeAllows`) in `short` mode and in the matrix's default short runs,
consistent with the long-step gating P2.4-P2.6 use. A sub-check that cannot run
because its opt-in backend is absent SHALL `t.Skipf` with a reason, never record a
pass. `e2e-testing.spec.md`'s phase list SHALL be amended to name these steps
rather than restating their content here; this spec owns their content.

| Proposed step | Slug | Mode | Gate |
|------|-------|------|------|
| P2.8 | `gateway-sharing-rolebindings` | long | long-tagged; owns a dedicated gateway (no primary-gateway race with P2.5) |
| P2.9 | `kata-runtime` | long | `E2E_QUALIFY_KATA=1` |
| P2.10 | `alerting-signal` | long | `E2E_QUALIFY_ALERTING=1` (deferred: no PrometheusRule yet, and the reconcile-fault provocation is not API-reachable under PLAT-DATA-23) |
| P2.11 | `cli-ui-parity` | long | none (Kind-hostable) |
| P2.4 (ext) | `managedcluster-lifecycle` | long | `E2E_MULTICLUSTER=1` (single-cluster skips the delete) |

#### Scenario: E2E-COV-07.01 -- New phases skipped in short mode

- GIVEN `E2E_MODE=short` (including the matrix's default short runs)
- WHEN the suite runs
- THEN each new phase above SHALL `t.Skipf` and SHALL NOT execute, exactly as P2.4-P2.6 do
- AND `E2E_MODE=perf` SHALL remain an invalid mode (performance is a separate benchmark)

#### Scenario: E2E-COV-07.02 -- New phases run in long mode

- GIVEN `E2E_MODE=long` (or unset)
- WHEN the suite runs
- THEN `gateway-sharing-rolebindings` and `cli-ui-parity` SHALL execute on every target
- AND `kata-runtime` and `alerting-signal` SHALL execute only when their opt-in variable is set, otherwise `t.Skipf`
- AND the P2.4 cluster-delete sub-check SHALL execute only under `E2E_MULTICLUSTER=1` and otherwise `t.Skipf`

## Environment Variables

These extend the variables the Go suite already reads (via `envOrDefault` /
`os.Getenv`); they are introduced by this spec.

| Env Var | Default | Description |
|---------|---------|-------------|
| `E2E_QUALIFY_KATA` | `0` | `1` runs the `kata-runtime` phase; requires a Kata RuntimeClass on the cluster |
| `E2E_KATA_RUNTIME_CLASS` | `kata-containers` | RuntimeClass name the `kata-runtime` gateway configures and asserts on the sandbox pod |
| `E2E_QUALIFY_ALERTING` | `0` | `1` runs the `alerting-signal` phase; deferred until a PrometheusRule and an API-reachable reconcile fault exist, so the phase skips when unset |

The `gateway-sharing-rolebindings` phase reuses the suite's existing
`E2E_DEV_USERNAME` / `E2E_DEV_PASSWORD` (the seeded second identity) and the admin
token (`authentication` phase P0.1, `admin-inventory-validation` phase P3.1) for
`/users` discovery, and resolves the `gateway:viewer` role id from `Roles().List`.
The `alerting-signal` phase is deferred (see Requirement E2E-COV-04): no
PrometheusRule is authored yet, and the reconcile-fault provocation it relied on is
no longer reachable through the API under PLAT-DATA-23, so it skips unless
`E2E_QUALIFY_ALERTING=1`.
The `cli-ui-parity` phase reuses `OPENSHELL_BIN` and
`scripts/install-openshell.sh`, and ports the version helpers
(`openshell_cli_image_tag`, `openshell_cli_matches_version`) from
`tests/e2e/lib.sh` (the Go suite has no version-derivation logic today). The
cross-component addition reuses the existing `E2E_MULTICLUSTER` and
`E2E_SEED_CLUSTER_NAME_2`.

## Design Decisions

- **New phase steps in the Go suite, not a parallel structure.** The five feature
  gaps plus the cluster-delete addition register as
  long-tagged steps of the existing `E2ESuite` (`p2ParallelSteps()` / the P2.4
  phase), reusing the driver contract, typed `apiclient`, `harness` helpers, and
  the parallel scheduler. This keeps one suite and one mode model; the epic's
  child stories can each reference the matching requirement section here.
- **Behavior coverage, not a route checklist.** A review of every published REST
  route found route-level validation (status codes, auth, persistence, paging,
  search) is already owned by the trex `integration_test.go` suites over real
  HTTP, so re-issuing those routes in e2e adds nothing. e2e earns its cost only
  where a route drives cross-component reconciliation no phase asserts - which,
  across the whole uncovered set, is exactly one route
  (`DELETE /managed_clusters/{id}` orphan-and-restore; the GatewayRelease and
  GatewayNetwork routes this analysis once counted were removed from the API on
  2026-10-06). Everything else uncovered is pure CRUD or is covered at a
  better layer (a prior phase, or a unit test the owning spec assigns), so it is
  left to those layers.
- **Opt-in backends skip, they do not block.** Vault, Kata, and alert-firing need
  infrastructure the Kind CI environment does not provide. Rather than reusing
  the Bash suite's `E2E_QUALIFY_*` qualification flags (still live in
  `tests/e2e/lib.sh` for the console suite), each is a new opt-in flag of the Go
  suite whose step `t.Skipf`s (the suite's three-outcome skip idiom)
  when the flag or infrastructure is absent - never a pass, never a failure. The
  Kind-hostable halves (Kubernetes Secrets, the `role_bindings` sharing flow,
  CLI/UI parity, the metric tier of alerting) run in long mode on
  every target.
- **Alerting follows the feature.** No alert artifact exists in the repository
  today. The actionable near-term coverage asserts the reconcile failure *metric*
  moves; the alert-*firing* contract is recorded but deferred behind
  `E2E_QUALIFY_ALERTING` so the e2e lands with, not before, a `PrometheusRule`.
- **Sharing covers the `role_bindings` surface distinctly.** `role_bindings` is an
  active public API alongside `/gateways/{id}/access`, and both surfaces write the
  same underlying RoleBinding records; the existing `gateway-access-management`
  phase (P2.5) covers `/access`, so the new phase drives the `role_bindings` REST
  contract itself - binding by `role_id`, the non-owner authorization guard, the
  Keycloak-only role refusals, and binding delete - which the `/access` phase
  never touches.
