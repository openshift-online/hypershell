# Implementation Plan: hsctl Gateway Placement Selector Flags

**Date:** 2026-10-29
**Branch:** `research/hsctl-placement-selectors` (implement on a fresh branch from `main`, e.g. `feat/hsctl-placement-selectors`)
**Companion research:** `docs/investigations/hsctl-placement-selectors.md`
**Scope:** CLI-only. No API server, control-plane, schema, proto, or SDK changes.

---

## Objective

Let `hsctl create gateway` submit the late-bound placement shape that the API
already supports (`GatewayCreateRequest.placement`), so a user can steer
placement by capability labels (`network/public`, `provider/ibm`,
`region/us-east-1`, ...) instead of pinning an opaque `--cluster-id` or
hand-writing a `--body` JSON file. Add an availability pre-check (`--dry-run`)
over the existing batched placement-availability endpoint.

Everything server-side already exists and is tested: selector validation and
normalization (`managedClusters.NormalizeSelector`), random resolution among
eligible clusters, late binding with the awaiting-placement status, and the
availability endpoint. The only prior CLI surface for placement is the
`.hypershell` manifest path (`hsctl apply`), which is unchanged by this plan.

## Decisions locked (user, 2026-10-29)

| Decision | Choice |
| --- | --- |
| Flag naming | **`--placement-selector`** and **`--placement-exclude-selector`**, repeatable (`StringArrayVar`), one capability label per use |
| Local-kind mode | **`--local-kind`** boolean, submits `placement: {mode: "local-kind"}` |
| Availability pre-check | **Yes, `--dry-run` in v1**: validates flags locally, calls `POST /gateways/placement-availability`, prints the result, creates nothing |
| Comma-separated values | **Rejected**: a flag value containing a comma is a client-side error with a pointer to the repeat syntax |
| Named sugar flags (`--region`, `--provider`) | Not in v1; a capability label is passed verbatim |
| Read-model `placement` exposure, `managed-cluster/<name>` pinning | Out of scope (server changes; see Follow-ups) |

## Design

### Flag contract

```
hsctl create gateway --name <n> \
    [--placement-selector <label>]...          # repeatable; match-all required labels
    [--placement-exclude-selector <label>]...  # repeatable; requires --placement-selector
    [--local-kind]                             # submits mode: local-kind
    [--cluster-id <id>]                        # existing pin shape, unchanged
    [--dedicated-route]                        # existing, composes with either shape
    [--dry-run]                                # availability pre-check; creates nothing
    [--body <file.json>]                       # existing escape hatch, unchanged
```

Rules (client-enforced; the server remains authoritative):

1. Any placement flag mutually exclusive with `--cluster-id`
   (`Cmd.MarkFlagsMutuallyExclusive`, convention at `apply/cmd.go:49`). The
   server's exactly-one-of rule is the backstop.
2. `--local-kind` mutually exclusive with `--placement-selector` and
   `--placement-exclude-selector` (the wire contract is a `oneOf`).
3. `--placement-exclude-selector` requires at least one
   `--placement-selector` (server rejects exclusion without selector; fail
   fast with the same meaning).
4. A placement flag value must be a single label: non-empty, no commas, no
   whitespace. Error text for commas:
   `--placement-selector takes one capability label per use (e.g. --placement-selector network/public); repeat the flag to combine labels`.
5. Label shape (mirrors `managedClusters/service.go:172-230` so typos fail
   fast with a client message instead of a server 400): `namespace/value`
   where namespace is one of `provider`, `network`, `region`, `runtime`,
   `environment`, and both segments match `^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`.
6. Duplicate labels across repeats are passed through; the server collapses
   them (`NormalizeSelector`). No client dedupe.
7. `--dry-run` mutually exclusive with `--body` (the escape hatch bypasses
   flag building, so there is nothing defined to check).
8. Excluding `--dry-run`, behavior when no placement flags and no
   `--cluster-id` are given is unchanged: the request has neither shape and
   the server's 400 ("a gateway must be created with either a cluster_id
   (pinned) or a placement selector (late-bound)") is surfaced verbatim.

### Request body mapping

In the existing `map[string]interface{}` builder, only when at least one
placement flag was passed and `--body` was not:

```go
// selector shape
request["placement"] = map[string]interface{}{
    "selector": args.placementSelector,          // []string
    // "excludedSelector" included only when non-empty:
    "excludedSelector": args.placementExcludeSelector,
}
// local-kind shape
request["placement"] = map[string]interface{}{"mode": "local-kind"}
```

Matches `GatewayCreateRequest.placement` /
`GatewayPlacementIntent` (`openapi.gateways.yaml:414-474`). `--dedicated-route`
continues to compose (both keys land in the same body). `--body` continues to
short-circuit flag building entirely, as today.

### Dry-run flow

1. Run the same client-side validation as a real create (rules 1-7 above).
   Invalid flags exit non-zero before any network call.
2. **Placement shape:** POST
   `/api/hypershell/v1/gateways/placement-availability` with
   `{"selectors": [<the combined required-label list>]}` (one selector entry;
   the endpoint accepts a batch but the CLI has one). RBAC: the endpoint
   requires `gateway:creator` (`authorization.go:560-563`), the same role
   needed to create, so no new entitlement.
   - available=true: print `Placement available: <labels>` and, when
     exclusions were given, the note below.
   - available=false (reason `no-eligible-cluster`): print
     `No connected managed cluster currently matches <labels>`. Include the
     platform semantics so it is not read as an error: `create would succeed
     and hold the gateway in Pending (awaiting placement) until a matching
     cluster connects`. Exit code 0: unavailability is a normal late-binding
     outcome, not a failure.
   - **Exclusions note** (always printed when `--placement-exclude-selector`
     is used): the availability endpoint evaluates required labels only
     (`plugin.go:287-313` builds `PlacementIntent{Selector: ...}` with no
     exclusions); excluded labels are enforced at create time.
   - **`--local-kind`:** make no availability request (the endpoint's
     contract requires at least one capability selector, which a local-kind
     intent does not declare); print that local-kind placement resolves only
     in the Kind development environment and is validated server-side at
     create. Exit 0.
   - **`--cluster-id`:** dry-run prints `pinned to cluster <id>; the server
     validates the cluster reference at create time` and makes no API call
     (availability is selector-based). No cluster lookup in v1.
3. No gateway is created in any dry-run path.

### Output of a real create

Unchanged mechanics (`dump.Pretty` of the created resource). A late-bound
gateway that resolved immediately shows its assigned `cluster_id`; one that
did not match shows empty `cluster_id`, `phase: Pending`, and the
`Awaiting placement: ...` status. One sentence is added to the command's
`Long` help explaining the two shapes and that an empty `cluster_id` in the
output means late binding, not failure.

## Implementation

### Phase 0: Spec (repo process requires spec-first)

1. New `specs/platform/hsctl-placement.spec.md`, group `CLI-PLC`,
   registered in `specs/ids.md` under the `CLI` section (alongside
   `CLI-EXT`/`CLI-GWC`/`CLI-REL`/`CLI-TUI`). Requirements and scenarios:
   - `CLI-PLC-01` flag contract: repeatable selector flags, `--local-kind`,
     mutual exclusions (cluster-id, local-kind vs selectors,
     exclusion-requires-selector), comma rejection, label vocabulary.
   - `CLI-PLC-02` request mapping: flags to `placement` body; composition
     with `--dedicated-route`; `--body` precedence.
   - `CLI-PLC-03` create UX: awaiting-placement output is not presented as
     failure; resolved `cluster_id` visible on get/list (already true).
   - `CLI-PLC-04` dry-run: availability call, output for
     available/unavailable/local-kind/pinned, exit 0 on unavailability,
     exclusions note, no create.
   - Non-Goals: set-based expressions, preferences/scoring, named-cluster
     pinning for users (deferred to the `managed-cluster/<name>` identity
     capability work), server-side changes.
2. Fix the drifted row in `specs/platform/data-model.spec.md` (~line 1691,
   API-to-CLI mapping): replace the not-yet-real
   `(--cluster-id <c> | --placement <sel>)` syntax with the shipped flags
   and keep status "implemented" only after this lands (until then the row
   should say the placement flags are planned).

### Phase 1: CLI validation helper (pure, testable)

New `components/cli/pkg/placement/placement.go`:

- `ValidateLabel(label string) error` - rules 4-5 (comma, empty, namespace
  allowlist, DNS-label segments). Duplicating the vocabulary list in the CLI
  is accepted (comment cross-referencing
  `managedClusters.NormalizeSelector` as authoritative; the server still
  validates).
- `BuildIntent(selectors, excludes []string, localKind bool) (map[string]any, error)`
  - validates every label, enforces rules 2-3 (local-kind exclusivity,
    exclusion-requires-selector), and returns the JSON-ready placement value
    (`selector` shape with optional `excludedSelector`, or `mode` shape).
- `AvailabilityRequest(selectors []string) map[string]any` - the
  `{"selectors": [labels]}` body for the dry-run call.

Keeping this out of `cmd.go` makes the mapping unit-testable without a
connection (the `run()` path dials config/connection immediately, as in every
generated command).

### Phase 2: command wiring

`components/cli/cmd/hsctl/create/gateway/cmd.go` (already hand-maintained in
`scripts/generate-cli.sh:38`; no generator changes):

- args struct: add `placementSelector []string`,
  `placementExcludeSelector []string`, `localKind bool`, `dryRun bool`.
- `init()`: `fs.StringArrayVar` for the two selector flags (convention:
  `--parameter` in `pkg/arguments/arguments.go:9-16`), `fs.BoolVar` for
  `--local-kind` and `--dry-run`.
  `Cmd.MarkFlagsMutuallyExclusive("cluster-id", "placement-selector",
  "placement-exclude-selector", "local-kind")` plus
  `("local-kind", "placement-selector")`, `("local-kind",
  "placement-exclude-selector")`, `("dry-run", "body")`. Note cobra's
  mutual-exclusion marks whole groups; the exclusion-requires-selector rule
  (3) is checked in code because it is a dependency, not an exclusivity.
- `run()`, before the existing body building:
  - if `--dry-run`: build + validate the intent (or pin echo), make the
    availability call, print, `return nil`.
  - else if any placement flag set: `intent, err := placement.BuildIntent(...)`;
    `request["placement"] = intent`.
- Update `Long` help and examples (both shapes, region example, dry-run
  example, awaiting-placement explanation).
- `components/cli/pkg/urls/ext.go`: add
  `GatewaysPlacementAvailabilityPath = GatewaysPath + "/placement-availability"`
  (hand-maintained path helpers live in `ext.go`; `urls.go` is generator-owned
  and `make check-cli-drift` reverts hand edits to it).

### Phase 3: tests

1. `components/cli/pkg/placement/placement_test.go` (new): table-driven -
   valid labels per namespace; invalid namespace/segment/comma/empty;
   `BuildIntent` selector shape, exclusion shape, `mode` shape; every
   mutual-exclusion and dependency error; `AvailabilityRequest` shape.
2. `components/cli/cmd/hsctl/create/gateway/cmd_test.go` (new): flag wiring
   (flags exist with right types/repeat-arity via `Cmd.Flags().Lookup`),
   and the dry-run/real-create branch selection where testable without a
   connection (keep pure logic in `pkg/placement`; here assert command
   registration and help).
3. Server-side: no changes, no new tests. (Placement validation, resolution,
   late binding, and availability are already covered by
   `placement_test.go`, `placement_validation_test.go`,
   `placement_late_binding_test.go`, `availability_handler_test.go`.)
4. E2E (optional slice, can defer): extend `tests/e2e` hsctl phases with a
   `hsctl create gateway --placement-selector network/public
   --placement-selector provider/<driver>` create/get/delete, and a
   `--dry-run` run, per driver (Kind local-kind, OpenShift selector shape;
   the payload builder at `tests/e2e/gateway_create_request_test.go:8-46`
   shows the per-driver shapes).

### Verification

```shell
cd components/cli && go test ./...     # unit (also runs under make unit-test-all)
make lint-cli                          # gofmt, go vet, golangci-lint
make check-cli-drift                   # must stay green: no generator changes,
                                       # create/gateway already hand-maintained
python3 scripts/check_forbidden_terms.py   # no em dashes etc. in new files
```

Manual smoke against a local Kind dev cluster (allowed; not the production
API): `hsctl create gateway --name t1 --dry-run --local-kind`, then a real
create, then `hsctl get gateway <id>` to watch late binding / assignment.

## Risks and mitigations

- **Vocabulary drift**: the CLI's namespace allowlist could fall behind a
  server-side addition. Mitigation: the client check is fail-fast only; the
  server message is authoritative and surfaced verbatim, and the spec
  cross-references `managedClusters.NormalizeSelector`. If this proves
  annoying, drop client-side namespace checking and keep only the
  comma/emptiness checks.
- **Cobra group exclusivity**: `MarkFlagsMutuallyExclusive` takes a set, so
  "any placement flag vs cluster-id" and "local-kind vs selectors" need
  separate calls; the dependency rule (exclusion requires selector) is
  enforced in `BuildIntent`, not by cobra.
- **Spec drift row**: `data-model.spec.md` claims `--placement` is
  implemented; Phase 0 corrects it in the same change so the spec never
  describes a third syntax.

## Follow-ups (explicitly out of scope here)

- Expose `placement` on the Gateway read model (server change; lets
  `hsctl get gateway` show what an awaiting gateway waits on).
- `managed-cluster/<name>` identity selector support once PR #745 lands
  (admin-only); revisit `--cluster-id` help text alongside the #831
  admin-only pinning change.
- Broader availability surfacing (standalone
  `hsctl placement availability` command) if dry-run proves insufficient.
- E2E parity-phase extension if deferred from Phase 3.
