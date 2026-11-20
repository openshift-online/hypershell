# Platform Policy Admin Console - manage gateway policies and enforcement

**ID prefix:** WEB-PADM

**Date:** 2026-11-20
**Status:** Draft
**Source mockups:** `specs/web-console/mockups/admin/` (platform-policies list, enforcement-toggle confirmation, policy detail, and the create/edit Cedar editor states), `specs/web-console/mockups/common/policy-rules.tsx` (shared rendered-rule presentation). These mockups and their design-review Storybook stories are delivered on the `mockups/platform-gateway-policy` branch; this spec cites the story states it governs and is finalized together with that mockup set.
**Related:** [`../platform/platform-gateway-policy.spec.md`](../platform/platform-gateway-policy.spec.md) (policy behavior, validation, enforcement mode, defaults, denial text, and the Cedar display renderer PLAT-PGP-01..20), [`../security/rbac-enforcement.spec.md`](../security/rbac-enforcement.spec.md) (`platform-policy:admin` SEC-RBAC-30, effective-policies and findings authorization SEC-RBAC-31), [`gateway-list-detail.spec.md`](./gateway-list-detail.spec.md) (read-only gateway Policies tab WEB-GLIS-26), [`../standards/ui/patternfly.spec.md`](../standards/ui/patternfly.spec.md) (code-editor admission UIS-PFLY-11), [`../standards/ui/hexagonal-architecture.spec.md`](../standards/ui/hexagonal-architecture.spec.md), [`../standards/ui/domain-observability.spec.md`](../standards/ui/domain-observability.spec.md), [`../standards/ui/content-localization.spec.md`](../standards/ui/content-localization.spec.md), [`../standards/ui/verification.spec.md`](../standards/ui/verification.spec.md), [`architecture.spec.md`](./architecture.spec.md) (console shell, routing, BFF, SDK boundary)

## Purpose

The web console gives a platform policy administrator a place to write, review, and
roll out `PlatformGatewayPolicy` records and to move the platform enforcement
toggle. The admin control panel is visible only to holders of `platform-policy:admin`
(or `platform:admin`). It presents the policy list, the global toggle with an impact
confirmation, a per-policy detail view with rollout and findings, and a Cedar policy
editor designed so an author knows exactly what the policy language can and cannot
express. All policy semantics (forbid-only, write-time validation, effective mode,
defaults, denial text, and the plain-words renderer) are defined in
[`../platform/platform-gateway-policy.spec.md`](../platform/platform-gateway-policy.spec.md);
this spec defines the console surface over that behavior.

## Requirements

### Requirement: WEB-PADM-01 -- Admin Panel Visibility and Authorization

The console SHALL show the platform policy administration entry point only to a user
who holds `platform-policy:admin` or `platform:admin`; for every other user the
navigation entry SHALL be hidden and a direct route to an admin policy view SHALL
render the standard not-authorized state rather than policy data. Hiding the
navigation SHALL NOT be the authorization boundary: the API server remains the
enforcement point (SEC-RBAC-30), and the console SHALL treat a `403` from a policy
write as a denied operation, not an error to retry.

#### Scenario: WEB-PADM-01.01 -- Entry Hidden Without the Role

- GIVEN a signed-in user without `platform-policy:admin` or `platform:admin`
- WHEN the console renders the navigation
- THEN the platform policy administration entry SHALL NOT appear
- AND navigating directly to an admin policy route SHALL render the not-authorized state, not policy data

#### Scenario: WEB-PADM-01.02 -- Write Denial Surfaced as Denied

- GIVEN a user whose role was changed so a policy write now returns `403`
- WHEN the user submits a policy edit
- THEN the console SHALL present a denied-operation message
- AND SHALL NOT retry the write as if it were a transient error

---

### Requirement: WEB-PADM-02 -- Platform Policies List

The admin panel SHALL list every `PlatformGatewayPolicy` with, per row: name, target
(all gateways or a summarized selector), effective mode (**Enforced** or **Audit
only**), rollout status across targeted gateways (for example applied, pending, and
unsupported counts), and a findings summary for the last 24 hours. The list SHALL
support the console's standard table state (sort, filter, and URL-addressable state)
and SHALL open a policy's detail view from its row.

#### Scenario: WEB-PADM-02.01 -- Row Shows Mode, Rollout, and Findings

- GIVEN two policies, one audit-only and one enforced on a subset of gateways
- WHEN the admin opens the policies list
- THEN each row SHALL show its effective mode, its per-gateway rollout counts, and its 24-hour findings summary
- AND selecting a row SHALL open that policy's detail view

---

### Requirement: WEB-PADM-03 -- Global Enforcement Toggle With Impact Confirmation

The admin panel SHALL present the global enforcement toggle (`audit` or `enforce`,
PLAT-PGP-06) and SHALL require a confirmation in both directions before it changes.
The confirmation SHALL show the gateways the change affects and the last 24 hours of
findings for the affected scope, so the administrator sees impact before enforcing.
Raising the toggle to `enforce` SHALL be presented as a higher-consequence action
than lowering it, consistent with the material-consequence content rules
(UIS-CONT-03).

#### Scenario: WEB-PADM-03.01 -- Switch to Enforced Shows Affected Gateways and Findings

- GIVEN the global toggle is `audit`
- WHEN the admin switches it to `enforce`
- THEN a confirmation SHALL show the affected gateways and the last 24 hours of findings
- AND the change SHALL apply only after the admin confirms

---

### Requirement: WEB-PADM-04 -- Policy Detail View

The policy detail view SHALL show the policy's rendered rules in plain words
(PLAT-PGP-17) alongside the Cedar source, the `userMessage`, and the
`documentationUrl`. It SHALL show, per targeted gateway, the rollout state
(`Applied`, `Pending`, or `Unsupported`) and that gateway's findings, and SHALL show
the policy's audit findings with filters. A statement the renderer cannot summarize
SHALL be shown as its Cedar source with the policy message, never hidden.

#### Scenario: WEB-PADM-04.01 -- Detail Shows Rendered Rules, Source, and Per-Gateway Rollout

- GIVEN an enforced policy targeting several gateways, one of which is `Unsupported`
- WHEN the admin opens the policy detail view
- THEN it SHALL show the rendered rules and the Cedar source
- AND it SHALL show each targeted gateway's rollout state including the `Unsupported` gateway, with that gateway's findings

---

### Requirement: WEB-PADM-05 -- Cedar Policy Editor and Live Validation

The create and edit experience SHALL provide a Cedar policy editor built on the
PatternFly code editor (`@patternfly/react-code-editor`, admitted per UIS-PFLY-11)
with line numbers and Cedar syntax highlighting. It SHALL validate the draft against
the HyperShell Cedar schema as the author edits and SHALL show diagnostics inline at
the offending line. A diagnostic SHALL name the line and suggest a fix, for example
that `context.cpuRequest` is not available and `context.cpuRequestMillis` (a `Long`
in millicores) should be used, or that `permit` is not allowed because platform
policies can only use `forbid` (PLAT-PGP-04). The editor SHALL NOT render
author-supplied policy text or diagnostics through any raw-HTML injection path; it
SHALL render React elements only.

#### Scenario: WEB-PADM-05.01 -- Inline Diagnostic Names the Line and Suggests a Fix

- GIVEN an author types a statement that uses `permit` or an unknown attribute
- WHEN the editor validates the draft
- THEN it SHALL show a diagnostic at that line naming the problem and suggesting the fix
- AND the author SHALL NOT be able to save a draft that fails write-time validation (PLAT-PGP-04)

---

### Requirement: WEB-PADM-06 -- Add-A-Rule Snippets

The editor SHALL offer an **Add a rule** menu of snippets (at least CPU request
range, CPU limit maximum, memory request range, memory limit maximum, GPU maximum,
allowed image registries, and roles that may use GPUs). Each snippet SHALL open a
small form that accepts Kubernetes quantities (for example `250m`, `16Gi`), convert
them to the schema units, and insert Cedar the plain-words renderer recognizes
(PLAT-PGP-03, PLAT-PGP-17), so an author can build a recognized rule without writing
Cedar by hand.

#### Scenario: WEB-PADM-06.01 -- Snippet Inserts Recognized Cedar From a Quantity Form

- GIVEN the author picks the CPU request range snippet and enters `250m` and `4`
- WHEN the form inserts the rule
- THEN the editor SHALL insert Cedar using `context.cpuRequestMillis` with the converted bounds
- AND the inserted rule SHALL be one the renderer displays in plain words

---

### Requirement: WEB-PADM-07 -- Reference Panel

The editor SHALL include a reference panel that lists what the language offers:
actions, principal attributes, resource attributes, and context attributes with
type, unit, and an example; the allowed operators; and what is **not** available for
v1 (`permit`, policy templates, quotas over other sandboxes, time conditions, and
the `ip()` extension until egress rules ship). The reference panel SHALL stay
consistent with the Cedar schema version the bundle carries (PLAT-PGP-03).

#### Scenario: WEB-PADM-07.01 -- Reference Lists Available and Unavailable Constructs

- GIVEN an author opens the reference panel
- WHEN it renders
- THEN it SHALL list the context attributes with type, unit, and example and the allowed operators
- AND it SHALL list the constructs not available in v1, including `permit` and `ip()`

---

### Requirement: WEB-PADM-08 -- Test Panel

The editor SHALL include a test panel that evaluates a sample request (resources,
image, and user roles, with missing values taking the fleet defaults) against the
draft and shows whether the request is Allowed or the statements that reject it. The
test panel SHALL use the same evaluation semantics as enforcement (forbid-only,
defaults applied), so a result in the panel matches what a gateway would decide.

#### Scenario: WEB-PADM-08.01 -- Sample Request Shows Allowed or the Rejecting Statements

- GIVEN a draft that forbids a CPU request above `4`
- WHEN the author tests a sample request with a CPU request of `8`
- THEN the panel SHALL show the request rejected and name the rejecting statement
- AND a sample request within the bounds SHALL show Allowed

---

### Requirement: WEB-PADM-09 -- Preview, Defaults Check, and Secrets Warning

The editor SHALL show a live preview of what users will see: the rendered rules on
the gateway Policies tab (statements the renderer cannot summarize marked "Shown as
Cedar") and the exact denial text for an example request (PLAT-PGP-14). It SHALL
check the draft against the fleet defaults (the default-request check, PLAT-PGP-04)
and block saving when the draft would forbid the defaults. It SHALL warn the author
not to put secrets, credentials, or internal hostnames in `userMessage`, because
every user who hits the rule can read it, and the `userMessage` SHALL be
externalized and localizable (UIS-I18N-04).

#### Scenario: WEB-PADM-09.01 -- Preview Shows Denial Text and Blocks a Defaults-Forbidding Draft

- GIVEN a draft whose rule would forbid the fleet default request
- WHEN the author views the preview
- THEN the preview SHALL show the example denial text and the rendered rules
- AND saving SHALL be blocked because the draft forbids the defaults (PLAT-PGP-04)

#### Scenario: WEB-PADM-09.02 -- Secrets Warning on userMessage

- GIVEN the author edits `userMessage`
- WHEN the field is shown
- THEN the editor SHALL warn against putting secrets, credentials, or internal hostnames in it

---

### Requirement: WEB-PADM-10 -- Hexagonal Boundary and Domain Probes

The admin panel SHALL keep its application workflows and external effects behind
application-owned ports, with React, TanStack Query, the BFF, and the generated SDK
outside the boundary (UIS-HEX). The API adapter SHALL map the policy endpoints to
domain types and map transport errors to typed operation errors (`403` to denied,
`409` to conflicted). The panel SHALL publish typed, versioned domain facts for its
workflows (policy loaded, validation outcome, mode-change confirmed, policy saved,
save denied) through the fan-out probe port (UIS-OBS), with no raw console or direct
telemetry calls and no secret or policy-author credential in a probe payload.

#### Scenario: WEB-PADM-10.01 -- Workflows and Effects Cross a Typed Boundary

- GIVEN the admin saves a policy and the API returns `409`
- WHEN the adapter maps the response
- THEN the panel SHALL receive a typed conflicted operation error, not a raw transport error
- AND the save-outcome domain fact SHALL be published through the probe port without any secret payload

---

### Requirement: WEB-PADM-11 -- Server-State Freshness and Localization

The admin panel SHALL load policies, rollout status, and findings through the
console's server-state layer with explicit staleness, refetch, and invalidation: a
successful policy write or mode change SHALL invalidate the affected list and detail
queries so the view reflects the new state without a full reload. All user-facing
copy SHALL be externalized and localizable (UIS-CONT, UIS-I18N); rendered rule text
and denial previews SHALL be composed from localizable messages rather than
concatenated fragments.

#### Scenario: WEB-PADM-11.01 -- A Write Invalidates and Refreshes the View

- GIVEN the admin changes a policy's mode
- WHEN the write succeeds
- THEN the affected list and detail queries SHALL be invalidated and refetched
- AND the new effective mode SHALL appear without a full-page reload

---

### Requirement: WEB-PADM-12 -- Verification

The admin panel's design-review states SHALL be covered by Storybook stories and
mockup parity stories under `specs/web-console/mockups/admin/`, and the production
pages SHALL pass the mandatory web-console completion gate (mockup-to-production
parity, accessibility, and the non-parity checks). The policies list, the
enforcement-toggle confirmation, the policy detail, and the Cedar editor states
(default, validation problems, add-a-rule snippet, and test request) SHALL each have
a governing story state.

#### Scenario: WEB-PADM-12.01 -- Design-Review States Have Stories and Pass the Gate

- GIVEN the admin panel pages and their mockups
- WHEN the web-console completion gate runs
- THEN each governed state (list, toggle confirmation, policy detail, and the editor states) SHALL have a story
- AND the production pages SHALL pass parity, accessibility, and the non-parity checks
