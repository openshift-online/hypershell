---
name: spec-review
description: "Review the spec changes on the work item's pull request and record a verdict."
---

## Inputs and tools

Your parameters come from the environment: `REPOSITORY`, `GITHUB_REPO_URL`, `LABEL_SCOPE`, `TRUSTED_BRANCH` (main), `DRY_RUN`, GITHUB_ISSUE_URL, ISSUE_NUMBER, GITHUB_PR_URL, PR_NUMBER. Use `gh` and the GitHub REST API for all GitHub actions; do not use MCP tools. The working directory is a checkout of `REPOSITORY`. Write your structured result to `$FULLSEND_OUTPUT_DIR/result.json`. Follow the workflow below.

---


# Workflow

Specification Review Workflow that assess a spec to be considered one of:

- `agent/review-spec-rejected`
- `agent/review-spec-approved`

## Input

```text
$GITHUB_PR_URL $GITHUB_ISSUE_URL [--dry-run]
```

Supported arguments:
- GITHUB_PR_URL -- execute against the provided GitHub pull request.
- GITHUB_ISSUE_URL -- the GitHub issue the PR is addressing.
- --dry-run -- [Optional] Execute the workflow, but without any WRITE actions.

## Workflow

### Step 1: Read Pull Request

Use the supplied `GITHUB_PR_URL` and `PR_NUMBER` and verify the explicit link to
`GITHUB_ISSUE_URL`. Do not substitute a newer or similarly named PR. Stop and
report ambiguity or a mismatched link without posting a verdict.

Clone the repo at the state of `$GITHUB_PR_URL` and familiarize yourself
with the specification changes made in the PR.

### Step 2: Review the Specification Changes

Read the [spec skill](../../../skills/plan/spec/SKILL.md).

Assess the specifications according to:
- the rules in the skill
- faithfulness to the intent found in `$GITHUB_ISSUE_URL`
- consistency with established design, architecture, and general project direction patterns.

Output: `$REVIEW_VERDICT` - One of
- `agent/review-spec-rejected`
- `agent/review-spec-approved`

### Step 3: Update Issue State

_Note: Create `$REVIEW_VERDICT` label if not exist._

Place label `$REVIEW_VERDICT` on `$GITHUB_ISSUE_URL` and `$GITHUB_PR_URL`

**Always post the review as a comment on `$GITHUB_PR_URL`, for BOTH an approved
and a rejected verdict.** An approved review MUST NOT be silent: the record of
the review must be visible on the pull request. Write the comment in
[simplified technical english](https://en.wikipedia.org/wiki/Simplified_Technical_English).
The comment MUST state:
- The verdict (`$REVIEW_VERDICT`).
- A substantive rationale: the strengths of the specification, how it meets the
  intent in `$GITHUB_ISSUE_URL`, and any non-blocking observations.
- For a rejected verdict, the blocking reasons, with inline code review comments
  where they apply.

IF `$REVIEW_VERDICT` == `agent/review-spec-approved`:
- Remove label `agent/reviewable-spec` from `$GITHUB_ISSUE_URL`.
- Post the review comment described above, stating the strengths and any
  non-blocking observations.

IF `$REVIEW_VERDICT` == `agent/review-spec-rejected`:
- Count prior spec rejections: fetch the issue event timeline. Count how many times
  `agent/review-spec-rejected` has been applied **since the most recent removal
  of `agent/blocked`** (or since the beginning of the timeline if `agent/blocked`
  has never been removed). Include this rejection in the count.
- If the rejection count is 3 or more:
  - Remove label `agent/reviewable-spec` from `$GITHUB_ISSUE_URL`.
  - Apply label `agent/blocked` to `$GITHUB_ISSUE_URL`.
  - Add a comment explaining the spec has been rejected 3 or more times and needs
    human review before the agent retries. Tell the human to remove `agent/blocked`
    and `agent/review-spec-rejected` to resume the loop.
  - STOP.
- Otherwise:
  - Remove label `agent/reviewable-spec` from `$GITHUB_ISSUE_URL`.
  - Post the review comment described above with the rejection rationale. Use
    [simplified technical english](https://en.wikipedia.org/wiki/Simplified_Technical_English),
    and inline code review comments if applicable.
