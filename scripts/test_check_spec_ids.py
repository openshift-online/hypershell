import importlib.util
import tempfile
import unittest
from pathlib import Path


SCRIPT_PATH = Path(__file__).with_name("check_spec_ids.py")
SPEC = importlib.util.spec_from_file_location("check_spec_ids", SCRIPT_PATH)
CHECKER = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(CHECKER)

REGISTRY = """# Spec ID Registry

| Prefix | File | Requirements |
|--------|------|--------------|
| `OSG-TLS` | `platform/tls.spec.md` | 2 |
| `OBS-GFT` | `platform/obs.spec.md` | 1 |
| `SDL-VIS` | `platform/vis.spec.md` | 1 |
"""

RENAME = """# Map

| Old ID | New ID | File |
|--------|--------|------|
| `GFT-01` | `OBS-GFT-01` | `platform/obs.spec.md` |
| `SDLC-VIS-12a` | `SDL-VIS-02` | `platform/vis.spec.md` |
| `SDLC-VIS-12` | `SDL-VIS-01` | `platform/vis.spec.md` |
"""

TLS = """# TLS

## Requirements

### Requirement: OSG-TLS-01 -- Renewal

Text.

#### Scenario: OSG-TLS-01.01 -- Renews

- GIVEN a
- THEN b

#### Scenario: OSG-TLS-01.02 -- Fails

- THEN c

### Requirement: OSG-TLS-02 -- Review

**Verification:** Inspection -- reviewed by hand

#### Scenario: OSG-TLS-02.01 -- Looks fine

- THEN d
"""

OBS = "# Obs\n\n### Requirement: OBS-GFT-01 -- Thing\n\n**Verification:** Run it and look.\n"
VIS = (
    "# Vis\n\n### Requirement: SDL-VIS-01 -- A\n\n### Requirement: SDL-VIS-02 -- B\n"
)


def base_tree():
    return {
        "specs/ids.md": REGISTRY,
        "specs/id-rename-map.md": RENAME,
        "specs/platform/tls.spec.md": TLS,
        "specs/platform/obs.spec.md": OBS,
        "specs/platform/vis.spec.md": VIS,
    }


def run_tree(tree):
    """Write the tree to a temp dir, load it like main() does, analyze it."""
    with tempfile.TemporaryDirectory() as tmp:
        root = Path(tmp)
        for name, text in tree.items():
            path = root / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text(text, encoding="utf-8")
        return CHECKER.analyze(CHECKER.load_files(root, sorted(tree)))


def messages(analysis):
    return [f"{p}:{n}: {m}" for p, n, m in analysis.errors]


class CheckTest(unittest.TestCase):
    def assertError(self, analysis, fragment):
        self.assertTrue(
            any(fragment in m for m in messages(analysis)),
            f"{fragment!r} not in {messages(analysis)}",
        )

    def test_clean_tree_has_no_errors(self):
        self.assertEqual([], messages(run_tree(base_tree())))

    # Rule 1
    def test_rule1_heading_without_id(self):
        tree = base_tree()
        tree["specs/platform/tls.spec.md"] += "\n### Requirement: No ID Here\n"
        self.assertError(run_tree(tree), "tls.spec.md:")
        self.assertError(run_tree(tree), "malformed heading")

    def test_rule1_missing_separator(self):
        tree = base_tree()
        tree["specs/platform/obs.spec.md"] += "\n### Requirement: OBS-GFT-02 Name\n"
        self.assertError(run_tree(tree), "malformed heading")

    def test_rule1_scenario_under_wrong_requirement(self):
        tree = base_tree()
        tree["specs/platform/tls.spec.md"] += "\n#### Scenario: OSG-TLS-01.03 -- Misplaced\n"
        self.assertError(run_tree(tree), "does not belong to requirement OSG-TLS-02")

    def test_rule1_headings_in_code_fence_ignored(self):
        tree = base_tree()
        tree["specs/platform/tls.spec.md"] += "\n```markdown\n### Requirement: broken\n```\n"
        self.assertEqual([], messages(run_tree(tree)))

    def test_rule1_legacy_heading_is_valid(self):
        tree = base_tree()
        tree["specs/platform/obs.spec.md"] += "\n### Legacy compatibility: OBS-GFT-02 -- Old\n"
        tree["specs/ids.md"] = REGISTRY
        self.assertEqual([], messages(run_tree(tree)))

    def test_rule1_prose_heading_is_not_a_requirement(self):
        tree = base_tree()
        tree["specs/platform/obs.spec.md"] += "\n## Requirement and Scenario IDs\n"
        self.assertEqual([], messages(run_tree(tree)))

    # Rule 2
    def test_rule2_duplicate_ids_name_every_location(self):
        tree = base_tree()
        tree["specs/platform/obs.spec.md"] += "\n### Requirement: OSG-TLS-01 -- Dup\n"
        analysis = run_tree(tree)
        self.assertError(analysis, "duplicate ID OSG-TLS-01 (specs/platform/obs.spec.md:7, specs/platform/tls.spec.md:5)")

    # Rule 3
    def test_rule3_unregistered_prefix(self):
        tree = base_tree()
        tree["specs/platform/obs.spec.md"] += "\n### Requirement: OBS-NEW-01 -- X\n"
        self.assertError(run_tree(tree), "prefix OBS-NEW is not registered")

    def test_rule3_prefix_owned_by_other_file(self):
        tree = base_tree()
        tree["specs/platform/obs.spec.md"] += "\n### Requirement: OSG-TLS-09 -- X\n"
        self.assertError(run_tree(tree), "registered to platform/tls.spec.md, not this file")

    def test_rule3_multi_file_prefix(self):
        tree = base_tree()
        tree["specs/ids.md"] += "| `OSG-TLS` | `platform/obs.spec.md` | 1 |\n"
        tree["specs/platform/obs.spec.md"] += "\n### Requirement: OSG-TLS-09 -- X\n"
        self.assertEqual([], [m for m in messages(run_tree(tree)) if "obs.spec.md" in m and "OSG-TLS-09" in m])

    # Rule 4
    def test_rule4_retired_id_reused(self):
        tree = base_tree()
        tree["specs/platform/tls.spec.md"] += "\n## Retired IDs\n\n- `OSG-TLS-07` replaced by none\n"
        tree["specs/platform/tls.spec.md"] = tree["specs/platform/tls.spec.md"].replace(
            "### Requirement: OSG-TLS-02 --", "### Requirement: OSG-TLS-07 --"
        ).replace("OSG-TLS-02.01", "OSG-TLS-07.01")
        self.assertError(run_tree(tree), "retired ID OSG-TLS-07 is reused")

    def test_rule4_replaced_by_note_is_not_retired(self):
        tree = base_tree()
        tree["specs/platform/tls.spec.md"] += (
            "\n## Retired IDs\n\n- `OSG-TLS-07`: replaced by OSG-TLS-02.\n\nProse about OSG-TLS-01.\n"
        )
        self.assertEqual([], messages(run_tree(tree)))

    def test_rule4_retired_id_may_be_cited(self):
        tree = base_tree()
        tree["specs/platform/tls.spec.md"] += "\n## Retired IDs\n\n- `OSG-TLS-07` gone\n"
        tree["docs/note.md"] = "See OSG-TLS-07 history.\n"
        self.assertEqual([], messages(run_tree(tree)))

    # Rule 5
    def test_rule5_unknown_citation(self):
        tree = base_tree()
        tree["docs/note.md"] = "ok OSG-TLS-01.02 but not OSG-TLS-55 or OSG-TLS-01.09\n"
        analysis = run_tree(tree)
        self.assertError(analysis, "docs/note.md:1: unknown ID OSG-TLS-55")
        self.assertError(analysis, "unknown ID OSG-TLS-01.09")
        self.assertEqual(2, len(messages(analysis)))

    def test_rule5_unregistered_prefix_ignored(self):
        tree = base_tree()
        tree["docs/note.md"] = "cipher AES-GCM-256 and ZZZ-YYY-01\n"
        self.assertEqual([], messages(run_tree(tree)))

    def test_rule5_exempt_files(self):
        tree = base_tree()
        tree["skills/plan/spec/SKILL.md"] = "example OSG-TLS-88\n"
        tree["specs/standards/platform/spec-identifiers.spec.md"] = "example OSG-TLS-89\n"
        self.assertEqual([], messages(run_tree(tree)))

    def test_rule5_boundaries(self):
        tree = base_tree()
        tree["docs/note.md"] = "xOSG-TLS-55 OSG-TLS-55x OSG-TLS-55-more _OSG-TLS-55\n"
        self.assertEqual([], messages(run_tree(tree)))

    # Rule 6
    def test_rule6_old_id_in_code(self):
        tree = base_tree()
        tree["pkg/a_test.go"] = "// see GFT-01 for details\n"
        self.assertError(run_tree(tree), "pkg/a_test.go:1: old ID GFT-01, now OBS-GFT-01")

    def test_rule6_new_id_containing_old_is_not_stale(self):
        tree = base_tree()
        tree["pkg/a.go"] = "// OBS-GFT-01 and OBS-GFT-01.00x\n"
        self.assertEqual([], [m for m in messages(run_tree(tree)) if "old ID" in m])

    def test_rule6_longest_match_first(self):
        tree = base_tree()
        tree["pkg/a.go"] = "// SDLC-VIS-12a then SDLC-VIS-12\n"
        found = [m for m in messages(run_tree(tree)) if "old ID" in m]
        self.assertEqual(2, len(found))
        self.assertIn("old ID SDLC-VIS-12, now SDL-VIS-01", found[0])
        self.assertIn("old ID SDLC-VIS-12a, now SDL-VIS-02", found[1])

    def test_rule6_inside_specs_and_allowed_files(self):
        tree = base_tree()
        tree["specs/platform/obs.spec.md"] += "\nSee GFT-01.\n"
        tree["specs/standards/platform/spec-identifiers.spec.md"] = "was GFT-01\n"
        found = [m for m in messages(run_tree(tree)) if "old ID" in m]
        self.assertEqual(1, len(found))
        self.assertIn("obs.spec.md", found[0])

    def test_rule6_reads_every_rename_table(self):
        tree = base_tree()
        tree["specs/id-rename-map.md"] += "\n## Later Renames\n\n| Old ID | New ID | File |\n|---|---|---|\n| `GSE-01` | `OSG-TLS-01` | `x` |\n"
        tree["pkg/a.go"] = "// GSE-01\n"
        self.assertError(run_tree(tree), "old ID GSE-01, now OSG-TLS-01")

    # Rule 7
    def test_rule7_valid_markers(self):
        tree = base_tree()
        tree["a_test.go"] = "// Verifies: OSG-TLS-01.01\nfunc TestX(t *testing.T) {}\n"
        tree["b.sh"] = "# Verifies: OSG-TLS-01, OBS-GFT-01\ntest_y() {\n}\n"
        self.assertEqual([], messages(run_tree(tree)))

    def test_rule7_malformed_marker(self):
        tree = base_tree()
        tree["a_test.go"] = "// Verifies: OSG-TLS-01 and more\n// Verifies: OSG-TLS-01,OBS-GFT-01\n//Verifies: OSG-TLS-01\n"
        found = [m for m in messages(run_tree(tree)) if "malformed marker" in m]
        self.assertEqual(3, len(found))

    def test_rule7_unknown_id_even_with_unregistered_prefix(self):
        tree = base_tree()
        tree["a.py"] = "# Verifies: ZZZ-YYY-01\n# Verifies: OSG-TLS-77\n"
        found = [m for m in messages(run_tree(tree)) if "Verifies marker" in m]
        self.assertEqual(2, len(found))

    def test_rule7_markdown_is_not_scanned_for_markers(self):
        tree = base_tree()
        tree["docs/a.md"] = "# Verifies: nonsense\n"
        self.assertEqual([], messages(run_tree(tree)))


class ReportTest(unittest.TestCase):
    def report(self):
        tree = base_tree()
        tree["a_test.go"] = (
            "package x\n\n// Verifies: OSG-TLS-01.01\nfunc TestRenew(t *testing.T) {\n}\n\n"
            "// Verifies: OSG-TLS-01.01, OBS-GFT-01\n\nt.Run(\"does thing\", func(t *testing.T) {\n"
        )
        return CHECKER.build_report(run_tree(tree))

    def test_totals(self):
        totals = self.report()["totals"]
        self.assertEqual(5, totals["requirements"])
        self.assertEqual(2, totals["requirements_covered"])
        self.assertEqual(1, totals["requirements_non_test"])
        self.assertEqual(2, totals["requirements_uncovered"])
        self.assertEqual(3, totals["scenarios"])
        self.assertEqual(1, totals["scenarios_covered"])
        self.assertEqual(1, totals["scenarios_non_test"])
        self.assertEqual(1, totals["scenarios_uncovered"])

    def test_requirement_covered_via_scenario(self):
        tls = self.report()["files"][1]
        req = tls["requirements"][0]
        self.assertTrue(req["covered"])
        self.assertEqual([], req["citations"])
        self.assertEqual(2, len(req["scenarios"][0]["citations"]))

    def test_citation_and_test_names(self):
        sc = self.report()["files"][1]["requirements"][0]["scenarios"][0]
        self.assertEqual(
            [
                {"location": "a_test.go:3", "test": "TestRenew"},
                {"location": "a_test.go:7", "test": "does thing"},
            ],
            sc["citations"],
        )

    def test_free_prose_verification_is_test(self):
        report = self.report()
        obs = report["files"][0]["requirements"][0]
        self.assertEqual("Test", obs["method"])
        tls = report["files"][1]["requirements"][1]
        self.assertEqual("Inspection", tls["method"])

    def test_uncovered_scenarios_listed(self):
        self.assertEqual(
            ["OSG-TLS-01.02"], [s["id"] for s in self.report()["uncovered_scenarios"]]
        )

    def test_markdown_has_no_em_dash(self):
        text = CHECKER.render_markdown(self.report())
        self.assertNotIn(chr(0x2014), text)
        self.assertIn("## Uncovered scenarios", text)

    def test_test_name_extraction(self):
        lines = ["// Verifies: X", "", "// note", "  it('works well', () => {", ""]
        self.assertEqual("works well", CHECKER.test_name(lines, 0))
        self.assertEqual("def_x", CHECKER.test_name(["# V", "x = 1 # def_x"], 0)[-5:])
        self.assertEqual("test_a", CHECKER.test_name(["# V", "    def test_a(self):"], 0))


if __name__ == "__main__":
    unittest.main()
