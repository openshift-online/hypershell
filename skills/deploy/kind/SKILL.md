---
name: kind
description: >
  Deploy HyperShell to a local Kind cluster for development and testing.
  Manages cluster lifecycle, image loading, port forwarding, and troubleshooting.
  Use when: "test in kind", "local cluster", "kind-up", "dev environment",
  "deploy locally", "rebuild images".
---

# Kind Local Development

## Platform Components

| Component | Location | Image | Deployment |
|-----------|----------|-------|------------|
| API Server | `components/api-server` | `hypershell:dev` | `hypershell-api-server` |
| Controller | `components/control-plane` | `hypershell-controller:dev` | `hypershell-controller` |
| PostgreSQL | (upstream image) | `postgres:13` | `hypershell-postgres` (StatefulSet) |

## Cluster Lifecycle

```bash
make kind-up        # Build images, create cluster, deploy all, wait for ready
make kind-down      # Destroy cluster
make kind-rebuild   # Rebuild images, reload into Kind, restart deployments
make kind-status    # Show cluster info, pods, services
```

## Workflow: Testing Changes

### Step 1: Analyze Changes
```bash
git diff --name-only main...HEAD
```

Map files to components:
- `components/api-server/` -> API server image
- `components/control-plane/` -> controller image

### Step 2: Build and Deploy

**New cluster:** `make kind-up`
**Existing cluster:** `make kind-rebuild`

### Step 3: Verify
```bash
kubectl get pods -n hypershell
kubectl rollout status deployment/hypershell-api-server -n hypershell
kubectl rollout status deployment/hypershell-controller -n hypershell
```

### Step 4: Test API
```bash
curl -s http://localhost:23080/api/hypershell/v1/fleets | jq .
```

### Step 5: Access Info
```
API: http://localhost:23080/api/hypershell/v1/fleets

Logs:
  kubectl logs -f -l app=hypershell-api-server -n hypershell
  kubectl logs -f -l app=hypershell-controller -n hypershell
```

## How Images Work in Kind

Kind has no registry. Images are built locally then loaded via archive:

```bash
podman save -o /tmp/hypershell-dev.tar hypershell:dev
kind load image-archive /tmp/hypershell-dev.tar --name hypershell-dev
```

All manifests use `imagePullPolicy: Never` to prevent Kubernetes from trying to
pull from a remote registry.

## Manifests

Located in `components/api-server/deploy/kind/`:

| File | Purpose |
|------|---------|
| `kind-config.yaml` | Kind cluster config (port mapping 30080 -> 23080) |
| `kustomization.yaml` | Kustomize entrypoint |
| `namespace.yaml` | `hypershell` namespace |
| `postgres.yaml` | PostgreSQL Secret, Service, StatefulSet |
| `api-server.yaml` | ServiceAccount, Deployment (with migrate init), Service |
| `api-server-nodeport.yaml` | NodePort service for external access |
| `controller.yaml` | ServiceAccount, Deployment |

## Differences from OpenShift Deployment

| Concern | Kind | OpenShift |
|---------|------|-----------|
| Registry | none (image archives) | Internal registry |
| Image pull | `imagePullPolicy: Never` | `imagePullPolicy: Always` |
| Namespace | `hypershell` | `hypershell-api` |
| External access | NodePort 30080 -> host 23080 | Route with TLS |
| PostgreSQL | `postgres:13` | `registry.redhat.io/rhel9/postgresql-13` |
| SecurityContext | none | `runAsNonRoot`, drop ALL |

## Troubleshooting

### Pods in ImagePullBackOff
Kind has no registry. Ensure `imagePullPolicy: Never` and images are loaded:
```bash
make kind-rebuild
```

### Pods in CrashLoopBackOff
```bash
kubectl logs -l app=<label> -n hypershell --tail=100
kubectl describe pod -l app=<label> -n hypershell
```

### Changes Not Reflected
```bash
make kind-rebuild
```

### Port Already in Use
Another Kind cluster may occupy port 23080. Check:
```bash
kind get clusters
```

## Quick Reference

| Task | Command |
|------|---------|
| Create cluster | `make kind-up` |
| Rebuild all | `make kind-rebuild` |
| Check status | `make kind-status` |
| View API logs | `kubectl logs -f -l app=hypershell-api-server -n hypershell` |
| View CP logs | `kubectl logs -f -l app=hypershell-controller -n hypershell` |
| Tear down | `make kind-down` |
| Test API | `curl -s http://localhost:23080/api/hypershell/v1/fleets` |
