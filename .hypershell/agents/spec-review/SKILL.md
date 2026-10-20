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

**Whole-tree coherence check (mandatory, do NOT scope to the diff).** When the
change establishes, supersedes, or alters a cross-cutting decision (an
architectural choice, an ownership rule, a `SHALL`/`SHALL NOT` other specs rely
on), verify the ENTIRE `specs/` tree agrees - not only the files in the pull
request and not only `specs/platform/`:

- Grep ALL of `specs/` (every subtree, explicitly including `specs/standards/`)
  for the decision's key terms and for the superseded phrasing it replaces.
- Check scenario titles and bodies (GIVEN/WHEN/THEN), not only prose. A normative
  scenario (`SHALL`/`SHALL NOT`) that still asserts the superseded contract is a
  contradiction even if it lives in a file the pull request never touched.
- Any single spec - anywhere in `specs/`, including a standard - that still states
  the superseded contract is a BLOCKING contradiction: reject with the exact
  `file:line`. Do not treat an unreconciled sibling spec as non-blocking.

Output: `$REVIEW_VERDICT` - One of
- `agent/review-spec-rejected`
- `agent/review-spec-approved`

### Step 3: Update Issue State

_Note: Create `$REVIEW_VERDICT` label if not exist._

Place label `$REVIEW_VERDICT` on `$GITHUB_ISSUE_URL` only.

The verdict labels are mutually exclusive. On `$GITHUB_ISSUE_URL`, remove the
opposite verdict label if present:
- Verdict `agent/review-spec-approved`: remove `agent/review-spec-rejected`.
- Verdict `agent/review-spec-rejected`: remove `agent/review-spec-approved`.

Do NOT place any `agent/*` phase label on `$GITHUB_PR_URL`. The issue is the only
holder of phase state:
- The next agent starts from the issue label and finds the pull request through
  the issue.
- The verdict labels are selector labels. The coordinator scans issues and pull
  requests, so a verdict label on the pull request starts an agent on the pull
  request. No other agent removes labels from the pull request, so the label
  stays and re-triggers the agent on every run.
- The review comment below keeps the verdict visible on the pull request.

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
