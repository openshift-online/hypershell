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
        for path in ("base", "openshift", "kind", "keycloak", "base/prometheus"):
            with self.subTest(path=path):
                self.assertTrue(render(path, legacy=True))

    def test_controller_uses_recreate_in_every_entry_point(self):
        # openshell-gateway-helm-adoption.spec.md: at most one controller pod
        # may run helm against a gateway release, so a rollout must stop the
        # old pod first.
        key = ("apps", "Deployment", "hypershell-system", "hypershell-controller")
        rendered = {
            "gitops/platform": self.platform,
            "hub": self.hub,
            **{path: render(path, legacy=True) for path in ("base", "openshift", "kind")},
        }
        for path, resources in rendered.items():
            with self.subTest(path=path):
                spec = resources[key]["spec"]
                self.assertEqual(spec["replicas"], 1)
                self.assertEqual(spec.get("strategy"), {"type": "Recreate"})

    def test_openshift_keycloak_guard_is_restricted_and_uses_jwks_url(self):
        resources = render("openshift", legacy=True)
        for deployment_name in ("hypershell-api-server", "hypershell-web-console"):
            with self.subTest(deployment=deployment_name):
                deployment = resources[("apps", "Deployment", "hypershell-system", deployment_name)]
                init_container = next(
                    container
                    for container in deployment["spec"]["template"]["spec"]["initContainers"]
                    if container["name"] == "wait-for-keycloak"
                )
                self.assertEqual(
                    init_container["env"][0],
                    {
                        "name": "KEYCLOAK_ENDPOINT",
                        "valueFrom": {
                            "secretKeyRef": {
                                "name": "hypershell-api-config",
                                "key": "api-service.jwkCertUrl",
                            }
                        },
                    },
                )
                self.assertEqual(
                    init_container["securityContext"],
                    {
                        "allowPrivilegeEscalation": False,
                        "privileged": False,
                        "readOnlyRootFilesystem": True,
                        "capabilities": {"drop": ["ALL"]},
                        "runAsNonRoot": True,
                        "seccompProfile": {"type": "RuntimeDefault"},
                    },
                )


class ProberBaseTests(unittest.TestCase):
    """Outside prober Kustomize base (OBS-PRB-11)."""

    @classmethod
    def setUpClass(cls):
        cls.prober = render("prober")

    def test_base_renders_the_prober_resources(self):
        # OBS-PRB-11.01: the base renders a Deployment, ServiceAccount, ConfigMap,
        # and Service for the prober. OBS-PRB-11.03: the base sets no namespace, so
        # each instance hub's overlay places the prober in its instance namespace;
        # the rendered base therefore has an empty metadata.namespace.
        for key in (
            ("apps", "Deployment", "", "hypershell-prober"),
            ("", "ServiceAccount", "", "hypershell-prober"),
            ("", "ConfigMap", "", "hypershell-prober-config"),
            ("", "Service", "", "hypershell-prober"),
        ):
            with self.subTest(resource=key):
                self.assertIn(key, self.prober)

    def test_deployment_runs_the_control_plane_image_in_probe_mode(self):
        # OBS-PRB-10: the Deployment runs the control-plane image with the probe
        # subcommand.
        deployment = self.prober[("apps", "Deployment", "", "hypershell-prober")]
        container = deployment["spec"]["template"]["spec"]["containers"][0]
        self.assertEqual(container.get("args"), ["probe"])
        self.assertIn("hypershell-control-plane", container["image"])

    def test_deployment_uses_restricted_security_context(self):
        # OBS-PRB-11.02: runAsNonRoot and drop ALL capabilities.
        deployment = self.prober[("apps", "Deployment", "", "hypershell-prober")]
        pod = deployment["spec"]["template"]["spec"]
        self.assertTrue(pod["securityContext"]["runAsNonRoot"])
        container = pod["containers"][0]
        self.assertEqual(container["securityContext"]["capabilities"]["drop"], ["ALL"])
        self.assertFalse(container["securityContext"]["allowPrivilegeEscalation"])

    def test_deployment_mounts_projected_token_with_configurable_audience(self):
        # OBS-PRB-11.02: a projected service-account token volume with a
        # configurable audience for the Keycloak token exchange (OBS-PRB-06).
        deployment = self.prober[("apps", "Deployment", "", "hypershell-prober")]
        pod = deployment["spec"]["template"]["spec"]
        volume = next(v for v in pod["volumes"] if v["name"] == "prober-token")
        source = volume["projected"]["sources"][0]["serviceAccountToken"]
        self.assertIn("audience", source)
        self.assertTrue(source["audience"])

    def test_service_exposes_the_metrics_port(self):
        # OBS-PRB-07: a Service exposes the metrics port the hub Prometheus scrapes.
        service = self.prober[("", "Service", "", "hypershell-prober")]
        ports = service["spec"]["ports"]
        self.assertTrue(any(p["name"] == "metrics" for p in ports))

    def test_deployment_declares_readiness_and_liveness_probes(self):
        # OBS-PRB-11: readiness and liveness probes for the prober container.
        deployment = self.prober[("apps", "Deployment", "", "hypershell-prober")]
        container = deployment["spec"]["template"]["spec"]["containers"][0]
        self.assertIn("readinessProbe", container)
        self.assertIn("livenessProbe", container)


if __name__ == "__main__":
    unittest.main(verbosity=2)
