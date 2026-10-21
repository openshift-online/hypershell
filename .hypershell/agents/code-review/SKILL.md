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

### Step 2: Review the Implementation (SDLC-REM-01)

Determine pull-request health **deterministically from the GitHub API** before
running the code review:

1. Query the pull request resource for `mergeable` and `mergeable_state` to
   determine conflict and rebase status.
2. Query the check-runs endpoint (`GET /repos/{owner}/{repo}/commits/{sha}/check-runs`)
   for each check's name, conclusion, status, and details URL (logs URL).
3. Query the commit status endpoint for any status contexts.

Record:

| Field | Source |
|-------|--------|
| `$MERGEABLE` | `mergeable` field from the pull request |
| `$MERGE_STATE` | `mergeable_state` field |
| `$CHECK_RUNS` | Array of `{name, status, conclusion, details_url}` from the check-runs endpoint |
| `$FAILING_CHECKS` | Subset of `$CHECK_RUNS` where `conclusion` is not `success`, `skipped`, or `neutral` |

Do NOT infer CI or conflict state from the diff, PR body, or any heuristic.

Execute [amber-review](../../../skills/review/amber-review/SKILL.md) against `$LINKED_PR`. Review in the
context of `$ISSUE_CONTENTS`.

Review must be rejected for any of the following reasons:
- Implementation review has findings
- Failing CI (based on `$FAILING_CHECKS`)
- Merge conflicts (based on `$MERGE_STATE` == `dirty`)
- Needs rebase (based on `$MERGE_STATE` == `behind`)

Verdict must be one of:
- `agent/review-code-rejected`
- `agent/review-code-approved`

Output: (`$VERDICT`, `$REJECTION_RATIONALE` (NULL or string))


### Step 3: Communicate Outcome (SDLC-REM-01)

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

**For a rejection due to CI failure, conflict, or rebase state**, the rejection
comment SHALL include structured diagnostic targets for the implementer:
- The specific failing check names and their conclusions (from `$FAILING_CHECKS`).
- The logs URL (`details_url`) for each failing check.
- The conflict or rebase state (from `$MERGE_STATE`).

This ensures code-implementation receives concrete targets for remediation
rather than a generic "failing CI" or "needs rebase" message.

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
