# Glass route contract and state matrix

**Spec:** `specs/platform/glass-url-addressable-navigation.spec.md`
(GLASS-NAV-01, GLASS-NAV-02, GLASS-NAV-06, GLASS-NAV-17)

This document is the single typed route and query contract for the Glass fleet
dashboard. It records every URL parameter, its default, its multi-value ordering,
its validation and limits, and its `pushState` or `replaceState` history behavior.
It also records the route and state matrix that classifies every user-addressable
piece of state.

> This file lives in the public app repository and names no fleet identity.
> Instances, clusters, tenants, namespaces, digests, and hostnames are runtime
> data discovered from the BFF payloads, never compiled in (fleet-identity
> firewall, `data-architecture.spec` section 3.5).

The codec lives framework-free in `src/domain/navigation/route.ts` (parse,
normalize, serialize) and `src/domain/navigation/actions.ts` (typed navigation
actions and the push/replace policy). The browser History integration lives in the
single driving adapter `src/adapters/routing/` (GLASS-NAV-04).

## Path

The active top-level tab is the last path segment. A leading base path is allowed
(the last segment still selects the tab), so a reverse-proxy base path works.

| Path    | Tab                                        |
| ------- | ------------------------------------------ |
| `/`     | Fleet Map (canonical default)              |
| `/map`  | Fleet Map (parsed, normalized back to `/`) |
| `/sdlc` | Agent SDLC                                 |
| `/e2e`  | E2E Runs                                   |

An unknown segment degrades to the Fleet Map. Everything else is a query
parameter, so the three tabs retain their own state independently and switching
tabs never reuses another tab's selected entity (GLASS-NAV-08).

## Query parameters

Default-valued state is omitted. Parameters are emitted in the fixed order below.
Each value is percent-encoded, so slashes, colons, spaces, Unicode, and reserved
characters round-trip exactly (GLASS-NAV-03). A single value is capped at 512
characters, scope lists at 20 entries, and the whole URL at 2048 characters; an
over-length URL degrades to the tab-only view.

### Shared

| Parameter | Meaning                                                                              | Identity                              | Default | History |
| --------- | ------------------------------------------------------------------------------------ | ------------------------------------- | ------- | ------- |
| `bundle`  | Shared freight-bar release bundle; drives both the map and the SDLC matrix highlight | Full release digest, version fallback | absent  | push    |

### Fleet Map (`node`/`gate` are mutually exclusive; `node` wins)

| Parameter  | Meaning                                                                                                                                                                      | Identity                              | Default                | History             |
| ---------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------- | ---------------------- | ------------------- |
| `node`     | Selected node drawer                                                                                                                                                         | Node id (instance key)                | absent                 | push                |
| `nodeTab`  | Node drawer sub-tab                                                                                                                                                          | `bundle`, `links` (`details` omitted) | `details`              | push                |
| `gate`     | Selected gate drawer                                                                                                                                                         | Gate id (`from->to`)                  | absent                 | push                |
| `gattempt` | Gate attempt mode (GLASS-NAV-10): `live` follows the current attempt; absent auto-pins the current attempt on a fresh open (E2E-LOG-12); any other value is a pinned attempt | `live`, a run address, or absent      | absent                 | push                |
| `gtab`     | Gate execution sub-tab                                                                                                                                                       | `logs` (`progress` omitted)           | `progress`             | push                |
| `gstep`    | Selected step scope; absent is whole-run                                                                                                                                     | Step id                               | absent                 | push                |
| `gssort`   | Gate step-table sort                                                                                                                                                         | `col-dir`, e.g. `1-asc`               | none (execution order) | push                |
| `glog`     | Expanded gate log viewer                                                                                                                                                     | `1`                                   | closed                 | push                |
| `glogq`    | Gate log search                                                                                                                                                              | text                                  | empty                  | replace (coalesced) |

### Agent SDLC

| Parameter     | Meaning                                            | Identity                       | Default | History             |
| ------------- | -------------------------------------------------- | ------------------------------ | ------- | ------------------- |
| `scope`       | Scope multi-select; repeated, sorted, deduplicated | Scope string                   | none    | push                |
| `q`           | Issue number/title search                          | text                           | empty   | replace (coalesced) |
| `card`        | Selected board or escalation card                  | Issue number                   | absent  | push                |
| `shipped`     | Selected shipped issue                             | Issue number                   | absent  | push                |
| `shippedVer`  | Version context for the shipped issue              | Version string                 | absent  | push                |
| `shiprow`     | Expanded shipped-bundle matrix row                 | Bundle seed (version fallback) | absent  | push                |
| `escalations` | Escalations notification drawer open               | `1`                            | closed  | push                |

With `escalations=1` and `card` both set, the view shows an escalation's issue
detail; browser Back and the drawer's Back-to-list both return to the still-open
list (GLASS-NAV-13).

### E2E Runs

| Parameter | Meaning                                                         | Identity                                        | Default                                   | History             |
| --------- | --------------------------------------------------------------- | ----------------------------------------------- | ----------------------------------------- | ------------------- |
| `runq`    | Run search                                                      | text                                            | empty                                     | replace (coalesced) |
| `runsort` | Runs-table sort                                                 | `col-dir`                                       | `2-desc` (Started, newest first; omitted) | push                |
| `run`     | Selected run                                                    | Collision-free address `tenant/namespace/runId` | absent                                    | push                |
| `rund`    | Drawer state when a run is selected; `0` is selected-but-closed | `0`                                             | open when `run` set                       | push                |
| `rtab`    | Run execution sub-tab                                           | `logs` (`progress` omitted)                     | `progress`                                | push                |
| `rstep`   | Selected step scope; absent is whole-run                        | Step id                                         | absent                                    | push                |
| `rssort`  | Step-table sort                                                 | `col-dir`                                       | none (execution order)                    | push                |
| `rlog`    | Expanded run log viewer                                         | `1`                                             | closed                                    | push                |
| `rlogq`   | Run log search                                                  | text                                            | empty                                     | replace (coalesced) |

The gate drawer (`g*`) and the run detail (`r*`) share one execution-detail codec
(GLASS-NAV-11): the same sub-tab, step scope, step sort, expanded-viewer, and log
search, under two parameter prefixes so each tab keeps its own state.

## Push versus replace (GLASS-NAV-06)

- Meaningful navigation (tab, entity, drawer, step, sort, filter commit) uses
  `pushState`, so Back and Forward retrace discrete investigation steps.
- Text input (all `*q*` search fields) uses `replaceState`, so one Back press does
  not undo one keystroke at a time.
- Back and Forward are handled from `popstate` by re-parsing the URL as the single
  source of truth; a programmatic push or replace never echoes a new entry, and a
  no-op navigation creates no duplicate entry (GLASS-NAV-07).

## Navigation affordances (GLASS-NAV-08)

The HTML inline-link affordances render real anchors through `NavLinkButton`
(`hrefFor` + `useNavLink`), so copy-link, open-in-new-tab, and Ctrl/Cmd-click work
while a plain click navigates in place: the SDLC shipped-bundle link to the Fleet
Map, the SDLC shipped-issue links, the E2E run revision links, and the gate's View
history cross-link. The Fleet Map node and gate affordances are drawn on the custom
SVG promotion canvas and the Agent SDLC board cards are the animated board widget;
both navigate in place on click, and their destinations remain fully URL-addressable
(`node`/`gate`/`nodeTab`, `card`) so reload, Back/Forward, and pasted deep links
reconstruct them.

## Canonicalization (GLASS-NAV-02)

Two navigation sequences that reach the same view serialize to the same URL:
default-valued state is dropped, scopes are sorted and deduplicated, the runs-table
default sort collapses to absent, a `node` present drops any `gate`, and execution
detail is dropped when its drawer is closed or its entity is not selected.

## State matrix (GLASS-NAV-01)

### Navigation and deliberate view state (URL-represented)

| State                       | Owner             | Identity                  | Default            | URL                     | History | Missing-data behavior                            |
| --------------------------- | ----------------- | ------------------------- | ------------------ | ----------------------- | ------- | ------------------------------------------------ |
| Active tab                  | shell             | enum                      | `map`              | path                    | push    | unknown segment to map                           |
| Shared bundle               | shell freight bar | digest (version fallback) | none               | `bundle`                | push    | highlight simply empty                           |
| Map node selection + drawer | Fleet Map         | node id                   | none               | `node`                  | push    | node not found to safe map                       |
| Map node sub-tab            | map drawer        | enum                      | `details`          | `nodeTab`               | push    | unknown to `details`                             |
| Map gate selection + drawer | Fleet Map         | gate id                   | none               | `gate`                  | push    | gate not found to safe map                       |
| Gate attempt (live/pinned)  | gate drawer       | `live` or run address     | auto-pin on open   | `gattempt`              | push    | pinned attempt stays; newer-attempt notice       |
| Gate exec sub-tab           | gate drawer       | enum                      | `progress`         | `gtab`                  | push    | unknown to `progress`                            |
| Gate step scope             | gate drawer       | step id                   | whole run          | `gstep`                 | push    | unknown step stays requested                     |
| Gate step sort              | gate drawer       | col-dir                   | execution order    | `gssort`                | push    | out-of-range dropped                             |
| Gate expanded log viewer    | gate drawer       | boolean                   | closed             | `glog`                  | push    | -                                                |
| Gate log search             | gate drawer       | text                      | empty              | `glogq`                 | replace | -                                                |
| SDLC scopes                 | Agent SDLC        | scope strings             | none               | `scope`                 | push    | capped, deduplicated, sorted                     |
| SDLC search                 | Agent SDLC        | text                      | empty              | `q`                     | replace | -                                                |
| SDLC card detail            | Agent SDLC        | issue number              | none               | `card`                  | push    | not found to safe view                           |
| SDLC shipped detail         | Agent SDLC        | issue number + version    | none               | `shipped`, `shippedVer` | push    | not found to safe view                           |
| SDLC expanded matrix row    | Agent SDLC        | bundle seed               | none               | `shiprow`               | push    | absent row stays collapsed                       |
| SDLC escalations drawer     | Agent SDLC        | boolean                   | closed             | `escalations`           | push    | -                                                |
| E2E run search              | E2E Runs          | text                      | empty              | `runq`                  | replace | -                                                |
| E2E runs sort               | E2E Runs          | col-dir                   | Started desc       | `runsort`               | push    | out-of-range dropped                             |
| E2E selected run            | E2E Runs          | run address               | none               | `run`                   | push    | expired/unauthorized explained, never reassigned |
| E2E drawer open             | E2E Runs          | boolean                   | open when selected | `rund`                  | push    | -                                                |
| E2E exec sub-tab            | run detail        | enum                      | `progress`         | `rtab`                  | push    | unknown to `progress`                            |
| E2E step scope              | run detail        | step id                   | whole run          | `rstep`                 | push    | unknown step stays requested                     |
| E2E step sort               | run detail        | col-dir                   | execution order    | `rssort`                | push    | out-of-range dropped                             |
| E2E expanded log viewer     | run detail        | boolean                   | closed             | `rlog`                  | push    | -                                                |
| E2E log search              | run detail        | text                      | empty              | `rlogq`                 | replace | -                                                |

### Intentionally transient state (NOT URL-represented)

Follow/pause tail state and the log timestamp toggle; typeahead open state and its
input text; the responsive overlay-versus-inline drawer layout; drawer pixel width;
the map pan, zoom, and reset viewport; board and matrix scroll offsets; the
node-detail temporal cursor; escalation-bucket collapse; hover and chart-cursor
samples; focus location; animation frames; loading and backend-unavailable flags;
live-stream connection status; relative-time clocks; and fetched snapshots and log
buffers. These are view ephemera or live or derived data; the URL identifies which
data to request, never the data itself and never access to it (GLASS-NAV-16).

## Subscriptions (GLASS-NAV-18)

The run detail owns exactly one live stream per selected run, keyed by the run
address. A URL-only change on the same run (sub-tab, step, sort, log search,
expanded viewer) does not restart the stream or reset follow/pause. Changing the
selected run closes the previous stream and opens exactly one new stream.
