---
name: dev-cluster
description: >
  Manages HyperShell development clusters (Kind) for testing changes locally.
  Use when deploying to Kind, bringing up local clusters, rebuilding images,
  or troubleshooting pod issues. Triggers on: "test in kind", "deploy locally",
  "kind cluster", "rebuild images", "bring up cluster", "kind-up", "dev environment".
---

# Development Cluster Management

## Platform Components

| Component | Location | Image | Deployment |
|-----------|----------|-------|------------|
| API Server | `components/api-server` | `hypershell:dev` | `hypershell-api-server` |
| Controller | `components/control-plane` | `hypershell-controller:dev` | `hypershell-controller` |

## Cluster Lifecycle

```bash
LOCAL_IMAGES=true make kind-up        # Create cluster with local images
make kind-teardown                    # Destroy cluster
make kind-api-server-up               # Build + swap API server from working tree
make kind-control-plane-up            # Build + swap control plane from working tree
make kind-status                      # Show cluster status
```

## Workflow: Testing Changes in Kind

### Step 1: Analyze Changes
```bash
git diff --name-only main...HEAD
```

Map changed files to components:
- `components/api-server/` -> API server
- `components/control-plane/` -> controller

### Step 2: Build and Deploy

**If cluster doesn't exist:** `make kind-up`
**If cluster exists:** `make kind-api-server-up` (for API server) and/or `make kind-control-plane-up` (for controller)

### Step 3: Verify Deployment
```bash
kubectl get pods -n hypershell
kubectl rollout status deployment/hypershell-api-server -n hypershell
kubectl rollout status deployment/hypershell-controller -n hypershell
```

### Step 4: Validate API
```bash
curl -s http://localhost:23080/api/hypershell/v1/fleets | jq .
```

### Step 5: Provide Access Info
```
API: http://localhost:23080/api/hypershell/v1/fleets

To view logs:
  kubectl logs -f -l app=hypershell-api-server -n hypershell
  kubectl logs -f -l app=hypershell-controller -n hypershell

To teardown: make kind-down
```

## Troubleshooting

### Pods in ImagePullBackOff
Kind has no registry. Ensure `imagePullPolicy: IfNotPresent`:
```bash
make kind-api-server-up
make kind-control-plane-up
```

### Pods in CrashLoopBackOff
```bash
kubectl logs -l app=<label> -n hypershell --tail=100
kubectl describe pod -l app=<label> -n hypershell
```

### Changes not reflected
```bash
make kind-api-server-up
make kind-control-plane-up
```

## Quick Reference

| Task | Command |
|------|---------|
| Create cluster | `LOCAL_IMAGES=true make kind-up` |
| Rebuild API server | `make kind-api-server-up` |
| Rebuild controller | `make kind-control-plane-up` |
| Check status | `make kind-status` |
| View API logs | `kubectl logs -f -l app=hypershell-api-server -n hypershell` |
| View CP logs | `kubectl logs -f -l app=hypershell-controller -n hypershell` |
| Tear down | `make kind-teardown` |
