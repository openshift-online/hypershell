---
name: spec
description: >
  Create or modify a spec following the project's spec format and conventions.
  Use when the user wants to write a new spec, add requirements or scenarios
  to an existing spec, or restructure spec content. Triggers on: "write a spec",
  "create a spec", "add a requirement", "spec this out", "define the behavior",
  "new spec for", "update the spec".
---

# Write or Modify a Spec

Help the user create or change a spec that describes desired system behavior.

## User Input

```text
$ARGUMENTS
```

## Steps

### Phase 1 -- Frame

- **Desired state only.** Ask what the system should do, not what's broken.
- **Scope boundary.** Which components does this change touch? (API, gRPC, CP, plugins)
- **Reserved terms check.** Verify no collision with HyperShell domain terms (Fleet, Gateway, GatewayNetwork, GatewayRelease, ManagedCluster, ManagedDatabase).

### Phase 2 -- Ground in the codebase

Read actual code and existing specs in the affected areas:

- Read existing specs in the target domain
- Grep the components identified in Phase 1
- Summarize back in 3-5 sentences
- Ask only where the codebase doesn't give a clear answer

### Phase 3 -- Draft the Spec

Follow the spec format:

- **Purpose section** -- one paragraph describing the domain or feature
- **Requirements** -- each states an observable behavior using RFC 2119 keywords (SHALL, MUST, SHOULD, MAY)
- **Scenarios** -- concrete Given/When/Then examples

### Phase 4 -- Critic Pass

Check for:

- Schema / migration impacts
- Cross-component consistency
- HyperShell terminology correctness
- Incremental API search debounce, cancellation, literal wildcard/escape semantics, and bounded request counts
- Server-state freshness, refetch, retry, and invalidation behavior rather than incidental framework defaults
- Localized caller-supplied fallback labels in reusable presentation helpers

### Phase 5 -- Apply and Verify

Apply all fixes. Place the file in `specs/{domain}/` with filename `<descriptive-title>.spec.md`.

## Spec Format Reference

### Required Format

```markdown
# <Domain> Specification

## Purpose

High-level description of this spec's domain.

## Requirements

### Requirement: <Name>

The system SHALL <observable behavior>.

#### Scenario: <Name>

- GIVEN <precondition>
- WHEN <action>
- THEN <expected outcome>
- AND <additional outcome>
```

### Principles

1. **Desired state, not issue tracking.**
2. **Living documents.** Amended, replaced, or deleted -- never archived.
3. **Behavior contracts, not implementation plans.**
4. **Named descriptively.** `<descriptive-title>.spec.md`
