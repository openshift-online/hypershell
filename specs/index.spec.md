# Specs

Desired state of the system. Code is the actual state. Development work reconciles the two.

## Sub-Specs

### [Platform](platform/)

Data model, API, control plane, gateway lifecycle, fleet management.

### [Web Console](web-console/)

Web-console architecture, technology choices, browser/server boundaries, and delivery requirements.

### [Security](security/)

RBAC enforcement, authentication, authorization.

### [Standards](standards/)

Cross-cutting engineering constraints by component.

UI standards cover accessible, usable, trustworthy, resilient, and verifiable web interfaces in `standards/ui/`.

## Spec Registry

Machine-readable index for autonomous reconciliation (`/reconcile` skill).

| Path | Domain | Primary Entities | Components | Depends On |
|------|--------|-----------------|------------|------------|
| `platform/data-model.spec.md` | platform | Fleet, Gateway, GatewayNetwork, GatewayRelease, ManagedCluster, ManagedDatabase | API, CP | - |
| `platform/control-plane.spec.md` | platform | Watcher, Reconciler, gRPC streams | CP | data-model |
| `platform/openshell-gateway.spec.md` | platform | Gateway, GatewayReconciler, provisioning | CP | data-model, control-plane |
| `platform/openshell-gateway-database.spec.md` | platform | PostgreSQL provisioning, credential security | CP | openshell-gateway |
| `platform/openshell-gateway-tls.spec.md` | platform | cert-manager, TLS certificates, SAN management | CP | openshell-gateway |
| `platform/openshell-gateway-routing.spec.md` | platform | GRPCRoute, BackendTLSPolicy, NetworkPolicy | CP | openshell-gateway, openshell-gateway-tls |
| `platform/openshell-gateway-oidc.spec.md` | platform | OIDC authentication, gateway.toml injection | CP | openshell-gateway, openshell-gateway-tls |
| `platform/openshell-gateway-credentials.spec.md` | platform | Credential storage drivers, KEK conditional provisioning | CP | openshell-gateway, openshell-gateway-database |
| `platform/openshell-gateway-secret-rotation.spec.md` | platform | Secret rotation: DB password, KEK, TLS certificates | CP | openshell-gateway-database, openshell-gateway-credentials, openshell-gateway-tls |
| `platform/openshell-gateway-keycloak.spec.md` | platform | Keycloak OIDC client provisioning, per-gateway OIDC role bridge | CP | openshell-gateway, openshell-gateway-oidc, rbac-enforcement |
| `platform/openshell-inference-routing.spec.md` | platform | Inference router, inference.local, credential-free sandbox model access, provider translation | CP | openshell-gateway, openshell-gateway-credentials |
| `platform/global-architecture.spec.md` | platform | Global hub, multi-cloud, CNPG, Tekton, ArgoCD, Vault | CP, ALL | data-model, control-plane |
| `web-console/architecture.spec.md` | web-console | Web console, BFF, browser session, UI routes | WEB, SDK, API | data-model, security, UI standards |
| `web-console/tracing.spec.md` | web-console | Browser OTel trace sink, BFF W3C propagation, telemetry ingest, dev Jaeger | WEB, BFF | web-console/architecture, domain-observability, local-development |
| `standards/platform/cross-cutting.spec.md` | standards | - | ALL | - |
| `standards/control-plane/conventions.spec.md` | standards | - | CP | - |
| `security/rbac-enforcement.spec.md` | security | User, Role, RoleBinding, RBAC middleware | API | data-model |
| `standards/security/security.spec.md` | standards | - | ALL | - |
| `platform/local-development.spec.md` | platform | Kind cluster, images, Make targets | ALL | cross-cutting, security |
| `platform/oidc-integration.spec.md` | platform | API JWT validation, BFF OIDC session, IdP client config, Kind opt-in | API, WEB, CP | local-development, openshell-gateway-oidc, web-console/architecture |
| `platform/e2e-testing.spec.md` | platform | Infra drivers, e2e test suite, CI workflow, deploy overlays | ALL | local-development, control-plane, openshell-gateway-routing |
| `platform/api-server-observability.spec.md` | platform | API OTel SDK bootstrap, HTTP/gRPC server spans, W3C trace continuation, request metrics | API | web-console/tracing, security, local-development, e2e-testing |
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
