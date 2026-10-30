---
name: code-review
description: "Review the code changes on the work item's pull request and record a verdict."
---

## Inputs and tools

Your parameters come from the environment: `REPOSITORY`, `GITHUB_REPO_URL`, `LABEL_SCOPE`, `TRUSTED_BRANCH` (main), `DRY_RUN`, GITHUB_ISSUE_URL, ISSUE_NUMBER, GITHUB_PR_URL, PR_NUMBER. Use `gh` and the GitHub REST API for read-only GitHub actions; do not use MCP tools. The working directory is a checkout of `REPOSITORY`.

You reach a verdict but you do NOT change any `agent/*` lifecycle label and you do NOT count prior rejections or apply a block. You write a structured result to `$FULLSEND_OUTPUT_DIR/agent-result.json`; a trusted post-script validates it, rejects it as stale if the PR head moved, gates an approval on required checks, counts prior rejections from the issue timeline to decide blocking, applies the verdict transition, posts your comment, and verifies the final state (see `specs/standards/sdlc/fullsend-lifecycle-label-hooks.spec.md`, SDLC-HOOK).

Write exactly one JSON object to `$FULLSEND_OUTPUT_DIR/agent-result.json`:

```json
{
  "schema_version": "1",
  "phase": "code-review",
  "outcome": "approved | rejected",
  "rationale": "short summary of the verdict",
  "pr_number": 123,
  "head_sha": "<40-hex head commit you reviewed>",
  "comment": "the full review in simplified technical english (required)"
}
```

`head_sha` MUST be the PR head commit you actually reviewed. `comment` is
REQUIRED for both verdicts. Follow the workflow below.

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

When `GITHUB_PR_URL` and `PR_NUMBER` are supplied, review that exact PR and
verify its explicit link to this issue. Do not substitute a newer or similarly
named PR. Stop and report ambiguity or a mismatched link without posting a verdict.
When no PR is supplied, resolve exactly one linked PR from the issue; apply the
same ambiguity rule.

Output: (`$LINKED_PR`, `$ISSUE_CONTENTS`)

### Step 2: Review the Implementation

Execute [amber-review](../../../skills/review/amber-review/SKILL.md) against `$LINKED_PR`. Review in the
context of `$ISSUE_CONTENTS`.

Review must be rejected for any of the following reasons:
- Implementation review has findings
- Failing CI
- Merge conflicts
- Needs rebase

Verdict must be one of `approved` or `rejected`.

Output: (`$VERDICT`, `$REJECTION_RATIONALE` (NULL or string))


### Step 3: Write the Result

Write the result object to `$FULLSEND_OUTPUT_DIR/agent-result.json`:
- `outcome`: `approved` or `rejected`.
- `pr_number`: the PR number. `head_sha`: the exact PR head commit you reviewed.
- `rationale`: a short summary of the verdict.
- `comment` (REQUIRED for both verdicts): the full review in
  [simplified technical english](https://en.wikipedia.org/wiki/Simplified_Technical_English).
  An approved review MUST NOT be silent. The comment MUST state the verdict, a
  substantive rationale (the strengths of the implementation, how it meets the
  intent in `$ISSUE_CONTENTS`, and any non-blocking observations), and, for a
  rejected verdict, the blocking reasons (`$REJECTION_RATIONALE`).

Do not apply, remove, or create any label, do not count prior rejections, and do
not decide blocking. The trusted post-script reads this result, rejects it as
stale if the PR head changed since the review started, applies the verdict
transition (`agent/review-code-approved` only when the required checks are green,
or `agent/review-code-rejected`, each removing the competing code-phase labels),
adds `agent/blocked` when the timeline shows this is the tenth or later rejection
since the last unblock, posts your `comment`, and verifies the final state. An
approved PR is ready to merge; a human must perform the merge.
