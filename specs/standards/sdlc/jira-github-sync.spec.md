# Jira and GitHub SDLC Synchronization

**Date:** 2026-10-14
**Status:** Active
**Ticket:** HYPERSHELL-374
**Related:** `.hypershell/execute-sdlc/SKILL.md` (funnel entrypoint and scope selector), `.hypershell/agents/triage/SKILL.md` (work discovery), `.hypershell/agents/spec-implementation/SKILL.md` (spec phase), `skills/plan/spec/SKILL.md` (spec authoring)

---

## Purpose

The agent-based SDLC funnel runs on GitHub: branches, specification files, pull
requests, and a label-driven state machine carry a work item from intent to
merged code. Managers track that same work in Jira. Today the two systems drift:
a human must hand-copy GitHub progress into Jira for a manager to see status and
give credit.

This specification makes GitHub-side SDLC activity and Jira stay continuously in
sync. Either a GitHub issue or a Jira epic or story SHALL be able to start the
funnel. GitHub is the execution substrate: it owns branches, specs, pull
requests, and the label state machine. Jira is the management view: it owns
intent, priority, assignment, and the epic hierarchy. A bidirectional reconciler
keeps the two converged under a per-field authority rule, so a manager sees
progress and credits the human owner without ever editing Jira by hand.

This is a cross-cutting process standard. It constrains the SDLC skills under
`.hypershell/` and defines the desired behavior the code phase implements. The
pull request that introduces this standard also makes the first realization of
it: the `execute-sdlc` entrypoint gains an origin-agnostic intake step (Step 0)
and a scope selector that accepts a Jira source. The remaining skill changes
listed under "Required Skill Changes" are realized by later code-phase work.

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
if missing.

A **canonical link** is the bidirectional, durable association between a Jira
item and its GitHub execution record. It is the identity that every other
requirement in this spec depends on.

---

## Phase-to-Status Mapping

The GitHub label state machine is the authority for phase. Jira status is
DERIVED from the GitHub phase and SHALL NOT be hand-set. The projection uses this
mapping:

| GitHub phase (label state) | Derived Jira status |
|----------------------------|---------------------|
| `agent/workable` (ingested, not started) | To Do |
| `agent/reviewable-spec`, `agent/review-spec-rejected` | In Progress |
| `agent/review-spec-approved` | In Progress |
| `agent/reviewable-code`, `agent/review-code-rejected` | In Review |
| `agent/review-code-approved` | In Review |
| Pull request merged to `main` | Done |
| `agent/needs-input` | Blocked |
| `agent/duplicate` (issue closed as duplicate) | Won't Do |

Target Jira statuses are named by their canonical workflow meaning. The code
phase SHALL resolve each to the concrete status id available on the Jira project
workflow and SHALL treat an unreachable transition as a reported error, not a
silent skip.

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
| Jira status | Derived from GitHub phase (see mapping) |
| Branch, pull requests, commits, specs | GitHub |

Jira status is a special case: it is a projection of the GitHub phase, so GitHub
is its effective source even though the value lives in Jira.

---

## Requirements

### Requirement: Origin-Agnostic Intake

The funnel scope selector SHALL accept a Jira source in addition to a GitHub
label. A Jira source is a Jira label, a JQL query, an epic key, or a combination
of these. `execute-sdlc` and `triage` SHALL discover workable items from either
origin.

For every in-scope work item, intake SHALL ensure a linked GitHub execution
record exists. Intake SHALL be create-if-missing and idempotent: a second intake
of the same work item SHALL make no change and SHALL return the existing
execution record.

A GitHub-origin item is its own execution record and SHALL NOT get a second one.
A Jira-origin item SHALL be ingested into a GitHub execution record created in
the funnel's GitHub repository, carrying the funnel scope label so the existing
state machine applies unchanged.

Intake SHALL resolve exactly one execution record for a work item. The canonical
link is the idempotency key. If intake finds more than one execution record for
one work item, the state is ambiguous: intake SHALL NOT guess, SHALL NOT create
another record, and SHALL STOP and report an error that names the work item and
asks a human to resolve the duplicates.

#### Scenario: Jira epic enters the funnel

- GIVEN a Jira scope that selects epic `HYPERSHELL-374`
- AND no GitHub execution record is linked to it
- WHEN the funnel runs intake
- THEN a GitHub issue is created as the execution record
- AND it carries the funnel scope label
- AND it is canonically linked to `HYPERSHELL-374`

#### Scenario: Intake is idempotent

- GIVEN a Jira-origin item already linked to a GitHub execution record
- WHEN intake runs again
- THEN no new GitHub issue is created
- AND the existing execution record is returned unchanged

#### Scenario: GitHub-origin item needs no second record

- GIVEN a GitHub issue carrying the funnel scope label
- WHEN intake runs
- THEN the issue itself is the execution record
- AND no additional GitHub issue is created

#### Scenario: Multiple execution records for one item stop intake

- GIVEN a Jira-origin work item
- AND more than one GitHub execution record matches that work item
- WHEN intake runs
- THEN intake does not guess which record to use
- AND intake creates no new GitHub execution record
- AND intake stops and reports an error that names the work item and asks a
  human to resolve the duplicate records

### Requirement: Canonical Identity Link

Every work item SHALL have a bidirectional canonical link between the Jira item
and its GitHub execution record. The link SHALL be created if missing in either
direction: a Jira item discovered first SHALL gain a link to its new GitHub
record, and a GitHub item that names a Jira key SHALL gain a link to that Jira
item.

The link SHALL be durable and machine-resolvable from either side, so later
phases and the reconciler locate the counterpart without re-deriving it.

#### Scenario: Link created from the Jira side

- GIVEN a Jira-origin item with no GitHub record
- WHEN intake creates the GitHub execution record
- THEN the Jira item references the GitHub record
- AND the GitHub record references the Jira item

#### Scenario: Link created from the GitHub side

- GIVEN a GitHub issue whose body names `HYPERSHELL-374`
- AND no link yet exists
- WHEN the funnel processes the issue
- THEN a bidirectional canonical link to `HYPERSHELL-374` is created

### Requirement: Status Projection

As the GitHub execution record moves through SDLC phases, the linked Jira item's
status SHALL reflect the current phase per the Phase-to-Status Mapping. Jira
status SHALL be derived from the GitHub phase and SHALL NEVER be hand-set by an
agent. A phase that maps to the Jira item's current status SHALL produce no
transition.

#### Scenario: Spec phase moves Jira to In Progress

- GIVEN a linked work item whose GitHub record enters `agent/reviewable-spec`
- WHEN status projection runs
- THEN the Jira item's status becomes In Progress

#### Scenario: Merge moves Jira to Done

- GIVEN a linked work item whose pull request merges to `main`
- WHEN status projection runs
- THEN the Jira item's status becomes Done

#### Scenario: Projection is a no-op when already aligned

- GIVEN a linked work item already in the status its phase maps to
- WHEN status projection runs
- THEN no transition is attempted

### Requirement: Artifact Remote Links

The branch, each pull request, and the merge commit of a work item SHALL appear
on the linked Jira item as remote links. Remote links SHALL be update-or-create:
re-running SHALL update the existing link for a given artifact rather than
creating a duplicate.

#### Scenario: Pull request becomes a remote link

- GIVEN a linked work item with an open pull request
- WHEN artifact sync runs
- THEN the pull request URL appears as a remote link on the Jira item

#### Scenario: Re-run does not duplicate links

- GIVEN a Jira item that already has the pull request as a remote link
- WHEN artifact sync runs again
- THEN the existing remote link is updated in place
- AND no duplicate remote link is created

### Requirement: Narrative Comments

Each SDLC phase transition SHALL post a Jira comment describing the transition.
The comment SHALL be attributed to the human owner, phrased as acting on behalf
of the owner. An agent SHALL NOT post a comment that claims the agent as the
author of the work.

#### Scenario: Phase transition posts an attributed comment

- GIVEN a linked work item whose GitHub record transitions phase
- WHEN narrative sync runs
- THEN a Jira comment records the transition
- AND the comment is attributed on behalf of the human owner

### Requirement: Bidirectional Reconciliation

Jira and GitHub SHALL be reconciled from each other under the Per-Field Authority
rule. For each field, the reconciler SHALL copy the value from the authoritative
system to the other and SHALL NOT write back toward the authority. Reconciliation
SHALL be idempotent and drift-correcting: a field that has diverged SHALL be
restored to the authority's value, and a run with no drift SHALL make no change.

#### Scenario: Jira-authoritative field drift is corrected

- GIVEN a work item whose description differs between Jira and GitHub
- WHEN reconciliation runs
- THEN the GitHub record's description matches the Jira description
- AND the Jira description is unchanged

#### Scenario: GitHub-authoritative field drift is corrected

- GIVEN a work item whose phase advanced in GitHub but whose Jira status lags
- WHEN reconciliation runs
- THEN the Jira status matches the phase per the mapping
- AND the GitHub phase is unchanged

#### Scenario: Second pass makes zero changes

- GIVEN a work item that was just fully reconciled
- WHEN reconciliation runs a second time
- THEN no field is written in either system

### Requirement: Safety and Approval Gates

All writes performed by intake, projection, artifact sync, narrative sync, and
reconciliation SHALL respect `--dry-run` and the funnel's existing approval
gates. Under `--dry-run`, every would-be write SHALL be reported and none SHALL
be performed. No partial failure SHALL be silently swallowed: every failed write
SHALL be propagated or collected and reported.

#### Scenario: Dry-run performs no writes

- GIVEN the funnel runs with `--dry-run`
- WHEN intake, projection, and reconciliation execute
- THEN no GitHub or Jira write occurs
- AND each would-be write is reported

#### Scenario: Partial failure is reported

- GIVEN a reconciliation pass in which one Jira write fails
- WHEN the pass completes
- THEN the failure is reported
- AND it is not silently ignored

### Requirement: Credit Preservation

The Jira assignee SHALL remain the human owner throughout the funnel. Agents
SHALL NOT set themselves as the Jira assignee and SHALL NOT claim authorship of
the work in any Jira field or comment.

#### Scenario: Assignee is unchanged after a run

- GIVEN a Jira item assigned to its human owner
- WHEN the funnel processes the item through any phase
- THEN the Jira assignee is still the human owner

---

## Required Skill Changes

These are the desired behaviors the later code phase SHALL implement in the SDLC
skills. They are stated here as contract, not as code.

### `execute-sdlc` Step 0 and Step 1 gain a Jira-origin intake path

`execute-sdlc` SHALL accept a Jira source as part of its scope, alongside the
existing GitHub `$LABEL_SCOPE`. Before triage and work selection, Step 0 SHALL
run intake (see the new `ingest` responsibility) so that every in-scope Jira item
has a linked GitHub execution record. Step 1 work selection SHALL then choose
from execution records regardless of origin, using the existing
`agent/workable` gate. A Jira-origin item that has been ingested SHALL be
selectable exactly like a GitHub-origin item.

### `triage` discovers work from either origin

`triage` SHALL discover in-scope items from both the GitHub label scope and the
Jira source. For a Jira-origin item, triage SHALL operate on its GitHub execution
record after ingestion, applying the same `agent/needs-input`, `agent/workable`,
and `agent/duplicate` classification. Triage SHALL NOT read Jira items that are
outside the configured Jira source.

### New `ingest` responsibility

A new `ingest` responsibility SHALL ensure a linked GitHub execution record for a
Jira work item. It SHALL:

- Resolve the Jira source to the set of in-scope Jira items.
- For each item without a canonical link, create a GitHub execution record
  carrying the funnel scope label and create the bidirectional canonical link.
- Be create-if-missing and idempotent, returning the existing record when one is
  already linked.
- Respect `--dry-run` and the funnel approval gates.

`ingest` SHALL run before triage in the `execute-sdlc` workflow so that the rest
of the funnel operates uniformly on GitHub execution records.

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
- A GitHub-origin issue and a Jira-origin story that describe the same intent
  converge to the same end state: linked record, projected status, remote links,
  and attributed comments.

#### Scenario: Jira epic drives a full run

- GIVEN the Jira epic `HYPERSHELL-374` in a configured Jira source
- WHEN `execute-sdlc` runs with scope `agent/scope/mturansk`
- THEN the epic is ingested into a linked GitHub execution record
- AND the record proceeds through the SDLC phases
- AND the Jira item ends with the mapped status, the pull request as a remote
  link, attributed phase comments, and an unchanged assignee

#### Scenario: Two origins converge

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
