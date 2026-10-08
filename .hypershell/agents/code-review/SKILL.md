---
name: code-review
description: >
  Code Review Workflow
---

# Workflow

Code review workflow that classifies issues as:
- `agent/review-code-rejected`
- `agent/review-code-approved`

Review is rejected for any of the following reasons:
- Implementation review has findings
- Failing CI
- Merge conflicts
- Needs rebase

## Input

```text
$GITHUB_ISSUE_URL [--dry-run]
```

Supported arguments:
- GITHUB_ISSUE_URL -- execute against the provided GitHub issue.
- --dry-run -- [Optional] Execute the workflow, but without any WRITE actions.

## Workflow

### Step 1: Read Issue and Linked PR

Using the GitHub API, fetch `$GITHUB_ISSUE_URL`.

Read the issue title and body and identify the linked PR.

Output: (`$LINKED_PR`, `$ISSUE_CONTENTS`)

### Step 2: Review the Implementation

Execute [amber-review](../../../skills/review/amber-review/SKILL.md) against `$LINKED_PR`. Review in the
context of `$ISSUE_CONTENTS`.

Review must be rejected for any of the following reasons:
- Implementation review has findings
- Failing CI
- Merge conflicts
- Needs rebase

Verdict must be one of:
- `agent/review-code-rejected`
- `agent/review-code-approved`

Output: (`$VERDICT`, `$REJECTION_RATIONALE` (NULL or string))


### Step 3: Communicate Outcome

_Note: If a label does not exist, create it._

Apply `$VERDICT` label to `$GITHUB_ISSUE_URL`.

IF `$VERDICT` == `agent/review-code-approved`:
- Remove label `agent/reviewable-code` from `$GITHUB_ISSUE_URL`.
- The PR is ready to merge. A human must perform the merge.

IF `$VERDICT` == `agent/review-code-rejected`:
- Count prior rejections: fetch the issue event timeline and count how many times
  `agent/review-code-rejected` has been applied (including this one).
- If the rejection count is 3 or more:
  - Apply label `agent/blocked` to `$GITHUB_ISSUE_URL` instead of re-queuing.
  - Add a comment explaining the issue has been rejected 3 or more times and needs
    human review before the agent retries.
  - STOP. Do not remove `agent/reviewable-code`; leave the issue in a blocked state.
- Otherwise:
  - Remove label `agent/reviewable-code` from `$GITHUB_ISSUE_URL`.
  - Add a comment to `$GITHUB_ISSUE_URL` communicating `$REJECTION_RATIONALE` that
    adheres to the [simplified technical english](https://en.wikipedia.org/wiki/Simplified_Technical_English) standard.
