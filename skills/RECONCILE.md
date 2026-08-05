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

**Last analyzed**: 2026-08-05 (UI standards registered; platform coverage unchanged)
**Spec corpus**: 12 specs across 2 domains
**Codebase commit**: (initial)

### Coverage Summary

| Domain | Specs | Requirements | Present | Partial | Missing | Coverage |
|--------|-------|-------------|---------|---------|---------|----------|
| Platform | 2 | 9 | 9 | 0 | 0 | 100% |
| Standards | 10 | 0 | 0 | 0 | 0 | N/A |
| **TOTAL** | **12** | **9** | **9** | **0** | **0** | **100%** |

### Spec Dependency Order

```
Layer 0 (roots):  data-model, standards/*
Layer 1:          control-plane
```

---

## Gap Table

No gaps identified in initial analysis.

---

## Reconciliation History

| Date | Commit | Action | Coverage | Notes |
|------|--------|--------|----------|-------|
| 2026-08-03 | initial | Initial setup | 100% | Baseline with 6 Kinds fully implemented |
| 2026-08-05 | working tree | Registered UI standards | 100% platform | UI standards are evaluated by `/ui-standards`, not counted as feature reconciliation requirements |
