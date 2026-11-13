# OpenShell Image Auto-Update

**Date:** 2026-09-03
**Status:** Active
**JIRA:** HYPERSHELL-46

## Purpose

This spec defines the desired state for continuously updating the downstream
OpenShell gateway and supervisor container images consumed by the HyperShell
control plane. The mechanism must detect new upstream image tags/digests, propose
a bump via pull request, gate that bump on the platform's full PR test suite, and
merge only on green - preventing both silent drift and unvalidated upgrades.

The spec also defines the desired state for the controller image's own OpenShell
coordinate: every built HyperShell controller image embeds, as image metadata, the
single OpenShell gateway and supervisor release it supports, sourced from the same
`OPENSHELL_TAG` that drives the deployment manifests. This lets an operator read a
controller's OpenShell compatibility from the image alone, and lets release
validation detect an unexpected controller and gateway release combination.

## Scope

**In scope:**

- OpenShell gateway image (`quay.io/opendatahub/odh-openshell-gateway`)
- OpenShell supervisor image (`quay.io/opendatahub/odh-openshell-supervisor`)
- Pinned references in `deploy/base/` deployment manifests
- The HyperShell controller image's embedded supported-OpenShell-release label
  and its build and release validation

**Out of scope:**

- Internal registry mirrors - these are
  cluster-local copies updated by the mirror process, not by Renovate.
- Go source constants for non-OpenShell images (`defaultOAuth2ProxyImage`,
  etc.) - separate concern. (The console image has no Go constant: the
  `HYPERSHELL_CONSOLE_IMAGE` env var in
  `deploy/base/platform-resources/controller.yaml` is synced by
  `sync_openshell_version.py` via `OPENSHELL_CONSOLE_IMAGE` /
  `OPENSHELL_CONSOLE_DIGEST` in `OPENSHELL_VERSION`.)
- Database-backed image defaults (future; tracked by existing TODO in
  `config.go`).

## Image Pinning Strategy

All OpenShell image references are pinned using the `tag@digest` format:

```
quay.io/opendatahub/odh-openshell-gateway:v0.1.8-rhaiv.2@sha256:<digest>
```

- **Tag** provides human readability and version ordering.
- **Digest** provides immutable, content-addressed reproducibility.
- Both must be updated together on every bump.

## Watched Sources

| Image | Registry | Pinned In |
|-------|----------|-----------|
| `odh-openshell-gateway` | `quay.io/opendatahub` | `deploy/base/platform-resources/controller.yaml` |
| `odh-openshell-supervisor` | `quay.io/opendatahub` | `deploy/base/platform-resources/controller.yaml` |

The images are set as environment variable values (`GATEWAY_IMAGE`,
`GATEWAY_SUPERVISOR_IMAGE`) on the control-plane Deployment, which the
`StaticImageDefaults` reads at runtime via `os.Getenv`.

## Update Mechanism

### Renovate Custom Manager

Renovate's built-in managers do not detect container image references inside YAML
`env` `value:` fields. A `customManagers` entry with `customType: "regex"` scans
the deployment manifests for the `GATEWAY_IMAGE` / `GATEWAY_SUPERVISOR_IMAGE` env
var pattern and extracts `depName`, `currentValue`, and `currentDigest` for the
Docker datasource.

The regex matches the two-line YAML pattern:

```yaml
- name: GATEWAY_IMAGE
  value: <registry>/<org>/<name>:<tag>@sha256:<digest>
```

### Version Filtering

The upstream quay.io repositories contain thousands of tags - commit SHAs,
architecture-specific manifests (`-linux-m2xlarge-amd64`), and build artifacts
(`.git`, `.prefetch`). A `regex` versioning template restricts Renovate to tags
matching the release convention:

```
v{major}.{minor}.{patch}-rhaiv.{build}
```

The `build` component (the `-rhaiv.N` suffix) represents the downstream rebuild
number. Higher values are newer. Renovate's regex versioning compares `build`
after `patch`, so `v0.0.113-rhaiv.2` correctly supersedes `v0.0.113-rhaiv.1`.

### Schedule and Constraints

The auto-update follows the repository's existing Renovate conventions:

- **Schedule:** Monday 00:00-07:00 ET (inherited from top-level `schedule`)
- **Minimum release age:** 14 days (inherited from top-level `minimumReleaseAge`)
- **Concurrent PR limit:** shared with other Renovate PRs (top-level
  `prConcurrentLimit`)
- **Grouping:** both images are grouped into a single "OpenShell images" PR so
  gateway and supervisor stay in lockstep.

### Validation Gate

Every bump PR must pass the full platform PR test suite before merge:

- GitHub Actions `e2e.yml` workflow
- Tekton PR pipelines (`.tekton/*-pull-request.yaml`)

A red check blocks the PR. Renovate will not merge a failing PR regardless of
automerge configuration.

### Merge Policy

Automerge is disabled. Renovate opens the bump PR and a platform maintainer
must review and merge it after all required status checks pass. If any check
fails:

- The PR stays open and blocked.
- The failure is visible in the PR checks - not swallowed.
- A platform maintainer must investigate before the image is adopted.

## Cross-Stack Consistency

After a bump, every `GATEWAY_IMAGE` and `GATEWAY_SUPERVISOR_IMAGE` reference in
`deploy/base/` must carry the same tag and digest. Renovate's regex manager
updates all matches in all files within a single PR, maintaining consistency.

Internal registry mirrors in environment-specific overlays (IBM, etc.) are
updated separately through their own mirror+redeploy process, not by Renovate.

### Sandbox Workload Image for E2E and Kind Preload

`OPENSHELL_VERSION` SHALL contain `OPENSHELL_SANDBOX_IMAGE`, a full workload
image reference derived from the selected vendored chart's `sandbox.image`
mapping. A nonempty digest SHALL take precedence over the tag; an empty tag
without a digest SHALL resolve to `latest`. This workload pin is independent of
`OPENSHELL_TAG` and SHALL NOT configure a control-plane image default.

`make vendor-openshell-chart` SHALL synchronize this entry after replacing the
chart, and `make sync-openshell-version` SHALL also refresh it. `make check`
SHALL reject a missing or mismatched entry and an unsupported chart image schema.
The update-openshell skill SHALL require reviewing the resolved image on every
chart upgrade, including an upstream registry, tag, or digest change.

Kind preload, the Go e2e suite, and the Bash smoke walkthrough SHALL use that
same entry. All e2e sandbox-create paths SHALL pass it explicitly with `--from`,
and the Go suite SHALL verify that the running sandbox workload pod uses it.
They SHALL NOT rely on an independent image literal or the CLI/gateway default.

## Controller Image OpenShell Release Label

A published controller image carries standard OCI metadata for the controller
itself, but nothing in the image states which OpenShell gateway and supervisor
release it was built to support. Operators and release tooling must identify that
compatibility without checking out the source or reading deployment manifests.

The controller image therefore carries one dedicated label,
`app.hypershell.openshell.version`, that names the single supported OpenShell
release. This label is distinct from the controller's own version
(`org.opencontainers.image.version`); it describes OpenShell compatibility only.
A controller build supports exactly one release value, so the label holds exactly
one identifier - never a list, a compatibility range, a wildcard, or separate
gateway and supervisor values.

This section adds build metadata and validation only. It does not introduce a
version catalog, multiple supported releases per controller, or an upgrade
scheduler.

### Requirement: CICD-IMG-01 -- Controller Image Declares Its Single Supported OpenShell Release

The final HyperShell controller image SHALL carry one image label,
`app.hypershell.openshell.version`, whose value is a single non-empty OpenShell
release identifier (for example `v0.1.8-rhaiv.2`) that names the one gateway and
supervisor release the build supports. The value SHALL NOT be a list, a
comma-separated set, a version range, or a wildcard.

#### Scenario: CICD-IMG-01.01 -- Operator inspects a published controller image

- GIVEN a published HyperShell controller image
- WHEN an operator inspects the image configuration
- THEN exactly one `app.hypershell.openshell.version` label is present
- AND its value is a single non-empty OpenShell release identifier that names the
  supported gateway and supervisor release

### Requirement: CICD-IMG-02 -- Release Label Value Comes From OPENSHELL_TAG

The label value SHALL be derived at build time from `OPENSHELL_TAG` in the
repository `OPENSHELL_VERSION` file, which is the same source the deployment
manifests and the gateway-version capability already use. The build SHALL NOT
read the value from a separately maintained copy of the release string.

#### Scenario: CICD-IMG-02.01 -- Release bump flows into the label

- GIVEN `OPENSHELL_TAG` is updated to a new OpenShell release
- WHEN the controller image is rebuilt
- THEN the `app.hypershell.openshell.version` label equals the new `OPENSHELL_TAG`
- AND no other manually maintained release string was edited to make this happen

### Requirement: CICD-IMG-03 -- Controller Version Label Stays Distinct

`org.opencontainers.image.version` SHALL continue to describe the HyperShell
controller's own version. The supported OpenShell release SHALL be published only
through `app.hypershell.openshell.version` and SHALL NOT replace or overload
`org.opencontainers.image.version`.

#### Scenario: CICD-IMG-03.01 -- Both labels are present and independent

- GIVEN a published HyperShell controller image
- WHEN the image configuration is inspected
- THEN `org.opencontainers.image.version` holds the controller's own version
- AND `app.hypershell.openshell.version` holds the supported OpenShell release
- AND the two values are maintained independently

### Requirement: CICD-IMG-04 -- Every Architecture Variant Carries The Same Value

Every published controller architecture variant SHALL carry
`app.hypershell.openshell.version` with an identical value.

#### Scenario: CICD-IMG-04.01 -- amd64 and arm64 variants agree

- GIVEN the amd64 and arm64 controller images produced by one build
- WHEN each image configuration is inspected
- THEN both carry the `app.hypershell.openshell.version` label
- AND the two label values are identical

### Requirement: CICD-IMG-05 -- Validation Rejects Missing, Multiple, Or Inconsistent Values

Build and release validation SHALL fail when the controller image's
`app.hypershell.openshell.version` label is missing, holds more than one value, or
disagrees with `OPENSHELL_TAG` or the gateway and supervisor release
configuration. An automated test SHALL inspect the built image configuration, not
only the Dockerfile text, to confirm the value.

#### Scenario: CICD-IMG-05.01 -- Missing label fails validation

- GIVEN a controller image built without the `app.hypershell.openshell.version`
  label
- WHEN build or release validation runs
- THEN validation fails and reports the missing label

#### Scenario: CICD-IMG-05.02 -- Inconsistent value fails validation

- GIVEN a controller image whose `app.hypershell.openshell.version` value differs
  from `OPENSHELL_TAG` or from the gateway and supervisor release configuration
- WHEN build or release validation runs
- THEN validation fails and reports the inconsistency

#### Scenario: CICD-IMG-05.03 -- Consistent single value passes validation

- GIVEN a controller image whose `app.hypershell.openshell.version` value is a
  single identifier equal to `OPENSHELL_TAG`
- AND the gateway and supervisor release configuration agrees with that value
- WHEN build or release validation runs
- THEN validation passes

### Requirement: CICD-IMG-06 -- Label Is Documented With An Inspection Example

Repository documentation SHALL describe `app.hypershell.openshell.version`, state
that it names exactly one supported gateway and supervisor release, and show a
short example of reading the value from a built image configuration.

**Verification:** Inspection

#### Scenario: CICD-IMG-06.01 -- Documentation explains and demonstrates the label

- GIVEN the repository documentation for the controller image
- WHEN a reader looks up the OpenShell compatibility of a controller image
- THEN the documentation names `app.hypershell.openshell.version` and its
  single-release meaning
- AND it shows a short command that reads the value from a built image
  configuration

## Verification

- `renovate-config-validator` passes on the updated `renovate.json` in CI.
- A Renovate dry-run detects the current pinned images and proposes the expected
  version.
- A representative bump PR exercises the e2e pipeline and is correctly gated.
- An automated test inspects a built controller image and confirms that
  `app.hypershell.openshell.version` carries the single `OPENSHELL_TAG` value on
  every published architecture variant.
