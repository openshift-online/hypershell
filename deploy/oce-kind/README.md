# Run HyperShell and OCE with one command

`make kind-up-with-oce` creates a local development environment containing
HyperShell, OpenClaw Enterprise (OCE), and a HyperShell-managed OpenShell gateway.
It also creates a working `codex-test` Agent in OCE, configured to use
`gpt-6-luna` through EnMaaS, and checks it with a real model request.

You supply a file containing your EnMaaS API key. The command pulls the latest
OCE controller and runtime images from Quay and handles the cluster, credentials,
integration, and browser access. It manages its own state directory and leaves your default
kubeconfig and Kubernetes context unchanged.

HyperShell uses registry images by default, just like `make kind-up`. Set
`LOCAL_IMAGES=true` to build HyperShell images from your working tree:

```shell
make kind-up-with-oce LOCAL_IMAGES=true \
  ENMAAS_API_KEY_FILE="$HOME/enmaas-key.txt"
```

With local images enabled, `BUILD_SOURCE=baseline` builds from `origin/main`, and
`KIND_SKIP_BUILD=true` reuses existing local images. `KIND_COVERAGE=true` implies
local builds. These settings apply to the initial HyperShell bring-up; reruns
preserve an already initialized HyperShell installation. OCE images come from Quay
in either mode.

This is a disposable, single-node development environment. HyperShell uses its
standard development credentials, and Docker publishes the kind load-balancer
ports on host interfaces. Use a firewall or an isolated development network.
The OCE browser forward listens only on loopback.

## Quick start

From the HyperShell repository root, run:

```shell
make kind-up-with-oce \
  ENMAAS_API_KEY_FILE="$HOME/enmaas-key.txt"
```

| Input | What to provide |
| --- | --- |
| `ENMAAS_API_KEY_FILE` | Path to a readable file containing one EnMaaS API key. A trailing newline is fine. Supply the path, not the key itself. |

This is the only required configuration input. **You do not need to set
`OCE_KIND_STATE`**, choose image tags, install Helm on the host, or run the manual
setup steps. The command uses Docker and does not prompt for sudo. Each
successful run includes a small inference request charged to your EnMaaS key.

### Prerequisites

Use a Linux host with Docker running and the following tools on `PATH`:

- kind 0.31.0, kubectl, and kustomize.
- Git, Go, and Node.js for the OCE CLI and operator helpers.
- jq, curl with HTTP/2 support, OpenSSL, and `flock` from util-linux.
- Python 3.11 or newer with PyYAML, plus GNU Make.

Allow about 16 GiB of RAM for the running stack, with additional memory and disk
space for image builds. Builds need access to GitHub, container registries, and
package registries; the model check needs access to EnMaaS. The setup uses
Kubernetes 1.35.0 and HyperShell's normal release defaults from
[`OPENSHELL_VERSION`](../../OPENSHELL_VERSION) and the [Makefile](../../Makefile).
Helm 3 is extracted automatically from the deployed HyperShell controller's image,
which Docker pulls if it is not already available locally.

OCE images come from `quay.io/aipcc/base-images/agentic/oce-controller:latest`
and `quay.io/aipcc/base-images/agentic/oce-runtime:latest`. A new installation
resolves both tags to immutable digests. Reruns keep those digests rather than
upgrading an existing installation.

The command fetches the controller image's OCE source revision into
`$OCE_KIND_STATE/oce-source` for the matching `occ` CLI, Helm chart, and operator
helpers. No preexisting checkout is required. It applies the namespace helper
fix from [OCE PR #4954](https://github.com/openclaw/openclaw-enterprise/pull/4954)
using [`openshell-namespace.patch`](openshell-namespace.patch), pinned to the
helper changes in commit `080810ac2a05b98f065655a9b29cdc9ec28ab14e`. The patch
is skipped when the source already supports `--openshell-namespace`. Patch
conflicts stop setup rather than continuing with incorrect network grants.
An explicit `OCE_REPO` remains available for engineers supplying compatible
local CLI, chart, and helpers; OCE images still come from Quay. The manual
runbook retains the separate source-image build workflow.

## What gets created

All components run in the Docker kind cluster **`hypershell-oce`**:

| Component | Responsibility |
| --- | --- |
| HyperShell | Provides its API and web console, backed by development Keycloak and PostgreSQL. Provisions the shared `dev-gateway` OpenShell gateway and its database. |
| OpenShell gateway | Runs Agent harnesses for OCE in operator workspace mode. HyperShell continues to own and reconcile it. |
| OCE | Runs its API, workers, and browser listener in `oce-system`. Manages tenant namespaces, Agent configuration, deployments, and per-Agent OpenClaw gateways. |
| `codex-test` Agent | Runs Codex in dedicated execution mode with `gpt-6-luna`, scoped EnMaaS credentials, and the native Agent UI enabled. |

The cluster uses kind's built-in networking and NetworkPolicy enforcement.
Runtime admission rules constrain the workloads OCE can launch. Network grants
allow the required Agent and gateway connections; the shared gateway's external
access is limited to the kind ingress proxy's current IP addresses.

## How the command works

The Makefile invokes [`up.sh`](up.sh), which runs these stages in order:

1. **Check inputs and select state.** Validate the tools and key file.
   Take a setup lock and select the private directory belonging to the
   running cluster, or allocate a fresh directory for a new cluster.
2. **Start HyperShell.** Create kind using [`kind.yaml`](kind.yaml), then run
   the existing `make kind-up` workflow with seeding deferred. It uses registry
   images by default and honors the same local-build settings as `make kind-up`.
   Prepare the operator RuntimeClass and controller trust settings, then create
   `dev-gateway` with `hsctl create gateway --operator-workspaces` through
   `make kind-seed SEED_OPERATOR_WORKSPACES=true SEED_STRICT=true`. Wait for the
   gateway to become ready. Reruns reuse an existing operator gateway and reject
   an existing gateway in another mode; the API default stays `managed`.
3. **Prepare OCE.** [`pull-oce.sh`](pull-oce.sh) pulls the latest controller and
   runtime images from Quay into Docker and kind, fetches the matching source,
   applies the namespace helper fix, and builds the `occ` CLI. It records
   immutable image digests for the installation.
4. **Install and connect OCE.** [`install-oce.sh`](install-oce.sh) prepares
   configuration, credentials, the
   database, and the Helm installation. [`activate-oce.sh`](activate-oce.sh)
   connects OCE to the HyperShell-owned gateway and configures tenant access.
   [`browser-access.sh`](browser-access.sh) sets up the OCE HTTPS listener.
5. **Create and test Codex.** [`seed-agent.sh`](seed-agent.sh) configures the
   EnMaaS Responses endpoint and registers the key. It uses OCE's provisioning
   API to create `codex-test`, its configuration, permissions, and first
   deployment. It applies the Agent's network grants, waits for the active
   revision, and requires a reply containing `OCE_READY`.
6. **Verify and print access details.** [`access.sh`](access.sh) starts or
   reuses the loopback OCE forward, verifies OCE HTTPS, HyperShell login, and
   external OpenShell gRPC health, then prints the final summary.

A failed stage stops the command and identifies the state directory. Resolve
the reported error, then rerun the same command with the same key file.

## Open the consoles and test the Agent

The final summary groups the live URLs and login details into **HyperShell**,
**OpenShell gateway**, **OCE and Codex**, and **Local access** sections. Like
`make kind-up`, it uses color in a terminal and honors `NO_COLOR`. Redirected
output and the saved `access.txt` remain plain text.

- **HyperShell:** open the printed web console URL and log in with
  `admin / admin` or `developer / developer`. The assigned host port can change
  when you recreate the cluster, so use the URL from the latest run.
- **OpenShell gateway:** use the printed API endpoint with the OpenShell CLI.
  Set `SSL_CERT_FILE` to the printed HyperShell CA file.
- **OCE:** open `https://console.hypershell-oce.oce.localhost:18443/console/` and
  log in as `admin@oce.localhost`. Read the password from the printed
  `initial-admin-password` file; the command does not print it.
- **Codex:** follow the test Agent link, then choose **Open Agent UI** to chat.

Trust the two printed local CA certificates in your browser for HyperShell and
OCE HTTPS. If the cluster runs on a remote host, SSH-forward the printed
HyperShell port and port `18443` to your laptop and use the same hostnames.

To retrieve the summary later:

```shell
cat "${XDG_STATE_HOME:-$HOME/.local/state}/oce-hypershell-kind-current/access.txt"
```

## Reruns, state, and source changes

Setup stores credentials, its private kubeconfig, image references, and install
receipts under `${XDG_STATE_HOME:-$HOME/.local/state}`. The symlink
`oce-hypershell-kind-current` points to the selected installation. These files
contain secrets: keep them private and outside version control.

On a rerun, setup matches saved state to the live cluster's Kubernetes identity.
It resumes incomplete work, reuses completed builds, and restores the OCE
forward if needed. It does not create another test Agent for that installation.
After cluster deletion, the next run allocates a fresh directory and fresh
credentials while preserving the old files. A stale `OCE_KIND_STATE` export or
make argument does not prevent this automatic recovery.

**Rerunning is not an image upgrade or a key rotation.** Completed HyperShell
baseline deployment and OCE builds are reused. To test changed source with a
clean installation, delete the disposable cluster as described below and rerun.
For an existing installation, use the individual build and install operations
in the [manual runbook](manual-setup.md). Changing the key file does not replace
the deployed model credential; use OCE's credential rotation flow.

## Troubleshooting

| Symptom | Next step |
| --- | --- |
| Setup stops partway through | Read the stage and error above the final failure message. Resolve it and rerun the same make command. State is preserved. |
| Port `18443` is occupied | Inspect the listener. Setup reuses a forward only if its HTTPS response validates against this installation's CA; it does not replace an unrelated process. |
| No saved installation matches the running cluster | Recover the cluster's original state. Setup refuses to adopt an unrecorded cluster or guess between multiple matching copies. |
| Codex deployment fails | Inspect the deployment in OCE and the saved `codex-provision-status.json`. Fix its reported error before retrying in OCE; the command does not create repeated revisions to mask a failure. |
| A credential write is reported as unconfirmed | Read back the OCE resource before retrying, as explained below. Do not delete a pending record blindly. |

If you moved state outside the managed directory or have multiple copies, pass
`OCE_KIND_STATE=/absolute/path/to/original-state` once to select it. Setup verifies
its cluster identity and remembers the choice. This is a recovery override;
normal setup and recreation do not require it.

The Secret and credential-source APIs do not provide idempotency keys. If a
response is lost, setup retains `codex-secret.json.pending` or
`codex-source.json.pending` and stops. Read back the corresponding resource and
save its confirmed response as the matching `.json` file. Remove a pending
record only after confirming that the resource was not created. Agent
provisioning uses a saved request ID and can resume after a lost response.

For individual commands, database checks, networking diagnosis, and installation
receipts, see the [manual setup and recovery runbook](manual-setup.md).

## Remove or recreate the environment

`make kind-teardown` removes both `hypershell-dev` and `hypershell-oce` when
present, including their in-cluster databases and volumes, and cleans up their
DNS containers and port forwarding. Saved OCE state remains on disk.

The following command deletes the `hypershell-oce` cluster, including its
in-cluster databases and volumes:

```shell
kind delete cluster --name hypershell-oce
```

Stop any remaining forwards for this installation and remove the
`hypershell-oce-dns` Docker container if present. See the
[manual removal instructions](manual-setup.md#remove-the-installation) for the
individual cleanup commands. Keep the state directory for diagnosis or remove
it deliberately when its credentials and records are no longer needed.

Run `make kind-up-with-oce` again with the same key file to create a fresh
installation. There is no state directory to rename or reset first.

## Validation and current limits

On 2026-11-09, the automated command completed on a fresh cluster with the
built-in kind networking and the default OCE Namespace. The `gpt-6-luna` Agent
returned `OCE_READY`, and external OpenShell gRPC health passed. That run retained
an old `OCE_KIND_STATE` argument and correctly selected fresh state.

On 2026-11-12, the Quay image workflow passed the OCE shell tests, repository
policy checks, and shell lint. The namespace patch applied to controller source
`5f498c7ec416e3a1d3b012f8644204edb13b477e`, and its `occ` CLI built successfully.
The Quay workflow has not yet been replayed on a fresh cluster with a live model request.

This integration is experimental and provides no high availability. The copied
OpenShell client certificate needs renewal synchronization before its 30-day
expiry; the local OCE browser CA also expires after 30 days. For a long-lived
environment, follow the [certificate guidance](manual-setup.md#inspect-an-installation)
and renew browser certificates. The live checks verify HTTP, gRPC, and a model
reply; they do not exercise interactive browser login or chat.
