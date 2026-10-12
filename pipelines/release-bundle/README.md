# Release bundle

This pipeline publishes the six Snapshot images after a successful Konflux
managed release as one OCI artifact. It uses the existing API server build repository:

```text
quay.io/redhat-user-workloads/hcm-eng-prod-tenant/hypershell-main/hypershell-api-server-main
```

The tags start with `release-bundle-`. The artifact contains `bundle.json` with
media type `application/vnd.hypershell.release.v1+json`. Each component has its
released image reference, with a SHA-256 digest, and its source Git revision.
The component images are in `quay.io/redhat-services-prod`.

The bundle also records `manifests.git.url` and `manifests.git.revision`. The
revision is the newest component source commit in the accepted Snapshot. All
component revisions must be ancestors of that commit, and that commit must be
in the fetched history of `main`. Sibling component commits are allowed when
another component commit in the Snapshot contains both. If no Snapshot component
commit contains every component revision, publication fails. SHA sort order does
not affect this rule. The publisher checks that the workload base,
Keycloak theme, and dashboard metrics component exist at that revision. It does
not select the current head of `main`, which can advance while a release waits.
This is an additive field in the version 1 bundle format.

Changes under `deploy/`, or to the bundle publisher, trigger the API-server push
build. That build records the new commit in a Snapshot, so a manifest-only change
can produce a bundle while retaining the other component images.

The bundle is a release record. It is not a workload image or a Tekton task
bundle. Consumers must select the bundle tags and download the JSON layer.

## Release checks

The publisher requires all of these conditions:

- The Release uses `hypershell-releaseplan` in `hcm-eng-prod-tenant`.
- The managed pipeline has status `True` and reason `Succeeded`.
- The Snapshot contains exactly the six expected components.
- The release lists at least one image, with no unknown or duplicate components.
- Each image listed in the release matches the Snapshot digest and release repository.
- All six Snapshot digests are available in their fixed release repositories.
- Each source revision belongs to the history of `hypershell` main.
- Any event-type metadata identifies a push event.

A push event can retain the number of its merged pull request. The publisher
accepts that number only when the same metadata prefix has an explicit `push`
event. Pull request and merge queue events remain blocked.

The final pipeline can run after a failed managed pipeline. It must check
`ManagedPipelineProcessed`; `Released` is not complete until the final pipeline
finishes. A failed check stops publication.

The managed pipeline can reduce its working Snapshot to one component. It can
also filter out images that were already released. Thus, `status.artifacts.images`
can contain only part of the original Snapshot. The publisher retains the full
Snapshot image set and checks each digest in its fixed release repository before
it writes the bundle. An omitted image that is unavailable there stops publication;
the publisher does not substitute a tag or another digest.

Konflux can keep an earlier image for an unchanged component. The bundle keeps
all six source revisions. A Snapshot can also contain an earlier image while
another component build is still running. This pipeline preserves the accepted
Snapshot; it does not add a test that waits for all builds from one commit.

The tag uses the Snapshot creation time and a hash of the Release UID. The OCI
creation time also uses the Snapshot time. A retry of an old release does not
receive a new creation time. Retries of the same Release produce the same content
and digest for a fixed publisher version. Retries compare the existing bundle bytes and refuse to overwrite changed content.
A publisher upgrade cannot rewrite an old bundle tag. Consumers must retain and use
the bundle digest.

## Enable the pipeline through a merge request

Merge the source PR first. In `releng/konflux-release-data`, edit:

```text
tenants-config/cluster/stone-prd-rh01/tenants/hcm-eng-prod-tenant/hypershell/appstudio.redhat.com.releaseplan.yaml
```

Set `spec.finalPipeline` to this pipeline through the Git resolver. Use
`https://github.com/openshift-online/hypershell.git`, revision `main`, and
`pipelines/release-bundle/pipeline.yaml`. Set `useEmptyDir: true` and
`serviceAccountName: build-pipeline-hypershell-api-server-main`.

Each new run resolves the pipeline from `main`. The pipeline includes the
publisher code, so a change to `main` during the run cannot change that code.
The publisher does not need a resolved Git revision or PipelineRun provenance.
It fetches main to check source ancestry and the manifest files at the selected
Snapshot commit.

Bind this service account to the approved `konflux-viewer-bot-actions` ClusterRole
in the tenant namespace. The config repository rejects custom roles. The viewer
role permits reads of Releases, Snapshots, and other Konflux resources. It does
not grant writes. The publisher uses only `get` on Releases and Snapshots.
Verify these reads in the first cluster run.

The existing build account already has Quay write access. Reuse its credential
through Tekton credential initialization and the `select-oci-auth` helper. No new
Quay token or repository is required. Do not put a token in Git.

The first run verifies the actual namespace permissions and registry credential.
If either is missing, the run fails and publishes no bundle. The source PR alone
does not enable the pipeline.

## GitOps consumer

`hypershell-gitops` uses Renovate, Argo CD Source Hydrator, GitOps Promoter,
and Argo CD. Renovate discovers a bundle tag and digest, then runs
`bin/expand-hypershell-release` to record the full bundle in
`versions/hypershell-release-lock.json`, update the three workload image pins,
and update shared manifest references in one pull request against `main`.
The CLI is retained in the lock and source inventory; it is not a workload.

Source Hydrator renders `main` into each environment's proposed branch.
GitOps Promoter advances the active environment branches after their gates pass.
Argo CD on each owning cluster deploys its active branch. The GitHub release
created here is a download and release record; it does not trigger promotion.

See the GitOps repository's [Renovate delivery specification](https://github.com/openshift-online/hypershell-gitops/blob/main/specs/renovate-delivery.md)
and [Promoter delivery specification](https://github.com/openshift-online/hypershell-gitops/blob/main/specs/promoter-delivery.md).

## First cluster test

1. Merge the source PR and then the tenant config MR.
2. Let a main component build complete and pass the configured release checks.
3. Check the final PipelineRun. Its `bundle` result must contain an OCI digest.
4. Download that digest and check the six component digests and source revisions.
5. Verify Renovate expands the six-component bundle and the existing GitOps
   promotion checks accept it.

No cluster login is required to submit these changes. The first Konflux run is
still required to prove the live credentials. The local checks cannot prove them.

## Local checks

Run `make test-release-bundle` and `make check`.

Edit `scripts/release_bundle.py`, then run
`python3 scripts/render_release_bundle_pipeline.py` to update the pipeline's
embedded script. Commit both files. The tests reject a pipeline whose embedded
script differs from the source. They also run the complete embedded script with
local substitutes for Kubernetes, Git, and registry commands.

References:

- [Konflux tenant and final pipelines](https://konflux-ci.dev/docs/releasing/tenant-release-pipelines/)
- [Managed release pipeline](https://github.com/konflux-ci/release-service-catalog/blob/production/pipelines/managed/rh-push-to-external-registry/rh-push-to-external-registry.yaml)
- [Konflux Snapshots](https://konflux-ci.dev/docs/testing/integration/snapshots/)
- [Tekton credentials](https://tekton.dev/docs/pipelines/auth/)

## CLI and GitHub release

See [the release specification](../../specs/platform/hsctl-release.spec.md).
The CLI and both agent runtime images are required bundle components. Its released image contains all five
platform binaries under `/releases/`; the publisher extracts these by digest.
The default runtime container is Linux amd64. Downloads cover Linux and macOS
amd64/arm64 and Windows amd64. Download a binary, rename it to `hsctl` (or
`hsctl.exe`), and on Unix run `chmod +x hsctl` before putting it on PATH.

Each component now includes `tags`, containing its full Git SHA release tag.
The publisher verifies this tag against the Snapshot digest. Release notes list
these tags and digests; floating `latest` tags are deliberately omitted.

Every new OCI bundle gets a GitHub release in `openshift-online/hypershell`, using
the bundle tag as its Git tag and release name. The tag points to the selected
manifest commit. Assets are the five raw binaries, `bundle.json`, and
`SHA256SUMS`. Unchanged CLI builds can appear in multiple releases. The release
is created as a draft and published after every asset is verified. It does not
change GitHub's latest designation. A retry resumes the same draft or validates
an already completed release. Never run concurrent retries for the same Release.

Quay publication precedes GitHub publication. A GitHub failure leaves the bundle
available to Renovate and marks the final pipeline failed; retry that same Konflux
Release to complete GitHub publication. This is eventual 1:1 correspondence,
not an atomic transaction across the two services. A new Release UID intentionally
creates a new identity, even for the same Snapshot. Old bundles are not backfilled.

### Rollout order

1. Merge GitOps reader compatibility first: accept historical three- and four-component
   bundles and new six-component bundles, retaining the CLI and agent runtimes without deploying them.
2. Create and install a release GitHub App on `openshift-online/hypershell`
   with repository Contents read/write, permitted to create `release-bundle-*`
   tags. Have the approved secret delivery mechanism provision
   `hypershell-github-release` in `hcm-eng-prod-tenant` with `app-id`,
   `installation-id`, and `private-key.pem`. The publisher exchanges a signed App
   JWT for a fresh token restricted to this repository and Contents write on each
   run. The token stays in process memory; it is not a stored secret.
3. Add the CLI and agent runtime managed release mappings with the same destination and tag policy
   as the existing services.
4. Merge the application source changes and the tenant CLI and agent runtime Component/ImageRepository
   registrations. All initial builds must complete and be promoted before a
   six-component bundle can succeed. During this transition, incomplete Snapshots
   fail closed; existing deployed bundles remain usable.
5. Verify the first final PipelineRun, its Quay bundle, the corresponding GitHub
   release and seven assets. Check the attached `bundle.json` matches the OCI
   layer byte-for-byte and every download matches `SHA256SUMS`.
6. Ensure downstream bundle consumers tolerate six components and select the
   three deployable services by name. CLI and agent runtime inclusion does not create a workload.

This source change does not provision secrets or apply the external tenant and
managed configuration. Those configuration changes require their own merges.

### GitHub App provisioning

The App and tenant credential were provisioned, and live GitHub release publication
succeeded on 2026-10-09. The agent runtime extension reuses that secret contract.
No new GitHub credentials are required.

The [RelEng secret SOP](https://gitlab.cee.redhat.com/konflux/docs/sop/-/blob/main/releng/secrets.md#managing-secrets)
uses AppSRE Vault and ExternalSecret resources in `konflux-release-data` to
provision secrets into the managed `rhtap-releng-tenant` namespace. Our final
pipeline runs in `hcm-eng-prod-tenant`, so that SOP alone does not establish
access to the credential. Confirm the supported tenant secret delivery mechanism
with RelEng before enabling this publisher. The person who stores the App key in
Vault needs the appropriate Vault role and must record `generated-by`,
`generated-on`, `owner-team`, and `expires` alongside the credential.

The private key is mounted read-only at `/var/run/github-app/private-key.pem`.
App and installation IDs are supplied as `GITHUB_APP_ID` and
`GITHUB_INSTALLATION_ID`. A replacement private key can be delivered through the
same secret mechanism without changing the pipeline. GitHub tokens expire after
one hour; each retry obtains a new token and resumes the existing release.

For the agent runtime rollout and separate tenant/managed MRs, see
[`components/agents/README.md`](../../components/agents/README.md).
