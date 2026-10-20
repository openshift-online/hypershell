# Specs

Desired state of the system. Code is the actual state. Development work reconciles the two.

## Sub-Specs

### [Platform](platform/)

Data model, API, control plane, gateway lifecycle, observability, and ADLC agent resources.

### [Web Console](web-console/)

Web-console architecture, technology choices, browser/server boundaries, and delivery requirements.

### [Security](security/)

RBAC enforcement, authentication, authorization.

### [Standards](standards/)

Cross-cutting engineering constraints by component.

UI standards cover accessible, usable, trustworthy, resilient, and verifiable web interfaces in `standards/ui/`.

SDLC standards cover the agent-based development funnel and its synchronization with Jira in `standards/sdlc/`.

## Requirement and Scenario IDs

Every requirement and scenario has a stable `MODULE-GROUP-NN` identifier
(`specs/standards/platform/spec-identifiers.spec.md`). The module/group
registry lives in [`ids.md`](ids.md).

## Spec Registry

Machine-readable index for autonomous reconciliation (`/reconcile` skill).

| Path | Domain | Primary Entities | Components | Depends On |
|------|--------|-----------------|------------|------------|
| `platform/data-model.spec.md` | platform | Gateway, ManagedCluster, AgentRuntime, SandboxTemplate, ProviderSpec, ProviderBinding, InferenceRoute, SecretSource, Repository | API, CP, CLI | managed-cluster-registration, openshell-gateway-service-accounts |
| `platform/control-plane.spec.md` | platform | Watcher, Reconciler, gRPC streams | CP | data-model |
| `platform/openshell-gateway.spec.md` | platform | Gateway, GatewayReconciler, provisioning | CP | data-model, control-plane |
| `platform/openshell-gateway-database.spec.md` | platform | Mounted admin credential Secret, per-gateway PostgreSQL provisioning, admin TLS verify-full, tenant TLS require, cleanup | CP, deploy | openshell-gateway, gateway-deletion-finalization |
| `platform/openshell-gateway-tls.spec.md` | platform | cert-manager, TLS certificates, SAN management | CP | openshell-gateway |
| `platform/openshell-gateway-routing.spec.md` | platform | GRPCRoute, BackendTLSPolicy, NetworkPolicy | CP | openshell-gateway, openshell-gateway-tls |
| `platform/openshell-gateway-oidc.spec.md` | platform | OIDC authentication, gateway.toml injection | CP | openshell-gateway, openshell-gateway-tls |
| `platform/openshell-gateway-credentials.spec.md` | platform | Credential storage drivers (Vault default, convention-addressed per-cluster Vault), serving-CA propagation, fail-closed reachability, KV purge on delete, KEK conditional provisioning | CP | openshell-gateway, openshell-gateway-database, gateway-provisioning-progress, openshell-gateway-health, gateway-deletion-finalization, security |
| `platform/openshell-gateway-secret-rotation.spec.md` | platform | Secret rotation: DB password, KEK, TLS certificates | CP | openshell-gateway-database, openshell-gateway-credentials, openshell-gateway-tls |
| `platform/openshell-gateway-keycloak.spec.md` | platform | Keycloak OIDC client provisioning, per-gateway OIDC role bridge | CP | openshell-gateway, openshell-gateway-oidc, rbac-enforcement |
| `platform/openshell-gateway-console.spec.md` | platform | Per-gateway OpenShell dashboard, oauth2-proxy, HTTP ingress | CP | openshell-gateway, openshell-gateway-routing, openshell-gateway-keycloak |
| `platform/openshell-gateway-service-accounts.spec.md` | platform | OpenShellGatewayServiceAccounts and Keycloak client-credentials lifecycle | API, CP, CLI, WEB, SDK | openshell-gateway-keycloak, openshell-gateway-oidc, rbac-enforcement, security, UI standards |
| `platform/hsctl-terminal-ui.spec.md` | platform | hsctl tui terminal interface generated from OpenAPI: entry point, pinned generator and runtime, session renewal | CLI | data-model, rbac-enforcement |
| `platform/hsctl-extension-commands.spec.md` | platform | hsctl get/list/create/delete for the Extension API kinds: generator boundary (v1 only), hand-written extgroup registration, Repository read-back and read-only status, no CLI/TUI drift | CLI | data-model, hsctl-terminal-ui |
| `platform/openshell-inference-routing.spec.md` | platform | Inference router, inference.local, credential-free sandbox model access, provider translation | CP | openshell-gateway, openshell-gateway-credentials |
| `platform/agent-workspace-gateway-api.spec.md` | platform | AgentWorkspace realized via OpenShell's EXISTING provisioning API (not a bespoke gateway gRPC contract): gateway admin session, idempotent ensure operations the openshell CLI drives, 422 admin-required and 409 name-collision enforcement, unconditional cascade deletion, provider realization coherent with late binding; supersedes the AgentWorkspaceProvisioning proto/client/endpoint gate | CP | data-model, control-plane, extension-api-openshell-api-mapping, agent-secret-late-binding, openshell-gateway-database, openshell-gateway-credentials, openshell-inference-routing |
| `platform/extension-api-openshell-api-mapping.spec.md` | platform | 1:1 map of HyperShell Extension kinds to OpenShell native kinds/API: per-kind field and lifecycle mapping, refresh-strategy grounding with upstream gaps, two-direction gap analysis, fully-declarative-over-OpenShell assertion | CP | agent-workspace-gateway-api, data-model, control-plane, agent-secret-late-binding, openshell-inference-routing |
| `platform/global-architecture.spec.md` | platform | Global hub, multi-cloud, cloud-managed PostgreSQL, Tekton, ArgoCD, Vault | CP, ALL | data-model, control-plane |
| `web-console/architecture.spec.md` | web-console | Web console, BFF, browser session, UI routes | WEB, SDK, API | data-model, security, openshell-gateway-service-accounts, UI standards |
| `web-console/gateway-provision-placement.spec.md` | web-console | Gateway placement intent, network/provider selection, random matching-cluster resolution | WEB, API, CP | architecture, managed-cluster-registration, openshell-gateway, UI standards |
| `web-console/operational-dashboard.spec.md` | web-console | Widgetized operational dashboard, Prometheus-backed metrics adapter, admin access | WEB | web-console/architecture, gateway-metrics-dashboard, gateway-fleet-total-trend, gateway-sandbox-active-trends, hub-cluster-utilization-trends, platform-inventory, registered-users, UI standards |
| `web-console/tracing.spec.md` | web-console | Browser OTel trace sink, BFF W3C propagation, telemetry ingest, dev Jaeger | WEB, BFF | web-console/architecture, domain-observability, local-development |
| `platform/gateway-metrics-dashboard.spec.md` | platform | Prometheus gateway phase metric, BFF metrics proxy, GatewayMetricsDashboard, operational dashboard gateway counts | API, WEB, deploy | data-model, web-console/architecture, local-development |
| `platform/platform-inventory.spec.md` | platform | Managed cluster inventory Prometheus collectors and operational dashboard widgets | API, WEB | web-console/operational-dashboard, managed-cluster-registration, rbac-enforcement |
| `platform/registered-users.spec.md` | platform | Registered user inventory API, Prometheus count, and operational dashboard widget | API, WEB | rbac-enforcement, web-console/operational-dashboard |
| `platform/gateway-managed-cluster-attribution.spec.md` | platform | managed_cluster label on gateway/active-sandbox gauges, per-spoke attribution, BFF grouping by managed_cluster | API, WEB | gateway-metrics-dashboard, managed-cluster-registration, platform-inventory |
| `platform/gateway-fleet-total-trend.spec.md` | platform | 7-day fleet gateway total usage trend for operational dashboard | WEB, deploy | gateway-metrics-dashboard, web-console/operational-dashboard |
| `platform/gateway-sandbox-active-trends.spec.md` | platform | Hourly and daily active sandbox usage trends for operational dashboard | WEB, deploy | gateway-metrics-dashboard, web-console/operational-dashboard |
| `platform/hub-cluster-utilization-trends.spec.md` | platform | 7-day hub cluster memory, CPU, and pod usage trends for operational dashboard | WEB, deploy | cluster-memory, cluster-cpu, cluster-pods, gateway-fleet-total-trend, web-console/operational-dashboard |
| `platform/cluster-memory.spec.md` | platform | Hub cluster memory utilization for operational dashboard | WEB, deploy | web-console/operational-dashboard, hub-cluster-utilization-trends, gateway-metrics-dashboard, local-development |
| `platform/cluster-cpu.spec.md` | platform | Hub cluster CPU utilization for operational dashboard | WEB, deploy | web-console/operational-dashboard, hub-cluster-utilization-trends, cluster-memory, gateway-metrics-dashboard, local-development |
| `platform/cluster-pods.spec.md` | platform | Hub cluster pod utilization for operational dashboard | WEB, deploy | web-console/operational-dashboard, hub-cluster-utilization-trends, cluster-memory, cluster-cpu, gateway-metrics-dashboard, local-development |
| `platform/cluster-nodes.spec.md` | platform | Hub cluster node inventory for operational dashboard | WEB, deploy | web-console/operational-dashboard, cluster-pods, gateway-metrics-dashboard, local-development |
| `platform/gateway-provision-time.spec.md` | platform | Gateway provision duration (mean, P50, P95) from control-plane histogram for operational dashboard | WEB, deploy, CP | web-console/operational-dashboard, control-plane-observability, cluster-memory |
| `standards/platform/spec-identifiers.spec.md` | standards | Requirement/scenario ID syntax, `ids.md` registry, stability rules, verification method | ALL | - |
| `standards/platform/cross-cutting.spec.md` | standards | - | ALL | - |
| `standards/platform/naming-multitenancy.spec.md` | standards | - | ALL | cross-cutting, global-architecture |
| `standards/control-plane/conventions.spec.md` | standards | - | CP | - |
| `standards/sdlc/jira-github-sync.spec.md` | standards | Work item, GitHub execution record, canonical link, phase-to-status projection, per-field authority, ingest (HYPERSHELL-374) | SDLC | - |
| `platform/managed-cluster-registration.spec.md` | platform | ManagedCluster self-registration for every control plane, name-collision 409, gateway cluster_id validation, watch-stream caller binding, last_seen_at heartbeat loop | API, CP | data-model, rbac-enforcement, control-plane, hub-grpc-tls |
| `platform/hub-grpc-tls.spec.md` | platform | Hub gRPC TLS-only listener, passthrough Route, DNS, authenticated watch streams before exposure (HYPERSHELL-333) | API, CP, GITOPS | control-plane, managed-cluster-registration, oidc-integration |
| `security/rbac-enforcement.spec.md` | security | User, Role, RoleBinding, RBAC middleware | API | data-model |
| `standards/security/security.spec.md` | standards | - | ALL | - |
| `platform/local-development.spec.md` | platform | Kind cluster, images, Make targets | ALL | cross-cutting, security |
| `platform/openshell-branch-build.spec.md` | platform | OpenShell branch builds, dev gateway provisioning | ALL | local-development, openshell-gateway |
| `platform/oidc-integration.spec.md` | platform | API JWT validation, BFF OIDC session, IdP client config, Kind opt-in | API, WEB, CP | local-development, openshell-gateway-oidc, web-console/architecture |
| `platform/e2e-testing.spec.md` | platform | Infra drivers, e2e test suite, CI workflow, deploy overlays | ALL | local-development, control-plane, openshell-gateway-routing |
| `platform/ephemeral-pr-environments.spec.md` | platform | Per-PR OpenShift namespaces, GitHub-brokered Keycloak, origin-only CI, timebox/reaper | CI, ALL | e2e-testing, openshift-development |
| `platform/ephemeral-test-credentials.spec.md` | platform | Test-tier Keycloak passwords, AWS SM, ESO, seed for env lifetime | CI, ALL | ephemeral-pr-environments, ephemeral-ci-secrets, e2e-testing, security |
| `platform/ephemeral-ci-secrets.spec.md` | platform | Standing GHA secrets in AWS SM, GitHub OIDC, ESO OAuth alignment | CI | ephemeral-pr-environments, security |
| `platform/openshell-image-auto-update.spec.md` | platform | Renovate customManager, OpenShell image bumps, merge policy | CI | e2e-testing, control-plane |
| `platform/api-server-observability.spec.md` | platform | API OTel SDK bootstrap, HTTP/gRPC server spans, W3C trace continuation, request metrics | API | web-console/tracing, security, local-development, e2e-testing |
| `platform/control-plane-observability.spec.md` | platform | CP OTel SDK bootstrap, reconcile spans, gRPC client spans, watch lifecycle, K8s API spans, reconcile metrics | CP | api-server-observability, control-plane, security, local-development |
| `platform/control-plane-reconciliation-metrics.spec.md` | platform | Reconciliation outcome taxonomy, duration/retry/terminal-failure metrics, bounded reason codes, BFF reconciliation query contract | CP, WEB | control-plane-observability, control-plane, security |
| `platform/reconcile-trace-correlation.spec.md` | platform | Trace context persistence, span links, reconcile-to-request correlation | API, CP | api-server-observability, control-plane-observability, data-model |
| `platform/agent-runtime-images.spec.md` | platform | Full and slim agent runtime images, pinned inputs, security and release bundle delivery | agents, CLI | hsctl-release |
| `platform/fleet-dashboard-sync-retry-visibility.spec.md` | platform | Argo Application sync-retry count (status.operationState.retryCount) threaded to the gate details pane as nullable syncRetries with plain-English tooltip | WEB | gateway-managed-cluster-attribution |
| `platform/fleet-dashboard-map-hub-spine-layout.spec.md` | platform | Promotion-topology map geometry: every stage HUB on one centered horizontal spine, change diamond + analysis gates + terminal gate riding that line, HUB-first ordering so the hub always takes the spine, visible provider lanes (AWS/IBM/...) with the hub-hosting lane centered on the spine under the single-cloud-hub assumption (multi-cloud tension noted), role and provider together govern vertical placement, pure deterministic baked-in-free geometry | WEB | fleet-dashboard-sync-retry-visibility |
| `platform/agent-sdlc-visibility-dashboard.spec.md` | platform | Agent SDLC tab for the glass dashboard: aging WIP board, rework meter, escalation lane, release x hub-cluster matrix; new /api/sdlc plane behind SdlcStateProvider port (GitHub adapter, Jira drop-in seam), shared cached bundle truth, label-event history from the GitHub timeline | WEB | fleet-dashboard-sync-retry-visibility, jira-github-sync, gateway-managed-cluster-attribution, UI standards |
| `platform/e2e-run-log-visibility.spec.md` | platform | Opt-in structured NDJSON run reporting (hypershell.e2e.v1) consumed by the glass dashboard E2E run view: new /api/e2e/runs, /api/e2e/runs/{id}, /api/e2e/stream planes; Loki query_range history + tail live streaming behind an app port; FD_E2E_LOKI_* config; snapshot/tail bridge with dedup by runId/sequence; run-to-promotion correlation by env+hydrated revision; degraded-state distinctions | WEB, BFF | e2e-testing, fleet-dashboard-sync-retry-visibility, gateway-managed-cluster-attribution, domain-observability, UI standards |
| `platform/glass-url-addressable-navigation.spec.md` | platform | One typed route/query contract for the glass dashboard making every tab, bundle, drawer, selection, sort, filter, and execution view URL-addressable; framework-free route codec + History adapter; pushState vs replaceState policy; hydration before data; popstate without loops; collision-free run identity and pinned-vs-live gate targets; bounded subscriptions under URL-driven navigation; SPA deep-link/base-path delivery | WEB | agent-sdlc-visibility-dashboard, e2e-run-log-visibility, hexagonal-architecture, accessibility, interaction, web-console/architecture |
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
| `platform/gateway-phase-vocabulary.spec.md` | platform | Canonical gateway phase and health vocabulary (representation) | API, CP, WEB | data-model, openshell-gateway-health |
| `platform/openshell-gateway-health.spec.md` | platform | Gateway phase lifecycle, workload and route readiness gating, continuous health reconciliation | CP | openshell-gateway, gateway-phase-vocabulary, openshell-gateway-routing |
| `platform/gateway-provisioning-progress.spec.md` | platform | Provisioning progress model, sub-phase decomposition | CP, WEB | openshell-gateway-health, gateway-phase-vocabulary |
| `platform/gateway-reconcile-concurrency.spec.md` | platform | Per-gateway reconcile queue, bounded worker pool, concurrency cap | CP | control-plane, openshell-gateway |
| `platform/gateway-deletion-finalization.spec.md` | platform | Gateway delete finalization, cross-cutting cleanup contract, finalizer ordering | API, CP | data-model, openshell-gateway, openshell-gateway-database |
| `platform/openshell-gateway-namespace-gc.spec.md` | platform | Orphaned-namespace garbage collection, cluster-filtered reclamation | CP | openshell-gateway, gateway-deletion-finalization |
| `platform/openshell-gateway-sandbox-count.spec.md` | platform | Active sandbox count derivation and gauge export | CP, WEB | openshell-gateway, gateway-metrics-dashboard |
| `platform/gateway-sandbox-attention-status.spec.md` | platform | Orphaned/expiring/idle sandbox attention gauges, BFF attention fields, widget | CP, WEB | gateway-metrics-dashboard, openshell-gateway-sandbox-count, web-console/operational-dashboard |
| `platform/openshell-gateway-helm-adoption.spec.md` | platform | Runtime Helm-chart install of gateways via Helm CLI, upstream-chart adoption | CP | openshell-gateway, control-plane |
| `platform/generated-gateway-config-validation.spec.md` | platform | Validation gate on the rendered gateway configuration artifact | CP | openshell-gateway, openshell-gateway-helm-adoption |
| `platform/watch-delete-events.spec.md` | platform | Full resource snapshot in delete watch events for reconciler cleanup | API, CP | data-model, control-plane |
| `platform/gateway-access-management.spec.md` | platform | Gateway-scoped access facade, RoleBinding-backed grants, Keycloak client-role projection, last-owner protection | API, CP | data-model, rbac-enforcement, openshell-gateway-keycloak, openshell-gateway-service-accounts |
| `platform/gateway-provision-outcomes.spec.md` | platform | Gateway provision success/failure outcomes counter, success-rate row, provision-reliability widget | CP, WEB, deploy | web-console/operational-dashboard, control-plane-observability, gateway-provision-time |
| `platform/gateway-release-distribution.spec.md` | platform | Fleet-wide gateway release adoption (gateways grouped by observed gateway_version) and the gateway-releases dashboard widget | WEB | web-console/operational-dashboard, gateway-metrics-dashboard |
| `platform/api-reliability-metrics.spec.md` | platform | API request reliability (success rate, latency histogram), hourly 24h trend, BFF reliability route | API, WEB, deploy | web-console/reliability-dashboard, control-plane-observability, web-console/operational-dashboard |
| `platform/hsctl-release.spec.md` | platform | hsctl and HyperShell bundle releases, build-once distribution, checksum verification | CLI, CI | data-model, hsctl-terminal-ui |
| `platform/openshell-install-docs-link.spec.md` | platform | Gateway connection tab link to OpenShell install documentation | WEB | web-console/architecture, openshell-gateway-console |
| `platform/openshift-development.spec.md` | platform | OpenShift dev and test driver model, make openshift-up, Go e2e driver | ALL, CI | local-development, e2e-testing, control-plane |
| `platform/e2e-console-browser-testing.spec.md` | platform | Playwright console e2e, driver selection, E2E_CONSOLE_URL discovery | WEB, CI | e2e-testing, web-console/architecture, local-development |
| `web-console/gateway-access-management.spec.md` | web-console | Gateway access-management console surface, Access tab over the access API | WEB, SDK | web-console/architecture, gateway-access-management, UI standards |
| `web-console/gateway-version-visibility.spec.md` | web-console | Running gateway version display, neutral version presentation | WEB | web-console/architecture, gateway-phase-vocabulary, UI standards |
| `web-console/reliability-dashboard.spec.md` | web-console | Reliability dashboard, same-origin BFF Prometheus routes, 24h hourly reconciliation trends | WEB | web-console/architecture, api-reliability-metrics, control-plane-reconciliation-metrics, UI standards |
| `web-console/provisioning-dashboard.spec.md` | web-console | Platform-admin provisioning dashboard: hub-cluster capacity and platform-wide gateway provisioning health, 7/30-day trend selection | WEB | web-console/architecture, web-console/operational-dashboard, hub-cluster-utilization-trends, gateway-provision-outcomes, UI standards |
