# Specs

Desired state of the system. Code is the actual state. Development work reconciles the two.

## Sub-Specs

### [Platform](platform/)

Data model, API, control plane, gateway lifecycle, fleet management.

### [Standards](standards/)

Cross-cutting engineering constraints by component.

UI standards cover accessible, usable, trustworthy, resilient, and verifiable web interfaces in `standards/ui/`.

## Spec Registry

Machine-readable index for autonomous reconciliation (`/reconcile` skill).

| Path | Domain | Primary Entities | Components | Depends On |
|------|--------|-----------------|------------|------------|
| `platform/data-model.spec.md` | platform | Fleet, Gateway, GatewayNetwork, GatewayRelease, ManagedCluster, ManagedDatabase | API, CP | - |
| `platform/control-plane.spec.md` | platform | Watcher, Reconciler, gRPC streams | CP | data-model |
| `standards/platform/cross-cutting.spec.md` | standards | - | ALL | - |
| `standards/control-plane/conventions.spec.md` | standards | - | CP | - |
| `standards/security/security.spec.md` | standards | - | ALL | - |
| `standards/ui/foundations.spec.md` | standards | UI foundations | WEB | - |
| `standards/ui/brand-color.spec.md` | standards | Red Hat brand color | WEB | foundations, accessibility |
| `standards/ui/interaction.spec.md` | standards | UI interaction | WEB | foundations |
| `standards/ui/accessibility.spec.md` | standards | UI accessibility | WEB | foundations, interaction |
| `standards/ui/content-localization.spec.md` | standards | UI content, localization | WEB | foundations |
| `standards/ui/trust-performance.spec.md` | standards | UI trust, performance, resilience | WEB | foundations, interaction |
| `standards/ui/verification.spec.md` | standards | UI verification | WEB | foundations, brand-color, interaction, accessibility, content-localization, trust-performance |
