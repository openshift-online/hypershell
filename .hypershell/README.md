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
├── kustomization.yaml          # root import: base + every agents/*/agent.yaml
├── base/                       # shared, reusable resources
│   ├── kustomization.yaml
│   ├── repository.yaml         # Repository (this repo) + credential reference
│   ├── sandbox-templates.yaml  # agent-runtime, agent-runtime-slim
│   └── provider-specs.yaml     # claude-inference, github-source-control
└── agents/<name>/
    ├── agent.yaml              # AgentRuntime + SecretSources + ProviderBindings + InferenceRoute
    ├── kustomization.yaml      # base + agent.yaml (for applying one agent standalone)
    └── SKILL.md                # the agent's workflow prompt (unchanged)
```

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

Values that are environment-specific are written as `REPLACE_WITH_*` and must be
supplied before the manifests are usable in a given environment:

| Placeholder | Where | Supply via |
|-------------|-------|------------|
| `REPLACE_WITH_CLUSTER_ID` | every `AgentRuntime.cluster_id` | overlay patch or importer binding |
| `REPLACE_WITH_GATEWAY_ID` | every `AgentRuntime.gateway_id` | overlay patch or importer binding |
| `REPLACE_WITH_GCP_PROJECT_ID` | `base/provider-specs.yaml` (inference) | overlay patch |
| `REPLACE_WITH_SA_EMAIL` | inference `ProviderBinding.refresh_strategy` | overlay patch |
| `REPLACE_WITH_GITHUB_APP_ID` | github `ProviderBinding.refresh_strategy` | overlay patch |

The GitHub App credential `SecretSource` named `github-app-credentials`
(referenced by `base/repository.yaml`) is a platform-provisioned secret and is
not declared here.

## Applying

```sh
# Render + reconcile every agent (what the import process does):
hsctl apply -k .hypershell

# Apply a single agent (base + that agent):
hsctl apply -k .hypershell/agents/triage

# Preview without writing:
hsctl apply -k .hypershell --dry-run
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
