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

## Scope

**In scope:**

- OpenShell gateway image (`quay.io/opendatahub/odh-openshell-gateway`)
- OpenShell supervisor image (`quay.io/opendatahub/odh-openshell-supervisor`)
- Pinned references in `deploy/base/` deployment manifests

**Out of scope:**

- Internal registry mirrors (e.g. `deploy/ibm/kustomization.yaml`) - these are
  cluster-local copies updated by the mirror process, not by Renovate.
- Go source constants for non-OpenShell images (`defaultConsoleImage`,
  `defaultOAuth2ProxyImage`, etc.) - separate concern.
- Database-backed image defaults (future; tracked by existing TODO in
  `config.go`).

## Image Pinning Strategy

All OpenShell image references are pinned using the `tag@digest` format:

```
quay.io/opendatahub/odh-openshell-gateway:v0.0.109-rhaiv.0@sha256:<digest>
```

- **Tag** provides human readability and version ordering.
- **Digest** provides immutable, content-addressed reproducibility.
- Both must be updated together on every bump.

## Watched Sources

| Image | Registry | Pinned In |
|-------|----------|-----------|
| `odh-openshell-gateway` | `quay.io/opendatahub` | `deploy/base/controller.yaml`, `deploy/base/control-plane/deployment.yaml` |
| `odh-openshell-supervisor` | `quay.io/opendatahub` | `deploy/base/controller.yaml`, `deploy/base/control-plane/deployment.yaml` |

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

- **Schedule:** Monday 00:00–07:00 ET (inherited from top-level `schedule`)
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

Automerge is enabled (`automerge: true`, `automergeType: "pr"`). Renovate merges
the PR only after all required status checks pass. If any check fails:

- The PR stays open and blocked.
- The failure is visible in the PR checks - not swallowed.
- A platform maintainer must investigate before the image is adopted.

## Cross-Stack Consistency

After a bump, every `GATEWAY_IMAGE` and `GATEWAY_SUPERVISOR_IMAGE` reference in
`deploy/base/` must carry the same tag and digest. Renovate's regex manager
updates all matches in all files within a single PR, maintaining consistency.

Internal registry mirrors in environment-specific overlays (IBM, etc.) are
updated separately through their own mirror+redeploy process, not by Renovate.

## Verification

- `renovate-config-validator` passes on the updated `renovate.json` in CI.
- A Renovate dry-run detects the current pinned images and proposes the expected
  version.
- A representative bump PR exercises the e2e pipeline and is correctly gated.
