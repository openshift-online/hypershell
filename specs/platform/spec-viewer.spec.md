# Spec Viewer

**ID prefix:** DEV-SPV
**Status:** Active
**Applies to:** `packages/spec-viewer-ui` (browser app and local dev server),
`components/fleet-dashboard/pkg/specs` (data API in the fleet dashboard)
**Related:** `specs/standards/platform/spec-identifiers.spec.md` (ID scheme,
test markers, traceability), `e2e-run-log-visibility.spec.md` (E2E-LOG-02
fleet-identity firewall, E2E-LOG-15 access control)

## Purpose

The spec viewer is a web UI for finding and reading the specs in `specs/`.
It also shows how well each spec follows the spec format, and which tests
cite each requirement and scenario.

It runs in two places:

1. Locally, with `make spec-ui`, over the developer's own checkout.
2. Inside the fleet dashboard at `/specs/`, over any branch of the product
   repository. The data is downloaded from GitHub at runtime.

The main view is a conformance table: a tree of folders, spec files,
requirements, and scenarios. A search box filters the table. A document
panel shows the selected spec file. Clicking a requirement or scenario ID
lists every file in the repository that mentions it.

### Terms

- **Spec**: a markdown heading that starts with `Requirement` (or
  `Legacy compatibility`), or that starts with a requirement ID.
- **Scenario**: a heading that starts with `Scenario:`. It belongs to the
  spec above it.
- **Branch**: a git branch of the repository, or, locally, the working tree
  with uncommitted edits.
- **Test marker**: a `Verifies:` comment above a test, as defined by
  STD-IDS-07.

## Requirements

### Requirement: DEV-SPV-01 -- Local Viewer Over the Working Tree

`make spec-ui` SHALL start the viewer on `http://127.0.0.1:5180`. It SHALL
read the specs, test markers, and repository files from the developer's
checkout, including uncommitted edits. The port SHALL be configurable with
`SPEC_VIEWER_PORT`, and the specs folder with `SPEC_VIEWER_SPECS_DIR`.

When a spec file changes on disk, the open page SHALL reload its data
without a manual refresh.

**Verification:** Demonstration -- run `make spec-ui`, edit a spec, and see
the open page update.

#### Scenario: DEV-SPV-01.01 -- An edited spec shows without a refresh

- GIVEN the viewer is open on the working tree
- WHEN a developer saves a change to a file under `specs/`
- THEN the table and the open document SHALL show the change
- AND the page SHALL NOT need a manual reload

### Requirement: DEV-SPV-02 -- Hosted in the Fleet Dashboard

The fleet dashboard SHALL serve the viewer's page at `/specs/` and its data
API at `/specs/api/`. Both SHALL sit behind the dashboard's existing login
and permission check (E2E-LOG-15). A request to `/specs/` SHALL return the
viewer's page, not the dashboard's own page.

#### Scenario: DEV-SPV-02.01 -- The data API needs a login

- GIVEN authentication is enabled in the fleet dashboard
- WHEN a caller without a session requests `/specs/api/specs.json`
- THEN the dashboard SHALL refuse the request through its existing
  authentication path
- AND no spec data SHALL be returned

#### Scenario: DEV-SPV-02.02 -- The folder path serves the viewer

- GIVEN the fleet dashboard is running
- WHEN a browser requests `/specs/`
- THEN the response SHALL be the spec viewer's `index.html`

### Requirement: DEV-SPV-03 -- No Spec Data in the Image

The fleet dashboard image is public, and the specs name fleet instances. The
viewer's build SHALL hold code only. Spec text, test markers, and repository
files SHALL be read at runtime, never compiled into the image. The
fleet-identity firewall check (E2E-LOG-02,
`scripts/check_fleet_dashboard_firewall.sh`) SHALL scan
`packages/spec-viewer-ui` as well as the dashboard's own sources.

#### Scenario: DEV-SPV-03.01 -- The build emits no spec data

- GIVEN the viewer package is built for the dashboard image
- WHEN the build output is listed
- THEN it SHALL contain no file with the text of a spec under `specs/`

### Requirement: DEV-SPV-04 -- Data API

The local dev server and the fleet dashboard SHALL serve the same data API,
relative to the page:

| Endpoint | Returns |
| --- | --- |
| `api/refs` | the branches, and which one is the default |
| `api/specs.json` | every markdown file under `specs/`, with its text |
| `api/references?id=ID` | the repository lines that mention `ID` as a whole word |
| `api/file?path=PATH` | the text of one repository file |
| `api/markers` | every test marker line, with the lines that follow it up to its test |

Every endpoint except `api/refs` SHALL take `ref=BRANCH`. Without it, the
server SHALL use its default: the working tree locally, and the repository's
default branch in the dashboard. Only a branch that `api/refs` lists SHALL be
readable.

The server SHALL serve only files in the branch's own file list: files git
lists, minus binary files and files over 1 MB. Any other path SHALL get a
`404`, so no path can reach outside the repository.

#### Scenario: DEV-SPV-04.01 -- An unknown branch is refused

- GIVEN `api/refs` does not list the branch `no-such-branch`
- WHEN a client requests `api/specs.json?ref=no-such-branch`
- THEN the server SHALL answer `404`
- AND it SHALL NOT download or read that branch

#### Scenario: DEV-SPV-04.02 -- A path outside the repository is refused

- GIVEN a client requests `api/file?path=../etc/passwd`
- WHEN the server looks the path up in the branch's file list
- THEN it SHALL answer `404`
- AND it SHALL NOT read any file outside the repository

#### Scenario: DEV-SPV-04.03 -- References match the whole ID

- GIVEN one file mentions `ABC-DEF-01`, another `ABC-DEF-010`, and another
  `ABC-DEF-01.01`
- WHEN a client requests `api/references?id=ABC-DEF-01`
- THEN only the file that mentions `ABC-DEF-01` itself SHALL be returned

### Requirement: DEV-SPV-05 -- Branches From GitHub in the Dashboard

In the fleet dashboard, the branch list SHALL come from the GitHub repository
named by `FD_UPSTREAM_REPO`, through the API at `FD_GITHUB_API_BASE`, with the
token in `FD_UPSTREAM_TOKEN_FILE` when it is set. These are the settings the
dashboard already uses; the viewer SHALL NOT add new ones.

Each branch SHALL be read as one archive download and kept in memory:

1. A copy SHALL be fresh for 5 minutes. After that, the server SHALL answer
   from the old copy while it downloads a new one in the background.
2. The server SHALL keep at most 3 branches, dropping the least recently
   used.
3. A failed download SHALL be retried no sooner than 30 seconds later.

When `FD_UPSTREAM_REPO` is empty, the page SHALL load and say that the data
source is not configured. A GitHub error SHALL show as an error message on
the page, not as an empty table.

#### Scenario: DEV-SPV-05.01 -- A stale branch answers while it refreshes

- GIVEN the branch `main` was downloaded more than 5 minutes ago
- WHEN a client requests `api/specs.json?ref=main`
- THEN the server SHALL answer at once from the copy it holds
- AND it SHALL start one background download of `main`

#### Scenario: DEV-SPV-05.02 -- A fourth branch drops the oldest

- GIVEN the server holds `main`, `feature/a`, and `feature/b`, and `main` is
  the least recently used
- WHEN a client requests `feature/c`
- THEN the server SHALL drop `main` and keep `feature/a`, `feature/b`, and
  `feature/c`

#### Scenario: DEV-SPV-05.03 -- No repository configured

- GIVEN `FD_UPSTREAM_REPO` is empty
- WHEN a user opens `/specs/`
- THEN the page SHALL load
- AND it SHALL say that the data source is not configured

### Requirement: DEV-SPV-06 -- Branch Picker

The header SHALL have a branch picker that lists the branches from
`api/refs`, with the default branch first and marked as the default.
Locally, the first entry SHALL be the working tree. The picker SHALL let the
user type to filter the list.

Choosing a branch SHALL reload the table, the document, the test markers,
and the references for that branch. The chosen branch SHALL be kept in the
URL (`?ref=BRANCH`), so a shared link opens the same branch.

#### Scenario: DEV-SPV-06.01 -- A link keeps the branch

- GIVEN a user has chosen the branch `feature/x`
- WHEN they copy the URL and open it in a new tab
- THEN the viewer SHALL show `feature/x`

### Requirement: DEV-SPV-07 -- Reading Specs Into Nodes

The viewer SHALL read every `.md` file under `specs/` and split it at its
headings:

1. A heading that starts with `Requirement` or `Legacy compatibility`, or
   that starts with a requirement ID, SHALL be a spec. Its ID SHALL show as a
   label.
2. A heading that starts with `Scenario:` SHALL be a scenario of the spec
   above it. Its ID, when present, SHALL show as a label.
3. Any other heading SHALL be plain text of the file. Text outside every
   spec SHALL be shown under its nearest heading, with a `[nospec] ` prefix.

A heading that only starts with an ID-like word, such as `HTTP-2 upgrade`,
SHALL NOT be read as an ID.

#### Scenario: DEV-SPV-07.01 -- A scenario belongs to its spec

- GIVEN a file with `### Requirement: ABC-DEF-01 -- Title` followed by
  `#### Scenario: ABC-DEF-01.01 -- Case`
- WHEN the viewer reads the file
- THEN `ABC-DEF-01.01` SHALL be a scenario of the spec `ABC-DEF-01`

### Requirement: DEV-SPV-08 -- Search

The header SHALL have a search box. A query SHALL be a list of terms, and
every term SHALL match:

1. A plain word SHALL match when it appears in the text, when it abbreviates
   a title, or when it is one or two letters off. An abbreviation starts at
   the start of a word, and each next part starts at the first letter of the
   very next word: `prvdur` matches "Provision Duration".
2. Text in quotes SHALL match literally, as those words in that order,
   ignoring case and line breaks.
3. Words in brackets SHALL match only when they are all on one line, in any
   order.

Terms SHALL be looked up across a whole spec: its own text and its
scenarios. When one scenario holds every term by itself, that scenario SHALL
be the match.

#### Scenario: DEV-SPV-08.01 -- An abbreviation does not skip words

- GIVEN a spec titled "Reject loop flies backward"
- WHEN the user searches `rbac`
- THEN that spec SHALL NOT match

#### Scenario: DEV-SPV-08.02 -- Terms combine across a spec and its scenarios

- GIVEN a spec whose text says "Keycloak" and whose scenario says "deleted"
- WHEN the user searches `keycloak deleted`
- THEN that spec SHALL match

### Requirement: DEV-SPV-09 -- Search Filters the Conformance Table

A search SHALL keep only the table rows that match, or that hold a match,
and SHALL expand the rows above each match. Text that matches inside a spec
or scenario SHALL count for that row. Text that matches outside every spec
SHALL count for its file row.

Hovering the title of a matching row SHALL show the matching text, with the
search terms marked. The header SHALL show how many matches were found, in
how many files.

#### Scenario: DEV-SPV-09.01 -- A match in a scenario expands its parents

- GIVEN a scenario whose text contains "renewed"
- WHEN the user searches `renewed`
- THEN the table SHALL show that scenario's folder, file, and spec,
  expanded
- AND rows with no match SHALL be hidden

#### Scenario: DEV-SPV-09.02 -- Hover shows the matching text

- GIVEN a search has matched a scenario
- WHEN the user hovers its title
- THEN a tooltip SHALL show the matching text with the terms marked

### Requirement: DEV-SPV-10 -- Stars

Each spec and scenario row SHALL have a star to the left of its title. An
empty gray star SHALL mean the item is not starred; a filled dark star SHALL
mean it is. The star SHALL stand out more on hover. Stars SHALL be saved in
the browser and SHALL stay across branches.

A star button next to the search box SHALL show only starred items. It SHALL
combine with the search. A starred spec SHALL bring its scenarios. A starred
item that does not exist on the chosen branch SHALL be hidden, and SHALL show
again when the user returns to a branch that has it.

#### Scenario: DEV-SPV-10.01 -- The starred filter shows a spec with its scenarios

- GIVEN the user has starred a spec and none of its scenarios
- WHEN they turn on the starred filter
- THEN the table SHALL show that spec and all its scenarios
- AND it SHALL hide every unstarred spec

### Requirement: DEV-SPV-11 -- Conformance Table

The table SHALL show, for each folder and file:

| Column | Meaning |
| --- | --- |
| Reqs | requirements |
| Scen. | scenarios under those requirements |
| OK scen. | scenarios with a `WHEN` step followed by a `THEN` step |
| No ID | requirements whose heading has no ID |
| No S-ID | scenarios whose heading has no ID |
| Tested | scenarios cited by a test marker |
| Conformance | the share of requirements that pass every check |

A requirement SHALL pass when:

1. It has at least one scenario.
2. Every scenario has a `WHEN` step and a later `THEN` step. A step is a line
   that starts with an uppercase `GIVEN`, `WHEN`, `THEN`, `AND`, or `BUT`,
   optionally bold or in a list.
3. Its own text uses `SHALL` or `MUST`.
4. Its heading has a requirement ID, and every scenario heading has a
   scenario ID.
5. Every scenario is cited by a test marker, unless the requirement's
   verification method (STD-IDS-05) is `Inspection`, `Analysis`, or
   `Demonstration`. Such a requirement SHALL show a "Verified by" label
   instead.

Each spec and scenario row SHALL list its problems as labels. A tested
scenario SHALL list the tests that cite it; clicking a test SHALL open its
file at the marker. Files with no requirements SHALL be listed below the
table. Hovering a file's title SHALL show its path; the path SHALL NOT be
shown in the row itself.

#### Scenario: DEV-SPV-11.01 -- A scenario without THEN fails its spec

- GIVEN a spec whose only scenario has `WHEN` but no `THEN`
- WHEN the table is built
- THEN that spec SHALL NOT pass
- AND its row SHALL show the problem as a label

#### Scenario: DEV-SPV-11.02 -- An inspection-verified spec needs no test

- GIVEN a spec annotated `**Verification:** Inspection -- reviewed by hand`
- AND no test marker cites it or its scenarios
- WHEN the table is built
- THEN the missing tests SHALL NOT fail that spec
- AND its row SHALL show a "Verified by" label

### Requirement: DEV-SPV-12 -- Test Traceability Matches the Repository Check

The viewer SHALL apply the marker grammar of `scripts/check_spec_ids.py`
(STD-IDS-07, STD-IDS-08) to the marker lines from `api/markers`, so its
Tested counts match `make traceability`. It SHALL NOT need Python. A test
SHALL run both over the repository and fail on any difference.

Until the marker scan finishes, or if it fails, the Tested check SHALL be
left out of the conformance result, and the page SHALL say so. A "Reload test
markers" button SHALL scan again.

#### Scenario: DEV-SPV-12.01 -- Counts match the repository check

- GIVEN the repository at any commit
- WHEN the parity test runs the viewer's grammar and the Python check over it
- THEN both SHALL report the same covered scenarios and requirements

### Requirement: DEV-SPV-13 -- Find References

Clicking a spec or scenario ID label SHALL list every repository file that
mentions the ID, test files first, with the lines that mention it. The label
SHALL show a "Find references" tooltip. This SHALL work for ID labels in the
table and for IDs in the open document (DEV-SPV-14).

The list SHALL replace the table. A Back button SHALL return to the table as
the user left it: the same search, stars filter, expanded rows, and scroll
position.

Clicking a file or a line SHALL open the file in the document panel with
syntax coloring. The line SHALL scroll into view and flash, and every mention
of the ID SHALL be marked.

#### Scenario: DEV-SPV-13.01 -- Back keeps the table state

- GIVEN the user has searched, expanded some rows, and clicked an ID label
- WHEN they press Back
- THEN the table SHALL show the same search results and expanded rows

#### Scenario: DEV-SPV-13.02 -- Opening a reference marks the ID

- GIVEN the references of `ABC-DEF-01` list a test file
- WHEN the user clicks one of its lines
- THEN the file SHALL open with that line in view and flashing
- AND every mention of `ABC-DEF-01` in the file SHALL be marked

### Requirement: DEV-SPV-14 -- Document Panel

Clicking a spec, scenario, or file title SHALL open its spec file in the
document panel, scroll to that heading, and flash it. The selection SHALL be
kept in the URL, so reloads and the browser's back button work.

In the open document:

1. A link to another spec SHALL open that spec in the viewer. A link to a
   file outside `specs/` that the viewer cannot open SHALL be shown as
   disabled, with a tooltip that says why.
2. Every requirement or scenario ID that exists on the chosen branch SHALL be
   shown as a label, like the ones in the table. A word that only looks like
   an ID SHALL stay plain text.
3. The search terms SHALL be marked.
4. Code SHALL have a light gray background, in both themes.
5. Spec markdown SHALL be rendered through React elements, not by injecting
   HTML into the document. Raw HTML in a spec SHALL remain inert text. Links
   SHALL resolve only to viewer-managed spec links or safe external URLs.

The document SHALL scroll on its own, with its own scrollbar.

#### Scenario: DEV-SPV-14.01 -- An ID in the text opens its references

- GIVEN the open document says "Builds on ABC-DEF-01"
- AND `ABC-DEF-01` exists on the chosen branch
- WHEN the user clicks the `ABC-DEF-01` label in the document
- THEN the references of `ABC-DEF-01` SHALL be listed

#### Scenario: DEV-SPV-14.02 -- A link keeps the selection

- GIVEN the user has opened a scenario
- WHEN they reload the page
- THEN the same scenario SHALL be open and in view

#### Scenario: DEV-SPV-14.03 -- Raw HTML stays inert

- GIVEN a spec contains a `<script>` element
- WHEN the user opens the document
- THEN the HTML SHALL be shown as text and SHALL not execute

#### Scenario: DEV-SPV-14.04 -- An unsafe link is disabled

- GIVEN a spec links to `javascript:alert(1)`
- WHEN the user opens the document
- THEN the link SHALL not have an executable URL

### Requirement: DEV-SPV-15 -- Layout and Theme

The table and the document SHALL sit side by side, split by a border that
the user can drag, or move with the arrow keys. The header SHALL have:

1. A button that hides the document, giving the table the full width.
   Opening an item SHALL show the document again.
2. A button that maximizes the document, folding the table away, and
   restores it. Finding references SHALL restore the table.
3. A button that switches between the light and dark themes. Until the user
   picks one, the viewer SHALL follow the system setting.

The panel widths, the layout, and the theme SHALL be saved in the browser.

#### Scenario: DEV-SPV-15.01 -- The theme follows the system until chosen

- GIVEN the user has never chosen a theme and the system uses dark mode
- WHEN the viewer opens
- THEN it SHALL use the dark theme

### Requirement: DEV-SPV-16 -- UI Conventions

The viewer SHALL follow the web UI standards (`specs/standards/ui/`):

1. It SHALL use PatternFly 6 components.
2. All user-facing text SHALL come from its message catalog (`react-intl`),
   not from strings in components.
3. Every control SHALL have an accessible name.
4. Parsing, search, conformance, and traceability SHALL live in pure domain
   code, with no React and no I/O. Data access SHALL sit behind
   application-owned ports.

**Verification:** Inspection -- reviewed in the package's lint and code
review; the domain code is covered by its own unit tests.

#### Scenario: DEV-SPV-16.01 -- No hard-coded user-facing text

- GIVEN a change adds a label, button, or message to the viewer
- WHEN the change is reviewed
- THEN its text SHALL be a message in `src/messages.ts`
- AND the domain code SHALL NOT import React or perform I/O

### Requirement: DEV-SPV-17 -- Removable as One Unit

The viewer SHALL be removable from the fleet dashboard without changing any
other dashboard feature. Its code SHALL live in `packages/spec-viewer-ui` and
`components/fleet-dashboard/pkg/specs`. The dashboard SHALL reference it only
through one mount in its `main.go`, the image build steps, and the CI
registrations listed in the package's README.

**Verification:** Inspection -- the README lists every place to edit, and a
review confirms no other dashboard code imports the viewer.

#### Scenario: DEV-SPV-17.01 -- Removing the viewer leaves the dashboard working

- GIVEN the steps in the package README's removal list are applied
- WHEN the fleet dashboard is built and its tests run
- THEN the build and tests SHALL pass
- AND every other dashboard page SHALL work as before

### Requirement: DEV-SPV-18 -- Branch Comparison

The branch picker SHALL show a compare button on every branch except the
selected one. Pressing it SHALL switch the viewer to diff mode, comparing the
selected branch (the current branch) with that branch. The viewer SHALL read
both branches through the same data API and compare them in the browser.

In diff mode:

1. The table SHALL be replaced by a tree of folders, files, requirements,
   and scenarios that differ between the two branches. A folder or file with
   no difference SHALL NOT show. A requirement with no difference SHALL show
   only when one of its scenarios differs.
2. Items SHALL be matched by ID. An item without an ID SHALL be matched by
   its file and heading. An item whose text differs only in line endings,
   trailing spaces, or blank lines SHALL count as equal.
3. An item that exists only in the current branch SHALL carry a "New" label.
   An item that exists only in the compared branch SHALL carry a "Removed"
   label. An item in both branches with different text SHALL carry a
   "Changed" label.
4. Opening a new or removed item SHALL show it as a normal document, from the
   branch that has it, without comparing.
5. Opening a changed item SHALL show a split view: the compared branch on the
   left, the current branch on the right. Changed lines SHALL sit side by
   side. Removed text SHALL have a red background and added text a green
   background, and the words that differ inside a changed line SHALL be
   marked more strongly.
   The split view SHALL have a switch between raw markdown and rendered
   markdown. Rendered SHALL be the default. Rendered SHALL show each
   markdown block as it reads, with a changed block in red on the left and
   green on the right, and the words that differ marked more strongly. Raw
   SHALL show the markdown source line by line.
6. The header SHALL name the compared branch and SHALL have a control that
   leaves diff mode. The search box and the starred filter SHALL be disabled.
7. The compared branch SHALL be kept in the URL (`?compare=BRANCH`), so a
   shared link opens the same comparison.

#### Scenario: DEV-SPV-18.01 -- New and removed items are labeled

- GIVEN the current branch has a requirement the compared branch lacks, and
  the compared branch has a file the current branch lacks
- WHEN the user presses the compare button on the compared branch
- THEN the tree SHALL show the requirement with a "New" label
- AND the tree SHALL show the file and its requirements with a "Removed" label
- AND opening a removed requirement SHALL show it without a split view

#### Scenario: DEV-SPV-18.02 -- A changed item shows a split view

- GIVEN a requirement reads "before expiry" on the current branch and
  "after expiry" on the compared branch
- WHEN the user opens the requirement in diff mode
- THEN the split view SHALL show the old line on the left and the new line on
  the right
- AND only the word that differs SHALL be marked inside each line
- AND the split view SHALL show formatted blocks by default, with the
  changed blocks in red and green
- AND switching to raw markdown SHALL show the source lines

#### Scenario: DEV-SPV-18.03 -- A link keeps the comparison

- GIVEN a user is comparing with the branch `feature/y`
- WHEN they open the URL in a new tab
- THEN the viewer SHALL be in diff mode against `feature/y`
- AND leaving diff mode SHALL remove `compare` from the URL
