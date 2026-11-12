#!/usr/bin/env bash
set -euo pipefail
set +x
umask 077
here=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
export HYPERSHELL_REPO
HYPERSHELL_REPO=$(cd "$here/../.." && pwd)
cd "$HYPERSHELL_REPO"

fail() { printf '%s\n' "$*" >&2; exit 1; }
: "${ENMAAS_API_KEY_FILE:?Set ENMAAS_API_KEY_FILE to a file containing the EnMaaS API key}"
[[ $(uname -s) == Linux ]] || fail 'The OCE kind setup currently requires Linux and Docker.'
for tool in docker kind kubectl kustomize git go node jq openssl flock python3 curl make; do
  command -v "$tool" >/dev/null || fail "Required tool missing: $tool (see deploy/oce-kind/README.md)"
done
python3 -c 'import tomllib, yaml' || fail 'Python 3.11+ with PyYAML is required.'
export ENMAAS_API_KEY_FILE
ENMAAS_API_KEY_FILE=$(realpath "$ENMAAS_API_KEY_FILE")
python3 - <<'PY'
import os
from pathlib import Path
key = Path(os.environ['ENMAAS_API_KEY_FILE']).read_text().strip()
if not key or any(c.isspace() or ord(c) < 33 for c in key):
    raise SystemExit('ENMAAS_API_KEY_FILE must contain one nonempty API key.')
PY
docker info >/dev/null

state_home=${XDG_STATE_HOME:-$HOME/.local/state}
mkdir -p "$state_home"
# Lock before choosing a directory so concurrent fresh runs cannot select separate installations.
exec 8>"$state_home/oce-hypershell-kind.lock"
flock -n 8 || fail 'Another hypershell-oce setup is running.'
export CONTAINER_ENGINE=docker KIND_EXPERIMENTAL_PROVIDER=docker
export OCE_KIND_STATE
OCE_KIND_STATE=$(python3 "$here/select-state.py" "$state_home")
mkdir -p "$OCE_KIND_STATE"
chmod 700 "$OCE_KIND_STATE"
OCE_KIND_STATE=$(cd "$OCE_KIND_STATE" && pwd)
export OCE_REPO="${OCE_REPO:-$OCE_KIND_STATE/oce-source}"
export KUBECONFIG="$OCE_KIND_STATE/kubeconfig" CONTEXT=kind-hypershell-oce
export KIND_CLUSTER_NAME=hypershell-oce KIND_NAMESPACE=hypershell-system KIND_NO_SUDO=true
export KIND_HOT_RELOAD=false
export CPK_LOG="$OCE_KIND_STATE/cpk.log" CURL_HOME="$OCE_KIND_STATE/curl"
mkdir -p "$CURL_HOME"
printf 'ipv4\n' > "$CURL_HOME/.curlrc"
stage=cluster
trap 'printf "OCE setup stopped during %s. State preserved in %s; resolve the error before rerunning.\n" "$stage" "$OCE_KIND_STATE" >&2' ERR
printf 'Installation state: %s\n' "$OCE_KIND_STATE"

clusters=$(kind get clusters)
if grep -qx hypershell-oce <<< "$clusters"; then
  [[ -s $KUBECONFIG && -s $OCE_KIND_STATE/cluster-uid ]] ||
    fail 'Cluster hypershell-oce already exists. Select its original OCE_KIND_STATE; setup will not adopt an unrecorded cluster.'
  uid=$(kubectl --context "$CONTEXT" get namespace kube-system -o jsonpath='{.metadata.uid}')
  [[ $uid == "$(cat "$OCE_KIND_STATE/cluster-uid")" ]] || fail 'Cluster identity differs from the saved state. Preserve it and use a fresh state directory for a fresh cluster.'
else
  [[ ! -e $OCE_KIND_STATE/cluster-uid && ! -e $OCE_KIND_STATE/install-receipt && ! -d $OCE_KIND_STATE/secrets ]] ||
    fail 'The cluster disappeared during setup. Rerun the same command to allocate fresh state; previous state is preserved.'
  cp "$here/pod-security.yaml" "$OCE_KIND_STATE/pod-security.yaml"
  python3 - <<'PY'
import os, yaml
from pathlib import Path
root, state = Path(os.environ['HYPERSHELL_REPO']), Path(os.environ['OCE_KIND_STATE'])
config = yaml.safe_load((root / 'deploy/oce-kind/kind.yaml').read_text())
for mount in config['nodes'][0]['extraMounts']:
    mount['hostPath'] = mount['hostPath'].replace('__HYPERSHELL_REPO__', str(root)).replace('__STATE__', str(state))
(state / 'kind.yaml').write_text(yaml.safe_dump(config))
PY
  kind create cluster --name hypershell-oce --config "$OCE_KIND_STATE/kind.yaml"
  kubectl --context "$CONTEXT" get namespace kube-system -o jsonpath='{.metadata.uid}' > "$OCE_KIND_STATE/cluster-uid"
fi
[[ $(kubectl config current-context) == "$CONTEXT" ]] || fail 'Saved kubeconfig has the wrong current context.'
ln -sfn "$OCE_KIND_STATE" "$state_home/oce-hypershell-kind-current.next"
mv -Tf "$state_home/oce-hypershell-kind-current.next" "$state_home/oce-hypershell-kind-current"
kubectl -n kube-system rollout status daemonset/kindnet --timeout=5m
kubectl wait node --all --for=condition=Ready --timeout=3m

stage=HyperShell
if [[ ! -e $OCE_KIND_STATE/hypershell-ready && ! -e $OCE_KIND_STATE/install-receipt ]]; then
  # Only the first bring-up reapplies baseline manifests; later runs preserve operator settings and ingress.
  make kind-up SKIP_SEED=true KIND_CLUSTER_NAME=hypershell-oce KIND_NAMESPACE=hypershell-system \
    CONTAINER_ENGINE=docker KIND_HOT_RELOAD=false KIND_KEYCLOAK_URL= 8>&-
  touch "$OCE_KIND_STATE/hypershell-ready"
fi
stage='gateway prerequisites'
bash "$here/prepare-oce.sh"
python3 - <<'PY'
import subprocess, yaml
from pathlib import Path
for patch in yaml.safe_load_all(Path('deploy/oce-kind/hypershell-operator.yaml').read_text()):
    subprocess.run(['kubectl', '-n', 'hypershell-system', 'patch', 'deployment',
                    patch['metadata']['name'], '--type=strategic', '--patch', yaml.safe_dump(patch)], check=True)
PY
for deployment in hypershell-api-server hypershell-controller hypershell-web-console; do
  kubectl -n hypershell-system rollout status "deployment/$deployment" --timeout=5m
done
keycloak_host=$(kubectl -n keycloak get deployment keycloak -o json |
  jq -er '.spec.template.spec.containers[].env[]|select(.name=="KC_HOSTNAME")|.value|ltrimstr("https://")')
make kind-seed SEED_OPERATOR_WORKSPACES=true SEED_STRICT=true KIND_CLUSTER_NAME=hypershell-oce KIND_NAMESPACE=hypershell-system \
  CONTAINER_ENGINE=docker KEYCLOAK_HOSTNAME="$keycloak_host" 8>&-
for attempt in {1..90}; do
  gateways=$(kubectl get deployment -A --field-selector metadata.name=openshell-gateway -o json)
  count=$(jq '.items|length' <<< "$gateways")
  [[ $count -le 1 ]] || fail 'Expected exactly one HyperShell gateway; found multiple gateways.'
  if [[ $count == 1 ]]; then
    export GATEWAY_NAMESPACE
    GATEWAY_NAMESPACE=$(jq -er '.items[0].metadata.namespace' <<< "$gateways")
    break
  fi
  [[ $attempt != 90 ]] || fail 'Gateway not provisioned; inspect the HyperShell controller logs.'
  sleep 2
done
kubectl -n "$GATEWAY_NAMESPACE" rollout status deployment/openshell-gateway --timeout=5m
printf '%s\n' "$GATEWAY_NAMESPACE" > "$OCE_KIND_STATE/gateway-namespace"
printf '%s\n' "$keycloak_host" > "$OCE_KIND_STATE/keycloak-host"

stage='OCE image preparation'
if [[ ! -s $OCE_KIND_STATE/images.json || ! -x $OCE_KIND_STATE/occ || ! -d $OCE_REPO ]]; then
  bash "$here/pull-oce.sh"
fi
for file in cmd/occ/main.go deploy/helm/openclaw-enterprise/Chart.yaml \
  docs/assets/openshell-quickstart/chat.mjs docs/snippets/openshell-quickstart/operator-session.sh \
  docs/snippets/openshell-quickstart/platform-kubernetes.sh docs/snippets/openshell-quickstart/codex-configuration.jq; do
  [[ -f $OCE_REPO/$file ]] || fail "OCE checkout is missing $file; see deploy/oce-kind/README.md."
done
python3 "$OCE_REPO/docs/snippets/openshell-quickstart/kubernetes/service-policies.py" --help |
  grep -q -- --openshell-namespace || fail 'OCE needs the namespace-aware OpenShell network helper; see the README.'
stage='Helm preparation'
mkdir -p "$OCE_KIND_STATE/bin" "$OCE_KIND_STATE/helm-plugins"
if [[ ! -x $OCE_KIND_STATE/bin/helm ]]; then
  # The HyperShell controller ships the Helm 3 required by OCC, even on Helm 4 hosts.
  controller_image=$(kubectl -n "$KIND_NAMESPACE" get deployment/hypershell-controller \
    -o 'jsonpath={.spec.template.spec.containers[?(@.name=="controller")].image}')
  container=$(docker create "$controller_image")
  if ! docker cp "$container:/usr/local/bin/helm" "$OCE_KIND_STATE/bin/helm"; then
    docker rm "$container"
    fail 'Could not extract Helm 3 from the HyperShell controller image.'
  fi
  docker rm "$container"
fi
export PATH="$OCE_KIND_STATE/bin:$PATH" HELM_PLUGINS="$OCE_KIND_STATE/helm-plugins"
for stage in install-oce activate-oce browser-access seed-agent; do
  printf '\n==> %s\n' "$stage"
  bash "$here/$stage.sh"
done
stage='browser access'
bash "$here/access.sh" 8>&-
