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

The current workflow requires a GitHub App token. The built-in `GITHUB_TOKEN`
can create a Release PR, but that PR does not start other GitHub Actions
workflows. The app token lets the Release PR start the normal required checks.

The client ID identifies the app. It is not a secret. The private key proves
the app identity and is a secret. The workflow uses both values to create a
short-lived installation token for this repository.

First, ask an `openshift-online` organization owner whether a suitable release
automation app already exists. If it does not exist, an organization owner or
a person with permission to manage the organization GitHub Apps must
[register a GitHub App][register-app]. Use these settings:

- Make the app private to the organization.
- Set the homepage URL to the HyperShell repository.
- Disable webhooks. This workflow does not use them.
- Do not request user authorization.
- Give the app these repository permissions:
  - Contents: Read and write
  - Issues: Read and write
  - Pull requests: Read and write

Do not give the app organization permissions. Install the app on the
`openshift-online` organization and give it access only to the `hypershell`
repository. The organization policy can require owner approval.

On the app settings page, copy the client ID. Under **Private keys**, select
**Generate a private key**. GitHub downloads a PEM file. Keep this file secure,
and do not commit it. If the file is lost, generate a new key.

In the HyperShell repository, go to **Settings > Secrets and variables >
Actions**. Add these values:

- Variable `RELEASE_PLEASE_APP_CLIENT_ID`: the app client ID
- Secret `RELEASE_PLEASE_APP_PRIVATE_KEY`: the complete PEM private key

The workflow does not store the generated installation token. GitHub expires
the token after one hour. A personal access token can also start subsequent
workflows, but it is tied to a person. A GitHub App is the preferred repository
credential for this automation.

[register-app]: https://docs.github.com/en/apps/creating-github-apps/registering-a-github-app/registering-a-github-app

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
