# Gateway Phase & Health Vocabulary

**Date:** 2026-09-03
**Status:** Active

## Purpose

A Gateway's health is reported through two fields - a lifecycle `phase` and a
human-readable `status` - that flow across three components: the control plane
writes them, the API server persists and exposes them (including a per-phase
metric), and the web console reads them to drive polling and the
ready-to-connect affordance. This spec defines the **canonical vocabulary** for
those values and requires every component to draw from a single shared source of
truth rather than duplicating string literals.

The *behavioral* semantics of each phase (when a gateway becomes `Running`, when
it degrades, how health is continuously reconciled) are defined in
[`openshell-gateway-health.spec.md`](./openshell-gateway-health.spec.md). This
spec is a companion that standardizes the **representation** of that vocabulary
so the phase a client reads means the same thing in every component and cannot
silently drift.

## Domain Vocabulary

A Gateway `phase` SHALL be exactly one of the following canonical values,
written in TitleCase:

| Phase | Meaning (see health spec for full semantics) |
|---|---|
| `Pending` | Accepted, not yet acted on by the reconciler. |
| `Provisioning` | Manifests being applied; workload/exposure not yet Ready. |
| `Running` | Fully serving (workload Ready, and exposure Ready if routed). |
| `Degraded` | Provisioned but currently unhealthy; recoverable. |
| `Failed` | Provisioning could not complete; requires a change to recover. |

`Pending`, `Provisioning`, and `Degraded` are the **recoverable** (non-terminal)
phases; `Running` is the healthy terminal phase and `Failed` is the
non-recoverable terminal phase.

The `status` field is a short human-readable descriptor that complements the
phase. When a gateway is fully healthy, its canonical status value is `Healthy`;
other status values carry a specific reason (e.g. a crash reason or
"route not ready after <window>") and remain human-readable rather than an
enumerated code.

## Requirements

### Requirement: Single Source of Truth for the Phase Vocabulary

The platform SHALL define the canonical Gateway phase vocabulary in exactly one
shared location that both the API server and the control plane import. No
component SHALL hardcode its own independent copy of the phase strings or the
allowed-phase set.

The shared definition SHALL expose, at minimum: the canonical phase constants,
the ordered set of all canonical phases, and a predicate that reports whether an
arbitrary string is a valid canonical phase (case-sensitive).

The canonical Go package is `components/api-server/pkg/gatewayhealth`. The web
console mirrors the same vocabulary via `gatewayCanonicalPhaseStrings` in
`@openshift-online/hypershell-gateway-management-ui`.

#### Scenario: Control plane and API server agree on the vocabulary

- GIVEN the control plane writes a Gateway `phase`
- AND the API server validates and reports that same `phase`
- WHEN a new phase value is added to or removed from the shared definition
- THEN the change SHALL take effect in both components without editing duplicated
  literals in either one

### Requirement: API Server Rejects Unknown Phase Values

The API server SHALL reject create and update requests that set a non-empty
`phase` outside the canonical vocabulary. An absent or empty `phase` SHALL remain
accepted so the field stays optional and pre-existing rows are not locked out.

#### Scenario: Invalid phase rejected on create

- GIVEN a client submits a Gateway create with `phase: "Booting"`
- WHEN the API server processes the request
- THEN the request SHALL be rejected with a validation error

#### Scenario: Absent phase accepted on create

- GIVEN a client submits a Gateway create without a `phase` field
- WHEN the API server processes the request
- THEN the request SHALL be accepted

### Requirement: Metrics Emit Every Canonical Phase

The `hypershell_gateways_total` collector and the BFF metrics proxy SHALL emit
every canonical phase on every scrape/response, defaulting absent phases to zero.

#### Scenario: Pending phase appears in metrics

- GIVEN no Gateways are in the `Pending` phase
- WHEN Prometheus scrapes `hypershell_gateways_total`
- THEN `hypershell_gateways_total{phase="Pending"}` SHALL be present with value `0`

### Requirement: Console Mirrors the Canonical Vocabulary

The web console SHALL derive gateway polling, metrics, and display-status
classification from the same canonical phase set as the Go package, via
`gatewayCanonicalPhases` / `gatewayCanonicalPhaseStrings` in
`gateway-management-ui`.

#### Scenario: New phase surfaces in metrics dashboard

- GIVEN the canonical vocabulary includes `Pending`
- WHEN the BFF returns Prometheus phase counts including `Pending`
- THEN the `GatewayMetricsDashboard` SHALL render a card for that phase

## Cross-References

- [`openshell-gateway-health.spec.md`](./openshell-gateway-health.spec.md) - behavioral health semantics
- [`gateway-metrics-dashboard.spec.md`](./gateway-metrics-dashboard.spec.md) - Prometheus metrics pipeline
