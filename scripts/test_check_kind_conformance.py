import importlib.util
import tempfile
import unittest
from pathlib import Path


SCRIPT_PATH = Path(__file__).with_name("check_kind_conformance.py")
SPEC = importlib.util.spec_from_file_location("check_kind_conformance", SCRIPT_PATH)
CHECKER = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(CHECKER)


class AutoMigrateScanTest(unittest.TestCase):
    def _with_plugins(self, files: dict[str, str]):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        for rel, content in files.items():
            path = root / rel
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text(content, encoding="utf-8")
        CHECKER.PLUGINS = root
        return root

    def test_flags_automigrate_call(self):
        self._with_plugins(
            {"gateways/migration.go": "func m(tx *gorm.DB) error {\n\treturn tx.AutoMigrate(&Gateway{})\n}\n"}
        )
        errors = CHECKER.check_no_automigrate()
        self.assertEqual(1, len(errors), errors)
        self.assertIn("gateways", errors[0])
        self.assertIn("AutoMigrate", errors[0])

    def test_ignores_automigrate_in_comment(self):
        self._with_plugins(
            {"ciProviders/migration.go": "// AutoMigrate's add-column path is unsafe.\n\treturn dbmigrate.CreateTable(tx, &X{})\n"}
        )
        self.assertEqual([], CHECKER.check_no_automigrate())

    def test_scans_secondary_migration_files(self):
        self._with_plugins(
            {"users/activity_migration.go": "\ttx.AutoMigrate(&UserDailyActivity{})\n"}
        )
        self.assertEqual(1, len(CHECKER.check_no_automigrate()))


class ParsingTest(unittest.TestCase):
    def test_parses_extkinds_allowlist(self):
        tmp = tempfile.NamedTemporaryFile("w", suffix=".go", delete=False)
        tmp.write(
            'var extKinds = map[string]bool{\n'
            '\t"ci_providers":     true,\n'
            '\t"secret_sources":   true,\n'
            '}\n'
        )
        tmp.close()
        self.addCleanup(lambda: Path(tmp.name).unlink())
        CHECKER.AUTHZ_GO = Path(tmp.name)
        self.assertEqual({"ci_providers", "secret_sources"}, CHECKER.ext_kinds_allowlist())

    def test_parses_conformance_fixtures(self):
        tmp = tempfile.NamedTemporaryFile("w", suffix=".go", delete=False)
        tmp.write(
            "var crudFixtures = map[string]func(t *testing.T, e *conformanceEnv) string{\n"
            '\t"ci_providers": func(t *testing.T, e *conformanceEnv) string {\n'
            "\t\treturn \"\"\n\t},\n"
            '\t"ci_trusts": func(t *testing.T, e *conformanceEnv) string {\n'
            "\t\treturn \"\"\n\t},\n}\n"
        )
        tmp.close()
        self.addCleanup(lambda: Path(tmp.name).unlink())
        CHECKER.CONFORMANCE_TEST = Path(tmp.name)
        self.assertEqual({"ci_providers", "ci_trusts"}, CHECKER.conformance_fixtures())

    def test_register_path_regex_ignores_registration_key(self):
        text = 'RegisterPrefixedRoutes("agentRuntimes", "ext", func() {})\n' \
               'presenters.RegisterPath(AgentRuntime{}, "agent_runtimes")\n'
        self.assertEqual(["agent_runtimes"], CHECKER.REGISTER_PATH.findall(text))

    def test_ext_registration_regex_matches(self):
        self.assertTrue(CHECKER.EXT_REGISTRATION.search('RegisterPrefixedRoutes("rbac", "ext", func'))
        self.assertFalse(CHECKER.EXT_REGISTRATION.search('RegisterPrefixedRoutes("gateways", "v1", func'))


if __name__ == "__main__":
    unittest.main()
