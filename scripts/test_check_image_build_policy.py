import importlib.util
from pathlib import Path
import unittest


SCRIPT_PATH = Path(__file__).with_name("check_image_build_policy.py")
SPEC = importlib.util.spec_from_file_location("check_image_build_policy", SCRIPT_PATH)
POLICY = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(POLICY)


class ImageMetadataPolicyTest(unittest.TestCase):
    def test_accepts_reordered_assignments_and_different_quotes(self):
        trim = "?" * 33
        dockerfile = f"""
        # syntax=docker/dockerfile:1
        ENV HYPERSHELL_BUILD_REVISION=$VCS_REF \\
            HYPERSHELL_BUILD_VERSION='${{BUILD_PREFIX}}-${{VCS_REF%{trim}}}'
        LABEL org.opencontainers.image.revision='${{VCS_REF}}' \\
            org.opencontainers.image.version="${{BUILD_PREFIX}}-${{VCS_REF%{trim}}}"
        """

        self.assertEqual([], POLICY.image_metadata_errors("Dockerfile", dockerfile))

    def test_reports_missing_assignments(self):
        errors = POLICY.image_metadata_errors("Dockerfile", "FROM scratch\n")

        self.assertEqual(4, len(errors))
        self.assertIn("Dockerfile must set one OCI build version", errors)
        self.assertIn("Dockerfile must set one runtime build version", errors)
        self.assertIn("Dockerfile must set OCI revision from VCS_REF", errors)
        self.assertIn("Dockerfile must set runtime revision from VCS_REF", errors)

    def test_reports_an_invalid_short_revision(self):
        trim = "?" * 32
        dockerfile = f"""
        ENV HYPERSHELL_BUILD_VERSION="${{BUILD_PREFIX}}-${{VCS_REF%{trim}}}" \\
            HYPERSHELL_BUILD_REVISION="${{VCS_REF}}"
        LABEL org.opencontainers.image.version="${{BUILD_PREFIX}}-${{VCS_REF%{trim}}}" \\
            org.opencontainers.image.revision="${{VCS_REF}}"
        """

        errors = POLICY.image_metadata_errors("Dockerfile", dockerfile)

        self.assertEqual(2, len(errors))
        for error in errors:
            self.assertIn("must shorten the revision to seven characters", error)


if __name__ == "__main__":
    unittest.main()
