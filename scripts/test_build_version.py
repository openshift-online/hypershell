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
        result = self.run_script("local", HYPERSHELL_VCS_REF=REVISION)
        self.assertEqual(0, result.returncode, result.stderr)
        self.assertEqual("dev-abcdef0\n", result.stdout)

    def test_ci_version_uses_version_file(self):
        with tempfile.TemporaryDirectory() as temporary_directory:
            version_file = Path(temporary_directory) / "VERSION"
            version_file.write_text("1.6.0\n", encoding="utf-8")
            result = self.run_script(
                "ci",
                HYPERSHELL_VCS_REF=REVISION,
                HYPERSHELL_VERSION_FILE=str(version_file),
            )
        self.assertEqual(0, result.returncode, result.stderr)
        self.assertEqual("v1.6.0-abcdef0\n", result.stdout)

    def test_rejects_an_abbreviated_revision(self):
        result = self.run_script("local", HYPERSHELL_VCS_REF="abcdef0")
        self.assertNotEqual(0, result.returncode)
        self.assertIn("full 40-character Git SHA", result.stderr)

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


if __name__ == "__main__":
    unittest.main()
