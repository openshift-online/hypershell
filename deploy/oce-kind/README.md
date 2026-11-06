# Run HyperShell and OpenClaw Enterprise in one kind cluster

This development installation runs HyperShell, OpenClaw Enterprise (OCE), and a
HyperShell-provisioned OpenShell gateway in `kind-hypershell-oce`. HyperShell owns
the gateway and its database. OCE owns its tenant namespaces, Agent lifecycle,
and per-Agent OpenClaw gateways. OCE uses OpenShell to run Agent harnesses.

Use a fresh, disposable cluster. This is not a production installation. The
OpenShell integration is experimental, and the single node provides no high
availability. HyperShell's development Keycloak and PostgreSQL retain their
documented development credentials. The kind load-balancer helper publishes
Docker ports on all host interfaces.
Use a firewall or an isolated development network to restrict them to this
machine; the OCE Console forward binds only to loopback.

## Prerequisites and source

Use Linux with Docker, kind, kubectl, Helm 3, kustomize, Go, Node.js, jq, OpenSSL,
Python 3 with PyYAML, and registry/GitHub access. This run uses Kubernetes 1.35.0,
Calico 3.31.0, Envoy Gateway 1.6.7, and the HyperShell versions in
[`OPENSHELL_VERSION`](../../OPENSHELL_VERSION) and [`Makefile`](../../Makefile).
Allow about 16 GiB of RAM for the running stack and additional RAM and disk for
parallel image builds. Use OCE commit
`2096c8cbde286c50ab7aafa08fd0f3bb3f4623c2`; its scripts and chart are part of the
installation inputs. Build both OCE images from this commit. The public
`latest` alias inspected during setup targets an older OpenShell integration.

Run from this HyperShell branch. These commands keep the default kubeconfig and
context unchanged. The private state directory contains credentials: do not
commit it, print its Secret files, or share it as troubleshooting evidence.

```bash
set -euo pipefail
umask 077
export HYPERSHELL_REPO="$PWD"
export OCE_REPO=/absolute/path/to/openclaw-enterprise
export OCE_KIND_STATE="$HOME/.local/state/oce-hypershell-kind"
mkdir -p "$OCE_KIND_STATE"
chmod 700 "$OCE_KIND_STATE"
export KUBECONFIG="$OCE_KIND_STATE/kubeconfig"
export CONTAINER_ENGINE=docker KIND_EXPERIMENTAL_PROVIDER=docker
export KIND_CLUSTER_NAME=hypershell-oce KIND_NO_SUDO=true
export KIND_HOT_RELOAD=false LOCAL_IMAGES=true
export CPK_LOG="$OCE_KIND_STATE/cpk.log"
# This Docker host exposes the development gateway on IPv4.
export CURL_HOME="$OCE_KIND_STATE/curl"
mkdir -p "$CURL_HOME"
printf 'ipv4\n' > "$CURL_HOME/.curlrc"
```

## Create the cluster and enforce network policy

Precreate kind so the CNI is ready before `make kind-up` waits for infrastructure.
The Pod Security configuration exempts only the named OpenShell RuntimeClass;
OCE still applies restricted Pod Security to its tenant namespaces. Treat
Kubernetes administrator access and OpenShell operator credentials as privileged.

```bash
cp deploy/oce-kind/pod-security.yaml "$OCE_KIND_STATE/pod-security.yaml"
python3 - <<'PY'
import os
from pathlib import Path
root = Path(os.environ['HYPERSHELL_REPO'])
state = Path(os.environ['OCE_KIND_STATE'])
text = (root/'deploy/oce-kind/kind.yaml').read_text()
(state/'kind.yaml').write_text(text.replace('__HYPERSHELL_REPO__', str(root)).replace('__STATE__', str(state)))
PY
kind create cluster --name "$KIND_CLUSTER_NAME" --config "$OCE_KIND_STATE/kind.yaml"
curl -fsSL https://raw.githubusercontent.com/projectcalico/calico/v3.31.0/manifests/calico.yaml \
  -o "$OCE_KIND_STATE/calico.yaml"
printf '%s  %s\n' 1990b1cec14002790718bb960b1b8445eb7b896cd16c32e952309f35a9026981 \
  "$OCE_KIND_STATE/calico.yaml" | sha256sum --check
kubectl apply -f "$OCE_KIND_STATE/calico.yaml"
kubectl -n kube-system set env daemonset/calico-node \
  CALICO_IPV4POOL_CIDR=10.244.0.0/16 CALICO_IPV4POOL_IPIP=Never CALICO_IPV4POOL_VXLAN=Always
kubectl -n kube-system rollout status daemonset/calico-node --timeout=5m
kubectl wait node --all --for=condition=Ready --timeout=3m
KIND_SKIP_SEED=true make kind-up
```

Expect healthy HyperShell API, controller, Console, Keycloak, PostgreSQL and
Vault workloads. Seeding is deferred because a gateway's workspace mode is
immutable. Do not create the first gateway until the next section is complete.

## Configure the HyperShell-owned gateway

The API [create default](../../components/api-server/plugins/gateways/field_policy.go)
saves the workspace mode. The controller [loads operator settings](../../components/control-plane/internal/config/config.go)
and carries them through the reconciler into the [Helm values builder](../../components/control-plane/internal/helm/values.go).
These are deployment settings, not user-writable API fields:

| Setting | Owner | Effect |
|---|---|---|
| `GATEWAY_DEFAULT_WORKSPACE_MODE` | API server | Selects the mode saved for newly created gateways; unset retains `managed`. Existing gateways retain their mode. |
| `GATEWAY_OPERATOR_WORKSPACE` | Controller | JSON with `namespaceLabelKey` and `allowedPodLabels`. Required for operator-mode gateways; invalid input fails controller startup. |

The controller combines the configured label key with each gateway's own
namespace as the value. For this installation, OCE labels each tenant namespace
`hypershell.io/gateway=<gateway namespace>`. Other gateways cannot discover it
through their operator selector. The controller enables external driver
configuration, disables resource-approval labels for this trusted operator, and
preserves mTLS authentication alongside OIDC. Give this gateway only trusted
operator accounts: it can attach resources in the selected namespaces.

Apply the supplied deployment patches with strategic merge, then seed through
HyperShell's normal authenticated API:

```bash
python3 - <<'PY'
import subprocess, yaml
from pathlib import Path
for resource in yaml.safe_load_all(Path('deploy/oce-kind/hypershell-operator.yaml').read_text()):
    subprocess.run(['kubectl','-n','hypershell-system','patch','deployment',
                    resource['metadata']['name'],'--type=strategic','--patch',yaml.safe_dump(resource)], check=True)
PY
set -a
source deploy/oce-kind/openshell-images.env
set +a
kubectl -n hypershell-system set env deployment/hypershell-controller \
  GATEWAY_IMAGE="$GATEWAY_IMAGE" GATEWAY_SUPERVISOR_IMAGE="$GATEWAY_SUPERVISOR_IMAGE" \
  GATEWAY_SANDBOX_RUNTIME_IMAGE="$GATEWAY_SANDBOX_RUNTIME_IMAGE"
kubectl -n hypershell-system rollout status deployment/hypershell-api-server
kubectl -n hypershell-system rollout status deployment/hypershell-controller
keycloak_host=$(kubectl -n keycloak get deployment keycloak -o json |
  jq -er '.spec.template.spec.containers[].env[]|select(.name=="KC_HOSTNAME")|.value|ltrimstr("https://")')
KIND_SEED_STRICT=true make kind-seed KEYCLOAK_HOSTNAME="$keycloak_host"
# Provisioning is asynchronous; do not continue until exactly one gateway exists.
for attempt in {1..90}; do
  GATEWAY_NAMESPACE=$(kubectl get deployment -A --field-selector metadata.name=openshell-gateway -o json |
    jq -r '.items|if length==1 then first.metadata.namespace else empty end')
  [[ -z $GATEWAY_NAMESPACE ]] || break
  [[ $attempt != 90 ]] || { echo 'Gateway not provisioned; inspect controller logs'; exit 1; }
  sleep 2
done
export GATEWAY_NAMESPACE
kubectl -n "$GATEWAY_NAMESPACE" rollout status deployment/openshell-gateway --timeout=5m
```

Use `make kind-api-server-up` or `make kind-control-plane-up` for later code
changes, with the same exported cluster settings. `make kind-up` restores
ordinary Kind manifests and can recreate gateway ingress addresses; if rerun,
reapply these patches and reverify OIDC URLs and network peers. Do not manually
upgrade the gateway's Helm release:
HyperShell reconciles it with `--reset-values` and would overwrite those edits.

## Build and load OCE

In the OCE checkout, build the CLI and images using its source manifest:

```bash
cd "$OCE_REPO"
GOTOOLCHAIN=auto go build -trimpath -o "$OCE_KIND_STATE/occ" ./cmd/occ
docker build --target runtime \
  --build-arg NODE_BASE_IMAGE=docker.io/library/node:24.22.1-bookworm@sha256:6449e19d8184d84062525b6a7524def03146c2cb04a7f08b076680d0e8e4240a \
  --build-arg OCC_BUILD_REVISION="$(git rev-parse HEAD)" \
  -t localhost/oce-hypershell/controller:local .
source_args=$(node scripts/lib/openclaw-source.mjs build-args)
openclaw_build_args=()
while IFS= read -r argument; do openclaw_build_args+=(--build-arg "$argument"); done <<< "$source_args"
docker build -f deploy/runtime/Dockerfile "${openclaw_build_args[@]}" \
  -t localhost/oce-hypershell/runtime:local .
for role in controller runtime; do
  tag="localhost/oce-hypershell/$role:local"
  docker save "$tag" | docker exec -i hypershell-oce-control-plane ctr -n k8s.io images import -
  digest=$(docker exec hypershell-oce-control-plane ctr -n k8s.io images ls |
    awk -v image="$tag" '$1==image {print $3}')
  [[ $digest =~ ^sha256:[a-f0-9]{64}$ ]]
  reference="localhost/oce-hypershell/$role@$digest"
  docker exec hypershell-oce-control-plane ctr -n k8s.io images tag "$tag" "$reference"
  printf '%s\n' "$reference" > "$OCE_KIND_STATE/$role-image"
done
jq -n --rawfile controller "$OCE_KIND_STATE/controller-image" --rawfile runtime "$OCE_KIND_STATE/runtime-image" \
  '{controller:($controller|rtrimstr("\n")),runtime:($runtime|rtrimstr("\n"))}' > "$OCE_KIND_STATE/images.json"
cd "$HYPERSHELL_REPO"
```

Use the imported manifest digests, not Docker image configuration IDs. Both
controller and Agent runtime references remain immutable.

## Install OCE and connect it to OpenShell

OCE's installation CLI requires Helm 3. If the host has Helm 4, extract the
pinned Helm 3.14 binary from the HyperShell controller image built above. Isolate
plugins so host Helm 4 plugins cannot interfere:

```bash
mkdir -p "$OCE_KIND_STATE/bin" "$OCE_KIND_STATE/helm-plugins"
container=$(docker create localhost/hypershell-controller:dev)
docker cp "$container:/usr/local/bin/helm" "$OCE_KIND_STATE/bin/helm"
docker rm "$container"
export PATH="$OCE_KIND_STATE/bin:$PATH"
export HELM_PLUGINS="$OCE_KIND_STATE/helm-plugins"
helm version --short
```

Run the supplied helpers in order:

```bash
bash deploy/oce-kind/prepare-oce.sh
bash deploy/oce-kind/install-oce.sh
bash deploy/oce-kind/activate-oce.sh
bash deploy/oce-kind/browser-access.sh
```

`prepare-oce.sh` verifies and installs Envoy Gateway 1.6.7 while preserving
HyperShell's Gateway API CRDs. It creates GatewayClass `eg`, namespace
`oce-system`, and RuntimeClass `openshell-sandbox`, and labels the control node.

`install-oce.sh` creates the separate
`openclaw_oce_kind` database on HyperShell's development PostgreSQL server, with
distinct limited application and migrator roles and verified TLS. It issues a
dedicated OpenShell client certificate with `CN=oce-controller` and
`OU=openshell-admin`, mounts only the leaf credentials in OCC,
renders OCE's existing OpenShell templates, prepares protected bootstrap storage,
and runs `occ installation install` with a durable receipt. It refuses to rerun
an attempted installation; inspect the receipt before resuming a failed attempt.

The generated `installation.json` points at
`https://openshell-gateway.<gateway namespace>.svc:8080`. No second OpenShell
gateway is installed by OCE. Keep `installation.json`, `values.json`,
`images.json`, and `install-receipt` with the private state directory.

`activate-oce.sh` pins the Envoy proxy, records its ready Pod addresses as
trusted peers, enables the OCE worker through the existing Installation update
flow, and grants OCE its tenant-local roles. It waits for the default Namespace
to become `ready`. This requires OCE to register its operator Workspace through
the HyperShell-created gateway with mTLS.

The RuntimeClass admission policy admits only the Agent Sandbox controller in
this gateway's labeled namespaces with the pinned OCE runtime and OpenShell
supervisor images. It rejects privileged containers, host namespaces and
hostPath volumes. Kubernetes administrator credentials remain privileged.

## Access and verification

Run the loopback port-forward printed by `browser-access.sh` in a separate
terminal. Open `https://console.hypershell-oce.oce.localhost:18443/console/`.
Use `admin@oce.localhost` and the password in
`$OCE_KIND_STATE/initial-admin-password`. Trust only the generated browser CA
`$OCE_KIND_STATE/owner-tls/ca.crt`; keep its private key on this machine.
The service key in `initial-admin-service-key.json` is for operator API calls.

On this devbox, the forward runs as the user service `oce-hypershell-console`.
Use `systemctl --user status oce-hypershell-console` to inspect it. For another
systemd host, prefix the printed forward command with
`systemd-run --user --unit=oce-hypershell-console --property=Restart=on-failure --collect`.
Otherwise, leave the printed command running in its terminal.

HyperShell is at `https://console.hypershell.localhost:<assigned port>`.
Use the port in `$keycloak_host`, printed by the startup banner. HyperShell's
local Keycloak uses its documented development users. On a remote devbox,
forward these ports through SSH to laptop loopback; the `.localhost` hosts must
resolve there. Never expose these development listeners publicly.

```bash
kubectl -n hypershell-system get deployments
kubectl -n "$GATEWAY_NAMESPACE" get deployments
kubectl -n oce-system get deployments
jq -e '.data.status=="ready"' "$OCE_KIND_STATE/namespace-proof.json"
curl --fail --silent --show-error --noproxy '*' \
  --cacert "$OCE_KIND_STATE/owner-tls/ca.crt" \
  --resolve console.hypershell-oce.oce.localhost:18443:127.0.0.1 \
  https://console.hypershell-oce.oce.localhost:18443/api/auth/session |
  jq -e '.data==null'
```

This setup proves the shared cluster, controller readiness, authenticated OCE
access, and operator Workspace registration. It does not configure model
credentials or prove an Agent model turn. Follow the matching OCE
[Agent deployment guide](https://github.com/openclaw/openclaw-enterprise/blob/2096c8cbde286c50ab7aafa08fd0f3bb3f4623c2/docs/guides/deploy/deploy-first-agent.md)
for that separate workflow, including per-Agent network grants. The normal OCE
quickstart assumes OpenShell is in `oce-system`; adapt those grants to the
HyperShell gateway namespace used here.

The deployment verified on 2026-11-06 uses OCE Namespace `hypershell`; the
initial empty `default` Namespace was deleted after diagnosing a certificate
role error. The README includes that correction; the complete sequence has not
been replayed on a second fresh cluster.
To rerun tenant permission checks for an existing named Namespace, set
`OCE_NAMESPACE_NAME=hypershell` when running `activate-oce.sh`.

## Recovery and removal

Read readiness and events before retrying:

```bash
kubectl get pods -A
kubectl -n hypershell-system logs deployment/hypershell-controller --tail=60
kubectl -n "$GATEWAY_NAMESPACE" get events --sort-by=.lastTimestamp
kubectl -n oce-system get jobs,pods
"$OCE_KIND_STATE/occ" installation doctor --kubeconfig "$KUBECONFIG" \
  --context kind-hypershell-oce --kube-namespace oce-system --release oce \
  --receipt "$OCE_KIND_STATE/install-receipt"
```

The OpenShell client certificate copied into `oce-system` is not synchronized
automatically. Before its 30-day expiry, wait for cert-manager renewal, copy the
renewed leaf Secret using the command in `install-oce.sh`, and restart OCE API
and worker. Preserve the gateway CA, database and signing keys across restarts.

To delete this installation and all its databases and volumes, first confirm
the selected cluster name, then run:

```bash
kind get clusters
kind delete cluster --name hypershell-oce
docker stop hypershell-oce-dns
docker rm hypershell-oce-dns
# If you used the user service:
systemctl --user stop oce-hypershell-console
```

Stop this installation's port forwards separately. Retain the private state
directory for diagnosis or delete it deliberately after confirming no credentials
or evidence need to be retained. Do not use a different cluster's kubeconfig.
