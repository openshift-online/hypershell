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

**SDLC flow**: `/reconcile` → `/spec` → `/full-stack-pipeline` → `/dev-cluster`

---

## Reconciliation State

**Last analyzed**: 2026-08-05 (web-console bootstrap implemented on `feat/web-console-bootstrap`)
**Spec corpus**: 14 specs across 3 domains
**Codebase commit**: working tree on `feat/web-console-bootstrap`

### Coverage Summary

| Domain | Specs | Requirements | Present | Partial | Missing | Coverage |
|--------|-------|-------------|---------|---------|---------|----------|
| Platform | 2 | 9 | 9 | 0 | 0 | 100% |
| Standards | 11 | 0 | 0 | 0 | 0 | N/A |
| Web console | 1 | 28 | 10 | 9 | 9 | 52% |
| **TOTAL** | **14** | **37** | **19** | **9** | **9** | **64%** |

### Spec Dependency Order

```
Layer 0 (roots):  data-model, standards/*
Layer 1:          control-plane, web-console architecture
```

---

## Gap Table

| Priority | Requirement group | State | Next increment |
|----------|-------------------|-------|----------------|
| P0 | WEB-AUTH-01..03, WEB-BFF-01 | Missing/partial | Add the selected OIDC provider, server-side rotating sessions, CSRF defenses, and authenticated API proxy. |
| P1 | WEB-DATA-01..04, WEB-API-01 | Missing | Deliver the authenticated fleet shell only after its fleet-scoped API, validation, concurrency, and recovery contracts exist. |
| P1 | WEB-DEPLOY-01, WEB-QUAL-03 | Partial | Add the Kubernetes workload security context/resources/probes and the full main/release browser and manual evidence matrix. |
| P2 | WEB-OBS-01..02 | Missing/partial | Add privacy-reviewed browser signals and correlated BFF metrics/traces before production availability. |

---

## Reconciliation History

| Date | Commit | Action | Coverage | Notes |
|------|--------|--------|----------|-------|
| 2026-08-03 | initial | Initial setup | 100% | Baseline with 6 Kinds fully implemented |
| 2026-08-05 | working tree | Registered UI standards | 100% platform | UI standards are evaluated by `/ui-standards`, not counted as feature reconciliation requirements |
| 2026-08-05 | working tree | Added PatternFly standard | 100% platform | PatternFly 6, canonical reuse, and duplicate-component prevention apply to the web console |
| 2026-08-05 | working tree | Web-console bootstrap increments 1-3 | 64% overall | Root pnpm migration, browser-compatible SDK, React Router/PatternFly scaffold, secure static BFF, tests, and production container; authenticated product increments remain open |
