# HyperShell web console

The web console is a React 19 single-page application built with React Router Framework Mode, Vite, PatternFly 6, TanStack Query, React Intl, and the generated HyperShell SDK. A separate Fastify package serves the production assets and browser-facing security controls.

## Application boundaries

React, React Router, and TanStack Query are driving adapters. A real product workflow enters a framework-independent application use case, which owns the purposeful ports needed for API, session, storage, time, and domain-probe effects. SDK and other infrastructure imports belong only in concrete adapters and the explicit runtime composition root. Pure helpers and presentational components remain direct code and do not receive ceremonial interfaces.

API-backed workflows enter application use cases through capability-shaped ports. Generated SDK access is isolated in adapters, while TanStack Query owns presentation-side cache and synchronization policy.

`domain-probes/` is the shared browser/BFF probe contract and fan-out implementation. Each real workflow must own a closed probe union and checked catalog, then compose at least two production sinks. ESLint enforces the inward dependency rule, SDK and telemetry adapter boundaries, external-effect restrictions, and the production `console.*` ban.

## Prerequisites

- Node.js 24.18.1 or newer in the 24.x line (see the repository `.node-version`)
- pnpm 11.15.1

Install the pinned pnpm artifact without relying on Node's bundled Corepack:

```sh
bash scripts/bootstrap_pnpm.sh
pnpm install --frozen-lockfile
```

The bootstrap script downloads the exact npm tarball over TLS and verifies its committed SHA-256 digest before installation.

## Hot-reload development

Start the API in its isolated no-auth development mode in one terminal:

```sh
cd components/api-server
make run-no-auth
```

From the repository root, start Vite with React Router hot-module replacement:

```sh
pnpm dev
```

Open <http://127.0.0.1:5173>. Browser requests retain the production-shaped relative `/api` path and Vite proxies them to `http://127.0.0.1:8000`. Set `WEB_CONSOLE_API_ORIGIN` to another loopback HTTP(S) origin when needed; credentials and non-loopback targets are rejected. No browser-visible flag enables authentication bypass.

## Verification

```sh
pnpm check:web
pnpm --filter @openshift-online/hypershell-web-console exec playwright install chromium
pnpm test:e2e:chromium
```

The complete browser matrix is available through `pnpm test:e2e` after installing Chromium, Firefox, and WebKit.

## Production container

Build from the repository root so the shared lockfile and SDK are available:

```sh
podman build --file components/web-console/Dockerfile --tag localhost/hypershell-web-console:dev .
podman run --rm \
  --read-only \
  --user 65532:65532 \
  --cap-drop ALL \
  --security-opt no-new-privileges \
  --publish 8080:8080 \
  localhost/hypershell-web-console:dev
```

The image is multi-stage and pins Red Hat's `hi/nodejs` builder and runtime
variants by per-architecture digest (`amd64` and `arm64`). `docker`/`podman`
`--platform` selects the pin. Its runtime contains the production BFF dependency closure and built assets, runs as a numeric non-root user, writes no files, exposes port 8080, and provides `/health/live` and `/health/ready` probes.

The BFF proxies `/api/*` to the fixed `HYPERSHELL_API_ORIGIN` origin, which defaults to `http://127.0.0.1:8000` for a colocated API. Set that variable to an HTTP(S) origin reachable from the container when the API is deployed separately. Browser cookies and authorization headers are not forwarded; the current product assumption is that all gateway records are visible. `HYPERSHELL_API_TIMEOUT_MS` bounds each upstream request and defaults to 30 seconds.

## Dashboard metrics

The BFF queries Prometheus from the server. The browser does not receive metrics
credentials. Existing dashboard access rules still apply.

| Variable                        | Purpose                                                                                                |
| ------------------------------- | ------------------------------------------------------------------------------------------------------ |
| `PROMETHEUS_URL`                | Application metrics endpoint. Defaults to `http://127.0.0.1:9090` for local development.               |
| `PROMETHEUS_NAMESPACE`          | Instance namespace for gateway counts and provision duration. Omit only for a dedicated metrics store. |
| `CLUSTER_PROMETHEUS_URL`        | CPU, memory, node, and pod metrics endpoint. Defaults to `PROMETHEUS_URL`.                             |
| `CLUSTER_PROMETHEUS_TOKEN_FILE` | File with the bearer token for the cluster endpoint.                                                   |
| `CLUSTER_PROMETHEUS_CA_FILE`    | File with the trusted CA bundle for the cluster endpoint.                                              |
| `PROMETHEUS_QUERY_TIMEOUT_MS`   | Timeout for each query. Defaults to 10 seconds.                                                        |

On OpenShift, use the application Prometheus endpoint for gateway metrics and
`https://thanos-querier.openshift-monitoring.svc:9091` for cluster metrics. Give
the console service account the `cluster-monitoring-metrics-api` Role through
a RoleBinding in `openshift-monitoring`. Mount a projected service account token
and a ConfigMap with the injected OpenShift service CA bundle. Set the two file
variables to these paths. The BFF reads both files for each request to support
rotation. File credentials require an explicit HTTPS cluster endpoint. TLS
verification stays enabled, and redirects cannot receive the token.

Cluster charts show the whole cluster. Gateway charts use the configured instance
namespace. Repeated gateway gauge samples from API replicas do not add to counts.
The instance filter is fixed by server configuration, not by browser input.
Query failures return HTTP 502. Provision duration has no data until a gateway
provision operation supplies observations.

The metric families have different namespace labels. API scrape metrics use
`hypershell_gateways_total{namespace="<instance>"}`. The controller sends provision
duration through OTLP, so `gateway_provision_duration_seconds_*` uses
`k8s_namespace_name="<instance>"`. Its `namespace` label identifies the collector's
namespace, not the instance. Do not use that scrape label for duration queries.

Cluster queries accept samples with a value of zero, including idle CPU use,
zero ready nodes, and zero pod counts in a phase. An empty result means the
required series is absent, not that its value is zero. The backend returns 502
in that case. A new CPU series also needs enough samples for `rate()`;
the chart can be unavailable until those samples exist. Provision duration is
unavailable when the selected instance has no retained observations.

Both metrics paths limit each response to 4 MiB. The application path applies
the limit after decompression. Credential failures identify the environment
variable and the failure type in server logs. File paths and credential values
are not logged; the browser still receives only `Metrics unavailable`.

The reusable OpenShift deployment settings are in
[`deploy/gitops-base/components/openshift-dashboard-metrics`](../../deploy/gitops-base/components/openshift-dashboard-metrics/README.md).
Site overlays select that component and supply their application metrics endpoint
and instance RoleBinding. This component can also be used with the GitOps wrapper
from PR #251. The application owns its deployment patch and credential mounts.
