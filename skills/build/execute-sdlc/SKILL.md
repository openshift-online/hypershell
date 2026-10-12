---
name: execute-sdlc
description: >
  Mechanically executes the SDLC workflow as defined in ../../../.hypershell/agents.
---

# Execute SDLC

Mechanically drive the 


## User Input

```text
$LABEL_SCOPE [$ISSUE_OVERRIDE (Optional)] [$LLM_OVERRIDE (Optional)] [$DRY_RUN]
```

- LABEL_SCOPE: A github issue label (eg. `agent/scope/int`) used to narrow
  the execution scope of the SDLC. Issues without LABEL_SCOPE are not considered
  by the SDLC.
- ISSUE_OVERRIDE (Optional): A single GitHub issue against which the SDLC will be
  executed. If provided, the SDLC will execute against `$ISSUE_OVERRIDE` instead
  of choosing from applicable issues within `$LABEL_SCOPE`. Note: If `$ISSUE_OVERRIDE`
  does not have `$LABEL_SCOPE` applied, the SDLC will exit.
- LLM_OVERRIDE (Optional): Defaults to `claude -p`. Override the command invoked
  to execute each of the agent steps. Eg. `codex --yolo e`
- $DRY_RUN: Optionally run without any write actions. TRUE if `--dry-run` provided, EMPTY otherwise.


## Invoking a Skill

Whenever `skill(<skill_path>, <args>)` is called, shell out to the following: 

```bash
$LLM_OVERRIDE "Execute skill: <skill_path> <args> $DRY_RUN"
```

Note: If LLM_OVERRIDE is not provided, default to `claude -p`.

## Workflow Steps

Mechanically execute the following steps:

### Step 0: Execute Triage

IF `$ISSUE_OVERRIDE` IS EMPTY:
  skill(../../../.hypershell/agents/triage, openshift-online/hypershell, $LABEL_SCOPE)

### Step 1: Choose Work Item

IF `$ISSUE_OVERRIDE` IS EMPTY:
    Choose randomly an issue that has both `$LABEL_SCOPE` and `agent/workable` labels.
ELSE:
    IF `$ISSUE_OVERRIDE` does not have `$LABEL_SCOPE` label applied:
        STOP. No further action. Exit with: "ERROR: $LABEL_SCOPE does not have required scope label: $LABEL_SCOPE.

OUTPUT: $SELECTED_WORK_ITEM

### Step 2: Invoke Agents

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