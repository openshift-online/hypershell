# Local SDLC Dispatch

**Date:** 2026-10-30
**Status:** Implemented in source
**ID prefix:** SDLC-LOC
**Related:** [Local development](local-development.spec.md),
[Jira/GitHub synchronization](../standards/sdlc/jira-github-sync.spec.md),
[Agent runtime images](agent-runtime-images.spec.md)

## Purpose and scope

Developers SHALL be able to run the repository's Fullsend agents against a remote
OpenShell gateway from their checkout, observe progress, detach, and resume.
Local dispatch owns scheduling, credentials, and presentation. It does not define
alternate agent responsibilities or replace the Jira/GitHub authority rules.
The terminal dashboard is distinct from the web-console SDLC dashboard.

## Requirements

### Requirement: SDLC-LOC-01 -- Entry points and registered scope

`make fullsend` SHALL start or resume watch mode using an explicitly configured
`agent/scope/*` label. It SHALL verify that the exact label exists in `REPOSITORY`
(default `openshift-online/hypershell`) before starting the dispatcher. Missing
scope, missing label, or failed verification SHALL fail with actionable guidance.
Scope registration SHALL NOT grant item authorization; writes still require
`agent/allowed`. `fullsend-help`, `fullsend-status`, and `fullsend-doctor` SHALL
provide readable command help, saved job state, and setup diagnostics respectively.
Status SHALL identify its data as persisted state, not live gateway verification.
A `fullsend-plan` Make target SHALL NOT be exposed.

#### Scenario: SDLC-LOC-01.01 -- Unknown scope

- GIVEN a scope whose label cannot be verified in the selected repository
- WHEN the developer runs `make fullsend`
- THEN no dispatcher or workers SHALL start
- AND the error SHALL explain how to configure or register the scope

### Requirement: SDLC-LOC-02 -- Configuration and credential setup

The launcher SHALL accept exported environment variables and an optional trusted
shell env file, defaulting to ignored `.hypershell/fullsend.env`. Explicit Make
arguments SHALL override the file, which SHALL override inherited environment.
An explicitly selected missing file SHALL fail. Setup documentation SHALL cover
GitHub authentication, Vertex project/region/model access and ADC, Jira, gateway
access, and required tools. Standard gcloud ADC discovery SHALL work without
requiring a separate credential path; an explicitly invalid path SHALL fail.
Jira SHALL support API-token credentials and an authenticated acli session, with
`https://redhat.atlassian.net` as the default tenant. Credentials SHALL NOT be
written to committed configuration, persisted job payloads, command arguments,
or diagnostic output. OAuth refresh SHALL remain outside agent execution;
workers SHALL receive only a private snapshot required for their attempt.

#### Scenario: SDLC-LOC-02.01 -- Explicit configuration wins

- GIVEN inherited settings and a selected env file
- WHEN a developer supplies a Make override for the same setting
- THEN the override SHALL win without interpreting its value as shell code

### Requirement: SDLC-LOC-03 -- Shared agent and harness definitions

Local dispatch SHALL reuse the canonical `ingest` and `reconcile` agent names,
prompts, and harness manifests. There SHALL NOT be separate local agent prompts
or complete local copies of those manifests. The runner MAY derive a private
per-attempt configuration that overrides scheduling and Jira transport. Image,
role, model, effort, timeout, task inputs, and result validation SHALL come from
the canonical harness. Runtime-specific providers, profiles, and temporary
resources SHALL use `local-dispatch-*` names. Existing state identifiers MAY be
retained for compatibility. Agent prompts SHALL describe tasks and tool contracts,
not launcher identity, host credential refresh, or sandbox setup procedures.

#### Scenario: SDLC-LOC-03.01 -- Canonical settings propagate

- GIVEN a change to a canonical lifecycle harness's model, role, or timeout
- WHEN the local runner derives its attempt configuration
- THEN the changed settings SHALL be preserved
- AND the source manifest and agent prompt SHALL remain unmodified

### Requirement: SDLC-LOC-04 -- Durable ownership and detachment

Job identities, attempts, outcomes, retry timing, and log paths SHALL persist
across dispatcher restarts. Concurrent dispatchers SHALL coordinate ownership so
that the same active job is not launched twice. Ctrl-C or quit SHALL detach the
UI without killing workers or deleting their sandboxes, and print complete log
paths, including failures. Restarting SHALL adopt monitoring of surviving workers
and use free capacity. An outcome that cannot be established SHALL be shown as
unknown and require explicit recovery rather than automatic replay. Renaming a
presented agent SHALL preserve existing ownership, history, and deduplication.

#### Scenario: SDLC-LOC-04.01 -- Resume with an active worker

- GIVEN a worker that survives the dispatcher's exit
- WHEN the dispatcher restarts or a second dispatcher is opened
- THEN it SHALL observe the existing run without duplicating it
- AND available capacity SHALL remain usable for other eligible work

### Requirement: SDLC-LOC-05 -- Continuous discovery and retry identity

Watch mode SHALL discover new work while existing workers run, subject to global
concurrency and per-record ownership. Discovery SHALL paginate the complete
selected issue set rather than stop at 200. Failed work SHALL use persisted
per-item backoff. Manual retry SHALL support failed and unknown-outcome jobs;
it SHALL NOT duplicate running work or bypass current authorization checks.
Successful work SHALL be deduplicated by relevant inputs. Implementation identity
SHALL include the linked PR head and the rejection occurrence so that returning
to an earlier label set after a new review rejection permits another attempt.

#### Scenario: SDLC-LOC-05.01 -- Repeated rejection

- GIVEN a successful implementation followed by a new review rejection
- WHEN labels return to the same values as the previous implementation input
- THEN the new rejection occurrence SHALL make another attempt eligible
- AND repeated discovery of that same occurrence SHALL remain deduplicated

### Requirement: SDLC-LOC-06 -- Complete lifecycle and explicit results

Jira mode SHALL run intake for the scope, reconcile authorized execution records
of either origin (including closed records), and then dispatch eligible phases.
Phase outcomes, changed inputs, and periodic drift checks SHALL trigger
reconciliation. Intake SHALL be repeated periodically for newly selected work.
Unchanged successful reconciliation SHALL NOT run on every discovery poll.
`github-only` mode SHALL explicitly omit Jira lifecycle work. Lifecycle agent
results SHALL contain `status`, `errors`, and `records`; missing output, reported
errors, or partial completion SHALL fail validation. PR resolution SHALL require
an explicit link or an exact closing reference and stop on multiple matches,
rather than selecting the newest PR. Scope and authorization SHALL be checked
again before dispatch and before agent writes.

#### Scenario: SDLC-LOC-06.01 -- Partial synchronization

- GIVEN a lifecycle attempt that performs some writes but reports an error
- WHEN the result is validated
- THEN the attempt SHALL be failed and subject to retry backoff
- AND dependent phase work SHALL remain gated on successful reconciliation

### Requirement: SDLC-LOC-07 -- Gateway recovery

The runner SHALL reuse a hidden, ignored gateway selection file and verify access
periodically, including when all worker slots are occupied. Failed access SHALL
pause new work, display the reason, and keep retrying without forgetting active
workers. Interactive recovery SHALL offer reconnect/sign-in, creation of a new
gateway, or continued waiting. Browser sign-in SHALL have a timeout and report
its outcome. Gateway creation SHALL preserve an existing gateway and avoid
replacing the connection used by active workers. If hsctl is missing, setup SHALL
offer an OS/architecture-appropriate release download with checksum verification.
Authentication recovery SHALL NOT blindly replay active jobs.

#### Scenario: SDLC-LOC-07.01 -- Authentication expires at capacity

- GIVEN all concurrency slots are occupied and gateway access expires
- WHEN the periodic check fails
- THEN existing worker ownership SHALL remain tracked
- AND the runner SHALL explain the failure and offer recovery while retrying

### Requirement: SDLC-LOC-08 -- Independent execution environments

Each worker SHALL receive an independent Git checkout that preserves applicable
working-tree edits without dangling host worktree metadata or shared writable
Git objects. Host credential configuration and hooks SHALL NOT be copied as Git
configuration. Runtime tooling SHALL be supplied by pinned harness images and
providers, not by instructions teaching agents how to repair the launcher.
Code and spec implementation harnesses SHALL allow 120 minutes; this SHALL NOT
implicitly increase review, intake, reconciliation, or other agent timeouts.

#### Scenario: SDLC-LOC-08.01 -- Linked worktree input

- GIVEN the developer launches from a linked Git worktree
- WHEN a worker checkout is prepared
- THEN Git operations SHALL work without access to the host's original `.git`
- AND the worker SHALL retain the developer's applicable source edits

### Requirement: SDLC-LOC-09 -- Cohesive terminal presentation

Overview, history, logs, details, help, and sign-in SHALL share the same visual
hierarchy, semantic colors, spacing, and terminal-sized frame. Scrollable regions
SHALL retain their height and expose scroll position. Color SHALL supplement text
and selection markers, not carry status alone; plain/no-color operation SHALL
remain understandable. Work SHALL be viewable by agent and by issue using actual
agent names, issue numbers, and titles with middle truncation when necessary.
Status, duration, and relative completion time SHALL provide temporal context.
Notifications SHALL use a consistent dismissible card; success notices SHALL
expire. Interactive sign-in choices SHALL support arrow-key selection.

#### Scenario: SDLC-LOC-09.01 -- Inspect a run

- GIVEN a selected issue with multiple agent attempts
- WHEN the developer opens logs or history
- THEN the issue and agent identity SHALL remain visible
- AND scrolling SHALL retain the frame height and expose the current position

### Requirement: SDLC-LOC-10 -- Navigation, filtering, and sorting

The dashboard SHALL support arrows and coherent Vim-style navigation, search,
filtering, history, details, and manual retry. Sorting SHALL cycle left-to-right
through issue number, status, and duration, with a direction arrow on the active
column. Title SHALL NOT be a sort key. Reverse sort SHALL have an explicit shortcut.
Footer shortcut keys SHALL be visually distinguishable from their descriptions.
Complete log paths SHALL remain available even when the screen truncates them.

#### Scenario: SDLC-LOC-10.01 -- Reverse the selected sort

- GIVEN an active sort column
- WHEN the developer invokes reverse sort
- THEN ordering and the column's directional arrow SHALL reverse together

## Verification and operational documentation

The dispatcher tests cover persistent ownership, detachment, discovery, retries,
lifecycle ordering, canonical harness propagation, gateway recovery, and terminal
interaction. The developer procedure is [Local Fullsend](../../docs/development/fullsend.md).
