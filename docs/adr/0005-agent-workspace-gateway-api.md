# ADR-0005: AgentWorkspace Realization via OpenShell's Existing API design decisions

Rationale relocated from [`specs/platform/agent-workspace-gateway-api.spec.md`](../../specs/platform/agent-workspace-gateway-api.spec.md) (AGT-WSP). The spec holds the normative behavior; this ADR holds the reasoning. Reference-only; not loaded by reconcile.

| Decision | Rationale |
|----------|-----------|
| Consume OpenShell's existing API, not a bespoke gateway gRPC contract | OpenShell already exposes workspace, member, provider, refresh, and inference operations (the `openshell` CLI drives them; `sdlc-coordinator` uses them in production). A HyperShell-specific gateway gRPC service would make OpenShell implement a contract for a convenience layer that should consume, not extend, OpenShell's own API |
| Keep the `ProviderRealizer` seam; change only its adapter | The seam, the provider model, the `SecretPending` gate, realize-before-prune, and cluster-local material all stay; only the realizer's target changes from a bespoke endpoint to OpenShell's existing API, minimizing churn |
| Gateway admin OIDC identity, short-lived session per operation | Matches how the coordinator authenticates to the gateway and keeps one operational and failure/retry discipline, reusing the per-operation admin-connection pattern of the database channel without reusing a bespoke contract |
| Use the `openshell` CLI operation surface as the authoritative API, wire details CONFIRM-AGAINST-UPSTREAM | OpenShell's server source is not reachable from this repository; grounding in the CLI operations keeps the spec real and defers exact endpoints/field names to OpenShell upstream without fabricating them |
| Carry durable material; mint only the GitHub token, through the provider API | Keeps the realization consistent with `agent-secret-late-binding.spec.md`: for OpenShell-refreshed strategies OpenShell holds and rotates the usable credential; for GitHub the control plane mints the installation token from the PEM and sets it through `provider update --credential`. In no case does the control plane place or rotate a raw Kubernetes `Secret` |
