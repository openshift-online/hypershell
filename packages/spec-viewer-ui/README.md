# Spec viewer

A web UI to find and read the HyperShell specs in `specs/`, with
conformance and test traceability.

It runs in two places:

1. Locally with `make spec-ui`, over your own checkout.
2. Inside the fleet dashboard at `/specs/`, over any branch of the product
   repository, downloaded from GitHub at runtime (see
   [Deployed in the fleet dashboard](#deployed-in-the-fleet-dashboard)).

## Run it

```shell
make spec-ui
```

Then open <http://127.0.0.1:5180>.

Options:

- `SPEC_VIEWER_PORT` changes the port (default `5180`).
- `SPEC_VIEWER_SPECS_DIR` reads another specs folder. A relative path
  resolves from this directory (default `../../specs`).

Edits to spec files reload in the open page automatically.

## Branches

The branch picker in the header switches every view to another branch:
search, documents, conformance, test markers, and file references.

- Locally, **Working tree** (the default) is your checkout with uncommitted
  edits. The other entries are your local branches, read from git.
- In the fleet dashboard, the default is the repository's default branch, and
  the list holds every branch on GitHub.
- The branch is kept in the URL (`?ref=feature/x`), so a link opens the same
  branch. Pins stay across branches; a pin that does not exist on the
  selected branch is hidden until you switch back.

## What it does

1. It reads every `.md` file under `specs/`.
2. It splits each file at its headings and gives each heading a title:
   - A heading that starts with `Requirement` is a **spec**. So is a heading
     that starts with a requirement ID, such as `## ARI-01 -- Title`. The ID
     (`ABC-DEF-01`, `ABC-A11Y-01`) shows as a label.
   - A heading that starts with `Scenario:` is a **scenario**. It belongs to
     the spec above it.
   - Any other heading is a plain markdown title.
3. The search box takes three kinds of terms. Every term must match.
   - A plain word is fuzzy. It matches when it appears in the text, when its
     letters appear in order in a title (`prvdur` finds "Provision
     Duration"), or when it is one or two letters off (`keybord` finds
     "Keyboard").
   - Text in quotes is literal: `"client secret"` matches only those words,
     in that order. Case and line breaks are ignored.
   - Words in brackets must be on the same line, in any order:
     `[database rotation]` finds "the database rotation" and "the rotation of
     the database password".

   Terms are looked up across a whole spec: its own text and its scenarios.
   So `keycloak deleted` finds a spec that says "Keycloak" in its body and
   "deleted" in one of its scenarios. When one scenario holds every term by
   itself, that scenario is the result.

4. Results show as a tree: file, then spec, then scenario. Text found inside a
   spec or scenario counts for that spec or scenario. Text found anywhere else
   counts for the nearest markdown title, shown with a `[nospec] ` prefix.
5. The pin buttons keep results in the left panel. **Pin results** pins every
   current result. Pins are saved in the browser.
6. Clicking a result or a pinned item opens the file, scrolls to that heading,
   and highlights it briefly. The URL keeps the selection, so reloads and the
   back button work.

Drag the borders between the panels to resize them, or focus a border and
use the arrow keys, then Enter. The widths are saved in the browser. The menu
button in the header hides or shows the pinned panel.

Two buttons at the right of the header change the layout, which is saved in
the browser:

- **Hide document** gives the list (search, conformance, or references) the
  full width. Opening an item shows the document again.
- **Maximize document** folds the pinned and list panels away so the document
  takes the full width. **Restore panels** brings them back.

The document scrolls on its own, with its own scrollbar.

## Conformance view

Switch to **Conformance** in the header to see how well the specs follow
the spec format. The view shows a tree of folders, files, requirements, and
scenarios. For each folder and file it shows:

- **Reqs** - the number of requirements.
- **Scen.** - the number of scenarios under those requirements.
- **OK scen.** - scenarios with a `WHEN` step followed by a `THEN` step.
- **No ID** - requirements whose heading has no ID such as `ABC-DEF-01`.
- **No S-ID** - scenarios whose heading has no ID such as `ABC-DEF-01.01`.
- **Tested** - scenarios that a test cites with a `// Verifies: ABC-DEF-01.01`
  marker (`# Verifies:` in shell and Python).
- **Conformance** - the share of requirements that pass every check.

A requirement passes when all of these are true:

1. It has at least one scenario.
2. Every scenario has a `WHEN` step and a later `THEN` step. Steps are lines
   that start with an uppercase `GIVEN`, `WHEN`, `THEN`, `AND`, or `BUT`,
   optionally bold or in a list.
3. Its own text uses `SHALL` or `MUST`.
4. Its heading has a requirement ID.
5. Every scenario heading has a scenario ID.
6. Every scenario is cited by a test marker. Requirements verified another
   way (`**Verification:** Inspection`, `Analysis`, or `Demonstration`) are
   exempt and show a "Verified by ..." label instead.

Each requirement and scenario row lists its problems as labels. A tested
scenario lists the tests that cite it; click a test to open its file at the
marker. Click a name to open it in the document panel.

The server returns every line that starts a `Verifies:` comment. The viewer
applies the marker grammar of `scripts/check_spec_ids.py` (rules STD-IDS-07
and STD-IDS-08) in `src/domain/traceability.ts`, so it needs no Python.
`server/traceability-parity.test.ts` runs both over the repository and fails
on any difference, so the counts match `make traceability`. Until the scan
finishes, or if it fails, the tested check is left out. Use **Reload test
markers** after editing tests. Files with no requirements at all,
such as personas and user flows, are listed below the table.

## Traceability view

Requirements and scenarios should carry IDs, and tests should mention them,
so you can see which tests cover which requirement.

- A requirement ID leads the heading: `### Requirement: ABC-DEF-01 -- Title`.
- A scenario ID leads the scenario title and is followed by `--` or `:`:
  `#### Scenario: ABC-DEF-01.01 -- Title`. The scheme is defined in
  `specs/standards/platform/spec-identifiers.spec.md`.

When the selected requirement or scenario has an ID, the ID appears as an
extra option in the header. Select it to list every file in the repository
that mentions the ID, test files first:

1. The list shows each file with the lines that mention the ID.
2. Click a file or a line to open the file with syntax coloring (Go,
   TypeScript, JavaScript, YAML, JSON, Markdown, shell, Python, Makefile,
   Dockerfile, and a few more).
3. The line scrolls into view and flashes, and every mention of the ID is
   marked.

An ID matches only as a whole: `ABC-DEF-01` does not match `ABC-DEF-010` or the
scenario ID `ABC-DEF-01.01`. The search covers the files git lists (tracked, or
untracked but not ignored) and skips binary files and files over 1 MB.

The conformance view also counts scenarios without an ID (**No S-ID**). A
requirement with any such scenario does not conform.

## Layout

The code follows the web console's hexagonal layout:

- `src/domain/` - pure parsing, search, and tree building. No React, no I/O.
- `src/application/` - ports the UI depends on, and the catalog use case.
- `src/adapters/` - port implementations: HTTP spec source, `marked`
  markdown lexer and renderer, `localStorage` pin store.
- `src/ui/` - React and PatternFly 6 components.
- `src/main.tsx` - composition root that binds ports to adapters.
- `server/` - Vite plugin for the dev server. It serves the data API below
  and pushes a reload event when a spec changes.

The data API, relative to the page:

Every endpoint except `api/refs` takes `ref=BRANCH`; without it, the server
uses its default (`@`, the working tree, locally; the default branch in the
dashboard). Only listed branches can be read.

| Endpoint               | Returns                                                      |
| ---------------------- | ------------------------------------------------------------ |
| `api/refs`             | the branches, and the default one                            |
| `api/specs.json`       | every markdown file under `specs/`, with its text            |
| `api/references?id=ID` | repository lines that mention `ID` as a whole word           |
| `api/file?path=PATH`   | the text of one repository file                              |
| `api/markers`          | every `Verifies:` marker line, with the lines up to its test |

The fleet dashboard implements the same API in Go
(`components/fleet-dashboard/pkg/specs`). Change both together.

## Deployed in the fleet dashboard

The fleet dashboard image embeds this package's build at `/specs/` and
serves the data API at `/specs/api/`, behind the dashboard's login and
permission check.

- The build holds code only. The specs name fleet instances, and the
  dashboard image is public (the fleet-identity firewall,
  `scripts/check_fleet_dashboard_firewall.sh`), so the data is read at
  runtime.
- The branches come from `FD_UPSTREAM_REPO` (default
  `openshift-online/hypershell`) through the GitHub API at
  `FD_GITHUB_API_BASE`, with the token in `FD_UPSTREAM_TOKEN_FILE` when set.
  These are the settings the dashboard already uses for its SDLC view.
- Each branch is one tarball download, kept in memory for 5 minutes and then
  refreshed in the background; the old copy answers meanwhile. The server
  keeps the 3 most recently used branches (about 20 MB of text each).
- If `FD_UPSTREAM_REPO` is empty, the page loads and says the data source is
  not configured.

To remove the viewer from the dashboard:

1. Delete `packages/spec-viewer-ui/` and `components/fleet-dashboard/pkg/specs/`.
2. Remove the spec viewer block (`specSource`, `specsAPI`) in
   `components/fleet-dashboard/cmd/fleet-dashboard/main.go`.
3. Remove the spec viewer lines from the fleet dashboard `Dockerfile`,
   `.dockerignore`, the `.tekton/hypershell-fleet-dashboard-*` path filters,
   and the CI registrations (`spec_viewer` in `.github/component-paths.json`,
   `lint-spec-viewer`, `.github/ci-impact-scopes.yml`).

## Checks

```shell
pnpm --filter @openshift-online/hypershell-spec-viewer-ui check
```
