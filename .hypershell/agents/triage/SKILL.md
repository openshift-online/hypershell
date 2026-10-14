---
name: triage
description: >
  GitHub Issue Triage Workflow.
---

# Workflow

GitHub issue triage workflow that classifies issues as:
- `agent/needs-input`
- `agent/workable`
- `agent/duplicate`

## Input

```text
$GITHUB_REPO_URL $LABEL_SCOPE [--dry-run]
```

Supported arguments:
- GITHUB_REPO_URL -- execute triage against the provided GitHub repository.
- LABEL_SCOPE -- Required: A GitHub label used to filter applicable issues.
- --dry-run -- [Optional] Execute the workflow, but without any WRITE actions.

## Workflow

### Step 1: Read Issues

Using the GitHub API, fetch the issues in `$GITHUB_REPO_URL` that have the
labels `agent/allowed` AND `$LABEL_SCOPE`. You MUST NOT read any issue that does not have the `agent/allowed` or `$LABEL_SCOPE` labels.

Exclude issues that are already in flight. An issue that already carries any
`agent/*` phase label (for example `agent/workable`, `agent/reviewable-spec`,
`agent/review-spec-approved`, `agent/review-spec-rejected`,
`agent/reviewable-code`, `agent/review-code-rejected`, `agent/review-code-approved`,
`agent/blocked`, `agent/needs-input`, `agent/duplicate`) has already been
triaged. Triage SHALL NOT re-read or re-classify it.

Output: `$LIST_OF_ISSUES`

### Step 2: Classify Issues

According to the criteria listed below, determine the classification each issue in `$LIST_OF_ISSUES`.
It MUST be one of the following:

- `agent/duplicate`
Definition: This issue is duplicated by an issue in `$LIST_OF_ISSUES` and contains equal or lesser clarity of intent. This issue will be closed.

- `agent/needs-input`
Definition: This issue requires additional human input due to ambiguous intent,
conflicting intent [with other issues], or the issue appears to be misaligned with the overall project direction.

- `agent/workable`
Definition: This issue contains clear intent, does not conflict with other issues, and is aligned with the overall project direction.


Output: `$ISSUE_TO_CLASSIFICATION_MAP`

### Step 3: Map Actions to Classification

For each issue in `$ISSUE_TO_CLASSIFICATION_MAP`, execute actions according to the label:

_Note: If the label does not exist, create it._

- `agent/duplicate`
Action: Label the issue as `agent/duplicate`. Then, close the issue.

- `agent/needs-input`
Action: Label the issue as `agent/needs-input`.

- `agent/workable`
Action: Label the issue as `agent/workable`.
