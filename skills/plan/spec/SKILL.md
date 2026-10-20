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

Note: Specifications are not purely additive. They codify _repository intent_ and
therefore must necessarily be utterly coherent and consistent always. More concretely,
most spec additions are accompanied by deletions & modifications in other specs. To that end, be
extremely thorough to understand the entire specification landscape when executing this
workflow.

## User Input

```text
$ARGUMENTS
```

## Steps

### Phase 1 -- Frame

- **Desired state only.** Ask what the system should do, not what's broken.
- **Scope boundary.** Which components does this change touch? (API, gRPC, CP, plugins)
- **Reserved terms check.** Verify no collision with HyperShell domain terms (Gateway, ManagedCluster).
- **Visual deliverable check.** If the spec describes a new or materially changed visual web-console page and the user provides an approved screenshot or visual reference, the deliverable includes a repository mockup under `specs/web-console/mockups/` and its Storybook design-review story. Treat both the written spec and visual artifact as required before finalizing.

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
- **IDs** -- a new requirement or scenario in an existing file gets the next
  free counter in that file's registered group (see "Requirement and
  Scenario IDs" below); never reuse a counter retired from that group. A new
  file registers its `MODULE-GROUP` in `specs/ids.md` before its first
  requirement is drafted.

### Phase 4 -- Critic Pass

Check for:

- Schema / migration impacts
- Cross-component consistency
- HyperShell terminology correctness
- Incremental API search debounce, cancellation, literal wildcard/escape semantics, and bounded request counts
- Server-state freshness, refetch, retry, and invalidation behavior rather than incidental framework defaults
- Localized caller-supplied fallback labels in reusable presentation helpers
- Every requirement and scenario heading carries an ID in the canonical
  `MODULE-GROUP-NN` / `MODULE-GROUP-NN.SS` form
- No ID is duplicated anywhere in `specs/`
- Every `MODULE-GROUP` used is present in `specs/ids.md`, and owned by this file
- No cross-reference to another spec's requirement cites a retired ID

### Phase 5 -- Apply and Verify

Apply all fixes. Place the file in `specs/{domain}/` with filename `<descriptive-title>.spec.md`.

When the Phase 1 visual deliverable check applies:

- Create or update the mockup TSX/CSS and Storybook story.
- Read `skills/web-console/ui-spec/references/patternfly-mockup-implementation.md`.
- Run `skills/web-console/ui-spec/scripts/check_mockup_patternfly.py` on every changed mockup TSX/CSS file.
- Reference the mockup story and source visual reference in the spec.
- Do not finalize the spec while the mockup or its checks are missing.

## Spec Format Reference

### Required Format

```markdown
# <Domain> Specification

**ID prefix:** <MODULE-GROUP>

## Purpose

High-level description of this spec's domain.

## Requirements

### Requirement: <MODULE-GROUP-NN> -- <Name>

The system SHALL <observable behavior>.

#### Scenario: <MODULE-GROUP-NN.SS> -- <Name>

- GIVEN <precondition>
- WHEN <action>
- THEN <expected outcome>
- AND <additional outcome>
```

A requirement that documents a retained, deprecated, or historical behavior
may use `### Legacy compatibility: <MODULE-GROUP-NN> -- <Name>` instead of
`### Requirement:`. It still gets an ID and still numbers its scenarios the
same way.

### Requirement and Scenario IDs

Full syntax, stability rules, and the enforcement contract are in
`specs/standards/platform/spec-identifiers.spec.md`. Summary used while
drafting:

- **MODULE-GROUP-NN**: MODULE and GROUP are each 3-4 uppercase letters or
  digits starting with a letter; NN is a 2-digit counter (`01`-`99`, 3
  digits only past 99). Look up or register the file's `MODULE-GROUP` in
  `specs/ids.md` before assigning `NN`.
- **Scenario IDs** are `<requirement-ID>.SS`, a 2-digit counter restarting
  at `01` for each requirement.
- **Never renumber or reuse.** Reordering requirements doesn't change their
  IDs. A deleted requirement's ID is retired (listed under a `## Retired
  IDs` section in its spec), never reassigned.
- **One file, one group**, unless the file already hosts more than one
  clearly distinct, independently-evolving sub-area -- check
  `specs/ids.md`'s "Multi-Group Files" section before assuming a second
  group is appropriate.

### Principles

1. **Desired state, not issue tracking.**
2. **Living documents.** Amended, replaced, or deleted -- never archived.
3. **Behavior contracts, not implementation plans.**
4. **Named descriptively.** `<descriptive-title>.spec.md`
5. **Every requirement and scenario has a stable ID.** Assigned once,
   carried for the life of the requirement, never reused after retirement.
   Cross-references between specs cite the ID, not a title.
