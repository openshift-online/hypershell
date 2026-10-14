---
name: ingest
description: >
  Bidirectional, origin-agnostic intake. Ensures every in-scope work item of
  EITHER origin ends as a canonically linked Jira-and-GitHub pair, with the
  GitHub issue as the execution record. Create-if-missing and idempotent.
---

# Workflow

Intake responsibility for the SDLC funnel. It makes the funnel origin-agnostic
and bidirectional: a Jira epic or story gets a GitHub execution record created
for it, and a GitHub-origin issue in scope gets a Jira epic created for it, so
both origins converge on the same canonically linked pair. In every case the
GitHub issue is the execution record that the rest of the funnel (triage, spec,
code, review) acts on unchanged. This responsibility is invoked by `execute-sdlc`
Step 0, before triage, so every later phase operates uniformly on GitHub
execution records of either origin.

Realizes the `Origin-Agnostic Intake`, `Canonical Identity Link`,
`Canonical-Link-Only Resolution`, and `ingest becomes bidirectional and
origin-agnostic` requirements of
`specs/standards/sdlc/jira-github-sync.spec.md`.

## Input

```text
$LABEL_SCOPE [$SOURCE (Optional)] [--dry-run]
```

Supported arguments:
- LABEL_SCOPE -- Required. The funnel scope selector (eg. `agent/scope/mturansk`).
  It is matched against GitHub issue labels AND Jira item labels, so the same
  scope selects work from either origin. Every execution record this
  responsibility creates carries this label.
- SOURCE -- [Optional] A narrower selector for a single-item run. It may be:
  - A Jira source: a Jira label, a JQL query, an epic key (eg. `HYPERSHELL-374`),
    or a combination. The in-scope Jira set is narrowed to it and the in-scope
    GitHub set is empty (the GitHub counterpart is created Jira-first).
  - A single GitHub issue (URL or number). The in-scope GitHub set is just that
    issue and the in-scope Jira set is empty (the Jira counterpart is created
    GitHub-first).
  When omitted (a scope run): the in-scope Jira set is the Jira items carrying
  `$LABEL_SCOPE` as a Jira label, AND the in-scope GitHub set is the GitHub
  issues carrying `$LABEL_SCOPE`.
- --dry-run -- [Optional] Execute the workflow, but without any WRITE actions.
  Every would-be write MUST be reported and none performed.

## Canonical-Link-Only Resolution

A GitHub execution record's Jira counterpart (and a Jira item's execution record)
SHALL be resolved ONLY through the **canonical link**: the `agent/execution-record`
marker label on the GitHub issue PLUS the exact Jira key recorded in that issue's
body for the purpose of the link. Resolution REQUIRES the marker: a GitHub issue
without `agent/execution-record` is NOT an established execution record and
resolves no Jira item.

A descriptive mention of a Jira key in an issue body - a key named to describe
related or prior work (for example a GitHub-origin issue that mentions
`HYPERSHELL-374` where that key canonically belongs to a different execution
record) - SHALL NEVER resolve that Jira item. Never infer a link from such a
mention. When no canonical link resolves a counterpart, the GitHub issue is a
valid GitHub-origin item with no Jira item yet, which the GitHub-first path below
ingests.

## Workflow

### Step 1: Resolve the in-scope set from BOTH origins

Resolve the work to intake from BOTH a Jira source AND GitHub issues carrying the
scope label, so GitHub-origin records are discovered, not only Jira-ingested ones.

Resolve `$JIRA_ITEMS`, the in-scope Jira items:

- IF `$SOURCE` is an epic key (eg. `HYPERSHELL-374`): the item is that epic. (Do
  NOT auto-expand the epic into its children; only items the source directly
  selects are in scope.)
- IF `$SOURCE` is a JQL query: the items are the query result.
- IF `$SOURCE` is a Jira label: the items are those carrying that label.
- IF `$SOURCE` is a single GitHub issue: `$JIRA_ITEMS` is EMPTY (this is a
  GitHub-first single-item run).
- IF `$SOURCE` is omitted: the items are those carrying `$LABEL_SCOPE` as a Jira
  label.

Resolve `$GITHUB_ITEMS`, the in-scope GitHub issues:

- IF `$SOURCE` is a single GitHub issue: `$GITHUB_ITEMS` is just that issue.
- IF `$SOURCE` is a Jira source: `$GITHUB_ITEMS` is EMPTY (this is a Jira-first
  single-item run).
- IF `$SOURCE` is omitted: `$GITHUB_ITEMS` is every GitHub issue (open AND
  closed) in `openshift-online/hypershell` carrying `$LABEL_SCOPE`. Discover open
  AND closed so an already-merged-but-closed GitHub-origin record is still linked
  and reconciled (symmetric with the Jira-first search in Step 2, which spans open
  AND closed). An in-scope GitHub issue that already carries the
  `agent/execution-record` marker is an established execution record; one carrying
  the scope label but NOT the marker is a GitHub-origin candidate the GitHub-first
  path ingests (Step 3), applying the marker as part of the canonical link.

You MUST NOT read Jira items that are outside the configured Jira source.

Output: `$JIRA_ITEMS`, `$GITHUB_ITEMS`

### Step 2: Jira-first - resolve or create an execution record for each Jira item

For each `$JIRA_ITEM` in `$JIRA_ITEMS`, ensure exactly one linked GitHub
execution record. This step is create-if-missing and IDEMPOTENT: running it again
on an already-linked item makes no change and returns the existing record.

1. Search `openshift-online/hypershell` issues (open AND closed) for the
   execution record of `$JIRA_ITEM`, using Canonical-Link-Only Resolution: a
   match REQUIRES both the `agent/execution-record` marker label AND the exact
   Jira key present in the issue body as the canonical link. A descriptive key
   mention is NOT a match. The exact-key-plus-marker match is the idempotency key
   and prevents duplicates.
2. IF exactly one match: reuse it. `$EXECUTION_RECORD` = that issue. Create
   nothing. Proceed to Step 4 to ensure the link.
3. IF more than one match: the state is ambiguous. DO NOT guess. DO NOT create
   another record. STOP and report an error that names the work item:
   "ERROR: multiple execution records for <JIRA_KEY>. A human must resolve the
   duplicate records." Collect the error and continue to the next item rather
   than silently skipping; the run MUST report every such item.
4. IF no match:
   - IF `--dry-run`: report the would-be creation ("WOULD create execution
     record for <JIRA_KEY>") and continue. Perform no write.
   - ELSE create ONE GitHub issue as the execution record:
     - Title: `<JIRA_KEY>: <jira summary>`
     - Body: a link to `https://redhat.atlassian.net/browse/<JIRA_KEY>`, the
       Jira key on its own line (for exact matching), and the Jira item intent
       (description and acceptance criteria copied from the Jira item, which is
       the authority for intent).
     - Labels: `agent/execution-record`, `agent/allowed`, and `$LABEL_SCOPE`
       (create any label that does not exist first).
   - `$EXECUTION_RECORD` = the new issue.

Intake SHALL resolve exactly one execution record per work item. A GitHub-origin
item is its own execution record and SHALL NOT receive a second one.

### Step 3: GitHub-first - create a Jira epic for each unlinked GitHub item

For each `$GITHUB_ITEM` in `$GITHUB_ITEMS`, ensure it has a canonically linked
Jira counterpart. The GitHub issue IS and REMAINS the execution record; the Jira
epic created here serves only as the management-view counterpart. This step is a
RECONCILE (update-or-create), not create-and-ignore: it is create-if-missing and
IDEMPOTENT, and it NEVER creates a duplicate Jira epic even if an earlier run
partially completed.

1. Resolve the Jira counterpart of `$GITHUB_ITEM` using Canonical-Link-Only
   Resolution: it is linked ONLY if `$GITHUB_ITEM` carries the
   `agent/execution-record` marker AND its body records an exact Jira key as the
   canonical link. A descriptive key mention does NOT count.
2. IF a canonical link already resolves a Jira item: the pair already exists.
   Create nothing. `$EXECUTION_RECORD` = `$GITHUB_ITEM`. Proceed to Step 4 to
   ensure both directions of the link are present (idempotent, never duplicated).
3. ELSE, before creating anything, check for an ORPHANED epic from a prior partial
   run (idempotency guard symmetric to Step 2.1). A GitHub-first run creates the
   epic first and writes the three link parts after, so a run that died in between
   can leave an epic whose only surviving link part is its Jira-to-GitHub remote
   link. Search Jira for an epic that already carries a Jira-to-GitHub remote link
   pointing at `$GITHUB_ITEM`'s URL:
   - IF exactly one such epic exists: REUSE it. Create no new epic. `$JIRA_EPIC` =
     that epic. Backfill only the missing canonical-link parts in Step 3.5
     (update-or-create). This is what prevents a duplicate epic on re-run.
   - IF more than one such epic exists: the state is ambiguous. DO NOT guess, DO
     NOT create another epic. Collect an error that names `$GITHUB_ITEM` ("ERROR:
     multiple Jira epics remote-linked to GitHub #<N>. A human must resolve the
     duplicates.") and continue to the next item.
   - IF none exists: there is no counterpart yet; create one in Step 3.4.
4. IF no linked and no orphaned epic was found (a GitHub-origin item with no Jira
   counterpart yet):
   - IF `--dry-run`: report the would-be creation ("WOULD create Jira epic for
     GitHub #<N> and link it") and continue. Perform no write.
   - ELSE create ONE Jira epic for `$GITHUB_ITEM`:
     - Summary: the GitHub issue title.
     - Description: the GitHub issue intent (Jira becomes the authority for intent
       once linked; copy the issue body intent across). Do NOT set the Jira status
       by hand (status is derived by `reconcile`). Do NOT set an agent as the
       assignee (credit is preserved for the human owner).
     - `$JIRA_EPIC` = the new epic.
5. Establish (or backfill) the three-part canonical link for `$JIRA_EPIC`,
   update-or-create and never duplicated. Under `--dry-run`, report each would-be
   write and perform none:
   - Write the EXACT Jira key into `$GITHUB_ITEM`'s body on its own line (for
     exact matching), if not already present.
   - Add the `agent/execution-record` marker label to `$GITHUB_ITEM` (create the
     label if it does not exist), if not already present. This is WHEN a
     GitHub-origin issue acquires the marker: ingest applies it during this
     GitHub-first path, as part of establishing the canonical link.
   - Add a Jira-to-GitHub remote link on `$JIRA_EPIC` pointing at `$GITHUB_ITEM`'s
     URL, if not already present.
   `$EXECUTION_RECORD` = `$GITHUB_ITEM` (unchanged; no second GitHub record is
   ever created for a GitHub-origin item).

Collect every failed GitHub-first write (epic create, Jira-key write, marker add,
remote-link write) and surface it in Step 5; a partial failure SHALL NOT be
silently swallowed.

### Step 4: Ensure the bidirectional canonical link

For every `$EXECUTION_RECORD` resolved in Step 2 or Step 3, ensure the canonical
link exists in BOTH directions (create-if-missing, update-or-create, never
duplicated):

- GitHub -> Jira: the execution record body contains the exact Jira key AND the
  record carries the `agent/execution-record` marker label. If a reused record is
  missing either, add it.
- Jira -> GitHub: the Jira item carries a remote link that points at the
  execution record issue URL. If the link is absent, create it once; if present,
  leave it unchanged.

The link can originate from either side, and its three parts (exact key in the
GitHub body, the `agent/execution-record` marker on the GitHub issue, and the
Jira-to-GitHub remote link) MUST all agree.

Under `--dry-run`, report each would-be link write and perform none.

### Step 5: Report

Output the set of resolved execution records (`$EXECUTION_RECORDS`) of either
origin, noting for each whether it was reused or newly created, and whether its
Jira counterpart was reused or newly created. Surface EVERY collected error and
partial failure: the ambiguity errors from Step 2.3 and Step 3.3, and any failed
GitHub-first write from Step 3 (epic create, Jira-key write, marker add,
remote-link write). If any error was collected, the run MUST surface it; partial
failures SHALL NOT be silently swallowed.

Output: `$EXECUTION_RECORDS`
