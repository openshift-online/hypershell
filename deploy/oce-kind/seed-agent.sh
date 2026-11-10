#!/usr/bin/env bash
set -euo pipefail
set +x
umask 077
: "${OCE_REPO:?Set OCE_REPO}"
: "${OCE_KIND_STATE:?Set OCE_KIND_STATE}"
: "${ENMAAS_API_KEY_FILE:?Set ENMAAS_API_KEY_FILE}"
export RUN_DIR="$OCE_KIND_STATE" KUBECONFIG="$OCE_KIND_STATE/kubeconfig" CONTEXT=kind-hypershell-oce
export PLATFORM_NS=oce-system RELEASE=oce
export OCE_CHART="$OCE_REPO/deploy/helm/openclaw-enterprise"
export INSTALLATION_FILE="$RUN_DIR/installation.json" VALUES_FILE="$RUN_DIR/values.json"
export OCC_SERVICE_KEY_FILE="$RUN_DIR/initial-admin-service-key.json"
exec 9>"$RUN_DIR/seed-agent.lock"
flock -n 9 || { echo 'Another Codex seed is running.' >&2; exit 1; }
assets="$OCE_REPO/docs/snippets/openshell-quickstart"
cp "$assets/request.mjs" "$RUN_DIR/request.mjs"
mkdir -p "$RUN_DIR/kubernetes"
cp "$assets/kubernetes/"{service-policies.py,trusted-envoy-peers.sh} "$RUN_DIR/kubernetes/"
cp "$OCE_REPO/docs/assets/openshell-quickstart/chat.mjs" "$RUN_DIR/chat.mjs"
source "$assets/operator-session.sh"
source "$assets/platform-kubernetes.sh"
read -r namespace < "$RUN_DIR/namespace-id"
export OPENSHELL_NAMESPACE TENANT_NAMESPACE
read -r OPENSHELL_NAMESPACE < "$RUN_DIR/gateway-namespace"
base="/namespaces/$namespace"
TENANT_NAMESPACE=$(kube get ns -l "openclaw.dev/namespace=$namespace" -o json |
  jq -er '.items|select(length==1)|first.metadata.name')
export ENVOY_IMAGE
ENVOY_IMAGE=$(kube -n oce-system get envoyproxy oce-agent-gateways -o json |
  jq -er '.spec.provider.kubernetes.envoyDeployment.container.image')
request GET "$base" | jq -e '.data.status=="ready"' >/dev/null

model_url=https://api.enmaas.devshift.net/v1
jq --arg url "$model_url" \
  '.drivers.compute.configuration.runtime.modelProvider={protocol:"openai-responses",baseUrl:$url}' \
  "$INSTALLATION_FILE" > "$RUN_DIR/installation-enmaas.json"
if ! cmp -s "$RUN_DIR/installation-enmaas.json" "$INSTALLATION_FILE"; then
  INSTALLATION_FILE="$RUN_DIR/installation-enmaas.json" upgrade_installation
  mv "$RUN_DIR/installation-enmaas.json" "$INSTALLATION_FILE"
else
  rm "$RUN_DIR/installation-enmaas.json"
fi

# These two APIs have no idempotency key. Retain uncertain writes for inspection, never replay them.
create_once() {
  local record="$RUN_DIR/$1.json" path=$2 id
  if [[ -e $record ]]; then
    cat >/dev/null
    id=$(jq -er '.data.id' "$record")
    request GET "$path/$id" > "$record.live"
    jq -e --slurpfile saved "$record" '.data.id==$saved[0].data.id and
      .data.namespaceId==$saved[0].data.namespaceId and .data.name==$saved[0].data.name' "$record.live" >/dev/null
    return
  fi
  if [[ -e $record.pending ]]; then
    cat >/dev/null
    printf 'Unconfirmed creation: %s. Read back %s and recover its response before retrying; do not blindly delete the pending file.\n' "$record.pending" "$path" >&2
    return 1
  fi
  request POST "$path" --data-binary @- > "$record.pending"
  jq -e '.data.id | strings | length>0' "$record.pending" >/dev/null
  mv "$record.pending" "$record"
}
jq -n --rawfile token "$ENMAAS_API_KEY_FILE" \
  '{name:"kind-codex-enmaas",value:($token|gsub("^\\s+|\\s+$";""))}|select(.value|length>0)' |
  create_once codex-secret "$base/secrets"
jq -n --slurpfile secret "$RUN_DIR/codex-secret.json" --arg url "$model_url" \
  '{name:"kind-codex-enmaas",type:"openai",config:{authorized_base_url:$url},secrets:{api_key:$secret[0].data.ref}}' |
  create_once codex-source "$base/credential-sources"
modelSource=$(jq -er '.data.id' "$RUN_DIR/codex-source.json")
request GET "$base/credential-sources/$modelSource" |
  jq -e --arg url "$model_url" '.data | .state=="ready" and .type=="openai" and .config.authorized_base_url==$url' >/dev/null

deployment_preflight
if [[ ! -e $RUN_DIR/codex-provision-request.json ]]; then
  request_id=$(python3 -c 'import uuid; print("req_" + str(uuid.uuid4()))')
  jq -n --arg model gpt-6-luna -f "$assets/codex-configuration.jq" |
    jq --arg request "$request_id" --arg source "$modelSource" \
      '{requestId:$request,name:"codex-test",configuration:.,executionMode:"dedicated",
        gatewayManagedMode:true,nativeAdmin:true,harnessAuth:{method:"credential_source",sourceId:$source}}' \
      > "$RUN_DIR/codex-provision-request.tmp"
  mv "$RUN_DIR/codex-provision-request.tmp" "$RUN_DIR/codex-provision-request.json"
fi
if [[ ! -s $RUN_DIR/codex-provision.json ]]; then
  # OCE deduplicates this exact saved request, including after a lost response.
  request POST "$base/agents/provision" --data-binary @- < "$RUN_DIR/codex-provision-request.json" > "$RUN_DIR/codex-provision.tmp"
  jq -e '.data.provisioning.workId | strings | length>0' "$RUN_DIR/codex-provision.tmp" >/dev/null
  mv "$RUN_DIR/codex-provision.tmp" "$RUN_DIR/codex-provision.json"
fi
work=$(jq -er '.data.provisioning.workId' "$RUN_DIR/codex-provision.json")
network_revision=''
for attempt in {1..120}; do
  request GET "$base/agents/provision/$work" > "$RUN_DIR/codex-provision-status.json"
  status=$(jq -er '.data.status' "$RUN_DIR/codex-provision-status.json")
  if [[ $status == failed ]]; then
    printf 'Codex provisioning failed. Inspect %s/codex-provision-status.json and the deployment before retrying in OCE.\n' "$RUN_DIR" >&2
    exit 1
  fi
  agent=$(jq -r '.data.agentId // empty' "$RUN_DIR/codex-provision-status.json")
  revision=$(jq -r '.data.revisionId // empty' "$RUN_DIR/codex-provision-status.json")
  if [[ -n $agent && -n $revision && $network_revision != "$revision" ]]; then
    # The upstream helper waits for the Service, grants exact peers, and checks the active revision.
    wait_deployment "$revision"
    network_revision=$revision
  fi
  if [[ $status == succeeded && -n $network_revision ]]; then break; fi
  [[ $attempt != 120 ]] || { echo 'Codex provisioning timed out; rerun to resume the recorded job.' >&2; exit 1; }
  sleep 3
done
printf '%s\n' "$agent" > "$RUN_DIR/codex-agent-id"
printf 'Checking codex-test (gpt-6-luna) through the OCE gateway...\n'
printf '%s\n' 'Reply with OCE_READY.' > "$RUN_DIR/codex-prompt"
turn "$RUN_DIR/codex-prompt" > "$RUN_DIR/codex-result.json"
jq -e '.outcome.status=="ok" and (.outcome.terminalReply.text|contains("OCE_READY"))' "$RUN_DIR/codex-result.json" >/dev/null
printf 'Codex agent answered OCE_READY.\n'
