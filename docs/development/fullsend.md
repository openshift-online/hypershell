# Run Fullsend locally

Use the local dispatcher to run HyperShell's agent lifecycle from your worktree.
The coordinator and dashboard run on your laptop; agents run in sandboxes on an
OpenShell gateway. You do not need a local Kubernetes cluster.

## Before you begin

Run the commands in this guide from the root of a HyperShell checkout or worktree.
You will need:

- GitHub access to the HyperShell repository and permission to perform agent work.
- A Google Cloud project approved for Claude inference, a supported model and
  region, and an account with access to that project. Ask your project
  administrator for these values; they cannot be inferred from your GitHub login.
- A Jira account with access to the team's project, unless you explicitly choose
  the GitHub-only lifecycle.
- Access to a HyperShell API and an OpenShell gateway, or permission to create a
  gateway on an available cluster.

The steps below install the local prerequisites, create your configuration,
authenticate each service, and start the dispatcher. After setup, the daily
command is `make fullsend`.

## First-time setup

### 1. Install host tools

Have these commands on your PATH:

| Tool | Purpose |
| --- | --- |
| Go 1.25+ | Build the local dispatcher; cached binaries are reused |
| Python 3, Bash, Git, Make | Launch, credential adapters, isolated worktree snapshots |
| `gh` | GitHub discovery and agent access |
| `fullsend` | Run the repository's harnesses; this workflow was exercised with 0.50.0 |
| `openshell` | Connect to the remote gateway and manage sandboxes |
| `jq` | Lifecycle result validation |
| `gcloud` | Create or renew local Google ADC credentials |
| `acli` | Reuse your Jira OAuth login; optional when using an API token |

Use your team's Fullsend installation. For OpenShell, use the version matching
your gateway. With `OPENSHELL_VERSION` set to that gateway release, run
`sh scripts/install-openshell.sh` and ensure `~/.local/bin` is on PATH. See the
[installer](../../scripts/install-openshell.sh). Gateway setup offers to install `hsctl`
from a GitHub release, selecting your OS/architecture and verifying checksums.

The Jira lifecycle needs PyYAML in the Python used by `python3`.
ACLI mode also needs OS keyring access:
`secret-tool` on Linux, or `security` on macOS. On Fedora these are provided by
`python3-pyyaml` and `libsecret`; on Debian/Ubuntu, `python3-yaml` and
`libsecret-tools`. Run from your logged-in desktop session so the keyring is
available. The adapter has been exercised with Linux ACLI 1.3.39; unsupported
keyring formats produce an error rather than guessing.

### 2. Create your local configuration

Copy the template if this worktree does not already have a configuration file:

```sh
test -e .hypershell/fullsend.env || \
  cp .hypershell/execute-sdlc/fullsend.env.example .hypershell/fullsend.env
chmod 600 .hypershell/fullsend.env
```

Edit `.hypershell/fullsend.env` and replace the placeholder values:

```sh
FULLSEND_SCOPE=agent/scope/YOUR_GITHUB_USERNAME/fullsend
CONCURRENCY=2
ANTHROPIC_VERTEX_PROJECT_ID=YOUR_APPROVED_GCP_PROJECT
CLOUD_ML_REGION=YOUR_MODEL_REGION
```

This is a gitignored shell configuration file for this worktree. You do not need
a separate inference env file. The launcher exports its assignments for you.

`FULLSEND_SCOPE` identifies the work queue this dispatcher may process. Use a
scope assigned to you or your team, and apply that exact label to the work you
intend to authorize. Work also requires `agent/allowed`. Scope is not inferred
from your username or local directory. Before starting, `make fullsend` verifies
that this exact label exists in the GitHub repository (`REPOSITORY`, default
`openshift-online/hypershell`). Ask a maintainer to register a new scope label
before using it. A missing label or failed GitHub lookup blocks startup; status
and help remain available offline.

The harnesses specify default models. To override them, add
`FULLSEND_MODEL=YOUR_APPROVED_MODEL_ID`; confirm that model is enabled in your
Google Cloud project and region. To skip Jira deliberately, add
`FULLSEND_LIFECYCLE=github-only`. Otherwise, setup below assumes the complete
Jira and GitHub lifecycle.

### 3. Authenticate GitHub

```sh
gh auth login
gh auth status
```

The dispatcher reuses GitHub CLI authentication when `GH_TOKEN` is not supplied.
Your account needs access to the repository and the operations your agents will
perform, including opening pull requests and updating issues.

### 4. Authenticate Vertex inference

Create local Application Default Credentials (ADC) for the Google account that
has access to your configured project:

```sh
gcloud auth application-default login
```

This creates the ADC JSON file used by the harness. `gcloud auth login` alone is
not the same setup. Confirm that the project and region in
`.hypershell/fullsend.env` match the values supplied by your project administrator:

```sh
ANTHROPIC_VERTEX_PROJECT_ID=YOUR_APPROVED_GCP_PROJECT
CLOUD_ML_REGION=YOUR_MODEL_REGION
# Optional when using the normal local ADC location:
# GOOGLE_APPLICATION_CREDENTIALS="$HOME/.config/gcloud/application_default_credentials.json"
```

The dispatcher searches `CLOUDSDK_CONFIG/application_default_credentials.json`,
or `~/.config/gcloud/application_default_credentials.json` when no explicit path
is set. An explicit path must exist and be readable; it is never silently
replaced. `CLOUD_ML_REGION` can fall back to `ANTHROPIC_VERTEX_REGION`.
The harness sets `CLAUDE_CODE_USE_VERTEX=1` and copies the credential file into
each sandbox. Host Claude settings are not a substitute for these dispatcher
variables.

The selected project must have Vertex API access, the requested Claude model
enabled, appropriate account permissions, and quota in the selected region.
A readable ADC file proves local setup, not model access. If Google reports a
missing quota project, configure it for your approved project:

```sh
gcloud auth application-default set-quota-project YOUR_APPROVED_GCP_PROJECT
```

Reference: [Google ADC login](https://docs.cloud.google.com/sdk/gcloud/reference/auth/application-default/login),
[ADC troubleshooting](https://docs.cloud.google.com/docs/authentication/troubleshoot-adc),
and [Claude Code on Vertex](https://code.claude.com/docs/en/google-vertex-ai).

### 5. Authenticate Jira

For the default OAuth path, sign in to Jira using ACLI:

```sh
acli jira auth login --web
```

With neither Jira token nor email configured, the dispatcher automatically uses
the selected ACLI account and site. You do not need to supply an email, token,
or Jira URL. To select OAuth mode explicitly, set:

```sh
JIRA_AUTH_MODE=acli
```

Alternatively, put API-token credentials in the gitignored env file:

```sh
JIRA_AUTH_MODE=api-token
JIRA_USER_EMAIL=you@example.com
JIRA_TOKEN=YOUR_JIRA_API_TOKEN
# Defaults to this tenant in API-token mode:
JIRA_BASE_URL=https://redhat.atlassian.net
```

Auto mode selects API-token authentication only when both email and token are
present. An explicit Jira URL must match the selected ACLI site in ACLI mode.
The host refreshes ACLI normally before each lifecycle attempt and passes only
an access-token snapshot into that sandbox. It does not copy your keyring or
refresh token, or require ACLI inside the sandbox. See the
[Jira reference](../../.hypershell/execute-sdlc/README.md#jira-authentication)
for permissions and token-expiry behavior.

### 6. Check setup and start the dispatcher

Check local setup:

```sh
make fullsend-doctor
```

Fix any items marked `Needs setup` before continuing. The doctor
checks local tools and credential configuration; it does not test Vertex model
access, API permissions, or gateway connectivity.

Confirm that your configured scope identifies the work you intend to authorize,
then start the dashboard:

```sh
make fullsend
```

This starts watch mode. It can create Jira records, update issues, and run
implementation agents for authorized work in your scope. It continues checking
for eligible work until you detach.

Press `s` if the gateway needs attention. Use the arrow
keys or `j/k` and Enter to reconnect through `hsctl`, or choose a cluster and
create a gateway. Complete browser sign-in when prompted.

Set `HYPERSHELL_API_URL` in your env file to select an API. The default is
`https://api.hyp0.infra.hypershell.app`. You can also authenticate ahead of time:

```sh
hsctl login --url https://api.hyp0.infra.hypershell.app
```

Gateway selection is saved in `.hypershell/.funnel-gateway.json`. Reconnect uses
fresh API connection metadata, which avoids stale service-account OIDC clients
that can produce `Invalid parameter: redirect_uri`. A temporary lookup failure
does not delete or automatically replace your gateway.

## Daily commands and overrides

| Command | Behavior |
| --- | --- |
| `make fullsend` | Start/resume the dashboard in watch mode |
| `make fullsend-status` | Show running jobs, failures, and a completed-job count for your scope |
| `make fullsend-doctor` | Check tools, local inference config, Jira profile/keyring access |
| `make fullsend-help` | Show local commands, setup steps, and override examples |
| `make fullsend MODEL=YOUR_APPROVED_MODEL_ID CONCURRENCY=4` | Override settings for this invocation |

The doctor does not contact Vertex, validate API permissions, or verify gateway
connectivity. Status reads saved local state; it does not run a live connection
check. Use the dashboard to inspect logs and completed-job history.

Configuration precedence is: explicit Make overrides, the selected env file,
then exported shell variables. `ENV_FILE=/path/to/file` selects a different file
instead of `.hypershell/fullsend.env`. Missing explicitly selected files fail.
If no local file exists, exported variables work alone. The env file is sourced
as trusted shell code; assignments are automatically exported. Never pass
credential values on the Make command line.

The direct script remains available for advanced options:

```sh
.hypershell/execute-sdlc/funnel-dispatch.sh agent/scope/YOUR_GITHUB_USERNAME/fullsend \
  --env-file .hypershell/fullsend.env --watch
```

To deliberately skip Jira, put `FULLSEND_LIFECYCLE=github-only` in the local env
file. Leave it unset for the complete lifecycle. Model overrides apply to all
agents; without one, each harness uses its own configured model.

## Detach, resume, and inspect failures

`q` or Ctrl-C detaches and prints complete log paths. Existing workers continue.
Run `make fullsend` again to resume monitoring; local durable state prevents
duplicate work. Keep the dispatcher attached when possible: detaching also stops
its periodic gateway authentication checks. State is shared across worktrees of
the same repository; do not change state directories just to bypass a lock.

Use Enter for a run's logs, `d` for details, `H` for history, and `r` to retry a
failed run. Unknown outcomes require confirming that the prior sandbox stopped.
`?` lists all navigation, filtering, sorting, and gateway shortcuts.

## Troubleshooting

| Symptom | Next step |
| --- | --- |
| Vertex requires an existing credential file | Run `gcloud auth application-default login`; fix or unset a stale `GOOGLE_APPLICATION_CREDENTIALS` override |
| Project or region missing | Set `ANTHROPIC_VERTEX_PROJECT_ID` and `CLOUD_ML_REGION` in the selected env file; shell-only assignments must be exported |
| Vertex permission/model/quota error | Check account, project, approved model, region, and quota; a local credential check cannot validate these |
| Jira asks for email/token although ACLI works | Set `JIRA_AUTH_MODE=acli`; check for a leftover email/token pair selecting API-token mode |
| ACLI profile/keyring error | Run `acli jira auth login --web`; check PyYAML, keyring tools, and your desktop session |
| Gateway token expired or redirect URI rejected | Press `s` and reconnect through `hsctl`; inspect diagnostics with `d` |
| Nothing is running | Check `make fullsend-status`, scope/allowed labels, completed runs (`c`), backoff, and gateway status |
| Another dispatcher owns the repository | Resume the existing instance or wait for ownership; do not delete state or locks |

For scheduling semantics, durable state, sandbox toolchains, and contributor
checks, see the [dispatcher reference](../../.hypershell/execute-sdlc/README.md).
