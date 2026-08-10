# UI Trust, Performance, and Resilience Standard

**Status:** Active
**Applies to:** Data use, consent, permissions, commitments, account controls, loading, network behavior, and production monitoring

## Purpose

Protect informed user agency and make responsiveness, stability, and recovery measurable parts of interface quality.

## Requirements

### Requirement UI-TRUST-01: Honest Choice Architecture

The interface SHALL NOT deceive, manipulate, obstruct, shame, disguise promotion, manufacture urgency or scarcity, repeatedly nag after a choice, or make the option benefiting HyperShell materially easier to select than its reasonable alternative.

**Verification:** Audit acquisition, consent, purchase, renewal, notification, cancellation, and deletion flows for asymmetric prominence, steps, defaults, wording, and consequences. Test user understanding before commitment.

### Requirement UI-TRUST-02: Data Minimization and Transparency

Personal or sensitive data SHALL be collected only for a documented purpose and SHALL have defined use, access, sharing, retention, and deletion behavior. Material processing SHALL be explained where the user decides, not hidden only in a policy.

**Verification:** Trace each collected field and inferred attribute to the data-flow and retention record. Compare the actual implementation with point-of-decision content and test comprehension.

### Requirement UI-TRUST-03: Consent and Withdrawal

Where consent is used, it SHALL be specific, informed, freely given, and unbundled as required. Optional consent SHALL NOT be preselected. Refusal and later withdrawal SHALL be as clear and practicable as acceptance.

**Verification:** Compare labels, prominence, defaults, step count, delay, and outcome for accept, refuse, review, and withdraw paths; verify the stored state and downstream effect.

### Requirement UI-TRUST-04: Account and Communication Control

Users SHALL be able to find and complete cancellation, account deletion, data export, notification changes, and subscription controls without unnecessary retention friction or loss of unrelated rights.

**Verification:** Complete each path from common entry points, counting dead ends, delays, forced contact, repeated persuasion, and post-action effects. Confirm an accurate receipt.

### Requirement UI-TRUST-05: Permissions, Automation, and Sensitive Exposure

Permission requests SHALL occur in context, explain benefit and refusal consequences, and recover after denial. Consequential automation SHALL be identifiable and provide correction, appeal, or human support proportional to impact. Sensitive information SHALL NOT leak through previews, notifications, URLs, shared-device persistence, or support handoffs.

**Verification:** Test allow, deny, partial, revoke, adverse automated output, appeal, lock-screen/background, logout, cache, and support-handoff states using a privacy threat model.

### Requirement UI-PERF-01: Core Web Vitals

At the 75th percentile of representative field visits, separately for mobile and desktop, traffic-bearing routes SHALL meet Largest Contentful Paint at or below 2.5 seconds, Interaction to Next Paint at or below 200 milliseconds, and Cumulative Layout Shift at or below 0.1. New or low-traffic routes SHALL use equivalent lab budgets until adequate field data exists.

**Verification:** Preserve the field query, date range, route/journey scope, percentile, device split, and sample size. Use lab tests for diagnosis, not as a substitute for field evidence when adequate field data exists.

### Requirement UI-PERF-02: Task Performance Budgets

Critical tasks SHALL have product-specific response and completion budgets for representative devices, networks, and data volumes. Tail performance and failure SHALL be visible; averages alone SHALL NOT establish a pass.

Paginated collection views SHALL request only the page needed for the current view unless an explicit bounded bulk workflow requires more. Search, filtering, and sorting that affect the collection SHALL execute at the authoritative data source for high-volume views, and their normalized state SHALL participate in request identity and reproducible navigation.

Incremental server-backed search and filtering SHALL coalesce rapid input with a documented, task-appropriate debounce and cancel obsolete requests. Text typeaheads SHOULD use 250 milliseconds unless task evidence or an external contract supports a different value. A rapid input burst SHALL NOT issue one upstream request per keystroke.

**Verification:** Benchmark cold, warm, throttled, degraded, and high-volume conditions against thresholds declared before final evaluation. Record upstream request count and transferred rows for initial load, page change, search, filter, and sort; test slow typing and a rapid multi-character burst against the declared debounce; and fail an ordinary paginated view that exhausts the collection before first render or an incremental search that requests every intermediate value.

### Requirement UI-PERF-03: Stable and Accurate Progress

The interface SHALL acknowledge action, prevent unexpected layout movement, expose accurate save/sync/progress state, and keep unrelated safe work available during long operations where feasible.

**Verification:** Measure event-to-feedback and layout shift, throttle operations, compare displayed progress with actual completion, and verify cancel/background behavior.

### Requirement UI-PERF-04: Resilient Failure and Recovery

Offline, timeout, partial failure, reconnect, retry, duplicate submission, conflict, and session-expiry behavior SHALL preserve work where feasible and explain what happened, what was committed, and what the user can safely do next.

**Verification:** Inject failures at each consequential network boundary and test reconnect, idempotency, resume, version conflict, partial completion, and support escalation.

### Requirement UI-PERF-05: Production Monitoring

Released interfaces SHALL monitor critical-task completion and failure, abandonment, support demand, accessibility regressions, performance percentiles, client errors, and harmful unintended use. Alerts SHALL have thresholds and owners.

**Verification:** Inspect production queries, segmentation, alerts, ownership, incident reviews, and evidence that findings lead to retest or design change.

## Primary Basis

- [W3C Ethical Web Principles](https://www.w3.org/TR/ethical-web-principles/)
- [W3C Privacy Principles](https://www.w3.org/TR/privacy-principles/)
- [FTC: Bringing Dark Patterns to Light](https://www.ftc.gov/reports/bringing-dark-patterns-light)
- [Core Web Vitals](https://web.dev/articles/vitals)
