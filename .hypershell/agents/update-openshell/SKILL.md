---
name: update-openshell
description: >
  Agent wrapper around the update-openshell tooling skill. Bumps the pinned
  OpenShell gateway/supervisor image to the latest stable midstream release and
  opens a pull request.
---

# Workflow

Keep HyperShell current with the latest **stable midstream** OpenShell release by
running the tooling skill and opening one pull request. This agent is the
scheduled, non-interactive entry point; all the judgment lives in the tooling
skill.

## Input

```text
$REPOSITORY [--dry-run]
```

Supported arguments:
- REPOSITORY -- the repository to update (e.g. `openshift-online/hypershell`).
- --dry-run -- [Optional] Resolve and report the target version and the files
  that would change, but make no commits, branches, pull requests, or writes.

## Workflow

### Step 1: Resolve the target version

Read the [update-openshell tooling skill](../../../skills/tooling/update-openshell/SKILL.md)
and resolve the **latest stable midstream** tag per its "Resolving versions"
section (the `opendatahub-io/openshell` `v*-rhaiv.*` tag list). Do not use the
agent-build track.

If the repository is already pinned to that tag, make no change and report that
the pin is current. STOP.

### Step 2: Execute the tooling skill

Follow the tooling skill's workflow to that target version: sweep the pinned image
versions across the repo, triage the release notes for contract-affecting
changes, verify the rendered gateway config still matches upstream, and fold any
lesson learned back into the tooling skill and the affected specs **in the same
change**.

### Step 3: Open the pull request

IF `DRY_RUN` is `true`:
- Report the resolved target version, the files that would change, and any
  contract-affecting findings. Make no writes. STOP.

IF `DRY_RUN` is `false`:
- Create a new branch of the form `implementer/update-openshell-<target>` where
  `<target>` is the resolved tag.
- Commit the version-pin sweep and any spec/skill updates.
- Push the branch and open one pull request against the default branch in
  `$REPOSITORY`. Summarize the version change and the triaged release notes in
  the PR body, in [simplified technical english](https://en.wikipedia.org/wiki/Simplified_Technical_English).
  Add the label `agent/reviewable-code`.
- Never merge, close, or force-push. The merge is always a human's.

Before you finish, confirm from the GitHub API that the pull request you claim to
have opened actually exists.

## Report the outcome (RESULT_FILE)

Write a single JSON object to `$RESULT_FILE` before you finish (a local write, not
a GitHub action; do it even under `DRY_RUN`):

```json
{"status": "success|skip|failed", "summary": "<one line under 100 chars>"}
```

- `success` -- opened a PR (or, under `DRY_RUN`, produced a complete preview).
- `skip` -- already pinned to the latest tag; no change.
- `failed` -- could not complete; include the reason in `summary`.
