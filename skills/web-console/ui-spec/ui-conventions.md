# HyperShell UI Conventions

These conventions define product-specific UI rules that go beyond or clarify
PatternFly guidance. Apply these conventions whenever creating or modifying a
web-console mockup or shared component.

## Terminology

- Use `gateway` when describing the gateway that connects to OpenShell.
- Use a lowercase `g` in `gateway` unless it begins a sentence or is a label.
- Always write `Red Hat` with both words capitalized and separated by a space.
  Do not use `RH`, `red hat`, or `RedHat`.
- Always write `HyperShell` as one word with a capital `H` and capital `S`.
- Always write `OpenShell` as one word with a capital `O` and capital `S`.
- Use `Red Hat Internal` only when referring to a data security level. For
  systems restricted to Red Hat employees or requiring VPN access, use
  `internal Red Hat` or `internal to Red Hat`, whichever fits the sentence.
  Capitalize `Internal` only when it begins a sentence or label.

## Alerts and errors

- Use the standard PatternFly danger alert when reporting an API error.

## Spacing

- Use PatternFly component defaults, layout props, utility classes, and design
  tokens first for spacing, typography, color, and sizing. Add custom CSS only
  when PatternFly has no suitable primitive.
- New page mockups start with no page-specific CSS module. Admit CSS only for a
  spec-backed capability gap after component props, layouts, responsive props,
  and utilities are exhausted.

## Page structure

- Keep the page header containing the title and optional refresh action in its
  own PatternFly `PageSection`, followed by a separate main-content section.
- Use breadcrumbs as the primary navigation method when a page moves deeper
  into a section. The parent breadcrumb should link back to its collection or
  parent page.
- Use the shared mockup shell and its PatternFly components for common page
  chrome. Duplicate the implementation into the mockup workspace only when
  production dependencies would make the mockup change automatically.
- Keep page-owned mockup content separate from shared shell chrome. Compose it
  inside the shared shell for `Mockups/...` design-review stories and render it
  without the shell for `Parity/...` implementation-comparison stories.
- Do not add masthead, product branding, global navigation, user controls, or
  the outer application `Page` to a page-owned parity component.
- Use PatternFly layouts, collections, content, status, and action components.
  Do not recreate them with styled native elements.

## Tables and actions

- Use the default PatternFly table spacing; do not use a condensed table unless
  the product design explicitly requires it.
- Make applicable table columns sortable. Show an active sort direction only
  on the column currently sorted.
- Put row actions in an unlabeled final column using the PatternFly action-cell
  and kebab-menu pattern.
- Mark external row actions with PatternFly's `isExternalLink` treatment and
  use the external destination URL.

## Status and shared actions

- Show a healthy status with the green PatternFly check-circle treatment beside
  the `Healthy` label.
- Use the green check-circle treatment for a good or clear state, the amber
  warning-triangle treatment for a warning state, and the red error-circle
  treatment for a danger state.
- For summary headings such as Needs attention, derive the icon from the
  highest-severity item: danger takes precedence over warning, and warning
  takes precedence over good.
- When a card presents a status summary, the card header SHALL include the
  corresponding semantic status icon; the card body MAY provide the detailed
  status text and affected resources.
- Reuse shared mockup components for recurring actions such as
  `Provision gateway` instead of recreating their markup in each page.

## Maintaining the conventions

When a design decision should remain consistent across mockups, add it here
instead of encoding it only in a page-specific component. Include recurring
rules for terminology, alert severity, spacing, capitalization, color, and
other UI treatments.
