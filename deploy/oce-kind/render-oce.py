#!/usr/bin/env python3
"""Render the existing OCE OpenShell profile for this HyperShell-owned gateway."""
import json
import os
from pathlib import Path
import re
import subprocess
import tomllib

state = Path(os.environ["OCE_KIND_STATE"]).resolve()
oce = Path(os.environ["OCE_REPO"]).resolve()
gateway_namespace = os.environ.get("GATEWAY_NAMESPACE")
kube = ["kubectl", "--kubeconfig", str(state / "kubeconfig"), "--context", "kind-hypershell-oce"]


def get(*args):
    return json.loads(subprocess.check_output([*kube, *args, "-o", "json"]))


def write(name, value):
    (state / name).write_text(json.dumps(value, indent=2) + "\n")


def render(name, strings, objects):
    args = ["jq", "-n"]
    for key, value in strings.items():
        args.extend(["--arg", key, value])
    for key, value in objects.items():
        args.extend(["--argjson", key, json.dumps(value)])
    return json.loads(subprocess.check_output([
        *args, "-f", str(oce / "docs/snippets/openshell-quickstart/kubernetes" / name)
    ]))


def gateway_scope(value):
    if isinstance(value, dict):
        return {key: gateway_scope(item) for key, item in value.items()}
    if isinstance(value, list):
        return [gateway_scope(item) for item in value]
    return gateway_namespace if value == "oce-system" else value


if not gateway_namespace:
    gateways = get("get", "deployments", "-A", "--field-selector", "metadata.name=openshell-gateway")["items"]
    if len(gateways) != 1:
        raise SystemExit(f"Expected exactly one OpenShell gateway, found {len(gateways)}; complete gateway setup or set GATEWAY_NAMESPACE explicitly.")
    gateway_namespace = gateways[0]["metadata"]["namespace"]
if not re.fullmatch(r"openshell-[a-f0-9]{16}", gateway_namespace):
    raise SystemExit("GATEWAY_NAMESPACE must be a HyperShell-created openshell-<16 hex digits> namespace")

images = json.loads((state / "images.json").read_text())
service = get("-n", gateway_namespace, "get", "service", "openshell-gateway")
gateway_config = tomllib.loads(get("-n", gateway_namespace, "get", "configmap", "openshell-gateway-config")["data"]["gateway.toml"])
driver_config = gateway_config["openshell"]["drivers"]["kubernetes"]
sandbox_account = driver_config["service_account_name"]
# Admission must allow the same image references the gateway uses, including tags or digests.
runtime_policy = Path(__file__).with_name("runtime-admission.yaml").read_text()
runtime_policy = runtime_policy.replace("__GATEWAY_NAMESPACE__", gateway_namespace)
for placeholder, image in {
    "__RUNTIME_IMAGE__": images["runtime"],
    "__SUPERVISOR_IMAGE__": driver_config["supervisor_image"],
    "__SANDBOX_IMAGE__": driver_config["sandbox_runtime_image"],
}.items():
    # Image references are embedded in CEL string literals; reject empty or unsafe values.
    if not isinstance(image, str) or not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9._:/@-]*", image):
        raise ValueError(f"invalid admission image reference for {placeholder}")
    runtime_policy = runtime_policy.replace(placeholder, image)
(state / "runtime-admission.yaml").write_text(runtime_policy)
database = get("-n", "external-cloud-db", "get", "service", "postgres")
database_pods = get("-n", "external-cloud-db", "get", "pods", "-l", "app=postgres")
api = get("-n", "default", "get", "service", "kubernetes")
endpoints = get("-n", "default", "get", "endpointslices", "-l", "kubernetes.io/service-name=kubernetes")
api_addresses = sorted({address for item in endpoints["items"] for endpoint in item["endpoints"] for address in endpoint["addresses"]})
api_ports = {port["port"] for item in endpoints["items"] for port in item["ports"]}
assert len(api_ports) == 1, "expected one Kubernetes API backend port"
database_addresses = [database["spec"]["clusterIP"], *[pod["status"]["podIP"] for pod in database_pods["items"]]]
hostname = "occ-gateway-362888b35fce.envoy-gateway-system.svc.cluster.local"
installation = render("installation.jq", {
    "cluster": "hypershell-oce", "runtime": images["runtime"], "storageClass": "standard",
    "hostname": hostname, "openshellIP": service["spec"]["clusterIP"],
}, {"proxyCidrs": ["127.0.0.1/32"]})
installation["backend"][0]["configuration"]["endpoint"] = f"https://openshell-gateway.{gateway_namespace}.svc:8080"
compute = installation["drivers"]["compute"]["configuration"]
compute["network"]["providerHarness"]["namespace"] = gateway_namespace
compute["runtime"]["nodeSelector"] = {"oce-role": "control"}
sandbox = installation["drivers"]["sandbox"]["configuration"]
sandbox["gateway"] = gateway_scope(sandbox["gateway"])
sandbox["gateway"]["operatorNamespaceLabels"] = {"hypershell.io/gateway": gateway_namespace}
# Tenant resources must use the ServiceAccount selected by HyperShell's chart.
for resource in sandbox["gateway"]["operatorWorkspaceResources"]:
    if resource["kind"] == "ServiceAccount" and resource["metadata"]["name"] == "openshell-sandbox":
        resource["metadata"]["name"] = sandbox_account
write("installation.json", installation)
values = render("values.jq", {
    "cluster": "hypershell-oce", "controller": images["controller"],
    "email": "admin@oce.localhost", "hostname": hostname,
}, {
    "databaseCidrs": [ip + "/32" for ip in database_addresses],
    "apiCidrs": [ip + "/32" for ip in [api["spec"]["clusterIP"], *api_addresses]],
})
values["cluster"]["port"] = next(iter(api_ports))
write("values.json", values)
controller_selector = {
    "matchLabels": {"app.kubernetes.io/name": "openclaw-enterprise", "app.kubernetes.io/instance": "oce"},
    "matchExpressions": [{"key": "app.kubernetes.io/component", "operator": "In", "values": ["api", "worker"]}],
}
write("management-policy.json", {
    "apiVersion": "networking.k8s.io/v1", "kind": "NetworkPolicy",
    "metadata": {"name": "oce-openshell-management", "namespace": "oce-system"},
    "spec": {"podSelector": controller_selector, "policyTypes": ["Egress"], "egress": [{
        "to": [
            {"ipBlock": {"cidr": service["spec"]["clusterIP"] + "/32"}},
            {"namespaceSelector": {"matchLabels": {"kubernetes.io/metadata.name": gateway_namespace}},
             "podSelector": {"matchLabels": service["spec"]["selector"]}},
        ], "ports": [{"protocol": "TCP", "port": 8080}],
    }]},
})
write("gateway-ingress.json", {
    "apiVersion": "networking.k8s.io/v1", "kind": "NetworkPolicy",
    "metadata": {"name": "oce-management-client", "namespace": gateway_namespace},
    "spec": {"podSelector": {"matchLabels": service["spec"]["selector"]},
             "policyTypes": ["Ingress"], "ingress": [{
                 "from": [{"namespaceSelector": {"matchLabels": {"kubernetes.io/metadata.name": "oce-system"}},
                           "podSelector": controller_selector}],
                 "ports": [{"protocol": "TCP", "port": 8080}],
             }]},
})
(state / "gateway-namespace").write_text(gateway_namespace + "\n")
