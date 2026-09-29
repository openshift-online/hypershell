#!/usr/bin/env python3
"""Render-only checks for the GitOps split; requires Kustomize v5 and PyYAML.

Run: python3 scripts/test_gitops_manifests.py
No Kind cluster, credentials, or live Kubernetes API is needed.
"""

import subprocess
import unittest
from pathlib import Path

import yaml

ROOT = Path(__file__).resolve().parents[1]


def render(path, *, legacy=False):
    command = ["kustomize", "build"]
    # Existing dev overlays reference individual files outside their root.
    # The new packages must work with default load restrictions.
    if legacy:
        command.append("--load-restrictor=LoadRestrictionsNone")
    command.append(str(ROOT / "deploy" / path))
    output = subprocess.check_output(command, text=True)
    resources = {}
    for resource in yaml.safe_load_all(output):
        if resource is None:
            continue
        api_version = resource["apiVersion"]
        group = api_version.split("/")[0] if "/" in api_version else ""
        metadata = resource["metadata"]
        key = (group, resource["kind"], metadata.get("namespace", ""), metadata["name"])
        if key in resources:
            raise ValueError(f"Duplicate resource: {key}")
        resources[key] = resource
    return resources


class GitOpsManifestTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.applications = render("gitops/applications")
        cls.platform = render("gitops/platform")
        cls.hub = render("hub")

    def test_application_package_contains_only_approved_namespaced_resources(self):
        # Fail closed: metadata.namespace alone does not prove resource scope.
        allowed = {
            ("v1", "ServiceAccount"),
            ("v1", "Service"),
            ("apps/v1", "Deployment"),
            ("route.openshift.io/v1", "Route"),
        }
        accounts = {"hypershell-api-server", "hypershell-web-console"}
        self.assertTrue(self.applications)
        for key, resource in self.applications.items():
            with self.subTest(resource=key):
                self.assertIn((resource["apiVersion"], resource["kind"]), allowed)
                self.assertEqual(resource["metadata"]["namespace"], "hypershell-system")
                if resource["kind"] in {"Deployment", "ServiceAccount"}:
                    self.assertIn(resource["metadata"]["name"], accounts)
                if resource["kind"] == "Deployment":
                    self.assertIn(resource["spec"]["template"]["spec"]["serviceAccountName"], accounts)
        self.assertIn(("apps", "Deployment", "hypershell-system", "hypershell-controller"), self.platform)
        self.assertIn(("", "ServiceAccount", "hypershell-system", "hypershell-controller"), self.platform)

    def test_packages_do_not_share_resources(self):
        self.assertFalse(self.applications.keys() & self.platform.keys())

    def test_packages_compose_the_existing_hub(self):
        self.assertEqual(self.hub, {**self.platform, **self.applications})

    def test_existing_entry_points_still_build(self):
        for path in ("base", "openshift", "ibm", "kind", "keycloak", "base/prometheus"):
            with self.subTest(path=path):
                self.assertTrue(render(path, legacy=True))


if __name__ == "__main__":
    unittest.main(verbosity=2)
