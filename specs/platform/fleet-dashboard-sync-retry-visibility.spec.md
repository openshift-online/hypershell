# Fleet Dashboard Sync-Retry Visibility

**Status:** Active
**Applies to:** `components/fleet-dashboard` BFF promotion source, `packages/fleet-dashboard-ui` promotion domain and map presentation

## Purpose

Surface the Argo CD sync-attempt (retry) count for a promotion gate's source
environment in the fleet dashboard's gate / analysis-run details pane, so an
operator can tell a wedged gate apart from a slow one. When a release-analysis
gate sits "pending", the dashboard today gives no signal to distinguish a healthy
analysis that is still running from an Argo sync operation that is looping on a
PostSync hook that can never pass. The retry count is the key diagnostic and is
already reachable, so this specification threads it from the Argo CD
`Application` the BFF already fetches through to the gate pane as a single
nullable integer, labelled and explained in plain English.

### Why a wedged gate looks like a pending one

The source environment's PostSync release-analysis hook validates that the
revision it checks is still the deployed one. When several bundle-update commits
land on an `env/<instance>` branch faster than one release-analysis completes,
the environment's synced revision advances past the in-flight operation's
revision; the hook then fails every attempt, its Job exhausts its backoff limit,
and the Argo sync operation retries on its own backoff. During that window the
gate reports "pending" with no on-dashboard signal that it is actually looping.
A visible, climbing retry count ("Sync attempts: 9") identifies the wedge
immediately.

### Data reachability (scope boundary)

| Source | Reachable | Notes |
| --- | --- | --- |
| Argo CD `Application` `status.operationState.retryCount` | Yes | A clean integer on the Application the BFF already fetches in its promotion source; no new Kubernetes query or cluster credential is needed. |
| Argo Rollouts `AnalysisRun` measurement / metric-retry counts | No | These live imperatively on the spoke clusters; the infra hub where the BFF runs has no `AnalysisRun` resources and only a hub Kubernetes client. Surfacing them would require new per-spoke cluster access and is out of scope. |

## Requirements

### Requirement: BFF reads the Argo sync-retry count

The fleet-dashboard BFF promotion source SHALL read
`status.operationState.retryCount` from each core Argo CD `Application` it already
resolves per instance, using the same already-fetched `Application` object that
supplies `status.health.status` and `status.sync.status`. The BFF SHALL NOT
introduce any new Kubernetes query, list, watch, or cluster credential to obtain
this value.

#### Scenario: Retry count present on the Application

- GIVEN an instance whose core Argo CD `Application` is resolved by the promotion source
- AND the Application reports `status.operationState.retryCount: 9`
- WHEN the BFF builds that environment
- THEN the environment's `syncRetries` field SHALL be `9`

#### Scenario: No new cluster access

- GIVEN the BFF already fetches the core Argo CD `Application` per instance
- WHEN the sync-retry count is added to the environment
- THEN the value SHALL be read from that already-fetched Application
- AND no additional Kubernetes query or credential SHALL be introduced

### Requirement: Absent sync data is nullable, not zero

The BFF SHALL represent "no operation or unknown retry data" as a null
`syncRetries`, distinct from a genuine `0` (synced on the first attempt). The
`syncRetries` field SHALL be omitted from the JSON payload when null.

#### Scenario: Synced on first attempt

- GIVEN an Application whose `status.operationState.retryCount` is `0`
- WHEN the BFF builds that environment
- THEN `syncRetries` SHALL be `0`

#### Scenario: No operation state available

- GIVEN an Application with no `status.operationState` (no recorded operation)
- WHEN the BFF builds that environment
- THEN `syncRetries` SHALL be null
- AND the field SHALL be omitted from the JSON payload

### Requirement: The UI carries the count onto the gate

The fleet-dashboard UI SHALL map the BFF's `syncRetries` field through the wire
adapter, the promotion domain, and the map model so that a gate carries the
sync-retry count of its source environment. A gate reports the source
environment's own analysis, so the source environment's sync retries SHALL belong
to that gate. The value SHALL remain nullable (`number | null`) end to end,
preserving the distinction between `0` and unknown.

#### Scenario: Gate inherits its source environment's retry count

- GIVEN a source environment whose `syncRetries` is `9`
- WHEN the map model builds the gate that reports that source environment
- THEN the gate SHALL carry `syncRetries` equal to `9`

#### Scenario: Unknown retry count stays null through the mapping

- GIVEN a source environment whose `syncRetries` is absent
- WHEN the wire adapter, domain, and map model process it
- THEN the gate's `syncRetries` SHALL be null at every layer

### Requirement: The gate pane shows "Sync attempts" with a plain-English tooltip

The gate details pane SHALL render a "Sync attempts" row showing the gate's
`syncRetries` value only when that value is not null. The row SHALL be omitted
(not rendered as "0" or blank) when the value is null. The row label SHALL carry
an accessible, plain-English tooltip that explains what the count means and that
a climbing value usually signals a wedged release-analysis gate (a newer commit
superseded the one being checked) rather than tests still running. The label and
tooltip strings SHALL be defined as localized messages.

#### Scenario: Count shown with tooltip

- GIVEN a selected gate whose `syncRetries` is `9`
- WHEN the operator views the gate details pane
- THEN the pane SHALL show a "Sync attempts" row reading `9`
- AND the row label SHALL expose a tooltip explaining the count and that a climbing value signals a wedged gate

#### Scenario: Row omitted when unknown

- GIVEN a selected gate whose `syncRetries` is null
- WHEN the operator views the gate details pane
- THEN no "Sync attempts" row SHALL be rendered

## Out of Scope

- `AnalysisRun`-level measurement or metric-retry counts (require spoke-cluster
  access the BFF does not have).
- Any change to the promotion or retry mechanics themselves; the livelock
  root-cause is a separate concern.
