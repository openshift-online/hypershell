"""Engine tests over synthetic tool-only graphs (no LangChain / no model calls).

Exercises dependency gating, deterministic gates, retries, human checkpoints,
dry-run planning, and the escalation ladder.
"""

import tempfile
import unittest
from pathlib import Path

from build_pipeline.engine import Engine, EngineConfig, _escalate
from build_pipeline.profiles import Profile
from build_pipeline.state import RunStore
from build_pipeline.types import Command, StepClass, StepSpec, Tier


def _profile() -> Profile:
    return Profile(
        name="t",
        tiers={
            "small": {"provider": "vertex", "model": "flash"},
            "standard": {"provider": "vertex", "model": "flash"},
            "deep": {"provider": "vertex", "model": "pro"},
        },
        overrides={},
    )


def _cfg(tmp, **kw) -> EngineConfig:
    return EngineConfig(
        repo_root=Path(tmp),
        profile=_profile(),
        store=RunStore(Path(tmp) / "runs"),
        **kw,
    )


class EscalationTests(unittest.TestCase):
    def test_ladder(self):
        self.assertEqual(_escalate(Tier.SMALL), Tier.STANDARD)
        self.assertEqual(_escalate(Tier.STANDARD), Tier.DEEP)
        self.assertEqual(_escalate(Tier.DEEP), Tier.DEEP)  # tops out


class EngineTests(unittest.TestCase):
    def _run(self, tmp, step, **cfgkw):
        cfg = _cfg(tmp, **cfgkw)
        engine = Engine(cfg)
        rec = cfg.store.new_run("t", cfgkw.get("mode", "supervised"), "")
        return engine.run([step], rec)

    def test_tool_only_action_and_gate_complete(self):
        with tempfile.TemporaryDirectory() as d:
            step = StepSpec("s", "s", 1, StepClass.TOOL_ONLY, action=Command(("true",)), gates=(Command(("true",)),))
            rec = self._run(d, step, mode="autonomous")
            self.assertEqual(rec.steps[0].status, "completed")
            self.assertTrue(rec.steps[0].gate_passed)

    def test_failing_gate_fails_after_retries(self):
        with tempfile.TemporaryDirectory() as d:
            step = StepSpec("s", "s", 1, StepClass.TOOL_ONLY, gates=(Command(("false",)),))
            rec = self._run(d, step, mode="autonomous", max_attempts=2)
            self.assertEqual(rec.steps[0].status, "failed")
            self.assertEqual(rec.steps[0].attempts, 2)
            self.assertEqual(rec.status, "failed")

    def test_failing_action_fails(self):
        with tempfile.TemporaryDirectory() as d:
            step = StepSpec("s", "s", 1, StepClass.TOOL_ONLY, action=Command(("false",)), gates=(Command(("true",)),))
            rec = self._run(d, step, mode="autonomous", max_attempts=1)
            self.assertEqual(rec.steps[0].status, "failed")

    def test_missing_input_blocks(self):
        with tempfile.TemporaryDirectory() as d:
            step = StepSpec("s", "s", 1, StepClass.TOOL_ONLY, consumes=("x",), gates=(Command(("true",)),))
            rec = self._run(d, step, mode="autonomous")
            self.assertEqual(rec.steps[0].status, "blocked")
            self.assertEqual(rec.status, "halted")

    def test_supplied_input_unblocks(self):
        with tempfile.TemporaryDirectory() as d:
            cfg = _cfg(d, mode="autonomous")
            step = StepSpec("s", "s", 1, StepClass.TOOL_ONLY, consumes=("x",), gates=(Command(("true",)),))
            rec = cfg.store.new_run("t", "autonomous", "")
            rec = Engine(cfg).run([step], rec, supplied={"x"})
            self.assertEqual(rec.steps[0].status, "completed")

    def test_human_supervised_awaits(self):
        with tempfile.TemporaryDirectory() as d:
            step = StepSpec("h", "h", 1, StepClass.REASONING, human=True)
            rec = self._run(d, step, mode="supervised")
            self.assertEqual(rec.steps[0].status, "awaiting-human")
            self.assertEqual(rec.status, "halted")

    def test_human_autonomous_completes_without_model(self):
        with tempfile.TemporaryDirectory() as d:
            step = StepSpec("h", "h", 1, StepClass.REASONING, human=True)
            rec = self._run(d, step, mode="autonomous")
            self.assertEqual(rec.steps[0].status, "completed")

    def test_dry_run_plans_and_resolves_models(self):
        with tempfile.TemporaryDirectory() as d:
            mech = StepSpec("m", "m", 1, StepClass.MECHANICAL, gates=(Command(("true",)),))
            rec = self._run(d, mech, dry_run=True)
            self.assertEqual(rec.steps[0].status, "planned")
            self.assertEqual(rec.steps[0].model, "vertex:flash")

    def test_resume_skips_completed(self):
        with tempfile.TemporaryDirectory() as d:
            cfg = _cfg(d, mode="autonomous")
            step = StepSpec("s", "s", 1, StepClass.TOOL_ONLY, gates=(Command(("true",)),))
            rec = cfg.store.new_run("t", "autonomous", "")
            rec = Engine(cfg).run([step], rec)
            self.assertEqual(rec.steps[0].status, "completed")
            # Resume: the completed step is not re-run (attempts stays 1).
            rec2 = Engine(cfg).run([step], cfg.store.load(rec.run_id))
            self.assertEqual(rec2.steps[0].attempts, 1)


if __name__ == "__main__":
    unittest.main()
