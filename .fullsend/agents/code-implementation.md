---
name: code-implementation
description: "Implement the code change on the work item's pull request."
---

## Inputs and tools

Your parameters come from the environment: `REPOSITORY`, `GITHUB_REPO_URL`, `LABEL_SCOPE`, `TRUSTED_BRANCH` (main), `DRY_RUN`, GITHUB_ISSUE_URL, ISSUE_NUMBER. Use `gh` and the GitHub REST API for all GitHub actions; do not use MCP tools. The working directory is a checkout of `REPOSITORY`.

You do the implementation work (reconcile the spec to code, fix CI, resolve conflicts, push) but you do NOT change any `agent/*` lifecycle label. You write a structured result to `$FULLSEND_OUTPUT_DIR/agent-result.json`; a trusted post-script validates it and applies the label transition (see `specs/standards/sdlc/fullsend-lifecycle-label-hooks.spec.md`, SDLC-HOOK).

Write exactly one JSON object to `$FULLSEND_OUTPUT_DIR/agent-result.json`:

```json
{
  "schema_version": "1",
  "phase": "code-implementation",
  "outcome": "completed | blocked",
  "rationale": "one or two sentences on what was done or why it is blocked",
  "pr_number": 123,
  "head_sha": "<40-hex head commit you pushed>",
  "comment": "optional markdown to post on the issue"
}
```

Use `completed` when the code is pushed and the PR is ready for code review;
`pr_number` and `head_sha` are then REQUIRED and MUST identify the PR and the
commit you pushed. Use `blocked` when a hard prerequisite fails (for example no
linked PR). On `completed` the post-script sets `agent/reviewable-code`; it never
takes a label name from this result. Follow the workflow below.

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
- Write a result with `outcome: "blocked"` and a `comment` explaining that no
  pull request is linked to the issue.
- STOP. Do not execute any further steps. The post-script applies `agent/blocked`.

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

### Step 6: Write the Result

Write the result object to `$FULLSEND_OUTPUT_DIR/agent-result.json`:
- `outcome`: `completed` when the code is pushed and the PR is ready for review.
- `pr_number` and `head_sha`: the PR number and the exact head commit you pushed.
- `rationale` and optional `comment`.

Do not add or remove any label. The trusted post-script reads this result,
removes any stale `agent/review-code-rejected` or `agent/review-code-approved`,
adds `agent/reviewable-code`, preserves `agent/review-spec-approved` as a
completed prerequisite, and verifies the final state.
