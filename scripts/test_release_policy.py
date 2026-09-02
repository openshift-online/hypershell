import importlib.util
import json
from pathlib import Path
import tempfile
import unittest


SCRIPT_PATH = Path(__file__).with_name("release_policy.py")
SPEC = importlib.util.spec_from_file_location("release_policy", SCRIPT_PATH)
POLICY = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(POLICY)


class PullRequestTitleTest(unittest.TestCase):
    def test_accepts_supported_titles(self):
        for title in (
            "feat: add releases",
            "fix(api): return the version",
            "feat(web-console)!: change the menu",
            "feat(release): [HYPERSHELL-123] add releases",
            "chore(main): release 0.1.0",
        ):
            with self.subTest(title=title):
                self.assertIsNone(POLICY.title_error(title))

    def test_rejects_invalid_titles(self):
        for title in (
            "Add releases",
            "Feature: add releases",
            "feat(API): add releases",
            "[HYPERSHELL-123] feat(release): add releases",
            "feat(release): [HYPERSHELL-ABC] add releases",
            "feat: ",
            "feat add releases",
            "merge: add releases",
        ):
            with self.subTest(title=title):
                self.assertIsNotNone(POLICY.title_error(title))


class ReleaseGateTest(unittest.TestCase):
    def test_runs_for_release_changes(self):
        for message in (
            "feat: add releases",
            "fix(api): return the version",
            "refactor(api)!: change the response",
            "refactor(api): change the response\n\nBREAKING CHANGE: clients must update",
            "chore(main): release 1.2.3",
        ):
            with self.subTest(message=message):
                self.assertTrue(POLICY.is_releasing_commit(message))

    def test_does_not_run_for_maintenance_only(self):
        messages = ["docs: explain releases", "chore(deps): update a tool"]
        self.assertFalse(POLICY.should_run_release(False, messages))

    def test_updates_an_open_release_pull_request(self):
        self.assertTrue(POLICY.should_run_release(True, ["docs: explain releases"]))


class ReleaseFilesTest(unittest.TestCase):
    def setUp(self):
        self.temporary_directory = tempfile.TemporaryDirectory()
        self.root = Path(self.temporary_directory.name)
        (self.root / "VERSION").write_text("1.2.3\n", encoding="utf-8")
        (self.root / "build-version.env").write_text(
            "# x-release-please-start-version\n"
            "BUILD_PREFIX=v1.2.3\n"
            "# x-release-please-end\n",
            encoding="utf-8",
        )
        (self.root / ".release-please-manifest.json").write_text(
            json.dumps({".": "1.2.3"}), encoding="utf-8"
        )
        config = {
            "always-update": True,
            "bump-minor-pre-major": True,
            "bump-patch-for-minor-pre-major": False,
            "include-v-in-tag": True,
            "changelog-sections": [
                {"type": commit_type, "section": commit_type, "hidden": False}
                for commit_type in POLICY.ALLOWED_TYPES
            ],
            "packages": {
                ".": {
                    "release-type": "simple",
                    "version-file": "VERSION",
                    "extra-files": ["build-version.env"],
                }
            },
        }
        (self.root / "release-please-config.json").write_text(
            json.dumps(config), encoding="utf-8"
        )

    def tearDown(self):
        self.temporary_directory.cleanup()

    def test_accepts_consistent_files(self):
        self.assertEqual([], POLICY.release_file_errors(self.root))

    def test_rejects_a_build_prefix_mismatch(self):
        (self.root / "build-version.env").write_text(
            "BUILD_PREFIX=v1.2.2\n", encoding="utf-8"
        )
        self.assertIn(
            "build-version.env BUILD_PREFIX must equal v plus VERSION",
            POLICY.release_file_errors(self.root),
        )

    def test_rejects_a_manifest_mismatch(self):
        (self.root / ".release-please-manifest.json").write_text(
            json.dumps({".": "1.2.2"}), encoding="utf-8"
        )
        self.assertIn(
            "the release manifest root version must equal VERSION",
            POLICY.release_file_errors(self.root),
        )


if __name__ == "__main__":
    unittest.main()
