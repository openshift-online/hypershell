---
name: code-review
description: >
  Code Review Workflow
---

# Workflow

Code review workflow that classifies issues as:
- `agent/review-code-rejected`
- `agent/review-code-approved`

Review is rejected for any of the following reasons:
- Implementation review has findings
- Failing CI
- Merge conflicts
- Needs rebase

## Input

```text
$GITHUB_ISSUE_URL [--dry-run]
```

Supported arguments:
- GITHUB_ISSUE_URL -- execute against the provided GitHub issue.
- --dry-run -- [Optional] Execute the workflow, but without any WRITE actions.

## Workflow

### Step 1: Read Issue and Linked PR

Using the GitHub API, fetch `$GITHUB_ISSUE_URL`.

Read the issue title and body and identify the linked PR.

Output: (`$LINKED_PR`, `$ISSUE_CONTENTS`)

### Step 2: Review the Implementation

Execute [amber-review](../../../skills/review/amber-review/SKILL.md) against `$LINKED_PR`. Review in the
context of `$ISSUE_CONTENTS`.

Review must be rejected for any of the following reasons:
- Implementation review has findings
- Failing CI
- Merge conflicts
- Needs rebase

Verdict must be one of:
- `agent/review-code-rejected`
- `agent/review-code-approved`

Output: (`$VERDICT`, `$REJECTION_RATIONALE` (NULL or string))


### Step 2.5: Debug a Failing OpenShift e2e Check

A bare "the OpenShift e2e failed" rejection sends the next implementation pass in
blind. When the failing CI is specifically the ephemeral OpenShift e2e run, extend
its environment and gather a concrete diagnosis so the fix is targeted.

Run this step only when the check named `Tests / E2E / OpenShift` on `$LINKED_PR`
is failing (it is one input to the `Failing CI` rejection reason above). Skip every
WRITE action when `--dry-run` is set.

1. Keep the ephemeral environment alive. Post the PR comment `/pr-extend` on
   `$LINKED_PR`. This retains and (re)deploys the `hypershell-ci-pr-<n>` namespace
   group for the current head commit and refreshes its expiry, so the reaper does
   not reclaim it mid-investigation (see
   `specs/platform/ephemeral-pr-environments.spec.md`, CICD-PREN-03/05/06). `<n>`
   is the PR number. `/pr-extend` is honored only from a commenter with
   write/maintain/admin permission on the origin repo (CICD-PREN-03); if the
   agent's GitHub identity lacks that permission the comment is acknowledged but
   changes nothing. Treat the extend as best-effort: the `pr-env-diagnostics-<n>`
   artifact in step 2 is uploaded before teardown and does not depend on the
   environment still being up, so diagnosis still proceeds either way.

2. Gather the CI-side evidence, which is always reachable headlessly through `gh`:
   - Read the failing job logs: `gh run view --log-failed` for the run that
     produced the failing `Tests / E2E / OpenShift` check.
   - Download the failure diagnostics the suite uploads: the `pr-env-diagnostics-<n>`
     artifact (`gh run download <run-id> -n pr-env-diagnostics-<n>`), plus
     `e2e-diagnostics` / `e2e-suite-logs` when present.

3. Best-effort live inspection of the deployed environment. Do NOT fetch
   credentials or log in from this skill. Only inspect the live env if the agent
   ALREADY has an `oc` session that can reach the `hypershell-ci-pr-<n>` namespace
   (a human or the surrounding environment logged in beforehand). Probe once, and
   if the namespace is reachable, read pod status, recent events, and logs for any
   crash-looping or failing component:

   ```bash
   # If this first call errors, the namespace is unreachable - stop and skip to
   # the fallback below rather than trying to obtain access.
   oc get pods -n hypershell-ci-pr-<n>
   oc get events -n hypershell-ci-pr-<n> --sort-by=.lastTimestamp
   oc logs -n hypershell-ci-pr-<n> <failing-pod> --previous
   ```

   If the namespace is not reachable (no active `oc` session, `oc` not installed,
   or access denied), do NOT attempt to obtain credentials. Print a short notice
   that live cluster access is unavailable and CONTINUE with only the CI logs and
   the diagnostics artifact from step 2. Do NOT block, retry, or fail the review on
   missing cluster access - the `pr-env-diagnostics-<n>` artifact already captures
   pods, descriptions, logs, events, and routes for the failed run.

4. Fold what you found - failing suite step, pod/log evidence, probable root cause -
   into `$REJECTION_RATIONALE`, so the next `code-implementation` pass acts on a
   concrete diagnosis instead of re-discovering the failure from scratch.

5. Name the failing phase or step ID explicitly in `$REJECTION_RATIONALE` (for
   example `P2.5`, or the whole `Phase-P2`). The Go E2E suite runs each phase and
   step in isolation, so a named step lets the next `code-implementation` pass
   reproduce ONLY that step instead of the whole suite, which is far faster:
   - Whole phase: `make e2e E2E_RUN=Phase-P2`
     (or `go -C tests/e2e test . -run 'TestE2E/TestPhases/Phase-P2' -v`)
   - Single step: `make e2e E2E_RUN='P2\.5'`
     (or `go -C tests/e2e test . -run 'TestE2E/TestPhases/.*/P2\.5' -v`)
   - Reproduce the `Tests / E2E / OpenShift` failure on the same driver with
     `E2E_INFRA_DRIVER=openshift make e2e E2E_RUN=...`.
   The suite provisions the primary gateway in setup (`ensurePrimaryGateway`), so a
   step that needs it provisions it on demand when run alone; set
   `E2E_PRIMARY_GATEWAY_ID=<id>` to adopt an already-running gateway and skip its
   teardown, cutting single-step turnaround further.


### Step 3: Communicate Outcome

_Note: If a label does not exist, create it._

Apply `$VERDICT` label to `$GITHUB_ISSUE_URL`.

**Protected labels (never remove).** The human gate `agent/review-spec-blocked`
and the role markers are NOT phase labels and MUST NOT be removed by this step,
even while normalizing phase state: `agent/review-spec-blocked` (human spec gate),
`agent/allowed` (authorization gate), `agent/scope/*` (scope selector), and
`agent/execution-record` (canonical-link marker). This step mutates ONLY the phase
labels named in the verdict branches below - it is not a license to remove
everything but one label.

**Always post the review as a comment on `$GITHUB_ISSUE_URL`, for BOTH an
approved and a rejected verdict.** An approved review MUST NOT be silent: the
record of the review must be visible. Write the comment in
[simplified technical english](https://en.wikipedia.org/wiki/Simplified_Technical_English).
The comment MUST state:
- The verdict (`$VERDICT`).
- A substantive rationale: the strengths of the implementation, how it meets the
  intent in `$ISSUE_CONTENTS`, and any non-blocking observations.
- For a rejected verdict, the blocking reasons (`$REJECTION_RATIONALE`), with
  inline code review comments where they apply.

IF `$VERDICT` == `agent/review-code-approved`:
- Remove label `agent/reviewable-code` from `$GITHUB_ISSUE_URL`.
- Post the review comment described above, stating the strengths and any
  non-blocking observations.
- The PR is ready to merge. A human must perform the merge.

IF `$VERDICT` == `agent/review-code-rejected`:
- Count prior rejections: fetch the issue event timeline. Count how many times
  `agent/review-code-rejected` has been applied **since the most recent removal
  of `agent/blocked`** (or since the beginning of the timeline if `agent/blocked`
  has never been removed). Include this rejection in the count.
- If the rejection count is 10 or more:
  - Remove label `agent/reviewable-code` from `$GITHUB_ISSUE_URL`.
  - Apply label `agent/blocked` to `$GITHUB_ISSUE_URL` instead of re-queuing.
  - Add a comment explaining the issue has been rejected 10 or more times and needs
    human review before the agent retries. Tell the human to remove `agent/blocked`
    and `agent/review-code-rejected` to resume the loop.
  - STOP.
- Otherwise:
  - Remove label `agent/reviewable-code` from `$GITHUB_ISSUE_URL`.
  - Post the review comment described above, communicating `$REJECTION_RATIONALE`,
    that adheres to the [simplified technical english](https://en.wikipedia.org/wiki/Simplified_Technical_English) standard.
