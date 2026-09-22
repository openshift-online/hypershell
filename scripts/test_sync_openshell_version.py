import importlib.util
import tempfile
import unittest
from pathlib import Path

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


    def test_detects_stale_console_digest(self):
        go_src = (
            'const defaultConsoleImage = '
            '"quay.io/gkrumbach07/openshell-dashboard'
            '@sha256:' + 'b' * 64 + '"\n'
        )
        with tempfile.NamedTemporaryFile(
            mode="w", suffix=".go", delete=False
        ) as f:
            f.write(go_src)
            f.flush()
            path = Path(f.name)

        mismatches = MOD.stamp_console_image(
            path,
            "quay.io/gkrumbach07/openshell-dashboard",
            "sha256:" + "a" * 64,
            check_only=True,
        )
        path.unlink()
        self.assertEqual(len(mismatches), 1)
        self.assertIn("b" * 64, mismatches[0])

    def test_stamps_console_digest(self):
        go_src = (
            'const defaultConsoleImage = '
            '"quay.io/gkrumbach07/openshell-dashboard'
            '@sha256:' + 'b' * 64 + '"\n'
        )
        with tempfile.NamedTemporaryFile(
            mode="w", suffix=".go", delete=False
        ) as f:
            f.write(go_src)
            f.flush()
            path = Path(f.name)

        new_digest = "sha256:" + "a" * 64
        MOD.stamp_console_image(
            path,
            "quay.io/gkrumbach07/openshell-dashboard",
            new_digest,
            check_only=False,
        )
        content = path.read_text()
        path.unlink()
        self.assertIn("@" + new_digest, content)
        self.assertNotIn("b" * 64, content)

    def test_error_when_console_constant_missing(self):
        go_src = 'package gateway\n\nconst somethingElse = "foo"\n'
        with tempfile.NamedTemporaryFile(
            mode="w", suffix=".go", delete=False
        ) as f:
            f.write(go_src)
            f.flush()
            path = Path(f.name)

        with self.assertRaises(RuntimeError) as ctx:
            MOD.stamp_console_image(
                path,
                "quay.io/gkrumbach07/openshell-dashboard",
                "sha256:" + "a" * 64,
                check_only=True,
            )
        path.unlink()
        self.assertIn("defaultConsoleImage constant not found", str(ctx.exception))

    def test_no_mismatch_when_console_current(self):
        digest = "sha256:" + "a" * 64
        go_src = (
            'const defaultConsoleImage = '
            '"quay.io/gkrumbach07/openshell-dashboard'
            '@' + digest + '"\n'
        )
        with tempfile.NamedTemporaryFile(
            mode="w", suffix=".go", delete=False
        ) as f:
            f.write(go_src)
            f.flush()
            path = Path(f.name)

        mismatches = MOD.stamp_console_image(
            path,
            "quay.io/gkrumbach07/openshell-dashboard",
            digest,
            check_only=True,
        )
        path.unlink()
        self.assertEqual(mismatches, [])


if __name__ == "__main__":
    unittest.main()
