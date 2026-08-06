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

**Last analyzed**: 2026-08-06 (post-reconciliation)
**Spec corpus**: 9 specs across 2 domains (5 gateway sub-specs added)
**Codebase commit**: 0585632+waves (add-gateway-reconciler-e2e)

### Coverage Summary

| Domain | Specs | Requirements | Present | Partial | Missing | Coverage |
|--------|-------|-------------|---------|---------|---------|----------|
| Platform — Core | 2 | 9 | 9 | 0 | 0 | 100% |
| Platform — Gateway | 1 | 10 | 8 | 1 | 1 | 85% |
| Platform — Gateway DB | 1 | 7 | 4 | 0 | 3 | 57% |
| Platform — Gateway TLS | 1 | 4 | 4 | 0 | 0 | 100% |
| Platform — Gateway OIDC | 1 | 4 | 3 | 1 | 0 | 88% |
| Platform — Gateway Routing | 1 | 7 | 6 | 1 | 0 | 93% |
| Standards | 3 | 0 | 0 | 0 | 0 | N/A |
| **TOTAL** | **9** | **41** | **34** | **3** | **4** | **85%** |

### Spec Dependency Order

```
Layer 0 (roots):  data-model, standards/*
Layer 1:          control-plane
Layer 2:          openshell-gateway (core)
Layer 3:          openshell-gateway-database, openshell-gateway-tls
Layer 4:          openshell-gateway-oidc (depends on TLS for trusted CA)
Layer 5:          openshell-gateway-routing (depends on TLS for BackendTLSPolicy)
```

---

## Gap Table

### openshell-gateway.spec.md (Core)

| # | Requirement | Status | Gap | Code Location | Wave |
|---|-------------|--------|-----|---------------|------|
| G1 | Gateway as API Resource | Present | — | api-server CRUD + gRPC watch | — |
| G2 | Shared Kustomize Library | Missing | Not yet implemented; CLI-only | — | Future |
| G3 | GatewayReconciler | Present | Deploys Deployment + PostgreSQL backend | `internal/gateway/reconciler.go` | W1 ✅ |
| G4 | Gateway Manifest Templating | Present | Working (NAMESPACE/IMAGE placeholders) | `internal/gateway/manifests.go` | — |
| G5 | TLS via cert-manager | Present | `reconcileCertManagerResources()` creates Issuer/Certificate chain | `internal/gateway/reconciler.go` | W2 ✅ |
| G6 | Trusted CA Bundle Injection | Present | `reconcileTrustedCABundle()` + `applyTrustedCAOverrides()` | `internal/gateway/reconciler.go` | W3 ✅ |
| G7 | Gateway Config Validation | Present | Working (image + DNS) | `internal/gateway/validation.go` | — |
| G8 | Labels: `managed-by=hypershell-control-plane` + `hypershell.redhat.io/managed=true` | Present | All manifests + inline resources updated | All manifests + reconciler.go | W1 ✅ |
| G9 | Gateway Deployment Resources | Present | `deployment.yaml` with PostgreSQL init container | `manifests/gateway/deployment.yaml` | W1 ✅ |
| G10 | OpenShift-Specific Provisioning | Partial | SCC + Route present; cert-manager detection added; full integration test needed | `reconciler.go` | W2 ✅ |

### openshell-gateway-database.spec.md

| # | Requirement | Status | Gap | Code Location | Wave |
|---|-------------|--------|-----|---------------|------|
| D1 | Gateway Database Config Field | Present | `DatabaseConfig` struct in `config.go`; proto field deferred | `internal/gateway/config.go` | W1 ✅ |
| D2 | PostgreSQL Resource Provisioning | Present | `database.yaml` with PVC, Deployment, Service, NetworkPolicy | `manifests/gateway/database.yaml` | W1 ✅ |
| D3 | Database Credential Security | Present | `reconcileDatabaseCredentials()` with `crypto/rand` | `internal/gateway/reconciler.go` | W1 ✅ |
| D4 | Manual Credential Rotation | Missing | No rotation annotation handler | — | Future |
| D5 | Gateway Workload Type: Deployment | Present | `deployment.yaml` with `OPENSHELL_DB_URL` from Secret | `manifests/gateway/deployment.yaml` | W1 ✅ |
| D6 | Database Field Immutability | Missing | No API validation | — | Future |
| D7 | Gateway Deletion Protection | Missing | No sandbox check on delete | — | Future |

### openshell-gateway-tls.spec.md

| # | Requirement | Status | Gap | Code Location | Wave |
|---|-------------|--------|-----|---------------|------|
| T1 | TLS via cert-manager (sole strategy) | Present | `reconcileCertManagerResources()` creates full cert chain | `internal/gateway/reconciler.go` | W2 ✅ |
| T2 | SAN Management + Cert Rotation | Present | SANs flow to cert-manager Certificate `dnsNames` | `internal/gateway/reconciler.go` | W2 ✅ |
| T3 | Trusted CA Bundle Injection | Present | `reconcileTrustedCABundle()` copies ConfigMap, mounts in Deployment | `internal/gateway/reconciler.go` | W3 ✅ |
| T4 | RBAC for TLS Resources | Present | `Issuer`/`Certificate` in `kindToResource` mapping | `internal/gateway/reconciler.go` | W2 ✅ |

### openshell-gateway-oidc.spec.md

| # | Requirement | Status | Gap | Code Location | Wave |
|---|-------------|--------|-----|---------------|------|
| O1 | Gateway OIDC API Fields | Present | `OIDCConfig` struct in `config.go`; proto field deferred | `internal/gateway/config.go` | W3 ✅ |
| O2 | OIDC Role Validation | Present | `ValidateOIDCConfig()` enforces both-or-neither rule | `internal/gateway/validation.go` | W3 ✅ |
| O3 | OIDC Configuration in gateway.toml | Present | OIDC TOML injection in `ApplyConfigOverrides()` | `internal/gateway/manifests.go` | W3 ✅ |
| O4 | OIDC Change Detection | Partial | ConfigMap updated on reconcile; hash annotation for restart not yet added | — | W3 ✅ |

### openshell-gateway-routing.spec.md

| # | Requirement | Status | Gap | Code Location | Wave |
|---|-------------|--------|-----|---------------|------|
| R1 | NLB-Backed IngressController | Present | Cluster prereq; documented | — | — |
| R2 | NetworkPolicy for Router Ingress | Present | `reconcileRouterNetworkPolicy()` exists | `reconciler.go:392-465` | — |
| R3 | Gateway API Detection | Present | `DetectGatewayAPI()` checks for GRPCRoute CRD | `internal/gateway/reconciler.go` | W4 ✅ |
| R4 | Gateway Route Config Field | Present | `RouteConfig` struct in `config.go`; proto field deferred | `internal/gateway/config.go` | W4 ✅ |
| R5 | GRPCRoute Provisioning | Present | `reconcileGatewayAPIResources()` creates GRPCRoute | `internal/gateway/reconciler.go` | W4 ✅ |
| R6 | BackendTLSPolicy | Present | BackendTLSPolicy + backend CA ConfigMap created | `internal/gateway/reconciler.go` | W4 ✅ |
| R7 | Route Address Discovery | Partial | Hostname derived; routeAddress PATCH to API server deferred | — | W4 ✅ |

### e2e-openshell.sh (Test Alignment)

| # | Item | Status | Gap | Line | Wave |
|---|------|--------|-----|------|------|
| E1 | StatefulSet → Deployment | Aligned | e2e now checks `deployment` | 196-216 | W1 ✅ |

---

## Wave Plan

### Wave 1: StatefulSet → Deployment + PostgreSQL Backend (Foundation)

**Scope:** G3, G8, G9, D1, D2, D3, D5, E1
**Dependency:** None (foundational change)

1. Create `manifests/gateway/deployment.yaml` (Deployment, not StatefulSet)
   - `--db-url` via `OPENSHELL_DB_URL` env from Secret
   - init container: `pg_isready` waiting for DB
   - Remove `volumeClaimTemplates`
   - Remove SQLite `--db-url` arg
2. Create `manifests/gateway/database.yaml` (PG Secret, PVC, Deployment, Service, NetworkPolicy)
3. Update `reconciler.go` deploy order: database.yaml before deployment.yaml
4. Remove `statefulset.yaml` from deploy order (keep file for migration)
5. Update all manifest labels: `managed-by: hypershell-control-plane` + `hypershell.redhat.io/managed: "true"`
6. Add `crypto/rand` credential generation in reconciler
7. Update `config.go`: add Database fields to GatewayConfig
8. Update `reconciler/reconciler.go`: populate Database config from gateway proto
9. Update e2e-openshell.sh: `statefulset` → `deployment`
10. Verify: `go build ./...`, `go vet ./...`

### Wave 2: cert-manager TLS (Replaces certgen-only)

**Scope:** G5, G10, T1, T2, T4
**Dependency:** Wave 1

1. Add `detectCertManager()` to reconciler (API discovery for `cert-manager.io`)
2. Create cert-manager resources inline in `reconcileGateway()`:
   - Self-signed Issuer → CA Certificate → CA Issuer → Server/Client Certificates
3. Update deploy order: cert-manager resources after namespace, before certgen job
4. certgen job remains for JWT keys (skips TLS if secrets exist)
5. Update ClusterRole: add `cert-manager.io` permissions

### Wave 3: OIDC + Trusted CA Bundle

**Scope:** G6, T3, O1-O4
**Dependency:** Wave 2 (cert-manager for TLS, trusted CA for OIDC discovery)

1. Add `oidc` JSONB field to Gateway proto + migrations
2. Add OIDC TOML injection to `ApplyConfigOverrides()`
3. Add OIDC role validation (both or neither admin/user role)
4. Add ConfigMap hash annotation on Deployment for restart-on-change
5. Implement trusted CA bundle copy + mount

### Wave 4: Gateway API Routing (GRPCRoute + BackendTLSPolicy)

**Scope:** R3-R7
**Dependency:** Wave 2 (cert-manager for BackendTLSPolicy CA)

1. Add `detectGatewayAPI()` to reconciler
2. Add `route` JSONB field to Gateway proto + migrations
3. Create GRPCRoute with ownerReferences to Deployment
4. Create BackendTLSPolicy + CA ConfigMap
5. Derive and PATCH `routeAddress` field

### Future (Deferred)

- G2: Shared Kustomize Library
- D4: Manual Credential Rotation
- D6: Database Field Immutability (API server validation)
- D7: Gateway Deletion Protection (API server validation)

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
