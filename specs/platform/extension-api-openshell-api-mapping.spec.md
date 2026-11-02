# HyperShell Extension API to OpenShell API Mapping Specification

**Date:** 2026-10-18
**Status:** Active
**Jira:** HYPERSHELL-412
**Related:** [`agent-workspace-gateway-api.spec.md`](./agent-workspace-gateway-api.spec.md) (how the control plane realizes an AgentRuntime through OpenShell's existing API), [`data-model.spec.md`](./data-model.spec.md) (the HyperShell Extension API kinds this maps), [`control-plane.spec.md`](./control-plane.spec.md) (the reconciler and the `ProviderRealizer` seam), [`agent-secret-late-binding.spec.md`](./agent-secret-late-binding.spec.md) (credentials realized as OpenShell providers; rotation ownership split by strategy - OpenShell native vs control-plane GitHub rotator), [`openshell-inference-routing.spec.md`](./openshell-inference-routing.spec.md) (the `inference.local` router the inference route configures)

## Purpose

HyperShell Extension API kinds (`AgentRuntime`, `AgentWorkspace`, `WorkspaceMembership`,
`ProviderSpec`, `ProviderBinding`, `InferenceRoute`, `SecretSource`) are a
**convenience and extension layer over OpenShell**. They are HyperShell's declarative,
PostgreSQL-backed desired state. OpenShell already provides the native resources an agent
workspace needs - workspaces, members, providers, provider credential refresh, and the
inference route - and exposes an existing API for them (the interface the `openshell` CLI
drives). HyperShell does NOT define a new contract OpenShell must implement; the control
plane **consumes OpenShell's existing API** and realizes each HyperShell Extension kind as
its OpenShell native counterpart.

This specification is the **1:1 map** between the two APIs. For every HyperShell Extension
kind it names the exact OpenShell resource it provisions, maps each field and each lifecycle
operation (create, update, delete) to the OpenShell operation that realizes it, and audits
coverage in both directions. It is the proof that HyperShell expresses a **fully declarative
model over OpenShell kinds**: applying the HyperShell Extension kinds declaratively provisions
the complete OpenShell resource graph an agent needs, with no imperative step outside the
model - the `openshell` CLI calls the `sdlc-coordinator` performs today become the control
plane's declarative realization.

### How this spec is grounded

OpenShell is an upstream dependency; only the pinned OpenShell ref
([`scripts/rh-trex-ai.ref`](../../scripts/rh-trex-ai.ref) and
[`scripts/install-openshell.sh`](../../scripts/install-openshell.sh)) and the installed
`openshell` CLI are reachable from this repository, not OpenShell's server source. This
mapping is therefore grounded in two reachable sources of truth:

1. The **installed `openshell` CLI** (version `0.1.2-rhaiv.0`) and its command and flag
   surface (`openshell workspace`, `workspace member`, `provider`, `provider refresh`,
   `sandbox`, `gateway`), which the CLI drives against OpenShell's existing gateway API.
2. The **production provisioning sequence** the `sdlc-coordinator` runs today
   (`bases/agents/sdlc-coordinator/run.sh`, function `provision_workspace_providers`),
   which is the authoritative example of the exact OpenShell calls HyperShell depends on.

Where the OpenShell **wire API** (REST or gRPC endpoint, exact request field names) behind a
CLI command cannot be confirmed from this repository, the mapping records the CLI command and
behavior and marks the wire detail **CONFIRM-AGAINST-UPSTREAM**. No OpenShell endpoint is
fabricated. Rows whose OpenShell capability itself is not confirmed from the installed CLI are
marked explicitly so a reviewer can resolve them against OpenShell upstream.

## Requirements

### Requirement: AGT-EXT-01 -- Extension Kind to OpenShell Resource Mapping

Each HyperShell Extension kind SHALL be realized as the OpenShell native resource named in
the mapping table below, and each of its lifecycle operations SHALL be realized through the
OpenShell operation named for it. The control plane SHALL NOT invent an OpenShell-side
contract; it SHALL use OpenShell's existing API (the interface the `openshell` CLI drives),
as specified in [`agent-workspace-gateway-api.spec.md`](./agent-workspace-gateway-api.spec.md).

| HyperShell Extension kind | OpenShell native resource | Create | Update | Delete |
|---|---|---|---|---|
| `AgentWorkspace` | OpenShell **workspace** | `workspace create --name <name>` | no mutable fields (name is identity); re-apply is a no-op | `workspace delete <name>` (cascade; see `DeleteWorkspace`) |
| `WorkspaceMembership` | OpenShell workspace **member** | `workspace member add --workspace <ws> --subject <oidc-subject> --role <admin\|member>` | member role change: `workspace member add` with the new role (upsert); `member remove` then `add` where the API requires it (CONFIRM-AGAINST-UPSTREAM: in-place role update vs remove+add) | `workspace member remove` for members pruned from desired state |
| `ProviderSpec` | OpenShell provider **profile** | `provider profile import --file <profile>` (from `ProviderSpec.profile`); shared, not per-workspace | re-import the profile document | not deleted on `AgentRuntime` delete (shared across workspaces) |
| `ProviderBinding` | OpenShell **provider** (+ its refresh config) | `provider create --workspace <ws> --name <ProviderBinding.name> --type <ProviderSpec type> --credential <KEY[=material]> [--config K=V]...` then `provider refresh configure <name> --credential-key <KEY> --strategy <s> --material K=V --secret-material-env/-key ...` when a refresh strategy applies | `provider update <name> --credential <KEY> [--config K=V] [--credential-expires-at ...]`; `provider refresh configure` to change refresh metadata | `provider delete <name>...` |
| `InferenceRoute` | OpenShell workspace **inference route** (`inference.local`) | `inference set --workspace <ws> --provider <ProviderBinding.name> --model <alias>` | `inference set` with the new provider or model alias (converges the single route) | cleared when the workspace is deleted (CONFIRM-AGAINST-UPSTREAM: explicit `inference unset`) |
| `SecretSource` | none (material supply) | consumed as provider material: `--credential`, refresh `--material` / `--secret-material-*` | n/a (material refreshed in the backing store) | n/a (durable material reclaimed by the gateway credential plane) |
| `AgentRuntime` | none (orchestration root) | realizes the whole graph above + cluster-side scheduling resources | re-reconciles the graph | `DeleteWorkspace` cascade + cluster resource teardown |

The control plane SHALL send OpenShell only gateway-agnostic values (the provider type from
the `ProviderSpec`, the `refresh_strategy` type and its non-secret parameters, the model
alias) plus durable **material** for secret-valued fields; it SHALL NOT send
HyperShell-internal identifiers or backend-specific field names, which are OpenShell's own
translation concern.

#### Scenario: AGT-EXT-01.01 -- AgentWorkspace realized as an OpenShell workspace

- GIVEN an `AgentRuntime` whose `AgentWorkspace.name` is `amber-reviews`
- WHEN the control plane reconciles it
- THEN it SHALL realize an OpenShell workspace named `amber-reviews` through OpenShell's existing workspace-create operation
- AND re-reconciling with the workspace already present SHALL make no change

#### Scenario: AGT-EXT-01.02 -- ProviderBinding realized as an OpenShell provider

- GIVEN a `ProviderBinding` named `claude` referencing an `inference` `ProviderSpec` of type `google-vertex-ai`, with a `google-service-account-jwt` `refresh_strategy` whose material is present cluster-local
- WHEN the control plane reconciles the owning `AgentRuntime`
- THEN it SHALL realize an OpenShell provider named `claude` of type `google-vertex-ai` with the service-account material and config (project, region)
- AND it SHALL configure the `google-service-account-jwt` refresh strategy so OpenShell mints and rotates the usable credential

---

### Requirement: AGT-EXT-02 -- Field-by-Field Mapping

Each HyperShell Extension field that crosses to OpenShell SHALL map to the OpenShell field or
flag named below. Fields marked "HyperShell-only" are desired-state or reconcile bookkeeping
that never cross to OpenShell.

**AgentWorkspace**

| HyperShell field | OpenShell target | Notes |
|---|---|---|
| `name` | workspace name (`workspace create --name`) | DNS-1123 label, max 19 chars (OpenShell constraint) |
| `gateway_id` | selects the gateway endpoint/session | resolves the OpenShell API endpoint, not a workspace field |
| `agent_runtime_id`, `status`, timestamps | HyperShell-only | ownership and reconcile status |

**WorkspaceMembership**

| HyperShell field | OpenShell target | Notes |
|---|---|---|
| `subject` | `workspace member add --subject` | OIDC subject UUID |
| `role` (`admin`\|`member`) | `workspace member add --role` | CLI roles are `user` and `admin`; HyperShell `member` maps to OpenShell `user` (CONFIRM-AGAINST-UPSTREAM: role-name equivalence) |
| `workspace_id` | the target workspace | resolves `--workspace` |

**ProviderSpec**

| HyperShell field | OpenShell target | Notes |
|---|---|---|
| `profile` | `provider profile import --file` | opaque profile document passed through to OpenShell |
| `capability.refresh_strategy` | the `provider refresh --strategy` value a binding uses | capability, not a direct call |
| `capability.config` | `provider create --config K=V` | non-secret provider config |
| `category` | constrains which OpenShell operations apply | `inference` -> `inference set` eligible |
| `name`, `status`, timestamps | HyperShell-only | |

**ProviderBinding**

| HyperShell field | OpenShell target | Notes |
|---|---|---|
| `name` | `provider create/update/delete <name>` | provider name, unique in the workspace |
| `provider_spec_id` -> `ProviderSpec` type | `provider create --type` | gateway-agnostic provider type |
| `secret_source_id` -> material | `provider create --credential KEY=<material>` | durable material read cluster-local |
| `refresh_strategy.type` | `provider refresh configure --strategy` | see Refresh Strategy Mapping below |
| `refresh_strategy.material` | `provider refresh configure --material K=V` | non-secret refresh parameters |
| `refresh_strategy.secret_material[]` | `provider refresh configure --secret-material-env/--secret-material-key` | secret refresh material by reference |

**InferenceRoute**

| HyperShell field | OpenShell target | Notes |
|---|---|---|
| `provider_binding_id` -> binding name | `inference set --provider <name>` | the active provider |
| `model` | `inference set --model <alias>` | model alias resolved by OpenShell at the route |
| `workspace_id` | `inference set --workspace` | |

#### Scenario: AGT-EXT-02.01 -- No backend-specific field names cross the boundary

- GIVEN a `ProviderBinding` with a `refresh_strategy`
- WHEN the control plane realizes it through OpenShell's existing API
- THEN it SHALL send the gateway-agnostic strategy type and its non-secret parameters plus durable material only
- AND it SHALL NOT send a HyperShell row id or an OpenShell backend driver field name

---

### Requirement: AGT-EXT-03 -- Refresh Strategy Mapping and Upstream Gaps

The HyperShell credential model names refresh strategies `app-installation-token`,
`google-service-account-jwt`, and `static-token`
([`agent-secret-late-binding.spec.md`](./agent-secret-late-binding.spec.md)). The installed
`openshell` CLI `provider refresh configure --strategy` enumerates
`oauth2-refresh-token`, `oauth2-client-credentials`, `google-service-account-jwt`, and
`aws-sts-assume-role`. The mapping SHALL record which strategies OpenShell refreshes and
which the control plane rotates, grounded in the installed CLI. The decided model (see
[`agent-secret-late-binding.spec.md`](./agent-secret-late-binding.spec.md), "Rotation
Ownership by Refresh Strategy") is: OpenShell owns rotation for the strategies it supports
natively; the control plane owns rotation for GitHub App credentials, because the installed
CLI has no `app-installation-token` refresh strategy.

| HyperShell `refresh_strategy.type` | OpenShell realization | Rotation owner / grounding |
|---|---|---|
| `google-service-account-jwt` | `provider refresh configure --strategy google-service-account-jwt --material client_email=... --secret-material-env private_key=...` then `provider refresh rotate` | **OpenShell** (confirmed): enumerated by the installed CLI and used verbatim by `provision_workspace_providers` for the Claude/Vertex provider |
| `static-token` | `provider create --type <t> --credential KEY=<token>`; no `provider refresh configure` | **No rotation** (confirmed pattern): a static credential provider has no refresh metadata. `static-token` is not a CLI `--strategy` value because it means "no refresh" |
| `app-installation-token` (GitHub App) | `provider create --type github --credential GH_TOKEN` / `provider update --credential GH_TOKEN`, where the control plane mints `GH_TOKEN` from the stored PEM and re-mints before expiry | **Control plane** (decided, HYPERSHELL-412): `app-installation-token` is NOT a CLI `--strategy`, so a bounded control-plane GitHub rotator mints the ~1h installation token from the PEM and sets it on the provider through `provider update --credential`, matching the production `provision_workspace_providers` GitHub path. OpenShell does NOT own GitHub rotation |
| `oauth2-refresh-token`, `oauth2-client-credentials`, `aws-sts-assume-role` | native OpenShell strategies | **OpenShell** (confirmed available) in the CLI; not currently used by any HyperShell `ProviderBinding` (reserved for future providers) |

#### Scenario: AGT-EXT-03.01 -- Vertex provider uses the OpenShell-owned refresh strategy

- GIVEN a Claude `ProviderBinding` with `refresh_strategy.type = google-service-account-jwt`
- WHEN the control plane realizes it
- THEN it SHALL configure the provider refresh with the `google-service-account-jwt` strategy, the `client_email` material, and the private key as secret material, matching the production coordinator sequence
- AND OpenShell SHALL own minting and rotation of the usable Vertex access token

#### Scenario: AGT-EXT-03.02 -- GitHub provider is rotated by the control plane

- GIVEN a GitHub `ProviderBinding` with `refresh_strategy.type = app-installation-token` whose `SecretSource` holds the GitHub App PEM
- WHEN the control plane realizes and maintains it
- THEN the bounded control-plane GitHub rotator SHALL mint the installation token from the PEM and set it through `provider create/update --credential`, re-minting before expiry
- AND OpenShell SHALL NOT own GitHub rotation and no raw Kubernetes `Secret` SHALL be placed

---

### Requirement: AGT-EXT-04 -- Two-Direction Gap Analysis

The mapping SHALL be auditable in both directions. Every OpenShell resource reachable from the
installed CLI SHALL be classified against a HyperShell Extension kind, and every HyperShell
Extension kind SHALL be classified against an OpenShell resource, each labeled **covered**,
**intentionally-not-mapped**, or **follow-up**.

**Direction 1 - OpenShell resources without a HyperShell Extension kind**

| OpenShell resource (CLI) | HyperShell kind | Classification |
|---|---|---|
| `workspace` | `AgentWorkspace` | covered |
| `workspace member` | `WorkspaceMembership` | covered |
| `provider` + `provider refresh` | `ProviderBinding` (+ `ProviderSpec`) | covered |
| `inference` route | `InferenceRoute` | covered |
| `provider profile` | `ProviderSpec.profile` | covered |
| `sandbox` (per-run instance) | none | intentionally-not-mapped: an ephemeral per-work-item instance created by the coordinator at runtime from the `SandboxTemplate`, not a desired-state resource |
| `sandbox template` | `SandboxTemplate` (partial) | follow-up: `SandboxTemplate` holds image, name prefix, and policy but is consumed by the coordinator via `sandbox create --from`; a declarative mapping to OpenShell `sandbox template` is not yet established (CONFIRM-AGAINST-UPSTREAM) |
| `policy` / `sandbox policy` | `SandboxTemplate.policy` | follow-up: enforced through the sandbox template, not a standalone declarative kind |
| `settings` (e.g. `providers_v2_enabled`) | none | follow-up: set imperatively by the coordinator today; see gap G-5 |
| `service`, `forward`, `logs`, `ssh-proxy`, `doctor`, `term` | none | intentionally-not-mapped: operational and interactive CLI surfaces, not provisioned desired state |

**Direction 2 - HyperShell Extension kinds without an OpenShell counterpart**

| HyperShell kind | OpenShell counterpart | Classification |
|---|---|---|
| `AgentRuntime` | none | intentionally-not-mapped: the orchestration root. It realizes the OpenShell workspace graph AND the cluster-side scheduling resources (`Namespace`, `ServiceAccount`, `NetworkPolicy`, coordinator `CronJob`); it is not a single OpenShell resource |
| `SecretSource` | none | intentionally-not-mapped: an abstract reference to durable material. It never becomes an OpenShell resource; its material is consumed into `provider create --credential` and `provider refresh configure` material |
| `AgentWorkspace`, `WorkspaceMembership`, `ProviderSpec`, `ProviderBinding`, `InferenceRoute` | as mapped above | covered |

#### Scenario: AGT-EXT-04.01 -- Every covered kind has a named OpenShell operation

- GIVEN the mapping table
- WHEN a reviewer audits each HyperShell Extension kind labeled covered
- THEN each SHALL name the OpenShell resource and the create, update, and delete operation that realizes it
- AND each gap SHALL be labeled covered, intentionally-not-mapped, or follow-up with a reason

---

### Requirement: AGT-EXT-05 -- Fully Declarative Model Over OpenShell Kinds

Applying the HyperShell Extension kinds declaratively SHALL provision the complete OpenShell
resource graph an agent needs to run, with no imperative step outside the declarative model.
The imperative `openshell` CLI calls the `sdlc-coordinator` `provision_workspace_providers`
performs today SHALL be fully reproduced by the control plane's declarative realization of the
Extension kinds, so the coordinator no longer self-provisions its workspace and providers.

The sequence the coordinator runs today maps to the declarative model as follows:

| Coordinator imperative call (`provision_workspace_providers`) | Declarative realization |
|---|---|
| `workspace create --name` | realize `AgentWorkspace` |
| `workspace member add --subject --role admin` | realize `WorkspaceMembership` |
| `provider create --type google-vertex-ai --credential ... --config ...` | realize Claude `ProviderBinding` |
| `provider refresh configure --strategy google-service-account-jwt ...` + `rotate` | realize that binding's `refresh_strategy` |
| `inference set --provider --model` | realize `InferenceRoute` |
| `provider profile import` | realize `ProviderSpec.profile` |
| `provider create/update --type github --credential GH_TOKEN` | realize GitHub `ProviderBinding`; the control-plane GitHub rotator mints `GH_TOKEN` from the stored PEM (G-4, resolved) |

The declarative `ProviderBinding` expresses the GitHub credential **intent** (the stored PEM
plus the `app-installation-token` strategy); the control-plane GitHub rotator realizes it by
minting the token and setting it through `provider update --credential`. The GitHub credential
value is therefore control-plane-supplied and control-plane-rotated, while the declarative model
still fully expresses the binding. The remaining imperative step SHALL be enumerated so the
"fully declarative" assertion is honest and auditable:

- **G-4 (GitHub App token minting) - RESOLVED (HYPERSHELL-412): control-plane-owned GitHub
  rotator.** The coordinator mints the GitHub App installation token and the Vertex access token
  today (`mint_github_app_token`, `mint_google_token`). For Vertex this is subsumed by the
  `google-service-account-jwt` refresh strategy (OpenShell owns rotation). For GitHub, because
  the installed CLI has no `app-installation-token` refresh strategy, the decided model moves
  the minting out of the coordinator and into a bounded control-plane GitHub rotator that mints
  the token from the stored PEM and sets it through `provider update --credential`. This is part
  of the declarative realization, not a residual coordinator step: the declarative
  `ProviderBinding` is the intent and the control-plane rotator is the realizer.
- **G-5 (global settings).** `settings set --global --key providers_v2_enabled` is a one-time
  gateway enablement toggle the coordinator sets imperatively. It remains a follow-up to fold
  into declarative gateway configuration rather than a per-agent reconcile step.

With G-4 resolved (control-plane-owned GitHub rotator) and G-5 tracked, the agent workspace
graph - workspace, memberships, providers with their refresh strategies, and the inference
route - SHALL be fully realizable from the declarative HyperShell Extension kinds; the GitHub
credential value is control-plane-supplied and rotated, which is the decided model.

#### Scenario: AGT-EXT-05.01 -- Coordinator self-provisioning is replaced by declarative realization

- GIVEN an `AgentRuntime` whose `AgentWorkspace`, memberships, provider bindings, and inference route are declared in HyperShell
- WHEN the control plane reconciles it against OpenShell's existing API
- THEN the complete OpenShell workspace graph SHALL be provisioned without the coordinator running `provision_workspace_providers`
- AND the GitHub credential value SHALL be minted and set by the control-plane GitHub rotator, and the only residual imperative step SHALL be the enumerated gap G-5

#### Scenario: AGT-EXT-05.02 -- Declarative re-apply is idempotent against the OpenShell graph

- GIVEN a fully realized OpenShell workspace graph for an `AgentRuntime`
- WHEN the control plane re-reconciles the unchanged Extension kinds
- THEN every OpenShell operation SHALL be a no-op success
- AND the OpenShell resource graph SHALL remain unchanged

## Design Decisions

| Decision | Rationale |
|----------|-----------|
| Map Extension kinds to OpenShell's existing API, not a new OpenShell contract | OpenShell already provides workspaces, members, providers, refresh, and the inference route and exposes them to the `openshell` CLI. HyperShell is a convenience layer; it consumes that API rather than asking OpenShell to implement a bespoke HyperShell contract |
| Ground the mapping in the installed CLI and the production coordinator sequence | These are the two reachable sources of OpenShell's real surface from this repository; grounding in them keeps the mapping real and lets unconfirmed wire details be flagged honestly |
| Mark unconfirmed OpenShell capability CONFIRM-AGAINST-UPSTREAM rather than invent it | Role-name equivalence and the wire protocol behind each CLI command cannot be confirmed from here; fabricating them would make the declarative assertion false. The GitHub `app-installation-token` gap is resolved to a control-plane rotator (a decision, not a fabricated OpenShell capability) rather than left open |
| Resolve G-4 to a control-plane GitHub rotator; enumerate the residual G-5 | The GitHub minting that the installed OpenShell CLI cannot do is moved into a bounded control-plane rotator inside the declarative realization, not left as an open gap. The "fully declarative" claim stays credible because the one remaining imperative step (G-5) is named as a tracked follow-up |
| `ProviderSpec` -> OpenShell provider profile; `ProviderBinding` -> OpenShell provider | Matches the reuse model: a shared profile/capability definition referenced by many per-workspace provider instances, mirroring `provider profile import` + per-workspace `provider create` |

## References

- [`agent-workspace-gateway-api.spec.md`](./agent-workspace-gateway-api.spec.md) - how the control plane realizes the Extension kinds through OpenShell's existing API (the `ProviderRealizer` seam, admin session, idempotency, retry, cascade delete)
- [`data-model.spec.md`](./data-model.spec.md) - the HyperShell Extension API kinds and their fields
- [`control-plane.spec.md`](./control-plane.spec.md) - the `AgentRuntimeReconciler` and the realization sequence
- [`agent-secret-late-binding.spec.md`](./agent-secret-late-binding.spec.md) - the refresh-strategy credential model; rotation ownership split by strategy (OpenShell native vs control-plane GitHub rotator)
- [`openshell-inference-routing.spec.md`](./openshell-inference-routing.spec.md) - the `inference.local` router the `InferenceRoute` configures
- `scripts/install-openshell.sh`, `scripts/rh-trex-ai.ref` - the pinned OpenShell CLI and ref this mapping is grounded against
