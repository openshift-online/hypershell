# Page mockup story structure

Create two Storybook surfaces from one page-owned surface component.

## Three ownership layers

1. **Application shell:** masthead, product branding, global navigation, user
   menu, theme control, skip link, and outer PatternFly `Page`. Exclude this
   layer from page parity.
2. **Page surface:** visible page title and description, page-owned breadcrumbs
   or actions, header `PageSection`, main `PageSection`, content background,
   page padding, width constraints, and responsive page layout. Always include
   this layer in page parity.
3. **Page content:** cards, tables, forms, status, and other feature content.
   Include this layer in page parity.

Shell-free means “without the application shell.” It never means “content
only.”

## Required structure

```text
<page>/<page>-surface.tsx          page frame plus page content
<page>/<page>-fixtures.ts          deterministic states shared by both stories
<page>/<page>.stories.tsx          full-shell design review: Mockups/...
<page>/<page>.parity.stories.tsx   shell-free comparison: Parity/...
```

Equivalent filenames are allowed when the ownership boundary remains explicit.

## Page-owned surface component

Include UI owned by the production page or component:

- visible page title and description;
- page header and main `PageSection` composition;
- page-owned padding, background treatment, width constraints, and responsive
  behavior;
- cards, tables, forms, controls, page-local navigation, and all reachable
  states.

Exclude shared application chrome:

- masthead, product logo, global navigation, user menu, theme control;
- application-wide skip link and outer `Page` shell;
- breadcrumbs supplied by a shared route shell.

When ownership is ambiguous, inspect the production composition boundary and
record the decision in the owning spec or mockup source. Do not duplicate shell
chrome inside the page-owned component.

### Required full-page region contract

Use this structure for every full-page mockup unless the owning spec explicitly
defines different PatternFly variants:

```tsx
export function ExamplePageSurface() {
  return (
    <>
      <PageSection data-page-region="header" variant="default">
        {/* visible title, description, and page actions */}
      </PageSection>
      <PageSection data-page-region="body" variant="secondary" isFilled>
        {/* feature content */}
      </PageSection>
    </>
  );
}
```

- Keep both regions inside `<Name>PageSurface`.
- Treat `data-page-region` as a parity contract marker, not a styling hook.
- Put the header background, body background, padding, and fill behavior on
  these `PageSection`s. Do not move them into a story decorator, shell, route,
  or content card.
- Use `variant="default"` for the white header and `variant="secondary"`
  plus `isFilled` for the gray body by default.
- Record any different region count or variant in the owning spec before
  implementation.

## Design-review stories

- Use title `Mockups/<area>/<page>`.
- Compose the page surface inside the shell-only `MockupShell`. Do not use a
  wrapper that separately injects the page title, page sections, or background.
- Expose every materially different state.
- Use these stories for stakeholder review only.

## Parity stories

- Use title `Parity/<area>/<page>`.
- Render the exported `<Name>PageSurface` without `MockupShell` or other shared
  application shell.
- Wrap the surface in `MockupParityFrame`; the frame supplies only the outer
  shell-free PatternFly `Page` and must not supply page regions or styling.
- Do not render a bare `<Name>Content` component as the parity root.
- Supply the same width constraints, content background, theme, locale, and
  deterministic fixture used by the corresponding production story.
- Expose the same state names as the design-review and production stories.

Production stories use title `Production/<area>/<page>` and render real
production page surfaces, including the visible title and page-owned
background. The visual gate compares only `Parity/...` with
`Production/...`. Test shared shell chrome through dedicated shell stories when
the shell changes.
