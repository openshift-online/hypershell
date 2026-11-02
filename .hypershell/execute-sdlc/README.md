# Local Fullsend dispatcher reference

For installation, Vertex ADC, Jira authentication, daily Make commands, and
troubleshooting, start with the [local development guide](../../docs/development/fullsend.md).

```sh
make fullsend  # Reads the optional, gitignored .hypershell/fullsend.env
```

The shell entry point builds and caches the Go dispatcher, then starts it in
this worktree. Go owns scheduling, worker supervision, durable SQLite state,
authentication polling, and the terminal UI. Python adapters provide forge
classification, Jira/keyring access, and gateway setup. The direct
`funnel-dispatch.sh` interface remains available; run `.hypershell/execute-sdlc/funnel-dispatch.sh --help` for flags.

## Gateway authentication

Before each launch and every 60 seconds while the dispatcher is running,
`openshell whoami` verifies authentication and lets OpenShell refresh a cached
token. Checks continue when all worker slots are busy or the queue is empty.
Set `--auth-check-interval` or `GATEWAY_AUTH_CHECK_SECONDS` to change the interval.
Authentication runs independently of discovery, so a slow GitHub request does
not delay refresh checks. Detaching the dispatcher also stops these periodic checks.
A refresh cannot repair a broken existing sandbox stream, and a network outage
can prevent token refresh as well as sandbox connectivity. A failed check or login is printed to stderr. New launches
pause and the coordinator keeps retrying every 30 seconds, with no attempt limit. Set
`GATEWAY_AUTH_RETRY_SECONDS` or `--auth-retry` to change the interval.

On an interactive terminal, press `s` to sign in or set up a gateway. A failed
check pauses new work and keeps retrying without interrupting the work list.
The sign-in screen uses an arrow-selectable menu. Press Up/Down to choose and
Enter to continue:

- **Reconnect and sign in** uses current connection metadata from `hsctl`.
- **Create a new gateway** lets you select a managed cluster and confirm creation.
- **Keep waiting** returns to automatic connection checks.

Cluster selection and confirmations use the same menu controls. Confirmations
start on Cancel and require an explicit answer. Gateway names use a text field.
Press `d` to toggle connection diagnostics from a menu or progress screen;
Escape cancels sign-in. Browser instructions and device codes remain visible
during connection. Setup output and gateway IDs stay out of the decision menus.

The dashboard menu waits for your choice. The standalone text recovery prompt
defaults to waiting after 30 seconds; its legacy numeric shortcuts remain usable.

If `hsctl` is missing, setup offers to download the latest GitHub release asset
for the host's Linux/macOS and amd64/arm64 architecture, verify `SHA256SUMS`, and
install it in `~/.local/bin`. Declining leaves dispatch paused. Setup uses
`--api-url` / `HYPERSHELL_API_URL`, the saved API, or
`https://api.hyp0.infra.hypershell.app`. It offers `hsctl login --url ...` when the
configured API or authentication needs changing.

The selection is saved atomically in `.hypershell/.funnel-gateway.json` in the
target worktree (gitignored, no credentials). It contains the API URL, gateway
ID/name, and local OpenShell registration name. `--gateway-state` overrides this
path. The saved gateway is reused ahead of shell/global defaults; an explicit
`--gateway NAME` overrides it. Creation is recorded before connecting, so a
restart resumes the same gateway. If the create response is lost, its saved name
must resolve to exactly one gateway before setup proceeds. Failed lookups never
automatically create another gateway or delete the old one.

Reconnect uses a separate, stable OpenShell registration derived from the
gateway ID and API-advertised OIDC metadata. This preserves existing registrations
used by other tools and avoids reusing an `hs-sa-...` service-account client for
interactive login. Gateway replacement waits until active workers finish.

Browser logins are bounded by `--gateway-login-timeout` (default 120 seconds).
A rejected `redirect_uri` can leave the CLI waiting without an exit status; the
timeout stops that wait and offers recovery again. New gateways are polled until
ready, with `--gateway-ready-timeout` (default 600 seconds) before offering
recovery again. Complete the browser sign-in after registration to resume work.

Without an interactive terminal, log in to the same gateway from another
terminal. Checks continue and work resumes automatically. Network failures are
also retried but do not start a browser login. Discovery and worker outcome
tracking continue while an interactive login is pending. A failed worker is
subject to work backoff; auth recovery never blindly replays a running job.

## Shared agents and runtime configuration

`ingest` and `reconcile` each have one agent definition in `.fullsend/agents/`.
The local worker copies Fullsend configuration into a private temporary directory
and applies scheduling and credential transport overrides to the canonical
`harness/ingest.yaml` or `harness/reconcile.yaml` under the same agent name.
Image, role, model, timeout, inputs, and result validation stay canonical;
the shared prompts describe inputs, tools, and workflow, not how they are launched.
The hosted configuration and label-event triggers remain unchanged. Temporary
configuration is removed when the worker exits. Historical job keys retain their
old names to preserve ownership and deduplication across upgrades.

## Lifecycle modes

The default `--lifecycle jira` runs the complete orchestration path:

1. Run the `ingest` harness for the scope before selecting work. It follows
   `.fullsend/agents/ingest.md`, including both Jira and GitHub origins,
   exact canonical links, pagination, and the human `agent/allowed` gate.
2. Discover authorized GitHub records, including closed records. Reconcile each
   through `reconcile` before dispatching an eligible phase.
3. Classify open records through the existing Fullsend CEL harness triggers.
   Run at most one phase per record at a time.
4. Reconcile after completed attempts, including failures that may have made
   partial changes. Watch mode also checks for merged PRs and closed records and
   checks for newly scoped issues every 900 seconds (`--lifecycle-interval`).
   Unchanged issues get a periodic Jira drift check six hours after their last
   successful sync (`--reconcile-interval`, default 21600 seconds). Changes to
   issue content, labels, PR state/head, or a completed worker attempt trigger
   sync on the next discovery, without waiting for that interval. Restarting or
   crossing a clock boundary does not itself invalidate a successful sync.

These are agent-executed workflows, with a result validator that rejects partial
failures; they do not replace the canonical workflow instructions with a second
implementation of Jira synchronization.

To deliberately run only the GitHub phases, add:

```sh
--lifecycle github-only
```

This mode explicitly reports that Jira intake and reconciliation are disabled.
`--dry-run` starts no workers or login and prints the next eligible jobs. In Jira
mode that may be just intake when it has not run yet; it is not a simulated Jira
sync or a complete prediction of later phase outputs.

## Jira authentication

`--jira-auth auto` is the default (also configurable as `JIRA_AUTH_MODE`):

- With both `JIRA_USER_EMAIL` and `JIRA_TOKEN` supplied, use API-token mode.
  `JIRA_BASE_URL` defaults to `https://redhat.atlassian.net`.
- Otherwise, reuse the selected `acli` Jira OAuth account and site. No Jira env
  variables are needed; an email or token left over alone does not select
  API-token mode. An explicit `JIRA_BASE_URL` must match that account's
  site; the dispatcher never silently switches accounts.

Use `--jira-auth acli` or `--jira-auth api-token` to select a mode explicitly.
Explicit API-token mode requires both credentials and fails if either is missing.
For ACLI mode, sign in on the host with `acli jira auth login --web`. The host
needs PyYAML and access to its OS keyring (`secret-tool` on Linux or `security`
on macOS). Metadata is read from `ACLI_CONFIG_DIR`, or
`${XDG_CONFIG_HOME:-~/.config}/acli`. The installed ACLI keyring format was
verified with Linux ACLI 1.3.39; it is an internal storage format, so unfamiliar
formats fail with an actionable error instead of guessing.

Before each lifecycle attempt, the host runs a read-only, scope-filtered ACLI
search to exercise its normal token refresh, then snapshots only the current
access token. Refreshes initiated by this dispatcher are serialized across its
workers. The sandbox receives a private credential file, never the desktop
keyring, ACLI profile directory, or refresh token. Changing the active account
requires restarting the dispatcher.

The sandbox uses `node .fullsend/scripts/jira-request.mjs` for both auth modes. This
reuses ACLI's authenticated session without running the ACLI binary inside the
sandbox. The helper sends Basic auth to the tenant for API-token mode, or Bearer
auth to `api.atlassian.com/ex/jira/<cloud-id>` for OAuth. The local `local-dispatch-jira`
network profile permits these Jira endpoints for Node. Requests cannot supply
arbitrary destination URLs and redirects are refused. This also supports Jira
REST operations that ACLI does not expose, such as artifact remote links.

Access-token snapshots expire. An expired or revoked snapshot fails the attempt
visibly; watch mode retries with persistent backoff and obtains fresh host
credentials. Sandbox workers never refresh copied tokens. If ACLI needs browser
reauthentication, sign in on the host and the next retry picks it up. OAuth scope
or permission failures are surfaced; API-token mode remains available when the
ACLI grant cannot authorize a required Jira operation.

## Durable coordination and retries

State defaults to `$XDG_STATE_HOME/hypershell/funnel/<repository-hash>` (or
`~/.local/state/hypershell/funnel/<repository-hash>`). All worktrees and scopes
for the same repository share a SQLite database and repository lock on this
machine. A second dispatcher waits visibly instead of launching duplicate work.
`--state-dir` changes the shared root; using different roots defeats coordination.
This is local coordination, not a distributed lock across different machines.

Workers record outcomes independently and hold per-run locks inherited by their
Fullsend subprocesses. The dispatcher alone holds the scheduling lock.
The Bubble Tea / Lip Gloss interface shows a compact work list with one running
count. Use Up/Down (or j/k) to select a row, Enter to view its log, `d` for run
details, `r` to request a retry of the selected failed job, `s` for gateway sign-in, and Escape to return. Logs and details share
an issue/agent/status header. Details group the last attempt and log paths, hide
empty problem sections, and scroll with Up/Down without changing the selected
agent. In the log view, Up/Down
scrolls within the latest 64 KiB; the full file remains available on disk. Long
issue titles shorten in the middle. The cross-issue intake step is labeled
"Find and link issues". Failed jobs say "Retry in 30s" or "Ready to retry".
Color supplements text: cyan for active work, green for completion, amber for
connection warnings, red for failures, and amber for unknown outcomes. Active status comes from the latest
recognized worker-log phase, not a liveness guarantee.

`--plain` disables the dashboard; `NO_COLOR` disables color. Redirected output
stays plain. The copy follows
[GOV.UK interface writing guidance](https://www.gov.uk/service-manual/design/writing-for-user-interfaces)
and [NN/g UX writing guidance](https://www.nngroup.com/articles/ux-writing-faqs/):
use familiar task names, keep labels brief, and reveal details when needed.

Gateway diagnostics are saved to
`dispatcher.log` in the log directory.

`q` or Ctrl-C closes the dashboard and prints complete, copyable log paths for all
failed attempts in the scope and any running or unresolved workers. Paths are
never clipped to the terminal width. Ctrl-C stops scheduling and leaves
already-running workers to finish. A restarted
coordinator immediately resumes monitoring those workers and can schedule new
work in free slots. It reads their durable outcomes without launching duplicates.
If another dispatcher is still active, the new process monitors recorded work
and automatically takes over when the scheduling lock is released.

Workers launched by older versions hold the shared scheduling lock. During this
one-time transition, the new dispatcher displays their state and resumes
scheduling as soon as they finish. It does not kill or duplicate them. Do not
downgrade to the older dispatcher while workers from the new version are active. If a
worker disappears without recording an outcome, the record is marked `uncertain`
and all its phases pause. Do not delete the state to bypass this condition.
Inspect the logs and remote sandbox before explicitly allowing another attempt:

```sh
make fullsend-status
.hypershell/execute-sdlc/funnel-dispatch.sh agent/scope/YOUR_GITHUB_USERNAME/fullsend \
  --env-file .hypershell/fullsend.env --retry-uncertain 123:code-implementation --watch
```

To retry a failed job immediately without waiting for backoff, select it and press
`r`, or submit a request from another terminal:

```sh
.hypershell/execute-sdlc/funnel-dispatch.sh agent/scope/YOUR_GITHUB_USERNAME/fullsend \
  --retry 123:code-implementation
```

The command records the request and exits without waiting for the active
dispatcher's lock. If no dispatcher is running, start it with `--watch` afterward.
Only the backoff delay is cleared. Scope, authorization, lifecycle prerequisites,
and available worker slots still apply. Running and successful jobs cannot be retried this way. In the interactive UI,
pressing `r` on an unknown outcome opens a confirmation: verify the previous
remote sandbox has stopped, then press `y` to retry or `n`/Escape to cancel.
The dispatcher rejects confirmations for changed runs and live local workers.
The noninteractive `--retry-uncertain` option remains available.

Failed jobs retry with per-item, per-agent exponential backoff, starting at 60
seconds and capped at 3600 seconds. Configure with `--backoff` and
`--backoff-max` (or `FAILURE_BACKOFF_SECONDS` and `FAILURE_BACKOFF_MAX_SECONDS`).
The attempt count, next eligible time, and unique log path survive restarts.
Successful jobs with unchanged inputs are not repeated; relevant issue changes
or a new PR head allow a new attempt. Human authorization is rechecked immediately
before launching a record's worker.

Watch mode rediscovers every `--poll-interval` seconds (default 30) and after
worker completions, without waiting for a batch to drain. Without `--watch`, the
coordinator processes eligible work once, with lifecycle reconciliation after
attempts, and exits nonzero for failed or uncertain work. Backoff retries are
performed by watch mode or a later invocation.

## Discovery and review selection

GitHub REST lists are paginated through all pages, without a 200-issue or search
result cap. A failed API call or CEL evaluation is reported rather than treated
as an empty successful discovery.

Review selection requires exactly one linked, open PR. Links are explicit PR
URLs in the issue body or closing references such as `Fixes #123` in the PR body.
Multiple linked PRs, including historical links, are reported for human
resolution. A descriptive Jira-key mention, branch-name resemblance, or newest
PR is insufficient. Jira reconciliation uses the same ambiguity rule and can
also resolve a merged PR.

## Verification

```sh
bash scripts/funnel-dispatch_test.sh
bash scripts/funnel-jira-auth_test.sh
bash scripts/funnel-gateway-setup_test.sh
bash scripts/funnel-terminal-ui_test.sh
make check
```

Tests use fake CLIs and temporary state, including real subprocess restart, resumed monitoring, free-slot scheduling,
Python-worker adoption, legacy-lock migration, lock inheritance, and PTY
keyboard/detach behavior. The dispatcher suite also runs Go race tests and vet. They do not create sandboxes or write to GitHub or Jira. The
Fullsend classifier can validate the local harness configuration independently
of executing an agent. ACLI authentication was also verified with a read-only
Jira `/myself` request inside an isolated container using an access-token snapshot.
Full OpenShell lifecycle execution and Jira writes still need a live smoke test.

## Worker checkouts and build tools

Each new worker snapshots `--target-repo` into a private temporary checkout.
Git objects are copied independently, so linked worktrees work inside a sandbox
without referring to the host filesystem. Tracked edits, deletions, executable
bits, symlinks, and non-ignored untracked files are preserved. Staged changes
become unstaged changes against HEAD. Ignored files and host Git configuration
are excluded. The worker cleans up its snapshot when Fullsend exits; detaching
the UI does not remove it or interrupt the worker. Edits made after the snapshot
are picked up by the next worker, not an already running one.

Harnesses use the digest-pinned HyperShell Fullsend sandbox component published
by the existing Konflux pipeline. This image includes Go 1.26.7, Node, npm,
Corepack, make, and GCC. Use `corepack pnpm` (or
`make PNPM='corepack pnpm' <target>`) for the repository's pinned pnpm version.
The built-in `package-registries` provider grants read-only dependency access to
npm, PyPI, and the Go proxy/checksum service. It does not grant curl general
internet access. Container-engine, cluster, and browser-dependent tests still
require their documented infrastructure. Existing workers retain the image and
policy they started with; these settings apply to newly created sandboxes.

## Issue overview and history

The overview defaults to agent groups, using the registered Fullsend agent name
as each heading and showing issue rows underneath. Groups with running jobs come
first, followed by groups needing attention. Completed runs are collapsed by
default; press `c` to show or hide them. Press Tab to switch to issue grouping,
which shows the highest-priority current job per issue. Switching views preserves
the selection where possible. `Run completed` describes an agent
run, not completion of the entire issue.

Press `H` on an issue to view its newest 200 recorded attempts, including retries,
with local start times and outcomes. Select an attempt and press Enter for its
log or `d` for details. Escape returns from logs to history, then to the issue
overview. `H` refreshes history. Retry applies to the current job from the
overview: failed jobs retry directly, while unknown outcomes require confirmation
that the previous sandbox has stopped. Historical failures do not imply that a
retry is scheduled.

## Terminal presentation

The Lip Gloss theme adapts PatternFly's semantic status colors, neutral group
surfaces, selection treatment, and consistent sections, with a restrained Red Hat
red accent. It supports light/dark terminal backgrounds, reduced color palettes,
and `NO_COLOR`; the terminal controls the font. References:
https://www.patternfly.org/foundations-and-styles/colors/ and
https://www.redhat.com/en/about/brand/standards/color.

Completed runs show elapsed time since the recorded finish, for example
`Run completed · 3min ago`, including historical attempts after a restart.
Records without a finish timestamp do not invent one. Logs and details fill a
fixed-height viewport, with a scrollbar, line ranges, and top/end indicators.
Scrolling clamps at the first and last page. The keyboard action bar stays at
the bottom when resizing or scrolling.

To render representative terminal frames for visual inspection:
`FUNNEL_PREVIEW_DIR=/existing/directory go test -run TestTerminalPreviewFrames`
from `.hypershell/execute-sdlc/dispatcher` writes ANSI overview/log/details/sign-in frames.

Text contrast tests cover light/dark themes and standard xterm 256-color
conversion, targeting 4.5:1 for text and 3:1 for scrollbar components. Selection
also has a text marker, and status labels do not rely on color alone. This is
not a screen-reader or terminal accessibility certification; use `--plain`
for linear output. Main lists, history, logs, details and sign-in diagnostics
share fixed-height viewports, showing scrollbars when their content overflows.


Notifications appear in a bordered bottom-right card above the keyboard action
bar on every screen. Press `x` to dismiss (`Ctrl-X` while entering a gateway
name). Success confirmations disappear after eight seconds; warnings remain
until dismissed or replaced by a newer notification. Dismissing the card does
not clear the underlying error or sign-in result.

Implementation deduplication includes the linked PR number and head commit.
When a code or spec rejection label is present, it also includes the latest
GitHub event ID that added that rejection label. A second rejection therefore
allows another implementation attempt, even when labels and the commit match
a prior successful run. Repeated discovery of the same event stays deduplicated
across restarts. Discovery and pre-launch authorization use the same inputs;
missing rejection history or ambiguous PR links pause that item with an error.


Navigation is consistent across lists, logs, details, help, and gateway menus:
`j/k` or arrows move; `gg/G` jump to the beginning/end; `Ctrl-d/Ctrl-u` move half
a page; `Ctrl-f/Ctrl-b` or Page Down/Up move a full page. `Enter/l` opens a run
or menu choice, and `Esc/h` goes back. `H` opens/refreshes history; `s` signs in.
`?` opens the shortcut reference. Text fields keep normal typing behavior.

In Agents, Issues, and history, `/` edits a live, case-insensitive filter across
issue number, title, exact agent name, and status. Space-separated terms must
all match. Enter keeps the filter; Escape cancels edits, or clears an applied
filter. `n/N` moves to the next/previous filtered result. Completed runs remain
hidden until `c` includes them. `o` cycles left to right through issue number, status, and duration; `O` reverses direction.
An up/down arrow appears on the active column heading. Duration is elapsed time
for running jobs and total runtime for finished attempts; unknown durations show
`-` and sort last in either direction. Agent
groups remain together. Filtering and sorting affect display only, not scheduling.
