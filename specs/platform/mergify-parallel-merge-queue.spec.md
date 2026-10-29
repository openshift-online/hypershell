# Mergify Open Source Merge Queue Specification

**Date:** 2026-10-29
**Status:** Approved
**ID prefix:** CICD-MRG
**Related:** `e2e-testing.spec.md`, `ephemeral-pr-environments.spec.md`

## Purpose

Use Mergify's Open Source plan with cumulative speculative validation, automatic
queue entry after review and CI, and dependency-aware selective CI. Serial mode
orders merges while up to ten speculative PRs validate concurrently. Independent
scope lanes, multi-PR batches, priorities and two-step CI are not used.

The existing filename is retained for links. The previous parallel-lane design
is superseded: local impact scopes are coverage-audit metadata only, not Mergify
configuration or runtime CI filters. `.github/component-paths.json` owns runtime
selection. E2E remains optional, runs on ordinary PRs and main, and is suppressed
on temporary queue PRs as described below.

Plan basis: [pricing](https://mergify.com/pricing),
[queue modes](https://docs.mergify.com/merge-queue/queue-modes/), and
[queue action](https://docs.mergify.com/workflow/actions/queue/), checked 2026-10-29.
The Open Source plan differs from the Free private-team plan.

## Requirements

### Requirement: CICD-MRG-01 -- Serial Queue with Speculative Checks

The queue SHALL use `mode: serial` and `max_parallel_checks: 10`. It SHALL NOT
configure Mergify scopes or use isolated mode. Each speculative head SHALL
include all preceding queued changes and merge in order after required checks.

#### Scenario: CICD-MRG-01.02 -- Cumulative validation

- GIVEN A changes an API contract and B changes a separate component
- WHEN B is tested speculatively after A
- THEN B's CI SHALL select affected components from both A and B
- AND unrelated changes on main after the fork SHALL NOT be attributed to B

### Requirement: CICD-MRG-02 -- Queue Rules

Queue entry SHALL require `base = main`, `agent/review-code-approved`, non-draft
status, and successful `Checks CI Gate` and `Tests CI Gate`. Branch protection
injection SHALL remain `queue` so additional GitHub requirements apply at entry
and merge. The queue SHALL squash with PR title, inherited body, and no trailers.
Batch size and wait options SHALL be omitted (default size one).

#### Scenario: CICD-MRG-02.01 -- Approval required

- GIVEN a PR lacks the approval label
- WHEN checks succeed
- THEN the queue SHALL NOT accept it

#### Scenario: CICD-MRG-02.02 -- Squash merge with PR title

- GIVEN an approved PR passes required checks
- WHEN it merges
- THEN its commits SHALL be squashed with its title and inherited body

### Requirement: CICD-MRG-03 -- Local Dependency-Aware Impact Audit

The repository SHALL retain the consumer impact model in
`.github/ci-impact-scopes.yml`, validated by `scripts/check_ci_impact_scopes.py`.
It SHALL classify tracked paths and guard shared input consistency without
configuring Mergify scopes. These groups describe potential build/runtime impact;
they do not determine queue order or directly schedule jobs. Runtime lint/unit
selection uses `.github/component-paths.json`; deployment/E2E and documentation
have separate workflows, so broad audit groups are not identical to unit filters.

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
    and CI scripts. This scope SHALL be separate from application impact groups when only
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

#### Scenario: CICD-MRG-03.01 -- API-only change does not select frontend impact

- GIVEN a pull request changes only `components/api-server/internal/handlers/`
- WHEN the local audit classifies its paths
- THEN it SHALL select the `api-server` scope only (plus `control-plane` and
  other consumers whose include patterns match `components/api-server/**`)
- AND it SHALL NOT select `web-console` or `fleet-dashboard` scopes
- AND it SHALL be classified separately from a concurrent frontend-only pull request

#### Scenario: CICD-MRG-03.02 -- OpenAPI change propagates to consumers

- GIVEN a pull request changes `components/api-server/openapi/openapi.yaml`
- WHEN the local audit classifies its paths
- THEN it SHALL select the `api-server`, `cli`, `control-plane`,
  `sdk-typescript`, `sdk-go`, `web-console`, and `agents` scopes
- AND it SHALL NOT select `fleet-dashboard` (absent a verified SDK dependency)

#### Scenario: CICD-MRG-03.03 -- Frontend lockfile does not select Go impact

- GIVEN a pull request changes only `pnpm-lock.yaml`
- WHEN the local audit classifies its paths
- THEN it SHALL select frontend consumer scopes (`sdk-typescript`, `web-console`,
  `fleet-dashboard`, `architecture-site`) and NOT Go-only scopes
  (`api-server`, `cli`, `control-plane`, `sdk-go`)

#### Scenario: CICD-MRG-03.04 -- Fleet dashboard excludes TypeScript SDK

- GIVEN a pull request changes only `components/sdk-typescript/` files
- WHEN the local audit classifies its paths
- THEN it SHALL select the `sdk-typescript` and `web-console` scopes
- AND it SHALL NOT select the `fleet-dashboard` scope

#### Scenario: CICD-MRG-03.05 -- Control plane includes API and CLI modules

- GIVEN a pull request changes `components/api-server/pkg/api/grpc/` files
- WHEN the local audit classifies its paths
- THEN it SHALL select the `control-plane` scope (because the control plane
  module replaces the API-server module and imports those packages)

#### Scenario: CICD-MRG-03.06 -- Agent images include CLI inputs

- GIVEN a pull request changes `components/cli/` files
- WHEN the local audit classifies its paths
- THEN it SHALL select the `cli`, `control-plane`, and `agents` scopes
  (because agent Containerfiles embed `hsctl`)

### Requirement: CICD-MRG-04 -- Global Input Classification

Global build and CI inputs SHALL remain classified in the local audit's
`barrier_files` list. In serial mode every PR is already cumulatively ordered;
these entries SHALL NOT be uploaded to Mergify or imply independent queue lanes.

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

`make check` SHALL audit tracked paths against local impact groups, global inputs,
and `.github/ci-impact-allowlist.txt`. Unmapped paths SHALL fail until classified
or explicitly acknowledged. Removing Mergify scopes SHALL NOT remove this audit.

#### Scenario: CICD-MRG-08.01 -- New component is flagged

- GIVEN an unmapped file in `components/new-service/`
- WHEN the coverage audit runs
- THEN it SHALL fail until an owner classifies the path

#### Scenario: CICD-MRG-08.02 -- Classified path passes

- GIVEN an API source file covered by the local impact model
- WHEN the audit runs
- THEN it SHALL pass

### Requirement: CICD-MRG-09 -- Cumulative Component Detection

Both Checks and Tests SHALL use full-history checkout and the shared detector.
For every PR, including Mergify draft PRs, detection SHALL diff target base to
head using the merge base, with rename detection disabled to include both sides
of cross-component moves. It SHALL NOT diff only the final queued PR or use
Mergify scope metadata. Git, event parsing and metadata errors SHALL fail the
job, not silently select no components. Required gates SHALL fail when detection
fails. Manual dispatch SHALL select all components.

The component map SHALL include upstream API helpers and CLI code consumed by
control-plane, API and SDK generators consumed by SDKs/console, domain probes
consumed by both shared UI packages, and shared frontend toolchain inputs.

#### Scenario: CICD-MRG-09.01 -- CLI change selects consumers

- GIVEN a CLI kustomize change
- WHEN detection runs
- THEN CLI and control-plane tests SHALL be selected

#### Scenario: CICD-MRG-09.02 -- OpenAPI change selects consumers

- GIVEN an OpenAPI change
- WHEN detection runs
- THEN API, CLI, control-plane, SDKs and web-console SHALL be selected
- AND unrelated fleet UI jobs SHALL remain unselected

### Requirement: CICD-MRG-10 -- Queue Integration

Mergify's bypass of the native GitHub merge-queue requirement is confirmed by
the repository owner. This change SHALL preserve that setup and the existing
required checks. Strict up-to-date checks SHALL remain disabled for squash
speculation. GitHub Actions SHALL run on Mergify draft PRs without a blanket
draft exclusion. Konflux PR pipelines may build optional images; those checks
SHALL NOT become required gates in this change. Their path filters remain
separate from lint/unit selection.

#### Scenario: CICD-MRG-10.01 -- Required checks on queue PRs

- GIVEN a temporary Mergify PR
- WHEN CI runs
- THEN both required gates SHALL report on that PR's revision
- AND E2E SHALL remain outside the merge gate

### Requirement: CICD-MRG-11 -- Deployment Impact Audit

Deployment/release inputs SHALL remain classified in deployment and runtime
consumer impact groups in the local audit. These conservative groups document
runtime impact, not a mandate to run every unit suite for YAML-only changes.
Existing deployment validation and optional E2E routing SHALL be preserved.

#### Scenario: CICD-MRG-11.01 -- Chart impact includes runtime consumers

- GIVEN a chart input change
- WHEN the local audit evaluates it
- THEN deployment and runtime consumer impact groups SHALL match

#### Scenario: CICD-MRG-11.02 -- API code is not deployment input

- GIVEN an API implementation change
- WHEN the local audit evaluates it
- THEN deployment SHALL NOT be selected

### Requirement: CICD-MRG-12 -- Automatic Queue Entry

A `pull_request_rules` queue action SHALL automatically enqueue non-draft PRs to
main with the approval label and both required CI gates successful, using the
named queue rule. The same conditions SHALL be present on the queue rule so
manual commands cannot bypass them. Removing approval SHALL make the PR
ineligible. No paid priorities, scheduling, batching, or two-step CI are needed.

#### Scenario: CICD-MRG-12.01 -- Approval and successful checks enqueue

- GIVEN a non-draft PR to main has the approval label
- WHEN both required gates succeed
- THEN Mergify SHALL automatically enqueue it without a manual command
- AND a missing, pending, failed or cancelled gate SHALL prevent automatic entry

## Rollout and Operations

Merge this configuration PR through the existing manual flow. After it lands,
observe an approved PR automatically enter and a speculative draft PR report both
gates. Existing approved, green PRs can also become eligible automatically.
Keep the approval label off PRs that should not merge. For an incident, remove
the approval label from affected original PRs and dequeue them before repair;
reapply approval only when ready. Avoid external merges and edits to queued PRs,
which invalidate speculative work. Monitor runner wait, required-check duration,
queue latency and reruns before increasing concurrency above ten.

## Retired IDs

- `CICD-MRG-01.01` - independent-lane behavior superseded by serial speculation.
- `CICD-MRG-01.03` - independent-lane behavior superseded by serial speculation.
- `CICD-MRG-04.01` - independent-lane behavior superseded by serial speculation.
- `CICD-MRG-04.02` - independent-lane behavior superseded by serial speculation.
