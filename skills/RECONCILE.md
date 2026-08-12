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

**Last analyzed**: 2026-08-12
**Spec corpus**: 26 specs across 5 domains (platform, security, web-console, standards/platform, standards/ui)
**Codebase commit**: working tree

### Coverage Summary

| Domain | Specs | Requirements | Present | Partial | Missing | Deferred | Coverage |
|--------|-------|-------------|---------|---------|---------|----------|----------|
| Platform - Data Model | 1 | 12 | 11 | 1 | 0 | 0 | 96% |
| Platform - Control Plane | 1 | 13 | 8 | 1 | 4 | 0 | 65% |
| Platform - Gateway (core) | 1 | 18 | 12 | 3 | 3 | 0 | 75% |
| Platform - Gateway DB | 1 | 9 | 5 | 0 | 4 | 0 | 56% |
| Platform - Gateway TLS | 1 | 7 | 3 | 2 | 2 | 0 | 57% |
| Platform - Gateway OIDC | 1 | 7 | 4 | 1 | 2 | 0 | 64% |
| Platform - Gateway Routing | 1 | 18 | 6 | 4 | 8 | 0 | 44% |
| Platform - Local Development | 1 | 25 | 23 | 0 | 1 | 1 | 96% |
| Platform - E2E Testing | 1 | 8 | 8 | 0 | 0 | 0 | 100% |
| Platform - OIDC Integration | 1 | 6 | 5 | 1 | 0 | 0 | 92% |
| Web Console - Architecture | 1 | 28 | 21 | 5 | 2 | 0 | 86% |
| Security - RBAC Enforcement | 1 | 13 | 9 | 2 | 0 | 2 | 79% |
| Standards | 13 | 0 | 0 | 0 | 0 | 0 | N/A |
| **TOTAL** | **26** | **165** | **120** | **20** | **26** | **3** | **78%** |

### Spec Dependency Order

```
Layer 0 (roots):  data-model, standards/*
Layer 1:          control-plane, local-development, web-console architecture
Layer 2:          openshell-gateway (core)
Layer 3:          openshell-gateway-database, openshell-gateway-tls
Layer 4:          openshell-gateway-oidc (depends on TLS for trusted CA)
Layer 5:          openshell-gateway-routing (depends on TLS for BackendTLSPolicy)
Layer 6:          local-development (depends on all platform specs)
Layer 1.5:        security/rbac-enforcement (depends on data-model)
Layer 7:          web-console/architecture (depends on data-model, security, UI standards)
```

---

## Gap Table

### data-model.spec.md

| # | Requirement | Status | Gap | Code Location | Wave |
|---|-------------|--------|-----|---------------|------|
| DM-1 | Fleet (Sector) Lifecycle CRUD | Present | Naming: spec says "Sector", code says "Fleet" | `plugins/fleets/` | - |
| DM-2 | Fleet-Scoped Resources (FK) | Present | `fleet_id` instead of `sector_id` | all child models | - |
| DM-3a | Gateway field: `image` | Present | Added to model, OpenAPI, proto, migration | `plugins/gateways/model.go` | W5 ✅ |
| DM-3b | Gateway field: `server_dns_names` | Present | Added as JSONB (model `*string`), proto `repeated string`, OpenAPI `[]string` | `plugins/gateways/model.go` | W5 ✅ |
| DM-3c | Gateway field: `oidc` (JSONB) | Present | Added to model, OpenAPI, proto, migration | `plugins/gateways/model.go` | W5 ✅ |
| DM-3d | Gateway field: `route` (JSONB) | Present | Added to model, OpenAPI, proto, migration | `plugins/gateways/model.go` | W5 ✅ |
| DM-3e | Gateway field: `route_address` (read-only) | Present | Added to model, OpenAPI (readOnly), proto, migration | `plugins/gateways/model.go` | W5 ✅ |
| DM-3f | Gateway field: `database` (JSONB) | Present | Added to model, OpenAPI, proto, migration; CP struct has `externalSecretRef` | `plugins/gateways/model.go` | W5 ✅ |
| DM-4 | Gateway phase + status fields | Partial | `phase` updated by CP; `status` field exists but never written | `plugins/gateways/model.go` | Future |
| DM-5 | Canary release strategy fields | Present | Fields exist; no logic implements canary | `plugins/gatewayReleases/model.go` | Future |
| DM-6 | Network topology fields | Present | Fields exist; reconciler is a stub | `plugins/gatewayNetworks/model.go` | Future |
| DM-7 | API endpoints (all 6 resources) | Present | - | `plugins/*/` | - |

### control-plane.spec.md

| # | Requirement | Status | Gap | Code Location | Wave |
|---|-------------|--------|-----|---------------|------|
| CP-1 | gRPC watch streams (6 kinds) | Present | No checkpoint/resume-token on reconnect | `watcher/watcher.go` | - |
| CP-2a | Deploy Gateway workloads | Present | - | `gateway/reconciler.go` | - |
| CP-2b | Provision PostgreSQL | Present | - | `reconcileDatabaseCredentials()` | - |
| CP-2c | TLS via cert-manager | Present | - | `reconcileCertManagerResources()` | - |
| CP-2d | GRPCRoute + BackendTLSPolicy | Present | - | `reconcileGatewayAPIResources()` | - |
| CP-2e | OIDC config injection | Present | - | `ApplyConfigOverrides()` | - |
| CP-2f | Network mesh reconciliation | Missing | Stub: only logs | `reconciler.go:279-295` | Future |
| CP-2g | Canary release rollout | Missing | Stub: only logs | `reconciler.go:99-124` | Future |
| CP-2h | Update resource status/phase | Partial | Only updates `phase`, not `status` | `updateGatewayPhase()` | Future |
| CP-2i | Read provisioning fields from proto | Present | GatewayReconciler populates GatewayConfig from proto fields via JSON unmarshal | `reconciler.go:248-280` | W5 ✅ |
| CP-3 | Delete K8s resources on Gateway deletion | Present | Label-based deletion of all namespaced resources + per-tenant ClusterRoleBinding | `gateway/reconciler.go:DeleteGatewayResources()` | W6 ✅ |
| CP-4 | Status synchronization / health checks | Missing | No periodic health polling | - | Future |
| CP-5 | Multi-cluster client pool | Missing | Single in-cluster client for all gateways | `main.go:58-68` | Future |

### openshell-gateway.spec.md (Core)

| # | Requirement | Status | Gap | Code Location | Wave |
|---|-------------|--------|-----|---------------|------|
| G1 | Gateway as API Resource | Present | CRUD + all provisioning fields (image, server_dns_names, oidc, route, route_address, database_config) | `gateways.proto` | W5 ✅ |
| G2 | Shared Kustomize Library | Missing | No library, no CLI, no examples | - | Future |
| G3 | GatewayReconciler | Present | DELETED handler with namespace cache and full resource cleanup | `reconciler.go` | W6 ✅ |
| G4 | Gateway Manifest Templating | Present | - | `manifests.go` | W1 ✅ |
| G5 | TLS via cert-manager | Present | - | `reconcileCertManagerResources()` | W2 ✅ |
| G6 | Trusted CA Bundle Injection | Present | - | `reconcileTrustedCABundle()` | W3 ✅ |
| G7 | Gateway Config Validation | Present | TOML validation absent | `validation.go` | - |
| G8 | Labels on all resources | Present | - | all manifests + reconciler | W1 ✅ |
| G9 | Gateway Deployment Resources | Partial | `/tmp` emptyDir volume missing from deployment.yaml | `deployment.yaml` | W7 |
| G10 | Per-Gateway RBAC | Present | Per-tenant ClusterRoleBinding `...-<namespace>` | `rbac.yaml` | W6 ✅ |
| G11 | JWT Certgen Job | Partial | Missing `runAsNonRoot`, missing resource requests/limits | `certgen-job.yaml` | W7 |
| G12 | Gateway NetworkPolicies | Present | - | `networkpolicy.yaml` | - |
| G13 | Configuration (gateway.toml) | Partial | `client_ca_path` missing from TLS section | `configmap.yaml` | W7 |
| G14 | OpenShift-Specific Provisioning | Present | - | `reconcileOpenShiftSCC()` | W2 ✅ |
| G15 | Deployment Failure Handling | Present | Relies on re-delivery rather than explicit requeue | `reconciler.go` | - |
| G16 | Separation from Agent Config | Present | - | - | - |
| G17 | SSH Payload Delivery | Missing | `internal/openshell/ssh_upload.go` does not exist | - | Future |
| G18 | Per-Tenant Gateway API Resource | Missing | Code creates GRPCRoute only; no per-tenant K8s Gateway | - | W8 |

### openshell-gateway-database.spec.md

| # | Requirement | Status | Gap | Code Location | Wave |
|---|-------------|--------|-----|---------------|------|
| D1 | Database config fields (storageSize, image) | Present | - | `config.go:32-33` | W1 ✅ |
| D2 | PostgreSQL resource provisioning (PVC, Deployment, Service, NP) | Present | - | `database.yaml` | W1 ✅ |
| D3 | Database credential security (crypto/rand) | Present | - | `reconcileDatabaseCredentials()` | W1 ✅ |
| D4 | Manual credential rotation | Missing | No rotation annotation handler | - | Future |
| D5 | Gateway uses Deployment + env from Secret | Present | - | `deployment.yaml` | W1 ✅ |
| D6 | Database field immutability | Missing | No API validation | - | Future |
| D7 | Gateway deletion protection | Missing | No sandbox check on delete | - | Future |
| D8 | `externalSecretRef` (Phase 2 reserved) | Missing | Not in `DatabaseConfig` struct | - | Future |
| D9 | Label-based cleanup on deletion | Present | Spec updated: label-based deletion via `DeleteGatewayResources()` replaces ownerReferences | `gateway/reconciler.go` | W6 ✅ |

### openshell-gateway-tls.spec.md

| # | Requirement | Status | Gap | Code Location | Wave |
|---|-------------|--------|-----|---------------|------|
| T1 | cert-manager detection + full cert chain | Present | - | `DetectCertManager()`, `reconcileCertManagerResources()` | W2 ✅ |
| T2 | SAN management via cert-manager Certificate | Present | - | `reconciler.go:948-975` | W2 ✅ |
| T3 | Trusted CA bundle copy + mount | Present | - | `reconcileTrustedCABundle()` | W3 ✅ |
| T4 | RBAC for cert-manager resources | Partial | Resources in `kindToResource`; ClusterRole not verified | `reconciler.go` | - |
| T5 | cert-manager absent: block deployment | Partial | Logs WARN but does NOT block deployment | `reconciler.go:54-55` | W7 |
| T6 | SAN change detection (ConfigMap vs API) | Missing | No comparison logic | - | W7 |
| T7 | Gateway restart on cert regeneration | Missing | No hash annotation mechanism | - | W7 |

### openshell-gateway-oidc.spec.md

| # | Requirement | Status | Gap | Code Location | Wave |
|---|-------------|--------|-----|---------------|------|
| O1 | OIDC API fields (issuer, audience, etc.) | Present | - | `config.go:23-29` | W3 ✅ |
| O2 | OIDC role validation (both-or-neither) | Present | - | `ValidateOIDCConfig()` | W3 ✅ |
| O3 | OIDC TOML injection in gateway.toml | Present | - | `ApplyConfigOverrides()` | W3 ✅ |
| O4 | OIDC change detection → ConfigMap update | Present | - | ConfigMap always regenerated | W3 ✅ |
| O5 | `jwks_ttl` default 3600 | Partial | Field exists; default not applied when value is 0 | `config.go:25` | W7 |
| O6 | Custom `config` field bypasses OIDC injection | Missing | No raw TOML `config` field in GatewayConfig | - | Future |
| O7 | Gateway restart on OIDC change | Missing | No hash annotation mechanism | - | W7 |

### openshell-gateway-routing.spec.md

| # | Requirement | Status | Gap | Code Location | Wave |
|---|-------------|--------|-----|---------------|------|
| R1 | Router NetworkPolicy | Partial | Uses namespaceSelector not podSelector; created unconditionally | `reconcileRouterNetworkPolicy()` | W7 |
| R2 | Gateway API detection at startup | Present | - | `DetectGatewayAPI()` | W4 ✅ |
| R3 | `GATEWAY_API_GATEWAY_CLASS` env var | Missing | Code uses NAME/NAMESPACE instead | - | W8 |
| R4 | Gateway API not available: disable + log | Present | - | `reconciler.go:76-80` | W4 ✅ |
| R5 | `route` config field (host, enabled) | Present | - | `config.go:17-20` | W4 ✅ |
| R6 | Auto-derived hostname convention | Partial | Extra `.hsgw.` subdomain vs spec | `reconciler.go:727-731` | W8 |
| R7 | DNS label validation (63-char limit) | Not needed | Shortened namespace (26 chars) + `gw-` prefix keeps all derived names under 63 chars | - | - |
| R8 | GRPCRoute provisioning | Present | - | `reconciler.go:735-769` | W4 ✅ |
| R9 | GRPCRoute parentRefs: per-tenant Gateway | Partial | Points to shared gateway, not per-tenant | `reconciler.go:749-753` | W8 |
| R10 | GRPCRoute managed label for cleanup | Present | Spec updated: `hypershell.redhat.io/managed` label replaces ownerReferences; cleanup via `deleteGatewayAPIResources()` | `gateway/reconciler.go` | W6 ✅ |
| R11 | BackendTLSPolicy + CA ConfigMap | Present | - | `reconciler.go:813-849` | W4 ✅ |
| R12 | Per-tenant K8s Gateway resource | Missing | Not created at all | - | W8 |
| R13 | Wildcard cert copy (`grpc-gateway-certs`) | Missing | No code copies cert to tenant NS | - | W8 |
| R14 | Route removal: delete resources, clear routeAddress | Partial | `deleteGatewayAPIResources()` removes GRPCRoute, BackendTLSPolicy, CA ConfigMap when route disabled; routeAddress clear deferred to W8 | `gateway/reconciler.go` | W6 ✅ |
| R15 | routeAddress write-back (PATCH to API) | Missing | No code writes routeAddress | - | W8 |
| R16 | Wait for Gateway Accepted+Programmed | Missing | No status polling | - | W8 |
| R17 | Workload restart on config change (hash annotation) | Missing | Cross-cutting: also needed by TLS/OIDC | - | W7 |
| R18 | `kindToResource` mapping for Gateway kind | Missing | Missing `"Gateway": "gateways"` entry | `reconciler.go:226-252` | W8 |

### local-development.spec.md (superseded by L-table below)

### web-console/architecture.spec.md

| # | Requirement | Status | Gap | Code Location | Wave |
|---|-------------|--------|-----|---------------|------|
| WEB-ARCH-01 | Client-rendered SPA | Present | - | `react-router.config.ts` | - |
| WEB-ARCH-02 | Gateway management routes | Present | - | `routes.ts`, `gateway-ui/` | - |
| WEB-ARCH-03 | Source/runtime boundaries | Present | - | `eslint.architecture.mjs` | - |
| WEB-PKG-01 | pnpm workspace | Present | - | `pnpm-workspace.yaml` | - |
| WEB-PKG-02 | Defensive resolution | Present | - | `pnpm-workspace.yaml` | - |
| WEB-PKG-03 | Policy migration completeness | Partial | Verify pnpm lockfile inspection | `check_dependency_age.py` | - |
| WEB-PKG-04 | Reusable gateway management UI package | Present | - | `packages/gateway-management-ui/` | - |
| WEB-SDK-01 | Browser-compatible SDK | Present | - | `components/sdk-typescript/` | - |
| WEB-AUTH-00 | No-auth dev mode | Present | - | `vite.config.ts` | - |
| WEB-AUTH-01 | OIDC BFF | Present | Auth code flow with PKCE via openid-client v6; /auth/login, /auth/callback, /auth/logout, /auth/session endpoints; proxy injects Bearer token | `bff/src/auth.ts`, `bff/src/app.ts` | OIDC ✅ |
| WEB-AUTH-02 | Session + CSRF protection | Present | @fastify/secure-session encrypted cookies; Origin header CSRF validation on mutating requests; session rotation on login | `bff/src/auth.ts`, `bff/src/app.ts` | OIDC ✅ |
| WEB-AUTH-03 | Browser session contract | Present | GET /auth/session returns display identity, roles, expiry; no tokens exposed | `bff/src/auth.ts` | OIDC ✅ |
| WEB-BFF-01 | Same-origin static + API BFF | Present | - | `bff/src/app.ts` | - |
| WEB-DATA-01 | Server-state ownership (TanStack Query) | Present | - | `root.tsx`, `gateway-data.ts` | - |
| WEB-DATA-02 | URL and local state | Partial | Routes encode ID; pagination/search TBD | `routes.ts` | - |
| WEB-DATA-03 | Retry, refresh, cancellation | Partial | Base config present; per-class policies TBD | `root.tsx` | - |
| WEB-DATA-04 | Forms + runtime validation | Present | - | `gateway-create.tsx` | - |
| WEB-UI-01 | PatternFly-first presentation | Present | - | PatternFly 6.6.0 | - |
| WEB-UI-02 | Shared component evidence (Storybook) | Partial | Stories for shell only; none in gateway-ui | `.storybook/` | - |
| WEB-UI-03 | Gateway connection experience | Partial | Components exist; command encoding TBD | `gateway-ui/src/gateways/` | - |
| WEB-I18N-01 | Localization from first implementation | Present | - | `i18n/`, `locales/en.json` | - |
| WEB-QUAL-01 | Static analysis | Present | - | `eslint.config.mjs` | - |
| WEB-QUAL-02 | Test layers | Present | - | `vitest`, `playwright`, `storybook` | - |
| WEB-QUAL-03 | Change and release gates | Partial | `check` script present; CI pipeline TBD | `package.json` | - |
| WEB-DEPLOY-01 | Reproducible container | Present | - | `web-console/Dockerfile` | - |
| WEB-DEPLOY-02 | Assets + runtime config | Present | - | `vite.config.ts`, `bff/src/config.ts` | - |
| WEB-SEC-01 | Browser security headers | Present | - | `bff/src/app.ts` (helmet) | - |
| WEB-OBS-01 | Web performance signals | Partial | `web-vitals` declared; wiring TBD | `domain-probes/` | - |

### e2e-testing.spec.md

| # | Requirement | Status | Gap | Code Location | Wave |
|---|-------------|--------|-----|---------------|------|
| E2E-1 | Infra Driver Abstraction | Present | tests/e2e/ with driver selection via E2E_INFRA_DRIVER | `tests/e2e/e2e-openshell.sh` | E2E-W2 ✅ |
| E2E-2a | discover_api_host (Kind) | Present | HTTPRoute lookup + port-forward fallback | `tests/e2e/drivers/kind.sh` | E2E-W2 ✅ |
| E2E-2b | discover_gateway_endpoint (Kind) | Present | GRPCRoute hostname + domain | `tests/e2e/drivers/kind.sh` | E2E-W2 ✅ |
| E2E-2c | get_cluster_domain (Kind) | Present | Returns gw.localhost | `tests/e2e/drivers/kind.sh` | E2E-W2 ✅ |
| E2E-2d | get_cli_binary (Kind) | Present | Returns kubectl | `tests/e2e/drivers/kind.sh` | E2E-W2 ✅ |
| E2E-2e | wait_for_gateway_route (Kind) | Present | Polls Gateway Programmed + GRPCRoute Accepted | `tests/e2e/drivers/kind.sh` | E2E-W2 ✅ |
| E2E-3 | E2E Test Suite Coverage (6 areas) | Present | Infra-agnostic version in tests/e2e/ | `tests/e2e/e2e-openshell.sh` | E2E-W2 ✅ |
| E2E-4 | CI E2E Workflow | Present | GitHub Actions workflow with detect-changes, Kind cluster, summary gate | `.github/workflows/e2e.yml` | E2E-W3 ✅ |
| E2E-5 | Konflux Image Consumption | Present | IMAGE_TAG override in up.sh via kubectl set image; Konflux digest wiring is follow-up | `scripts/kind/up.sh` | E2E-W1 ✅ |
| E2E-6 | CI Artifact Collection | Present | Pod logs, events, describes uploaded on failure only | `.github/workflows/e2e.yml` | E2E-W3 ✅ |
| E2E-7 | Deploy Base/Overlay Structure | Present | deploy/base/ + deploy/kind/ overlay + deploy/openshift/ stub | `deploy/base/`, `deploy/kind/kustomization.yaml` | E2E-W1 ✅ |
| E2E-8 | Backward Compatibility | Present | make kind-up unchanged; IMAGE_TAG now overrides initial deploy images | `scripts/kind/up.sh` | E2E-W1 ✅ |

### oidc-integration.spec.md

| # | Requirement | Status | Gap | Code Location | Wave |
|---|-------------|--------|-----|---------------|------|
| OI-1 | API Server JWT Validation (`development_oidc` env) | Present | New environment with JWT enabled, JWKS config, gRPC bypass methods | `environments/e_development_oidc.go`, `environments.go` | OIDC ✅ |
| OI-2 | BFF OIDC Authorization Code Flow | Present | Auth code + PKCE, encrypted cookies, token refresh, RP-initiated logout | `bff/src/auth.ts`, `bff/src/app.ts` | OIDC ✅ |
| OI-3 | BFF Session Security | Present | @fastify/secure-session, CSRF Origin validation, session rotation | `bff/src/auth.ts`, `bff/src/app.ts` | OIDC ✅ |
| OI-4 | BFF Browser Session Contract | Present | GET /auth/session with identity, roles, expiry; no tokens | `bff/src/auth.ts` | OIDC ✅ |
| OI-5 | Opt-In Kind OIDC | Present | KIND_ENABLE_OIDC wired through Makefile/lib.sh/up.sh/status.sh | `scripts/kind/`, `Makefile` | OIDC ✅ |
| OI-6 | Identity Provider Client Security | Partial | redirectUris restricted but port wildcard pattern not supported by Keycloak; needs explicit port URIs | `keycloak.yaml` | Follow-up |

### rbac-enforcement.spec.md

| # | Requirement | Status | Gap | Code Location | Wave |
|---|-------------|--------|-----|---------------|------|
| RBAC-1 | Scope-Aware Permission Evaluation | Present | `rbacAuthzMiddleware` evaluates scope-aware bindings; controlled by `RBAC_ENFORCE` env var | `pkg/rbac/authorization.go` | WR4 ✅ |
| RBAC-2 | Resource List Filtering | Partial | Authz middleware blocks unauthorized requests; per-query DAO filtering deferred | `pkg/rbac/authorization.go` | WR4 ✅ |
| RBAC-3 | User Auto-Provisioning | Present | `UserProvisioningMiddleware` upserts User from JWT claims on every authenticated request | `pkg/rbac/user_provisioning.go`, `plugins/users/service.go` | WR1 ✅, WR3 ✅ |
| RBAC-4 | Bootstrap via Fleet Creation | Present | `fleetHandler.Create` calls `CreateOwnerBinding` atomically in same DB transaction | `plugins/fleets/handler.go`, `pkg/rbac/fleet_bootstrap.go` | WR3 ✅ |
| RBAC-5 | Platform Admin Bootstrap | Deferred | First platform:admin created via DB migration; no CLI command by design | - | Future |
| RBAC-6 | RoleBinding Mutation Authorization | Present | Strictly-below hierarchy enforcement on Create; advisory-locked last-owner protection on Delete | `plugins/roleBindings/service.go` | WR2 ✅, WR8 ✅ |
| RBAC-7 | Gateway OIDC Role Bridge | Deferred | CP does not propagate RBAC role changes to gateway OIDC config | - | Future |
| RBAC-8 | Auth-Exempt Endpoints | Present | `isExemptEndpoint` exempts POST /fleets, GET /roles, GET /roles/{id}, GET /metadata, GET /openapi | `pkg/rbac/authorization.go` | WR4 ✅, WR8 ✅ |
| RBAC-9 | gRPC Authorization | Present | `isGRPCAuthorized` evaluates bindings against method type (Get/List/Watch=read, Create/Update=write, Delete=owner-only); lazy init via `RegisterPostAuthGRPC*Interceptor` | `pkg/rbac/grpc_interceptor.go`, `plugins/rbac/grpc_init.go` | WR6 ✅, WR8 ✅ |
| RBAC-10 | Service Caller Bypass | Present | Authz middleware checks for service caller (ClientID-based) and bypasses RBAC | `pkg/rbac/authorization.go` | WR4 ✅ |
| RBAC-11 | Error Response Opacity | Present | Singleton GETs return 404 when unauthorized; mutations return generic 403 | `pkg/rbac/authorization.go` | WR4 ✅ |
| RBAC-12 | Production Rollout | Present | `RBAC_ENFORCE=true` env var enables enforcement; separate from framework `enable-authz` | `plugins/rbac/plugin.go` | WR4 ✅ |
| RBAC-13 | Integration Test Coverage | Present | Unit tests: 18 authorization + 6 gRPC (pkg/rbac/). Integration tests: roles (4), roleBindings (12 including hierarchy enforcement, scope FK validation, last-owner protection) | `pkg/rbac/*_test.go`, `plugins/roles/integration_test.go`, `plugins/roleBindings/integration_test.go` | WR7 ✅, WR8 ✅ |

### e2e-openshell.sh (Test Alignment)

| # | Item | Status | Gap | Line | Wave |
|---|------|--------|-----|------|------|
| E1 | StatefulSet → Deployment | Aligned | e2e now checks `deployment` | 196-216 | W1 ✅ |

### local-development.spec.md

| # | Requirement | Status | Gap | Code Location |
|---|-------------|--------|-----|---------------|
| L1 | Single-Command Setup (`kind-up`) | Present | Root Makefile + `scripts/kind/up.sh`; registry-pulled baseline images | `Makefile`, `scripts/kind/up.sh` |
| L2 | Idempotent Subsequent Run | Present | `cluster_exists()` check in lib.sh; manifests reapplied idempotently | `scripts/kind/lib.sh` |
| L3 | Per-Component Swap (up) | Present | `swap-component.sh` for api-server, control-plane, web-console; rebuilds on every call | `scripts/kind/swap-component.sh` |
| L4 | Per-Component Revert (down) | Present | Reverts to baseline image; prints info when not swapped | `scripts/kind/swap-component.sh` |
| L5 | Cluster Teardown (`kind-down` + `kind-teardown`) | Present | `down.sh` removes namespace; `teardown.sh` destroys cluster, stops cloud-provider-kind, DNS, port forwarding | `scripts/kind/down.sh`, `teardown.sh` |
| L6 | Cluster Status (`kind-status`) | Present | Pods, services, swap state, DNS, port forwarding status | `scripts/kind/status.sh` |
| L7 | Configurable Cluster Name | Present | `KIND_CLUSTER_NAME` defaults to `hypershell-dev` | `scripts/kind/lib.sh` |
| L8 | Hostname-Based Service Access | Present | CoreDNS wildcard DNS + pfctl/iptables port forwarding + Gateway API HTTPRoutes | `deploy/kind/prerequisites/`, `scripts/kind/lib.sh` |
| L9 | Container Engine Support | Present | Auto-detects podman/docker; podman 6+ fix via patched cloud-provider-kind | `Makefile`, `scripts/kind/lib.sh` |
| L10 | Image Reference Consistency | Present | Makefile defines refs, exported to scripts, used in manifests | `Makefile` |
| L11 | Security Context Compliance | Present | `runAsNonRoot`, `drop ALL`, `allowPrivilegeEscalation: false` on all containers | `deploy/kind/*.yaml` |
| L12 | Swap Tracking (`.kind-swaps`) | Present | `track_swap()`, `clear_swap()`, `is_swapped()` functions; up.sh preserves swaps | `scripts/kind/lib.sh` |
| L13 | Developer Documentation | Present | `DEVELOPMENT.md` with prerequisites, quickstart, env var ref, troubleshooting | `DEVELOPMENT.md` |
| L14 | Hot Reload Support | Present | Web console: scale down, redirect Service → host Vite via Endpoints, pnpm dev with trap | `scripts/kind/swap-component.sh` |
| L15 | Container Registry | Present | `IMAGE_REGISTRY` + `IMAGE_TAG` configurable | `Makefile` |
| L16 | Offline Development (`LOCAL_IMAGES`) | Present | `build-images.sh` builds all images from `origin/main` via git worktree | `scripts/kind/build-images.sh` |
| L17 | Red Hat HI Images | Present | `hi/postgresql:18.4` digest-pinned; `KIND_DB_IMAGE` override for OSS contributors | `deploy/kind/postgres.yaml`, `Makefile` |
| L18 | Gateway API CRDs | Present | Experimental channel from upstream at `GATEWAY_API_VERSION` (v1.5.1) | `scripts/kind/up.sh` |
| L19 | cloud-provider-kind | Present | Patched build (podman 6+ fix); `--enable-lb-port-mapping`; verified in PATH | `Makefile`, `scripts/kind/up.sh` |
| L20 | cert-manager | Present | Installed from release manifest; waits for deployments ready | `scripts/kind/up.sh` |
| L21 | Keycloak | Present | Full realm with `hypershell-frontend`, `hypershell-provisioner`, users, custom theme; `KIND_KEYCLOAK_URL` skips | `deploy/kind/prerequisites/keycloak.yaml` |
| L22 | Gateway Resource | Present | User-initiated via REST API; `kind-up` seeds prerequisites (Fleet, Cluster, Release, DB) but not the Gateway itself; documented in DEVELOPMENT.md | `DEVELOPMENT.md` |
| L23 | Gateway API Routing | Present | Networking Gateway + HTTPRoutes + wildcard TLS certs via cert-manager | `deploy/kind/prerequisites/` |
| L24 | Multi-Namespace Deployments | Missing | Manifests have hardcoded `hypershell-system`; no namespace templating or scoped HTTPRoutes | - |
| L25 | Single Root Makefile | Present | All kind-* targets in root Makefile; component Makefiles deprecated | `Makefile` |
| L26 | NodePort Fallback | Dropped | Replaced by Gateway API routing + port forwarding | - |

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

**Wave 6 summary:** Implemented `DeleteGatewayResources()` with label-based deletion of all namespaced resources + per-tenant ClusterRoleBinding cleanup. Added in-memory namespace cache for DELETED event handling (gRPC DELETE events have nil resource). Changed ClusterRoleBinding to per-tenant naming (`...-<namespace>`). Added `deleteGatewayAPIResources()` for route removal when routing disabled. ownerReferences deferred - explicit deletion covers the cleanup need.

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
10. ~~DNS label validation~~ Not needed: shortened namespace (26 chars) + `gw-` prefix keeps all derived names under 63 chars
11. Verify: `go build ./...`, `go vet ./...`

### Wave 8: Per-Tenant Gateway API Resources + routeAddress

**Scope:** G18, R3, R6, R9, R12, R13, R15, R16, R18
**Dependency:** Wave 5, Wave 6

1. Add `"Gateway": "gateways"` to `kindToResource` mapping
2. Add `GATEWAY_API_GATEWAY_CLASS` env var (default `openshift-default`)
3. Create per-tenant `gateway.networking.k8s.io/v1 Gateway` resource with gatewayClassName, listener, hostname, TLS termination
4. Copy wildcard cert Secret (`grpc-gateway-certs`) from CP namespace to tenant namespace
5. Update GRPCRoute `parentRefs` to reference per-tenant Gateway (same namespace)
6. Fix hostname convention: `gw-<ns>.<base-domain>` (shortened prefix)
7. Implement routeAddress discovery: poll Gateway status for `Accepted: True` + `Programmed: True`, extract address
8. PATCH `routeAddress` field back to API server via gRPC
9. Verify: `go build ./...`, `go vet ./...`

### Wave E2E-W1: Deploy Base/Overlay + Image Overrides ✅

**Scope:** E2E-5, E2E-7, E2E-8 | **Status:** Complete

Moved shared manifests to `deploy/base/`, created kustomize overlays for Kind and OpenShift, added IMAGE_TAG override support in `up.sh`, verified `kustomize build` for all overlays.

### Wave E2E-W2: E2E Test Framework + Kind Driver ✅

**Scope:** E2E-1, E2E-2a-e, E2E-3 | **Status:** Complete

Created `tests/e2e/lib.sh` (shared utilities), `tests/e2e/drivers/kind.sh` (5 driver functions), `tests/e2e/e2e-openshell.sh` (infra-agnostic test adapted from `components/pr-test/e2e-openshell.sh`). Driver validation at startup with available driver listing.

### Wave E2E-W3: CI E2E Workflow ✅

**Scope:** E2E-4, E2E-5, E2E-6 | **Status:** Complete

Created `.github/workflows/e2e.yml` with PR/push/merge_group triggers, concurrency groups, component detection (api_server, control_plane, e2e, pr_test), Kind cluster creation, e2e test execution, failure-only diagnostic artifacts, 20-min timeout, summary gate. Added `e2e` component to `.github/component-paths.json`.

### Wave R1-R8: RBAC COMPLETED

| Wave | Scope | Status |
|------|-------|--------|
| WR1 | Data Model Foundation (users, roles, roleBindings plugins, 6 built-in roles) | ✅ Complete |
| WR2 | API Surface (handlers, presenters, routes for roles + roleBindings) | ✅ Complete |
| WR3 | User Auto-Provisioning + Fleet Bootstrap (middleware + fleet:owner binding) | ✅ Complete |
| WR4 | Authorization Middleware (scope-aware evaluation, exempt endpoints, enforcement flag) | ✅ Complete |
| WR6 | gRPC Authorization (unary + stream interceptors with lazy init) | ✅ Complete |
| WR7 | Integration Tests (roles: 4 tests, roleBindings: 7 tests) | ✅ Complete |
| WR8 | Security Hardening (12 PR review findings resolved) | ✅ Complete |

**Wave R1 summary:** Created `plugins/users/`, `plugins/roles/`, `plugins/roleBindings/` plugins with models, migrations, DAOs, services. Seeded 6 built-in roles with permissions JSONB and hierarchy levels (0=platform:admin, 1=fleet:owner, 2=fleet:editor/platform:viewer, 3=fleet:viewer/gateway:viewer).

**Wave R2 summary:** Added OpenAPI specs (`openapi.roles.yaml`, `openapi.role_bindings.yaml`), handlers (roles: read-only List/Get; roleBindings: Create/List/Get/Delete), presenters, route registration. Updated openapi_embed_test.go operation count from 31 to 37.

**Wave R3 summary:** `UserProvisioningMiddleware` upserts User from JWT claims (username, email, name) on every authenticated request. `fleetBootstrapper.CreateOwnerBinding` creates fleet:owner RoleBinding atomically in same DB transaction as fleet creation. Central `plugins/rbac/plugin.go` wires middleware on apiV1Router.

**Wave R4 summary:** `rbacAuthzMiddleware` implements `auth.AuthorizationMiddleware` with scope-aware evaluation: loads caller's RoleBindings via `FindBindingsByUserID`, matches against resource scope extracted from URL. Exempt endpoints: POST /fleets, GET /metadata, GET /openapi. Service caller bypass via ClientID detection. Error opacity: 404 for unauthorized singleton GETs, 403 for mutations. `RBAC_ENFORCE=true` env var controls enforcement.

**Wave R6 summary:** `RBACUnaryInterceptor` and `RBACStreamInterceptor` apply same scope-aware evaluation to gRPC calls. `lazyRBACInterceptor` with `sync.Once` resolves services on first call (registered at init time via `RegisterPostAuthGRPC*Interceptor`). `provisionUserForGRPC` extracts JWT payload and provisions user before authorization.

**Wave R7 summary:** Integration tests for roles (TestRoleListReturnsBuiltInRoles, TestRoleGetById, TestRoleGetNotFound, TestRoleListUnauthenticated) and roleBindings (TestRoleList, TestRoleGet, TestRoleBindingCreate, TestRoleBindingDelete, TestRoleBindingList, TestRoleBindingScopeValidation, TestFleetCreationCreatesOwnerBinding).

**Wave R8 summary:** Resolved 12 PR review security findings. Blockers: (1) gRPC interceptor now evaluates bindings against method type via `isGRPCAuthorized` instead of blanket pass-through; (2) RoleBinding Create enforces strictly-below hierarchy with platform:admin exception via `validateHierarchy`. Majors: (3) `matchesFleetRole` fixed `fleet:editor` DELETE bug (`|| true` removed); (4) gateway-scoped bindings now compare `b.GatewayID` against request `gatewayID`; (5) `isExemptEndpoint` now exempts GET /roles and GET /roles/{id}; (6) gateway scope validation rejects `fleet_id` (exactly one FK); (7) last-owner protection uses `NewNonBlockingLock` advisory lock to prevent races. Verified: fleet owner bootstrap IS atomic via framework `TransactionMiddleware`. Added 24 unit tests (`pkg/rbac/`) + 5 new integration tests. All admin seeding references removed from spec and RECONCILE.md.

### Future (Deferred)

| # | Item | Domain | Reason |
|---|------|--------|--------|
| RBAC-5 | Platform Admin Bootstrap | Security | First admin created via DB migration; no CLI by design |
| RBAC-7 | Gateway OIDC Role Bridge | Security | Depends on CP + Keycloak integration design |
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
| WEB-AUTH-* | OIDC BFF + session + CSRF | Web Console | Implemented in OIDC wave; 3 minor follow-ups remain |

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
| 2026-08-11 | working tree | Local-dev spec reconciliation | 73% | KIND_DB_IMAGE env var wired through Makefile/lib.sh/controller.yaml; spec updated: Gateway creation is user-initiated (not automatic in kind-up); DEVELOPMENT.md env var table updated; gap table refreshed - 23/25 requirements present (was 3/24); only multi-namespace deployments remain |
| 2026-08-11 | 049d1a8 | Gap analysis for e2e-testing.spec.md | 58% | New spec: 8 requirements (0 present, 1 partial, 7 missing); 3 waves planned (deploy restructuring, test framework, CI workflow) |
| 2026-08-11 | working tree | Executed E2E waves W1-W3 | 75% | Deploy base/overlay restructuring, e2e test framework with Kind driver, CI e2e workflow; all 8 requirements now present |
| 2026-08-11 | 458c359 | OIDC integration spec authored | 75% | Platform OIDC integration spec covering API JWT, BFF OIDC, IdP config, Kind opt-in |
| 2026-08-11 | working tree | RBAC gap analysis | 63% | New spec `security/rbac-enforcement.spec.md` analyzed; 13 requirements, all missing; 7 RBAC waves planned (R1-R7); Gateway OIDC Role Bridge deferred |
| 2026-08-11 | working tree | Executed Waves R1-R4,R6-R7: RBAC Enforcement | 72% | Full RBAC implementation: 3 new plugins (users, roles, roleBindings), user auto-provisioning middleware, fleet:owner bootstrap, scope-aware HTTP+gRPC authorization, 11 integration tests. 9 present, 2 partial (list filtering, escalation prevention), 2 deferred (admin bootstrap via DB migration, OIDC role bridge) |
| 2026-08-12 | ed3725a | OIDC reconciliation complete | 77% | API server development_oidc env; BFF auth code flow with PKCE (22 tests); CP client_credentials TokenProvider + gRPC PerRPCCredentials; KIND_ENABLE_OIDC opt-in; Keycloak hypershell-control-plane client; verified end-to-end on Kind (8/8 checks pass) |
