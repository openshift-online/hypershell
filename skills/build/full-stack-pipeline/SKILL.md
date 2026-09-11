---
name: full-stack-pipeline
description: >
  Autonomous workflow for implementing spec-driven changes across the HyperShell
  platform. Orchestrates gap analysis, wave-based execution, and cross-component
  integration. Use when: "implement this spec", "build this feature end-to-end",
  "run the pipeline", "gap analysis", "full-stack change".
---

# Full-Stack Pipeline

Implement spec-driven changes across all HyperShell platform components using a wave-based pipeline.

## User Input

```text
$ARGUMENTS
```

## The Pipeline

Changes flow downstream in a fixed dependency order:

```
Spec (data-model.spec.md)
  └─► API (openapi/*.yaml)
        └─► SDK Generator
              ├─► Go SDK (types, builders, clients in components/sdk-go/)
              └─► TypeScript SDK (types, clients in components/sdk-typescript/)
                    ├─► BE  (plugins/*/model, handler, dao, service, migration)
                    ├─► CLI (cmd/hypershell/*/cmd.go commands)
                    ├─► gRPC (proto → generated stubs → grpc_handler, grpc_presenter)
                    └─► CP  (watcher, reconciler)
```

Each stage depends on the stage above it being settled.

## Workflow Steps

### Step 1 -- Read the Spec

Read the relevant spec in full. Extract:
- All entities and their fields
- All relationships and API routes
- Design decisions

This is the **desired state**. Everything else is measured against it.

### Step 2 -- Gap Analysis

Compare the spec against the current state of the code. Check all three directions for every Kind:

1. Spec ERD → `model.go` - spec says field exists; is it in the model?
2. `model.go` → Spec ERD - model has a field; is it documented in the spec?
3. OpenAPI `required[]` → Spec ERD - OpenAPI marks it required; does the spec agree?

| Component | What to check |
|-----------|---------------|
| **API** | Does `openapi/*.yaml` have all spec entities, routes, and fields? Check schema `required[]` arrays field-by-field. |
| **SDK (Go)** | Do generated types/builders/clients exist in `components/sdk-go/` for all spec entities? |
| **SDK (TS)** | Do generated types/clients exist in `components/sdk-typescript/` for all spec entities? |
| **BE** | Read `plugins/<kind>/model.go` for every Kind. Compare field-by-field against the Spec. |
| **CLI** | Does `hypershell` implement every route marked ✅ implemented in the spec CLI table? Check `components/cli/cmd/hypershell/*/cmd.go`. |
| **gRPC** | Do proto definitions cover all fields? Do handlers and presenters exist? |
| **CP** | Does the watcher subscribe to all Kinds? Does the reconciler handle all events? |

Produce a gap table:

```
ENTITY          COMPONENT   STATUS      GAP
Fleet           API         present     --
Gateway.phase   SDK (Go)    missing     no Phase field in Gateway type
Gateway.phase   BE          missing     no phase field in model
Gateway         CLI         partial     get/list implemented, delete missing
```

### Step 3 -- Break Into Waves

**Wave 1 -- Spec consensus** (no code; human approval)
- Confirm gap table is complete and agreed upon
- Freeze spec for this run

**Wave 2 -- API** (gates everything downstream)
- Update `openapi/*.yaml` for all new entities and routes
- Register routes in `routes.go`; add handler stubs
- Security gate: new routes use `environments.JWTMiddleware` (when auth enabled)
- Acceptance: `make test`, `make binary`, `make lint` clean

**Wave 3 -- SDK** (gates BE, CLI)
- Run SDK generator against updated `openapi.yaml`:
  ```bash
  cd scripts/sdk-generator
  go run . --spec ../../components/api-server/openapi/openapi.yaml \
           --go-out ../../components/sdk-go \
           --ts-out ../../components/sdk-typescript
  ```
- Commit generated types, builders, client methods (both Go and TypeScript)
- Verify nested resource paths for project-scoped resources
- Acceptance: `go build ./...` clean in sdk-go; TypeScript compiles

**Wave 4 -- BE + gRPC** (parallel after Wave 3)
- BE: migrations, DAOs, service logic, gRPC presenters
- gRPC: proto updates, `make proto`, handler/presenter implementation
- Security gate: all handler paths check user token (when auth enabled); no tokens in logs
- Acceptance: `make test`, `go vet ./... && golangci-lint run` clean

**Wave 5 -- CLI** (parallel after Wave 3)
- Implement all planned commands from spec CLI table
- Commands go in `components/cli/cmd/hypershell/{verb}/{resource}/cmd.go`
- Follow existing patterns: create, get, list, delete
- Use SDK client for all API calls
- Acceptance: CLI commands work against running API server

**Wave 6 -- CP** (after Wave 4)
- Watcher subscription for new Kinds
- Reconciler logic for new resources
- Acceptance: `go build ./...`, `go vet ./...` clean

**Wave 7 -- Integration**
- End-to-end smoke test in Kind cluster (`make kind-api-server-up` / `make kind-control-plane-up`) or OpenShift (`/deploy-cluster`)
- Test CLI commands against deployed API
- Verify CRUD on all affected Kinds via both API and CLI
- Run e2e test suite: `bash components/pr-test/e2e-openshell.sh`

Each wave is a gate. Do not start downstream work against an unstable upstream.

### Step 4 -- Verify Each Wave

After each wave:
- Re-run the gap table for that component only
- If gaps remain, iterate
- If clean, mark complete and proceed

## Plugin Code Generation

New Kinds use the generator:

```bash
cd components/api-server
go run ./scripts/generator.go \
  --kind YourKind \
  --fields "name:string:required,cluster_id:string:required,status:string" \
  --project hypershell \
  --repo github.com/openshift-online/hypershell/components/api-server \
  --library github.com/openshift-online/rh-trex-ai
```

## Build and Deploy

### Image Builds

`rh-trex-ai` is a Go module dependency (not a local sibling). Dockerfiles use
`GOPRIVATE=github.com/openshift-online/rh-trex-ai` during `go mod download`.

Build all container images from the repository root:

```bash
make build-all  # Build all container images (API server + controller)
```

### Proto Regeneration

Proto `go_package` must use the full module path:
`github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1`

Ensure `protoc-gen-go` version matches `google.golang.org/protobuf` in `go.mod`:
```bash
go install google.golang.org/protobuf/cmd/protoc-gen-go@$(go list -m -f '{{.Version}}' google.golang.org/protobuf)
cd proto && buf generate
```

### Deploy Targets

- **Kind**: `make kind-up` / `make kind-api-server-up` / `make kind-control-plane-up` (see `/dev-cluster` skill)
- **OpenShift**: Build, push to internal registry, `oc kustomize deploy/openshift/` (see `/deploy-cluster` skill)

## SDK Generator

The SDK generator reads OpenAPI specs and generates type-safe clients.

### Go SDK Generation

Located in `scripts/sdk-generator/`, generates:
- Types in `components/sdk-go/types/`
- Client methods in `components/sdk-go/client/`

Template files:
- `scripts/sdk-generator/templates/go/type.go.tmpl`
- `scripts/sdk-generator/templates/go/client.go.tmpl`

### TypeScript SDK Generation

Generates:
- Types in `components/sdk-typescript/src/types/`
- Client methods in `components/sdk-typescript/src/client.ts`

Template files:
- `scripts/sdk-generator/templates/ts/types.ts.tmpl`
- `scripts/sdk-generator/templates/ts/client.ts.tmpl`

### Common Pitfalls

- **Nested resource base path (TS):** Generator uses first path segment as base - wrong for nested resources like `/gateways/{gateway_id}/service_accounts`. May need hand-crafted extensions.
- **Required fields:** OpenAPI `required[]` must match spec ERD exactly
- **Generated variable names:** Ensure route path params match handler `mux.Vars()` keys
- **CLI output formats:** Always support `-o json` for scriptability

## Constraints

- **Pipeline order is strict**: no downstream wave starts until upstream is merged and SDK is regenerated
- **One active session per wave**: runs to completion before next wave begins
- **Spec is frozen during execution**: queue changes for next cycle
- **PRs are atomic per wave per component**: one PR per wave
- **SDK regeneration is mandatory**: after any OpenAPI change, regenerate SDKs before touching BE/CLI
