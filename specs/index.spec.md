# Specs

Desired state of the system. Code is the actual state. Development work reconciles the two.

## Sub-Specs

### [Platform](platform/)

Data model, API, control plane, gateway lifecycle, fleet management.

### [Standards](standards/)

Cross-cutting engineering constraints by component.

## Spec Registry

Machine-readable index for autonomous reconciliation (`/reconcile` skill).

| Path | Domain | Primary Entities | Components | Depends On |
|------|--------|-----------------|------------|------------|
| `platform/data-model.spec.md` | platform | Fleet, Gateway, GatewayNetwork, GatewayRelease, ManagedCluster, ManagedDatabase | API, CP | - |
| `platform/control-plane.spec.md` | platform | Watcher, Reconciler, gRPC streams | CP | data-model |
| `standards/platform/cross-cutting.spec.md` | standards | - | ALL | - |
| `standards/control-plane/conventions.spec.md` | standards | - | CP | - |
| `standards/security/security.spec.md` | standards | - | ALL | - |
| `platform/local-development.spec.md` | platform | Kind cluster, images, Make targets | ALL | cross-cutting, security |
