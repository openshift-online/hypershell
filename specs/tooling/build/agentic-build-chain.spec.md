# Agentic Build Chain

**Status:** Draft
**Applies to:** a new Python/LangChain component (`components/build-pipeline`), the `skills/build/full-stack-pipeline` workflow it executes, and the existing build, code-generation, and test tooling it orchestrates
**Jira:** HYPERSHELL-301

## Purpose

The `full-stack-pipeline` skill defines HyperShell's spec-driven change workflow as a fixed, largely **deterministic** sequence: read a spec, run a gap analysis, break the work into dependency-ordered waves (Spec → API → SDK → {BE, CLI, gRPC} → CP → Integration), and gate each wave on concrete acceptance commands (`make test`, `make lint`, `go build`, the SDK generator, the e2e suite). Most steps are mechanical — an edit that follows an existing pattern, or a tool invocation whose success is decided by an exit code — while a few steps (gap analysis, wave planning, reconciler logic, failure diagnosis) require genuine reasoning over broad context.

This specification defines an **agentic build chain**: an executable encoding of that skill as a graph of typed steps, where each step is bound to a language model **by configuration rather than by code**. Mechanical steps run on small, fast, inexpensive models (or no model at all, for pure command invocations); reasoning steps run on larger, more capable models with more context. Because model selection is configuration, the same chain runs under many configurations — all-small, all-large, or a tiered mix — and every run emits per-step and per-run metrics (model, tokens, cost, latency, tool calls, retries, gate outcome) so configurations can be compared on cost and speed while deterministic gates hold output correctness constant.

The chain **augments** the skill, it does not replace its definition: the skill remains the human-readable source of truth for *what* the workflow is, and the chain is its automated executor. The chain SHALL orchestrate the project's existing build and generator tooling; it SHALL NOT reimplement build, code-generation, or test logic. The Python implementation lives under `components/`, beside the Go and TypeScript components it drives.

This specification covers the orchestration component only. The workflow steps themselves are defined by `skills/build/full-stack-pipeline/SKILL.md`; the acceptance commands are defined by that skill and by `platform/e2e-testing.spec.md`.

## Requirements

### Requirement: AB-01 -- Workflow Modeled as a Typed Step Graph

The chain SHALL represent the `full-stack-pipeline` workflow as a directed acyclic graph of **steps** (nodes) whose edges encode the pipeline's fixed dependency order. Each step SHALL declare its typed **inputs** and **outputs** (its artifacts), so a step runs only once every input it depends on is produced.

The graph SHALL preserve the pipeline's wave gating: a downstream step SHALL NOT start until every upstream step it depends on has completed and passed its gate (`AB-04`). The stage order SHALL be Spec → API → SDK → {BE, gRPC, CLI} → CP → Integration, matching the skill; steps within a wave that share no dependency (for example BE, gRPC, and CLI after the SDK wave) MAY run concurrently.

The graph definition SHALL be data, separate from the model bindings of `AB-03`, so the same graph is reused across every configuration.

**Verification:** Load the graph and confirm its edges match the skill's dependency order; confirm a downstream step cannot be scheduled while an upstream step is incomplete or failing its gate; confirm independent same-wave steps are schedulable concurrently.

#### Scenario: Downstream step blocked on upstream gate

- GIVEN the API step has not yet passed its acceptance gate
- WHEN the chain schedules ready steps
- THEN the SDK step SHALL NOT start
- AND it SHALL become schedulable only after the API step completes and its gate passes

#### Scenario: Independent same-wave steps run concurrently

- GIVEN the SDK step has completed and passed its gate
- WHEN the chain schedules the next wave
- THEN the BE, gRPC, and CLI steps SHALL be eligible to run concurrently
- AND the CP step SHALL remain blocked until the BE and gRPC steps complete

### Requirement: AB-02 -- Step Classification and Default Model Tiers

Every step SHALL be classified by the kind of work it performs, and each class SHALL map to a default model **tier**:

| Step class | Description | Default tier |
|---|---|---|
| `tool-only` | A deterministic command invocation with no model (for example running the SDK generator, `make test`, `make proto`) | none |
| `mechanical` | A pattern-following edit or scaffold whose shape is dictated by existing code (for example adding a DAO, a CLI command, a proto field) | `small` |
| `reasoning` | Analysis, synthesis, or judgment over broad context (for example gap analysis, wave planning, reconciler logic, failure diagnosis) | `deep` |

The tiers SHALL be `none`, `small`, `standard`, and `deep`, ordered by capability and cost. The classification of each step SHALL be part of the graph definition (`AB-01`). A `tool-only` step SHALL invoke its command without any model call.

**Verification:** Enumerate the steps and confirm each carries a class and a resolved default tier; confirm a `tool-only` step performs no model call; confirm the pipeline's mechanical edits default to `small` and its analysis/planning steps default to `deep`.

#### Scenario: Mechanical step defaults to a small model

- GIVEN no per-step override is configured
- WHEN the chain runs the SDK-generation and CLI-scaffold steps
- THEN the SDK-generation step (`tool-only`) SHALL make no model call
- AND the CLI-scaffold step (`mechanical`) SHALL use the `small` tier

#### Scenario: Reasoning step defaults to a deep model

- GIVEN no per-step override is configured
- WHEN the chain runs the gap-analysis step
- THEN the step SHALL use the `deep` tier

### Requirement: AB-03 -- Per-Step Model Binding via Run Configuration

The model used by each step SHALL be resolved from a named **run configuration** (a *profile*), never hardcoded in step logic. A profile SHALL bind each tier (`AB-02`) to a concrete model, MAY override the model for any individual step by step identifier, and SHALL carry the provider and endpoint settings needed to construct that model.

Model construction SHALL be provider-agnostic through LangChain's chat-model interface, so a profile can target any supported provider (hosted or local) without step-logic changes. Resolution order for a step's model SHALL be: per-step override, then the step class's tier binding, then the profile's tier default.

The project SHALL ship at least these profiles: `tiered` (the default: `mechanical`→`small`, `reasoning`→`deep`), `all-small`, and `all-deep`. Selecting a profile SHALL be the only change required to run the whole chain under a different configuration.

**Verification:** Run the chain under `tiered`, `all-small`, and `all-deep`; confirm each step's resolved model matches the profile and any override; confirm switching providers requires only a profile change, not code changes.

#### Scenario: Per-step override wins over tier default

- GIVEN a profile that binds `reasoning`→`deep` and overrides the gap-analysis step to `standard`
- WHEN the chain resolves the gap-analysis step's model
- THEN it SHALL use the `standard` model, not the `deep` tier default

#### Scenario: One profile change reconfigures the whole run

- GIVEN the chain ran under the `tiered` profile
- WHEN it is rerun under the `all-small` profile with no other change
- THEN every non-`tool-only` step SHALL use the `all-small` model bindings

### Requirement: AB-04 -- Deterministic, Model-Independent Acceptance Gates

Each step (or wave) SHALL have an **acceptance gate**: a deterministic command whose exit status decides pass or fail. Gate commands SHALL be the acceptance commands defined by the `full-stack-pipeline` skill and `platform/e2e-testing.spec.md` (for example `make test`, `make lint`, `go build ./...`, `go vet ./...`, SDK-generator idempotence, the e2e suite).

A step SHALL NOT be marked complete until its gate passes. Gates SHALL be independent of which model produced the step's output, so the correctness bar is identical across every profile and cross-configuration comparisons (`AB-08`) measure cost and latency at held-constant quality. A gate failure SHALL prevent downstream steps from starting (`AB-01`) and SHALL trigger the failure handling of `AB-10`.

**Verification:** Force a step to emit output that fails its gate and confirm the step is not marked complete and downstream steps do not start; confirm the same gate command and pass criteria are applied regardless of the profile in use.

#### Scenario: Gate failure blocks completion

- GIVEN the BE step produced code that does not compile
- WHEN its gate `go build ./...` runs
- THEN the gate SHALL fail
- AND the BE step SHALL NOT be marked complete
- AND the CP step SHALL NOT start

#### Scenario: Same gate across profiles

- GIVEN the API step ran under `all-small` in one run and `all-deep` in another
- WHEN each run reaches the API gate
- THEN both runs SHALL apply the identical gate command and pass criteria

### Requirement: AB-05 -- Orchestrates Existing Tooling; Skill Is the Source of Truth

The chain SHALL invoke the project's existing build, code-generation, and test commands as subprocess tool calls (for example `make` targets, the SDK generator, the plugin generator, `buf generate`, `git`, the e2e script). It SHALL NOT reimplement build, code-generation, or test logic in Python.

The `full-stack-pipeline` skill SHALL remain the single source of truth for the workflow's steps and their acceptance commands. The chain's graph (`AB-01`) SHALL correspond to that skill; when the skill changes, the graph SHALL be updated to match. The project SHALL provide a check that flags divergence between the skill's defined steps/commands and the chain's graph so the two cannot silently drift.

**Verification:** Confirm each step shells out to an existing command rather than reimplementing it; run the divergence check against a skill edited to add a step and confirm it reports the drift.

#### Scenario: Step invokes an existing command

- GIVEN the SDK-generation step runs
- WHEN it executes
- THEN it SHALL invoke the existing SDK generator command
- AND it SHALL NOT contain a reimplementation of SDK generation

#### Scenario: Drift between skill and graph is detected

- GIVEN the skill defines a step or acceptance command absent from the chain's graph
- WHEN the divergence check runs
- THEN it SHALL report the missing step or command

### Requirement: AB-06 -- Composable Execution Modes

The chain SHALL be runnable end-to-end and as a subgraph. It SHALL support running from a named stage, to a named stage, or a single stage in isolation, provided the required upstream artifacts (`AB-07`) are present or supplied. Dependency edges (`AB-01`) SHALL be enforced in every mode: a subgraph run SHALL refuse to start a step whose inputs are neither produced in the run nor supplied.

**Verification:** Run the full chain; run `--from SDK`; run only the CLI stage with supplied SDK artifacts; confirm each honors dependency edges and refuses to start a step with missing inputs.

#### Scenario: Run a single stage with supplied inputs

- GIVEN the SDK artifacts from a prior run are supplied
- WHEN the chain runs only the CLI stage
- THEN the CLI step SHALL run using the supplied SDK artifacts
- AND no upstream stage SHALL be executed

#### Scenario: Subgraph refuses to start with missing inputs

- GIVEN no SDK artifacts are present or supplied
- WHEN the chain is asked to run only the CLI stage
- THEN it SHALL refuse to start and SHALL report the missing SDK input

### Requirement: AB-07 -- Durable Run State and Typed Artifacts

Each step SHALL consume and produce **typed artifacts** (for example the parsed spec model, the gap table, the OpenAPI change set, the generated SDK, the wave plan). The chain SHALL persist run state — completed steps, their artifacts, and their gate outcomes — to a durable store keyed by a run identifier.

A run SHALL be **resumable**: after an interruption or a gate failure that is later fixed, rerunning SHALL continue from the last incomplete step rather than repeating completed, gated work, unless a full rerun is explicitly requested. Re-executing a completed step SHALL replace its artifacts atomically so downstream steps never observe a partially written artifact.

**Verification:** Interrupt a run mid-wave and resume; confirm completed and gated steps are not repeated and the run continues from the first incomplete step; confirm artifacts are typed and validated at step boundaries.

#### Scenario: Resume after interruption

- GIVEN a run completed the API and SDK steps and was interrupted during the BE step
- WHEN the run is resumed
- THEN the API and SDK steps SHALL NOT be re-executed
- AND the run SHALL continue from the BE step using the persisted SDK artifacts

### Requirement: AB-08 -- Metrics Collection and Cross-Configuration Comparison

The chain SHALL record metrics for every step and every run. Per-step metrics SHALL include at least: step identifier and class, resolved model and provider, prompt and completion token counts, estimated cost, wall-clock duration, number of tool calls, retry and escalation counts, gate outcome, and a summary of artifacts changed. Per-run metrics SHALL include at least: run identifier, profile name, the git base revision, aggregate tokens, aggregate cost, total duration, waves completed, and the final gate outcome.

Metrics SHALL be persisted to a durable, queryable store so that runs of the **same** graph under **different** profiles can be compared on cost, latency, token usage, and gate outcomes. The chain MAY additionally export these metrics over OTLP, consistent with `platform/api-server-observability.spec.md` and `platform/control-plane-observability.spec.md`. Metrics SHALL NOT contain secrets or credentials, consistent with `standards/security/security.spec.md`.

**Verification:** Run the chain under two profiles; confirm per-step and per-run metrics are persisted with the required fields; produce a comparison of the two runs on cost, latency, and gate outcomes; confirm no secrets appear in the metrics.

#### Scenario: Compare two profiles on the same work

- GIVEN the chain ran the same graph under `tiered` and under `all-deep`
- WHEN the two runs' metrics are compared
- THEN the comparison SHALL show per-run cost, token usage, and duration for each profile
- AND both runs SHALL report their final gate outcomes so quality can be held constant while cost and latency are compared

#### Scenario: Secrets excluded from metrics

- GIVEN a step invoked a command that received a token or credential
- WHEN its metrics are recorded
- THEN no token, credential, or secret SHALL appear in any metric field

### Requirement: AB-09 -- Human-in-the-Loop Checkpoints

The chain SHALL support configurable human checkpoints. By default the **spec-consensus** step (the skill's Wave 1: confirm the gap table and freeze the spec) SHALL require explicit human approval before downstream code work begins. A profile MAY run in a fully autonomous mode that removes non-safety checkpoints, and MAY add checkpoints at additional stages.

Where the skill requires atomic pull requests per wave per component, the chain SHALL support producing those pull requests at wave boundaries rather than a single combined change, so review remains per-wave.

**Verification:** Run in supervised mode and confirm the chain halts for approval at the spec-consensus checkpoint before any code wave; run in autonomous mode and confirm it proceeds without halting except at safety checkpoints; confirm per-wave pull requests can be produced.

#### Scenario: Spec consensus halts for approval

- GIVEN the chain runs in the default supervised mode
- WHEN it completes the gap analysis and reaches the spec-consensus checkpoint
- THEN it SHALL halt and request human approval
- AND no code wave SHALL start until approval is given

### Requirement: AB-10 -- Failure Handling, Retries, and Escalation

When a step fails its gate (`AB-04`), the chain SHALL apply bounded retries. A `mechanical` step that fails its gate MAY **escalate** to the next higher model tier (for example `small`→`standard`→`deep`) before a subsequent attempt; a step that exhausts its retry and escalation budget SHALL surface to a human rather than proceeding past a failed gate. Retry, escalation, and hand-off decisions SHALL be recorded in the run metrics (`AB-08`).

Escalation SHALL never bypass a gate: an escalated attempt SHALL still be required to pass the same deterministic gate before its step is marked complete.

**Verification:** Force a `small`-tier step to fail its gate and confirm the chain retries, escalates to a higher tier, records the escalation, and — if still failing after the budget — halts for a human rather than continuing; confirm no escalated attempt is accepted without passing the gate.

#### Scenario: Escalate a failing mechanical step

- GIVEN a `mechanical` step on the `small` tier fails its acceptance gate
- WHEN the chain retries within budget
- THEN it MAY escalate the step to a higher tier for the next attempt
- AND the escalated attempt SHALL still be required to pass the same gate
- AND the escalation SHALL be recorded in the run metrics

#### Scenario: Exhausted budget surfaces to a human

- GIVEN a step has exhausted its retry and escalation budget while still failing its gate
- WHEN the chain evaluates the step
- THEN it SHALL halt and surface the failure to a human
- AND it SHALL NOT start any downstream step

### Requirement: AB-11 -- Component Location and Packaging

The orchestration code SHALL live in a new Python component under `components/` (for example `components/build-pipeline`), beside the Go and TypeScript components it drives. Configuration — profiles, model and provider bindings, and endpoints — SHALL be data separate from code, so a new configuration is added without changing step logic (`AB-03`). The component SHALL expose a command-line entry point that selects a profile and an execution mode (`AB-06`) and runs the chain.

**Verification:** Confirm the component resides under `components/`, that profiles are external configuration rather than code, and that the command-line entry point runs the chain under a chosen profile and mode.

#### Scenario: Add a configuration without code change

- GIVEN a new profile that binds tiers to a different provider's models
- WHEN it is added as configuration and selected on the command line
- THEN the chain SHALL run under the new profile with no change to step logic

## Design Decisions

| Decision | Rationale |
| --- | --- |
| The chain executes the skill; the skill stays the source of truth | Avoids two competing definitions of the workflow drifting apart; the skill remains human-readable and the chain stays a faithful executor, with a drift check (`AB-05`) to enforce it |
| Model is bound per step by configuration, not code | This is the core value: the same deterministic workflow runs under many model configurations so cost and quality tradeoffs can be explored and compared |
| Deterministic gates are model-independent | Holds output correctness constant across profiles, so cross-configuration comparisons measure cost and latency rather than correctness drift |
| A `none`/`tool-only` tier for pure commands | The cheapest correct choice for a deterministic command (SDK generation, `make test`) is no model at all; forcing a model on these steps would add cost and variance for no benefit |
| Tiered profile is the default | Matches the observation that most pipeline steps are mechanical and cheap while a few need deep reasoning; gives a sensible cost/quality baseline out of the box |
| Escalation ladder small→standard→deep→human | Cheap-first with a safety net: recovers from small-model failures automatically within budget, then hands off rather than proceeding past a failed gate |
| Metrics persisted per run and comparable | The comparison across configurations is a first-class output of this component, not a side effect |
| Spec-consensus remains a human checkpoint by default | Confirming the gap table and freezing the spec is a judgment call that defines the desired state; automating it away would undermine the spec-driven contract |
| Python + LangChain under `components/` | LangChain provides a provider-agnostic chat-model and tool-calling interface; Python is the natural orchestration ecosystem; co-locating with the other components keeps the build tooling in one tree |

## References

- `skills/build/full-stack-pipeline/SKILL.md` -- the workflow this chain executes (source of truth for steps and acceptance commands)
- `platform/e2e-testing.spec.md` -- the e2e acceptance gate and infra drivers
- `platform/api-server-observability.spec.md`, `platform/control-plane-observability.spec.md` -- OTLP metrics conventions the chain MAY reuse
- `standards/security/security.spec.md` -- no secrets in telemetry or metrics
- `standards/platform/naming-multitenancy.spec.md` -- naming conventions
