import contextlib
import importlib.util
import io
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

SCRIPT_PATH = Path(__file__).with_name("sync_openshell_version.py")
SPEC = importlib.util.spec_from_file_location("sync_openshell_version", SCRIPT_PATH)
MOD = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MOD)

CONTROLLER_YAML = """\
          env:
            - name: GATEWAY_IMAGE
              value: quay.io/opendatahub/odh-openshell-gateway:{tag}
            - name: GATEWAY_SUPERVISOR_IMAGE
              value: quay.io/opendatahub/odh-openshell-supervisor:{tag}
          ports:
"""

IBM_YAML = """\
                  - name: GATEWAY_IMAGE
                    value: image-registry.openshift-image-registry.svc:5000/openshift/openshell-gateway:{tag}
                  - name: GATEWAY_SUPERVISOR_IMAGE
                    value: image-registry.openshift-image-registry.svc:5000/openshift/openshell-supervisor:{tag}
"""


class SyncOpenshellVersionTest(unittest.TestCase):
    def test_detects_stale_tag(self):
        with tempfile.NamedTemporaryFile(
            mode="w", suffix=".yaml", delete=False
        ) as f:
            f.write(CONTROLLER_YAML.format(tag="v0.0.109-rhaiv.0"))
            f.flush()
            path = Path(f.name)

        mismatches = MOD.stamp_file(
            path,
            ["GATEWAY_IMAGE", "GATEWAY_SUPERVISOR_IMAGE"],
            "v0.0.116-rhaiv.6",
            check_only=True,
        )
        path.unlink()
        self.assertEqual(len(mismatches), 2)
        self.assertIn("v0.0.109-rhaiv.0", mismatches[0])

    def test_stamps_new_tag(self):
        with tempfile.NamedTemporaryFile(
            mode="w", suffix=".yaml", delete=False
        ) as f:
            f.write(CONTROLLER_YAML.format(tag="v0.0.109-rhaiv.0"))
            f.flush()
            path = Path(f.name)

        MOD.stamp_file(
            path,
            ["GATEWAY_IMAGE", "GATEWAY_SUPERVISOR_IMAGE"],
            "v0.0.116-rhaiv.6",
            check_only=False,
        )
        content = path.read_text()
        path.unlink()
        self.assertIn(":v0.0.116-rhaiv.6", content)
        self.assertNotIn(":v0.0.109-rhaiv.0", content)

    def test_no_mismatch_when_current(self):
        with tempfile.NamedTemporaryFile(
            mode="w", suffix=".yaml", delete=False
        ) as f:
            f.write(CONTROLLER_YAML.format(tag="v0.0.116-rhaiv.6"))
            f.flush()
            path = Path(f.name)

        mismatches = MOD.stamp_file(
            path,
            ["GATEWAY_IMAGE", "GATEWAY_SUPERVISOR_IMAGE"],
            "v0.0.116-rhaiv.6",
            check_only=True,
        )
        path.unlink()
        self.assertEqual(mismatches, [])

    def test_handles_different_registries(self):
        with tempfile.NamedTemporaryFile(
            mode="w", suffix=".yaml", delete=False
        ) as f:
            f.write(IBM_YAML.format(tag="v0.0.109-rhaiv.0"))
            f.flush()
            path = Path(f.name)

        MOD.stamp_file(
            path,
            ["GATEWAY_IMAGE", "GATEWAY_SUPERVISOR_IMAGE"],
            "v0.0.116-rhaiv.6",
            check_only=False,
        )
        content = path.read_text()
        path.unlink()
        self.assertIn("openshell-gateway:v0.0.116-rhaiv.6", content)
        self.assertIn("openshell-supervisor:v0.0.116-rhaiv.6", content)
        self.assertIn("image-registry.openshift-image-registry.svc:5000", content)

    def test_strips_stale_digest(self):
        yaml = (
            "            - name: GATEWAY_IMAGE\n"
            "              value: quay.io/example/gw:v0.0.109-rhaiv.0"
            "@sha256:" + "a" * 64 + "\n"
        )
        with tempfile.NamedTemporaryFile(
            mode="w", suffix=".yaml", delete=False
        ) as f:
            f.write(yaml)
            f.flush()
            path = Path(f.name)

        MOD.stamp_file(
            path, ["GATEWAY_IMAGE"], "v0.0.116-rhaiv.6", check_only=False
        )
        content = path.read_text()
        path.unlink()
        self.assertIn(":v0.0.116-rhaiv.6", content)
        self.assertNotIn("@sha256:", content)


    CAPABILITY_YAML = (
        "                  - name: HYPERSHELL_MANAGED_CLUSTER_CAPABILITIES\n"
        '                    value: "{value}"\n'
        "                  - name: OIDC_ISSUER\n"
    )

    def _write_capability(self, value: str) -> Path:
        with tempfile.NamedTemporaryFile(
            mode="w", suffix=".yaml", delete=False
        ) as f:
            f.write(self.CAPABILITY_YAML.format(value=value))
            f.flush()
            return Path(f.name)

    def test_capabilities_adds_missing_label(self):
        path = self._write_capability("provider/aws,network/public")
        MOD.stamp_capabilities(path, "v0.1.2-rhaiv.7", check_only=False)
        content = path.read_text()
        path.unlink()
        self.assertIn(
            'value: "provider/aws,network/public,gateway-version/v0.1.2-rhaiv.7"',
            content,
        )

    def test_capabilities_replaces_stale_version(self):
        path = self._write_capability(
            "provider/aws,gateway-version/v0.1.1-rhaiv.3,network/public"
        )
        mismatches = MOD.stamp_capabilities(
            path, "v0.1.2-rhaiv.7", check_only=False
        )
        content = path.read_text()
        path.unlink()
        self.assertEqual(len(mismatches), 1)
        self.assertIn("gateway-version/v0.1.2-rhaiv.7", content)
        self.assertNotIn("v0.1.1-rhaiv.3", content)
        # The stale label is replaced in place, not duplicated.
        self.assertEqual(content.count("gateway-version/"), 1)

    def test_capabilities_no_mismatch_when_current(self):
        path = self._write_capability(
            "provider/aws,network/public,gateway-version/v0.1.2-rhaiv.7"
        )
        mismatches = MOD.stamp_capabilities(
            path, "v0.1.2-rhaiv.7", check_only=True
        )
        path.unlink()
        self.assertEqual(mismatches, [])

    CONSOLE_REPO = "quay.io/gkrumbach07/openshell-dashboard"

    def _write_console_manifest(self, value_line):
        with tempfile.NamedTemporaryFile(
            mode="w", suffix=".yaml", delete=False
        ) as f:
            f.write(
                "          env:\n"
                "            - name: HYPERSHELL_CONSOLE_IMAGE\n"
                f"{value_line}\n"
                "            - name: OTHER\n"
                "              value: keep\n"
            )
            path = Path(f.name)
        self.addCleanup(path.unlink)
        return path

    def test_detects_stale_console_digest(self):
        path = self._write_console_manifest(
            f"              value: {self.CONSOLE_REPO}@sha256:{'b' * 64}"
        )
        mismatches = MOD.stamp_console_image(
            path, self.CONSOLE_REPO, "sha256:" + "a" * 64, check_only=True
        )
        self.assertEqual(len(mismatches), 1)
        self.assertIn("HYPERSHELL_CONSOLE_IMAGE", mismatches[0])
        self.assertIn("b" * 64, mismatches[0])
        # Check mode does not write.
        self.assertIn("b" * 64, path.read_text())

    def test_stamps_console_digest(self):
        path = self._write_console_manifest(
            f"              value: {self.CONSOLE_REPO}@sha256:{'b' * 64}"
        )
        new_digest = "sha256:" + "a" * 64
        MOD.stamp_console_image(
            path, self.CONSOLE_REPO, new_digest, check_only=False
        )
        content = path.read_text()
        self.assertIn(
            f"              value: {self.CONSOLE_REPO}@{new_digest}\n", content
        )
        self.assertNotIn("b" * 64, content)
        self.assertIn("value: keep", content)

    def test_stamps_mirrored_console_image(self):
        # A manifest pointing at another registry is moved to the pinned image.
        path = self._write_console_manifest(
            "              value: registry.example.com/dashboard:v0.0.9"
        )
        digest = "sha256:" + "a" * 64
        MOD.stamp_console_image(path, self.CONSOLE_REPO, digest, check_only=False)
        self.assertIn(f"value: {self.CONSOLE_REPO}@{digest}", path.read_text())

    def test_error_when_console_env_missing(self):
        with tempfile.NamedTemporaryFile(
            mode="w", suffix=".yaml", delete=False
        ) as f:
            f.write("          env:\n            - name: OTHER\n              value: x\n")
            path = Path(f.name)
        self.addCleanup(path.unlink)

        with self.assertRaises(RuntimeError) as ctx:
            MOD.stamp_console_image(
                path, self.CONSOLE_REPO, "sha256:" + "a" * 64, check_only=True
            )
        self.assertIn("HYPERSHELL_CONSOLE_IMAGE env var not found", str(ctx.exception))

    def test_error_when_console_value_missing(self):
        path = self._write_console_manifest("              valueFrom: {}")
        with self.assertRaises(RuntimeError) as ctx:
            MOD.stamp_console_image(
                path, self.CONSOLE_REPO, "sha256:" + "a" * 64, check_only=True
            )
        self.assertIn("has no value line", str(ctx.exception))

    def test_no_mismatch_when_console_current(self):
        digest = "sha256:" + "a" * 64
        path = self._write_console_manifest(
            f"              value: {self.CONSOLE_REPO}@{digest}"
        )
        mismatches = MOD.stamp_console_image(
            path, self.CONSOLE_REPO, digest, check_only=True
        )
        self.assertEqual(mismatches, [])


class SandboxImageSyncTest(unittest.TestCase):
    def setUp(self):
        directory = tempfile.TemporaryDirectory()
        self.addCleanup(directory.cleanup)
        self.values = Path(directory.name) / "values.yaml"
        self.version = Path(directory.name) / "OPENSHELL_VERSION"
        self.directory = Path(directory.name)

    CONSOLE_PINS = (
        "OPENSHELL_CONSOLE_IMAGE=example/dashboard\n"
        "OPENSHELL_CONSOLE_DIGEST=sha256:" + "c" * 64 + "\n"
    )

    def _console_manifest(self):
        path = self.directory / "controller.yaml"
        path.write_text(
            "            - name: HYPERSHELL_CONSOLE_IMAGE\n"
            "              value: example/dashboard@sha256:" + "c" * 64 + "\n"
        )
        return path

    def test_chart_image_resolution(self):
        for fields, expected in (
            ('repository: nvcr.io/example/workload\n    tag: "24.04"\n    digest: ""',
             "nvcr.io/example/workload:24.04"),
            ('repository: "registry.example:5000/workload"\n    tag: ""',
             "registry.example:5000/workload:latest"),
            ('repository: example/workload\n    tag: null\n    digest: ~',
             "example/workload:latest"),
            ("repository: 'example/workload'\n    tag: ignored\n    digest: 'sha256:abc' # pinned",
             "example/workload@sha256:abc"),
        ):
            with self.subTest(expected=expected):
                self.values.write_text(f"sandbox:\n  image:\n    {fields}\nother: true\n")
                self.assertEqual(MOD.chart_sandbox_image(self.values), expected)

    def test_schema_drift_fails_instead_of_retaining_old_image(self):
        for values in (
            "sandbox:\n  workloadImage: example/renamed:tag\n",
            'sandbox:\n  image:\n    repository: ""\n',
            "sandbox:\n  image:\n    repository: *sharedImage\n",
            "sandbox:\n  image:\n    registry: new.example\n    repository: workload\n",
        ):
            with self.subTest(values=values):
                self.values.write_text(values)
                with self.assertRaises(ValueError):
                    MOD.chart_sandbox_image(self.values)

    def test_check_reports_drift_and_stamp_repairs_only_workload_pin(self):
        self.values.write_text('sandbox:\n  image:\n    repository: example/new\n    tag: "2"\n')
        for original in (
            "OPENSHELL_TAG=v1.2.3\nOPENSHELL_SANDBOX_IMAGE=example/old:1\n",
            "OPENSHELL_TAG=v1.2.3\n",
        ):
            with self.subTest(original=original):
                self.version.write_text(original)
                mismatches = MOD.sync_sandbox_image(self.version, self.values, check_only=True)
                self.assertEqual(len(mismatches), 1)
                self.assertEqual(self.version.read_text(), original)
                MOD.sync_sandbox_image(self.version, self.values, check_only=False)
                self.assertEqual(
                    self.version.read_text(),
                    "OPENSHELL_TAG=v1.2.3\nOPENSHELL_SANDBOX_IMAGE=example/new:2\n",
                )
                self.assertEqual(
                    MOD.sync_sandbox_image(self.version, self.values, check_only=True), []
                )

    def test_main_check_fails_on_workload_drift(self):
        self.values.write_text('sandbox:\n  image:\n    repository: example/new\n    tag: "2"\n')
        original = (
            "OPENSHELL_TAG=v1.2.3\nOPENSHELL_SANDBOX_IMAGE=example/old:1\n"
            + self.CONSOLE_PINS
        )
        self.version.write_text(original)
        output = io.StringIO()
        with patch.multiple(
            MOD, VERSION_FILE=self.version, CHART_VALUES_FILE=self.values,
            MANAGED_FILES=[], CAPABILITY_FILES=[],
            CONSOLE_IMAGE_FILE=self._console_manifest(),
        ), patch.object(MOD.sys, "argv", ["sync_openshell_version.py"]), contextlib.redirect_stdout(output):
            self.assertEqual(MOD.main(), 1)
        self.assertIn("OPENSHELL_SANDBOX_IMAGE=example/new:2", output.getvalue())
        self.assertEqual(self.version.read_text(), original)

    def test_main_fails_without_console_pins(self):
        self.values.write_text('sandbox:\n  image:\n    repository: example/new\n    tag: "2"\n')
        self.version.write_text("OPENSHELL_TAG=v1.2.3\nOPENSHELL_SANDBOX_IMAGE=example/new:2\n")
        output = io.StringIO()
        with patch.multiple(
            MOD, VERSION_FILE=self.version, CHART_VALUES_FILE=self.values,
            MANAGED_FILES=[], CAPABILITY_FILES=[],
            CONSOLE_IMAGE_FILE=self._console_manifest(),
        ), patch.object(MOD.sys, "argv", ["sync_openshell_version.py"]), contextlib.redirect_stdout(output):
            self.assertEqual(MOD.main(), 1)
        self.assertIn("OPENSHELL_CONSOLE_DIGEST must", output.getvalue())


if __name__ == "__main__":
    unittest.main()
