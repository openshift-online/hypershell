# PatternFly 6 and Component Reuse Standard

**Status:** Active
**Applies to:** Every HyperShell web-console dependency, component, page, prototype intended for implementation, and product-specific UI extension

## Purpose

Use PatternFly 6 as HyperShell's default implementation system and maintain one canonical implementation for each reusable interface concept. PatternFly does not replace the other UI standards or establish accessibility and usability compliance by itself.

## Requirements

### Requirement: UIS-PFLY-01 -- PatternFly 6 Default

The web console SHALL use PatternFly 6 components, patterns, layouts, icons, typography, and design tokens whenever they support the intended need. It SHALL NOT introduce another general-purpose design system. Native semantic HTML MAY be used when it is the simpler correct primitive or PatternFly has no applicable abstraction.

**Verification:** Map each implemented interaction to a PatternFly 6 component/pattern, a native semantic primitive, or an approved custom-component decision. Inspect rendered class names, imports, dependencies, and design artifacts for parallel design systems.

### Requirement: UIS-PFLY-02 -- Version Consistency

All direct PatternFly dependencies SHALL use major version 6 and compatible releases. The repository SHALL pin resolved dependency versions, SHALL NOT mix PatternFly major-version class names or packages, and SHALL review upgrades as deliberate dependency changes.

**Verification:** Inspect manifests and lockfiles for `@patternfly/*` packages, resolved versions, duplicate installations, and legacy `.pf-v5-*` or earlier classes. Run the supported build and component tests after an upgrade.

### Requirement: UIS-PFLY-03 -- Tokens and Red Hat Brand Mapping

Components SHALL use PatternFly 6 semantic tokens rather than hard-coded values or a parallel spacing, typography, color, elevation, breakpoint, or motion system. Product-level brand mappings SHALL conform to `brand-color.spec.md`, including Red Hat red and reserved information-color meanings.

**Verification:** Trace rendered styles to PatternFly or approved HyperShell semantic tokens. Flag unexplained literal colors/dimensions, copied PatternFly CSS, token aliases with conflicting meaning, and brand mappings that fail contrast or state semantics.

### Requirement: UIS-PFLY-04 -- Canonical Reusable Components

Each repeated UI concept or behavior SHALL have one canonical implementation. Generic presentation, interaction, and domain components SHALL live in the shared component surface and expose a documented reusable API. Route-specific code MAY own orchestration and composition but SHALL NOT duplicate reusable behavior or styling.

**Verification:** Inventory component definitions and imports by purpose, semantics, state model, visible result, and styling-not filename alone. Confirm pages import the canonical component and that a second implementation of the same concept is absent or backed by a decision explaining the material semantic difference.

### Requirement: UIS-PFLY-05 -- Reuse Before Creation

Before creating a component, contributors SHALL use this selection order:

1. reuse an existing HyperShell shared component;
2. use the applicable PatternFly 6 component or documented pattern directly;
3. compose PatternFly 6 components or native semantic primitives;
4. create one shared custom component for a documented unmet need.

Variants SHALL use props, slots, composition, and semantic tokens rather than copied markup, styles, or forks. Copy-pasted and near-duplicate components are prohibited.

**Verification:** Require repository and PatternFly catalog search evidence with each new component. Compare similar names, rendered output, ARIA roles, event behavior, CSS, tests, and state models; flag a second implementation even when renamed.

### Requirement: UIS-PFLY-06 -- Custom Component Admission

A custom component SHALL exist only when no existing HyperShell or PatternFly component/composition meets the evidenced need. Its decision record SHALL identify the gap, alternatives, why extension or composition is insufficient, intended consumers, reusable API, accessibility contract, and retirement or upstream-contribution path.

**Verification:** Review the decision record and prototype against alternatives. Confirm the component is shared at the narrowest stable boundary and is not a speculative abstraction without a real consumer.

### Requirement: UIS-PFLY-07 -- Wrapper Discipline

A wrapper around a PatternFly component SHALL add stable cross-product behavior, domain semantics, accessibility policy, brand policy, or a materially simpler reusable API. Pass-through wrappers, cosmetic renames, and wrappers that conceal required PatternFly props or accessibility behavior are prohibited.

**Verification:** Compare wrapper API and behavior with the underlying component. Remove wrappers whose only effect is import indirection, fixed incidental layout, or copied defaults.

### Requirement: UIS-PFLY-08 -- Complete Reusable Contract

Every shared component SHALL document its purpose, consumers, props, composition boundaries, content rules, supported states, responsive behavior, localization constraints, and accessibility behavior. It SHALL cover applicable default, hover, focus, active, selected, expanded, disabled, loading, empty, success, warning, error, overflow, long-content, and destructive states.

**Verification:** Inspect discoverable examples and component tests. Exercise applicable states with realistic and pseudo-localized content, keyboard, screen reader, zoom/reflow, touch, reduced motion, light/dark themes, and failure conditions.

### Requirement: UIS-PFLY-09 -- PatternFly Usage and Accessibility Guidance

Implementations SHALL follow the current PatternFly 6 design and accessibility guidance for each selected component, including required labels, landmarks, focus behavior, keyboard interaction, status text, and consumer-supplied accessibility props. PatternFly use SHALL NOT be reported as WCAG or journey compliance.

**Verification:** Trace each component to its PatternFly guidance and test the composed page against `accessibility.spec.md`. Record consumer responsibilities and every customization that changes documented behavior.

### Requirement: UIS-PFLY-10 -- Stable Components and Lifecycle

Production flows SHOULD use stable PatternFly components. Beta/deprecated components SHALL require a documented risk or migration plan. HyperShell shared components SHALL have an owner, version/change policy, deprecation path, consumer migration plan, and regression coverage.

**Verification:** Inspect dependency release notes, beta/deprecation markers, component ownership, usage search, migration status, and removal of obsolete implementations after consumers move.

### Requirement: UIS-PFLY-11 -- In-Browser Code Editor Component

When a feature needs an in-browser code or policy editor (for example the Cedar policy
editor, `web-console/platform-policy-admin.spec.md` WEB-PADM-05), the web console SHALL
use the PatternFly code editor (`@patternfly/react-code-editor`) as the canonical
editor implementation, installed only when a feature consumes it (UIS-PFLY-01,
UIS-PFLY-02) and admitted as a specialized widget with a documented decision and an
accessibility and bundle assessment (UIS-PFLY-06). There SHALL be one shared editor
wrapper reused across features rather than a second editor implementation (UIS-PFLY-04,
UIS-PFLY-05). The editor SHALL render React elements only and SHALL NOT render
author-supplied content or diagnostics through any raw-HTML injection path, consistent
with the browser security rule. Editor usage SHALL follow the stable-component
lifecycle (UIS-PFLY-10).

**Verification:** Confirm a single `@patternfly/react-code-editor`-based wrapper is the
only in-browser editor, pinned to PatternFly 6, consumed only by features that need it,
and backed by an admission decision with accessibility and bundle notes. Inspect the
wrapper and its consumers for raw-HTML injection of editor content or diagnostics, and
for duplicate editor implementations.

## Primary Basis

- [Why Red Hat uses PatternFly](https://www.patternfly.org/get-started/about-patternfly/)
- [PatternFly 6 design kit and tokens](https://www.patternfly.org/get-started/design/)
- [Develop with PatternFly 6](https://www.patternfly.org/get-started/develop/)
- [PatternFly component accessibility example](https://www.patternfly.org/components/page/accessibility)
