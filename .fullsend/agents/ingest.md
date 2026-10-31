---
name: ingest
description: "Bidirectional Jira <-> GitHub intake. Link every in-scope work item as a canonically linked pair."
---

## Inputs and tools

Your parameters come from the environment: `REPOSITORY`, `GITHUB_REPO_URL`, `LABEL_SCOPE`, `TRUSTED_BRANCH` (main), `DRY_RUN`, `JIRA_BASE_URL`. Use `gh` and the GitHub REST API for all GitHub actions; use `node .fullsend/scripts/jira-request.mjs METHOD /rest/api/3/PATH [request.json]` for Jira requests. The helper handles authentication; do not construct authentication headers or inspect credentials. Paginate discovery and report request failures. Do not use MCP tools. The working directory is a checkout of `REPOSITORY`. Write your structured result to `$FULLSEND_OUTPUT_DIR/result.json`. Follow the workflow below.

---

# Workflow

Intake responsibility for the SDLC funnel. It makes the funnel origin-agnostic
and bidirectional: a Jira epic or story gets a GitHub execution record created
for it, and a GitHub-origin issue in scope gets a Jira epic created for it, so
both origins converge on the same canonically linked pair. In every case the
GitHub issue is the execution record that the rest of the funnel (triage, spec,
code, review) acts on unchanged. Intake precedes triage so every later phase operates uniformly on GitHub
execution records of either origin.

Realizes the `Authorization Gate Is Load-Bearing`, `Origin-Agnostic Intake`,
`Canonical Identity Link`, `Canonical-Link-Only Resolution`, and `ingest becomes
bidirectional and origin-agnostic` requirements of
`specs/standards/sdlc/jira-github-sync.spec.md`.

## The Authorization Gate

`agent/allowed` is the human authorization gate for the ENTIRE funnel, separate
from the `$LABEL_SCOPE` run selector. Scope selects an item for a run;
authorization is a separate, human-controlled decision that the item MAY be acted
on. The two are orthogonal: an item that is scoped but NOT authorized SHALL NOT be
acted on by intake. Intake SHALL NOT create a Jira epic, a GitHub execution
record, or a canonical link for an item that does not carry `agent/allowed`.

Authorization is a human act. `ingest` SHALL NOT apply `agent/allowed` to any
record: an agent that granted the gate to a record it created would defeat the
gate. A GitHub execution record created by `ingest` therefore starts
UN-authorized, carrying only `agent/execution-record` and `$LABEL_SCOPE`, and
stays inert until a human grants `agent/allowed`.

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
The in-scope set is GATED by `agent/allowed` (see The Authorization Gate): an item
is in scope only if it carries `agent/allowed` in addition to the scope selector.
An item that is scoped but not authorized SHALL NOT be in scope for intake.

Resolve `$JIRA_ITEMS`, the in-scope Jira items (each MUST carry the `agent/allowed`
authorization marker as a Jira label; drop any that do not):

- IF `$SOURCE` is an epic key (eg. `HYPERSHELL-374`): the item is that epic, only
  if it carries `agent/allowed`. (Do NOT auto-expand the epic into its children;
  only items the source directly selects are in scope.)
- IF `$SOURCE` is a JQL query: the items are the query result that also carry
  `agent/allowed`.
- IF `$SOURCE` is a Jira label: the items are those carrying that label AND
  `agent/allowed`.
- IF `$SOURCE` is a single GitHub issue: `$JIRA_ITEMS` is EMPTY (this is a
  GitHub-first single-item run).
- IF `$SOURCE` is omitted: the items are those carrying BOTH `$LABEL_SCOPE` AND
  `agent/allowed` as Jira labels.

Resolve `$GITHUB_ITEMS`, the in-scope GitHub issues (each MUST carry
`agent/allowed`; drop any that do not, symmetric with the `triage` contract):

- IF `$SOURCE` is a single GitHub issue: `$GITHUB_ITEMS` is just that issue, only
  if it carries `agent/allowed`.
- IF `$SOURCE` is a Jira source: `$GITHUB_ITEMS` is EMPTY (this is a Jira-first
  single-item run).
- IF `$SOURCE` is omitted: `$GITHUB_ITEMS` is every GitHub issue (open AND
  closed) in `openshift-online/hypershell` carrying BOTH `$LABEL_SCOPE` AND
  `agent/allowed`. Discover open AND closed so an already-merged-but-closed
  GitHub-origin record is still linked and reconciled (symmetric with the
  Jira-first search in Step 2, which spans open AND closed). An in-scope GitHub
  issue that already carries the `agent/execution-record` marker is an established
  execution record; one carrying the scope label and `agent/allowed` but NOT the
  marker is a GitHub-origin candidate the GitHub-first path ingests (Step 3),
  applying the marker as part of the canonical link.

An item that carries the scope selector but not `agent/allowed` is NOT in scope:
intake creates no epic, record, or link for it. Authorization is a human act;
adding `agent/allowed` by hand lets the next run ingest it.

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
     - Labels: `agent/execution-record` and `$LABEL_SCOPE` (create any label that
       does not exist first). Do NOT apply `agent/allowed`: authorization is a
       human act and `ingest` SHALL NOT self-authorize a record it creates (see
       The Authorization Gate). The created record starts un-authorized and stays
       inert until a human grants `agent/allowed`.
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
   epic with `$GITHUB_ITEM`'s URL embedded in the epic description in the SAME
   create call (Step 3.4 item 1), then writes the Jira-to-GitHub remote link (Step
   3.4 item 2), then the GitHub-side key and marker (Step 3.5). Because the GitHub
   URL is set atomically with creation, EVERY epic that exists carries a
   guard-findable backlink from the instant it is created - even if the run died
   before any later write, including the gap between epic creation and the remote
   link. Search Jira for an epic linked to `$GITHUB_ITEM` by EITHER signal: a
   Jira-to-GitHub remote link pointing at `$GITHUB_ITEM`'s URL (prefer this signal
   when both are present), OR `$GITHUB_ITEM`'s exact URL recorded on its own line
   as the backlink in the epic description. Match the backlink by the exact
   URL-on-its-own-line pattern Step 3.4 writes, NOT a loose substring, so an epic
   that merely mentions the URL descriptively does NOT false-match (symmetric with
   Canonical-Link-Only Resolution):
   - IF exactly one such epic exists: REUSE it. Create no new epic. `$JIRA_EPIC` =
     that epic. Backfill only the missing canonical-link parts in Step 3.5 and
     Step 4 (update-or-create). This is what prevents a duplicate epic on re-run.
   - IF more than one such epic exists: the state is ambiguous. DO NOT guess, DO
     NOT create another epic. Collect an error that names `$GITHUB_ITEM` ("ERROR:
     multiple Jira epics linked to GitHub #<N>. A human must resolve the
     duplicates.") and continue to the next item.
   - IF none exists: there is no counterpart yet; create one in Step 3.4.
4. IF no linked and no orphaned epic was found (a GitHub-origin item with no Jira
   counterpart yet), create the epic and its Jira-to-GitHub remote link in this
   EXACT order, so the write order matches the Step 3.3 guard premise and the
   operation is crash-safe and idempotent:
   - IF `--dry-run`: report the would-be creation ("WOULD create Jira epic for
     GitHub #<N> and link it") and continue. Perform no write.
   - ELSE:
     1. Create ONE Jira epic for `$GITHUB_ITEM`, with `$GITHUB_ITEM`'s URL
        embedded in the description as part of this single create call:
        - Summary: the GitHub issue title.
        - Description: the GitHub issue intent (Jira becomes the authority for
          intent once linked; copy the issue body intent across) AND `$GITHUB_ITEM`'s
          exact URL on its own line as a backlink. Writing the URL in the create
          call makes the epic guard-findable (Step 3.3) from the instant it exists,
          which closes the window between creation and the remote link: a crash
          anywhere after this call still leaves a guard-findable orphan, so a
          re-run reuses it and NEVER creates a second epic. Do NOT set the Jira
          status by hand (status is derived by `reconcile`). Do NOT set an agent as
          the assignee (credit is preserved for the human owner).
        - `$JIRA_EPIC` = the new epic.
     2. IMMEDIATELY add the Jira-to-GitHub remote link on `$JIRA_EPIC` pointing at
        `$GITHUB_ITEM`'s URL (if not already present). This is the durable,
        machine-resolvable canonical link part (per the spec's Canonical Identity
        Link), written right after creation and BEFORE any GitHub-side part.
5. Backfill the GitHub-side parts of the canonical link for `$JIRA_EPIC`,
   update-or-create and never duplicated (these run AFTER the remote link of Step
   3.4, so a crash before they complete still leaves a guard-findable orphan).
   Under `--dry-run`, report each would-be write and perform none:
   - Write the EXACT Jira key into `$GITHUB_ITEM`'s body on its own line (for
     exact matching), if not already present.
   - Add the `agent/execution-record` marker label to `$GITHUB_ITEM` (create the
     label if it does not exist), if not already present. This is WHEN a
     GitHub-origin issue acquires the marker: ingest applies it during this
     GitHub-first path, as part of establishing the canonical link.
   Step 4 then re-verifies all three parts idempotently for every record.
   `$EXECUTION_RECORD` = `$GITHUB_ITEM` (unchanged; no second GitHub record is
   ever created for a GitHub-origin item).

Collect every failed GitHub-first write (epic create, remote-link write,
Jira-key write, marker add) and surface it in Step 5; a partial failure SHALL NOT
be silently swallowed.

### Step 4: Ensure the bidirectional canonical link

For every `$EXECUTION_RECORD` resolved in Step 2 or Step 3, ensure the canonical
link exists in BOTH directions (create-if-missing, update-or-create, never
duplicated). First form the union of the Step 2 and Step 3 records and
de-duplicate it by execution record issue, so a record that already carries the
scope label AND is reachable from the Jira source (thus resolved from both
`$GITHUB_ITEMS` in Step 3 and `$JIRA_ITEMS` in Step 2 - typically a pre-existing
record on a re-run) is linked and reconciled exactly once, not twice:

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
Jira counterpart was reused or newly created. De-duplicate the set by execution
record issue: a record that already carries the scope label AND is reachable from
the Jira source is resolved both from `$JIRA_ITEMS` (Step 2) and from
`$GITHUB_ITEMS` (Step 3), so it MUST appear in `$EXECUTION_RECORDS` exactly once
and be reconciled once. Surface EVERY
collected error and partial failure: the ambiguity errors from Step 2.3 and Step
3.3, and any failed GitHub-first write from Step 3 (epic create, remote-link
write, Jira-key write, marker add). If any error was collected, the run MUST
surface it; partial failures SHALL NOT be silently swallowed.

Output: `$EXECUTION_RECORDS`

## Structured result

Write `$FULLSEND_OUTPUT_DIR/result.json`:

```json
{"status":"success","errors":[],"records":[{"url":"https://github.com/owner/repo/issues/1"}]}
```

Use `status: "failed"` and list every error if any write or resolution failed.
Partial completion is a failure. Honor `DRY_RUN` and report unauthorized records
as no-write skips. Never apply `agent/allowed`.
