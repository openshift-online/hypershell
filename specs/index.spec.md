# Specs

Desired state of the system. Code is the actual state. Development work reconciles the two.

## Sub-Specs

### [Platform](platform/)

Data model, API, control plane, gateway lifecycle, fleet management.

### [Web Console](web-console/)

Web-console architecture, technology choices, browser/server boundaries, and delivery requirements.

### [Standards](standards/)

Cross-cutting engineering constraints by component.

UI standards cover accessible, usable, trustworthy, resilient, and verifiable web interfaces in `standards/ui/`.

## Spec Registry

Machine-readable index for autonomous reconciliation (`/reconcile` skill).

| Path | Domain | Primary Entities | Components | Depends On |
|------|--------|-----------------|------------|------------|
| `platform/data-model.spec.md` | platform | Fleet, Gateway, GatewayNetwork, GatewayRelease, ManagedCluster, ManagedDatabase | API, CP | - |
| `platform/control-plane.spec.md` | platform | Watcher, Reconciler, gRPC streams | CP | data-model |
| `web-console/architecture.spec.md` | web-console | Web console, BFF, browser session, UI routes | WEB, SDK, API | data-model, security, UI standards |
| `standards/platform/cross-cutting.spec.md` | standards | - | ALL | - |
| `standards/control-plane/conventions.spec.md` | standards | - | CP | - |
| `standards/security/security.spec.md` | standards | - | ALL | - |
| `standards/ui/foundations.spec.md` | standards | UI foundations | WEB | - |
| `standards/ui/brand-color.spec.md` | standards | Red Hat brand color | WEB | foundations, accessibility |
| `standards/ui/interaction.spec.md` | standards | UI interaction | WEB | foundations |
| `standards/ui/patternfly.spec.md` | standards | PatternFly 6, reusable components | WEB | foundations, brand-color, accessibility |
| `standards/ui/accessibility.spec.md` | standards | UI accessibility | WEB | foundations, interaction |
| `standards/ui/content-localization.spec.md` | standards | UI content, localization | WEB | foundations |
| `standards/ui/trust-performance.spec.md` | standards | UI trust, performance, resilience | WEB | foundations, interaction |
| `standards/ui/hexagonal-architecture.spec.md` | standards | UI application ports, adapters, composition | WEB, BFF, SDK | foundations |
| `standards/ui/domain-observability.spec.md` | standards | Domain probes, fan-out telemetry | WEB, BFF | hexagonal-architecture, trust-performance, security |
| `standards/ui/verification.spec.md` | standards | UI verification | WEB | foundations, brand-color, interaction, patternfly, accessibility, content-localization, trust-performance, hexagonal-architecture, domain-observability |
