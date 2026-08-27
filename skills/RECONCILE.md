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

**Last analyzed**: 2026-08-27 (OpenShell Gateway Console GC-W1 complete)
**Spec corpus**: 40 spec files; the coverage table tracks 32 analyzed feature/spec groups after adding OpenShell Gateway Console
**Codebase commit**: 9984ed0 (fix/console OpenShift Route ingress)

### Coverage Summary

| Domain | Specs | Requirements | Present | Partial | Missing | Deferred | Coverage |
|--------|-------|-------------|---------|---------|---------|----------|----------|
| Platform - Data Model | 1 | 12 | 11 | 1 | 0 | 0 | 96% |
| Platform - Control Plane | 1 | 13 | 8 | 1 | 4 | 0 | 65% |
| Platform - Gateway (core) | 1 | 18 | 12 | 3 | 3 | 0 | 75% |
| Platform - Gateway DB | 1 | 14 | 11 | 0 | 3 | 0 | 79% |
| Platform - Gateway TLS | 1 | 7 | 3 | 2 | 2 | 0 | 57% |
| Platform - Gateway OIDC | 1 | 9 | 6 | 1 | 2 | 0 | 72% |
| Platform - Gateway Routing | 1 | 18 | 6 | 4 | 8 | 0 | 44% |
| Platform - Gateway Console | 1 | 9 | 9 | 0 | 0 | 0 | 100% |
| Platform - Gateway Keycloak | 1 | 9 | 9 | 0 | 0 | 0 | 100% |
| Platform - Gateway Service Accounts | 1 | 15 | 15 | 0 | 0 | 0 | 100% |
| Platform - Gateway Secret Rotation | 1 | 8 | 5 | 0 | 1 | 2 | 63% |
| Platform - Namespace GC | 1 | 6 | 6 | 0 | 0 | 0 | 100% |
| Platform - Sandbox Count | 1 | 6 | 6 | 0 | 0 | 0 | 100% |
| Platform - Local Development | 1 | 25 | 23 | 0 | 1 | 1 | 96% |
| Platform - E2E Testing | 1 | 8 | 8 | 0 | 0 | 0 | 100% |
| Platform - OIDC Integration | 1 | 6 | 5 | 1 | 0 | 0 | 92% |
| Web Console - Architecture | 1 | 28 | 21 | 5 | 2 | 0 | 86% |
| Security - RBAC Enforcement | 1 | 13 | 11 | 0 | 0 | 2 | 85% |
| Standards | 13 | 0 | 0 | 0 | 0 | 0 | N/A |
| **TOTAL** | **32** | **224** | **175** | **18** | **26** | **5** | **82%** |

### Spec Dependency Order

```
Layer 0 (roots):  data-model, standards/*
Layer 1:          control-plane, local-development, web-console architecture
Layer 2:          openshell-gateway (core)
Layer 3:          openshell-gateway-database, openshell-gateway-tls
Layer 4:          openshell-gateway-oidc (depends on TLS for trusted CA)
Layer 4.5:        openshell-gateway-secret-rotation (depends on database, credentials, TLS)
Layer 5:          openshell-gateway-routing (depends on TLS for BackendTLSPolicy)
Layer 5.5:        openshell-gateway-keycloak (depends on oidc, rbac-enforcement)
Layer 5.6:        openshell-gateway-console (depends on routing, keycloak)
Layer 5.75:       openshell-gateway-service-accounts (depends on keycloak, oidc, rbac-enforcement, security)
Layer 6:          local-development (depends on all platform specs)
Layer 1.5:        security/rbac-enforcement (depends on data-model)
Layer 7:          web-console/architecture (depends on data-model, security, UI standards)
```

---

## Gap Table

### openshell-gateway-console.spec.md

| # | Requirement | Status | Gap | Code Location | Wave |
|---|-------------|--------|-----|---------------|------|
| GC-1 | Console enablement follows the selected ingress mode | Present | - | `control-plane/internal/gateway/{reconciler.go,console.go}`, `reconciler/health.go` | GC-W1 |
| GC-2 | Confidential console Keycloak client | Present | - | `control-plane/internal/keycloak/client.go`, `gateway/console.go` | - |
| GC-3 | Stable console credential Secret | Present | - | `control-plane/internal/gateway/console.go` | - |
| GC-4 | Console Deployment | Present | - | `control-plane/internal/gateway/console.go` | - |
| GC-5 | Service and mode-selected HTTP exposure | Present | - | `control-plane/internal/gateway/console.go`, `deploy/base/controller-rbac.yaml` | GC-W1 |
| GC-6 | Console NetworkPolicies | Present | The policy source uses the configured ingress namespace and supports both ingress controllers. | `control-plane/internal/gateway/console.go` | - |
| GC-7 | Console lifecycle and cleanup | Present | - | `control-plane/internal/gateway/{console.go,reconciler.go}`, `reconciler/health.go` | GC-W1 |
| GC-8 | Provisioning atomicity and idempotency | Present | - | `control-plane/internal/gateway/console.go`, `internal/keycloak/client.go` | - |
| GC-9 | Console address discovery | Present | - | `control-plane/internal/reconciler/reconciler.go`, `gateway/console.go` | GC-W1 |

**Scoped analysis notes:**

- The API, data model, SDKs, CLI, Keycloak client, Secret, Deployment, Service, and NetworkPolicy contracts need no change.
- GC-W1 added the OpenShift Route adapter and made provisioning, readiness, cleanup, and health repair use the selected ingress mode.
- The base controller role permits Route CRUD and create and update access to `routes/custom-host` because the console Route sets `spec.host`.

### openshell-gateway-service-accounts.spec.md

| # | Requirement | Status | Gap | Code Location | Wave |
|---|-------------|--------|-----|---------------|------|
| SA-1 | Synchronous provisioning and one-time delivery | Present | - | `plugins/serviceAccounts/`, `pkg/keycloak/service_accounts.go` | SA-W1..W3 |
| SA-2 | Federated Keycloak is the identity system of record | Present | - | `deploy/base/api-server.yaml`, `pkg/keycloak/service_accounts.go` | SA-W3 |
| SA-3 | Client Credentials token issuance | Present | - | `pkg/keycloak/service_accounts.go`, lifecycle sweep | SA-W3 |
| SA-4 | Single-gateway isolation | Present | - | `pkg/keycloak/service_accounts.go` | SA-W3 |
| SA-5 | User-selected, RBAC-capped OpenShell role | Present | - | `pkg/rbac/authorization.go`, `plugins/serviceAccounts/service.go` | SA-W3 |
| SA-6 | Expiration, revocation, and deletion | Present | - | `plugins/serviceAccounts/` | SA-W3 |
| SA-7 | Replacement-based credential rotation | Present | - | `plugins/serviceAccounts/`, `components/cli/`, `packages/gateway-management-ui/src/service-accounts/` | SA-W3..W5 |
| SA-8 | Gateway lifecycle cleanup | Present | - | `plugins/gateways/deletion_cleanup.go`, `control-plane/internal/keycloak/client.go` | SA-W3 |
| SA-9 | Secret-safe management UI | Present | - | `packages/gateway-management-ui/src/service-accounts/`, `components/web-console/` | SA-W5 |
| SA-10 | CLI and CI workflow | Present | - | `scripts/cli-generator/`, `components/cli/cmd/hypershell/{create,list,get,revoke,delete}/`, `components/cli/pkg/serviceaccount/` | SA-W4 |
| SA-11 | Workspace membership is a separate grant | Present | - | `plugins/serviceAccounts/presenter.go`, `components/cli/pkg/serviceaccount/`, `packages/gateway-management-ui/src/service-accounts/` | SA-W1, W4, W5 |
| SA-12 | Scopes are not configurable in version 1 | Present | - | `openapi.serviceAccounts.yaml`, `pkg/keycloak/service_accounts.go` | SA-W1, W3 |
| SA-13 | Auditability and secret redaction | Present | - | `plugins/serviceAccounts/`, `pkg/keycloak/`, generated SDKs, `components/web-console/bff/`, `packages/gateway-management-ui/src/service-accounts/` | SA-W3, W5 |
| SA-14 | Reconciliation and drift repair | Present | Structural reconciliation intentionally does not fetch a delivered secret or mint a token; this follows the stronger secret rule and records the spec contradiction | `plugins/serviceAccounts/service.go`, `pkg/keycloak/service_accounts.go` | SA-W3 |
| SA-15 | Verification coverage | Present | - | `pkg/keycloak/*_test.go`, `plugins/serviceAccounts/*_test.go`, `pkg/rbac/*_test.go`, `components/cli/pkg/serviceaccount/*_test.go`, `packages/gateway-management-ui/src/service-accounts/*_test.ts*`, `components/web-console/**/*test*` | SA-W1..W6 |

**Scoped analysis notes:**

- The nested API, generated SDKs, CLI, Keycloak lifecycle, reconciliation, cleanup barrier, and gateway-detail management UI now implement the resource's public behavior.
- The SDK and CLI generators now project the nested service-account collection. Generated clients and commands remain reproducible from the OpenAPI contract.
- The spec forbids retrieving or regenerating a delivered client secret during reconciliation, while the full-scope drift scenario asks reconciliation to issue and inspect another Client Credentials token. Creation can perform this token test because it still holds the new secret. Later reconciliation can verify and repair structural Keycloak state but cannot perform a new grant without violating the stronger one-time-secret rule. This remains a specification mismatch; reconciliation will not fetch the secret.
- The BFF forwards `Cache-Control` and `Pragma`, preserving the one-time response's no-store policy end to end.

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
| DM-3f | Gateway `database_config` column removal | Partial | Field removed from Go/API (W8); DROP COLUMN migration not yet added | `plugins/gateways/migration.go` | W8 |
| DM-4 | Gateway phase + status fields | Partial | `phase` updated by CP; `status` field exists but never written | `plugins/gateways/model.go` | Future |
| DM-5 | Canary release strategy fields | Present | Fields exist; no logic implements canary | `plugins/gatewayReleases/model.go` | Future |
| DM-6 | Network topology fields | Present | Fields exist; reconciler is a stub | `plugins/gatewayNetworks/model.go` | Future |
| DM-7 | API endpoints (all 6 resources) | Present | - | `plugins/*/` | - |

### control-plane.spec.md

| # | Requirement | Status | Gap | Code Location | Wave |
|---|-------------|--------|-----|---------------|------|
| CP-1 | gRPC watch streams (6 kinds) | Present | No checkpoint/resume-token on reconnect | `watcher/watcher.go` | - |
| CP-2a | Deploy Gateway workloads | Present | - | `gateway/reconciler.go` | - |
| CP-2b | Provision database via CNPG | Present | ManagedDatabaseReconciler creates CNPG Cluster; GatewayReconciler creates DatabaseRole/Database/Kubernetes Secret CRs | `reconciler.go`, `gateway/reconciler.go` | W8 ✅ |
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
| G1 | Gateway as API Resource | Present | CRUD + all provisioning fields (image, server_dns_names, oidc, route, route_address) | `gateways.proto` | W5 ✅ |
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
| D1 | ManagedDatabase Reconciliation (provider=cnpg) | Present | ManagedDatabaseReconciler creates CNPG Cluster CRs | `reconciler.go` | W8 ✅ |
| D2 | Per-gateway Database/DatabaseRole/Secret CRs | Present | GatewayReconciler provisions CNPG Database+DatabaseRole+Secret in ManagedDB namespace | `gateway/reconciler.go` | W8 ✅ |
| D3 | ManagedDatabase Deletion Protection | Present | API rejects delete (409) when gateways reference it | `plugins/managedDatabases/service.go` | W8 ✅ |
| D4 | Gateway Database Resolution (auto fleet+db) | Present | fleet_id auto-resolved from cluster_id; database_id auto-assigned when sole DB in fleet | `plugins/gateways/service.go` | W8 ✅ |
| D5 | Gateway Credentials Secret (tenant namespace) | Present | `openshell-gateway-db-credentials` created in tenant NS with host/port/dbname/user/password/uri | `gateway/reconciler.go` | W8 ✅ |
| D6 | Database Provisioning Readiness | Present | `waitForCNPGDatabase()` waits 2min for CNPG Database CR `status.applied: true` | `gateway/reconciler.go` | W8 ✅ |
| D7 | Database Credential Security (crypto/rand) | Present | 32-byte hex password; create-or-skip semantics | `gateway/reconciler.go` | W8 ✅ |
| D8 | Manual Credential Rotation (CNPG-based) | Present | `rotateCNPGDatabaseCredentials()` updates CNPG password Secret; CNPG applies to PostgreSQL | `gateway/reconciler.go` | W8 ✅ |
| D9 | Gateway workload uses Deployment + env from Secret | Present | openshell-gateway-db-credentials Secret referenced in Deployment | `deployment.yaml` | W1 ✅ |
| D10 | CNPG Operator Detection at startup | Present | `DetectCNPG()` checks for `postgresql.cnpg.io/v1` API group | `gateway/config.go`, `reconciler.go` | W8 ✅ |
| D11 | Label-based cleanup on deletion (CNPG resources) | Present | CNPG resources in ManagedDB namespace cleaned via `hypershell.redhat.io/gateway-namespace` label | `gateway/reconciler.go` | W8 ✅ |
| D12 | DROP COLUMN migration for database_config | Missing | database_config column still in DB schema; no DROP COLUMN migration added | - | Future |
| D13 | Database field immutability | Missing | No API validation prevents database_id reassignment | - | Future |
| D14 | Gateway Deletion Protection (active sandboxes) | Missing | No sandbox check on delete | - | Future |

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
| O8 | OIDC fields are read-only (auto-populated by CP) | Present | `oidc` field in OpenAPI has `readOnly: true`; auto-populated by Keycloak provisioning | `openapi.gateways.yaml:274-276` | KC-W1 ✅ |
| O9 | Auto-provisioned roles always complete | Present | `reconcileKeycloakClient()` always sets roles_claim, admin_role, user_role from Keycloak config | `gateway/reconciler.go` | KC-W1 ✅ |

### openshell-gateway-routing.spec.md

| # | Requirement | Status | Gap | Code Location | Wave |
|---|-------------|--------|-----|---------------|------|
| R1 | Router NetworkPolicy | Partial | Uses namespaceSelector not podSelector; created unconditionally | `reconcileRouterNetworkPolicy()` | W7 |
| R2 | Gateway API detection at startup | Present | - | `DetectGatewayAPI()` | W4 ✅ |
| R3 | `GATEWAY_API_GATEWAY_NAME` env var (required) | Present | Controller requires this env var to reference the pre-existing shared Gateway | `reconciler.go` | W8 ✅ |
| R4 | Gateway API not available: disable + log | Present | - | `reconciler.go:76-80` | W4 ✅ |
| R5 | `route` config field (host, enabled) | Present | - | `config.go:17-20` | W4 ✅ |
| R6 | Auto-derived hostname convention | Partial | Extra `.hsgw.` subdomain vs spec | `reconciler.go:727-731` | W8 |
| R7 | DNS label validation (63-char limit) | Not needed | Shortened namespace (26 chars) + `gw-` prefix keeps all derived names under 63 chars | - | - |
| R8 | GRPCRoute provisioning | Present | - | `reconciler.go:735-769` | W4 ✅ |
| R9 | GRPCRoute parentRefs: shared Gateway with sectionName | Present | Points to shared gateway with `sectionName: grpc` | `reconciler.go` | W8 ✅ |
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
| OI-5 | Kind OIDC Always-On | Present | OIDC enabled unconditionally in kind-up; KIND_ENABLE_OIDC removed | `scripts/kind/`, `Makefile` | OIDC ✅ |
| OI-6 | Identity Provider Client Security | Partial | redirectUris restricted but port wildcard pattern not supported by Keycloak; needs explicit port URIs | `keycloak.yaml` | Follow-up |

### rbac-enforcement.spec.md

| # | Requirement | Status | Gap | Code Location | Wave |
|---|-------------|--------|-----|---------------|------|
| RBAC-1 | Scope-Aware Permission Evaluation | Present | `rbacAuthzMiddleware` evaluates scope-aware bindings; controlled by `RBAC_ENFORCE` env var | `pkg/rbac/authorization.go` | WR4 ✅ |
| RBAC-2 | Resource List Filtering | Present | Per-query visibility filtering via `GatewayVisibilityFilter` in gateway List handler; `FindGatewayIDsByUserID` DAO method | `plugins/gateways/handler.go`, `pkg/rbac/visibility_filter.go` | KC-W3 ✅ |
| RBAC-3 | User Auto-Provisioning | Present | `UserProvisioningMiddleware` upserts User from JWT claims on every authenticated request | `pkg/rbac/user_provisioning.go`, `plugins/users/service.go` | WR1 ✅, WR3 ✅ |
| RBAC-4 | Bootstrap via Fleet Creation | Present | `fleetHandler.Create` calls `CreateOwnerBinding` atomically in same DB transaction | `plugins/fleets/handler.go`, `pkg/rbac/fleet_bootstrap.go` | WR3 ✅ |
| RBAC-5 | Platform Admin Bootstrap | Deferred | First platform:admin created via DB migration; no CLI command by design | - | Future |
| RBAC-6 | RoleBinding Mutation Authorization | Present | Strictly-below hierarchy enforcement on Create; advisory-locked last-owner protection on Delete | `plugins/roleBindings/service.go` | WR2 ✅, WR8 ✅ |
| RBAC-7 | Gateway OIDC Role Bridge | Present | RoleBindingReconciler watches RoleBinding events via gRPC stream; maps gateway:owner→openshell-admin, gateway:viewer→openshell-user via Keycloak Admin REST API | `reconciler/role_binding_reconciler.go`, `keycloak/client.go` | KC-W2 ✅ |
| RBAC-8 | Auth-Exempt Endpoints | Present | `isExemptEndpoint` exempts POST /fleets, GET /roles, GET /roles/{id}, GET /metadata, GET /openapi | `pkg/rbac/authorization.go` | WR4 ✅, WR8 ✅ |
| RBAC-9 | gRPC Authorization | Present | `isGRPCAuthorized` evaluates bindings against method type (Get/List/Watch=read, Create/Update=write, Delete=owner-only); lazy init via `RegisterPostAuthGRPC*Interceptor` | `pkg/rbac/grpc_interceptor.go`, `plugins/rbac/grpc_init.go` | WR6 ✅, WR8 ✅ |
| RBAC-10 | Service Caller Bypass | Present | Authz middleware checks for service caller (ClientID-based) and bypasses RBAC | `pkg/rbac/authorization.go` | WR4 ✅ |
| RBAC-11 | Error Response Opacity | Present | Singleton GETs return 404 when unauthorized; mutations return generic 403 | `pkg/rbac/authorization.go` | WR4 ✅ |
| RBAC-12 | Production Rollout | Present | `RBAC_ENFORCE=true` env var enables enforcement; separate from framework `enable-authz` | `plugins/rbac/plugin.go` | WR4 ✅ |
| RBAC-13 | Integration Test Coverage | Present | Unit tests: 18 authorization + 6 gRPC (pkg/rbac/). Integration tests: roles (4), roleBindings (12 including hierarchy enforcement, scope FK validation, last-owner protection) | `pkg/rbac/*_test.go`, `plugins/roles/integration_test.go`, `plugins/roleBindings/integration_test.go` | WR7 ✅, WR8 ✅ |

### openshell-gateway-keycloak.spec.md

| # | Requirement | Status | Gap | Code Location | Wave |
|---|-------------|--------|-----|---------------|------|
| KC-1 | Keycloak Service Account Access | Present | `hypershell-keycloak-admin` Secret read at CP startup; token cache with 80% TTL refresh | `main.go:106-118`, `keycloak/client.go` | KC-W1 ✅ |
| KC-2 | Per-Gateway OIDC Client Provisioning | Present | `ProvisionGatewayClient()` creates public client with PKCE, fullScopeAllowed=false, redirectUris | `keycloak/client.go`, `gateway/reconciler.go` | KC-W1 ✅ |
| KC-3 | Client Role Provisioning | Present | `openshell-admin` + `openshell-user` roles created via Admin REST API | `keycloak/client.go:createClientRoles()` | KC-W1 ✅ |
| KC-4 | Protocol Mapper Provisioning | Present | audience, sub, client-roles mappers created on client | `keycloak/client.go:createProtocolMappers()` | KC-W1 ✅ |
| KC-5 | RBAC-Driven Keycloak Role Assignment (OIDC Role Bridge) | Present | `WatchRoleBindings` gRPC stream; RoleBindingReconciler maps gateway:owner→openshell-admin, gateway:viewer→openshell-user | `role_bindings.proto`, `grpc_handler.go`, `role_binding_reconciler.go` | KC-W2 ✅ |
| KC-6 | Auto-Populated OIDC Configuration | Present | `reconcileKeycloakClient()` auto-populates OIDC from Keycloak config and patches Gateway via `UpdateOIDC` callback | `gateway/reconciler.go`, `reconciler/reconciler.go` | KC-W1 ✅ |
| KC-7 | Gateway Visibility Scoping | Present | `GatewayVisibilityFilter` in List handler filters results by caller's RoleBinding gateway_ids | `handler.go`, `visibility_filter.go`, `dao.go:FindGatewayIDsByUserID()` | KC-W3 ✅ |
| KC-8 | Keycloak Client Cleanup | Present | `DeleteGatewayClient()` called in `DeleteGatewayResources()` opts | `gateway/reconciler.go`, `keycloak/client.go` | KC-W1 ✅ |
| KC-9 | Provisioning Atomicity | Present | `ProvisionGatewayClient()` rolls back client on role/mapper creation failure | `keycloak/client.go` | KC-W1 ✅ |

### openshell-gateway-secret-rotation.spec.md

| # | Requirement | Status | Gap | Code Location | Wave |
|---|-------------|--------|-----|---------------|------|
| SR-1 | Database Password Rotation (annotation-triggered) | Present | `rotateCNPGDatabaseCredentials()` updates CNPG password Secret; CNPG applies to PostgreSQL; updates gateway credentials Secret | `gateway/reconciler.go` | W8 ✅ |
| SR-2 | Database Rotation Failure Handling | Present | CNPG password Secret updated first, then gateway credentials Secret; retry is safe because mismatch detected via annotation | `gateway/reconciler.go` | W8 ✅ |
| SR-3 | Config-Hash Coverage for Database Credentials | Present | `applyConfigHashAnnotation` now loops over both `openshell-server-tls` AND `openshell-gateway-db-credentials` Secrets | `gateway/reconciler.go` | SR-W1 ✅ |
| SR-5 | KEK Rotation (Day-2) | Deferred | Explicitly deferred in spec; no gateway re-encryption API exists | - | Future |
| SR-6 | TLS Certificate Rotation (cert-manager) | Present | cert-manager handles renewal; `applyConfigHashAnnotation` includes TLS Secret; config-hash triggers restart | `reconciler.go:540-554` | W7 ✅ |
| SR-7 | Provider Credential Rotation by Driver Type | Deferred | Credential driver fields not yet implemented in reconciler; driver-specific rotation is platform-managed (K8s SA, Vault) | - | Future |
| SR-8 | Interaction Between Credential Driver and DB Password Rotation | Present | DB password rotation is independent of credential driver; wired after `reconcileDatabaseCredentials()` via `RotateDBCredentials` opt | `gateway/reconciler.go` | SR-W1 ✅ |

### openshell-gateway-namespace-gc.spec.md

| # | Requirement | Status | Gap | Code Location | Wave |
|---|-------------|--------|-----|---------------|------|
| NGC-1 | Gateway deletion reaps the gateway namespace (cascade + out-of-namespace cleanup) | Present | Delete event deletes the managed namespace (cascading in-namespace resources incl. sandbox pods); ClusterRoleBinding, Keycloak client, cross-namespace credential RBAC cleaned explicitly; best-effort/idempotent; never gated on sandbox count | `gateway/reconciler.go` `DeleteGatewayResources()`, `gateway/namespace.go` | NGC ✅ |
| NGC-2 | Periodic GC of orphaned namespaces (env-configurable) | Present | `NamespaceGCReconciler` sweeps managed namespaces (both management labels required); `GATEWAY_NAMESPACE_GC_ENABLED`/`_INTERVAL`/`_GRACE_PERIOD` default true/5m/10m | `reconciler/namespace.go`, `config/config.go` | NGC ✅ |
| NGC-3 | Grace period prevents premature deletion (durable annotation) | Present | `hypershell.redhat.io/gc-eligible-since` (RFC3339) stamped on and measured from the namespace so it survives restarts; cleared when a live Gateway reappears | `gateway/namespace.go` `MarkGCEligible`/`ClearGCEligible` | NGC ✅ |
| NGC-4 | Do not reap namespaces of live gateways (abort on list failure) | Present | Liveness derived from API-reported Gateways; sweep aborts entirely if Gateways cannot be listed; existing gateway preserved regardless of phase (Degraded/Failed) | `reconciler/namespace.go` | NGC ✅ |
| NGC-5 | Preserve a durable record before deletion | Present | `GarbageCollected` Event recorded in the control-plane namespace summarizing orphan duration, pod state, and active sandbox count; summary best-effort, never blocks the reap | `reconciler/namespace.go:203` | NGC ✅ |
| NGC-6 | Surface active sandbox count before deletion (console warning) | Present | Delete-confirmation dialog surfaces `active_sandbox_count` as a pluralized warning; advisory only, never gates deletion | `packages/gateway-management-ui/src/gateways/gateway-delete-dialog.tsx` | NGC ✅ |

### openshell-gateway-sandbox-count.spec.md

| # | Requirement | Status | Gap | Code Location | Wave |
|---|-------------|--------|-----|---------------|------|
| SC-1 | Event-driven active sandbox accounting (informer, no full LIST) | Present | Label-selected pod informer on `agents.x-k8s.io/sandbox-name-hash`; increments/decrements on active-set (Running/Pending) transitions; no steady-state full-namespace pod LIST; legacy `managed-by` label dropped | `reconciler/sandboxcount.go`, `gateway/sandbox.go` | S2 ✅ |
| SC-2 | Atomic, non-negative updates | Present | `AdjustActiveSandboxCount` single-column atomic SQL, floored at zero, NULL treated as zero, `IS DISTINCT FROM` guard | `plugins/gateways/dao.go`, `grpc_handler.go` | S1 ✅ |
| SC-3 | Convergence and self-heal (reconcile to cache, restart recovery) | Present | Periodic `selfHeal` sets the absolute count from the informer cache for every gateway namespace (incl. drift-to-zero); immediate baseline after cache sync recovers the count post-restart with no intervening event | `reconciler/sandboxcount.go` | S2 ✅ |
| SC-4 | Control-plane-owned, read-only surfacing | Present | `active_sandbox_count` OpenAPI `readOnly`, excluded from the patch request, written only over the gRPC path (Adjust/Set); nullable `*int` column (NULL = never counted) | `plugins/gateways/model.go`, `openapi/`, `presenter.go` | S1 ✅ |
| SC-5 | Console surfaces the count in the gateways table | Present | Non-sortable "Active sandboxes" column adjacent to name; unset renders a localized not-available fallback (NULL→N/A, 0→"0") | `packages/gateway-management-ui/src/pages/gateway-pages.tsx` | S3 ✅ |
| SC-6 | Advisory semantics (never gates deletion) | Present | Count is an advisory recent value consumed only as an operator warning; never gates gateway or namespace deletion | `reconciler/sandboxcount.go`, `gateway-delete-dialog.tsx` | S2 ✅ |

### e2e-openshell.sh (Test Alignment)

| # | Item | Status | Gap | Line | Wave |
|---|------|--------|-----|------|------|
| E1 | StatefulSet → Deployment | Aligned | e2e now checks `deployment` | 196-216 | W1 ✅ |
| E2 | active_sandbox_count accounting | Aligned | e2e now polls the API for the count on sandbox create/delete (1/2/1) | step 8 | W-S ✅ |

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
| L17 | Red Hat HI Images | Present | Hub DB uses CNPG Cluster manifest with `HYPERSHELL_DATABASE_IMAGE`; gateway DBs use `OPENSHELL_DATABASE_IMAGE` | `deploy/base/hypershell-db-cluster.yaml`, `Makefile` |
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

### GC-W1: OpenShift Route support for the Gateway Console

**Scope:** GC-1, GC-5, GC-7, GC-9
**Dependency:** Existing Gateway Console and Route ingress implementations
**Status:** Complete

1. Select console exposure from the effective gateway ingress mode.
2. Create an edge-terminated OpenShift Route for `openshell-console` in Route mode.
3. Observe HTTPRoute acceptance or OpenShift Route admission for address publication.
4. Reconcile initial provisioning, health repair, inactive exposure removal, and teardown for both modes.
5. Add `routes/custom-host` controller RBAC.
6. Add unit tests for resource shape, readiness, mode selection, and cleanup.
7. Run control-plane build, vet, test, alignment, and review checks.

**GC-W1 summary:** Added an edge-terminated OpenShift Route for the console, selected readiness by ingress mode, removed inactive console exposures, aligned route-enable semantics, and added custom-host RBAC. The complete control-plane test suite, affected-package race tests, vet, lint, build, Kustomize renders, alignment scan, and independent review passed.

### HYPERSHELL-49 OpenShellGatewayServiceAccount waves

| Wave | Scope | Status |
|------|-------|--------|
| SA-W1 | Nested REST/OpenAPI contract with separate create/get/list models and no-store one-time response | Complete |
| SA-W2 | Extend generators for nested resources and regenerate Go/TypeScript SDKs | Complete |
| SA-W3 | Persistence, nested RBAC, synchronous Keycloak adapter, lifecycle/reconciliation, cleanup, audit, deployment wiring | Complete |
| SA-W4 | HyperShell CLI create/list/get/revoke/delete and secret-safe output | Complete |
| SA-W5 | Gateway-detail Service accounts tab, host adapter, local-only handoff, setup commands, management table | Complete |
| SA-W6 | Integration verification, alignment, review-guidance audit, and checkpoint closure | Complete |

Waves execute in this order because every later consumer depends on the public contract. Specs stay frozen. Generated SDK and CLI output is regenerated from OpenAPI rather than edited by hand.

**SA-W5 summary:** Added the gateway-detail `Service accounts` tab, URL-backed collection state, server-side search/filter/sort/pagination, capability-driven creation, one-time local credential handoff, safe OpenShell and Client Credentials command generation, repeatable non-secret setup, and revoke/delete management. The BFF preserves no-store response headers.

**SA-W6 summary:** Hardened concealment, lifecycle retries, orphan cleanup, Keycloak pagination, exact role/scope replacement, expiration enforcement, and gateway endpoint normalization. Verified changed Go packages with the race detector; verified all Go modules with `go vet`; and passed the complete web formatting, lint, type, architecture, localization, unit-coverage, production-build, Storybook, and BFF checks. The repository-wide API integration run remains dependent on a correctly credentialed local PostgreSQL instance.

### Wave 1-6: COMPLETED

| Wave | Scope | Status |
|------|-------|--------|
| W1 | StatefulSet → Deployment + PostgreSQL Backend | ✅ Complete |
| W2 | cert-manager TLS | ✅ Complete |
| W3 | OIDC + Trusted CA Bundle | ✅ Complete |
| W4 | Gateway API Routing (GRPCRoute + BackendTLSPolicy) | ✅ Complete |
| W5 | Gateway Proto Schema + API Fields | ✅ Complete |
| W6 | Gateway Deletion + Cleanup + Route Removal | ✅ Complete |

**Wave 5 summary:** Added 5 gateway provisioning fields (image, server_dns_names, route_address, oidc, route) across proto, OpenAPI, model, migration, presenters, and gRPC/HTTP handlers. Control plane reconciler populates GatewayConfig from proto fields. (`database_config` field added in W5 was superseded by CNPG ManagedDatabase integration in W8 and has been removed.)

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

### Wave 8: CNPG Integration + Per-Tenant Gateway API Resources + routeAddress

**Scope:** D1-D11, D-SR updates, G18, R3, R6, R9, R12, R13, R15, R16, R18
**Dependency:** Wave 5, Wave 6

**Wave 8 partial summary (d1fc36b):** CNPG operator integration complete: ManagedDatabaseReconciler (Cluster CRs), GatewayReconciler (DatabaseRole/Database/Secret CRs), ManagedDatabase deletion protection, gateway fleet/database auto-resolution, CNPG operator detection, credential rotation updated to CNPG Secret approach (no ALTER ROLE). `database_config` field removed from API, SDK, CLI (pb.go + OpenAPI models still need `make proto` + `make generate`). Items R12, R13, R15, R16, R18 (routing) and G18 remain pending.

Remaining routing items:
1. ~~Require `GATEWAY_API_GATEWAY_NAME` env var~~ R3: already Present
2. ~~GRPCRoute parentRef with sectionName~~ R9: already Present
3. Fix hostname convention: `gw-<ns>.<base-domain>` (shortened prefix)
4. Derive routeAddress deterministically from hostname, PATCH to API server via gRPC
5. Per-tenant K8s Gateway resource (R12)
6. Wildcard cert copy `grpc-gateway-certs` to tenant namespace (R13)
7. Wait for Gateway Accepted+Programmed (R16)
8. Add `kindToResource` mapping for Gateway kind (R18)
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

### Wave KC-W1: Keycloak Client Provisioning + OIDC Auto-Population ✅

**Scope:** KC-1, KC-2, KC-3, KC-4, KC-6, KC-8, KC-9 | **Status:** Complete

Created `keycloak/client.go` with full Admin REST API client (token cache with 80% TTL refresh, client CRUD, role/mapper provisioning, atomic rollback). Read `hypershell-keycloak-admin` Secret at CP startup. `reconcileKeycloakClient()` provisions OIDC client with PKCE, fullScopeAllowed=false, openshell-admin/openshell-user roles, audience/sub/client-roles mappers. Auto-populates Gateway `oidc` field via `UpdateOIDC` callback. Keycloak client deleted on Gateway deletion. OpenAPI `oidc` field marked `readOnly: true`.

### Wave KC-W2: OIDC Role Bridge (RoleBinding Event Handling) ✅

**Scope:** KC-5, RBAC-7 | **Status:** Complete

Added `role_bindings.proto` with `WatchRoleBindings` RPC. Created `grpc_handler.go` and `grpc_presenter.go` in roleBindings plugin with role name enrichment via RoleService. Added `WatchRoleBindings` to watcher package. Created `RoleBindingReconciler` that maps gateway:owner→openshell-admin, gateway:viewer→openshell-user and calls `keycloak.AssignClientRole`/`RemoveClientRole`. 7th watch stream launched conditionally when Keycloak is configured. Added `GetUnscoped` to DAO/service for soft-deleted event handling.

### Wave KC-W3: Gateway Visibility Scoping (Per-Query DAO Filtering) ✅

**Scope:** KC-7, RBAC-2 (list filtering) | **Status:** Complete

Added `FindGatewayIDsByUserID` DAO method (distinct gateway_id from role_bindings). Created `GatewayVisibilityFilter` interface and `rbac.NewGatewayVisibilityFilter` adapter. Gateway List handler filters results by accessible gateway IDs when user is authenticated. Singleton GET returns 404 for unauthorized gateways (unchanged, already in RBAC middleware).

### Wave SR-W1: Database Password Rotation ✅

**Scope:** SR-1, SR-2, SR-3, SR-4, SR-8 | **Status:** Complete

Added `database/sql` + `lib/pq` to control plane. `rotateDatabaseCredentials()` checks `rotate-db-credentials` annotation vs `last-db-rotation` on Secret; generates 32-byte hex password via crypto/rand; connects to PostgreSQL and executes `ALTER ROLE`; updates Secret with new password+URL; sets `last-db-rotation` annotation. ALTER ROLE before Secret update for safety. `applyConfigHashAnnotation` now includes `openshell-gateway-db-credentials` Secret. Wired into `ReconcileGateway` after `reconcileDatabaseCredentials()`.

### Wave NGC + S1-S4: Namespace GC + Event-Driven Sandbox Count ✅

**Scope:** NGC-1..NGC-6, SC-1..SC-6 | **Status:** Complete

**Namespace GC (NGC):** `NamespaceGCReconciler` sweeps managed namespaces (both
`app.kubernetes.io/managed-by=hypershell-control-plane` and
`hypershell.redhat.io/managed=true` required) and reaps those orphaned past the
grace period. Grace timer persisted on the `hypershell.redhat.io/gc-eligible-since`
annotation (RFC3339) and cleared when a Gateway reappears. Sweep aborts entirely
if Gateways cannot be listed, so a transient API failure never reaps a live
namespace. A `GarbageCollected` Event is recorded in the control-plane namespace
before deletion. Env-configurable (`GATEWAY_NAMESPACE_GC_ENABLED`/`_INTERVAL`/
`_GRACE_PERIOD`, defaults true/5m/10m). Delete-driven cleanup deletes the
namespace (cascading in-namespace resources incl. sandbox pods) and explicitly
reaps out-of-namespace state (ClusterRoleBinding, Keycloak client, cross-namespace
credential RBAC).

**Sandbox Count (S1-S4):** Migrated active-sandbox accounting from a periodic
full-namespace pod LIST (previously in the health reconciler) to an event-driven
label-selected pod informer.
- **S1:** `AdjustActiveSandboxCount(namespace, delta)` and
  `SetActiveSandboxCount(namespace, count)` gRPC RPCs with atomic single-column
  SQL, floored at zero, NULL-as-zero, `IS DISTINCT FROM` guard. `active_sandbox_count`
  made a nullable `*int` column, OpenAPI `readOnly`, excluded from patch.
- **S2:** `SandboxCountReconciler`: informer on `agents.x-k8s.io/sandbox-name-hash`;
  increments/decrements on active-set transitions; `synced` gate suppresses the
  initial-LIST add burst; periodic `selfHeal` sets the absolute count from the cache
  for every gateway namespace (drift-to-zero + post-restart recovery). Legacy
  `openshell.ai/managed-by=openshell` sandbox label dropped from code and spec.
  Health reconciler's sandbox-counting block removed. Full unit-test suite
  (`sandboxcount_test.go`) incl. a `-race` concurrency test.
- **S3:** Non-sortable "Active sandboxes" console column adjacent to the gateway
  name; unset renders the localized not-available fallback (NULL→N/A, 0→"0").
  New reusable-package message re-extracted into web-console's `en.json`.
- **S4:** Verified: control-plane `go build`/`vet`/`test -race` clean, golangci-lint
  0 issues (control-plane + api-server gateways plugin), reusable UI package and
  web-console `check` green. (api-server unit tests run on CI; the local
  Apple-Silicon go-m1cpu cgo crash at package init is environmental.)

### Future (Deferred)

| # | Item | Domain | Reason |
|---|------|--------|--------|
| RBAC-5 | Platform Admin Bootstrap | Security | First admin created via DB migration; no CLI by design |
| G2 | Shared Kustomize Library + CLI | Gateway | Architectural; needs design |
| G17 | SSH Payload Delivery | Gateway | New feature; needs design |
| D13 | Database Field Immutability | DB | API server validation |
| D14 | Gateway Deletion Protection | DB | API server validation |
| D12 | DROP COLUMN migration for `database_config` | DB | Column still in DB schema; destructive migration deferred |
| O6 | Custom raw TOML `config` field | OIDC | Advanced; not blocking |
| SR-5 | KEK Rotation | Secret Rotation | Requires gateway re-encryption API (Day-2) |
| SR-7 | Provider Credential Rotation | Secret Rotation | Platform-managed (K8s SA, Vault); no CP action needed |
| DM-4 | Gateway `status` writeback | Data Model | Depends on CP-4 |
| lib/pq dead dep | `lib/pq` in go.mod but not imported | CP | Run `go mod tidy` in control-plane to remove dead dependency |
| DM-5 | Canary release logic | Data Model | GatewayReleaseReconciler is stub |
| DM-6 | Network mesh logic | Data Model | GatewayNetworkReconciler is stub |
| CP-4 | Status synchronization / health checks | CP | Needs periodic reconcile loop |
| CP-5 | Multi-cluster client pool | CP | Architecture: per-cluster kubeconfig |
| LD-* | Local development (most items) | Local Dev | Spec recently authored; MVP first |
| WEB-AUTH-* | OIDC BFF + session + CSRF | Web Console | Implemented in OIDC wave; 3 minor follow-ups remain |

### Cross-Cutting Findings

1. **Stale `statefulset.yaml`**: `manifests/gateway/statefulset.yaml` exists but is unreferenced (uses SQLite). Should be removed.
2. **Naming: Sector vs Fleet**: Spec now uses "Fleet" (aligned with code). No longer a gap.
3. **No restart mechanism (cross-spec)**: TLS, OIDC, and Routing specs all require workload restart on config changes. Addressed in Wave 7 via hash annotation.
4. **Label-based cleanup (cross-spec)**: Database and Routing specs updated to use `hypershell.redhat.io/managed` label-based deletion instead of ownerReferences. ownerReferences were infeasible (DB resources created before Deployment) and unnecessary given explicit cleanup in `DeleteGatewayResources()`.
5. ~~**Config-hash missing DB credentials**~~: Resolved in SR-W1. `applyConfigHashAnnotation` now includes `openshell-gateway-db-credentials` Secret.
6. ~~**RoleBinding watcher missing**~~: Resolved in KC-W2. CP now watches 7 resource types (added RoleBindings via `WatchRoleBindings` gRPC stream).

---

## Reconciliation History

| Date | Commit | Action | Coverage | Notes |
|------|--------|--------|----------|-------|
| 2026-08-27 | 9984ed0 | Completed Gateway Console GC-W1 | 82% | Added mode-selected Route exposure, admission readiness, lifecycle cleanup, custom-host RBAC, and tests. All nine console requirements are present. |
| 2026-08-27 | 612b373 | Gateway Console scoped gap analysis | 81% | Added the console spec to the registry and found four partial requirements. Planned one control-plane wave for OpenShift Route exposure, readiness, cleanup, RBAC, and tests. |
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
| 2026-08-07 | working tree | Executed Wave 5: Gateway Proto Schema + API Fields | 60% | 5 provisioning fields added to proto/OpenAPI/model/migration; CP reconciler populates GatewayConfig from proto |
| 2026-08-07 | working tree | Executed Wave 6: Gateway Deletion + Cleanup + Route Removal | 60% | DeleteGatewayResources() with label-based cleanup; namespace cache for DELETED events; per-tenant ClusterRoleBinding; deleteGatewayAPIResources() for route disable; ownerReferences deferred |
| 2026-08-11 | working tree | Local-dev spec reconciliation | 73% | KIND_DB_IMAGE env var wired through Makefile/lib.sh/controller.yaml; spec updated: Gateway creation is user-initiated (not automatic in kind-up); DEVELOPMENT.md env var table updated; gap table refreshed - 23/25 requirements present (was 3/24); only multi-namespace deployments remain |
| 2026-08-11 | 049d1a8 | Gap analysis for e2e-testing.spec.md | 58% | New spec: 8 requirements (0 present, 1 partial, 7 missing); 3 waves planned (deploy restructuring, test framework, CI workflow) |
| 2026-08-11 | working tree | Executed E2E waves W1-W3 | 75% | Deploy base/overlay restructuring, e2e test framework with Kind driver, CI e2e workflow; all 8 requirements now present |
| 2026-08-11 | 458c359 | OIDC integration spec authored | 75% | Platform OIDC integration spec covering API JWT, BFF OIDC, IdP config, Kind opt-in |
| 2026-08-11 | working tree | RBAC gap analysis | 63% | New spec `security/rbac-enforcement.spec.md` analyzed; 13 requirements, all missing; 7 RBAC waves planned (R1-R7); Gateway OIDC Role Bridge deferred |
| 2026-08-11 | working tree | Executed Waves R1-R4,R6-R7: RBAC Enforcement | 72% | Full RBAC implementation: 3 new plugins (users, roles, roleBindings), user auto-provisioning middleware, fleet:owner bootstrap, scope-aware HTTP+gRPC authorization, 11 integration tests. 9 present, 2 partial (list filtering, escalation prevention), 2 deferred (admin bootstrap via DB migration, OIDC role bridge) |
| 2026-08-12 | ed3725a | OIDC reconciliation complete | 77% | API server development_oidc env; BFF auth code flow with PKCE (22 tests); CP client_credentials TokenProvider + gRPC PerRPCCredentials; KIND_ENABLE_OIDC opt-in; Keycloak hypershell-control-plane client; verified end-to-end on Kind (8/8 checks pass) |
| 2026-08-12 | working tree | OIDC always-on + Keycloak stability | 77% | Removed KIND_ENABLE_OIDC toggle; OIDC unconditional in kind-up; Keycloak memory 1Gi→2Gi + startup/liveness probes |
| 2026-08-13 | 1055647 | Gap analysis for keycloak + secret-rotation specs | 73% | 2 new specs: keycloak (9 reqs, 6 deferred, 1 partial), secret-rotation (8 reqs, 6 deferred, 1 present, 1 partial). OIDC spec updated: 2 new requirements (O8 read-only, O9 auto-provisioned roles) deferred to KC wave. Data model spec: Sector→Fleet naming aligned. 4 new waves planned (KC-W1/W2/W3, SR-W1). Overall coverage drops from 78% to 73% due to new spec requirements. |
| 2026-08-13 | working tree | Executed KC-W1/W2/W3 + SR-W1 | 80% | Keycloak Admin REST API client (token cache, atomic provisioning, cleanup). RoleBinding gRPC watch stream with role name enrichment. RoleBindingReconciler for OIDC Role Bridge (gateway:owner→openshell-admin, gateway:viewer→openshell-user). Gateway visibility filtering via FindGatewayIDsByUserID. Database password rotation (ALTER ROLE, config-hash). Coverage: 138/183 present (80%), keycloak 100%, secret-rotation 69%. |
| 2026-08-18 | 432b210 | Reconciled namespace-gc + sandbox-count sub-specs | 82% | 2 new platform sub-specs (namespace-gc 6 reqs, sandbox-count 6 reqs), both 100% present. Namespace GC reconciler (dual-label managed-namespace reaping, durable gc-eligible-since grace timer, abort-on-list-failure, GarbageCollected Event, env-configurable). Event-driven sandbox count: atomic Adjust/Set gRPC RPCs (nullable readOnly column), label-selected pod informer with synced-gate + self-heal (drift-to-zero + restart recovery), health reconciler's LIST-based counting removed, legacy `openshell.ai/managed-by` label dropped from code+spec, non-sortable console column adjacent to name with N/A fallback. Coverage: 150/195 present (82%). |
| 2026-08-19 | d1fc36b | CNPG operator integration (Wave 8 partial) | ~74% | ManagedDatabaseReconciler + GatewayReconciler CNPG provisioning; ManagedDatabase deletion protection; auto fleet/db resolution; CNPG operator detection; credential rotation updated to CNPG Secret (no ALTER ROLE); specs updated (gateway-database, control-plane, local-dev, secret-rotation, gateway core) |
| 2026-08-20 | working tree | Remove `database_config` field from API, SDK, CLI | ~74% | Field removed from proto/OpenAPI/model/migration/handlers/presenters/sdk-go/cli; pb.go and OpenAPI models regenerated via `make proto` + `make generate` |
| 2026-08-21 | 361305e | HYPERSHELL-49 scoped gap analysis | 69% | Added 15 OpenShellGatewayServiceAccount requirements, all initially missing. Planned strict API -> SDK -> service/Keycloak -> CLI -> UI -> integration waves. Recorded the post-delivery token-verification contradiction without changing specs. |
| 2026-08-21 | working tree | Executed HYPERSHELL-49 SA-W1..W3 | pending final recount | Added nested REST/OpenAPI, generated SDKs, durable persistence/audit, exact gateway-scoped Keycloak clients, one-time verified secret delivery, role-capped authorization, expiration/revoke/delete reconciliation, and gateway cleanup barriers. |
| 2026-08-21 | working tree | Executed HYPERSHELL-49 SA-W4 | pending final recount | Extended the CLI generator for the nested gateway collection; added create/list/get/revoke/delete commands, explicit mode-0600 one-time credential output, expiration handling, workspace guidance, and secret-redaction tests. |
