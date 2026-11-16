"""Pure-logic tests for the delivery replay (no network)."""

import unittest

from scripts import delivery_replay as replay
from scripts.delivery_replay import Estimate, PhaseView, Run

MIN = 60.0


class EstimateTest(unittest.TestCase):
    def test_uses_only_runs_that_ended_before(self):
        # Verifies: FLT-DLV-21.01
        runs = [Run(10, 1 * MIN), Run(20, 2 * MIN), Run(30, 3 * MIN), Run(40, 100 * MIN)]
        e = replay.estimate(runs, at=35)
        self.assertEqual(e, Estimate(median=2 * MIN, slowest=3 * MIN))

    def test_median_and_slowest_of_last_five(self):
        # Verifies: FLT-DLV-09.02
        durations = [30, 31, 32, 40, 41, 42, 90]
        runs = [Run(i, d * MIN) for i, d in enumerate(durations)]
        e = replay.estimate(runs, at=100)
        self.assertEqual(e, Estimate(median=41 * MIN, slowest=90 * MIN))

    def test_fewer_than_three_runs_has_no_estimate(self):
        # Verifies: FLT-DLV-09.01
        self.assertIsNone(replay.estimate([Run(1, MIN), Run(2, MIN)], at=10))

    def test_slowest_component_decides(self):
        # Verifies: FLT-DLV-09.04
        e = replay.combine([Estimate(8 * MIN, 10 * MIN), Estimate(14 * MIN, 15 * MIN)])
        self.assertEqual(e, Estimate(14 * MIN, 15 * MIN))
        self.assertIsNone(replay.combine([Estimate(8 * MIN, 10 * MIN), None]))
        self.assertIsNone(replay.combine([]))


class ContainmentTest(unittest.TestCase):
    # Commits c0..c4 on a linear default branch, oldest first.
    pos = {f"c{i}": i for i in range(5)}

    def test_failed_build_is_not_in_the_next_bundle(self):
        # Verifies: FLT-DLV-06.05
        # c2 rebuilt web; its build failed, so B1 still carries web from c1.
        b1 = {"web": "c1", "api": "c3"}
        b2 = {"web": "c4", "api": "c3"}
        self.assertFalse(replay.bundle_contains(b1, ["web"], self.pos, 2, "c3"))
        self.assertTrue(replay.manifest_contains("c3", self.pos, 2))
        self.assertTrue(replay.bundle_contains(b2, ["web"], self.pos, 2, "c4"))

    def test_no_rebuilt_component_uses_manifest_rule(self):
        # Verifies: FLT-DLV-06
        b = {"web": "c0"}
        self.assertTrue(replay.bundle_contains(b, [], self.pos, 2, "c3"))
        self.assertFalse(replay.bundle_contains(b, ["outside"], self.pos, 2, "c1"))

    def test_revision_older_than_window_is_before(self):
        # Verifies: FLT-DLV-06
        self.assertFalse(replay.bundle_contains({"web": "old"}, ["web"], self.pos, 0, "c0"))


class ArrivalTest(unittest.TestCase):
    def test_range_before_publication(self):
        # Verifies: FLT-DLV-10.04
        phases = [
            PhaseView(False, 4 * MIN, Estimate(10 * MIN, 15 * MIN)),
            PhaseView(False, 0, Estimate(5 * MIN, 8 * MIN)),
            PhaseView(False, 0, Estimate(30 * MIN, 45 * MIN)),
            PhaseView(False, 0, Estimate(10 * MIN, 15 * MIN)),
            PhaseView(False, 0, Estimate(20 * MIN, 30 * MIN)),
        ]
        early, late = replay.arrival(0, phases, published=False)
        self.assertAlmostEqual(early, 0.8 * 71 * MIN)
        self.assertAlmostEqual(late, 109 * MIN)

    def test_one_time_after_publication(self):
        # Verifies: FLT-DLV-10.05
        phases = [
            PhaseView(True, 0, None),
            PhaseView(False, 4 * MIN, Estimate(10 * MIN, 15 * MIN)),
            PhaseView(False, 0, Estimate(20 * MIN, 30 * MIN)),
        ]
        self.assertEqual(replay.arrival(0, phases, published=True), (26 * MIN, 26 * MIN))

    def test_missing_estimate_has_no_arrival(self):
        # Verifies: FLT-DLV-10
        self.assertIsNone(replay.arrival(0, [PhaseView(False, 0, None)], published=True))


class OverdueTest(unittest.TestCase):
    est = Estimate(10 * MIN, 14 * MIN)

    def test_rules(self):
        # Verifies: FLT-DLV-04.01, FLT-DLV-04.02, FLT-DLV-21
        self.assertEqual(
            replay.overdue_flags(18 * MIN, self.est),
            {"2x-estimate": False, "slowest-of-5": True, "slowest-of-5+5min": False},
        )
        self.assertTrue(replay.overdue_flags(20 * MIN, self.est)["slowest-of-5+5min"])
        self.assertFalse(any(replay.overdue_flags(99 * MIN, None).values()))

    def test_real_stall(self):
        # Verifies: FLT-DLV-21
        self.assertFalse(replay.real_stall(29 * MIN, self.est))
        self.assertTrue(replay.real_stall(30 * MIN, self.est))


class ParsingTest(unittest.TestCase):
    def test_classify_check(self):
        # Verifies: FLT-DLV-05
        self.assertEqual(replay.classify_check("K / app-web-main-on-push"), ("build", "app-web-main"))
        self.assertEqual(
            replay.classify_check("K / app-main-enterprise-contract / app-web-main"), ("policy", "app-web-main")
        )
        self.assertIsNone(replay.classify_check("K / app-web-main-on-merge-queue"))

    def test_gate_runs_prefer_branch_run_and_flag_reruns(self):
        # Verifies: FLT-DLV-09.05
        runs = [
            {"name": "health/env/a", "status": "completed", "conclusion": "success",
             "started_at": "2026-11-01T10:00:00Z", "completed_at": "2026-11-01T10:05:00Z"},
            {"name": "health", "status": "completed", "conclusion": "success",
             "started_at": "2026-11-03T10:00:00Z", "completed_at": "2026-11-03T10:00:00Z"},
            {"name": "analysis/env/a", "status": "completed", "conclusion": "success",
             "started_at": "2026-11-01T10:00:00Z", "completed_at": "2026-11-01T10:20:00Z"},
        ]
        at, passed, rerun = replay.gate_runs(runs, ["health", "analysis"], "env/a")
        self.assertTrue(passed)
        self.assertFalse(rerun)
        self.assertEqual(replay.iso(at), "2026-11-01 10:20")
        runs.append(dict(runs[2], started_at="2026-11-01T11:00:00Z", completed_at="2026-11-01T11:30:00Z"))
        self.assertTrue(replay.gate_runs(runs, ["health", "analysis"], "env/a")[2])

    def test_strategy_groups_follow_dependencies(self):
        # Verifies: FLT-DLV-03.01
        text = """apiVersion: v1
kind: PromotionStrategy
spec:
  activeCommitStatuses:
  - key: health
  environments:
  - branch: env/c-x
    dependsOn:
    - env/b
  - branch: env/b
    dependsOn:
    - env/a
    activeCommitStatuses:
    - key: analysis
  - branch: env/a
    autoMerge: true
"""
        envs, deps, gates = replay.parse_strategy(text)
        self.assertEqual(envs, ["env/c-x", "env/b", "env/a"])
        self.assertEqual(gates["env/b"], ["analysis", "health"])
        groups = replay.env_groups(envs, deps, r"^env/([^-]+)")
        self.assertEqual([g for g, _ in groups], ["a", "b", "c"])


if __name__ == "__main__":
    unittest.main()
