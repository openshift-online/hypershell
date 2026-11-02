# Playwright console end-to-end testing

**Status:** Implemented
**Upstream:** [Playwright](https://playwright.dev/)

## Purpose

The deployed HyperShell web console and per-gateway OpenShell console are tested
as a user experiences them. Browser behavior is implemented in TypeScript with
Playwright. Bash may discover cluster routes and launch the suite, but it must
not implement browser interactions or assertions.

## Architecture

- `tests/e2e/e2e-console.sh` is a thin Kind/OpenShift infrastructure adapter. It
  selects the existing driver, discovers `E2E_CONSOLE_URL`, and launches the
  Playwright test.
- `components/web-console/playwright.live.config.ts` configures live-cluster
  Chromium, HTTPS handling, retries, traces, screenshots, videos, and reports.
- `components/web-console/e2e-live/console.live.spec.ts` owns the user journey.
- Playwright and Chromium use the versions already pinned by the web-console
  workspace. There is no second browser CLI. Container mode (below) uses the
  Playwright image whose tag matches that pinned version, so its Chromium is
  the same revision.
- `tests/e2e/e2e-console-container.sh` is the container-mode launcher. Like
  `e2e-console.sh`, it only prepares the environment and starts the same
  Playwright test.

## User-facing locator policy

Browser interactions must use Playwright's user-facing locators, in this order:

1. `getByRole` with an accessible name
2. `getByLabel`
3. `getByText` for visible content

Tests must not interact through CSS/XPath selectors, raw DOM evaluation,
generated element references, or `data-testid`. An inaccessible control is a
product accessibility defect, not a reason for the e2e suite to bypass the
user-facing interface.

## Required journey

The suite must:

1. Log in through Keycloak and verify the BFF session identity.
2. In long mode, submit the empty provisioning form and observe its validation.
3. Provision a uniquely named gateway and select `local-kind` when offered.
4. Wait for the API-reported `Running` phase and verify the rendered status.
5. Wait for the published console address and verify the accessible console link.
6. Log into the per-gateway console and verify its workspaces view is usable.
7. Verify the local placement choice and external CLI documentation link.
8. Delete the gateway through the visible actions menu.
9. Best-effort delete through the authenticated BFF if a prior assertion fails,
   unless `E2E_SKIP_CLEANUP=1`.

The existing distributed tracing test remains in the same Playwright live suite.

## Modes and configuration

- `E2E_MODE=short` runs the CI journey without extended form validation.
- `E2E_MODE=long` is the default and includes extended checks.
- `E2E_CONSOLE_URL` overrides route discovery.
- `E2E_OIDC_USERNAME` and `E2E_OIDC_PASSWORD` select the login identity.
- `E2E_GATEWAY_NAME` overrides the generated gateway name.
- `E2E_PROVISION_TIMEOUT` and `E2E_CONSOLE_READY_TIMEOUT` are seconds.
- `E2E_GATEWAY_VERSION_TIMEOUT` is the seconds allowed for the reconciled
  gateway runtime version to appear before the CLI documentation link is
  required.
- `E2E_BROWSER_PAGE_TIMEOUT_MS` bounds rendered-page assertions.
- `E2E_CONSOLE_ARTIFACT_DIR` selects Playwright's failure artifact directory.
- `E2E_SKIP_CLEANUP=1` preserves the gateway for investigation.
- `E2E_CONSOLE_CONTAINER=true` runs the suite in container mode (Kind only).
- `E2E_BROWSER_HOST_RESOLVER_RULES` passes Chromium a `--host-resolver-rules`
  value. Container mode sets it; it is unset otherwise.

## Container mode

`E2E_CONSOLE_CONTAINER=true make e2e-console` runs the suite inside the official
Playwright image (`mcr.microsoft.com/playwright:v<pinned version>-noble`) on the
container network of the Kind nodes (`kind`). It needs neither host port 443
forwarding nor Chromium's system libraries on the host. It is supported only
with the `kind` driver; `e2e-console.sh` SHALL refuse it with any other driver.

- The repository SHALL be mounted at its host path, and the workspace's own
  Playwright SHALL run the suite, so the test code and versions are the ones
  the host would use.
- The address of the cloud-provider-kind Envoy SHALL be read from the
  `hypershell-gw` Gateway status. Node-side requests (`page.request`) SHALL
  reach the console host through an `--add-host` entry for it.
- Chromium sends `*.localhost` names to loopback regardless of `/etc/hosts`, so
  the browser SHALL get `--host-resolver-rules=MAP *.localhost <Envoy address>`
  through `E2E_BROWSER_HOST_RESOLVER_RULES`. That covers the console, Keycloak,
  and the per-gateway OpenShell consoles.
- The container SHALL run as the invoking user (with `--userns=keep-id` on
  podman), so artifacts written to the mounted repository keep that ownership.
- Every `E2E_*` variable and `CI` SHALL be passed into the container.

## Commands

```shell
make e2e-console
E2E_MODE=short make e2e-console
E2E_CONSOLE_CONTAINER=true make e2e-console
pnpm --filter @openshift-online/hypershell-web-console test:e2e:console:live
pnpm --filter @openshift-online/hypershell-web-console test:e2e:tracing:live
pnpm test:e2e:all
```

Kind requires the normal port 443 forwarding established by `make kind-up`,
except in container mode.
The root `test:e2e:all` script establishes Kind with Jaeger first and then runs
the mocked Chromium, live console, and live tracing suites sequentially.
CI runs the short console suite concurrently with the non-browser OpenShell e2e
suite, then runs live distributed-trace verification.
