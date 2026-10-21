# SDLC Funnel Deployment Validation

**ID prefix:** SDLC-VAL

## Purpose

The agent-based SDLC funnel depends on a stack of external services and runtime
capabilities: the GitHub REST API for issue reads and label mutations, git
operations for branching, committing, and pushing, pull-request creation, and
the spec skill's file-system workflow. A newly deployed agent runtime must prove
that it can reach every integration point and drive a work item through the
pipeline before it processes production work.

This specification defines the deployment validation path: a test issue that
flows through the same label state machine, authorization gate, and scope rules
as any production work item. No special-case paths or label exemptions exist
for test issues. A successful pass confirms the deployed runtime environment is
operational; a failure at any integration point surfaces as a normal funnel
error, observable through the same channels as production failures.

## Requirements

### Requirement: SDLC-VAL-01 -- Deployment Validation Through the Funnel

The SDLC funnel SHALL be validatable end-to-end by processing a test issue
through the full pipeline: intake, triage, spec-implementation, and
spec-review. The validation SHALL prove that the deployed agent runtime can
reach all external services and produce artifacts (branches, specification
files, pull requests) under the same constraints as production work.

A test issue SHALL follow the same label state machine
(`.hypershell/README.md`), authorization gate (`agent/allowed`), and scope
rules (`agent/scope/*`) as any production work item, per
`specs/standards/sdlc/jira-github-sync.spec.md`. No bypass, shortcut, or
special-case path SHALL exist for test issues.

#### Scenario: SDLC-VAL-01.01 -- Test issue traverses the funnel like production work

- GIVEN a GitHub issue created for deployment validation
- AND the issue carries `agent/allowed` and an `agent/scope/*` label
- WHEN the SDLC funnel processes the issue
- THEN the issue SHALL pass through the same intake, triage,
  spec-implementation, and spec-review phases as a production work item
- AND no phase SHALL be skipped or bypassed on account of the issue being a
  test issue

#### Scenario: SDLC-VAL-01.02 -- Test issue without authorization is inert

- GIVEN a test issue carrying an `agent/scope/*` label but NOT `agent/allowed`
- WHEN the SDLC funnel runs
- THEN the issue SHALL NOT be processed by any funnel phase
- AND no artifacts SHALL be created for it

### Requirement: SDLC-VAL-02 -- Integration Point Coverage

The funnel validation SHALL exercise the following integration points, each of
which is a prerequisite for production work:

1. **GitHub Issue API** - read issue metadata, read and apply labels
2. **Git operations** - clone the repository, create a branch, commit changes,
   push to the remote
3. **Pull-request creation** - open a pull request with a closing reference to
   the test issue
4. **Spec skill workflow** - create or update specification files in `specs/`,
   register IDs in `specs/ids.md`, and verify whole-tree coherence

A failure at any integration point SHALL surface as a normal funnel error,
observable through the label state machine (the issue stays in its current
phase) and through the agent SDLC visibility dashboard
(`specs/platform/agent-sdlc-visibility-dashboard.spec.md`) when deployed.

#### Scenario: SDLC-VAL-02.01 -- Successful validation produces a specification artifact

- GIVEN a test issue processed by the spec-implementation agent
- WHEN the agent completes
- THEN a specification file SHALL exist in `specs/`
- AND a pull request SHALL be open against the repository with a closing
  reference to the test issue
- AND the issue SHALL carry the `agent/reviewable-spec` label

#### Scenario: SDLC-VAL-02.02 -- Git integration failure keeps the issue in phase

- GIVEN a test issue in the `agent/workable` phase
- AND the agent runtime cannot push to the remote (a git integration failure)
- WHEN the spec-implementation agent runs
- THEN the issue SHALL remain in the `agent/workable` phase
- AND the failure SHALL be reported, not silently swallowed

### Requirement: SDLC-VAL-03 -- Idempotent Re-Validation

Re-running the funnel against a test issue that has already been processed
SHALL be safe and idempotent, matching the funnel's general idempotency
contract (`specs/standards/sdlc/jira-github-sync.spec.md`, SDLC-JIRA-12). A
re-run SHALL NOT create duplicate branches, duplicate pull requests, or
duplicate specification files. If the issue has already reached a terminal or
paused phase, the funnel SHALL exit without error.

#### Scenario: SDLC-VAL-03.01 -- Re-run after spec delivery creates no duplicates

- GIVEN a test issue that already has an open specification pull request
- AND the issue carries `agent/reviewable-spec`
- WHEN the SDLC funnel re-runs against the issue
- THEN no additional pull request SHALL be created
- AND the existing pull request and branch SHALL remain unchanged

#### Scenario: SDLC-VAL-03.02 -- Re-run at a terminal phase exits cleanly

- GIVEN a test issue that has reached `agent/review-code-approved`
- WHEN the SDLC funnel runs against the issue
- THEN the funnel SHALL exit without error
- AND no write action SHALL be performed

### Requirement: SDLC-VAL-04 -- Validation Scope Isolation

A test issue SHALL be scoped to its own `agent/scope/*` label so that
validation runs do not interfere with production work items. The scope label
ensures the funnel processes only the test issue when run with that scope, and
production scope runs do not pick up the test issue unless it shares a
production scope label.

#### Scenario: SDLC-VAL-04.01 -- Test scope does not include production work

- GIVEN a test issue carrying `agent/scope/hyp0`
- AND production work items carrying `agent/scope/hyp0`
- WHEN the SDLC funnel runs with scope `agent/scope/hyp0`
- THEN the funnel SHALL process both the test issue and production items
  carrying that scope
- AND the test issue SHALL follow the same rules as every other item in scope

#### Scenario: SDLC-VAL-04.02 -- Test issue is invisible to an unrelated scope

- GIVEN a test issue carrying only `agent/scope/hyp0`
- WHEN the SDLC funnel runs with scope `agent/scope/hyp1`
- THEN the test issue SHALL NOT appear in the candidate set
