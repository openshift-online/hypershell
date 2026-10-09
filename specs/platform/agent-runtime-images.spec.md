# Agent runtime images

**Status:** Implemented in source; activation requires tenant and managed release configuration.
**Applies to:** `components/agents/*.docker`, `.tekton/hypershell-agent-runtime*-*.yaml`, `.hypershell/agents`, release bundle publisher and GitOps bundle consumers.

## ARI-01 -- Two runtime profiles

`agent-runtime-slim` SHALL provide shell utilities, Python, hsctl, GitHub CLI,
Atlassian CLI and native Claude Code. It SHALL NOT include Node.js/npm, Go or Rust
compilers. `agent-runtime` SHALL additionally include Go, Node.js/npm/pnpm, the Rust
compiler, Cargo, Clippy, rustfmt, standard library and source, plus GCC/G++, linker,
make, pkg-config, Python development headers and OpenSSL development headers.
Both SHALL support the existing agent scripts with a harness-provided checkout.
Neither image SHALL embed credentials, repository prompts, or a container daemon.

## ARI-02 -- Pinned inputs and staged builds

All base images SHALL be Red Hat Hardened Images pinned by SHA-256. Downloaded
tools and the complete OS package closure SHALL be versioned and SHA-256 locked.
Builds SHALL verify checksums, retain RPM signature checks, and install packages
without consulting live repositories. Lock regeneration SHALL be a separate,
reviewed maintenance operation. Unverified remote install scripts are prohibited.

The hsctl build SHALL reuse `components/cli/build-release.sh` and its pinned Go
compiler, with CGO disabled, trimpath and identical linker flags. Runtime builds
MAY select only their native Linux target; standalone CLI builds SHALL retain
all five download targets. Each runtime's hsctl corresponds to that image's own
source revision, which MAY differ from the standalone CLI in a later bundle.

Go module downloads SHALL precede source copies. Downloaded archives and build
caches SHALL stay outside final images. Shared stages SHALL allow build cache
reuse without requiring another runtime image to have been released first.
The initial Konflux images are Linux amd64; locks and Containerfiles also support
native arm64 workers. Multi-architecture indexes require separate worker setup.

## ARI-03 -- Runtime security and metadata

Images SHALL default to UID 10001, group 0, with group-writable application home
and workspace for arbitrary OpenShift UIDs. Other executables SHALL be root-owned
and non-writable by the runtime user. Images SHALL contain no setuid/setgid files
or system package manager. Automatic Claude Code updates SHALL be disabled.
Image definitions SHALL carry OCI title, description, source, documentation,
vendor, version, revision, license and base-image identity labels. Konflux SHALL
stamp source revision/version with the full build SHA. Bundled third-party
software retains its own licenses; the image-definition license is Apache-2.0.

The harness SHALL provide isolated per-run writable volumes, short-lived scoped
credentials, network controls, dropped capabilities, no privilege escalation and
RuntimeDefault seccomp. Image hardening alone SHALL NOT be represented as full
sandbox isolation or a guarantee of zero vulnerabilities.

## ARI-04 -- Build and delivery

Both Components SHALL use repository-root context and the corresponding
`components/agents/*.docker` file. Push, pull-request and merge-queue pipelines
SHALL include changes to agent image inputs and CLI source; merge-queue images
SHALL NOT auto-release. Existing Konflux scanning and signing tasks SHALL apply.

The managed mapping SHALL promote both components using the existing application
registry prefix and full Git SHA tags. New bundles SHALL require exactly the six
known release components, excluding the independently distributed fleet dashboard.
Incomplete snapshots SHALL fail before writes and name missing components.
GitHub releases SHALL retain one-to-one bundle identities, list all six image
references, and attach the existing five standalone CLI binaries plus bundle.json
and SHA256SUMS. Agent images SHALL NOT create additional GitHub binary assets.

GitOps consumers SHALL accept historical three/four-component bundles and the
new runtime components, preserve complete inventory, and update only the three
service deployments. Consumer compatibility SHALL precede publisher activation.
The tenant and managed release-data changes SHALL remain separate MRs. The first
complete six-component Snapshot is required for successful new publication.
