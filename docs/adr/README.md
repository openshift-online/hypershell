# Architecture Decision Records (ADRs)

This directory holds the **why** behind the platform: the rationale, trade-offs, and
consequences of design decisions. It is the reference home for knowledge that does NOT
belong in the hot path.

## What goes here vs in `specs/`

- **`specs/` is the "what" and "how"** - the normative requirements (SHALL/MUST), the
  scenarios, the data/config contracts. These are loaded constantly during code
  reconciliation (`skills/build/reconcile`) and by any LLM working the code, so they are
  kept lean.
- **`docs/adr/` is the "why"** - the reasoning that was previously in each spec's
  `## Design Decisions` table and in trailing "because .../rather than ..." rationale.
  It is reference-only and is NOT loaded by reconcile.

A requirement keeps a "because ..." clause inline ONLY when the reason changes the
normative behavior (for example "fail closed because a missing CA means ..."). Pure
justification lives here.

## Format

One ADR file per spec's decisions (for spec-local rationale), or a standalone ADR for a
cross-cutting decision. Numbered `NNNN-slug.md`. Each decision records:

- **Decision** - the choice made (one line).
- **Rationale** - why, including trade-offs considered.
- **Consequences** - what follows (optional).

Specs point back with a single line in place of the removed table, e.g.
`Rationale: [ADR-0001](../../docs/adr/0001-data-model-decisions.md)`.

## Index

| ADR | Covers |
|-----|--------|
| [0001](0001-data-model-decisions.md) | Data model (PLAT-DATA) design decisions |
| [0002](0002-control-plane-decisions.md) | Control plane (CTRL-CORE) design decisions |
| [0003](0003-managed-cluster-registration-decisions.md) | Managed cluster registration (PLAT-CLUS) design decisions |
| [0004](0004-agent-secret-late-binding.md) | Agent Credential Late Binding (AGT-SEC) design decisions |
| [0005](0005-agent-workspace-gateway-api.md) | AgentWorkspace Realization via OpenShell's Existing API (AGT-WSP) design decisions |
| [0006](0006-api-server-observability.md) | API Server Observability (API-OBS) design decisions |
| [0007](0007-control-plane-observability.md) | Control Plane Observability (OBS-CPL) design decisions |
| [0008](0008-control-plane-reconciliation-metrics.md) | Control-Plane Reconciliation Metrics (OBS-CRM) design decisions |
| [0009](0009-e2e-testing.md) | E2E Testing and CI Integration design decisions |
| [0010](0010-ephemeral-ci-secrets.md) | Ephemeral CI Secrets (CICD-SEC) design decisions |
| [0011](0011-ephemeral-pr-environments.md) | Ephemeral Pull-Request Environments (CICD-PREN) design decisions |
| [0012](0012-ephemeral-test-credentials.md) | Ephemeral E2E Test Credentials (CICD-CRED) design decisions |
| [0013](0013-extension-api-openshell-api-mapping.md) | HyperShell Extension API to OpenShell API Mapping (AGT-EXT) design decisions |
| [0014](0014-gateway-deletion-finalization.md) | Gateway Deletion and Finalization (GTW-DEL) design decisions |
| [0015](0015-gateway-operator-workspaces.md) | Gateway Operator-Managed Workspaces (OSG-OPWS) design decisions |
| [0016](0016-gateway-reconcile-concurrency.md) | Gateway Reconcile Concurrency (GTW-CONC) design decisions |
| [0017](0017-global-architecture.md) | Global Architecture (PLAT-ARCH) design decisions |
| [0018](0018-hub-grpc-tls.md) | Hub gRPC External TLS Exposure (CTRL-TLS) design decisions |
| [0019](0019-local-development.md) | Local Development Environment (DEV-LOC) design decisions |
| [0020](0020-oidc-integration.md) | Platform OIDC Integration (SEC-OIDC) design decisions |
| [0021](0021-openshell-gateway-namespace-gc.md) | OpenShell Gateway Namespace Garbage Collection (OSG-NSGC) design decisions |
| [0022](0022-openshell-gateway-sandbox-count.md) | OpenShell Gateway Active Sandbox Count (OSG-SBX) design decisions |
| [0023](0023-reconcile-trace-correlation.md) | Reconcile-to-Request Trace Correlation (CTRL-TRC) design decisions |
| [0024](0024-watch-delete-events.md) | Watch Delete Events (CTRL-WDEL) design decisions |
| [0025](0025-rbac-enforcement.md) | RBAC Enforcement (SEC-RBAC) design decisions |
| [0026](0026-reconciliation-contract.md) | Control Plane Reconciliation Contract (STD-RCON) design decisions |
| [0027](0027-fullsend-lifecycle-label-hooks.md) | Fullsend Lifecycle Label Hooks (SDLC-HOOK) design decisions |
| [0028](0028-jira-github-sync.md) | Jira and GitHub SDLC Synchronization (SDLC-JIRA) design decisions |
| [0029](0029-architecture.md) | Web Console Architecture (WEB-ARCH) design decisions |
| [0030](0030-tracing.md) | Web Console Distributed Tracing (WEB-TRC) design decisions |

See also [`../lessons-learned.md`](../lessons-learned.md) for operational gotchas learned
from running this codebase (distinct from design rationale).
