# Red Hat Brand Color Standard

**Status:** Active
**Applies to:** Every Red Hat-branded HyperShell web interface, visualization, diagram, illustration, and generated visual asset

## Purpose

Make HyperShell unmistakably Red Hat while preserving accessibility and semantic clarity. Brand color supports hierarchy; it SHALL NOT override the accessibility, content, trust, or interaction standards.

## Requirements

### Requirement UI-BRAND-01: Red Hat Color Identity

A brand-bearing composition SHALL include Red Hat red (`red-50`, `#ee0000`) through a restrained accent, identifying element, or approved logo treatment. Red SHALL highlight what matters and SHALL NOT flood the interface or represent error, failure, decrease, or danger.

**Verification:** Inspect rendered pages, themes, exports, diagrams, and generated imagery. Confirm red is present, intentional, not dominant without an approved exception, and never used as a negative semantic signal.

### Requirement UI-BRAND-02: Approved Palette and Tokens

Interfaces SHALL use the approved colors below through named design tokens. Secondary colors SHALL accompany the core palette rather than replace it; a composition SHOULD use no more than one or two secondary colors. Interfaces SHALL NOT introduce colors outside this palette.

**Verification:** Resolve every rendered color to its semantic token and approved swatch. Flag hard-coded duplicates, off-palette colors, and secondary-only compositions.

### Requirement UI-BRAND-03: Semantic Information Colors

Information colors SHALL retain these meanings across UI, charts, diagrams, and reports:

| Family | Reserved meaning |
|---|---|
| `red` | Red Hat brand; never a negative state |
| `danger-orange` | Error, destructive failure, or decrease |
| `orange` | Caution or non-destructive problem |
| `yellow` | Warning requiring action to prevent harm |
| `success-green` | Success, increase, or positive completion |
| `teal` | General or neutral information/action |
| `interaction-blue` | Link, interactive object, or state change |
| `purple` | Informational note, tip, or help |
| `gray` | Null, unavailable, disabled, or intentionally de-emphasized |

Color SHALL NOT be the only carrier of any state or data category.

**Verification:** Compare semantic tokens and visible labels/icons/patterns across all states. Test monochrome and color-vision-deficiency simulations, and confirm screen-reader/state semantics agree with the visual meaning.

### Requirement UI-BRAND-04: Restraint, Balance, and Gradients

Layouts SHALL use generous neutral space and reserve saturated colors for hierarchy, navigation, interaction, or emphasis. Large surfaces SHOULD use light tints, dark shades, or an approved subtle gradient. Interfaces SHALL NOT invent gradients or make gradients the primary focus.

**Verification:** Audit color-area balance and attention direction at page and journey level. Resolve every gradient to a repository-approved Red Hat gradient token; if none exists, do not use a gradient.

### Requirement UI-BRAND-05: Accessible Color Application

All colors SHALL satisfy `UI-A11Y-05`: normally 4.5:1 for text, 3:1 for WCAG large text, and 3:1 for informative graphics and essential control/state boundaries, subject to WCAG exceptions. Text labels, icons, shapes, patterns, or position SHALL supplement color. Similar saturated colors SHALL NOT be combined where they create visual vibration or indistinct edges.

**Verification:** Measure rendered foreground/background combinations in every state and theme; do not infer contrast from palette membership. Review charts and status displays with color-vision simulation and without color.

### Requirement UI-BRAND-06: Visual Assets and People Colors

Illustrated skin tones SHALL use one approved cool- or warm-tone family per person and SHALL NOT be reused as decorative or semantic UI colors. Photography, illustration, 3D, and AI-generated imagery SHALL be evaluated and adjusted to the approved palette before use.

**Verification:** Inspect each asset's palette, semantic role, and people-color use. Reject unreviewed generated imagery and cross-family skin-tone construction without an approved illustration rationale.

## Approved Swatches

Use exact hex values. Choose lighter or darker steps according to hierarchy and measured contrast.

### Core palette

| Family | Tokens |
|---|---|
| Red | `red-05 #fef0f0`; `red-10 #fce3e3`; `red-20 #fbc5c5`; `red-30 #f9a8a8`; `red-40 #f56e6e`; `red-50 #ee0000`; `red-60 #a60000`; `red-70 #5f0000`; `red-80 #3f0000` |
| Neutral | `white #ffffff`; `gray-10 #f2f2f2`; `gray-20 #e0e0e0`; `gray-30 #c7c7c7`; `gray-40 #a3a3a3`; `gray-45 #8c8c8c`; `gray-50 #707070`; `gray-60 #4d4d4d`; `gray-70 #383838`; `gray-80 #292929`; `gray-90 #1f1f1f`; `gray-95 #151515`; `black #000000` |

### Secondary palette

| Family | Tokens |
|---|---|
| Orange | `orange-10 #ffe8cc`; `orange-20 #fccb8f`; `orange-30 #f8ae54`; `orange-40 #f5921b`; `orange-50 #ca6c0f`; `orange-60 #9e4a06`; `orange-70 #732e00`; `orange-80 #4d1f00` |
| Yellow | `yellow-10 #fff4cc`; `yellow-20 #ffe072`; `yellow-30 #ffcc17`; `yellow-40 #dca614`; `yellow-50 #b98412`; `yellow-60 #96640f`; `yellow-70 #73480b`; `yellow-80 #54330b` |
| Teal | `teal-10 #daf2f2`; `teal-20 #b9e5e5`; `teal-30 #9ad8d8`; `teal-40 #63bdbd`; `teal-50 #37a3a3`; `teal-60 #147878`; `teal-70 #004d4d`; `teal-80 #003333` |
| Purple | `purple-10 #ece6ff`; `purple-20 #d0c5f4`; `purple-30 #b6a6e9`; `purple-40 #876fd4`; `purple-50 #5e40be`; `purple-60 #3d2785`; `purple-70 #21134d`; `purple-80 #1b0d33` |

### Information palette

| Family | Tokens |
|---|---|
| Success green | `success-green-10 #e9f7df`; `success-green-20 #d1f1bb`; `success-green-30 #afdc8f`; `success-green-40 #87bb62`; `success-green-50 #63993d`; `success-green-60 #3d7317`; `success-green-70 #204d00`; `success-green-80 #183301` |
| Danger orange | `danger-orange-10 #ffe3d9`; `danger-orange-20 #fbbea8`; `danger-orange-30 #f89b78`; `danger-orange-40 #f4784a`; `danger-orange-50 #f0561d`; `danger-orange-60 #b1380b`; `danger-orange-70 #731f00`; `danger-orange-80 #4c1405` |
| Interaction blue | `interaction-blue-10 #e0f0ff`; `interaction-blue-20 #b9dafc`; `interaction-blue-30 #92c5f9`; `interaction-blue-40 #4394e5`; `interaction-blue-50 #0066cc`; `interaction-blue-60 #004d99`; `interaction-blue-70 #003366`; `interaction-blue-80 #032142` |

### People palette

| Family | Tokens |
|---|---|
| Cool tone | `cool-tone-10 #ffe3dc`; `cool-tone-20 #f7c8bb`; `cool-tone-30 #e8a997`; `cool-tone-40 #ce8873`; `cool-tone-50 #a66552`; `cool-tone-60 #724335`; `cool-tone-70 #40251d` |
| Warm tone | `warm-tone-10 #ffe9dc`; `warm-tone-20 #f9d5c0`; `warm-tone-30 #edbea1`; `warm-tone-40 #d8a381`; `warm-tone-50 #b88564`; `warm-tone-60 #8e6549`; `warm-tone-70 #664934` |

## Source

Red Hat Brand Standards: Color, Color Swatches, and Accessibility guidance supplied for HyperShell on 2026-08-05.
