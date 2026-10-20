# hsctl Extension Resource Commands Specification

**Related:** [`data-model.spec.md`](data-model.spec.md) (the Extension API kinds and the `hsctl` CLI reference), [`hsctl-terminal-ui.spec.md`](hsctl-terminal-ui.spec.md) (the generated `hsctl tui`, which scopes out the resource commands codified here)

## Purpose

The `hsctl` CLI exposes the HyperShell API as scripting commands: `get`, `list`,
`create`, `update`, `delete`, and `apply`. The core `/api/hypershell/v1` kinds get
these commands from the rh-trex-ai CLI generator. The Extension API kinds
(`AgentRuntime`, `SandboxTemplate`, `ProviderSpec`, `ProviderBinding`,
`InferenceRoute`, `SecretSource`, `Repository`), served under
`/api/hypershell/ext/`, do not come from that generator: the generator targets a
single API prefix and emits only the `v1` kinds, so the extension resource
commands are hand-maintained. This specification defines which extension commands
exist, how they are registered, and how the command set is kept free of drift
against the API and the generated terminal UI descriptor.

## Scope Boundary

In scope: the `hsctl get`, `hsctl list`, `hsctl create`, `hsctl update`, and
`hsctl delete` commands for the Extension API kinds, how they are registered, the
extension URL paths they call, and the read-only fields a `get` renders.

Not in scope: the behavior and generation of `hsctl tui` (see
[`hsctl-terminal-ui.spec.md`](hsctl-terminal-ui.spec.md)), the semantics of the
Extension API endpoints themselves (see [`data-model.spec.md`](data-model.spec.md)),
and the `hsctl apply` reconcile loop, which already handles every Extension kind
including `Repository`.

## Requirements

### Requirement: CLI-EXT-01 -- HEC-01 - Generator Boundary and Registration

The rh-trex-ai CLI generator SHALL generate resource commands only for the
`/api/hypershell/v1` kinds, because it handles a single API prefix. The Extension
API kinds, served under `/api/hypershell/ext/`, SHALL have hand-written per-kind
command packages registered on the generated `get`, `list`, `create`, and
`delete` parent commands through the shared extension registrar (package
`extgroup`), which places them in the parent command's extension help section.
Their endpoint paths SHALL be hand-maintained alongside the generated core paths.

The extension resource commands SHALL NOT be hard-coded into the generated parent
command files; the generated files SHALL remain editable only by regeneration.

#### Scenario: CLI-EXT-01.01 -- A new extension kind gains resource commands

- GIVEN a new Extension API kind served under `/api/hypershell/ext/`
- WHEN its `get` and `list` command packages are added and registered through the extension registrar
- THEN `hsctl get <kind>` and `hsctl list <kinds>` SHALL appear in the extension help section of their parent commands
- AND no generated parent command file SHALL be hand-edited to make them appear

### Requirement: CLI-EXT-02 -- HEC-02 - Read-Back for Every Listable Extension Kind

Every Extension API kind the API server serves with both a list endpoint
(`GET /api/hypershell/ext/<kinds>`) and a get-by-id endpoint
(`GET /api/hypershell/ext/<kinds>/{id}`) SHALL have a corresponding
`hsctl list <kinds>` command and a `hsctl get <kind> <id>` command, consistent
with the other Extension kinds (`AgentRuntime`, `SandboxTemplate`, `ProviderSpec`,
`ProviderBinding`, `InferenceRoute`, `SecretSource`). This SHALL include
`Repository`: `hsctl list repositories` SHALL list registered Repositories and
`hsctl get repository <id>` SHALL fetch one Repository by id.

`hsctl list <kinds>` SHALL accept the standard list flags (search, order-by,
column, output-format) and SHALL page through the full result set the same way the
other Extension list commands do. Looking up a Repository by a human-readable
value SHALL be served through `hsctl list repositories --search`, consistent with
every other Extension list command; `hsctl get` resolves by id.

#### Scenario: CLI-EXT-02.01 -- List registered Repositories

- GIVEN one or more Repositories are registered
- WHEN the user runs `hsctl list repositories`
- THEN the command SHALL call `GET /api/hypershell/ext/repositories` and print the registered Repositories

#### Scenario: CLI-EXT-02.02 -- Read back an applied Repository

- GIVEN a Repository was registered with `hsctl apply`
- WHEN the user runs `hsctl get repository <id>` with that Repository's id
- THEN the command SHALL call `GET /api/hypershell/ext/repositories/{id}` and print that Repository

#### Scenario: CLI-EXT-02.03 -- Apply then verify loop closes

- GIVEN a Repository registered through `hsctl apply`
- WHEN the user lists repositories and then gets the repository by id
- THEN both commands SHALL succeed, so the apply-then-verify loop is complete from `hsctl` alone

### Requirement: CLI-EXT-03 -- HEC-03 - get Renders Read-Only Status

`hsctl get repository <id>` SHALL render the full Repository object returned by
the API, including the read-only `status` field (registration and import health).
No extension `get` command SHALL omit a read-only field the API returns.

#### Scenario: CLI-EXT-03.01 -- Inspect import health

- GIVEN a registered Repository whose API representation carries a `status` value
- WHEN the user runs `hsctl get repository <id>`
- THEN the printed object SHALL include the `status` field so the user can follow the Repository's registration and import health

### Requirement: CLI-EXT-04 -- HEC-04 - No CLI or TUI Drift

The committed CLI command set and the generated terminal UI descriptor SHALL stay
consistent with the API. When an Extension kind is added to the API, its generated
terminal UI descriptor entry SHALL be regenerated through `make generate-tui`, and
`make check` SHALL fail when the committed descriptor differs from a fresh
generation (see [`hsctl-terminal-ui.spec.md`](hsctl-terminal-ui.spec.md), TUI-02).
The hand-written extension resource commands SHALL be registered consistently
across `get`, `list`, `create`, and `delete` so that the Extension kinds a user
can create are also the Extension kinds a user can read back.

#### Scenario: CLI-EXT-04.01 -- Descriptor regenerated for a new kind

- GIVEN `Repository` is served by the Extension API
- WHEN a developer runs `make generate-tui` and commits the result
- THEN `make check` SHALL pass and `hsctl tui` SHALL list Repositories without any other source change

#### Scenario: CLI-EXT-04.02 -- Stale terminal UI descriptor

- GIVEN the OpenAPI description changed and the descriptor was not regenerated
- WHEN `make check` runs
- THEN it SHALL fail and name `make generate-tui`
