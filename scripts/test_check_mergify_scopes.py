import importlib.util
import unittest
from pathlib import Path


SCRIPT_PATH = Path(__file__).with_name("check_mergify_scopes.py")
SPEC = importlib.util.spec_from_file_location("check_mergify_scopes", SCRIPT_PATH)
mod = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(mod)


class GlobSemanticsTest(unittest.TestCase):
    def match(self, pattern: str, path: str) -> bool:
        return mod._compiled(pattern).match(path) is not None

    def test_single_star_stays_within_segment(self):
        # Unlike shell `[[ str == * ]]`, a single star must NOT cross '/'. This
        # is the fnmatch trap the issue warns about.
        self.assertTrue(self.match("scripts/gen-postgres-tls*", "scripts/gen-postgres-tls.sh"))
        self.assertFalse(self.match("scripts/*", "scripts/ci/deploy.sh"))
        self.assertTrue(self.match("scripts/*", "scripts/deploy.sh"))

    def test_doublestar_spans_segments(self):
        self.assertTrue(self.match("components/api-server/**", "components/api-server/internal/x.go"))
        self.assertTrue(self.match("components/api-server/**", "components/api-server/openapi/openapi.yaml"))
        self.assertFalse(self.match("components/api-server/**", "components/cli/main.go"))

    def test_leading_doublestar_matches_zero_or_more_segments(self):
        self.assertTrue(self.match("**/README.md", "README.md"))
        self.assertTrue(self.match("**/README.md", "components/api-server/README.md"))
        self.assertFalse(self.match("**/README.md", "components/api-server/READMExmd"))

    def test_star_md_at_any_depth(self):
        self.assertTrue(self.match("**/*.md", "README.md"))
        self.assertTrue(self.match("**/*.md", "docs/guide.md"))
        self.assertFalse(self.match("**/*.md", "docs/guide.txt"))

    def test_question_mark_single_char(self):
        self.assertTrue(self.match("file?.txt", "file1.txt"))
        self.assertFalse(self.match("file?.txt", "file12.txt"))
        self.assertFalse(self.match("file?.txt", "file/.txt"))

    def test_exact_literal(self):
        self.assertTrue(self.match("OPENSHELL_VERSION", "OPENSHELL_VERSION"))
        self.assertFalse(self.match("OPENSHELL_VERSION", "OPENSHELL_VERSION.bak"))


class ParseMergifyTest(unittest.TestCase):
    def test_parses_real_config(self):
        scopes, barriers = mod.parse_mergify(mod.MERGIFY_PATH.read_text(encoding="utf-8"))
        # All 13 CICD-MRG-03 consumer scopes must be present.
        expected = {
            "api-server", "cli", "control-plane", "sdk-typescript", "sdk-go",
            "web-console", "fleet-dashboard", "agents", "deployment",
            "test-infrastructure", "architecture-site", "documentation",
            "repository-tooling",
        }
        self.assertEqual(expected, set(scopes))
        for name, spec in scopes.items():
            self.assertTrue(spec["include"], f"{name} has no include patterns")
        self.assertIn(".mergify.yml", barriers)
        self.assertIn("Makefile", barriers)
        # commit_message_format sub-keys (title/body) must not be parsed as scopes.
        self.assertNotIn("title", scopes)
        self.assertNotIn("body", scopes)

    def test_expected_scopes_matches_real_config(self):
        # main() guards the parsed scope set against EXPECTED_SCOPES; keep that
        # constant in sync with the committed .mergify.yml (finding 3).
        scopes, _ = mod.parse_mergify(mod.MERGIFY_PATH.read_text(encoding="utf-8"))
        self.assertEqual(mod.EXPECTED_SCOPES, frozenset(scopes))

    def test_parses_reindented_block(self):
        # The parser reads indentation relative to the `files:` / `barrier_files:`
        # anchors, so shifting the whole scopes block by extra spaces must not
        # drop scopes (finding 3: no silent partial parse on reindent).
        text = (
            "scopes:\n"
            "      source:\n"
            "        files:\n"
            "          api-server:\n"
            "            include:\n"
            "              - components/api-server/**\n"
            "          cli:\n"
            "            include:\n"
            "              - components/cli/**\n"
            "      barrier_files:\n"
            "        include:\n"
            "          - .mergify.yml\n"
        )
        scopes, barriers = mod.parse_mergify(text)
        self.assertEqual({"api-server", "cli"}, set(scopes))
        self.assertEqual(["components/api-server/**"], scopes["api-server"]["include"])
        self.assertEqual([".mergify.yml"], barriers)

    def test_ignores_queue_rule_list_items(self):
        text = (
            "queue_rules:\n"
            "  - name: q\n"
            "    queue_conditions:\n"
            "      - base = main\n"
            "scopes:\n"
            "  source:\n"
            "    files:\n"
            "      api-server:\n"
            "        include:\n"
            "          - components/api-server/**\n"
            "        exclude:\n"
            "          - '**/README.md'\n"
            "  barrier_files:\n"
            "    include:\n"
            "      - .mergify.yml\n"
        )
        scopes, barriers = mod.parse_mergify(text)
        self.assertEqual({"api-server"}, set(scopes))
        self.assertEqual(["components/api-server/**"], scopes["api-server"]["include"])
        self.assertEqual(["**/README.md"], scopes["api-server"]["exclude"])
        self.assertEqual([".mergify.yml"], barriers)


class ScopeSelectionTest(unittest.TestCase):
    SCOPES = {
        "api-server": {
            "include": ["components/api-server/**"],
            "exclude": ["**/README.md"],
        },
        "documentation": {
            "include": ["**/README.md", "specs/**"],
            "exclude": ["specs/web-console/mockups/**"],
        },
        "web-console": {
            "include": ["specs/web-console/mockups/**"],
            "exclude": [],
        },
    }

    def test_exclude_routes_readme_away_from_runtime(self):
        self.assertEqual(
            {"documentation"},
            mod.scopes_for("components/api-server/README.md", self.SCOPES),
        )

    def test_runtime_file_selects_runtime_scope(self):
        self.assertEqual(
            {"api-server"},
            mod.scopes_for("components/api-server/main.go", self.SCOPES),
        )

    def test_mockup_is_web_console_not_documentation(self):
        self.assertEqual(
            {"web-console"},
            mod.scopes_for("specs/web-console/mockups/App.tsx", self.SCOPES),
        )


class CoverageAuditHelpersTest(unittest.TestCase):
    SCOPES = {
        "api-server": {"include": ["components/api-server/**"], "exclude": []},
    }
    BARRIERS = ["Makefile"]

    def test_is_classified(self):
        self.assertTrue(mod.is_classified("components/api-server/x.go", self.SCOPES, self.BARRIERS))
        self.assertTrue(mod.is_classified("Makefile", self.SCOPES, self.BARRIERS))
        self.assertFalse(mod.is_classified("components/new-service/x.go", self.SCOPES, self.BARRIERS))

    def test_allowlist_glob_rescues_tree(self):
        allowlist = [".hypershell/**"]
        self.assertTrue(mod.matches_any(".hypershell/agents/x/SKILL.md", allowlist))
        self.assertFalse(mod.matches_any("components/new-service/x.go", allowlist))


class SharedGroupDriftGuardTest(unittest.TestCase):
    def test_real_config_has_no_drift(self):
        # The committed .mergify.yml must satisfy every shared input group.
        scopes, _ = mod.parse_mergify(mod.MERGIFY_PATH.read_text(encoding="utf-8"))
        self.assertEqual([], mod.shared_group_errors(scopes))

    def test_missing_shared_pattern_in_one_member_is_flagged(self):
        # Drop a single shared deployment input from one member scope and confirm
        # the guard reports it (silent drift becomes a loud failure).
        scopes, _ = mod.parse_mergify(mod.MERGIFY_PATH.read_text(encoding="utf-8"))
        scopes["api-server"]["include"].remove("charts/**")
        errors = mod.shared_group_errors(scopes)
        self.assertTrue(errors)
        self.assertTrue(any("api-server" in e and "charts/**" in e for e in errors))

    def test_group_members_are_known_scopes(self):
        # Every scope named in a shared group must be a real scope.
        for members, _ in list(mod.SHARED_INCLUDE_GROUPS.values()) + list(
            mod.SHARED_EXCLUDE_GROUPS.values()
        ):
            self.assertLessEqual(members, mod.EXPECTED_SCOPES)


class LoadComponentPathsTest(unittest.TestCase):
    def test_skips_non_dict_top_level_values(self):
        # A future metadata key (e.g. "$schema": "...") must not raise on .get.
        import json
        import tempfile
        from pathlib import Path as _Path

        original = mod.COMPONENT_PATHS_PATH
        try:
            with tempfile.TemporaryDirectory() as tmp:
                path = _Path(tmp) / "component-paths.json"
                path.write_text(
                    json.dumps(
                        {"$schema": "https://example/schema.json", "cli": {"paths": ["x"]}}
                    ),
                    encoding="utf-8",
                )
                mod.COMPONENT_PATHS_PATH = path
                result = mod.load_component_paths()
            self.assertEqual({"cli": ["x"]}, result)
        finally:
            mod.COMPONENT_PATHS_PATH = original


class RealConfigIntegrationTest(unittest.TestCase):
    def test_check_passes_on_repository(self):
        # The full check (scenarios, detector alignment, coverage) must pass on
        # the committed configuration.
        self.assertEqual(0, mod.main())


if __name__ == "__main__":
    unittest.main()
