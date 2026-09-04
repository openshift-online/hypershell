# HyperShell architecture atlas

This directory contains the canonical Markdown and Mermaid sources for the
HyperShell architecture site. The generated HTML is intentionally not checked
in: CI builds it into `dist/architecture` and deploys that artifact to GitHub
Pages.

Start with the [source atlas](./source/index.md), or build the complete site
locally to view the rendered diagrams.

## Rendering approach

Each page contains:

- an inline SVG rendered from Mermaid syntax with
  [`beautiful-mermaid`](https://github.com/lukilabs/beautiful-mermaid), the
  open-source renderer behind Craft's
  [`agents.craft.do/mermaid`](https://agents.craft.do/mermaid) page;
- the Mermaid source in a collapsible **View Mermaid source** section; and
- a citation to the repository specification or component documentation that
  defines the view.

This keeps the pages usable as ordinary static files: they do not need a
JavaScript runtime, a Mermaid server, or the HyperShell application to render.
To revise a drawing, copy its source into the
[Craft Mermaid editor](https://agents.craft.do/mermaid/editor), update the
source, rebuild the site, and preserve the source citation.

## Page catalog

| Source | Scope |
| --- | --- |
| [`source/index.md`](./source/index.md) | End-to-end map and catalog |
| [`source/platform-topology.md`](./source/platform-topology.md) | Global Hub, Cloud Hubs, and ManagedClusters |
| [`source/api-server.md`](./source/api-server.md) | REST/gRPC API, Kind plugins, and persistence |
| [`source/control-plane.md`](./source/control-plane.md) | Watcher, reconcilers, and multi-cluster clients |
| [`source/gateway-workload.md`](./source/gateway-workload.md) | Gateway namespace, Supervisor, Sandboxes, and policy |
| [`source/client-surfaces.md`](./source/client-surfaces.md) | CLI, Go SDK, TypeScript SDK, and web API surfaces |
| [`source/data-plane.md`](./source/data-plane.md) | CNPG and standalone PostgreSQL modes |
| [`source/ingress.md`](./source/ingress.md) | Gateway API and OpenShift Route exposure modes |
| [`source/identity.md`](./source/identity.md) | Keycloak federation, OIDC, and service accounts |
| [`source/web-console.md`](./source/web-console.md) | React host, reusable UI package, BFF, and trust boundary |
| [`source/observability.md`](./source/observability.md) | Traces, metrics, probes, and dashboard aggregation |
| [`source/delivery.md`](./source/delivery.md) | Terraform, Tekton, ArgoCD, and tenant reconciliation |
| [`source/domain-model.md`](./source/domain-model.md) | HyperShell resource relationships |

## Build locally

From the repository root:

```shell
pnpm run check:architecture
pnpm run build:architecture
python3 -m http.server 9876 --directory dist/architecture
```

Then open <http://localhost:9876/>. The `Architecture site` GitHub Actions
workflow publishes the generated `dist/architecture` directory to the
repository's GitHub Pages site after changes land on `main`.

The pages intentionally describe the current repository architecture, including
configuration-selected alternatives such as `DATABASE_PROVIDER` and
`GATEWAY_INGRESS_MODE`. They are documentation artifacts, not a replacement for
the authoritative specifications cited within them.
