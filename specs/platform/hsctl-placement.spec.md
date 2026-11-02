# hsctl Gateway Placement Selector Flags Specification

**ID prefix:** CLI-PLC

**Related:** [`web-console/gateway-provision-placement.spec.md`](../web-console/gateway-provision-placement.spec.md)
(the placement intent contract this CLI surface submits),
[`platform/data-model.spec.md`](../platform/data-model.spec.md) PLAT-DATA-02
(the exactly-one-of create contract) and PLAT-DATA-23 (field ownership),
[`platform/managed-cluster-registration.spec.md`](../platform/managed-cluster-registration.spec.md)
(the capability-label vocabulary)

## Purpose

`hsctl create gateway` SHALL expose the late-bound placement shape the API
already accepts: a gateway is created with capability-label selectors (or the
local-kind mode) instead of a pinned `cluster_id`. The server-side placement
system (selector validation, random resolution among eligible clusters, late
binding, and the batched placement-availability endpoint) is defined by
[`gateway-provision-placement.spec.md`](../web-console/gateway-provision-placement.spec.md)
and is unchanged by this spec; this spec covers only the CLI contract that
submits it. An availability pre-check SHALL let a user check a selector before
creating.

## Requirements

### Requirement: CLI-PLC-01 -- Placement Flags

`hsctl create gateway` SHALL accept:

- `--placement-selector <label>`, repeatable, each use carrying exactly one
  ManagedCluster capability label (a required, match-all label);
- `--placement-exclude-selector <label>`, repeatable, each use carrying
  exactly one capability label the candidate cluster SHALL NOT advertise;
- `--local-kind`, a boolean that submits the local-kind placement mode.

The placement flags SHALL be mutually exclusive with `--cluster-id` (the
pinned shape), mirroring the server's exactly-one-of contract
(PLAT-DATA-02). `--local-kind` SHALL be mutually exclusive with both
selector flags, mirroring the `oneOf` wire contract. A non-empty
`--placement-exclude-selector` set SHALL require at least one
`--placement-selector` use, mirroring the server's rejection of an excluded
selector without a selector. A flag value that is empty or contains a comma
SHALL be rejected before any request is sent, with an error that names the
repeat syntax. A flag value SHALL be rejected before any request is sent when
it is not `namespace/value` with a namespace among `provider`, `network`,
`region`, `runtime`, `environment` and both segments matching a lowercase
DNS-label; the server's validation remains authoritative and its message is
surfaced verbatim when the client check passes but the server rejects.

#### Scenario: CLI-PLC-01.01 -- Selector flags map to the placement intent

- GIVEN the user runs `hsctl create gateway --name gw --placement-selector network/public --placement-selector provider/ibm`
- THEN the request body SHALL contain `placement: {selector: ["network/public", "provider/ibm"]}`
- AND the request body SHALL NOT contain `cluster_id`

#### Scenario: CLI-PLC-01.02 -- Excluded selector rides the intent

- GIVEN the user runs `hsctl create gateway --name gw --placement-selector provider/ibm --placement-exclude-selector runtime/kata`
- THEN the request body SHALL contain `placement: {selector: ["provider/ibm"], excludedSelector: ["runtime/kata"]}`

#### Scenario: CLI-PLC-01.03 -- Local-kind flag maps to the mode shape

- GIVEN the user runs `hsctl create gateway --name gw --local-kind`
- THEN the request body SHALL contain `placement: {mode: "local-kind"}`

#### Scenario: CLI-PLC-01.04 -- Placement flags and cluster pin are mutually exclusive

- GIVEN the user runs `hsctl create gateway --name gw --cluster-id <id> --placement-selector network/public`
- THEN the command SHALL fail before any request is sent
- AND the error SHALL identify the flags as mutually exclusive

#### Scenario: CLI-PLC-01.05 -- Comma-separated value is rejected

- GIVEN the user runs `hsctl create gateway --name gw --placement-selector network/public,provider/ibm`
- THEN the command SHALL fail before any request is sent
- AND the error SHALL state that the flag takes one capability label per use and may be repeated

#### Scenario: CLI-PLC-01.06 -- Exclusion without a selector is rejected

- GIVEN the user runs `hsctl create gateway --name gw --placement-exclude-selector runtime/kata`
- THEN the command SHALL fail before any request is sent
- AND the error SHALL state that an excluded selector requires `--placement-selector`

#### Scenario: CLI-PLC-01.07 -- Unknown namespace is rejected client-side

- GIVEN the user runs `hsctl create gateway --name gw --placement-selector cloud/aws`
- THEN the command SHALL fail before any request is sent
- AND the error SHALL name the recognized capability namespaces

#### Scenario: CLI-PLC-01.08 -- Placement composes with the dedicated-route flag

- GIVEN the user runs `hsctl create gateway --name gw --local-kind --dedicated-route`
- THEN the request body SHALL contain both `placement: {mode: "local-kind"}` and `dedicated_route: true`

#### Scenario: CLI-PLC-01.09 -- Body file bypasses flag building

- GIVEN the user runs `hsctl create gateway --body request.json` with any placement flags also present on the command line
- THEN the request body SHALL be the file's contents, unchanged, as before this spec

### Requirement: CLI-PLC-02 -- Create Output and Late Binding

A successful create SHALL print the created Gateway as today. When the
placement intent matches no eligible managed cluster, the create SHALL
succeed and the output SHALL show the empty `cluster_id`, `phase` `Pending`,
and the awaiting-placement `status` exactly as the API returns them; the
command's help SHALL explain that an empty `cluster_id` means the gateway is
waiting for a matching cluster, not that the create failed. `hsctl get
gateway` and `hsctl list gateways` need no change: the resolved `cluster_id`
and the awaiting-placement status are already part of the read model.

#### Scenario: CLI-PLC-02.01 -- Awaiting placement is visible, not an error

- GIVEN a placement selector that no connected managed cluster matches
- WHEN the user runs `hsctl create gateway --name gw --placement-selector region/moon-base`
- THEN the command SHALL exit 0
- AND the printed Gateway SHALL have an empty `cluster_id` and `phase` `Pending`
- AND the printed `status` SHALL state that no connected managed cluster matches the placement selector

### Requirement: CLI-PLC-03 -- Availability Pre-Check

`hsctl create gateway --dry-run` SHALL validate the placement flags with the
same rules as a real create, then query `POST
/api/hypershell/v1/gateways/placement-availability` with the combined
required-label list as a single selector entry, and SHALL NOT create a
gateway. `--dry-run` SHALL be mutually exclusive with `--body`. For the
`--local-kind` shape the pre-check SHALL make no availability request (the
endpoint's contract is capability-selector based and the local-kind intent
declares none) and SHALL report that local-kind placement is resolved only
in the Kind development environment and validated server-side at create
time. For the `--cluster-id` shape the pre-check SHALL print that the
gateway would be pinned to the given cluster and that the server validates
the reference at create time, and SHALL make no availability request. The
pre-check SHALL exit 0 when no eligible cluster matches, printing that the
create would succeed and hold the gateway in `Pending` until a matching
cluster connects. When excluded selectors are supplied, the output SHALL note
that the availability endpoint evaluates required labels only and that
excluded labels are enforced at create time.

#### Scenario: CLI-PLC-03.01 -- Available selector

- GIVEN at least one connected managed cluster matches `network/public`
- WHEN the user runs `hsctl create gateway --name gw --dry-run --placement-selector network/public`
- THEN the output SHALL state that placement is available
- AND no gateway SHALL be created

#### Scenario: CLI-PLC-03.02 -- Unavailable selector is not an error

- GIVEN no connected managed cluster matches `region/moon-base`
- WHEN the user runs `hsctl create gateway --name gw --dry-run --placement-selector region/moon-base`
- THEN the command SHALL exit 0
- AND the output SHALL state that no connected managed cluster currently matches
- AND the output SHALL state that the create would succeed and late-bind when a matching cluster connects

#### Scenario: CLI-PLC-03.03 -- Exclusion note

- GIVEN the user runs `hsctl create gateway --name gw --dry-run --placement-selector provider/ibm --placement-exclude-selector runtime/kata`
- THEN the output SHALL note that the availability check evaluates required labels only

#### Scenario: CLI-PLC-03.04 -- Local-kind pre-check

- GIVEN the user runs `hsctl create gateway --name gw --dry-run --local-kind`
- THEN the output SHALL state that local-kind placement resolves only in the Kind development environment and is validated at create time
- AND no availability request SHALL be sent
- AND no gateway SHALL be created

#### Scenario: CLI-PLC-03.05 -- Dry-run with a body file is rejected

- GIVEN the user runs `hsctl create gateway --dry-run --body request.json`
- THEN the command SHALL fail before any request is sent

### Requirement: CLI-PLC-04 -- Help and Discovery

The `hsctl create gateway` help SHALL document both creation shapes (pinned
and late-bound), the repeat syntax for the selector flags, a region example
(`--placement-selector region/<region>`), the `--dry-run` pre-check, and the
awaiting-placement meaning of an empty `cluster_id`. The API-to-CLI mapping
table in [`platform/data-model.spec.md`](../platform/data-model.spec.md)
SHALL describe these flags as the CLI surface of the gateway create request.

#### Scenario: CLI-PLC-04.01 -- Help shows both shapes and the repeat syntax

- WHEN the user runs `hsctl create gateway --help`
- THEN the output SHALL show a pinned example using `--cluster-id`
- AND a late-bound example repeating `--placement-selector`
- AND a `--dry-run` example

## Non-Goals

- Set-based or Kubernetes-style selector expressions (`in`, `!label`); the
  platform's match-all literal semantics are unchanged (WEB-PLC-07).
- Placement preferences, weights, or capacity-aware selection; rejected by
  the placement spec's Non-Goals.
- Selecting a concrete managed cluster by name or identity capability on the
  user path; deferred to the `managed-cluster/<name>` identity capability
  work and the `cluster_id` authorization change.
- Any API server, control-plane, schema, proto, or SDK change.
- Exposing the persisted `placement` document on the Gateway read model.
- Changes to `hsctl apply` or the declarative `.hypershell` manifest path,
  which already carries placement.
