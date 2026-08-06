# Skills Directory & Reconciliation Checkpoint

This file is the **entrypoint** for autonomous spec-to-code reconciliation.
It describes the skill directory, holds the current gap state, and is the
checkpoint that makes `/reconcile` idempotent across sessions.

**How it works**: The `/reconcile` skill reads this file first. If the gap
table below is populated, it skips Phases 1-4 (discovery, dependency graph,
gap analysis, merge) and jumps directly to Phase 5 (wave planning) or
Phase 6 (execution). After each wave or dry-run, the agent updates this
file with the new state.

**Idempotency contract**: Running `/reconcile` with no arguments always
produces the same result for the same spec+code state.

---

## Skill Directory

```
skills/
├── build/
│   ├── reconcile/            # Meta-orchestrator: reads this file, executes waves
│   ├── full-stack-pipeline/  # Single-spec wave-based implementation pipeline
│   └── dev-cluster/          # Kind cluster lifecycle for local testing
├── deploy/
│   ├── deploy-cluster/       # OpenShift deployment (internal registry, kustomize)
│   └── kind/                 # Kind local development (image loading, NodePort)
├── plan/
│   └── spec/                 # Spec authoring (desired state)
├── review/
│   ├── amber-review/         # General code and security review
│   ├── review-guidance/      # PR review checklists
│   └── ui-standards/         # UI audit and intent-driven recommendations
└── tooling/
    ├── align/                # Convention compliance scoring
    ├── jira-log/             # Jira work logging
    ├── maintain-ci/          # CI and component registration maintenance
    └── memory/               # Project memory management
```

**SDLC flow**: `/reconcile` → `/spec` → `/full-stack-pipeline` → `/deploy-cluster` or `/kind`

---

## Reconciliation State

**Last analyzed**: 2026-08-07
**Spec corpus**: 22 specs across 4 domains (platform, web-console, standards/platform, standards/ui)
**Codebase commit**: b83c635

### Coverage Summary

| Domain | Specs | Requirements | Present | Partial | Missing | Deferred | Coverage |
|--------|-------|-------------|---------|---------|---------|----------|----------|
| Platform — Data Model | 1 | 12 | 11 | 1 | 0 | 0 | 96% |
| Platform — Control Plane | 1 | 13 | 8 | 1 | 4 | 0 | 65% |
| Platform — Gateway (core) | 1 | 18 | 12 | 3 | 3 | 0 | 75% |
| Platform — Gateway DB | 1 | 9 | 5 | 0 | 4 | 0 | 56% |
| Platform — Gateway TLS | 1 | 7 | 3 | 2 | 2 | 0 | 57% |
| Platform — Gateway OIDC | 1 | 7 | 4 | 1 | 2 | 0 | 64% |
| Platform — Gateway Routing | 1 | 18 | 6 | 4 | 8 | 0 | 44% |
| Platform — Local Development | 1 | 24 | 3 | 5 | 16 | 0 | 23% |
| Web Console — Architecture | 1 | 28 | 18 | 8 | 2 | 0 | 79% |
| Standards | 13 | 0 | 0 | 0 | 0 | 0 | N/A |
| **TOTAL** | **22** | **136** | **70** | **25** | **41** | **0** | **61%** |

### Spec Dependency Order

```
Layer 0 (roots):  data-model, standards/*
Layer 1:          control-plane, local-development, web-console architecture
Layer 2:          openshell-gateway (core)
Layer 3:          openshell-gateway-database, openshell-gateway-tls
Layer 4:          openshell-gateway-oidc (depends on TLS for trusted CA)
Layer 5:          openshell-gateway-routing (depends on TLS for BackendTLSPolicy)
Layer 6:          local-development (depends on all platform specs)
Layer 7:          web-console/architecture (depends on data-model, security, UI standards)
```

---

## Gap Table

### data-model.spec.md

| # | Requirement | Status | Gap | Code Location | Wave |
|---|-------------|--------|-----|---------------|------|
| DM-1 | Fleet (Sector) Lifecycle CRUD | Present | Naming: spec says "Sector", code says "Fleet" | `plugins/fleets/` | — |
| DM-2 | Fleet-Scoped Resources (FK) | Present | `fleet_id` instead of `sector_id` | all child models | — |
| DM-3a | Gateway field: `image` | Present | Added to model, OpenAPI, proto, migration | `plugins/gateways/model.go` | W5 ✅ |
| DM-3b | Gateway field: `server_dns_names` | Present | Added as JSONB (model `*string`), proto `repeated string`, OpenAPI `[]string` | `plugins/gateways/model.go` | W5 ✅ |
| DM-3c | Gateway field: `oidc` (JSONB) | Present | Added to model, OpenAPI, proto, migration | `plugins/gateways/model.go` | W5 ✅ |
| DM-3d | Gateway field: `route` (JSONB) | Present | Added to model, OpenAPI, proto, migration | `plugins/gateways/model.go` | W5 ✅ |
| DM-3e | Gateway field: `route_address` (read-only) | Present | Added to model, OpenAPI (readOnly), proto, migration | `plugins/gateways/model.go` | W5 ✅ |
| DM-3f | Gateway field: `database` (JSONB) | Present | Added to model, OpenAPI, proto, migration; CP struct has `externalSecretRef` | `plugins/gateways/model.go` | W5 ✅ |
| DM-4 | Gateway phase + status fields | Partial | `phase` updated by CP; `status` field exists but never written | `plugins/gateways/model.go` | Future |
| DM-5 | Canary release strategy fields | Present | Fields exist; no logic implements canary | `plugins/gatewayReleases/model.go` | Future |
| DM-6 | Network topology fields | Present | Fields exist; reconciler is a stub | `plugins/gatewayNetworks/model.go` | Future |
| DM-7 | API endpoints (all 6 resources) | Present | — | `plugins/*/` | — |

### control-plane.spec.md

| # | Requirement | Status | Gap | Code Location | Wave |
|---|-------------|--------|-----|---------------|------|
| CP-1 | gRPC watch streams (6 kinds) | Present | No checkpoint/resume-token on reconnect | `watcher/watcher.go` | — |
| CP-2a | Deploy Gateway workloads | Present | — | `gateway/reconciler.go` | — |
| CP-2b | Provision PostgreSQL | Present | — | `reconcileDatabaseCredentials()` | — |
| CP-2c | TLS via cert-manager | Present | — | `reconcileCertManagerResources()` | — |
| CP-2d | GRPCRoute + BackendTLSPolicy | Present | — | `reconcileGatewayAPIResources()` | — |
| CP-2e | OIDC config injection | Present | — | `ApplyConfigOverrides()` | — |
| CP-2f | Network mesh reconciliation | Missing | Stub: only logs | `reconciler.go:279-295` | Future |
| CP-2g | Canary release rollout | Missing | Stub: only logs | `reconciler.go:99-124` | Future |
| CP-2h | Update resource status/phase | Partial | Only updates `phase`, not `status` | `updateGatewayPhase()` | Future |
| CP-2i | Read provisioning fields from proto | Present | GatewayReconciler populates GatewayConfig from proto fields via JSON unmarshal | `reconciler.go:248-280` | W5 ✅ |
| CP-3 | Delete K8s resources on Gateway deletion | Present | Label-based deletion of all namespaced resources + per-tenant ClusterRoleBinding | `gateway/reconciler.go:DeleteGatewayResources()` | W6 ✅ |
| CP-4 | Status synchronization / health checks | Missing | No periodic health polling | — | Future |
| CP-5 | Multi-cluster client pool | Missing | Single in-cluster client for all gateways | `main.go:58-68` | Future |

### openshell-gateway.spec.md (Core)

| # | Requirement | Status | Gap | Code Location | Wave |
|---|-------------|--------|-----|---------------|------|
| G1 | Gateway as API Resource | Present | CRUD + all provisioning fields (image, server_dns_names, oidc, route, route_address, database_config) | `gateways.proto` | W5 ✅ |
| G2 | Shared Kustomize Library | Missing | No library, no CLI, no examples | — | Future |
| G3 | GatewayReconciler | Present | DELETED handler with namespace cache and full resource cleanup | `reconciler.go` | W6 ✅ |
| G4 | Gateway Manifest Templating | Present | — | `manifests.go` | W1 ✅ |
| G5 | TLS via cert-manager | Present | — | `reconcileCertManagerResources()` | W2 ✅ |
| G6 | Trusted CA Bundle Injection | Present | — | `reconcileTrustedCABundle()` | W3 ✅ |
| G7 | Gateway Config Validation | Present | TOML validation absent | `validation.go` | — |
| G8 | Labels on all resources | Present | — | all manifests + reconciler | W1 ✅ |
| G9 | Gateway Deployment Resources | Partial | `/tmp` emptyDir volume missing from deployment.yaml | `deployment.yaml` | W7 |
| G10 | Per-Gateway RBAC | Present | Per-tenant ClusterRoleBinding `...-<namespace>` | `rbac.yaml` | W6 ✅ |
| G11 | JWT Certgen Job | Partial | Missing `runAsNonRoot`, missing resource requests/limits | `certgen-job.yaml` | W7 |
| G12 | Gateway NetworkPolicies | Present | — | `networkpolicy.yaml` | — |
| G13 | Configuration (gateway.toml) | Partial | `client_ca_path` missing from TLS section | `configmap.yaml` | W7 |
| G14 | OpenShift-Specific Provisioning | Present | — | `reconcileOpenShiftSCC()` | W2 ✅ |
| G15 | Deployment Failure Handling | Present | Relies on re-delivery rather than explicit requeue | `reconciler.go` | — |
| G16 | Separation from Agent Config | Present | — | — | — |
| G17 | SSH Payload Delivery | Missing | `internal/openshell/ssh_upload.go` does not exist | — | Future |
| G18 | Per-Tenant Gateway API Resource | Missing | Code creates GRPCRoute only; no per-tenant K8s Gateway | — | W8 |

### openshell-gateway-database.spec.md

| # | Requirement | Status | Gap | Code Location | Wave |
|---|-------------|--------|-----|---------------|------|
| D1 | Database config fields (storageSize, image) | Present | — | `config.go:32-33` | W1 ✅ |
| D2 | PostgreSQL resource provisioning (PVC, Deployment, Service, NP) | Present | — | `database.yaml` | W1 ✅ |
| D3 | Database credential security (crypto/rand) | Present | — | `reconcileDatabaseCredentials()` | W1 ✅ |
| D4 | Manual credential rotation | Missing | No rotation annotation handler | — | Future |
| D5 | Gateway uses Deployment + env from Secret | Present | — | `deployment.yaml` | W1 ✅ |
| D6 | Database field immutability | Missing | No API validation | — | Future |
| D7 | Gateway deletion protection | Missing | No sandbox check on delete | — | Future |
| D8 | `externalSecretRef` (Phase 2 reserved) | Missing | Not in `DatabaseConfig` struct | — | Future |
| D9 | Label-based cleanup on deletion | Present | Spec updated: label-based deletion via `DeleteGatewayResources()` replaces ownerReferences | `gateway/reconciler.go` | W6 ✅ |

### openshell-gateway-tls.spec.md

| # | Requirement | Status | Gap | Code Location | Wave |
|---|-------------|--------|-----|---------------|------|
| T1 | cert-manager detection + full cert chain | Present | — | `DetectCertManager()`, `reconcileCertManagerResources()` | W2 ✅ |
| T2 | SAN management via cert-manager Certificate | Present | — | `reconciler.go:948-975` | W2 ✅ |
| T3 | Trusted CA bundle copy + mount | Present | — | `reconcileTrustedCABundle()` | W3 ✅ |
| T4 | RBAC for cert-manager resources | Partial | Resources in `kindToResource`; ClusterRole not verified | `reconciler.go` | — |
| T5 | cert-manager absent: block deployment | Partial | Logs WARN but does NOT block deployment | `reconciler.go:54-55` | W7 |
| T6 | SAN change detection (ConfigMap vs API) | Missing | No comparison logic | — | W7 |
| T7 | Gateway restart on cert regeneration | Missing | No hash annotation mechanism | — | W7 |

### openshell-gateway-oidc.spec.md

| # | Requirement | Status | Gap | Code Location | Wave |
|---|-------------|--------|-----|---------------|------|
| O1 | OIDC API fields (issuer, audience, etc.) | Present | — | `config.go:23-29` | W3 ✅ |
| O2 | OIDC role validation (both-or-neither) | Present | — | `ValidateOIDCConfig()` | W3 ✅ |
| O3 | OIDC TOML injection in gateway.toml | Present | — | `ApplyConfigOverrides()` | W3 ✅ |
| O4 | OIDC change detection → ConfigMap update | Present | — | ConfigMap always regenerated | W3 ✅ |
| O5 | `jwks_ttl` default 3600 | Partial | Field exists; default not applied when value is 0 | `config.go:25` | W7 |
| O6 | Custom `config` field bypasses OIDC injection | Missing | No raw TOML `config` field in GatewayConfig | — | Future |
| O7 | Gateway restart on OIDC change | Missing | No hash annotation mechanism | — | W7 |

### openshell-gateway-routing.spec.md

| # | Requirement | Status | Gap | Code Location | Wave |
|---|-------------|--------|-----|---------------|------|
| R1 | Router NetworkPolicy | Partial | Uses namespaceSelector not podSelector; created unconditionally | `reconcileRouterNetworkPolicy()` | W7 |
| R2 | Gateway API detection at startup | Present | — | `DetectGatewayAPI()` | W4 ✅ |
| R3 | `GATEWAY_API_GATEWAY_CLASS` env var | Missing | Code uses NAME/NAMESPACE instead | — | W8 |
| R4 | Gateway API not available: disable + log | Present | — | `reconciler.go:76-80` | W4 ✅ |
| R5 | `route` config field (host, enabled) | Present | — | `config.go:17-20` | W4 ✅ |
| R6 | Auto-derived hostname convention | Partial | Extra `.hsgw.` subdomain vs spec | `reconciler.go:727-731` | W8 |
| R7 | DNS label validation (63-char, truncation+hash) | Missing | No validation | — | W7 |
| R8 | GRPCRoute provisioning | Present | — | `reconciler.go:735-769` | W4 ✅ |
| R9 | GRPCRoute parentRefs: per-tenant Gateway | Partial | Points to shared gateway, not per-tenant | `reconciler.go:749-753` | W8 |
| R10 | GRPCRoute managed label for cleanup | Present | Spec updated: `hypershell.redhat.io/managed` label replaces ownerReferences; cleanup via `deleteGatewayAPIResources()` | `gateway/reconciler.go` | W6 ✅ |
| R11 | BackendTLSPolicy + CA ConfigMap | Present | — | `reconciler.go:813-849` | W4 ✅ |
| R12 | Per-tenant K8s Gateway resource | Missing | Not created at all | — | W8 |
| R13 | Wildcard cert copy (`grpc-gateway-certs`) | Missing | No code copies cert to tenant NS | — | W8 |
| R14 | Route removal: delete resources, clear routeAddress | Partial | `deleteGatewayAPIResources()` removes GRPCRoute, BackendTLSPolicy, CA ConfigMap when route disabled; routeAddress clear deferred to W8 | `gateway/reconciler.go` | W6 ✅ |
| R15 | routeAddress write-back (PATCH to API) | Missing | No code writes routeAddress | — | W8 |
| R16 | Wait for Gateway Accepted+Programmed | Missing | No status polling | — | W8 |
| R17 | Workload restart on config change (hash annotation) | Missing | Cross-cutting: also needed by TLS/OIDC | — | W7 |
| R18 | `kindToResource` mapping for Gateway kind | Missing | Missing `"Gateway": "gateways"` entry | `reconciler.go:226-252` | W8 |

### local-development.spec.md

| # | Requirement | Status | Gap | Code Location | Wave |
|---|-------------|--------|-----|---------------|------|
| LD-1 | `make kind-up` single-command setup | Partial | Builds locally (not registry); `\|\| true` on create; no web console | `Makefile`, `api-server/Makefile` | Future |
| LD-2 | Idempotent subsequent run | Partial | `\|\| true` instead of `kind get clusters` check | `api-server/Makefile` | Future |
| LD-3 | Per-component local swap targets | Missing | No `kind-api-server-up`, `kind-control-plane-up`, `kind-web-console-up` | — | Future |
| LD-4 | Per-component revert targets | Missing | No `kind-*-down` per component | — | Future |
| LD-5 | Cluster teardown (`make kind-down`) | Present | — | `Makefile`, `api-server/Makefile` | — |
| LD-6 | Cluster status with swap state | Partial | Shows pods/services; no swap state reporting | `Makefile` | Future |
| LD-7 | Configurable cluster name | Present | `KIND_CLUSTER_NAME?=hypershell-dev` | `api-server/Makefile` | — |
| LD-8 | Hostname-based service access | Missing | NodePort only; no Gateway API routing | — | Future |
| LD-9 | Container engine support (Podman/Docker) | Present | Auto-detects via `CONTAINER_ENGINE` | `Makefile` | — |
| LD-10 | Security contexts on all containers | Missing | No securityContext on any Kind manifest | `deploy/kind/*.yaml` | Future |
| LD-11 | Swap tracking (`.kind-swaps`) | Missing | `.gitignore` entry exists; no logic | — | Future |
| LD-12 | Developer documentation | Missing | No `DEVELOPMENT.md` | — | Future |
| LD-13 | Hot reload support | Missing | No `KIND_HOT_RELOAD`, no `extraMounts` | — | Future |
| LD-14 | Registry-pulled baseline images | Missing | Always builds locally | — | Future |
| LD-15 | Red Hat hardened DB image | Missing | `postgres:13` from Docker Hub | `deploy/kind/postgres.yaml` | Future |
| LD-16 | Gateway API CRDs | Missing | Not installed | — | Future |
| LD-17 | cloud-provider-kind | Missing | Not started | — | Future |
| LD-18 | cert-manager | Missing | Not installed | — | Future |
| LD-19 | Keycloak | Missing | Not deployed | — | Future |
| LD-20 | HyperShell Gateway resource in Kind | Missing | Not created | — | Future |
| LD-21 | Gateway API routing (HTTPRoute, GRPCRoute) | Missing | No routing resources | — | Future |
| LD-22 | Multiple namespace deployments | Missing | No `kind-deploy`/`kind-undeploy` | — | Future |
| LD-23 | Single root Makefile (deprecate component) | Partial | Root delegates to `api-server/Makefile` | `Makefile` | Future |
| LD-24 | NodePort fallback (`KIND_USE_NODEPORT`) | Partial | Only API server HTTP port mapped | `kind-config.yaml` | Future |

### web-console/architecture.spec.md

| # | Requirement | Status | Gap | Code Location | Wave |
|---|-------------|--------|-----|---------------|------|
| WEB-ARCH-01 | Client-rendered SPA | Present | — | `react-router.config.ts` | — |
| WEB-ARCH-02 | Gateway management routes | Present | — | `routes.ts`, `gateway-ui/` | — |
| WEB-ARCH-03 | Source/runtime boundaries | Present | — | `eslint.architecture.mjs` | — |
| WEB-PKG-01 | pnpm workspace | Present | — | `pnpm-workspace.yaml` | — |
| WEB-PKG-02 | Defensive resolution | Present | — | `pnpm-workspace.yaml` | — |
| WEB-PKG-03 | Policy migration completeness | Partial | Verify pnpm lockfile inspection | `check_dependency_age.py` | — |
| WEB-PKG-04 | Reusable gateway-ui package | Present | — | `components/gateway-ui/` | — |
| WEB-SDK-01 | Browser-compatible SDK | Present | — | `components/sdk-typescript/` | — |
| WEB-AUTH-00 | No-auth dev mode | Present | — | `vite.config.ts` | — |
| WEB-AUTH-01 | OIDC BFF | Partial | `openid-client` declared; endpoints not implemented | `bff/src/app.ts` | Future |
| WEB-AUTH-02 | Session + CSRF protection | Missing | No session management | — | Future |
| WEB-AUTH-03 | Browser session contract | Missing | No session resource endpoint | — | Future |
| WEB-BFF-01 | Same-origin static + API BFF | Present | — | `bff/src/app.ts` | — |
| WEB-DATA-01 | Server-state ownership (TanStack Query) | Present | — | `root.tsx`, `gateway-data.ts` | — |
| WEB-DATA-02 | URL and local state | Partial | Routes encode ID; pagination/search TBD | `routes.ts` | — |
| WEB-DATA-03 | Retry, refresh, cancellation | Partial | Base config present; per-class policies TBD | `root.tsx` | — |
| WEB-DATA-04 | Forms + runtime validation | Present | — | `gateway-create.tsx` | — |
| WEB-UI-01 | PatternFly-first presentation | Present | — | PatternFly 6.6.0 | — |
| WEB-UI-02 | Shared component evidence (Storybook) | Partial | Stories for shell only; none in gateway-ui | `.storybook/` | — |
| WEB-UI-03 | Gateway connection experience | Partial | Components exist; command encoding TBD | `gateway-ui/src/gateways/` | — |
| WEB-I18N-01 | Localization from first implementation | Present | — | `i18n/`, `locales/en.json` | — |
| WEB-QUAL-01 | Static analysis | Present | — | `eslint.config.mjs` | — |
| WEB-QUAL-02 | Test layers | Present | — | `vitest`, `playwright`, `storybook` | — |
| WEB-QUAL-03 | Change and release gates | Partial | `check` script present; CI pipeline TBD | `package.json` | — |
| WEB-DEPLOY-01 | Reproducible container | Present | — | `web-console/Dockerfile` | — |
| WEB-DEPLOY-02 | Assets + runtime config | Present | — | `vite.config.ts`, `bff/src/config.ts` | — |
| WEB-SEC-01 | Browser security headers | Present | — | `bff/src/app.ts` (helmet) | — |
| WEB-OBS-01 | Web performance signals | Partial | `web-vitals` declared; wiring TBD | `domain-probes/` | — |

### e2e-openshell.sh (Test Alignment)

| # | Item | Status | Gap | Line | Wave |
|---|------|--------|-----|------|------|
| E1 | StatefulSet → Deployment | Aligned | e2e now checks `deployment` | 196-216 | W1 ✅ |

### local-development.spec.md

| # | Requirement | Status | Gap | Code Location | Priority |
|---|-------------|--------|-----|---------------|----------|
| L1 | Single-Command Setup (`kind-up`) | Present | Root Makefile + `scripts/kind/up.sh` | `Makefile`, `scripts/kind/up.sh` | P0 |
| L2 | Per-Component Swap | Missing | Swap targets not yet implemented | — | P1 |
| L3 | Cluster Teardown (`kind-down`) | Present | Root Makefile + `scripts/kind/down.sh` | `Makefile`, `scripts/kind/down.sh` | P0 |
| L4 | Cluster Status (`kind-status`) | Present | Root Makefile + `scripts/kind/status.sh` | `Makefile`, `scripts/kind/status.sh` | P0 |
| L5 | Configurable Cluster Name | Present | `KIND_CLUSTER_NAME` in lib.sh | `scripts/kind/lib.sh` | — |
| L6 | Hostname Routing (Gateway API) | Partial | HTTPRoutes + `/etc/hosts` wired; multi-namespace routing missing | `deploy/kind/prerequisites/` | P1 |
| L7 | NodePort Fallback | Partial | Config variables defined; nodeport-services.yaml exists | `scripts/kind/lib.sh` | P2 |
| L8 | Gateway via REST API | Present | Fleet + Gateway seeded via curl in up.sh | `scripts/kind/up.sh` | P0 |
| L9 | Controller RBAC | Present | ClusterRole + ClusterRoleBinding | `deploy/kind/controller-rbac.yaml` | P0 |
| L10 | API Server Database | Present | postgres.yaml (Secret + Deployment + Service) | `deploy/kind/postgres.yaml` | P0 |
| L11 | Hot Reload | Missing | Not yet implemented | — | P2 |
| L12 | Multi-Namespace Deployments | Missing | Not yet implemented | — | P2 |
| L13 | Swap Tracking | Missing | `.kind-swaps` file not implemented | — | P2 |
| L14 | Developer Documentation | Present | `DEVELOPMENT.md` created | `DEVELOPMENT.md` | P0 |
| L15 | Container Engine Support | Present | Auto-detection (Podman preferred) | `scripts/kind/lib.sh` | — |
| L16 | Offline Development (`LOCAL_IMAGES`) | Present | `build-images.sh` + up.sh integration | `scripts/kind/build-images.sh` | — |
| L17 | Red Hat HI Images | Partial | Spec requires HI; `KIND_DB_IMAGE` override exists but default still standard RHEL | — | P1 |

---

## Wave Plan

### Wave 1-6: COMPLETED

| Wave | Scope | Status |
|------|-------|--------|
| W1 | StatefulSet → Deployment + PostgreSQL Backend | ✅ Complete |
| W2 | cert-manager TLS | ✅ Complete |
| W3 | OIDC + Trusted CA Bundle | ✅ Complete |
| W4 | Gateway API Routing (GRPCRoute + BackendTLSPolicy) | ✅ Complete |
| W5 | Gateway Proto Schema + API Fields | ✅ Complete |
| W6 | Gateway Deletion + Cleanup + Route Removal | ✅ Complete |

**Wave 5 summary:** Added 6 gateway provisioning fields (image, server_dns_names, route_address, oidc, route, database_config) across proto, OpenAPI, model, migration, presenters, and gRPC/HTTP handlers. Control plane reconciler populates GatewayConfig from proto fields. Added ExternalSecretRef to DatabaseConfig.

**Wave 6 summary:** Implemented `DeleteGatewayResources()` with label-based deletion of all namespaced resources + per-tenant ClusterRoleBinding cleanup. Added in-memory namespace cache for DELETED event handling (gRPC DELETE events have nil resource). Changed ClusterRoleBinding to per-tenant naming (`...-<namespace>`). Added `deleteGatewayAPIResources()` for route removal when routing disabled. ownerReferences deferred — explicit deletion covers the cleanup need.

### Wave 7: Cross-Cutting Fixes + Workload Restart Mechanism

**Scope:** G9, G11, G13, T5, T6, T7, O5, O7, R1, R7, R17
**Dependency:** Wave 5

1. Add `/tmp` emptyDir volume to `deployment.yaml`
2. Add `client_ca_path` to `[openshell.gateway.tls]` in `configmap.yaml`
3. Add `runAsNonRoot: true` to certgen job container SecurityContext
4. Add resource requests/limits to certgen job (cpu:50m/200m, memory:64Mi/128Mi)
5. Implement hash-annotation mechanism: compute SHA256 of ConfigMap + Secret data, annotate Deployment pod template → triggers rolling restart on config/cert changes
6. Apply `jwks_ttl` default (3600) when value is 0 in `ApplyConfigOverrides()`
7. Block gateway deployment when cert-manager is absent (not just WARN)
8. Add SAN change detection (compare ConfigMap `server_sans` to API `serverDnsNames`)
9. Fix router NetworkPolicy: use `podSelector` with gateway label; only create when `route` config present
10. Add DNS label validation (63-char limit, truncation+hash) for route hostnames
11. Verify: `go build ./...`, `go vet ./...`

### Wave 8: Per-Tenant Gateway API Resources + routeAddress

**Scope:** G18, R3, R6, R9, R12, R13, R15, R16, R18
**Dependency:** Wave 5, Wave 6

1. Add `"Gateway": "gateways"` to `kindToResource` mapping
2. Add `GATEWAY_API_GATEWAY_CLASS` env var (default `openshift-default`)
3. Create per-tenant `gateway.networking.k8s.io/v1 Gateway` resource with gatewayClassName, listener, hostname, TLS termination
4. Copy wildcard cert Secret (`grpc-gateway-certs`) from CP namespace to tenant namespace
5. Update GRPCRoute `parentRefs` to reference per-tenant Gateway (same namespace)
6. Fix hostname convention: `openshell-gateway-<ns>.<base-domain>` (remove `.hsgw.` segment)
7. Implement routeAddress discovery: poll Gateway status for `Accepted: True` + `Programmed: True`, extract address
8. PATCH `routeAddress` field back to API server via gRPC
9. Verify: `go build ./...`, `go vet ./...`

### Future (Deferred)

| # | Item | Domain | Reason |
|---|------|--------|--------|
| G2 | Shared Kustomize Library + CLI | Gateway | Architectural; needs design |
| G17 | SSH Payload Delivery | Gateway | New feature; needs design |
| D4 | Manual Credential Rotation | DB | Operational; not blocking |
| D6 | Database Field Immutability | DB | API server validation |
| D7 | Gateway Deletion Protection | DB | API server validation |
| D8 | `externalSecretRef` (Phase 2) | DB | Intentionally deferred |
| O6 | Custom raw TOML `config` field | OIDC | Advanced; not blocking |
| DM-4 | Gateway `status` writeback | Data Model | Depends on CP-4 |
| DM-5 | Canary release logic | Data Model | GatewayReleaseReconciler is stub |
| DM-6 | Network mesh logic | Data Model | GatewayNetworkReconciler is stub |
| CP-4 | Status synchronization / health checks | CP | Needs periodic reconcile loop |
| CP-5 | Multi-cluster client pool | CP | Architecture: per-cluster kubeconfig |
| LD-* | Local development (most items) | Local Dev | Spec recently authored; MVP first |
| WEB-AUTH-* | OIDC BFF + session + CSRF | Web Console | After no-auth dev mode |

### Cross-Cutting Findings

1. **Stale `statefulset.yaml`**: `manifests/gateway/statefulset.yaml` exists but is unreferenced (uses SQLite). Should be removed.
2. **Naming: Sector vs Fleet**: Spec uses "Sector", code uses "Fleet". Cosmetic; consistent within code.
3. **No restart mechanism (cross-spec)**: TLS, OIDC, and Routing specs all require workload restart on config changes. Addressed in Wave 7 via hash annotation.
4. **Label-based cleanup (cross-spec)**: Database and Routing specs updated to use `hypershell.redhat.io/managed` label-based deletion instead of ownerReferences. ownerReferences were infeasible (DB resources created before Deployment) and unnecessary given explicit cleanup in `DeleteGatewayResources()`.

---

## Reconciliation History

| Date | Commit | Action | Coverage | Notes |
|------|--------|--------|----------|-------|
| 2026-08-03 | initial | Initial setup | 100% | Baseline with 6 Kinds fully implemented |
| 2026-08-05 | working tree | Registered UI standards | 100% platform | UI standards are evaluated by `/ui-standards`, not counted as feature reconciliation requirements |
| 2026-08-05 | working tree | Added PatternFly standard | 100% platform | PatternFly 6, canonical reuse, and duplicate-component prevention apply to the web console |
| 2026-08-05 | working tree | Added UI architecture and observability standards | 100% platform | Narrow ports/adapters boundaries and typed fan-out domain probes apply to browser and BFF workflows |
| 2026-08-05 | working tree | Web-console bootstrap increments 1-3 | 64% overall | Root pnpm migration, browser-compatible SDK, React Router/PatternFly scaffold, secure static BFF, tests, and production container; authenticated product increments remain open |
| 2026-08-06 | 0585632 | Gap analysis after gateway spec update | 44% | 5 gateway sub-specs added; 19 missing, 7 partial, 15 present |
| 2026-08-06 | 0585632+W1-W4 | Executed waves 1-4 | 85% | 4 waves: Deployment+PG, cert-manager, OIDC+CA, GatewayAPI |
| 2026-08-06 | working tree | Local-dev reconciliation | 73% | Kind cluster scripts, deploy manifests, REST API seeding, controller RBAC, DEVELOPMENT.md |
| 2026-08-06 | f27730f | Rebased on main (PR #14 gateway reconciler merged) | 73% | Gateway reconciler in codebase; updated Dockerfiles with dropreplace + -mod=mod; control-plane Dockerfile with manifests COPY |
| 2026-08-07 | b83c635 | Full re-analysis after spec expansion | 62% | 22 specs (was 9); 165 requirements; local-dev and web-console specs added; gateway core spec detailed with 18 requirements; routing gaps surfaced |
| 2026-08-07 | working tree | Executed Wave 5: Gateway Proto Schema + API Fields | 60% | 6 provisioning fields added to proto/OpenAPI/model/migration; CP reconciler populates GatewayConfig from proto; ExternalSecretRef added to DatabaseConfig |
| 2026-08-07 | working tree | Executed Wave 6: Gateway Deletion + Cleanup + Route Removal | 60% | DeleteGatewayResources() with label-based cleanup; namespace cache for DELETED events; per-tenant ClusterRoleBinding; deleteGatewayAPIResources() for route disable; ownerReferences deferred |
