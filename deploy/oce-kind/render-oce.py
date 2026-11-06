#!/usr/bin/env python3
"""Render the existing OCE OpenShell profile for this HyperShell-owned gateway."""
import json
import os
from pathlib import Path
import subprocess

state = Path(os.environ["OCE_KIND_STATE"]).resolve()
oce = Path(os.environ["OCE_REPO"]).resolve()
gateway_namespace = os.environ["GATEWAY_NAMESPACE"]
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


images = json.loads((state / "images.json").read_text())
service = get("-n", gateway_namespace, "get", "service", "openshell-gateway")
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
