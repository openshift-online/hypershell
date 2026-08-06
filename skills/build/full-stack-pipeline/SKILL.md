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
  └-> API (openapi/*.yaml)
        └-> OpenAPI Client (generated)
              ├-> BE  (plugins/*/model, handler, dao, service, migration)
              ├-> gRPC (proto → generated stubs → grpc_handler, grpc_presenter)
              └-> CP  (watcher, reconciler)
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

Compare the spec against the current state of the code:

| Component | What to check |
|-----------|---------------|
| **API** | Does `openapi/*.yaml` have all spec entities, routes, and fields? |
| **BE** | Read `plugins/<kind>/model.go` for every Kind. Compare field-by-field. |
| **gRPC** | Do proto definitions cover all fields? Do handlers and presenters exist? |
| **CP** | Does the watcher subscribe to all Kinds? Does the reconciler handle all events? |

Produce a gap table:

```
ENTITY          COMPONENT   STATUS      GAP
Fleet           API         present     --
Gateway.phase   BE          missing     no phase field in model
```

### Step 3 -- Break Into Waves

**Wave 1 -- Spec consensus** (no code; human approval)
- Confirm gap table is complete

**Wave 2 -- API** (gates everything downstream)
- Update `openapi/*.yaml` for all new entities and routes
- Acceptance: `make test`, `make binary` clean

**Wave 3 -- OpenAPI Client** (gates BE, CP)
- Run `make generate` against updated openapi.yaml
- Acceptance: `go build ./...` clean

**Wave 4 -- BE + gRPC** (parallel after Wave 3)
- BE: migrations, DAOs, service logic, presenters
- gRPC: proto updates, `make proto`, handler/presenter implementation
- Acceptance: `make test`, `go vet ./...` clean

**Wave 5 -- CP** (after Wave 4)
- Watcher subscription for new Kinds
- Reconciler logic for new resources
- Acceptance: `go build ./...`, `go vet ./...` clean

**Wave 6 -- Integration**
- End-to-end smoke test in Kind cluster (`make kind-rebuild`) or OpenShift (`/deploy-cluster`)
- Verify CRUD on all affected Kinds via the API route

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
  --fields "name:string:required,fleet_id:string:required,status:string" \
  --project hypershell \
  --repo github.com/openshift-online/hypershell/components/api-server \
  --library github.com/openshift-online/rh-trex-ai
```

## Build and Deploy

### Image Builds

`rh-trex-ai` is a Go module dependency (not a local sibling). Dockerfiles use
`GOPRIVATE=github.com/openshift-online/rh-trex-ai` during `go mod download`.

```bash
cd components/api-server
make image            # API server (build context: .)
make image-controller # Controller (build context: components/)
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

- **Kind**: `make kind-up` / `make kind-rebuild` (see `/kind` skill)
- **OpenShift**: Build, push to internal registry, `oc kustomize deploy/openshift/` (see `/deploy-cluster` skill)

## Constraints

- Pipeline order is strict: no downstream wave starts until upstream is settled
- Spec is frozen during execution
- PRs are atomic per wave per component
