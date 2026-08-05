# UI Interaction Standard

**Status:** Active
**Applies to:** Navigation, controls, forms, workflows, and dynamic interface states

## Purpose

Require understandable, predictable, recoverable interaction without prescribing a visual style or a single component library.

## Requirements

### Requirement UI-INT-01: Discoverable Controls

Interactive elements SHALL be distinguishable from content and expose a persistent, specific label, current state, availability, and predictable result before activation. Unfamiliar or consequential icons and gestures SHALL have visible text or instruction.

**Verification:** Review every control and state visually and in the accessibility tree. Test first use without coaching and inspect name, role, value, state, relationships, and visible-label/name agreement.

### Requirement UI-INT-02: Timely and Truthful Feedback

Every action SHALL receive perceivable feedback that distinguishes accepted input, work in progress, success, partial success, failure, and changed state. The interface SHALL NOT report success before durable completion.

**Verification:** Exercise operations under fast, slow, offline, repeated, background, partial, and failed conditions. Verify visual feedback and assistive-technology announcement without unexpected focus movement.

### Requirement UI-INT-03: Progress and Waiting

Operations that interrupt flow SHOULD acknowledge input within about 100 ms, show working feedback after about one second, and provide meaningful progress plus safe interruption for waits around ten seconds or more when feasible. These are response-time heuristics, not conformance thresholds.

**Verification:** Measure event-to-feedback latency under representative conditions. Confirm determinate progress is accurate, indeterminate progress does not imply precision, and cancellation or background continuation leaves truthful state.

### Requirement UI-INT-04: Agency, Exit, and State

Users SHALL be able to back out, cancel, close, edit, or safely leave a mode or flow without accidental commitment or unexplained data loss. Recoverable work and focus SHALL survive validation, navigation, interruption, and retry where the task permits.

**Verification:** Test Back, Escape, Close, Cancel, refresh, deep link, session interruption, and resume with supported inputs. Confirm any unavoidable loss is explained before it occurs.

### Requirement UI-INT-05: Error Prevention and Recovery

The interface SHALL prevent foreseeable errors, accept harmless input variation, preserve valid work, identify the affected item, explain the problem without blame, and provide a feasible correction. Duplicate activation SHALL NOT produce duplicate consequential effects.

**Verification:** Use valid boundary data and deliberate user, network, permission, timeout, and system failures. Test correction, retry, double activation, refresh, and recovery—not only error-message appearance.

### Requirement UI-INT-06: Reversibility and Consequence

Frequent reversible actions SHOULD offer Undo. Irreversible, legal, financial, privacy, security, or destructive actions SHALL provide proportionate review or confirmation before commitment and an accurate receipt afterward.

**Verification:** Attempt the wrong action, cancellation, correction, undo, and recovery. Verify material scope and consequences are visible before the final action.

### Requirement UI-INT-07: Navigation and Findability

Navigation SHALL be stable, task-oriented, and free of dead ends. Users SHALL be able to identify their location, return to a stable point, predict destinations, and locate substantial content through more than one reasonable route unless it is a step in a process.

**Verification:** Test deep links, browser Back, refresh, current location, navigation labels, focus order, bypass mechanisms, search/zero-results where present, and realistic findability tasks with predeclared thresholds.

### Requirement UI-INT-08: Minimal and Forgiving Forms

Forms SHALL request only necessary information and use persistent labels, appropriate native controls, explicit required/optional and format guidance, logical grouping, autofill, paste, review, and correction. Previously supplied data SHALL be reused unless re-entry is necessary.

**Verification:** Trace every field to a purpose and retention rule. Test keyboard, touch, screen reader, zoom, paste, autofill/password manager, back, error, save/resume, international data, and the longest valid values.

### Requirement UI-INT-09: Helpful Validation

Validation SHALL occur at a meaningful completion event and SHALL NOT interrupt valid intermediate input. Detected errors SHALL be associated with their fields, summarized when useful, announced accessibly, and retained until resolved.

**Verification:** Test slow typing, composition, dictation, paste, blur, submission, asynchronous validation, and correction. Confirm no premature block or lost input.

### Requirement UI-INT-10: Predictable Dynamic Behavior

Automatic updates SHALL NOT unexpectedly move focus, change context, submit, dismiss content, overwrite choices, or reset filters and position. Selection, expansion, mode, filtering, sorting, save, and synchronization state SHALL remain visible and predictable.

**Verification:** Exercise live updates, autosave, refresh, navigation, overlays, and concurrent changes with pointer, keyboard, and screen reader. Confirm updates are announced when required and user choices survive or are explicitly resolved.

## Primary Basis

- [ISO 9241-110 interaction principles](https://www.iso.org/standard/75258.html)
- [Nielsen's usability heuristics](https://www.nngroup.com/articles/ten-usability-heuristics/)
- [Shneiderman's Eight Golden Rules](https://www.cs.umd.edu/users/ben/goldenrules.html)
- [W3C Forms Tutorial](https://www.w3.org/WAI/tutorials/forms/)
- [GOV.UK validation pattern](https://design-system.service.gov.uk/patterns/validation/)
