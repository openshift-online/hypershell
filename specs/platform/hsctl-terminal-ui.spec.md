# hsctl Terminal UI Specification

**Date:** 2026-10-07
**Status:** Draft
**Related:** [`../security/rbac-enforcement.spec.md`](../security/rbac-enforcement.spec.md), [`data-model.spec.md`](data-model.spec.md)

## Purpose

`hsctl tui` is an interactive, full-screen terminal interface for the HyperShell
API server, built into the `hsctl` binary. It is not hand-written: the screens,
tables, forms, and actions are produced by the rh-trex-ai TUI generator from the
API server's OpenAPI description, and rendered by the shared rh-trex-ai terminal
runtime. HyperShell contributes only the command entry point, the embedded
descriptor, and the credential source. The behavior of the generated screens
(navigation, filtering, sorting, detail and raw views, action forms,
confirmation, error dialogs, polling) is defined by the rh-trex-ai
`specs/codegen/tui-generator.spec.md` at the commit pinned in
`scripts/rh-trex-ai.ref`; this specification does not restate it.

## Scope Boundary

In scope: the `hsctl tui` command, the generated descriptor,
how the pinned generator and runtime are kept consistent, and how the command
obtains and renews credentials.

Not in scope: the appearance and key bindings of generated screens, which belong
to the rh-trex-ai runtime, and the `hsctl` resource commands (`get`, `list`,
`create`, `update`, `delete`, `apply`), which remain the scripting interface.

## Requirements

### Requirement: TUI-01 - Command Entry Point

`hsctl` SHALL provide a `tui` subcommand that starts the terminal interface. The
command SHALL load the saved `hsctl` configuration and use the saved server URL
and `insecure` setting.

When there is no saved login, the command SHALL exit with a non-zero status and
the same "not logged in" error the other commands print, without entering
full-screen mode. When standard input or standard output is not a terminal, the
command SHALL exit with a non-zero status and an error stating that `hsctl tui`
requires an interactive terminal. These checks SHALL run before the descriptor is
read or the terminal is taken over.

The command SHALL accept the flags of the rh-trex-ai terminal command:
`--server` and `--insecure`, which override the saved login when given,
`--token-file`, `--trust-origin`, and `--refresh-interval` (the polling interval,
default 5 seconds, `0` disables polling).

#### Scenario: Start without a login

- GIVEN no `hsctl` configuration file exists
- WHEN the user runs `hsctl tui`
- THEN the command SHALL print `not logged in, server URL isn't set, run the 'login' command`
- AND it SHALL exit with status 1 without switching to the alternate screen

#### Scenario: Output redirected

- GIVEN the user is logged in
- WHEN the user runs `hsctl tui > out.txt`
- THEN the command SHALL exit with status 1 and an error mentioning an interactive terminal

### Requirement: TUI-02 - Generated From the API Description

The resources, columns, relationships, and actions shown by `hsctl tui` SHALL
come from the generated descriptor under `components/cli/data/generated/tui`,
produced by the rh-trex-ai TUI generator from
`components/api-server/openapi/openapi.yaml`. No resource kind, column, or
action SHALL be hard-coded in HyperShell source for this interface. An action
SHALL be offered only when the OpenAPI description declares the operation.

The generator SHALL be run through `make generate-tui`, from the rh-trex-ai
commit recorded in `scripts/rh-trex-ai.ref`. `make check` SHALL fail when the
committed descriptor differs from a fresh generation.

#### Scenario: A resource is added to the API

- GIVEN a new resource is added to the OpenAPI description
- WHEN a developer runs `make generate-tui` and commits the result
- THEN `hsctl tui` SHALL list the resource without any other source change

#### Scenario: Stale descriptor

- GIVEN the OpenAPI description changed and the descriptor was not regenerated
- WHEN `make check` runs
- THEN it SHALL fail and name `make generate-tui`

### Requirement: TUI-03 - Pinned Generator and Runtime

The descriptor generator and the terminal runtime linked into `hsctl` SHALL come
from the same rh-trex-ai commit, so that every descriptor field the generator
emits is understood by the runtime. The generator commit SHALL be the full SHA in
`scripts/rh-trex-ai.ref`; the runtime SHALL be the `rh-trex-ai` module version in
`components/cli/go.mod`.

#### Scenario: Bumping the pin

- GIVEN a new rh-trex-ai commit is to be adopted
- WHEN a developer updates `scripts/rh-trex-ai.ref` and the `go.mod` module version to that commit
- THEN `make generate-cli`, `make generate-tui`, and the `hsctl` unit tests SHALL pass before the change is merged

### Requirement: TUI-04 - Credentials and Session Renewal

Every API request made by the terminal interface SHALL carry the current access
token, obtained from the saved login through a token provider that renews an
expired token with the saved refresh token before the request is sent. A renewed
token SHALL be saved to the configuration file without erasing the refresh
token, issuer, or client ID. The token SHALL NOT be written to the terminal
output.

When the session cannot be renewed, the interface SHALL show the API error in its
error dialog and the user SHALL be told to run `hsctl login`.

#### Scenario: Long session

- GIVEN the user stays in `hsctl tui` longer than the access token lifetime
- WHEN the next request is made
- THEN the token SHALL be renewed and the request SHALL succeed without restarting the command

## CLI distribution

The terminal UI ships inside the `hsctl` executable. Platform builds, Quay image packaging, bundle membership, and GitHub release assets SHALL follow [hsctl and HyperShell bundle releases](hsctl-release.spec.md).
