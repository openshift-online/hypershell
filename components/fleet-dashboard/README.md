# fleet-dashboard

A small, read-only **fleet operational dashboard**: a Go BFF that aggregates
Prometheus fleet metrics, GitOps Promoter + Argo CD promotion state, and topology
ConfigMaps into a compact, cached JSON surface, with an embedded React SPA.

This is **not** the HyperShell product console; it is an operator-facing view of
fleet health and release-promotion flow.

## Design specifications

This component implements the `operational-dashboard` specifications
(`data-architecture` and `ui-architecture`), which are maintained in the
deployment GitOps repository rather than here. The specs are the source of truth
for the API surface, the data model, and the deployment topology; this repository
holds only the generic, deployment-agnostic implementation.

## Fleet-identity firewall

This image is public. Per the data-architecture spec (§3.5, "the fleet-identity
firewall"), the build **must not** contain anything that reveals the fleet's
structure: instance names, cluster names, hostnames, namespaces, service-account
or group names, repository slugs, or allowlists. Every such value arrives at
runtime through `FD_*` environment variables (see `pkg/config`) or is discovered
dynamically from the live cluster / promotion API. A CI grep
(`scripts/check_fleet_dashboard_firewall.sh`) enforces this on every change.

Product-level names that are already public are permitted as defaults: HyperShell
metric names, the GitOps Promoter CRD group, and the `delivery.hypershell.app/*`
label schema.

## Layout

| Path | Purpose |
| --- | --- |
| `cmd/fleet-dashboard` | entrypoint |
| `pkg/config` | runtime configuration (the firewall boundary) |
| `pkg/sources` | Prometheus, promotion, and topology data sources |
| `pkg/api` | HTTP handlers for `/api/{fleet,promotion,topology,instances}` |
| `pkg/auth` | TokenReview + SubjectAccessReview gate |
| `pkg/server` | HTTP server, static SPA serving, `/healthz` `/readyz` `/metrics` |
| `web` | embeds the built UI bundle (`packages/fleet-dashboard-ui`) |

## Configuration

All runtime configuration is via `FD_*` environment variables; see `pkg/config`
for the full list, defaults, and which values are required. Notable required
values: `FD_PROM_URL` and `FD_PROMOTER_NAMESPACE`. Authorization attributes
(`FD_SAR_*`) are required when `FD_AUTH_ENABLED=true` (the default).
