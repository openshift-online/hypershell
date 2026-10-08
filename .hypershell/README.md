# .hypershell/

This directory configures HyperShell agent automation for this repository.
It is to HyperShell agents what `.github/` is to GitHub Actions: repo-local
automation that decorates the repo with agent capabilities without being part
of the core platform.

## agents/

Each subdirectory under `agents/` corresponds to one named agent in the SDLC
automation pipeline. Agents may use skills or deterministic scripts. General
platform skills (amber-review, plan/spec, build/reconcile, tooling/, etc.)
remain in `skills/`.

### Defined Agents

| Agent | Queue label(s) | Output label(s) |
|-------|----------------|-----------------|
| `triage` | `agent/allowed` (no triage label) | `agent/workable`, `agent/needs-input`, `agent/duplicate` |
| `spec-implementation` | `agent/workable` or `agent/review-spec-rejected` (no `agent/reviewable-spec`) | `agent/reviewable-spec` |
| `spec-review` | `agent/reviewable-spec` (no verdict) | `agent/review-spec-approved`, `agent/review-spec-rejected` |
| `code-implementation` | `agent/review-spec-approved` (no `agent/reviewable-code`) | `agent/reviewable-code` |
| `code-review` | `agent/reviewable-code` (no verdict) | `agent/review-code-approved`, `agent/review-code-rejected` |
| `merge-pr` | `agent/review-code-approved` | `agent/merged`, or `agent/cant-merge` on CI failure, conflicts, or queue removal |
| `release-verification` | `agent/release-pending` (no verdict) | `agent/release-verified`, `agent/release-verification-failed` |

### Label state machine

```
[new issue]
    │ human adds agent/allowed
    ▼
  triage ──────────────────────────────► agent/needs-input (wait for human)
    │                                  ► agent/duplicate   (issue closed)
    │ agent/workable
    ▼
spec-implementation ◄──────────────────── agent/review-spec-rejected (retry)
    │ agent/reviewable-spec
    ▼
spec-review ──────────────────────────── agent/review-spec-rejected
    │ agent/review-spec-approved
    ▼
code-implementation ◄──────────────────── agent/review-code-rejected (retry)
    │ agent/reviewable-code
    ▼
code-review ──────────────────────────── agent/review-code-rejected
    │ agent/review-code-approved
    ▼
merge-pr ────────────────────────────── agent/cant-merge
    │ PR enqueued → agent/merged
    ▼
release-verification
    │
    ├─► agent/release-verified
    ├─► agent/release-verification-failed
    └─► agent/blocked
```

---

## discover.sh contract

`discover.sh` runs in the **coordinator pod** (not inside an OpenShell sandbox).
Its job is to find work items and emit them so the coordinator can fan out one
sandbox per item.

### Inputs (environment variables)

| Variable | Required | Description |
|----------|----------|-------------|
| `REPOSITORY` | yes | `owner/repo` (e.g. `openshift-online/hypershell`) |
| `MAX_ITEMS` | yes | Maximum items to emit per run (coordinator sets this) |
| `DRY_RUN` | yes | `true` or `false` |

The `gh` CLI is pre-authenticated in the coordinator pod.

### Outputs

- **stdout**: newline-delimited JSON objects, one per work item. Empty output means no work.
- **stderr**: human-readable progress messages.
- **exit code**: always `0`. Failure to discover is not fatal; coordinator logs stderr.

### Requirements

- Idempotent: running twice with the same GitHub state produces the same output.
- Never emit more than `MAX_ITEMS` lines.
- Validate `REPOSITORY` format before making any API calls.
- Sort items oldest-first (ascending issue/PR number) for stable priority ordering.

---

## run.sh contract

`run.sh` runs **inside an OpenShell sandbox** after the coordinator uploads it.
Skill-based agents invoke Claude against one work item using the `inference.local`
gateway. `merge-pr` executes deterministic GitHub API calls and requires `gh` and
`jq`, with no `SKILL.md`, inference gateway, or `CLAUDE_MODEL` dependency.

### Inputs (environment variables)

| Variable | Required | Description |
|----------|----------|-------------|
| `HYPERSHELL_CHECKOUT` | no | Path to hypershell repo checkout. Default: `/sandbox/hypershell` |
| `REPOSITORY` | yes | `owner/repo` |
| `DRY_RUN` | yes | `true` or `false` |
| `CLAUDE_MODEL` | yes | Set by coordinator in sandbox env |
| Agent-specific | varies | See each agent's `run.sh` for required vars (e.g. `ITEM_URL`, `PR_URL`) |

`merge-pr` requires `REPOSITORY`, `ITEM_NUMBER`, and `DRY_RUN`. It finds PRs from
issue timeline references and PR URLs in the issue body, limited to the same
repository. Missing or ambiguous links are skipped. Pending CI, draft PRs, and
unknown mergeability wait for a future run. Already queued PRs and PRs closed
without merging are skipped. Once the linked PR merges, approval is replaced
with `agent/merged`. Discovery includes closed issues because merging can
automatically close the issue. Approved issues stay discoverable while queued so later queue
removal can be detected from GitHub's timeline across sandbox restarts. A new
approval after a removal permits another enqueue attempt. CI failures, conflicts,
and queue removals replace approval with `agent/cant-merge` on the issue, creating
the label if needed. API failures report infrastructure errors and preserve
approval for retry. Dry runs perform reads and write the result file, but make no
GitHub changes. Enqueue uses GitHub's `enqueuePullRequest` mutation with the
expected head commit; it never merges directly.

### Outputs

- **`/tmp/result.json`**: written before exit. Schema:
  ```json
  {"status": "success|skip|failed", "summary": "one-line outcome"}
  ```
  On failure: `{"status": "failed", "error": "reason"}`.
- **exit code**: `0` on success or skip. Non-zero only on infrastructure failure (missing env var, skill file not found).

### Claude invocation pattern

```bash
ANTHROPIC_BASE_URL=https://inference.local \
  ANTHROPIC_API_KEY=unused \
  claude \
  --model "$CLAUDE_MODEL" \
  --dangerously-skip-permissions \
  --verbose \
  --output-format stream-json \
  -p "$prompt"
```

Where `$prompt` is a heredoc runtime context block concatenated with the content
of the agent's `SKILL.md`.

---

## Coordinator protocol

The gitops coordinator:

1. Clones this repo (`openshift-online/hypershell`) at `TRUSTED_BRANCH` into
   `/sandbox/hypershell` (or a coordinator-local path for discovery).
2. Calls `.hypershell/agents/<name>/discover.sh` to get the work queue.
3. For each emitted JSON line, extracts fields and launches one sandbox Job.
4. Inside the Job, sets agent-specific env vars from the JSON fields, then
   executes `.hypershell/agents/<name>/run.sh`.
5. Reads `/tmp/result.json` after the Job completes and logs the outcome.

The coordinator is responsible for: gateway login, `openshell` installation,
sandbox lifecycle, secret injection, and GitHub App token refresh. These
concerns do not belong in `discover.sh` or `run.sh`.
