# HyperShell agent rules

## Mandatory web-console completion gate

For every new or materially changed production page with an approved mockup,
read and execute `skills/build/ui-build-gate/SKILL.md`. Do not report or hand
off the page until the gate returns `Complete`. Treat pending stories,
fixtures, captures, inspection, or mismatches as implementation work.

When a build or reconcile task changes UI and no corresponding mockup exists,
warn the user and continue. Do not create a mockup unless the task includes
mockup or spec work. Skip `ui-build-gate` and all mockup-to-production parity
checks. Continue running all non-parity checks required by the task.

## Spec-to-mockup requirement

When a feature spec describes a new or materially changed visual web-console
page and includes an approved screenshot or visual reference:

1. Create or update the corresponding repository mockup under
   `specs/web-console/mockups/`.
2. Treat the mockup as part of the spec deliverable, not optional later
   implementation work.
3. Add or update the mockup Storybook story and required styles.
4. Run `check_mockup_patternfly.py` for every changed mockup TSX/CSS file.
5. Reference the mockup story and source visual reference in the feature spec.
6. Do not finalize the spec until the mockup exists and the mockup checks pass.

When the condition above applies, the deliverable requires both artifacts:

- the written feature spec
- the repository mockup and Storybook design-review story

## Repository conventions

Read `CLAUDE.md` before implementation and follow its repository commands and
critical conventions.
