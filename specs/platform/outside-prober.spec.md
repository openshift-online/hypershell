# Outside Availability Prober for the API and Gateways

**Date:** 2026-11-19
**Status:** Draft
**ID prefix:** OBS-PRB
**Related:** [`openshell-gateway-health.spec.md`](./openshell-gateway-health.spec.md) (gateway phase and the internal `/health` reconciler, OSG-HLTH-04), [`gateway-phase-vocabulary.spec.md`](./gateway-phase-vocabulary.spec.md) (phase values), [`data-model.spec.md`](./data-model.spec.md) (Gateway `created_at`, `deleted_at`, `phase`, `route_address`, `cluster_id`, PLAT-DATA), [`gateway-managed-cluster-attribution.spec.md`](./gateway-managed-cluster-attribution.spec.md) (`managed_cluster` label), [`ci-trust-ext-api.spec.md`](./ci-trust-ext-api.spec.md) (keyless Keycloak token exchange precedent, PLAT-CITR), [`../security/rbac-enforcement.spec.md`](../security/rbac-enforcement.spec.md) (prober fleet-read role SEC-RBAC-29), [`api-reliability-metrics.spec.md`](./api-reliability-metrics.spec.md) (in-pod request SLI this prober complements), [`global-architecture.spec.md`](./global-architecture.spec.md) (Cloud Hub topology)

## Purpose

HyperShell measures API reliability only from counters recorded **inside** the
API-server pod ([`api-reliability-metrics.spec.md`](./api-reliability-metrics.spec.md)).
That signal disappears exactly when it matters most: when the pods stop, when a
gateway is unreachable, or when authentication is broken, no request reaches the
in-pod counter, so no Service Level Indicator records the outage and no burn-rate
alert fires. There is also no Service Level Indicator for whether a **gateway**
answers its users.

This spec defines an **outside prober**: a black-box availability probe that runs
on a Cloud Hub, separate from the clusters it measures, and continuously probes
gateways and the production API from the outside. It produces the raw Service
Level Indicator data for two Service Level Objectives defined in the parent
initiative:

- **Gateway availability** - does each gateway that should be running answer the
  OpenShell `Health` call through its public route.
- **API availability**, including an **authentication** signal that request
  counters cannot see, because a `401` is a good event for request counters yet a
  broken login makes the product unusable.

The prober only emits metrics. The Service Level Objective recording rules,
burn-rate alerts, and the worst-gateways dashboard panel live in the platform
GitOps repository and consume the counters this spec defines; they are out of
scope here.

## Domain Vocabulary

- **Target** - the single thing one probe measures: one gateway (for a gateway
  health probe) or one production API instance (for an API read or API
  authentication probe).
- **Probe** - one attempt against one target at one time. A probe is classified
  exactly once, as a **good** probe or a **failed** probe.
- **Production instance** - one deployed, production HyperShell API environment
  the prober is configured to measure. The set of production instances is
  deployment configuration, not an API resource (see OBS-PRB-08).
- **Eligible gateway** - a gateway that the Gateway availability objective counts,
  derived from desired state (see OBS-PRB-02).

## Requirements

### Requirement: OBS-PRB-01 -- Prober Runs Outside the Clusters It Measures

The prober SHALL run on a Cloud Hub (the infrastructure hub), in a failure domain
separate from the clusters it measures: separate from the ManagedClusters that
host the gateways it probes, and separate from the API-server pods it probes. It
SHALL reach every target only through that target's public route or public API
endpoint, exactly as an external user would, so that a probe exercises the same
ingress, routing, and serving path a user depends on.

The prober SHALL NOT read gateway health from in-cluster Kubernetes state, from
the control plane's internal gateway `/health` reconciliation (OSG-HLTH-04), or
from any other in-pod signal. Those signals are blind to the same outages this
prober exists to catch.

The prober's own availability SHALL NOT depend on the clusters it measures: a
total outage of a measured cluster SHALL leave the prober running and emitting
failed probes rather than stopping it.

#### Scenario: OBS-PRB-01.01 -- Measured cluster outage does not stop the prober

- GIVEN the prober is running on the Cloud Hub
- WHEN a ManagedCluster hosting probed gateways becomes completely unreachable
- THEN the prober SHALL continue running
- AND it SHALL record failed probes for every target on that cluster
- AND it SHALL continue probing every reachable target on other clusters

### Requirement: OBS-PRB-02 -- Gateway Eligibility Comes From Desired State

The prober SHALL derive the set of eligible gateways for the Gateway
availability objective from **desired state**, not from observed `phase`. A
gateway SHALL be eligible when all of the following hold:

1. it exists,
2. it is not being deleted (its `deleted_at` is unset; see
   [`data-model.spec.md`](./data-model.spec.md)), and
3. it was created more than 10 minutes ago (`created_at` is older than the
   configured eligibility grace window, default 10 minutes).

The prober SHALL NOT exclude a gateway from the eligible set because its `phase`
is `Degraded` or `Failed`. Those phases are precisely the failures the Gateway
availability objective must count: an eligible gateway that does not answer its
`Health` call SHALL be recorded as a failed probe, whatever its stored `phase`.

A gateway whose `cluster_id` does not resolve to a registered ManagedCluster
SHALL be attributed to `managed_cluster="unknown"` (consistent with
[`gateway-managed-cluster-attribution.spec.md`](./gateway-managed-cluster-attribution.spec.md)),
not dropped.

#### Scenario: OBS-PRB-02.01 -- Degraded gateway stays eligible and counts as failed when it does not answer

- GIVEN a gateway created more than 10 minutes ago, not being deleted, with
  `phase` `Degraded`
- WHEN the prober runs a probe cycle
- AND the gateway does not answer its `Health` call
- THEN the gateway SHALL be in the eligible set
- AND the prober SHALL record a failed gateway health probe for it

#### Scenario: OBS-PRB-02.02 -- Recently created gateway is not yet eligible

- GIVEN a gateway created 2 minutes ago
- WHEN the prober builds the eligible set
- THEN the gateway SHALL NOT be eligible
- AND the prober SHALL NOT record a probe result for it

#### Scenario: OBS-PRB-02.03 -- Deleting gateway is dropped from the eligible set

- GIVEN a gateway whose `deleted_at` is set
- WHEN the prober builds the eligible set
- THEN the gateway SHALL NOT be eligible

### Requirement: OBS-PRB-03 -- Gateway Health Probe

At a fixed interval (default 30 seconds), the prober SHALL call each eligible
gateway's OpenShell `Health` call through the gateway's public route, taken from
its `route_address` (a `grpcs://host:443` address, see
[`data-model.spec.md`](./data-model.spec.md) PLAT-DATA-04). This call needs no
login - the gateway `grpc.health.v1.Health` service is exempt from JWT validation
on the public listener
([`oidc-integration.spec.md`](./oidc-integration.spec.md) SEC-OIDC) - so the
gateway health probe SHALL be unauthenticated.

Each gateway health probe SHALL use a bounded timeout shorter than the probe
interval. A probe SHALL be classified as **good** only when the gateway answers
the `Health` call successfully within the timeout. A transport failure, a
timeout, or an unsuccessful `Health` response SHALL be classified as a **failed**
probe.

#### Scenario: OBS-PRB-03.01 -- Answering gateway is a good probe

- GIVEN an eligible gateway whose public route answers the `Health` call
  successfully within the timeout
- WHEN the prober probes it
- THEN the prober SHALL record a good gateway health probe for that gateway

#### Scenario: OBS-PRB-03.02 -- Unreachable gateway is a failed probe

- GIVEN an eligible gateway whose public route does not answer within the timeout
- WHEN the prober probes it
- THEN the prober SHALL record a failed gateway health probe for that gateway

### Requirement: OBS-PRB-04 -- API Availability Probe

At a fixed interval (default 30 seconds), the prober SHALL issue one real API
read against each production instance. The read SHALL exercise the database read
path so that it detects a database-backed outage. The prober SHALL NOT use
`/healthcheck` (or any liveness endpoint that answers without reading the
database) as the API availability probe, because such an endpoint can answer
while the database read path is down.

An API availability probe SHALL be classified as **failed** only on an HTTP `5xx`
status, a transport failure, or a timeout. Any non-`5xx` response, **including a
`4xx` such as `401`**, SHALL be classified as a **good** probe. This matches the
API availability objective, in which a `4xx` is a good event; an authentication
outage is measured separately by OBS-PRB-05, so it SHALL NOT pull down the API
availability signal.

#### Scenario: OBS-PRB-04.01 -- API 5xx is a failed probe

- GIVEN a production instance whose real API read returns HTTP `503`
- WHEN the prober probes it
- THEN the prober SHALL record a failed API availability probe for that instance

#### Scenario: OBS-PRB-04.02 -- API 401 is a good availability probe

- GIVEN a production instance whose real API read returns HTTP `401`
- WHEN the prober probes it
- THEN the prober SHALL record a good API availability probe for that instance

#### Scenario: OBS-PRB-04.03 -- Unreachable API is a failed probe

- GIVEN a production instance that does not answer within the timeout
- WHEN the prober probes it
- THEN the prober SHALL record a failed API availability probe for that instance

### Requirement: OBS-PRB-05 -- API Authentication Probe

An authentication outage is invisible to request counters, because a `401` is a
good event for them. At a fixed interval (default 30 seconds), the prober SHALL,
for each production instance:

1. obtain an access token from Keycloak using its own identity (OBS-PRB-06), then
2. call one authenticated API read on that instance.

An authentication probe SHALL be classified as **good** only when both steps
succeed and the authenticated read returns a success (`2xx`) status. A failure at
any step - token acquisition fails, the authenticated read returns a non-`2xx`
status (including `401`/`403`), or a transport failure or timeout occurs - SHALL
be classified as a **failed** probe.

The authentication probe SHALL use the same identity used to list gateways and
read their route addresses (OBS-PRB-06).

#### Scenario: OBS-PRB-05.01 -- Full authenticated read is a good probe

- GIVEN Keycloak issues the prober a token
- AND the authenticated API read returns HTTP `200`
- WHEN the prober runs the authentication probe
- THEN the prober SHALL record a good API authentication probe

#### Scenario: OBS-PRB-05.02 -- Token acquisition failure is a failed probe

- GIVEN Keycloak does not issue the prober a token within the timeout
- WHEN the prober runs the authentication probe
- THEN the prober SHALL record a failed API authentication probe
- AND it SHALL NOT call the authenticated API read

#### Scenario: OBS-PRB-05.03 -- Authenticated read rejected is a failed probe

- GIVEN Keycloak issues the prober a token
- AND the authenticated API read returns HTTP `401`
- WHEN the prober runs the authentication probe
- THEN the prober SHALL record a failed API authentication probe

### Requirement: OBS-PRB-06 -- Keyless Workload Identity for the Prober

The prober MUST read all gateways and their route addresses to build its target
set, and it MUST authenticate for the authentication probe. It SHALL do both with
one workload identity and SHALL NOT use a static secret.

The prober SHALL obtain its HyperShell API access token by exchanging its
projected Kubernetes service-account token at Keycloak (workload identity
federation), reusing the platform token-exchange federation that
[`ci-trust-ext-api.spec.md`](./ci-trust-ext-api.spec.md) establishes for keyless
CI. The concrete Keycloak realm wiring - the identity provider, the
token-exchange grant, and the mapping to the prober's role - is realized by the
platform GitOps federation configuration and is out of scope for this spec, which
defines only the HyperShell-side contract: a keyless, short-lived, exchanged
token. The prober token exchange SHALL be config-driven over the same
token-exchange generations fleet Keycloaks run, consistent with the mode
selection in [`ci-trust-ext-api.spec.md`](./ci-trust-ext-api.spec.md)
(PLAT-CITR-08); it SHALL NOT hard-code one Keycloak token-exchange mechanism.

The prober's authorization SHALL be the narrow, read-only fleet role
`fleet:reader` ([`../security/rbac-enforcement.spec.md`](../security/rbac-enforcement.spec.md)
SEC-RBAC-29), which grants read-only visibility of all gateways (enough to list
them and read `id`, `phase`, `cluster_id`, `route_address`, `created_at`, and
`deleted_at`) and nothing else. The prober SHALL NOT be granted `platform:admin`,
which confers delete and access-management authority the prober does not need.

#### Scenario: OBS-PRB-06.01 -- Prober uses an exchanged token, not a static secret

- GIVEN the prober needs to list gateways
- WHEN it acquires a HyperShell API token
- THEN it SHALL exchange its projected Kubernetes service-account token at
  Keycloak for a short-lived access token
- AND it SHALL NOT read a long-lived client secret from configuration

#### Scenario: OBS-PRB-06.02 -- Prober identity cannot mutate gateways

- GIVEN the prober's `fleet:reader` identity
- WHEN it attempts any gateway write or delete, or any access-management grant
- THEN the API server SHALL reject the call as unauthorized

### Requirement: OBS-PRB-07 -- Probe Results Are Exported as Prometheus Counters

The prober SHALL expose its probe results as Prometheus counters in exposition
format on a metrics endpoint that the Cloud Hub Prometheus scrapes. Each counter
SHALL be labelled by target and by result, so a Service Level Objective recording
rule can compute a good-over-valid ratio.

The prober SHALL emit, at minimum:

| Counter | Labels | Meaning |
| --- | --- | --- |
| `hypershell_probe_gateway_health_total` | `gateway_id`, `managed_cluster`, `result` | One gateway health probe (OBS-PRB-03) |
| `hypershell_probe_api_read_total` | `instance`, `result` | One API availability probe (OBS-PRB-04) |
| `hypershell_probe_api_auth_total` | `instance`, `result` | One API authentication probe (OBS-PRB-05) |

The `result` label SHALL take the value `success` for a good probe and `failure`
for a failed probe. The gateway health counter SHALL carry one series per
gateway, keyed by `gateway_id` and `managed_cluster`, so the worst-gateways panel
can rank individual gateways. The `managed_cluster` label SHALL be sourced
consistently with
[`gateway-managed-cluster-attribution.spec.md`](./gateway-managed-cluster-attribution.spec.md).

The prober SHALL increment the matching counter once per probe. It SHALL NOT
synthesize a `success` when a probe did not complete.

The per-gateway `gateway_id` label is a deliberate, bounded exception to the
convention that raw resource identifiers SHALL NOT become metric labels
([`api-server-observability.spec.md`](./api-server-observability.spec.md)
API-OBS-06). That convention governs the API server's own request and trace
telemetry, whose identifier cardinality is unbounded (per request). The prober's
gateway series cardinality is bounded by the eligible-gateway count (fleet size),
and a per-gateway series is required so the worst-gateways panel can rank
individual gateways. The prober SHALL NOT add further unbounded labels (for
example a per-probe identifier or the route address) to these counters.

#### Scenario: OBS-PRB-07.01 -- Gateway probe increments a per-gateway series

- GIVEN a failed gateway health probe for gateway `g-123` on managed cluster
  `hyp0-spoke0`
- WHEN the prober records the result
- THEN it SHALL increment
  `hypershell_probe_gateway_health_total{gateway_id="g-123",managed_cluster="hyp0-spoke0",result="failure"}`

#### Scenario: OBS-PRB-07.02 -- Counters are scrapable by hub Prometheus

- GIVEN the prober is running on the Cloud Hub
- WHEN the Cloud Hub Prometheus scrapes the prober metrics endpoint
- THEN it SHALL read the probe counters in Prometheus exposition format

### Requirement: OBS-PRB-08 -- Probe Cadence, Timeouts, and Targets Are Configured

The probe interval, per-probe timeout, the gateway eligibility grace window, and
the set of production instances SHALL be configuration, not code, so that
operators change them without a code change (consistent with the project
"separate configuration from code" convention).

The per-probe timeout SHALL be shorter than the probe interval, and the interval
and timeout SHALL be chosen so that a newly started outage is reflected as a
failed probe within one minute (OBS-PRB-09). The default probe interval SHALL be
30 seconds and the default eligibility grace window SHALL be 10 minutes.

A single slow or unreachable target SHALL NOT delay probes of other targets
beyond the probe interval; the prober SHALL probe targets concurrently with a
bounded worker count.

#### Scenario: OBS-PRB-08.01 -- One slow target does not stall the cycle

- GIVEN one gateway whose public route hangs until the per-probe timeout
- AND other eligible gateways that answer quickly
- WHEN the prober runs a probe cycle
- THEN the slow gateway SHALL NOT delay recording results for the other gateways
  beyond the probe interval

### Requirement: OBS-PRB-09 -- Outages Are Reflected Within One Minute

For every probe family, a newly started outage SHALL appear as failed probes
within one minute, so a burn-rate alert built on these counters can fire
promptly.

#### Scenario: OBS-PRB-09.01 -- Gateway outage within one minute

- GIVEN an eligible gateway that was answering its `Health` call
- WHEN the gateway stops answering
- THEN the prober SHALL record failed gateway health probes for it within one
  minute

#### Scenario: OBS-PRB-09.02 -- API outage within one minute

- GIVEN a production instance that was serving API reads
- WHEN the API becomes unavailable
- THEN the prober SHALL record failed API availability probes for it within one
  minute

#### Scenario: OBS-PRB-09.03 -- Authentication outage within one minute

- GIVEN a production instance whose authentication was working
- WHEN Keycloak becomes unavailable so the prober cannot obtain a token
- THEN the prober SHALL record failed API authentication probes within one minute

### Requirement: OBS-PRB-10 -- Prober Ships as a Probe Mode of the Control-Plane Image

The prober SHALL ship as a dedicated run mode of the existing control-plane
image, selected by a subcommand of the control-plane binary (for example
`hypershell-controller probe`), rather than as a separate Konflux component. This
mode reuses the generated OpenShell gRPC client the control-plane image already
carries (`components/control-plane/internal/openshellclient`) so the gateway
`Health` probe (OBS-PRB-03) needs no new client. Packaging the prober in the
existing image SHALL avoid adding a new Konflux component, CI registration, and
release-bundle entry.

The probe mode SHALL run only the prober. It SHALL NOT start the reconciler, the
gRPC watch streams, gateway database provisioning, or Helm-based gateway
management; a misconfiguration of those reconciler-only preconditions SHALL NOT
block the probe mode from starting. The probe mode SHALL authenticate with the
keyless prober identity (OBS-PRB-06) and SHALL NOT use the control plane's static
OIDC client secret.

#### Scenario: OBS-PRB-10.01 -- Probe mode runs the prober and not the reconciler

- GIVEN the control-plane image started in its probe mode
- WHEN the process comes up
- THEN it SHALL run the prober probe loops (OBS-PRB-03 to OBS-PRB-05) and serve
  the metrics endpoint (OBS-PRB-07)
- AND it SHALL NOT start the reconciler, the gRPC watch streams, gateway database
  provisioning, or Helm-based gateway management

#### Scenario: OBS-PRB-10.02 -- Probe mode does not require reconciler-only configuration

- GIVEN the control-plane image started in its probe mode without the gateway
  database admin credentials the reconciler requires
- WHEN the process comes up
- THEN the probe mode SHALL start and probe normally
- AND it SHALL NOT fail on the reconciler-only startup preconditions

### Requirement: OBS-PRB-11 -- Repository Provides a Kustomize Base for the Prober

This repository SHALL provide a Kustomize base (for example under `deploy/`) that
the platform GitOps repository (hypershell-gitops) consumes to deploy the prober
on the infrastructure hub. The base SHALL render with `kustomize` and SHALL pass
the repository policy checks (`make check`).

The base SHALL declare:

1. a Deployment that runs the control-plane image in its probe mode (OBS-PRB-10)
   with a restricted pod security context (`runAsNonRoot`, drop `ALL`
   capabilities), consistent with the project "Restricted SecurityContext on all
   containers" convention;
2. a ServiceAccount with a projected service-account token volume whose audience
   is configurable, so the prober can exchange that token at Keycloak
   (OBS-PRB-06);
3. a ConfigMap carrying the prober configuration (probe interval, per-probe
   timeout, eligibility grace window, production instances, and Keycloak
   token-exchange settings) defined in OBS-PRB-08;
4. a Service that exposes the metrics port the Cloud Hub Prometheus scrapes
   (OBS-PRB-07); and
5. readiness and liveness probes for the prober container.

The Kustomize base SHALL NOT embed environment-specific secrets or the Keycloak
realm wiring; that configuration is realized by hypershell-gitops (OBS-PRB-06 and
the Non-Goals below).

#### Scenario: OBS-PRB-11.01 -- Prober base renders and passes policy checks

- GIVEN the prober Kustomize base in this repository
- WHEN `kustomize build` renders the base and the repository policy checks run
- THEN the base SHALL render a Deployment, ServiceAccount, ConfigMap, and Service
  for the prober
- AND the render SHALL pass the repository policy checks

#### Scenario: OBS-PRB-11.02 -- Prober Deployment uses a restricted security context and a projected token

- GIVEN the rendered prober Deployment
- WHEN its pod template is inspected
- THEN the container SHALL set `runAsNonRoot` and drop `ALL` capabilities
- AND the pod SHALL mount a projected service-account token volume with a
  configurable audience for the Keycloak token exchange (OBS-PRB-06)

## Non-Goals

- Service Level Objective recording rules, burn-rate alerts, response-label
  routing, and the worst-gateways dashboard panel. These live in the platform
  GitOps repository and consume the counters defined here.
- A deep gateway probe that checks the gateway database or the gateway Keycloak
  client (explicitly out of scope in the parent initiative).
- A heartbeat that detects a broken alert path (an open question in the parent
  initiative).
- The specific Keycloak realm wiring for the prober's token exchange, which is
  realized by GitOps federation configuration (OBS-PRB-06).
- Replacing the in-pod API request SLI in
  [`api-reliability-metrics.spec.md`](./api-reliability-metrics.spec.md); the
  prober complements it, it does not replace it.
- Moving the control plane off its static OIDC client secret to tokenless
  identity (a parent-initiative follow-up after this prober identity design).
