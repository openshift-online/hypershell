# HyperShell architecture atlas

This directory contains the source and generated static architecture pages for
HyperShell. Start with [`index.html`](./index.html), then use the navigation on
each page to move between the platform topology, management plane, tenant
runtime, data, ingress, identity, web console, observability, delivery, and
domain-model views.

The canonical Markdown and Mermaid sources are in [`source/`](./source/).
The checked-in HTML pages are the current rendered baseline; CI rebuilds the
site into `dist/architecture` for GitHub Pages.

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
source and rendered SVG together, and preserve the source citation.

## Page catalog

| Page | Scope |
| --- | --- |
| [`index.html`](./index.html) | End-to-end map and catalog |
| [`platform-topology.html`](./platform-topology.html) | Global Hub, Cloud Hubs, and ManagedClusters |
| [`api-server.html`](./api-server.html) | REST/gRPC API, Kind plugins, and persistence |
| [`control-plane.html`](./control-plane.html) | Watcher, reconcilers, and multi-cluster clients |
| [`gateway-workload.html`](./gateway-workload.html) | Gateway namespace, Supervisor, Sandboxes, and policy |
| [`client-surfaces.html`](./client-surfaces.html) | CLI, Go SDK, TypeScript SDK, and web API surfaces |
| [`data-plane.html`](./data-plane.html) | CNPG and standalone PostgreSQL modes |
| [`ingress.html`](./ingress.html) | Gateway API and OpenShift Route exposure modes |
| [`identity.html`](./identity.html) | Keycloak federation, OIDC, and service accounts |
| [`web-console.html`](./web-console.html) | React host, reusable UI package, BFF, and trust boundary |
| [`observability.html`](./observability.html) | Traces, metrics, probes, and dashboard aggregation |
| [`delivery.html`](./delivery.html) | Terraform, Tekton, ArgoCD, and tenant reconciliation |
| [`domain-model.html`](./domain-model.html) | HyperShell resource relationships |

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
