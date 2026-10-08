# Repository checks

Run all repository policy checks directly with:

```shell
make check
```

The checks scan Git-tracked files for forbidden or discouraged text, mutable
external dependencies, dependencies younger than 14 days, and incomplete CI
component registration. Typographic em dashes (U+2014) are discouraged; use
ASCII punctuation instead. Every directory under `components/` must declare
its detection paths and lint job, and that job must be included in the stable
lint summary gate. GitHub Actions and Git dependencies require full commit
SHAs, remote images require sha256 digests, and Buf plugins require exact
versions and revisions. Dockerfiles use digest-pinned build stages instead of
mutable OS package installation. Locally built `localhost/` images are allowed
only with `imagePullPolicy: Never` because they are never fetched from a registry.
Images built and pushed by the OpenShift deployment workflow may use the `:dev`
tag only under `image-registry.openshift-image-registry.svc:5000/`; this narrow
exception does not apply to other tags or registries.

The dependency-age check covers every resolved Go module, every package and
workspace importer in the root pnpm lockfile, legacy npm lockfiles during
migration, and every tool listed in `dependency-age-tools.json`. Metadata lookup
fails closed. JavaScript packages use the root pnpm workspace and frozen
`pnpm-lock.yaml`; nested lockfiles are prohibited.
An unavoidable version-specific exception belongs in
`dependency-age-allowlist.json` and requires `kind`, `name`, `version`, `reason`,
and `compensatingVerification` fields.

The checks run in CI and through both configured Git hook stages. Install the
repository's pinned Lefthook tool and both hooks after cloning with:

```shell
make hooks-install
```

Run the hook suite manually with `make hooks-run`.

## Generated CLI and terminal UI

`generate-cli.sh` and `generate-tui.sh` regenerate the `hsctl` commands and the
terminal UI descriptor with the rh-trex-ai generators. Both source
`lib/trex-checkout.sh`, which checks out rh-trex-ai at the commit recorded in
`rh-trex-ai.ref` (a full SHA) into `~/.cache/hypershell/rh-trex-ai`, and refuses
a checkout whose files were modified. Run them through `make generate-cli` and
`make generate-tui`; pass `--check` (the `generate-*-check` targets) to fail on
drift instead of writing. `trex-checkout.sh` prints the checkout path for
callers such as the dependency age check, which also scans the generators' own
Go module graphs. `generate-cli.sh` lists the files and commands that are
hand-maintained, and `--check` also fails on resource commands the generator no
longer emits. See the CLI and Terminal UI section of `DEVELOPMENT.md` for the
workflow and for updating the pin.

## Whitelist guidance

When an unavoidable usage needs an exception, add an entry to the root-level
`.forbidden-terms-whitelist.json` file:

```json
[
  {
    "filename": "path/to/file.ext",
    "line": 42,
    "rationale": "Required to preserve a legacy external identifier."
  }
]
```

Filenames must be normalized paths relative to the repository root, lines must
be positive integers, and rationales must be non-empty. The check fails on
duplicate or stale entries so exceptions remain tied to a current usage.
