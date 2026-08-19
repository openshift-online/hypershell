import importlib.util
import unittest
from pathlib import Path


SCRIPT_PATH = Path(__file__).with_name("check_forbidden_terms.py")
SPEC = importlib.util.spec_from_file_location("check_forbidden_terms", SCRIPT_PATH)
CHECKER = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(CHECKER)


class ForbiddenTermsTest(unittest.TestCase):
    def test_rejects_em_dash(self):
        em_dash = chr(0x2014)

        self.assertEqual(
            {("docs/example.md", 2): {"em dash (U+2014)"}},
            CHECKER._find_matches(
                "docs/example.md", f"Allowed punctuation\nNot allowed {em_dash} here"
            ),
        )

    def test_allows_ascii_hyphen(self):
        self.assertEqual(
            {},
            CHECKER._find_matches(
                "docs/example.md", "Allowed punctuation - including a hyphen"
            ),
        )

    def test_excludes_generated_lockfile(self):
        self.assertFalse(CHECKER._scannable("pnpm-lock.yaml"))
        self.assertTrue(CHECKER._scannable("components/api-server/main.go"))

    def test_excludes_the_whitelist_itself(self):
        self.assertFalse(CHECKER._scannable(CHECKER.WHITELIST_PATH.name))


if __name__ == "__main__":
    unittest.main()
