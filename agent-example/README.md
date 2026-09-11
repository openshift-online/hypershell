# agent-example

A complete, deployable example agent built on `bases/agent/base/`. Use this as a
concrete reference when building a new agent - copy the overlay structure, replace
the placeholders, and add your workload logic to `run.sh`.

## What's here

```
bases/agent-example/
  kustomization.yaml          # Overlay on bases/agent/base; set schedule + env here
  scripts/
    run.sh                    # Example agent entrypoint (sources agent-lib.sh)
  policy/
    policy.yaml               # Sandbox policy: filesystem, network, process rules
  provider/
    provider.yaml             # Provider credential bundle (register in gateway workspace)
```

The base provides `agent-lib.sh`, `install-openshell.sh`, the CronJob skeleton,
RBAC, namespace, and ServiceAccount. This overlay adds the schedule, workload
env vars, `run.sh`, and the concrete policy.

## What it does

1. Authenticates with the gateway using OIDC client credentials
2. Cleans up sandboxes left by any previously crashed run (idempotent)
3. Verifies the `my-service-provider` provider is registered in the workspace
4. Creates a sandbox sourced from `ubi9`, waits for it to become Ready
5. Runs a hello-world workload inside the sandbox (replace with your logic)
6. Deletes the sandbox on exit via a cleanup trap

## Before you deploy

### 1. Register the provider

```bash
# Upload the API token to the gateway secret store
openshell --gateway example-gateway secret set my-service-token \
    --value "$MY_SERVICE_TOKEN"

# Register the provider
openshell --gateway example-gateway provider create \
    --file bases/agent-example/provider/provider.yaml
```

### 2. Create the OIDC secret

```bash
kubectl create secret generic example-agent-oidc \
    --namespace agents-example \
    --from-literal=client-secret="$OIDC_CLIENT_SECRET"
```

> The CronJob reads from a Secret named `agent-oidc` in its namespace. The
> `namePrefix: example-` in the overlay turns that into `example-agent-oidc`.

### 3. Patch the openshell SHA256

In `kustomization.yaml`, replace `REPLACE_WITH_ACTUAL_SHA256` with the SHA256
digest of the openshell binary for your target version. Find digests in the
[release notes](https://github.com/openshift-online/openshell/releases).

### 4. Set your gateway endpoint

In `kustomization.yaml`, replace:
- `gateway.example.com` with your actual gateway hostname
- `sso.example.com/realms/example` with your OIDC issuer URL
- `example-gateway` with the gateway name in your workspace
- `example-agent` with your OIDC client ID

And in `policy/policy.yaml`, update the `network.egress` hosts to match.

## Deploy

```bash
kubectl apply -k bases/agent-example/
```

Or reference this overlay from a cluster's GitOps config:

```yaml
# clusters/my-cluster/apps/example-agent/kustomization.yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - ../../../../bases/agent-example
```

## Customizing for a real agent

| To... | Edit... |
|-------|---------|
| Change the workload | Replace the `exec_in_sandbox` body in `scripts/run.sh` |
| Add more providers | Add `--provider <name>` flags to `create_sandbox` in `run.sh` and extra entries in `policy/policy.yaml` network.egress |
| Use long-running work | Replace `exec_in_sandbox` with `exec_detached_in_sandbox` (see `agent-lib.sh`) |
| Adjust the schedule | Change `schedule:` in the patch in `kustomization.yaml` |
| Use a different sandbox source | Set `SANDBOX_SOURCE` env var |
| Run in a different namespace | Override `namespace:` in `kustomization.yaml` and update the OIDC secret target |

## Migrating to production

Once tested, create a cluster-specific overlay that references
`bases/agent-example` (or your own agent's base) and patches production
values (endpoint, image digests, resource limits). Add it to the cluster's
ArgoCD ApplicationSet like any other app.
