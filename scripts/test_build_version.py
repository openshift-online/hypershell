import os
from pathlib import Path
import subprocess
import tempfile
import unittest


SCRIPT_PATH = Path(__file__).with_name("build-version.sh")
REVISION = "abcdef0123456789abcdef0123456789abcdef01"


class BuildVersionTest(unittest.TestCase):
    def run_script(self, mode: str, **environment: str):
        command_environment = os.environ.copy()
        command_environment.pop("HYPERSHELL_GIT_DIRTY", None)
        command_environment.pop("HYPERSHELL_GIT_WORK_TREE", None)
        command_environment.update(environment)
        return subprocess.run(
            ("bash", str(SCRIPT_PATH), mode),
            check=False,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
            env=command_environment,
        )

    def test_local_version_uses_dev_prefix(self):
        result = self.run_script(
            "local", HYPERSHELL_GIT_DIRTY="0", HYPERSHELL_VCS_REF=REVISION
        )
        self.assertEqual(0, result.returncode, result.stderr)
        self.assertEqual("dev-abcdef0\n", result.stdout)

    def test_local_version_marks_all_work_tree_change_types(self):
        for change_type in ("staged", "unstaged", "untracked"):
            with self.subTest(change_type=change_type):
                with tempfile.TemporaryDirectory() as temporary_directory:
                    work_tree = Path(temporary_directory)
                    self.run_git(work_tree, "init", "--quiet")
                    tracked_file = work_tree / "tracked.txt"
                    tracked_file.write_text("original\n", encoding="utf-8")
                    self.run_git(work_tree, "add", "tracked.txt")
                    self.run_git(
                        work_tree,
                        "-c",
                        "user.name=Build Test",
                        "-c",
                        "user.email=build-test@example.test",
                        "commit",
                        "--quiet",
                        "-m",
                        "initial",
                    )
                    revision = self.run_git(
                        work_tree, "rev-parse", "HEAD"
                    ).stdout.strip()
                    clean_result = self.run_script(
                        "local",
                        HYPERSHELL_GIT_WORK_TREE=str(work_tree),
                        HYPERSHELL_VCS_REF=revision,
                    )
                    self.assertEqual(
                        0, clean_result.returncode, clean_result.stderr
                    )
                    self.assertEqual(
                        f"dev-{revision[:7]}\n", clean_result.stdout
                    )

                    if change_type == "untracked":
                        (work_tree / "untracked.txt").write_text(
                            "new\n", encoding="utf-8"
                        )
                    else:
                        tracked_file.write_text("changed\n", encoding="utf-8")
                        if change_type == "staged":
                            self.run_git(work_tree, "add", "tracked.txt")

                    result = self.run_script(
                        "local",
                        HYPERSHELL_GIT_WORK_TREE=str(work_tree),
                        HYPERSHELL_VCS_REF=revision,
                    )

                self.assertEqual(0, result.returncode, result.stderr)
                self.assertEqual(f"dev-{revision[:7]}-modified\n", result.stdout)

    def test_ci_version_uses_version_file(self):
        with tempfile.TemporaryDirectory() as temporary_directory:
            version_file = Path(temporary_directory) / "VERSION"
            version_file.write_text("1.6.0\n", encoding="utf-8")
            result = self.run_script(
                "ci",
                HYPERSHELL_GIT_DIRTY="1",
                HYPERSHELL_VCS_REF=REVISION,
                HYPERSHELL_VERSION_FILE=str(version_file),
            )
        self.assertEqual(0, result.returncode, result.stderr)
        self.assertEqual("v1.6.0-abcdef0\n", result.stdout)

    def test_rejects_an_abbreviated_revision(self):
        result = self.run_script(
            "local", HYPERSHELL_GIT_DIRTY="0", HYPERSHELL_VCS_REF="abcdef0"
        )
        self.assertNotEqual(0, result.returncode)
        self.assertIn("full 40-character Git SHA", result.stderr)

    def test_rejects_an_invalid_dirty_state(self):
        result = self.run_script(
            "local", HYPERSHELL_GIT_DIRTY="yes", HYPERSHELL_VCS_REF=REVISION
        )
        self.assertNotEqual(0, result.returncode)
        self.assertIn("HYPERSHELL_GIT_DIRTY must be 0 or 1", result.stderr)

    def test_rejects_an_invalid_ci_version(self):
        with tempfile.TemporaryDirectory() as temporary_directory:
            version_file = Path(temporary_directory) / "VERSION"
            version_file.write_text("v1.6.0\n", encoding="utf-8")
            result = self.run_script(
                "ci",
                HYPERSHELL_VCS_REF=REVISION,
                HYPERSHELL_VERSION_FILE=str(version_file),
            )
        self.assertNotEqual(0, result.returncode)
        self.assertIn("stable semantic version", result.stderr)

    @staticmethod
    def run_git(work_tree: Path, *arguments: str):
        return subprocess.run(
            ("git", "-C", str(work_tree), *arguments),
            check=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
        )


if __name__ == "__main__":
    unittest.main()
