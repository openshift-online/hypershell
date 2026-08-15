---
name: deploy-cluster
description: >
  Deploy HyperShell to an OpenShift cluster using the internal image registry.
  Covers image builds, registry push, kustomize deploy, rollout verification,
  and troubleshooting. Use when: "deploy to openshift", "deploy to cluster",
  "install hypershell", "push images", "openshift deploy".
---

# HyperShell OpenShift Deployment

> **Scope:** this skill deploys the **platform services** (API server, controller,
> PostgreSQL) and is cloud-agnostic across OpenShift distributions (ROSA, ROKS,
> self-managed). It does **not** stand up tenant-gateway ingress. The shared
> `Gateway` + wildcard DNS/TLS that tenant traffic needs is a separate one-time
> per-cluster bootstrap - see [`cloud-hub-ingress-bootstrap`](../cloud-hub-ingress-bootstrap/SKILL.md).
> Without it, the controller runs but every gateway reconcile fails with
> `GATEWAY_API_GATEWAY_NAME is required`.

## Cloud-Hub Parameter Overrides

The steps below use AWS/ROSA defaults. On other clouds, override only these values;
the `oc`-based flow is otherwise identical.

| Parameter | AWS / ROSA | IBM Cloud / ROKS |
|-----------|------------|-------------------|
| Cluster login | `oc login` | `ibmcloud login` → `ibmcloud oc cluster config -c <cluster>` → `oc` |
| Internal registry route | `...elb/openshift-image-registry` host | `default-route-openshift-image-registry...appdomain.cloud` |
| Namespace | `hypershell-api` | `hypershell-api` (same) |
| PostgreSQL storage class | cluster default | `ibmc-vpc-block-10iops-tier` (pin on the postgres PVC) |
| Tenant-gateway ingress | via `cloud-hub-ingress-bootstrap` (ELB) | via `cloud-hub-ingress-bootstrap` (VPC LB) |

## Platform Components

| Component | Image | Deployment | Ports |
|-----------|-------|------------|-------|
| PostgreSQL | `registry.redhat.io/rhel9/postgresql-13:latest` | StatefulSet `hypershell-postgres` | 5432 |
| API Server | `hypershell:dev` | Deployment `hypershell-api-server` | 8000 (HTTP), 9000 (gRPC), 4434 (health) |
| Controller | `hypershell-controller:dev` | Deployment `hypershell-controller` | none (outbound gRPC) |

## Prerequisites

- `oc` CLI authenticated as cluster-admin (or namespace admin with image push rights)
- `podman` or `docker` available locally
- Access to the OpenShift internal image registry route
- Go toolchain for local builds

## Step 1: Create Namespace

```bash
oc new-project hypershell-api
```

## Step 2: Build Images

From `components/api-server/`:

```bash
make image            # Builds hypershell:dev (API server)
make image-controller # Builds hypershell-controller:dev (control plane)
```

**Build context**: API server Dockerfile uses `.` (the api-server directory). Controller
Dockerfile uses `components/` (parent directory containing both api-server and control-plane).

**Key detail**: `rh-trex-ai` is consumed as a Go module dependency (not a sibling COPY).
The Dockerfiles set `GOPRIVATE=github.com/openshift-online/rh-trex-ai` for `go mod download`.

## Step 3: Push to Internal Registry

```bash
REGISTRY=$(oc get route default-route -n openshift-image-registry -o jsonpath='{.spec.host}')
podman login --tls-verify=false -u $(oc whoami) -p $(oc whoami -t) $REGISTRY

podman tag localhost/hypershell:dev $REGISTRY/hypershell-api/hypershell:dev
podman push --tls-verify=false $REGISTRY/hypershell-api/hypershell:dev

podman tag localhost/hypershell-controller:dev $REGISTRY/hypershell-api/hypershell-controller:dev
podman push --tls-verify=false $REGISTRY/hypershell-api/hypershell-controller:dev
```

Verify image streams:
```bash
oc get imagestream -n hypershell-api
```

## Step 4: Deploy with Kustomize

```bash
cd components/api-server
oc kustomize deploy/openshift/ | oc apply -f -
```

This creates: Namespace, Secret (DB creds), ServiceAccounts, PostgreSQL StatefulSet,
API Server Deployment (with migrate init container), Controller Deployment, Service,
and Route.

## Step 5: Wait for Rollout

```bash
oc wait --for=condition=ready pod -l app=hypershell-postgres -n hypershell-api --timeout=120s
oc wait --for=condition=available deployment/hypershell-api-server -n hypershell-api --timeout=120s
oc wait --for=condition=available deployment/hypershell-controller -n hypershell-api --timeout=120s
```

## Step 6: Verify Installation

```bash
oc get pods -n hypershell-api

ROUTE=$(oc get route hypershell-api -n hypershell-api -o jsonpath='{.spec.host}')
curl -sk https://$ROUTE/api/hypershell/v1/fleets | python3 -m json.tool
```

Expected: 3 pods running (postgres-0, api-server-*, controller-*), API returns `FleetList`.

## Image Update Workflow

After code changes, rebuild and push:

```bash
make image && make image-controller
REGISTRY=$(oc get route default-route -n openshift-image-registry -o jsonpath='{.spec.host}')
podman tag localhost/hypershell:dev $REGISTRY/hypershell-api/hypershell:dev
podman push --tls-verify=false $REGISTRY/hypershell-api/hypershell:dev
podman tag localhost/hypershell-controller:dev $REGISTRY/hypershell-api/hypershell-controller:dev
podman push --tls-verify=false $REGISTRY/hypershell-api/hypershell-controller:dev
oc rollout restart deployment/hypershell-api-server -n hypershell-api
oc rollout restart deployment/hypershell-controller -n hypershell-api
```

## OpenShift-Specific Differences from Kind

| Concern | Kind | OpenShift |
|---------|------|-----------|
| Image registry | `localhost/` with `imagePullPolicy: Never` | Internal registry with `imagePullPolicy: Always` |
| Namespace | `hypershell` | `hypershell-api` |
| External access | NodePort on 30080 | Route with TLS edge termination |
| PostgreSQL image | `postgres:13` | `registry.redhat.io/rhel9/postgresql-13:latest` |
| PostgreSQL env vars | `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB` | `POSTGRESQL_USER`, `POSTGRESQL_PASSWORD`, `POSTGRESQL_DATABASE` |
| PostgreSQL data dir | `/var/lib/postgresql/data` | `/var/lib/pgsql/data` |
| SecurityContext | none | `runAsNonRoot`, `drop: ALL`, `allowPrivilegeEscalation: false` |
| Auth | disabled (dev) | disabled (dev); enable for production |

## Troubleshooting

### Init Container CrashLoopBackOff (migrate)

Check migrate logs:
```bash
oc logs -n hypershell-api deployment/hypershell-api-server -c migrate
```

Common causes:
- **Database does not exist**: RHEL PostgreSQL image requires `POSTGRESQL_USER` + `POSTGRESQL_PASSWORD` + `POSTGRESQL_DATABASE` together. Using only `POSTGRESQL_ADMIN_PASSWORD` creates the superuser but no custom database.
- **Connection refused**: PostgreSQL not ready yet. The init container will retry via CrashLoopBackOff.

### Protobuf Panic on Startup

```
panic: runtime error: slice bounds out of range [-5:]
```

Caused by `go_package` mismatch in `.proto` files. The proto `go_package` must match
the Go module path: `github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1`.

Fix: update all proto files and regenerate with `cd proto && buf generate`.
Ensure `protoc-gen-go` version matches the `google.golang.org/protobuf` version in `go.mod`.

### Old Image Cached on Nodes

If pods run stale code after a push, ensure `imagePullPolicy: Always` is set on all
containers. The `:dev` tag defaults to `IfNotPresent`.

### Pods in ImagePullBackOff

Verify image streams exist:
```bash
oc get imagestream -n hypershell-api
```

Verify the service account can pull:
```bash
oc policy add-role-to-user system:image-puller system:serviceaccount:hypershell-api:default -n hypershell-api
```

## Teardown

```bash
oc delete project hypershell-api
```
