# UI Foundations Standard

**Status:** Active
**Applies to:** Every HyperShell user interface, including prototypes used to make product decisions

## Purpose

Define the product, inclusion, clarity, and governance conditions that make an interface fit for use. `SHALL` is a release requirement. `SHOULD` is the default unless a dated decision record contains stronger contextual evidence. Visual preference never overrides safety, accessibility, or informed user agency.

## Requirements

### Requirement UI-FND-01: Observed Outcome and Context

Each interface SHALL be designed for named users, critical tasks, environments, abilities, constraints, risks, and desired outcomes supported by current evidence rather than an assumed average user.

**Verification:** Trace each critical flow to a context-of-use record and a measurable user outcome. Test the flow with representative users and report unassisted success, critical errors, abandonment, time, and assistance against thresholds declared before evaluation.

### Requirement UI-FND-02: Inclusive Participation

Research and evaluation SHALL include material differences in disability, assistive technology, language, device, expertise, bandwidth, and situational constraint. Aggregate results SHALL NOT conceal a material segment failure.

**Verification:** Compare participant and test-environment coverage with the context record. Report gaps and segmented outcomes with uncertainty; assign every unresolved exclusion risk an owner.

### Requirement UI-FND-03: Orientation and Hierarchy

Every view SHALL make its purpose, current location or state, important information, available actions, and likely next step clear. Visual prominence SHALL follow task importance and reading order.

**Verification:** Inspect unique titles, headings, landmarks, grouping, current state, and action hierarchy. In a brief predeclared orientation probe and first-use task, representative users identify the purpose and next action without coaching.

### Requirement UI-FND-04: User Language and Mental Models

Navigation, labels, concepts, and sequences SHALL use the intended users' vocabulary and understanding of the activity. Internal organization, storage models, and unexplained technical terms SHALL NOT determine the interface.

**Verification:** Trace critical terminology to research, support/search evidence, or accepted domain language. Test label, icon, state, and consequence comprehension before activation; test findability with realistic content.

### Requirement UI-FND-05: Essential Simplicity

The primary path SHALL contain only information and controls that serve the task, understanding, safety, trust, or recovery. Progressive disclosure MAY defer secondary complexity but SHALL NOT hide prerequisites, material cost or risk, critical status, or the only route to a task.

**Verification:** Map every primary-view element to a need. Test that first-use users find essential actions and that people who need deferred content can discover, operate, and retain its state with keyboard and assistive technology.

### Requirement UI-FND-06: Consistency and Convention

The same concept SHALL have consistent names, semantics, state models, and behavior. The interface SHOULD follow established web and supported-platform conventions unless comparative user evidence demonstrates a better accessible result.

**Verification:** Compare the component and content inventory across routes and states. Record and test every deliberate convention deviation.

### Requirement UI-FND-07: Learnability and Efficiency

The default path SHALL work for a first-time user. Repeated-use accelerators, bulk actions, safe defaults, and personalization SHOULD improve efficiency without obscuring the default path or reducing accessibility.

**Verification:** Benchmark first-use and experienced-user tasks separately. Confirm every shortcut has a discoverable operable equivalent and that expert features improve the intended outcome without increasing critical errors.

### Requirement UI-FND-08: Complete Service

The designed experience SHALL include discovery, entry, authentication, transaction, confirmation, notifications, support, interruption and return, amendment, cancellation, deletion, and failure recovery where applicable—not only the happy-path screen.

**Verification:** Walk a service blueprint and execute every applicable cross-channel handoff and recovery path end to end. Verify consistent state, terminology, ownership, and outcome.

### Requirement UI-FND-09: Governed Components and Content

Shared components and content SHALL have an owner, documented purpose, supported states, behavior, content rules, accessibility behavior, localization constraints, tests, version, known limitations, and retirement path.

**Verification:** Compare design-system records with rendered code in default, loading, empty, success, error, disabled, permission, timeout, and destructive states. Component compliance SHALL NOT be presented as page or journey compliance.

### Requirement UI-FND-10: Explicit Tradeoffs

Exceptions and conflicting guidance SHALL be resolved in this order: applicable law and normative requirements; user safety, accessibility, and informed agency; observed outcomes in context; platform convention; practitioner heuristic. A waiver SHALL remain recorded as risk rather than a pass.

**Verification:** Require a dated decision record naming scope, affected users, alternatives, evidence, residual risk, owner, monitoring, and expiry.

## Primary Basis

- [ISO 9241-11 usability in context](https://www.iso.org/standard/63500.html)
- [ISO 9241-210 human-centred design](https://www.iso.org/standard/77520.html)
- [GOV.UK Design Principles](https://www.gov.uk/guidance/government-design-principles)
- [W3C cognitive accessibility guidance](https://www.w3.org/TR/coga-usable/)
- [Microsoft Inclusive Design](https://inclusive.microsoft.design/)
