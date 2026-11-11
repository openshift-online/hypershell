# Manual HyperShell and OCE setup

For normal use, run [`make kind-up-with-oce`](README.md). It automates these
steps and manages the state directory for you. This runbook is for engineers
who need to run an individual stage, diagnose a failure, or maintain the
integration. Its shell exports and manual state management are not prerequisites
for the Makefile command.

## Prerequisites and source

Use Linux with Docker, kind 0.31.0, kubectl, Helm 3, kustomize, Go, Node.js, jq, curl with HTTP/2,
OpenSSL, flock (from util-linux), Python 3.11 or newer with PyYAML, and registry/GitHub access. This run
uses Kubernetes 1.35.0, Envoy Gateway 1.6.7, and the HyperShell versions in
[`OPENSHELL_VERSION`](../../OPENSHELL_VERSION) and [`Makefile`](../../Makefile).
Allow about 16 GiB of RAM for the running stack and additional RAM and disk for
parallel image builds. Use OCE branch `fix/hypershell-runtime-networking` from
[`bsquizz/openclaw-enterprise`](https://github.com/bsquizz/openclaw-enterprise/tree/fix/hypershell-runtime-networking),
pinned to commit [`080810ac2`](https://github.com/bsquizz/openclaw-enterprise/commit/080810ac2a05b98f065655a9b29cdc9ec28ab14e),
for the namespace-aware network helper. The change is published in companion
draft [OCE PR #4954](https://github.com/openclaw/openclaw-enterprise/pull/4954) and is
based on tested runtime source `2096c8cbde286c50ab7aafa08fd0f3bb3f4623c2` and
changes only operator helpers and instructions. Build both OCE images from
that checkout. The public
`latest` alias inspected during setup targets an older OpenShell integration.

Run from this HyperShell branch. These commands keep the default kubeconfig and
context unchanged. The private state directory contains credentials: do not
commit it, print its Secret files, or share it as troubleshooting evidence.
Use a new state directory for each fresh cluster. Reuse one only when resuming
the same cluster; installation receipts, database markers, and credentials from
a deleted cluster do not describe its replacement.
Paste one block at a time into Bash. The setup block disables interactive
`errexit` and `nounset`, including settings left by older versions of this
README. Operations run in guarded child shells with strict error handling:
a failed block stops its work and leaves your terminal open. Resolve its error
before continuing to another block.

```bash
# Keep command failures and missing variables from closing the interactive shell.
set +e +u
umask 077
export HYPERSHELL_REPO=/absolute/path/to/hypershell  # change this
export OCE_REPO=/absolute/path/to/openclaw-enterprise  # change this
export OCE_KIND_STATE="$HOME/.local/state/oce-hypershell-kind"  # choose a new path for a new cluster
export KUBECONFIG="$OCE_KIND_STATE/kubeconfig"
export CONTAINER_ENGINE=docker KIND_EXPERIMENTAL_PROVIDER=docker
export KIND_CLUSTER_NAME=hypershell-oce KIND_NO_SUDO=true
export KIND_HOT_RELOAD=false LOCAL_IMAGES=true
export CPK_LOG="$OCE_KIND_STATE/cpk.log"
# This Docker host exposes the development gateway on IPv4.
export CURL_HOME="$OCE_KIND_STATE/curl"
if bash -euo pipefail <<'PREPARE_STATE'
mkdir -p "$OCE_KIND_STATE"
chmod 700 "$OCE_KIND_STATE"
mkdir -p "$CURL_HOME"
printf 'ipv4\n' > "$CURL_HOME/.curlrc"
PREPARE_STATE
then
  echo 'Private state directory ready.'
else
  echo 'State directory setup failed. Do not continue; inspect the error above.' >&2
fi
```

## Create the cluster and enforce network policy

Use Kind's built-in `kindnetd` for pod networking and Kubernetes `NetworkPolicy`
enforcement, which Kind supports out of the box starting with
[Kind v0.24.0](https://github.com/kubernetes-sigs/kind/releases/tag/v0.24.0).
No additional networking plugin is needed. OCE's default-deny policies and
explicit allowances for Agent and gateway traffic remain enforced, so local
testing can catch missing or incorrect allowances.

Create a fresh cluster with this configuration. Existing clusters created with
a different networking plugin must be recreated to use this setup.

Precreate kind so the CNI is ready before `make kind-up` waits for infrastructure.
The Pod Security configuration exempts only the named OpenShell RuntimeClass;
OCE still applies restricted Pod Security to its tenant namespaces. Treat
Kubernetes administrator access and OpenShell operator credentials as privileged.

```bash
if bash -euo pipefail <<'CREATE_CLUSTER'
cp deploy/oce-kind/pod-security.yaml "$OCE_KIND_STATE/pod-security.yaml"
# Replace the template's __HYPERSHELL_REPO__ and __STATE__ placeholders with
# local paths so Kind can mount the repository and Pod Security configuration.
# Write the rendered config to "$OCE_KIND_STATE/kind.yaml" for cluster creation.
python3 - <<'PY'
import os
from pathlib import Path
root = Path(os.environ['HYPERSHELL_REPO'])
state = Path(os.environ['OCE_KIND_STATE'])
text = (root/'deploy/oce-kind/kind.yaml').read_text()
(state/'kind.yaml').write_text(text.replace('__HYPERSHELL_REPO__', str(root)).replace('__STATE__', str(state)))
PY
kind create cluster --name "$KIND_CLUSTER_NAME" --config "$OCE_KIND_STATE/kind.yaml"
kubectl -n kube-system rollout status daemonset/kindnet --timeout=5m
kubectl wait node --all --for=condition=Ready --timeout=3m
KIND_SKIP_SEED=true make kind-up
CREATE_CLUSTER
then
  echo 'Cluster and HyperShell infrastructure ready.'
else
  echo 'Cluster setup failed. Do not continue; inspect the error above.' >&2
fi
```

Expect healthy HyperShell API, controller, Console, Keycloak, PostgreSQL and
Vault workloads. Seeding is deferred because a gateway's workspace mode is
immutable. Do not create the first gateway until the next section is complete.

## Configure the HyperShell-owned gateway

`hsctl create gateway --operator-workspaces` selects operator mode at creation
using [PR #1016](https://github.com/openshift-online/hypershell/pull/1016).
The caller must hold `gateway:operator-workspace-creator` or `platform:admin`;
the local `admin` account qualifies. The controller [loads operator settings](../../components/control-plane/internal/config/config.go)
and carries them through the reconciler into the [Helm values builder](../../components/control-plane/internal/helm/values.go).
The remaining controller settings are administrator-owned:

| Setting                          | Owner      | Effect                                                                                                                             |
| -------------------------------- | ---------- | ---------------------------------------------------------------------------------------------------------------------------------- |
| `GATEWAY_OPERATOR_NAMESPACE_LABEL_KEY` | Controller | Scopes tenant namespace discovery to this gateway. |
| `GATEWAY_OPERATOR_SANDBOX_RUNTIME_CLASS` | Controller | Selects the operator sandbox RuntimeClass, which must exist before provisioning. |
| `GATEWAY_OPERATOR_WORKSPACE` | Controller | OCE trust settings: namespace label, allowed Pod labels, and explicit driver, resource-admission, and mTLS-auth permissions. |

The controller combines the configured label key with each gateway's own
namespace as the value. For this installation, OCE labels each tenant namespace
`hypershell.io/gateway=<gateway namespace>`. Other gateways cannot discover it
through their operator selector. The OCE overlay explicitly enables external driver
configuration, disables resource-approval labels for this trusted operator, and
preserves mTLS authentication alongside OIDC. Give this gateway only trusted
operator accounts: it can attach resources in the selected namespaces.
Operator-mode certificates also cover `*.openshell.localhost`, the internal
domain OpenShell advertises for sandbox services. This keeps Codex WebSocket
connections subject to hostname verification.

Prepare the RuntimeClass and OCE infrastructure, apply the controller trust
settings, then create `dev-gateway` through the authenticated `hsctl` flow.
Ordinary gateways keep the API's `managed` default.

```bash
if bash -euo pipefail <<'CONFIGURE_GATEWAY'
# The controller checks the operator RuntimeClass before provisioning the gateway.
bash deploy/oce-kind/prepare-oce.sh

# Configure namespace discovery, OCE's allowed Pod labels, and the operator-only
# openshell-sandbox RuntimeClass. Keep mTLS and external resource attachments enabled.
python3 - <<'PY'
import subprocess, yaml
from pathlib import Path

# Merge the controller patch, preserving other container settings.
for resource in yaml.safe_load_all(Path('deploy/oce-kind/hypershell-operator.yaml').read_text()):
    subprocess.run(['kubectl','-n','hypershell-system','patch','deployment',
                    resource['metadata']['name'],'--type=strategic','--patch',yaml.safe_dump(resource)], check=True)
PY

# Wait for the controller to run with the new settings before seeding.
# OpenShell images retain HyperShell's normal release defaults.
kubectl -n hypershell-system rollout status deployment/hypershell-controller

# Read this cluster's configured Keycloak hostname for authenticated seeding.
# Strip the URL scheme because kind-seed expects a hostname.
keycloak_host=$(kubectl -n keycloak get deployment keycloak -o json |
  jq -er '.spec.template.spec.containers[].env[]|select(.name=="KC_HOSTNAME")|.value|ltrimstr("https://")')

# Reuse authenticated seeding to build hsctl and create dev-gateway with:
#   hsctl create gateway --name dev-gateway --placement-selector provider/kind --operator-workspaces
# An existing operator gateway is reused; another mode is immutable and rejected.
make kind-seed SEED_OPERATOR_WORKSPACES=true SEED_STRICT=true KEYCLOAK_HOSTNAME="$keycloak_host"

# The controller provisions the gateway asynchronously and assigns its namespace.
# Poll every two seconds for exactly one gateway Deployment; never guess its namespace.
for attempt in {1..90}; do
  GATEWAY_NAMESPACE=$(kubectl get deployment -A --field-selector metadata.name=openshell-gateway -o json |
    jq -r '.items|if length==1 then first.metadata.namespace else empty end')

  # Continue once a unique match exists, or fail after the final attempt.
  [[ -z $GATEWAY_NAMESPACE ]] || break
  [[ $attempt != 90 ]] || { echo 'Gateway not provisioned; inspect controller logs'; exit 1; }
  sleep 2
done

# Wait for the gateway Deployment before saving the values for later steps.
kubectl -n "$GATEWAY_NAMESPACE" rollout status deployment/openshell-gateway --timeout=5m
printf '%s\n' "$GATEWAY_NAMESPACE" > "$OCE_KIND_STATE/gateway-namespace"
printf '%s\n' "$keycloak_host" > "$OCE_KIND_STATE/keycloak-host"
CONFIGURE_GATEWAY
then
  # Export the child shell's results into this interactive session.
  if read -r GATEWAY_NAMESPACE < "$OCE_KIND_STATE/gateway-namespace" &&
     read -r keycloak_host < "$OCE_KIND_STATE/keycloak-host"; then
    export GATEWAY_NAMESPACE keycloak_host
  else
    echo 'Could not load the gateway settings. Do not continue.' >&2
  fi
else
  echo 'Gateway setup failed. Do not continue; inspect the error above.' >&2
fi
```

Use `make kind-api-server-up` or `make kind-control-plane-up` for later code
changes, with the same exported cluster settings. `make kind-up` restores
ordinary Kind manifests and can recreate gateway ingress addresses; if rerun,
reapply these patches and reverify OIDC URLs and network peers. Do not manually
upgrade the gateway's Helm release:
HyperShell reconciles it with `--reset-values` and would overwrite those edits.

## Build and load OCE

In the OCE checkout, build the CLI and images using its build scripts, image
settings, and source manifest. If you opened a new terminal, repeat the
prerequisite exports first. A failure stops the build and leaves the error
visible in your terminal.

```bash
# Handle the child process's failure so the interactive shell stays open.
if bash -euo pipefail <<'BUILD_OCE'
bash deploy/oce-kind/build-oce.sh
BUILD_OCE
then
  echo 'OCE images built and loaded successfully.'
else
  echo 'OCE build/load failed. Do not continue; inspect the error above.' >&2
fi
```

Use the imported manifest digests, not Docker image configuration IDs. Both
controller and Agent runtime references remain immutable.

## Install OCE and connect it to OpenShell

OCE's installation CLI requires Helm 3. If the host has Helm 4, extract the
pinned Helm 3.14 binary from the HyperShell controller image built above. Isolate
plugins so host Helm 4 plugins cannot interfere:

```bash
if bash -euo pipefail <<'PREPARE_HELM'
mkdir -p "$OCE_KIND_STATE/bin" "$OCE_KIND_STATE/helm-plugins"
container=$(docker create localhost/hypershell-controller:dev)
docker cp "$container:/usr/local/bin/helm" "$OCE_KIND_STATE/bin/helm"
docker rm "$container"
export PATH="$OCE_KIND_STATE/bin:$PATH"
export HELM_PLUGINS="$OCE_KIND_STATE/helm-plugins"
helm version --short
PREPARE_HELM
then
  # Keep the working Helm installation available to subsequent steps.
  export PATH="$OCE_KIND_STATE/bin:$PATH"
  export HELM_PLUGINS="$OCE_KIND_STATE/helm-plugins"
else
  echo 'Helm setup failed. Do not continue; inspect the error above.' >&2
fi
```

If this is a new terminal, first [restore the installation environment](#resume-installation-in-a-new-terminal).
If the cluster was recreated, first [check for stale state](#state-left-over-from-a-deleted-cluster).
On first installation, the installer discovers the live gateway when
`GATEWAY_NAMESPACE` is unset.
You can rerun this block with the same state directory: an existing deployed
OCE release skips initialization and continues access setup. Run one copy of
the block at a time.

Run the supplied helpers in order:

```bash
if bash -euo pipefail <<'INSTALL_OCE'
# Report which helper failed, and stop before running any later helper.
trap 'printf "Failed command: %s\n" "$BASH_COMMAND" >&2' ERR
bash deploy/oce-kind/install-oce.sh
bash deploy/oce-kind/activate-oce.sh
bash deploy/oce-kind/browser-access.sh
INSTALL_OCE
then
  # Restore the namespace even when this terminal started without it.
  if read -r GATEWAY_NAMESPACE < "$OCE_KIND_STATE/gateway-namespace"; then
    export GATEWAY_NAMESPACE
    echo 'OCE installation and browser access ready.'
  else
    echo 'Could not load the installed gateway namespace. Do not continue.' >&2
  fi
else
  echo 'OCE setup failed. Do not continue; inspect the failed helper and saved installation receipt before retrying.' >&2
fi
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
and runs `occ installation install` with a durable receipt. On subsequent runs,
it verifies the cluster's identity and requires the existing Helm release to be
`deployed`. It preserves credentials, `installation.json`, and `values.json`,
including changes made by activation, and retrieves any missing bootstrap
credentials before continuing. Initialization is never replayed automatically.
A failed or pending Helm release, interrupted database setup, an existing
release without its receipt, or a mismatched cluster stops the helper for
diagnosis. Older state without a `cluster-uid` checkpoint must pass
`occ installation doctor` against its original
receipt before it can be adopted; an already replaced initialization Job cannot
establish that original attempt.
On first installation, if `GATEWAY_NAMESPACE` is unset, it discovers the single live OpenShell gateway
in `kind-hypershell-oce` and records its namespace in
`$OCE_KIND_STATE/gateway-namespace`. Discovery fails when there are zero or
multiple gateways; set `GATEWAY_NAMESPACE` explicitly to select among multiple
gateways. A saved namespace from an older cluster is not used for discovery.
The renderer reads the gateway's configured sandbox ServiceAccount name and
uses it for the tenant ServiceAccount.

The generated `installation.json` points at
`https://openshell-gateway.<gateway namespace>.svc:8080`. No second OpenShell
gateway is installed by OCE. Keep `installation.json`, `values.json`,
`images.json`, `cluster-uid`, and `install-receipt` with the private state directory.

`activate-oce.sh` pins the Envoy proxy, records its ready Pod addresses as
trusted peers, enables the OCE worker through the existing Installation update
flow, and grants OCE its tenant-local roles. It waits for the default Namespace
to become `ready`. This requires OCE to register its operator Workspace through
the HyperShell-created gateway with mTLS.

The RuntimeClass admission policy permits the Agent Sandbox controller to create
workloads and the owning OpenShell gateway to update them to release scheduling
gates. Workloads must use this gateway's labeled namespaces, the pinned OCE
runtime, and the OpenShell supervisor image from the gateway's configuration.
The bootstrap init container must use its configured OpenShell sandbox runtime
image. The renderer reads these image references from `openshell-gateway-config`,
so the allowlist follows HyperShell's selected release. After changing those
images, rerun `render-oce.py` and apply the generated
`$OCE_KIND_STATE/runtime-admission.yaml` before starting new Agent workloads.
The policy rejects privileged containers,
host namespaces and hostPath volumes. Kubernetes administrator credentials
remain privileged.

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
if bash -euo pipefail <<'VERIFY_OCE'
kubectl -n hypershell-system get deployments
kubectl -n "$GATEWAY_NAMESPACE" get deployments
kubectl -n oce-system get deployments
jq -e '.data.status=="ready"' "$OCE_KIND_STATE/namespace-proof.json"
curl --fail --silent --show-error --noproxy '*' \
  --cacert "$OCE_KIND_STATE/owner-tls/ca.crt" \
  --resolve console.hypershell-oce.oce.localhost:18443:127.0.0.1 \
  https://console.hypershell-oce.oce.localhost:18443/api/auth/session |
  jq -e '.data==null'
VERIFY_OCE
then
  echo 'OCE verification passed.'
else
  echo 'OCE verification failed. Do not continue; inspect the error above.' >&2
fi
```

## Deploy a Codex Agent

### Configure EnMaaS

Complete the OCE installation and activation above, then use these steps to run
Codex with an EnMaaS API key and `gpt-6-luna`. Continue in the same operator shell.

#### 1. Set the proxy endpoint

Set the Installation's
[shared model provider](https://github.com/openclaw/openclaw-enterprise/blob/2096c8cbde286c50ab7aafa08fd0f3bb3f4623c2/docs/reference/drivers/kubernetes-compute/model-provider.md).
This setting applies to new API-key deployments across the Installation and
currently has no Console editor.
The Agent's JSON `models.providers.codex.baseUrl` does not select this endpoint.
Keep that field at `http://127.0.0.1:9`; the OpenClaw gateway reaches Codex over
authenticated WebSocket transport.

```bash
# Send model API requests to EnMaaS using the Installation's shared provider.
export MODEL_BASE_URL='https://api.enmaas.devshift.net/v1'

# Reuse the private state directory from the OCE installation steps.
export RUN_DIR="$OCE_KIND_STATE"

# Select the Kind cluster, OCE namespace, Helm release, and local chart.
export CONTEXT=kind-hypershell-oce
export PLATFORM_NS=oce-system RELEASE=oce
export OCE_CHART="$OCE_REPO/deploy/helm/openclaw-enterprise"

# Point the operator helper at the saved configuration and admin credentials.
export INSTALLATION_FILE="$RUN_DIR/installation.json" VALUES_FILE="$RUN_DIR/values.json"
export OCC_SERVICE_KEY_FILE="$RUN_DIR/initial-admin-service-key.json"

# Stop on errors in a child shell while keeping this terminal open.
if bash -euo pipefail <<'CONFIGURE_MODEL_PROVIDER'
# Save the current settings before changing the shared model provider.
cp "$INSTALLATION_FILE" "$RUN_DIR/installation-before-model-provider.json"

# Set the provider's protocol and endpoint, preserving other installation settings.
# Write to a temporary file so jq never reads from the file it is overwriting.
jq --arg url "$MODEL_BASE_URL" \
  '.drivers.compute.configuration.runtime.modelProvider={protocol:"openai-responses",baseUrl:$url}' \
  "$INSTALLATION_FILE" > "$RUN_DIR/installation-next.json"
mv "$RUN_DIR/installation-next.json" "$INSTALLATION_FILE"

# Load OCE's operator functions and apply the updated installation configuration.
source "$OCE_REPO/docs/snippets/openshell-quickstart/operator-session.sh"
upgrade_installation
CONFIGURE_MODEL_PROVIDER
then
  # Load the functions in this shell too; child-shell functions do not persist here.
  if source "$OCE_REPO/docs/snippets/openshell-quickstart/operator-session.sh"; then
    echo 'Model provider configured.'
  else
    echo 'Could not load the operator functions. Do not continue.' >&2
  fi
else
  echo 'Model provider configuration failed. Do not continue; inspect the saved upgrade evidence.' >&2
fi
```

Wait for the OCE API and worker rollouts to complete.

#### 2. Register the EnMaaS API key

Generate an API key at the
[EnMaaS support page](https://devservices.dpp.openshift.com/support/enmaas/).

Complete **1. Set the proxy endpoint** before registering the key. Exporting
`MODEL_BASE_URL` alone does not configure OCE: the Installation upgrade must
succeed so its shared provider matches the credential source's destination.

Paste the generated key at the prompt below. Input is hidden, and the block
exports it as `ENMAAS_API_KEY` for the registration commands. The key is not
saved in shell history, and the variable disappears when the child shell exits.
Use names that are not already registered in the selected Namespace.
If Secret creation succeeded but credential-source registration failed, keep
`enmaas-secret.json` in the current state directory. After fixing the reported
error, rerun this block: it verifies and reuses that Secret without prompting
for the key again or changing its value.

```bash
# Run registration in a child shell so errors stop this step but leave the terminal open.
if bash -euo pipefail <<'REGISTER_MODEL_CREDENTIAL'
# Restore the operator settings here instead of relying on exports from step 1.
export RUN_DIR="${OCE_KIND_STATE:?Set OCE_KIND_STATE to the current cluster state directory}"
export KUBECONFIG="$RUN_DIR/kubeconfig" CONTEXT=kind-hypershell-oce
export PLATFORM_NS=oce-system RELEASE=oce
export OCC_SERVICE_KEY_FILE="$RUN_DIR/initial-admin-service-key.json"
export MODEL_BASE_URL='https://api.enmaas.devshift.net/v1'

# Load OCE's API helpers and select the Namespace created during activation.
source "${OCE_REPO:?Set OCE_REPO to your OCE checkout}/docs/snippets/openshell-quickstart/operator-session.sh"
namespace=$(cat "$RUN_DIR/namespace-id")
base="/namespaces/$namespace"

# Catch a skipped proxy configuration step before creating any credentials.
if ! jq -e --arg url "$MODEL_BASE_URL" \
  '.drivers.compute.configuration.runtime.modelProvider |
   .protocol == "openai-responses" and .baseUrl == $url' \
  "$RUN_DIR/installation.json" > /dev/null; then
  echo 'Complete step 1 and wait for the OCE rollouts before registering an EnMaaS credential.' >&2
  exit 1
fi

if [[ -s "$RUN_DIR/enmaas-secret.json" ]]; then
  # Resume a partial registration only after verifying the saved Secret still exists.
  secret_id=$(jq -er --arg namespace "$namespace" \
    '.data | select(.namespaceId == $namespace and .name == "enmaas-api-key") |
     .id | strings | select(length > 0)' "$RUN_DIR/enmaas-secret.json")
  request GET "$base/secrets/$secret_id" |
    jq -e --arg namespace "$namespace" --slurpfile saved "$RUN_DIR/enmaas-secret.json" \
      '.data | .namespaceId == $namespace and .name == "enmaas-api-key" and
       .ref == $saved[0].data.ref' > /dev/null
else
  # Keep the key out of command traces and read it without displaying it.
  set +x
  read -rsp 'EnMaaS API key: ' ENMAAS_API_KEY < /dev/tty
  printf '\n'
  export ENMAAS_API_KEY
  : "${ENMAAS_API_KEY:?An EnMaaS API key is required}"

  # Store the key as an OCE Secret. jq reads it from the environment, not an argument.
  # Keep a successful response so a later retry can reuse its Secret reference.
  jq -n '{name:"enmaas-api-key",value:env.ENMAAS_API_KEY}' |
    request POST "$base/secrets" --data-binary @- > "$RUN_DIR/enmaas-secret-next.json"
  mv "$RUN_DIR/enmaas-secret-next.json" "$RUN_DIR/enmaas-secret.json"
fi

# Register a credential source that uses this Secret only with the EnMaaS endpoint.
jq --arg url "$MODEL_BASE_URL" \
  '{name:"enmaas",type:"openai",config:{authorized_base_url:$url},secrets:{api_key:.data.ref}}' \
  "$RUN_DIR/enmaas-secret.json" |
  request POST "$base/credential-sources" --data-binary @- > "$RUN_DIR/enmaas-source.json"
REGISTER_MODEL_CREDENTIAL
then
  # Keep the state directory available in this shell for the remaining steps.
  export RUN_DIR="$OCE_KIND_STATE"

  # Confirm the source is ready, then export its ID for the Agent deployment steps.
  if read -r namespace < "$OCE_KIND_STATE/namespace-id" &&
     modelSource=$(jq -er '.data|select(.state=="ready")|.id' "$RUN_DIR/enmaas-source.json"); then
    base="/namespaces/$namespace"
    export namespace base modelSource
  else
    echo 'Model credential source is not ready. Do not continue.' >&2
  fi
else
  echo 'Model credential registration failed. Do not continue; inspect the error above.' >&2
fi
```

The source must authorize `https://api.enmaas.devshift.net/v1` explicitly.
If source registration returns HTTP 400 while the Secret was created, check
that step 1 completed: OCE rejects an EnMaaS destination while its shared
provider still uses OpenAI's default endpoint. Complete step 1, then rerun
this block using the saved Secret response.
Omitting `authorized_base_url` authorizes OpenAI's default endpoint and can
cause HTTP 401/403. Existing sources cannot change destination authorization;
register a replacement using their existing Secret reference instead of
creating another Secret.

#### 3. Select the model and bind the credential

Use `gpt-6-luna` as the Codex model ID. OCE's existing configuration template
adds the `codex/` prefix and retains the disabled HTTP fallback:

```bash
export MODEL=gpt-6-luna
if bash -euo pipefail <<'RENDER_MODEL_CONFIG'
jq -n --arg model "$MODEL" \
  -f "$OCE_REPO/docs/snippets/openshell-quickstart/codex-configuration.jq" \
  > "$RUN_DIR/enmaas-configuration.json"
RENDER_MODEL_CONFIG
then
  echo 'Agent model configuration rendered.'
else
  echo 'Agent model configuration failed. Do not continue; inspect the error above.' >&2
fi
```

For an existing Agent, update its Configuration so `agents.defaults.model` is
`codex/gpt-6-luna`, the matching `agents.defaults.models` entry selects the
`codex` runtime, and `models.providers.codex.models` contains ID `gpt-6-luna`.
Preserve its other settings; do not replace the whole Configuration with the
new-Agent template.

When creating the Agent in the OCE Console, select the saved `enmaas-api-key`
Secret and set **Authorized base URL** to `https://api.enmaas.devshift.net/v1`.
The form registers its own Credential Source from the selected Secret; it does
not automatically reuse the `enmaas` source from step 2. Leaving this field blank
authorizes OpenAI's default endpoint, which conflicts with the EnMaaS Installation
setting.

Check **Enable Native admin UI** before clicking **Create Agent**.
Once deployment creates the Agent's gateway Service, complete
[Apply the Agent network grants](#apply-the-agent-network-grants) before opening
chat. This is required for every new or recreated Agent, including Agents with
the same display name as one previously deployed.

If the form reports **Credential destination authorization must match the
Installation model provider**, click **Check credential registration**. Once it
confirms that no registration exists for this attempt, correct **Authorized base
URL** and click **Create Agent** again. Keep the selected Secret; no new API key
is needed.

Follow the matching OCE
[Agent deployment guide](https://github.com/openclaw/openclaw-enterprise/blob/2096c8cbde286c50ab7aafa08fd0f3bb3f4623c2/docs/guides/deploy/deploy-first-agent.md)
to create or update a dedicated Agent using the registered `modelSource` and
selected `MODEL`; skip its credential-registration step. Set
`harnessAuth.method` to `credential_source`, set `harnessAuth.sourceId` to
`$modelSource`, and include that source ID in `credentialSources`. Grant the Agent's principal
`operate` on that exact source, then deploy a new version.

### Apply the Agent network grants

Activation applies the tenant-level supervisor and private-route grants before
the first Agent starts. These include ingress to the HyperShell gateway on TCP
8080; the initial `oce-management-client` policy permits only the OCE API and
worker. Without the supervisor grant, startup policy fetches time out even
though supervisor egress is allowed. For an existing installation, rerun
`activate-oce.sh` with its original `OCE_KIND_STATE` and `OCE_REPO` before deploying
Agents. Set `OCE_NAMESPACE_NAME` when using a logical Namespace other than
`default`.

**Required before chat:** after the Agent's gateway Service exists, apply the
per-Agent network grants below using its current Agent ID from the Console URL.
Activation's tenant-level grants do not replace this step. An Agent can have
Ready pods and a successful deployment while its chat connection is blocked.
The OCE helper's `--openshell-namespace` option selects the HyperShell gateway.
No OCE image rebuild is needed when applying this helper fix to an existing
installation.

```bash
if bash -euo pipefail <<'GRANT_AGENT_NETWORK'
export KUBECONFIG="$OCE_KIND_STATE/kubeconfig" CONTEXT=kind-hypershell-oce
read -r GATEWAY_NAMESPACE < "$OCE_KIND_STATE/gateway-namespace"
namespace=$(cat "$OCE_KIND_STATE/namespace-id")
agent='<Current Agent ID from the Console URL, starting with agt_>'
tenant=$(kubectl get namespace -l "openclaw.dev/namespace=$namespace" -o json |
  jq -er '.items|select(length==1)|first.metadata.name')
python3 "$OCE_REPO/docs/snippets/openshell-quickstart/kubernetes/service-policies.py" \
  --tenant "$tenant" --namespace-id "$namespace" --agent-id "$agent" \
  --openshell-namespace "$GATEWAY_NAMESPACE" > "$OCE_KIND_STATE/agent-service-peers.json"
kubectl apply -f "$OCE_KIND_STATE/agent-service-peers.json"
kubectl -n "$tenant" get pods
GRANT_AGENT_NETWORK
then
  echo 'Agent network grants applied.'
else
  echo 'Agent network setup failed. Do not continue; inspect the error above.' >&2
fi
```

If the renderer reports that the gateway Service is absent, wait for the first
deployment to create it and rerun the command. These grants cover the Agent
gateway, OpenShell management, supervisor, DNS, and private Envoy route. They
retain OCE's default-deny policy. Repeat for each new or recreated Agent with
its new ID: grants for a deleted Agent do not apply to its replacement, even
when the display name is unchanged. Rerun after a redeployment replaces its
gateway Service, because the policies also include that Service's IP. Keep
these grants scoped to the Agent; do not substitute a namespace-wide allow rule.
For another logical Namespace, use its ID instead of the saved default.

If chat times out and the Agent gateway logs report `codex app-server remote
WebSocket connection failed` with `Opening handshake has timed out`, check that
these grants select the current Agent ID. Missing grants block the Agent gateway
from reaching OpenShell on TCP 8080. Rerun the block with the current ID, then
retry chat; this network repair does not require an image rebuild or redeployment.

If a deployment already failed, inspect its error before
deploying another version. A supervisor that already exited has
`restartPolicy: Never`; network grants alone do not restart that failed Pod.

During verification on 2026-11-06, the OpenClaw gateway, Codex sandbox and
supervisor all reached Kubernetes Ready. The sandbox enforced Landlock, Codex
passed its model probe, and OCE activated the Agent revision. A request through
the private OpenClaw WebSocket returned `OCE_READY` using the configured HTTPS
Responses proxy. Live admission checks accepted the pinned controller workload
and rejected both an unapproved image and Pod creation by the gateway identity.

An earlier deployment failed with `RUNTIME_AUTHENTICATION_FAILED` because a
proxy key was sent to OpenAI's default endpoint. Correct both the Installation
provider and the source's destination authorization before deploying a new
version. The held authentication failure does not retry automatically.

The deployment verified on 2026-11-06 uses OCE Namespace `hypershell`; the
initial empty `default` Namespace was deleted after diagnosing a certificate
role error. This runbook includes that correction. On 2026-11-09,
`make kind-up-with-oce` completed on a fresh cluster with built-in Kind networking
and the `default` OCE Namespace. The Codex Agent returned `OCE_READY` using
`gpt-6-luna`, and external OpenShell gRPC health passed. That run retained an old
`OCE_KIND_STATE` argument and automatically selected fresh state.
To rerun tenant permission checks for an existing named Namespace, set
`OCE_NAMESPACE_NAME=hypershell` when running `activate-oce.sh`.

## Recovery and removal

### Resume installation in a new terminal

Shell exports disappear when a terminal closes. From the HyperShell repository
root, restore the OCE checkout and the state directory belonging to the current
cluster. If you prepared a new state directory during recovery, use that path
here. Do not rerun cluster creation or gateway provisioning just to restore
these variables.

```bash
# Clear strict settings left in the interactive shell by older instructions.
set +e +u
export OCE_REPO=/absolute/path/to/openclaw-enterprise  # change this
export OCE_KIND_STATE=/absolute/path/to/current-cluster-state  # change this
export KUBECONFIG="$OCE_KIND_STATE/kubeconfig"

# Restore the private Helm 3 binary and isolated plugin directory.
export PATH="$OCE_KIND_STATE/bin:$PATH"
export HELM_PLUGINS="$OCE_KIND_STATE/helm-plugins"

# Let the installer discover the current gateway, instead of retaining an old namespace.
# With multiple gateways, explicitly export the intended GATEWAY_NAMESPACE instead.
unset GATEWAY_NAMESPACE

if bash -euo pipefail <<'CHECK_RESUME'
# Verify access before retrying installation. This does not change the cluster.
test -s "$KUBECONFIG"
kubectl --context kind-hypershell-oce get nodes
helm version --short
CHECK_RESUME
then
  echo 'Environment restored. Rerun the guarded helper block with the current cluster state directory.'
else
  echo 'Environment check failed. Do not continue; inspect the error above.' >&2
fi
```

Rerun the complete guarded helper block in
[Install OCE and connect it to OpenShell](#install-oce-and-connect-it-to-openshell).
Keep the surrounding `if bash ... fi`: it stops at the failed helper while
leaving your terminal open. The installer checks existing state automatically.
If it reports a failed or uncertain initialization, inspect the receipt with
`occ installation doctor` below before deciding how to recover.

### State left over from a deleted cluster

A replacement cluster can have the same name but a different gateway namespace,
database, and credentials. Its predecessor's `install-receipt`, `database-ready`,
`bootstrap-volume-ready`, and `cluster-uid` files are not evidence that setup ran
on the new cluster. Do not delete those files to bypass the installer's checks.

The error `Kubernetes connection does not match the recorded installation target`
means the receipt was recorded against a different Kubernetes connection. The
installer prints the state directory and kubeconfig it selected. It always uses
`$OCE_KIND_STATE/kubeconfig`, so changing only `KUBECONFIG` does not fix this.

**If you already prepared a new state directory**, use its path in the
[environment restoration block](#resume-installation-in-a-new-terminal), then
rerun the guarded installation block. Paste those exports into your current
terminal before retrying: the installer cannot change its parent shell's
environment. Keep using that same directory for later steps and reruns.

**If you intended to resume the original cluster**, restore the kubeconfig and
state directory for that cluster. A changed connection is not proof that the
original cluster was deleted. Other `occ installation doctor` errors, such as a
failed Job or lost connectivity, need their reported problem resolved first.

**If the cluster was recreated and you have no new state directory yet**, and
the replacement cluster has no OCE installation or OCE database yet:

1. Create a new private state directory with mode `700`. Preserve the old
   directory in place: the running kind node may still mount its
   `pod-security.yaml` from there.
2. Copy only the current cluster's `kubeconfig` and reusable build artifacts:
   `occ`, `images.json`, `controller-image`, and `runtime-image`.
   The images must already be loaded into the replacement cluster; otherwise
   repeat **Build and load OCE**. Do not copy receipts, `cluster-uid`, readiness markers,
   rendered installation files, saved gateway namespaces, or credentials.
3. Use the new directory in the environment block above, repeat the Helm setup
   block to prepare its binary and plugin directory, then run the guarded
   installation helper block. The installer discovers the gateway and generates
   fresh configuration and credentials.

For an existing or partially installed OCE deployment on the current cluster,
keep its state directory and diagnose the attempt before retrying. Switching to
an empty directory would lose the information needed to resume safely.

### Inspect an installation

Read readiness and events before retrying:

```bash
if bash -euo pipefail <<'INSPECT_OCE'
kubectl get pods -A
kubectl -n hypershell-system logs deployment/hypershell-controller --tail=60
# Include gateway events even if this terminal has no saved gateway namespace.
kubectl get events -A --sort-by=.lastTimestamp
kubectl -n oce-system get jobs,pods
"$OCE_KIND_STATE/occ" installation doctor --kubeconfig "$KUBECONFIG" \
  --context kind-hypershell-oce --kube-namespace oce-system --release oce \
  --receipt "$OCE_KIND_STATE/install-receipt"
INSPECT_OCE
then
  echo 'Diagnostics complete.'
else
  echo 'A diagnostic command failed; inspect its error above.' >&2
fi
```

The OpenShell client certificate copied into `oce-system` is not synchronized
automatically. Before its 30-day expiry, wait for cert-manager renewal, copy the
renewed leaf Secret using the command in `install-oce.sh`, and restart OCE API
and worker. Preserve the gateway CA, database and signing keys across restarts.

### Remove the installation

To delete this installation and all its databases and volumes, first confirm
the selected cluster name, then run:

```bash
if bash -euo pipefail <<'REMOVE_OCE'
kind get clusters
kind delete cluster --name hypershell-oce
docker stop hypershell-oce-dns
docker rm hypershell-oce-dns
# If you used the user service:
systemctl --user stop oce-hypershell-console
REMOVE_OCE
then
  echo 'OCE cluster and helpers removed.'
else
  echo 'Removal stopped after an error; inspect which resources remain before retrying.' >&2
fi
```

Stop this installation's port forwards separately. Retain the private state
directory for diagnosis or delete it deliberately after confirming no credentials
or evidence need to be retained. Do not use a different cluster's kubeconfig.
