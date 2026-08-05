---
name: ui-standards
description: >
  Audit or design web user interfaces against HyperShell's UI/UX standards.
  Use when reviewing UI changes, diffs, branches, or PRs; auditing accessibility,
  usability, Red Hat brand/color, content, forms, navigation, responsive behavior,
  trust, performance, or localization; or turning a product intent into UI/UX recommendations, flows,
  acceptance criteria, and test plans. Not for general backend or security review.
---

# Apply UI Standards

Use the repository's standards as the authority for interface requirements. Separate what source inspection proves from behavior that still needs runtime or representative-user evidence.

## Load the Standard

1. Read `CLAUDE.md`.
2. Read every `specs/standards/ui/*.spec.md` file completely.
3. Inspect the relevant product specs, UI architecture, routes, components, design tokens, content, tests, and existing patterns.
4. Use the requirement IDs from the specs in every finding and acceptance criterion.

Do not substitute remembered best practices for the repository standard. Follow the linked normative source when an exact WCAG exception or interpretation determines the result.

## Select a Mode

- Use **Audit mode** for a diff, PR, implementation, mockup, existing flow, or request to review/check/audit.
- Use **Advisory mode** for an intent, feature idea, user need, proposed flow, or request for suggestions.
- If the request includes both, recommend the intended experience first and then audit the supplied implementation against it.

## Ground the Work

Before either mode:

1. Identify the intended users, critical task, context, consequence of failure, supported environments, and evidence already available.
2. Derive missing facts from the repository before asking the user.
3. State assumptions that materially affect the result. Ask only when an unanswered choice would produce a substantially different design or audit scope.
4. Cover default, loading, empty, partial, success, validation, error, permission, offline, timeout, destructive, cancellation, and recovery states where applicable.

## Audit Mode

### 1. Establish the change and evaluation scope

- Prefer an explicitly supplied diff, PR, file list, route, or flow.
- Otherwise inspect committed branch changes against the merge base plus staged, unstaged, and relevant untracked files.
- Trace changed components into the composed page and complete journey. Inspect enough unchanged code to evaluate state, semantics, responsive behavior, and downstream effects.
- Record what cannot be evaluated from source alone, such as rendered contrast, focus behavior, assistive-technology output, field performance, or user comprehension.

### 2. Build the applicable requirement set

- Enumerate every requirement heading in `specs/standards/ui/`.
- Mark each `PASS`, `FAIL`, `PARTIAL`, `NOT_TESTED`, `BLOCKED`, or justified `N/A`.
- Audit every applicable WCAG 2.2 A/AA criterion in `accessibility.spec.md`; do not infer full conformance from a scanner or design-system component.
- Treat `SHALL` as a release requirement and `SHOULD` as the default requiring evidence for departure.
- Never use a percentage score that can hide a standards, critical-task, or segment failure.

### 3. Gather evidence

- Cite `file:line`, route/state, rendered artifact, DOM/accessibility-tree result, command/test output, or research evidence for each conclusion.
- Run safe, relevant existing checks when available: type/lint/unit/component/E2E tests, accessibility rules, keyboard tests, responsive/zoom checks, and performance tools.
- Preserve tool name/version and distinguish machine-detectable results from manual judgment.
- Do not invent user evidence. Mark outcome and comprehension requirements `NOT_TESTED` when no valid study or field evidence exists.

### 4. Report findings first

Order findings by `Critical`, `High`, `Medium`, then `Low`. For each finding include:

- requirement ID and concise title;
- location and affected state/journey;
- observed evidence, not preference;
- user impact and affected users;
- precise fix direction;
- retest method;
- confidence (`High`, `Medium`, or `Low`).

Then include:

1. an overall decision: `PASS`, `COMMENT`, or `REQUEST_CHANGES`;
2. a compact requirement coverage table with every applicable failure, partial, blocked, and not-tested item visible;
3. tests/checks run and their results;
4. scope limitations and residual risks.

Do not manufacture findings. If no issue is supported, say so and report remaining untested risk.

## Advisory Mode

### 1. Translate intent into an experience contract

State the intended users, critical outcome, entry and exit, context, constraints, risks, and measurable acceptance criteria. Distinguish facts from assumptions.

### 2. Recommend the smallest complete experience

Describe:

- information architecture and primary flow;
- page/view purpose, hierarchy, content, and actions;
- component behavior and state transitions;
- prevention, validation, recovery, undo, and support;
- responsive, keyboard, screen-reader, zoom, touch, reduced-motion, and locale behavior;
- Red Hat palette, semantic color, contrast, and visual hierarchy;
- privacy, permissions, commitment, cancellation, and data handling;
- loading, latency, offline, partial-failure, and monitoring behavior.

Prefer established repository components and web conventions. Introduce a new pattern only for a documented unmet need.

### 3. Make the recommendation verifiable

End with:

1. a flow/state outline;
2. a standards trace mapping decisions to requirement IDs;
3. deterministic acceptance checks;
4. representative-user tasks and predeclared outcome measures;
5. open decisions and risks.

Do not present a heuristic, five-user study, automated score, visual mockup, or component-library claim as proof that the proposed experience works.

## Boundary with General Code Review

Keep this review focused on user outcomes, accessibility, Red Hat brand/color, interaction, content, trust, localization, resilience, and UI evidence. For a mixed change, apply `ui-standards` to the interface and the repository's `amber-review` skill to general code, architecture, and security concerns without duplicating findings.
