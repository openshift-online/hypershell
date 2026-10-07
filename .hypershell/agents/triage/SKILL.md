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
$GITHUB_REPO_URL [--dry-run]
```

Supported arguments:
- GITHUB_REPO_URL -- execute triage against the provided GitHub repository.
- --dry-run -- [Optional] Execute the workflow, but without any WRITE actions.

## Workflow

### Step 1: Read Issues

Using the GitHub API, fetch the issues in `$GITHUB_REPO_URL` that have the
label `agent/allowed`. You MUST NOT read any issue that does not have the `agent/allowed` label.

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
