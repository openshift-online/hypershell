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

## Kind Completeness: mandatory manifest, idempotent fill, enforced gate

A Kind is not "done" because its happy path compiles. **Every registered Kind
MUST have every artifact in the manifest below.** Do not rely on the implementer
to remember the list - it is stated here so nothing is left to memory or
inference. This section is the guarantee that a generated Kind actually works and
that an incomplete one fails the FIRST PR, not production.

### The Kind Completeness Manifest (every Kind, v1 and ext)

For `plugins/<kind>/` every one of these MUST exist:

- `model.go`, `dao.go`, `service.go`, `handler.go`, `presenter.go`,
  `grpc_handler.go`, `grpc_presenter.go`, `plugin.go` (registers service, REST
  routes, gRPC service, migration, controller), `mock_dao.go`.
- `migration.go` that creates/evolves the table with **raw SQL via `tx.Exec`**
  (`CREATE TABLE IF NOT EXISTS`, `ALTER TABLE ... ADD COLUMN IF NOT EXISTS`).
  **`AutoMigrate` is forbidden** - it fails closed on the production postgres
  driver (`lib/pq` + `PreferSimpleProtocol`), poisons the transaction, and makes
  every query on the table abort. The conformance gate rejects any `AutoMigrate`.

Cross-cutting, per Kind:

- **OpenAPI**: `openapi/openapi.<kinds>.yaml` with schema + every path, wired into
  `openapi.yaml`; `required[]` matches the spec ERD field-by-field.
- **SDK**: Go type+client in `components/sdk-go/` and TS type+client in
  `components/sdk-typescript/`, produced by the generator (never hand-edited).
- **CLI**: `hsctl create`/`get`/`list`/`delete`/`apply` for the Kind plus its
  `urls` path. A Kind absent from a spec CLI table still gets all five.
- **RBAC**: ext Kinds MUST be in `pkg/rbac` `extKinds` (absence = fail-closed 403
  for every caller) AND have an explicit authorization rule; v1 Kinds have their
  rule. Tenancy (per-gateway, etc.) enforced where the spec requires.
- **Proto/CP**: proto message + RPCs regenerated; if the control plane watches the
  Kind, the watcher subscription and reconciler handle it.
- **Tests (MANDATORY, not optional)**: a **DB-backed integration test**
  (testcontainers, `API_ENV=integration_testing`) that drives
  create -> get -> list -> delete over the real HTTP+DB stack, and an
  authorization test. Mock-only grpc tests do NOT satisfy this - they never touch
  the migration or schema, which is precisely how broken Kinds have shipped.

### Conformance pass (idempotent - run it every time)

Before any wave closes, run the conformance pass over **all** registered Kinds
(not just the ones this change touched):

1. Enumerate every Kind from the plugin registrations (`main.go` side-effect
   imports / `RegisterPrefixedRoutes`), the `extKinds` map, and `urls`.
2. For each Kind, diff it against the manifest above.
3. For every missing artifact, generate and add it. This is **additive and
   idempotent**: a Kind that already conforms produces no change; a second run of
   the pipeline yields an empty diff. Re-running the pipeline is how pre-existing
   incomplete Kinds get filled in - never a one-off hand fix to a single Kind.

### Conformance gate (the thing that stops the surprise)

The pipeline MUST establish and keep green an automated gate so a non-conformant
Kind cannot merge:

- **Static check** wired into `make check` (runs in the `Checks CI Gate`):
  enumerate every registered Kind and FAIL, naming each Kind and each missing
  item - any manifest file, OpenAPI/SDK/CLI/`extKinds` entry, an `AutoMigrate` in
  a migration, or the absence of a DB-backed integration test.
- **Generic CRUD conformance test** wired into `make test` (testcontainers):
  iterate every registered ext Kind and assert a full create -> get -> list ->
  delete round-trip over the real stack. A broken migration or missing CRUD fails
  CI. This operationalizes "100% of generated Kinds work" as an enforced
  invariant, not a hope.

If either gate is red, the wave is not done. Treat a Kind that the gate reports
as incomplete as implementation work, exactly like a failing test.

## UI completion contract

For a new or materially changed production page with an approved mockup,
execute `skills/build/ui-build-gate/SKILL.md` before closing its wave. Pending
gate work is not a terminal state.

If the UI change has no corresponding mockup, warn the user and continue the
wave. Do not create a mockup unless the task includes mockup or spec work. Skip
`ui-build-gate` and all mockup-to-production parity checks; run the remaining
wave checks.

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

For any spec that includes web-console UI, check
`specs/web-console/mockups/` for the corresponding mockup before implementation.

- If no corresponding mockup exists, warn the user and continue without
  mockup-to-production parity checks.
- If an approved mockup exists, read its stories and relevant shared components
  and use it as the visual and interaction reference. The browser experience
  SHOULD remain recognizably consistent with it.

Do not copy mockup code into production. The implementer MAY refine structure,
component selection, responsive behavior, accessibility, localization, and
application integration. Follow repository UI standards and application
boundaries; mockup PatternFly choices are recommendations.

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
| **CLI** | Does `hypershell` implement every route marked ✅ implemented in the spec CLI table? Check `components/cli/cmd/hsctl/*/cmd.go`. |
| **gRPC** | Do proto definitions cover all fields? Do handlers and presenters exist? |
| **CP** | Does the watcher subscribe to all Kinds? Does the reconciler handle all events? |

Produce a gap table:

```
ENTITY          COMPONENT   STATUS      GAP
ManagedCluster  API         present     --
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
- BE: migrations (raw SQL, never `AutoMigrate`), DAOs, service logic, presenters
- gRPC: proto updates, `make proto`, handler/presenter implementation
- Tests: the mandatory DB-backed integration test (create/get/list/delete over the
  real stack) and authorization test for every Kind in this change
- RBAC: ext Kinds registered in `pkg/rbac` `extKinds` with an explicit rule
- Security gate: all handler paths check user token (when auth enabled); no tokens in logs
- **Run the idempotent conformance pass over ALL Kinds** and make the conformance
  gate green (see "Kind Completeness" above)
- Acceptance: `make test`, `make check`, `go vet ./... && golangci-lint run` clean

**Wave 5 -- CLI** (parallel after Wave 3)
- Implement all planned commands from spec CLI table
- Commands go in `components/cli/cmd/hsctl/{verb}/{resource}/cmd.go`
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
- Run e2e test suite: `E2E_INFRA_DRIVER=kind bash tests/e2e/e2e-openshell.sh`
  (OpenShift: `E2E_INFRA_DRIVER=openshift`; pull-request CI is the ephemeral
  PR environment workflow. The legacy `components/pr-test/e2e-openshell.sh`
  script has been removed.)

#### Capability-gated Storybook screenshot hard stop

For a new or materially changed production page with an approved mockup, read
and execute `skills/build/ui-build-gate/SKILL.md` after the production page and
story render together. If the agent runtime can inspect images, wait for
`Complete`, and record the validated manifest path in the wave handoff.

If the runtime cannot inspect images, record `Complete without visual review`
and the capability reason in the wave handoff, then continue all non-parity
checks. Do not block the wave on unavailable visual comparison.

If no corresponding mockup exists, retain the earlier warning in the wave
handoff and skip this hard stop.

Each wave is a gate. Do not start downstream work against an unstable upstream.

### Kind Removal: Reverse-Dependency Execution

When a gap item is prefixed `RM-` (kind removal), the pipeline runs in **reverse** dependency order - the opposite of the normal forward order above. Use the Kind Removal Wave Pattern documented in `/reconcile` SKILL.md:

```
FE (RM-W1) -> CLI (RM-W2) -> CP (RM-W3) -> BE+gRPC (RM-W4) -> API+SDK (RM-W5) -> DB migration (RM-W6)
```

Key differences from an addition wave:
- Start from the leaf layers (UI/CLI) and work inward toward the data layer
- Gate: `go build ./...` + `go vet ./...` must pass after RM-W3 before deleting the backend plugin
- Gate: no application code may import the dropped model before adding the RM-W6 drop migration
- Verify each layer with `grep -r "<KindName>" components/` to confirm no stale references remain

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
  --library github.com/openshift-online/rh-trex-ai/components/api-server
```

The generator is a starting point, not the finish line. After running it you MUST
bring the Kind to full conformance with the Kind Completeness Manifest above:

- **Rewrite the generated migration to raw SQL** (`tx.Exec` `CREATE TABLE` /
  `ALTER TABLE ... IF NOT EXISTS`). If the generator emitted `AutoMigrate`, it is
  wrong - replace it. The conformance gate rejects `AutoMigrate`.
- **Add the DB-backed integration test and the authorization test** - the
  generator does not produce them, and their absence is the exact gap that let
  broken Kinds ship. A Kind without them is incomplete.
- **Register the ext Kind in `pkg/rbac` `extKinds`** and add its authorization
  rule, or it fails closed (403) for every caller.
- **Add the CLI `create`/`get`/`list`/`delete`/`apply`** and regenerate the SDKs.

Then run the conformance pass (it is idempotent) and confirm the conformance gate
is green before the wave closes.

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
- **Kind conformance is mandatory and enforced**: every registered Kind MUST satisfy the Kind Completeness Manifest; the conformance gate (`make check` static + `make test` CRUD round-trip) MUST be green. A non-conformant Kind is an incomplete feature, not a follow-up.
- **The conformance pass is idempotent**: running the pipeline fills in every missing artifact for every Kind and, run twice, produces no diff. Never hand-fix one Kind in isolation - fix the pipeline and let it fill in all Kinds.
- **No `AutoMigrate`**: migrations are raw SQL via `tx.Exec`; the gate rejects `AutoMigrate`.
- **Tests cite the scenario they prove**: put `// Verifies: <scenario-ID>` (or `# Verifies:`) on the line above each test that asserts a spec scenario's THEN clause. The format is in `specs/standards/platform/spec-identifiers.spec.md` (STD-IDS-07). `make check` rejects unknown IDs; `make traceability` shows coverage.
