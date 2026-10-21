# Code-Implementation Remediation Specification

**ID prefix:** SDLC-REM

## Purpose

The code-implementation agent pushes code and hands it to code-review. When the
review rejects (failing CI, merge conflicts, needs rebase), the implementer must
diagnose and fix the problem before re-queuing. Today the skill says "address
any failing CI checks" and "address any merge conflicts" but specifies no
procedure for either. The agent never reproduces failing tests locally, never
fetches check logs, never rebases, and has no model of what the sandbox can and
cannot run. The result is blind pushes, CI ping-pong, and eventual escalation
with no actionable detail.

This specification defines the remediation contract that code-implementation and
code-review SHALL satisfy: deterministic PR health assessment, a rebase
procedure, log-driven test remediation, sandbox-capability routing, bounded
iteration, and structured observability. It complements the existing 10-rejection
cap in code-review (`.hypershell/agents/code-review/SKILL.md`) and the rework
meter in the SDLC visibility dashboard (`specs/platform/agent-sdlc-visibility-dashboard.spec.md`,
SDLC-VIS-09) without replacing either.

---

## Requirements

### Requirement: SDLC-REM-01 -- Deterministic PR Health

Code-implementation and code-review SHALL determine pull-request health from the
GitHub API, not from inference or heuristics. The agent SHALL query the pull
request's `mergeable` state for conflict and rebase status, and the check-runs
endpoint for CI conclusions. For each check run the agent SHALL record the check
name, conclusion, and logs URL.

A code-review rejection for CI failure, conflict, or rebase state SHALL name the
specific failing checks (name and conclusion) and the conflict or rebase state in
the rejection comment, so the implementer receives concrete targets.

#### Scenario: SDLC-REM-01.01 -- PR health is read from the API

- GIVEN an open pull request linked to an issue in the funnel
- WHEN code-implementation or code-review assesses PR health
- THEN it SHALL query the GitHub API for the pull request's mergeable state
- AND it SHALL query the check-runs endpoint for each check's name, conclusion, and logs URL
- AND it SHALL NOT infer CI or conflict state from the diff or PR body

#### Scenario: SDLC-REM-01.02 -- Rejection names specific checks

- GIVEN a pull request with two failing checks named `lint` and `unit-test`
- WHEN code-review rejects the implementation
- THEN the rejection comment SHALL name both `lint` and `unit-test`
- AND it SHALL include each check's conclusion and logs URL

#### Scenario: SDLC-REM-01.03 -- Rejection names conflict state

- GIVEN a pull request whose mergeable state indicates a conflict
- WHEN code-review rejects the implementation
- THEN the rejection comment SHALL state the pull request is conflicted

### Requirement: SDLC-REM-02 -- Rebase and Conflict Resolution

When a pull request is behind `main` or has merge conflicts, code-implementation
SHALL rebase the implementer branch onto the latest `main`. The agent SHALL:

1. Fetch the latest `main` from origin.
2. Rebase the implementer branch onto `origin/main`.
3. Resolve conflicts in favor of preserving the pull request's intent while
   incorporating the upstream change.
4. Re-run the local gate (the CI-equivalent lint and vet checks).
5. Force-push the rebased branch.

If conflicts cannot be resolved automatically -- for example, semantic conflicts
that require design judgment beyond the change's scope -- the agent SHALL stop,
comment the specific unresolved files and reasons on the issue, and label the
issue `agent/blocked`. The agent SHALL NOT push a broken rebase.

#### Scenario: SDLC-REM-02.01 -- Clean rebase

- GIVEN a pull request that is behind `main` with no conflicting files
- WHEN code-implementation runs remediation
- THEN it SHALL fetch the latest `main`
- AND it SHALL rebase the branch onto `origin/main`
- AND it SHALL re-run the local gate and require it green
- AND it SHALL force-push the rebased branch

#### Scenario: SDLC-REM-02.02 -- Resolvable conflict

- GIVEN a pull request with a merge conflict in one file
- AND the conflict is mechanically resolvable (non-overlapping intent)
- WHEN code-implementation runs remediation
- THEN it SHALL rebase and resolve the conflict
- AND it SHALL re-run the local gate
- AND it SHALL force-push the rebased branch
- AND it SHALL comment on the issue naming the resolved files

#### Scenario: SDLC-REM-02.03 -- Unresolvable conflict

- GIVEN a pull request with a semantic conflict beyond the change's scope
- WHEN code-implementation attempts to rebase
- THEN it SHALL NOT push a broken rebase
- AND it SHALL comment on the issue naming the unresolved files and the reason
- AND it SHALL label the issue `agent/blocked`

### Requirement: SDLC-REM-03 -- Log-Driven Test Remediation

When a pull request has failing CI checks, code-implementation SHALL fetch the
logs of each failing check and diagnose the root cause before changing code. The
agent SHALL NOT make speculative fixes without first reading the failure output.

For checks that are reproducible in the sandbox (see Sandbox-Capability Routing,
SDLC-REM-04), the agent SHALL reproduce the failure locally, apply a targeted
fix, and verify the check passes before pushing. The agent SHALL NOT consider the
remediation step complete while a reproducible check is still failing locally.

#### Scenario: SDLC-REM-03.01 -- Logs fetched before fix

- GIVEN a pull request with a failing check named `unit-test`
- WHEN code-implementation begins remediation for that check
- THEN it SHALL fetch the check's logs from the logs URL
- AND it SHALL diagnose the root cause from the log output before changing code

#### Scenario: SDLC-REM-03.02 -- Reproducible failure fixed and verified

- GIVEN a failing check that is classified as reproducible in the sandbox
- WHEN code-implementation remediates it
- THEN it SHALL reproduce the failure locally
- AND it SHALL apply a targeted fix
- AND it SHALL verify the check passes locally before pushing

#### Scenario: SDLC-REM-03.03 -- Reproducible check still failing blocks push

- GIVEN a remediation attempt for a reproducible check
- AND the check still fails locally after the fix
- WHEN code-implementation evaluates whether to push
- THEN it SHALL NOT push
- AND it SHALL continue iterating within the bounded remediation limit (SDLC-REM-05)

### Requirement: SDLC-REM-04 -- Sandbox-Capability Routing

The code-implementation skill SHALL classify each failing check as either
**reproducible in-sandbox** or **requires-CI-infra**.

A check is reproducible in-sandbox when it needs only tools available in the
agent sandbox: Go compiler, linter, `go vet`, unit tests that need no container
engine or database, shell script tests, and frontend lint or type checks.

A check requires-CI-infra when it depends on a container engine (Docker, Podman),
PostgreSQL (testcontainers-based API server tests), Kubernetes (e2e), Konflux
pipeline infrastructure, or other services not present in the sandbox.

For requires-CI-infra failures, the agent SHALL make a best-effort targeted fix
informed by the log output, state in the PR or issue comment that the check
cannot be reproduced in the sandbox and is deferred to CI, and SHALL NOT loop
on the non-reproducible check. The agent SHALL clearly distinguish deferred
checks from verified-green checks in its comment.

#### Scenario: SDLC-REM-04.01 -- Lint classified as in-sandbox

- GIVEN a failing `lint` check
- WHEN code-implementation classifies it
- THEN it SHALL classify `lint` as reproducible in-sandbox

#### Scenario: SDLC-REM-04.02 -- Testcontainers test classified as requires-CI-infra

- GIVEN a failing `api-server-integration` check that uses testcontainers
- WHEN code-implementation classifies it
- THEN it SHALL classify it as requires-CI-infra

#### Scenario: SDLC-REM-04.03 -- Requires-CI-infra check gets best-effort fix

- GIVEN a failing check classified as requires-CI-infra
- WHEN code-implementation remediates it
- THEN it SHALL make a targeted fix informed by the log output
- AND it SHALL comment that the check cannot be reproduced in the sandbox
- AND it SHALL state the check is deferred to CI
- AND it SHALL NOT loop waiting for the check to pass

### Requirement: SDLC-REM-05 -- Bounded Self-Remediation

Within a single code-implementation run, the agent SHALL attempt remediation of
reproducible checks up to a bounded number of local fix-and-verify iterations. If
a reproducible check is still failing after the bound, the agent SHALL label the
issue `agent/blocked`, comment the remaining failure with log output, and stop.

The agent SHALL NOT hand a still-red pull request back to code-review. This
bound complements (does not replace) the existing 10-rejection cap in
code-review (`.hypershell/agents/code-review/SKILL.md`).

#### Scenario: SDLC-REM-05.01 -- Fix within bound

- GIVEN a reproducible check that fails on the first iteration
- WHEN the agent fixes it on the second iteration and verifies it green
- THEN it SHALL push the fix and proceed

#### Scenario: SDLC-REM-05.02 -- Exhausted bound

- GIVEN a reproducible check that still fails after the bounded number of iterations
- WHEN the agent evaluates whether to continue
- THEN it SHALL label the issue `agent/blocked`
- AND it SHALL comment the remaining failure with log output
- AND it SHALL stop without pushing a still-failing branch
- AND it SHALL NOT re-queue the issue for code-review

#### Scenario: SDLC-REM-05.03 -- Bound is per-run, not cumulative

- GIVEN an issue that was previously blocked by a remediation bound exhaustion
- AND a human has removed `agent/blocked` and re-queued the issue
- WHEN code-implementation runs again
- THEN the remediation iteration count SHALL start from zero

### Requirement: SDLC-REM-06 -- Idempotency and Observability

The agent SHALL operate on the existing linked pull request and branch. It SHALL
NOT open a duplicate pull request. If the branch and pull request already exist,
the agent SHALL work on them in place.

The agent SHALL comment on the issue with the concrete actions taken during
remediation:

- Rebased onto `<sha>` (when a rebase occurred).
- Resolved `<files>` (when conflicts were resolved).
- Fixed `<check>` (when a check was remediated, with a summary of the fix).
- Local gate result (pass or fail, with the command run).
- Deferred `<check>` to CI (when a check was classified as requires-CI-infra).
- CI expectation (which checks the agent expects to pass on the next run).

The comment SHALL be written in simplified technical English.

#### Scenario: SDLC-REM-06.01 -- No duplicate PR

- GIVEN an issue with an existing linked pull request
- WHEN code-implementation runs remediation
- THEN it SHALL push to the existing pull request's branch
- AND it SHALL NOT open a new pull request

#### Scenario: SDLC-REM-06.02 -- Remediation comment includes actions

- GIVEN a remediation run that rebased and fixed a lint check
- WHEN the run completes
- THEN the issue comment SHALL name the rebase target SHA
- AND it SHALL name the fixed check and summarize the fix
- AND it SHALL state the local gate result
- AND it SHALL state the CI expectation

#### Scenario: SDLC-REM-06.03 -- Deferred checks are distinguished

- GIVEN a remediation run with one verified-green check and one deferred check
- WHEN the run completes
- THEN the issue comment SHALL clearly distinguish the verified-green check from the deferred check
- AND it SHALL state the deferred check cannot be reproduced in the sandbox

---

## Required Skill Changes

These are the behaviors the SDLC skills SHALL implement. They are stated here as
the normative contract the skills must satisfy, regardless of when the
corresponding skill changes land.

### `code-implementation` gains a structured remediation phase

Step 2 of code-implementation SHALL query PR health deterministically from the
GitHub API (SDLC-REM-01). Between Step 4 (reconcile) and Step 5 (review), a
remediation phase SHALL:

1. Classify each failing check as in-sandbox or requires-CI-infra (SDLC-REM-04).
2. Fetch logs for each failing check (SDLC-REM-03).
3. Rebase if the branch is behind `main` or conflicted (SDLC-REM-02).
4. For each in-sandbox check, reproduce, fix, and verify locally within the
   bounded iteration limit (SDLC-REM-05).
5. For each requires-CI-infra check, make a best-effort log-driven fix and
   document the deferral (SDLC-REM-04).
6. Comment the actions taken (SDLC-REM-06).

Step 5 SHALL NOT re-queue the issue for code-review if any reproducible check
is still failing locally.

### `code-review` emits structured rejection rationale

Code-review Step 2 SHALL determine PR health from the GitHub API (SDLC-REM-01)
rather than from inference. A rejection for CI failure, conflict, or rebase state
SHALL name the specific failing checks (name, conclusion, logs URL) and the
conflict or rebase state in the rejection comment, so code-implementation
receives concrete diagnostic targets.

---

## Design Decisions

| Decision | Rationale |
|----------|-----------|
| PR health from API, not inference | Deterministic assessment eliminates false positives from diff heuristics and ensures the implementer receives the exact check names and logs it needs. |
| Rebase before test remediation | A branch behind `main` may carry false test failures from merge skew. Rebasing first establishes a clean baseline. |
| Sandbox-capability classification | The agent sandbox has no container engine or database. Looping on an infra-dependent check wastes iterations and blocks progress. Routing to best-effort-plus-deferral is the pragmatic middle ground. |
| Bounded remediation with `agent/blocked` | Unbounded loops on a failing check burn tokens and delay human attention. A bound makes the worst case predictable and the escalation carries actionable logs. |
| Complement, not replace, the 10-rejection cap | The remediation bound catches a failing-test loop before the PR ever returns to code-review. The 10-rejection cap remains as the outer safety net for review-level ping-pong. |
| Force-push after rebase | Rebase rewrites history. The implementer branch is agent-owned and force-push is the standard Git mechanism for updating a rebased branch. |
| Comment actions in simplified technical English | The comment is the diagnostic record. Structured, specific, and machine-parseable fields (SHA, file list, check name) make the record useful for both humans and the SDLC visibility dashboard. |
