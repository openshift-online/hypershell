# Agent runtime images

**Status:** Implemented
**Applies to:** `components/agents/Containerfile.*`, `components/agents/install-tools.py`, `components/agents/locks/*`, `.tekton/hypershell-agent-runtime*-*.yaml`, `.hypershell/agents`, `.hypershell/base/platform/sandbox-template.yaml`, the api-server generation and integration-test targets reached by the `agent-runtime` sandbox, release bundle publisher and GitOps bundle consumers.

## ARI-01 -- Two runtime profiles

`agent-runtime-slim` SHALL provide shell utilities, Python, hsctl, GitHub CLI,
Atlassian CLI and native Claude Code. It SHALL NOT include Node.js/npm, Go or Rust
compilers. `agent-runtime` SHALL additionally include Go, Node.js/npm/pnpm, the Rust
compiler, Cargo, Clippy, rustfmt, standard library and source, plus GCC/G++, linker,
make, pkg-config, Python development headers and OpenSSL development headers.

`agent-runtime` SHALL also provide the container-free generation and verification
toolchain the `code-implementation` agent needs, so a feature that adds or changes
an API kind, a web-console page, or integration-tested behavior can be generated,
built, and tested inside the sandbox:

| Tool | Needed for | Notes |
|---|---|---|
| `buf` at the CI-pinned version | `make proto` (`buf generate` with remote plugins) | Locked binary. |
| Headless Java runtime and the `openapi-generator-cli` 7.16.0 JAR | `make generate` without a container engine | Same version and arguments as `components/api-server/Dockerfile.openapi`. |
| PostgreSQL server binaries (`initdb`, `pg_ctl`, `postgres`) at the production major version | api-server integration tests with `DB_FACTORY_MODE=external` | Run as the agent user on a unix socket or localhost port. |
| Headless Chromium and its runtime libraries, matching the repository's Playwright version | `skills/build/ui-build-gate` screenshot capture and web-console Playwright flows | Baked into the image; no browser download at run time. |

These additions SHALL NOT introduce a container daemon, a container runtime, a
Kubernetes runtime, or a browser download at agent run time; `agent-runtime`
SHALL remain daemon-free.

Both SHALL support the existing agent scripts with a harness-provided checkout.
Neither image SHALL embed credentials, repository prompts, or a container daemon.

## ARI-02 -- Pinned inputs and staged builds

All base images SHALL be Red Hat Hardened Images pinned by SHA-256. Downloaded
tools and the complete OS package closure SHALL be versioned and SHA-256 locked.
Builds SHALL verify checksums, retain RPM signature checks, and install packages
without consulting live repositories. Lock regeneration SHALL be a separate,
reviewed maintenance operation. Unverified remote install scripts are prohibited.

The `agent-runtime` generation and verification tools -- `buf`, the Java runtime,
the `openapi-generator-cli` JAR, the PostgreSQL server binaries, and the headless
Chromium bundle -- SHALL be pinned and SHA-256 locked through
`components/agents/install-tools.py` and `components/agents/locks/full-*.json`, or
installed from the locked OS package closure. None SHALL be downloaded from an
unpinned source at build time, and none SHALL be downloaded at agent run time. The
`openapi-generator-cli` version SHALL match `components/api-server/Dockerfile.openapi`,
and `buf` and the headless Chromium SHALL match the versions CI and the repository's
Playwright configuration use, so locally generated output and captures match CI.

The hsctl build SHALL reuse `components/cli/build-release.sh` and its pinned Go
compiler, with CGO disabled, trimpath and identical linker flags. Runtime builds
MAY select only their native Linux target; standalone CLI builds SHALL retain
all five download targets. Each runtime's hsctl corresponds to that image's own
source revision, which MAY differ from the standalone CLI in a later bundle.

Go module downloads SHALL precede source copies. Downloaded archives and build
caches SHALL stay outside final images. Shared stages SHALL allow build cache
reuse without requiring another runtime image to have been released first.
Konflux SHALL build Linux amd64 and arm64 on native workers using matching
`TARGETARCH` build arguments and distinct architecture-suffixed staging tags.
The normal component tag SHALL reference an image index containing both builds;
index publication SHALL require both builds to succeed. Release promotion and
`bundle.json` SHALL preserve the index digest. The Konflux instance is assumed to
provide both worker platforms. Clone and prefetch SHALL publish immutable Trusted
Artifacts consumed by both native builds in isolated task storage. The architecture
builds SHALL depend on prefetch rather than each other, allowing concurrent builds
without shared writable source or SBOM files. Architecture-aware scans SHALL cover
both platforms. Source scans and optional source-image builds SHALL consume the
same prefetched artifacts. No shared source PVC SHALL be required.

## ARI-03 -- Runtime security and metadata

Images SHALL default to UID 10001, group 0, with group-writable application home
and workspace for arbitrary OpenShift UIDs. Other executables SHALL be root-owned
and non-writable by the runtime user. Images SHALL contain no setuid/setgid files
or system package manager. Automatic Claude Code updates SHALL be disabled.
Image definitions SHALL carry OCI title, description, source, documentation,
vendor, version, revision, license and base-image identity labels. Konflux SHALL
stamp source revision/version with the full build SHA. Final images SHALL also
carry the Red Hat release-policy labels `com.redhat.component` (the runtime's
component name), `distribution-scope=public`, `url` (the project URL), and
`vendor=Red Hat, Inc.`; the OCI vendor label SHALL match. These labels SHALL be
set explicitly because the final scratch stage does not inherit base labels.
Bundled third-party
software retains its own licenses; the image-definition license is Apache-2.0.

The harness SHALL provide isolated per-run writable volumes, short-lived scoped
credentials, network controls, dropped capabilities, no privilege escalation and
RuntimeDefault seccomp. Image hardening alone SHALL NOT be represented as full
sandbox isolation or a guarantee of zero vulnerabilities.

## ARI-04 -- Build and delivery

Both Components SHALL use repository-root context and the corresponding
`components/agents/Containerfile.*` file. Push, pull-request and merge-queue pipelines
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
service deployments.

## ARI-05 -- code-implementation sandbox template, egress, and sizing

The `code-implementation` agent SHALL run in a sandbox template named
`agent-runtime` that uses the full `hypershell-agent-runtime` image, pinned by
digest and updated by the same release-bundle flow that pins the
`agent-runtime-slim` template. `.hypershell/agents/code-implementation/agent.yaml`
SHALL set `sandbox_template_id: agent-runtime`. Every other agent in
`.hypershell/agents` SHALL continue to use `agent-runtime-slim`. The
`SandboxTemplate` and `SandboxPolicy` schema remains owned by
[`data-model.spec.md`](./data-model.spec.md) (`PLAT-DATA-20`); this requirement
fixes the concrete `agent-runtime` template declared under `.hypershell`.

The `agent-runtime` template's `SandboxPolicy` network section SHALL keep the
existing inference (`inference.local:443`, binary `claude`) and GitHub
(`api.github.com:443`, `github.com:443`, binaries `gh`, `git`) egress groups and
SHALL add the following groups, each limited to the single set of binaries that
needs it:

| Group | Hosts | Binaries |
|---|---|---|
| Go modules | `proxy.golang.org:443`, `sum.golang.org:443` | `go` |
| npm packages | `registry.npmjs.org:443` | `node`, `pnpm` |
| buf remote plugins | `buf.build:443`, `api.buf.build:443` | `buf` |

The template SHALL NOT permit general internet egress; a request to any host
outside the declared groups SHALL fail. Where an internal Red Hat proxy for Go
modules or npm packages is available, the template SHOULD prefer it, and the hosts
it uses SHALL be recorded in the template. The repository's `minimumReleaseAge` and
dependency-age checks SHALL still apply to anything fetched through these groups.

The template SHALL declare CPU and memory requests and limits sized for Go builds,
`pnpm install`, Storybook builds, and a local PostgreSQL, starting at 4 CPU and
8Gi and tuned from observed metrics. It SHALL provide writable space for module and
package caches under `/home/agent` or `/tmp`, with cache locations set by
environment (`GOMODCACHE`, `GOCACHE`, and the pnpm store directory).

## ARI-06 -- Container-free generation and external-database tests

So the `code-implementation` agent can generate code and run tests without a
container engine, the repository SHALL offer container-free paths that match the
container paths they stand in for:

- `make generate` in `components/api-server` SHALL support a container-free OpenAPI
  generator selection (for example `OPENAPI_GENERATOR=jar`) that runs the same
  generator version with the same arguments and post-processing as
  `components/api-server/Dockerfile.openapi` (delete `go.mod`, `go.sum`, and
  `test/`; `gofmt -w`). A CI job SHALL assert that the container path and the JAR
  path produce byte-identical output.
- The api-server SHALL offer an external-database integration target (for example
  `make test-integration-external-db`) that starts a temporary PostgreSQL with
  `initdb` and `pg_ctl` in a temporary directory, runs the integration tests with
  `DB_FACTORY_MODE=external` and matching database settings, and stops and removes
  the database on every exit.
- The default developer path with a container engine SHALL be unchanged: `make
  generate` and `make test` behave as they do today.

The `code-implementation` agent SHALL use these container-free paths and SHALL
report a missing tool as a blocker only after the documented path also fails.

## ARI-07 -- Sandboxed-agent e2e gate

The `code-implementation` agent's sandbox cannot run Kind or any container-based
end-to-end suite. The agent SHALL NOT run e2e locally and SHALL NOT report e2e as
passing from a run it did not perform. It SHALL push its branch and treat the pull
request's CI e2e jobs as the e2e gate, per
[`e2e-testing.spec.md`](./e2e-testing.spec.md) (`E2E-CORE-51`).
