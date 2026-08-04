# HyperShell

Distributed API gateway fleet management platform that orchestrates gateway deployments across multiple Kubernetes clusters and cloud providers. Built with Go (API server, control plane). PostgreSQL is the source of truth; the control plane reconciles via gRPC watch streams.

## Before Developing

Install the repository's pinned Git hooks from the repository root before making
changes:

```shell
make hooks-install
```

The pre-commit and pre-push hooks run the repository policy checks. Run the same
checks manually with `make check`.

## Structure

- `components/api-server/` - Go REST + gRPC API microservice (rh-trex-ai framework), PostgreSQL-backed
- `components/control-plane/` - Go service, watches API server via gRPC and reconciles gateway resources into K8s
- `specs/` - Desired state of the system ([platform](specs/platform/), [standards](specs/standards/))
- `skills/` - Agent skills: [reconcile](skills/build/reconcile), [spec](skills/plan/spec), [full-stack-pipeline](skills/build/full-stack-pipeline), [dev-cluster](skills/build/dev-cluster), [review](skills/review/review-guidance), [amber-review](skills/review/amber-review), [tooling](skills/tooling/)
- `apm.yml` - APM manifest declaring upstream skill dependencies

## Key Files

- API server entry point: `components/api-server/cmd/hypershell/main.go`
- Control plane reconciler: `components/control-plane/internal/reconciler/reconciler.go`
- Control plane watcher: `components/control-plane/internal/watcher/watcher.go`
- OpenAPI specs: `components/api-server/openapi/`
- Protobuf definitions: `components/api-server/proto/`

## Domain Model

| Kind | Purpose |
|------|---------|
| **Fleet** | Top-level organizational unit (tenant/project) |
| **Gateway** | API gateway instance deployed on a cluster |
| **GatewayNetwork** | Network connectivity topology between gateways |
| **GatewayRelease** | Versioned container images for gateway deployments |
| **ManagedCluster** | Kubernetes cluster registered into a fleet |
| **ManagedDatabase** | Database instance provisioned for a fleet |

## Resource Flow

```
Fleet Created -> Clusters/DBs Registered -> Release Published ->
Gateway Deployed on Cluster -> Network Mesh Established -> Traffic Flows
```

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

`/reconcile` is the top-level entrypoint. It reads `skills/RECONCILE.md` for checkpoint
state (coverage summary, gap table, wave plan), then executes waves to close gaps.
Idempotent: safe to run repeatedly.

Support skills available at any point:
- `/review-guidance` -- PR review checklist
- `/amber-review` -- Amber agent comprehensive code review
- `/align` -- convention health check
- `/maintain-ci` -- CI workflow and component registration maintenance
- `/memory` -- project memory management

## Commands

```shell
# API Server
cd components/api-server && make binary        # Build binary
cd components/api-server && make run           # Migrate + serve (with auth)
cd components/api-server && make run-no-auth   # Migrate + serve (no auth, dev mode)
cd components/api-server && make test           # Run tests
cd components/api-server && make test-integration  # Integration tests
cd components/api-server && make generate      # Regenerate OpenAPI client
cd components/api-server && make proto         # Regenerate gRPC stubs
cd components/api-server && make db/setup      # Start PostgreSQL
cd components/api-server && make db/teardown   # Stop PostgreSQL

# Control Plane
cd components/control-plane && go build ./...  # Build
cd components/control-plane && go vet ./...    # Vet

# All Components
make build-all                                 # Build all container images
make kind-up                                   # Start local Kind cluster
make kind-down                                 # Destroy Kind cluster
make kind-rebuild                              # Rebuild + redeploy
make kind-status                               # Show cluster status
make lint                                      # Lint all Go code
```

## Critical Conventions

Cross-cutting rules that apply across ALL components.

- **No `panic()` in production**: Return explicit `fmt.Errorf` with context
- **PostgreSQL for persistent storage**: All resource state lives in the API server's database
- **Conventional commits**: Squashed on merge to `main`
- **Reconcile, don't create-or-skip**: Use update-or-create patterns, not create-and-ignore-`AlreadyExists`
- **Never silently swallow partial failures**: Every error path must propagate or be collected
- **Restricted SecurityContext on all containers**: `runAsNonRoot`, drop `ALL` capabilities
- **Image references must match across the stack**: After changing an image name or tag, grep all overlays and manifests
- **Register every component in CI**: Use `/maintain-ci` when adding, renaming, moving, or removing a component
- **Verify contracts and references**: Before building on an assumption, verify the contract
- **Separate configuration from code**: Config changes must not require code changes

Component-specific conventions:
- Control Plane: [conventions](specs/standards/control-plane/conventions.spec.md)
- Security: [security standards](specs/standards/security/security.spec.md)
