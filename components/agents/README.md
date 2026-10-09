# Agent runtime images

Build context: repository root. Containerfiles: `agent-runtime.docker` and
`agent-runtime-slim.docker`. Contract: [agent-runtime-images.spec.md](../../specs/platform/agent-runtime-images.spec.md).

| Contents | agent-runtime-slim | agent-runtime |
| --- | --- | --- |
| bash, coreutils, git, curl, jq, ripgrep, tar/gzip/unzip, find/diff/patch/which, CA roots | yes | yes |
| Python, hsctl, native Claude Code, Atlassian CLI (`acli`), GitHub CLI (`gh`) | yes | yes |
| Go, Rust compiler/Cargo/Clippy/rustfmt/standard library/source | | yes |
| Node.js 24, npm and pnpm 11.15.1 | | yes |
| GCC/G++, linker, make, pkg-config, OpenSSL and Python development headers | | yes |

Rust comes from Red Hat's signed RPM toolchain, including its standard library
and source; rustup and automatic toolchain downloads are not installed. Claude
Code uses its native executable; slim contains no Node.js/npm or compilers.
There is no container engine, Kubernetes CLI, browser, or database server.
Release-verification agents needing cluster checks must receive the appropriate
cluster tooling and credentials from their harness or a dedicated future variant.

## Builds and cache

```sh
podman build --layers -f components/agents/agent-runtime-slim.docker -t localhost/agent-runtime-slim .
podman build --layers -f components/agents/agent-runtime.docker -t localhost/agent-runtime .
```

The initial Konflux pipelines use the existing native Linux amd64 builder. The
Containerfiles and dependency locks also support native arm64 builds: on an arm64
builder, pass `--build-arg TARGETARCH=arm64`. Do not use that argument to claim a
cross-built image on an amd64 worker. A multi-architecture image index requires
separate native workers and the Konflux multi-platform configuration.

Build stages isolate tool downloads, RPM installation, and CLI compilation.
The final stage copies the complete assembled Hardened Python root filesystem
into scratch, avoiding duplicate base layers and inherited upstream image labels.
The two variants share identical tool and hsctl stages, enabling local/shared
builder cache reuse. Konflux cache availability depends on its workers; no
cross-PipelineRun cache is assumed. Go modules are downloaded before source
copies. The shared CLI release script builds only the native Linux target here,
with the same CGO/trimpath/linker flags as the standalone CLI release image.
Agent definitions are loaded from the harness checkout, avoiding rebuilds when
only prompts or orchestration change.

## Pins and updates

Every external FROM is pinned to a SHA-256 digest. Runtime bases are Red Hat
Hardened Python; hsctl and the full image's Go toolchain use the same Hardened Go
pins as `components/cli/Dockerfile`. Keep those Go pins synchronized.

`locks/{slim,full}-{amd64,arm64}.json` records the entire RPM dependency closure,
including URLs and SHA-256 checksums. The assembler downloads and verifies every
RPM, then installs offline with RPM signature checking and weak dependencies
disabled. Builds never resolve current repository metadata. RPM inventories
remain available for Konflux scans; dnf is confined to the assembler stage.
`update-rpm-locks.py` regenerates the RPM locks for review using the pinned
assembler. Review and commit the resulting changes; regeneration is explicit.

`locks/full-tools-*.json` pins the pnpm npm tarball (verified against npm integrity).
`locks/tools-*.json` pins native Claude Code, gh, acli, and ripgrep archives or
executables. Checksums come from vendor release manifests: [Claude installer](https://claude.ai/install.sh),
[GitHub CLI releases](https://github.com/cli/cli/releases),
[Atlassian's formula](https://github.com/atlassian/homebrew-acli/blob/main/Formula/acli.rb),
and [ripgrep releases](https://github.com/BurntSushi/ripgrep/releases).
When updating, review both architectures and preserve archive member paths.
No `curl | sh`, `latest` downloads, npm global installs, or unverified binaries
are used by the build. Automatic Claude updates are disabled. Renovate discovers
both `.docker` files; RPM/tool lock updates require explicit regeneration/review.
Pinned packages require regular rebuilds and vulnerability review; pinning alone
does not ensure absence of vulnerabilities. Third-party programs retain their
upstream licenses; the Apache-2.0 label describes HyperShell's image definition.

## Runtime contract

Default UID is 10001, group 0. `/home/agent` and `/sandbox` are group-writable for
OpenShift arbitrary UIDs. For a read-only root filesystem, provide writable
volumes at `/home/agent`, `/sandbox`, and `/tmp` with appropriate ownership/fsGroup.
Run with `allowPrivilegeEscalation: false`, dropped capabilities, and
`seccompProfile: RuntimeDefault`. No privileged daemon, sudo, setuid executable,
credential, repository checkout, or preauthorized CLI session is included.
The harness supplies short-lived scoped credentials and network restrictions.
Use a separate sandbox per agent run, because agents execute repository code.

`GIT_VERSION` and `GIT_REVISION` build arguments are set to the source SHA by
Konflux for OCI and descriptive image labels. No ports are exposed. The default
command is bash; the harness executes the chosen `.hypershell/agents/*/run.sh`.

## Release integration and rollout

Components are `hypershell-agent-runtime-main` and
`hypershell-agent-runtime-slim-main`. Managed promotion destinations follow the
existing application's mapping:

- `quay.io/redhat-services-prod/hcm-eng-prod-tenant/hypershell-main/hypershell-agent-runtime-main`
- `quay.io/redhat-services-prod/hcm-eng-prod-tenant/hypershell-main/hypershell-agent-runtime-slim-main`

Both images are required in new six-component bundles and listed in matching
GitHub release notes with commit tags and digests. GitHub binary assets continue
to come only from `hypershell-cli-main`. Each runtime records its own source SHA;
unchanged runtime builds can be reused across bundles.

Rollout order:

1. Merge the GitOps reader compatibility change; historical bundles still work.
2. Merge the managed release-data MR adding both image mappings.
3. Coordinate the HyperShell pipeline/publisher PR with the tenant release-data
   MR registering both Components and ImageRepositories. Registration may open
   generated pipeline PRs; close duplicates of the maintained pipelines.
4. Trigger native push builds for both new Components after their service accounts
   exist. A source push affecting `components/agents` or a Konflux UI rebuild
   can do this. Both image promotions must finish before a complete bundle can
   publish. Old/incomplete snapshots fail with the missing component names.
5. Confirm the six-component bundle, GitHub release, and GitOps expansion, then
   select runtime digests from the bundle in the harness configuration.

There can be a temporary publication gap while the first two images build.
Retrying an incomplete immutable Snapshot cannot fill its missing components;
use the later complete Snapshot. Existing deployments are unaffected.
