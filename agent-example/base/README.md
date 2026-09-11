# Generic Agent Base

A reusable kustomize base and shell library for building CronJob-based agents
that consume OpenShell sandboxes. Teams building new agents (CI bots, security
scanners, test runners, data pipelines) use this as a starting point instead of
reverse-engineering a working agent from scratch.

See `bases/agent-example/` for a complete, deployable example built on this base.

## What this base provides

| Artifact | Purpose |
|----------|---------|
| `base/cronjob.yaml` | Parameterized CronJob skeleton with proven volume layout |
| `base/scripts/agent-lib.sh` | Shell library sourced by every agent's `run.sh` |
| `base/scripts/install-openshell.sh` | Init container: download + verify openshell binary |
| `base/policy/policy-template.yaml` | Annotated sandbox policy skeleton |
| `base/provider/provider-template.yaml` | Annotated provider credential bundle skeleton |
| `base/namespace.yaml` | Namespace with Pod Security Standards label |
| `base/serviceaccount.yaml` | ServiceAccount with no automounted token |
| `base/rbac.yaml` | Minimal Role/RoleBinding (read OIDC secret) |

## Volume layout

Every agent CronJob uses the same five volumes:

| Volume | Mount | Mode | Purpose |
|--------|-------|------|---------|
| `runner` | `/opt/agent` | ro | Agent scripts via ConfigMap (`agent-lib.sh` + overlay's `run.sh`) |
| `policy` | `/etc/agent` | ro | Sandbox policy file via ConfigMap |
| `tools` | `/tools` | rw | openshell binary written by init container |
| `work` | `/work` | rw | Per-sandbox HOME / gateway state / intermediate files |
| `tmp` | `/tmp` | rw | Scratch space |

## Building an agent in 5 steps

### Step 1 - register a provider

A provider grants sandboxes access to an external API. Do this once in the gateway
workspace before deploying the agent.

```bash
# Upload the credential to the gateway secret store
openshell secret set my-service-token --value "$MY_SERVICE_TOKEN"

# Copy and fill in bases/agent/base/provider/provider-template.yaml
# then register it
openshell provider create --file provider.yaml
```

### Step 2 - write a policy file

Copy `bases/agent/base/policy/policy-template.yaml` into your overlay and set the
gateway hostname, OIDC issuer host, and any API endpoints your workload calls.

### Step 3 - write run.sh

Your `run.sh` sources `agent-lib.sh` and calls the lifecycle functions in order:

```bash
#!/usr/bin/env bash
set -euo pipefail

source /opt/agent/agent-lib.sh

CONFIG_ROOT=$(new_config_root /work "$SANDBOX_NAME")

# Auth
configure_gateway "$CONFIG_ROOT"
renew_gateway_login "$CONFIG_ROOT"
_start_token_refresh_loop "$CONFIG_ROOT"

# Pre-flight
cleanup_stale_sandboxes "$CONFIG_ROOT" "managed-by=my-agent"
verify_providers "$CONFIG_ROOT" my-service-provider

# Sandbox lifecycle
create_sandbox "$CONFIG_ROOT" "$SANDBOX_NAME" "$SANDBOX_SOURCE" \
    --provider my-service-provider \
    --policy   /etc/agent/policy.yaml \
    --label    "managed-by=my-agent"

register_cleanup "$CONFIG_ROOT" "$SANDBOX_NAME"
wait_for_sandbox "$CONFIG_ROOT" "$SANDBOX_NAME"

# Workload
exec_in_sandbox "$CONFIG_ROOT" "$SANDBOX_NAME" 300 -- /bin/sh -c 'echo hello'
```

### Step 4 - create a kustomize overlay

```yaml
# my-agent/kustomization.yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

resources:
  - ../../agent/base

namePrefix: my-agent-
namespace: my-agent-ns

patches:
  - target:
      kind: CronJob
      name: agent
    patch: |
      apiVersion: batch/v1
      kind: CronJob
      metadata:
        name: agent
      spec:
        schedule: "*/30 * * * *"
        jobTemplate:
          spec:
            template:
              spec:
                initContainers:
                  - name: install-openshell
                    env:
                      - name: OPENSHELL_VERSION
                        value: "v0.8.2"
                      - name: OPENSHELL_SHA256_AMD64
                        value: "<sha256>"
                containers:
                  - name: agent
                    env:
                      - name: GATEWAY_ENDPOINT
                        value: "https://gateway.myorg.com"
                      - name: GATEWAY_NAME
                        value: "my-gateway"
                      - name: OIDC_ISSUER_URL
                        value: "https://sso.myorg.com/realms/myorg"
                      - name: OIDC_CLIENT_ID
                        value: "my-agent"

configMapGenerator:
  - name: agent-scripts
    behavior: merge
    files:
      - scripts/run.sh
    options:
      disableNameSuffixHash: true
  - name: agent-policy
    behavior: replace
    files:
      - policy/policy.yaml
    options:
      disableNameSuffixHash: true
```

### Step 5 - create the OIDC secret

The CronJob reads `OIDC_CLIENT_SECRET` from a Secret named `agent-oidc` in the agent namespace:

```bash
kubectl create secret generic agent-oidc \
  --namespace my-agent-ns \
  --from-literal=client-secret="$MY_OIDC_CLIENT_SECRET"
```

## agent-lib.sh function reference

### Gateway lifecycle

| Function | Description |
|----------|-------------|
| `configure_gateway <config_root>` | Register gateway endpoint in isolated config root |
| `renew_gateway_login <config_root> [force]` | Authenticate; pass `force` to skip cache |
| `_start_token_refresh_loop <config_root>` | Background loop refreshing every `TOKEN_REFRESH_INTERVAL` seconds |

### Provider verification

| Function | Description |
|----------|-------------|
| `verify_providers <config_root> <name>...` | Assert all named providers exist; exit non-zero if any missing |

### Sandbox lifecycle

| Function | Description |
|----------|-------------|
| `create_sandbox <config_root> <name> <source> [flags...]` | Create sandbox with `--keep`; pass `--provider`, `--policy`, `--upload`, `--label` as needed |
| `wait_for_sandbox <config_root> <name> [timeout]` | Poll until Running or timeout (default: `SANDBOX_TIMEOUT`) |
| `exec_in_sandbox <config_root> <name> <timeout> [--env K=V]... -- cmd` | Run command; streams output |
| `exec_detached_in_sandbox <config_root> <name> <status_file> <poll> <timeout> -- cmd` | Run detached via `setsid --fork`; poll status file for completion |
| `delete_sandbox <config_root> <name>` | Delete sandbox (idempotent) |

### Cleanup and isolation

| Function | Description |
|----------|-------------|
| `register_cleanup <config_root> <sandbox_name>` | Register EXIT/INT/TERM traps to delete sandbox + stop refresh loop |
| `cleanup_stale_sandboxes <config_root> <label_selector>` | Delete sandboxes matching selector (call at start of each run) |
| `new_config_root <base_dir> <name>` | Create isolated HOME/.config/.local under base_dir; returns path |

## Environment variables

| Variable | Default | Description |
|----------|---------|-------------|
| `GATEWAY_ENDPOINT` | (required) | Gateway base URL |
| `GATEWAY_NAME` | (required) | Gateway name in workspace |
| `OIDC_CLIENT_ID` | (required) | OIDC client ID |
| `OIDC_CLIENT_SECRET` | (required, from Secret) | OIDC client secret |
| `OIDC_ISSUER_URL` | (required) | OIDC token endpoint base URL |
| `OPENSHELL_BIN` | `/tools/openshell` | Path to openshell binary |
| `TOKEN_REFRESH_INTERVAL` | `300` | Seconds between gateway login refreshes |
| `SANDBOX_TIMEOUT` | `120` | Seconds to wait for sandbox ready |
| `AGENT_LABEL` | `managed-by=agent` | Label applied to sandboxes for stale cleanup |

## Security defaults

- `runAsNonRoot: true`, `runAsUser: 1000`
- `allowPrivilegeEscalation: false`, capabilities `drop: ALL`
- `seccompProfile: RuntimeDefault`
- `automountServiceAccountToken: false`
- Both volumes carrying scripts are mounted read-only
- Each sandbox run gets an isolated config root under `/work` so parallel
  agents cannot stomp each other's gateway tokens
