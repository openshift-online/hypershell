# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

HyperShell is a distributed API gateway fleet management platform that orchestrates gateway deployments across multiple Kubernetes clusters and cloud providers. Built with Go (API server, control plane). PostgreSQL is the source of truth; the control plane reconciles via gRPC watch streams.

## Before Developing

```shell
make hooks-install   # install pinned Git hooks (runs make check on commit/push)
make check           # run all policy checks manually
```

## Structure

- `components/api-server/` - Go REST + gRPC API microservice (rh-trex-ai framework), PostgreSQL-backed
- `components/control-plane/` - Go service, watches API server via gRPC and reconciles gateway resources into K8s
- `packages/gateway-management-ui/` - Private reusable React package containing canonical gateway management workflows
- `specs/` - Desired state of the system ([platform](specs/platform/), [standards](specs/standards/))
- `skills/` - Agent skills: [reconcile](skills/build/reconcile), [spec](skills/plan/spec), [full-stack-pipeline](skills/build/full-stack-pipeline), [dev-cluster](skills/build/dev-cluster), [ibm-cluster](skills/deploy/ibm-cluster), [deploy-cluster](skills/deploy/deploy-cluster), [cloud-hub-ingress-bootstrap](skills/deploy/cloud-hub-ingress-bootstrap), [review](skills/review/review-guidance), [amber-review](skills/review/amber-review), [ui-standards](skills/review/ui-standards), [tooling](skills/tooling/)
- `apm.yml` - APM manifest declaring upstream skill dependencies

## Commands

```shell
# API Server (separate Go module: components/api-server/)
cd components/api-server && make binary            # build binary
cd components/api-server && make run               # migrate + serve with auth
cd components/api-server && make run-no-auth       # migrate + serve without auth (dev mode)
cd components/api-server && make test              # go test -v ./... (unit)
cd components/api-server && make test-integration  # API_ENV=integration_testing go test -p 1 -v ./plugins/...
cd components/api-server && make generate          # regenerate OpenAPI client (pkg/api/openapi/)
cd components/api-server && make proto             # regenerate gRPC stubs from proto/
cd components/api-server && make db/setup          # start PostgreSQL via Podman/Docker
cd components/api-server && make db/teardown       # stop PostgreSQL

# Run a single API server test package
cd components/api-server && API_ENV=integration_testing go test -p 1 -v ./plugins/gateways/ -run TestName

# Control Plane (separate Go module: components/control-plane/)
cd components/control-plane && go build ./...      # build
cd components/control-plane && go vet ./...        # vet
cd components/control-plane && go test ./...       # unit tests
cd components/control-plane && go test -race ./internal/reconciler/  # race-test a package

# Lint
make lint                    # all components (Go + JS/TS)
make lint-api-server         # gofmt + go vet + golangci-lint for api-server
make lint-control-plane      # gofmt + go vet + golangci-lint for control-plane

# Kind cluster (local development)
make kind-up                 # create cluster + deploy all components (OIDC on)
KIND_NO_SUDO=true make kind-up  # skip sudo for port forwarding (autonomous/CI use)
LOCAL_IMAGES=true make kind-up   # build + deploy from working tree (default)
LOCAL_IMAGES=true BUILD_SOURCE=baseline make kind-up  # build from origin/main
LOCAL_IMAGES=true KIND_SKIP_BUILD=true make kind-up   # reuse existing local images (skip build)
KIND_SKIP_SEED=true make kind-up  # defer seeding (run kind-seed later)
make kind-seed               # seed platform resources into a running cluster
make kind-down               # remove namespace and its resources
make kind-teardown           # destroy cluster entirely
make kind-status             # show pods, services, swap state
make kind-api-server-up      # build + swap API server from working tree
make kind-control-plane-up   # build + swap control plane from working tree
make kind-web-console-up     # hot-reload web console (set KIND_HOT_RELOAD=false for full image)
make kind-fix-ports          # re-establish host port 443 forwarding after cluster restart

# Code generation (run after editing .proto or openapi/*.yaml)
cd components/api-server && make proto     # regenerate pb.go from proto/
cd components/api-server && make generate  # regenerate OpenAPI Go/TS clients
```

### Integration Test Prerequisites

Integration tests use `testcontainers-go` to spin up a PostgreSQL per package. On Linux with Podman:

```shell
systemctl --user start podman.socket
export DOCKER_HOST=unix:///run/user/1000/podman/podman.sock
export RYUK_DISABLED=true   # if the reaper container fails
```

## Go Module Structure

Two distinct Go modules share a single repo root. They cross-reference via `replace` directives in development:

| Module | Path | `go.mod` |
|--------|------|----------|
| `github.com/openshift-online/hypershell/components/api-server` | `components/api-server/` | owns all API, OpenAPI, proto, plugins |
| `github.com/openshift-online/hypershell/components/control-plane` | `components/control-plane/` | imports api-server proto via replace directive |

Never edit `components/api-server/pkg/api/openapi/` manually; it is fully generated by `make generate`.

## Architecture

### API Server Plugin System

Each Kind is a self-contained plugin in `components/api-server/plugins/{kinds}/`:

| File | Role |
|------|------|
| `plugin.go` | `init()` registers service, routes, controller, migration |
| `model.go` | Gorm model + patch request struct |
| `handler.go` | HTTP handlers (Create, Get, List, Patch, Delete) |
| `grpc_handler.go` | gRPC handlers (Watch, Create, Get, List, Patch, Delete) |
| `service.go` | Business logic + event handlers (`OnUpsert`, `OnDelete`) |
| `dao.go` | Data access layer |
| `presenter.go` | OpenAPI ↔ model conversion |
| `grpc_presenter.go` | Protobuf ↔ model conversion |
| `migration.go` | Gormigrate migration |
| `placement.go` | (Gateways only) database placement logic |
| `provider.go` | (Gateways only) `DATABASE_PROVIDER` env var resolution |

Plugins are imported in `main.go` as side-effect imports (`_ "..."`). The framework auto-discovers registered routes, controllers, and migrations.

**Adding a new DATABASE_PROVIDER:** Update `resolveDatabaseProvider()` in both `components/api-server/plugins/gateways/provider.go` and `components/control-plane/internal/config/config.go` (parallel switch statements), add a `PlacementResolver` implementation in `placement.go`, implement the `DatabaseReconciler` interface in `components/control-plane/internal/gateway/` and register it in `newDatabaseReconciler()` in `db_reconciler.go`.

### Control Plane Reconciler Pattern

The control plane opens gRPC watch streams to the API server for every resource Kind and drives reconciliation from events:

```
watcher.Watch[T] → event → reconciler.Handle() → per-resource serialized handleOne()
```

`ManagedDatabaseReconciler.handleOne()` branches on `db.Provider` (`cnpg` / `deployment` / `external`) via the `DatabaseReconciler` interface. `newDatabaseReconciler()` in `db_reconciler.go` is the factory that maps provider to implementation (`cnpgDatabaseReconciler`, `deploymentDatabaseReconciler`, `externalDatabaseReconciler`). `GatewayReconciler.ReconcileGateway()` selects the implementation via `ReconcileOpts.DatabaseProvider`.

`ReconcileOpts` (in `components/control-plane/internal/gateway/config.go`) is the central configuration struct passed to `ReconcileGateway`. New provider fields go here.

### gRPC + Proto Workflow

Protobuf definitions live in `components/api-server/proto/`. Generated stubs go to `components/api-server/pkg/api/grpc/`. After editing `.proto` files, run `make proto` in `components/api-server/` to regenerate both the Go stubs and the descriptor; then run `make generate` to regenerate the OpenAPI client if the OpenAPI spec also changed.

## Key Files

- API server entry point: `components/api-server/cmd/hypershell/main.go`
- API server plugin registry: `components/api-server/plugins/*/plugin.go`
- Gateway placement: `components/api-server/plugins/gateways/placement.go`
- DATABASE_PROVIDER resolution (api-server): `components/api-server/plugins/gateways/provider.go`
- DATABASE_PROVIDER resolution (control-plane): `components/control-plane/internal/config/config.go`
- Control plane entry point: `components/control-plane/cmd/main.go`
- Control plane reconciler: `components/control-plane/internal/reconciler/reconciler.go`
- Gateway reconcile logic: `components/control-plane/internal/gateway/reconciler.go`
- Gateway config/ReconcileOpts: `components/control-plane/internal/gateway/config.go`
- OpenAPI specs: `components/api-server/openapi/`
- Protobuf definitions: `components/api-server/proto/`
- Kind deploy overlay: `deploy/kind/kustomization.yaml`

## Domain Model

| Kind | Purpose |
|------|---------|
| **Gateway** | API gateway instance deployed on a cluster |
| **GatewayNetwork** | Network connectivity topology between gateways |
| **GatewayRelease** | Versioned container images for gateway deployments |
| **ManagedCluster** | Kubernetes cluster registered into the platform |
| **ManagedDatabase** | Database instance provisioned for gateway use (`provider`: `deployment` \| `cnpg` \| `external`) |

All resources are top-level; no Fleet/Sector grouping. Tenancy enforced by RBAC.

## SDLC Workflow

The development lifecycle follows 6 steps, each backed by a skill:

```
0. /reconcile             -- autonomous spec-to-code reconciliation (build/reconcile)
1. /spec                  -- define desired state (plan/spec)
2. /full-stack-pipeline   -- build the feature (build/full-stack-pipeline)
3. /dev-cluster           -- test locally in Kind (build/dev-cluster)
4. /pr-test               -- deploy PR to cluster (test/pr-test)
5. /deploy-cluster        -- ship to production (deploy/deploy-cluster)
```

`/reconcile` reads `skills/RECONCILE.md` for checkpoint state (coverage summary, gap table, wave plan) and executes waves to close gaps. Idempotent.

Support skills available at any point:
- `/cloud-hub-ingress-bootstrap` -- one-time shared Gateway + wildcard DNS/TLS per Cloud Hub (AWS/IBM)
- `/review-guidance` -- PR review checklist
- `/amber-review` -- Amber agent comprehensive code review
- `/ui-standards` -- UI/UX audit or intent-driven design guidance
- `/align` -- convention health check
- `/maintain-ci` -- CI workflow and component registration maintenance
- `/update-openshell` -- sync HyperShell to a new upstream OpenShell release (self-reinforcing)
- `/memory` -- project memory management

## Critical Conventions

- **No `panic()`**: Return `fmt.Errorf` with context on every error path
- **PostgreSQL for persistent storage**: All resource state lives in the API server's database
- **Conventional commits**: Squashed on merge to `main`
- **Reconcile, don't create-or-skip**: Use update-or-create patterns
- **Never swallow partial failures**: Every error path must propagate or be collected
- **Generated files are never hand-edited**: `pkg/api/openapi/`, `pkg/api/grpc/` (run `make generate` / `make proto`)
- **Restricted SecurityContext on all containers**: `runAsNonRoot`, drop `ALL` capabilities
- **Image references must match across the stack**: After changing an image name or tag, grep all overlays and manifests
- **Register every component in CI**: Use `/maintain-ci` when adding, renaming, moving, or removing a component
- **Verify contracts and references**: Before building on an assumption, verify the contract
- **Separate configuration from code**: Config changes must not require code changes
- **PatternFly 6 for web UI**: Reuse PatternFly and shared components; no duplicate UI components
- **Narrow hexagonal UI boundary**: Application workflows behind ports; keep React, TanStack Query, Fastify, generated SDKs outside
- **Domain probes for UI observability**: Publish typed workflow and dependency facts through a fan-out port; no raw console or direct telemetry calls in production browser/BFF code
- **No em dashes**: Use hyphens (`-`) instead of em dashes (`—` U+2014) in all text files; the pre-commit hook rejects them

Component-specific conventions:
- Control Plane: [conventions](specs/standards/control-plane/conventions.spec.md)
- Security: [security standards](specs/standards/security/security.spec.md)
- Web UI: [UI standards](specs/standards/ui/)
