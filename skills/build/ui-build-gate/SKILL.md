---
name: ui-build-gate
description: >
  Executes and enforces Storybook screenshot parity for new or materially
  changed HyperShell web-console UI with an approved mockup. Produces required
  evidence; use before reporting UI implementation complete.
---

# UI build gate

Own Storybook parity completion. Calling skills must stop their completion path
until this gate returns `Complete` or `Blocked`.

## Terminal states

The task has only two terminal states:

- `Complete`: `visual-parity.json` validates with `status: complete`.
- `Blocked`: an external failure prevents capture or inspection after safe
  recovery attempts. Record the attempted recovery and external cause.

Never return `Implemented, incomplete`. Missing code, providers, fixtures,
stories, screenshots, or review is implementation work, not a blocker.

## Required loop

Advance in order. Do not skip a state or report a later state without evidence
from the earlier one.

### 1. Inventory

Identify equivalent `Production/...` and approved `Parity/...` story ids and
states. Do not use the full-shell `Mockups/...` design-review story. Use
deterministic fixtures, matching providers, the same theme, locale, parity
frame, and desktop `1440x900` plus narrow `390x844` viewports unless the spec
requires other dimensions.

If the approved mockup has only a full-shell story, split it according to
`skills/web-console/ui-spec/references/mockup-story-structure.md` before
implementing or capturing production. Do not recreate mock shell chrome in the
production component.

Both parity stories MUST render the complete page-owned surface: visible title,
description, header section, main section, owned background, padding, and
feature content. Reject a parity story that renders only inner content. Shared
masthead/global chrome remains excluded.

For full pages, both real page-surface source files MUST contain the same
explicit region contract:

- `PageSection data-page-region="header" variant="default"` owns the visible
  title, description, and page actions.
- `PageSection data-page-region="body" variant="secondary" isFilled` owns the
  content and gray filled background.

The owning spec may define different variants. In that case, update both
surfaces to the spec and pass the documented contract to the gate; never move
background ownership into the Storybook wrapper or application shell.

Create or update the production story before continuing. It MUST import and
render the real production component. Do not use a static copy of the mockup,
live API dependency, permanently loading state, or default-state-only coverage
when the implementation has other materially different states.

If the production component reads session, API, router-loader, query, or other
runtime data, read
[`references/deterministic-story-harness.md`](references/deterministic-story-harness.md)
and implement a deterministic harness before capture. Mocking these boundaries
inside Storybook is required implementation work. Never report live data as a
reason the gate could not run.

Before capture, run `scripts/check_production_data_boundary.py` for the runtime
route, production view, and production story. Pass every real hook/adapter from
the visible-data source table as `--required-source`. Do not capture until it
passes.

### 2. Build and capture

Run from the repository root:

```bash
python3 skills/build/ui-build-gate/scripts/visual_parity_gate.py capture \
  --production-story <story-id> \
  --mockup-story <story-id> \
  --route <runtime-route.tsx> \
  --view <production-view.tsx> \
  --story-file <production-story.stories.tsx> \
  --mockup-story-file <mockup.parity.stories.tsx> \
  --mockup-view <mockup-page-surface.tsx> \
  --required-source <real-hook-or-adapter> \
  --output <task-evidence-directory>
```

The default region contract is a `default` header and filled `secondary` body.
When the owning spec explicitly defines another treatment, pass
`--header-variant`, `--body-variant`, or `--body-fill unfilled` with the
documented values. Apply identical values to mockup and production surfaces.

Repeat `--required-source` for every dynamic source in the visible-data source
table. Capture runs the production/story data-boundary checker first and cannot
continue when runtime code contains fixtures, scenario selectors, or missing
real sources.

This command validates that both page surfaces own marked header/body regions,
that the parity story renders `<Name>PageSurface` without shell chrome, builds
both Storybooks, serves their static output, captures the four required images,
and writes `visual-parity.json`. Fix any
validation, build, render, provider, fixture, blank, error, loading, or
live-network failure and rerun the command. Capture rejects requests outside
Storybook's static assets.

### 3. Inspect

Call `view_image` on all four paths from `visual-parity.json` in the current
task. Inspect production desktop vs mockup desktop and production narrow vs
mockup narrow.

Compare layout, page-owned chrome, background colors, section order, content type and
placement, typography hierarchy, controls, icons, status treatment, spacing,
page-local navigation, accessibility affordances, and responsive stacking.
Exclude the shared
masthead, global navigation, product branding, user menu, and application shell
unless the implementation task changes the shell itself.

Do not infer visual parity from source code, filenames, file existence, a
successful screenshot command, or a passing build.

### 4. Resolve or attest

If either pair differs without a spec-backed reason, change production code,
fixtures, or story harness and repeat Build and capture + Inspect. Do not edit
the approved mockup to make production pass unless the owning spec is also
being intentionally revised with user authorization.

Equivalent fixture values may differ when content type, hierarchy, placement,
and visual treatment match.

After personally opening all four images, record the result:

```bash
python3 skills/build/ui-build-gate/scripts/visual_parity_gate.py review \
  --manifest <task-evidence-directory>/visual-parity.json \
  --result pass \
  --notes "<summary; include spec references for intentional differences>"
```

Use `--result fail` when parity is not met. Add one `--difference` for every
visible mismatch using this exact format:

```text
--difference "<desktop|narrow|both>::<element>::<mockup appearance>::<production appearance>::<production file to edit>"
```

Record differences in background, component type, icon/status treatment,
spacing, typography, sectioning, placement, and responsive behavior
separately. Never record `pass` before all four `view_image` calls.

`review --result fail` requires a nonempty difference ledger and exits nonzero.
Edit every listed production file immediately. Do not send a final response,
summarize, hand off, or run final validation. Rerun capture, open all four new
images, and review again. The manifest retains failed ledgers across attempts.
The next capture is rejected unless a production, story, fixture, or owning-spec
file changed after the failed comparison.

### 5. Validate completion

```bash
python3 skills/build/ui-build-gate/scripts/visual_parity_gate.py validate \
  --manifest <task-evidence-directory>/visual-parity.json
```

Do not report UI work or its parent wave complete unless this exits zero. The
final handoff must link the production story, mockup story, manifest, all four
captures, viewport sizes, review result, fixes made, and spec-backed intentional
differences.

Do not send a final response containing unresolved parity language such as
"still needs", "needs one iteration", "remaining visual difference", "close
enough", or "parity pending". Such a finding requires another implementation
iteration in the current task. Unrelated typecheck, lint, or SDK failures do not
authorize skipping or stopping this visual loop.
