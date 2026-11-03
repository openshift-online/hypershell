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

The `pre_script`, `validation_loop`, and `post_script` hooks, the `skipped=true` / `reason` protocol
from a pre-script, the `FULLSEND_VALIDATED_ITERATION_DIR` output contract, and
harness-level output-schema enforcement follow the
[harness field reference](https://fullsend.sh/docs/reference/harness-reference)
and [ADR 0022](https://fullsend.sh/docs/ADRs/0022-harness-level-output-schema-enforcement).
Native skipping is tested against installed Fullsend 0.50.0 using stubbed
external CLIs: a denied precondition produces `skipped=true` plus `reason`, and
Fullsend exits before creating a sandbox. Custom pre-script outputs are logged
and exposed to CI outputs; Fullsend does not export them as post-script environment
variables. The pre-script therefore atomically writes a mode-0600
`.lifecycle-state.json` next to `FULLSEND_PRESCRIPT_OUTPUT` in the host run root.
The post-script runs in that root and reads the private snapshot. It verifies the
file ownership/mode, phase, repository, issue, PR number and recorded head, never
sources shell text, and never obtains trusted identity from agent output.
Missing or invalid review state is a non-zero failure, so the dispatcher can
retry rather than record an unapplied verdict as successful.

The snapshot remains in the run artifacts for recovery. An operator recovering
an older run can supply the original trusted `LIFECYCLE_*` values explicitly;
the agent's reported head is not a substitute for pre-script evidence. The
script-chain test uses separate clean environments and the host state file,
without manually forwarding the relay values. The full sandbox-to-post-hook
path still requires validation against a live gateway.

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

| Phase | Outcome | Issue: add | Issue: remove | Linked PR |
|-------|---------|------------|---------------|-----------|
| triage | workable | `agent/workable` | none | none |
| triage | needs-input | `agent/needs-input` | none | none |
| triage | duplicate | `agent/duplicate` (+ close issue) | none | none |
| spec-implementation | completed | `agent/reviewable-spec` | `agent/workable`, `agent/review-spec-rejected`, stale `agent/review-spec-approved` | none |
| spec-implementation | blocked | `agent/blocked` | none | none |
| spec-review | approved | `agent/review-spec-approved` | `agent/reviewable-spec`, `agent/review-spec-rejected` | none |
| spec-review | rejected | `agent/review-spec-rejected` (+ `agent/blocked` at limit 3) | `agent/reviewable-spec`, `agent/review-spec-approved` | none |
| code-implementation | completed | `agent/reviewable-code` | `agent/review-code-rejected`, stale `agent/review-code-approved`, `agent/code-conflicts` | none |
| code-implementation | blocked | `agent/blocked` | none | none |
| code-review | approved | `agent/review-code-approved` | `agent/reviewable-code`, `agent/review-code-rejected` | Add and verify `agent/review-code-approved` before completing issue approval |
| code-review | rejected | `agent/review-code-rejected` (+ `agent/blocked` at limit 10) | `agent/reviewable-code`, `agent/review-code-approved` | Remove `agent/review-code-approved` |

The columns above distinguish the execution record (issue) from its linked PR.
An eligible code-review approval labels **both**. The post-script verifies the PR
label before completing the issue transition. If the PR write fails, the attempt
fails and the issue remains recoverable; reapplication can also repair a missing
PR label when the issue already has its approval. Scope and authorization labels
on the execution record are not copied to the PR.

Readiness maintenance also changes these targets:

| Condition | Execution record (issue) | Linked PR |
|-----------|--------------------------|-----------|
| Approval deferred by pending/failing CI or uncertain PR readiness | Keep `agent/reviewable-code` on a fresh review; post a deferral record | Remove any `agent/review-code-approved`; do not grant queue approval |
| Confirmed conflicts after review or approval | Add `agent/code-conflicts`; remove stale code approval, rejection, and reviewable labels | Remove `agent/review-code-approved` first |
| Previously approved PR becomes ineligible without confirmed conflicts | Return to `agent/reviewable-code`; remove stale code approval/rejection | Remove `agent/review-code-approved` first |

The code-implementation completed transition preserves `agent/review-spec-approved`
as a completed prerequisite.

## Evidence and eligibility (SDLC-HOOK-07, SDLC-HOOK-12)

Before applying a transition the post-script re-reads GitHub and:

- Rejects a review result as stale if the PR head moved since the pre-script
  recorded it, or if the agent's reviewed `head_sha` does not match.
- Requires an open, non-draft PR with known mergeability and no confirmed
  conflicts, and gates code-review approval on the PR head's checks being green. When the base
  branch defines required status checks through classic branch protection or
  active repository/organization rulesets, the gate combines their contexts and is scoped
  to those contexts only (an advisory or always-pending non-required check does
  not block, and a required context that has not reported yet is treated as
  pending). Failed API responses, including the classic protection endpoint's
  404 response on a ruleset-only repository, are discarded. Rules are paginated.
  When neither source supplies required contexts,
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

The phase post-scripts and shared readiness maintenance hook write phase labels
through the same transition contract. The maintenance hook handles later CI and
mergeability changes; its scheduling is described under CI and mergeability
recovery below. `ingest` writes the
execution-record marker and the scope label; `reconcile` writes Jira status from
the GitHub phase labels. Neither writes a GitHub phase label, so there is no
competing writer.

Documented exception: the `update-openshell` agent opens its own pull request
outside the SDLC funnel (it is not driven by a work-item issue through the phase
harnesses) and applies `agent/reviewable-code` to that pull request directly. This
is a deliberate bootstrap hand-off, not a competing writer on a funnel issue: the
`update-openshell` pull request has no lifecycle pre-script or post-script in its
harness, so no phase script ever writes labels for it. Routing it through the
shared contract is tracked as a follow-up; until then this is an
acknowledged agent-side exception to the shared script contract,
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
pull requests and checks. Classic branch-protection read is optional: reading
`required_status_checks` needs repository administration (read) permission, which
a least-privilege runner token may lack. When the token can read it, the
code-review approval gate combines those contexts with the required checks from
`repos/{owner}/{repo}/rules/branches/{branch}`. That endpoint resolves active
rulesets applying to the branch. An unavailable source never contributes its error
body as a context. When neither source yields required checks, the gate falls
back to requiring every reported signal green. Ensure the runner can read
applicable protection and rules if you need the scoped behavior in production.

## Recovery (SDLC-HOOK-10)

The validated result is retained in the Fullsend run artifacts at
`iteration-<N>/output/agent-result.json`, and the iteration Fullsend validated is
named by `FULLSEND_VALIDATED_ITERATION_DIR`. The post-script is idempotent and
re-checks live state before acting, so an operator can reapply a validated result
without rerunning the agent by invoking `scripts/post-lifecycle.sh` from the run
directory with the same `PHASE`, `ISSUE_URL`, and recorded `LIFECYCLE_*`
environment (where the deployed Fullsend interfaces support re-invoking a
post-script).

When `FULLSEND_VALIDATED_ITERATION_DIR` is set (the normal flow), the post-script
acts only on the iteration Fullsend marked as validated. When it is unset, the
post-script falls back to the highest-numbered `iteration-<N>/output/agent-result.json`.
That fallback is a HUMAN RECOVERY path only: there is no validated-iteration marker,
so the post-script cannot prove the selected result passed the native
`validation_loop`. It therefore logs a loud `WARNING` naming the path, and it still
re-checks every trusted field (schema version, phase, outcome, the `pr_number`
against the recorded PR, and head freshness) before acting, but it does not re-run
the full JSON Schema. Reserve this path for a deliberate operator recovery of a run
whose validated-iteration marker was lost, not for normal operation.

Failure classes are distinguishable from the post-script logs:
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


## CI and mergeability recovery

Code readiness includes the PR head/base, state, draft flag, mergeability, and
normalized required checks. The local dispatcher uses that snapshot in job
fingerprints. When a deferred review's checks turn green, another review attempt
can run even though its issue labels and PR head stayed the same. This currently
reruns review; it does not automatically replay a saved agent artifact.

The code-review post-script applies and verifies `agent/review-code-approved` on
the linked PR as well as the issue. PR label failure prevents issue completion;
reapplication repairs partial transitions after rechecking freshness. Approval
requires a known mergeable, open, non-draft PR with green checks.

To observe changes that occur after the post-script exits, use the shared
maintenance hook:

```sh
python3 .fullsend/scripts/refresh-code-state.py \
  --issue-url https://github.com/OWNER/REPO/issues/123 \
  --pr 456 --scope agent/scope/YOUR_SCOPE
```

Add `--inspect` for read-only observation. The local watch loop invokes this for
code-stage issues automatically and avoids mutations while a worker is running
or its outcome is unknown. Hosted execution needs its scheduler to invoke this
same command periodically or on PR/check changes; the post-script cannot watch
a PR after its process exits, and this change does not install a hosted trigger.

Confirmed conflicts revoke the PR queue label and move the issue to
`agent/code-conflicts`. Implementation repairs the PR, then returns it to review.
The conflict is not recorded as a review rejection. Unknown mergeability does
not trigger conflict repair; previously approved records are returned to review
while eligibility is uncertain. Human blocks and authorization/scope revocation
stop maintenance writes. A CI or mergeability change is not itself a new code
review approval, and new head commits still need fresh review.


`lifecycle_preflight_test.sh` loads both review harnesses through the installed
Fullsend CLI using dummy settings and a failing OpenShell stub. It verifies
that native environment validation succeeds without opening a gateway or
creating a sandbox. Review preflight commands contain no shell temporary
`${...}` expressions, which Fullsend would interpret as host-variable templates.


Paginated GitHub API calls use `gh api --paginate --slurp` and pipe the returned
JSON through the standalone `jq` command. They do not combine `--slurp` with
`gh --jq`, which the installed GitHub CLI rejects. The hook fixtures enforce
that CLI restriction and cover multi-page check runs and rejection timelines.
