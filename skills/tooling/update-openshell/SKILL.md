---
name: update-openshell
description: >
  Update HyperShell to a target upstream OpenShell release (github.com/NVIDIA/OpenShell).
  Bumps the pinned gateway/supervisor image versions across the repo, triages the
  release notes for contract-affecting changes, verifies the rendered gateway config
  still matches upstream, and folds every mistake or lesson back into this skill and
  the specs. Use when a new OpenShell version ships upstream, when asked to "update
  OpenShell", "bump the gateway version", "pull the latest openshell", or to sync
  HyperShell with an upstream release. Releases are frequent (roughly daily), so this
  is a regular operation.
---

# Update OpenShell

Sync HyperShell to a target upstream OpenShell release. OpenShell ships very
frequently (roughly one release per day), so treat this as routine maintenance,
not a one-off. The mechanical part is a version-pin sweep; the part that needs
judgment is deciding whether a new upstream feature changes HyperShell's own
approach.

**This skill is self-reinforcing.** Every run that surfaces a mistake, a missed
file, a schema drift, or a judgment call MUST be folded back into this skill
(the [Version footprint](#version-footprint), [Contract surfaces](#contract-surfaces-to-triage),
or [Learnings log](#learnings-log)) and into the affected spec, in the same PR.
The next run should never re-learn what this run learned.

## Usage

```text
/update-openshell            # update to the latest upstream release
/update-openshell v0.0.106   # update to a specific tag
```

## User Input

```text
$ARGUMENTS
```

## Source of truth

The **authoritative** current version pins live in the control-plane deployment
manifest `deploy/base/controller.yaml` as environment
variables:

```yaml
- name: GATEWAY_IMAGE
  value: quay.io/opendatahub/odh-openshell-gateway:<VERSION>@sha256:<DIGEST>
- name: GATEWAY_SUPERVISOR_IMAGE
  value: quay.io/opendatahub/odh-openshell-supervisor:<VERSION>@sha256:<DIGEST>
```

`config.go` reads these at runtime via `os.Getenv("GATEWAY_IMAGE")` /
`os.Getenv("GATEWAY_SUPERVISOR_IMAGE")` - there are no hardcoded fallback
constants. A control-plane deployed without these env vars will fail to
provision gateways. Every other occurrence of the version in the repo is a copy
of these and MUST agree with them after a run.

## Version footprint

Update the version in every file below. Discover the live list before editing -
do not trust this table blindly; it is a checklist, not a source of truth:

```bash
grep -rln "openshell/\(gateway\|supervisor\):" . | grep -v '\.git/'
grep -rn  "<OLD_VERSION>" . | grep -v '\.git/'      # must return only intentional fixtures afterwards
```

| File | What to change | Notes |
|------|----------------|-------|
| `deploy/base/controller.yaml` | `GATEWAY_IMAGE`, `GATEWAY_SUPERVISOR_IMAGE` env vars | **Source of truth** - change here first |
| `specs/platform/data-model.spec.md` | `supervisor_image` default | Spec citation |
| `specs/platform/openshell-gateway.spec.md` | gateway + supervisor defaults | Spec citation (multiple) |
| `specs/platform/openshell-gateway-credentials.spec.md` | example manifests | Spec citation |
| `specs/platform/global-architecture.spec.md` | version in the API-compat note | Also re-check the `v1beta1` claim (see below) |
| `components/pr-test/e2e-openshell-roks.sh` | `GW_IMAGE`, `GW_SUPERVISOR_IMAGE` defaults | ROKS e2e |
| `specs/platform/openshell-gateway-database.spec.md` | example gateway image refs | **Was pinned to a git SHA, not a semver tag** - normalize to the release tag |
| `scripts/kind/lib.sh` | `GATEWAY_IMAGE` default | Local Kind |
| `skills/deploy/ibm-cluster/SKILL.md` | mirror + `openshell gateway add` commands + `v1beta1` API-version note | ROKS mirror docs |

**Do NOT change** version strings that are illustrative fixtures or historical
facts rather than the active pin:
- the example tag in
  `components/control-plane/internal/gateway/validation_test.go` (it exercises the
  image-reference regex, not the deployed version);
- the sentence in `specs/platform/openshell-gateway-credentials.spec.md` that says
  "Upstream OpenShell **v0.0.101 introduced** pluggable credential storage
  drivers" - this records *which* release added a feature and must not be bumped
  (note it is written `v0.0.101` with the `v` prefix; the pins are
  `gateway:0.0.101` without it, so a `gateway:<ver>`-anchored substitution
  naturally skips it).

If unsure whether an occurrence is a pin or a fixture, treat it as a pin and note
the ambiguity in the [Learnings log](#learnings-log).

**Beware the forbidden-terms whitelist.** `make check` runs
`scripts/check_forbidden_terms.py`, which forbids em dashes (U+2014) and a few
terms, with exceptions listed **by line number** in
`.forbidden-terms-whitelist.json`. If a spec edit *inserts or removes lines* in a
file that has whitelist entries (e.g. `global-architecture.spec.md`, whose Mermaid
control-plane node ids and an example GitOps path are whitelisted), those line
numbers shift and `make check` fails until the whitelist is updated. Always run
`make check` and fix the whitelist line numbers in the same change.

**Also check the sandbox base image** in `config.go`
(`defaultSandboxImage`). It currently floats on `:latest`
(`ghcr.io/nvidia/openshell-community/sandboxes/base:latest`), which is a separate
image line from gateway/supervisor and is NOT version-locked to the release. If
upstream starts publishing versioned sandbox base images, pin it here and add it
to the footprint table.

## Workflow

1. **Resolve versions.** Read the current version from `config.go`. Resolve the
   target: for `latest`, `gh api repos/NVIDIA/OpenShell/releases --jq '.[0].tag_name'`
   (skip the `dev` tag); for an explicit tag, verify it exists with
   `gh api repos/NVIDIA/OpenShell/releases/tags/<tag>`. Strip the leading `v` for
   the image tag (`v0.0.106` -> `0.0.106`).

2. **Triage the release range.** For every release between current and target
   (exclusive of current, inclusive of target), read the body and the Full
   Changelog:

   ```bash
   for t in <ranged tags>; do
     gh api repos/NVIDIA/OpenShell/releases/tags/$t --jq '.name, .body'
   done
   ```

   Classify each change against the [Contract surfaces](#contract-surfaces-to-triage).
   Produce a short **impact report**: `mechanical` (tag bump is enough) vs
   `needs-decision` (upstream changed a contract HyperShell renders, or shipped a
   feature that overlaps something HyperShell hand-rolls). Surface every
   `needs-decision` item to the user before finalizing - do not silently absorb it.

3. **Bump the pins.** Edit `config.go` first, then sweep the rest of the
   [Version footprint](#version-footprint). Per the repo convention *"Image
   references must match across the stack"*, grep all overlays and manifests too:

   ```bash
   grep -rn "openshell/\(gateway\|supervisor\)" components/*/deploy components/control-plane/manifests
   ```

4. **Verify contracts.** For each `needs-decision` item from step 2, check the
   thing it touches:
   - **Gateway config (`gateway.toml`)**: diff the keys the control plane renders
     (`components/control-plane/manifests/gateway/configmap.yaml`,
     `components/control-plane/internal/gateway/config.go`) against the new
     gateway's expected config. A renamed/removed/added key is a code change, not
     a tag bump.
   - **Sandbox API version**: confirm the `agents.x-k8s.io` API version the new
     gateway requires still matches the RBAC and manifests
     (`components/control-plane/manifests/gateway/rbac.yaml`,
     `.../networkpolicy.yaml`, `deploy/base/controller-rbac.yaml`).
   - **Credential drivers**: if upstream changed the pluggable credential storage
     surface, re-check `ValidateCredentialDriverConfig` and the credentials spec.
   - **PKI / TLS / Route**: if upstream shipped ingress/PKI features, evaluate
     against HyperShell's own approach (see the [Learnings log](#learnings-log)).

5. **Build and test.**
   ```bash
   cd components/control-plane && go build ./... && go vet ./... && go test ./...
   make check
   ```
   For ROKS, note that the new images must be **re-mirrored** into the internal
   registry before they can be pulled - see
   [`ibm-cluster`](../../deploy/ibm-cluster/SKILL.md). This skill does not perform
   the mirroring; it only updates the tags the mirror commands reference.

6. **Update specs.** Bump the version citations (footprint table) and correct any
   spec claim the triage invalidated (e.g. the `v1beta1` API-version note in
   `global-architecture.spec.md`).

7. **Feed the loop.** Before committing, complete
   [Self-reinforcement](#self-reinforcement) below.

8. **Commit + report.** Conventional commit
   (`chore(deps): bump OpenShell to <version>`), summarize the impact report in
   the body, and open follow-up issues for any `needs-decision` item deferred for
   a maintainer call.

## Contract surfaces to triage

A release is only a mechanical tag bump if it touches **none** of these. If it
does, the item is `needs-decision`:

| Surface | Why it matters | Where HyperShell depends on it |
|---------|----------------|--------------------------------|
| Gateway/supervisor config schema (TOML) | Control plane renders `gateway.toml` | `manifests/gateway/configmap.yaml`, `internal/gateway/config.go` |
| Sandbox CR / `agents.x-k8s.io` API version | Gateway manages sandboxes; RBAC grants on it | `manifests/gateway/rbac.yaml`, `networkpolicy.yaml`, `deploy/ibm/controller-clusterrbac.yaml` |
| Credential storage drivers | HyperShell selects/validates drivers | `ValidateCredentialDriverConfig`, `openshell-gateway-credentials.spec.md` |
| gRPC/proto surface | API server + control plane speak gRPC | `components/api-server/proto/` |
| Gateway/supervisor CLI flags & env | Control plane sets them | `configmap.yaml`, deployment manifests |
| PKI / TLS / ingress (Route, cert-manager, Gateway API) | HyperShell hand-rolls per-tenant PKI + ingress | `internal/gateway/` reconciler, ingress specs |
| Auth (OIDC) | OIDC is the client auth mechanism (no client mTLS) | `ValidateOIDCConfig`, gateway OIDC config |

## Verification

Before considering the update done:

1. `grep -rn "<OLD_VERSION>" .` returns only intentional fixtures (and this
   line, and the Learnings log if it records an old version).
2. `config.go` and every footprint file agree on the new version.
3. `go build ./...`, `go vet ./...`, `go test ./...`, and `make check` pass.
4. Every `needs-decision` item is either resolved in this PR or filed as a
   tracked follow-up - never silently dropped.
5. [Self-reinforcement](#self-reinforcement) is complete.

## Self-reinforcement

This is the step that keeps the skill from decaying. It is **mandatory** on every
run:

1. **Any file that was pinned but missing from the footprint table** -> add it to
   the table.
2. **Any occurrence that was a fixture (should not change) but looked like a
   pin** -> note it explicitly in the table's "do not change" list.
3. **Any contract drift, build break, test failure, or manual fix** encountered
   -> add a dated entry to the [Learnings log](#learnings-log) describing the
   symptom and the fix, and update the affected spec so the fact lives in the
   desired-state docs too.
4. **Any judgment call surfaced to the user** (adopt upstream feature vs keep
   HyperShell's approach) -> record the decision and its rationale in the
   Learnings log.

If a run produced no new lessons, that is itself worth a one-line log entry
(`vX.Y.Z: clean mechanical bump, no drift`) so the cadence is visible.

## Learnings log

Newest first. Each entry: version, date, what happened, what changed in the repo.

- **v0.0.109 (2026-08-19, second run, 0.0.106 -> 0.0.109; validated on ROKS):**
  Unlike 106, this bump was NOT config-schema-neutral - three regressions only
  surfaced running the full ROKS e2e (`components/pr-test/e2e-openshell-roks.sh`),
  which ended at **22/22 passing**. Pin-only diffing would have missed all three;
  the lesson is to run the live sandbox e2e on a version bump, not just `make check`.
  - **Sandbox client TLS is required in `combined` topology.** A prior mTLS-removal
    sweep had conflated two distinct concerns and deleted BOTH: (A) external-client
    mTLS (`client_ca_path`, `tls-client-ca` volume) - correctly removed for
    Route+OIDC; and (B) sandbox client-TLS provisioning (`client_tls_secret_name` +
    the `openshell-client` cert-manager Certificate) - **wrongly** removed. 0.0.109's
    Combined topology needs (B) so sandbox runners get `OPENSHELL_TLS_CA` to verify
    the gateway server cert; without it the sandbox agent crash-loops on
    `OPENSHELL_TLS_CA is required`. Restored the `openshell-client` Certificate in
    `components/control-plane/internal/gateway/reconciler.go` and
    `client_tls_secret_name` in `manifests/gateway/configmap.yaml`, each with a
    comment stating it is internal sandbox↔gateway TLS, NOT external mTLS.
  - **StatefulSet/Deployment collision.** `manifests/gateway/statefulset.yaml` was
    still in the deploy `order` slice alongside the Deployment, racing it and
    leaving an orphaned crash-looping `openshell-gateway-0`. Removed the entry from
    the order slice in `reconciler.go` and `git rm`'d the file (spec: "Always
    Deployment"). Verify a fresh tenant NS has a Deployment and no StatefulSet.
  - **Workspace membership is a second, non-claim-derived authz layer.** 0.0.109
    gates `sandbox create` on BOTH the OIDC role AND an explicit workspace
    membership record. A standard `openshell-user` is not implicitly a member of
    `default`, so create fails with `not a member of workspace 'default'` until an
    admin runs `openshell workspace member add --workspace default --subject <sub>
    --role user`. Added that grant to the ROKS e2e's developer-RBAC step (mirrors
    `tests/e2e/e2e-openshell.sh`) and documented it in `ibm-cluster/SKILL.md` §5.9.
  - **No downloadable 0.0.109 CLI.** `install.sh` with `OPENSHELL_VERSION=0.0.109`
    404s; only the gateway *container image* is published at 0.0.109. The 0.0.98
    CLI reads the same config format and drives a 0.0.109 gateway fine (and has the
    `workspace` subcommand that the ancient system `/bin/openshell` 0.0.55 lacks),
    so the e2e now defaults `OPENSHELL` to `~/.local/bin/openshell`.
  - **v1beta1 confirmed** (the 106-entry's unverified claim): the running 0.0.109
    gateway serves sandboxes via `agents.x-k8s.io/v1beta1` (agent-sandbox v0.5.5),
    matching the note in `global-architecture.spec.md` / `ibm-cluster/SKILL.md`.

- **v0.0.106 (2026-08-19, initial skill + first run, 0.0.101 -> 0.0.106):**
  - Range 102-106 was mechanically safe for the pin bump; no config-schema,
    Sandbox-API, or credential-driver break observed in the release notes.
  - `needs-decision` surfaced: **v0.0.106 shipped upstream
    "cert-manager external issuer + OpenShift passthrough Route"
    (NVIDIA/OpenShell#2468)**, which overlaps HyperShell's hand-rolled per-tenant
    self-signed CA + passthrough Route. Recorded as a follow-up to evaluate
    adopting upstream's issuer instead of the self-signed root (ties to the PKI
    hardening follow-up and the OIDC-over-mTLS decision). NOT adopted in the bump.
  - `defaultSandboxImage` floats on `:latest` and is not tied to the release tag;
    left as-is, flagged in the footprint.
  - The example tag in `validation_test.go` is a regex fixture, not a pin - do not
    bump it.
  - **Footprint gap found by discovery grep:**
    `specs/platform/openshell-gateway-database.spec.md` was NOT in the original
    footprint table AND pinned the gateway image to a **git SHA**
    (`21da343c9f...`) instead of a semver tag. Normalized it to `0.0.106` and
    added it to the table. Lesson: always run the discovery grep; the table is a
    checklist, not the source of truth.
  - **Historical fact, not a pin:** `openshell-gateway-credentials.spec.md`
    line ~12 ("v0.0.101 introduced pluggable credential storage drivers") must
    not be bumped. Added to the do-not-change list.
  - **`make check` gotcha (cost real time):** two prior spec commits on this
    branch used em dashes (forbidden) and inserted lines that shifted the
    `.forbidden-terms-whitelist.json` line numbers for the Mermaid control-plane
    node ids and an example GitOps path. `make check` failed until em dashes were
    replaced with ` - `
    and the whitelist line numbers were corrected (27/76/77/912 -> 35/84/85/925).
    Now documented in the "Beware the forbidden-terms whitelist" note above.
  - **Build hygiene:** `go build` failed on an unrelated accidental paste (two
    stray URLs appended to `reconciler.go` in the working tree, uncommitted).
    Removing it unblocked the build. Lesson: run `go build`/`go vet`/`go test`
    even for a "docs + pin" change - the working tree may carry unrelated breakage
    the pin bump would otherwise mask.
  - **Unverified claim:** the `v1beta1` Sandbox API-version note (in
    `global-architecture.spec.md` and `ibm-cluster/SKILL.md`) was version-bumped
    to "gateway 0.0.106 prefers v1beta1" by substitution. Release notes 102-106
    show sandbox changes (105 stop/start, 103 policy revisions) but no
    API-version bump, so v1beta1 almost certainly still holds - but this was NOT
    independently verified against the 0.0.106 image. Verify against the running
    gateway on the next ROKS deploy and correct if wrong.
