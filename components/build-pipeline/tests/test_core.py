"""Deterministic-core tests: graph, profile resolution, gates, metrics, state.

Pure stdlib -- no LangChain required. Run from components/build-pipeline:
    python -m unittest discover -s tests
"""

import tempfile
import unittest
from pathlib import Path

from build_pipeline import gates, graph, metrics
from build_pipeline.profiles import Profile, load_profile
from build_pipeline.state import RunStore
from build_pipeline.types import Command, StepClass, StepResult, StepSpec, Tier


class GraphTests(unittest.TestCase):
    def test_validate_ok(self):
        graph.validate()

    def test_topo_respects_dependency_order(self):
        order = [s.id for s in graph.topo_order()]
        self.assertLess(order.index("api"), order.index("sdk"))
        self.assertLess(order.index("sdk"), order.index("be"))
        self.assertLess(order.index("be"), order.index("cp"))
        self.assertLess(order.index("cp"), order.index("integration"))

    def test_select_range_is_inclusive_and_bounded(self):
        ids = [s.id for s in graph.select(from_="sdk", to="cli")]
        self.assertIn("sdk", ids)
        self.assertNotIn("parse-spec", ids)
        self.assertNotIn("integration", ids)

    def test_select_only_single_step(self):
        self.assertEqual([s.id for s in graph.select(only="sdk")], ["sdk"])

    def test_unknown_dependency_rejected(self):
        bad = (StepSpec(id="a", name="a", wave=1, step_class=StepClass.TOOL_ONLY, deps=("nope",)),)
        with self.assertRaises(ValueError):
            graph.validate(bad)

    def test_cycle_detected(self):
        bad = (
            StepSpec("a", "a", 1, StepClass.TOOL_ONLY, deps=("b",)),
            StepSpec("b", "b", 1, StepClass.TOOL_ONLY, deps=("a",)),
        )
        with self.assertRaises(ValueError):
            graph.topo_order(bad)


class ProfileTests(unittest.TestCase):
    def test_tool_only_resolves_to_no_model(self):
        self.assertIsNone(load_profile("tiered").resolve(graph.by_id("sdk")))

    def test_mechanical_defaults_to_small(self):
        self.assertEqual(load_profile("tiered").resolve(graph.by_id("api")).tier, Tier.SMALL)

    def test_reasoning_defaults_to_deep(self):
        self.assertEqual(load_profile("tiered").resolve(graph.by_id("gap-analysis")).tier, Tier.DEEP)

    def test_override_wins_over_tier_default(self):
        p = Profile(
            name="x",
            tiers={
                "small": {"provider": "vertex", "model": "s"},
                "standard": {"provider": "vertex", "model": "m"},
                "deep": {"provider": "vertex", "model": "d"},
            },
            overrides={"gap-analysis": "standard"},
        )
        self.assertEqual(p.resolve(graph.by_id("gap-analysis")).model, "m")

    def test_all_small_maps_every_tier_to_one_model(self):
        p = load_profile("all-small")
        self.assertEqual(
            p.resolve(graph.by_id("gap-analysis")).model,  # reasoning/deep
            p.resolve(graph.by_id("api")).model,           # mechanical/small
        )


class GateTests(unittest.TestCase):
    def test_passing_command(self):
        self.assertTrue(gates.run_command(Command(("true",)), Path.cwd()).passed)

    def test_failing_command(self):
        self.assertFalse(gates.run_command(Command(("false",)), Path.cwd()).passed)

    def test_missing_binary_is_127(self):
        r = gates.run_command(Command(("this-binary-does-not-exist-xyz",)), Path.cwd())
        self.assertEqual(r.exit_code, 127)
        self.assertFalse(r.passed)

    def test_gates_stop_at_first_failure(self):
        ok, results = gates.run_gates((Command(("false",)), Command(("true",))), Path.cwd())
        self.assertFalse(ok)
        self.assertEqual(len(results), 1)

    def test_empty_gates_pass(self):
        ok, results = gates.run_gates((), Path.cwd())
        self.assertTrue(ok)
        self.assertEqual(results, [])


class MetricsStateTests(unittest.TestCase):
    def test_run_roundtrip_and_summary(self):
        with tempfile.TemporaryDirectory() as d:
            store = RunStore(Path(d))
            rec = store.new_run("tiered", "supervised", "abc123")
            rec.steps.append(
                StepResult(
                    step_id="api", step_class="mechanical", status="completed",
                    prompt_tokens=10, completion_tokens=5, cost_usd=0.01, duration_s=1.0,
                )
            )
            store.save(rec)
            loaded = store.load(rec.run_id)
            self.assertEqual(loaded.steps[0].step_id, "api")
            s = metrics.summarize(loaded)
            self.assertEqual(s["prompt_tokens"], 10)
            self.assertEqual(s["steps_completed"], 1)

    def test_compare_renders_all_runs(self):
        with tempfile.TemporaryDirectory() as d:
            store = RunStore(Path(d))
            r1 = store.new_run("all-small", "autonomous", "")
            r2 = store.new_run("all-deep", "autonomous", "")
            table = metrics.compare([r1, r2])
            self.assertIn("all-small", table)
            self.assertIn("all-deep", table)


if __name__ == "__main__":
    unittest.main()
