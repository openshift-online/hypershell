# OpenShell Gateway Additional Destination CA Specification

**Date:** 2026-08-25
**Status:** In progress (upstream [NVIDIA/OpenShell#3292](https://github.com/NVIDIA/OpenShell/pull/3292) MERGED to main 2026-10-27 as `2c31ac7e` and re-diffed against this plan; remaining gate is the opendatahub fork syncing + tagging a release with images built)
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

This document was a **plan** written against the upstream PR diff; the merged
result has since been re-validated against it (2026-10-27, see "Re-diff
status" under Background) with no material divergence.

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
  `/etc/openshell-tls/additional-ca-source/ca.crt` (subPath). A writable
  `gateway-state` emptyDir at `/var/lib/openshell-state` and
  `XDG_STATE_HOME=/var/lib/openshell-state` on the gateway container are
  rendered UNCONDITIONALLY in the merged result (the PR diff had tied them to
  additional CA; re-diff 2026-10-27).
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
cross-driver e2e (`e2e/rust/tests/additional_ca.rs`) and Helm chart unit
tests (`deploy/helm/openshell/tests/network_additional_ca_test.yaml`) --
the latter is the reference for this spec's deferred chart-render test.

**Re-diff status (2026-10-27):** the PR merged to upstream main as
`2c31ac7e` (merged 2026-10-27). All chart-surface claims above were verified
against the merged result: value path
`supervisor.network.additionalCaConfigMapName`, mount path/subPath, TOML
derivation and the `gatewayConfig` table guard, shared-mode namespaced Role
`configmaps get/create` gating, ClusterRole for non-shared modes only, the
`checksum/gateway-config` non-coverage comment, and the top-level
`podAnnotations` surface this spec's rotation mechanism relies on. The only
deltas from the PR-diff description are the unconditional state volume noted
above and the chart unit tests. The opendatahub fork's `main`
(`v0.1.5-rhaiv.4`) does NOT yet contain the merge: the fork must sync
upstream main and cut a new tag (and build the `odh-openshell-*` images at
it) before Phase 1 can vendor.

The upstream value names ONE ConfigMap per OpenShell gateway deployment. Because
HyperShell deploys one OpenShell gateway (one Helm release) per Gateway resource
in its own namespace, per-gateway bundles map directly onto the upstream
surface: each gateway's chart values name that gateway's own materialized
ConfigMap.

---

## Requirements

### Requirement: OSG-ACA-01 -- Per-gateway additional destination CA bundle

The Gateway API resource SHALL have an optional `additional_ca_bundle` string
field (inline PEM), part of the REST and gRPC create and update contracts, the
`gateways` table (via schema migration), and the Gateway provisioning fields
table in `data-model.spec.md`. When set, the control plane SHALL compose it with
the operator tier (OSG-ACA-04) into an effective bundle, and SHALL materialize
that bundle as a CONTENT-ADDRESSED ConfigMap named
`openshell-destination-ca-<sha256[:8]>` (key `ca.crt`, labeled
`hypershell.redhat.io/managed: "true"`) in the gateway's tenant namespace,
setting `supervisor.network.additionalCaConfigMapName` to that name in the
rendered OpenShell chart values. The name embeds the bundle digest, so any
change to the effective trust produces a new ConfigMap name, which changes the
workload volume reference and rolls the gateway through the normal Helm upgrade
path -- no imperative restart, no pod annotation, and old generations remain
mountable until namespace cleanup. The control plane SHALL additionally
x509-validate the user bundle on its own before composition, so a valid
operator bundle cannot mask invalid user material inside a merged PEM (defense
in depth behind the API boundary).

#### Scenario: OSG-ACA-01.01 -- bundle set at create

- GIVEN a Gateway create request with `additional_ca_bundle` containing one or
  more PEM certificate blocks
- WHEN the GatewayReconciler provisions the gateway
- THEN a content-addressed ConfigMap with key `ca.crt` SHALL exist in the
  gateway's tenant namespace containing the effective bundle (the user bundle
  when no operator tier applies, otherwise the composed bundle per OSG-ACA-04)
- AND the rendered chart values SHALL set
  `supervisor.network.additionalCaConfigMapName` to that content-addressed name
- AND the rendered gateway TOML SHALL contain
  `[openshell.supervisor.network] additional_ca_bundle` pointing at the staged
  mount

#### Scenario: OSG-ACA-01.02 -- bundle absent

- GIVEN a Gateway with `additional_ca_bundle` unset or empty
- WHEN the GatewayReconciler provisions or reconciles the gateway
- THEN no user-tier trust SHALL apply; when the operator tier is configured it
  still materializes per OSG-ACA-04, otherwise the chart value SHALL NOT be set
- AND gateway startup and sandbox egress SHALL use default trust roots (or the
  operator tier only), with no failure and no user-tier rendered mount

#### Scenario: OSG-ACA-01.03 -- bundle rejected at the API boundary

- GIVEN a Gateway create or update request with `additional_ca_bundle` that is
  not one or more PEM certificate blocks (empty after trim, DER bytes, private
  key blocks, non-PEM text) or exceeds the API size bound (256 KiB)
- WHEN the request is submitted
- THEN the API server SHALL reject it with a validation error naming the field
  and the reason, WITHOUT persisting the gateway change
- AND the error SHALL NOT echo the bundle contents

#### Scenario: OSG-ACA-01.04 -- invalid materialized bundle (defense in depth)

- GIVEN a gateway's materialized `ca.crt` contains material the gateway rejects
  (possible only if the API validation was bypassed, e.g. direct DB writes)
- WHEN the gateway pod starts
- THEN the gateway SHALL fail startup with the field and source path, without
  leaking certificate contents
- AND the control plane SHALL surface the gateway's degraded state through
  existing gateway health checks (`openshell-gateway-health.spec.md`)

### Requirement: OSG-ACA-02 -- rotation updates the running gateway

Updating `additional_ca_bundle` SHALL rotate the gateway's trust: the control
plane SHALL re-materialize the effective bundle and, because the chart's
`checksum/gateway-config` annotation does not cover the operator-owned
ConfigMap and subPath mounts are not live-updated, SHALL roll the gateway
workload when the effective bundle changes. The roll is INHERENT to the
content-addressed naming (OSG-ACA-01): a changed bundle produces a new
`openshell-destination-ca-<digest>` name, which changes the workload volume
reference, so the Helm upgrade rolls the gateway Deployment with no imperative
restart and no pod annotation. Because the name is derived from content rather
than from the Gateway spec generation, trust changes also bypass the
generation-gate convergence check (`DestinationCATrustNeedsUpdate`), so
operator-tier and user-tier rotations apply to converged gateways. The control
plane SHALL NOT attempt hot-reload (startup-only gateway configuration
upstream). Running sandboxes retain their trust generation until stopped;
stop/start of a sandbox reconciles it to the new generation while preserving
sandbox identity and durable workspace contents (upstream lifecycle behavior).

#### Scenario: OSG-ACA-02.01 -- operator or user rotates the bundle

- GIVEN a gateway is running with trust generation N
- WHEN the Gateway's `additional_ca_bundle` is updated via the API and the
  GatewayReconciler runs
- THEN a NEW content-addressed ConfigMap generation SHALL be materialized (the
  old generation remains mountable until namespace cleanup)
- AND the gateway workload SHALL roll (the volume reference changes to the new
  name, so the Helm upgrade restarts the Deployment; Helm's own
  `checksum/gateway-config` would not detect the change)
- AND the new gateway pods SHALL start with generation N+1
- AND running sandboxes SHALL retain generation N until stopped
- AND stop/start of a sandbox SHALL reconcile it to N+1 while preserving sandbox
  identity and durable workspace contents

### Requirement: OSG-ACA-03 -- clearing the bundle removes the trust

Removing the bundle SHALL be declarative (follow-source semantics): clearing
`additional_ca_bundle` (set to null/empty) SHALL cause the control plane to
stop including user-tier trust in the effective bundle. When the operator tier
is configured it resumes unchanged (a new content-addressed generation rolls
the gateway onto operator trust only); otherwise no destination-CA ConfigMap is
referenced and the gateway rolls back to default trust roots on the next
reconcile. Superseded generations are reclaimed by namespace cleanup.

#### Scenario: OSG-ACA-03.01 -- bundle cleared

- GIVEN a gateway is running with a materialized additional CA bundle
- WHEN the Gateway's `additional_ca_bundle` is cleared via the API and the
  GatewayReconciler runs
- THEN the effective bundle SHALL no longer contain the user PEM
- AND the chart value on the subsequent Helm upgrade SHALL reference the
  operator tier's generation, or SHALL NOT be set when no operator tier applies
- AND the gateway workload SHALL restart without the user-tier trust (operator
  trust only, or default trust roots)
- AND sandboxes SHALL reconcile to default trust on stop/start

---

### Requirement: OSG-ACA-04 -- user and operator tier composition

The hub-wide operator tier (`GATEWAY_ADDITIONAL_CA_CONFIGMAP_NAME`, optionally
capability-gated by `GATEWAY_ADDITIONAL_CA_REQUIRED_CAPABILITY`; see
`destination_ca.go`) and the per-gateway user tier SHALL compose per a single
hub-wide policy, `GATEWAY_ADDITIONAL_CA_MERGE_MODE`:

- `merge` (DEFAULT): when a gateway declares user trust AND the operator tier
  is configured, the effective bundle concatenates both PEMs, so fleet-wide
  operator trust (for example a private GitLab CA) keeps applying alongside
  user-authored trust. Gateways without a user bundle take the operator tier
  unchanged; gateways on hubs without an operator tier take the user bundle
  unchanged.
- `user-only`: a gateway with a user bundle replaces the operator tier for
  that gateway; gateways without a user bundle still take the operator tier.

#### Scenario: OSG-ACA-04.01 -- merge is the default

- GIVEN a hub with the operator tier configured and a gateway whose
  `additional_ca_bundle` is set
- WHEN the GatewayReconciler materializes destination trust
- THEN the effective bundle SHALL contain both PEMs
- AND the content-addressed name SHALL differ from either tier alone

#### Scenario: OSG-ACA-04.02 -- user-only policy replaces operator trust

- GIVEN a hub with `GATEWAY_ADDITIONAL_CA_MERGE_MODE=user-only` and a gateway
  whose `additional_ca_bundle` is set
- THEN the effective bundle SHALL be exactly the user PEM

#### Scenario: OSG-ACA-04.03 -- no user bundle keeps the operator tier

- GIVEN a hub with the operator tier configured and a gateway whose
  `additional_ca_bundle` is empty
- THEN the effective bundle SHALL be exactly the operator PEM, unchanged by
  the merge policy

## Integration plan

### Phase 0 -- Trigger (upstream; fork tag remains the blocker)

1. Upstream PR merged 2026-10-27 as `2c31ac7e` (previously `gator:blocked`).
   Re-diff of the merged result against this plan: DONE 2026-10-27 (see
   "Re-diff status" above) -- no value-name, mount-path, or RBAC changes;
   only the unconditional state volume and chart unit tests differ from the
   PR-diff description.
2. The opendatahub fork (`OPENSHELL_CHART_REPO`) tagged releases containing
   the merge (`v0.1.6-rhaiv.*`); HyperShell main vendored `v0.1.6-rhaiv.4`
   (the additional-CA chart surfaces are in `charts/openshell` on main). DONE.

### Phase 1 -- Vendored chart bump (this repo; after the fork tag)

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

LANDED (rebased onto main, which had since shipped the operator tier and the
v0.1.6-rhaiv.4 chart). The user tier builds on main's operator-tier machinery
rather than duplicating it:

1. **API field** (`components/api-server`): `additional_ca_bundle` on the
   Gateway schema (`openapi.gateways.yaml`), proto messages
   (`Gateway=33`/`CreateGatewayRequest=21`/`UpdateGatewayRequest=25`, renumbered
   twice as main claimed earlier numbers), the GORM model, migration
   `migrationAddAdditionalCABundle`, presenters, and the PLAT-DATA-23
   field-policy classification (user-writable, the allowlist amendment).
2. **API validation** (`additional_ca_bundle_validation.go`): PEM CERTIFICATE
   blocks only, no `PRIVATE KEY`, no other `BEGIN` block types, at most 256 KiB
   (comfortably under the upstream ~1 MiB ConfigMap bound). Field-naming
   validation errors that never echo contents; REST PATCH distinguishes omitted
   from explicit null (both null and empty string clear).
3. **Control-plane composition** (`internal/gateway/destination_ca.go`):
   `destinationCABundle` composes the user bundle with the operator tier per
   OSG-ACA-04, x509-validates the user PEM on its own before composition
   (fail closed), and derives the content-addressed
   `openshell-destination-ca-<sha256[:8]>` name.
   `reconcileDestinationCABundle` materializes the effective bundle in the
   tenant namespace (key `ca.crt`, managed labels; old generations retained).
   `DestinationCATrustNeedsUpdate` (the generation-gate bypass) takes the user
   bundle so user rotations reach converged gateways.
4. **Values plumbing**: `ValuesBuilder.AdditionalCAConfigMapName` (main's
   operator-tier field, now carrying the composed name) →
   `supervisor.network.additionalCaConfigMapName`. Do NOT pass
   `openshell.supervisor.network` through `gatewayConfig` -- the chart fails
   rendering if that table is set there (enforced upstream).
5. **Gateway roll on change**: inherent to the content-addressed naming (see
   OSG-ACA-02) -- a changed effective bundle changes the volume reference and
   the Helm upgrade rolls the Deployment. No pod annotation, no imperative
   restart.
6. **Tests** (landed): API validation/presenter/desired-state/gRPC CRUD
   coverage; `values_test.go` (name set only when present, omitted when empty;
   the chart-render test `TestBuild_AdditionalCARendersInChart` asserts the
   rendered TOML derives `additional_ca_bundle` and the workload mounts the
   named ConfigMap against the vendored v0.1.6 chart);
   `destination_ca_test.go` (operator tier, user tier, merge and user-only
   composition, fail-closed validation, rotation generations, capability
   gating).

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
2. CLI ergonomics: the `additional_ca_bundle` properties in
   `openapi.gateways.yaml` carry `x-file-input: true`, so once the pinned
   rh-trex-ai cli-generator ships the file-input extension (branch
   `feat/cli-file-input` in the generator repo) and the pin is bumped, hsctl
   gains `--additional-ca-bundle-file <path>` on create and update alongside
   the raw-value switch. The annotation is inert under the current pinned
   generator (verified: no CLI or TUI drift). Until the pin bump, the
   ergonomic path is `hsctl create gateway --body request.json`.
3. Web-console gateway provisioning form exposure of the field is a follow-up
   (requires a mockup and the ui-build-gate per web-console conventions); the
   API + hsctl surface is the initial cut.

---

## Decisions

1. **Per-gateway user tier, composed with the operator tier.** The user
   bundle is a Gateway field (the stated requirement: gateway owners configure
   and rotate their own trust). HyperShell main later shipped a hub-wide
   operator tier (`GATEWAY_ADDITIONAL_CA_CONFIGMAP_NAME`); rather than
   duplicating it, the user tier composes with it per a merge policy (OSG-ACA-04
   and decision 6).
2. **Inline PEM field, not a resource reference.** `additional_ca_bundle` is a
   plain string on Gateway create/update, like `credential_driver` and `image`.
   A reference to a secret-like resource would require a new resource kind or
   extending `SecretSource` (which is AgentRuntime-owned today) -- a much larger
   API surface for non-secret material. CA certificates are public material;
   storing PEM in the `gateways` table is acceptable.
3. **Content-addressed ConfigMap naming** (`openshell-destination-ca-<digest>`,
   shared with the operator tier). SUPERSEDES the original fixed-name
   `gateway-additional-ca` decision: adopting main's content-addressed pattern
   makes the roll-on-rotation inherent (a name change changes the workload
   volume reference), deletes the pod-annotation machinery, and retains old
   generations for running pods until namespace cleanup.
4. **Follow-source removal semantics.** Clearing the field removes user-tier
   trust from the effective bundle (declarative; the API is the source of
   truth); the operator tier, if configured, resumes unchanged. The
   alternative -- keeping the last mirror ("sticky" trust) -- has no clean
   removal path and can drift silently. Accidental clearing is visible in the
   API and reversible by re-setting the field.
5. **Control plane owns the gateway roll on rotation.** Upstream deliberately
   excludes the operator-owned ConfigMap from `checksum/gateway-config`, and
   subPath mounts are not live-updated, so the control plane must roll the
   gateway workload when the bundle changes. Mechanism: the content-addressed
   name change (decision 3), through the normal Helm upgrade path. This was
   upstream-neutral in the original plan (an optional enhancement); it is
   REQUIRED now that rotation is user-facing.
6. **Tier composition is a hub-wide policy, merge by default.** When both
   tiers exist for one gateway the default (`GATEWAY_ADDITIONAL_CA_MERGE_MODE`
   unset or `merge`) concatenates the PEMs: the operator tier already applies
   hub-wide when enabled, so merging adds nothing the operator did not opt
   into, and a user setting a bundle does not silently drop fleet trust.
   `user-only` makes the user bundle replace the operator tier for gateways
   that declare one (user decision, 2026-11: "offer to merge optionally").
7. **PLAT-DATA-23 allowlist amendment.** `additional_ca_bundle` is
   user-writable (create + update) -- user-authored trust material, not
   platform provisioning config -- recorded as an explicit exception in the
   default-deny Gateway field policy (see `data-model.spec.md`).

---

## Work-item checklist

- [x] Phase 0a: upstream merge (merged 2026-10-27, `2c31ac7e`) + re-diff (done, no material divergence)
- [x] Phase 0b: fork sync + tag + images (`v0.1.6-rhaiv.*`; HyperShell main vendored `v0.1.6-rhaiv.4`)
- [x] Phase 1: vendored chart on main carries the additional-CA surfaces; chart-render test landed (`TestBuild_AdditionalCARendersInChart`)
- [x] Phase 2: API field + validation + migration + PLAT-DATA-23 classification (landed)
- [x] Phase 2: control-plane two-tier composition + values plumbing + inherent roll + tests (landed, unified with main's operator tier)
- [ ] Phase 3: kind validation, ROKS e2e scenario (user tier + composition + rotation)
- [ ] Phase 4: rotation runbook; console form exposure (follow-up)
