# Fullsend Lifecycle Label Hooks

**ID prefix:** SDLC-HOOK

**Status:** Active
**Applies to:** `.fullsend/` harnesses, agents, schemas, and scripts for the SDLC funnel phases (triage, spec implementation, spec review, code implementation, code review); local and cloud Fullsend execution
**Tracks:** [HYPERSHELL-469](https://redhat.atlassian.net/browse/HYPERSHELL-469)
**Related:** `specs/standards/sdlc/jira-github-sync.spec.md` (SDLC-JIRA: execution record, `agent/allowed` gate, `agent/execution-record` marker, phase-to-status mapping), `specs/platform/agent-sdlc-visibility-dashboard.spec.md` (SDLC-VIS: reads the phase labels this spec maintains)

---

## Purpose

The agent SDLC funnel moves a GitHub execution record through phases with a
label state machine. When a model applies those labels from prose instructions,
it must remember every required label step and its order. An agent can finish
its task and leave a label update incomplete, a sequence of separate API calls
can fail partway through a transition, and conflicting labels can prevent a
trigger from matching or leave the next phase unclear.

Lifecycle label transitions therefore live in shared, deterministic scripts that
native Fullsend harness hooks invoke, not in model instructions.
An agent SHALL perform its assigned work and return a structured result only.
Fullsend SHALL validate that result. A trusted post-script SHALL then apply the
permitted label changes and verify the final state. The behavior SHALL live under
`.fullsend/` so the same harnesses run identically in local and cloud Fullsend
execution.

This is a cross-cutting process standard. It constrains the `.fullsend/`
harnesses, agents, schemas, and scripts and defines the behavior they SHALL
implement. It is a desired-state contract, independent of when any given harness
or agent change lands. It complements SDLC-JIRA: SDLC-JIRA owns the Jira-and-
GitHub synchronization, the `agent/allowed` authorization gate, the
`agent/execution-record` canonical-link marker, and the phase-to-status mapping;
this specification owns HOW the GitHub phase labels those depend on are applied -
by shared scripts through harness hooks, from a validated result, never by free-
hand agent action.

---

## Label Categories

Not every `agent/*` label represents the same kind of state. A transition SHALL
treat each category differently:

- **Phase labels** carry the funnel phase and its within-phase outcome:
  `agent/workable`, `agent/needs-input`, `agent/duplicate`,
  `agent/reviewable-spec`, `agent/review-spec-approved`,
  `agent/review-spec-rejected`, `agent/code-conflicts`, `agent/reviewable-code`,
  `agent/review-code-approved`, `agent/review-code-rejected`. A transition MAY
  add and remove phase labels within the rules of the transition contract.
- **Scope labels** (`agent/scope/*`) select an item for a run. A transition SHALL
  NEVER add or remove a scope label.
- **Authorization** (`agent/allowed`) is the human authorization gate
  (see SDLC-JIRA-01). A transition SHALL NEVER add or remove `agent/allowed`.
- **Execution record marker** (`agent/execution-record`) is the canonical-link
  marker (see SDLC-JIRA-04). A transition SHALL NEVER add or remove it.
- **Human blocks** (`agent/blocked` and any human-applied spec or code block gate)
  represent a human decision to hold the record. A transition SHALL NOT silently
  remove a human-applied block.
- **Unrelated labels** are every other label on the record (triage categories a
  human added, release markers, free-form labels). A transition SHALL preserve
  them unchanged.

Within a single review phase, the `reviewable`, `approved`, and `rejected`
outcomes are mutually exclusive: at most one of `agent/reviewable-spec`,
`agent/review-spec-approved`, and `agent/review-spec-rejected` SHALL be active at
a time, and likewise at most one of `agent/reviewable-code`,
`agent/review-code-approved`, and `agent/review-code-rejected`. A completed
prerequisite that a later phase needs (for example `agent/review-spec-approved`
when the code phase runs) SHALL be preserved until the transition that
legitimately supersedes it.

---

## The Transition Contract

A **transition** is a named, allowed movement of a record from a required
starting state to an expected final state, caused by one phase outcome. The
contract documents every allowed outcome for triage, spec implementation, spec
review, code implementation, and code review. For each outcome it defines: the
required starting state, the labels to add, the labels to remove, the labels that
MUST remain unchanged, the evidence required before applying it, and the expected
final state. The post-script SHALL apply only the label changes that the matched
transition names, and no others.

The representative contract below is normative for the named outcomes. Each
"remove" list is the set of superseded phase labels only; scope, authorization,
execution-record, human-block, and unrelated labels are preserved in every case.

| Phase | Outcome | Required start | Add | Remove (superseded phase labels) |
|-------|---------|----------------|-----|----------------------------------|
| Triage | workable | triaged-eligible, no phase label | `agent/workable` | none |
| Triage | needs-input | triaged-eligible, no phase label | `agent/needs-input` | none |
| Triage | duplicate | triaged-eligible, no phase label | `agent/duplicate` (and close) | none |
| Spec implementation | completed | `agent/workable` or `agent/review-spec-rejected` | `agent/reviewable-spec` | `agent/workable`, `agent/review-spec-rejected`, stale `agent/review-spec-approved` |
| Spec review | approved | `agent/reviewable-spec` | `agent/review-spec-approved` | `agent/reviewable-spec`, `agent/review-spec-rejected` |
| Spec review | rejected (under limit) | `agent/reviewable-spec` | `agent/review-spec-rejected` | `agent/reviewable-spec`, `agent/review-spec-approved` |
| Spec review | rejected (limit reached) | `agent/reviewable-spec` | `agent/review-spec-rejected`, `agent/blocked` | `agent/reviewable-spec`, `agent/review-spec-approved` |
| Code implementation | completed | `agent/review-spec-approved`, `agent/review-code-rejected`, or `agent/code-conflicts` | `agent/reviewable-code` | `agent/review-code-rejected`, stale `agent/review-code-approved`, `agent/code-conflicts` |
| Code review | approved | `agent/reviewable-code` | `agent/review-code-approved` | `agent/reviewable-code`, `agent/review-code-rejected` |
| Code review | rejected (under limit) | `agent/reviewable-code` | `agent/review-code-rejected` | `agent/reviewable-code`, `agent/review-code-approved` |
| Code review | rejected (limit reached) | `agent/reviewable-code` | `agent/review-code-rejected`, `agent/blocked` | `agent/reviewable-code`, `agent/review-code-approved` |

The code-implementation completed transition preserves `agent/review-spec-approved`:
the spec approval is a completed prerequisite the record still carries while the
code phase runs. It removes any stale `agent/review-code-approved` so a new
revision of the code is reviewable again.

---

## Hook Ordering

Each phase harness SHALL use the native Fullsend hooks in this order, with
stage-specific configuration drawn from trusted harness input:

1. A shared **pre-script** establishes and records the starting state before the
   agent runs (see SDLC-HOOK-03).
2. The **agent** performs its work and writes a structured result only (see
   SDLC-HOOK-05).
3. A native **validation_loop** validates the result read-only before anything is
   applied (see SDLC-HOOK-06).
4. A shared **post-script** consumes only a validated result, checks evidence and
   current state, and applies the matched transition, then verifies it (see
   SDLC-HOOK-07, SDLC-HOOK-08).

---

## Requirements

### Requirement: SDLC-HOOK-01 -- Shared Lifecycle Transition Contract

Lifecycle phase-label transitions for triage, spec implementation, spec review,
code implementation, and code review SHALL be defined by a single shared
transition contract and applied by shared scripts that the harnesses invoke
through native Fullsend hooks. The contract SHALL enumerate, for every allowed
outcome of each phase, the required starting state, the labels to add, the labels
to remove, the labels that MUST remain unchanged, the evidence required before
applying the transition, and the expected final state. Blocking outcomes and the
existing review retry limits SHALL be part of the contract (see SDLC-HOOK-12).

A transition SHALL apply only the label changes its matched outcome names. Phase
labels that represent competing outcomes within one phase SHALL NOT remain active
together: a transition SHALL leave at most one of `reviewable`, `approved`, and
`rejected` active for its phase. A completed prerequisite that a later phase needs
SHALL be preserved until a transition legitimately supersedes it.

#### Scenario: SDLC-HOOK-01.01 -- Rejected code review ends with one code phase label

- GIVEN a record carrying `agent/reviewable-code`
- AND, from an earlier revision, `agent/review-code-approved`
- WHEN the code-review rejected transition is applied under the retry limit
- THEN `agent/reviewable-code` and `agent/review-code-approved` are removed
- AND `agent/review-code-rejected` is present
- AND no other label on the record is changed

#### Scenario: SDLC-HOOK-01.02 -- Completed code implementation re-opens review for a new revision

- GIVEN a record carrying `agent/review-spec-approved` and `agent/review-code-rejected`
- WHEN the code-implementation completed transition is applied
- THEN `agent/review-code-rejected` and any stale `agent/review-code-approved` are removed
- AND `agent/reviewable-code` is present
- AND `agent/review-spec-approved` is preserved as a completed prerequisite

#### Scenario: SDLC-HOOK-01.03 -- Approved spec review leaves only the approval

- GIVEN a record carrying `agent/reviewable-spec`
- WHEN the spec-review approved transition is applied
- THEN `agent/reviewable-spec` and any `agent/review-spec-rejected` are removed
- AND `agent/review-spec-approved` is present

### Requirement: SDLC-HOOK-02 -- Protected Label Categories

A transition SHALL classify every label by category and SHALL mutate only phase
labels. A transition SHALL NEVER add or remove a scope label (`agent/scope/*`),
the authorization gate (`agent/allowed`), or the execution-record marker
(`agent/execution-record`). A transition SHALL NOT silently remove a human-applied
block (`agent/blocked` or a human spec or code block gate). A transition SHALL
preserve every unrelated label unchanged.

A transition SHALL NOT replace the full label set of a record from a stored
snapshot. It SHALL add and remove only the specific labels its outcome names, so
labels a human applied during the run survive.

#### Scenario: SDLC-HOOK-02.01 -- Scope and authorization labels survive a transition

- GIVEN a record carrying `agent/scope/<user>`, `agent/allowed`, and `agent/execution-record`
- WHEN any phase transition is applied
- THEN `agent/scope/<user>`, `agent/allowed`, and `agent/execution-record` remain unchanged

#### Scenario: SDLC-HOOK-02.02 -- A human block added during the run is preserved

- GIVEN a record whose starting state was captured before the agent ran
- AND a human adds `agent/blocked` while the agent is running
- WHEN the post-script applies the matched transition
- THEN `agent/blocked` is still present
- AND the transition does not restore the full label set from the captured snapshot

#### Scenario: SDLC-HOOK-02.03 -- Unrelated labels are untouched

- GIVEN a record carrying an unrelated human-applied label
- WHEN any phase transition is applied
- THEN the unrelated label is unchanged

### Requirement: SDLC-HOOK-03 -- Harness Pre-Scripts Establish Starting State

Each phase harness SHALL run a shared pre-script, with stage-specific
configuration, that fetches the current issue and linked pull request state
before the agent starts. The pre-script SHALL confirm authorization
(`agent/allowed`), scope, the stage prerequisites, and the issue-to-pull-request
relationship. It SHALL record the relevant starting state, including the pull
request head commit, for later checks. When the prerequisites no longer hold, the
pre-script SHALL use Fullsend's supported skip mechanism to prevent the agent from
running.

The pre-script SHALL take its context from native Fullsend inputs and environment
configuration. It SHALL NOT require local dispatcher state or a local database.

#### Scenario: SDLC-HOOK-03.01 -- Prerequisites no longer hold

- GIVEN a code-review harness whose trigger fired on `agent/reviewable-code`
- AND a human removed `agent/allowed` before the agent started
- WHEN the pre-script runs
- THEN it detects the missing authorization
- AND it skips the run through the supported Fullsend mechanism
- AND the agent does not run

#### Scenario: SDLC-HOOK-03.02 -- Starting state and pull request head are recorded

- GIVEN a review harness about to run against a linked pull request
- WHEN the pre-script runs
- THEN it records the starting phase labels and the pull request head commit
- AND that recorded state is available to the post-script

Pre-script skipping SHALL use Fullsend's native `skipped=true` and `reason`
outputs. An arbitrary `skip=<reason>` output SHALL NOT be treated as a skip.
Custom outputs SHALL NOT be assumed to become post-script environment variables.
The trusted phase/record/PR identity SHALL instead be handed off through a private
host-owned, atomically written run snapshot, outside sandbox/model output.
The post-script SHALL validate file permissions, ownership, and identity binding
before using it; it SHALL NOT source the contents as code. Missing/invalid review
state SHALL fail the attempt rather than report a successful no-op.

#### Scenario: SDLC-HOOK-03.03 -- Native skip prevents execution

- GIVEN a missing authorization or stage prerequisite
- WHEN the pre-script runs through the installed Fullsend CLI
- THEN Fullsend SHALL honor `skipped=true` and the reason
- AND no sandbox or model run SHALL start

#### Scenario: SDLC-HOOK-03.04 -- Separate hook processes share trusted state

- GIVEN a pre-script that records the reviewed head in the host run root
- WHEN the post-script starts in a separate environment without relay exports
- THEN it SHALL read and verify that private snapshot for the same phase and record
- AND missing, mismatched, or non-private state SHALL fail without advancing labels

### Requirement: SDLC-HOOK-04 -- Structured, Versioned Agent Results

Each phase SHALL define a versioned JSON schema for its agent result. The result
SHALL include the outcome, a rationale, the relevant pull request and commit, and
the evidence the stage requires. The outcome SHALL be one of a fixed list of
permitted values for that phase. The result SHALL NOT supply arbitrary label
names, repository targets, or shell commands.

The target repository and issue SHALL be derived from trusted harness input, not
from the result. Any identifier the agent returns (pull request number, commit)
SHALL be verified against the trusted input before it is used.

#### Scenario: SDLC-HOOK-04.01 -- Outcome is a fixed enum

- GIVEN a spec-review result whose outcome is not one of the permitted spec-review values
- WHEN the result is validated
- THEN validation fails
- AND no transition is applied

#### Scenario: SDLC-HOOK-04.02 -- A result cannot name an arbitrary label

- GIVEN a result that includes an arbitrary label name
- WHEN the post-script applies the transition
- THEN the label set applied is derived from the matched outcome in the contract
- AND the arbitrary label name is ignored

#### Scenario: SDLC-HOOK-04.03 -- Returned identifiers are verified against trusted input

- GIVEN a result whose pull request number differs from the one in the trusted harness input
- WHEN the post-script processes the result
- THEN the mismatch is detected
- AND the transition is not applied

### Requirement: SDLC-HOOK-05 -- Agents Produce Results, Not Label Changes

The implementation and review agents SHALL produce the structured result defined
for their phase and SHALL NOT add or remove lifecycle labels directly. Agent
instructions SHALL describe producing the result, and no referenced instruction
SHALL tell an agent to change a lifecycle label.

#### Scenario: SDLC-HOOK-05.01 -- An agent does not write a lifecycle label

- GIVEN a code-review agent that has reached a verdict
- WHEN it finishes its task
- THEN it writes its verdict and evidence to its structured result
- AND it does not add or remove any `agent/*` lifecycle label itself

### Requirement: SDLC-HOOK-06 -- Read-Only Validation Before Application

Each phase harness SHALL configure a native Fullsend `validation_loop` that
validates the result format and required fields before any lifecycle label change.
Validation SHALL use bounded retries and SHALL return useful feedback for invalid
output. Validation SHALL be read-only: it SHALL NOT change any label while deciding
whether the result is valid.

The post-script SHALL consume only an output that passed validation, using the
supported Fullsend output contract for the deployed version. A missing, malformed,
failed, or unvalidated result SHALL NOT advance the workflow and SHALL NOT cause
any label change.

#### Scenario: SDLC-HOOK-06.01 -- Malformed result is rejected without mutation

- GIVEN an agent result that is not valid JSON
- WHEN the validation loop runs
- THEN validation fails
- AND no label is changed during validation
- AND the post-script makes no label change

#### Scenario: SDLC-HOOK-06.02 -- Unvalidated result does not advance the workflow

- GIVEN a run that produced no validated output
- WHEN the post-script runs
- THEN it applies no transition
- AND the record's phase labels are unchanged

### Requirement: SDLC-HOOK-07 -- Evidence Checked Against GitHub

A schema-valid result SHALL NOT be treated as proof that the work is complete.
Before applying a transition, the post-script SHALL check the relevant evidence
against GitHub, including the linked pull request and the commit that was
implemented or reviewed. The contract SHALL define how required checks affect
eligibility to advance. Code approval requires an open, non-draft PR with known
mergeability, no confirmed conflicts, green required checks, and an unchanged
reviewed head. The head and base SHALL be checked again before granting the PR
queue label. Unknown mergeability SHALL defer approval, not authorize it.
Required contexts SHALL include the union of classic branch protection and all
active rulesets applying to the base branch, including paginated rules. Failed
API response bodies SHALL NOT be interpreted as check names. If neither source
supplies contexts, the gate SHALL require all reported check-run and commit-status
signals to pass, and SHALL defer if no signals have reported. A missing required
context SHALL remain pending; an advisory context SHALL NOT block a known set
of green required contexts.

A review result SHALL be rejected as stale if the reviewed pull request head
commit changed after the review. A stale review result SHALL NOT advance the
workflow.

#### Scenario: SDLC-HOOK-07.01 -- Stale review result is rejected

- GIVEN a code-review result for a pull request head commit recorded by the pre-script
- AND the pull request head advanced to a new commit before the post-script runs
- WHEN the post-script processes the result
- THEN it detects the changed head
- AND it rejects the review result as stale
- AND it applies no code-review verdict transition

#### Scenario: SDLC-HOOK-07.02 -- Required checks gate eligibility

- GIVEN a validated code-review approval whose required checks are pending or unknown
- WHEN the post-script evaluates eligibility
- THEN approval is deferred with a visible record and the item stays reviewable
- AND no rejection is counted

#### Scenario: SDLC-HOOK-07.03 -- Ruleset protection with a classic 404

- GIVEN classic branch protection returns HTTP 404 with a JSON error body
- AND applicable rulesets define required status checks
- WHEN the gate evaluates a validated approval
- THEN it uses the ruleset contexts and discards the classic error body
- AND approval is eligible only when all required contexts are green
- AND an unreported or failing required context still prevents approval

#### Scenario: SDLC-HOOK-07.04 -- Failed required CI requests implementation

- GIVEN a fresh validated code-merit approval with a completed failed required check
- WHEN the post-script evaluates eligibility
- THEN it applies `code-review/rejected`, not a deferred approval
- AND its required review record explains the failed CI and implementation handoff
- AND it revokes the PR queue label and applies the normal timeline rejection limit

### Requirement: SDLC-HOOK-08 -- Post-Script Applies and Verifies Transitions

A shared post-script, invoked by each phase harness, SHALL apply transitions.
Before applying a result it SHALL fetch the current issue and pull request state
again and confirm the result still applies to the current work. It SHALL apply
only the changes the matched transition allows. It SHALL verify the final labels
through a fresh API read and SHALL report success only after that verification.

A transition that cannot finish SHALL return an actionable error. Partial failure
SHALL NOT be reported as success, and a run SHALL NOT report a successful
transition when the labels were not applied.

#### Scenario: SDLC-HOOK-08.01 -- Final state is verified before success is reported

- GIVEN a validated, non-stale result whose transition is eligible
- WHEN the post-script applies the transition
- THEN it re-reads the labels from GitHub
- AND it confirms the expected final state
- AND only then reports success

#### Scenario: SDLC-HOOK-08.02 -- A label-application failure is visible

- GIVEN a transition whose label write fails
- WHEN the post-script completes
- THEN it returns an actionable error
- AND it does not report the transition as successful

### Requirement: SDLC-HOOK-09 -- Idempotent and Interruption-Safe Application

Applying the same validated result again SHALL produce the same final state
without duplicate comments or other repeated side effects. The post-script SHALL
define safe behavior for interrupted or overlapping transitions and SHALL NOT
assume that several GitHub API calls form a transaction. It SHALL use bounded
retries for transient API failures.

#### Scenario: SDLC-HOOK-09.01 -- Reapplying a validated result changes nothing new

- GIVEN a validated result that was already applied and verified
- WHEN the same validated result is applied again
- THEN the final label state is identical
- AND no duplicate comment or other side effect is produced

#### Scenario: SDLC-HOOK-09.02 -- An interrupted transition converges on reapplication

- GIVEN a transition that was interrupted after adding its label but before removing a superseded one
- WHEN the validated result is applied again
- THEN the record reaches the transition's expected final state

### Requirement: SDLC-HOOK-10 -- Recovery Through Fullsend Artifacts

Fullsend run artifacts SHALL retain enough of the validated result and transition
information to diagnose and safely retry a failed application step. There SHALL be
a documented way to reapply a validated result without starting the agent again,
where the deployed Fullsend interfaces support it; reapplication SHALL recheck the
current state before it acts. If reapplication needs an upstream capability that is
not available, the gap SHALL be documented rather than satisfied by a dependency
on the local dispatcher.

Recovery SHALL clearly distinguish agent failure, invalid output, stale output,
and label-application failure.

#### Scenario: SDLC-HOOK-10.01 -- A validated result is reapplied without rerunning the agent

- GIVEN a run whose agent produced a validated result but whose label application failed
- WHEN an operator reapplies the validated result through the documented procedure
- THEN the agent is not started again
- AND the current state is rechecked before the transition is applied

#### Scenario: SDLC-HOOK-10.02 -- Failure classes are distinguished

- GIVEN a failed run
- WHEN its artifacts are inspected
- THEN agent failure, invalid output, stale output, and label-application failure are distinguishable

### Requirement: SDLC-HOOK-11 -- Local and Cloud Parity, Single Writer

The transition scripts, result schemas, and harness configuration SHALL live
under `.fullsend/` so the same harnesses run in local and cloud Fullsend
execution. Required runner tools and the GitHub credential with permission to
update issue labels SHALL be declared through supported Fullsend configuration.
The behavior SHALL NOT depend on workstation paths, interactive sign-in state,
local dispatcher environment setup, or a local database.

Where intake or reconciliation can also write these lifecycle labels, those writes
SHALL be routed through the same transition contract or that responsibility SHALL
be removed, so there are no competing writers of the phase labels.

#### Scenario: SDLC-HOOK-11.01 -- Same harness runs locally and in cloud

- GIVEN the harnesses, scripts, and schemas under `.fullsend/`
- WHEN a phase runs in local Fullsend execution and in cloud Fullsend execution
- THEN both use the same transition contract and scripts
- AND neither depends on the local dispatcher, its shell launcher, or its Go program

#### Scenario: SDLC-HOOK-11.02 -- No competing writer of phase labels

- GIVEN reconciliation that could set a phase label
- WHEN it needs to change a phase label
- THEN it routes the change through the shared transition contract
- OR it does not write the phase label at all

### Requirement: SDLC-HOOK-12 -- Blocking Outcomes and Review Retry Limits

The contract SHALL define the blocking outcomes. When spec review rejects a record
3 or more times since the most recent removal of `agent/blocked` (or since the
start of the timeline if `agent/blocked` was never removed), the transition SHALL
add `agent/blocked` in addition to `agent/review-spec-rejected`. When code review
rejects a record 10 or more times under the same counting rule, the transition
SHALL add `agent/blocked` in addition to `agent/review-code-rejected`. The rejection
count SHALL be evidence read from the issue timeline, not a value supplied by the
agent result.

A blocked record SHALL stay blocked until a human removes `agent/blocked` and the
superseding rejection label, and the funnel SHALL NOT retry it while blocked.

#### Scenario: SDLC-HOOK-12.01 -- Third spec rejection blocks the record

- GIVEN a record that spec review has rejected twice since the last `agent/blocked` removal
- WHEN spec review rejects it a third time
- THEN the transition adds `agent/review-spec-rejected` and `agent/blocked`
- AND `agent/reviewable-spec` is removed

#### Scenario: SDLC-HOOK-12.02 -- Rejection count comes from the timeline, not the result

- GIVEN a code-review rejection result
- WHEN the post-script decides whether to block
- THEN it counts prior `agent/review-code-rejected` events from the issue timeline
- AND it does not take the count from the agent result

### Requirement: SDLC-HOOK-14 -- Code Readiness, Queue Labels, and Later Recovery

The shared observation contract SHALL include PR head and base commits, open/closed
state, draft status, mergeability and mergeability state, and the normalized
required-check result. Local discovery and pre-launch authorization SHALL use this
state in code-review and implementation fingerprints. A successful review attempt
that deferred approval on pending checks SHALL become eligible again when CI
changes to green, even if issue content, labels, and PR head are unchanged.
The local runner MAY rerun review; it SHALL NOT silently treat the earlier process
exit as proof that approval was applied. Manual replay of the validated result
remains supported while its evidence is current.

With automatic merge approval enabled (the default), an eligible code-review
post-script SHALL apply and verify
`agent/review-code-approved` on both the linked PR and execution record. It SHALL
verify the PR link and current authorization, head, base, checks, draft status,
and mergeability. It SHALL label the PR before completing the issue transition;
failure to label the PR SHALL fail the attempt and leave recovery possible.
Reapplication SHALL repair a missing PR label even when the issue already has its
approval. Rejected or ineligible approval results SHALL NOT retain the PR queue
label. A changed head SHALL require fresh review; a stale result SHALL never
approve the new head.

Trusted runner policy `FULLSEND_AUTO_MERGE=disabled` SHALL preserve approval on
an eligible execution record while withholding/removing the linked PR approval
label and verifying its absence. Enabled policy SHALL retain dual-target approval.
The policy SHALL NOT originate in agent output or alter the agent's responsibilities.
Invalid policy values SHALL fail before mutation. Both policies SHALL retain all
freshness, authorization, readiness, rejection and dry-run gates. Reapplying a
fresh approved result with disabled policy SHALL converge without queuing its PR.

A shared maintenance hook under `.fullsend/scripts/` SHALL revisit code-stage
records after approval, because a post-script alone cannot observe future changes.
The local watch loop SHALL call this hook during discovery, including when no
agent phase currently matches an approved issue. Hosted schedulers SHALL invoke
the same hook from periodic or PR/check-change processing to obtain the same
recovery behavior. Inspect/dry-run calls SHALL be read-only. The local runner
SHALL avoid recovery mutations on records with running or uncertain worker ownership.

For confirmed conflicts (`mergeable: false`, `mergeable_state: dirty`), the hook
SHALL revoke the PR queue label first, then converge the execution record onto
`agent/code-conflicts`, removing stale code approval/rejection/reviewable labels.
The code implementation harness and pre-script SHALL accept this trigger; completed
repair SHALL clear it and return the record to code review. Conflict detection
SHALL NOT count as a review rejection. For a reviewable or previously approved PR
whose required checks fail, the hook
SHALL revoke queue eligibility and converge onto `agent/review-code-rejected`,
remove superseded code approval/reviewable labels, and request implementation.
It SHALL post a visible CI-recovery record before applying the rejection and use
the shared timeline rejection budget. Repeated polls SHALL NOT add rejections.
A rerun that changes failing checks to pending SHALL invalidate the failure
observation before writes. For a previously approved PR whose checks become
pending/unknown or whose mergeability becomes unknown, the hook SHALL revoke
queue eligibility and reopen code review without asserting failure or conflict.
Closed PRs, revoked authorization/scope, human blocks, changing observations, and
ambiguous PR linkage SHALL NOT authorize recovery writes. Recovery uses the same
pure lifecycle transition contract as the post-script. It SHALL be idempotent
across polling, retries, and partial GitHub writes.

#### Scenario: SDLC-HOOK-14.01 -- CI finishes after deferred review

- GIVEN an approved review attempt that exited successfully while CI was pending
- AND the issue and reviewed PR head have not changed
- WHEN required checks become green
- THEN the new readiness fingerprint SHALL permit another code-review attempt
- AND its post-script SHALL grant approval only after rechecking live evidence

#### Scenario: SDLC-HOOK-14.02 -- Conflicts appear after green CI and approval

- GIVEN an approved PR whose base branch subsequently advances
- WHEN GitHub confirms merge conflicts during the next maintenance observation
- THEN the PR queue label SHALL be revoked before issue-state changes
- AND the issue SHALL become eligible for code implementation through `agent/code-conflicts`
- AND repair SHALL require another code review before queue approval is restored
- AND repeated polling SHALL NOT add review rejections or duplicate active work

#### Scenario: SDLC-HOOK-14.03 -- PR approval write fails

- GIVEN an eligible review result for an explicitly linked PR
- WHEN applying the PR approval label fails
- THEN the attempt SHALL fail without completing the issue approval transition
- AND reapplying the same fresh result SHALL be able to complete both labels

#### Scenario: SDLC-HOOK-14.04 -- Uncertain or unauthorized recovery

- GIVEN unknown mergeability, revoked authorization, a human block, or a changed head/base
- WHEN the maintenance hook evaluates the record
- THEN it SHALL NOT interpret unknown mergeability as confirmed conflicts
- AND revoked authorization, blocks, and changing observations SHALL cause no recovery writes

#### Scenario: SDLC-HOOK-14.05 -- Failed CI recovers a deferred review

- GIVEN a successful review process whose approval was deferred
- WHEN required CI is confirmed failing at discovery with no running worker
- THEN maintenance marks the execution record code-review rejected
- AND code implementation becomes eligible without a fresh review LLM
- AND unchanged polling neither posts duplicate records nor consumes more retries

### Requirement: SDLC-HOOK-13 -- Automated Tests and Documentation

GitHub pagination tests SHALL model the installed CLI restriction against using
`--slurp` with `--jq` or `--template`. Timeline rejection counts and latest check
results SHALL include all pages; JSON filtering MAY run through standalone `jq`
after page acquisition, preserving request and parse failures.

Review harness preflight commands SHALL be accepted by Fullsend's native host
environment validation. Internal shell temporaries SHALL NOT be expressed as
host-variable templates. Tests with an installed Fullsend binary SHALL load both
review harnesses using dummy host settings and prevent all gateway execution,
so shell tests alone cannot miss interpolation errors in the native loader.


The transition behavior SHALL be covered by automated tests that exercise all
allowed transitions and invalid starting states, including: missing or malformed
JSON and unsupported outcomes; incorrect issue or pull request references; a pull
request head change after review; a human block or authorization change during a
run; conflicting phase labels and preservation of unrelated labels; partial API
failure, interrupted application, and repeated application of one result; failed
validation preventing any post-script mutation; and successful application being
verified before the run reports completion.

There SHALL be a native Fullsend harness integration test that does not invoke the
local dispatcher. The result contract, the transition rules, the required runner
tools and permissions, and the recovery procedure SHALL be documented.

#### Scenario: SDLC-HOOK-13.01 -- Failed validation prevents post-script mutation is tested

- GIVEN the automated test suite for the transition scripts
- WHEN it runs
- THEN it includes a case where failed validation prevents any post-script label change
- AND a case where a repeated application of one result produces no new side effect

#### Scenario: SDLC-HOOK-13.02 -- Harness integration test avoids the local dispatcher

- GIVEN the native Fullsend harness integration test
- WHEN it runs
- THEN it exercises the harness hooks without invoking the local dispatcher

---

## Acceptance Criteria

- Lifecycle label transitions are implemented by shared scripts invoked through
  native Fullsend harness hooks, not by agent prose.
- Agents return structured, versioned results and do not directly manage
  lifecycle labels.
- Every result is validated read-only before any lifecycle label change.
- A missing, malformed, failed, stale, or unvalidated result cannot advance the
  workflow.
- Allowed transitions produce the documented final state, keep the within-phase
  outcomes mutually exclusive, and preserve scope, authorization, execution-record,
  human-block, and unrelated labels.
- Reapplying a validated result is safe and produces no duplicate comment or other
  repeated side effect.
- Failures remain visible, are not reported as success, and carry enough context
  for recovery; agent failure, invalid output, stale output, and label-application
  failure are distinguishable.
- The same harnesses work in local and cloud Fullsend execution without depending
  on the local dispatcher, its shell launcher, or its Go program.
- Intake and reconciliation do not compete as a second writer of the phase labels.
- Automated tests cover the allowed transitions, the invalid starting states, and
  failure recovery, and a harness integration test runs without the local
  dispatcher.

---

## Design Decisions

| Decision | Rationale |
|----------|-----------|
| Label transitions live in shared scripts, not agent prose | A deterministic script applies the same labels in the same order every time. Prose leaves the model responsible for remembering every step, so a finished task can still leave a transition half-applied. |
| Agents return a validated structured result | A fixed-enum, versioned result keeps arbitrary label names, repository targets, and shell commands out of the model output and gives the post-script a trusted contract to act on. |
| Validation is read-only and precedes any mutation | Deciding whether output is valid must never change workflow state, so an invalid or unvalidated result cannot advance the funnel or leave labels half-changed. |
| Evidence is checked against GitHub before applying | A schema-valid result is not proof of completed work. Rechecking the pull request and head commit rejects stale reviews and prevents advancing on work that changed. |
| The post-script re-reads and verifies the final state | Several GitHub API calls are not a transaction. Re-reading after applying, and reporting success only after verification, keeps a partial failure visible instead of falsely successful. |
| Only the named label changes are applied, never a snapshot replace | Replacing the full label set from an old snapshot would erase a human block or authorization change made during the run. Adding and removing only the named labels preserves human decisions. |
| Protected label categories are never mutated by a transition | Scope, authorization, execution-record, and human-block labels carry decisions outside one phase. Treating them as protected stops a phase transition from undoing a human gate or the canonical link. |
| Retry counts come from the issue timeline | The number of prior rejections is authoritative evidence on GitHub, not a value the agent can assert, so the block decision cannot be gamed by the result. |
| Behavior lives under `.fullsend/`, not the local dispatcher | Keeping scripts, schemas, and configuration under `.fullsend/` lets the same harnesses run identically in local and cloud Fullsend execution, with no dependency on a workstation or a local database. |
| One writer for phase labels | Routing intake and reconciliation label writes through the same contract, or removing that responsibility, prevents competing writers from leaving conflicting phase labels. |
