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
3. Update `.github/workflows/lint.yml` with a detector output, component job, and entry
   in the stable `Lint CI gate` summary. A skipped component job is acceptable; detector
   failures, cancellations, and component failures must fail the summary.
4. Add or update a path-filtered drift workflow when generated output is committed.
   Include generator inputs, generated outputs, generator configuration, and the workflow
   itself in its path filters.
5. Use `pull_request` for PR validation and restrict `push` to `main` to avoid duplicate
   feature-branch runs. Include `merge_group` when the check is required for merge queues.
6. Pin every action to a full commit SHA, every container image to a digest, and every
   installed tool to an exact version. Run `make check` to enforce repository policy.

## Component Lifecycle Checklist

For an added or renamed component, verify all of the following:

- The detector emits a dedicated output and matches component-local changes.
- Changes to shared or upstream contracts also select every affected downstream component.
- The lint workflow uses the component's own toolchain and dependency cache files.
- The summary job declares the component job in `needs` and evaluates its result.
- Committed generated code has a reproducible regeneration command and drift gate.
- `CLAUDE.md` documents any new local development command.

For a removed component, remove its detector paths, workflow output, job, summary dependency,
and generation gate together.

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

Do not consider a component registered until detection, execution, summary gating, pinning,
and generated-code coverage have all been evaluated.
