# Implementation log

## Progress

- Installed repository hooks with `make hooks-install`.
- Rebasing `docs/architecture-site` onto current `main` completed.
- Resolved the `pnpm-lock.yaml` importer conflict by retaining the current operational-dashboard importer and the architecture-site importer.
- Confirmed the five original review findings were already addressed by commit `abb7961` before the rebase; their review threads remain outdated but unresolved.

## Changes

- Replaced `informer–reconciler` with `informer-reconciler` in `docs/architecture/source/control-plane.md`.
- Added a Marked HTML renderer override in `tools/architecture-site/build.mjs` that escapes source HTML instead of passing it into published pages.

## Remaining work

- Commit and push the rebased branch, reply to all review threads, and resolve them.

## Verification

- `pnpm install --frozen-lockfile --filter @openshift-online/hypershell-architecture-site` through the repository-pinned Node and pnpm versions: passed.
- Architecture source validation: 13 pages and Mermaid diagrams passed.
- Architecture site build: passed.
- Temporary `<script>` Markdown fixture: generated output contained escaped `&lt;script&gt;` text and no executable `<script>` tag.
- `make check` under Node 24.18.1: passed, including forbidden terms, dependency pins, CI registration, and dependency age policy.
- `git diff --check`: passed.
- Initial direct `pnpm` commands could not run because pnpm was not on the shell PATH; reran through mise/Corepack with Node 24.18.1 and pnpm 11.15.1.
- One root `pnpm run` attempt failed because its script recursively expected `pnpm` on PATH; reran the equivalent filtered package commands directly and they passed.
