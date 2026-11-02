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

1. `pre_script: scripts/pre-lifecycle.sh` - records the starting state and, for a
   phase that runs against an existing pull request, its head commit; confirms
   authorization, scope, and stage prerequisites; and skips the run when a
   prerequisite no longer holds (SDLC-HOOK-03). `spec-implementation` CREATES the
   pull request, so the pre-script does not skip it for a missing pull request and
   records the pull request only if one already exists.
2. `validation_loop` with `scripts/validate-output-schema.sh` and the phase's
   `schemas/<phase>-result.schema.json` - read-only JSON Schema validation of the
   agent result before any label change (SDLC-HOOK-06, ADR 0022).
3. `post_script: scripts/post-lifecycle.sh` - consumes only a validated result,
   re-reads live state, checks evidence, applies the matched transition, and
   verifies it (SDLC-HOOK-07, SDLC-HOOK-08).

The phase is selected by the `PHASE` environment variable set in each harness.

The `pre_script`, `validation_loop`, and `post_script` hooks, the `skip=` relay
from a pre-script, the `FULLSEND_VALIDATED_ITERATION_DIR` output contract, and
harness-level output-schema enforcement follow the
[harness field reference](https://fullsend.sh/docs/reference/harness-reference)
and [ADR 0022](https://fullsend.sh/docs/ADRs/0022-harness-level-output-schema-enforcement).
No HyperShell harness used `pre_script` or the `skip=` key before this change, so
verify hook ordering, the skip relay, the validated-iteration output path, and
failure propagation against the deployed Fullsend version before relying on them in
production, and record the verified version here. These scripts are written so the
direct-invocation tests pass independently of the runtime, so a runtime mismatch
surfaces as a harness-wiring problem rather than a silent label change.

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
the result names is checked against the pre-script's recorded value. For
`spec-implementation` (which creates the pull request) on a fresh issue where no
pull request was recorded, the agent's `pr_number` is accepted only after
confirming it is an open pull request in this repo that closes this issue.

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
| code-implementation | completed | `agent/reviewable-code` | `agent/review-code-rejected`, stale `agent/review-code-approved` |
| code-implementation | blocked | `agent/blocked` | none |
| code-review | approved | `agent/review-code-approved` | `agent/reviewable-code`, `agent/review-code-rejected` |
| code-review | rejected | `agent/review-code-rejected` (+ `agent/blocked` at limit 10) | `agent/reviewable-code`, `agent/review-code-approved` |

The code-implementation completed transition preserves `agent/review-spec-approved`
as a completed prerequisite.

## Evidence and eligibility (SDLC-HOOK-07, SDLC-HOOK-12)

Before applying a transition the post-script re-reads GitHub and:

- Rejects a review result as stale if the PR head moved since the pre-script
  recorded it, or if the agent's reviewed `head_sha` does not match.
- Gates a code-review approval on the PR head's checks being green. When the base
  branch defines required status checks in branch protection, the gate is scoped
  to those contexts only (an advisory or always-pending non-required check does
  not block, and a required context that has not reported yet is treated as
  pending). When branch protection is unreadable or defines no required checks,
  the gate falls back to requiring every reported signal green.
- Counts prior rejections from the issue timeline (never from the result) since
  the most recent removal of `agent/blocked`, and adds `agent/blocked` when the
  count reaches 3 (spec) or 10 (code).

A blocked record stays blocked until a human removes `agent/blocked` and the
superseding rejection label; the pre-script skips any phase while `agent/blocked`
is present.

Deferred approval (required checks not yet green): when a code-review approval is
not eligible because a required check is still pending or failing, the post-script
does NOT exit silently. It leaves `agent/reviewable-code` in place (the item stays
awaiting review) and posts a visible deferral record through the phase sticky
comment, so the approval is never a log-only exit. The approval is applied when
this same validated result is reapplied once the required checks are green; because
the deferral uses the phase sticky marker, that reapplication edits the record in
place and produces no duplicate comment (SDLC-HOOK-09). A later check turning green
is not a label event, so it does not re-fire the harness trigger on its own: either
re-run the review phase once the checks are green, or reapply the validated result
through the recovery procedure below. The deferral record on the issue makes a
pending approval visible rather than leaving it to be inferred from run logs.

## Single writer (SDLC-HOOK-11)

Only the five phase post-scripts write phase labels. `ingest` writes the
execution-record marker and the scope label; `reconcile` writes Jira status from
the GitHub phase labels. Neither writes a GitHub phase label, so there is no
competing writer.

Documented exception: the `update-openshell` agent opens its own pull request
outside the SDLC funnel (it is not driven by a work-item issue through the phase
harnesses) and applies `agent/reviewable-code` to that pull request directly. This
is a deliberate bootstrap hand-off, not a competing writer on a funnel issue: the
`update-openshell` pull request has no lifecycle pre-script or post-script in its
harness, so no phase script ever writes labels for it. Routing it through the
shared contract is tracked as a follow-up; until then this is the single
acknowledged place outside the five phase post-scripts that writes a phase label,
and it only ever writes to its own fresh pull request.

## Posting the outcome comment (SDLC-HOOK-10)

The post-script posts the agent's `comment` through the repository's
sticky-comment primitive, `fullsend issues post-comment` (the same tool
`post-update-openshell.sh` uses), keyed by a per-phase HTML-comment marker
(`fullsend:<phase>-agent`) so a reapplied result edits the one comment in place
instead of posting a duplicate.

The comment is posted BEFORE any label change. For a review phase the comment is
the required review record (the result schema requires a non-empty `comment`), so
if it cannot be posted the post-script fails before the verdict labels move: the
verdict never lands without its record, and a rerun retries the sticky post and
then the transition. For a non-review phase the comment is optional; a failed post
logs a warning and the transition still applies.

## Required runner tools and permissions

The tooling splits by hook:

- The **pre-script and post-script** need `gh` (authenticated by `GH_TOKEN`) and
  `jq`. The **post-script** additionally needs the `fullsend` CLI (the repository
  sticky-comment primitive used for `issues post-comment`, exactly as
  `post-update-openshell.sh` uses it) to post the outcome comment. For the **review
  phases** this comment is the required review record, so `fullsend` is a required
  runner tool asserted at `preflight_check` and a missing CLI fails fast. For the
  non-review phases (`triage`, `spec-implementation`, `code-implementation`) the
  comment is optional, so a missing `fullsend` only logs a warning and the
  transition still applies. These scripts do not use `python3`.
- The **validation step** (`validate-output-schema.sh`) needs `python3` with the
  `jsonschema` package, declared as the harness `preflight_check`. Validation runs
  Python in isolated mode (`python3 -I`) so the untrusted output tree cannot
  shadow the stdlib or the installed package.
- The **review harnesses** (`spec-review`, `code-review`) extend their
  `preflight_check` to also assert the `fullsend` CLI is present (SDLC-HOOK-11),
  because a review verdict comment is required: a missing `fullsend` then fails
  fast at preflight with a clear message instead of wedging every verdict in the
  post-script's required-comment hard-fail on each rerun.

`curl` is deliberately unavailable (base policy). The minted token uses the
harness `role:` and needs permission to read and write issue labels and to read
pull requests and checks. Branch-protection read is optional: reading
`required_status_checks` needs repository administration (read) permission, which
a least-privilege runner token may lack. When the token can read it, the
code-review approval gate is scoped to the required contexts; when it cannot (403)
or the base branch has no required checks, the gate safely falls back to requiring
every reported signal green, and that fallback is the operative gate. Grant the
runner token branch-protection read if you need the scoped behavior in
production.

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
identifier problem (invalid output), `review result is stale` (stale output),
`FAILED to post the required ... review comment` (comment-posting failure, before
any label change, so a rerun is safe), and `final-state verification FAILED`
(label-application failure).

## Tests

`scripts/tests/lifecycle_test.sh` covers the pure contract and decision logic.
`scripts/tests/lifecycle_pre_test.sh` drives `pre-lifecycle.sh` (the bootstrap,
existing-PR, authorization, scope, block, missing-PR, and prerequisite skip
paths) with a mocked `gh` (no local dispatcher). `scripts/tests/lifecycle_post_test.sh`
drives `post-lifecycle.sh` end to end with a mocked `gh` and `fullsend` (no local
dispatcher), including the deferred-approval record path. `scripts/tests/lifecycle_schema_test.sh`
covers the native read-only schema validation. All are auto-discovered by
`make ci-test`.

`scripts/tests/lifecycle_harness_test.sh` is the native Fullsend harness
integration test (SDLC-HOOK-13.02). It does not invoke the local dispatcher, its
shell launcher, or its Go program; it reads the real `.fullsend/harness/*.yaml`
and `.fullsend/schemas/*` files and drives exactly what they declare, in two parts:

- **Part B (runs everywhere, including the plain CI shell runner):** it reads a
  harness YAML, extracts the declared `pre_script`, `validation_loop.script`,
  `validation_loop.schema`, `post_script`, and `env.runner.PHASE`, then runs that
  exact hook chain in order (pre-script records state -> `validation_loop` validates
  the result read-only -> post-script applies and verifies the transition) against a
  mocked `gh` and `fullsend` comment primitive. It also asserts every phase harness
  wires the same three shared hook scripts, so there is one wiring contract across
  phases. This needs no deployed Fullsend binary.
- **Part A (runs where the `fullsend` binary is on `PATH`):** it uses the deployed
  `fullsend dispatch` against the real harness CEL triggers to confirm a synthetic
  GitHub event routes to the correct harness, and is excluded when a competing phase
  label is already present or the scope label is absent. Where the binary is not
  present (the plain CI shell runner on `ubuntu-24.04`, which does not install
  `fullsend`), Part A records a skip with its reason rather than failing, per the
  project's "report the limitation explicitly" guidance; Part B still provides the
  hook-chain coverage there.

Record the deployed Fullsend version Part A was verified against here when running
it on a Fullsend runner: verified with `fullsend version 0.50.0` (dispatch routing
of the lifecycle triggers). The harness `pre_script`/`validation_loop`/`post_script`
runtime ordering and the `FULLSEND_VALIDATED_ITERATION_DIR` output contract should
still be confirmed against the deployed Fullsend version before production reliance;
Part B asserts the scripts the harness references chain together regardless of the
runtime.
