# UI Verification Standard

**Status:** Active
**Applies to:** Design proposals, change reviews, release audits, and post-release evaluation

## Purpose

Define the evidence needed to claim that an interface follows the UI standards. “UX compliant” is not a valid unscoped claim: every result is limited to named requirements, users, tasks, environments, states, and product versions.

## Requirements

### Requirement UI-VER-01: Audit Contract

Before final evaluation, the team SHALL record the product/build, routes and components, roles, users and contexts, critical tasks, platforms, viewports, input and assistive technologies, locales, states, third parties, applicable requirements, exclusions, and release thresholds.

**Verification:** Reject an audit that cannot identify what was tested, what was excluded, and which decision the evidence supports.

### Requirement UI-VER-02: Representative Coverage

An audit SHALL cover every unique component/template and every critical journey end to end. It SHALL include entry, authentication, navigation, help, exit, loading, empty, success, validation, error, permission, offline, timeout, destructive, cancellation, and recovery states where applicable.

**Verification:** Compare the route/component/state inventory with the tested sample. Sampling SHALL follow a documented rationale and SHALL NOT turn untested pages into passes.

### Requirement UI-VER-03: Reproducible Evidence

Each result SHALL cite the requirement ID, product location and state, build/version, environment, method, observed result, evidence artifact, evaluator, and date. Findings SHALL include user impact, severity, fix direction, owner, and retest status.

**Verification:** An independent reviewer can reproduce a sample of findings and trace closed findings to build-specific retest evidence.

### Requirement UI-VER-04: Status Integrity

Requirement results SHALL use `PASS`, `FAIL`, `PARTIAL`, `NOT_TESTED`, `BLOCKED`, or a justified `N/A`. `NOT_TESTED`, `BLOCKED`, `N/A`, a waiver, and an automated-tool non-finding SHALL NOT be counted as a pass.

**Verification:** Recompute the release decision from criterion-level records. No aggregate score may hide an applicable standards failure, critical task failure, material segment failure, or unknown coverage.

### Requirement UI-VER-05: Complementary Methods

Evaluation SHALL combine deterministic checks, protocol-based expert inspection, complete-task input/assistive-technology testing, and representative-user outcome testing in proportion to risk. Qualitative judgment without a recorded protocol SHALL NOT be called compliance evidence.

**Verification:** Preserve tool/version/configuration and raw output, inspection steps and artifacts, environment matrix, participant criteria, protocol, interventions, raw outcomes, segmented analysis, and limitations.

### Requirement UI-VER-06: Outcome Measures

Critical tasks SHALL define unassisted completion, critical errors, failure/abandonment, time, assistance, and false-confidence or comprehension measures as relevant. Thresholds SHALL be set before final evaluation and SHALL be stricter when failure threatens safety, rights, money, privacy, or irreversible data.

**Verification:** Report numerator and denominator, uncertainty and limitations, and relevant segments. A formative study MAY discover problems but SHALL NOT be presented as a population estimate.

### Requirement UI-VER-07: Severity and Release Gate

Severity SHALL be based on user impact, affected breadth, exposure, consequence, and recoverability—not cosmetic preference or implementation effort. A release SHALL NOT pass with an applicable normative failure, unresolved critical finding, failed critical-task threshold, or material segment failure hidden by an aggregate.

**Verification:** Apply these levels: `Critical` blocks a critical task or creates likely material harm/severe exclusion; `High` seriously degrades a major task or violates an applicable obligation; `Medium` creates recoverable delay/error/confusion; `Low` has limited task impact. Preserve accountable, expiring risk acceptance separately from pass status.

### Requirement UI-VER-08: Invalid Shortcuts

Audits SHALL NOT treat any of the following as universal proof: three-click limits; seven-item menus; five-user sufficiency; “users do not scroll”; blanket component bans; icon universality; placeholders as labels; an automated accessibility score; an accessible component library; WCAG alone as usable accessibility; SUS 68; conversion, engagement, NPS, or time-on-site; fixed grids or aesthetic ratios; or a human-factors principle as a universal layout formula.

**Verification:** Replace each shortcut with the applicable standard, explicit task/context evidence, or a documented design hypothesis and test.

### Requirement UI-VER-09: Recurring Assurance

UI verification SHALL run per change and release, after incidents or material platform/standards changes, and periodically against field outcomes. The standards, supported test matrix, exceptions, and component evidence SHALL be reviewed at least annually.

**Verification:** Inspect CI checks, release evidence, field monitoring, incident follow-up, exception expiry, source review dates, and regression coverage.

## Minimum Release Evidence

1. Every applicable `SHALL` and WCAG 2.2 A/AA criterion passes or remains visibly failed under an authorized, expiring risk decision.
2. Representative users meet predeclared critical-task thresholds without unplanned assistance.
3. Relevant keyboard, zoom/reflow, screen-reader, touch, voice, reduced-motion, responsive, and locale paths work end to end.
4. Purpose, state, consequence, cost, data use, commitment, and recovery are clear.
5. No critical deception, inaccessible dead end, irreversible surprise, or hidden segment failure remains.
6. Production performance, completion, failure, support demand, and regressions have owned monitoring.

## Primary Basis

- [WCAG-EM](https://www.w3.org/WAI/test-evaluate/conformance/wcag-em/)
- [ACT Rules](https://www.w3.org/WAI/standards-guidelines/act/rules/)
- [W3C Easy Checks limitations](https://www.w3.org/WAI/test-evaluate/preliminary/)
- [GOV.UK usability benchmarking](https://www.gov.uk/service-manual/measuring-success/usability-benchmarking-a-website-or-whole-service)
