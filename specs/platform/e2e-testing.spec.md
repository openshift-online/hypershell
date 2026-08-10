# E2E Testing and CI Integration

**Date:** 2026-08-10
**Status:** Draft
**Jira:** HYPERSHELL-18
**Related:** `local-development.spec.md` -- Kind cluster setup;
             `control-plane.spec.md` -- reconciler behavior;
             `openshell-gateway-routing.spec.md` -- GRPCRoute provisioning

## Purpose

HyperShell requires infrastructure-agnostic end-to-end testing that validates the full provisioning path: API creation of a Gateway, control plane reconciliation, gateway pod readiness, route connectivity, and sandbox lifecycle. The same test suite SHALL run against Kind (local development, CI) and OpenShift (staging, QE) with infrastructure-specific logic isolated into driver scripts. A CI workflow SHALL execute these tests automatically on pull requests that modify e2e-relevant components.

The existing e2e test (`components/pr-test/e2e-openshell.sh`) validates 6 areas -- gateway provisioning, infrastructure verification, route discovery, connectivity, sandbox lifecycle, and sandbox interaction -- but is hardcoded for OpenShift. This spec defines the driver abstraction, CI workflow, and deploy restructuring required to run the same tests across multiple infrastructure targets.

### Scope

This spec covers the **Kind driver** and the **Kind-based CI workflow**. The OpenShift driver implementation and its corresponding CI job that deploys to an OpenShift cluster are follow-up work. The driver interface is defined here to establish the contract, but only the Kind driver is implemented in this iteration.

## Architecture

```
tests/e2e/e2e-openshell.sh (infra-agnostic test logic)
    │
    ├── sources tests/e2e/lib.sh (shared utilities)
    │
    └── sources driver via E2E_INFRA_DRIVER (required)
        │
        ├── tests/e2e/drivers/kind.sh         (this spec)
        └── tests/e2e/drivers/openshift.sh    (follow-up)
```

The driver model separates test logic from infrastructure mechanics. The main test script calls a fixed set of driver functions; each driver implements those functions for its target infrastructure. Adding a new infrastructure target requires only a new driver file.

### Driver Interface

Each driver exports shell functions that abstract infrastructure-specific operations. The table maps each function to the OpenShift-specific construct it replaces in the current `e2e-openshell.sh`:

| Function | Purpose | Kind Implementation | OpenShift Implementation |
|----------|---------|---------------------|--------------------------|
| `discover_api_host` | Find the HyperShell API server URL | HTTPRoute hostname `api.hypershell.localhost` or port-forward to `svc/hypershell-api-server` | `oc get route hypershell-api -o jsonpath='{.spec.host}'` |
| `discover_gateway_endpoint` | Find the gateway gRPC endpoint | GRPCRoute hostname `<gw-name>.gw.localhost` via Gateway status address | Route with `spec.tls.termination=passthrough` targeting `svc/openshell-gateway` |
| `get_cluster_domain` | Get the base domain for constructing gateway DNS names | `gw.localhost` (static, matching `GATEWAY_API_BASE_DOMAIN` in `deploy/kind/`) | `oc get ingresses.config.openshift.io cluster -o jsonpath='{.spec.domain}'` |
| `get_cli_binary` | Return the Kubernetes CLI binary path | `oc` | `oc` |
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
    ├── make kind-up IMAGE_TAG=<konflux-digest> (create Kind cluster with PR images)
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
  kind/                     ← Kind overlay (extends base)
    kustomization.yaml      ← references ../base
    kind-config.yaml
    coredns/
      Corefile
    prerequisites/
      certificates.yaml
      networking-gateway.yaml
      httproutes.yaml
      keycloak.yaml
  openshift/                ← OpenShift overlay (extends base)
    kustomization.yaml      ← references ../base
    route.yaml
    scc.yaml
```

## Requirements

### Requirement: Infra Driver Abstraction

The e2e test framework SHALL isolate infrastructure-specific logic into driver scripts located at `tests/e2e/drivers/<driver>.sh`. The main test script (`tests/e2e/e2e-openshell.sh`) SHALL remain infrastructure-agnostic and call only driver interface functions for infrastructure-specific operations. The `E2E_INFRA_DRIVER` environment variable SHALL select the driver and MUST be set -- the script SHALL exit with an error if it is unset.

#### Scenario: Kind Driver Selected

- GIVEN `E2E_INFRA_DRIVER=kind`
- WHEN the e2e test script starts
- THEN the `tests/e2e/drivers/kind.sh` driver SHALL be sourced
- AND all infrastructure functions SHALL use `oc` and Kind-specific discovery (HTTPRoute hostnames, Gateway API status)

#### Scenario: OpenShift Driver Selected (Follow-Up)

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

Each driver script SHALL export the following shell functions. The main test script SHALL call only these functions for infrastructure-specific operations. A driver that does not implement all required functions SHALL cause the test script to exit with an error at startup. This spec covers the Kind driver implementations; OpenShift driver implementations are follow-up work.

#### Scenario: API Host Discovery -- Kind

- GIVEN the `kind` driver is active
- WHEN `discover_api_host` is called
- THEN it SHALL return `api.hypershell.localhost` (the HTTPRoute hostname from `deploy/kind/prerequisites/httproutes.yaml`)
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
- THEN it SHALL return `oc`

#### Scenario: Wait for Gateway Route -- Kind

- GIVEN the `kind` driver is active
- AND a gateway named `$GW_NAME` has been provisioned
- WHEN `wait_for_gateway_route` is called
- THEN it SHALL poll the Gateway API Gateway resource's status conditions for `Programmed=True`
- AND verify the corresponding GRPCRoute's parent status reports `Accepted=True`
- AND return success when both conditions are met or fail after `E2E_PROVISION_TIMEOUT` seconds

### Requirement: E2E Test Suite Coverage

The e2e test suite SHALL validate the following 6 areas, preserving the existing test structure from `components/pr-test/e2e-openshell.sh`. All test areas SHALL be infrastructure-agnostic -- they call driver functions for infra-specific operations and use the Kubernetes API for resource inspection.

1. **Gateway provisioning via HyperShell API** -- create a gateway via the REST API and wait for the control plane to reconcile it to `Running` phase
2. **Gateway infrastructure verification** -- confirm the gateway deployment, service, TLS secret, and certgen job exist and are healthy
3. **Route discovery + openshell CLI registration** -- discover the gateway endpoint via the driver, register it with the openshell CLI
4. **Gateway connectivity** -- verify the openshell CLI can connect to the gateway and report status
5. **Sandbox lifecycle** -- create a sandbox, wait for the pod to reach `Running` state
6. **Sandbox interaction** -- execute commands inside the sandbox (`uname -a`, `ls /workspace`)

#### Scenario: Full Suite Execution

- GIVEN a running HyperShell environment (Kind or OpenShift)
- WHEN the e2e test suite runs
- THEN all 6 test areas SHALL be executed in sequence
- AND results SHALL be reported as pass/fail counts with per-test detail

#### Scenario: Gateway Provisioning

- GIVEN the HyperShell API is reachable (via `discover_api_host`)
- WHEN a gateway is created via `POST /api/hypershell/v1/gateways`
- THEN the test SHALL poll the API until the gateway phase is `Running` or `PROVISION_TIMEOUT` seconds have elapsed
- AND a timeout SHALL be reported as a test failure

#### Scenario: Infrastructure Verification

- GIVEN a gateway has reached `Running` phase
- WHEN the test verifies infrastructure
- THEN it SHALL confirm: deployment `openshell-gateway` exists and has at least 1 ready replica, service `openshell-gateway` exists with a ClusterIP, TLS secret `openshell-server-tls` exists, certgen job `openshell-gateway-certgen` has succeeded

#### Scenario: Sandbox Lifecycle

- GIVEN the openshell CLI is connected to the gateway
- WHEN `sandbox create --name <name>` is invoked
- THEN a pod matching `default--<name>` SHALL appear in the gateway namespace
- AND the pod SHALL reach `Running` state within `SANDBOX_TIMEOUT` seconds

### Requirement: CI E2E Workflow

The system SHALL provide a GitHub Actions workflow at `.github/workflows/e2e.yml` that runs the e2e test suite against a Kind cluster on every pull request and push to `main`. The workflow SHALL follow the same structural patterns as `.github/workflows/lint.yml` (concurrency groups, component detection, conditional jobs, summary gate). The workflow SHALL gate on Konflux image builds completing and pull those images by digest -- it SHALL NOT rebuild component images itself.

#### Scenario: PR Triggers Workflow

- GIVEN a pull request is opened or updated
- AND Konflux has built images for changed components
- WHEN the `e2e` workflow triggers
- THEN it SHALL: check out the repository, detect which components changed (using `.github/scripts/detect-components.sh`), create a Kind cluster via `make kind-up` with Konflux image digests passed via `IMAGE_TAG`, run `tests/e2e/e2e-openshell.sh` with `E2E_INFRA_DRIVER=kind`, and report the CI status

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

- GIVEN the PR modifies only files outside the e2e-relevant component paths (e.g., only `docs/`, `packages/gateway-management-ui/`, or `components/sdk-typescript/`)
- WHEN the `e2e` workflow evaluates the change detection outputs
- THEN the e2e job SHALL be skipped
- AND the workflow SHALL report `success` (to avoid blocking merges)

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

#### Scenario: Image Override via Make Target

- GIVEN Konflux-built images are available at specific digests
- WHEN the CI workflow runs `make kind-up` with image overrides (e.g., `IMAGE_TAG=sha256:abc123` or per-component image variables)
- THEN the Kind cluster SHALL deploy using the specified Konflux images
- AND no local image build or `kind load` step SHALL be required

### Requirement: CI Artifact Collection

On test failure, the CI workflow SHALL collect diagnostic artifacts to aid debugging. On success, no artifacts SHALL be uploaded.

#### Scenario: Failure Diagnostics

- GIVEN an e2e test failure
- WHEN the workflow reaches its post-test phase
- THEN it SHALL collect: all pod logs from the HyperShell namespace (`kubectl logs --all-containers`), cluster events (`kubectl get events --sort-by=.lastTimestamp`), pod descriptions (`kubectl describe pods`), the e2e test script stdout/stderr
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
- AND Kind-specific resources: networking Gateway with `gatewayClassName: cloud-provider-kind`, cert-manager certificates for `*.hypershell.localhost` and `*.gw.localhost`, HTTPRoutes for component services, CoreDNS Corefile, Keycloak deployment

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
  lib.sh                   -- shared test utilities (pass/fail tracking, colors, retry helpers)
  drivers/
    kind.sh                -- Kind infra driver (this spec)
    openshift.sh           -- OpenShift infra driver (follow-up)
deploy/
  base/
    kustomization.yaml
    namespace.yaml
    postgres.yaml
    api-server.yaml
    controller.yaml
    controller-rbac.yaml
    web-console.yaml
  kind/                    -- overlay (existing directory, restructured)
    kustomization.yaml     -- references ../base
    kind-config.yaml
    coredns/
      Corefile
    prerequisites/
      certificates.yaml
      networking-gateway.yaml
      httproutes.yaml
      keycloak.yaml
  openshift/               -- new overlay (follow-up)
    kustomization.yaml     -- references ../base
    route.yaml
    scc.yaml
.github/workflows/
  e2e.yml                  -- CI e2e workflow
```

`components/pr-test/e2e-openshell.sh` SHALL be deprecated in a follow-up once all tests are migrated to `tests/e2e/`.

## Environment Variables

| Env Var | Default | Description |
|---------|---------|-------------|
| `E2E_INFRA_DRIVER` | (required) | Infra driver to use: `kind`, `openshift` (follow-up) |
| `E2E_NAMESPACE` | `openshell-e2e` | Namespace for e2e test resources (gateway deployment) |
| `E2E_GATEWAY_NAME` | `e2e-gw` | Gateway name for the e2e test |
| `E2E_SANDBOX_TIMEOUT` | `120` | Seconds to wait for sandbox pod readiness |
| `E2E_PROVISION_TIMEOUT` | `180` | Seconds to wait for gateway provisioning |
| `E2E_SKIP_CLEANUP` | `0` | Set to `1` to keep test resources after run |
| `OPENSHELL_BIN` | `openshell` | Path to the openshell CLI binary |

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
| e2e workflow skips for irrelevant changes | SDK-only or docs-only PRs do not affect the e2e path. Skipping avoids CI time and Konflux build overhead. The `detect-components.sh` infrastructure tracks `api_server`, `control_plane`, and `pr_test` component paths |
| `make kind-up` accepts image overrides | Passing `IMAGE_TAG=<digest>` or per-component image variables to `make kind-up` allows CI to deploy Konflux-built images directly without a separate load step. Developers can also use this to test specific image versions locally |
| Backward-compatible migration | The refactoring does not change `make kind-up`. `scripts/kind/up.sh` can be migrated to use `kustomize build deploy/kind/` incrementally. The spec defines the target state; the migration path is incremental |
| OpenShift overlay and driver are follow-up | The `deploy/openshift/` overlay (Route, SCC) and the `tests/e2e/drivers/openshift.sh` driver are not part of this spec's implementation scope. The driver interface is defined here to establish the contract; the OpenShift implementation and its CI job come in a subsequent iteration |
| Env vars renamed with `E2E_` prefix | The existing `e2e-openshell.sh` uses `SANDBOX_TIMEOUT`, `PROVISION_TIMEOUT`, `SKIP_CLEANUP`, and `GATEWAY_NAMESPACE`. These are renamed to `E2E_SANDBOX_TIMEOUT`, `E2E_PROVISION_TIMEOUT`, `E2E_SKIP_CLEANUP`, and `E2E_NAMESPACE` to avoid namespace collisions with non-e2e configuration and make the e2e origin of these variables explicit |
| CI uses `make kind-up`, not raw `kind create cluster` | Reuses the same cluster setup path developers use locally. Ensures the CI environment is identical to local development. Avoids a second "create a Kind cluster" implementation that could drift |
