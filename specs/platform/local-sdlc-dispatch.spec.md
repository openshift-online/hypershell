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
Phase outcomes and changed inputs SHALL trigger reconciliation. Periodic Jira
checks SHALL be read-only scripts comparing persisted fingerprints of relevant
fields and remote links, using existing ACLI or API-token authentication. Passage
of time alone SHALL NOT launch an agent for an unchanged open or closed record.
Successful synchronization SHALL refresh the observed baseline. Probe failures
SHALL be visible, retry with bounded delay, and SHALL NOT count as an unchanged
observation or start a model to diagnose authentication/network failure. Persisted
observations SHALL contain hashes and scheduling metadata, never credentials or
Jira content. On upgrade, established successful records SHALL reuse their saved
identity; Jira edits newer than their last successful sync SHALL still trigger
reconciliation. Intake discovery SHALL run as a mechanical script every 180 seconds by default.
Its interval SHALL be configurable through Make, the exported environment/local
env file, and a CLI flag. Explicit invocation options SHALL override the env file,
which overrides inherited values. Both Jira and GitHub discovery SHALL paginate
all scoped, authorized records, including closed records. Healthy canonical pairs
SHALL NOT launch intake models when their ordinary content or lifecycle labels
change; reconciliation owns those updates. New, unlinked, or ambiguous pairs
SHALL produce stable work identities based on meaningful inputs. Merely reaching
the next interval SHALL NOT launch an intake model. Observations SHALL persist
across restarts. Failed/incomplete discovery SHALL be visible and retried with
backoff, without overwriting a successful observation or treating an API failure
as an empty backlog. Failed intake work SHALL retain normal execution backoff.
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

#### Scenario: SDLC-LOC-06.02 -- Mechanical checks of converged records

- GIVEN a successfully reconciled closed execution record and unchanged Jira data
- WHEN the drift interval elapses or the dispatcher restarts
- THEN the script SHALL refresh its observation without launching an LLM
- WHEN relevant Jira content, the GitHub record, or its linked PR changes
- THEN reconciliation SHALL become eligible subject to ownership and authorization
- AND an unchanged observation after that successful run SHALL remain deduplicated

#### Scenario: SDLC-LOC-06.03 -- Configurable mechanical intake discovery

- GIVEN healthy canonical pairs and a three-minute default discovery interval
- WHEN discovery runs repeatedly or the dispatcher restarts
- THEN no intake agent SHALL launch for unchanged pairs
- WHEN new, unlinked, or ambiguously linked work appears
- THEN its stable input identity SHALL make intake eligible
- AND an API error or incomplete page SHALL remain an error rather than erase work
- AND configuring a different interval SHALL change the script cadence

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
A successful review process SHALL NOT imply that approval was applied. Reviews
awaiting required CI, draft resolution, or known mergeability SHALL remain visible
in the main lists with explicit waiting text and a non-success semantic color,
even when completed runs are hidden. Logs and details SHALL share that status;
details SHALL explain the blocker, linked PR, and the automatic next step.
Waiting reviews SHALL NOT count as completed runs. History SHALL continue to
report the completed attempt, independently of current readiness.

A review of an older PR head SHALL be shown as `Awaiting new review`, not as a
current review waiting to apply approval. New attempts SHALL save the PR head
used at launch. Discovery SHALL compare that identity to the live head and detect
implementation work started since the previous review. The old attempt duration
SHALL NOT appear as current work duration; it SHALL remain available in history.
Details SHALL distinguish Jira synchronization failures blocking review launch
from required CI blocking approval, and identify an expired Jira authentication
snapshot explicitly when that is the recorded synchronization failure.

#### Scenario: SDLC-LOC-09.01 -- Inspect a run

- GIVEN a selected issue with multiple agent attempts
- WHEN the developer opens logs or history
- THEN the issue and agent identity SHALL remain visible
- AND scrolling SHALL retain the frame height and expose the current position

#### Scenario: SDLC-LOC-09.02 -- A new revision has not inherited an old review

- GIVEN implementation has pushed a new PR head since the last completed review
- AND Jira synchronization is retrying because ACLI authentication expired
- WHEN the developer inspects the work list or details
- THEN the review SHALL say `Awaiting new review` with no current duration
- AND details SHALL identify Jira authentication and pending CI separately
- AND pending CI SHALL be described as an approval gate, not a review-launch gate

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

### Requirement: SDLC-LOC-11 -- Confirmed graceful cancellation

The overview, logs, and details SHALL offer **Cancel run** for a selected running
job in the current scope. Confirmation SHALL identify the issue and actual agent,
explain partial changes and cleanup, support arrow/Vim selection, and default to
**Keep running**. Cancellation SHALL target an immutable attempt identity and
reject stale confirmations, other scopes, completed runs, and unsupported legacy
workers without signaling unverified PIDs.

A durable request SHALL survive dispatcher detachment and restart. The detached
supervisor SHALL send SIGTERM to Fullsend, retain its ownership lock until
Fullsend exits and the outcome is persisted, and expose **Canceling** during
cleanup. Cancellation SHALL NOT force-kill Fullsend, terminate unrelated workers,
or release capacity while cleanup is active. Completed cancellation SHALL be
recorded distinctly in the overview and attempt history. The issue SHALL remain
paused until manual retry; retry SHALL still respect ownership and authorization.

Cancellation SHALL NOT fabricate agent results or lifecycle verdicts. Fullsend's
validation gates remain authoritative for post-script execution, which is not
guaranteed on interruption. Documentation SHALL explain retained partial writes,
remote cleanup verification, and the difference between cancellation and detach.

#### Scenario: SDLC-LOC-11.01 -- Cancel while cleanup takes time

- GIVEN a running worker and an explicitly confirmed cancellation
- WHEN Fullsend handles SIGTERM but has not finished cleanup
- THEN the run SHALL remain owned and ineligible for retry
- AND a restarted dashboard SHALL show the persisted cancellation request
- WHEN Fullsend exits and the outcome is saved
- THEN the attempt SHALL appear as canceled, with its logs preserved
- AND automatic discovery SHALL NOT restart work on the canceled issue

### Requirement: SDLC-LOC-12 -- Visible, persistent configuration

The terminal SHALL provide a Configuration tab using the shared frame, visual
hierarchy, keyboard navigation, and scroll indicators. It SHALL show effective
dispatcher settings, their application timing, and masked credential presence.
The absolute home settings file path SHALL be prominent in both browsing and
editing, with an explicit disclosure that saving writes the file on disk.
Read-only values SHALL remain inspectable in full and explain how they are managed.

Edits SHALL validate before saving and use atomic, private JSON storage under
`$XDG_CONFIG_HOME/hypershell/fullsend.json` (default `~/.config/hypershell/fullsend.json`),
with `FULLSEND_CONFIG_FILE` available as an explicit location override. Concurrent
writers SHALL preserve unrelated keys. Malformed files, invalid values, symlinks,
or failed saves SHALL remain visible errors and SHALL NOT discard existing data.
Shell env files SHALL NOT be rewritten or evaluated as preference files.
Credentials SHALL NOT be displayed or written into these preferences.

Worker concurrency and discovery/authentication intervals SHALL update the active
dispatcher. Lowering concurrency SHALL let running workers finish. Model, inference,
and failure-backoff changes SHALL apply to new workers, preserving existing worker
inputs and retry deadlines. Sign-in timeout changes SHALL apply to the next sign-in.
Scope, repository, storage paths, and authentication-mode changes SHALL be saved
with a persistent restart-required marker while the current session and helpers
retain their original identities. The UI SHALL distinguish saved and active values.

Explicit launch options SHALL override saved preferences on restart. Saved
preferences SHALL override worktree env defaults, which override inherited
settings. The launcher SHALL reuse a saved scope when no explicit Make scope is
supplied, while retaining registered-scope validation. Manual preference-file edits
SHALL load on restart; this limitation and launch-override precedence SHALL be clear.

#### Scenario: SDLC-LOC-12.01 -- Save a live limit and a restart-only identity

- GIVEN two running workers and the Configuration tab
- WHEN the operator saves a worker limit of one
- THEN the file SHALL contain the new value and the live limit SHALL change
- AND existing workers SHALL continue with their original inputs
- WHEN the operator saves a different repository
- THEN the saved value SHALL be shown as requiring restart
- AND discovery, authorization, and running work SHALL retain the current repository

## Verification and operational documentation

The dispatcher tests cover persistent ownership, detachment, discovery, retries,
lifecycle ordering, canonical harness propagation, gateway recovery, and terminal
interaction. The developer procedure is [Local Fullsend](../../docs/development/fullsend.md).

### Requirement: SDLC-LOC-13 -- Readable, complete, controllable logs

The log pane SHALL have a light grey bounding border and a terminal-sized,
fixed-height viewport. Raw log text, unwrapped lines, and colors SHALL be the
default. The developer SHALL be able to toggle Markdown (`m`), wrapping (`w`),
and color (`a`) independently. Markdown SHALL render through the same terminal
palette without fetching remote resources or consulting environment-selected
stylesheets. Markdown SHALL apply only to recognizable Fullsend `💬` agent prose
blocks, not CLI progress, commands, tool output, warnings or errors. CLI ANSI colors
SHALL remain intact when Markdown is enabled. Unmarked output SHALL remain
literal. Source-line filtering SHALL retain agent-message context even when the
marker line does not match the filter. `NO_COLOR` SHALL prevent ANSI colors even when the color toggle is on.

The complete source file SHALL remain accessible, including content older than
64 KiB and the ends of long lines. With wrapping off, Left/Right and H/L SHALL
pan horizontally; 0/$ SHALL reach the left/right edge. A visible column range
SHALL indicate hidden horizontal content. Wrapping SHALL reflow to the available
width, including long unbroken tokens and wide Unicode text. Vertical scroll
indicators SHALL remain present; resizing or reaching an edge SHALL NOT collapse
the pane. Source colors and semantic success/warning/error colors MAY be shown;
terminal cursor, erase, title, hyperlink, clipboard, blink and conceal controls
from source logs SHALL NOT be executed.

`/` SHALL filter source lines by a case-insensitive substring independently of
work-list/history filters. Filtering SHALL remain live as output grows. Enter
SHALL retain the filter; Esc SHALL restore an in-progress edit or clear an
applied filter before navigating back. Filtering text entry SHALL take precedence
over action shortcuts. A no-match state and source match count SHALL be visible.

New output SHALL be followed while at the bottom. Scrolling up SHALL pause
following and preserve the inspected position when output is appended. Scrolling
back to the bottom or pressing f/G SHALL resume following. Following/paused state
and each rendering mode SHALL be visible in the top pane border. Line/column
ranges and active-filter match counts SHALL appear in the bottom border, without
a redundant source-line count row. Filter editing MAY use one temporary input row. Reads SHALL detect truncation or file
replacement, preserve errors as visible diagnostics, and avoid rereading an
unchanged file or rerendering unchanged content on every poll. Toggles SHALL NOT
change the original log file.

#### Scenario: SDLC-LOC-13.01 -- Read earlier output while the worker continues

- GIVEN a growing log larger than 64 KiB with long Unicode lines
- WHEN the developer scrolls to the top and the worker appends output
- THEN the same earlier content SHALL remain visible without jumping
- AND the entire line SHALL remain reachable by horizontal panning
- WHEN the developer returns to the bottom
- THEN following SHALL resume automatically

#### Scenario: SDLC-LOC-13.02 -- Filter and render without altering the source

- GIVEN ANSI-colored log output containing Markdown and terminal-control sequences
- WHEN the developer filters lines and toggles Markdown, wrapping, or color
- THEN the selected lines SHALL remain readable within the pane
- AND source terminal controls SHALL NOT affect the surrounding interface
- AND original file bytes and work-list filters SHALL remain unchanged
