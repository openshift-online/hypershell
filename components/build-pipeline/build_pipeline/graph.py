"""The build chain graph (AB-01).

This is the executable encoding of skills/build/full-stack-pipeline/SKILL.md:
one node per workflow step, edges in the skill's fixed dependency order
(Spec -> API -> SDK -> {BE, gRPC, CLI} -> CP -> Integration). The skill remains
the source of truth (AB-05); ``check_drift`` in cli.py flags divergence.

Gate/action commands below are the acceptance commands the skill documents.
They are data, so a profile or override can adjust them without touching engine
logic, and the drift check reconciles them against the skill.
"""

from __future__ import annotations

from .types import Command, StepClass, StepSpec


def _c(argv: list[str], cwd: str = ".", name: str = "") -> Command:
    return Command(argv=tuple(argv), cwd=cwd, name=name)


# Convenience gate builders for the Go components.
def _go_build(cwd: str) -> Command:
    return _c(["go", "build", "./..."], cwd=cwd, name="go build")


def _go_vet(cwd: str) -> Command:
    return _c(["go", "vet", "./..."], cwd=cwd, name="go vet")


API = "components/api-server"
SDK_GO = "components/sdk-go"
SDK_TS = "components/sdk-typescript"
CLI = "components/cli"
CP = "components/control-plane"
SDKGEN = "scripts/sdk-generator"


PIPELINE: tuple[StepSpec, ...] = (
    StepSpec(
        id="parse-spec",
        name="Read the spec",
        wave=1,
        step_class=StepClass.REASONING,
        produces=("spec_model",),
        description=(
            "Read the target spec in full and extract entities, fields, "
            "relationships, API routes, and design decisions. This is the "
            "desired state everything else is measured against."
        ),
    ),
    StepSpec(
        id="gap-analysis",
        name="Gap analysis",
        wave=1,
        step_class=StepClass.REASONING,
        deps=("parse-spec",),
        consumes=("spec_model",),
        produces=("gap_table",),
        description=(
            "Compare the spec against current code across all components "
            "(API, SDK, BE, CLI, gRPC, CP) in all three directions and "
            "produce a gap table of ENTITY/COMPONENT/STATUS/GAP rows."
        ),
    ),
    StepSpec(
        id="spec-consensus",
        name="Spec consensus (human)",
        wave=1,
        step_class=StepClass.REASONING,
        deps=("gap-analysis",),
        consumes=("gap_table",),
        produces=("frozen_spec",),
        human=True,
        description=(
            "Confirm the gap table is complete and agreed, and freeze the "
            "spec for this run. Human approval gate (AB-09)."
        ),
    ),
    StepSpec(
        id="plan-waves",
        name="Break into waves",
        wave=1,
        step_class=StepClass.REASONING,
        deps=("spec-consensus",),
        consumes=("gap_table", "frozen_spec"),
        produces=("wave_plan",),
        description="Turn the agreed gap table into an ordered wave execution plan.",
    ),
    StepSpec(
        id="api",
        name="Wave 2 - API",
        wave=2,
        step_class=StepClass.MECHANICAL,
        deps=("plan-waves",),
        consumes=("wave_plan",),
        produces=("openapi",),
        gates=(_c(["make", "test"], cwd=API, name="make test"),
               _c(["make", "lint"], cwd=API, name="make lint")),
        description=(
            "Update openapi/*.yaml for new entities/routes, register routes, "
            "add handler stubs. Gates everything downstream."
        ),
    ),
    StepSpec(
        id="sdk",
        name="Wave 3 - SDK generation",
        wave=3,
        step_class=StepClass.TOOL_ONLY,
        deps=("api",),
        consumes=("openapi",),
        produces=("sdk_go", "sdk_ts"),
        action=_c(
            ["go", "run", ".",
             "--spec", "../../components/api-server/openapi/openapi.yaml",
             "--go-out", "../../components/sdk-go",
             "--ts-out", "../../components/sdk-typescript"],
            cwd=SDKGEN, name="sdk-generator"),
        gates=(_go_build(SDK_GO),),
        description=(
            "Regenerate the Go and TypeScript SDKs from the updated OpenAPI "
            "spec. Deterministic command -- no model (AB-02)."
        ),
    ),
    StepSpec(
        id="be",
        name="Wave 4 - Backend",
        wave=4,
        step_class=StepClass.MECHANICAL,
        deps=("sdk",),
        consumes=("sdk_go",),
        produces=("backend",),
        gates=(_c(["make", "test"], cwd=API, name="make test"), _go_vet(API)),
        description="Migrations, DAOs, service logic, and gRPC presenters.",
    ),
    StepSpec(
        id="grpc",
        name="Wave 4 - gRPC",
        wave=4,
        step_class=StepClass.MECHANICAL,
        deps=("sdk",),
        consumes=("sdk_go",),
        produces=("grpc",),
        gates=(_go_build(API),),
        description="Proto updates, `make proto`, handler and presenter implementation.",
    ),
    StepSpec(
        id="cli",
        name="Wave 5 - CLI",
        wave=5,
        step_class=StepClass.MECHANICAL,
        deps=("sdk",),
        consumes=("sdk_go",),
        produces=("cli",),
        gates=(_go_build(CLI),),
        description="Implement the planned CLI commands following existing patterns.",
    ),
    StepSpec(
        id="cp",
        name="Wave 6 - Control plane",
        wave=6,
        step_class=StepClass.REASONING,
        deps=("be", "grpc"),
        consumes=("backend", "grpc"),
        produces=("control_plane",),
        gates=(_go_build(CP), _go_vet(CP)),
        description="Watcher subscription and reconciler logic for the new Kinds.",
    ),
    StepSpec(
        id="integration",
        name="Wave 7 - Integration",
        wave=7,
        step_class=StepClass.REASONING,
        deps=("cli", "cp"),
        consumes=("cli", "control_plane"),
        produces=("integration",),
        gates=(_c(["bash", "components/pr-test/e2e-openshell.sh"], name="e2e"),),
        description=(
            "End-to-end smoke test; diagnose failures. The run is mechanical; "
            "diagnosis benefits from a deep model, so this step is reasoning."
        ),
    ),
)


def by_id(step_id: str) -> StepSpec:
    for s in PIPELINE:
        if s.id == step_id:
            return s
    raise KeyError(f"no such step: {step_id}")


def validate(pipeline: tuple[StepSpec, ...] = PIPELINE) -> None:
    """Raise if any dep is unknown or the graph has a cycle."""
    ids = {s.id for s in pipeline}
    for s in pipeline:
        for d in s.deps:
            if d not in ids:
                raise ValueError(f"step {s.id!r} depends on unknown step {d!r}")
    topo_order(pipeline)  # raises on cycle


def topo_order(pipeline: tuple[StepSpec, ...] = PIPELINE) -> list[StepSpec]:
    """Kahn's algorithm; ties broken by (wave, id) for stable ordering."""
    remaining = {s.id: set(s.deps) for s in pipeline}
    spec = {s.id: s for s in pipeline}
    ordered: list[StepSpec] = []
    while remaining:
        ready = sorted(
            (sid for sid, deps in remaining.items() if not deps),
            key=lambda sid: (spec[sid].wave, sid),
        )
        if not ready:
            raise ValueError(f"cycle detected among steps: {sorted(remaining)}")
        for sid in ready:
            ordered.append(spec[sid])
            del remaining[sid]
            for deps in remaining.values():
                deps.discard(sid)
    return ordered


def select(
    from_: str | None = None,
    to: str | None = None,
    only: str | None = None,
    pipeline: tuple[StepSpec, ...] = PIPELINE,
) -> list[StepSpec]:
    """Choose a subgraph to execute (AB-06), preserving topological order.

    ``only`` selects a single step. ``from_``/``to`` bound the wave range by
    the selected steps' waves (inclusive). Dependency enforcement (missing
    inputs must be supplied) is the engine's responsibility.
    """
    ordered = topo_order(pipeline)
    if only:
        return [by_id(only)]
    lo = by_id(from_).wave if from_ else min(s.wave for s in ordered)
    hi = by_id(to).wave if to else max(s.wave for s in ordered)
    return [s for s in ordered if lo <= s.wave <= hi]
