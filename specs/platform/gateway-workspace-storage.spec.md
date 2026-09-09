# Gateway Workspace Storage Specification

## Purpose

Operators select storage defaults for new sandbox workspace volumes through the
Hypershell control plane. The gateway owns these volumes and applies the defaults.

## Requirements

### Requirement: Operator workspace storage defaults

The control plane SHALL accept `GATEWAY_WORKSPACE_STORAGE_CLASS` and
`GATEWAY_WORKSPACE_DEFAULT_STORAGE_SIZE`. It SHALL write configured values as
`workspace_storage_class` and `workspace_default_storage_size` in the gateway's
`[openshell.drivers.kubernetes]` configuration table. Empty values SHALL omit the
corresponding keys and preserve the selected gateway image's defaults.

The storage class SHALL be a valid Kubernetes DNS subdomain name. The size SHALL
be a positive Kubernetes storage quantity. Invalid values SHALL cause control-plane
startup and gateway configuration rendering to fail. These settings SHALL NOT
change existing PVCs or alter sandbox security permissions.

#### Scenario: Dedicated sandbox storage class

- GIVEN an operator configures a storage class and a default size of `5Gi`
- WHEN Hypershell reconciles a gateway
- THEN the Kubernetes driver table contains both configured values
- AND OIDC and credential driver configuration remain in their own tables

#### Scenario: Existing image defaults

- GIVEN both settings are empty
- WHEN Hypershell reconciles a gateway
- THEN the workspace storage keys are omitted

#### Scenario: Invalid storage settings

- GIVEN a class name is invalid or a size is zero, negative, or malformed
- WHEN the control plane starts
- THEN startup fails with the name of the invalid setting
