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
The intake itself is owned by the [`ingest` responsibility](../../../.hypershell/agents/ingest/SKILL.md),
which this step delegates to; it is create-if-missing, idempotent, and dry-run
aware.

Determine the origin of the work item (`$ISSUE_OVERRIDE`, or each candidate
found under `$LABEL_SCOPE`):

- IF the work item is a GitHub issue (URL or number): the issue IS its own
  execution record. OUTPUT `$EXECUTION_RECORD` = that issue.

- IF the work item is a Jira key / URL (eg. `HYPERSHELL-374`), OR `$ISSUE_OVERRIDE`
  is EMPTY (a scope-based run that may include Jira-origin items), run intake via
  the `ingest` responsibility so every in-scope Jira item has a linked GitHub
  execution record before triage:

  skill(../../../.hypershell/agents/ingest, $LABEL_SCOPE, $ISSUE_OVERRIDE)

  `ingest` resolves each Jira item to exactly one execution record (reusing an
  existing one on an exact Jira-key match, creating one only when missing),
  ensures the bidirectional canonical link, and STOPs on ambiguity (more than one
  execution record for one work item). It never creates a second record for a
  GitHub-origin item. When `$ISSUE_OVERRIDE` is a single Jira key, OUTPUT
  `$EXECUTION_RECORD` = the record `ingest` resolved for it.

For a scope-based run (`$ISSUE_OVERRIDE` empty), `ingest` resolves a SET of
execution records. The per-record steps below (artifact association, initial
phase derivation, and the newly-ingested reconcile) SHALL be applied to EACH
execution record `ingest` resolved. For an `$ISSUE_OVERRIDE` run they apply to
the single `$EXECUTION_RECORD`.

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

- Reconcile the newly ingested record so its initial phase projects into Jira
  (an `agent/workable` record projects to `To Do`) and its canonical link and
  any existing artifacts surface as remote links:

  skill(../../../.hypershell/agents/reconcile, $EXECUTION_RECORD)

  This runs with no `$PREVIOUS_PHASE`, so it projects status and syncs artifacts
  without posting a transition comment. It is idempotent, so an already-advanced
  record writes nothing new.

From here on, `$SELECTED_WORK_ITEM` refers to `$EXECUTION_RECORD`.

### Step 1: Execute Triage

IF `$ISSUE_OVERRIDE` IS EMPTY:
  skill(../../../.hypershell/agents/triage, openshift-online/hypershell, $LABEL_SCOPE)

Triage classifies only items that are not already in flight. An item that already
carries any active or terminal `agent/*` phase label (for example
`agent/workable`, `agent/reviewable-spec`, `agent/review-spec-approved`,
`agent/reviewable-code`, `agent/review-code-approved`, `agent/blocked`) has
already been triaged and SHALL be left unchanged.

### Step 2: Choose Work Item

The funnel resumes in-flight work; it does not only start new `agent/workable`
items. The ACTIVE phase labels are: `agent/workable`, `agent/reviewable-spec`,
`agent/review-spec-approved`, `agent/review-spec-rejected`,
`agent/reviewable-code`, `agent/review-code-rejected`.

IF `$ISSUE_OVERRIDE` IS EMPTY:
    Let the candidate set be every execution record that has `$LABEL_SCOPE` AND
    at least one ACTIVE phase label. Exclude records that are terminal or paused
    (`agent/review-code-approved`, `agent/needs-input`, `agent/duplicate`,
    `agent/blocked`). Choose one candidate at random.
    IF the candidate set is empty:
        STOP. No further action. Exit with: "No in-flight or workable items in scope: $LABEL_SCOPE."
ELSE:
    IF `$EXECUTION_RECORD` does not have `$LABEL_SCOPE` label applied:
        STOP. No further action. Exit with: "ERROR: work item does not have required scope label: $LABEL_SCOPE.

OUTPUT: $SELECTED_WORK_ITEM

### Step 3: Invoke Agents

Keep Jira in sync as the GitHub phase advances. Before the loop, record the
current phase as `$PREVIOUS_PHASE`. After every iteration (every phase
transition), project the new phase into Jira and sync artifacts and the phase
narrative by invoking the [`reconcile` responsibility](../../../.hypershell/agents/reconcile/SKILL.md).
Reconciliation is idempotent, so a no-op transition writes nothing.

WHILE(Labels Applied to $SELECTED_WORK_ITEM NOT INCLUDES `agent/review-code-approved`):

  $PREVIOUS_PHASE = the current phase label of $SELECTED_WORK_ITEM

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

  # After the phase transition, keep Jira converged with the new GitHub phase:
  # derived status, artifact remote links, and an attributed phase-narrative
  # comment. Idempotent and dry-run aware.
  skill(../../../.hypershell/agents/reconcile, $SELECTED_WORK_ITEM, $PREVIOUS_PHASE)

# The loop ended at `agent/review-code-approved`. The last loop iteration already
# ran reconcile for the `reviewable-code -> review-code-approved` transition, so
# the approved state (which projects to In Review) and the artifact remote links
# are already synced. The pull request is approved but NOT yet merged (a human
# performs the merge), so Jira is NOT yet Done.

# Terminal Done projection: reconcile is idempotent and drift-correcting, keyed
# only on the execution record. Once the human merges the pull request, reconcile
# MUST be run one more time against this record so it detects the merge and
# projects the Jira item to Done with the merge commit as a remote link. Request
# that post-merge reconcile in the handoff below.
STOP: Provide the PR URL for $SELECTED_WORK_ITEM to the human for review, and ask
them to merge it and then re-run reconcile against this record so Jira projects to
Done:
    skill(../../../.hypershell/agents/reconcile, $SELECTED_WORK_ITEM)