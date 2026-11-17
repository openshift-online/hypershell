# ADR-0013: HyperShell Extension API to OpenShell API Mapping design decisions

Rationale relocated from [`specs/platform/extension-api-openshell-api-mapping.spec.md`](../../specs/platform/extension-api-openshell-api-mapping.spec.md) (AGT-EXT). The spec holds the normative behavior; this ADR holds the reasoning. Reference-only; not loaded by reconcile.

| Decision | Rationale |
|----------|-----------|
| Map Extension kinds to OpenShell's existing API, not a new OpenShell contract | OpenShell already provides workspaces, members, providers, refresh, and the inference route and exposes them to the `openshell` CLI. HyperShell is a convenience layer; it consumes that API rather than asking OpenShell to implement a bespoke HyperShell contract |
| Ground the mapping in the installed CLI and the production coordinator sequence | These are the two reachable sources of OpenShell's real surface from this repository; grounding in them keeps the mapping real and lets unconfirmed wire details be flagged honestly |
| Mark unconfirmed OpenShell capability CONFIRM-AGAINST-UPSTREAM rather than invent it | Role-name equivalence and the wire protocol behind each CLI command cannot be confirmed from here; fabricating them would make the declarative assertion false. The GitHub `app-installation-token` gap is resolved to a control-plane rotator (a decision, not a fabricated OpenShell capability) rather than left open |
| Resolve G-4 to a control-plane GitHub rotator; enumerate the residual G-5 | The GitHub minting that the installed OpenShell CLI cannot do is moved into a bounded control-plane rotator inside the declarative realization, not left as an open gap. The "fully declarative" claim stays credible because the one remaining imperative step (G-5) is named as a tracked follow-up |
| `ProviderSpec` -> OpenShell provider profile; `ProviderBinding` -> OpenShell provider | Matches the reuse model: a shared profile/capability definition referenced by many per-workspace provider instances, mirroring `provider profile import` + per-workspace `provider create` |
