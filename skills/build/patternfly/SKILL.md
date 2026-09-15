---
name: patternfly
description: >
  Implement UI with PatternFly 6 using HyperShell conventions. Use when
  building or changing web-console, gateway-management-ui, or
  operational-dashboard-ui screens. Not for UI audits - use ui-standards.
---

# PatternFly Implementation

Practical guidance for building UI with PatternFly 6. Normative requirements live in `specs/standards/ui/patternfly.spec.md`; audits use `skills/review/ui-standards`.

## Research Order

When investigating how to implement something, follow this order:

1. **In-repo docs** - `specs/standards/ui/`, canonical implementations in `packages/gateway-management-ui/`, `packages/operational-dashboard-ui/`, and `components/web-console/app/`
2. **PatternFly MCP** - `user-patternfly-docs` for component APIs, props, patterns, and accessibility notes
3. **PatternFly website** - https://www.patternfly.org when MCP is insufficient or not available
4. **Other** - only after the above (team conventions, issue history, external references)

Search by purpose and rendered result, not filename alone.

## Component Selection

Use PatternFly design guidelines to pick the right building block:

- **Components** - base interactive and layout primitives
- **Patterns** - composed solutions for common flows (forms, tables, wizards, etc.)
- **Extensions** - PatternFly ecosystem add-ons where applicable
- **Component groups** - higher-level compositions for complex data display and navigation

Follow reuse order `UI-PF-05`: existing HyperShell shared component, then applicable PatternFly component or documented pattern, then composition of PatternFly primitives, then one new shared component with admission evidence.

Do not build custom markup when a PatternFly component, pattern, or composition covers the need (`UI-PF-01`).

## Styling

Use PatternFly design tokens and utility classes instead of custom CSS when they cover the need (`UI-PF-03`):

- Semantic tokens: `--pf-t--global--*`; component tokens: `--pf-v6-c--*`
- PatternFly layout, spacing, and typography utilities before writing new rules
- CSS Modules only for product-specific layout gaps tokens do not cover

No Tailwind, Bootstrap, MUI, Sass, or CSS-in-JS.

## Accessibility

PatternFly defaults are a starting point, not sufficient for WCAG compliance (`UI-PF-09`).

- Provide explicit `aria-label`, `aria-labelledby`, or `aria-describedby` when defaults are vague or missing
- Icon-only controls always need a localized `aria-label` via `react-intl`
- Do not rely on PatternFly's built-in label text without verifying it matches the context
- Verify each component exposes the correct ARIA `role` for its purpose. Check the component's accessibility docs (PatternFly MCP) for what PatternFly sets by default; if the role is missing or wrong for the use case, set it explicitly so assistive technologies understand the object's purpose
- `Alert` used as a warning or error: confirm `role="alert"` is present. Add it when PatternFly does not set it or the default role does not match intent. For dynamically appearing toast alerts, use `AlertGroup` with `isLiveRegion` per PatternFly guidance

## Boundaries

| Need | Use instead |
|------|-------------|
| Normative requirements (`SHALL`, `UI-PF-*`) | `specs/standards/ui/patternfly.spec.md` |
| PR audit, acceptance criteria | `skills/review/ui-standards` |
| Application/domain logic | No PatternFly imports (`hexagonal-architecture.spec.md`) |
