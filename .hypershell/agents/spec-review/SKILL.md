---
name: spec-review
description: >
  Specification Review Workflow
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

IF `$REVIEW_VERDICT`  == `agent/review-spec-rejected`:
- Write rejection rationale as a comment on the PR. Use [simplified technical english](https://en.wikipedia.org/wiki/Simplified_Technical_English), and inline code review comments if applicable.
