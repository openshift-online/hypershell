# Gateway Image Pull Access Specification

## Purpose

Hypershell grants a gateway's sandbox service account access to operator-approved
images. New gateways receive this access without a client Kubernetes credential or
manual namespace grants.

## Requirements

### Requirement: Explicit image Roles

The control plane SHALL accept `GATEWAY_SANDBOX_IMAGE_PULL_ROLES` as a JSON array
of objects with `namespace` and `role` fields. It SHALL reject unknown fields,
duplicate selections, invalid names, and more than 32 selections. An empty or
unset value SHALL disable grants and remove bindings this gateway previously owned.

Each referenced Role MUST exist and grant only `get` on named
`image.openshift.io/imagestreams/layers` resources. Hypershell SHALL NOT create
or modify these operator-owned Roles. The operator SHALL authorize the Hypershell
controller to bind only the selected Roles in their namespaces.

### Requirement: Exact sandbox identity

Hypershell SHALL read the sandbox service account from the gateway's Kubernetes
driver configuration and confirm that the account exists. Grants SHALL require
shared workspace mode; the empty mode uses the pinned gateway's shared default.
Managed and operator workspace modes SHALL fail closed. No grant SHALL name all
service accounts or another gateway namespace.

Each generated RoleBinding SHALL have a gateway Namespace owner reference and
durable gateway, namespace UID, and controller identity markers. Hypershell SHALL
repair its own bindings and SHALL NOT adopt a binding owned by another resource.

### Requirement: Access lifecycle and health

Provisioning and health reconciliation SHALL reconcile image pull bindings.
Failures SHALL prevent a Healthy gateway status. A Role that gains permissions
beyond named image pulls SHALL cause grant removal and a failed reconciliation.
Removed configuration and gateway deletion SHALL remove owned bindings. Deletion
SHALL remain incomplete while a binding remains or a cleanup request fails.

#### Scenario: A new client workspace

- GIVEN an operator has selected a named runner-image Role
- WHEN Hypershell creates a gateway for a new client workspace
- THEN its sandbox account receives the image pull RoleBinding
- AND the client does not receive Kubernetes access or a copied registry credential

#### Scenario: Binding drift or a failed API request

- GIVEN a gateway owns an image pull RoleBinding
- WHEN its subject changes or a Kubernetes update fails
- THEN reconciliation repairs the subject or reports the failure
- AND the gateway is not Healthy until access is reconciled

#### Scenario: Gateway deletion

- GIVEN two gateways have separate image pull bindings
- WHEN one gateway is deleted
- THEN only that gateway's bindings are removed
- AND cleanup errors or pending finalizers delay deletion completion
