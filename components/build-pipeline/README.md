# build-pipeline

Agentic build chain for HyperShell (**HYPERSHELL-301**). It executes the
[`full-stack-pipeline`](../../skills/build/full-stack-pipeline/SKILL.md) workflow
as a typed step graph where **each step's model is chosen by configuration**:
tool-only and mechanical steps run cheap (or no) models, reasoning steps run a
deep model. The same graph runs under many profiles; deterministic gates hold
correctness constant so runs are comparable on cost and latency.

Spec: [`specs/tooling/build/agentic-build-chain.spec.md`](../../specs/tooling/build/agentic-build-chain.spec.md).

## Layout

```
build_pipeline/
  graph.py      # the pipeline as a typed step DAG (mirrors the skill)
  profiles.py   # load a profile; resolve each step's model (override > class tier)
  models.py     # build a LangChain chat model from a binding (lazy provider import)
  tools.py      # read_file / write_file / run_shell tools for agent steps
  steps.py      # bounded tool-calling loop for mechanical/reasoning steps
  gates.py      # deterministic, model-independent acceptance gates
  engine.py     # scheduler: deps, gates, retry/escalation, human checkpoints
  metrics.py    # per-run summary + cross-profile comparison
  state.py      # durable, resumable run state + artifacts
  cli.py        # `build-pipeline` entry point
  profiles/     # tiered.toml, all-small.toml, all-deep.toml
```

Only the LLM path (`models`, `tools`, `steps`) imports LangChain, and it does so
lazily -- `list`, `profiles`, and `--dry-run` work with the standard library
alone.

## Install

```bash
python3 -m venv .venv && . .venv/bin/activate
pip install -e components/build-pipeline          # runtime deps (LangChain + Vertex)
pip install -e "components/build-pipeline[dev]"   # + pytest for tests
```

## Explore without credentials

```bash
build-pipeline list                 # the step graph, classes, tiers, gates
build-pipeline profiles             # available profiles
build-pipeline profiles tiered      # resolved model per step under 'tiered'
build-pipeline run --dry-run        # plan the whole chain; resolve models; call nothing
```

## Local run with a Vertex key (a normal day)

You have a Vertex AI service-account key locally. Point Application Default
Credentials at it and set your project/region:

```bash
export GOOGLE_APPLICATION_CREDENTIALS=/path/to/vertex-key.json
export GOOGLE_CLOUD_PROJECT=my-gcp-project
export GOOGLE_CLOUD_LOCATION=us-central1
```

Then, from the repo root, a typical loop:

```bash
# 1. Start the chain for the spec you're implementing. Supervised mode halts at
#    the spec-consensus checkpoint so you can review the gap table first.
build-pipeline run --profile tiered

# 2. Approve the frozen spec and let it proceed wave by wave. Each wave is gated
#    by real commands (make test/lint, go build/vet, SDK generator, e2e); a
#    failing mechanical step retries and escalates small -> standard -> deep,
#    and surfaces to you if it still can't pass.
build-pipeline run --profile tiered --approve-human

# 3. Re-run a single wave while iterating (inputs from the prior run are reused):
build-pipeline run --from api --to sdk --profile tiered

# 4. Fully autonomous (no human halts) once you trust a change:
build-pipeline run --profile tiered --mode autonomous --approve-human
```

Runs are recorded under `.build-pipeline/runs/<run_id>/`. Compare configurations:

```bash
build-pipeline run --profile all-small --mode autonomous --approve-human
build-pipeline run --profile all-deep  --mode autonomous --approve-human
build-pipeline runs
build-pipeline compare <run_id_small> <run_id_deep>
```

Because gates are identical across profiles, the comparison reads as cost/latency
at equal quality when both runs report `completed`.

Keep the graph honest against the skill:

```bash
build-pipeline check-drift          # flags graph commands missing from the skill
```

## Configuration

Profiles are TOML (`build_pipeline/profiles/*.toml`), separate from code. A
profile binds each tier (`small`/`standard`/`deep`) to a concrete model and may
override a step's tier by id. Add a profile by dropping in a new `.toml` and
selecting it with `--profile <name-or-path>` -- no code change (AB-03/AB-11).
The `price_*_per_m` params are for local cost estimates only; set them to your
actual Vertex pricing. Provider is pluggable (`vertex` today; `openai` and
`anthropic` via the optional extras).

## Tests

```bash
cd components/build-pipeline
python -m unittest discover -s tests      # deterministic core; no credentials needed
```

## Container / cluster (later)

The program is 12-factor: all configuration is environment (`GOOGLE_*`) plus a
profile file, and the only host state is the runs directory. That makes it
straightforward to build into an image and run in a sandbox on a cluster later
-- mount the Vertex key (or use workload identity), select a profile, point
`--runs-dir` at a writable volume, and run in `--mode autonomous`.
