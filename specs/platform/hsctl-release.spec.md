# hsctl and HyperShell bundle releases

**Status:** Implemented in source; requires Konflux tenant and managed release configuration.
**Applies to:** `components/cli/Dockerfile`, `components/cli/build-release.sh`, `.tekton/hypershell-cli-main-*.yaml`, `scripts/release_bundle.py`, `pipelines/release-bundle/pipeline.yaml`

## Purpose

Distribute the CLI built by Konflux as both a Quay image and executable downloads in a GitHub release corresponding to each HyperShell release bundle.

## Requirements

### HSR-01 -- Build once and distribute the same binaries

Konflux SHALL build `hypershell-cli-main` from `components/cli` on main pushes and validate changes on pull requests and merge queues. The image SHALL contain a runnable Linux CLI at `/usr/local/bin/hsctl` and these regular executable files under `/releases/`:

| OS | Architecture | Asset |
| --- | --- | --- |
| Linux | amd64 | `hsctl-linux-amd64` |
| Linux | arm64 | `hsctl-linux-arm64` |
| macOS | amd64 | `hsctl-darwin-amd64` |
| macOS | arm64 | `hsctl-darwin-arm64` |
| Windows | amd64 | `hsctl-windows-amd64.exe` |

All targets SHALL use CGO disabled. The initial Konflux runtime image is Linux amd64; the download matrix is independent of runtime image platforms. The publisher SHALL extract these files from the released CLI image by digest, verify layer digests, and reject incomplete sets or nonregular files. It SHALL NOT rebuild the CLI during publication. Binary signing and macOS notarization are outside this initial implementation.

### HSR-02 -- Six required bundle components

A newly published bundle SHALL require API server, control plane, web console, CLI, agent-runtime, and agent-runtime-slim. Fleet dashboard remains independently distributed. Each component SHALL record its name, immutable released image digest, source repository and commit, and `tags`, containing its full Git SHA tag in the released repository. The publisher SHALL verify that the tag resolves to the recorded digest. Mutable `latest` aliases and release timestamp aliases are not canonical bundle tags.

These are additive fields to schema version 1. Consumers SHALL select workloads by component name and tolerate the additional CLI and agent runtime components. They SHALL use digests for deployment. Existing three- and four-component bundles remain historical records; new publication requires six components.

Given a Snapshot missing any required component, publication SHALL fail before registry writes. Given an unchanged CLI, its earlier accepted build MAY be reused in later bundles. A bundle does not promise all component builds share a source commit or wait for other in-flight builds.

### HSR-03 -- One GitHub release per bundle identity

The GitHub release and Git tag SHALL use the existing `release-bundle-<snapshot-time>-<release-uid-hash>` identifier. The Git tag SHALL identify `manifests.git.revision`, the Snapshot component commit containing all component revisions. Individual component revisions SHALL remain in the attached bundle. One Konflux Release UID produces one identity; a new Konflux Release resource for the same Snapshot produces a new bundle and GitHub release.

The release SHALL attach all five binaries, the exact `bundle.json` stored in the OCI artifact, and `SHA256SUMS` covering the other six assets. Its notes SHALL include the OCI bundle digest and each component's full commit tag and immutable image reference. Automatic publication SHALL leave GitHub's `latest` designation unchanged, so retries of old bundles cannot replace the user's latest selection.

### HSR-04 -- Failure and retry behavior

Publication SHALL require a successful managed release, accepted push metadata, valid source ancestry, and all six released images and tags. It SHALL publish the OCI bundle before completing the GitHub release. GitHub assets SHALL be uploaded to a draft; the release SHALL become public only after all assets have matching SHA-256 digests and sizes.

Quay and GitHub cannot commit atomically. A failed GitHub operation MAY leave a Quay bundle and a draft. Retrying the same Konflux Release SHALL reuse that bundle and resume the same draft. Existing bundle contents, Git tag targets, release notes, and uploaded assets SHALL be validated; conflicting contents SHALL fail rather than be overwritten. A completed matching release SHALL be a successful no-op. An unfinished GitHub upload in `starter` state MAY be deleted and retried while the release is still a draft.

The final pipeline SHALL report success only after both publications succeed. Operations SHALL serialize retries of a given Konflux Release; concurrent final pipelines for the same identity are not supported. Historical bundles SHALL NOT be backfilled automatically.

### HSR-05 -- Configuration and credentials

The tenant SHALL register the CLI Component and ImageRepository. The managed release mapping SHALL promote it to `quay.io/redhat-services-prod/hcm-eng-prod-tenant/hypershell-main/hypershell-cli-main` with the same Git SHA tag convention as the services.

The final pipeline SHALL mount the GitHub App private key from Secret `hypershell-github-release` in `hcm-eng-prod-tenant`, with keys `app-id`, `installation-id`, and `private-key.pem`. On every run it SHALL sign an RS256 App JWT and request an installation token restricted to the `hypershell` repository and Contents write permission. The token SHALL remain in process memory. The App installation must permit creating the bundle tag. Credentials SHALL NOT be stored in source or passed on command lines.

The GitHub App and tenant secret were provisioned and live publication succeeded on 2026-10-09. Their absence SHALL NOT cause a fallback to an anonymous or developer credential. A missing secret prevents the publication step from starting; invalid App configuration or token exchange fails before publication.

The documented RelEng Vault/ExternalSecret SOP provisions secrets in `rhtap-releng-tenant`. It SHALL NOT be assumed to provision this tenant's secret. Before activation, RelEng must confirm an approved secret delivery mechanism for `hcm-eng-prod-tenant`. If using Vault, the source record must include `generated-by`, `generated-on`, `owner-team`, and `expires` provenance metadata. Existing Quay write access for the API-server build repository is reused for bundle publication; released component images are public.

## Validation

Local tests cover incomplete Snapshots, mismatched digests, ancestry, extraction, draft publication and retries. Cross-compilation validates all five targets. The four-component flow was verified in a live Konflux release on 2026-10-09. The six-component extension requires the rollout in `agent-runtime-images.spec.md`.
