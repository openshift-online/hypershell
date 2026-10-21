# Jira and GitHub SDLC Synchronization

**Date:** 2026-10-14
**Status:** Active
**Ticket:** HYPERSHELL-374
**Related:** `.hypershell/execute-sdlc/SKILL.md` (funnel entrypoint and scope selector), `.hypershell/agents/ingest/SKILL.md` (bidirectional intake), `.hypershell/agents/reconcile/SKILL.md` (bidirectional reconciliation), `.hypershell/agents/triage/SKILL.md` (work discovery), `.hypershell/agents/spec-implementation/SKILL.md` (spec phase), `skills/plan/spec/SKILL.md` (spec authoring)

---

## Purpose

The agent-based SDLC funnel runs on GitHub: branches, specification files, pull
requests, and a label-driven state machine carry a work item from intent to
merged code. Managers track that same work in Jira. Today the two systems drift:
a human must hand-copy GitHub progress into Jira for a manager to see status and
give credit.

This specification makes GitHub-side SDLC activity and Jira stay continuously in
sync. Either a GitHub issue or a Jira epic or story SHALL be able to start the
funnel, and intake SHALL be fully bidirectional: a Jira-first work item gets a
GitHub execution record created for it, and a GitHub-first work item gets a Jira
epic created for it, so both origins converge on the same canonically linked
pair. In every case the GitHub issue is the execution record and drives all SDLC
phases. GitHub is the execution substrate: it owns branches, specs, pull
requests, and the label state machine. Jira is the management view: it owns
intent, priority, assignment, and the epic hierarchy. A bidirectional reconciler
keeps the two converged under a per-field authority rule, so a manager sees
progress and credits the human owner without ever editing Jira by hand.

The reconciler converges records of EITHER origin. A scope run reconciles every
in-scope execution record, so a GitHub-origin record receives the same status
projection, artifact remote links, attributed narrative, and terminal closure as
a Jira-origin record. Resolution of the counterpart across systems is by the
canonical link ALONE - the `agent/execution-record` marker label plus the exact
Jira key - never by a descriptive mention of some other ticket's key in a body.

This is a cross-cutting process standard. It constrains the SDLC skills under
`.hypershell/` and defines the behavior those skills SHALL implement. It is a
desired-state contract, independent of when any given skill change lands:
`execute-sdlc` SHALL carry an origin-agnostic intake step (Step 0) and a
Jira-capable scope selector, the `ingest` and `reconcile` responsibilities SHALL
be bidirectional and origin-agnostic, and triage SHALL be origin-agnostic. The
"Required Skill Changes" section states that behavior as the normative contract
the skills SHALL satisfy.

---

## Origins and the Execution Record

A **work item** is a unit of intent that enters the funnel. It has exactly one
**origin**:

- A **GitHub origin**: a GitHub issue carrying the funnel's scope label.
- A **Jira origin**: a Jira epic or story selected by a Jira scope (a Jira
  label, a JQL query, an epic key, or a combination).

Every work item, regardless of origin, is executed through a **GitHub execution
record**: the GitHub issue that the funnel's triage, spec, code, and review
phases act on. A GitHub-origin work item is its own execution record. A
Jira-origin work item is ingested into a GitHub execution record that is created
if missing. Intake is bidirectional and origin-agnostic: a GitHub-origin work
item without a Jira counterpart is ingested the other way too - a Jira epic is
created for it and canonically linked - so that both origins end in the same
linked state and the GitHub issue always drives execution.

A **canonical link** is the bidirectional, durable association between a Jira
item and its GitHub execution record. It is the identity that every other
requirement in this spec depends on. The canonical link, and ONLY the canonical
link, resolves the counterpart in either direction. It is composed of three
parts that MUST all agree: the exact Jira key in the GitHub execution record
body, the `agent/execution-record` marker label on that GitHub issue, and a
Jira-to-GitHub remote link that points at the execution record. A descriptive
mention of a different ticket's key in a body (for example a GitHub-origin issue
that mentions `HYPERSHELL-374` to describe related work) is NOT a canonical link
and SHALL NEVER resolve a Jira item.

---

## The Authorization Gate

The funnel separates two independent decisions about a work item, each carried by
its own label:

- **Scope** (`agent/scope/<user>`, the funnel scope selector): selects an item
  for a given run. It is matched against both GitHub issue labels and Jira item
  labels so the same scope selects work from either origin.
- **Authorization** (`agent/allowed`): a separate, human-controlled decision that
  the item MAY be acted on by the funnel. It is the human authorization gate for
  the ENTIRE funnel.

Scope and authorization are orthogonal. An item that a human has scoped but has
deliberately NOT authorized SHALL NOT be acted on by any funnel step that performs
an outward-facing write. Concretely, no step SHALL create a Jira epic for it,
create a GitHub execution record for it, project its status or any
Jira-authoritative field, apply marker labels to it, or select it for an SDLC
phase unless it carries `agent/allowed`.

`agent/allowed` is load-bearing, not advisory. It gates every write-performing
step of the funnel - intake, reconciliation, and work selection - not only
triage. Because `agent/allowed` is a human act of authorization, NO agent step
SHALL apply it: an agent that granted the gate to a record it created would defeat
the gate. A record created by intake therefore starts UN-authorized and remains
inert until a human grants `agent/allowed`.

The gate is carried on the item in whichever system holds it before an execution
record exists, symmetric with the scope selector: on the GitHub issue for a
GitHub-origin item, and on the Jira item (as a Jira label) for a Jira-origin item.
Once an execution record exists, the gate is carried on the GitHub execution
record.

---

## Phase-to-Status Mapping

The GitHub label state machine is the authority for phase. Jira status is
DERIVED from the GitHub phase and SHALL NOT be hand-set. The projection targets
a Jira **status category**, not a hardcoded status name. Every Jira project has
exactly three status categories - `To Do` (key `new`), `In Progress` (key
`indeterminate`), and `Done` (key `done`) - while the concrete status names are
per-project workflow configuration. Targeting the category and resolving the name
per project keeps status names out of the code (the "separate configuration from
code" convention) and makes the mapping portable across any Jira workflow. The
projection uses this mapping:

| GitHub phase (label state) | Derived Jira status category |
|----------------------------|------------------------------|
| No execution record yet | `To Do` |
| `agent/needs-input` | `To Do` |
| `agent/workable` | `In Progress` |
| `agent/reviewable-spec`, `agent/review-spec-rejected` | `In Progress` |
| `agent/review-spec-approved` | `In Progress` |
| `agent/reviewable-code`, `agent/review-code-rejected` | `In Progress` |
| `agent/review-code-approved` | `In Progress` |
| `agent/blocked` | `In Progress` |
| Pull request merged to `main` | `Done` |
| GitHub execution record Closed (no detectable merged pull request) | `Done` |
| `agent/duplicate` (issue closed as duplicate) | `Done` |

The reconciler SHALL resolve the concrete Jira status for a mapped category by
selecting an available transition whose target status has that `statusCategory`.
If several transitions qualify, it SHALL pick the project's canonical (first)
status in that category. If no status in the mapped category exists on the
workflow, the reconciler SHALL report an error and SHALL NOT hand-set an
arbitrary status. A phase whose mapped category already matches the Jira item's
current status category SHALL produce no transition (no-op). This never silently
skips an unreachable category.

The `Done` category has two independent triggers. The first is a linked pull
request merged to `main` (which also closes the execution record - see Terminal
Execution Record Closure). The second is the GitHub execution record being
Closed: because GitHub is authoritative for the record's open/closed state, a
Closed record is itself an authoritative terminal signal, so the reconciler SHALL
project the Jira item to the `Done` category even when no merged pull request is
detectable. Either trigger alone SHALL be sufficient. This closes the gap where a
record closed by hand, or whose merge landed under a pull request that references
a different issue, would otherwise strand its Jira item in `In Progress`.

---

## Per-Field Authority

Reconciliation is governed by per-field authority. Each field has exactly one
authoritative system; the reconciler copies from the authority to the other
system and never the reverse:

| Field | Authoritative system |
|-------|---------------------|
| Intent, description, acceptance criteria | Jira |
| Priority | Jira |
| Assignee | Jira |
| Epic linkage and hierarchy | Jira |
| Phase, labels, SDLC state machine | GitHub |
| Execution record open/closed | GitHub |
| Jira status | Derived from GitHub phase (see mapping) |
| Branch, pull requests, commits, specs | GitHub |
| Authorization to act (`agent/allowed`) | Human (granted by hand; no agent SHALL set it) |

Jira status is a special case: it is a projection of the GitHub phase, so GitHub
is its effective source even though the value lives in Jira.

---

## Requirements

### Requirement: SDLC-JIRA-01 -- Authorization Gate Is Load-Bearing

`agent/allowed` SHALL be the human authorization gate for the entire funnel. No
funnel step that performs an outward-facing write SHALL act on a work item that
does not carry `agent/allowed`. This gate SHALL apply to intake, reconciliation,
and work selection, not only to triage.

Scope and authorization are distinct: `agent/scope/*` selects an item for a run,
while `agent/allowed` authorizes the funnel to act on it. An in-scope item that is
not authorized SHALL be inert across the whole funnel.

- Intake SHALL restrict its in-scope set to items carrying `agent/allowed`
  alongside the funnel scope. A GitHub-origin issue SHALL be in scope only if it
  carries BOTH the funnel scope label AND `agent/allowed`. A Jira-origin item
  SHALL be in scope only if the Jira item carries the `agent/allowed`
  authorization marker (as a Jira label) in addition to the Jira scope. Intake
  SHALL NOT create a Jira epic, a GitHub execution record, or a canonical link for
  an item that is not authorized.
- Reconciliation SHALL perform no Jira or GitHub write for an execution record
  that does not carry `agent/allowed`. Such a record SHALL be reported as skipped,
  NOT as a failure.
- Work selection SHALL require `agent/allowed` in its candidate-set predicate,
  alongside the scope and active-phase conditions. An un-authorized record SHALL
  NOT be selected for any SDLC phase.

Authorization SHALL be granted only by a human. NO agent step SHALL apply
`agent/allowed`. An execution record created by intake SHALL start un-authorized
and SHALL carry only its canonical-link marker (`agent/execution-record`) and the
funnel scope label; it SHALL remain inert until a human grants `agent/allowed`.

#### Scenario: SDLC-JIRA-01.01 -- Scoped but unauthorized item is not ingested

- GIVEN a GitHub issue carrying `agent/scope/<user>` but NOT `agent/allowed`
- WHEN intake runs
- THEN no Jira epic is created for the issue
- AND no canonical link is created
- AND the issue is not treated as an in-scope execution record

#### Scenario: SDLC-JIRA-01.02 -- Reconcile skips an unauthorized record

- GIVEN an execution record carrying the funnel scope label but NOT `agent/allowed`
- WHEN reconciliation runs against it
- THEN no Jira or GitHub write is performed
- AND the record is reported as skipped, not failed

#### Scenario: SDLC-JIRA-01.03 -- Work selection excludes an unauthorized record

- GIVEN an execution record carrying the funnel scope label and an active phase
  label but NOT `agent/allowed`
- WHEN work selection builds its candidate set
- THEN the record is not a candidate
- AND no SDLC phase runs against it

#### Scenario: SDLC-JIRA-01.04 -- Intake does not self-authorize a created record

- GIVEN an authorized, in-scope Jira item with no linked GitHub execution record
- WHEN intake creates the GitHub execution record
- THEN the new record carries `agent/execution-record` and the funnel scope label
- AND the new record does NOT carry `agent/allowed`

#### Scenario: SDLC-JIRA-01.05 -- Granting authorization lets the funnel proceed

- GIVEN a scoped execution record that was inert because it lacked `agent/allowed`
- WHEN a human adds `agent/allowed` to the record
- AND the funnel runs again
- THEN the record is reconciled and becomes selectable for its phase

### Requirement: SDLC-JIRA-02 -- Origin-Agnostic Intake

The funnel scope selector SHALL accept a Jira source in addition to a GitHub
label. A Jira source is a Jira label, a JQL query, an epic key, or a combination
of these. `execute-sdlc` and `triage` SHALL discover workable items from either
origin.

The in-scope set SHALL be gated by `agent/allowed` (see Authorization Gate Is
Load-Bearing): a GitHub-origin issue is in scope only if it carries BOTH the
funnel scope label AND `agent/allowed`, and a Jira-origin item is in scope only if
the Jira item carries `agent/allowed` (as a Jira label) in addition to the Jira
scope. Intake SHALL NOT create a counterpart or canonical link for an
un-authorized item.

Intake SHALL be bidirectional and origin-agnostic. For every in-scope work item,
regardless of origin, intake SHALL ensure BOTH counterparts exist and are
canonically linked:

- Jira-first: for an in-scope Jira item with no linked GitHub execution record,
  intake SHALL create the GitHub execution record and the canonical link
  (existing behavior).
- GitHub-first: for an in-scope GitHub-origin execution record with no linked
  Jira item, intake SHALL create a Jira epic for it and the canonical link.

In both directions the GitHub issue IS the execution record and drives all SDLC
phases. Intake SHALL be create-if-missing and idempotent: a second intake of the
same work item SHALL make no change and SHALL return the existing linked pair. No
duplicate GitHub execution record, Jira epic, or link SHALL ever be created.

A GitHub-origin item is its own execution record and SHALL NOT get a second one.
A Jira-origin item SHALL be ingested into a GitHub execution record created in
the funnel's GitHub repository, carrying the funnel scope label so the existing
state machine applies unchanged. A GitHub-origin item SHALL retain its own issue
as the execution record; the Jira epic created for it SHALL serve only as the
management-view counterpart.

Intake SHALL resolve exactly one execution record for a work item and exactly one
Jira counterpart. The canonical link is the idempotency key, and the Jira
counterpart SHALL be resolved ONLY through the canonical link (see the
Canonical-Link-Only Resolution requirement). If intake finds more than one
execution record for one work item, the state is ambiguous: intake SHALL NOT
guess, SHALL NOT create another record, and SHALL STOP and report an error that
names the work item and asks a human to resolve the duplicates.

#### Scenario: SDLC-JIRA-02.01 -- GitHub-first item gets a Jira epic and canonical link

- GIVEN an in-scope GitHub-origin execution record carrying the funnel scope
  label and the `agent/execution-record` marker
- AND no Jira item is canonically linked to it
- WHEN the funnel runs intake
- THEN a Jira epic is created for the GitHub issue
- AND a bidirectional canonical link is established between them
- AND the GitHub issue remains the execution record that drives execution
- AND no second GitHub execution record is created

#### Scenario: SDLC-JIRA-02.02 -- GitHub-first intake is idempotent

- GIVEN a GitHub-origin item already canonically linked to a Jira epic
- WHEN intake runs again
- THEN no new Jira epic is created
- AND the existing linked pair is returned unchanged

#### Scenario: SDLC-JIRA-02.03 -- Jira epic enters the funnel

- GIVEN a Jira scope that selects epic `HYPERSHELL-374`
- AND no GitHub execution record is linked to it
- WHEN the funnel runs intake
- THEN a GitHub issue is created as the execution record
- AND it carries the funnel scope label
- AND it is canonically linked to `HYPERSHELL-374`

#### Scenario: SDLC-JIRA-02.04 -- Intake is idempotent

- GIVEN a Jira-origin item already linked to a GitHub execution record
- WHEN intake runs again
- THEN no new GitHub issue is created
- AND the existing execution record is returned unchanged

#### Scenario: SDLC-JIRA-02.05 -- GitHub-origin item needs no second record

- GIVEN a GitHub issue carrying the funnel scope label
- WHEN intake runs
- THEN the issue itself is the execution record
- AND no additional GitHub issue is created

#### Scenario: SDLC-JIRA-02.06 -- Multiple execution records for one item stop intake

- GIVEN a Jira-origin work item
- AND more than one GitHub execution record matches that work item
- WHEN intake runs
- THEN intake does not guess which record to use
- AND intake creates no new GitHub execution record
- AND intake stops and reports an error that names the work item and asks a
  human to resolve the duplicate records

### Requirement: SDLC-JIRA-03 -- Canonical Identity Link

Every work item SHALL have a bidirectional canonical link between the Jira item
and its GitHub execution record. The link SHALL be created if missing in either
direction: a Jira item discovered first SHALL gain a link to its new GitHub
record, and a GitHub item that names a Jira key SHALL gain a link to that Jira
item.

The link SHALL be durable and machine-resolvable from either side, so later
phases and the reconciler locate the counterpart without re-deriving it.

#### Scenario: SDLC-JIRA-03.01 -- Link created from the Jira side

- GIVEN a Jira-origin item with no GitHub record
- WHEN intake creates the GitHub execution record
- THEN the Jira item references the GitHub record
- AND the GitHub record references the Jira item

#### Scenario: SDLC-JIRA-03.02 -- Link created from the GitHub side

- GIVEN a GitHub-origin execution record with the `agent/execution-record` marker
- AND no Jira counterpart yet exists
- WHEN the funnel processes the issue
- THEN a Jira epic is created for it
- AND a bidirectional canonical link to that epic is created

### Requirement: SDLC-JIRA-04 -- Canonical-Link-Only Resolution

`ingest` and `reconcile` SHALL resolve a GitHub execution record's Jira
counterpart ONLY through the canonical link: the `agent/execution-record` marker
label on the GitHub issue PLUS the exact Jira key recorded in that issue's body
for the purpose of the link. Resolution SHALL require the marker; a GitHub issue
without the `agent/execution-record` marker SHALL NOT be treated as an execution
record and SHALL NOT resolve any Jira item.

A descriptive mention of a Jira key in an issue body - a key named to describe
related or prior work, such as a GitHub-origin issue that mentions
`HYPERSHELL-374` where that key canonically belongs to a different execution
record - SHALL NEVER cause resolution of that Jira item. The reconciler SHALL NOT
infer a link from such a mention and SHALL treat the record as having no Jira
counterpart unless its own canonical link resolves one. This prevents a
GitHub-origin record from being reconciled against the wrong Jira item.

When no canonical link resolves a Jira counterpart, the record is a valid
GitHub-origin item with no Jira item yet; intake SHALL create and link one (see
Origin-Agnostic Intake), and `reconcile` invoked before that link exists SHALL
report nothing to reconcile and STOP without error.

#### Scenario: SDLC-JIRA-04.01 -- Descriptive key mention does not resolve a Jira item

- GIVEN a GitHub-origin execution record whose body mentions `HYPERSHELL-374` to
  describe related work
- AND `HYPERSHELL-374` canonically belongs to a different execution record
- AND this record has no canonical link of its own
- WHEN reconciliation runs
- THEN it does NOT resolve `HYPERSHELL-374`
- AND it does not write to `HYPERSHELL-374`
- AND it treats this record as having no Jira counterpart

#### Scenario: SDLC-JIRA-04.02 -- Marker plus exact key resolves the correct counterpart

- GIVEN a GitHub execution record carrying the `agent/execution-record` marker
- AND its body records the exact Jira key of its own canonically linked epic
- WHEN reconciliation resolves the counterpart
- THEN it resolves exactly that epic and no other

### Requirement: SDLC-JIRA-05 -- Scope Reconciliation Covers Both Origins

A scope run SHALL reconcile EVERY in-scope execution record, of either origin.
Reconciliation SHALL NOT be limited to records that intake resolved from a Jira
source. A GitHub-origin execution record in scope SHALL receive the same
reconciliation as a Jira-origin record: Jira status projection, artifact remote
links, attributed narrative comments, and terminal closure on merge.

The set of records a scope run reconciles SHALL be every execution record
carrying the funnel scope label AND `agent/allowed`, independent of how it entered
the funnel. After intake has ensured each in-scope record has a canonical Jira
counterpart, the reconciler SHALL converge all of them under the Per-Field
Authority rule. A record that carries the scope label but not `agent/allowed`
SHALL be skipped (no write), not failed (see Authorization Gate Is Load-Bearing).

#### Scenario: SDLC-JIRA-05.01 -- GitHub-origin record is reconciled by a scope run

- GIVEN an in-scope GitHub-origin execution record whose pull request has merged
  to `main`
- AND intake has created and canonically linked its Jira epic
- WHEN a scope run reconciles
- THEN the record's Jira status is projected to the `Done` category
- AND the merge commit appears as a remote link on the Jira item
- AND the GitHub execution record is closed

#### Scenario: SDLC-JIRA-05.02 -- Scope run reconciles records of both origins

- GIVEN a scope containing one Jira-origin and one GitHub-origin execution record
- WHEN a scope run reconciles
- THEN both records receive status projection, artifact links, and narrative
- AND neither origin is skipped

### Requirement: SDLC-JIRA-06 -- Status Projection

As the GitHub execution record moves through SDLC phases, the linked Jira item's
status SHALL reflect the current phase per the Phase-to-Status Mapping. The
mapping targets a status category; the reconciler resolves the concrete status by
selecting an available transition whose target status has the mapped category.
Jira status SHALL be derived from the GitHub phase and SHALL NEVER be hand-set by
an agent. A phase whose mapped category matches the Jira item's current status
category SHALL produce no transition. An unreachable category SHALL be reported as
an error, never a hand-set arbitrary status. A Closed execution record projects
to the `Done` category independently of merge detection (see Closed Execution
Record Projects Done).

#### Scenario: SDLC-JIRA-06.01 -- Spec phase moves Jira to In Progress

- GIVEN a linked work item whose GitHub record enters `agent/reviewable-spec`
- WHEN status projection runs
- THEN the Jira item's status becomes In Progress

#### Scenario: SDLC-JIRA-06.02 -- Merge moves Jira to Done

- GIVEN a linked work item whose pull request merges to `main`
- WHEN status projection runs
- THEN the Jira item's status becomes Done

#### Scenario: SDLC-JIRA-06.03 -- Projection is a no-op when already aligned

- GIVEN a linked work item already in the status its phase maps to
- WHEN status projection runs
- THEN no transition is attempted

### Requirement: SDLC-JIRA-07 -- Closed Execution Record Projects Done

GitHub is authoritative for an execution record's open/closed state, so a Closed
record is an authoritative terminal signal in its own right. When the GitHub
execution record is Closed, the reconciler SHALL project the linked Jira item to
the `Done` category, regardless of whether a merged pull request is detectable.
The concrete status SHALL be resolved from the category via an available
transition, exactly as for every other projection; the reconciler SHALL NEVER
hand-set an arbitrary status.

This is a second, independent path to `Done` that complements the merge-based
behavior; it SHALL NOT replace it. A merged linked pull request SHALL still close
an open execution record and project `Done` (see Terminal Execution Record
Closure), and the Closed-record path SHALL additionally cover a record that is
Closed without a detectable merged pull request - for example a record closed by
hand, or one whose merge landed under a pull request that references a different
issue.

Projection from the Closed state SHALL be idempotent and drift-correcting: a
Closed record whose Jira item is already in the `Done` category SHALL produce no
transition, and a re-run SHALL make no change. It SHALL be dry-run aware, and it
SHALL honor per-field authority - it projects status only and SHALL NOT write
assignee, priority, or intent. An open execution record with no merged pull
request SHALL remain projected to `In Progress` and SHALL NOT be forced to `Done`.

#### Scenario: SDLC-JIRA-07.01 -- Closed record with no merged pull request projects Done

- GIVEN a linked execution record that is Closed
- AND no merged pull request is detectable for it
- WHEN reconciliation runs
- THEN the Jira item's status is in the `Done` category

#### Scenario: SDLC-JIRA-07.02 -- Open record without a merged pull request stays In Progress

- GIVEN a linked execution record that is Open
- AND has no merged pull request
- WHEN reconciliation runs
- THEN the Jira item's status stays in the `In Progress` category

#### Scenario: SDLC-JIRA-07.03 -- Closed-record Done projection is idempotent

- GIVEN a Closed execution record whose Jira item is already in the `Done`
  category
- WHEN reconciliation runs again
- THEN no transition is attempted and no field is written

### Requirement: SDLC-JIRA-08 -- Terminal Execution Record Closure

When a work item reaches its terminal Done state - the linked pull request is
merged to `main` - the reconciler SHALL converge the ENTIRE stack of tickets to a
consistent terminal state in a single run. In addition to projecting the Jira
item to the `Done` category and linking the merge commit as a remote link, the
reconciler SHALL close the GitHub execution record issue.

Closure SHALL be derived from the merge state, scoped to the merged case: the
GitHub execution record SHALL be closed if and only if its linked pull request is
merged to `main`. This covers only the merge path to `Done`; the `agent/duplicate`
path to `Done` (a record already closed as a duplicate during triage) SHALL NOT
trigger a separate closure here. An execution record whose pull request is not yet
merged SHALL remain Open.

Closure SHALL be idempotent and drift-correcting: closing an already-closed
execution record SHALL be a no-op, and a run over a record whose pull request has
merged but which is still Open SHALL close it. A reconcile pass over a scope whose
record has a merged pull request SHALL therefore converge the record, the pull
request, and the Jira item to a consistent terminal state, including closing any
already-merged-but-open execution record. Re-running SHALL change nothing.

#### Scenario: SDLC-JIRA-08.01 -- Merge closes the execution record

- GIVEN a linked work item whose pull request has merged to `main`
- AND whose GitHub execution record is still Open
- WHEN reconciliation runs
- THEN the GitHub execution record is closed
- AND the Jira item's status is in the `Done` category
- AND the merge commit appears as a remote link on the Jira item

#### Scenario: SDLC-JIRA-08.02 -- Unmerged record stays open

- GIVEN a linked work item whose pull request is not yet merged
- WHEN reconciliation runs
- THEN the GitHub execution record remains Open

#### Scenario: SDLC-JIRA-08.03 -- Closing an already-closed record is a no-op

- GIVEN a linked work item whose pull request is merged
- AND whose GitHub execution record is already closed
- WHEN reconciliation runs again
- THEN no change is made to the execution record

### Requirement: SDLC-JIRA-09 -- Artifact Remote Links

The branch, each pull request, and the merge commit of a work item SHALL appear
on the linked Jira item as remote links. Remote links SHALL be update-or-create:
re-running SHALL update the existing link for a given artifact rather than
creating a duplicate.

#### Scenario: SDLC-JIRA-09.01 -- Pull request becomes a remote link

- GIVEN a linked work item with an open pull request
- WHEN artifact sync runs
- THEN the pull request URL appears as a remote link on the Jira item

#### Scenario: SDLC-JIRA-09.02 -- Re-run does not duplicate links

- GIVEN a Jira item that already has the pull request as a remote link
- WHEN artifact sync runs again
- THEN the existing remote link is updated in place
- AND no duplicate remote link is created

### Requirement: SDLC-JIRA-10 -- Narrative Comments

Each SDLC phase transition SHALL post a Jira comment describing the transition.
The comment SHALL be attributed to the human owner, phrased as acting on behalf
of the owner. An agent SHALL NOT post a comment that claims the agent as the
author of the work.

#### Scenario: SDLC-JIRA-10.01 -- Phase transition posts an attributed comment

- GIVEN a linked work item whose GitHub record transitions phase
- WHEN narrative sync runs
- THEN a Jira comment records the transition
- AND the comment is attributed on behalf of the human owner

### Requirement: SDLC-JIRA-11 -- Bidirectional Reconciliation

Jira and GitHub SHALL be reconciled from each other under the Per-Field Authority
rule. For each field, the reconciler SHALL copy the value from the authoritative
system to the other and SHALL NOT write back toward the authority. Reconciliation
SHALL be idempotent and drift-correcting: a field that has diverged SHALL be
restored to the authority's value, and a run with no drift SHALL make no change.

#### Scenario: SDLC-JIRA-11.01 -- Jira-authoritative field drift is corrected

- GIVEN a work item whose description differs between Jira and GitHub
- WHEN reconciliation runs
- THEN the GitHub record's description matches the Jira description
- AND the Jira description is unchanged

#### Scenario: SDLC-JIRA-11.02 -- GitHub-authoritative field drift is corrected

- GIVEN a work item whose phase advanced in GitHub but whose Jira status lags
- WHEN reconciliation runs
- THEN the Jira status matches the phase per the mapping
- AND the GitHub phase is unchanged

#### Scenario: SDLC-JIRA-11.03 -- Second pass makes zero changes

- GIVEN a work item that was just fully reconciled
- WHEN reconciliation runs a second time
- THEN no field is written in either system

### Requirement: SDLC-JIRA-12 -- Safety and Approval Gates

All writes performed by intake, projection, artifact sync, narrative sync, and
reconciliation SHALL respect `--dry-run` and the funnel's existing approval
gates, including the `agent/allowed` authorization gate (see Authorization Gate
Is Load-Bearing). Under `--dry-run`, every would-be write SHALL be reported and
none SHALL be performed. No partial failure SHALL be silently swallowed: every
failed write SHALL be propagated or collected and reported.

#### Scenario: SDLC-JIRA-12.01 -- Dry-run performs no writes

- GIVEN the funnel runs with `--dry-run`
- WHEN intake, projection, and reconciliation execute
- THEN no GitHub or Jira write occurs
- AND each would-be write is reported

#### Scenario: SDLC-JIRA-12.02 -- Partial failure is reported

- GIVEN a reconciliation pass in which one Jira write fails
- WHEN the pass completes
- THEN the failure is reported
- AND it is not silently ignored

### Requirement: SDLC-JIRA-13 -- Credit Preservation

The Jira assignee SHALL remain the human owner throughout the funnel. Agents
SHALL NOT set themselves as the Jira assignee and SHALL NOT claim authorship of
the work in any Jira field or comment.

#### Scenario: SDLC-JIRA-13.01 -- Assignee is unchanged after a run

- GIVEN a Jira item assigned to its human owner
- WHEN the funnel processes the item through any phase
- THEN the Jira assignee is still the human owner

---

## Required Skill Changes

These are the behaviors the SDLC skills SHALL implement. They are stated here as
the normative contract the skills must satisfy, regardless of when the
corresponding skill changes land. Each subsection is a desired-state requirement,
not a record of work already performed.

### `execute-sdlc` gains a Jira-origin intake path (Step 0)

`execute-sdlc` SHALL accept a Jira source as part of its scope, alongside the
existing GitHub `$LABEL_SCOPE`. Before triage (Step 1) and work selection
(Step 2), a new Step 0 SHALL run intake (see the new `ingest` responsibility) so
that every in-scope, authorized Jira item has a linked GitHub execution record.
Step 2 work selection SHALL then choose from execution records regardless of
origin, requiring `agent/allowed` in its candidate-set predicate alongside the
scope and active-phase conditions (see Authorization Gate Is Load-Bearing). A
Jira-origin item that has been ingested SHALL be selectable exactly like a
GitHub-origin item, and in neither case SHALL an un-authorized record be
selected.

### `triage` discovers work from either origin

`triage` SHALL discover in-scope items from both the GitHub label scope and the
Jira source. For a Jira-origin item, triage SHALL operate on its GitHub execution
record after ingestion, applying the same `agent/needs-input`, `agent/workable`,
and `agent/duplicate` classification. Triage SHALL NOT read Jira items that are
outside the configured Jira source.

### `ingest` becomes bidirectional and origin-agnostic

The `ingest` responsibility SHALL ensure a canonically linked Jira-and-GitHub
pair for every in-scope work item, from EITHER origin. It SHALL:

- Resolve the Jira source to the set of in-scope Jira items carrying
  `agent/allowed`, and resolve the GitHub scope to the set of in-scope GitHub
  issues carrying BOTH the funnel scope label AND `agent/allowed` (symmetric with
  the `triage` contract). An item that is scoped but not authorized SHALL NOT be
  in scope for intake. An in-scope GitHub issue that already carries the
  `agent/execution-record` marker is an established execution record; one
  carrying the scope label and `agent/allowed` but not the marker is a
  GitHub-origin candidate that the GitHub-first path ingests.
- Jira-first: for each in-scope Jira item without a canonical link, create a
  GitHub execution record carrying the funnel scope label and the
  `agent/execution-record` marker, and create the bidirectional canonical link.
  The created record SHALL NOT carry `agent/allowed`: authorization is a human act
  and `ingest` SHALL NOT self-authorize a record it creates (see Authorization
  Gate Is Load-Bearing).
- GitHub-first: for each in-scope GitHub-origin issue with no canonically linked
  Jira item, create a Jira epic for it and create the bidirectional canonical
  link, applying the `agent/execution-record` marker to the GitHub issue as part
  of that link. (This is when a GitHub-origin issue acquires the marker: `ingest`
  applies it during the GitHub-first path.) The GitHub issue remains the
  execution record.
- Resolve the counterpart ONLY through the canonical link (the
  `agent/execution-record` marker plus the exact Jira key), never from a
  descriptive key mention.
- Be create-if-missing and idempotent, returning the existing linked pair when
  one already exists, never creating a duplicate record, epic, or link.
- Respect `--dry-run` and the funnel approval gates, and collect and report every
  partial failure rather than swallowing it.

`ingest` SHALL run before triage in the `execute-sdlc` workflow so that the rest
of the funnel operates uniformly on GitHub execution records of either origin.

### Scope reconciliation covers every in-scope record

A scope run of `execute-sdlc` SHALL reconcile every in-scope execution record,
not only the records intake resolved from the Jira source. After intake, the
funnel SHALL reconcile each execution record carrying the funnel scope label AND
`agent/allowed`, regardless of origin, so authorized GitHub-origin records receive
status projection, artifact remote links, attributed narrative, and terminal
closure on merge.

`reconcile` SHALL enforce the authorization gate itself: if the resolved
execution record does not carry `agent/allowed`, `reconcile` SHALL perform no Jira
or GitHub write and SHALL report the record as skipped (not failed). This makes
the gate load-bearing even when `reconcile` is invoked directly on a single
record, not only through a scope run (see Authorization Gate Is Load-Bearing).

### Self-heal note

After this standard ships, a scope run SHALL converge `#537`, the GitHub-origin
execution record whose pull request `#538` is already merged but which was left
Open by the earlier Jira-source-driven intake. The run SHALL create its Jira
counterpart, link it canonically, project its status, and - because its pull
request is merged - close `#537`. This is a self-healing consequence of the
bidirectional intake and both-origins scope reconciliation defined above; no
manual closure is required.

After the Closed Execution Record Projects Done requirement ships, a scope run
SHALL also converge `#524`, the Closed execution record whose Wave B work merged
under pull request `#528` (which references `#521`, not `#524`), so no merged
pull request is detectable for `#524`. Because `#524` is Closed, the reconciler
SHALL project its linked epic `HYPERSHELL-392` to the `Done` category without
requiring a detectable merged pull request. This is a self-healing consequence of
the Closed-record Done path; no manual Jira edit is required.

---

## Acceptance Criteria

- Running `Execute skills/build/execute-sdlc agent/scope/mturansk`, fed the Jira
  epic `HYPERSHELL-374`, ingests the epic and opens linked GitHub execution
  records.
- A single requirement run ends with the Jira status transitioned per the
  mapping, the pull request present as a Jira remote link, phase comments posted
  on behalf of the human owner, and the Jira assignee unchanged.
- A second reconcile pass over the same work item makes zero changes in either
  system.
- After the funnel runs against a scope whose execution record has a merged pull
  request, the whole stack is terminal and consistent: the GitHub execution
  record is Closed, the Jira item is in the `Done` category, and the merge commit
  is linked. The execution record is closed by the funnel, not by hand, and
  re-running changes nothing.
- A GitHub-origin issue and a Jira-origin story that describe the same intent
  converge to the same end state: linked record, projected status, remote links,
  and attributed comments.
- A GitHub-first work item in scope gets a Jira epic created and canonically
  linked, GitHub drives execution, and on merge the execution record closes while
  Jira projects to the `Done` category.
- `reconcile` never resolves a Jira item from a descriptive key mention; it
  resolves a counterpart only via the `agent/execution-record` marker plus the
  exact key.
- A scope run converges `#537`: it creates the Jira counterpart, links it
  canonically, projects status, and closes `#537` because pull request `#538` is
  merged. Re-running changes nothing, creating no duplicate epics, records, or
  links.
- An item carrying `agent/scope/<user>` but not `agent/allowed` is inert across
  the whole funnel: intake creates no epic, record, or link for it; `reconcile`
  skips it without writing; and work selection never selects it. Adding
  `agent/allowed` by hand lets the next run ingest, reconcile, and select it.
- No agent ever applies `agent/allowed`; an execution record intake creates starts
  with only `agent/execution-record` and the funnel scope label and stays inert
  until a human authorizes it.
- A scope run projects the Jira item of a Closed execution record to the `Done`
  category even when no merged pull request is detectable; `HYPERSHELL-392` (the
  epic for the Closed record `#524`) reaches `Done`. An open record with no merged
  pull request stays `In Progress`. Re-running changes nothing.

#### Example: GitHub-first item converges on merge

- GIVEN an in-scope GitHub-first execution record with no Jira counterpart
- WHEN the funnel runs
- THEN a Jira epic is created and canonically linked
- AND GitHub drives execution through the SDLC phases
- AND on merge the execution record is closed and the Jira item is projected to
  the `Done` category

#### Example: Scope run self-heals the stranded GitHub-origin record

- GIVEN `#537`, a GitHub-origin execution record whose pull request `#538` is
  merged but which is still Open and has no Jira counterpart
- WHEN a scope run of `execute-sdlc` reconciles the scope
- THEN a Jira epic is created for `#537` and canonically linked
- AND `#537`'s status is projected to the `Done` category
- AND `#537` is closed
- AND re-running makes no further change

#### Example: Jira epic drives a full run

- GIVEN the Jira epic `HYPERSHELL-374` in a configured Jira source
- WHEN `execute-sdlc` runs with scope `agent/scope/mturansk`
- THEN the epic is ingested into a linked GitHub execution record
- AND the record proceeds through the SDLC phases
- AND the Jira item ends with the mapped status, the pull request as a remote
  link, attributed phase comments, and an unchanged assignee

#### Example: Two origins converge

- GIVEN one GitHub-origin issue and one Jira-origin story of equal intent
- WHEN each is run through the funnel
- THEN both reach the same linked, projected, and artifact-synced end state

---

## Non-Goals

- Enabling the native GitHub "Development" panel is a separate administrative
  task. This spec relies on Jira remote links, not the native panel, to surface
  branches, pull requests, and commits on the Jira item.

---

## Design Decisions

| Decision | Rationale |
|----------|-----------|
| GitHub is the execution substrate; Jira is the management view | Branches, specs, pull requests, and the label state machine already live on GitHub and drive the agents. Jira is where managers plan and credit. Splitting authority by system matches how each is actually used. |
| Every work item runs through a GitHub execution record | The existing funnel acts on GitHub issues. Ingesting Jira items into GitHub records lets triage, spec, code, and review stay unchanged and origin-blind. |
| Jira status is derived, never hand-set | A single derivation from the GitHub phase removes drift at the source and keeps managers from editing status by hand. |
| Per-field authority rather than last-writer-wins | Last-writer-wins would let an agent overwrite human-owned intent or an epic link. Fixed authority per field makes reconciliation deterministic and safe to run repeatedly. |
| Remote links instead of the native Development panel | Remote links work without the one-time admin enablement of the native panel and are update-or-create, so artifact sync stays idempotent. |
| Comments attributed on behalf of the human owner | Managers give credit to people. Agents report progress but never claim authorship, so the assignee and the narrative both point to the owner. |
| Intake is idempotent and create-if-missing | The funnel is safe to run repeatedly. Re-running never creates duplicate execution records or duplicate links. |
| Intake is bidirectional, not Jira-source-driven only | A Jira-source-only intake strands GitHub-origin records: their per-record reconcile never runs, so they get no status projection or terminal closure. Creating a Jira counterpart for a GitHub-first item and reconciling every in-scope record of either origin makes the funnel origin-symmetric. |
| Resolution by canonical link only, never by a descriptive key mention | A GitHub issue may mention another ticket's key to describe related work. Resolving from that mention would reconcile against the wrong Jira item. Requiring the `agent/execution-record` marker plus the exact linked key makes resolution unambiguous and safe. |
| The GitHub issue always drives execution, whichever side initiates | The label state machine, branches, and pull requests live on GitHub. Keeping GitHub as the execution substrate regardless of origin means the triage, spec, code, and review phases stay origin-blind. |
| `agent/allowed` gates the whole funnel, not only triage | Intake, reconcile, and selection all perform outward-facing writes. Gating only triage left the gate advisory: a scoped-but-unauthorized item could still get an epic and status projection. Making every write-performing step require `agent/allowed` makes the human authorization decision load-bearing. |
| No agent applies `agent/allowed` | Authorization is a human decision. An agent that stamped the gate onto a record it created would authorize its own work and defeat the gate, so intake leaves created records un-authorized until a human grants it. |
| Scope and authorization are separate labels | `agent/scope/*` answers "is this item in this run's set"; `agent/allowed` answers "may the funnel act on it". Conflating them would force authorizing everything in a scope or scoping only authorized work. Keeping them orthogonal lets a human stage work into scope and authorize it independently. |
| A Closed execution record projects Done independently of merge detection | GitHub is authoritative for the record's open/closed state. Keying Done only on a detectable merged pull request stranded records that were closed by hand or whose merge referenced a different issue. Treating the Closed state as a second, independent terminal trigger removes that gap while leaving the merge-based close-and-project behavior unchanged. |
