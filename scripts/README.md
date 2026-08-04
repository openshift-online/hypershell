# Repository checks

Run all repository policy checks directly with:

```shell
make check
```

The checks scan Git-tracked files for forbidden terminology, mutable external
dependencies, and incomplete CI component registration. Every directory under
`components/` must declare its detection paths and lint job, and that job must
be included in the stable lint summary gate. GitHub Actions and Git dependencies
require full commit SHAs, remote images require sha256 digests, and Buf plugins
require exact versions and revisions. Dockerfiles use digest-pinned build stages
instead of mutable OS package installation. Locally built `localhost/` images
are allowed only with `imagePullPolicy: Never` because they are never fetched
from a registry.

The checks run in CI and through both configured Git hook stages. Install the
repository's pinned Lefthook tool and both hooks after cloning with:

```shell
make hooks-install
```

Run the hook suite manually with `make hooks-run`.

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
