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
- Address any failing CI checks found in Step 2.
- Address any merge conflicts found in Step 2.
- Commit (if not already) and push code to `$LINKED_PULL_REQUEST`.
- Remove label `agent/review-code-rejected` from `$GITHUB_ISSUE_URL` if present.
- Write label `agent/reviewable-code` to `$GITHUB_ISSUE_URL`.

_Note: If a label does not exist, create it._
