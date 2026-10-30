---
name: code-implementation
description: "Implement the code change on the work item's pull request."
---

## Runtime context (supplied by the harness as environment variables)

Your parameters come from the environment: `REPOSITORY`, `GITHUB_REPO_URL`, `LABEL_SCOPE`, `TRUSTED_BRANCH` (main), `DRY_RUN`, GITHUB_ISSUE_URL, ISSUE_NUMBER. Use `gh` and the GitHub REST API for all GitHub actions; do not use MCP tools. The working directory is a checkout of `REPOSITORY`. Write your structured result to `$FULLSEND_OUTPUT_DIR/result.json`. Follow the workflow below.

### Sandbox checkout and build tools

The local dispatcher supplies an independent Git checkout with the host's local
edits, including non-ignored untracked files. Inspect `git status` before editing;
keep unrelated local work out of your commits. Use this checkout instead of
cloning a fresh copy to work around a linked-worktree path.

The HyperShell sandbox image includes Go, Node, npm, Corepack, make, and GCC.
Use `corepack pnpm` for frontend commands; it selects the version pinned in
`package.json`. For Make targets that invoke pnpm, use
`make PNPM='corepack pnpm' <target>`. Package registry access is provided through
Fullsend's `package-registries` provider. Run relevant builds and unit tests in
the sandbox. Tests requiring a container engine, cluster, or browser installation
still need their documented infrastructure; report those limitations explicitly.

---


# Workflow

The code implementation workflow is responsible for reconciling a set of spec changes
to code, ensuring CI passes, and resolving any merge conflicts or rebase needs on an open PR.

## Input

```text
$GITHUB_ISSUE_URL [--dry-run]
```

Supported arguments:
- GITHUB_ISSUE_URL -- execute against the provided GitHub issue.
- --dry-run -- [Optional] Execute the workflow, but without any WRITE actions.

## Workflow

### Step 1: Read Issue and Open PR

Fetch `$GITHUB_ISSUE_URL` and fully read its title and body.

Discover the linked pull request.

If no linked pull request:
- apply label `agent/blocked` to `$GITHUB_ISSUE_URL` (_Note: If the label does not exist, create it._)
- write a comment on the issue noting the reason.
- STOP. Do not execute any further steps.

Output: `$LINKED_PULL_REQUEST`

### Step 2: Identify State of Implementation

Read contents of `$LINKED_PULL_REQUEST`, `$GITHUB_ISSUE_URL`, CI checks on `$LINKED_PULL_REQUEST`, and
mergeability of `$LINKED_PULL_REQUEST`.


### Step 3: Identify Spec Changes

Clone the branch associated with `$LINKED_PULL_REQUEST`. Using the git history, identify the specification
changes introduced in `$LINKED_PULL_REQUEST`. Note that specification changes may not be purely additive, and
may touch many parts of the application. Removal of, and changes to, specifications are just as important
as new specifications.


### Step 4: Reconcile Specifications

Execute the [reconcile skill](../../../skills/build/reconcile/SKILL.md) against the specification changes identified in Step 3.


### Step 5: Review Changes

Execute the [amber-review skill](../../../skills/review/amber-review/SKILL.md) against the implementation produced by Step 4.

If the review produces findings:
- Return to Step 4 and address the findings
Else:
- Address any failing CI checks found in Step 2. Before pushing, run the
  authoritative CI-equivalent gate locally (the commands in the reconcile skill
  Phase 6 step 3, or `make lint`) and require it green to avoid format/gofmt/SDK
  drift CI ping-pong.
- Address any merge conflicts found in Step 2.
- Commit (if not already) and push code to `$LINKED_PULL_REQUEST`.
- Remove label `agent/review-code-rejected` from `$GITHUB_ISSUE_URL` if present.
- Write label `agent/reviewable-code` to `$GITHUB_ISSUE_URL`.

_Note: If a label does not exist, create it._
