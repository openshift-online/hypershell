# Mergify Parallel Merge Queue Specification

**Date:** 2026-10-21
**Status:** Draft
**Related:** `e2e-testing.spec.md` (HYPERSHELL-18) - CI E2E workflow, Tests CI Gate,
             merge-queue gate, and Konflux image consumption;
             `ephemeral-pr-environments.spec.md` (HYPERSHELL-240) - automated
             OpenShift pull-request CI, environment lifecycle on merge-queue entries

**ID prefix:** CICD-MRG

## Purpose

HyperShell uses Mergify to manage the merge queue for `main`. The default serial
queue mode validates and merges pull requests one at a time, which means
independent changes (for example a documentation fix and an API server bug fix)
wait behind each other even when neither can affect the other. Parallel mode with
dependency-aware scopes lets the queue validate and merge pull requests with
disjoint scopes concurrently while preserving ordering for pull requests whose
scopes overlap. This reduces merge latency for independent work without
sacrificing correctness for related changes.

The scope model classifies every tracked file into one or more consumer lanes.
A change to a producer file (for example an OpenAPI contract) selects its own
scope and every consumer scope that depends on that contract, so the queue
serializes those pull requests. A change that touches only a single consumer's
implementation (for example an API server handler) selects only its own scope, so
it can merge concurrently with a CLI-only change. Barrier files (global build
infrastructure, workflow definitions, required-check machinery) serialize the
entire queue around the pull request that changes them, because they can affect
every scope.

E2E testing is valuable but its 10-20 minute runtime creates merge latency when
it gates the merge path. This spec removes E2E from the required merge gate
(`Tests CI Gate`) while keeping it available on ordinary pull requests and running
on `main` after merge. The required merge gate depends only on change detection
and unit tests. E2E is suppressed on Mergify temporary queue pull requests so
the queue validates quickly. Post-merge E2E on `main` has its own concurrency
policy so rapid merges do not cancel each other's validation runs.

## Requirements

### Requirement: CICD-MRG-01 -- Parallel Queue Mode

The Mergify merge queue SHALL operate in `parallel` mode. Pull requests with
disjoint scopes SHALL validate and merge independently. Pull requests with
overlapping scopes SHALL be validated in dependency order. The queue SHALL NOT
use `isolated` mode: ordering between scopes that can interact SHALL be
preserved.

#### Scenario: CICD-MRG-01.01 -- Disjoint scopes merge independently

- GIVEN pull request A touches only `components/api-server/` files
- AND pull request B touches only `docs/README.md`
- WHEN both are enqueued
- THEN the queue SHALL validate A and B concurrently in independent lanes
- AND the faster lane SHALL merge without waiting for the slower lane

#### Scenario: CICD-MRG-01.02 -- Overlapping scopes preserve ordering

- GIVEN pull request A modifies `components/api-server/openapi/openapi.yaml`
- AND pull request B modifies `components/cli/` files
- WHEN both are enqueued
- THEN A and B SHALL share the `cli` scope (because OpenAPI inputs are part of
  the CLI scope)
- AND the queue SHALL validate B against a speculative merge that includes A

#### Scenario: CICD-MRG-01.03 -- Barrier file serializes the queue

- GIVEN pull request A modifies `.mergify.yml`
- AND pull requests B and C are enqueued with disjoint scopes
- WHEN A is enqueued
- THEN the queue SHALL serialize B and C behind A until A merges
- AND after A merges, B and C SHALL resume independent validation if their scopes
  remain disjoint

### Requirement: CICD-MRG-02 -- Queue Rules

The merge queue SHALL preserve the existing approval workflow. The queue rule
SHALL require the `agent/review-code-approved` label in `queue_conditions`. The
queue SHALL target only the `main` branch and use squash merges. The commit
message format SHALL use the pull request title and inherited body with no
trailers.

The queue SHALL NOT configure multi-pull-request batching: `batch_size` and
`batch_max_wait_time` SHALL be omitted so batches stay at the default size of 1.
The Merge Queue Batch feature (batch sizes greater than 1) requires a higher
Mergify subscription tier than this organization has, so it is intentionally
unused; parallel scopes still provide independent lanes. The queue SHALL enable
branch-protection injection so required checks continue to come from GitHub's
rules.

#### Scenario: CICD-MRG-02.01 -- Approval label required for enqueue

- GIVEN a pull request without the `agent/review-code-approved` label
- WHEN it is proposed for the merge queue
- THEN the queue SHALL NOT accept it until the label is present

#### Scenario: CICD-MRG-02.02 -- Squash merge with PR title

- GIVEN a pull request is ready to merge from the queue
- WHEN the queue merges it
- THEN it SHALL squash the pull request's commits into one
- AND the commit title SHALL be the pull request title
- AND the commit body SHALL be the pull request body

### Requirement: CICD-MRG-03 -- Dependency-Aware Scope Definitions

The queue SHALL classify changes into file-pattern scopes that represent consumer
dependency lanes. Each scope SHALL include the consumer's own source files and
all upstream producer inputs that can affect it. A change to a producer input
SHALL select its own scope and every consumer scope whose include patterns match
that file.

The following consumer scopes SHALL be defined:

1. **api-server** - API server implementation, build pipelines, and runtime
   deployment inputs.
2. **cli** - CLI implementation, build pipelines, API OpenAPI and generation
   toolchain, and deployment inputs.
3. **control-plane** - Control plane implementation, build pipelines, the
   API-server module (including helpers beyond protobuf), the CLI module and its
   generation inputs, and deployment inputs.
4. **sdk-typescript** - TypeScript SDK generated and runtime sources, API
   contracts, SDK generator and IR tooling, and frontend workspace toolchain.
5. **sdk-go** - Go SDK sources, API contracts, and SDK generator and IR tooling.
6. **web-console** - Console, BFF, domain probes, shared gateway and operational
   UI packages, TypeScript SDK and contract/generator inputs, mockup Storybook
   sources, frontend toolchain and brand assets, and deployment inputs.
7. **fleet-dashboard** - Go host, fleet UI package, frontend toolchain and brand
   assets, the fleet firewall check, and deployment inputs. The TypeScript SDK
   SHALL NOT be included absent a verified dependency.
8. **agents** - Agent build inputs and pipelines, embedded CLI and transitive CLI
   generation inputs, and deployment inputs.
9. **deployment** - Charts, deployment overlays, dashboards, version pins, cluster
   helpers, and release-bundle inputs.
10. **test-infrastructure** - E2E and PR test harnesses, environment automation,
    and CI scripts. This scope SHALL be independent of application lanes when only
    the optional harness changes.
11. **architecture-site** - Site tooling, published architecture sources and
    assets, the architecture site workflow, and frontend toolchain.
12. **documentation** - Prose Markdown anywhere in the tree (the catch-all so no
    prose file falls through the coverage audit), specification prose, and skill
    content. Compiled mockups and architecture-site inputs SHALL be excluded. A
    runtime Markdown file additionally keeps its component scope, so this lane
    never hides a dependency.
13. **repository-tooling** - Repository administration, policy and tooling inputs,
    scripts, executable documentation helpers, and dependency management. Markdown
    files SHALL be excluded from this scope. Runtime helpers that affect actual
    consumers SHALL also select those consumer scopes through their path patterns.

Each scope SHALL exclude known informational files (`README.md`, `AGENTS.md`,
`CLAUDE.md`) from runtime directory patterns. Markdown SHALL NOT be
blanket-excluded from all patterns because it may be embedded runtime content,
prompts, or generator input.

#### Scenario: CICD-MRG-03.01 -- API-only change is independent of frontend

- GIVEN a pull request changes only `components/api-server/internal/handlers/`
- WHEN it is enqueued
- THEN it SHALL select the `api-server` scope only (plus `control-plane` and
  other consumers whose include patterns match `components/api-server/**`)
- AND it SHALL NOT select `web-console` or `fleet-dashboard` scopes
- AND it SHALL merge independently of a concurrent frontend-only pull request

#### Scenario: CICD-MRG-03.02 -- OpenAPI change propagates to consumers

- GIVEN a pull request changes `components/api-server/openapi/openapi.yaml`
- WHEN it is enqueued
- THEN it SHALL select the `api-server`, `cli`, `control-plane`,
  `sdk-typescript`, `sdk-go`, `web-console`, and `agents` scopes
- AND it SHALL NOT select `fleet-dashboard` (absent a verified SDK dependency)

#### Scenario: CICD-MRG-03.03 -- Frontend lockfile does not block Go

- GIVEN a pull request changes only `pnpm-lock.yaml`
- WHEN it is enqueued
- THEN it SHALL select frontend consumer scopes (`sdk-typescript`, `web-console`,
  `fleet-dashboard`, `architecture-site`) and NOT Go-only scopes
  (`api-server`, `cli`, `control-plane`, `sdk-go`)

#### Scenario: CICD-MRG-03.04 -- Fleet dashboard excludes TypeScript SDK

- GIVEN a pull request changes only `components/sdk-typescript/` files
- WHEN it is enqueued
- THEN it SHALL select the `sdk-typescript` and `web-console` scopes
- AND it SHALL NOT select the `fleet-dashboard` scope

#### Scenario: CICD-MRG-03.05 -- Control plane includes API and CLI modules

- GIVEN a pull request changes `components/api-server/pkg/api/grpc/` files
- WHEN it is enqueued
- THEN it SHALL select the `control-plane` scope (because the control plane
  module replaces the API-server module and imports those packages)

#### Scenario: CICD-MRG-03.06 -- Agent images include CLI inputs

- GIVEN a pull request changes `components/cli/` files
- WHEN it is enqueued
- THEN it SHALL select the `cli`, `control-plane`, and `agents` scopes
  (because agent Containerfiles embed `hsctl`)

### Requirement: CICD-MRG-04 -- Barrier Files

The queue SHALL define barrier files that serialize the entire queue around pull
requests that change global build and check infrastructure. Barrier files SHALL
include:

- `.mergify.yml` (queue configuration itself)
- `Makefile` (root build orchestration)
- `.dockerignore` (build context for all images)
- `.github/component-paths.json` (change detection input)
- `.github/scripts/**` (detection and CI helper scripts)
- `.github/actions/detect-konflux-components/**` (Konflux detection)
- `.github/workflows/checks.yml` (required Checks CI Gate)
- `.github/workflows/tests.yml` (required Tests CI Gate orchestrator)
- `.github/workflows/unit-tests.yml` (unit test stage)
- `.github/workflows/e2e.yml` (E2E stage)
- Repository policy check scripts (`scripts/check_ci_components.py`,
  `scripts/check_dependency_pins.py`, `scripts/check_dependency_age.py`,
  `scripts/check_forbidden_terms.py`, `scripts/check_spec_ids.py`, and their
  corresponding test scripts)
- `scripts/run-shell-unit-tests.sh` (shell test discovery)

Barrier files SHALL NOT include every script, all documentation, or every
component build file. Only changes that affect all validation SHALL be barriers.

#### Scenario: CICD-MRG-04.01 -- Workflow change blocks all lanes

- GIVEN pull request A modifies `.github/workflows/tests.yml`
- AND pull requests B and C with disjoint scopes are in the queue
- WHEN A is enqueued
- THEN B and C SHALL wait for A to merge before resuming validation

#### Scenario: CICD-MRG-04.02 -- Component Makefile is not a barrier

- GIVEN a pull request modifies only `components/api-server/Makefile`
- WHEN it is enqueued
- THEN it SHALL be classified by scope patterns, not as a barrier
- AND it SHALL NOT serialize the entire queue

### Requirement: CICD-MRG-05 -- Tests CI Gate Excludes E2E

The `Tests CI Gate` job in `.github/workflows/tests.yml` SHALL depend only on
`detect-changes` and `unit`. It SHALL NOT depend on the `e2e` stage. The gate
SHALL remove the `E2E_RESULT` environment variable and its failure-handling
logic. The gate SHALL fail only when `detect-changes` did not succeed or the
`unit` stage failed or was cancelled. A skipped unit stage SHALL pass the gate.

The gate's required check name SHALL remain `Tests CI Gate`. The `Checks CI
Gate` in `checks.yml` SHALL remain unchanged. Both gates SHALL continue to be
required in branch protection.

E2E SHALL continue to run as a stage of `tests.yml` on pull requests and on push
to `main`. Its result SHALL be visible as `E2E / <job>` checks on the pull
request. A failed E2E SHALL NOT block the merge queue; it SHALL remain visible as
a pending or failed check for developer awareness.

#### Scenario: CICD-MRG-05.01 -- Unit passes, E2E fails, gate passes

- GIVEN a pull request's unit tests pass
- AND the E2E stage fails
- WHEN the `Tests CI Gate` job evaluates
- THEN the gate SHALL pass because it depends only on detection and unit
- AND the pull request SHALL be eligible for the merge queue

#### Scenario: CICD-MRG-05.02 -- Unit fails, gate fails regardless of E2E

- GIVEN a pull request's unit tests fail
- AND the E2E stage passes
- WHEN the `Tests CI Gate` job evaluates
- THEN the gate SHALL fail because the unit stage failed

#### Scenario: CICD-MRG-05.03 -- E2E skipped, gate passes

- GIVEN a pull request changes only documentation files
- AND the unit stage is skipped
- AND the E2E stage is skipped
- WHEN the `Tests CI Gate` job evaluates
- THEN the gate SHALL pass because skipped stages do not fail the gate

### Requirement: CICD-MRG-06 -- E2E Suppression on Queue Pull Requests

The E2E workflow SHALL suppress the expensive e2e reusable-workflow call on both
`merge_group` events and Mergify-generated temporary queue pull requests. Mergify
temporary queue pull requests SHALL be detected by the configured queue branch
prefix on the head branch of same-repository pull requests. Detection SHALL match
Mergify's documented default `queue_branch_prefix` (`mergify/merge-queue/`) and
its legacy `tmp-mergify/merge-queue/` variant, so a prefix change across Mergify
versions does not silently re-enable E2E on queue pull requests. The suppression
SHALL NOT skip all drafts or all bot pull requests. The `merge_group` guard is a
defensive no-op because this repository uses Mergify's draft-PR queue rather than
GitHub's native merge queue, so `merge_group` events do not fire.

The suppression SHALL apply to both Kind and OpenShift deployment and suite work.
Cheap required validation (detection, unit tests) SHALL continue to run on
temporary queue pull requests.

#### Scenario: CICD-MRG-06.01 -- Mergify queue PR skips E2E

- GIVEN a Mergify temporary queue pull request with head branch
  `mergify/merge-queue/main/pr-42`
- WHEN the `tests.yml` workflow triggers
- THEN the `e2e` stage SHALL be skipped or its jobs SHALL be suppressed
- AND the `unit` stage SHALL still run
- AND the `Tests CI Gate` SHALL still evaluate and pass when unit passes

#### Scenario: CICD-MRG-06.02 -- Non-queue PR runs E2E normally

- GIVEN a normal pull request with head branch `feature/my-change`
- WHEN the `tests.yml` workflow triggers
- THEN the `e2e` stage SHALL run normally according to change detection
- AND E2E results SHALL be visible as checks on the pull request

#### Scenario: CICD-MRG-06.03 -- Draft PRs still run E2E

- GIVEN a draft pull request with head branch `feature/wip`
- WHEN the `tests.yml` workflow triggers
- THEN the `e2e` stage SHALL run normally (draft status alone does not suppress
  E2E)

#### Scenario: CICD-MRG-06.04 -- Legacy-prefix queue PR skips E2E

- GIVEN a Mergify temporary queue pull request with head branch
  `tmp-mergify/merge-queue/main/pr-42`
- WHEN the `tests.yml` workflow triggers
- THEN the `e2e` stage SHALL be skipped or its jobs SHALL be suppressed
- AND the `unit` stage SHALL still run

### Requirement: CICD-MRG-07 -- Post-Merge E2E Concurrency

Post-merge E2E on push to `main` SHALL have an independent concurrency policy
that prevents rapid merges from cancelling each other's E2E runs. Each push to
`main` SHALL be able to finish its E2E run using a SHA-specific concurrency key.
The concurrency policy SHALL NOT cancel a nearly-complete `main` E2E validation
on every merge.

Per-pull-request cancellation for superseded PR revisions SHALL be preserved.
Main E2E SHALL run only after needed images are available and SHALL report
failures visibly.

#### Scenario: CICD-MRG-07.01 -- Rapid merges do not cancel main E2E

- GIVEN merge A lands on `main` and starts an E2E run
- WHEN merge B lands on `main` 2 minutes later
- THEN merge A's E2E run SHALL continue to completion
- AND merge B's E2E run SHALL start independently
- AND both runs SHALL use SHA-specific concurrency keys

#### Scenario: CICD-MRG-07.02 -- PR revision cancels previous PR E2E

- GIVEN pull request #42 push A starts an E2E run
- WHEN pull request #42 push B arrives
- THEN push A's E2E run SHALL be cancelled by the PR-scoped concurrency policy
- AND push B's E2E run SHALL start

### Requirement: CICD-MRG-08 -- Unknown Path Coverage Audit

New or unmapped tracked paths SHALL NOT silently pass as documentation-only or as
having no scope. The implementation SHALL include an audit that identifies
unclassified tracked paths and requires an explicit owner decision for new
production directories.

A pull request that introduces files in an unclassified path SHALL be flagged by
the audit. A universal `**` repository scope SHALL NOT be added as a fallback,
because it either couples every pull request or risks treating unknown runtime
files as independent maintenance.

#### Scenario: CICD-MRG-08.01 -- New component directory is flagged

- GIVEN a pull request adds files under a new `components/new-service/` directory
- AND `components/new-service/` is not classified by any scope or barrier pattern
- WHEN the coverage audit runs
- THEN it SHALL flag the new path as unclassified
- AND the pull request SHALL require explicit scope classification before merging

#### Scenario: CICD-MRG-08.02 -- Existing classified path passes audit

- GIVEN a pull request modifies only files under `components/api-server/`
- AND `components/api-server/` is classified by the `api-server` scope
- WHEN the coverage audit runs
- THEN it SHALL pass without flagging

### Requirement: CICD-MRG-09 -- Component Detector Alignment

The Mergify scope definitions and the CI change detector
(`.github/component-paths.json`) SHALL recognize the same dependency
relationships for each component. When a file change selects a Mergify scope, the
CI detector SHALL run the corresponding component's tests. Intentional
differences SHALL be documented (for example, optional E2E harness changes do not
force application merge ordering but do run the E2E tests).

The implementation SHALL audit `.github/component-paths.json` and job wiring for
the same dependency relationships as the Mergify scopes, particularly:

- Control-plane imports of API helper packages and CLI code
- Shared-package changes running the tests that consume them
- Agent images embedding the CLI

#### Scenario: CICD-MRG-09.01 -- CLI change runs control-plane tests

- GIVEN a pull request changes `components/cli/pkg/kustomize/` files
- WHEN the CI detector evaluates the change
- THEN it SHALL flag the `control_plane` component as changed
- AND the control-plane unit tests SHALL run

#### Scenario: CICD-MRG-09.02 -- Scope and detector agree on OpenAPI consumers

- GIVEN a pull request changes `components/api-server/openapi/openapi.yaml`
- WHEN the CI detector evaluates the change
- THEN it SHALL flag `cli`, `sdk_typescript`, `sdk_go`, and `web_console` as
  changed
- AND the Mergify scope SHALL select the corresponding consumer scopes

### Requirement: CICD-MRG-10 -- Konflux Pipeline Audit on Queue PRs

Konflux triggers on Mergify temporary queue pull requests SHALL be audited.
Existing dedicated merge-queue pipelines match push branches under
`gh-readonly-queue/main/`, whereas Mergify queue pull requests exercise
pull-request pipelines. Required checks SHALL report for the SHA that Mergify
evaluates. Optional image and E2E builds SHALL NOT exhaust runners or cluster
capacity and indirectly delay cheap CI.

GitHub merge-group assumptions SHALL NOT be copied onto Mergify queue branches
without verification.

#### Scenario: CICD-MRG-10.01 -- Required checks report for queue PR SHA

- GIVEN a Mergify temporary queue pull request
- WHEN cheap required validation (detection, unit tests, policy checks) runs
- THEN the checks SHALL report for the SHA that Mergify evaluates
- AND the checks SHALL pass or fail independently of optional E2E

### Requirement: CICD-MRG-11 -- Deployment Scope Propagation

Deployment and release input changes (charts, deployment overlays, dashboards,
version pins, cluster helpers, release-bundle inputs) SHALL select the
`deployment` scope and all runtime consumer scopes in this initial conservative
design. Ordinary API, CLI, or console implementation changes SHALL remain
independent except where actual upstream dependencies overlap.

The deployment scope propagation MAY be narrowed in a later change with evidence
from build and runtime ownership that demonstrates fewer consumers are affected.

#### Scenario: CICD-MRG-11.01 -- Chart change selects all runtime consumers

- GIVEN a pull request changes `charts/hypershell/values.yaml`
- WHEN it is enqueued
- THEN it SHALL select the `deployment` scope and all runtime consumer scopes
  (`api-server`, `cli`, `control-plane`, `web-console`, `fleet-dashboard`,
  `agents`)

#### Scenario: CICD-MRG-11.02 -- API implementation is independent of charts

- GIVEN a pull request changes only `components/api-server/internal/`
- WHEN it is enqueued
- THEN it SHALL NOT select the `deployment` scope
- AND it SHALL merge independently of a deployment-only pull request

### Requirement: CICD-MRG-12 -- No Automatic Enqueue Policy

This change SHALL NOT introduce a new automatic-enqueue policy. The existing
manual, dashboard, or command enqueue flow SHALL be preserved. Adding a label to
`queue_conditions` alone does not establish autoqueue behavior.

#### Scenario: CICD-MRG-12.01 -- Label alone does not enqueue

- GIVEN a pull request has the `agent/review-code-approved` label
- AND no explicit enqueue action has been taken
- WHEN the queue evaluates its conditions
- THEN the pull request SHALL NOT be automatically enqueued

## Retired IDs

None.
