# .hypershell/

This directory configures HyperShell agent automation for this repository.
It is to HyperShell agents what `.github/` is to GitHub Actions: repo-local
automation that decorates the repo with agent capabilities without being part
of the core platform.

Automation is **declarative**. This directory is a Kustomize root. A server-side
import process (see `Repository.import_path` in
`specs/platform/data-model.spec.md`) builds `kustomization.yaml` and reconciles
the rendered HyperShell resources into the API. The former imperative
`discover.sh` / `run.sh` script contract has been removed; an `AgentRuntime` with
a `selector` now expresses discovery declaratively.

## Layout

```
.hypershell/
├── kustomization.yaml          # root import (base dev composition): -> agents
├── base/                       # shared, reusable resources
│   ├── kustomization.yaml
│   ├── repository.yaml         # Repository (this repo) + credential reference
│   ├── gateway.yaml            # Gateway the agents connect to (agents-gateway)
│   ├── sandbox-templates.yaml  # agent-runtime, agent-runtime-slim
│   └── provider-specs.yaml     # claude-inference, github-source-control
├── agents/
│   ├── kustomization.yaml      # full composition: base + every agent's agent.yaml
│   └── <name>/
│       ├── agent.yaml          # AgentRuntime + SecretSources + ProviderBindings + InferenceRoute
│       ├── kustomization.yaml  # base + agent.yaml (for applying one agent standalone)
│       └── SKILL.md            # the agent's workflow prompt (unchanged)
└── overlays/
    └── hypc-ibm-00/            # environment overlay: patches cluster/exposure
        └── kustomization.yaml
```

The base composition targets the local kind cluster. Each `AgentRuntime`
references the shared `Gateway` by name (`gateway_id: agents-gateway`) and shares
its `cluster_id` (base default `local-kind`). Environment overlays under
`overlays/` render the full composition and repoint the environment-specific
values.

`SKILL.md` holds each agent's workflow prompt and is still authored by hand; it
has no declarative equivalent and is loaded by the coordinator at run time.

## Defined Agents

Each `AgentRuntime` carries a `selector` (a list of labels). On each `cron` tick
the coordinator queries the referenced `Repository` for items carrying **any**
selector label and fans out one sandbox per match. `update-openshell` has no
selector: it is purely scheduled.

| Agent | Sandbox | Selector label(s) | Output label(s) |
|-------|---------|-------------------|-----------------|
| `triage` | slim | `agent/allowed` | `agent/workable`, `agent/needs-input`, `agent/duplicate` |
| `spec-implementation` | slim | `agent/workable`, `agent/review-spec-rejected` | `agent/reviewable-spec` |
| `spec-review` | slim | `agent/reviewable-spec` | `agent/review-spec-approved`, `agent/review-spec-rejected` |
| `code-implementation` | full | `agent/review-spec-approved` | `agent/reviewable-code` |
| `code-review` | full | `agent/reviewable-code` | `agent/review-code-approved`, `agent/review-code-rejected` |
| `review-response` | full | `agent/needs-review-response` | re-queues spec/code review |
| `release-verification` | slim | `agent/release-pending` | `agent/release-verified`, `agent/release-verification-failed` |
| `update-openshell` | slim | (scheduled, no selector) | opens an update PR (`agent/reviewable-code`) |

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

## Reference resolution

Foreign keys in these manifests reference other declared resources **by name**
(e.g. `repository_id: hypershell`, `sandbox_template_id: agent-runtime`,
`provider_spec_id: claude-inference`, `workspace_id: <runtime-name>`). The import
process resolves each name to the concrete resource ID at reconcile time.
`workspace_id` references the runtime's own name because the control plane
creates exactly one `AgentWorkspace` per `AgentRuntime`; the importer maps the
runtime to its workspace.

## Placeholders

`cluster_id` and `gateway_id` carry base (kind) defaults and are repointed per
environment by an overlay (see `overlays/hypc-ibm-00`). Remaining credential and
project values are environment-specific and written as `REPLACE_WITH_*`; supply
them via an overlay patch before the manifests are usable in that environment:

| Placeholder | Where | Supply via |
|-------------|-------|------------|
| `REPLACE_WITH_GCP_PROJECT_ID` | `base/provider-specs.yaml` (inference) | overlay patch |
| `REPLACE_WITH_SA_EMAIL` | inference `ProviderBinding.refresh_strategy` | overlay patch |
| `REPLACE_WITH_GITHUB_APP_ID` | github `ProviderBinding.refresh_strategy` | overlay patch |

The GitHub App credential `SecretSource` named `github-app-credentials`
(referenced by `base/repository.yaml`) is a platform-provisioned secret and is
not declared here.

## Overlays

Each environment gets an overlay under `overlays/` that renders the full agent
composition (`../../agents`) and patches the environment-specific values. The
`hypc-ibm-00` overlay repoints `cluster_id` to the registered ROKS cluster and
switches the gateway to route-mode (ClusterIP) exposure. Add a sibling directory
per additional environment.

## Applying

```sh
# Render + reconcile the base (kind) composition (what the import process does):
hsctl apply -k .hypershell

# Render + reconcile for a specific environment:
hsctl apply -k .hypershell/overlays/hypc-ibm-00

# Apply a single agent (base + that agent):
hsctl apply -k .hypershell/agents/triage

# Preview without writing:
hsctl apply -k .hypershell/overlays/hypc-ibm-00 --dry-run
```

## Agent runtime images

The image definitions live in [`components/agents`](../components/agents/README.md).
`agent-runtime` carries compilers for code implementation/review;
`agent-runtime-slim` serves triage/spec/review-response/scheduled automation.
Both provide native Claude Code, Atlassian CLI, GitHub CLI, hsctl, Python and
shell utilities. The harness resolves their image digests from the selected
release bundle, clones the trusted repository into `/sandbox/hypershell`, and
injects per-run credentials. Agent scripts and credentials are not embedded in
these images.
