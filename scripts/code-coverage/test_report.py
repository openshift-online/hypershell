"""Tests for scripts/code-coverage/report.py (DEV-LOC-18)."""

import importlib.util
import json
import sys
import tempfile
import unittest
from pathlib import Path


SCRIPT_PATH = Path(__file__).with_name("report.py")
SPEC = importlib.util.spec_from_file_location("coverage_report", SCRIPT_PATH)
mod = importlib.util.module_from_spec(SPEC)
sys.modules["coverage_report"] = mod  # dataclasses resolve types via sys.modules
SPEC.loader.exec_module(mod)

P = "github.com/openshift-online/hypershell/components/"


def profile(*blocks: str) -> str:
    return "mode: atomic\n" + "\n".join(blocks) + "\n"


def write(root: Path, rel: str, text: str) -> None:
    path = root / rel
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(text, encoding="utf-8")


class ReportTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.root = Path(self.tmp.name)

    def tearDown(self):
        self.tmp.cleanup()

    def build(self) -> str:
        return mod.build_report(self.root)

    # A block counts once and is covered if either run hit it.
    # Verifies: DEV-LOC-18.03
    def test_merges_unit_and_e2e_by_block(self):
        write(self.root, "unit/go/control-plane.out", profile(
            f"{P}control-plane/internal/a/a.go:1.1,2.1 4 1",
            f"{P}control-plane/internal/a/a.go:3.1,4.1 6 0",
            f"{P}control-plane/internal/a/a.go:5.1,6.1 10 0",
        ))
        write(self.root, "e2e/go.out", profile(
            f"{P}control-plane/internal/a/a.go:1.1,2.1 4 3",
            f"{P}control-plane/internal/a/a.go:3.1,4.1 6 2",
            f"{P}control-plane/internal/a/a.go:5.1,6.1 10 0",
        ))
        report = self.build()
        # 20 statements: unit hits 4, e2e hits 10, together 10.
        self.assertIn("| control-plane | 20 | 20.0% | 50.0% | **50.0%** |", report)

    def test_generated_code_is_reported_apart(self):
        write(self.root, "unit/go/api-server.out", profile(
            f"{P}api-server/plugins/gateways/handler.go:1.1,2.1 10 1",
            f"{P}api-server/pkg/api/openapi/client.go:1.1,2.1 90 0",
        ))
        report = self.build()
        self.assertIn("| api-server | 10 | 100.0% | not run | **100.0%** |", report)
        self.assertIn("Generated code (reported apart):", report)
        self.assertIn("| api-server | 90 | 0.0% |", report)

    def test_test_support_package_is_skipped(self):
        write(self.root, "unit/go/api-server.out", profile(
            f"{P}api-server/plugins/users/service.go:1.1,2.1 10 1",
            f"{P}api-server/test/helper.go:1.1,2.1 50 0",
            f"{P}api-server/testdata/x.go:1.1,2.1 10 0",
        ))
        report = self.build()
        # api-server/test is skipped; a sibling that only shares the prefix is not.
        self.assertIn("| api-server | 20 | 50.0% |", report)
        self.assertNotIn("api-server/test |", report)

    def test_cli_is_marked_not_run_in_e2e(self):
        write(self.root, "unit/go/cli.out", profile(f"{P}cli/pkg/oidc/oidc.go:1.1,2.1 5 0"))
        write(self.root, "e2e/go.out", profile(f"{P}control-plane/cmd/x/main.go:1.1,2.1 5 1"))
        report = self.build()
        self.assertIn("| cli | 5 | 0.0% | not run | **0.0%** |", report)

    def test_writes_merged_profile_per_component(self):
        write(self.root, "unit/go/control-plane.out", profile(f"{P}control-plane/internal/a/a.go:1.1,2.1 4 0"))
        write(self.root, "e2e/go.out", profile(f"{P}control-plane/internal/a/a.go:1.1,2.1 4 7"))
        self.build()
        merged = (self.root / "report/go/control-plane.out").read_text()
        self.assertEqual(merged, f"mode: set\n{P}control-plane/internal/a/a.go:1.1,2.1 4 1\n")

    def test_frontend_and_bff_sections(self):
        write(self.root, "unit/frontend/bff/coverage-summary.json", json.dumps({
            "total": {"lines": {"total": 200, "pct": 86.3}, "branches": {"pct": 79.0}},
        }))
        write(self.root, "e2e/bff-summary.json", json.dumps({"lines": 200, "unit": 172, "e2e": 47, "both": 175}))
        report = self.build()
        self.assertIn("| bff | 200 | 86.3% | 79.0% |", report)
        self.assertIn("| 200 | 86.0% | 23.5% | **87.5%** |", report)

    def test_missing_inputs_say_not_run(self):
        report = self.build()
        self.assertIn("Frontend unit tests were not run.", report)
        self.assertIn("BFF e2e coverage was not collected.", report)


if __name__ == "__main__":
    unittest.main()
