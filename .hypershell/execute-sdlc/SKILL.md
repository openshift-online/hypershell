---
name: execute-sdlc
description: >
  Mechanically executes the SDLC workflow as defined in ../../../.hypershell/agents.
---

# Execute SDLC

Mechanically drive the agent-based SDLC.

Example usage:

```
> Execute `skills/build/execute-sdlc` `agent/scope/my-user`
```


## User Input

```text
$LABEL_SCOPE [$ISSUE_OVERRIDE (Optional)] [$LLM_OVERRIDE (Optional)] [$DRY_RUN]
```

- LABEL_SCOPE: A scope label (eg. `agent/scope/int`) used to narrow the
  execution scope of the SDLC. It is matched against GitHub issue labels AND
  Jira item labels, so the same scope selects work from either origin. Items
  without LABEL_SCOPE are not considered by the SDLC.
- ISSUE_OVERRIDE (Optional): A single work item against which the SDLC will be
  executed, given as EITHER a GitHub issue (URL or number) OR a Jira key / URL
  (eg. `HYPERSHELL-374` or `https://redhat.atlassian.net/browse/HYPERSHELL-374`).
  If provided, the SDLC executes against `$ISSUE_OVERRIDE` instead of choosing
  from applicable items within `$LABEL_SCOPE`. Note: If `$ISSUE_OVERRIDE` does
  not have `$LABEL_SCOPE` applied, the SDLC will exit.
- LLM_OVERRIDE (Optional): Defaults to `claude -p`. Override the command invoked
  to execute each of the agent steps. Eg. `codex --yolo e`
- $DRY_RUN: Optionally run without any write actions. TRUE if `--dry-run` provided, EMPTY otherwise.


## Invoking a Skill

Whenever `skill(<skill_path>, <args>)` is called, shell out to the following: 

```bash
CLAUDE_CODE_PRINT_BG_WAIT_CEILING_MS=0 $LLM_OVERRIDE "Execute skill: <skill_path> <args> $DRY_RUN"
```

Note: `claude -p` and its wrappers default to a background-wait ceiling of about
10 minutes, which terminates in-flight subagents before the step can commit and
push its work, silently losing it. Setting `CLAUDE_CODE_PRINT_BG_WAIT_CEILING_MS=0`
forces the invoked process to wait indefinitely for its subagents to complete.

Note: If LLM_OVERRIDE is not provided, default to `claude -p`.

## Workflow Steps

Mechanically execute the following steps.

Every phase of the SDLC (triage, spec, code, review) acts on a GitHub
execution record: a GitHub issue in `openshift-online/hypershell`. A
GitHub-origin work item is its own execution record. A Jira-origin work item is
ingested into a GitHub execution record. Step 0 guarantees that record exists
before any phase runs.

### Step 0: Resolve Execution Record (Ingest)

Resolve exactly one GitHub execution record for the work item. This step is
IDEMPOTENT: running it again resolves the same record and creates nothing new.

Determine the origin of the work item (`$ISSUE_OVERRIDE`, or each candidate
found under `$LABEL_SCOPE`):

- IF the work item is a GitHub issue (URL or number): the issue IS its own
  execution record. OUTPUT `$EXECUTION_RECORD` = that issue.

- IF the work item is a Jira key / URL (eg. `HYPERSHELL-374`), resolve a GitHub
  execution record for that Jira key:
  1. Search `openshift-online/hypershell` issues (open AND closed) for an
     execution record for this Jira key. A match REQUIRES both: the exact Jira
     key present in the issue body, AND the marker label `agent/execution-record`.
     The exact-key match is the idempotency key and prevents duplicates.
  2. IF exactly one match: reuse it. OUTPUT `$EXECUTION_RECORD` = that issue.
     Create nothing.
  3. IF more than one match: STOP. Exit with:
     "ERROR: multiple execution records for <JIRA_KEY>. Resolve duplicates."
  4. IF no match AND NOT `$DRY_RUN`: create ONE GitHub issue:
     - Title: `<JIRA_KEY>: <jira summary>`
     - Body: a link to `https://redhat.atlassian.net/browse/<JIRA_KEY>`, the
       Jira key on its own line (for exact matching), and the Jira item intent.
     - Labels: `agent/execution-record`, `agent/allowed`, and `$LABEL_SCOPE`
       (create any label that does not exist first).
     Then, once only, add a web link / comment on the Jira item pointing at the
     new GitHub issue. OUTPUT `$EXECUTION_RECORD` = the new issue.

- Associate existing artifacts (idempotent reuse, prevents duplicate PRs):
  Find any PR whose head branch or title contains `<JIRA_KEY>` (eg.
  `implementer/spec-<JIRA_KEY>-*`). IF found and not already linked to
  `$EXECUTION_RECORD`, add a reference comment linking the PR to the record.

- Derive the initial phase label (only if `$EXECUTION_RECORD` carries NO phase
  label yet, so this never overwrites an advanced state):
  - an open `implementer/code-<JIRA_KEY>-*` PR exists -> `agent/reviewable-code`
  - an open `implementer/spec-<JIRA_KEY>-*` PR exists -> `agent/reviewable-spec`
  - otherwise -> `agent/workable`

- Phase labels are GitHub-authoritative. Do NOT keep SDLC phase labels on the
  Jira item; the Jira item keeps only its scope selector and its (derived)
  status. Remove any stale phase label from the Jira item if present.

From here on, `$SELECTED_WORK_ITEM` refers to `$EXECUTION_RECORD`.

### Step 1: Execute Triage

IF `$ISSUE_OVERRIDE` IS EMPTY:
  skill(../../../.hypershell/agents/triage, openshift-online/hypershell, $LABEL_SCOPE)

### Step 2: Choose Work Item

IF `$ISSUE_OVERRIDE` IS EMPTY:
    Choose randomly an execution record that has both `$LABEL_SCOPE` and `agent/workable` labels.
ELSE:
    IF `$EXECUTION_RECORD` does not have `$LABEL_SCOPE` label applied:
        STOP. No further action. Exit with: "ERROR: work item does not have required scope label: $LABEL_SCOPE.

OUTPUT: $SELECTED_WORK_ITEM

### Step 3: Invoke Agents

WHILE(Labels Applied to $SELECTED_WORK_ITEM NOT INCLUDES `agent/review-code-approved`):

  SWITCH(Labels Applied to $SELECTED_WORK_ITEM):
    CASE `agent/workable`:
    CASE `agent/review-spec-rejected`:
      skill(../../../.hypershell/agents/spec-implementation, $SELECTED_WORK_ITEM)
      break

      CASE `agent/reviewable-spec`:
        Remove Label `agent/reviewable-spec`
        skill(../../../.hypershell/agents/spec-review, $SELECTED_WORK_ITEM)
        break

      CASE `agent/review-spec-approved`:
      CASE `agent/review-code-rejected`:
        Remove Label `agent/review-spec-approved`
        skill(../../../.hypershell/agents/code-implementation, $SELECTED_WORK_ITEM)
        break

      CASE `agent/reviewable-code`:
        Remove Label `agent/reviewable-code`
        skill(../../../.hypershell/agents/code-review, $SELECTED_WORK_ITEM)
        break
      
STOP: Provide URL to PR associated with $SELECTED_WORK_ITEM to the human for review.