# E2E Testing and CI Integration

**Date:** 2026-08-10
**Status:** Draft
**Jira:** HYPERSHELL-18
**Related:** `local-development.spec.md` -- Kind cluster setup;
             `openshift-development.spec.md` (HYPERSHELL-44) -- OpenShift e2e driver, `make openshift-*` lifecycle, blessed `deploy/openshift/` overlay, cluster infrastructure bootstrap, OpenShift CI;
             `control-plane.spec.md` -- reconciler behavior;
             `openshell-gateway-routing.spec.md` -- GRPCRoute provisioning;
             `openshell-gateway-namespace-gc.spec.md` -- gateway deletion + namespace GC;
             `openshell-gateway-sandbox-count.spec.md` -- active sandbox count accounting

## Purpose

HyperShell requires infrastructure-agnostic end-to-end testing that validates the full provisioning path: API creation of a Gateway, control plane reconciliation, gateway pod readiness, route connectivity, and sandbox lifecycle. The same test suite SHALL run against Kind (local development, CI) and OpenShift (manual, on-demand runs against any OpenShift cluster) with infrastructure-specific logic isolated into driver scripts. A CI workflow SHALL execute these tests automatically on pull requests that modify e2e-relevant components.

The existing e2e test (`components/pr-test/e2e-openshell.sh`) validates 6 areas -- gateway provisioning, infrastructure verification, route discovery, connectivity, sandbox lifecycle, and sandbox interaction -- but is hardcoded for OpenShift. This spec defines the driver abstraction, CI workflow, and deploy restructuring required to run the same tests across multiple infrastructure targets.

HyperShell also needs a **performance test**. The performance test measures how the platform behaves under scale. It provisions a large fleet of gateways on the target cluster. It then runs the functional e2e suite to confirm the platform still works correctly under that load. The performance test reuses the same driver abstraction, so it runs against Kind for local checks and against any OpenShift cluster for on-demand load tests (see [Performance Testing](#performance-testing)).

### Scope

This spec covers the **e2e driver interface contract** (for all targets), the **Kind driver**, the **Kind-based CI workflow**, and the **infra-agnostic performance test**.

The **OpenShift e2e driver**, the `make openshift-*` lifecycle, the **blessed `deploy/openshift/` overlay**, the **cluster infrastructure bootstrap**, and the **OpenShift CI workflow** are specified in `openshift-development.spec.md` (HYPERSHELL-44). This spec does not duplicate them. It requires the OpenShift e2e driver so a user can run `make e2e` and `make e2e-performance` **manually** against any OpenShift cluster the user is already logged in to (via `oc login`). That is the target environment for scale and performance testing.

An automated OpenShift CI job is **out of scope** here; it belongs to HYPERSHELL-44. In this spec, OpenShift runs are manual and on-demand, and only the Kind e2e workflow runs in CI.

## Architecture

```
tests/e2e/e2e-openshell.sh (infra-agnostic test logic)
    │
    ├── sources tests/e2e/lib.sh (shared utilities)
    │
    └── sources driver via E2E_INFRA_DRIVER (required)
        │
        ├── tests/e2e/drivers/kind.sh         (this spec)
        └── tests/e2e/drivers/openshift.sh    (this spec)
```

The driver model separates test logic from infrastructure mechanics. The main test script calls a fixed set of driver functions; each driver implements those functions for its target infrastructure. Adding a new infrastructure target requires only a new driver file.

### Driver Interface

Each driver exports shell functions that abstract infrastructure-specific operations. The table maps each function to the OpenShift-specific construct it replaces in the current `e2e-openshell.sh`:

| Function | Purpose | Kind Implementation | OpenShift Implementation |
|----------|---------|---------------------|--------------------------|
| `discover_api_host` | Find the HyperShell API server URL | HTTPRoute hostname `api.hypershell.localhost` or port-forward to `svc/hypershell-api-server` | `oc get route hypershell-api -o jsonpath='{.spec.host}'` |
| `discover_gateway_endpoint` | Find the gateway gRPC endpoint | GRPCRoute hostname `<gw-name>.gw.localhost` via Gateway status address | Route with `spec.tls.termination=passthrough` targeting `svc/openshell-gateway` |
| `get_cluster_domain` | Get the base domain for constructing gateway DNS names | `gw.localhost` (static, matching `GATEWAY_API_BASE_DOMAIN` in `deploy/kind/`) | `oc get ingresses.config.openshift.io cluster -o jsonpath='{.spec.domain}'` |
| `get_cli_binary` | Return the Kubernetes CLI binary path | `kubectl` | `oc` |
| `wait_for_gateway_route` | Block until the gateway is externally reachable | Check Gateway API Gateway status conditions and GRPCRoute parent status | Check OpenShift Route `.status.ingress[].conditions` for `Admitted` |

### CI Pipeline

Images are built by Konflux, not by the CI workflow. The e2e workflow gates on Konflux builds completing, then pulls those images by digest into the Kind cluster. This avoids duplicating the build step and ensures the images tested in CI are identical to the images that ship.

```
PR opened/updated
    │
    ├── Konflux builds changed component images (existing pipeline)
    │
    ├── e2e workflow triggers (gates on Konflux build completion)
    │
    ├── detect-changes (reuse .github/scripts/detect-components.sh)
    │
    ├── [skip if no e2e-relevant components changed]
    │
    ├── make kind-up (create Kind cluster with baseline images, overlapping Konflux builds)
    │
    ├── wait for each changed component's Konflux on-pull-request build to conclude
    │
    ├── scripts/kind/set-component-images.sh (swap in Konflux-built images by digest)
    │
    ├── E2E_INFRA_DRIVER=kind bash tests/e2e/e2e-openshell.sh
    │
    ├── [on failure: collect diagnostic artifacts]
    │
    └── report CI status
```

### Deploy Structure

```
deploy/
  base/                     ← shared resources (all infra targets)
    kustomization.yaml
    namespace.yaml
    postgres.yaml
    api-server.yaml
    controller.yaml
    controller-rbac.yaml
    web-console.yaml
    keycloak/               ← Keycloak deployment + realm import + theme
      kustomization.yaml
      keycloak.yaml
      namespace.yaml
    certificates/           ← CA chain for TLS
      kustomization.yaml
      ca-chain.yaml
    networkpolicies.yaml
  kind/                     ← Kind overlay (extends base)
    kustomization.yaml      ← references ../base, patches for OIDC + Kind env
    kind-config.yaml
    certificates.yaml       ← cert-manager Certificates + Issuers
    gateway.yaml            ← networking Gateway (cloud-provider-kind)
    httproutes.yaml         ← HTTPRoutes for component services
    oidc-secrets.yaml       ← control-plane OIDC client secret
    coredns/
      Corefile
    infrastructure/         ← CRDs + controllers (cert-manager, Gateway API, Agent Sandbox)
      kustomization.yaml
  openshift/                ← OpenShift overlay (extends base)
    kustomization.yaml      ← references ../base
    route.yaml
    scc.yaml
    certificates.yaml
    networkpolicies.yaml
    infrastructure/
      kustomization.yaml
      gatewayclass.yaml
```

## Requirements

### Requirement: Infra Driver Abstraction

The e2e test framework SHALL isolate infrastructure-specific logic into driver scripts located at `tests/e2e/drivers/<driver>.sh`. The main test script (`tests/e2e/e2e-openshell.sh`) SHALL remain infrastructure-agnostic and call only driver interface functions for infrastructure-specific operations. The `E2E_INFRA_DRIVER` environment variable SHALL select the driver and MUST be set -- the script SHALL exit with an error if it is unset.

#### Scenario: Kind Driver Selected

- GIVEN `E2E_INFRA_DRIVER=kind`
- WHEN the e2e test script starts
- THEN the `tests/e2e/drivers/kind.sh` driver SHALL be sourced
- AND all infrastructure functions SHALL use `kubectl` and Kind-specific discovery (HTTPRoute hostnames, Gateway API status)

#### Scenario: OpenShift Driver Selected

- GIVEN `E2E_INFRA_DRIVER=openshift`
- WHEN the e2e test script starts
- THEN the `tests/e2e/drivers/openshift.sh` driver SHALL be sourced
- AND all infrastructure functions SHALL use `oc` and OpenShift-specific discovery (Routes, `ingresses.config.openshift.io`)

#### Scenario: New Driver Extensibility

- GIVEN a developer creates `tests/e2e/drivers/eks.sh` implementing all interface functions
- WHEN a user runs the e2e tests with `E2E_INFRA_DRIVER=eks`
- THEN the tests SHALL execute against EKS without modifying the main test script

#### Scenario: Driver Not Set

- GIVEN `E2E_INFRA_DRIVER` is not set
- WHEN the e2e test script starts
- THEN the script SHALL exit with a non-zero status
- AND print a message listing available drivers from `tests/e2e/drivers/*.sh`

#### Scenario: Unknown Driver

- GIVEN `E2E_INFRA_DRIVER=nonexistent`
- WHEN the e2e test script starts
- THEN the script SHALL exit with a non-zero status
- AND print available drivers by listing `tests/e2e/drivers/*.sh`

### Requirement: Driver Interface Contract

Each driver script SHALL export the following shell functions. The main test script SHALL call only these functions for infrastructure-specific operations. A driver that does not implement all required functions SHALL cause the test script to exit with an error at startup. This spec defines the contract for all drivers and covers the Kind driver implementation. The OpenShift implementation of this contract (the `oc` commands, Route discovery, cluster-domain lookup, and OIDC issuer derivation) is specified in `openshift-development.spec.md` (HYPERSHELL-44); the table below is the contract it implements.

#### Scenario: API Host Discovery -- Kind

- GIVEN the `kind` driver is active
- WHEN `discover_api_host` is called
- THEN it SHALL return `api.hypershell.localhost` (the HTTPRoute hostname from `deploy/kind/httproutes.yaml`)
- OR fall back to `localhost:<port>` via `kubectl port-forward svc/hypershell-api-server` if the HTTPRoute is not reachable

#### Scenario: Gateway Endpoint Discovery -- Kind

- GIVEN the `kind` driver is active
- AND a gateway named `$GW_NAME` exists in namespace `$GW_NAMESPACE`
- WHEN `discover_gateway_endpoint` is called
- THEN it SHALL return `https://<gw-name>.gw.localhost:443` derived from the GRPCRoute hostname and the networking Gateway's status address

#### Scenario: Cluster Domain -- Kind

- GIVEN the `kind` driver is active
- WHEN `get_cluster_domain` is called
- THEN it SHALL return `gw.localhost` (matching the `GATEWAY_API_BASE_DOMAIN` environment variable set on the control plane in the Kind deployment)

#### Scenario: CLI Binary -- Kind

- GIVEN the `kind` driver is active
- WHEN `get_cli_binary` is called
- THEN it SHALL return `kubectl`

#### Scenario: Wait for Gateway Route -- Kind

- GIVEN the `kind` driver is active
- AND a gateway named `$GW_NAME` has been provisioned
- WHEN `wait_for_gateway_route` is called
- THEN it SHALL poll the Gateway API Gateway resource's status conditions for `Programmed=True`
- AND verify the corresponding GRPCRoute's parent status reports `Accepted=True`
- AND return success when both conditions are met or fail after `E2E_PROVISION_TIMEOUT` seconds

The OpenShift driver implements the same five functions with OpenShift constructs (Route host for `discover_api_host`, passthrough Route for `discover_gateway_endpoint`, `ingresses.config.openshift.io` for `get_cluster_domain`, `oc` for `get_cli_binary`, Route `Admitted` + GRPCRoute `Accepted` for `wait_for_gateway_route`), as the interface table above shows. The OpenShift driver is specified in `openshift-development.spec.md` (HYPERSHELL-44). This spec depends on it for the manual OpenShift runs defined next.

### Requirement: Custom OpenShift Runs

A user SHALL be able to run `make e2e` and `make e2e-performance` **manually** against any OpenShift cluster, so scale and performance testing can target a real OpenShift environment. Each target SHALL default `E2E_INFRA_DRIVER` to `kind` and SHALL honor a command-line override. A user SHALL run `E2E_INFRA_DRIVER=openshift make e2e` or `E2E_INFRA_DRIVER=openshift make e2e-performance` against the cluster their current `oc` context selects. These runs SHALL NOT create a cluster and SHALL NOT create a namespace beyond the gateways the suite provisions; the environment is a precondition.

**Preconditions (owned by HYPERSHELL-44).** These runs assume HyperShell is already deployed on the cluster through `kustomize build deploy/openshift/` (for example via `make openshift-up`), and that the cluster infrastructure bootstrap (shared Gateway, GatewayClass, certificate issuer, wildcard certificate) is in place per `openshift-development.spec.md`. The suite SHALL fail with a clear error, not a broken run, when the API Route or the gateway infrastructure is absent.

**Driver behavior needed for parity.** For the shared suite to pass on OpenShift, the OpenShift driver SHALL derive the OIDC issuer and related OIDC variables from the running cluster's domain (not the Kind default `keycloak.hypershell.localhost`), and SHALL provide the same Keycloak admin and role-assignment helpers the Kind driver provides, so the RBAC areas (developer and platform-admin) run unchanged. The OpenShift deployment SHALL enforce RBAC (`RBAC_ENFORCE=true`) and SHALL keep the OpenShift SCC posture (per-namespace privileged SCC for sandbox pods), so the sandbox and RBAC areas behave the same as on Kind. These behaviors are specified in HYPERSHELL-44; this spec only depends on them.

**Namespace GC timing.** Area 8 exercises the periodic namespace reaper. To make it pass on OpenShift without waiting the production GC defaults (5m sweep / 10m grace), the OpenShift deployment SHOULD set shortened `GATEWAY_NAMESPACE_GC_INTERVAL` and `GATEWAY_NAMESPACE_GC_GRACE_PERIOD` (as the Kind overlay does), or the user SHOULD raise `E2E_ORPHAN_GC_TIMEOUT` and `E2E_GC_TIMEOUT` to fit the cluster's configured timing.

**Not in CI.** OpenShift e2e and performance runs SHALL NOT be wired into CI in this iteration (see [Design Decisions](#design-decisions)). CI runs the Kind e2e workflow only.

#### Scenario: e2e Against OpenShift

- GIVEN a user is logged in to an OpenShift cluster with HyperShell deployed (`deploy/openshift/`)
- AND the cluster infrastructure bootstrap is in place per `openshift-development.spec.md`
- WHEN the user runs `E2E_INFRA_DRIVER=openshift make e2e`
- THEN the suite SHALL run against that cluster using the OpenShift driver
- AND no Kind cluster SHALL be created

#### Scenario: Performance Against OpenShift

- GIVEN a user is logged in to an OpenShift cluster with HyperShell deployed
- WHEN the user runs `E2E_INFRA_DRIVER=openshift make e2e-performance`
- THEN the performance harness SHALL run against that cluster using the OpenShift driver
- AND it SHALL provision the perf fleet on that cluster and report metrics

#### Scenario: Missing Environment Fails Fast

- GIVEN an OpenShift cluster where the HyperShell API Route or gateway infrastructure is absent
- WHEN the user runs `E2E_INFRA_DRIVER=openshift make e2e`
- THEN the suite SHALL fail with a clear error about the missing environment
- AND it SHALL NOT report false passes

#### Scenario: OpenShift Not in CI

- GIVEN the CI configuration for this iteration
- WHEN a pull request runs the e2e workflow
- THEN it SHALL run only the Kind e2e job
- AND it SHALL NOT run e2e or performance tests against OpenShift

### Requirement: E2E Test Suite Coverage

The e2e test suite SHALL validate the following 8 areas, extending the original test structure from `components/pr-test/e2e-openshell.sh`. All test areas SHALL be infrastructure-agnostic -- they call driver functions for infra-specific operations and use the Kubernetes API for resource inspection.

1. **Gateway provisioning via HyperShell API** -- create a gateway via the REST API and wait for the control plane to reconcile it to `Running` phase
2. **Gateway infrastructure verification** -- confirm the gateway deployment, service, TLS secret, and certgen job exist and are healthy
3. **Route discovery + openshell CLI registration** -- discover the gateway endpoint via the driver, register it with the openshell CLI
4. **Gateway connectivity** -- verify the openshell CLI can connect to the gateway and report status (over trusted TLS, no insecure bypass -- see Gateway TLS Trust)
5. **Sandbox lifecycle** -- create a sandbox as the admin user, wait for the pod to reach `Running` state, and verify the gateway's `active_sandbox_count` accounting reflects sandbox create and delete (see Active Sandbox Count Accounting)
6. **Sandbox interaction** -- execute commands inside the sandbox (`uname -a`, `ls /workspace`)
7. **Developer user RBAC verification** -- authenticate as the `developer` user (the `openshell-user` tier) and confirm it MAY create a sandbox but MAY NOT create a gateway via the HyperShell API (see Developer RBAC Enforcement)
8. **Gateway deletion + namespace garbage collection** -- validate both garbage-collection paths from `openshell-gateway-namespace-gc.spec.md`: (a) seed a synthetic orphaned managed namespace after gateway provisioning and validate periodic `NamespaceGCReconciler` reap + `GarbageCollected` Event in step 11 (while steps 3–10 run in parallel with the reaper); (b) delete-driven reap of the gateway's managed namespace (see Gateway Deletion and Namespace GC)

The admin-user OIDC flow that authenticates areas 1--6 and 8 is validated separately (see OIDC Authentication in E2E Tests).

#### Scenario: Full Suite Execution

- GIVEN a running HyperShell environment (Kind or OpenShift)
- WHEN the e2e test suite runs
- THEN all 8 test areas SHALL be executed in sequence
- AND results SHALL be reported as pass/fail counts with per-test detail

#### Scenario: Gateway Provisioning

- GIVEN the HyperShell API is reachable (via `discover_api_host`)
- WHEN a gateway is created via `POST /api/hypershell/v1/gateways`
- THEN the test SHALL poll the API until the gateway phase is `Running` or `E2E_PROVISION_TIMEOUT` seconds have elapsed
- AND a timeout SHALL be reported as a test failure

#### Scenario: Infrastructure Verification

- GIVEN a gateway has reached `Running` phase
- WHEN the test verifies infrastructure
- THEN it SHALL confirm: deployment `openshell-gateway` exists and has at least 1 ready replica, service `openshell-gateway` exists with a ClusterIP, TLS secret `openshell-server-tls` exists, certgen job `openshell-gateway-certgen` has succeeded

#### Scenario: Sandbox Lifecycle

- GIVEN the openshell CLI is connected to the gateway
- WHEN `sandbox create --name <name>` is invoked
- THEN a pod matching `default--<name>` SHALL appear in the gateway namespace
- AND the pod SHALL reach `Running` state within `E2E_SANDBOX_TIMEOUT` seconds

### Requirement: Active Sandbox Count Accounting

The e2e test suite SHALL validate that a gateway's read-only `active_sandbox_count`
field, reported by the HyperShell API, tracks sandbox creation and deletion. The
suite SHALL create two to three sandboxes on a `Running` gateway, poll the API
until `active_sandbox_count` reflects the created sandboxes, then delete one or
more and poll until the count decrements accordingly. The suite SHALL reuse the
existing sandbox create/delete steps and the API-polling helper. Because the
count is an advisory recent value that may lag real time (see
`openshell-gateway-sandbox-count.spec.md`), assertions SHALL poll up to
`E2E_SANDBOX_TIMEOUT` seconds for the expected value rather than checking once.

#### Scenario: Count increments as sandboxes are created

- GIVEN a `Running` gateway with `active_sandbox_count` of 0
- WHEN two to three sandboxes are created on that gateway and their pods reach
  `Running` state
- THEN polling `GET /api/hypershell/v1/gateways/<id>` SHALL report an
  `active_sandbox_count` equal to the number of sandboxes created, within
  `E2E_SANDBOX_TIMEOUT` seconds
- AND a timeout without the expected count SHALL be reported as a test failure

#### Scenario: Count decrements as sandboxes are deleted

- GIVEN a `Running` gateway whose `active_sandbox_count` reflects the created
  sandboxes
- WHEN one or more of those sandboxes are deleted and their pods terminate
- THEN polling the API SHALL report an `active_sandbox_count` reduced by the
  number of sandboxes deleted, within `E2E_SANDBOX_TIMEOUT` seconds
- AND the suite SHALL delete any remaining sandboxes to leave a clean state

### Requirement: Developer RBAC Enforcement

The e2e test suite SHALL verify the RBAC boundary of the `openshell-user` tier by exercising both an operation it is allowed to perform and one it is not. The `developer` user (credentials `E2E_DEV_USERNAME` / `E2E_DEV_PASSWORD`) maps to `gateway:viewer` -> `openshell-user` per `specs/security/rbac-enforcement.spec.md`. This tier is a legitimate *user* of a gateway it can reach: it MAY create sandboxes on that gateway (the `openshell-user` role is authorized for sandbox create/list/exec per `specs/platform/openshell-gateway-oidc.spec.md`), but it is NOT a `gateway:creator`, so it MUST NOT be able to create gateways via the HyperShell API. The suite SHALL assert both halves -- the allowed operation succeeds and the denied operation returns `403 Forbidden`.

#### Scenario: Openshell User May Create a Sandbox

- GIVEN a valid OIDC token has been acquired for the `developer` user
- AND the openshell CLI is registered against the gateway with that token
- WHEN `sandbox create` is invoked
- THEN a sandbox pod matching `default--<name>` SHALL be created within `E2E_SANDBOX_TIMEOUT` seconds
- AND the test SHALL record a pass and delete the sandbox to leave a clean state

#### Scenario: Openshell User May Not Create a Gateway

- GIVEN a valid OIDC token has been acquired for the `developer` user
- WHEN the developer calls `POST /api/hypershell/v1/gateways` with that token
- THEN the API SHALL return `403 Forbidden` (the developer lacks the platform-scoped `gateway:creator` role)
- AND the test SHALL record a pass for the denial

#### Scenario: Unexpected Success Is a Failure

- GIVEN the `developer` user attempts to create a gateway
- WHEN the API returns a 2xx status despite the missing `gateway:creator` role
- THEN the test SHALL record a failure (RBAC not enforced)
- AND the test SHALL delete the erroneously-created gateway to leave a clean state

### Requirement: Gateway Deletion and Namespace GC

The e2e test suite SHALL validate both garbage-collection paths described in
`openshell-gateway-namespace-gc.spec.md`:

1. **Periodic reaper** -- the `NamespaceGCReconciler` sweeps managed namespaces
   with no live Gateway, respects the grace period, records a `GarbageCollected`
   Kubernetes Event in the control-plane namespace via `recordGCEvent`, then
   deletes the namespace.
2. **Delete-driven reap** -- deleting a Gateway through the HyperShell API drives
   the control plane to remove the gateway record and reap its managed namespace
   (`DeleteManagedNamespace`; this path does not emit the periodic GC Event).

#### Periodic orphan namespace GC (Kind e2e)

To exercise the periodic path without waiting for production defaults (5m sweep /
10m grace), the Kind overlay SHALL patch the control-plane deployment with
`GATEWAY_NAMESPACE_GC_INTERVAL` and `GATEWAY_NAMESPACE_GC_GRACE_PERIOD` set to
short Go duration strings (for example `30s`; any positive value accepted by
`time.ParseDuration` is valid). Immediately after gateway provisioning succeeds,
the suite SHALL seed a synthetic orphaned managed namespace (`openshell-e2e-orphan-*`)
labeled with both required management labels (`hypershell.redhat.io/managed=true`
and `app.kubernetes.io/managed-by=hypershell-control-plane`) and a name matching
the gateway prefix (not `openshell-db-*`), annotate it with a
backdated `hypershell.redhat.io/gc-eligible-since` timestamp so the next sweep can
reap without waiting a full grace period. Steps 3–10 SHALL run while the periodic
reaper may delete that namespace in the background, so the suite is not blocked
waiting on the sweep interval. In step 11 the suite SHALL validate delete-driven
gateway namespace GC first, then assert the orphan namespace was reaped and a
`GarbageCollected` Event exists in the control-plane namespace
(`E2E_HS_NAMESPACE`, default `hypershell-system`) with `involvedObject.name`
equal to the orphan namespace name. The orphan reap deadline SHALL be measured
from seed time (`E2E_ORPHAN_GC_TIMEOUT` seconds after creation); if the namespace
is already gone when step 11 runs, validation SHALL pass without additional
waiting. Failure to reap or to record the Event SHALL be reported with GC
diagnostics (namespace state and control-plane logs).

#### Delete-driven gateway namespace GC

Before deletion the suite SHALL confirm the gateway's managed namespace exists,
so its later disappearance is a real garbage-collection signal rather than a
namespace that never existed. After issuing
`DELETE /api/hypershell/v1/gateways/<id>`, the suite SHALL poll until the gateway
record returns `404` (the delete event has been processed) and until the managed
namespace is gone, within `E2E_GC_TIMEOUT` seconds. A namespace that is not reaped
within the timeout SHALL be reported as a test failure with GC diagnostics (the
namespace's remaining state and control-plane logs).

Deletion SHALL NOT be gated on the gateway's active sandbox count: even with
active sandboxes the delete is accepted and the namespace is reaped, cascading
removal of the in-namespace sandbox resources (see
`openshell-gateway-namespace-gc.spec.md` and `openshell-gateway-database.spec.md`).

#### Scenario: Periodic orphan namespace garbage collected

- GIVEN the Kind overlay has shortened `GATEWAY_NAMESPACE_GC_INTERVAL` and
  `GATEWAY_NAMESPACE_GC_GRACE_PERIOD` (for example `30s`)
- AND a synthetic managed namespace was seeded after gateway provisioning with
  both management labels, a gateway-style name, and a backdated
  `hypershell.redhat.io/gc-eligible-since`
  annotation, with no live Gateway backing it
- AND steps 3–10 have run while the periodic reaper may have deleted it
- WHEN the suite validates orphan GC in step 11 (after delete-driven GC)
- THEN the namespace SHALL be gone within `E2E_ORPHAN_GC_TIMEOUT` seconds of
  seeding
- AND a namespace still present after that deadline SHALL be reported as a
  failure with GC diagnostics

#### Scenario: GarbageCollected Event recorded for periodic reap

- GIVEN the periodic reaper has deleted the synthetic orphan namespace
- WHEN the suite queries Events in the control-plane namespace
- THEN a `GarbageCollected` Event SHALL exist with `involvedObject.name` equal to
  the orphan namespace name
- AND the absence of such an Event SHALL be reported as a test failure

#### Scenario: Namespace present before deletion

- GIVEN a `Running` gateway whose managed namespace exists
- WHEN the suite checks for the namespace before deleting the gateway
- THEN the namespace SHALL be present
- AND its absence SHALL be reported as a failure, because the GC check cannot then be validated

#### Scenario: Gateway record removed after delete

- GIVEN the gateway has been deleted via `DELETE /api/hypershell/v1/gateways/<id>` (accepted with `204 No Content`)
- WHEN the suite polls `GET /api/hypershell/v1/gateways/<id>`
- THEN the API SHALL report `404` once the control plane has processed the delete event

#### Scenario: Managed namespace garbage collected

- GIVEN the gateway delete has been accepted
- WHEN the suite polls for the gateway's managed namespace
- THEN the namespace SHALL be gone within `E2E_GC_TIMEOUT` seconds
- AND a namespace still present after the timeout SHALL be reported as a failure with GC diagnostics (namespace state and control-plane logs)

### Requirement: Gateway TLS Trust (No Insecure Bypass)

The e2e test suite SHALL connect to the gateway over trusted TLS and SHALL NOT disable certificate verification. The `OPENSHELL_GATEWAY_INSECURE=true` bypass SHALL NOT be used. Instead, the suite SHALL trust the cluster's self-signed CA: it extracts the CA certificate issued by cert-manager (the same CA that signs the gateway's serving certificate and the `*.gw.localhost` wildcard listener cert) and points the openshell CLI at it via `SSL_CERT_FILE`. This ensures the e2e path exercises the same TLS trust chain a real client uses, rather than skipping validation.

#### Scenario: CLI Trusts the Cluster CA

- GIVEN the cluster CA certificate has been extracted from the cert-manager-issued secret
- AND `SSL_CERT_FILE` points the openshell CLI at that CA
- WHEN the CLI connects to the gateway gRPC endpoint over TLS
- THEN the connection SHALL succeed with certificate verification enabled
- AND `OPENSHELL_GATEWAY_INSECURE` SHALL NOT be set

#### Scenario: Insecure Bypass Removed

- GIVEN the e2e test scripts (`tests/e2e/e2e-openshell.sh`, `components/pr-test/e2e-openshell.sh`)
- WHEN they establish a gateway connection
- THEN they SHALL NOT set `OPENSHELL_GATEWAY_INSECURE=true`

### Requirement: CI E2E Workflow

The system SHALL provide a GitHub Actions workflow at `.github/workflows/e2e.yml` that runs the e2e test suite against a Kind cluster on every pull request, on every merge-queue entry (`merge_group`), and on push to `main`. The workflow SHALL follow the same structural patterns as `.github/workflows/lint.yml` (concurrency groups, component detection, conditional jobs, summary gate). The workflow SHALL gate on Konflux image builds completing and pull those images by digest -- it SHALL NOT rebuild component images itself.

#### Scenario: PR Triggers Workflow

- GIVEN a pull request is opened or updated
- AND Konflux has built images for changed components
- WHEN the `e2e` workflow triggers
- THEN it SHALL: check out the repository, detect which components changed (using `.github/scripts/detect-components.sh`), create a Kind cluster via `make kind-up` with baseline images (overlapping cluster creation with the Konflux builds in progress), wait for each changed component's Konflux on-pull-request build to conclude, swap in the Konflux-built image digests via `scripts/kind/set-component-images.sh`, run `tests/e2e/e2e-openshell.sh` with `E2E_INFRA_DRIVER=kind`, and report the CI status

#### Scenario: Tests Pass

- GIVEN the e2e tests complete with 0 failures
- WHEN the workflow finishes
- THEN the CI status check SHALL be reported as `success`

#### Scenario: Tests Fail

- GIVEN one or more e2e tests fail
- WHEN the workflow finishes
- THEN the CI status check SHALL be reported as `failure`
- AND diagnostic artifacts SHALL be uploaded (see CI Artifact Collection)

#### Scenario: Skip for Irrelevant Changes

- GIVEN the PR modifies only files outside the e2e-relevant component paths (e.g., only `docs/` or `components/sdk-typescript/`)
- WHEN the `e2e` workflow evaluates the change detection outputs
- THEN the e2e job SHALL be skipped
- AND the workflow SHALL report `success` (to avoid blocking merges)

#### Scenario: Infrastructure-Only Changes (No Source Components)

- GIVEN the PR modifies e2e-relevant files (Makefile, `.github/`, `deploy/`, `tests/e2e/`) but no files under `components/api-server/`, `components/control-plane/`, `components/web-console/`, or `packages/gateway-management-ui/`
- AND the PR does not change any component's Konflux pipeline definition (`.tekton/hypershell-<component>-main-pull-request.yaml`) or the root `Dockerfile`
- WHEN the `e2e` workflow evaluates the change detection outputs
- THEN the e2e job SHALL run using baseline registry images (no Konflux build wait)
- AND the workflow SHALL NOT poll for Konflux check runs

#### Scenario: Component Pipeline Definition Changed

- GIVEN the PR changes a component's Konflux pull-request pipeline definition (`.tekton/hypershell-<component>-main-pull-request.yaml`), or the root `Dockerfile` for control-plane, without touching that component's source tree
- AND Konflux therefore fires that component's on-pull-request build, because its CEL trigger matches the pipeline file itself
- WHEN the `e2e` workflow evaluates the change detection outputs
- THEN it SHALL wait for that component's on-pull-request build and consume its `on-pr-<head_sha>` image, exactly as if the component source had changed
- AND the workflow's build detection SHALL mirror each component's Konflux CEL trigger, so it never falls back to a baseline image while Konflux is building an on-pr image the PR produced

#### Scenario: Gateway Management UI Package Changed

- GIVEN the PR modifies files only in `packages/gateway-management-ui/`
- AND Konflux has built and pushed the web console image
- WHEN the e2e workflow runs
- THEN it SHALL wait for the web-console Konflux on-pull-request build
- AND it SHALL pull the Konflux-built web console image
- AND the API server and control plane SHALL use baseline registry images

#### Scenario: Merge Queue Gate

- GIVEN a pull request enters the GitHub merge queue
- AND the merge queue pushes the batched merge commit to a `gh-readonly-queue/main/...` branch
- WHEN the `e2e` workflow triggers on the `merge_group` event
- THEN it SHALL always run the e2e job (the merge queue is the pre-merge gate, so change detection SHALL NOT skip it)
- AND for each component whose source the merge batch changed it SHALL wait for that component's dedicated merge-queue Konflux build, keyed on the merge-commit SHA (`github.sha`)
- AND components the merge batch did not change SHALL use baseline registry images
- AND the browser distributed-trace verification SHALL NOT run on `merge_group` (it is covered at pull-request time and re-verified on push to `main`)

#### Scenario: Merge Queue Images Are Distinct and Ephemeral

- GIVEN the merge queue builds images through the dedicated Konflux merge-queue pipelines (`.tekton/hypershell-<component>-main-merge-queue.yaml`)
- WHEN a merge-queue pipeline fires on a push whose target branch starts with `gh-readonly-queue/main/`
- THEN it SHALL push an ephemeral `on-merge-queue-<merge_sha>` image tag (`image-expires-after` set) that is distinct from the pull-request pipelines' `on-pr-<head_sha>` tag, so a merge-queue build is never confused with an already-tested PR image
- AND the merge-queue pipeline SHALL NOT auto-release (`release.appstudio.openshift.io/auto-release: "false"`)
- AND the pull-request pipelines SHALL fire only on the `pull_request` event, not on merge-queue pushes

#### Scenario: Workflow Timeout

- GIVEN the e2e job starts
- WHEN the total job runtime exceeds 20 minutes
- THEN the job SHALL be terminated with a timeout failure

### Requirement: Konflux Image Consumption

The CI workflow SHALL NOT build component images itself. Images are built by Konflux (the existing build pipeline). The e2e workflow SHALL gate on Konflux builds completing, then pull the built images by digest into the Kind cluster. Unchanged components SHALL use baseline images from the container registry. This avoids duplicating the build step and ensures the images tested in CI are identical to the images that ship. This is expected to cover HYPERSHELL-16.

#### Scenario: Single Component Changed

- GIVEN the PR modifies files only in `components/api-server/`
- AND Konflux has built and pushed the API server image
- WHEN the e2e workflow runs
- THEN it SHALL pull the Konflux-built API server image by digest
- AND the control plane and web console SHALL use baseline registry images

#### Scenario: Multiple Components Changed

- GIVEN the PR modifies files in `components/api-server/` AND `components/control-plane/`
- AND Konflux has built both images
- WHEN the e2e workflow runs
- THEN it SHALL pull both Konflux-built images by digest

#### Scenario: Image Override via Make Target (Local/Dev)

- GIVEN a developer wants to test specific Konflux-built images locally
- WHEN they run `make kind-up` with image overrides (e.g., `IMAGE_TAG=sha256:abc123` or per-component image variables)
- THEN the Kind cluster SHALL deploy using the specified images
- AND no local image build or `kind load` step SHALL be required
- NOTE: the CI workflow does not use this path -- it creates the cluster with baseline images via `make kind-up` and swaps in Konflux-built digests afterward via `scripts/kind/set-component-images.sh`, once each changed component's build has concluded, so cluster creation overlaps the Konflux builds instead of waiting on them first

### Requirement: CI Artifact Collection

On test failure, the CI workflow SHALL collect diagnostic artifacts to aid debugging. On success, no artifacts SHALL be uploaded.

#### Scenario: Failure Diagnostics

- GIVEN an e2e test failure
- WHEN the workflow reaches its post-test phase
- THEN it SHALL collect diagnostics inside collapsible `::group::` sections for readable CI output:
  - All pods across all namespaces (`kubectl get pods -A -o wide`)
  - Pod descriptions from the HyperShell namespace (`kubectl describe pods`)
  - Pod logs from all HyperShell components (`kubectl logs --all-containers --prefix --tail=200`)
  - Keycloak logs (`kubectl logs -l app=keycloak -n keycloak`)
  - Cluster events sorted by timestamp (`kubectl get events --sort-by=.lastTimestamp`)
  - Gateway API resources (Gateways, GRPCRoutes, HTTPRoutes) and managed namespace resources
- AND each section SHALL be written to both stdout (via `tee`) and a file in `e2e-diagnostics/`
- AND upload them as a GitHub Actions artifact named `e2e-diagnostics`

#### Scenario: Success -- No Artifacts

- GIVEN all e2e tests pass
- WHEN the workflow completes
- THEN no diagnostic artifacts SHALL be uploaded

### Requirement: Deploy Base/Overlay Structure

The `deploy/` directory SHALL use a kustomize base/overlay structure to support multiple infrastructure targets. Shared resources SHALL live in `deploy/base/`; infrastructure-specific additions SHALL live in per-driver overlays (`deploy/kind/`, `deploy/openshift/`).

#### Scenario: Kind Overlay

- GIVEN `deploy/kind/kustomization.yaml` references `../base` as a resource
- WHEN `kustomize build deploy/kind/` is executed
- THEN the output SHALL include all base resources (namespace, postgres, api-server, controller, controller-rbac, web-console)
- AND Kind-specific resources: networking Gateway with `gatewayClassName: cloud-provider-kind`, cert-manager certificates for `*.hypershell.localhost` and `*.gw.localhost`, HTTPRoutes for component services, CoreDNS Corefile, OIDC secrets, and Kustomize patches for OIDC configuration (JWT flags, Keycloak hostname, control-plane and web-console OIDC env vars) and shortened namespace GC timing (`GATEWAY_NAMESPACE_GC_INTERVAL` and `GATEWAY_NAMESPACE_GC_GRACE_PERIOD`, for example `30s`, so e2e can exercise the periodic reaper)

#### Scenario: OpenShift Overlay

- GIVEN `deploy/openshift/kustomization.yaml` references `../base` as a resource
- WHEN `kustomize build deploy/openshift/` is executed
- THEN the output SHALL include all base resources
- AND OpenShift-specific resources: Route for the API server with edge TLS termination, SecurityContextConstraints bindings

#### Scenario: Base Resource Propagation

- GIVEN a new resource is added to `deploy/base/kustomization.yaml`
- WHEN either overlay is built
- THEN the new resource SHALL appear in both overlay outputs without duplication

#### Scenario: Image Portability

- GIVEN the base resources reference container images
- WHEN an overlay is built
- THEN image references SHALL be configurable via kustomize `images` transformers or environment variable substitution, allowing each overlay to use different registries or tags

### Requirement: Backward Compatibility

The refactored deploy structure SHALL maintain backward compatibility with existing workflows. `make kind-up` SHALL continue to work as before and SHALL additionally accept image overrides for CI use.

#### Scenario: Make Kind-Up Unchanged

- GIVEN the deploy directory has been restructured with base/overlays
- WHEN a developer runs `make kind-up`
- THEN the behavior SHALL be identical to the current implementation
- AND `scripts/kind/up.sh` SHALL continue to function (the migration to `kustomize build deploy/kind/` within the scripts is incremental and transparent)

#### Scenario: Make Kind-Up With Image Overrides

- GIVEN Konflux-built images are available at specific digests
- WHEN a developer or CI runs `make kind-up` with image tag or per-component image overrides (e.g., `IMAGE_TAG=<digest>`)
- THEN the Kind cluster SHALL deploy using the specified images instead of the default registry tags
- AND no local build step SHALL be required

## File Layout

```
tests/e2e/
  e2e-openshell.sh         -- main test script (infra-agnostic, sources driver)
  e2e-performance.sh       -- performance harness (infra-agnostic, sources driver + lib)
  lib.sh                   -- shared test utilities (pass/fail tracking, colors, retry helpers)
  perf-lib.sh              -- perf utilities (timing, latency percentiles, bounded concurrency)
  drivers/
    kind.sh                -- Kind infra driver (this spec)
    openshift.sh           -- OpenShift infra driver (specified in openshift-development.spec.md / HYPERSHELL-44)
scripts/
  perf-report.sh           -- tabulate recent perf runs from perf-results/*.json (make e2e-performance-report)
deploy/
  base/
    kustomization.yaml
    namespace.yaml
    postgres.yaml
    api-server.yaml
    controller.yaml
    controller-rbac.yaml
    web-console.yaml
    keycloak/              -- Keycloak deployment + realm import + theme
    certificates/          -- CA chain for TLS
    networkpolicies.yaml
  kind/                    -- overlay (extends base)
    kustomization.yaml     -- references ../base, OIDC patches
    kind-config.yaml
    certificates.yaml      -- cert-manager Certificates + Issuers
    gateway.yaml           -- networking Gateway (cloud-provider-kind)
    httproutes.yaml        -- HTTPRoutes for component services
    oidc-secrets.yaml      -- control-plane OIDC client secret
    coredns/
      Corefile
    infrastructure/        -- CRDs + controllers (cert-manager, Gateway API, Agent Sandbox)
      kustomization.yaml
  openshift/               -- OpenShift overlay (extends base)
    kustomization.yaml     -- references ../base
    route.yaml
    scc.yaml
    certificates.yaml
    networkpolicies.yaml
    infrastructure/
      kustomization.yaml
      gatewayclass.yaml
.github/workflows/
  e2e.yml                  -- CI e2e workflow
```

`components/pr-test/e2e-openshell.sh` SHALL be deprecated in a follow-up once all tests are migrated to `tests/e2e/`.

## Environment Variables

| Env Var | Default | Description |
|---------|---------|-------------|
| `E2E_INFRA_DRIVER` | (required) | Infra driver to use: `kind`, `openshift` (OpenShift driver per HYPERSHELL-44) |
| `E2E_NAMESPACE` | `openshell-e2e` | Namespace for e2e test resources (gateway deployment) |
| `E2E_GATEWAY_NAME` | `e2e-gw` | Gateway name for the e2e test |
| `E2E_MODE` | `long` | Run depth: `long` runs every step, `short` runs the essential steps of each area (see [E2E Short and Long Modes](#requirement-e2e-short-and-long-modes)) |
| `E2E_SANDBOX_TIMEOUT` | `120` | Seconds to wait for sandbox pod readiness |
| `E2E_PROVISION_TIMEOUT` | `180` | Seconds to wait for gateway provisioning |
| `E2E_GC_TIMEOUT` | `180` | Seconds to wait for the managed namespace to be garbage collected after a gateway delete |
| `E2E_ORPHAN_GC_TIMEOUT` | `90` | Seconds from orphan namespace seed time for the periodic reaper to delete the synthetic orphan (validated in step 11) |
| `E2E_SKIP_CLEANUP` | `0` | Set to `1` to keep test resources after run |
| `E2E_OIDC_USERNAME` | `admin` | Admin OIDC user (member of `hypershell-admins` + `hypershell-users`) used for areas 1--6 |
| `E2E_OIDC_PASSWORD` | `admin` | Password for the admin OIDC user (local dev only) |
| `E2E_DEV_USERNAME` | `developer` | Standard OIDC user (`openshell-user` tier) used for the RBAC boundary assertions |
| `E2E_DEV_PASSWORD` | `developer` | Password for the developer OIDC user (local dev only) |
| `OPENSHELL_BIN` | `openshell` | Path to the openshell CLI binary |
| `SSL_CERT_FILE` | (set by the suite) | Path to the extracted cluster CA so the openshell CLI trusts the gateway's TLS cert (replaces the removed `OPENSHELL_GATEWAY_INSECURE` bypass) |
| `E2E_CONSOLE_URL` | `https://console.hypershell.localhost` | Base URL of the deployed web console for the browser trace verification |
| `E2E_JAEGER_URL` | `https://jaeger.hypershell.localhost` | Base URL of the Jaeger query API queried by the trace verification |

### Requirement: OIDC Authentication in E2E Tests

The e2e test suite SHALL run with OIDC authentication enabled. The CI workflow SHALL deploy the Kind cluster with `KIND_ENABLE_OIDC=true`. All API calls SHALL be authenticated with a Bearer token obtained from Keycloak. This ensures e2e tests exercise the same authentication path as production.

The test suite SHALL verify OIDC integration as part of its standard flow:
1. Acquire a token from Keycloak and authenticate all API calls
2. Verify unauthenticated API requests are rejected with 401
3. Verify the BFF `/auth/login` endpoint redirects to Keycloak with PKCE parameters
4. Verify the BFF `/auth/session` endpoint returns `{ "authenticated": false }` without a session
5. Verify the control plane's gRPC watch streams are active (no `Unauthenticated` errors in logs)

#### Scenario: API JWT Rejection

- GIVEN the API server is running with `API_ENV=development_oidc`
- WHEN an unauthenticated GET is made to `/api/hypershell/v1/gateways`
- THEN the response SHALL be 401 Unauthorized

#### Scenario: Authenticated API Calls

- GIVEN a valid OIDC token has been acquired via `acquire_oidc_token`
- WHEN API calls are made with `Authorization: Bearer <token>`
- THEN the API server SHALL accept the requests

#### Scenario: BFF OIDC Endpoints

- WHEN `GET /auth/login` is requested from the web console
- THEN the response SHALL be 302 with a Location header pointing to the Keycloak authorization endpoint with PKCE parameters (`code_challenge`, `code_challenge_method=S256`)

#### Scenario: BFF Session Contract

- WHEN `GET /auth/session` is requested without a session cookie
- THEN the response SHALL contain `{ "authenticated": false }`

#### Scenario: Control Plane gRPC Auth

- WHEN the control plane logs are inspected
- THEN there SHALL be no `Unauthenticated` gRPC errors

#### Scenario: CI Deployment

- GIVEN the CI e2e workflow
- WHEN the Kind cluster is created
- THEN `make kind-up` SHALL be invoked with `KIND_ENABLE_OIDC=true`

### Requirement: Web Console Distributed Trace Verification

The CI e2e workflow SHALL verify web console distributed tracing end to end, satisfying `web-console/tracing.spec.md` (`WEB-TRACE-11`). The Kind cluster SHALL be created with tracing enabled (`KIND_JAEGER=true`) so Jaeger is deployed and the web-console BFF exports to it. After the bash suite runs, the workflow SHALL drive a representative gateway workflow through a real browser against the deployed console and assert that Jaeger holds one trace joining the browser and the BFF. The check SHALL use the same Node and Chromium setup as the web-console lint job and SHALL run from the deployed console, not a mocked dev server. The trace check SHALL fail the workflow if no cross-service trace appears within a bounded polling window, and failure diagnostics SHALL include Jaeger workload status and logs and the web-console tracing configuration.

#### Scenario: Tracing Enabled for E2E

- GIVEN the CI e2e workflow
- WHEN the Kind cluster is created
- THEN `make kind-up` SHALL be invoked with `KIND_JAEGER=true`
- AND Jaeger SHALL be deployed and the web-console BFF SHALL be configured to export to it

#### Scenario: Cross-Service Trace Asserted

- GIVEN the cluster is running with tracing enabled and the console is reachable
- WHEN the trace verification drives a gateway workflow in a real browser and queries Jaeger
- THEN it SHALL find one trace whose spans include a bounded browser workflow span and the BFF server span joined by the same trace identifier
- AND the workflow SHALL fail if no such trace appears within the polling window

#### Scenario: Trace Failure Diagnostics

- GIVEN the trace verification fails
- WHEN the workflow reaches its post-test phase
- THEN it SHALL collect Jaeger workload status and logs and the web-console tracing configuration alongside the existing diagnostics

## Performance Testing

The performance test measures how the platform behaves when many gateways run at the same time. It provisions a large fleet of gateways on the target cluster in batches. After every batch it runs a fast mini test (the e2e suite in short mode against a canary gateway) and appends a checkpoint record to the results, so a regression is pinned to the scale at which it appears rather than surfacing only at the end. Once the fleet is fully provisioned it runs the full functional e2e suite to confirm the platform still works correctly under that load. A user runs the test with `make e2e-performance`.

The performance test reuses the e2e driver abstraction. It runs against any infrastructure target that supplies a driver. A user selects the target with `E2E_INFRA_DRIVER`, the same as the e2e suite. A user runs the test against Kind for local checks. A user runs the test against any OpenShift cluster for on-demand load tests.

The performance test does not build images and does not create the cluster. It targets a cluster that already runs. It reuses the resources that `make kind-up` (or the OpenShift deploy) already seeded: one fleet, one managed cluster, one release, and one managed database. Each perf gateway create body reuses those ids, the same as the e2e suite (see `discover ... managed_databases` flow in `tests/e2e/e2e-openshell.sh`).

### Performance Architecture

```
tests/e2e/e2e-performance.sh (infra-agnostic performance harness)
    │
    ├── sources tests/e2e/lib.sh       (pass/fail tracking, retry, colors, env defaults)
    ├── sources tests/e2e/perf-lib.sh  (timing, latency percentiles, bounded concurrency)
    │
    ├── sources driver via E2E_INFRA_DRIVER (required: kind | openshift)
    │
    ├── Phase 1  Preflight        -- discover API host, OIDC token, fleet/db ids, baseline;
    │                                provision the canary gateway and grant its OIDC role once
    ├── Phase 2  Batched scale-up -- add E2E_PERF_BATCH_SIZE gateways (bounded concurrency),
    │                                then run the e2e suite in short mode against the canary
    │                                and append a checkpoint; repeat to N (optional early-stop)
    ├── Phase 3  Functional check -- run the e2e suite in long mode (all steps) on a dedicated gateway
    ├── Phase 4  Report           -- write metrics + per-batch checkpoint series (summary + JSON)
    │                                to perf-results/
    └── Phase 5  Teardown         -- delete perf gateways and the canary, wait for namespace GC
```

The harness holds no infrastructure-specific logic. It calls only driver interface functions for infrastructure operations. This is the same rule the e2e suite follows. A new infrastructure target needs only a new driver file, not a change to the harness.

### Requirement: Performance Test Entry Point

The system SHALL provide a `make e2e-performance` target. The target SHALL run `tests/e2e/e2e-performance.sh`. The target SHALL default `E2E_INFRA_DRIVER` to `kind` for local use, the same pattern as the `make e2e` target. A user SHALL be able to override the driver on the command line. This lets the same target run against any OpenShift cluster (see [Custom OpenShift Runs](#requirement-custom-openshift-runs)).

#### Scenario: Local Kind Run

- GIVEN a developer has a running Kind cluster from `make kind-up`
- WHEN the developer runs `make e2e-performance`
- THEN the harness SHALL run with `E2E_INFRA_DRIVER=kind`
- AND it SHALL provision `E2E_PERF_GATEWAY_COUNT` gateways and report performance metrics

#### Scenario: OpenShift Run

- GIVEN a user is logged in to an OpenShift cluster with HyperShell deployed
- AND the `openshift` driver is present at `tests/e2e/drivers/openshift.sh` (per HYPERSHELL-44)
- WHEN the user runs `E2E_INFRA_DRIVER=openshift make e2e-performance`
- THEN the harness SHALL run against the OpenShift cluster with no change to the harness code
- AND all infrastructure operations SHALL use the OpenShift driver (`oc`, Routes)

### Requirement: Infra-Agnostic Performance Harness

The performance harness (`tests/e2e/e2e-performance.sh`) SHALL be infrastructure-agnostic. It SHALL call only the driver interface functions for infrastructure operations. It SHALL select the driver with `E2E_INFRA_DRIVER`, the same as the e2e suite. It SHALL exit with a non-zero status at startup if `E2E_INFRA_DRIVER` is unset or names a missing driver, and SHALL list the available drivers. It SHALL NOT contain any `kind`-only or `oc`-only command.

The OpenShift driver is specified in `openshift-development.spec.md` (HYPERSHELL-44); the performance harness depends on it for OpenShift runs (see [Scope](#scope)). The harness SHALL contain no infra-specific code: it works with either driver with no change. OpenShift runs are manual and on-demand; the performance test is not wired into CI for any target (see [Design Decisions](#design-decisions)).

#### Scenario: Driver Not Set

- GIVEN `E2E_INFRA_DRIVER` is not set
- WHEN the performance harness starts
- THEN it SHALL exit with a non-zero status
- AND print the available drivers from `tests/e2e/drivers/*.sh`

#### Scenario: No Infra-Specific Command in the Harness

- GIVEN the harness source `tests/e2e/e2e-performance.sh`
- WHEN a reviewer inspects it for infrastructure operations
- THEN every infrastructure operation SHALL go through a driver function
- AND no `kubectl`-only or `oc`-only command SHALL appear in the harness itself

### Requirement: Gateway Fleet Scale-Up

The harness SHALL provision `E2E_PERF_GATEWAY_COUNT` gateways on the target cluster. It SHALL provision them in batches of `E2E_PERF_BATCH_SIZE`, running a checkpoint mini test after each batch (see [Incremental Scale-Up Checkpoints](#requirement-incremental-scale-up-checkpoints)). Within a batch it SHALL create the gateways with bounded concurrency, capped at `E2E_PERF_CONCURRENCY`. Bounded concurrency prevents a thundering herd against the API server and the control plane. Each gateway SHALL use a deterministic name: `<E2E_PERF_GATEWAY_PREFIX>-<index>`. Each gateway create body SHALL reuse the seeded fleet, cluster, release, and managed database ids, the same as the e2e suite. The harness SHALL follow a reuse-or-create pattern: an existing gateway with the same name SHALL be reused, not duplicated. This makes the test safe to re-run.

The harness SHALL wait until each gateway reaches `Running` phase, or until `E2E_PERF_PROVISION_TIMEOUT` seconds pass. It SHALL record the create latency and the time-to-`Running` for each gateway. A gateway that does not reach `Running` in time SHALL count as a failed provision, but SHALL NOT stop the run: the harness reports it in the metrics.

#### Scenario: Fleet Provisioned

- GIVEN a running target cluster with the seeded fleet and managed database
- WHEN the harness runs the scale-up phase with `E2E_PERF_GATEWAY_COUNT=N`
- THEN it SHALL create N gateways named `<prefix>-1` through `<prefix>-N`
- AND it SHALL wait until each gateway reports `Running` phase or the provision timeout elapses

#### Scenario: Bounded Concurrency Respected

- GIVEN `E2E_PERF_CONCURRENCY=C`
- WHEN the harness provisions the fleet
- THEN it SHALL run at most C create-and-wait operations at the same time

#### Scenario: Re-Run Reuses Existing Gateways

- GIVEN a prior run left perf gateways in place (`E2E_SKIP_CLEANUP=1`)
- WHEN the harness runs again with the same prefix and count
- THEN it SHALL reuse the existing gateways by name
- AND it SHALL NOT create duplicate gateways

### Requirement: Incremental Scale-Up Checkpoints

The harness SHALL validate the platform incrementally as the fleet grows, so a scale problem is caught as it appears rather than only at the end. It SHALL provision the fleet in batches of `E2E_PERF_BATCH_SIZE` (default 5; a value of 5--10 is recommended). After each batch reaches `Running` (or times out), the harness SHALL run a checkpoint mini test and SHALL append one checkpoint record to the run results before starting the next batch. This means the results file is written incrementally across the run, not only at teardown.

The checkpoint mini test SHALL be the e2e suite run in **short mode** (`E2E_MODE=short`, see [E2E Short and Long Modes](#requirement-e2e-short-and-long-modes)), not the full suite (running every step of all 11 areas after every batch would dominate the run). Short mode runs the essential steps of every area -- so the checkpoint touches a slice of each portion of the test -- while long mode (the default, used for the final run) runs all steps. This reuses the suite's real assertions and driver code; the harness adds no separate probe.

The mini test SHALL run against a dedicated **canary** gateway that the harness provisions once during preflight and whose per-gateway OIDC role it grants once, so no batch pays repeated Keycloak setup or gateway provisioning. The canary is separate from the counted fleet and is named `<E2E_PERF_GATEWAY_PREFIX>-canary`. The checkpoint SHALL invoke short mode with `E2E_GATEWAY_NAME=<prefix>-canary` and `E2E_SKIP_CLEANUP=1` so the suite reuses the canary and does not tear it down between batches.

Each checkpoint record SHALL capture the cumulative number of gateways `Running`, the time-to-`Running` latency percentiles for the batch just added, the mode run (`short`), the mini-test duration in seconds, the mini-test result (`pass`/`fail`), and a timestamp.

A user SHALL be able to disable checkpoints by setting `E2E_PERF_CHECKPOINT=0`, in which case the harness provisions the full fleet in one pass and records no checkpoint entries. When a checkpoint mini test fails and `E2E_PERF_STOP_ON_CHECKPOINT_FAILURE=1` (the default), the harness SHALL stop scaling, record the cumulative gateway count as the breaking scale, run the failure diagnostics, and proceed to reporting and teardown; the run SHALL then be a failure. When `E2E_PERF_STOP_ON_CHECKPOINT_FAILURE=0`, the harness SHALL record the failed checkpoint and continue scaling, so a user can observe whether the platform recovers at a higher scale.

Setting `E2E_PERF_BATCH_SIZE` greater than or equal to `E2E_PERF_GATEWAY_COUNT` provisions the fleet in a single batch with one final checkpoint, which is equivalent to the non-incremental behavior.

#### Scenario: Checkpoint After Each Batch

- GIVEN `E2E_PERF_GATEWAY_COUNT=20` and `E2E_PERF_BATCH_SIZE=5`
- WHEN the harness runs the scale-up phase
- THEN it SHALL run the e2e suite in short mode after each batch of 5 gateways reaches `Running`
- AND it SHALL append a checkpoint record (cumulative count, batch latency percentiles, mode, mini-test duration, mini-test result) after each batch

#### Scenario: Canary Provisioned Once

- GIVEN checkpoints are enabled (`E2E_PERF_CHECKPOINT=1`)
- WHEN the harness runs the preflight phase
- THEN it SHALL provision one canary gateway named `<prefix>-canary` and grant its OIDC role once
- AND each checkpoint mini test SHALL reuse that canary without re-granting the role

#### Scenario: Early Stop on Checkpoint Failure

- GIVEN `E2E_PERF_STOP_ON_CHECKPOINT_FAILURE=1` (default)
- WHEN a checkpoint mini test fails at cumulative count `K`
- THEN the harness SHALL stop scaling before the next batch
- AND it SHALL record `K` as the breaking scale, run diagnostics, and fail the run

#### Scenario: Continue Past Checkpoint Failure

- GIVEN `E2E_PERF_STOP_ON_CHECKPOINT_FAILURE=0`
- WHEN a checkpoint mini test fails at cumulative count `K`
- THEN the harness SHALL record the failed checkpoint and continue provisioning the remaining batches

#### Scenario: Checkpoints Disabled

- GIVEN `E2E_PERF_CHECKPOINT=0`
- WHEN the harness runs the scale-up phase
- THEN it SHALL provision the full fleet without running checkpoint mini tests
- AND the results SHALL contain an empty `checkpoints` array

### Requirement: E2E Short and Long Modes

The e2e suite (`tests/e2e/e2e-openshell.sh`) SHALL support two run depths selected by `E2E_MODE`: `long` (the default) and `short`. The depth is chosen per step, not per area: each area's checks SHALL be organized as named steps, and each step SHALL declare the minimum mode it belongs to. A step tagged `short` runs in both modes; a step tagged `long` runs only in long mode. Long mode therefore runs every step (the current full behavior), and short mode runs the `short`-tagged subset of every area -- a slice of each portion of the test, exercising each area's essential path while skipping its deep or slow steps.

When `E2E_MODE` is unset or `long`, the suite SHALL run every step, so existing invocations (the CI e2e job and the final run of the performance test) are unchanged. When `E2E_MODE=short`, the suite SHALL run only the `short`-tagged steps, in the suite's normal order. The suite SHALL exit non-zero if `E2E_MODE` is set to any value other than `short` or `long`. The tags SHALL live in the suite so both modes run the same assertion code; there SHALL be no second copy of any check.

Short mode SHALL stay fast enough to run after every scale-up batch. Guidance for which steps are `long`-only (illustrative, not exhaustive):

| Area | Short (essential steps) | Long-only (deep / slow steps) |
|------|-------------------------|-------------------------------|
| Provisioning | create gateway, wait `Running` | n/a (both modes need a running gateway) |
| Connectivity | one route reachability check | n/a |
| Sandbox lifecycle | one sandbox create -> ready -> delete, `active_sandbox_count` = 1 then 0 | second concurrent sandbox to assert the count increments |
| Network policy | presence check where applicable | full traffic-isolation assertions |
| RBAC | one boundary assertion (developer 403) | full admin + developer + platform-admin matrix |
| Namespace GC | delete-driven GC with a bounded wait, on a throwaway namespace (not the reused gateway) | periodic-reaper orphan GC over the full timeout window; delete-driven GC of the run's own gateway |

Short mode SHALL NOT delete a gateway supplied via `E2E_GATEWAY_NAME` (so a reused canary survives repeated short runs); its GC coverage SHALL use a throwaway namespace instead.

#### Scenario: Long Mode by Default

- GIVEN `E2E_MODE` is unset
- WHEN the e2e suite runs
- THEN it SHALL run every step of every area, the same as before mode selection existed

#### Scenario: Short Mode Runs a Slice of Each Area

- GIVEN `E2E_MODE=short`
- WHEN the e2e suite runs
- THEN it SHALL run the `short`-tagged steps of every area
- AND it SHALL skip the `long`-only steps (for example the second sandbox and the full RBAC matrix)

#### Scenario: Invalid Mode Fails Fast

- GIVEN `E2E_MODE=medium`
- WHEN the e2e suite starts
- THEN it SHALL exit non-zero and state that the valid modes are `short` and `long`

### Requirement: Functional Validation Under Load

After the fleet is fully provisioned, the harness SHALL run the functional e2e suite (`tests/e2e/e2e-openshell.sh`) against the target cluster as the final, comprehensive gate. Where the per-batch checkpoint runs the suite in short mode (see [E2E Short and Long Modes](#requirement-e2e-short-and-long-modes)), this phase runs it in long mode -- every step of all 11 areas -- to confirm the platform still works correctly while the large gateway fleet runs. The functional suite SHALL use a dedicated gateway name (`E2E_PERF_FUNCTIONAL_GATEWAY_NAME`, default `perf-e2e-gw`) so it does not collide with the perf fleet or the canary. The functional suite SHALL use the same `E2E_INFRA_DRIVER`. The functional suite exits non-zero when any of its checks fail. The harness SHALL treat that non-zero exit as a performance test failure.

A user SHALL be able to skip the functional phase by setting `E2E_PERF_RUN_FUNCTIONAL=0`. This supports pure load measurement without the functional gate.

#### Scenario: Suite Passes Under Load

- GIVEN the perf fleet is provisioned and running
- WHEN the harness runs `tests/e2e/e2e-openshell.sh` with `E2E_GATEWAY_NAME=perf-e2e-gw`
- THEN the functional suite SHALL exit `0`
- AND the harness SHALL record the functional phase as a pass

#### Scenario: Suite Failure Fails the Run

- GIVEN the perf fleet is provisioned
- WHEN the functional suite exits non-zero (one or more checks failed)
- THEN the harness SHALL record the functional phase as a failure
- AND the performance test SHALL exit with a non-zero status

#### Scenario: Functional Phase Skipped

- GIVEN `E2E_PERF_RUN_FUNCTIONAL=0`
- WHEN the harness finishes the scale-up phase
- THEN it SHALL NOT run the functional suite
- AND it SHALL still report the scale-up metrics

### Requirement: Performance Metrics and Reporting

The harness SHALL compute and report metrics for the scale-up phase:

- total gateways requested, provisioned, and failed
- provisioning success rate (percent)
- time-to-`Running` latency percentiles: p50, p90, p99, and max
- provisioning throughput (gateways per minute)
- total wall-clock time for the scale-up phase
- the per-batch checkpoint series (cumulative count, batch latency percentiles, mode, mini-test duration, and mini-test result per checkpoint)

The human-readable stdout summary is the primary digest: a user reads it right after a run. The harness SHALL print an aligned summary table to stdout, including a checkpoint table with one row per batch so a user can see how latency and the mini-test result track with the growing fleet. The harness SHALL also write a machine-readable JSON summary for tooling (see [Performance Results Consumption](#requirement-performance-results-consumption)).

#### Scenario: Metrics Printed

- GIVEN the scale-up phase has finished
- WHEN the harness reaches its report phase
- THEN it SHALL print the success rate, the latency percentiles, and the throughput to stdout as an aligned table

#### Scenario: JSON Summary Written

- GIVEN the report phase runs
- WHEN it writes results
- THEN it SHALL write a JSON summary file under `E2E_PERF_RESULTS_DIR`
- AND the JSON SHALL follow the documented schema (see [Performance Results Consumption](#requirement-performance-results-consumption))

### Requirement: Performance SLO Gating

The harness SHALL support optional pass/fail thresholds. The thresholds SHALL be off by default, so a plain run only reports metrics. When a user sets `E2E_PERF_MIN_SUCCESS_RATE`, the harness SHALL fail the run if the provisioning success rate is below that value. When a user sets `E2E_PERF_MAX_PROVISION_P99`, the harness SHALL fail the run if the p99 time-to-`Running` is above that value. This lets a manual or on-demand run gate on a performance regression without a change to the harness.

#### Scenario: Success Rate Below Threshold

- GIVEN `E2E_PERF_MIN_SUCCESS_RATE=95`
- WHEN the provisioning success rate is below 95 percent
- THEN the harness SHALL exit with a non-zero status

#### Scenario: p99 Above Threshold

- GIVEN `E2E_PERF_MAX_PROVISION_P99=300`
- WHEN the p99 time-to-`Running` is above 300 seconds
- THEN the harness SHALL exit with a non-zero status

#### Scenario: No Thresholds Set

- GIVEN neither `E2E_PERF_MIN_SUCCESS_RATE` nor `E2E_PERF_MAX_PROVISION_P99` is set
- WHEN the harness finishes the scale-up phase
- THEN it SHALL report the metrics
- AND it SHALL NOT fail on the metrics (only a functional failure or a provision error can fail the run)

### Requirement: Performance Results Consumption

The performance test is run locally or against any OpenShift cluster, not in CI (see [Design Decisions](#design-decisions)). The results SHALL therefore be easy to digest from a terminal, with no CI service required. Three things support this: a stable JSON schema, per-run history files, and a local report target.

**Stable, versioned JSON schema.** The harness SHALL write the run summary as JSON with a top-level `schema_version` string. The schema SHALL be documented in this spec, so `jq` filters keep working across runs. The JSON SHALL contain the driver, the run timestamps, the run config, the scale-up metrics, the per-batch checkpoint series, the functional result, the SLO result, and the overall result. The shape SHALL be:

```json
{
  "schema_version": "1",
  "driver": "kind",
  "started_at": "2026-08-21T15:30:00Z",
  "finished_at": "2026-08-21T15:41:12Z",
  "config": {
    "gateway_count": 20,
    "batch_size": 5,
    "concurrency": 4,
    "provision_timeout": 600,
    "gateway_prefix": "perf-gw"
  },
  "scale_up": {
    "requested": 20,
    "provisioned": 20,
    "failed": 0,
    "success_rate": 100.0,
    "wall_clock_seconds": 512,
    "throughput_per_min": 2.34,
    "time_to_running_seconds": { "p50": 118, "p90": 205, "p99": 233, "max": 240 },
    "stopped_early": false,
    "breaking_scale": null
  },
  "checkpoints": [
    {
      "gateways_running": 5,
      "at": "2026-08-21T15:33:10Z",
      "batch_time_to_running_seconds": { "p50": 92, "p90": 140, "p99": 150, "max": 152 },
      "mode": "short",
      "mini_test": "pass",
      "mini_test_seconds": 34
    },
    {
      "gateways_running": 10,
      "at": "2026-08-21T15:35:41Z",
      "batch_time_to_running_seconds": { "p50": 110, "p90": 180, "p99": 190, "max": 192 },
      "mode": "short",
      "mini_test": "pass",
      "mini_test_seconds": 39
    }
  ],
  "functional": { "ran": true, "passed": true, "gateway_name": "perf-e2e-gw" },
  "slo": { "min_success_rate": null, "max_provision_p99": null, "passed": true },
  "result": "pass"
}
```

A field that does not apply SHALL be `null` (for example an unset SLO threshold, or `functional` fields when `E2E_PERF_RUN_FUNCTIONAL=0` with `ran: false`). `checkpoints` SHALL be an array with one entry per completed batch, in scale order, and SHALL be empty when `E2E_PERF_CHECKPOINT=0`. `scale_up.stopped_early` SHALL be `true` and `scale_up.breaking_scale` SHALL hold the cumulative gateway count when an early stop is triggered by a failing checkpoint; otherwise `stopped_early` is `false` and `breaking_scale` is `null`. A breaking change to the shape SHALL bump `schema_version`.

**Per-run history files.** The harness SHALL write each run to a new timestamped file, `<E2E_PERF_RESULTS_DIR>/<driver>-<UTC-timestamp>.json` (for example `perf-results/kind-20260821T153000Z.json`). It SHALL write the file incrementally: it SHALL update the file after each checkpoint so a partial result survives an interrupt or an early stop, and it SHALL finalize the file in the report phase. It SHALL NOT overwrite prior runs. This keeps a local history a user can compare across runs, the same benefit a CI trend chart gives, without a CI service. The harness MAY also write or update a `perf-results/latest.json` pointer to the most recent run for convenience.

The results directory holds local run output, not source. The default `E2E_PERF_RESULTS_DIR` (`perf-results/`) SHALL be gitignored, so run artifacts (JSON history and the optional CSV) are never committed.

**Local report target.** The system SHALL provide a `scripts/perf-report.sh` script and a `make e2e-performance-report` target. The report SHALL read the JSON history files under `E2E_PERF_RESULTS_DIR` and print an aligned table of the most recent `E2E_PERF_REPORT_LIMIT` runs (default 10), one row per run, with the timestamp, driver, gateway count, success rate, p99, throughput, and result. This lets a user spot a regression across local runs from the terminal. The report SHALL also be able to render the per-batch checkpoint series of a single run (cumulative count, batch p99, mini-test duration, and mini-test result per checkpoint), so a user can see at which scale latency climbs or the mini test starts failing within one run. The report SHALL depend only on tools already used by the suite (`bash`, `python3`); it SHALL NOT require `jq` or any external service.

**Optional CSV export.** When `E2E_PERF_CSV=1`, the harness SHALL also write a CSV row per run to `<E2E_PERF_RESULTS_DIR>/history.csv` (append, with a header on first write), so a user can open the history in a spreadsheet. CSV export SHALL be off by default.

#### Scenario: JSON Follows the Documented Schema

- GIVEN a completed run
- WHEN a user reads the run's JSON file
- THEN it SHALL contain `schema_version` and the documented top-level keys (`driver`, `started_at`, `finished_at`, `config`, `scale_up`, `checkpoints`, `functional`, `slo`, `result`)
- AND `jq` filters written against the documented schema SHALL succeed

#### Scenario: Checkpoint Series Written Incrementally

- GIVEN checkpoints are enabled and the harness has completed at least one batch
- WHEN a user reads the run's JSON file mid-run
- THEN it SHALL already contain a `checkpoints` entry for each completed batch
- AND if the run is later interrupted, the file SHALL retain the checkpoints written so far

#### Scenario: Runs Are Not Overwritten

- GIVEN a prior run wrote `perf-results/kind-<t1>.json`
- WHEN a new run finishes at a later time `<t2>`
- THEN it SHALL write a new file `perf-results/kind-<t2>.json`
- AND the earlier file SHALL remain unchanged

#### Scenario: Local Report Tabulates Recent Runs

- GIVEN two or more run JSON files exist under `E2E_PERF_RESULTS_DIR`
- WHEN a user runs `make e2e-performance-report`
- THEN it SHALL print an aligned table with one row per run, most recent first
- AND each row SHALL show the timestamp, driver, gateway count, success rate, p99, throughput, and result

#### Scenario: Report Needs No External Tooling

- GIVEN a machine with only `bash` and `python3` (no `jq`, no network)
- WHEN a user runs `make e2e-performance-report`
- THEN it SHALL print the report without error

#### Scenario: CSV Export Opt-In

- GIVEN `E2E_PERF_CSV=1`
- WHEN a run finishes
- THEN the harness SHALL append a row to `<E2E_PERF_RESULTS_DIR>/history.csv`
- AND it SHALL write a header row when the file is first created

### Requirement: Performance Test Cleanup

The harness SHALL delete every gateway it created. It SHALL delete the gateways with bounded concurrency, capped at `E2E_PERF_CONCURRENCY`. It SHALL run cleanup from an `EXIT` trap, so a failure or an interrupt still triggers cleanup. It SHALL poll until each managed namespace is garbage collected, the same signal the e2e suite uses (see [Gateway Deletion and Namespace GC](#requirement-gateway-deletion-and-namespace-gc)). A user SHALL be able to keep the fleet by setting `E2E_SKIP_CLEANUP=1`, the same variable the e2e suite uses. This helps a user inspect the cluster after a run.

#### Scenario: Fleet Deleted After Run

- GIVEN the harness provisioned a perf fleet
- WHEN the run finishes
- THEN the harness SHALL delete every perf gateway it created
- AND it SHALL poll until each managed namespace is gone

#### Scenario: Cleanup on Failure

- GIVEN the harness fails during the scale-up or functional phase
- WHEN the `EXIT` trap runs
- THEN the harness SHALL still delete the perf gateways it created

#### Scenario: Cleanup Skipped

- GIVEN `E2E_SKIP_CLEANUP=1`
- WHEN the run finishes
- THEN the harness SHALL keep the perf fleet in place for inspection

### Requirement: Performance Failure Diagnostics

On failure, the harness SHALL collect diagnostics that explain resource pressure. A large fleet can exhaust node CPU, memory, or pod capacity. The diagnostics SHALL include: pending pods and their reasons, node capacity and allocatable resources, the phases of the perf gateways, and control-plane logs. The harness SHALL print the diagnostics in collapsible `::group::` sections, the same style the e2e workflow uses (see [CI Artifact Collection](#requirement-ci-artifact-collection)). This makes a scale failure easy to read in CI output.

#### Scenario: Resource Pressure Surfaced

- GIVEN a scale-up failure (one or more gateways did not reach `Running`)
- WHEN the harness reaches its diagnostics phase
- THEN it SHALL report pending pods with reasons, node capacity, perf gateway phases, and control-plane logs

### Performance Environment Variables

| Env Var | Default | Description |
|---------|---------|-------------|
| `E2E_PERF_GATEWAY_COUNT` | `10` | Number of gateways to provision in the scale-up phase |
| `E2E_PERF_BATCH_SIZE` | `5` | Gateways added per batch before a checkpoint mini test (5--10 recommended) |
| `E2E_PERF_CHECKPOINT` | `1` | Run the checkpoint mini test after each batch (`0` provisions in one pass, no checkpoints) |
| `E2E_PERF_STOP_ON_CHECKPOINT_FAILURE` | `1` | Stop scaling and fail on a failing checkpoint (`0` records it and continues) |
| `E2E_PERF_CONCURRENCY` | `4` | Max concurrent create / provision / delete operations |
| `E2E_PERF_GATEWAY_PREFIX` | `perf-gw` | Name prefix for the perf gateway fleet (canary is `<prefix>-canary`) |
| `E2E_PERF_PROVISION_TIMEOUT` | `600` | Seconds to wait for each gateway to reach `Running` under load |
| `E2E_PERF_RUN_FUNCTIONAL` | `1` | Run the e2e functional suite after scale-up (`0` to skip) |
| `E2E_PERF_FUNCTIONAL_GATEWAY_NAME` | `perf-e2e-gw` | Gateway name for the nested functional suite (avoids collision with the fleet) |
| `E2E_PERF_RESULTS_DIR` | `perf-results` | Directory for the per-run JSON history files (and optional CSV) |
| `E2E_PERF_REPORT_LIMIT` | `10` | Number of recent runs `make e2e-performance-report` tabulates |
| `E2E_PERF_CSV` | `0` | Set to `1` to also append each run to `<results-dir>/history.csv` |
| `E2E_PERF_MIN_SUCCESS_RATE` | (unset) | Optional SLO: min provisioning success rate percent; below this fails the run |
| `E2E_PERF_MAX_PROVISION_P99` | (unset) | Optional SLO: max p99 time-to-`Running` seconds; above this fails the run |
| `E2E_INFRA_DRIVER` | (required; `make e2e-performance` defaults to `kind`) | Infra driver: `kind`, `openshift` (OpenShift driver per HYPERSHELL-44) |
| `E2E_SKIP_CLEANUP` | `0` | Set to `1` to keep the perf fleet after the run |

**Capacity note:** a small Kind cluster cannot run hundreds of gateways. Each gateway provisions a deployment, a service, a TLS secret, a certgen job, a per-gateway Keycloak client, and a managed namespace. Keep `E2E_PERF_GATEWAY_COUNT` small on Kind (for example 5--10). Use a larger count on an OpenShift cluster that has spare capacity. The harness reports resource pressure on failure so a user can find the ceiling.

## Design Decisions

| Decision | Rationale |
|----------|-----------|
| Shell-based drivers as starting point | The e2e test is a shell script; shell functions provide the simplest driver abstraction without adding a new language or build step. Each driver is a single file implementing a known function interface. If the test suite grows in complexity -- structured assertions, parallel execution, direct Kubernetes API client usage -- migrating to a Go-based e2e framework (e.g., `go test` with client-go) is a natural follow-up. The driver interface contract is function-shape-agnostic, so the same logical abstraction applies in either language |
| `E2E_INFRA_DRIVER` is required, no auto-detection | Explicit driver selection avoids ambiguity and makes CI invocations self-documenting. Each environment sets the driver it intends to test against |
| Tests live in `tests/e2e/`, not `components/pr-test/` | A top-level `tests/` tree is the natural home for e2e tests and their drivers. `components/pr-test/` will be deprecated in a follow-up once migration is complete |
| Shared test utilities in `tests/e2e/lib.sh` | Pass/fail tracking, color output, and retry helpers are currently inline in `e2e-openshell.sh`. Extracting them into `lib.sh` makes them reusable across future test scripts without duplicating code |
| Kustomize base/overlay for deploy | The current deploy structure has full resource duplication between Kind and the legacy `components/api-server/deploy/openshift/` manifests. A base/overlay eliminates drift by sharing core resource definitions. The Kind overlay adds cloud-provider-kind, certificates, DNS; the OpenShift overlay adds Routes, SCC |
| CI pulls Konflux-built images, not rebuild | Images are built by Konflux (the existing build pipeline). The e2e workflow gates on those builds and pulls images by digest, avoiding duplicate builds and ensuring CI tests the exact images that ship. This is expected to cover HYPERSHELL-16 |
| Diagnostic artifacts only on failure | Uploading pod logs, events, and describes on every run wastes GitHub Actions storage. Conditional upload on failure provides debugging information when needed |
| 20-minute CI timeout | Kind cluster creation takes ~2 min, image pulls ~1-2 min, e2e tests ~5-8 min. A 20-minute ceiling provides margin for slow GitHub runners while preventing runaway jobs |
| e2e workflow skips for irrelevant changes | SDK-only or docs-only PRs do not affect the e2e path. Skipping avoids CI time and Konflux build overhead. The `detect-components.sh` infrastructure tracks `api_server`, `control_plane`, `pr_test`, and `e2e` component paths for "should we re-run e2e" decisions. Separately, Konflux image builds only trigger on changes under `components/<name>/` source paths -- the workflow checks the actual diff to distinguish e2e-relevant infrastructure changes (which use baseline images) from source changes (which require Konflux-built images) |
| `make kind-up` accepts image overrides | Passing `IMAGE_TAG=<digest>` or per-component image variables to `make kind-up` allows CI to deploy Konflux-built images directly without a separate load step. Developers can also use this to test specific image versions locally |
| Backward-compatible migration | The refactoring does not change `make kind-up`. `scripts/kind/up.sh` can be migrated to use `kustomize build deploy/kind/` incrementally. The spec defines the target state; the migration path is incremental |
| OpenShift driver defined in HYPERSHELL-44; consumed here for manual runs | The `tests/e2e/drivers/openshift.sh` driver, the `make openshift-*` lifecycle, the `deploy/openshift/` overlay, the cluster infrastructure bootstrap, and the OpenShift CI are specified in `openshift-development.spec.md` (HYPERSHELL-44). This spec does not duplicate that work. It defines the driver interface contract (both columns) and requires the OpenShift driver so `make e2e` and `make e2e-performance` run manually against any OpenShift cluster for scale and performance testing. The automated OpenShift CI job stays out of this spec |
| Env vars renamed with `E2E_` prefix | The existing `e2e-openshell.sh` uses `SANDBOX_TIMEOUT`, `PROVISION_TIMEOUT`, `SKIP_CLEANUP`, and `GATEWAY_NAMESPACE`. These are renamed to `E2E_SANDBOX_TIMEOUT`, `E2E_PROVISION_TIMEOUT`, `E2E_SKIP_CLEANUP`, and `E2E_NAMESPACE` to avoid namespace collisions with non-e2e configuration and make the e2e origin of these variables explicit |
| CI uses `make kind-up`, not raw `kind create cluster` | Reuses the same cluster setup path developers use locally. Ensures the CI environment is identical to local development. Avoids a second "create a Kind cluster" implementation that could drift |
| Performance harness reuses the e2e driver interface | The performance test needs the same cross-infrastructure portability as the e2e suite: run on Kind locally, run on any OpenShift cluster for on-demand load tests. Reusing the driver interface means the harness holds no infra-specific code and a new target needs only a new driver file. It also keeps one abstraction to maintain, not two |
| Performance test runs the e2e suite for functional validation | The user requirement is "spin up a ton of gateways, then confirm things still function." The e2e suite already validates the full functional path (provisioning, connectivity, sandbox lifecycle, RBAC, GC) and exits non-zero on any failure. Running it while the perf fleet is up proves the platform still works correctly under load, without duplicating functional assertions in the perf harness |
| Bounded concurrency for scale-up and teardown | Creating hundreds of gateways at once would flood the API server and control plane and would not model a realistic ramp. `E2E_PERF_CONCURRENCY` caps in-flight operations so the client applies steady, controllable load and the harness itself does not become the bottleneck |
| Batched scale-up with per-batch checkpoints | Provisioning all N gateways and validating once at the end hides the scale at which a problem first appears. Adding gateways in batches of `E2E_PERF_BATCH_SIZE` (5--10) and running the e2e suite in short mode after each batch produces a time series (count vs latency, count vs pass/fail), so a regression is pinned to a scale and the results file is written incrementally. Optional early-stop reports the breaking scale instead of pushing to a guaranteed failure. The suite runs once in long mode at the end as the comprehensive gate |
| Short vs long mode by step tag, not area selector | The checkpoint needs to touch every area but stay fast. Tagging each step `short` or `long` (rather than selecting whole areas by name) lets short mode run a slice of each portion of the test -- the essential path of every area -- while long mode runs everything. Both modes execute the same assertion code in `e2e-openshell.sh`, so there is one copy of each check and the incremental signal is trustworthy. `E2E_MODE` defaults to `long`, so CI and the final run are unchanged |
| Dedicated canary gateway for the mini test | Short mode needs a stable target it can reuse across batches without re-paying gateway provisioning and per-gateway OIDC role setup each time. A single canary gateway, provisioned once with its role granted once, is passed via `E2E_GATEWAY_NAME`; short mode does not delete a supplied gateway, so the canary survives repeated runs. The canary is kept separate from the counted fleet so its own lifecycle is unaffected by fleet churn and it is not double-counted in scale metrics |
| Deterministic gateway names + reuse-or-create | Naming perf gateways `<prefix>-<index>` makes a run idempotent and makes cleanup a simple prefix match. This follows the repo-wide "reconcile, don't create-or-skip" convention and lets a developer re-run the test without accumulating duplicate fleets |
| SLO gating is optional and off by default | A plain run should just report metrics so a developer can explore capacity. Gating (`E2E_PERF_MIN_SUCCESS_RATE`, `E2E_PERF_MAX_PROVISION_P99`) is opt-in so a manual or on-demand run can fail on a regression without forcing thresholds on every local run |
| Performance test is not wired into PR CI | A large-scale provisioning run is too heavy and too slow for the per-PR e2e gate (20-minute ceiling). The performance test is run on demand locally, against any OpenShift cluster, or on a schedule. Keeping it out of the PR path avoids flaky, resource-bound CI failures |
| Terminal-native results: versioned JSON + history files + local report | Because the test is run manually and not in CI, results must digest from a terminal with no CI service. A documented, versioned JSON schema keeps `jq` filters stable across runs; per-run timestamped files build a local history instead of overwriting; a `make e2e-performance-report` target tabulates recent runs so a user can spot a regression. This gives the trend-comparison benefit of a CI benchmark chart without a CI dependency, and leaves the JSON as a clean hook to add `github-action-benchmark` later if the test is ever wired into CI |
| Report depends only on bash + python3 | The suite already uses `bash` and `python3`; the report and JSON writing reuse them. Avoiding a hard `jq` dependency means the report runs on a plain machine with no extra install. CSV export is opt-in for users who prefer a spreadsheet |
