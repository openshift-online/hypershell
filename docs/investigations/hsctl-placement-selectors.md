# Investigation: hsctl placement selectors (steering gateway placement from the CLI)

**Date:** 2026-10-29
**Status:** Investigation / design proposal - no product code changed
**Goal:** Let an `hsctl` user steer where a gateway is placed (which cluster,
spoke or region) at create time, instead of accepting the platform's random
selection among eligible clusters or pinning an opaque `cluster_id`.
Read-only investigation - no product code was changed on this branch.

---

## Verdict

The hard part already exists and is shipped. The API server has a complete
capability-label placement system - `GatewayCreateRequest.placement` with
`selector` / `excludedSelector` / `mode: local-kind`, server-side resolution,
late binding, and a batched availability endpoint - and the platform spec
(`data-model.spec.md`'s API↔CLI mapping table) already documents
`hsctl create gateway --name <n> (--cluster-id <c> | --placement <sel>)` as
"✅ implemented". **That flag does not exist.** `hsctl create gateway`
(`components/cli/cmd/hsctl/create/gateway/cmd.go`) exposes only `--cluster-id`,
`--dedicated-route`, `--name`, and the `--body` JSON escape hatch. Placement is
reachable from the CLI only through `--body request.json` or `hsctl apply` of a
`.hypershell` Gateway manifest.

The recommended minimal first version is therefore a pure CLI feature: add
repeatable `--placement-selector` / `--placement-exclude-selector` flags (plus a
`--local-kind` convenience) to the already hand-maintained `hsctl create
gateway` command, mutually exclusive with `--cluster-id`, mapping 1:1 onto the
existing request fields. No API, scheduler, schema, or SDK change is required.
A second, optional increment surfaces the existing
`POST /gateways/placement-availability` endpoint as
`hsctl create gateway --dry-run` (or a `hsctl placement availability` helper).
Section 6 has the full plan; Section 8 lists open questions.

---

## 1. Placement today: how the platform decides where a gateway lands

### 1.1 The two creation shapes

A gateway is created with exactly one of two placement shapes
(`specs/platform/data-model.spec.md:269`, "Requirement: PLAT-DATA-02 - Gateway
Placement and Late Binding"):

- **Pinned.** The request carries `cluster_id` referencing a registered
  `ManagedCluster` (non-empty `oidc_subject`). Enforced by
  `validateClusterReference` at
  `components/api-server/plugins/gateways/cluster_reference.go:43` - an unknown
  or manually created cluster is a 400 because no control plane would ever
  watch it.
- **Late-bound.** The request carries `placement` - a capability-label
  selector or a `local-kind` mode. The API server resolves it server-side.

Exactly one must be supplied; both, or neither, are rejected
(`components/api-server/plugins/gateways/handler.go:190-203` for REST;
`components/api-server/plugins/gateways/grpc_handler.go:161-182` for gRPC).
`placement` and `cluster_id` are both **create-only** user-writable fields,
in the same ownership class as `dedicated_route`
(`components/api-server/plugins/gateways/field_policy.go:46-47`).

### 1.2 The data model: clusters, capabilities, regions, spokes

- A **ManagedCluster** (`components/api-server/openapi/openapi.managedClusters.yaml:290`)
  has `name` (the human-readable **spoke** name, e.g. `hyp0-mc1`, unique per
  platform), `provider`, `region`, `visibility` (public/vpn), and
  `capabilities` - a read-only set of self-declared capability labels.
- **Capabilities are namespaced labels** with a closed vocabulary:
  `provider/*`, `network/*`, `region/*`, `runtime/*`, `environment/*`
  (`components/api-server/plugins/managedClusters/service.go:181`). Each
  segment must be a lowercase DNS label
  (`capabilitySegmentRE`, `service.go:172`). `provider/`, `region/`, and
  `environment/` are single-valued namespaces (`service.go:182`).
- **Region and provider ARE capability labels** in the placement sense: a
  spoke that self-declares `region/us-east-1` is selectable by that label. The
  inventory presenter derives its region/provider columns from the
  `region/` and `provider/` capability labels with the column as fallback
  (`components/api-server/plugins/managedClusters/presenter.go:38-41`).
  `spoke name` (the `name` column) is NOT a capability today - it is only
  usable as a `cluster_id` pin after looking the KSUID up via
  `GET /api/hypershell/v1/managed_clusters`.
- Clusters self-declare capabilities at registration, replacing the whole set
  on each heartbeat: the control plane sends
  `HYPERSHELL_MANAGED_CLUSTER_CAPABILITIES`
  (`components/control-plane/internal/config/config.go:193`, parsed at
  `config.go:274`). Examples in-tree: `provider/kind,network/public,environment/kind`
  (`deploy/kind/kustomization.yaml:71-72`), `provider/aws,network/public`
  (`deploy/base/platform-resources/controller.yaml:63-64`),
  `provider/ibm,network/public` (`deploy/ibm/kustomization.yaml:67-68`).

### 1.3 The scheduler (placement resolver)

The entire scheduler lives in the gateways plugin:

- **Eligibility** - `eligiblePlacementCandidates`
  (`components/api-server/plugins/gateways/placement.go:45`): a candidate is
  eligible when its control plane is *connected* (registration heartbeat
  younger than 5 minutes, `PlacementControlPlaneConnected`,
  `placement.go:24-31`), its capability set **contains every** selector label
  (match-all, literal, case-sensitive - `capabilitySetContains`,
  `placement.go:79`), and **contains none** of the `excludedSelector` labels
  (`capabilitySetExcludes`, `placement.go:90`).
- **Selection** - `resolvePlacement` (`placement.go:63`): uniform
  **crypto-random selection among eligible clusters**. No capacity, scoring,
  ranking, or preferences - an explicit spec decision
  (`specs/web-console/gateway-provision-placement.spec.md`, WEB-PLC-07:
  "The backend SHALL NOT inspect, score, or rank clusters by CPU, memory, pod
  usage, or other capacity data").
- **Wiring** - `registeredPlacementService`
  (`components/api-server/plugins/gateways/plugin.go:180`): `snapshot()`
  (`plugin.go:189`) loads registered clusters (non-empty `oidc_subject`,
  connected) into `PlacementCandidate{ID, Capabilities, Connected}`;
  `Resolve()` (`plugin.go:237`) handles the `local-kind` mode (only valid in
  the Kind dev environment - WEB-PLC-08, hard error otherwise) and delegates
  managed placement to `ResolvePlacement`.
- **Late binding** - when no eligible cluster matches, the create still
  succeeds: the gateway is persisted with empty `cluster_id`, `phase=Pending`,
  and status "Awaiting placement: no connected managed cluster matches the
  placement selector" (`applyAwaitingPlacement`,
  `components/api-server/plugins/gateways/handler.go:149`;
  `placement_persistence.go:15`). The `GatewayPlacementRebinder` controller
  (`plugin.go:394`) runs `resolvePendingPlacements`
  (`components/api-server/plugins/gateways/placement_rebind.go:26`) on every
  ManagedCluster registration/capability change and assigns `cluster_id` as
  soon as a match appears (writing as the control-plane writer kind per the
  field policy).
- **Persistence** - the selector is stored on the Gateway row as a stable JSON
  document `{"selector":[...],"excludedSelector":[...],"mode":"..."}`
  (`storedPlacement`, `placement_persistence.go:20`; jsonb column
  `Placement` at `components/api-server/plugins/gateways/model.go:27`, added by
  `migrationAddPlacement` at `migration.go:288`). `FindUnplaced` scans for
  rows with empty `cluster_id` and non-null `placement`
  (`components/api-server/plugins/gateways/dao.go:189`).

### 1.4 Other placeable resources

- **AgentRuntime** is the only other resource with placement today. Its
  `cluster_id` is optional on create; when omitted it is *derived from the
  referenced Gateway's placement* server-side (spec PLAT-DATA-08,
  `specs/platform/data-model.spec.md:808`; effective-cluster resolution in
  `components/api-server/plugins/agentRuntimes/grpc_handler.go:74`). The CLI
  surfaces this as `hsctl create agentRuntime --cluster-id <id>` documented
  "When omitted, derived from the gateway's placement."
  (`components/cli/cmd/hsctl/create/agentRuntime/cmd.go:44`). Note:
  AgentRuntime's *other* `selector` field (repository label match-any scanner,
  PLAT-DATA-09) is an unrelated concept that happens to share the word.
- Everything else (service accounts, sandboxes, routes) is gateway-scoped and
  inherits the gateway's cluster.

### 1.5 Existing selector/affinity concepts

| Concept | Where | Semantics |
|---|---|---|
| Gateway `placement.selector` | REST + gRPC create | match-all equality over capability labels, create-only, persisted, late-bound |
| Gateway `placement.excludedSelector` | REST + gRPC create | match-none (negative) labels, same validation |
| Gateway `placement.mode: local-kind` | REST + gRPC create | named-mode pin to the dev Kind cluster, hard-fails outside Kind |
| Gateway `cluster_id` | create (and historic PATCH) | pin to a specific registered cluster by KSUID |
| AgentRuntime `cluster_id` / gateway-derived | create | inherits or pins |
| ManagedCluster `capabilities` | registration | self-declared label set, source of truth for matching |

There is **no** set-based expression syntax (`k in (a,b)`, `!k`), no affinity
weights/preferences, and no capacity concept anywhere - all deliberately
rejected (Section 4).

## 2. API surface: what exists, what would need adding

**Nothing needs adding for the core feature.** The public API already accepts
placement on create:

- REST: `GatewayCreateRequest` (`components/api-server/openapi/openapi.gateways.yaml:414`)
  has `name` (required), `cluster_id` (pin, "Mutually exclusive with
  placement"), `dedicated_route`, and `placement` → `GatewayPlacementIntent`
  (`openapi.gateways.yaml:448`): a `oneOf` of capability-selector
  (`selector`, `excludedSelector` - `GatewayPlacementSelector`, an array of
  strings, minItems 1) or `mode: local-kind`.
- gRPC: `GatewayPlacementIntent` / `CapabilitySelectorPlacement` messages
  (`components/api-server/proto/hypershell/v1/gateways.proto:24-33`),
  `placement` field 18 on the create request.
- Availability: `POST /api/hypershell/v1/gateways/placement-availability`
  (`openapi.gateways.yaml:93`) - batch of up to 32 selectors, returns
  `{selector, available, reason: no-eligible-cluster}` per selector plus a
  `local_kind` boolean. Handler:
  `components/api-server/plugins/gateways/handler.go:94`; resolver
  `plugin.go:287`.
- Validation: `validatePlacementIntent`
  (`cluster_reference.go:22`) - exactly one of selector or mode; both selector
  lists normalized through the capability vocabulary
  (`managedClusters.NormalizeSelector`, `service.go:205`), which rejects
  unknown namespaces, bad segments, and (for declarations) duplicates.
- Persistence: jsonb `placement` column on `gateways`
  (`model.go:27`, `migration.go:288`); rebind sweep as above.

Two smaller API-side gaps are worth noting (both optional for v1):

1. **Read model does not expose `placement`.** The Gateway response schema
   (`openapi.gateways.yaml:260-410`) carries `cluster_id` (empty while
   awaiting) and the awaiting-placement `status`, but not the persisted
   selector itself - `PresentGateway`
   (`components/api-server/plugins/gateways/presenter.go:21`) copies no
   placement field. A user cannot ask "which selector is this gateway waiting
   on?" through the public API.
2. **`--dry-run` style pre-check** would reuse the availability endpoint
   as-is; no server change needed.

If the read-model gap is closed, the change surface is the familiar
`#815` checklist: OpenAPI fragment → `make -C components/api-server generate`
→ presenter → `make generate-sdk-go` / `generate-sdk-typescript` /
`generate-cli` / `generate-tui` (all present as Makefile targets; the CLI
generator is `scripts/generate-cli.sh`).

## 3. hsctl today: create commands, flag conventions, and the #815 template

### 3.1 Structure

The CLI is a cobra application under `components/cli/cmd/hsctl/`, one package
per verb per resource (`create/gateway/cmd.go` etc.). Most commands are
**generated** from the OpenAPI spec by the pinned rh-trex-ai cli-generator
(`scripts/generate-cli.sh`); a hand-maintained allowlist
(`scripts/generate-cli.sh:20-44`) protects commands the generator cannot
produce. `create gateway` is on that list (line 38) precisely because
create-only request fields like `dedicated_route` are `readOnly` in the
response schema and the generator derives flags from the response - the
rationale is documented in the script.

The CLI does **not** use the generated Go SDK (`components/sdk-go` is
consumed by the fleet dashboard and e2e, not the CLI). Commands use the raw
HTTP layer: `config.Load()` → `connection.NewConnectionBuilder()` →
`conn.Post(urls.GatewaysPath, body)` with `pkg/urls` path constants, and build
request bodies as plain `map[string]interface{}` marshaled to JSON
(`create/gateway/cmd.go:63-78`).

### 3.2 `hsctl create gateway` today

`components/cli/cmd/hsctl/create/gateway/cmd.go`:

- flags (lines 36-41): `--cluster-id` (string), `--dedicated-route` (bool),
  `--name` (string), `--body` (JSON file escape hatch);
- mapping (lines 63-78): each flag is added to the request map only when
  non-zero - `--cluster-id` becomes `request["cluster_id"]` verbatim;
- no client-side validation of `cluster_id` (existence/registration is
  enforced server-side with an actionable 400);
- output: the raw created resource, pretty-printed (`dump.Pretty`).

`hsctl get gateway <id>` prints the pretty JSON of the response
(`get/gateway/cmd.go`), so it already shows `cluster_id`, `phase`, and the
awaiting-placement `status`. `hsctl list gateways` defaults columns
`id, active_sandbox_count, can_delete, can_edit, cluster_id, created_at`
(`list/gateways/cmd.go:47`).

### 3.3 Flag conventions

| Pattern | Example | Where |
|---|---|---|
| one kebab-case flag per snake_case field | `--cluster-id` → `cluster_id` | all create/update commands |
| repeated flag (StringArray) | `--parameter name=value` (repeatable) | `pkg/arguments/arguments.go:9-16` |
| comma-separated list | `--columns a,b,c`; `--scope` (StringSlice) | `arguments.go:19-26`; `login/cmd.go:56` |
| `name=value` pair parser | `arguments.ParseNameValuePair` | `arguments.go:39-48` |
| mutual exclusion | `Cmd.MarkFlagsMutuallyExclusive("filename", "kustomize")` | `apply/cmd.go:49` |
| JSON escape hatch | `--body request.json` | every create/update |

There is **no existing `--label k=v` or Kubernetes-style `--selector`
expression syntax anywhere in the CLI**. The server's own selector values are
already single tokens (`network/public`), not `k=v` pairs.

### 3.4 The #815 (`dedicated_route`) template

Commit `8bd9b15e` (PR #815) is the model for a create-time option. Its CLI
diff was deliberately small because the command was already hand-maintained:
+1 field in the args struct, +1 `fs.BoolVar`, +1 `request[...]` entry, and the
help example updated. The heavy parts were all server-side (OpenAPI + proto +
model/migration/field_policy/service/handler/RBAC role + SDK regens + specs).
**For placement, all of the server-side work already shipped** (issues
#499/#505, #511/#513, #623/#625, #630/#639); only the CLI flag work remains.

## 4. Specs and prior discussion

### 4.1 Canonical specs (all in-tree, decided, implemented)

- `specs/web-console/gateway-provision-placement.spec.md` - the placement
  contract despite living under web-console (registered WEB/API/CP-wide in
  `specs/index.spec.md`). WEB-PLC-01 (intent contract), WEB-PLC-04 (batched
  availability), WEB-PLC-07 (resolution: random, no capacity, late-bind),
  WEB-PLC-08 (local-kind), WEB-PLC-09 (compat: pinned shape "remains
  available to hsctl apply and the declarative .hypershell import").
- `specs/platform/data-model.spec.md` PLAT-DATA-02 (placement + late binding),
  PLAT-DATA-23 (field ownership: name + placement create-only), and the
  API↔CLI mapping table (~line 1691) that already promises
  `--placement <sel>`.
- `specs/platform/managed-cluster-registration.spec.md` PLAT-CLUS-06/07/09
  (self-declared capabilities; the decided rationale at line 530:
  "Namespaced string capabilities with match-all selectors … remain
  independent and extensible without a provider-by-network schema matrix").
- `specs/platform/control-plane.spec.md` CTRL-CORE-14 (.hypershell import maps
  `placement`; the working example is `.hypershell/base/platform/gateway.yaml`
  - `placement: {selector: [network/public, provider/ibm]}`).

### 4.2 Decided / rejected / open history (GitHub)

- **#499 / PR #505** - founding decision: replaced the hard-coded
  provider×network availability matrix with capability labels + selectors.
- **#623 / PR #625** - `cluster_id` optional; late binding for declarative
  gateways.
- **#511 / PR #513, then PR #823** - version-based placement via a
  `gateway-version/<tag>` capability was added and then **removed** ("no
  fleet unit advertises it"). Precedent: a placement dimension only exists
  when fleet units actually advertise the capability.
- **#404 (NOT_PLANNED)** - "let the user pick a specific cluster in the
  provision form" was rejected; spec Non-Goal: "Allowing users to select a
  concrete managed cluster from the provision form."
- **#750 / #831 (closed, decided; not yet implemented on this branch)** -
  security review: any `gateway:creator` can pin any registered cluster via
  `cluster_id`. Decision: `cluster_id` pinning becomes platform-admin-only on
  the user path; identity-targeting selectors (anything that resolves to one
  named cluster) are likewise disallowed for normal users - "identity-by-
  selector is the same capability as cluster_id". A narrow pinning role was
  floated. Note the current code (`handler.go` Create +
  `validateClusterReference`) still allows any creator to pin; the fix is
  in-flight.
- **PR #745 (open)** - server-assigned `managed-cluster/<name>` identity
  capability: admins could target a specific spoke by name via
  `placement.selector: ["managed-cluster/<name>"]`; requirement-only selectors
  keep random resolution. Not merged; spec changes live on the PR branch.
- **#720 (open)** - e2e matrix pinning, to be reworked onto #745.
- **#677 (open)** - spec/test mismatch on empty `cluster_id` vs invalid
  placement rejections in e2e.
- Capacity-aware placement, scoring, preferences: rejected everywhere
  (Non-Goals).

### 4.3 Process for new features

`skills/plan/spec/SKILL.md` defines the spec workflow: Frame → ground in
codebase → draft (Purpose / Requirements / Given-When-Then scenarios with
stable `MODULE-GROUP-NN` IDs registered in `specs/ids.md`) → critic pass →
apply under `specs/{domain}/`. A CLI-only placement feature would most
naturally amend `specs/platform/data-model.spec.md` (API↔CLI mapping) and/or
add an `hsctl-*` spec; `placement` semantics themselves are already fully
specified and need no changes. No web-console page changes → no mockup
needed. This investigation follows the `docs/investigations/` convention
(see `per-gateway-certificates.md`, the direct ancestor of #815).

## 5. Selector semantics options

| Option | Data-model support today | What would be new | No-match behaviour |
|---|---|---|---|
| **Equality labels** (`network/public`), match-all | ✅ fully supported (WEB-PLC-07): validated closed vocabulary, persisted, late-bound | nothing (CLI flags only) | create succeeds, `Pending` + awaiting-placement status, rebinds on cluster changes |
| **Excluded labels** (`--exclude runtime/kata`) | ✅ fully supported (`excludedSelector`) | nothing (CLI flag only) | same as above |
| **Set-based / K8s-style expressions** (`provider in (aws,ibm)`, `!kata`) | ❌ none; spec explicitly forbids: "partial, wildcard, and match-any semantics SHALL NOT apply" (WEB-PLC-07) | new selector DSL in API validation + resolver + availability + specs; contradicts a decided Non-Goal | n/a - recommend not doing |
| **Named cluster / spoke pin** (`--cluster hyp0-mc1`) | partial: `cluster_id` pin by KSUID exists; PR #745 proposes `managed-cluster/<name>` identity capability (admin-only) | name→KSUID lookup in CLI, or land #745; **RBAC**: #831 decision restricts pinning to platform-admin (or a narrow role) for users | hard 400 today (unregistered cluster) |
| **Region pin** (`--region us-east-1`) | ✅ as a plain capability label `region/us-east-1` (single-valued namespace) | nothing - it is just an equality selector; optionally sugar in the CLI | late-binds |
| **Soft preferences vs hard requirements** | ❌ none; uniform random among eligible, deliberately | preference weights, scoring - contradicts decided Non-Goals (no capacity/scoring) | n/a - recommend not doing |

### 5.1 RBAC and tenancy

- **Requirement selectors are tenant-safe**: any `gateway:creator` may submit
  any capability selector; the platform chooses the concrete cluster, so no
  cross-tenant targeting is possible. This is exactly why the console form
  submits intent rather than `cluster_id` (WEB-PLC-01: "The client SHALL NOT
  submit a user-selected cluster_id").
- **Pin-to-specific-cluster is being locked down** (#831, decided): a plain
  user pinning `cluster_id` (or an identity selector) bypasses placement
  policy. The hsctl surface should therefore treat `--cluster-id` as the
  admin/power-user escape hatch it already is, and not add new
  user-level identity-targeting affordances ahead of #745/#831 landing.
- There is **no per-cluster entitlement** concept - tenancy is per-gateway
  (owner bindings), not per-cluster.

### 5.2 Interaction with dedicated routes and per-gateway certs

`dedicated_route` is orthogonal to placement: it is a create-only,
RBAC-gated (`gateway:dedicated-route-creator`) request flag
(`components/api-server/plugins/gateways/dedicated_route.go:109`) translated
server-side into the platform-managed `route.certificate` opt-in. It composes
freely with both pinned and late-bound shapes - the CLI already has
`--dedicated-route`, and the recommended placement flags must simply be
combinable with it in one command (both map into the same request body).
One nuance worth documenting for users: a late-bound gateway stays
unplaced until a match appears, so a dedicated-route gateway created with an
unsatisfiable selector will hold in Pending indefinitely with its cert path
not yet provisioned - the awaiting-placement `status` is the signal.

## 6. Recommended minimal first version

**Scope: CLI-only. No API, scheduler, schema, or SDK changes.**

### 6.1 CLI flags and syntax

In `components/cli/cmd/hsctl/create/gateway/cmd.go` (already hand-maintained,
so no generator changes):

```
hsctl create gateway --name my-gw \
    [--placement-selector network/public] \        # repeatable
    [--placement-exclude-selector runtime/kata] \  # repeatable
    [--local-kind] \                               # shorthand for mode: local-kind
    [--cluster-id <id>] \                          # unchanged pin shape
    [--dedicated-route] [--body file.json]
```

- `--placement-selector` / `--placement-exclude-selector`: `StringArrayVar`
  (repeatable, like `--parameter`), each value a single capability label.
  Comma-separated input can be rejected with a clear message rather than
  silently split (labels themselves never contain commas; splitting
  `provider/aws,network/public` is convenient but ambiguous for typo'd
  values - see Open Questions).
- `--local-kind` sets `placement: {mode: "local-kind"}`; mutually exclusive
  with both selector flags.
- Placement flags mutually exclusive with `--cluster-id` via
  `Cmd.MarkFlagsMutuallyExclusive` (convention exists in `apply/cmd.go:49`),
  with the server's exactly-one-of rule as backstop.
- Body mapping: selectors → `request["placement"] = map{"selector": [...],
  "excludedSelector": [...]}` (or `{"mode": "local-kind"}`), only when at
  least one placement flag was passed; `--body` continues to bypass flag
  building entirely.
- Client-side pre-validation (nice-to-have): reuse the vocabulary rules -
  namespace in {provider, network, region, runtime, environment}, lowercase
  DNS-label segments - to fail fast with the same message shape the server
  returns. The server remains authoritative.
- Help text updated with examples, including the region case
  (`--placement-selector region/us-east-1`).

### 6.2 API fields, validation, scheduler

None. `GatewayCreateRequest.placement`, `validatePlacementIntent`,
`NormalizeSelector`, `ResolvePlacement`, the rebind controller, and the
availability endpoint all already exist and are tested. The e2e suite
already exercises the placement-shaped payload directly
(`tests/e2e/gateway_create_request_test.go:8-46`).

### 6.3 How it's stored and shown

- Storage: unchanged (persisted jsonb selector + assigned `cluster_id`).
- `hsctl get gateway <id>` / `hsctl list gateways` already show `cluster_id`,
  `phase`, `status` - an awaiting-placement gateway is visible as empty
  `cluster_id` + the awaiting status. Default list columns need no change.
- Optional follow-up (server change, not v1): expose `placement` on the
  Gateway read model so `hsctl get gateway` shows which selector a
  Pending gateway is waiting on. Today the only trace is the human-readable
  status string.

### 6.4 Optional second increment

`hsctl create gateway --dry-run` (or a dedicated
`hsctl placement availability --selector ... --selector ...`) calling the
existing batched `POST /gateways/placement-availability` endpoint, printing
per-selector availability. Pure client work; helps users discover valid
labels before submitting. A `hsctl list managedClusters` (already
hand-maintained to show `capabilities`) is the discovery path today.

### 6.5 Tests

- Unit (`components/cli`, run via `go test ./...` under `make unit-test-all`):
  - flag → request-body mapping (selectors, exclude, local-kind, mutual
    exclusion, combination with `--dedicated-route`, `--body` precedence);
  - command-tree test (`main_test.go` TestCommandTree) still passes.
- Drift: `make check-cli-drift` must stay green (no generator changes, so it
  will).
- E2E (extend `tests/e2e/`): `hsctl create gateway --placement-selector
  network/public --placement-selector provider/<driver-provider>` against
  Kind/OpenShift drivers, asserting the created gateway's `cluster_id`
  resolves and that `--cluster-id` + selector together fail with the
  server's 400.
- No api-server tests change (nothing server-side moves).

### 6.6 Spec work

Per `skills/plan/spec/SKILL.md`: update the API↔CLI mapping row in
`specs/platform/data-model.spec.md` (~line 1691) so the documented
`--placement <sel>` syntax matches what actually ships (today that row claims
"✅ implemented" for a flag that does not exist - a real spec/reality drift
this work fixes). If a fuller spec is wanted, add scenarios to a new
`specs/platform/hsctl-placement.spec.md` (CLI-PLC-xx IDs registered in
`specs/ids.md`): flag contract, mutual exclusion, mapping, no-match UX
(awaiting-placement status surfaced to the user), local-kind flag,
availability helper. Do not touch `gateway-provision-placement.spec.md`
semantics.

## 7. Open questions for the user

1. **Flag granularity**: one repeatable `--placement-selector` (recommended)
   vs a single `--placement` taking a comma-separated list vs named
   conveniences (`--region`, `--provider`, `--network`) that expand to
   capability labels. Sugar improves discoverability but duplicates the
   vocabulary in the CLI.
2. **Comma-splitting**: should `--placement-selector provider/aws,network/public`
   be split, rejected, or is repeat-only fine?
3. **Naming**: `--placement-selector` is verbose but unambiguous;
   `--selector` alone collides with AgentRuntime's unrelated repository
   `--selector` concept and Kubernetes muscle memory (set-based expressions
   that the platform deliberately does not support).
4. **local-kind**: boolean flag `--local-kind`, or `--placement-mode
   local-kind` mirroring the wire format exactly?
5. **Availability surfacing**: is the `--dry-run` / availability helper wanted
   in v1, or later?
6. **Read model**: expose `placement` on Gateway reads (server change) so
   users can see what an awaiting gateway is waiting for - now or later?
7. **Cluster pin UX**: with #831 (admin-only pinning) and #745
   (`managed-cluster/<name>` identity capability) in flight, should hsctl
   de-emphasize `--cluster-id` (help-text warning) or add
   `--placement-selector managed-cluster/<name>` support once #745 lands?
8. **e2e scope**: extend the hsctl e2e parity phases, or unit tests only for
   v1?

## 8. What the spec would contain (outline, not opened)

If the user opts for the fuller spec route
(`specs/platform/hsctl-placement.spec.md`, IDs `CLI-PLC-*` registered in
`specs/ids.md`):

- **Purpose**: surface the existing placement intent contract in hsctl.
- **Requirement: CLI-PLC-01 - Create flags**: `--placement-selector`
  (repeatable), `--placement-exclude-selector` (repeatable), `--local-kind`;
  exactly one of placement flags or `--cluster-id`; scenarios for each shape,
  mutual exclusion, and combination with `--dedicated-route`/`--body`.
- **Requirement: CLI-PLC-02 - Validation UX**: client-side vocabulary
  pre-check mirroring server messages; server 400s surfaced verbatim.
- **Requirement: CLI-PLC-03 - Awaiting-placement UX**: created output shows
  empty `cluster_id` + awaiting status without implying failure; `hsctl get
  gateway` reflects late binding when it resolves.
- **Requirement: CLI-PLC-04 - Availability helper** (if in scope): maps to
  `POST /gateways/placement-availability`, batch output per selector.
- **Non-Goals**: set-based selector expressions, preferences/scoring,
  user-level cluster pinning by name (deferred to #745/#831), capacity.
- Plus the one-line fix to the `data-model.spec.md` API↔CLI mapping row.
