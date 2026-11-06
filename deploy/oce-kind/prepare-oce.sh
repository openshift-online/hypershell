#!/usr/bin/env bash
set -euo pipefail
umask 077
: "${OCE_KIND_STATE:?Set OCE_KIND_STATE}"
export KUBECONFIG="$OCE_KIND_STATE/kubeconfig"
[[ $(kubectl config current-context) == kind-hypershell-oce ]]
curl -fsSL https://github.com/envoyproxy/gateway/releases/download/v1.6.7/install.yaml -o "$OCE_KIND_STATE/envoy.yaml"
printf '%s  %s\n' 9a250c698d78b92c670d9d2bd6bd54615f1dee41ddd520ece9704edf63088df8 "$OCE_KIND_STATE/envoy.yaml" | sha256sum --check
# HyperShell owns the shared Gateway API CRDs. Install only Envoy's resources.
python3 - <<'PY'
import os, yaml
from pathlib import Path
state = Path(os.environ['OCE_KIND_STATE'])
resources = [r for r in yaml.safe_load_all((state/'envoy.yaml').read_text()) if r and not (
    r['kind']=='CustomResourceDefinition' and r['metadata']['name'].endswith('.gateway.networking.k8s.io'))]
(state/'envoy-filtered.yaml').write_text(yaml.safe_dump_all(resources))
PY
kubectl apply --server-side -f "$OCE_KIND_STATE/envoy-filtered.yaml"
kubectl -n envoy-gateway-system rollout status deployment/envoy-gateway --timeout=5m
kubectl create namespace oce-system --dry-run=client -o yaml | kubectl apply -f -
kubectl label node hypershell-oce-control-plane oce-role=control --overwrite
kubectl apply -f - <<'YAML'
apiVersion: gateway.networking.k8s.io/v1
kind: GatewayClass
metadata:
  name: eg
spec:
  controllerName: gateway.envoyproxy.io/gatewayclass-controller
---
apiVersion: node.k8s.io/v1
kind: RuntimeClass
metadata:
  name: openshell-sandbox
handler: runc
YAML
