---
name: spec-implementation
description: >
  GitHub Issue to Specification Workflow
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
  This linkage is load-bearing: `review-response` reads it to find the issue, and the
  fleet dashboard uses it to attribute shipped work to a release bundle. A work item
  may be delivered by several PRs; each such PR MUST carry the closing reference.
- PR Body must conform to [SIMPLIFIED TECHNICAL ENGLISH STANDARD](https://en.wikipedia.org/wiki/Simplified_Technical_English).

### Step 4: Update Issue State

Remove label `agent/review-spec-rejected` from `$GITHUB_ISSUE_URL` if present.
Place label `agent/reviewable-spec` on `$GITHUB_ISSUE_URL`.
