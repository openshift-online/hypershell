---
name: jira-reconciliation
description: >
  Jira Reconciliation Workflow. Reflects release-verified work into Jira:
  projects canonically linked items to Done and retroactively creates Jira
  tasks and bugs for completed work that was never Jira-driven, then
  classifies the sync as synced or blocked.
---

# Workflow

Jira reconciliation workflow that classifies a release-verified work item as:
- `agent/jira-synced`
- `agent/jira-sync-blocked`

The sync is **synced** when every completed work unit of the verified item was
reflected into Jira: canonically linked units were projected to the `Done`
category, unlinked units had a Jira task or bug created retroactively, and no
open question remains.

The sync is **blocked** when the workflow cannot complete the reflection: a
work unit is ambiguous, a Jira write fails, or a finding exceeds this
workflow's scope. `blocked` means "needs a human", not "bad".

This workflow is the retroactive counterpart of `ingest`: intake links a work
item to Jira at funnel entry, and this workflow links completed work at
delivery. It captures the work that was never Jira-driven - a bugfix or task
that just got done without planning - and makes sure it is reflected in Jira.
It realizes the Retroactive Jira Reflection requirement of
`specs/standards/sdlc/jira-github-sync.spec.md`.

## Input

```text
$GITHUB_ISSUE_URL $JIRA_PROJECT [--dry-run]
```

Supported arguments:
- GITHUB_ISSUE_URL -- the release-verified issue whose completed delivery is
  reflected into Jira.
- JIRA_PROJECT -- the Jira project key (eg `HYPERSHELL`).
- --dry-run -- [Optional] Execute the workflow, but skip every WRITE action
  (Jira creates, transitions, comments, remote links, GitHub labels, GitHub
  comments). All read and verify actions still run.

Reach Jira through the workspace's configured Jira provider binding, using the
Atlassian CLI (`acli`) or the Jira REST API.

## Non-negotiable rules

1. **Mainline is the delivery boundary.** Work is completed only when the
   relevant commit is reachable from `origin/main`. A branch name, an open
   pull request, or a commit that is not an ancestor of main is not proof of
   delivery. Verify ancestry explicitly, for example:
   `git merge-base --is-ancestor <commit> origin/main`.
2. **GitHub references are not Jira keys.** A commit message or pull request
   containing `#158` is not evidence for `HYPERSHELL-158`.
3. **Canonical-link-only resolution.** Resolve a work unit's Jira counterpart
   ONLY through the canonical link (the `agent/execution-record` marker plus
   the exact Jira key recorded for the link) or through a Jira remote link
   that points at the unit's GitHub artifact. A descriptive mention of a Jira
   key never resolves a counterpart.
4. **Only task and bug issues are created or transitioned.** Never create or
   transition a story, epic, or any other issue type. When a finding needs a
   broader type or roadmap-level change, stage it and block.
5. **Jira status is derived, never hand-set.** Project status by resolving the
   mapped status category (`Done`) through the item's available transitions.
   If no transition targets the category, report an error; never pick an
   arbitrary status.
6. **Idempotency: search before create.** Never create a Jira item for a work
   unit that a canonical link or remote link already covers. A second run over
   reflected work makes zero writes.
7. **Never claim an unwritten change.** Confirm every Jira write by re-reading
   the issue afterwards. If the provider fails, report the exact failure.
8. **Credit preservation.** Never set the Jira assignee and never claim
   authorship in any Jira field or comment.
9. **No credentials in artifacts.** Do not write tokens, cookies, passwords,
   or authenticated URLs to comments or evidence.
10. **Do not overclaim.** When Jira access, repository history, or delivery
    evidence is unavailable, say so and block.

## Workflow

_Note: If a label does not exist, create it._

### Step 1: Read the Issue and Confirm Release Verification

Using the GitHub API, fetch `$GITHUB_ISSUE_URL`. Read the title and body and
identify the linked pull requests and merged commits.

Confirm the issue carries the `agent/release-verified` label. If it does not,
`block` and STOP.

Output: (`$LINKED_CHANGES`, `$ISSUE_CONTENTS`)

### Step 2: Establish the Delivery Boundary

For the repository of each linked change, record the mainline commit
(`origin/main`). For every commit claimed as delivery, verify mainline
ancestry:

```shell
git -C <repository> merge-base --is-ancestor <commit> origin/main
```

Build an evidence ledger. Label each change `merged mainline`, `branch-only`,
or `absent`. Only `merged mainline` evidence may support a Jira write. Record
branch-only evidence and report it in the outcome, but never reflect a Jira
item for work whose acceptance depends on it.

Output: `$EVIDENCE_LEDGER`

### Step 3: Identify Work Units and Resolve Counterparts

A **work unit** is the verified issue itself when it is the delivered work, or
each merged change linked to it. For each work unit:

1. Resolve the canonical link per rule 3: the `agent/execution-record` marker
   plus the exact Jira key recorded in the GitHub issue body, or a Jira remote
   link pointing at the unit's GitHub issue or pull request.
2. Search Jira for an existing item that remote-links to the unit's GitHub
   artifacts (issue URL, pull request URL). This is the idempotency search.

Classify each unit:
- `linked`: a canonical counterpart resolves. Record the key, issue type,
  status, and status category.
- `unlinked`: no canonical counterpart and no existing Jira item references
  the unit. This is retroactive-capture territory.
- `ambiguous`: a descriptive key mention suggests related Jira work but no
  canonical link resolves; an existing Jira item partially matches the unit;
  the unit bundles several pieces of work with no clear split; or the unit's
  scope or intent cannot be determined from the merged change.

Output: `$WORK_UNITS` (each with classification and evidence)

### Step 4: Reflect Each Work Unit

#### Linked units: terminal projection

1. Read the item's current status and its `statusCategory`. IF it is already
   in the `Done` category, make no transition.
2. ELSE discover the available transitions and select one whose target status
   has the `Done` status category. IF several qualify, pick the project's
   canonical (first) status in that category. IF none qualify, report an
   error and `block`.
3. Ensure the artifact remote links (update-or-create, never duplicate): the
   pull request and the merge commit.
4. Post a Jira comment, in
   [simplified technical english](https://en.wikipedia.org/wiki/Simplified_Technical_English),
   stating that release verification passed, linking the pull request and
   naming the merge commit. Phrase it on behalf of the human owner; claim no
   authorship.
5. Re-read the issue and confirm the status and links.

#### Unlinked units: retroactive capture

1. Infer the issue type conservatively: `Bug` when the change is a fix
   (conventional commit `fix:`, a `Fixes` reference, regression or crash
   language); `Task` otherwise. Never infer a story or epic.
2. Compose the summary from the pull request title or issue title: concise and
   factual. Compose the description from the delivery evidence: what changed,
   the pull request URL, the merge commit, and the release verification
   reference. Include no credentials.
3. Create the Jira issue in `$JIRA_PROJECT` with that type, summary, and
   description. Do not set the assignee.
4. Create the canonical link where a GitHub anchor exists:
   - Jira -> GitHub: remote links (update-or-create) on the new Jira item
     pointing at the work unit's GitHub issue and pull request.
   - GitHub -> Jira: add the exact Jira key to the GitHub issue body so later
     funnel runs resolve the counterpart canonically.
5. Project the new item to the `Done` category per the linked-unit steps above.
6. Post a Jira comment noting the item was created retroactively from
   delivered work, with the delivery evidence.
7. Re-read the issue and confirm the type, status, and links.

IF the work unit is a single coherent change with a clear type and summary,
the capture is unambiguous: perform it. Otherwise treat the unit as
`ambiguous`.

#### Ambiguous units: stage, do not write

Do not write to Jira. Prepare a staged proposal: for a would-be creation, the
exact proposed type, summary, and description with the delivery evidence; for
a would-be projection, an exact old-value/new-value table:

```text
| Field | Old value | New value | Action |
```

Include the evidence source and the specific question that needs a human
decision.

### Step 5: Communicate Outcome

Determine `$VERDICT`:
- `agent/jira-synced` -- at least one work unit was reflected, every unit was
  unambiguous, and every write succeeded.
- `agent/jira-sync-blocked` -- any unit was ambiguous, a Jira write failed, or
  a finding exceeded this workflow's scope.

Apply `$VERDICT` to `$GITHUB_ISSUE_URL`.

Remove the `agent/release-verified` label from `$GITHUB_ISSUE_URL` so the
coordinator does not re-run this workflow for the issue. A human re-applies
`agent/release-verified` to retry after resolving a blocker.

Add a comment to `$GITHUB_ISSUE_URL` summarizing, in simplified technical
english:
- each Jira item created retroactively, with its key, type, and status;
- each linked item projected, with its old and new status;
- each staged proposal for ambiguous units, with the question that needs a
  decision;
- findings that belong to the interactive `jira-roadmap-reconciliation` skill
  (rank, epic, summary, duplicate, or ownership concerns), recorded but not
  acted on.

## Blocking

When a step directs you to `block`:
- apply label `agent/jira-sync-blocked` to `$GITHUB_ISSUE_URL`;
- remove the `agent/release-verified` label from `$GITHUB_ISSUE_URL`;
- add a comment to `$GITHUB_ISSUE_URL` noting the reason (ambiguity, access
  failure, or scope), in
  [simplified technical english](https://en.wikipedia.org/wiki/Simplified_Technical_English);
- STOP. No further action.

Under `--dry-run`, skip the labels, the label removal, the comments, and every
Jira write, but still STOP. Report each would-be write.
