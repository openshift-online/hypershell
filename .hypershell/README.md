# .hypershell/

This directory configures HyperShell agent automation for this repository.
It is to HyperShell agents what `.github/` is to GitHub Actions: repo-local
automation that decorates the repo with agent capabilities without being part
of the core platform.

## agents/

Each subdirectory under `agents/` corresponds to one named agent in the SDLC
automation pipeline. Only `skills/agent-loop/` agents belong here. General
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
    │ agent/review-code-approved → PR merged
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
It invokes Claude against one work item using the `inference.local` gateway.

### Inputs (environment variables)

| Variable | Required | Description |
|----------|----------|-------------|
| `HYPERSHELL_CHECKOUT` | no | Path to hypershell repo checkout. Default: `/sandbox/hypershell` |
| `REPOSITORY` | yes | `owner/repo` |
| `DRY_RUN` | yes | `true` or `false` |
| `CLAUDE_MODEL` | yes | Set by coordinator in sandbox env |
| Agent-specific | varies | See each agent's `run.sh` for required vars (e.g. `ITEM_URL`, `PR_URL`) |

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
