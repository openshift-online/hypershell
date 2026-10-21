# Merge Queue Parallel Scopes Specification

**ID prefix:** `CICD-MRG`

## Purpose

HyperShell uses Mergify to manage the merge queue for `main`. This spec
defines the parallel merge queue mode, file-based dependency scopes, barrier
file classification, queue policy, and the CI integration changes that let
independent pull requests merge concurrently while preserving ordering for
overlapping scopes. It also specifies the separation of end-to-end testing
from the required merge gate, post-merge E2E concurrency, and coverage
auditing that prevents unmapped paths from bypassing queue dependency tracking.

This spec owns the desired state of `.mergify.yml` and the CI workflow
changes that realize merge queue throughput. The E2E workflow itself remains
specified in `e2e-testing.spec.md`; this spec owns only the gating and
suppression decisions that affect which CI jobs run on queue entries.

## Requirements

### Requirement: CICD-MRG-01 -- Parallel Queue Mode

The Mergify merge queue SHALL operate in `parallel` mode with
`max_parallel_checks: 10`. Parallel mode lets pull requests with disjoint
file-based scopes validate and merge independently, while pull requests with
overlapping scopes retain dependency ordering. The system SHALL NOT use
`isolated` mode: overlapping scopes must be tested against each other.

#### Scenario: CICD-MRG-01.01 -- Disjoint scopes merge independently

- GIVEN two pull requests in the merge queue
- AND pull request A touches only files in the `api-server` scope
- AND pull request B touches only files in the `fleet-dashboard` scope
- WHEN Mergify evaluates their scopes
- THEN both pull requests SHALL validate concurrently
- AND the faster lane SHALL merge without waiting for the slower lane

#### Scenario: CICD-MRG-01.02 -- Overlapping scopes preserve ordering

- GIVEN two pull requests in the merge queue
- AND both touch files in the `control-plane` scope (which includes
  `components/api-server/**` as an upstream input)
- WHEN Mergify evaluates their scopes
- THEN Mergify SHALL validate them in queue order
- AND the later pull request SHALL be tested against the speculative merge
  that includes the earlier one

#### Scenario: CICD-MRG-01.03 -- Mode is enabled atomically

- GIVEN the delivered `.mergify.yml` configuration
- WHEN it is merged to `main`
- THEN `merge_queue.mode` SHALL be `parallel`
- AND scope definitions, barrier files, and queue rules SHALL all be present
  in the same commit
- AND no intermediate serial-mode state SHALL be required

### Requirement: CICD-MRG-02 -- Queue Rule Policy

The merge queue SHALL define one queue rule named `agent/review-code-approved`
with the following policy:

- `queue_conditions` SHALL require `base = main` and
  `label = agent/review-code-approved`.
- `merge_method` SHALL be `squash`.
- `batch_size` SHALL be `min: 1`, `max: 3`.
- `batch_max_wait_time` SHALL be `30 seconds`.
- `branch_protection_injection_mode` SHALL be `queue`.
- `commit_message_format` SHALL use `title: pr-title`, `body: inherit`,
  `trailers: []`.

These are initial tuning values. Parallel-mode batching requires matching
scope sets; overlapping sets establish ordering rather than sharing a batch.
The system SHALL NOT introduce a new automatic-enqueue policy in this change.

#### Scenario: CICD-MRG-02.01 -- Queue conditions match existing flow

- GIVEN a pull request labeled `agent/review-code-approved` targeting `main`
- WHEN the pull request is enqueued
- THEN Mergify SHALL accept it into the merge queue
- AND the merge method SHALL be squash
- AND the commit message SHALL use the pull request title

#### Scenario: CICD-MRG-02.02 -- Branch protection injection preserved

- GIVEN `branch_protection_injection_mode: queue`
- WHEN a queue entry is created
- THEN required checks SHALL continue to come from GitHub's branch protection
  rules
- AND the system SHALL NOT disable injection to remove E2E from the required
  path

### Requirement: CICD-MRG-03 -- File-Based Scope Definitions

The Mergify configuration SHALL define file-based scopes under
`scopes.source.files` for the following twelve consumer or lane categories.
Each scope SHALL include patterns for the consumer's own source, its upstream
inputs (contract and generator files, shared modules), and its deployment and
release inputs. Each scope SHALL exclude informational files (`**/README.md`,
`**/AGENTS.md`, `**/CLAUDE.md`) from runtime directory patterns.

The twelve scopes are:

| Scope name | Primary consumer | Key upstream inputs |
|---|---|---|
| `api-server` | API server source and build | Deployment, release, runtime cluster inputs |
| `cli` | CLI source and build | API OpenAPI contracts, generation toolchain, deployment and release inputs |
| `control-plane` | Control plane source and build | API server module, CLI module and generation inputs, chart, deployment, and release inputs |
| `sdk-typescript` | TypeScript SDK | API contracts, SDK generator and IR tooling, frontend workspace and toolchain |
| `sdk-go` | Go SDK | API contracts, SDK generator and IR tooling |
| `web-console` | Web console, BFF, domain probes, shared UI packages | TypeScript SDK and contract/generator inputs, mockup Storybook sources, frontend toolchain, brand assets, deployment and release inputs |
| `fleet-dashboard` | Fleet dashboard Go host and fleet UI package | Frontend toolchain and brand assets, fleet firewall check, deployment and release inputs; no SDK relationship |
| `agents` | Agent images and build | CLI source and transitive CLI generation inputs, deployment and release inputs |
| `deployment` | Charts, overlays, dashboards, version pins, cluster helpers, release bundle | Own deployment and release inputs |
| `test-infrastructure` | E2E and PR test harnesses, environment automation | Independent of application lanes when only the harness changes |
| `architecture-site` | Architecture site tooling, published sources and assets | Frontend toolchain, its own workflow |
| `documentation` | Root prose, component README/instruction files, specification prose, skill content | Excludes compiled mockups and architecture-site inputs |
| `repository-tooling` | Repository administration, policy and tooling inputs, scripts | Runtime helpers also select their actual consumers |

A scope name itself does not establish a dependency or automatically select
CI jobs. An ordinary consumer implementation edit selects its own scope; a
producer or input edit selects its own scope and all affected consumer scopes.
An ordinary consumer edit does not select all its siblings. The system SHALL
NOT define a universal `application` or `shared` scope: that would erase the
intended parallelism.

#### Scenario: CICD-MRG-03.01 -- API server change selects API server scope

- GIVEN a pull request that modifies only files under `components/api-server/`
- AND those files are not OpenAPI contracts or generation inputs
- WHEN Mergify evaluates the pull request's scopes
- THEN only the `api-server` scope SHALL be selected (plus any deployment
  overlap from shared deployment patterns)
- AND the `cli`, `web-console`, and `sdk-typescript` scopes SHALL NOT be
  selected unless the change touches their explicit upstream input patterns

#### Scenario: CICD-MRG-03.02 -- OpenAPI contract change propagates to consumers

- GIVEN a pull request that modifies files under
  `components/api-server/openapi/`
- WHEN Mergify evaluates the pull request's scopes
- THEN the `api-server`, `cli`, `control-plane`, `sdk-typescript`, `sdk-go`,
  `web-console`, and `agents` scopes SHALL all be selected
- AND the `fleet-dashboard` scope SHALL NOT be selected (no verified SDK
  dependency)

#### Scenario: CICD-MRG-03.03 -- Documentation change is independent

- GIVEN a pull request that modifies only `README.md` at the repository root
- WHEN Mergify evaluates the pull request's scopes
- THEN only the `documentation` scope SHALL be selected
- AND no application or deployment scope SHALL be selected

#### Scenario: CICD-MRG-03.04 -- Frontend lockfile selects frontend consumers

- GIVEN a pull request that modifies `pnpm-lock.yaml` or `.node-version`
- WHEN Mergify evaluates the pull request's scopes
- THEN the `sdk-typescript`, `web-console`, `fleet-dashboard`, and
  `architecture-site` scopes SHALL be selected
- AND no Go-only scope (`api-server`, `control-plane`, `sdk-go`) SHALL be
  selected unless the pull request also touches their patterns

#### Scenario: CICD-MRG-03.05 -- Control plane includes API server and CLI modules

- GIVEN the `control-plane` scope definition
- WHEN inspected
- THEN it SHALL include `components/api-server/**` and `components/cli/**`
  as upstream inputs
- AND it SHALL include the CLI generation toolchain
  (`scripts/generate-cli.sh`, `scripts/generate-tui.sh`,
  `scripts/trex-checkout.sh`, `scripts/rh-trex-ai.ref`, `scripts/lib/**`)
- AND it SHALL include API OpenAPI contracts
  (`components/api-server/openapi/**`)

#### Scenario: CICD-MRG-03.06 -- Agent images include embedded CLI inputs

- GIVEN the `agents` scope definition
- WHEN inspected
- THEN it SHALL include `components/cli/**` and CLI generation inputs
- AND it SHALL include API OpenAPI contracts
- AND a change to CLI source SHALL select both the `cli` and `agents` scopes

#### Scenario: CICD-MRG-03.07 -- Deployment change selects all runtime consumers

- GIVEN a pull request that modifies files under `deploy/` or `charts/`
- WHEN Mergify evaluates the pull request's scopes
- THEN the `deployment` scope and all runtime consumer scopes (`api-server`,
  `cli`, `control-plane`, `web-console`, `fleet-dashboard`, `agents`) SHALL
  be selected
- AND the `test-infrastructure`, `architecture-site`, `documentation`, and
  `repository-tooling` scopes SHALL NOT be selected unless their own patterns
  also match

#### Scenario: CICD-MRG-03.08 -- Mockup files select web-console, not documentation

- GIVEN a pull request that modifies files under `specs/web-console/mockups/`
- WHEN Mergify evaluates the pull request's scopes
- THEN the `web-console` scope SHALL be selected
- AND the `documentation` scope SHALL NOT be selected (mockups are excluded
  from its patterns)

#### Scenario: CICD-MRG-03.09 -- TypeScript SDK excluded from fleet dashboard

- GIVEN the `fleet-dashboard` scope definition
- WHEN inspected
- THEN it SHALL NOT include `components/sdk-typescript/**`
- AND a change solely to TypeScript SDK source SHALL NOT select the
  `fleet-dashboard` scope

### Requirement: CICD-MRG-04 -- Barrier Files

The Mergify configuration SHALL define `scopes.barrier_files` for changes
that affect every scope and serialize the parallel queue around them. Barrier
files include:

- `.mergify.yml`
- `Makefile`
- `.dockerignore`
- `.github/component-paths.json`
- `.github/scripts/**`
- `.github/actions/detect-konflux-components/**`
- `.github/workflows/checks.yml`
- `.github/workflows/tests.yml`
- `.github/workflows/unit-tests.yml`
- `.github/workflows/e2e.yml`
- Repository policy and CI check scripts (`scripts/check_ci_components.py`,
  `scripts/check_dependency_pins.py`, `scripts/check_dependency_age.py`,
  `scripts/check_forbidden_terms.py`, `scripts/check_spec_ids.py` and their
  corresponding test scripts)
- `scripts/run-shell-unit-tests.sh`

Barrier candidates SHALL be reviewed carefully: barriers block independent
work as well. The system SHALL NOT make every script, all documentation, or
every component build file a global barrier. Frontend-only lock/toolchain
changes instead select their frontend consumer scopes without serializing
unrelated Go-only changes. Component-specific Tekton files select their
component and downstream consumers, not all scopes.

#### Scenario: CICD-MRG-04.01 -- Barrier serializes the queue

- GIVEN a pull request that modifies `.mergify.yml`
- WHEN Mergify evaluates its scopes
- THEN the pull request SHALL be treated as a barrier
- AND all other queue entries SHALL be ordered around it

#### Scenario: CICD-MRG-04.02 -- Component Tekton file is not a barrier

- GIVEN a pull request that modifies only
  `.tekton/hypershell-api-server-main-pull-request.yaml`
- WHEN Mergify evaluates its scopes
- THEN it SHALL select the `api-server` scope and downstream consumers
- AND it SHALL NOT be treated as a barrier

### Requirement: CICD-MRG-05 -- Tests CI Gate Covers Detection and Unit Only

The `tests-gate` job (`Tests CI Gate`) in `.github/workflows/tests.yml`
SHALL roll up only the `detect-changes` job and the `unit` stage. It SHALL
NOT include the `e2e` stage in its `needs` list and SHALL NOT evaluate the
E2E stage result when determining pass or fail. The gate SHALL fail when
`detect-changes` did not succeed or the `unit` stage's result is `failure`
or `cancelled`. A fully skipped unit stage SHALL pass the gate.

E2E test results SHALL remain visible as separate checks on pull requests
and pushes to `main` but SHALL NOT block the merge queue. This separates
the 10-20 minute E2E runtime from merge latency while keeping E2E available
for pull request validation.

#### Scenario: CICD-MRG-05.01 -- Gate passes with unit success, E2E failure

- GIVEN a pull request whose unit tests pass
- AND whose E2E tests fail
- WHEN the `Tests CI Gate` check evaluates
- THEN the gate SHALL report `success`
- AND the E2E failure SHALL remain visible as a separate failed check

#### Scenario: CICD-MRG-05.02 -- Gate fails on unit failure

- GIVEN a pull request whose unit tests fail
- AND whose E2E tests pass
- WHEN the `Tests CI Gate` check evaluates
- THEN the gate SHALL report `failure`

#### Scenario: CICD-MRG-05.03 -- Gate passes when both stages are skipped

- GIVEN a documentation-only pull request
- AND both the `unit` and `e2e` stages are skipped by path filtering
- WHEN the `Tests CI Gate` check evaluates
- THEN the gate SHALL report `success`

### Requirement: CICD-MRG-06 -- E2E Suppression on Queue Entries

The E2E reusable-workflow call in `tests.yml` SHALL be suppressed on both
merge queue entry types:

1. **GitHub native merge queue** -- `merge_group` events.
2. **Mergify temporary queue PRs** -- pull requests whose head branch
   matches the Mergify queue branch prefix (`mergify/merge-queue/`).

Detection of Mergify temporary queue PRs SHALL use the head branch prefix,
not a blanket draft-PR or bot-PR filter. The system SHALL NOT skip all draft
pull requests or all bot-authored pull requests.

Cheap required validation (detection, unit tests, policy checks) SHALL
continue to run on temporary queue PRs so that `Tests CI Gate` and
`Checks CI Gate` report status for the SHA Mergify evaluates.

#### Scenario: CICD-MRG-06.01 -- E2E skipped on merge_group

- GIVEN a `merge_group` event triggers `tests.yml`
- WHEN the orchestrator evaluates stages
- THEN the `e2e` stage SHALL be skipped
- AND the `unit` stage and `detect-changes` SHALL still run
- AND the `Tests CI Gate` SHALL report based on unit results only

#### Scenario: CICD-MRG-06.02 -- E2E skipped on Mergify temporary PR

- GIVEN a pull request whose head branch starts with `mergify/merge-queue/`
- WHEN `tests.yml` triggers on the `pull_request` event
- THEN the `e2e` stage SHALL be skipped
- AND the `unit` stage SHALL still run
- AND `Tests CI Gate` SHALL report based on unit results only

#### Scenario: CICD-MRG-06.03 -- E2E still runs on ordinary PRs

- GIVEN an ordinary pull request (head branch does not start with
  `mergify/merge-queue/`)
- WHEN `tests.yml` triggers on the `pull_request` event
- THEN the `e2e` stage SHALL run according to change detection and
  `should_run` as `e2e-testing.spec.md` specifies

#### Scenario: CICD-MRG-06.04 -- E2E still runs on push to main

- GIVEN a push to `main`
- WHEN `tests.yml` triggers on the `push` event
- THEN the `e2e` stage SHALL run as `e2e-testing.spec.md` specifies
- AND its concurrency SHALL follow the post-merge policy
  (CICD-MRG-07)

### Requirement: CICD-MRG-07 -- Post-Merge E2E Concurrency

Post-merge E2E runs on push to `main` SHALL use a per-commit concurrency
key (for example `e2e-main-<sha>`) so that each merge's E2E run can complete
independently. The system SHALL NOT cancel a nearly-complete main E2E
validation when a subsequent merge lands. The cheap required CI (unit tests,
checks) MAY use a concurrency policy that cancels older runs on newer pushes,
but the E2E concurrency group SHALL be separate from that policy.

If runner capacity makes per-commit E2E impractical, the project SHALL make
an explicit decision for periodic or latest-completed coverage. GitHub
concurrency pending-run replacement SHALL NOT silently become the policy.

#### Scenario: CICD-MRG-07.01 -- Sequential merges do not cancel E2E

- GIVEN merge A's E2E run is in progress on `main`
- WHEN merge B lands and triggers a new push to `main`
- THEN merge A's E2E run SHALL continue to completion
- AND merge B's E2E run SHALL start independently

#### Scenario: CICD-MRG-07.02 -- Per-PR cancellation preserved

- GIVEN a pull request with an E2E run in progress
- WHEN a new commit is pushed to the same pull request
- THEN the older E2E run SHALL be cancelled
- AND the new commit's E2E run SHALL start

### Requirement: CICD-MRG-08 -- Tracked File Coverage Audit

The system SHALL provide a coverage audit that lists all tracked files not
classified by any scope or barrier definition. New application files SHALL
NOT pass as documentation-only. Unclassified files in a mixed-path pull
request SHALL cause the audit to fail, requiring an explicit owner decision
before merging. The audit SHALL NOT add a universal `**` repository scope:
that would either couple every pull request or risk treating unknown runtime
files as independent maintenance.

#### Scenario: CICD-MRG-08.01 -- New unmapped component fails audit

- GIVEN a pull request that adds a new directory `components/new-service/`
- AND no scope definition includes `components/new-service/**`
- WHEN the coverage audit runs
- THEN it SHALL report the unclassified paths
- AND the pull request SHALL NOT merge until the paths are explicitly
  classified in a scope

#### Scenario: CICD-MRG-08.02 -- Mixed PR cannot hide component behind docs

- GIVEN a pull request that modifies both `README.md` and
  `components/new-service/main.go`
- AND `components/new-service/**` is not classified in any scope
- WHEN Mergify evaluates scopes
- THEN the `documentation` scope SHALL be selected for `README.md`
- AND the unclassified `components/new-service/main.go` SHALL cause the
  coverage audit to fail
- AND the pull request SHALL NOT be treated as documentation-only

#### Scenario: CICD-MRG-08.03 -- All existing tracked files are classified

- GIVEN the delivered scope and barrier definitions
- WHEN the coverage audit runs against the current `main` branch
- THEN every tracked file SHALL be classified by at least one scope or
  barrier definition
- AND any intentional exclusion SHALL be documented

### Requirement: CICD-MRG-09 -- Component Path Detector Alignment

The `.github/component-paths.json` detector and Mergify scopes express
overlapping but distinct dependency relationships. Mergify scopes decide
queue independence; detector paths decide what CI jobs are actually tested.
The two SHALL be aligned where the same dependency relationship applies.
The control plane detector SHALL include API helper packages and CLI code
that `components/control-plane/go.mod` replaces with local modules. Shared
UI package changes SHALL trigger the tests that consume them.

Intentional differences between scope and detector coverage SHALL be
documented. For example, optional E2E harness changes do not force
application merge ordering, and the test-infrastructure scope is independent
of application scopes in the queue even though a harness change may run
application tests in CI.

#### Scenario: CICD-MRG-09.01 -- Control plane detector includes API helpers

- GIVEN the `component-paths.json` definition for `control_plane`
- WHEN inspected
- THEN it SHALL include paths for API server protobuf, `gatewayhealth`,
  `secretstore`, and CLI `pkg/kustomize` code
- AND a change to those paths SHALL trigger control plane unit tests

#### Scenario: CICD-MRG-09.02 -- Shared UI package triggers consumer tests

- GIVEN the `component-paths.json` definition for `web_console`
- WHEN inspected
- THEN it SHALL include `packages/gateway-management-ui/**` and
  `packages/operational-dashboard-ui/**`
- AND a change to those packages SHALL trigger web console tests

### Requirement: CICD-MRG-10 -- Konflux Pipeline Behavior on Queue PRs

Konflux triggers on Mergify temporary queue PRs SHALL be audited. Existing
dedicated merge-queue pipelines (`.tekton/hypershell-<component>-main-merge-queue.yaml`)
match push branches under `gh-readonly-queue/main/`, whereas Mergify queue
PRs normally exercise pull-request pipelines. Required checks SHALL report
for the SHA Mergify evaluates. Optional image and E2E builds SHALL NOT
exhaust runners or cluster capacity and indirectly delay cheap CI. The
system SHALL NOT copy GitHub merge-group assumptions onto Mergify queue
branches.

#### Scenario: CICD-MRG-10.01 -- Queue PR does not start expensive builds

- GIVEN a Mergify temporary queue pull request
- WHEN Konflux evaluates its pipeline triggers
- THEN the E2E deployment SHALL NOT start
- AND optional image builds SHALL NOT delay the required `Tests CI Gate`
  or `Checks CI Gate` checks

### Requirement: CICD-MRG-11 -- Scope Validation

The Mergify configuration SHALL be validated before deployment. Validation
SHALL cover:

- Mergify configuration and schema validation.
- Rule simulation against representative change scenarios.
- Glob handling of direct child files, nested files, dotfiles, exclusions,
  component pipeline names, and renamed files at old and new paths.
- A table-driven scope and dependency check using the same glob semantics
  as Mergify (or service simulation), not a generic `fnmatch` substitute
  with assumed identical `**` behavior.

#### Scenario: CICD-MRG-11.01 -- Representative scenarios produce expected scopes

- GIVEN the representative change scenarios in the issue's acceptance criteria
- WHEN scope evaluation is simulated
- THEN each scenario SHALL produce the expected scopes and jobs
- AND the results SHALL be captured for review

#### Scenario: CICD-MRG-11.02 -- Glob semantics match Mergify

- GIVEN a scope pattern like `components/api-server/**`
- WHEN evaluated against direct child files and nested files
- THEN it SHALL match identically to Mergify's documented glob behavior
- AND dotfiles within the pattern SHALL be matched

## Retired IDs

None.
