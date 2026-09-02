# Source Releases

HyperShell uses Conventional Commits and Release Please. Release Please keeps
one Release PR open against `main`. The PR is not a release candidate tag. It
is the list of changes for the next source release.

## Pull Request Titles

Use this form for each pull request title:

```text
<type>(<optional-scope>)<optional-!>: <description>
```

To keep Jira tracking in the title, put the Jira key after the colon:

```text
feat(release): [HYPERSHELL-123] add managed source releases
```

The conventional type must remain first. Do not put the Jira key before the
type. Release Please cannot parse a squash commit that starts with the Jira
key.

The allowed types are `feat`, `fix`, `perf`, `refactor`, `docs`, `spec`,
`deps`, `build`, `ci`, `test`, `style`, `chore`, and `revert`. The `feat` type
increments the minor version. The `fix` type increments the patch version. A
`!` marker or a `BREAKING CHANGE` footer increments the major version. Before
`1.0.0`, a breaking change increments the minor version. Other types enter the
next changelog, but they do not open a Release PR by themselves.

GitHub uses the pull request title as the squash commit subject. Do not change
the subject during merge.

## Release Operation

The Release Please workflow runs after a successful `main` E2E workflow. It
opens a Release PR after a feature, fix, or breaking change enters `main`. If a
Release PR is open, later `main` commits update the same PR. The PR updates
`CHANGELOG.md`, `VERSION`, `.release-please-manifest.json`, and
`build-version.env`.

Keep the Release PR open until the contents are ready. Merge it through the
normal merge queue to start a release. Do not enable automatic merge for this
PR. After the final `main` E2E workflow succeeds, Release Please creates the
`vX.Y.Z` tag and the published GitHub release. Immutable releases must remain
enabled in the repository settings.

The first manifest version is `0.0.0`. The history baseline is the commit
before this release strategy. Thus, the first feature proposes `0.1.0`, and the
first changelog does not contain older repository history.

## GitHub App Setup

Create and install a GitHub App for Release Please. Give the installation these
repository permissions:

- Contents: Read and write
- Issues: Read and write
- Pull requests: Read and write

Set the app client ID in the `RELEASE_PLEASE_APP_CLIENT_ID` repository
variable. Set the private key in the `RELEASE_PLEASE_APP_PRIVATE_KEY`
repository secret. The app token lets checks run on a Release PR that the app
creates or updates.

## Failure Recovery

If release automation fails, correct the cause and run the Release Please
workflow from the Actions page. The workflow uses the committed release files
and can run again safely. Do not create the tag or GitHub release by hand while
this recovery is in progress.

## Image and Deployment Boundary

Normal `main` pushes build only components with changed build inputs. Release
PR updates do not build all images. When the Release PR merges, its `VERSION`
change makes the final `main` push build the API server, control plane, and web
console once.

The build keeps the existing full-SHA registry tags. Argo image bump continues
to select deployment images. This process does not promote images, create
semantic registry aliases, or change Argo configuration.
