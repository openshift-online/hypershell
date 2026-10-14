---
name: reconcile
description: >
  Bidirectional Jira <-> GitHub reconciliation under a per-field authority rule.
  Projects Jira status from the GitHub phase, syncs artifact remote links, posts
  attributed phase comments, preserves credit, and corrects drift. Idempotent and
  dry-run aware.
---

# Workflow

Keeps a work item's Jira item and its GitHub execution record converged. It is
invoked by `execute-sdlc` after each phase transition and after merge, so a
manager sees progress in Jira without ever editing it by hand.

Realizes the `Status Projection`, `Terminal Execution Record Closure`,
`Artifact Remote Links`, `Narrative Comments`, `Bidirectional Reconciliation`,
`Safety and Approval Gates`, and `Credit Preservation` requirements of
`specs/standards/sdlc/jira-github-sync.spec.md`.

Reconciliation is IDEMPOTENT and drift-correcting: a field that has diverged is
restored to its authority's value; a run with no drift makes no change; a second
pass writes nothing in either system.

## Input

```text
$EXECUTION_RECORD [$PREVIOUS_PHASE (Optional)] [--dry-run]
```

Supported arguments:
- EXECUTION_RECORD -- Required. The GitHub execution record (issue URL or number)
  to reconcile with its linked Jira item.
- PREVIOUS_PHASE -- [Optional] The phase label the record held before the
  transition that triggered this run. When provided and different from the
  current phase, a narrative comment is posted for the transition (Step 4).
- --dry-run -- [Optional] Execute the workflow, but without any WRITE actions.
  Every would-be write MUST be reported and none performed.

Collect every failed write and report it at the end. No partial failure SHALL be
silently swallowed.

## Per-Field Authority

Each field has exactly one authoritative system. The reconciler copies FROM the
authority TO the other system and NEVER writes back toward the authority.

| Field | Authority | Direction |
|-------|-----------|-----------|
| Intent, description, acceptance criteria | Jira | Jira -> GitHub |
| Priority | Jira | Jira -> GitHub |
| Assignee | Jira | Jira -> GitHub (never set by an agent) |
| Epic linkage and hierarchy | Jira | Jira -> GitHub |
| Phase, labels, SDLC state machine | GitHub | not copied to Jira as labels |
| Execution record open/closed | GitHub | closed on terminal merge (Step 3) |
| Jira status | Derived from GitHub phase | GitHub phase -> Jira status |
| Branch, pull requests, commits, specs | GitHub | GitHub -> Jira remote links |

## Workflow

### Step 1: Resolve the canonical link

Resolve the Jira item linked to `$EXECUTION_RECORD`: read the exact Jira key from
the execution record body. IF no Jira key / no canonical link is present, there is
nothing to reconcile; report and STOP without error (a GitHub-origin item with no
Jira counterpart is a valid state).

The **human owner** referenced throughout this responsibility (for comment
attribution in Step 4 and credit preservation in Step 6) is the Jira item's
assignee. Resolve it here as `$HUMAN_OWNER` so attribution is deterministic.

Output: `$JIRA_ITEM`, `$JIRA_KEY`, `$HUMAN_OWNER`

### Step 2: Reconcile Jira-authoritative fields into GitHub

For each Jira-authoritative field (intent/description/acceptance criteria,
priority, epic linkage), compare the Jira value with the GitHub execution record.
IF they diverge, copy the Jira value into the GitHub record and leave Jira
unchanged. IF they already match, make no write.

Do NOT write any of these fields back toward Jira: Jira is the authority.

### Step 3: Project Jira status from the GitHub phase, and close on terminal merge

Read the current GitHub phase (the record's `agent/*` label state) and merge
state. Derive the target Jira status **category** per this mapping. The mapping
targets a category, NOT a hardcoded status name: every Jira project has exactly
three categories - `To Do` (key `new`), `In Progress` (key `indeterminate`), and
`Done` (key `done`) - while the concrete status names (eg. `New`, `Refinement`,
`Closed`) are per-project workflow configuration. Targeting the category keeps
status names out of this skill (the "separate configuration from code"
convention).

| GitHub phase | Derived Jira status category |
|--------------|------------------------------|
| No execution record yet | `To Do` |
| `agent/needs-input` | `To Do` |
| `agent/workable` | `In Progress` |
| `agent/reviewable-spec`, `agent/review-spec-rejected` | `In Progress` |
| `agent/review-spec-approved` | `In Progress` |
| `agent/reviewable-code`, `agent/review-code-rejected` | `In Progress` |
| `agent/review-code-approved` | `In Progress` |
| `agent/blocked` | `In Progress` |
| Pull request merged to `main` | `Done` |
| `agent/duplicate` (issue closed as duplicate) | `Done` |

Resolve the concrete Jira status for the mapped category by category, not by
name:

1. Read the Jira item's CURRENT status and its `statusCategory.key`
   (`new` / `indeterminate` / `done`).
2. IF the current status's category already equals the mapped category: make no
   transition (no-op). The item is already converged.
3. ELSE discover the available transitions
   (`GET /rest/api/3/issue/{key}/transitions`) and select one whose target
   status's `statusCategory` matches the mapped category. IF several qualify,
   pick the project's canonical (first) status in that category.
4. Perform that transition (`POST .../transitions`).
5. IF no transition targets a status in the mapped category (the category is
   unreachable on this workflow): report it as an error. Do NOT silently skip it
   and do NOT hand-set an arbitrary status.

Jira status is DERIVED and SHALL NEVER be hand-set to an arbitrary value.

#### Terminal execution record closure

When the mapped category is `Done` BECAUSE the linked pull request is merged to
`main`, the execution record has reached its terminal state. Converge the whole
stack in this single run: in addition to the Done status projection above and the
merge-commit remote link (Step 5), CLOSE the GitHub execution record issue.

Closure is DERIVED from the merge state and scoped to the merged case ONLY. The
`agent/duplicate` Done path does not close the record here (a duplicate is already
closed as part of triage), so do NOT close the record for the duplicate path:

1. Read the execution record's open/closed state and its linked pull request's
   merge state.
2. IF the linked pull request is merged to `main` AND the execution record is
   still Open: close it (`gh issue close $EXECUTION_RECORD`).
3. IF the linked pull request is merged AND the record is already Closed: make no
   write (no-op).
4. IF the linked pull request is NOT merged: leave the record Open. NEVER close a
   record whose pull request has not merged.

Closure is IDEMPOTENT and drift-correcting: a run over a record whose pull request
has merged but which is still Open closes it, and a re-run changes nothing. Under
`--dry-run`, report the would-be close and perform no write. The record's
open/closed state is GitHub-authoritative; the Jira item is never closed from
here, only projected to the `Done` category. Any failure to close MUST be
collected and reported (Step 7), never silently swallowed.

### Step 4: Narrative comment on phase transition

IF `$PREVIOUS_PHASE` is provided AND differs from the current phase (a real
transition occurred), post a Jira comment describing the transition (eg. "Spec
submitted for review", "Code review approved; pull request ready to merge").

The comment MUST be attributed to the human owner, phrased as acting on behalf of
the owner. An agent SHALL NOT post a comment that claims the agent as the author
of the work. Use plain, factual language.

Make this write-once per transition: if a comment for this exact transition
already exists, do not post a duplicate.

### Step 5: Artifact remote links

Collect the work item's artifacts from GitHub: the implementation branch, each
pull request, and (once merged) the merge commit. For each artifact, ensure a
Jira remote link exists, UPDATE-OR-CREATE:
- IF a remote link for that artifact already exists: update it in place.
- ELSE create it.
- NEVER create a duplicate remote link for the same artifact.

This spec relies on Jira remote links, not the native GitHub Development panel,
to surface branches, pull requests, and commits.

### Step 6: Credit preservation

The Jira assignee SHALL remain the human owner. Verify the reconciler did not,
and does not, set an agent as the Jira assignee, and did not claim authorship in
any Jira field or comment. If the assignee was changed away from the human owner
by a prior erroneous run, this is drift against the Jira authority: do NOT
overwrite it from GitHub; report it for a human to correct, because assignee is
Jira-authoritative.

### Step 7: Safety and reporting

- Under `--dry-run`, every write in Steps 2-5 MUST be reported as a would-be
  write and none performed.
- Report every change made (status transition, execution record closure, field
  copy, comment, remote link) and every collected failure. A pass with no drift
  reports "no changes".
- A second pass over an already-reconciled item MUST make zero writes in either
  system.
