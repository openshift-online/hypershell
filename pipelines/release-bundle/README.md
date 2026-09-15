# Release bundle

This pipeline publishes the three images from a successful Konflux managed
release as one OCI artifact. It uses the existing API server build repository:

```text
quay.io/redhat-user-workloads/hcm-eng-prod-tenant/hypershell-main/hypershell-api-server-main
```

The tags start with `release-bundle-`. The artifact contains `bundle.json` with
media type `application/vnd.hypershell.release.v1+json`. Each component has its
released image reference, with a SHA-256 digest, and its source Git revision.
The component images are in `quay.io/redhat-services-prod`.

The bundle is a release record. It is not a workload image or a Tekton task
bundle. Consumers must select the bundle tags and download the JSON layer.

## Release checks

The publisher requires all of these conditions:

- The Release uses `hypershell-releaseplan` in `hcm-eng-prod-tenant`.
- The managed pipeline has status `True` and reason `Succeeded`.
- The Snapshot and release artifacts contain exactly the three expected components.
- Each released digest matches the Snapshot and is available in the release repository.
- Each source revision belongs to the history of `hypershell` main.
- Any event-type metadata identifies a push event.

The final pipeline can run after a failed managed pipeline. It must check
`ManagedPipelineProcessed`; `Released` is not complete until the final pipeline
finishes. A failed check stops publication.

Konflux can keep an earlier image for an unchanged component. The bundle keeps
all three source revisions. A Snapshot can also contain an earlier image while
another component build is still running. This pipeline preserves the accepted
Snapshot; it does not add a test that waits for all builds from one commit.

The tag uses the Snapshot creation time and a hash of the Release UID. The OCI
creation time also uses the Snapshot time. A retry of an old release does not
receive a new creation time. Retries of the same Release produce the same content
and digest. Consumers must retain and use the bundle digest.

## Enable the pipeline through a merge request

Merge the source PR first. In `releng/konflux-release-data`, edit:

```text
tenants-config/cluster/stone-prd-rh01/tenants/hcm-eng-prod-tenant/hypershell/appstudio.redhat.com.releaseplan.yaml
```

Set `spec.finalPipeline` to this pipeline through the Git resolver. Use
`https://github.com/openshift-online/hypershell.git`, revision `main`, and
`pipelines/release-bundle/pipeline.yaml`. Set `useEmptyDir: true` and
`serviceAccountName: build-pipeline-hypershell-api-server-main`.

Each new run resolves the pipeline from `main`. The publisher reads the resolved
commit from `status.provenance.refSource.digest.sha1` on its PipelineRun and
fetches the Python script from that commit. A change to `main` during the run
cannot change the script version. Missing provenance stops publication.

Bind this service account to the approved `konflux-viewer-bot-actions` ClusterRole
in the tenant namespace. The config repository rejects custom roles. The viewer
role permits reads of Releases, Snapshots, and other Konflux resources. It does
not grant writes. The publisher uses only `get` on Releases, Snapshots, and its
PipelineRun. Verify these reads in the first cluster run.

The existing build account already has Quay write access. Reuse its credential
through Tekton credential initialization and the `select-oci-auth` helper. No new
Quay token or repository is required. Do not put a token in Git.

The first run verifies the actual namespace permissions and registry credential.
If either is missing, the run fails and publishes no bundle. The source PR alone
does not enable the pipeline.

## Kargo consumer

Use one image subscription for the bundle repository. For example:

```yaml
spec:
  subscriptions:
    - image:
        repoURL: quay.io/redhat-user-workloads/hcm-eng-prod-tenant/hypershell-main/hypershell-api-server-main
        imageSelectionStrategy: Lexical
        allowTagsRegexes:
          - '^release-bundle-[0-9]{8}T[0-9]{12}Z-[0-9a-f]{16}$'
```

A promotion must download the selected Freight digest with `oci-download` and
select media type `application/vnd.hypershell.release.v1+json`. It can then read
the component references from the JSON and commit the three image digest changes
to `hypershell-gitops`. Argo CD in the destination cluster pulls that commit.

Kargo selects the latest eligible tag at each poll. It does not guarantee a
separate deployment for every intermediate release. A later deployment step must
also prevent an older Snapshot from replacing a newer deployed image set.

## First cluster test

1. Merge the source PR and then the tenant config MR.
2. Let a main component build complete and pass the configured release checks.
3. Check the final PipelineRun. Its `bundle` result must contain an OCI digest.
4. Download that digest and check the three component digests and source revisions.
5. Configure Kargo to discover that artifact before enabling automatic promotion.

No cluster login is required to submit these changes. The first Konflux run is
still required to prove the live credentials. The local checks cannot prove them.

## Local checks

Run `make test-release-bundle` and `make check`.

References:

- [Konflux tenant and final pipelines](https://konflux-ci.dev/docs/releasing/tenant-release-pipelines/)
- [Konflux Snapshots](https://konflux-ci.dev/docs/testing/integration/snapshots/)
- [Tekton credentials](https://tekton.dev/docs/pipelines/auth/)
- [Kargo Warehouses](https://docs.kargo.io/user-guide/how-to-guides/working-with-warehouses)
- [Kargo OCI download](https://docs.kargo.io/user-guide/reference-docs/promotion-steps/oci-download)
