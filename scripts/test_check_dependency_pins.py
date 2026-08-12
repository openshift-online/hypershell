import importlib.util
import unittest
from pathlib import Path


SCRIPT_PATH = Path(__file__).with_name("check_dependency_pins.py")
SPEC = importlib.util.spec_from_file_location("check_dependency_pins", SCRIPT_PATH)
CHECKER = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(CHECKER)


class DependencyPinTest(unittest.TestCase):
    def manifest_violations(self, image: str, pull_policy: str = "Always"):
        return CHECKER._manifest_violations(
            "components/example/deploy/example.yaml",
            [
                "      containers:",
                "        - name: example",
                f"          image: {image}",
                f"          imagePullPolicy: {pull_policy}",
            ],
        )

    def test_allows_openshift_internal_dev_image(self):
        self.assertEqual(
            [],
            self.manifest_violations(
                "image-registry.openshift-image-registry.svc:5000/"
                "hypershell-api/hypershell:dev"
            ),
        )

    def test_rejects_other_openshift_internal_tags(self):
        self.assertEqual(
            [
                (
                    "components/example/deploy/example.yaml",
                    3,
                    "container image lacks a sha256 digest",
                )
            ],
            self.manifest_violations(
                "image-registry.openshift-image-registry.svc:5000/"
                "hypershell-api/hypershell:latest"
            ),
        )

    def test_rejects_external_dev_image(self):
        self.assertEqual(
            [
                (
                    "components/example/deploy/example.yaml",
                    3,
                    "container image lacks a sha256 digest",
                )
            ],
            self.manifest_violations("quay.io/example/hypershell:dev"),
        )

    def test_allows_project_image(self):
        self.assertEqual(
            [],
            self.manifest_violations(
                "quay.io/redhat-services-prod/hcm-eng-prod-tenant/"
                "hypershell-main/hypershell-api-server-main:latest"
            ),
        )

    def test_allows_digest_pinned_external_image(self):
        self.assertEqual(
            [],
            self.manifest_violations(
                "registry.redhat.io/rhel9/postgresql-13@sha256:"
                "e98e67042d5b53f372231c1b9a2266afc4410664651d83c3224ab87ce3a2c4a9"
            ),
        )


if __name__ == "__main__":
    unittest.main()
