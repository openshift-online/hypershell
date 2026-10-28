# Konflux Enterprise Contract Conformance Specification

**ID prefix:** CICD-ECC

## Purpose

HyperShell builds every shipped container image in Konflux, and Konflux validates
each application Snapshot with the `enterprise-contract` (Conforma) policy. A
Snapshot validates one component's new image together with the latest image of
every other component in the `hypershell-main` application, so a single
non-conformant image fails `enterprise-contract` for the whole application. This
spec defines the desired state in which every Konflux component image is
Conforma-conformant, so `enterprise-contract` passes consistently on `main`.

Conformance has four standing properties for each component image: its build
attributes every dependency (resolving each from-source ecosystem through Hermeto
where the component builds from one, and verifying checksum-pinned downloads by
their published digest where it does not) so the SBOM is complete; its build
produces a valid, trusted SLSA provenance attestation; it ships no
non-reproducible (`+dirty`) or otherwise unattributed baked binary; and its
Konflux pipeline definition has a single source of truth in `.tekton/`. The
platform also keeps base images current so images do not accumulate unapplied
base-image fixes.

The Konflux components in scope are the eight that build images in this
repository. Each has three pipeline files,
`.tekton/hypershell-<component>-main-{push,pull-request,merge-queue}.yaml`:

| Component | Image definition | From-source ecosystems |
|---|---|---|
| `api-server` | `components/api-server/Dockerfile` | Go (gomod, `components/api-server`) |
| `control-plane` | `components/control-plane/Dockerfile` | Go (gomod, `components/control-plane`) |
| `cli` | `components/cli/Dockerfile` | Go (gomod, `components/cli`) |
| `web-console` | `components/web-console/Dockerfile` | Node (pnpm, shared root workspace lockfile) |
| `fleet-dashboard` | `components/fleet-dashboard/Dockerfile` | Go (gomod, `components/fleet-dashboard`) + Node (pnpm, shared root workspace lockfile) |
| `agent-runtime` | `components/agents/Containerfile.agent-runtime` | Go (gomod, `hsctl` from `components/cli`); tool closure SHA-256-locked |
| `agent-runtime-slim` | `components/agents/Containerfile.agent-runtime-slim` | Go (gomod, `hsctl` from `components/cli`); tool closure SHA-256-locked |
| `fullsend-sandbox` | `components/agents/Containerfile.fullsend-sandbox` | None from source (self-contained UBI9 image; checksum-pinned downloads) |

`fullsend-sandbox` is a HyperShell-owned, self-contained image built on Red Hat
UBI9 (`registry.access.redhat.com/ubi9/ubi`, digest-pinned). It builds no
repository module from source: it assembles its toolchain and the `tirith`
security-hook binary from checksum-verified downloads. It therefore has no
Hermeto-resolvable from-source ecosystem, and its dependency attribution and
hermetic migration are addressed by Requirement CICD-ECC-02's checksum-pinned
path rather than by a `gomod`/`npm`/`pip` prefetch.

The two agent runtime images build one repository module from source through
Hermeto, the `hsctl` CLI from `components/cli` (`gomod`), but they do not resolve
Python or Node dependencies from a repository `pip` or `npm` manifest. Their tool
closures are versioned, SHA-256-locked downloads assembled by
`components/agents/install-tools.py`, consistent with
[`agent-runtime-images.spec.md`](./agent-runtime-images.spec.md), which already
requires downloaded tools to be SHA-256-locked and checksum-verified. Those
downloads are attributed through Requirement CICD-ECC-02's checksum-pinned path,
the same way `fullsend-sandbox` attributes its downloads.

The Konflux build particulars of the two agent runtime images (staged builds,
native multi-arch, Trusted Artifacts from clone and prefetch) are specified in
[`agent-runtime-images.spec.md`](./agent-runtime-images.spec.md) (`ARI`); this
spec adds the Conforma conformance invariant those images SHALL also satisfy,
and does not restate their build layout. Mintmaker and Renovate base-image and
dependency bumps are the update mechanism referenced by Requirement
CICD-ECC-06; OpenShell gateway image bumps are a separate concern owned by
[`openshell-image-auto-update.spec.md`](./openshell-image-auto-update.spec.md).

## Requirements

### Requirement: CICD-ECC-01 -- Enterprise Contract Conformance Across All Components

**Verification:** Demonstration -- the `enterprise-contract` check result on the
`hypershell-main` application is the evidence.

Every Konflux component image in the `hypershell-main` application SHALL pass the
`enterprise-contract` (Conforma) check on `main`. Because a Snapshot validates one
component's image together with the latest image of every other component, the
platform SHALL treat `enterprise-contract` conformance as an application-wide
invariant that holds for all eight in-scope components simultaneously, not as a
per-component property that may regress one image at a time.

A change that introduces a new Konflux component, or that materially changes an
existing component's build inputs, SHALL bring that component to conformance
(Requirements CICD-ECC-02 through CICD-ECC-05) before it is treated as shipped,
so the application-wide check does not regress.

#### Scenario: CICD-ECC-01.01 -- Enterprise contract passes for every component

- GIVEN the `hypershell-main` application with all eight in-scope components built on `main`
- WHEN Konflux validates any component's Snapshot
- THEN the `enterprise-contract` check SHALL pass
- AND the result SHALL NOT depend on which component triggered the Snapshot

#### Scenario: CICD-ECC-01.02 -- A single non-conformant image fails the whole check

- GIVEN one component image on `main` that violates a Conforma rule
- WHEN any component's Snapshot is validated
- THEN `enterprise-contract` SHALL fail for that Snapshot
- AND the failure SHALL be attributed to the specific non-conformant image and rule codes in the `verify` step output

#### Scenario: CICD-ECC-01.03 -- A new component is conformant before it ships

- GIVEN a change that adds a new Konflux component to the `hypershell-main` application
- WHEN that component's pipelines are introduced
- THEN the component SHALL satisfy Requirements CICD-ECC-02 through CICD-ECC-05 before it is treated as shipped
- AND the application-wide `enterprise-contract` check SHALL NOT regress

### Requirement: CICD-ECC-02 -- Hermeto-Prefetched Hermetic Builds With Full SBOM Attribution

**Verification:** Inspection -- the `.tekton` pipeline parameters and the resulting
image SBOM are reviewed against the component's from-source ecosystems.

Each component image SHALL attribute every dependency in its SBOM. For every
Hermeto-resolvable ecosystem a component builds from source (per the Purpose
table: every component that builds Go from source has a `gomod` ecosystem,
`web-console` is the Node-only exception with a pnpm ecosystem, and
`fullsend-sandbox` has no from-source ecosystem),
the build SHALL resolve those dependencies through Hermeto. Every such component
SHALL set, in all three pipeline files:

- `enable-package-registry-proxy: "true"` -- dependency prefetch uses the package
  registry proxy. This param is read by Conforma only on a results-producing
  (Trusted Artifacts, `-oci-ta`) prefetch task, so the component's prefetch SHALL
  run on that task variant.
- `prefetch-input` -- a specification that covers every ecosystem that component
  builds from source (per the Purpose table), with one entry per module whose
  `path` points at the directory containing that module's manifest or lockfile in
  this monorepo. A single-ecosystem component MAY use one object; a
  multi-ecosystem component SHALL use one object per ecosystem.

A component whose entire build has no remaining input that Hermeto cannot prefetch
(for example the Go-only components, and the Node components once their pnpm
bootstrap is prefetched) SHALL additionally set `hermetic: "true"` in all three
pipeline files, so the build runs with network isolation and may use only
prefetched inputs. A component that still pulls an input Hermeto cannot prefetch
SHALL keep `hermetic: "false"` and attribute that input by its SHA-256 digest (see
below), while still declaring the proxy and `prefetch-input` for its
Hermeto-resolvable ecosystems.

`prefetch-input` paths SHALL address the real module location for each ecosystem
in this monorepo. A `gomod` entry SHALL point at that component's Go module
directory (for example `components/fleet-dashboard`) and SHALL NOT assume the
repository root. This repository builds every Node component from a single shared
pnpm workspace lockfile at the repository root (`pnpm-lock.yaml` with
`sharedWorkspaceLockfile: true`); an `npm`/pnpm entry SHALL therefore resolve that
root workspace lockfile rather than assume a per-package lockfile, and SHALL use
the Hermeto handling that resolves pnpm (not an npm/yarn assumption) so Node
dependencies are attributed.

Enabling `hermetic: "true"` SHALL be a complete migration, not a flag flip: any
ad-hoc network fetch in that component's Dockerfile or Containerfile stages SHALL
be moved into Hermeto prefetch so the hermetic build still succeeds. The image
SBOM SHALL list the prefetched dependencies for every from-source ecosystem of
that component. A hermetic flag SHALL be flipped for a component only after that
component's hermetic build is confirmed green (traced from the Konflux build log
or `ec validate`); an unverified flip that breaks the component build is not
permitted, because a broken or stale image makes the application-wide check
worse, not better.

Converting a component to the Trusted Artifacts (`-oci-ta`) task variants that the
proxy-attributed prefetch above requires also builds that component as a native
multi-architecture image index. A Go component whose Dockerfile already carries
per-architecture base images and a `BUILDARCH`/`TARGETARCH` toolchain split (for
example `components/api-server/Dockerfile`, which documents "Bump amd64 and arm64
together") SHALL build both `linux/amd64` and `linux/arm64` natively and publish
their combined image index (`ALWAYS_BUILD_INDEX: "true"`), matching the native
multi-arch layout the agent runtime image trio already uses
([`agent-runtime-images.spec.md`](./agent-runtime-images.spec.md)). The second
architecture is an intended standardization on that shared layout, recorded here
so the change is traceable; it is not a side effect of copying a template.
Conforma conformance (Requirement CICD-ECC-01) does not by itself require the
second architecture, so a component whose Dockerfile does not yet carry
per-architecture base images SHALL stay single-architecture until it does, rather
than add an architecture it cannot build natively.

Some components also pull inputs that Hermeto cannot prefetch. `fullsend-sandbox`
has no Hermeto-resolvable from-source ecosystem at all: it assembles its toolchain
and the `tirith` binary from checksum-pinned downloads. The `agent-runtime` and
`agent-runtime-slim` images resolve their `gomod` ecosystem (the `hsctl` build)
through Hermeto, but assemble their Python and Node tool closures from
SHA-256-locked downloads through `components/agents/install-tools.py`. For every
such input the build SHALL verify it against its published SHA-256 digest so no
input is unattributed or non-reproducible, and the image SHALL still satisfy
Requirements CICD-ECC-03 through CICD-ECC-05. A component whose build still fetches
any such input over the network SHALL NOT set `hermetic: "true"`, because network
isolation would break that fetch; vendoring those downloads into prefetch so the
component can build hermetically is a deferred goal of this specification. This
applies to `fullsend-sandbox` and to the agent runtime tool closures until their
downloads are vendored.

#### Scenario: CICD-ECC-02.01 -- Single-ecosystem Go component prefetches gomod

- GIVEN a Go-only component that builds from repository source (for example `api-server`, `control-plane`, or `cli`)
- WHEN its pipeline files are configured
- THEN `hermetic` SHALL be `"true"` and `enable-package-registry-proxy` SHALL be `"true"`
- AND `prefetch-input` SHALL declare a `gomod` entry whose `path` is that component's module directory
- AND the built image SBOM SHALL attribute the component's Go dependencies

#### Scenario: CICD-ECC-02.02 -- Multi-ecosystem component prefetches every ecosystem

- GIVEN a component that builds from more than one Hermeto-resolvable ecosystem (for example `fleet-dashboard` with a `gomod` module and a pnpm workspace)
- WHEN its pipeline files are configured
- THEN `prefetch-input` SHALL declare one entry per from-source ecosystem, the `gomod` entry pointing at that module's directory and the `npm`/pnpm entry resolving the shared root workspace lockfile
- AND the built image SBOM SHALL attribute dependencies for every one of those ecosystems

#### Scenario: CICD-ECC-02.03 -- pnpm dependencies are attributed

- GIVEN a Node component built from the repository's shared pnpm workspace lockfile (for example `web-console`)
- WHEN its Node dependencies are prefetched
- THEN the prefetch configuration SHALL resolve the root workspace `pnpm-lock.yaml` with pnpm handling
- AND the built image SBOM SHALL attribute the Node dependencies

#### Scenario: CICD-ECC-02.04 -- Hermetic build has no ad-hoc network fetch

- GIVEN a component whose Dockerfile previously fetched an input over the network during build
- WHEN `hermetic` is set to `"true"`
- THEN that input SHALL be supplied through Hermeto prefetch
- AND the hermetic build SHALL succeed without a live network fetch

#### Scenario: CICD-ECC-02.05 -- Checksum-pinned component attributes its downloads

- GIVEN a component with no Hermeto-resolvable from-source ecosystem (for example `fullsend-sandbox`)
- WHEN its image is built from checksum-pinned downloaded toolchains and binaries
- THEN every downloaded input SHALL be verified against its published SHA-256 digest
- AND no input SHALL appear in the SBOM as unattributed or non-reproducible
- AND the component SHALL NOT set `hermetic: "true"` until those downloads are vendored for a hermetic build

#### Scenario: CICD-ECC-02.06 -- Agent runtime attributes gomod through Hermeto and tools by digest

- GIVEN `agent-runtime` or `agent-runtime-slim`, which builds `hsctl` from `components/cli` and assembles its Python and Node tool closure from SHA-256-locked downloads
- WHEN its image is built
- THEN the `gomod` ecosystem SHALL be attributed through Hermeto with a `prefetch-input` entry pointing at `components/cli`
- AND every downloaded tool SHALL be verified against its published SHA-256 digest so no tool is unattributed or non-reproducible
- AND the component SHALL NOT set `hermetic: "true"` until those tool downloads are vendored for a hermetic build

#### Scenario: CICD-ECC-02.07 -- Trusted Artifacts Go component builds a native multi-arch index

- GIVEN a Go component converted to the Trusted Artifacts task variants whose Dockerfile already carries per-architecture base images and a `BUILDARCH`/`TARGETARCH` split (for example `api-server`)
- WHEN its image is built
- THEN the pipeline SHALL build `linux/amd64` and `linux/arm64` natively and publish their combined image index with `ALWAYS_BUILD_INDEX: "true"`
- AND the multi-architecture index SHALL be the intended, recorded build layout, consistent with the agent runtime image trio, not an unrecorded side effect
- AND a component whose Dockerfile lacks per-architecture base images SHALL remain single-architecture until it adds them

### Requirement: CICD-ECC-03 -- Valid, Trusted Build Attestation

**Verification:** Inspection -- the Conforma `verify` step output and the
pipeline's standard task set are reviewed.

Each component image SHALL carry a valid SLSA provenance attestation that Conforma
trusts: a trusted builder identity, the required task set, base images drawn only
from allowed registries, git-clone provenance, and a present, trusted PipelineRun
attestation. All component pipelines SHALL carry the identical standard Konflux
task set; the platform SHALL NOT add tasks to a build pipeline to satisfy
attestation rules, because adding a task to the pipeline itself causes Conforma to
flag a `trusted_task.trusted` violation.

When a provenance or attestation rule fails, the platform SHALL start from the
exact rule codes in the `verify` step output and SHALL confirm the attestation is
present and trusted before changing task definitions, so a resolvable
attestation-trust problem is not misdiagnosed as a missing task.

#### Scenario: CICD-ECC-03.01 -- Attestation is present and trusted

- GIVEN a built component image on `main`
- WHEN Conforma validates its provenance
- THEN a PipelineRun attestation SHALL be present and trusted
- AND the builder identity, required tasks, allowed base-image registries, and git-clone provenance rules SHALL pass

#### Scenario: CICD-ECC-03.02 -- Attestation failures are diagnosed before changing tasks

- GIVEN a `verify` step reporting a provenance or attestation rule failure
- WHEN the failure is investigated
- THEN the exact rule codes SHALL be the starting point
- AND the attestation SHALL be confirmed present and trusted before any task definition is changed

#### Scenario: CICD-ECC-03.03 -- No extra task is added to the build pipeline

- GIVEN a standard Konflux build pipeline for a component
- WHEN attestation conformance is addressed
- THEN no task SHALL be added to the pipeline beyond the standard task set
- AND Conforma SHALL NOT report a `trusted_task.trusted` violation caused by an added task

### Requirement: CICD-ECC-04 -- No Non-Reproducible Baked Binaries

**Verification:** Inspection -- shipped-image SBOMs are reviewed for
non-reproducible build markers.

No shipped component image SHALL contain a non-reproducible binary, including any
binary that appears in the SBOM as a `+dirty` build (built from an uncommitted
tree). Every binary baked into a shipped image SHALL be built from a clean,
committed revision and SHALL have its dependencies attributed through Hermeto
(Requirement CICD-ECC-02).

When a baked binary's ingestion path is not evident from any `go.mod`,
`Dockerfile`, or `Containerfile` on `main`, the platform SHALL trace it from the
image build log (for example a base image or a copy-from step) and SHALL resolve
it either by building it from a tagged, clean revision or by not shipping it in
the product image.

#### Scenario: CICD-ECC-04.01 -- No dirty binary in a shipped image

- GIVEN a shipped component image
- WHEN its SBOM is inspected
- THEN no binary SHALL be marked as a `+dirty` or otherwise non-reproducible build

#### Scenario: CICD-ECC-04.02 -- An unattributed baked binary is traced and resolved

- GIVEN a binary baked into an image that no `go.mod`, `Dockerfile`, or `Containerfile` on `main` references
- WHEN its ingestion path is investigated
- THEN the path SHALL be traced from the image build log
- AND the binary SHALL either be rebuilt from a tagged, clean revision with Hermeto-attributed dependencies or be removed from the shipped image

### Requirement: CICD-ECC-05 -- Single Pipeline Source of Truth Per Component

**Verification:** Inspection -- the `.tekton` directory is reviewed for a single
authoritative set of pipeline files per component.

Each Konflux component SHALL have exactly one authoritative set of
`.tekton/hypershell-<component>-main-{push,pull-request,merge-queue}.yaml` files
in the repository. A redundant Konflux-generated pipeline pull request that
duplicates a component already defined on `main` SHALL be closed or reconciled
into the single authoritative set, so a component never has two competing sources
of truth for its pipeline definition.

#### Scenario: CICD-ECC-05.01 -- Each component has one pipeline source

- GIVEN a Konflux component that is first-class on `main` (for example `fullsend-sandbox`, merged in #764)
- WHEN its pipeline files are reviewed
- THEN exactly one authoritative set of push, pull-request, and merge-queue pipeline files SHALL exist for it

#### Scenario: CICD-ECC-05.02 -- Redundant generated pipeline PR is closed or reconciled

- GIVEN a Konflux-generated pipeline pull request duplicating a component already defined on `main`
- WHEN the duplication is resolved
- THEN the redundant pull request SHALL be closed or reconciled into the authoritative files
- AND the component SHALL retain a single source of truth for its `.tekton` files

### Requirement: CICD-ECC-06 -- Base Images Kept Current

**Verification:** Inspection -- base-image references and the state of automated
base-image bump pull requests are reviewed.

The platform SHALL keep the Red Hat base images referenced by component image
definitions current by merging the automated Mintmaker base-image bump pull
requests promptly after their required checks pass, so component images do not
accumulate unapplied base-image fixes that would make them non-conformant over
time. This applies to every Red Hat base registry the components use, including
the `registry.access.redhat.com/hi/*` Hardened Images used by most components and
the `registry.access.redhat.com/ubi9/*` image used by `fullsend-sandbox`. Base
images SHALL be drawn only from allowed Red Hat registries (Requirement
CICD-ECC-03) and SHALL be digest-pinned.

#### Scenario: CICD-ECC-06.01 -- Mintmaker base-image bump is merged promptly

- GIVEN an automated Mintmaker pull request bumping a Red Hat base image (for example `registry.access.redhat.com/hi/*` or `registry.access.redhat.com/ubi9/*`)
- WHEN its required checks pass
- THEN the bump SHALL be merged promptly
- AND the component images SHALL NOT accumulate unapplied base-image fixes
