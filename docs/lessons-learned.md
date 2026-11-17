# Lessons Learned

Operational gotchas learned from building and running this codebase. Distinct from
[ADRs](adr/) (design rationale): these are "things that bit us, so you do not repeat
them." Reference-only; not loaded by reconcile. Append as new lessons surface; keep each
entry to the trap + the fix.

## CI / build

- **Konflux PR image builds only trigger on `target_branch == main`.** A PR created with
  a non-`main` base (for example stacked on another feature branch), or force-pushed
  before it is retargeted to `main`, never fires its Konflux build, and the `E2E` gate
  then fails after ~18 min waiting for an image that was never built. Fix: after the base
  is `main`, retrigger with a PR comment `/test <pipeline-name>` (for example
  `/test hypershell-control-plane-main-on-pull-request`), wait for it green, then re-run
  the E2E workflow.

- **Re-run the whole E2E run, never `--failed`.** `gh run rerun <id> --failed` fails
  instantly because the `Deploy OpenShift Environment` step is skipped on a partial rerun
  (the environment was torn down). Use `gh run rerun <id>`.

- **`check-spec-ids` only sees git-tracked files.** A brand-new `*.spec.md` must be
  `git add`ed before cross-references to its IDs resolve; otherwise every reference to its
  new IDs reports "unknown ID" even though the file exists on disk.

## Specs

- **Spec-ID merge collisions are invisible to git.** Two concurrent PRs can both grab the
  same next sequential requirement ID; git merges cleanly and the duplicate only surfaces
  as a `check-spec-ids` failure on the merged tree. After merging `main`, re-run
  `check-spec-ids` on the merged result and renumber your IDs (and every cross-reference)
  if they collided.

- **Em dashes (U+2014) are rejected repo-wide by the pre-commit hook**, including in
  vendored and generated files. Sanitize upstream content (replace with `-`) before
  committing it.

## OpenShell gateway

- **CLI/gateway proto compatibility is version-pinned.** A downstream `*-rhaiv` gateway
  needs the exact version-matched CLI from `quay.io/opendatahub/odh-openshell-cli:<tag>`;
  a public NVIDIA/OpenShell release is not guaranteed proto-compatible. To build a client,
  vendor the proto from `opendatahub-io/openshell@<tag>` (the fork the gateway is built
  from) and confirm the wire contract via `grpcurl -insecure <gw>:443 describe`.

- **`openshell-admin` bypasses per-workspace membership; `openshell-user` does not.** An
  admin can operate in any workspace; a user must be an explicit member of the workspace
  (managed mode). Agent service accounts work because they hold admin; a federated CI user
  (`openshell-user`) is rejected with "not a member of workspace" until membership is
  provisioned.
