---
name: maintain-ci
description: Maintain HyperShell CI workflows and component-aware check routing. Use when adding, renaming, moving, or removing a component; changing GitHub Actions, linting, tests, generation, or drift checks; editing component path detection; or reviewing whether CI covers a repository change without redundant runs.
---

# Maintain CI

Keep required CI coverage complete while running expensive checks only for affected components.

## Workflow

1. Inspect the component's language, module files, build commands, generated artifacts,
   and dependencies on other components.
2. Update `.github/component-paths.json`. Register the component's own paths plus any
   shared contracts or upstream paths that can affect it. Set `directory` and `lint_job`;
   `make check` rejects missing or stale component registrations.
3. Wire the component through change detection and the lint stage. Change detection runs
   once in the `detect-changes` job of `.github/workflows/ci.yml`: add a detector output
   there and pass it into the `lint` (and `unit`/`e2e`) stage as a `with:` input. In
   `.github/workflows/lint.yml`, declare the matching `workflow_call` input and add the
   component job gated on `inputs.<component> == 'true'`. The stage workflow needs no
   summary job of its own: `ci.yml` has one rollup gate job per stage (`Lint CI Gate`,
   `Unit Tests CI Gate`, `E2E CI Gate`) that runs `if: always()`, reads the stage's
   rolled-up `result`, and is the required branch-protection check; a skipped component
   job is acceptable and still passes the gate. `make check` enforces this wiring end to
   end.
4. Update `.github/workflows/unit-tests.yml` the same way when the component has unit
   tests: add its `workflow_call` input (and the `with:` pass-through in `ci.yml`) and
   gate the job on `inputs.<component>`. Frontend packages share `test-frontend`; Go
   modules get their own jobs; `*_test.sh` files are auto-discovered by `make ci-test`
   and do not need a job allowlist. Stage wiring is owned by the
   `.github/workflows/ci.yml` orchestrator via native `needs:` edges, not by
   in-workflow poller jobs: lint, unit, and e2e are reusable workflows
   (`on: workflow_call`) with no event triggers of their own. The shape is
   fan-out then join -- lint and unit each `needs: detect-changes` and run
   concurrently, and e2e joins on both (`needs: [detect-changes, lint, unit]`)
   so the expensive Kind run is gated behind the two cheap stages without
   serializing lint and unit against each other. Do not add
   `pull_request`/`push` triggers to a stage workflow (that would double every
   run) and do not add a job that polls for a preceding stage's gate.
5. Add or update a path-filtered drift workflow when generated output is committed.
   Include generator inputs, generated outputs, generator configuration, and the workflow
   itself in its path filters.
6. For the lint/unit-tests/e2e pipeline, event triggers live only on `ci.yml`; the
   stage workflows stay `on: workflow_call`. For a standalone workflow (e.g. a drift
   gate), use `pull_request` for PR validation and restrict `push` to `main` to avoid
   duplicate feature-branch runs. Include `merge_group` when the check is required for
   merge queues.
7. Pin every action to a full commit SHA, every container image to a digest, and every
   installed tool to an exact version. Register tools that are not part of a module or
   lockfile in `dependency-age-tools.json`. Run `make check` to enforce immutable pins
   and the minimum dependency age.

## Component Lifecycle Checklist

For an added or renamed component, verify all of the following:

- The `detect-changes` job in `ci.yml` emits a dedicated output and matches
  component-local changes.
- Changes to shared or upstream contracts also select every affected downstream component.
- The detector output is passed into each stage as a `with:` input, and the stage jobs
  gate on `inputs.<component>`.
- The lint workflow uses the component's own toolchain and dependency cache files.
- Unit tests for the component run in `.github/workflows/unit-tests.yml` (concurrently with Lint).
- Committed generated code has a reproducible regeneration command and drift gate.
- `CLAUDE.md` documents any new local development command.

For a removed component, remove its detector output and `with:` pass-through in `ci.yml`,
its `workflow_call` input and job in the stage workflows, its detector paths, and its
generation gate together.

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
