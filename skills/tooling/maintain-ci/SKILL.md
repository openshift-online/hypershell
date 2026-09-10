---
name: maintain-ci
description: Maintain HyperShell CI workflows and component-aware check routing. Use when adding, renaming, moving, or removing a component; changing GitHub Actions, linting, tests, generation, or drift checks; editing component path detection; or reviewing whether CI covers a repository change without redundant runs.
---

# Maintain CI

Keep required CI coverage complete while running expensive checks only for affected components.

## Workflow

HyperShell's CI is two independently-triggered, top-level workflows that run
fully concurrently: `.github/workflows/checks.yml` (lint, repository policy,
SDK drift) and `.github/workflows/tests.yml` (unit tests, e2e). GitHub
Actions `needs:` only orders jobs within one workflow file, so the two do
not gate each other, and each runs its own `detect-changes` job rather than
sharing one.

1. Inspect the component's language, module files, build commands, generated artifacts,
   and dependencies on other components.
2. Update `.github/component-paths.json`. Register the component's own paths plus any
   shared contracts or upstream paths that can affect it. Set `directory` and `lint_job`
   (if it has a lint job), and set `unit_tested: true` if it has a unit-test job in
   `unit-tests.yml`; `make check` rejects missing or stale component registrations for
   both, including a `unit_tested: true` component missing its `unit-tests.yml` wiring.
3. Wire the component through `.github/workflows/checks.yml`'s change detection and lint
   job. Add a detector output to its own `detect-changes` job, then add the component's
   lint job with `needs: detect-changes` gated on
   `needs.detect-changes.outputs.<component> == 'true'`. The workflow needs no separate
   summary job for this: `checks.yml` has one rollup gate job, `checks-gate` (`Checks CI
   Gate`), that runs `if: always()`, reads every job's rolled-up `result`, and is the
   required branch-protection check. A skipped component job is acceptable and still
   passes the gate. `make check` enforces this wiring end to end.
4. Update `.github/workflows/unit-tests.yml` the same way when the component has unit
   tests: add its `workflow_call` input (and the `with:` pass-through in
   `.github/workflows/tests.yml`'s own `detect-changes` job) and gate the job on
   `inputs.<component>`. Frontend packages share `test-frontend`; Go modules get their own
   jobs; `*_test.sh` files are auto-discovered by `make ci-test` and do not need a job
   allowlist. Stage wiring is owned by the `tests.yml` orchestrator via native `needs:`
   edges, not by in-workflow poller jobs: unit and e2e are reusable workflows (`on:
   workflow_call`) with no event triggers of their own. `unit` depends only on
   `detect-changes`, and `e2e` joins on it (`needs: [detect-changes, unit]`) so the
   expensive Kind run is gated behind the cheap unit stage. GitHub Actions skips a job by
   default if any needed job failed *or was skipped*, so `e2e` also carries an explicit
   `if: ${{ !cancelled() && needs.detect-changes.result == 'success' && needs.unit.result
   != 'failure' }}` -- without it, a change touching only e2e-owned paths (every `unit`
   job path-filtered away, so the `unit` caller job itself resolves to `skipped`) would
   silently skip `e2e` too. `tests.yml`'s `tests-gate` job (`Tests CI Gate`) rolls both
   stages' results into the other required branch-protection check. The two gate jobs are
   named distinctly (`Checks CI Gate` / `Tests CI Gate`) rather than both plain `CI Gate`:
   this repo's branch protection is a ruleset whose `required_status_checks` match by
   `(context name, integration_id)` only, not by workflow file, so identically-named gates
   from the two workflows would be indistinguishable to it. Do not add `pull_request`/
   `push` triggers to a stage workflow (that would double every run) and do not add a job
   that polls for a preceding stage's gate, or for the separate `checks.yml` workflow.
5. Add or update a generated-code drift check as a job in `.github/workflows/checks.yml`,
   gated on `needs.detect-changes.outputs.<component>` for the component(s) whose paths
   affect the generator (e.g. `needs.detect-changes.outputs.sdk_go == 'true' ||
   needs.detect-changes.outputs.sdk_typescript == 'true'`). Do not create a new standalone,
   independently-triggered workflow for this -- that would duplicate change detection
   instead of sharing `checks.yml`'s one `detect-changes` pass. Whole-repo checks that are
   not tied to a specific component (e.g. repository policy) go in `checks.yml` with no
   `if:` condition, so they always run.
6. Event triggers for the checks pipeline live only on `checks.yml`; for the
   unit-tests/e2e pipeline they live only on `tests.yml`. The unit-tests and e2e stage
   workflows stay `on: workflow_call`.
7. Pin every action to a full commit SHA, every container image to a digest, and every
   installed tool to an exact version. Register tools that are not part of a module or
   lockfile in `dependency-age-tools.json`. Run `make check` to enforce immutable pins
   and the minimum dependency age.

## Component Lifecycle Checklist

For an added or renamed component, verify all of the following:

- The `detect-changes` job in `checks.yml` emits a dedicated output and matches
  component-local changes; if the component has unit tests, `tests.yml`'s own
  `detect-changes` job does too.
- Changes to shared or upstream contracts also select every affected downstream component.
- The `checks.yml` lint job gates on `needs.detect-changes.outputs.<component>`; a
  component with unit tests also gets a `with:` input pass-through in `tests.yml` and
  gates its `unit-tests.yml` job on `inputs.<component>`.
- The lint job uses the component's own toolchain and dependency cache files.
- Unit tests for the component run in `.github/workflows/unit-tests.yml`. If they connect
  to a real dependency instead of mocking it, check whether the component's test framework
  already self-provisions it (e.g. the API server's `rh-trex-ai`-based tests spin up their
  own PostgreSQL via `testcontainers-go` when `API_ENV=integration_testing` -- Docker on
  the `ubuntu-24.04` runner is enough, no `services:` block needed) before adding a manual
  `services:` container; a self-provisioning framework run without the right env var fails
  as a connection error or an explicit "not implemented for non-integration-test env" panic,
  not a build error, and can go unnoticed until the job's `if:` first evaluates to true.
- Committed generated code has a reproducible regeneration command and a drift check job
  in `checks.yml`.
- `CLAUDE.md` documents any new local development command.

For a removed component, remove its detector output and job in `checks.yml`, its detector
output and `with:` pass-through in `tests.yml` (if it had unit tests), its `workflow_call`
input and job in `unit-tests.yml`, its detector paths, and its generation gate together.

## Validation

Before committing:

1. Run `bash -n .github/scripts/detect-components.sh` and `jq empty
   .github/component-paths.json`.
2. Simulate a docs-only change, each component change, each shared-contract change, and
   `workflow_dispatch` with `.github/scripts/detect-components.sh`.
3. Run formatting, vet, and the pinned linter for every new or changed job.
4. Run each regeneration command and require a clean `git status` for generated paths.
5. Validate workflow syntax with `actionlint`, run `make check`, and run `git diff --check`.
6. After pushing, confirm one PR-triggered run exists and only selected component jobs ran.

Do not consider a component registered until detection, input wiring, execution, pinning,
and generated-code coverage have all been evaluated.
