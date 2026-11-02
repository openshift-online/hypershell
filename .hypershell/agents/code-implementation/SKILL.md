---
name: code-implementation
description: >
  Code Implementation Workflow
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
  - For a failing E2E check (`Tests / E2E / Kind` or `Tests / E2E / OpenShift`),
    reproduce only the failing phase or step rather than the whole suite: the Go
    E2E suite runs each phase/step in isolation. If `$REJECTION_RATIONALE` names a
    step (for example `P2.5`), run `make e2e E2E_RUN='P2\.5'` (a whole phase with
    `E2E_RUN=Phase-P2`; prefix `E2E_INFRA_DRIVER=openshift` to match the OpenShift
    check). The suite provisions the primary gateway on demand in setup, and
    `E2E_PRIMARY_GATEWAY_ID=<id>` adopts an already-running gateway to cut
    turnaround. See `tests/e2e/README.md`.
- Address any merge conflicts found in Step 2.
- Commit (if not already) and push code to `$LINKED_PULL_REQUEST` as **one commit
  per component** (api-server, CLI, SDK (Go/TS), proto/gRPC, control-plane, tests,
  docs, as applicable), each with a conventional-commit subject - not one squashed
  blob. The funnel PR already carries the spec commit from the spec phase; add the
  per-component code commits onto that same PR and NEVER open a second PR (one
  PR, many commits; see "Pull request and commit model" in
  `../../execute-sdlc/SKILL.md`). Ensure the PR body & title reflect the current
  state of the PR.
- Advance the phase on `$GITHUB_ISSUE_URL`:
  - Write label `agent/reviewable-code`.
  - Remove the prior PHASE labels this transition supersedes, if present:
    `agent/review-spec-approved` (the first code pass) and
    `agent/review-code-rejected` (a re-implementation after code review).
    Removing a label that is already absent is a no-op.

**Protected labels (never remove).** The human gate `agent/review-spec-blocked`
and the role markers are NOT phase labels and MUST NOT be removed by this step,
even while normalizing phase state: `agent/review-spec-blocked` (human spec gate),
`agent/allowed` (authorization gate), `agent/scope/*` (scope selector), and
`agent/execution-record` (canonical-link marker). The paused-state holds
`agent/blocked`, `agent/needs-input`, and `agent/duplicate` are also NOT phase
labels and MUST NOT be removed here; removing one would silently un-pause the item.
This step removes ONLY the prior phase labels named above - it is not a license to
remove everything but one label.

_Note: If a label does not exist, create it._
