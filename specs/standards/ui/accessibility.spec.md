# UI Accessibility Standard

**Status:** Active
**Applies to:** Every HyperShell web page, view, component, state, and complete process

## Purpose

Establish WCAG 2.2 Level A and AA as the minimum web conformance baseline while requiring complete-task testing with disabled people and relevant assistive technologies. Passing a scanner or component library is never sufficient evidence.

## Requirements

### Requirement UI-A11Y-01: WCAG 2.2 Level AA

Every applicable WCAG 2.2 Level A and AA success criterion SHALL pass for every page and complete process in the declared scope. Sampling MAY reduce evaluation effort but SHALL NOT redefine conformance. Former criterion 4.1.1 Parsing is intentionally absent from WCAG 2.2.

**Verification:** Record `PASS`, `FAIL`, or a justified `N/A` for each criterion in the inventory below, tied to the tested build, route, state, viewport, and environment. `NOT_TESTED` SHALL NOT count as a pass.

### Requirement UI-A11Y-02: Semantic Structure and Controls

Information, relationships, reading order, headings, landmarks, labels, instructions, tables, groups, and control name/role/value/state SHALL be programmatically determinable and match the visible interface.

**Verification:** Inspect DOM and accessibility tree, disable visual styling, and complete tasks with a representative screen reader. Prefer native semantics; when a custom widget is necessary, test its complete keyboard and announcement behavior against the appropriate WAI-ARIA pattern.

### Requirement UI-A11Y-03: Keyboard and Focus

Every function SHALL be operable by keyboard without a trap. Focus order SHALL preserve meaning; focus SHALL remain visible and not be entirely obscured; overlays and route/state changes SHALL place and restore focus predictably.

**Verification:** Complete every critical journey keyboard-only in default, loading, empty, error, timeout, overlay, disabled, and success states. Verify skip/bypass behavior, focus order, indicator visibility, entry, exit, and restoration.

### Requirement UI-A11Y-04: Alternatives and Multiple Modalities

Meaning SHALL NOT depend only on sight, color, shape, location, sound, motion, hover, or a complex gesture. Meaningful non-text content and prerecorded/live media SHALL provide the alternatives required by WCAG.

**Verification:** Review text alternatives, captions, transcripts, audio description, sensory instructions, hover/focus disclosure, color-independent states, and single-pointer alternatives. Test with images/styles/audio disabled where useful.

### Requirement UI-A11Y-05: Contrast and Visual States

Normal text SHALL have at least 4.5:1 contrast and large text at least 3:1, subject to WCAG exceptions. Visual information required to identify active controls, states, and meaningful graphics SHALL have at least 3:1 contrast. Focus, error, selection, and state SHALL NOT rely on color alone.

**Verification:** Measure rendered colors in every theme, image/background, and interactive state. Do not infer runtime compliance from design tokens alone.

### Requirement UI-A11Y-06: Resize, Reflow, and Preferences

Text SHALL resize to 200% without loss. At 400% browser zoom, vertically scrolling content SHALL reflow to 320 CSS pixels wide and horizontally scrolling content to 256 CSS pixels high without unintended two-dimensional scrolling, subject to WCAG exceptions. Text-spacing overrides, orientation, contrast, and reduced-motion preferences SHALL preserve content and function.

**Verification:** Test representative and extreme content at required zoom, text size, spacing, viewport, orientation, theme, forced/high contrast, and reduced motion. Record essential exceptions.

### Requirement UI-A11Y-07: Targets, Pointer, and Voice

Web pointer targets SHALL be at least 24 by 24 CSS pixels or satisfy a WCAG 2.5.8 exception. Touch-first targets SHOULD be at least 44 by 44 CSS pixels where layout permits. Dragging, multipoint, path-based, and motion actions SHALL have single-pointer or control alternatives unless essential. The accessible name SHALL contain the visible label.

**Verification:** Measure the actual hit region and spacing, then test crowded controls with touch, pointer, keyboard, and voice control. Verify drag and gesture alternatives.

### Requirement UI-A11Y-08: Time, Motion, and Flashing

Users SHALL be able to adjust relevant time limits and pause, stop, hide, or control qualifying moving and auto-updating content. Content SHALL NOT violate WCAG flash thresholds. Motion-triggered functions SHALL have controls and respect reduced-motion preferences unless essential.

**Verification:** Test expiry warnings/extensions, moving and updating content, reduced motion, device motion, pause/stop controls, and flash analysis under every relevant state.

### Requirement UI-A11Y-09: Errors, Authentication, and Status

Errors SHALL be identified in text with labels/instructions and correction suggestions where known. Consequential submissions SHALL be reversible, checked, or reviewable. Re-entering data SHALL be avoided, authentication SHALL NOT impose an unsupported cognitive-function test, and status messages SHALL be announced without moving focus.

**Verification:** Complete invalid, repeated-data, authentication, timeout, recovery, and consequential-submission paths with keyboard, password manager, and screen reader.

### Requirement UI-A11Y-10: Mixed-Method Evaluation

Accessibility evaluation SHALL combine automated rules, manual code and visual inspection, keyboard testing, zoom/reflow testing, representative assistive-technology testing, and disabled-user task evaluation. Tool versions and untested coverage SHALL be reported.

**Verification:** Preserve criterion-level evidence and raw tool output. Manually evaluate every criterion that automation cannot establish and retest fixes in the composed journey.

## WCAG 2.2 A/AA Criterion Inventory

Audit every applicable criterion; use the [normative WCAG text](https://www.w3.org/TR/WCAG22/) and its linked Understanding documents for exceptions and techniques.

- **1.1 Text Alternatives:** 1.1.1 Non-text Content (A)
- **1.2 Time-based Media:** 1.2.1 Audio-only and Video-only (Prerecorded) (A); 1.2.2 Captions (Prerecorded) (A); 1.2.3 Audio Description or Media Alternative (Prerecorded) (A); 1.2.4 Captions (Live) (AA); 1.2.5 Audio Description (Prerecorded) (AA)
- **1.3 Adaptable:** 1.3.1 Info and Relationships (A); 1.3.2 Meaningful Sequence (A); 1.3.3 Sensory Characteristics (A); 1.3.4 Orientation (AA); 1.3.5 Identify Input Purpose (AA)
- **1.4 Distinguishable:** 1.4.1 Use of Color (A); 1.4.2 Audio Control (A); 1.4.3 Contrast (Minimum) (AA); 1.4.4 Resize Text (AA); 1.4.5 Images of Text (AA); 1.4.10 Reflow (AA); 1.4.11 Non-text Contrast (AA); 1.4.12 Text Spacing (AA); 1.4.13 Content on Hover or Focus (AA)
- **2.1 Keyboard Accessible:** 2.1.1 Keyboard (A); 2.1.2 No Keyboard Trap (A); 2.1.4 Character Key Shortcuts (A)
- **2.2 Enough Time:** 2.2.1 Timing Adjustable (A); 2.2.2 Pause, Stop, Hide (A)
- **2.3 Seizures and Physical Reactions:** 2.3.1 Three Flashes or Below Threshold (A)
- **2.4 Navigable:** 2.4.1 Bypass Blocks (A); 2.4.2 Page Titled (A); 2.4.3 Focus Order (A); 2.4.4 Link Purpose (In Context) (A); 2.4.5 Multiple Ways (AA); 2.4.6 Headings and Labels (AA); 2.4.7 Focus Visible (AA); 2.4.11 Focus Not Obscured (Minimum) (AA)
- **2.5 Input Modalities:** 2.5.1 Pointer Gestures (A); 2.5.2 Pointer Cancellation (A); 2.5.3 Label in Name (A); 2.5.4 Motion Actuation (A); 2.5.7 Dragging Movements (AA); 2.5.8 Target Size (Minimum) (AA)
- **3.1 Readable:** 3.1.1 Language of Page (A); 3.1.2 Language of Parts (AA)
- **3.2 Predictable:** 3.2.1 On Focus (A); 3.2.2 On Input (A); 3.2.3 Consistent Navigation (AA); 3.2.4 Consistent Identification (AA); 3.2.6 Consistent Help (A)
- **3.3 Input Assistance:** 3.3.1 Error Identification (A); 3.3.2 Labels or Instructions (A); 3.3.3 Error Suggestion (AA); 3.3.4 Error Prevention (Legal, Financial, Data) (AA); 3.3.7 Redundant Entry (A); 3.3.8 Accessible Authentication (Minimum) (AA)
- **4.1 Compatible:** 4.1.2 Name, Role, Value (A); 4.1.3 Status Messages (AA)

## Primary Basis

- [WCAG 2.2](https://www.w3.org/TR/WCAG22/)
- [WCAG-EM evaluation methodology](https://www.w3.org/WAI/test-evaluate/conformance/wcag-em/)
- [WAI-ARIA Authoring Practices](https://www.w3.org/WAI/ARIA/apg/)
- [ACT Rules](https://www.w3.org/WAI/standards-guidelines/act/rules/)
