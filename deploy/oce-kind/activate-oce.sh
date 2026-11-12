#!/usr/bin/env bash
set -euo pipefail
umask 077
: "${OCE_REPO:?Set OCE_REPO}"
: "${OCE_KIND_STATE:?Set OCE_KIND_STATE}"
RUN_DIR="$OCE_KIND_STATE"
export KUBECONFIG="$RUN_DIR/kubeconfig" CONTEXT=kind-hypershell-oce
PLATFORM_NS=oce-system RELEASE=oce
OCE_CHART="$OCE_REPO/deploy/helm/openclaw-enterprise"
INSTALLATION_FILE="$RUN_DIR/installation.json" VALUES_FILE="$RUN_DIR/values.json"
OCC_SERVICE_KEY_FILE="$RUN_DIR/initial-admin-service-key.json"
assets="$OCE_REPO/docs/snippets/openshell-quickstart/kubernetes"
read -r GATEWAY_NAMESPACE < "$RUN_DIR/gateway-namespace"
cp "$OCE_REPO/docs/snippets/openshell-quickstart/request.mjs" "$RUN_DIR/request.mjs"
source "$OCE_REPO/docs/snippets/openshell-quickstart/operator-session.sh"
export ENVOY_IMAGE=docker.io/envoyproxy/envoy:distroless-v1.36.6@sha256:947974108bb93df64469fedaa0ab7191cdf86c608217d6ff9b497c4e96ff1069
kube -n "$PLATFORM_NS" patch envoyproxy oce-agent-gateways --type=merge \
  -p "$(jq -nc --arg image "$ENVOY_IMAGE" '{spec:{provider:{type:"Kubernetes",kubernetes:{envoyDeployment:{container:{image:$image}}}}}}')"
kube -n "$PLATFORM_NS" wait gateway/oce-agent-gateways --for=condition=Programmed --timeout=5m
kube -n envoy-gateway-system wait --for=create service/occ-gateway-362888b35fce --timeout=3m
# Wait for the pinned proxy rollout before recording its trusted addresses.
for attempt in {1..60}; do
  if bash "$assets/trusted-envoy-peers.sh" "$KUBECONFIG" > "$RUN_DIR/trusted-peers.json" 2> "$RUN_DIR/envoy-readiness.log"; then break; fi
  [[ $attempt != 60 ]] || { cat "$RUN_DIR/envoy-readiness.log" >&2; exit 1; }
  sleep 3
done
jq --slurpfile peers "$RUN_DIR/trusted-peers.json" \
  '.drivers.compute.configuration.network.gatewayTrustedProxyCidrs=$peers[0]' "$INSTALLATION_FILE" > "$RUN_DIR/installation-next.json"
jq '.worker.replicaCount=1' "$VALUES_FILE" > "$RUN_DIR/values-next.json"
if ! cmp -s "$RUN_DIR/installation-next.json" "$INSTALLATION_FILE" || ! cmp -s "$RUN_DIR/values-next.json" "$VALUES_FILE"; then
  # Commit local configuration only after the upgrade succeeds, so failures remain retryable.
  INSTALLATION_FILE="$RUN_DIR/installation-next.json" VALUES_FILE="$RUN_DIR/values-next.json" upgrade_installation
  mv "$RUN_DIR/installation-next.json" "$INSTALLATION_FILE"
  mv "$RUN_DIR/values-next.json" "$VALUES_FILE"
else
  rm "$RUN_DIR/installation-next.json" "$RUN_DIR/values-next.json"
fi
namespace=$(request GET /namespaces | jq -er --arg name "${OCE_NAMESPACE_NAME:-default}" '[.data[]|select(.name==$name)]|select(length==1)|first.id')
printf '%s\n' "$namespace" > "$RUN_DIR/namespace-id"
for attempt in {1..60}; do
  tenant=$(kube get ns -l "openclaw.dev/namespace=$namespace" -o json | jq -r '.items | if length==1 then first.metadata.name else empty end')
  [[ -z $tenant ]] || break
  [[ $attempt != 60 ]] || { echo 'OCE did not create its tenant namespace' >&2; exit 1; }
  sleep 3
done
kube get ns "$tenant" -o json | jq -e '.metadata.labels["pod-security.kubernetes.io/enforce"]=="restricted"' >/dev/null
kube -n "$tenant" create role oce-workspace-provisioner \
  --verb=get,list,watch,create,update,patch,delete --resource=roles,rolebindings --dry-run=client -o json |
  jq '.rules += [{apiGroups:["rbac.authorization.k8s.io"],resources:["roles"],resourceNames:["openshell-workspace-sandbox"],verbs:["bind","escalate"]}]' | kube apply -f -
kube -n "$tenant" create rolebinding oce-workspace-provisioner --role=oce-workspace-provisioner \
  --serviceaccount=oce-system:openclaw-enterprise-worker --dry-run=client -o json | kube apply -f -
for binding in api:oce-api-secrets:oce-openclaw-tenant-api api:oce-api-configuration:oce-openclaw-tenant-configuration worker:openclaw-enterprise-worker:oce-openclaw-tenant-worker; do
  IFS=: read -r role name clusterrole <<< "$binding"
  kube -n "$tenant" create rolebinding "$name" --clusterrole="$clusterrole" \
    --serviceaccount="oce-system:openclaw-enterprise-$role" --dry-run=client -o json | kube apply -f -
done
for attempt in {1..60}; do
  request GET "/namespaces/$namespace" > "$RUN_DIR/namespace-proof.json"
  if jq -e '.data.status=="ready"' "$RUN_DIR/namespace-proof.json" >/dev/null; then
    # Supervisors fetch startup policy before any per-Agent Service grants exist.
    python3 "$assets/service-policies.py" --tenant "$tenant" --namespace-id "$namespace" \
      --openshell-namespace "$GATEWAY_NAMESPACE" > "$RUN_DIR/namespace-service-peers.json"
    kube apply -f "$RUN_DIR/namespace-service-peers.json"
    printf 'OCE Namespace ready: %s (%s)\n' "$namespace" "$tenant"
    exit 0
  fi
  [[ $attempt != 60 ]] || { echo 'Namespace not ready; inspect OCE worker logs' >&2; exit 1; }
  sleep 3
done
