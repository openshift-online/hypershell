"""The scheduler (AB-01, AB-04, AB-06, AB-07, AB-09, AB-10).

Runs selected steps in topological order, enforcing dependency inputs, human
checkpoints, deterministic gates, and bounded retry/escalation. The dry-run and
tool-only paths import nothing from LangChain; only a real LLM-backed step
lazily imports the model/agent layer.
"""

from __future__ import annotations

import time
from dataclasses import dataclass
from pathlib import Path

from . import gates as gate_runner
from .metrics import summarize
from .profiles import Profile
from .state import RunStore
from .types import (
    ESCALATION_LADDER,
    ModelBinding,
    RunRecord,
    StepClass,
    StepResult,
    StepSpec,
    Tier,
)


@dataclass
class EngineConfig:
    repo_root: Path
    profile: Profile
    store: RunStore
    mode: str = "supervised"          # supervised | autonomous
    dry_run: bool = False
    approve_human: bool = False        # pre-approve human checkpoints
    max_attempts: int = 3              # attempts per step before halting (AB-10)
    gate_timeout: float | None = None


def _escalate(tier: Tier) -> Tier:
    if tier in ESCALATION_LADDER:
        i = ESCALATION_LADDER.index(tier)
        if i + 1 < len(ESCALATION_LADDER):
            return ESCALATION_LADDER[i + 1]
    return tier


class Engine:
    def __init__(self, cfg: EngineConfig):
        self.cfg = cfg

    # -- public ---------------------------------------------------------------

    def run(
        self,
        steps: list[StepSpec],
        rec: RunRecord,
        supplied: set[str] | None = None,
    ) -> RunRecord:
        cfg = self.cfg
        self._active_run_id = rec.run_id
        available: set[str] = set(supplied or set())
        # Resume: completed steps' outputs are already produced and persisted.
        for done in rec.steps:
            if done.status == "completed":
                available.update(_by_id(steps, done.step_id).produces if _has(steps, done.step_id) else ())
        done_ids = rec.completed_ids()

        for step in steps:
            if step.id in done_ids:
                continue  # resume: skip already-completed work (AB-07)

            res = self._run_one(step, available)
            _upsert(rec, res)
            cfg.store.save(rec)

            if res.status == "completed":
                available.update(step.produces)
                for name in step.produces:
                    if not cfg.store.has_artifact(rec.run_id, name):
                        cfg.store.write_artifact(rec.run_id, name, res.error or "produced")
                continue

            if res.status == "planned":
                continue  # dry-run: keep planning the rest

            # blocked | awaiting-human | failed -> stop the run
            cfg.store.finish(rec, "halted" if res.status != "failed" else "failed")
            return rec

        cfg.store.finish(rec, "completed" if not cfg.dry_run else "running")
        return rec

    # -- per-step -------------------------------------------------------------

    def _run_one(self, step: StepSpec, available: set[str]) -> StepResult:
        cfg = self.cfg
        binding = cfg.profile.resolve(step)
        res = StepResult(
            step_id=step.id,
            step_class=step.step_class.value,
            tier=(binding.tier.value if binding else Tier.NONE.value),
            model=(binding.label() if binding else ""),
        )

        if cfg.dry_run:
            res.status = "planned"
            return res

        missing = [c for c in step.consumes if c not in available]
        if missing:
            res.status = "blocked"
            res.error = "missing inputs: " + ", ".join(missing)
            return res

        # A human checkpoint's "work" is the approval itself, not model work:
        # once approved it completes without invoking an LLM (AB-09).
        if step.human:
            if cfg.mode == "supervised" and not cfg.approve_human:
                res.status = "awaiting-human"
                res.error = "human approval required; re-run with --approve-human or --mode autonomous"
                return res
            res.status = "completed"
            return res

        return self._attempt_loop(step, binding, res)

    def _attempt_loop(self, step: StepSpec, binding: ModelBinding | None, res: StepResult) -> StepResult:
        cfg = self.cfg
        tier = binding.tier if binding else Tier.NONE
        for attempt in range(1, cfg.max_attempts + 1):
            res.attempts = attempt
            if attempt > 1:
                res.retries += 1
                if step.step_class is StepClass.MECHANICAL and binding is not None:
                    new_tier = _escalate(tier)
                    if new_tier != tier:
                        tier = new_tier
                        binding = _rebind(cfg.profile, tier)
                        res.escalated = True
                        res.tier = tier.value
                        res.model = binding.label()

            work_err = self._do_work(step, binding, res)
            if work_err:
                res.error = work_err
                continue

            ok, results = gate_runner.run_gates(step.gates, cfg.repo_root, cfg.gate_timeout)
            res.gates = results
            res.gate_passed = ok
            if ok:
                res.status = "completed"
                res.error = ""
                return res
            res.error = "gate failed: " + "; ".join(
                f"{g.name}(exit={g.exit_code})" for g in results if not g.passed
            )

        res.status = "failed"  # exhausted retries/escalation (AB-10) -> surface to human
        return res

    def _do_work(self, step: StepSpec, binding: ModelBinding | None, res: StepResult) -> str:
        """Perform the step's work. Returns an error string, or '' on success."""
        cfg = self.cfg
        if step.step_class is StepClass.TOOL_ONLY:
            if step.action is None:
                return ""  # nothing to do; acceptance is decided purely by gates
            start = time.monotonic()
            gr = gate_runner.run_command(step.action, cfg.repo_root, cfg.gate_timeout)
            res.duration_s += round(time.monotonic() - start, 3)
            return "" if gr.passed else f"action failed (exit={gr.exit_code}): {gr.name}"

        # mechanical | reasoning -> LLM agent (lazy imports; needs LangChain + creds)
        try:
            from . import models, steps, tools
        except Exception as exc:  # pragma: no cover - only when deps missing
            return f"LLM layer unavailable ({exc}); install the 'llm' extra"
        try:
            model = models.build_chat_model(binding)
        except Exception as exc:
            return f"model construction failed: {exc}"
        if model is None:
            return "no model resolved for a non-tool-only step"

        start = time.monotonic()
        outcome = steps.run_llm_step(step, model, tools.build_tools(cfg.repo_root), cfg.repo_root)
        res.duration_s += round(time.monotonic() - start, 3)
        res.prompt_tokens += outcome.input_tokens
        res.completion_tokens += outcome.output_tokens
        res.tool_calls += outcome.tool_calls
        res.cost_usd += models.estimate_cost(binding, outcome.input_tokens, outcome.output_tokens)
        for a in outcome.artifacts_written:
            if a and a not in res.artifacts_changed:
                res.artifacts_changed.append(a)
        if outcome.error:
            return outcome.error
        # For gate-less analysis/planning steps, persist the model's output as
        # the produced artifact so downstream steps and resume can consume it.
        if not step.gates and step.produces:
            self.cfg.store.write_artifact(self._active_run_id, step.produces[0], outcome.final_text or "")
        return ""


# -- small helpers ------------------------------------------------------------

def _rebind(profile: Profile, tier: Tier) -> ModelBinding:
    cfg = profile.tiers[tier.value]
    return ModelBinding(tier=tier, provider=cfg["provider"], model=cfg["model"], params=dict(cfg.get("params", {})))


def _by_id(steps: list[StepSpec], sid: str) -> StepSpec:
    for s in steps:
        if s.id == sid:
            return s
    raise KeyError(sid)


def _has(steps: list[StepSpec], sid: str) -> bool:
    return any(s.id == sid for s in steps)


def _upsert(rec: RunRecord, res: StepResult) -> None:
    for i, s in enumerate(rec.steps):
        if s.step_id == res.step_id:
            rec.steps[i] = res
            return
    rec.steps.append(res)



def run_summary(rec: RunRecord) -> dict:
    return summarize(rec)
