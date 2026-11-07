# HyperShell UI Conventions

These conventions define product-specific UI rules that go beyond or clarify
PatternFly guidance. Apply these conventions whenever creating or modifying a
web-console mockup or shared component.

## Terminology

- Use `gateway` when describing the gateway that connects to OpenShell.
- Use a lowercase `g` in `gateway` unless it begins a sentence or is a label.
- Use `Provision gateway` instead of `Add gateway` for the action that creates
  a gateway.
- Always write `Red Hat` with both words capitalized and separated by a space.
  Do not use `RH`, `red hat`, or `RedHat`.
- Always write `HyperShell` as one word with a capital `H` and capital `S`.
- Always write `OpenShell` as one word with a capital `O` and capital `S`.
- Use `Red Hat Internal` only when referring to a data security level. For
  systems restricted to Red Hat employees or requiring VPN access, use
  `internal Red Hat` or `internal to Red Hat`, whichever fits the sentence.
  Capitalize `Internal` only when it begins a sentence or label.

## Dates

- Use `MMM dd, yyyy` for English date-only values, with a zero-padded two-digit
  day, for example `Oct 05, 2026`.
- When a value includes a time, keep the same date format and append the
  localized time, for example `Oct 05, 2026, 9:35 PM`. Consequential
  timestamps must also include the required time-zone context.

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
- For pages with multiple distinct content sections, such as a home or landing
  page, group each section's content inside PatternFly cards and use the
  PatternFly secondary main-section treatment. In light mode this renders a
  gray page background with white card surfaces.
- For pages with a single primary content type, such as one table, form, list,
  or detail view, use the default white main-section treatment. Do not place
  gray-backed content directly on a gray page background or add cards solely to
  compensate for a secondary page background.
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
- When a table requires pagination, render PatternFly pagination controls both
  above and below the table. The two controls must share the same pagination
  state and page-size options.
- Enable sorting on every table column unless sorting is not technically
  possible or the data set is very small. Show an active sort direction only on
  the column currently sorted.
- Put row actions in an unlabeled final column using the PatternFly action-cell
  and kebab-menu pattern.
- Mark external row actions with PatternFly's `isExternalLink` treatment and
  use the external destination URL.
- Use the PatternFly deletion pattern when a modal confirms that something will
  be deleted or permanently disabled with no easy recovery. Place a warning
  icon to the left of the modal title and give the confirming primary action
  the PatternFly danger treatment.

## Status and shared actions

- Use the shared `HealthStatus` mockup component for recurring health and
  status treatments. Pass an explicit semantic appearance rather than
  choosing an icon in page-owned markup.
- Use a red circle with an exclamation mark (`ExclamationCircleIcon`) for
  danger, error, failed, or otherwise unhealthy states.
- Use a yellow triangle with an exclamation mark (`ExclamationTriangleIcon`)
  for warning, degraded, or otherwise impaired states.
- Use a green circle with a checkmark (`CheckCircleIcon`) for good, healthy,
  or clear states. The default label is `Healthy`.
- Keep the icon and label together as one status treatment. Do not use color
  alone to communicate status.
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
