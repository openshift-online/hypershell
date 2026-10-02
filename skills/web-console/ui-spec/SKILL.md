---
name: web-console-ui-spec
description: >
  Design HyperShell web-console UI from a product spec, including a standalone
  mockup and Storybook preview, using existing flows, UI standards, PatternFly,
  and repository components.
---

# Web-console UI specification and preview

Use this skill when a web-console spec needs a UI design, mockup, or preview.
It complements `skills/plan/spec`; it does not replace the product contract.

## Before design or implementation

Read:

- `CLAUDE.md`
- `skills/build/patternfly/SKILL.md`
- `skills/review/ui-standards/SKILL.md`
- `skills/plan/spec/SKILL.md` when the task includes spec work
- `skills/web-console/ui-spec/ui-conventions.md`

Inspect:

- the relevant section of `specs/web-console/user_flows.md`;
- relevant files under `specs/web-console/personas/`;
- related `specs/web-console/*.spec.md` files;
- current routes, composition roots, components, messages, tests, and stories;
- canonical components in `packages/gateway-management-ui/` and
  `packages/operational-dashboard-ui/`;
- existing terminology, statuses, colors, icons, layouts, and interactions by
  rendered purpose.

Resolve the intended user, task, entry, exit, constraints, known facts, and
open decisions. Ask one concise question at a time when an unresolved choice
materially changes scope, interaction, authorization, accessibility, or
terminology. Do not ask for information already established by the repository.
If flow and persona conflict, surface the conflict.

## Design rules

- Design only the information, controls, navigation, and states needed for the
  user's task and next decision. Do not add speculative or decorative UI.
- Verify page entry, primary action, and exit against the user flow. Include the
  navigation needed to complete the flow.
- Use this reuse order: existing HyperShell component; PatternFly 6 component
  or documented pattern; composition of PatternFly primitives; new shared
  component only when repository search shows a real gap.
- Use existing terminology, status vocabulary, semantic colors, icons, spacing,
  responsive behavior, navigation, and destructive-action patterns.
- Use PatternFly tokens/utilities before CSS Modules. Do not use Tailwind,
  Bootstrap, MUI, Sass, CSS-in-JS, or ad hoc colors.
- Keep domain logic, SDK imports, telemetry, infrastructure, and live services
  out of mockups. Use deterministic static data.
- Design all reachable states: loading, empty, success, validation, error,
  partial failure, permission, recovery, and destructive states as applicable.

## Mockups

- Store page mockups in `specs/web-console/mockups/`, separate from production
  code. Start from the shared mockup template and reuse shared mockup elements.
- Add a colocated `*.stories.tsx` using the repository's Storybook conventions.
- Stories must show the default state and materially different states, use
  deterministic data, and test the primary interaction when practical.
- Keep mockup code independent of production page implementations.
- After production implementation, compare the browser page with the mockup.
  Resolve differences deliberately by updating the correct artifact.

## Feedback and visual references

Before editing a mockup in response to user feedback or a visual reference:

1. Check the request against PatternFly guidance, UI standards, and
   `ui-conventions.md`.
2. Check existing mockups and shared components for the same pattern, including
   layout, status treatment, actions, icons, labels, and responsive behavior.
3. If the request deviates, warn the user, explain the mismatch, and offer a
   convention-aligned alternative. Ask before proceeding when the deviation
   changes a material product or interaction decision.

After an approved change:

- update the mockup and stories;
- update the owning spec and `specs/web-console/user_flows.md` when requirements,
  terminology, actors, or states change;
- reuse or extract a shared component when the pattern is reusable;
- update `ui-conventions.md` only when the decision establishes a durable
  product-wide convention.

Do not silently override established conventions with a visual reference or
user preference.

## Production handoff

Production UI belongs under `components/web-console/app/features/` unless
repository evidence supports a canonical package. Production code may refine
mockup structure, component choices, responsiveness, accessibility,
localization, and application integration. The resulting browser experience
must remain recognizably consistent with the approved mockup. Mockup PatternFly
choices are recommendations, not code to copy verbatim; production code must
follow repository UI standards and technical boundaries.

Report changed files, reused or newly shared components, available states,
unresolved decisions, and checks run. Do not report completion while a major
product decision remains open or the preview cannot build.
