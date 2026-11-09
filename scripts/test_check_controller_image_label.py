"""Inspect controller image configuration and verify the release label.

These tests feed representative image *configuration* (OCI config Labels, as
`skopeo inspect --config` would report them), not Dockerfile text, and confirm
the single supported OpenShell release contract across architecture variants
(specs/platform/openshell-image-auto-update.spec.md, CICD-IMG-04, CICD-IMG-05).
"""
import importlib.util
import json
import unittest
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import patch

SCRIPT_PATH = Path(__file__).with_name("check_controller_image_label.py")
SPEC = importlib.util.spec_from_file_location(
    "check_controller_image_label", SCRIPT_PATH
)
MOD = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MOD)

TAG = "v0.1.6-rhaiv.4"


def labels(release=TAG, version="0.0.1", **extra):
    out = {}
    if release is not None:
        out[MOD.RELEASE_LABEL] = release
    if version is not None:
        out[MOD.CONTROLLER_VERSION_LABEL] = version
    out.update(extra)
    return out


class ReleaseLabelValueTest(unittest.TestCase):
    # Verifies: CICD-IMG-01.01
    def test_accepts_single_identifier(self):
        MOD.validate_release_value(TAG)  # no raise

    def test_rejects_missing(self):
        with self.assertRaises(MOD.LabelError):
            MOD.validate_release_value(None)

    def test_rejects_empty(self):
        with self.assertRaises(MOD.LabelError):
            MOD.validate_release_value("")

    # Verifies: CICD-IMG-01.01
    def test_rejects_list_range_and_wildcard(self):
        for bad in ("v1,v2", "v1 v2", ">=v1.0", "v0.1.*", "v1||v2", "v1 - v2"):
            with self.subTest(value=bad), self.assertRaises(MOD.LabelError):
                MOD.validate_release_value(bad)


class ImageConfigInspectionTest(unittest.TestCase):
    # Verifies: CICD-IMG-05.03
    def test_consistent_single_value_passes(self):
        configs = {"amd64": labels(), "arm64": labels()}
        MOD.validate_image_configs(configs, TAG)  # no raise

    # Verifies: CICD-IMG-05.01
    def test_missing_label_fails(self):
        configs = {"amd64": labels(release=None)}
        with self.assertRaises(MOD.LabelError):
            MOD.validate_image_configs(configs, TAG)

    # Verifies: CICD-IMG-05.02
    def test_value_disagreeing_with_expected_fails(self):
        configs = {"amd64": labels(release="v0.0.1-rhaiv.0")}
        with self.assertRaises(MOD.LabelError):
            MOD.validate_image_configs(configs, TAG)

    # Verifies: CICD-IMG-04.01
    def test_architectures_must_agree(self):
        configs = {
            "amd64": labels(),
            "arm64": labels(release="v0.1.5-rhaiv.0"),
        }
        with self.assertRaises(MOD.LabelError):
            MOD.validate_image_configs(configs, TAG)

    # Verifies: CICD-IMG-03.01
    def test_controller_version_must_be_present_and_distinct(self):
        with self.assertRaises(MOD.LabelError):
            MOD.validate_image_configs({"amd64": labels(version=None)}, TAG)
        with self.assertRaises(MOD.LabelError):
            MOD.validate_image_configs({"amd64": labels(version=TAG)}, TAG)

    def test_no_variants_fails(self):
        with self.assertRaises(MOD.LabelError):
            MOD.validate_image_configs({}, TAG)


def fake_skopeo(raw_manifests, per_arch_config):
    """A stand-in for `skopeo inspect` over a multi-arch image.

    raw_manifests: the manifest-list platforms as `skopeo inspect --raw` reports.
    per_arch_config: arch -> OCI image config dict (as `--config` reports).
    """
    index = {
        "manifests": [
            {"platform": {"os": "linux", "architecture": arch}}
            for arch in raw_manifests
        ]
    }

    def run(*args):
        if "--raw" in args:
            return json.dumps(index)
        # The last arg is docker://<ref>; the arch is the --override-arch value.
        arch = args[args.index("--override-arch") + 1]
        return json.dumps(per_arch_config[arch])

    return run


class SkopeoInspectionPathTest(unittest.TestCase):
    """Drive the real inspection/parsing code over skopeo-shaped output."""

    # Verifies: CICD-IMG-05.03
    def test_multiarch_consistent_image_passes(self):
        config = {"config": {"Labels": labels()}}
        runner = fake_skopeo(["amd64", "arm64"], {"amd64": config, "arm64": config})
        per_arch = MOD.fetch_per_arch_labels("example/controller:tag", runner)
        self.assertEqual(set(per_arch), {"amd64", "arm64"})
        MOD.validate_image_configs(per_arch, TAG)  # no raise

    # Verifies: CICD-IMG-04.01
    def test_multiarch_divergent_image_fails(self):
        runner = fake_skopeo(
            ["amd64", "arm64"],
            {
                "amd64": {"config": {"Labels": labels()}},
                "arm64": {"config": {"Labels": labels(release="v0.1.5-rhaiv.0")}},
            },
        )
        per_arch = MOD.fetch_per_arch_labels("example/controller:tag", runner)
        with self.assertRaises(MOD.LabelError):
            MOD.validate_image_configs(per_arch, TAG)

    # Verifies: CICD-IMG-05.01
    def test_image_without_label_fails(self):
        runner = fake_skopeo(
            ["amd64"], {"amd64": {"config": {"Labels": labels(release=None)}}}
        )
        per_arch = MOD.fetch_per_arch_labels("example/controller:tag", runner)
        with self.assertRaises(MOD.LabelError):
            MOD.validate_image_configs(per_arch, TAG)

    def test_single_arch_fallback(self):
        def run(*args):
            return json.dumps(
                {"architecture": "amd64", "config": {"Labels": labels()}}
            )

        per_arch = MOD.fetch_per_arch_labels("example/controller:tag", run)
        self.assertEqual(set(per_arch), {"amd64"})
        MOD.validate_image_configs(per_arch, TAG)  # no raise

    # Verifies: CICD-IMG-05.03
    def test_local_engine_inspection(self):
        # podman/docker inspect: Labels as JSON, Architecture as a bare string.
        outputs = [json.dumps(labels()), "arm64\n"]

        def fake_run(cmd, **kwargs):
            return SimpleNamespace(stdout=outputs.pop(0))

        with patch.object(MOD.subprocess, "run", side_effect=fake_run):
            per_arch = MOD.fetch_local_labels("localhost/controller:dev", "podman")
        self.assertEqual(set(per_arch), {"arm64"})
        MOD.validate_image_configs(per_arch, TAG)  # no raise


class DockerfileWiringTest(unittest.TestCase):
    # Verifies: CICD-IMG-02.01, CICD-IMG-03.01
    def test_real_controller_dockerfile_is_wired(self):
        MOD.validate_dockerfile_wiring(MOD.CONTROLLER_DOCKERFILE.read_text())

    def test_hardcoded_release_is_rejected(self):
        text = f'LABEL {MOD.RELEASE_LABEL}="{TAG}"\nARG {MOD.BUILD_ARG}\n'
        with self.assertRaises(MOD.LabelError):
            MOD.validate_dockerfile_wiring(text)

    def test_missing_arg_is_rejected(self):
        text = f'LABEL {MOD.RELEASE_LABEL}="${{{MOD.BUILD_ARG}}}"\n'
        with self.assertRaises(MOD.LabelError):
            MOD.validate_dockerfile_wiring(text)


if __name__ == "__main__":
    unittest.main()
