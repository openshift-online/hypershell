# OpenShell Gateway Additional Destination CA Specification

**Date:** 2026-08-25
**Status:** Draft (planning -- blocked on upstream [NVIDIA/OpenShell#3292](https://github.com/NVIDIA/OpenShell/pull/3292) merging and shipping in a tagged release)
**Parent:** `openshell-gateway.spec.md` - core gateway provisioning
**Related:** `openshell-gateway-tls.spec.md` - trusted CA bundle injection (the pattern this feature extends); `openshell-gateway-oidc.spec.md` - OIDC issuer trust (separate trust domain); `data-model.spec.md` - Gateway provisioning fields (this spec adds one)

---

## Purpose

This specification defines how HyperShell gateways consume the OpenShell
"additional destination CAs" feature so a gateway's sandboxes can trust private
PKI for egress destinations (e.g. private GitLab, internal artifact registries)
without replacing default trust roots and without touching OIDC, mTLS, or proxy
trust domains.

The bundle is **per-gateway and user-supplied**: an optional
`additional_ca_bundle` field on the Gateway API resource, set at create time and
updatable for rotation. It is NOT a hub-wide operator setting -- that was
considered and rejected for the initial cut (see Decisions). The scope covers
the API field and validation, control-plane materialization into the tenant
namespace, Helm value plumbing, gateway workload rolling on bundle change, and
certificate-rotation operations.

This document is a **plan** written against the upstream PR diff; it must be
re-validated against the merged result (the PR is currently labeled
`gator:blocked` and has unchecked CI lanes).

---

## Background: what upstream delivers

[NVIDIA/OpenShell#3292](https://github.com/NVIDIA/OpenShell/pull/3292) (branch
`feat/network-supervisor-additional-ca`) adds, per the PR diff:

- Chart value `supervisor.network.additionalCaConfigMapName` (string, empty
  disables the feature). The referenced ConfigMap must have a `ca.crt` key with
  one or more PEM certificate blocks (no inline PEM, no DER, no keys; bounded at
  `MAX_NETWORK_SUPERVISOR_TRUST_BUNDLE_BYTES` = 1 MiB minus the `ca.crt` key
  length to stay under the ConfigMap data limit).
- Gateway workload template (`_gateway-workload.tpl`): when the value is set, the
  operator-owned ConfigMap is mounted read-only at
  `/etc/openshell-tls/additional-ca-source/ca.crt` (subPath), a writable
  `gateway-state` emptyDir is added at `/var/lib/openshell-state`, and
  `XDG_STATE_HOME=/var/lib/openshell-state` is set on the gateway container.
- Gateway TOML template (`_toml.tpl`): derives
  `[openshell.supervisor.network] additional_ca_bundle` from the mount path.
  `gatewayConfig` defining `openshell.supervisor.network` fails rendering -- the
  chart owns that table.
- RBAC: the namespaced Role (shared workspace mode) grants
  `configmaps` `get`/`create` when additional CA is enabled, for the Kubernetes
  driver's immutable, content-addressed generation ConfigMaps. The ClusterRole
  grant applies to non-shared workspace modes (`managed`, `operator`). Since
  HyperShell main made `workspace_mode` platform-managed and server-defaulted
  to `managed` (OSG-CORE-18), NEW gateways run managed mode and take the
  ClusterRole path; only legacy pre-field gateways (chart default) run shared.
  Phase 1 must verify the ClusterRole is rendered and bound in our adoption
  shape for managed-mode gateways.
- Supervisor network: rustls roots augmented with the normalized bundle;
  child-process trust in delegated mode; proxy CA and gateway mTLS trust kept
  separate.
- Lifecycle: each runtime records its trust generation; after a gateway restart
  with a new bundle, stop/start of a sandbox reconciles stale runtime resources
  (Docker/Podman metadata, K8s content-addressed ConfigMap mounts, VM overlays)
  while preserving sandbox identity and durable workspace contents.
- **Rollout caveat:** the `checksum/gateway-config` pod annotation does NOT
  cover the operator-owned additional-CA ConfigMap. Changing the bundle data
  requires an explicit gateway workload restart. SubPath mounts are also not
  live-updated by the kubelet, so a restart is required even for the mounted
  file's contents to change.

Upstream depends on #3996 (already merged to main) and carries its own
cross-driver e2e (`e2e/rust/tests/additional_ca.rs`).

The upstream value names ONE ConfigMap per OpenShell gateway deployment. Because
HyperShell deploys one OpenShell gateway (one Helm release) per Gateway resource
in its own namespace, per-gateway bundles map directly onto the upstream
surface: each gateway's chart values name that gateway's own materialized
ConfigMap.

---

## Requirements

### Requirement: Per-gateway additional destination CA bundle

The Gateway API resource SHALL have an optional `additional_ca_bundle` string
field (inline PEM), part of the REST and gRPC create and update contracts, the
`gateways` table (via schema migration), and the Gateway provisioning fields
table in `data-model.spec.md`. When set, the control plane SHALL materialize the
bundle as a ConfigMap named `gateway-additional-ca` (key `ca.crt`, labeled
`hypershell.redhat.io/managed: "true"`) in the gateway's tenant namespace, and
SHALL set `supervisor.network.additionalCaConfigMapName` to
`gateway-additional-ca` in the rendered OpenShell chart values. The chart-owned
name space is `openshell-gateway-*` (fullname-prefixed), so a control-plane
owned `gateway-additional-ca` cannot collide with chart-adopted objects -- the
same separation the `gateway-trusted-ca` ConfigMap already relies on.

#### Scenario: bundle set at create

- GIVEN a Gateway create request with `additional_ca_bundle` containing one or
  more PEM certificate blocks
- WHEN the GatewayReconciler provisions the gateway
- THEN a ConfigMap named `gateway-additional-ca` with key `ca.crt` SHALL exist
  in the gateway's tenant namespace containing the bundle verbatim
- AND the rendered chart values SHALL set
  `supervisor.network.additionalCaConfigMapName: gateway-additional-ca`
- AND the rendered gateway TOML SHALL contain
  `[openshell.supervisor.network] additional_ca_bundle` pointing at the staged
  mount

#### Scenario: bundle absent

- GIVEN a Gateway with `additional_ca_bundle` unset or empty
- WHEN the GatewayReconciler provisions or reconciles the gateway
- THEN no `gateway-additional-ca` ConfigMap SHALL exist in the tenant namespace
- AND the chart value SHALL NOT be set
- AND gateway startup and sandbox egress SHALL use default trust roots only
  (no failure, no rendered mount)

#### Scenario: bundle rejected at the API boundary

- GIVEN a Gateway create or update request with `additional_ca_bundle` that is
  not one or more PEM certificate blocks (empty after trim, DER bytes, private
  key blocks, non-PEM text) or exceeds the API size bound (256 KiB)
- WHEN the request is submitted
- THEN the API server SHALL reject it with a validation error naming the field
  and the reason, WITHOUT persisting the gateway change
- AND the error SHALL NOT echo the bundle contents

#### Scenario: invalid materialized bundle (defense in depth)

- GIVEN a gateway's materialized `ca.crt` contains material the gateway rejects
  (possible only if the API validation was bypassed, e.g. direct DB writes)
- WHEN the gateway pod starts
- THEN the gateway SHALL fail startup with the field and source path, without
  leaking certificate contents
- AND the control plane SHALL surface the gateway's degraded state through
  existing gateway health checks (`openshell-gateway-health.spec.md`)

### Requirement: rotation updates the running gateway

Updating `additional_ca_bundle` SHALL rotate the gateway's trust: the control
plane SHALL re-materialize the ConfigMap and, because the chart's
`checksum/gateway-config` annotation does not cover the operator-owned
ConfigMap and subPath mounts are not live-updated, SHALL explicitly roll the
gateway workload when the materialized bundle data changes. The roll SHALL be
driven through Helm: the bundle's SHA-256 checksum is set as a `podAnnotations`
entry (`hypershell.redhat.io/additional-ca-checksum`) on the gateway workload
whenever the bundle is present, so a bundle change changes the pod template and
the Helm upgrade rolls the StatefulSet automatically (the same values-driven
label/annotation pattern `dev_build` uses; `podAnnotations` is a chart surface
that already exists in the vendored chart). The control plane
SHALL NOT attempt hot-reload (startup-only gateway configuration upstream).
Running sandboxes retain their trust generation until stopped; stop/start of a
sandbox reconciles it to the new generation while preserving sandbox identity
and durable workspace contents (upstream lifecycle behavior).

#### Scenario: operator or user rotates the bundle

- GIVEN a gateway is running with trust generation N
- WHEN the Gateway's `additional_ca_bundle` is updated via the API and the
  GatewayReconciler runs
- THEN the `gateway-additional-ca` ConfigMap data SHALL be updated
- AND the gateway workload SHALL roll (the checksum pod annotation changes the
  pod template, so the Helm upgrade restarts the StatefulSet; Helm's own
  `checksum/gateway-config` would not detect the change)
- AND the new gateway pods SHALL start with generation N+1
- AND running sandboxes SHALL retain generation N until stopped
- AND stop/start of a sandbox SHALL reconcile it to N+1 while preserving sandbox
  identity and durable workspace contents

### Requirement: clearing the bundle removes the trust

Removing the bundle SHALL be declarative (follow-source semantics): clearing
`additional_ca_bundle` (set to null/empty) SHALL cause the control plane to
delete the tenant `gateway-additional-ca` ConfigMap and stop setting the chart
value, rolling the gateway back to default trust roots on the next reconcile.

#### Scenario: bundle cleared

- GIVEN a gateway is running with a materialized additional CA bundle
- WHEN the Gateway's `additional_ca_bundle` is cleared via the API and the
  GatewayReconciler runs
- THEN the tenant `gateway-additional-ca` ConfigMap SHALL be deleted
- AND the chart value SHALL NOT be set on the subsequent Helm upgrade
- AND the gateway workload SHALL restart without the additional-CA mount and
  use default trust roots
- AND sandboxes SHALL reconcile to default trust on stop/start

---

## Integration plan

### Phase 0 -- Trigger (upstream; BLOCKER)

1. Upstream PR merges (currently `gator:blocked`; unchecked: full `mise run
   test`, Docker/K8s/VM CI e2e lanes). **Re-diff the merged result against this
   plan** -- value names, mount paths, and RBAC gating are taken from the PR diff
   and may move. Do not execute Phase 1 without this re-diff.
2. The opendatahub fork (`OPENSHELL_CHART_REPO`) tags a release containing the
   merge.

### Phase 1 -- Vendored chart bump (this repo; after Phase 0)

1. Bump `OPENSHELL_TAG` in `OPENSHELL_VERSION`; run `make sync-openshell-version`
   and `make vendor-openshell-chart` (Makefile ~line 257; clones the fork tag
   into `charts/openshell`).
2. Review the vendored chart diff for the surfaces above
   (`values.yaml`, `_gateway-workload.tpl`, `_toml.tpl`, `role.yaml`,
   `clusterrole.yaml`) and reconcile any helm-adoption deltas
   (`openshell-gateway-helm-adoption.spec.md`).
3. Confirm additional-CA RBAC for BOTH workspace modes: new gateways run
   `workspaceMode: "managed"` (server-defaulted, OSG-CORE-18), so the
   ClusterRole's `configmaps get/create` grant must render and bind correctly
   in our adoption shape; legacy shared-mode gateways take the namespaced Role
   path. Verify `rolebinding.yaml`/`clusterrolebinding.yaml` changes don't
   alter our adoption shape.

### Phase 2 -- API field + control-plane plumbing (this repo; CAN START PRE-MERGE)

Everything in this phase is inert until the Phase 1 chart bump lands: setting
`supervisor.network.additionalCaConfigMapName` on a chart that does not read it
is a Helm no-op, and no user can set the new API field before it ships. It can
therefore be implemented and merged before upstream merges.

1. **API field** (`components/api-server`): add `additional_ca_bundle` to the
   Gateway schema in `openapi.gateways.yaml` (Gateway, create request, patch
   request), the `Gateway`/`CreateGatewayRequest`/`UpdateGatewayRequest`
   messages in `proto/hypershell/v1/gateways.proto` (regenerate `pb.go`), the
   GORM model and wire structs (`plugins/gateways/model.go`, following
   `supervisor_image`), and an up/down migration adding
   `additional_ca_bundle TEXT` to `gateways` (`plugins/gateways/migration.go`).
2. **API validation**: on create and update, when the field is non-empty it must
   contain at least one `-----BEGIN CERTIFICATE-----` /
   `-----END CERTIFICATE-----` block pair, no `PRIVATE KEY` blocks, and be at
   most 256 KiB (our bound, comfortably under the upstream ~1 MiB ConfigMap
   bound). Reject with a field-naming validation error; never echo contents.
3. **Control-plane materialization** (`components/control-plane`): thread the
   field `pb.Gateway` → `gateway.GatewayConfig.AdditionalCABundle`
   (`internal/gateway/config.go`, next to `SupervisorImage`) →
   `internal/reconciler/reconciler.go` gwConfig assembly (~line 460). Add
   `reconcileAdditionalCABundle` to `internal/gateway/reconciler.go`, modeled on
   `reconcileTrustedCABundle` (~line 1034): non-empty bundle → create-or-update
   `gateway-additional-ca` in the tenant namespace (key `ca.crt`, managed
   labels); empty → delete the ConfigMap if present. Returns presence and
   whether the data changed.
4. **Values plumbing**: thread the presence bool through `deployGatewayViaHelm`
   (`helm_deploy.go` ~line 14) into `ValuesBuilder` (`internal/helm/values.go`)
   as `HasAdditionalCA` (next to `HasTrustedCA` ~line 97 / `HasVaultCA`
   ~line 99); in `Build`, when set:
   `setNestedValue(values, "gateway-additional-ca", "supervisor", "network",
   "additionalCaConfigMapName")`. Do NOT pass `openshell.supervisor.network`
   through `gatewayConfig` -- the chart fails rendering if that table is set
   there (enforced upstream).
5. **Gateway roll on change**: when the bundle is present, set its SHA-256 as a
   Helm `podAnnotations` entry
   (`hypershell.redhat.io/additional-ca-checksum`). The vendored chart already
   applies top-level `podAnnotations` to the gateway workload pod template
   (`_gateway-workload.tpl`), so any bundle change changes the pod template and
   the Helm upgrade rolls the StatefulSet automatically -- no imperative
   restart, no post-Helm patching, no uncovered-checksum gap. This uses a chart
   surface that exists in the current vendored chart (pre-bump), so rotation
   behavior is complete before Phase 1. Clearing the bundle omits the
   annotation, which rolls the gateway back to default trust the same way.
6. **Tests**: API model/presenter/validation tests (accept/rotate/clear/reject
   paths); `values_test.go` (value set only when bundle present; not set when
   absent; checksum annotation tracks the bundle); `helm_deploy_test.go`
   (signature threading); reconciler unit tests for materialization
   (create/update/delete/absent). The chart-render test asserting the rendered
   TOML/mount belongs to Phase 1 (the current vendored chart ignores the value)
   -- add it when the chart lands.

### Phase 3 -- Validation

1. Dev: kind workflow (`deploy/kind`) with a test private CA; verify a sandbox
   reaches a private-PKI HTTPS endpoint only when the gateway's bundle is set.
2. E2E: extend the ROKS pr-test lane
   (`components/pr-test/e2e-openshell-roks.sh`) with an additional-CA scenario
   (create gateway with bundle → sandbox egress to private-PKI endpoint →
   verify trust; update bundle → verify gateway roll → stop/start sandbox →
   verify new generation; clear bundle → verify default trust).
3. Fleet rollout: stage hub → canary profile hub → remaining hubs
   (ROKS `hypc-ibm-*`, ROSA HCP `hypc-aws-*` once deployed). No gitops changes
   are required -- the bundle rides the Gateway API, not cluster config.

### Phase 4 -- Operations

1. Rotation runbook: update `additional_ca_bundle` via the API (hsctl or
   console) → control plane rolls the gateway workload automatically →
   stop/start affected sandboxes to reconcile them to the new generation. The
   gateway roll is automatic (Phase 2 item 5); only the sandbox stop/start is a
   manual/agent-driven step.
2. Web-console gateway provisioning form exposure of the field is a follow-up
   (requires a mockup and the ui-build-gate per web-console conventions); the
   API + hsctl surface is the initial cut.

---

## Decisions

Resolved during spec review (2026-08-25):

1. **Per-gateway, not per-hub.** The bundle is a user-supplied Gateway field.
   A hub-wide operator-managed default was considered and rejected for the
   initial cut: one mechanism, one reconcile surface, and it directly serves the
   stated requirement that gateway owners configure and rotate their own trust.
   A hub-level base merged with a per-gateway override remains a possible
   follow-up if fleet operators later need to push one CA to all gateways; that
   would add a merge step in the reconciler, not change this spec's API surface.
2. **Inline PEM field, not a resource reference.** `additional_ca_bundle` is a
   plain string on Gateway create/update, like `credential_driver` and `image`.
   A reference to a secret-like resource would require a new resource kind or
   extending `SecretSource` (which is AgentRuntime-owned today) -- a much larger
   API surface for non-secret material. CA certificates are public material;
   storing PEM in the `gateways` table is acceptable.
3. **Tenant ConfigMap name `gateway-additional-ca`.** Same name shape as the
   `gateway-trusted-ca` precedent (control-plane-owned, unprefixed; the chart
   owns `openshell-gateway-*`). No collision with chart-adopted objects.
4. **Follow-source removal semantics.** Clearing the field deletes the tenant
   ConfigMap and unsets the chart value (declarative; gitops/API is the source
   of truth). The alternative -- keeping the last mirror ("sticky" trust) -- has
   no clean removal path and can drift silently. Accidental clearing is visible
   in the API and reversible by re-setting the field.
5. **Control plane owns the gateway roll on rotation.** Upstream deliberately
   excludes the operator-owned ConfigMap from `checksum/gateway-config`, and
   subPath mounts are not live-updated, so the HyperShell control plane must
   roll the gateway workload when the bundle changes. Mechanism: the bundle
   checksum rides Helm `podAnnotations`, so the roll happens through the normal
   Helm upgrade path (identical to how `dev_build` passes pod labels). This was
   upstream-neutral in the original plan (an optional enhancement); it is
   REQUIRED now that rotation is user-facing.

---

## Work-item checklist

- [ ] Phase 0: upstream merge + fork tag (BLOCKED; re-diff before Phase 1)
- [ ] Phase 2: API field + validation + migration (pre-merge safe)
- [ ] Phase 2: control-plane materialization + values plumbing + roll-on-change + tests (pre-merge safe)
- [ ] Phase 1: `OPENSHELL_TAG` bump, `vendor-openshell-chart`, chart diff review, chart-render test
- [ ] Phase 3: kind validation, ROKS e2e scenario
- [ ] Phase 4: rotation runbook; console form exposure (follow-up)
