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

**Always post the review as a comment on `$GITHUB_ISSUE_URL`, for BOTH an
approved and a rejected verdict.** An approved review MUST NOT be silent: the
record of the review must be visible. Write the comment in
[simplified technical english](https://en.wikipedia.org/wiki/Simplified_Technical_English).
The comment MUST state:
- The verdict (`$VERDICT`).
- A substantive rationale: the strengths of the implementation, how it meets the
  intent in `$ISSUE_CONTENTS`, and any non-blocking observations.
- For a rejected verdict, the blocking reasons (`$REJECTION_RATIONALE`), with
  inline code review comments where they apply.

IF `$VERDICT` == `agent/review-code-approved`:
- Remove label `agent/reviewable-code` from `$GITHUB_ISSUE_URL`.
- Post the review comment described above, stating the strengths and any
  non-blocking observations.
- The PR is ready to merge. A human must perform the merge.

IF `$VERDICT` == `agent/review-code-rejected`:
- Count prior rejections: fetch the issue event timeline. Count how many times
  `agent/review-code-rejected` has been applied **since the most recent removal
  of `agent/blocked`** (or since the beginning of the timeline if `agent/blocked`
  has never been removed). Include this rejection in the count.
- If the rejection count is 10 or more:
  - Remove label `agent/reviewable-code` from `$GITHUB_ISSUE_URL`.
  - Apply label `agent/blocked` to `$GITHUB_ISSUE_URL` instead of re-queuing.
  - Add a comment explaining the issue has been rejected 10 or more times and needs
    human review before the agent retries. Tell the human to remove `agent/blocked`
    and `agent/review-code-rejected` to resume the loop.
  - STOP.
- Otherwise:
  - Remove label `agent/reviewable-code` from `$GITHUB_ISSUE_URL`.
  - Post the review comment described above, communicating `$REJECTION_RATIONALE`,
    that adheres to the [simplified technical english](https://en.wikipedia.org/wiki/Simplified_Technical_English) standard.
