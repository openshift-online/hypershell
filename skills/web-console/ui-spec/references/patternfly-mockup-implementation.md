# PatternFly mockup implementation

Build implementation-intent mockups from PatternFly 6 and existing HyperShell
components. A visually similar custom recreation is a failed mockup.

## Required selection pass

Before JSX edits, map every visible purpose to one selection:

1. existing HyperShell shared component;
2. PatternFly documented pattern;
3. PatternFly component, layout, or utility;
4. native semantic element only when PatternFly has no applicable abstraction;
5. custom component/CSS only with a spec-backed PatternFly-gap decision.

Search repository implementations by rendered purpose. Then inspect the
PatternFly documentation for the exact component API, composition, responsive
props, and accessibility requirements. Do not select a component from memory
when documentation is available.

Record the mapping in task commentary before implementation. Include the
PatternFly pattern or component name for every page section, collection,
status, action, form control, navigation element, and responsive layout.

## Common mappings

| Purpose | Prefer |
|---|---|
| Page regions | `PageSection`, `Content`, `Title` |
| Responsive layout | `Grid`, `Gallery`, `Flex`, `Stack`, layout props, utilities |
| Grouped surface | `Card`, `CardHeader`, `CardTitle`, `CardBody` |
| Simple textual list | `List`, `ListItem` |
| Structured resource rows | `DataList` or `Table` pattern |
| Status | `Label`, `Alert`, `HelperText`, shared status component |
| Actions and links styled as actions | `Button` with the appropriate variant/component |
| Metrics and term/value data | `DescriptionList`, `Grid`, existing dashboard component |
| Empty/loading/error | PatternFly empty-state, skeleton/progress, and alert patterns |

Do not recreate these with styled `div`, `span`, `ul`, `li`, `a`, or native
form controls.

For full pages, the parity root is the page surface, not an inner content grid.
Use `PageSection data-page-region="header" variant="default"` for the visible
header and `PageSection data-page-region="body" variant="secondary" isFilled`
for the main region. Preserve the approved variant or background treatment in
the surface component. Put neither region in a story decorator or shell.

## CSS admission

Start with no page-specific CSS. Use this order:

1. component props and variants;
2. PatternFly layouts and responsive props;
3. PatternFly utility classes;
4. PatternFly semantic tokens for a product-specific gap.

Do not apply a utility when the selected component already has the required
prop, variant, or semantic size. For every utility class:

1. verify the exact class exists in the installed `@patternfly/react-styles`
   version;
2. verify the computed style in the rendered story.

`@patternfly/react-core/dist/styles/base.css` does not load the PatternFly
utility stylesheets. HyperShell intentionally imports
`@patternfly/react-styles/css/utilities/_index.css` in the application,
production Storybook, and mockup Storybook. PatternFly 6 utility classes are
available in all three surfaces. Do not replace an available utility with
page-specific CSS.

Do not use CSS for layout, spacing, typography, color, status treatment,
breakpoints, or behavior already available from PatternFly.

When a real gap remains, add this comment at the top of the CSS module:

```css
/* patternfly-gap: specs/<owning-spec>.md - <missing capability and rejected alternatives> */
```

The reference must identify the owning spec decision. A comment without a real
gap is invalid. Never use literal colors or copy PatternFly component CSS.

## Completion

Run `scripts/check_mockup_patternfly.py` on every changed mockup TSX/CSS file.
Treat checker findings, undocumented custom markup, and undocumented CSS as
implementation failures. Continue until the checker and Storybook build pass.

Reopen the rendered story and verify each selection against its PatternFly
documentation: intended purpose, required composition, content rules,
responsive behavior, and accessibility props. The checker cannot determine
whether a valid PatternFly component is the correct component. Fix semantic
mis-selections before handoff.

The mockup handoff must include the component/pattern mapping, any admitted CSS
gap and spec reference, checker result, and Storybook build result. Do not
report the mockup complete when this evidence is absent.
