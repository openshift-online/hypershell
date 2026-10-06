# E2E Coverage for Credential Drivers, Gateway Sharing, Kata, Alerting, and CLI/UI Parity

**Date:** 2026-10-06
**Status:** Draft
**Jira:** HYPERSHELL-368
**Related:** `e2e-testing.spec.md` (driver contract, `E2E_MODE` short/long/perf tags, the numbered area model this spec extends, the TLS-trust rule, the `E2E_QUALIFY_*` opt-in pattern);
             `e2e-console-browser-testing.spec.md` (browser suite; CON-E2E-14 install-docs link);
             `openshell-gateway-credentials.spec.md` (credential driver fields, TOML, RBAC, Vault SA token, KEK);
             `managed-cluster-registration.spec.md` and the Admin Inventory area (user-id discovery via `/v1/users`);
             `openshell-gateway-oidc.spec.md` and `specs/security/rbac-enforcement.spec.md` (`gateway:owner`/`gateway:viewer` -> `openshell-admin`/`openshell-user`);
             `control-plane-reconciliation-metrics.spec.md` (reconcile outcome/error metrics the alerting area asserts on)

---

## Purpose

Five platform features are implemented and shipped but carry no (or only
indirect) end-to-end coverage in `tests/e2e/e2e-openshell.sh` and
`tests/e2e/e2e-console.sh`: per-gateway provider credential storage drivers
(Vault and Kubernetes Secrets), sharing a gateway between users through the
`role_bindings` API, Kata-runtime sandboxes, automated alerting / incident
signal, and parity between the CLI commands the web console renders and the
commands the e2e suite actually executes. This specification defines the e2e
coverage that closes those gaps.

The five feature gaps become **numbered areas 15-19** of the functional e2e suite,
continuing the area model defined by the E2E Test Suite Coverage requirement in
`e2e-testing.spec.md` (areas 1-14). This spec additionally defines two
cross-component coverage **extensions to existing areas 12 and 13** (see
[Cross-Component Route Coverage Extensions](#cross-component-route-coverage-extensions)),
which close the only two API routes whose cross-component behavior no area asserts
today. Like areas 12-14, every area here is
**long-only**: it is skipped in `short` and `perf` modes (it mutates shared
state, acts as a second identity, or depends on infrastructure the quick gate
does not provide). Each area reuses the existing driver contract and harness
vocabulary (`e2e_area`, `e2e_step`, `e2e_multi_identity`, `api_curl`,
`acquire_oidc_token`, `acquire_gateway_token_with_role`, `retry_until`, `pass`,
`fail_test`) and adds no new required driver functions.

Backends that the Kind CI gate cannot host (a running Vault server; a
`kata-containers` RuntimeClass) are gated behind opt-in `E2E_QUALIFY_*` variables
in the same manner as the existing DNS/TLS-renewal, DB-rotation, backup/restore,
and restricted-registry qualifications. The halves that Kind can host (the
Kubernetes Secrets credential driver, the full gateway-sharing flow, CLI/UI
command parity, and the metric-signal half of alerting) run in long mode on
every target, including the Kind merge-queue gate's long runs.

### Scope

In scope: the five new functional e2e areas (15-19), the two cross-component
coverage extensions to existing areas 12-13, their short/long-mode placement, and
the environment variables they add. Out of scope: the feature
implementations themselves (owned by the specs cross-referenced above), the
browser console suite's own areas (owned by `e2e-console-browser-testing.spec.md`,
except where this spec notes a browser counterpart), and authoring a
`PrometheusRule` for the alerting feature (see
[Alerting and Incident Signal Coverage](#requirement-alerting-and-incident-signal-coverage)).

---

## Requirements

### Requirement: Provider Credential Driver Coverage

The e2e test suite SHALL validate the per-gateway credential storage drivers
specified in `openshell-gateway-credentials.spec.md`, not only the default
encrypted-database store. This is area 15 and is long-only (it provisions
additional gateways and, for Vault, depends on external infrastructure).

The suite SHALL cover the **Kubernetes Secrets** driver unconditionally in long
mode on every target, because Kind can host it. It SHALL provision a gateway
with `credential_driver.type = kubernetes-secrets` through
`POST /api/hypershell/v1/gateways`, wait for `Running`, and assert the
control-plane reconciled the driver: the generated gateway configuration carries
`credential_drivers = ["kubernetes-secrets"]` and omits
`[openshell.gateway.credential_storage]`, the Secret
`openshell-gateway-credential-kek` is absent, the KEK env var
`OPENSHELL_GATEWAY_CREDENTIAL_KEY_ENCRYPTION_KEY` is absent from the gateway
Deployment, and the Role and RoleBinding `openshell-gateway-credential-secrets`
exist in the credential namespace bound to the `openshell-gateway`
ServiceAccount.

The suite SHALL cover the **Vault** driver only when `E2E_QUALIFY_VAULT=1` and a
reachable Vault is supplied through `E2E_VAULT_ADDR` / `E2E_VAULT_ROLE`, because
the Kind gate hosts no Vault server. When enabled, it SHALL provision a gateway
with `credential_driver.type = vault`, wait for `Running`, and assert the
generated configuration carries `credential_drivers = ["vault"]`, omits the KEK
sections, and that the gateway Deployment mounts a projected ServiceAccount token
volume with audience `vault` at `/var/run/secrets/vault`. When
`E2E_QUALIFY_VAULT` is unset the suite SHALL record a skip for the Vault steps,
never a false pass.

The suite SHALL assert the **default** driver is unaffected: the primary gateway
from area 2 (no `credential_driver`) still provisions its KEK Secret and
`[openshell.gateway.credential_storage]` section. It SHALL also assert the two
negative paths from `openshell-gateway-credentials.spec.md`: an unsupported
`type` is rejected by the API or surfaced as a reconcile error, and a
driver-change after credentials exist is rejected.

Credential-driver gateways SHALL be owned and torn down by the area (delete plus
namespace-GC wait, as area 11 does for the run's own gateway), leaving nothing
behind.

#### Scenario: Kubernetes Secrets driver reconciled

- GIVEN a long-mode run on any target
- WHEN the suite creates a gateway with `credential_driver.type = kubernetes-secrets` and waits for `Running`
- THEN the generated gateway configuration SHALL contain `credential_drivers = ["kubernetes-secrets"]` and SHALL NOT contain `[openshell.gateway.credential_storage]`
- AND the Secret `openshell-gateway-credential-kek` SHALL NOT exist and the env var `OPENSHELL_GATEWAY_CREDENTIAL_KEY_ENCRYPTION_KEY` SHALL NOT be on the Deployment
- AND a Role and RoleBinding `openshell-gateway-credential-secrets` SHALL exist in the credential namespace, bound to the `openshell-gateway` ServiceAccount

#### Scenario: Vault driver qualification (opt-in)

- GIVEN `E2E_QUALIFY_VAULT=1` with `E2E_VAULT_ADDR` and `E2E_VAULT_ROLE` set to a reachable Vault
- WHEN the suite creates a gateway with `credential_driver.type = vault` and waits for `Running`
- THEN the generated configuration SHALL contain `credential_drivers = ["vault"]` and SHALL omit the KEK sections
- AND the gateway Deployment SHALL mount a projected ServiceAccount token volume with audience `vault` at `/var/run/secrets/vault`

#### Scenario: Vault qualification skipped by default

- GIVEN `E2E_QUALIFY_VAULT` is unset
- WHEN the suite reaches the Vault steps of area 15
- THEN it SHALL record a skip and continue, and SHALL NOT report a false pass

#### Scenario: Default driver unaffected

- GIVEN the primary gateway created in area 2 with no `credential_driver`
- WHEN area 15 inspects its credential configuration
- THEN the `[openshell.gateway.credential_storage]` section and the KEK Secret `openshell-gateway-credential-kek` SHALL be present, proving the external drivers did not regress the default path

#### Scenario: Invalid driver and post-storage change rejected

- GIVEN a gateway-create request with `credential_driver.type = s3`
- WHEN the suite submits it through the API
- THEN the request SHALL be rejected (at the API, or surfaced as a reconcile error) rather than silently accepted
- AND when the suite attempts to change a gateway's `credential_driver` after provider credentials have been stored, the API SHALL reject the update

### Requirement: Gateway Sharing Coverage

The e2e test suite SHALL validate a complete "owner shares a gateway with a
second user" flow through the `role_bindings` API, exercising the product path
that area 9's `assign_gateway_client_role` driver shortcut currently bypasses.
This is area 16, long-only (it acts as a second identity and mutates bindings),
and gated on `e2e_multi_identity`.

The flow SHALL use the gateway owned by the admin (the owner RoleBinding the
control plane auto-creates at provisioning). The suite SHALL discover the second
user's id through the admin `/v1/users` inventory (ensuring that user record
exists first by acquiring a token for `E2E_DEV_USERNAME`), then as the owner
`POST /api/hypershell/v1/role_bindings` with `scope = gateway`,
`role = gateway:viewer`, `user_id = <second user>`, and `gateway_id = <owned gateway>`.
It SHALL then prove the RoleBinding reconciler propagated the grant into Keycloak
by acquiring a per-gateway token for the second user with
`acquire_gateway_token_with_role` and asserting the `openshell-user` role lands
in it, and by confirming the second user MAY now create a sandbox on that gateway.

The suite SHALL then revoke access: `DELETE /role_bindings/<id>` as the owner and
assert the second user loses the grant (a freshly minted per-gateway token no
longer carries `openshell-user`, and a sandbox create is denied), confirming the
reconciler's delete-side role recomputation. It SHALL also assert the
authorization guard: a caller who is not `gateway:owner` on the target gateway
SHALL be rejected when creating a `gateway:viewer`/`gateway:owner` binding, and a
`POST /role_bindings` for `gateway:creator` or `platform:admin` SHALL be refused
(those are Keycloak-only).

The suite SHALL clean up any binding it creates on every exit path.

#### Scenario: Owner shares viewer access and the reconciler propagates it

- GIVEN the admin owns a `Running` gateway and the second user's id has been resolved from `/v1/users`
- WHEN the owner `POST`s a `role_bindings` entry with `scope = gateway`, `role = gateway:viewer`, and the second user's id
- THEN `acquire_gateway_token_with_role` for the second user SHALL observe the `openshell-user` role on the per-gateway client within the role-reconcile timeout
- AND the second user SHALL be able to create a sandbox on that gateway

#### Scenario: Revoking the binding removes access

- GIVEN a `gateway:viewer` binding the owner created for the second user
- WHEN the owner `DELETE`s that binding
- THEN a newly minted per-gateway token for the second user SHALL NOT carry `openshell-user`
- AND a sandbox create by the second user SHALL be denied

#### Scenario: Non-owner cannot share and privileged roles are refused

- GIVEN a caller who does not hold `gateway:owner` on the target gateway
- WHEN that caller `POST`s a `gateway:viewer` binding for the gateway
- THEN the API SHALL reject the request
- AND a `POST /role_bindings` requesting `gateway:creator` or `platform:admin` SHALL be refused as Keycloak-only, regardless of caller

### Requirement: Kata Runtime Sandbox Coverage

The e2e test suite SHALL validate that a sandbox honors a configured Kata
RuntimeClass, closing the gap that the suite exercises only the cluster-default
runtime. This is area 17, long-only. Because the Kind gate hosts no
`kata-containers` RuntimeClass, the area is gated on `E2E_QUALIFY_KATA=1`; when
unset the suite SHALL record a skip, not a pass.

The runtime class is a gateway-level configuration value
(`server.defaultRuntimeClassName`, rendered as `default_runtime_class_name` in
the gateway configuration), with a per-request override possible on sandbox
create. When enabled, the suite SHALL provision a gateway configured with the
Kata RuntimeClass (name from `E2E_KATA_RUNTIME_CLASS`, default
`kata-containers`), create a sandbox, and assert the resulting Kubernetes pod
carries `spec.runtimeClassName` equal to that RuntimeClass. A run against a
cluster where that RuntimeClass admits the pod SHALL additionally wait for the
pod to reach `Running`; where the qualification cluster provides the
RuntimeClass object but no node handler, asserting the pod's `runtimeClassName`
field is set SHALL suffice and the readiness wait SHALL be reported as a skip,
not a failure.

#### Scenario: Sandbox lands on the configured Kata runtime

- GIVEN `E2E_QUALIFY_KATA=1` and a gateway configured with `default_runtime_class_name = <E2E_KATA_RUNTIME_CLASS>`
- WHEN the suite creates a sandbox on that gateway
- THEN the sandbox pod's `spec.runtimeClassName` SHALL equal `E2E_KATA_RUNTIME_CLASS`
- AND where the cluster can schedule that runtime, the pod SHALL reach `Running` within `E2E_SANDBOX_TIMEOUT`

#### Scenario: Kata qualification skipped by default

- GIVEN `E2E_QUALIFY_KATA` is unset
- WHEN the suite reaches area 17
- THEN it SHALL record a skip and continue, and SHALL NOT report a false pass

### Requirement: Alerting and Incident Signal Coverage

The e2e test suite SHALL validate that an operational fault surfaces an
observable signal. This is area 18, long-only.

The repository today defines **metrics but no alert rules**: there is no
`PrometheusRule` and no alert definition in any spec (the observability and
reconciliation-metrics specs define counters and histograms only). The area
therefore has two tiers:

1. **Metric signal (unconditional, long mode).** The suite SHALL provoke a
   reconcile fault condition and assert the corresponding control-plane metric
   moves. It SHALL read the control-plane `/metrics` endpoint, capture a baseline
   for the reconcile outcome/error series defined in
   `control-plane-reconciliation-metrics.spec.md`, drive a condition that fails a
   reconcile (for example a gateway referencing an unresolvable release or a
   provisioning precondition that cannot be met), and assert the failure series
   increments while the condition holds. This is the actionable near-term
   coverage and is testable on Kind.

2. **Alert firing (deferred, opt-in).** Asserting that an alert actually fires
   requires an alert rule to exist first. Until a `PrometheusRule` (or equivalent
   alerting artifact) is authored for the platform, this tier SHALL be gated on
   `E2E_QUALIFY_ALERTING=1` and SHALL record a skip when unset. When a rule and an
   Alertmanager are present, the suite SHALL provoke the rule's condition and
   assert the alert reaches `firing` (via the Alertmanager/Prometheus API) and is
   cleared when the condition resolves. This tier SHALL NOT be added to the Kind
   gate until the alerting feature lands; its presence here records the contract
   so the e2e follows the feature rather than blocking on it.

The suite SHALL restore any state it perturbed (delete the fault-inducing
gateway, wait out its namespace GC) on every exit path.

#### Scenario: Reconcile fault increments the failure metric

- GIVEN a baseline reading of the reconcile failure/outcome series from the control-plane `/metrics` endpoint
- WHEN the suite drives a condition that fails a reconcile
- THEN the failure series SHALL increment above the baseline within the provisioning timeout
- AND the suite SHALL clear the condition and clean up the gateway it created

#### Scenario: Alert firing qualification (deferred)

- GIVEN `E2E_QUALIFY_ALERTING=1`, an authored alert rule, and a reachable Alertmanager/Prometheus
- WHEN the suite provokes the rule's condition
- THEN the corresponding alert SHALL reach `firing` and SHALL clear when the condition resolves

#### Scenario: Alert firing skipped until the feature exists

- GIVEN `E2E_QUALIFY_ALERTING` is unset (the default while no `PrometheusRule` exists)
- WHEN the suite reaches the alert-firing tier of area 18
- THEN it SHALL record a skip and continue, and SHALL NOT report a false pass

### Requirement: CLI and UI Command Parity Coverage

The e2e test suite SHALL assert that the CLI commands the web console renders are
the same commands the e2e suite executes, and that they stay valid across
OpenShell upgrades. This is area 19, long-only. Today the UI's
`buildOpenShellInstallCommand` / `buildProviderCreateCommand`
(`packages/gateway-management-ui/src/gateways/gateway-connections.ts`) and the
suite's `openshell_cli_image_tag` (`tests/e2e/lib.sh`) share version-derivation
logic coupled only by a "keep in sync" comment and separate unit tests; nothing
asserts identical command strings end to end.

The suite SHALL derive the install command the UI would render for the live,
reconciled `gateway_version` (invoking the same `buildOpenShellInstallCommand`
logic, or scraping the rendered command from the console's connection tab) and
assert it string-equals the command the suite executes for that same version,
including the version-suffix handling (for example `v0.0.116-rhaiv.6` preserved,
not truncated) and the correct branch of `scripts/install-openshell.sh` (the
quay image path for suffixed versions, the GitHub-release-with-checksum path for
plain semver). It SHALL then execute the UI-rendered install command through
`scripts/install-openshell.sh` with the UI's `OPENSHELL_VERSION` value and assert
the installed `openshell --version` matches the deployed gateway
(`openshell_cli_matches_version`), proving the command the console shows a user
actually produces a working, version-matched CLI.

The suite SHALL additionally cover the provider-add command: it SHALL derive the
UI's `buildProviderCreateCommand` (`openshell provider create ... --from-gcloud-adc`)
and execute it (or a credential-free equivalent) end to end against the connected
gateway, asserting the provider is created, so the `--from-gcloud-adc` path has
coverage it lacks today.

Validity across upgrades SHALL be exercised by driving the parity assertion from
the live reconciled `gateway_version` rather than a pinned constant, so a version
bump that breaks suffix handling or branch selection fails this area.

#### Scenario: UI install command equals the executed command

- GIVEN the live reconciled `gateway_version` of a `Running` gateway
- WHEN the suite derives the UI-rendered install command and the command it executes for that version
- THEN the two command strings SHALL be equal, including version-suffix handling and the install-script branch selected

#### Scenario: UI-rendered install command produces a matching CLI

- GIVEN the UI-rendered install command and its `OPENSHELL_VERSION` value
- WHEN the suite runs it through `scripts/install-openshell.sh`
- THEN the installed `openshell --version` SHALL match the deployed gateway version (`openshell_cli_matches_version`)
- AND the checksum-verified GitHub-release branch SHALL be used for plain-semver versions and the quay-image branch for suffixed versions

#### Scenario: Provider-add command runs end to end

- GIVEN a connected gateway and the UI's `buildProviderCreateCommand` output
- WHEN the suite executes that `openshell provider create ... --from-gcloud-adc` command (or a credential-free equivalent)
- THEN the provider SHALL be created on the gateway, giving the `--from-gcloud-adc` path e2e coverage

#### Scenario: Parity is driven by the live version, not a constant

- GIVEN an OpenShell upgrade that changes `gateway_version`
- WHEN area 19 runs against the upgraded gateway
- THEN the parity assertions SHALL use the reconciled `gateway_version`, so a regression in suffix handling or branch selection fails the area rather than passing against a stale pinned value

## Cross-Component Route Coverage Extensions

A review of every published REST route against the e2e suite found that
route-level validation (does a route accept input, enforce auth, return the right
status, and persist a record) is already owned by the trex integration tests -
each resource plugin's `integration_test.go` drives List, Get, Create, Patch,
Delete, paging, and search over real HTTP against a testcontainers Postgres,
including 401 and 404 cases. e2e adds nothing by re-issuing those routes. e2e adds
value only where a route triggers **cross-component** behavior (an API write that
the control plane must reconcile across the api-server, the control plane, and the
cluster) that no existing area asserts.

Of the routes no area exercises today, exactly two carry untested cross-component
behavior. Both are extensions to existing long-only areas, not new areas:
`PATCH /gateway_releases/{id}` (area 13) and `DELETE /managed_clusters/{id}`
(area 12). The remaining uncovered routes are pure CRUD or reads
(`GET /roles`, `GET /gateway_releases` List, `GET /role_bindings/{id}`,
`PATCH`/`GET` `/gateway_networks`, `GET /metrics/gateways`, metadata/openapi/
healthcheck) whose only observable effect is already covered - by the trex
integration tests, by a prior e2e area (gateway-network `status` write-back at
area 13; sandbox-count gRPC flow at areas 7-8), or by a dedicated unit test
(`plugins/gateways/metrics_test.go` for the `managed_cluster` metric attribution,
which `gateway-managed-cluster-attribution.spec.md` GMCA-06 assigns to unit
tests). Those SHALL NOT be re-tested in e2e.

### Requirement: Release Image Fan-Out Coverage

The e2e test suite SHALL validate the release-image fan-out path:
updating a referenced `GatewayRelease`'s image (`PATCH /gateway_releases/{id}`)
and asserting that every gateway referencing that release rolls to the new image,
as `gateway-release-rollout.spec.md` and `gateway-release-reconciliation.spec.md`
define. This extends area 13, which today exercises only the other rollout trigger
- repointing a single gateway's `release_id` - and never mutates a shared
release's image. It is long-only.

Against a dedicated disposable release and gateway (so the seeded release and
other gateways are not perturbed), the suite SHALL: create a release, create a
gateway referencing it and wait for `Running`, then `PATCH` that release's image to
a new valid image and assert the referencing gateway rolls revision-aware and
last-good-preserving - the prior revision stays serving until the new revision is
Ready, `observed_release_id` advances only after the health gates pass, and a
subsequent `PATCH` to an unpullable image surfaces `Degraded` while preserving the
last-good revision. Under `E2E_MULTICLUSTER=1`, the suite SHALL place two gateways
referencing the same release on different clusters and assert the single release
`PATCH` fans out to both, each control plane rolling its own gateway. The suite
SHALL delete every release and gateway it created on all exit paths.

#### Scenario: Release image update fans out to referencing gateways

- GIVEN a disposable release and a `Running` gateway referencing it
- WHEN the suite `PATCH`es the release's image to a new valid image
- THEN the referencing gateway SHALL roll to the new image, the prior revision SHALL stay serving until the new revision is Ready, and `observed_release_id` SHALL advance only after the health gates pass

#### Scenario: Failed image roll preserves last-good

- GIVEN a `Running` gateway referencing a disposable release
- WHEN the suite `PATCH`es the release to an unpullable image
- THEN the gateway SHALL surface `Degraded` and SHALL keep serving the last-good revision rather than going down

#### Scenario: Fan-out spans clusters

- GIVEN `E2E_MULTICLUSTER=1` and two gateways on different clusters referencing one release
- WHEN the suite `PATCH`es that release's image once
- THEN both gateways SHALL roll to the new image, each reconciled by its own control plane

### Requirement: ManagedCluster Delete and Re-Registration Coverage

The e2e test suite SHALL validate the orphan-on-delete and
restore-on-re-registration semantics in `managed-cluster-registration.spec.md`:
deleting a registered ManagedCluster's record (`DELETE /managed_clusters/{id}`)
detaches the gateways placed on it until the control plane registers again, and
the control plane's next `/registration` restores the **same** `cluster_id` so the
gateways re-attach. This extends area 12, which today deletes only the inert
placeholder record (empty `oidc_subject`, which by spec holds no gateways) and
never a registered cluster with a gateway on it. It is long-only.

Because deleting a registered cluster's record disrupts every gateway on that
cluster, the suite SHALL run this against a **second** real cluster, gated on
`E2E_MULTICLUSTER=1`, so the primary cluster the rest of the suite depends on is
never disrupted; on a single-cluster run it SHALL record a skip (a single real
control plane cannot be deleted without breaking the run, and a synthetic record
cannot hold a gateway). It SHALL place a throwaway gateway on the second cluster,
delete that cluster's record, assert the gateway is detached (no longer
reconciled) within a bounded window, then wait for the second control plane's next
`/registration` to restore the same `cluster_id` and the gateway to re-attach,
leaving the fleet as it found it. The suite SHALL delete the throwaway gateway on
all exit paths.

#### Scenario: Deleting a registered cluster orphans its gateways

- GIVEN `E2E_MULTICLUSTER=1` and a throwaway gateway on the second registered cluster
- WHEN the suite deletes that cluster's record
- THEN the gateway SHALL be detached and no longer reconciled within a bounded window

#### Scenario: Re-registration restores the cluster and re-attaches gateways

- GIVEN a deleted second-cluster record whose control plane is still running
- WHEN that control plane issues its next `/registration`
- THEN the record SHALL be restored with the same `cluster_id` and the gateway SHALL re-attach and reconcile again

#### Scenario: Single-cluster run skips the delete

- GIVEN a single-cluster run (no `E2E_MULTICLUSTER`)
- WHEN the suite reaches the cluster-delete coverage
- THEN it SHALL record a skip, because deleting the only registered cluster would break the run and a synthetic record cannot hold a gateway

### Requirement: Area Placement and Mode Tags

Areas 15-19 SHALL extend the numbered area model in `e2e-testing.spec.md`, and the
two cross-component extensions above SHALL extend existing areas 13 and 12. All
SHALL be long-only: skipped in `short` and `perf` modes, consistent with the
`e2e_step long` / `e2e_multi_identity` gating that areas 12-14 use. The E2E Test
Suite Coverage requirement and the short/long mode table in `e2e-testing.spec.md`
SHALL be amended to list areas 15-19 as long-only and to note the area-12/13
extensions rather than restating them here; this spec owns their content.

| Area | Title | Mode | Gate |
|------|-------|------|------|
| 15 | Provider credential drivers | long-only | Kubernetes Secrets always; Vault on `E2E_QUALIFY_VAULT=1` |
| 16 | Gateway sharing via `role_bindings` | long-only | `e2e_multi_identity` |
| 17 | Kata runtime sandbox | long-only | `E2E_QUALIFY_KATA=1` |
| 18 | Alerting / incident signal | long-only | metric tier always; alert-firing on `E2E_QUALIFY_ALERTING=1` |
| 19 | CLI/UI command parity | long-only | none (Kind-hostable) |
| 13 (ext) | Release image fan-out | long-only | `E2E_MULTICLUSTER=1` for the cross-cluster scenario |
| 12 (ext) | ManagedCluster delete + re-registration | long-only | `E2E_MULTICLUSTER=1` (single-cluster skips) |
| 20 | API route coverage completeness | long-only | none (Kind-hostable) |

#### Scenario: New areas skipped in short and perf

- GIVEN `E2E_MODE=short` or `E2E_MODE=perf`
- WHEN the suite runs
- THEN areas 15-19 SHALL each print a skip notice and SHALL NOT execute, exactly as areas 12-14 do
- AND the area-12/13 cross-component extensions, being long-only, SHALL likewise not execute

#### Scenario: New areas run in long mode

- GIVEN `E2E_MODE=long` (or unset)
- WHEN the suite runs
- THEN areas 15, 16, and 19 SHALL execute on every target
- AND areas 17 and 18's infra-gated tiers SHALL execute only when their `E2E_QUALIFY_*` variable is set, otherwise record a skip
- AND the area-13 release fan-out extension SHALL execute on every target (its cross-cluster scenario only under `E2E_MULTICLUSTER=1`), while the area-12 cluster-delete extension SHALL execute only under `E2E_MULTICLUSTER=1` and otherwise record a skip

## Environment Variables

These extend the table in `e2e-testing.spec.md`.

| Env Var | Default | Description |
|---------|---------|-------------|
| `E2E_QUALIFY_VAULT` | `0` | `1` runs the Vault credential-driver qualification (area 15); requires a reachable Vault |
| `E2E_VAULT_ADDR` | (unset) | Vault server URL for the Vault qualification |
| `E2E_VAULT_ROLE` | (unset) | Vault role for the gateway's Kubernetes auth in the Vault qualification |
| `E2E_QUALIFY_KATA` | `0` | `1` runs the Kata runtime sandbox qualification (area 17); requires a Kata RuntimeClass on the cluster |
| `E2E_KATA_RUNTIME_CLASS` | `kata-containers` | RuntimeClass name the area 17 gateway configures and asserts on the sandbox pod |
| `E2E_QUALIFY_ALERTING` | `0` | `1` runs the deferred alert-firing tier of area 18; requires an authored alert rule and a reachable Alertmanager/Prometheus |
| `E2E_SHARE_USERNAME` | `${E2E_DEV_USERNAME}` | The second user with whom area 16 shares the gateway (defaults to the developer identity) |

Area 16 reuses `E2E_DEV_USERNAME` / `E2E_DEV_PASSWORD` for the second identity and
the admin token (areas 1, 14) for `/v1/users` discovery. Area 18's metric tier
reads the control-plane `/metrics` endpoint already used by the observability
specs. Area 19 reuses `OPENSHELL_INSTALL_SCRIPT_URL`, `E2E_OPENSHELL_CLI_IMAGE`,
and the version helpers already defined for the CLI install path.

## Design Decisions

- **One spec, five areas plus two targeted extensions, not fragment files.** The
  epic's five rows share the e2e driver contract, the mode model, and the area
  numbering; batching them as areas 15-19 keeps the area count in one place,
  exactly as areas 12-14 were added together. The epic's child stories can each
  reference the matching requirement section here.
- **Route coverage is behavior coverage, not a route checklist.** A review of
  every published REST route found that route-level validation (status codes,
  auth, persistence, paging, search) is already owned by the trex
  `integration_test.go` suites over real HTTP, so re-issuing those routes in e2e
  adds nothing. e2e earns its cost only where a route drives cross-component
  reconciliation no area asserts - which, across the whole uncovered set, is
  exactly two routes (`PATCH /gateway_releases/{id}` fan-out, and
  `DELETE /managed_clusters/{id}` orphan-and-restore). Those become extensions to
  the areas that already own those resources (13 and 12), not a catch-all route
  area. Everything else uncovered is pure CRUD or is covered at a better layer
  (prior e2e area, or a unit test the owning spec assigns), so it is deliberately
  left to those layers.
- **Infra-heavy backends are opt-in, not gate-blocking.** Vault, Kata, and
  alert-firing need infrastructure the Kind merge-queue gate does not provide, so
  they follow the established `E2E_QUALIFY_*` opt-in pattern and record skips (not
  passes, not failures) when their infrastructure is absent. The Kind-hostable
  halves (Kubernetes Secrets, the full sharing flow, CLI/UI parity, the metric
  tier of alerting) run in long mode on every target.
- **Alerting follows the feature.** No alert artifact exists in the repository
  today. The actionable near-term coverage asserts the reconcile failure *metric*
  moves; the alert-*firing* contract is recorded but deferred behind
  `E2E_QUALIFY_ALERTING` so the e2e lands with, not before, a `PrometheusRule`.
- **Sharing exercises the product path, not the test shortcut.** Area 16
  deliberately drives `POST /role_bindings` -> RoleBinding reconciler -> Keycloak
  client role, the bridge that area 9's `assign_gateway_client_role` driver
  shortcut forges directly. Once proven, area 9 may adopt this path.
