# Fullsend Lifecycle Label Hooks

This directory implements `specs/standards/sdlc/fullsend-lifecycle-label-hooks.spec.md`
(prefix `SDLC-HOOK`). Lifecycle phase-label transitions for the SDLC funnel are
applied by shared, deterministic scripts invoked through native Fullsend harness
hooks, not by agent prose. An agent does its work and returns a validated,
structured result only; a trusted post-script applies the permitted label change
and verifies the final state.

Everything lives under `.fullsend/`, so the same harnesses run identically in
local and cloud Fullsend execution with no dependency on the local dispatcher,
its shell launcher, or its Go program (SDLC-HOOK-11).

## Hook ordering

Each phase harness (`triage`, `spec-implementation`, `spec-review`,
`code-implementation`, `code-review`) wires three hooks:

1. `pre_script: scripts/pre-lifecycle.sh` - records the starting state and the
   pull request head commit, confirms authorization, scope, and stage
   prerequisites, and skips the run when a prerequisite no longer holds
   (SDLC-HOOK-03).
2. `validation_loop` with `scripts/validate-output-schema.sh` and the phase's
   `schemas/<phase>-result.schema.json` - read-only JSON Schema validation of the
   agent result before any label change (SDLC-HOOK-06, ADR 0022).
3. `post_script: scripts/post-lifecycle.sh` - consumes only a validated result,
   re-reads live state, checks evidence, applies the matched transition, and
   verifies it (SDLC-HOOK-07, SDLC-HOOK-08).

The phase is selected by the `PHASE` environment variable set in each harness.

## The result contract (SDLC-HOOK-04)

Agents write exactly one JSON object to `$FULLSEND_OUTPUT_DIR/agent-result.json`.
Common fields:

| Field | Meaning |
|-------|---------|
| `schema_version` | always `"1"` |
| `phase` | the phase slug; must match the harness `PHASE` |
| `outcome` | a fixed per-phase enum (never a label name) |
| `rationale` | short explanation |
| `comment` | optional (required for reviews) markdown posted by the post-script |
| `pr_number` | PR the result concerns (verified against trusted input) |
| `head_sha` | commit implemented or reviewed (evidence) |

Permitted outcomes:

| Phase | Outcomes |
|-------|----------|
| triage | `workable`, `needs-input`, `duplicate` |
| spec-implementation | `completed`, `blocked` |
| spec-review | `approved`, `rejected` |
| code-implementation | `completed`, `blocked` |
| code-review | `approved`, `rejected` |

The result never supplies label names, repository targets, or shell commands.
The target repo and issue come from the trusted work-item URL; any `pr_number`
the result names is checked against the pre-script's recorded value.

## Transition rules (SDLC-HOOK-01, SDLC-HOOK-02)

The single source of truth is `scripts/lib/lifecycle-contract.sh`. For each
phase outcome it names the phase labels to add and the superseded phase labels to
remove. Scope (`agent/scope/*`), authorization (`agent/allowed`), the
execution-record marker (`agent/execution-record`), and unrelated labels are
never named by a transition and so are preserved. `agent/blocked` is only ever
added (a review retry limit, or a hard implementation block), never removed, so a
transition cannot silently clear a human block.

| Phase | Outcome | Add | Remove |
|-------|---------|-----|--------|
| triage | workable | `agent/workable` | none |
| triage | needs-input | `agent/needs-input` | none |
| triage | duplicate | `agent/duplicate` (+ close issue) | none |
| spec-implementation | completed | `agent/reviewable-spec` | `agent/workable`, `agent/review-spec-rejected`, stale `agent/review-spec-approved` |
| spec-implementation | blocked | `agent/blocked` | none |
| spec-review | approved | `agent/review-spec-approved` | `agent/reviewable-spec`, `agent/review-spec-rejected` |
| spec-review | rejected | `agent/review-spec-rejected` (+ `agent/blocked` at limit 3) | `agent/reviewable-spec`, `agent/review-spec-approved` |
| code-implementation | completed | `agent/reviewable-code` | stale `agent/review-code-rejected`, `agent/review-code-approved` |
| code-implementation | blocked | `agent/blocked` | none |
| code-review | approved | `agent/review-code-approved` | `agent/reviewable-code`, `agent/review-code-rejected` |
| code-review | rejected | `agent/review-code-rejected` (+ `agent/blocked` at limit 10) | `agent/reviewable-code`, `agent/review-code-approved` |

The code-implementation completed transition preserves `agent/review-spec-approved`
as a completed prerequisite.

## Evidence and eligibility (SDLC-HOOK-07, SDLC-HOOK-12)

Before applying a transition the post-script re-reads GitHub and:

- Rejects a review result as stale if the PR head moved since the pre-script
  recorded it, or if the agent's reviewed `head_sha` does not match.
- Gates a code-review approval on the PR head's required checks being green.
- Counts prior rejections from the issue timeline (never from the result) since
  the most recent removal of `agent/blocked`, and adds `agent/blocked` when the
  count reaches 3 (spec) or 10 (code).

A blocked record stays blocked until a human removes `agent/blocked` and the
superseding rejection label; the pre-script skips any phase while `agent/blocked`
is present.

## Single writer (SDLC-HOOK-11)

Only the five phase post-scripts write phase labels. `ingest` writes the
execution-record marker and the scope label; `reconcile` writes Jira status from
the GitHub phase labels. Neither writes a GitHub phase label, so there is no
competing writer.

## Required runner tools and permissions

The trusted runner needs `gh` (authenticated by `GH_TOKEN`), `jq`, the `fullsend`
CLI (for the sticky `issues post-comment`), and `python3` with the `jsonschema`
package (for validation; declared as `preflight_check`). `curl` is deliberately
unavailable (base policy). The minted token uses the harness `role:` and needs
permission to read and write issue labels and to read pull requests and checks.

## Recovery (SDLC-HOOK-10)

The validated result is retained in the Fullsend run artifacts at
`iteration-<N>/output/agent-result.json`, and the iteration Fullsend validated is
named by `FULLSEND_VALIDATED_ITERATION_DIR`. The post-script is idempotent and
re-checks live state before acting, so an operator can reapply a validated result
without rerunning the agent by invoking `scripts/post-lifecycle.sh` from the run
directory with the same `PHASE`, `ISSUE_URL`, and recorded `LIFECYCLE_*`
environment (where the deployed Fullsend interfaces support re-invoking a
post-script). Failure classes are distinguishable from the post-script logs:
`no validated agent result` (agent failure), a non-zero exit on a schema or
identifier problem (invalid output), `review result is stale` (stale output), and
`final-state verification FAILED` (label-application failure).

## Tests

`scripts/tests/lifecycle_test.sh` covers the pure contract and decision logic.
`scripts/tests/lifecycle_post_test.sh` drives `post-lifecycle.sh` end to end with
a mocked `gh` and `fullsend` (no local dispatcher). `scripts/tests/lifecycle_schema_test.sh`
covers the native read-only schema validation. All three are auto-discovered by
`make ci-test`.
