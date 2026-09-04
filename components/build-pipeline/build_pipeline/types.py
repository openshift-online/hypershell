"""Core data types for the agentic build chain.

These types are pure stdlib (dataclasses + enums) so the graph, profile
resolution, gate runner, engine, and metrics can be imported and exercised
without LangChain or any model provider installed. Only the LLM step executor
(``steps.py`` / ``models.py``) imports provider libraries, and it does so
lazily. See specs/tooling/build/agentic-build-chain.spec.md.
"""

from __future__ import annotations

from dataclasses import dataclass, field
from enum import Enum


class Tier(str, Enum):
    """Model capability/cost tier a step's model is drawn from (AB-02)."""

    NONE = "none"          # no model at all -- a pure command invocation
    SMALL = "small"        # fast / cheap
    STANDARD = "standard"  # mid
    DEEP = "deep"          # most capable / most expensive


class StepClass(str, Enum):
    """What kind of work a step performs (AB-02)."""

    TOOL_ONLY = "tool-only"    # deterministic command, no model
    MECHANICAL = "mechanical"  # pattern-following edit/scaffold
    REASONING = "reasoning"    # analysis / synthesis / judgment


# Default tier for each step class (AB-02). A profile maps these tiers to
# concrete models (AB-03); this table only fixes which tier a class draws from.
DEFAULT_TIER: dict[StepClass, Tier] = {
    StepClass.TOOL_ONLY: Tier.NONE,
    StepClass.MECHANICAL: Tier.SMALL,
    StepClass.REASONING: Tier.DEEP,
}

# Escalation ladder for a failing mechanical step (AB-10). NONE is excluded --
# a tool-only step never escalates to a model, it just retries its command.
ESCALATION_LADDER: tuple[Tier, ...] = (Tier.SMALL, Tier.STANDARD, Tier.DEEP)


@dataclass(frozen=True)
class Command:
    """A subprocess invocation. ``cwd`` is relative to the repository root."""

    argv: tuple[str, ...]
    cwd: str = "."
    name: str = ""

    def display(self) -> str:
        return f"({self.cwd}) {' '.join(self.argv)}"


@dataclass(frozen=True)
class StepSpec:
    """A node in the build chain graph (AB-01)."""

    id: str
    name: str
    wave: int
    step_class: StepClass
    deps: tuple[str, ...] = ()
    consumes: tuple[str, ...] = ()
    produces: tuple[str, ...] = ()
    # tool-only work: the command a tool-only step runs. None => the LLM agent
    # performs the work for mechanical/reasoning steps.
    action: Command | None = None
    # deterministic, model-independent acceptance gates (AB-04). All must pass.
    gates: tuple[Command, ...] = ()
    human: bool = False  # human checkpoint before this step proceeds (AB-09)
    description: str = ""

    def default_tier(self) -> Tier:
        return DEFAULT_TIER[self.step_class]


@dataclass(frozen=True)
class ModelBinding:
    """A resolved model for a step (AB-03). ``None`` binding => tool-only."""

    tier: Tier
    provider: str
    model: str
    params: dict = field(default_factory=dict)

    def label(self) -> str:
        return f"{self.provider}:{self.model}"


@dataclass
class GateResult:
    name: str
    command: str
    passed: bool
    exit_code: int
    duration_s: float
    stdout_tail: str = ""
    stderr_tail: str = ""


@dataclass
class StepResult:
    """Per-step metrics and outcome (AB-08)."""

    step_id: str
    step_class: str
    status: str = "pending"  # pending|completed|failed|skipped|blocked|awaiting-human
    tier: str = ""
    model: str = ""  # "" for tool-only
    prompt_tokens: int = 0
    completion_tokens: int = 0
    cost_usd: float = 0.0
    duration_s: float = 0.0
    tool_calls: int = 0
    attempts: int = 0
    retries: int = 0
    escalated: bool = False
    gate_passed: bool | None = None
    gates: list[GateResult] = field(default_factory=list)
    artifacts_changed: list[str] = field(default_factory=list)
    error: str = ""


@dataclass
class RunRecord:
    """Per-run metrics and state (AB-07, AB-08)."""

    run_id: str
    profile: str
    mode: str
    git_base: str = ""
    started_at: str = ""
    finished_at: str = ""
    status: str = "running"  # running|completed|failed|halted
    steps: list[StepResult] = field(default_factory=list)

    def result_for(self, step_id: str) -> StepResult | None:
        for s in self.steps:
            if s.step_id == step_id:
                return s
        return None

    def completed_ids(self) -> set[str]:
        return {s.step_id for s in self.steps if s.status == "completed"}
