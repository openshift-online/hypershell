---
name: execute-sdlc
description: >
  Mechanically executes the SDLC workflow as defined in ../agents.
---

# Execute SDLC

Mechanically drive the agent-based SDLC.

Example usage:

```
> Execute `.hypershell/execute-sdlc` `agent/scope/my-user`
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
  to execute each of the agent steps. Eg. `codex --yolo e`. Only consulted when
  `USE_HEADLESS` is true.
- $DRY_RUN: Optionally run without any write actions. TRUE if `--dry-run` provided, EMPTY otherwise.
- USE_HEADLESS (Optional environment variable, not a positional argument): Selects
  HOW each `skill(...)` step is dispatched. Read it from the environment; when
  unset or empty, treat it as TRUE. TRUE keeps the historical behavior - shell out
  to a headless CLI (`$LLM_OVERRIDE`). FALSE dispatches each step as an in-harness
  agent subtask instead, which is what lets a non-Claude agent runtime execute this
  SDLC by mapping `skill(...)` onto its own sub-agent primitive rather than assuming
  `claude -p` exists.


## Invoking a Skill

Each `skill(<skill_path>, <args>)` in the steps below is one unit of work that must
run to completion in its own fresh agent context. Both dispatch modes run the SAME
skill with the SAME arguments and the SAME `$DRY_RUN` flag; they differ only in the
execution mechanism. Choose the mechanism from `USE_HEADLESS` (default TRUE).

### USE_HEADLESS=true (default): headless CLI shell-out

Shell out to the configured LLM CLI:

```bash
CLAUDE_CODE_PRINT_BG_WAIT_CEILING_MS=0 $LLM_OVERRIDE "Execute skill: <skill_path> <args> $DRY_RUN"
```

- If `$LLM_OVERRIDE` is not provided, default to `claude -p`. Override it to target
  another CLI (eg. `codex --yolo e`).
- `CLAUDE_CODE_PRINT_BG_WAIT_CEILING_MS=0` matters because `claude -p` and its
  wrappers default to a background-wait ceiling of about 10 minutes, which
  terminates in-flight subagents before the step can commit and push its work,
  silently losing it. Setting it to 0 forces the invoked process to wait
  indefinitely for its subagents to complete.

### USE_HEADLESS=false: in-harness agent subtask

Do NOT shell out to a CLI. Instead schedule the work as an agent subtask using
whatever subagent/Task primitive the running harness provides, passing the same
instruction verbatim as the subtask prompt:

```text
Execute skill: <skill_path> <args> $DRY_RUN
```

This path assumes nothing about `claude -p` being installed, so the orchestrator
stays portable across agent runtimes. It also unlocks parallelism: where a step
below is marked parallelizable (Step 2 and Step 3), `USE_HEADLESS=false` SHOULD
fan those subtasks out concurrently, one subtask per in-scope work item. Under
`USE_HEADLESS=true` the same steps run one item at a time, because serial CLI
shell-outs are the only safe option there.

## Workflow Steps

Mechanically execute the following steps.

Every phase of the SDLC (triage, spec, code, review) acts on a GitHub
execution record: a GitHub issue in `openshift-online/hypershell`. A
GitHub-origin work item is its own execution record. A Jira-origin work item is
ingested into a GitHub execution record. Step 0 guarantees that record exists
before any phase runs.

### Step 0: Resolve Execution Records (Ingest)

Resolve the GitHub execution record(s) for the in-scope work. This step is
IDEMPOTENT: running it again resolves the same records and creates nothing new.
Intake is owned by the [`ingest` responsibility](../agents/ingest/SKILL.md),
which this step delegates to; it is bidirectional, origin-agnostic,
create-if-missing, idempotent, and dry-run aware. For every in-scope work item of
EITHER origin it ensures BOTH a GitHub execution record and a canonically linked
Jira counterpart exist before triage runs.

Delegate intake to `ingest`, passing any `$ISSUE_OVERRIDE` through as its source
selector (a single Jira key / URL, a single GitHub issue, or empty for a full
scope run):

  skill(../agents/ingest, $LABEL_SCOPE, $ISSUE_OVERRIDE)

`ingest` resolves the in-scope set from BOTH the Jira source AND GitHub issues
carrying `$LABEL_SCOPE`, so GitHub-origin records are discovered, not only
Jira-ingested ones. For each item it resolves exactly one execution record
(reusing an existing one on a canonical-link match, creating one only when
missing), creates the missing counterpart (a GitHub record for a Jira-first item,
a Jira epic for a GitHub-first item), ensures the bidirectional canonical link,
and STOPs on ambiguity (more than one execution record for one work item). It
never creates a second record for a GitHub-origin item and never a duplicate Jira
epic. A GitHub-origin issue acquires the `agent/execution-record` marker here,
during `ingest`'s GitHub-first path.

`ingest` gates its in-scope set on `agent/allowed` (the human authorization gate
for the whole funnel): it resolves only items carrying `agent/allowed` alongside
the scope, and it never self-authorizes a record it creates (see
`Authorization Gate Is Load-Bearing` in
`specs/standards/sdlc/jira-github-sync.spec.md`). A scoped but un-authorized item
is inert: `ingest` creates no epic, record, or link for it.

`ingest` returns the SET of in-scope execution records (`$EXECUTION_RECORDS`), of
EITHER origin:
- For `$ISSUE_OVERRIDE` empty: every execution record carrying BOTH `$LABEL_SCOPE`
  AND `agent/allowed` (Jira-ingested and GitHub-origin alike).
- For `$ISSUE_OVERRIDE` a single Jira key or a single GitHub issue: the one record
  it resolved. OUTPUT `$EXECUTION_RECORD` = that record.

The per-record steps below (artifact association, initial phase derivation, and
the newly-ingested reconcile) SHALL be applied to EVERY execution record `ingest`
resolved, regardless of origin. Reconciling every in-scope record of either
origin - not only the Jira-ingested set - is what gives a GitHub-origin record
the same status projection, artifact links, narrative, and terminal closure as a
Jira-origin record (Scope Reconciliation Covers Both Origins). For an
`$ISSUE_OVERRIDE` run they apply to the single `$EXECUTION_RECORD`.

- Associate existing artifacts (idempotent reuse, prevents duplicate PRs):
  Find any PR whose head branch or title contains `<JIRA_KEY>` (eg.
  `implementer/spec-<JIRA_KEY>-*`). For a GitHub-origin record the Jira key is
  freshly minted, so its pre-existing PR will NOT match that pattern; also
  discover the record's natively linked PRs (those referencing this GitHub issue,
  eg. `gh pr list --search <issue-number>` or the issue's linked pull requests).
  IF found and not already linked to `$EXECUTION_RECORD`, add a reference comment
  linking the PR to the record. (Terminal closure does not depend on this step:
  `reconcile` reads the record's linked PR merge state directly.)

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

  skill(../agents/reconcile, $EXECUTION_RECORD)

  This runs with no `$PREVIOUS_PHASE`, so it projects status and syncs artifacts
  without posting a transition comment. It is idempotent, so an already-advanced
  record writes nothing new.

From here on, `$SELECTED_WORK_ITEM` refers to `$EXECUTION_RECORD`.

### Step 1: Execute Triage

IF `$ISSUE_OVERRIDE` IS EMPTY:
  skill(../agents/triage, openshift-online/hypershell, $LABEL_SCOPE)

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

`agent/allowed` is the human authorization gate for the whole funnel, so work
selection requires it in the candidate-set predicate alongside the scope and
active-phase conditions. An un-authorized record SHALL NOT be selected for any
SDLC phase (see `Authorization Gate Is Load-Bearing` in
`specs/standards/sdlc/jira-github-sync.spec.md`).

IF `$ISSUE_OVERRIDE` IS EMPTY:
    Let the candidate set be every execution record that has `$LABEL_SCOPE` AND
    `agent/allowed` AND at least one ACTIVE phase label. Exclude records that are
    terminal or paused (`agent/review-code-approved`, `agent/needs-input`,
    `agent/duplicate`, `agent/blocked`).
    Also exclude any record held at the human spec-approval gate: a record whose
    highest-precedence ACTIVE phase is `agent/review-spec-approved` that ALSO carries
    the human-applied `agent/review-spec-blocked` label AND whose spec PR does not
    yet have an approving review from a human (see Step 3). Such a record is waiting
    on a person, so picking it would only stall the run; it rejoins the candidate set
    automatically once the human approves its spec PR.
    IF the candidate set is empty:
        STOP. No further action. Exit with: "No in-flight or workable items in scope: $LABEL_SCOPE."
    Selection depends on the dispatch mode, because the candidate set is safely
    parallelizable: each execution record advances through its own phases without
    touching another record's state or branch.
    - USE_HEADLESS=true (default): choose ONE candidate at random. Serial
      shell-outs make fan-out unsafe, so a single run advances a single record.
    - USE_HEADLESS=false: select the ENTIRE candidate set and run Step 3 for every
      record concurrently, one agent subtask per record (see Step 3).
ELSE:
    IF `$EXECUTION_RECORD` does not have `$LABEL_SCOPE` label applied:
        STOP. No further action. Exit with: "ERROR: work item does not have required scope label: $LABEL_SCOPE.
    IF `$EXECUTION_RECORD` does not have the `agent/allowed` label applied:
        STOP. No further action. Exit with: "ERROR: work item is not authorized (missing agent/allowed): $EXECUTION_RECORD."

OUTPUT: $SELECTED_WORK_ITEMS (the single chosen record under USE_HEADLESS=true or
an `$ISSUE_OVERRIDE` run; the whole candidate set under USE_HEADLESS=false)

### Step 3: Invoke Agents

Run the per-record phase loop below for each record in `$SELECTED_WORK_ITEMS`.
Under USE_HEADLESS=false fan the records out concurrently, one agent subtask per
record, and let each subtask run the whole loop for its record to completion; the
records are independent, so they never need to coordinate. Under USE_HEADLESS=true
there is exactly one record, run inline. In BOTH modes the phases WITHIN a single
record stay strictly sequential - a record carries exactly one ACTIVE phase label
at a time, so there is nothing to parallelize inside one record, only across them.


Keep Jira in sync as the GitHub phase advances. Before the loop, record the
current phase as `$PREVIOUS_PHASE`. After every iteration (every phase
transition), project the new phase into Jira and sync artifacts and the phase
narrative by invoking the [`reconcile` responsibility](../agents/reconcile/SKILL.md).
Reconciliation is idempotent, so a no-op transition writes nothing.

The phase is single-valued: exactly one ACTIVE phase label drives the item. The
ACTIVE phase labels, furthest-along first, map to one step each:

  1. `agent/reviewable-code`      -> code-review
  2. `agent/review-code-rejected` -> code-implementation
  3. `agent/review-spec-approved` -> code-implementation
  4. `agent/reviewable-spec`      -> spec-review
  5. `agent/review-spec-rejected` -> spec-implementation
  6. `agent/workable`             -> spec-implementation

`agent/review-code-approved` is terminal. `agent/needs-input`, `agent/duplicate`,
and `agent/blocked` are paused; the loop does not act on them.

`agent/review-spec-blocked` is an OPTIONAL human gate a person applies to force a
human spec sign-off before any code is written. It is NOT a phase label and does
not change the phase map above; it only gates the `agent/review-spec-approved ->
code-implementation` transition (handled in the loop below). It stays inert during
the spec phases, so the item still advances to `agent/review-spec-approved` and
opens its spec PR for the human to review - gating earlier would deadlock, since
there would be no spec PR to approve.

WHILE $SELECTED_WORK_ITEM has at least one ACTIVE phase label:

  # Pick the FURTHEST-ALONG active phase, so a stale earlier label never re-runs a
  # completed phase. This precedence is what makes a full top-to-bottom run correct
  # and idempotent.
  $CURRENT_PHASE = the highest-precedence ACTIVE phase label on $SELECTED_WORK_ITEM
  $PREVIOUS_PHASE = $CURRENT_PHASE

  SWITCH($CURRENT_PHASE):
    CASE `agent/workable`:
    CASE `agent/review-spec-rejected`:
      skill(../agents/spec-implementation, $SELECTED_WORK_ITEM)
      break
    CASE `agent/reviewable-spec`:
      skill(../agents/spec-review, $SELECTED_WORK_ITEM)
      break
    CASE `agent/review-spec-approved`:
      # Human spec-approval gate (opt-in). If a person applied
      # `agent/review-spec-blocked`, do not start coding until they have approved the
      # spec PR. The spec PR is this record's `implementer/spec-<JIRA_KEY>-*` PR;
      # "approved" means it carries an approving review from a HUMAN reviewer (eg.
      # `gh pr view <pr> --json reviewDecision` == APPROVED, or a human review with
      # state APPROVED - ignore agent/bot reviews). This gate is NOT applied to
      # `agent/review-code-rejected` below: that is a re-implementation after code
      # review, and the spec was already signed off earlier.
      IF $SELECTED_WORK_ITEM has `agent/review-spec-blocked`:
        IF the spec PR has a human approving review:
          # Gate satisfied. Clear the human gate (it is not a phase label, so the
          # normalize step would not remove it) and fall through to implementation.
          Remove `agent/review-spec-blocked` from $SELECTED_WORK_ITEM.
        ELSE:
          # Still waiting on a person. Do NOT run code-implementation and do NOT
          # change any labels. Under fan-out this stops only this record's subtask;
          # otherwise it ends the run. The next run re-checks and proceeds once the
          # human approves the spec PR.
          STOP this record with: "Waiting on human spec approval
          (agent/review-spec-blocked) for $SELECTED_WORK_ITEM."
      skill(../agents/code-implementation, $SELECTED_WORK_ITEM)
      break
    CASE `agent/review-code-rejected`:
      skill(../agents/code-implementation, $SELECTED_WORK_ITEM)
      break
    CASE `agent/reviewable-code`:
      skill(../agents/code-review, $SELECTED_WORK_ITEM)
      break

  # Normalize to a single phase label (idempotent, self-correcting). The invoked
  # skill applies the OUTPUT phase label for this transition. Remove EVERY other
  # phase label (including `$CURRENT_PHASE` and any stale label) so exactly one
  # phase label remains on the record. This keeps the state single-valued whether
  # the funnel runs end-to-end or is re-entered, and removes the whole class of
  # stale-label bugs. Removing a label that is already absent is a no-op.

  # Keep Jira converged with the new phase: derived status, artifact remote links,
  # and an attributed phase-narrative comment. Idempotent and dry-run aware.
  skill(../agents/reconcile, $SELECTED_WORK_ITEM, $PREVIOUS_PHASE)

# The loop ended at `agent/review-code-approved`. The last loop iteration already
# ran reconcile for the `reviewable-code -> review-code-approved` transition, so
# the approved state (which projects to In Review) and the artifact remote links
# are already synced. The pull request is approved but NOT yet merged (a human
# performs the merge), so Jira is NOT yet Done.

# Terminal Done projection: reconcile is idempotent and drift-correcting, keyed
# only on the execution record. Once the human merges the pull request, reconcile
# MUST be run one more time against this record so it detects the merge and
# converges the whole stack: it projects the Jira item to Done with the merge
# commit as a remote link AND closes this GitHub execution record (closure is
# derived from the merge state, so the record is closed by the funnel, not by
# hand). Request that post-merge reconcile in the handoff below.
STOP: Provide the PR URL for $SELECTED_WORK_ITEM to the human for review, and ask
them to merge it and then re-run reconcile against this record so Jira projects to
Done and this execution record is closed automatically:
    skill(../agents/reconcile, $SELECTED_WORK_ITEM)