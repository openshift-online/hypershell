---
name: issue-triage
description: "Triage and label new issues."
---

## Inputs and tools

Your parameters come from the environment: `REPOSITORY`, `GITHUB_REPO_URL`, `LABEL_SCOPE`, `TRUSTED_BRANCH` (main), `DRY_RUN`, `GITHUB_ISSUE_URL` (the single issue that triggered this run). Use `gh` and the GitHub REST API for read-only GitHub actions; do not use MCP tools. The working directory is a checkout of `REPOSITORY`.

You do not change any label. You classify the triggered issue and write a structured result to `$FULLSEND_OUTPUT_DIR/agent-result.json`. A trusted post-script validates that result and applies the lifecycle label transition (see `specs/standards/sdlc/fullsend-lifecycle-label-hooks.spec.md`, SDLC-HOOK). Do NOT add, remove, or create any `agent/*` lifecycle label, and do NOT close the issue yourself.

Write exactly one JSON object to `$FULLSEND_OUTPUT_DIR/agent-result.json`:

```json
{
  "schema_version": "1",
  "phase": "triage",
  "outcome": "workable | needs-input | duplicate",
  "rationale": "one or two sentences explaining the classification",
  "comment": "optional markdown to post on the issue"
}
```

`outcome` MUST be one of the three permitted values. The post-script applies the label (`agent/workable`, `agent/needs-input`, or `agent/duplicate`) and, for `duplicate`, closes the issue. Follow the workflow below.

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

When `GITHUB_ISSUE_URL` is supplied, read and classify
only that issue. Re-check that it carries both `LABEL_SCOPE` and `agent/allowed`.
Do not scan or modify other issues.
The scope-wide discovery below applies only when no individual issue is supplied.

Using the GitHub API, fetch the issues in `$GITHUB_REPO_URL` that have the
labels `agent/allowed` AND `$LABEL_SCOPE`. You MUST NOT read any issue that does not have the `agent/allowed` or `$LABEL_SCOPE` labels.

Exclude issues that are already in flight. An issue that already carries any
`agent/*` phase label (for example `agent/workable`, `agent/reviewable-spec`,
`agent/review-spec-approved`, `agent/review-spec-rejected`,
`agent/reviewable-code`, `agent/review-code-rejected`, `agent/review-code-approved`,
`agent/blocked`, `agent/needs-input`, `agent/duplicate`) has already been
triaged. Triage SHALL NOT re-read or re-classify it.

Triage is origin-agnostic. A Jira-origin work item reaches triage as its GitHub
execution record (created by the `ingest` responsibility before triage runs), so
triage classifies it exactly like a GitHub-origin issue. Triage operates only on
GitHub issues and SHALL NOT read Jira items directly, including any Jira item
outside the configured Jira source.

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

### Step 3: Write the Result

Determine the classification of the triggered issue `$GITHUB_ISSUE_URL` (the
other issues provide context for duplicate detection only).

Write the result object to `$FULLSEND_OUTPUT_DIR/agent-result.json` with:
- `outcome`: `workable`, `needs-input`, or `duplicate` for `$GITHUB_ISSUE_URL`.
- `rationale`: why this classification.
- `comment` (optional): any note to post on the issue; for `duplicate`, name
  the issue it duplicates.

Do not apply labels and do not close the issue. The trusted post-script reads
this result, applies `agent/workable`, `agent/needs-input`, or `agent/duplicate`,
and closes the issue on `duplicate`.
