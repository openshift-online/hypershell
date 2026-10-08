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
- Update PR body to match actual diff, if necessary.

ELSE:
- Create a new branch of the form `implementer/spec-($JIRA_ISSUE ?? $ISSUE_NUMBER)`
- Push to branch
- Open Pull Request, link it to the issue
- PR Body must conform to [SIMPLIFIED TECHNICAL ENGLISH STANDARD](https://en.wikipedia.org/wiki/Simplified_Technical_English).

### Step 4: Update Issue State

Place label `agent/reviewable-spec` on `$GITHUB_ISSUE_URL`.
