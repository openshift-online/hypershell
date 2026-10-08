# .hypershell/

This directory configures HyperShell agent automation for this repository.
It is to HyperShell agents what `.github/` is to GitHub Actions: repo-local
automation that decorates the repo with agent capabilities without being part
of the core platform.

## agents/

Each subdirectory under `agents/` is one named agent. Two agent categories exist:

- **SDLC pipeline agents** (`skills/agent-loop/` driven) -- process individual GitHub issues/PRs
  through a label state machine. Coordinator runs `discover.sh` to fan out, then one sandbox per item.
- **Cron-driven reviewer agents** -- run on a schedule, fan out over all open PRs, and are
  orchestrated by a separate gitops outer runner rather than the SDLC coordinator.
  Their `discover.sh` and `run.sh` contracts differ from the SDLC pattern.

General platform skills (amber-review, plan/spec, build/reconcile, tooling/, etc.)
remain in `skills/`.

### SDLC Pipeline Agents

| Agent | Queue label(s) | Output label(s) |
|-------|----------------|-----------------|
| `triage` | `agent/allowed` (no triage label) | `agent/workable`, `agent/needs-input`, `agent/duplicate` |
| `spec-implementation` | `agent/workable` or `agent/review-spec-rejected` (no `agent/reviewable-spec`) | `agent/reviewable-spec` |
| `spec-review` | `agent/reviewable-spec` (no verdict) | `agent/review-spec-approved`, `agent/review-spec-rejected` |
| `code-implementation` | `agent/review-spec-approved` (no `agent/reviewable-code`) | `agent/reviewable-code` |
| `code-review` | `agent/reviewable-code` (no verdict) | `agent/review-code-approved`, `agent/review-code-rejected` |
| `release-verification` | `agent/release-pending` (no verdict) | `agent/release-verified`, `agent/release-verification-failed` |

### Cron-driven Reviewer Agents

| Agent | Trigger | Scope |
|-------|---------|-------|
| `reviewer` | cron (every 5 min) | all open PRs in `REPOSITORY` |

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

---

## Reviewer agent contract

The `reviewer` agent has a different contract from SDLC pipeline agents. It is
orchestrated by a gitops outer `run.sh` (not the SDLC coordinator), and its
`discover.sh` and `run.sh` run inside **OpenShell sandboxes** rather than
coordinator-local processes.

### reviewer/discover.sh

Runs inside a **discovery sandbox**. Scans all open PRs for ones that need an
Amber review at their current head SHA.

#### Inputs (environment variables)

| Variable | Required | Description |
|----------|----------|-------------|
| `REPOSITORY` | yes | `owner/repo` |
| `SKIP_DRAFT_PRS` | yes | `true` or `false` |
| `MAX_REVIEWS_PER_RUN` | yes | Maximum PRs to emit per run |
| `COMMENT_ORG_MEMBERS` | no | `true` (default) -- include org members as allowed comment authors |
| `COMMENT_ALLOWED_AUTHORS` | no | Comma-separated extra GitHub accounts to allow |
| `GITHUB_APP_SLUG` | no | GitHub App slug; used to derive bot login when `GET /user` is forbidden |

#### Outputs

- **stdout**: TSV rows, one per PR needing review: `pr_number<TAB>head_sha`. Empty means no work.
- **stderr**: human-readable progress.
- **exit code**: non-zero on fatal API or data-validation errors (unlike SDLC discover.sh, which is always 0).

The TSV format is used instead of JSON because each row carries exactly two
values (PR number and head SHA) that the outer runner splits and passes as
separate `--env` flags to the review sandbox.

### reviewer/run.sh

Runs inside a **per-PR review sandbox**. Invokes Claude (via `inference.local`)
to perform the Amber review and post it to GitHub.

#### Inputs (environment variables)

| Variable | Required | Description |
|----------|----------|-------------|
| `REPOSITORY` | yes | `owner/repo` |
| `PR_NUMBER` | yes | PR to review |
| `EXPECTED_HEAD_SHA` | yes | Head SHA at discovery time; review is aborted if it changes |
| `SKIP_DRAFT_PRS` | yes | `true` or `false` |
| `HYPERSHELL_REF` | yes | Branch/ref to clone as the trusted instruction source |
| `CLAUDE_MODEL` | yes | Model ID |
| `CLAUDE_MAX_ATTEMPTS` | yes | Retry limit for transient model errors |
| `CLAUDE_RETRY_DELAY_SECONDS` | yes | Base delay between retries |
| `GITHUB_APP_SLUG` | no | GitHub App slug for bot-login derivation |

#### Outputs

- **GitHub**: a submitted pull request review + status comment on the PR.
- **No `/tmp/result.json`**: the review outcome is the GitHub review itself, not a local file. The outer runner observes the sandbox exit code.
- **exit code**: `0` on success or graceful skip (PR closed, head changed, draft). Non-zero on Claude failure or unexpected state.

#### Claude invocation

```bash
ANTHROPIC_BASE_URL=https://inference.local \
  ANTHROPIC_API_KEY=unused \
  claude --bare \
  --model "$CLAUDE_MODEL" \
  --dangerously-skip-permissions \
  -p "$prompt"
```

`--bare` (not `--verbose --output-format stream-json`) because output is not parsed
by a harness -- it is logged for human inspection only. The prompt is a fully
inline heredoc that directs Claude to read
`skills/review/amber-review/SKILL.md` from the trusted clone.

There is no `SKILL.md` in `agents/reviewer/` because the Amber review skill is a
general platform skill in `skills/review/amber-review/` that predates the agent
convention. The inline prompt references it directly from the trusted clone path.
