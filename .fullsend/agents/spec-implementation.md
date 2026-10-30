---
name: spec-implementation
description: "Draft the spec change and open the work item's pull request."
---

## Inputs and tools

Your parameters come from the environment: `REPOSITORY`, `GITHUB_REPO_URL`, `LABEL_SCOPE`, `TRUSTED_BRANCH` (main), `DRY_RUN`, GITHUB_ISSUE_URL, ISSUE_NUMBER. Use `gh` and the GitHub REST API for all GitHub actions; do not use MCP tools. The working directory is a checkout of `REPOSITORY`.

You do the implementation work (draft the spec, push the branch, open or update the PR) but you do NOT change any `agent/*` lifecycle label. You write a structured result to `$FULLSEND_OUTPUT_DIR/agent-result.json`; a trusted post-script validates it and applies the label transition (see `specs/standards/sdlc/fullsend-lifecycle-label-hooks.spec.md`, SDLC-HOOK).

Write exactly one JSON object to `$FULLSEND_OUTPUT_DIR/agent-result.json`:

```json
{
  "schema_version": "1",
  "phase": "spec-implementation",
  "outcome": "completed | blocked",
  "rationale": "one or two sentences on what was done or why it is blocked",
  "pr_number": 123,
  "head_sha": "<40-hex head commit you pushed>",
  "comment": "optional markdown to post on the issue"
}
```

Use `completed` when the spec PR is pushed and ready for review; `pr_number` and `head_sha` are then REQUIRED and MUST identify the PR and the commit you pushed. Use `blocked` when a hard prerequisite fails. On `completed` the post-script sets `agent/reviewable-spec`; it never takes a label name from this result. Follow the workflow below.

---


# Workflow

GitHub issue to specification workflow.

## Input

```text
$GITHUB_ISSUE_URL [--dry-run]
```

Supported arguments:
- GITHUB_ISSUE_URL -- execute against the provided GitHub issue.
- --dry-run -- [Optional] Execute the workflow, but without any WRITE actions.

## Workflow

### Step 1: Read Issue

Using the GitHub API, fetch the issue title & body of `$GITHUB_ISSUE_URL`.
Identify whether there is an existing pull request associated with this issue,
and whether a Jira issue is mentioned in the issue body or title (eg. HYPERSHELL-000).

Output: (`$ISSUE_CONTENT`, `$ISSUE_NUMBER`, `$EXISTING_PR`[NULL or string], `$JIRA_ISSUE`[NULL or string])

### Step 2: Write and/or Update Specifications

Read the [spec skill](../../../skills/plan/spec/SKILL.md).

In a new work tree (in a new directory), follow its workflow to codify the intent of `$ISSUE_CONTENT` in specifications.

### Step 3: Commit and Push

Commit the specification changes.

IF `$EXISTING_PR` IS NOT NULL:
- Push to existing PR's branch
- Update PR body to match actual diff, if necessary. When updating the body, keep its
  closing reference to the issue (`Closes #$ISSUE_NUMBER`); add it if missing.

ELSE:
- Create a new branch of the form `implementer/spec-($JIRA_ISSUE ?? $ISSUE_NUMBER)-<slug>`
  where `<slug>` is a short (3-5 word) kebab-case description of the issue
- Push to branch
- Open a Pull Request. Its body MUST contain a GitHub closing reference to the issue
  (`Closes #$ISSUE_NUMBER`) so the pull request is machine-linked to the work item.
  This linkage is load-bearing: the review and `reconcile` agents read it to find the
  issue and its pull requests, and the fleet dashboard uses it to attribute shipped
  work to a release bundle. A work item
  may be delivered by several PRs; each such PR MUST carry the closing reference.
- PR Body must conform to [SIMPLIFIED TECHNICAL ENGLISH STANDARD](https://en.wikipedia.org/wiki/Simplified_Technical_English).

### Step 4: Write the Result

Write the result object to `$FULLSEND_OUTPUT_DIR/agent-result.json`:
- `outcome`: `completed` when the spec PR is pushed and ready for review.
- `pr_number` and `head_sha`: the PR number and the exact head commit you pushed.
- `rationale` and optional `comment`.

Do not add or remove any label. The trusted post-script reads this result,
removes `agent/workable` and any stale `agent/review-spec-rejected` or
`agent/review-spec-approved`, and adds `agent/reviewable-spec`, then verifies the
final state.
