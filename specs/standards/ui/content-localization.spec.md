# UI Content and Localization Standard

**Status:** Active
**Applies to:** Interface copy, help, notifications, data entry/display, translation, and bidirectional layouts

## Purpose

Make content understandable, actionable, governable, and structurally capable of serving supported languages and cultures.

## Requirements

### Requirement UI-CONTENT-01: Task Language

Content SHALL use concise, concrete language that names the user's task, objects, state, choices, and consequences. Unexplained jargon, internal process language, blame, and unnecessary reading SHALL be removed.

**Verification:** Review critical content with the controlled vocabulary and test comprehension/action with representative users. Readability scores MAY inform editing but SHALL NOT substitute for comprehension evidence.

### Requirement UI-CONTENT-02: Specific Labels and Instructions

Titles, headings, links, controls, fields, errors, and help SHALL be specific to their purpose and distinguishable where immediate visual context is unavailable. Instructions SHALL appear before the decision or input they govern.

**Verification:** Inspect page, heading, link, control, and error lists outside their visual layout. Test that users predict consequential actions and provide correct input without hidden instructions.

### Requirement UI-CONTENT-03: Material Consequences

Cost, recurrence, duration, data use, eligibility, risk, scope, and reversibility SHALL be stated before a user commits. Confirmation and receipts SHALL accurately describe what occurred and what remains possible.

**Verification:** Audit every commitment point and ask representative users to explain the material consequence before acting.

### Requirement UI-CONTENT-04: Content at the Point of Need

Task-critical content and recovery guidance SHALL be available at the point of need. Optional detail MAY be progressively disclosed when it remains signposted, operable, and persistent.

**Verification:** Observe missed information, detours, requests for help, and premature interruption during complete tasks. Test deferred content with people who need it.

### Requirement UI-CONTENT-05: Content Lifecycle

Every durable content item SHALL have an owner, authoritative source, review condition/date, lifecycle state, and update or removal process.

**Verification:** Sample live content against governance records and report stale, conflicting, orphaned, or untestable content.

### Requirement UI-I18N-01: Language, Encoding, and Direction

Interfaces SHALL use Unicode, declare page and passage language, and derive writing direction from content. Right-to-left and mixed-direction content SHALL preserve reading order, cursor behavior, alignment, and the correct mirroring exceptions.

**Verification:** Inspect language/direction metadata and test a real RTL locale with mixed-script values, keyboard, screen reader, and native-language review.

### Requirement UI-I18N-02: Culturally Flexible Data

Data models and forms SHALL NOT assume English, Western given/family names, fixed address or phone structures, mandatory honorifics, or one cultural identity model unless an external requirement is verified and explained.

**Verification:** Test a locale-informed corpus including mononyms, multipart names, diacritics, non-Latin scripts, varied addresses, and relevant identifiers. Confirm storage and redisplay do not corrupt valid data.

### Requirement UI-I18N-03: Unambiguous Local Formats

Dates, times, time zones, calendars, numbers, currency, units, collation, and pluralization SHALL be locale-aware and unambiguous for the task. Consequential timestamps SHALL include the needed date, zone, and offset context.

**Verification:** Test locale boundaries, daylight-saving transitions, time zones, negative/large values, entry-display round trips, sorting, and plural categories.

### Requirement UI-I18N-04: Localizable Layout and Strings

User-facing strings SHALL be externalized as complete translatable messages with supported plural and grammatical variation. Layout SHALL tolerate text expansion, contraction, wrapping, alternate fonts, and different reading directions without clipping or lost function.

**Verification:** Run pseudo-localization and longest-string tests across responsive and scaled-text states. Reject concatenated fragments that translators cannot reorder correctly.

### Requirement UI-I18N-05: In-context Locale Quality

Each released locale SHALL receive in-context linguistic and functional review proportional to risk. Machine translation alone SHALL NOT approve critical tasks or consequential content.

**Verification:** Preserve qualified speaker review and representative task/comprehension results for each released critical journey.

## Primary Basis

- [W3C Writing for Web Accessibility](https://www.w3.org/WAI/tips/writing/)
- [W3C cognitive accessibility guidance](https://www.w3.org/TR/coga-usable/)
- [W3C Internationalization Quick Tips](https://www.w3.org/International/quicktips/index)
- [W3C Personal Names Around the World](https://www.w3.org/International/questions/qa-personal-names.en)
