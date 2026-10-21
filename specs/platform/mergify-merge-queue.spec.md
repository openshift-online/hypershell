# Mergify Merge Queue Specification

**Date:** 2026-10-21
**Status:** Draft
**Related:** `e2e-testing.spec.md` (HYPERSHELL-18) - CI workflow orchestration,
             Tests CI Gate, E2E suppression on queue validation PRs, and
             post-merge E2E concurrency;
             `ephemeral-pr-environments.spec.md` (HYPERSHELL-240) - ephemeral
             pull-request environments (not used by queue validation PRs);
             `openshell-branch-build.spec.md` - Konflux build pipelines;
             `agent-runtime-images.spec.md` - agent image pipelines

**ID prefix:** CICD-MRG

## Purpose

HyperShell uses Mergify to manage the merge queue for the `main` branch. The
merge queue SHALL operate in parallel mode with dependency-aware file-pattern
scopes so that pull requests touching disjoint areas of the repository validate
and merge independently, pull requests touching overlapping areas preserve
dependency ordering, and changes to genuinely global build or check machinery
serialize around a barrier. The configuration preserves the existing
`agent/review-code-approved` approval label, squash-merge commit policy, and
the two required GitHub branch-protection checks (`Tests CI Gate` and
`Checks CI Gate`) while removing E2E from the merge-critical path (see
`e2e-testing.spec.md` for the CI gate and E2E suppression changes that
accompany this configuration).

### Scope

This spec covers:

- The `.mergify.yml` queue rule, merge-queue mode, batch settings, and
  branch-protection injection
- File-pattern scope definitions and their dependency model
- Barrier file classification
- The relationship between Mergify scopes and CI change detection
- Coverage audit requirements for unclassified paths

This spec does not cover:

- CI workflow job selection or gating logic (`e2e-testing.spec.md`)
- Ephemeral pull-request environments (`ephemeral-pr-environments.spec.md`)
- GitHub branch-protection ruleset configuration (preserved as-is)
- Mergify subscription or account management

## Requirements

### Requirement: CICD-MRG-01 -- Parallel Queue Mode and Queue Rule

The Mergify merge queue SHALL operate in `parallel` mode. The queue rule SHALL
enqueue pull requests labeled `agent/review-code-approved` targeting the `main`
branch. The rule SHALL squash-merge with the pull request title as the commit
title and the pull request body as the commit body. The rule SHALL not add
trailers.

The queue SHALL allow up to 10 parallel speculative checks
(`max_parallel_checks: 10`). Dynamic batching SHALL group 1 to 3 pull requests
per batch with a maximum fill wait of 30 seconds. These are initial tuning
values, not commitments; they may be adjusted based on observed throughput and
resource contention.

Branch-protection injection SHALL remain enabled
(`branch_protection_injection_mode: queue`) so that required checks come from
GitHub's ruleset. The queue rule SHALL not introduce automatic-enqueue behavior;
pull requests enter the queue through the existing manual or dashboard flow.

#### Scenario: CICD-MRG-01.01 -- Queue rule preserves existing policy

- GIVEN a pull request targeting `main` with label `agent/review-code-approved`
- WHEN the pull request enters the Mergify queue
- THEN it SHALL be squash-merged with the PR title and body
- AND no trailers SHALL be added to the commit message
- AND branch-protection injection SHALL remain enabled

#### Scenario: CICD-MRG-01.02 -- Parallel mode enables independent lanes

- GIVEN two pull requests in the queue with disjoint scopes
- WHEN both pull requests are being validated
- THEN Mergify SHALL validate them in parallel without ordering constraints
- AND the faster lane SHALL merge without waiting for the slower lane

#### Scenario: CICD-MRG-01.03 -- Batching groups compatible pull requests

- GIVEN three pull requests in the queue with matching scope sets
- WHEN the batch fill wait expires or the batch reaches 3 pull requests
- THEN Mergify SHALL validate them as a single batch
- AND a failing pull request in the batch SHALL cause the batch to be split
  and retried

### Requirement: CICD-MRG-02 -- Dependency-Aware File-Pattern Scopes

The Mergify configuration SHALL define file-pattern scopes under
`scopes.source.files` representing the repository's consumer and lane
structure. A producer or input edit SHALL select its own scope and every
downstream consumer scope. An ordinary consumer implementation edit SHALL
select only its own scope, not all siblings.

The scopes SHALL be:

| Scope | Primary directory | Upstream inputs included |
|-------|------------------|------------------------|
| `api-server` | `components/api-server/` | Deployment, release, cluster scripts |
| `cli` | `components/cli/` | API OpenAPI and generation toolchain, deployment, release |
| `control-plane` | `components/control-plane/` | API server module, CLI module and generation inputs, deployment, release |
| `sdk-typescript` | `components/sdk-typescript/` | API contracts, SDK generator/IR tooling, frontend workspace/toolchain |
| `sdk-go` | `components/sdk-go/` | API contracts, SDK generator/IR tooling |
| `web-console` | `components/web-console/`, shared UI packages | TypeScript SDK, contract/generator inputs, mockups, frontend toolchain, brand assets, deployment, release |
| `fleet-dashboard` | `components/fleet-dashboard/`, `packages/fleet-dashboard-ui/` | Frontend toolchain, brand assets, firewall check, deployment, release (no SDK) |
| `agents` | `components/agents/` | CLI and CLI generation inputs, deployment, release |
| `deployment` | `deploy/`, `charts/`, `dashboards/` | Version pins, cluster helpers, release-bundle inputs |
| `test-infrastructure` | `tests/`, `components/pr-test/` | E2E harnesses, CI scripts, PR environment actions |
| `architecture-site` | `tools/architecture-site/`, `docs/architecture/` | Site workflow, frontend toolchain |
| `documentation` | Root prose, component READMEs, specs, skills | Excludes compiled mockups and architecture-site inputs |
| `repository-tooling` | `scripts/`, `.github/ISSUE_TEMPLATE/`, policy files | Excludes prose Markdown and architecture-site inputs |

Each scope SHALL exclude known informational files (`README.md`, `AGENTS.md`,
`CLAUDE.md`) from runtime directory patterns via exclude rules. Markdown SHALL
NOT be blanket-excluded from all scopes because it may be embedded runtime
content, prompts, or generator input.

#### Scenario: CICD-MRG-02.01 -- API contract change propagates to consumers

- GIVEN a pull request that changes files under `components/api-server/openapi/`
- WHEN Mergify evaluates the pull request's scopes
- THEN it SHALL match the `api-server`, `cli`, `control-plane`,
  `sdk-typescript`, `sdk-go`, `web-console`, and `agents` scopes
- AND it SHALL NOT match the `fleet-dashboard` scope (no verified SDK
  dependency)

#### Scenario: CICD-MRG-02.02 -- Isolated consumer change stays in its lane

- GIVEN a pull request that changes only files under
  `components/api-server/internal/`
- AND no files in shared contract, deployment, or upstream input directories
- WHEN Mergify evaluates the pull request's scopes
- THEN it SHALL match only the `api-server` scope
- AND pull requests in other scopes SHALL validate and merge independently

#### Scenario: CICD-MRG-02.03 -- Frontend toolchain change selects frontend consumers

- GIVEN a pull request that changes `pnpm-lock.yaml` or `.node-version`
- WHEN Mergify evaluates the pull request's scopes
- THEN it SHALL match `sdk-typescript`, `web-console`, `fleet-dashboard`,
  and `architecture-site`
- AND it SHALL NOT match Go-only scopes (`api-server`, `control-plane`,
  `sdk-go`)

#### Scenario: CICD-MRG-02.04 -- Documentation change is independent

- GIVEN a pull request that changes only `README.md` at the repository root
- WHEN Mergify evaluates the pull request's scopes
- THEN it SHALL match only the `documentation` scope
- AND it SHALL NOT block or be blocked by any runtime application scope

#### Scenario: CICD-MRG-02.05 -- Deployment change selects runtime consumers

- GIVEN a pull request that changes files under `deploy/` or `charts/`
- WHEN Mergify evaluates the pull request's scopes
- THEN it SHALL match the `deployment` scope and all runtime consumer scopes
  (`api-server`, `cli`, `control-plane`, `web-console`, `fleet-dashboard`,
  `agents`)
- AND it SHALL preserve ordering with pull requests in those consumer scopes

#### Scenario: CICD-MRG-02.06 -- Fleet dashboard excludes TypeScript SDK

- GIVEN a pull request that changes only files under
  `components/sdk-typescript/`
- WHEN Mergify evaluates the pull request's scopes
- THEN it SHALL match `sdk-typescript` and `web-console`
- AND it SHALL NOT match `fleet-dashboard` (no verified dependency)

#### Scenario: CICD-MRG-02.07 -- Control plane includes upstream modules

- GIVEN a pull request that changes files under `components/api-server/` that
  are imported by the control plane (protobuf, `gatewayhealth`, `secretstore`)
- WHEN Mergify evaluates the pull request's scopes
- THEN it SHALL match both `api-server` and `control-plane` scopes
- AND the two pull requests in those scopes SHALL preserve dependency ordering

#### Scenario: CICD-MRG-02.08 -- Agent images include CLI inputs

- GIVEN a pull request that changes files under `components/cli/`
- WHEN Mergify evaluates the pull request's scopes
- THEN it SHALL match `cli`, `control-plane`, and `agents` scopes
- AND this reflects that agent Containerfiles compile and embed `hsctl`

### Requirement: CICD-MRG-03 -- Barrier Files

The Mergify configuration SHALL define `scopes.barrier_files` for changes that
affect the correctness of every scope. A barrier change SHALL serialize the
parallel queue around it: all scopes SHALL be ordered relative to a barrier
pull request.

Barrier files SHALL include:

- `.mergify.yml` (merge queue configuration itself)
- `Makefile` (root build orchestration)
- `.dockerignore` (build context for all container images)
- `.github/component-paths.json` (change detection input)
- `.github/scripts/**` (change detection scripts)
- `.github/actions/detect-konflux-components/**` (Konflux detection)
- `.github/workflows/checks.yml` (required gate workflow)
- `.github/workflows/tests.yml` (required gate workflow)
- `.github/workflows/unit-tests.yml` (unit test stage)
- `.github/workflows/e2e.yml` (E2E stage)
- Repository policy scripts (`check_ci_components.py`,
  `check_dependency_pins.py`, `check_dependency_age.py`,
  `check_forbidden_terms.py`, `check_spec_ids.py` and their test files)
- `scripts/run-shell-unit-tests.sh` (shell test runner)

Barrier candidates SHALL be reviewed carefully: barriers block independent work,
so every script, all documentation, or every component build file SHALL NOT be
a barrier. Only changes that genuinely affect all validation remain barriers.

#### Scenario: CICD-MRG-03.01 -- Barrier serializes the queue

- GIVEN a pull request that changes `.github/workflows/tests.yml`
- AND two other pull requests in different scopes are being validated
- WHEN the barrier pull request enters the queue
- THEN Mergify SHALL serialize all three pull requests around the barrier
- AND the barrier pull request SHALL NOT merge until it has been validated
  against the state that includes the results of the preceding pull requests

#### Scenario: CICD-MRG-03.02 -- Non-barrier script stays in its scope

- GIVEN a pull request that changes only
  `scripts/sdk-generator/generate.sh`
- WHEN Mergify evaluates the pull request's scopes
- THEN it SHALL match the `sdk-typescript` and `sdk-go` scopes (via their
  scope definitions)
- AND it SHALL NOT be treated as a barrier

### Requirement: CICD-MRG-04 -- Fast Merge Gate Excludes E2E

The `Tests CI Gate` required check SHALL evaluate only the `detect-changes`
and `unit` stage results. E2E SHALL NOT be a `needs` dependency of the
`tests-gate` job and its result SHALL NOT be evaluated by the gate logic. The
`Tests CI Gate` check name SHALL be preserved for branch-protection continuity.

This requirement is implemented by the CI workflow changes specified in
`e2e-testing.spec.md` (requirements E2E-CORE-24 and E2E-CORE-25). The
motivation is to remove E2E's 10-20 minute runtime from merge latency while
preserving it as available validation on ordinary pull requests and as a
post-merge gate on `main`.

#### Scenario: CICD-MRG-04.01 -- Gate passes without E2E

- GIVEN a pull request whose unit tests pass
- AND whose E2E is still running or has failed
- WHEN the `Tests CI Gate` evaluates
- THEN the gate SHALL pass if `detect-changes` succeeded and the `unit`
  stage's result is `success` or `skipped`
- AND the E2E result SHALL NOT affect the gate outcome

#### Scenario: CICD-MRG-04.02 -- Unit failure still blocks merge

- GIVEN a pull request whose unit tests fail
- WHEN the `Tests CI Gate` evaluates
- THEN the gate SHALL fail because the `unit` stage's result is `failure`
- AND merge SHALL be blocked regardless of the E2E result

### Requirement: CICD-MRG-05 -- E2E Suppression on Queue Validation PRs

The E2E reusable-workflow call SHALL be suppressed on both `merge_group`
events and Mergify-generated temporary queue pull requests. Mergify temporary
queue PRs SHALL be detected by the configured queue branch prefix
(`mergify/merge-queue/`) on the head branch. Detection SHALL NOT suppress all
drafts or all bot pull requests.

Cheap required validation (detection, unit tests, repository policy, lint)
SHALL continue to run on temporary queue pull requests. Only the expensive
E2E deployment and suite execution SHALL be suppressed.

This requirement is implemented by the CI workflow changes specified in
`e2e-testing.spec.md` (requirement E2E-CORE-25).

#### Scenario: CICD-MRG-05.01 -- Mergify queue PR skips E2E

- GIVEN a Mergify temporary queue pull request with head branch
  `mergify/merge-queue/main/pr-42`
- WHEN CI evaluates which jobs to run
- THEN the E2E Kind and OpenShift deployment and suite jobs SHALL be
  suppressed
- AND the unit test, lint, and repository policy jobs SHALL run normally

#### Scenario: CICD-MRG-05.02 -- GitHub merge group skips E2E

- GIVEN a `merge_group` event for a batched merge commit
- WHEN CI evaluates which jobs to run
- THEN the E2E reusable-workflow call SHALL be suppressed
- AND the unit test, lint, and repository policy jobs SHALL run normally

#### Scenario: CICD-MRG-05.03 -- Normal PR retains E2E

- GIVEN an ordinary pull request (not a queue validation PR)
- WHEN CI evaluates which jobs to run
- THEN E2E SHALL run according to the existing e2e-relevant path gate
  (`should_run`)
- AND E2E results SHALL be visible on the pull request but SHALL NOT block
  the merge gate

### Requirement: CICD-MRG-06 -- Post-Merge E2E Concurrency

Post-merge E2E runs on push to `main` SHALL use a per-commit concurrency key
(SHA-specific) so that each merge's E2E run may complete without being
cancelled by the next merge. Frequent merges SHALL NOT repeatedly cancel
post-merge E2E before it finishes.

Per-pull-request cancellation for superseded PR revisions SHALL be preserved:
a new push to the same PR branch SHALL cancel the older run's E2E.

If per-commit main E2E proves impractical due to capacity constraints, a
separate explicit decision for periodic or latest-completed coverage SHALL be
made. GitHub concurrency pending-run replacement SHALL NOT silently become the
de facto coverage policy.

This requirement is implemented by the CI workflow changes specified in
`e2e-testing.spec.md`.

#### Scenario: CICD-MRG-06.01 -- Consecutive merges complete their E2E

- GIVEN two consecutive merges to `main` (commit A then commit B)
- WHEN commit B's push triggers the Tests workflow
- THEN commit A's E2E run SHALL NOT be cancelled by commit B's trigger
- AND both runs SHALL complete independently

#### Scenario: CICD-MRG-06.02 -- PR revision cancels older run

- GIVEN a pull request with a running E2E job for commit X
- WHEN a new commit Y is pushed to the same pull request branch
- THEN commit X's E2E run SHALL be cancelled
- AND commit Y's run SHALL start

### Requirement: CICD-MRG-07 -- Unknown Path Coverage Audit

Unknown or new paths that do not match any defined scope SHALL receive
conservative queue treatment. The configuration SHALL NOT add a universal
`**` repository scope that either couples every pull request or risks treating
unknown runtime files as independent maintenance.

A coverage audit SHALL list unclassified tracked paths and require an explicit
owner decision for new production directories. Unclassified mixed-path pull
requests SHALL be rejected by that audit until mapped, rather than inheriting
only a harmless documentation scope.

#### Scenario: CICD-MRG-07.01 -- New component directory is detected

- GIVEN a pull request that adds files under a new directory
  `components/new-service/`
- AND that directory is not covered by any scope definition
- WHEN the coverage audit runs
- THEN it SHALL report the unclassified paths
- AND the pull request SHALL NOT merge until the paths are explicitly
  classified in a scope

#### Scenario: CICD-MRG-07.02 -- Mixed classified and unclassified paths

- GIVEN a pull request that changes `components/api-server/main.go` and
  `components/new-service/handler.go`
- AND `components/new-service/` is not in any scope
- WHEN the coverage audit runs
- THEN it SHALL report the unclassified paths
- AND the pull request SHALL NOT be treated as only `api-server` scope

### Requirement: CICD-MRG-08 -- Scope and Detector Alignment

Mergify scopes decide queue independence; the CI change detector
(`.github/scripts/detect-components.sh` and `.github/component-paths.json`)
decides what is actually tested. The two systems SHALL express the same
dependency relationships where they overlap. Intentional differences SHALL be
documented (for example, optional E2E harness changes do not force application
merge ordering).

Mergify scopes do not automatically select CI jobs. A scope name does not
establish a CI dependency. The change detector independently determines which
component tests to run based on the changed files.

#### Scenario: CICD-MRG-08.01 -- Scope and detector agree on contract changes

- GIVEN a pull request that changes API OpenAPI definitions
- WHEN the change detector runs
- THEN it SHALL detect changes for the same set of consumers that the
  Mergify scopes identify (CLI, SDKs, web console, agents, control plane)
- AND no consumer whose Mergify scope includes the contract input SHALL be
  missing from the detector's output

#### Scenario: CICD-MRG-08.02 -- Test infrastructure scope is independent

- GIVEN a pull request that changes only files under `tests/e2e/`
- AND no files under any application component directory
- WHEN Mergify evaluates the pull request's scopes
- THEN it SHALL match only the `test-infrastructure` scope
- AND the pull request SHALL merge independently of application lanes
- AND the change detector SHALL detect `e2e` changes but SHALL NOT trigger
  application component tests

### Requirement: CICD-MRG-09 -- Konflux Pipeline Alignment

Konflux pipeline triggers on Mergify temporary queue pull requests SHALL be
audited for correctness. Existing dedicated merge-queue pipelines match push
branches under `gh-readonly-queue/main/`, whereas Mergify queue pull requests
exercise pull-request pipelines. Required checks SHALL report for the SHA
Mergify evaluates. Optional image and E2E builds SHALL NOT exhaust runners or
cluster capacity and thereby indirectly delay cheap CI.

GitHub merge-group assumptions SHALL NOT be copied onto Mergify queue branches
without verification.

#### Scenario: CICD-MRG-09.01 -- Required checks report on Mergify queue SHA

- GIVEN a Mergify temporary queue pull request
- WHEN required CI checks run
- THEN `Tests CI Gate` and `Checks CI Gate` SHALL report their status on the
  SHA that Mergify evaluates
- AND the check status SHALL be visible to Mergify's queue evaluation

#### Scenario: CICD-MRG-09.02 -- Optional builds do not starve required CI

- GIVEN a Mergify temporary queue pull request
- WHEN Konflux pull-request pipelines trigger
- THEN optional image builds SHALL NOT consume enough runner or cluster
  capacity to delay the required unit test and lint jobs
