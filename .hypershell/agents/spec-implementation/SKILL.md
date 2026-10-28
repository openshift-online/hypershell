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

**Whole-tree coherence (reconcile every sibling spec).** When the change
establishes, supersedes, or alters a cross-cutting decision - an architectural
choice, an ownership rule, or any `SHALL`/`SHALL NOT` other specs rely on - you
MUST reconcile EVERY affected spec across the ENTIRE `specs/` tree, not only the
file or directory you set out to edit. Before finishing:

- Grep ALL of `specs/` (every subtree, explicitly including `specs/standards/`,
  not just `specs/platform/`) for the decision's key terms AND the superseded
  phrasing it replaces.
- Inspect scenario titles and bodies (GIVEN/WHEN/THEN), not only prose: a
  normative scenario in a standard can still assert the old contract even when the
  prose is updated.
- Reconcile every hit so the whole tree states one coherent model. A single
  contradiction left anywhere in `specs/` is an incomplete change and will be
  rejected. Record the grep audit (terms searched + why each remaining hit is
  coherent) in the pull request.

### Step 3: Commit and Push

Commit the specification changes as a single `docs(spec): ...` commit. This is the
FIRST commit of the work item's one funnel pull request (one PR carries both the
spec and code phases; see "Pull request and commit model" in
`../../execute-sdlc/SKILL.md`). The code phase later adds per-component commits to
this same PR, so do not open or expect a separate code PR.

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

### Step 4: Update Issue State

Advance the phase on `$GITHUB_ISSUE_URL`:
- Place label `agent/reviewable-spec`.
- Remove the prior PHASE labels this transition supersedes, if present:
  `agent/workable` (the phase that triggered this step) and
  `agent/review-spec-rejected` (a prior spec-review verdict). Removing a label
  that is already absent is a no-op.

**Protected labels (never remove).** `agent/review-spec-blocked` is NOT a phase
label - it is an OPTIONAL human gate a person applies to force a human spec
sign-off before any code is written. NEVER remove it in this step, even while
normalizing phase state; removing it silently destroys a human's intent to gate
the spec-to-code transition. The role markers are likewise NOT phase labels and
MUST NOT be removed here: `agent/allowed` (authorization gate), `agent/scope/*`
(scope selector), and `agent/execution-record` (canonical-link marker). This step
removes ONLY the prior phase labels named above - it is not a license to remove
everything but one label.
