---
name: code-implementation
description: >
  Code Implementation Workflow
---

# Workflow

The code implementation workflow is responsible for reconciling a set of spec changes
to code, ensuring CI passes, and resolving any merge conflicts or rebase needs on an open PR.

## Input

```text
$GITHUB_ISSUE_URL [--dry-run]
```

Supported arguments:
- GITHUB_ISSUE_URL -- execute against the provided GitHub issue.
- --dry-run -- [Optional] Execute the workflow, but without any WRITE actions.

## Workflow

### Step 1: Read Issue and Open PR

Fetch `$GITHUB_ISSUE_URL` and fully read its title and body.

Discover the linked pull request.

If no linked pull request:
- apply label `agent/blocked` to `$GITHUB_ISSUE_URL` (_Note: If the label does not exist, create it._)
- write a comment on the issue noting the reason.
- STOP. Do not execute any further steps.

Output: `$LINKED_PULL_REQUEST`

### Step 2: Identify State of Implementation (SDLC-REM-01)

Determine pull-request health **deterministically from the GitHub API**:

1. Query the pull request resource for `mergeable` and `mergeable_state` to
   determine conflict and rebase status.
2. Query the check-runs endpoint (`GET /repos/{owner}/{repo}/commits/{sha}/check-runs`)
   for each check's name, conclusion, status, and details URL (logs URL).
3. Query the commit status endpoint for any status contexts.

Record the following for use in later steps:

| Field | Source |
|-------|--------|
| `$MERGEABLE` | `mergeable` field from the pull request |
| `$MERGE_STATE` | `mergeable_state` field (`clean`, `dirty`, `behind`, `blocked`, `unknown`) |
| `$CHECK_RUNS` | Array of `{name, status, conclusion, details_url}` from the check-runs endpoint |
| `$FAILING_CHECKS` | Subset of `$CHECK_RUNS` where `conclusion` is not `success`, `skipped`, or `neutral` |

Do NOT infer CI or conflict state from the diff, PR body, or any heuristic.

Read the full contents of `$LINKED_PULL_REQUEST` and `$GITHUB_ISSUE_URL`.

### Step 3: Identify Spec Changes

Clone the branch associated with `$LINKED_PULL_REQUEST`. Using the git history, identify the specification
changes introduced in `$LINKED_PULL_REQUEST`. Note that specification changes may not be purely additive, and
may touch many parts of the application. Removal of, and changes to, specifications are just as important
as new specifications.

### Step 4: Reconcile Specifications

Execute the [reconcile skill](../../../skills/build/reconcile/SKILL.md) against the specification changes identified in Step 3.

### Step 4.5: Remediation (SDLC-REM-02 through SDLC-REM-06)

After reconciliation and before review, run a structured remediation phase to
address any rebase needs, merge conflicts, and failing CI checks.

#### 4.5.1 Rebase and Conflict Resolution (SDLC-REM-02)

If `$MERGE_STATE` is `dirty` (conflicted) or `behind`:

1. Fetch the latest `main` from origin (`git fetch origin main`).
2. Rebase the implementer branch onto `origin/main`.
3. Resolve conflicts in favor of preserving the pull request's intent while
   incorporating the upstream change.
4. Re-run the local gate (see Step 4.5.4).
5. Force-push the rebased branch.

If conflicts cannot be resolved automatically -- for example, semantic conflicts
that require design judgment beyond the change's scope -- STOP:
- Comment on the issue naming the specific unresolved files and the reason.
- Label the issue `agent/blocked`.
- Do NOT push a broken rebase.

Record the rebase target SHA for the observability comment (Step 4.5.6).

#### 4.5.2 Classify Failing Checks (SDLC-REM-04)

For each check in `$FAILING_CHECKS`, classify it as one of:

| Classification | Criteria |
|----------------|----------|
| **reproducible in-sandbox** | Needs only tools available in the agent sandbox: Go compiler, linter, `go vet`, unit tests that need no container engine or database, shell script tests, frontend lint or type checks. Examples: `lint`, `Repository policy`, `Go - control plane` (build/vet only), `PR test script validation`. |
| **requires-CI-infra** | Depends on a container engine (Docker, Podman), PostgreSQL (testcontainers-based API server tests), Kubernetes (e2e), Konflux pipeline infrastructure, or other services not present in the sandbox. Examples: `Unit / Go - API server` (testcontainers), `E2E / Kind`, `E2E / OpenShift`. |

#### 4.5.3 Log-Driven Test Remediation (SDLC-REM-03)

For each check in `$FAILING_CHECKS`:

1. Fetch the check's logs from its `details_url` (the GitHub Actions run log).
2. Diagnose the root cause from the log output **before** changing code. Do NOT
   make speculative fixes without first reading the failure output.

For **reproducible in-sandbox** checks:
3. Reproduce the failure locally by running the corresponding command.
4. Apply a targeted fix.
5. Verify the check passes locally before proceeding.
6. If the check still fails locally, iterate within the bounded limit (Step 4.5.5).

For **requires-CI-infra** checks:
3. Make a best-effort targeted fix informed by the log output.
4. Document in the observability comment (Step 4.5.6) that the check cannot be
   reproduced in the sandbox and is deferred to CI.
5. Do NOT loop waiting for the check to pass.

#### 4.5.4 Local Gate Verification

Run the CI-equivalent gate locally. These are the authoritative checks from the
reconcile skill Phase 6 step 3:

- Go: `make lint` (runs `gofmt`, `go vet`, golangci-lint for all Go components)
- Frontend (if changed): `pnpm --filter <package> check`
- Contract/SDK drift (if applicable): `cd components/api-server && make generate generate-sdk`
- Repo policy: `make check`

The local gate MUST be green before pushing. Do NOT push while any reproducible
check is still failing locally.

#### 4.5.5 Bounded Self-Remediation (SDLC-REM-05)

Within a single code-implementation run, attempt remediation of reproducible
checks up to **3 local fix-and-verify iterations** per check.

If a reproducible check is still failing after the bound:
- Label the issue `agent/blocked`.
- Comment the remaining failure with log output.
- STOP. Do NOT push a still-failing branch. Do NOT hand a still-red pull request
  back to code-review.

The bound is per-run, not cumulative. If a human removes `agent/blocked` and
re-queues the issue, the iteration count starts from zero.

This bound complements (does not replace) the existing 10-rejection cap in
code-review (`.hypershell/agents/code-review/SKILL.md`).

#### 4.5.6 Observability Comment (SDLC-REM-06)

After remediation, comment on the issue with the concrete actions taken. The
comment SHALL be written in simplified technical English and include:

- **Rebased onto `<sha>`** (when a rebase occurred).
- **Resolved `<files>`** (when conflicts were resolved).
- **Fixed `<check>`** (when a check was remediated, with a summary of the fix).
- **Local gate result** (pass or fail, with the command run).
- **Deferred `<check>` to CI** (when a check was classified as requires-CI-infra).
- **CI expectation** (which checks the agent expects to pass on the next run).

Clearly distinguish verified-green checks from deferred checks.

### Step 5: Review Changes

Execute the [amber-review skill](../../../skills/review/amber-review/SKILL.md) against the implementation produced by Step 4.

If the review produces findings:
- Return to Step 4 and address the findings
Else:
- Address any failing CI checks remaining after Step 4.5 by repeating the
  remediation phase. Before pushing, run the local gate (Step 4.5.4) and
  require it green.
- Commit (if not already) and push code to `$LINKED_PULL_REQUEST`, ensuring the body & title are updated to reflect the current state of the PR.
- Operate on the existing linked pull request and branch (SDLC-REM-06). Do NOT
  open a duplicate pull request.
- Remove label `agent/review-code-rejected` from `$GITHUB_ISSUE_URL` if present.
- Write label `agent/reviewable-code` to `$GITHUB_ISSUE_URL`.

_Note: If a label does not exist, create it._
