---
name: spec-review
description: "Review the spec changes on the work item's pull request and record a verdict."
---

## Inputs and tools

Your parameters come from the environment: `REPOSITORY`, `GITHUB_REPO_URL`, `LABEL_SCOPE`, `TRUSTED_BRANCH` (main), `DRY_RUN`, GITHUB_ISSUE_URL, ISSUE_NUMBER, GITHUB_PR_URL, PR_NUMBER. Use `gh` and the GitHub REST API for read-only GitHub actions; do not use MCP tools. The working directory is a checkout of `REPOSITORY`.

You reach a verdict but you do NOT change any `agent/*` lifecycle label and you do NOT count prior rejections or apply a block. You write a structured result to `$FULLSEND_OUTPUT_DIR/agent-result.json`; a trusted post-script validates it, rejects it as stale if the PR head moved, counts prior rejections from the issue timeline to decide blocking, applies the verdict transition, posts your comment, and verifies the final state (see `specs/standards/sdlc/fullsend-lifecycle-label-hooks.spec.md`, SDLC-HOOK).

Write exactly one JSON object to `$FULLSEND_OUTPUT_DIR/agent-result.json`:

```json
{
  "schema_version": "1",
  "phase": "spec-review",
  "outcome": "approved | rejected",
  "rationale": "short summary of the verdict",
  "pr_number": 123,
  "head_sha": "<40-hex head commit you reviewed>",
  "comment": "the full review in simplified technical english (required)"
}
```

`head_sha` MUST be the PR head commit you actually reviewed, so the post-script
can detect a stale review. `comment` is REQUIRED for both verdicts. Follow the
workflow below.

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

Output: `$REVIEW_VERDICT` - One of `approved` or `rejected`.

### Step 3: Write the Result

Write the result object to `$FULLSEND_OUTPUT_DIR/agent-result.json`:
- `outcome`: `approved` or `rejected`.
- `pr_number`: the PR number. `head_sha`: the exact PR head commit you reviewed.
- `rationale`: a short summary of the verdict.
- `comment` (REQUIRED for both verdicts): the full review in
  [simplified technical english](https://en.wikipedia.org/wiki/Simplified_Technical_English).
  An approved review MUST NOT be silent. The comment MUST state the verdict, a
  substantive rationale (the strengths of the specification, how it meets the
  intent in `$GITHUB_ISSUE_URL`, and any non-blocking observations), and, for a
  rejected verdict, the blocking reasons.

Do not apply, remove, or create any label, do not count prior rejections, and do
not decide blocking. The trusted post-script reads this result, rejects it as
stale if the PR head changed since the review started, applies the verdict
transition (`agent/review-spec-approved` or `agent/review-spec-rejected`, each
removing the competing spec-phase labels), adds `agent/blocked` when the
timeline shows this is the third or later rejection since the last unblock, posts
your `comment`, and verifies the final state.
