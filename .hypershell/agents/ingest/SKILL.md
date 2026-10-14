---
name: ingest
description: >
  Origin-agnostic intake. Ensures every in-scope Jira work item has a linked
  GitHub execution record, create-if-missing and idempotent.
---

# Workflow

Intake responsibility for the SDLC funnel. It makes the funnel origin-agnostic:
a Jira epic or story becomes a GitHub execution record that the rest of the
funnel (triage, spec, code, review) acts on unchanged. This responsibility is
invoked by `execute-sdlc` Step 0, before triage, so every later phase operates
uniformly on GitHub execution records.

Realizes the `Origin-Agnostic Intake`, `Canonical Identity Link`, and
`New ingest responsibility` requirements of
`specs/standards/sdlc/jira-github-sync.spec.md`.

## Input

```text
$LABEL_SCOPE [$JIRA_SOURCE (Optional)] [--dry-run]
```

Supported arguments:
- LABEL_SCOPE -- Required. The funnel scope selector (eg. `agent/scope/mturansk`).
  It is matched against GitHub issue labels AND Jira item labels, so the same
  scope selects work from either origin. Every execution record this
  responsibility creates carries this label.
- JIRA_SOURCE -- [Optional] A narrower Jira source: a Jira label, a JQL query,
  an epic key (eg. `HYPERSHELL-374`), or a combination. When omitted, the Jira
  source is the set of Jira items that carry `$LABEL_SCOPE` as a Jira label.
- --dry-run -- [Optional] Execute the workflow, but without any WRITE actions.
  Every would-be write MUST be reported and none performed.

## Workflow

### Step 1: Resolve the Jira source

Resolve the Jira source to the set of in-scope Jira items (`$JIRA_ITEMS`):

- IF `$JIRA_SOURCE` is an epic key (eg. `HYPERSHELL-374`): the item is that
  epic. (Do NOT auto-expand the epic into its children; only items that the
  source directly selects are in scope.)
- IF `$JIRA_SOURCE` is a JQL query: the items are the query result.
- IF `$JIRA_SOURCE` is a Jira label: the items are those carrying that label.
- IF `$JIRA_SOURCE` is omitted: the items are those carrying `$LABEL_SCOPE` as a
  Jira label.

You MUST NOT read Jira items that are outside the configured Jira source.

Output: `$JIRA_ITEMS`

### Step 2: Resolve or create an execution record for each item

For each `$JIRA_ITEM` in `$JIRA_ITEMS`, ensure exactly one linked GitHub
execution record. This step is create-if-missing and IDEMPOTENT: running it
again on an already-linked item makes no change and returns the existing record.

1. Search `openshift-online/hypershell` issues (open AND closed) for the
   execution record of `$JIRA_ITEM`. A match REQUIRES both: the exact Jira key
   present in the issue body, AND the marker label `agent/execution-record`. The
   exact-key match is the idempotency key and prevents duplicates.
2. IF exactly one match: reuse it. `$EXECUTION_RECORD` = that issue. Create
   nothing. Proceed to Step 3 to ensure the link.
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

### Step 3: Ensure the bidirectional canonical link

The canonical link is the durable, machine-resolvable association between the
Jira item and its GitHub execution record. Ensure it exists in BOTH directions
(create-if-missing, update-or-create, never duplicated):

- GitHub -> Jira: the execution record body contains the exact Jira key (set in
  Step 2 on creation; if a reused record is missing it, add it).
- Jira -> GitHub: the Jira item carries a remote link (or a single marker
  comment) that points at the execution record issue URL. If the link is absent,
  create it once; if present, leave it unchanged.

A GitHub-origin issue whose body names a Jira key but which has no link yet SHALL
gain the Jira -> GitHub link here, so links can originate from either side.

Under `--dry-run`, report each would-be link write and perform none.

### Step 4: Report

Output the set of resolved execution records (`$EXECUTION_RECORDS`), noting for
each whether it was reused or newly created, plus any collected errors from Step
2.3. If any error was collected, the run MUST surface it; partial failures SHALL
NOT be silently swallowed.

Output: `$EXECUTION_RECORDS`
