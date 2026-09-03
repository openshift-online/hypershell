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

#### Scenario: Control plane and API server agree on the vocabulary

- GIVEN the control plane writes a Gateway `phase`
- AND the API server validates and reports that same `phase`
- WHEN a new phase value is added to or removed from the shared definition
- THEN the change SHALL take effect in both components without editing duplicated
  literals in either one

### Requirement: API Server Rejects Unknown Phase Values

The API server SHALL reject any write (REST create/patch or gRPC
create/update) that sets a Gateway `phase` to a value outside the canonical
vocabulary, returning a validation error (HTTP 400 / gRPC `InvalidArgument`). An
absent or empty `phase` SHALL be accepted, so the field remains optional and a
caller that does not set it is unaffected.

#### Scenario: Unknown phase rejected on gRPC update

- GIVEN a client calls `UpdateGateway` with `phase` set to `"Booting"`
- WHEN the API server validates the request
- THEN it SHALL reject the request with `InvalidArgument`
- AND it SHALL NOT persist the value

#### Scenario: Canonical phase accepted

- GIVEN the control plane calls `UpdateGateway` with `phase` set to `"Running"`
- WHEN the API server validates the request
- THEN it SHALL accept the request and persist the value

#### Scenario: Absent phase accepted

- GIVEN a client creates a Gateway without setting `phase`
- WHEN the API server validates the request
- THEN it SHALL accept the request

### Requirement: Phase Metric Covers the Full Canonical Set

The API server's per-phase gateway gauge SHALL pre-seed and report every
canonical phase, so graphs never omit a phase. The set of phases the metric
reports SHALL be derived from the shared vocabulary rather than an independent
hardcoded list.

#### Scenario: Every canonical phase appears in the metric

- GIVEN no gateways exist in the `Pending` phase
- WHEN the gateway phase metric is scraped
- THEN the gauge SHALL still emit a `Pending` series with value `0`
- AND it SHALL emit a series for every other canonical phase

### Requirement: Control Plane Uses the Canonical Vocabulary

The control plane SHALL derive every Gateway `phase` value it reads (the phase
gate) or writes (provisioning path and continuous health reconciliation) from
the shared vocabulary, and SHALL use the canonical `Healthy` status constant for
the fully-healthy case, rather than duplicating string literals.

#### Scenario: Health reconciler writes canonical values

- GIVEN the control plane observes a gateway workload return to Ready
- WHEN it writes the recovered health back to the API server
- THEN the `phase` it writes SHALL be the canonical `Running` value
- AND the `status` it writes SHALL be the canonical `Healthy` value

### Requirement: Console Vocabulary Aligns with the Canonical Phases

The web console SHALL recognize the canonical phase set as its source of truth
for classifying a gateway as recoverable (keep polling), healthy, or terminally
failed, so its polling and ready-to-connect decisions stay consistent with the
phases the control plane actually emits. The console MAY additionally tolerate
broader status descriptors, but its canonical phase classification SHALL match
this vocabulary.

#### Scenario: Recoverable phase keeps polling

- GIVEN a Gateway displayed with a canonical recoverable phase
  (`Pending`, `Provisioning`, or `Degraded`)
- WHEN the console evaluates whether to poll for status
- THEN it SHALL continue polling

#### Scenario: Failed phase stops polling

- GIVEN a Gateway displayed with the canonical `Failed` phase
- WHEN the console evaluates whether to poll for status
- THEN it SHALL treat the gateway as terminally failed and stop polling
