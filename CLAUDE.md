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
- `packages/gateway-management-ui/` - Private reusable React package containing canonical gateway management workflows
- `specs/` - Desired state of the system ([platform](specs/platform/), [standards](specs/standards/))
- `skills/` - Agent skills: [reconcile](skills/build/reconcile), [spec](skills/plan/spec), [full-stack-pipeline](skills/build/full-stack-pipeline), [dev-cluster](skills/build/dev-cluster), [ibm-cluster](skills/deploy/ibm-cluster), [deploy-cluster](skills/deploy/deploy-cluster), [cloud-hub-ingress-bootstrap](skills/deploy/cloud-hub-ingress-bootstrap), [review](skills/review/review-guidance), [amber-review](skills/review/amber-review), [ui-standards](skills/review/ui-standards), [tooling](skills/tooling/), [security-audit](skills/security/run-security-audit)
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
| **Gateway** | API gateway instance deployed on a cluster |
| **GatewayNetwork** | Network connectivity topology between gateways |
| **GatewayRelease** | Versioned container images for gateway deployments |
| **ManagedCluster** | Kubernetes cluster registered into the platform |
| **ManagedDatabase** | Database instance provisioned for gateway use |

All resources are top-level; there is no Fleet/Sector grouping. Tenancy is
enforced by RBAC (platform-level and per-gateway), not by a resource grouping.

## Resource Flow

```
Clusters/DBs Registered -> Release Published ->
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
- `/cloud-hub-ingress-bootstrap` -- one-time shared Gateway + wildcard DNS/TLS per Cloud Hub (AWS/IBM)
- `/review-guidance` -- PR review checklist
- `/amber-review` -- Amber agent comprehensive code review
- `/ui-standards` -- UI/UX audit or intent-driven design guidance
- `/align` -- convention health check
- `/maintain-ci` -- CI workflow and component registration maintenance
- `/update-openshell` -- sync HyperShell to a new upstream OpenShell release (self-reinforcing)
- `/run-security-audit` -- comprehensive security audit (threat model + code audit)
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
pnpm --filter @openshift-online/hypershell-gateway-management-ui check  # Verify reusable gateway UI
make kind-up                                   # Start local Kind cluster
make kind-down                                 # Destroy Kind cluster
make kind-status                               # Show cluster status
make lint                                      # Lint all Go code

# APM & Security
make apm-install                               # Install APM dependencies + run security scan
make apm-audit                                 # Run APM security audit
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
- **PatternFly 6 for web UI**: Reuse PatternFly and canonical shared components; do not create duplicate UI components
- **Narrow hexagonal UI boundary**: Put application workflows and external effects behind application-owned ports; keep React, TanStack Query, Fastify, generated SDKs, and infrastructure outside
- **Domain probes for UI observability**: Publish typed workflow and dependency facts through a fan-out port; no raw console or direct telemetry calls in production browser/BFF code

Component-specific conventions:
- Control Plane: [conventions](specs/standards/control-plane/conventions.spec.md)
- Security: [security standards](specs/standards/security/security.spec.md)
- Web UI: [UI standards](specs/standards/ui/)
