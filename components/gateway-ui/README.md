# HyperShell Gateway UI

Private workspace package containing HyperShell's canonical OpenShell gateway
management experience. The standalone web console is its first host.

## Public contract

`GatewayUiProvider` receives a host implementation of the purpose-shaped
`GatewayOperations` entry port and a typed navigation adapter. The host maps
that port to its transport or SDK and also supplies the React Intl and TanStack
Query providers, so an embedded page shares localization, cache, and request
policy with the surrounding product.

The package exports the gateway collection, detail, and provisioning pages;
gateway query helpers used by host chrome; and the message descriptors needed
for gateway-aware navigation. It does not export or own an application shell,
route tree, authentication implementation, BFF, generated SDK, or deployment
configuration.

The package is private and source-consumed inside this pnpm workspace. Publishing
it requires the compiled artifact, compatibility, asset, lifecycle, and
versioning contracts in `specs/web-console/architecture.spec.md`.
