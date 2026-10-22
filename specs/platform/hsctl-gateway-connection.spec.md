# hsctl Gateway Connection Instructions Specification

**ID prefix:** CLI-GWC

**Related:** [`web-console/architecture.spec.md`](../web-console/architecture.spec.md)
WEB-UIEX-05 (the equivalent web console Connection tab ordered steps)

## Purpose

`hsctl get gateway --show-connection` prints the OpenShell commands an operator
runs to connect to a Gateway and start using it: logging in, configuring an
inference provider, and creating a sandbox. A provider type's profile must be
imported into the gateway's profile catalog before `openshell provider create`
can use that provider type. Printing the provider-create command without first
printing a profile-import step leads the operator straight into a "profile not
found" error with no guidance on how to resolve it.

## Requirements

### Requirement: CLI-GWC-01 -- Provider Profile Import Step

`hsctl get gateway --show-connection` SHALL print a numbered step that imports
the provider profile before the numbered step that creates the provider. The
profile-import step SHALL show a command that downloads the profile file for
the connection script's provider type, followed by the
`openshell provider profile import -f <profile>.yaml` command that imports it.
A documentation link alone is not sufficient: the operator SHALL be able to
run the printed instructions verbatim without separately locating the file.

#### Scenario: CLI-GWC-01.01 -- Profile import precedes provider create

- GIVEN a ready Gateway
- WHEN the user runs `hsctl get gateway --show-connection`
- THEN the output SHALL contain a numbered step that runs `openshell provider profile import -f google-vertex-ai.yaml --global`
- AND that step's number SHALL be lower than the numbered step that runs `openshell provider create`

#### Scenario: CLI-GWC-01.02 -- Profile download command precedes profile import

- GIVEN a ready Gateway
- WHEN the user runs `hsctl get gateway --show-connection`
- THEN the output SHALL contain a `curl` command that downloads the provider profile file
- AND that command SHALL appear before the `openshell provider profile import` command

### Requirement: CLI-GWC-02 -- Profile and Provider Scope Must Match

The profile-import step and the provider-create step SHALL use the same
profile scope. The printed instructions SHALL carry a comment line, adjacent
to the two commands, stating that the import scope (`--global` or workspace)
must match the scope used by `provider create` (or its `--global-profile`
flag), because a scope mismatch reproduces the "profile not found" error even
when the profile has already been imported.

#### Scenario: CLI-GWC-02.01 -- Scope note accompanies the two commands

- GIVEN the printed connection instructions
- WHEN the user reads the profile-import and provider-create steps
- THEN a comment line SHALL state that the import scope and the provider-create scope must match
