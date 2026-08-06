# HyperShell Gateway UI

Private workspace package containing HyperShell's canonical OpenShell gateway
management experience. The standalone web console is its first host.

## Public contract

`GatewayUiProvider` receives a host implementation of the purpose-shaped
`GatewayOperations` entry port and a typed navigation adapter. The host maps
the package-owned `GatewayControlPlane` driven port to its transport or SDK,
then constructs `GatewayOperations` with the package use-case factory, a
workflow runtime, and a typed domain-probe publisher. The host also supplies the
React Intl and TanStack Query providers, so an embedded page shares
localization, cache, and request policy with the surrounding product.

The package exports the gateway collection, detail, and provisioning pages;
gateway query helpers used by host chrome; and the message descriptors needed
for gateway-aware navigation. It also exports stable gateway application
values, ports, use cases, and the versioned gateway-probe catalog. It does not
export or own an application shell, route tree, authentication implementation,
BFF, generated SDK, or deployment configuration.

The package is private and source-consumed inside this pnpm workspace. Publishing
it requires the compiled artifact, compatibility, asset, lifecycle, and
versioning contracts in `specs/web-console/architecture.spec.md`.
