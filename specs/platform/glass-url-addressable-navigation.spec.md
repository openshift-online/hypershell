# Glass URL-Addressable Navigation

**Status:** Draft
**Applies to:** `packages/fleet-dashboard-ui` (new routing adapter, domain route codec, tab shell, map, Agent SDLC, e2e-run presentation), `components/fleet-dashboard` BFF (`pkg/server` SPA fallback and base-path behavior)
**Tracks:** [HYPERSHELL-404](https://redhat.atlassian.net/browse/HYPERSHELL-404)

## Purpose

Make every meaningful, user-addressable view of the fleet dashboard (the "glass"
dashboard, `glass.infra.hypershell.app`) reconstructible from its URL. Today Glass
keeps almost all navigation, selection, and deliberate view state in
component-local React state: the active tab, the shared release-bundle selection,
map node/gate/drawer selection and nested tabs, the Agent SDLC scope filter,
search, selected card, escalations drawer, and shipped selection, and the e2e-run
list filter, sort, selected run, selected step, log scope, and log controls. A
reload, a pasted link, opening in a new tab, or an authentication round trip loses
that context, and browser Back/Forward cannot retrace an investigation.

This specification defines a single typed route/query contract for Glass and the
browser-history behavior around it. A copied link SHALL reopen the same
investigation in a fresh authenticated browser session; navigating within Glass
SHALL cooperate with Back/Forward without full-page reloads or duplicate live
streams. This covers the whole Glass dashboard across all three top-level tabs
(Fleet Map, Agent SDLC, E2E Runs), not only the E2E history view.

This spec is a follow-up to the Agent SDLC tab
([`agent-sdlc-visibility-dashboard.spec.md`](agent-sdlc-visibility-dashboard.spec.md),
HYPERSHELL-401) and the E2E execution drawers, central logging, and bundle/target
attribution ([`e2e-run-log-visibility.spec.md`](e2e-run-log-visibility.spec.md),
HYPERSHELL-402). It SHALL NOT regress either. It adds the URL layer those views
lacked; it does not change their data sources, planes, or verdict semantics.

### What URL-addressable means here

"Every bit" means all meaningful navigation, selection, and deliberate view state.
It does not mean serializing fetched records, log buffers, secrets, hover, tooltip
visibility, focus location, animation frames, loading flags, live-stream connection
status, clocks, or transient autocomplete-menu state. Derived or live status stays
derived from its authorized data source; the URL identifies *which* data to request,
never the data itself and never access to it.

## Requirements

### Requirement: GLASS-NAV-01 -- Route and State Matrix

The implementation SHALL be preceded by a documented route/state matrix covering
every user-addressable piece of Glass state, including state added since this spec
was written. For each entry the matrix SHALL record: its stable identity, its
default, its URL representation (or an explicit decision that it is not represented),
its history behavior (`pushState`, `replaceState`, coalesced, or none), its
loading/missing-data behavior, and its classification as navigation, filter, sort,
a continuous view gesture, deliberate layout, or intentionally transient
interaction. Ambiguous state (chart cursors, scroll offsets, resize gestures,
popovers, collision-group popovers, the map viewport) SHALL be classified
explicitly rather than omitted by accident. The matrix SHALL be documented in this
repository alongside the route-contract documentation (GLASS-NAV-17).

The matrix SHALL at minimum classify the following current state. Navigation and
deliberate view state (URL-represented unless a documented exception applies):
active top-level tab; the shared freight-bar selected release bundle (by bundle
seed, a full release digest falling back to version); map node/gate selection and
drawer open/closed; map node-detail sub-tab (Details/Bundle/Links); gate-execution
pinned attempt plus Progress/Logs sub-tab, selected step scope, and expanded log
viewer; Agent SDLC scope multi-select, issue search, selected board card, shipped
selection, escalations drawer and selected escalation, and expanded buckets/rows;
E2E run selection and drawer open/closed, run and step table sorts, run search,
Progress/Logs sub-tab, selected step, log search, timestamp toggle, and expanded
log viewer. Intentionally transient (NOT URL-represented): fetched snapshots and
log buffers, loading and backend-unavailable flags, live-stream connection status,
relative-time clocks, follow/pause tail state, hover and temporal-cursor samples,
typeahead open/input text, focus location, animation frames, responsive
overlay-vs-inline layout, drawer pixel width, and map pan/zoom/scroll gestures
except where GLASS-NAV-09 defines an explicit shareable viewport.

#### Scenario: Matrix precedes implementation

- GIVEN the URL-addressable navigation work begins
- WHEN the route/state matrix is produced
- THEN it SHALL list every user-addressable state across all three tabs with identity, default, URL representation, history behavior, missing-data behavior, and navigation/transient classification
- AND ambiguous state SHALL be classified explicitly rather than omitted

### Requirement: GLASS-NAV-02 -- One Typed Route Contract

Glass SHALL have one documented, typed route/query contract. It SHALL prefer
readable paths and query parameters over an opaque serialized JSON blob. Equivalent
views SHALL produce equivalent URLs: the contract SHALL define canonical defaults,
omit redundant default-valued state, and order multi-value parameters
deterministically. Every parameter SHALL have a validated type; enum parameters
SHALL reject unknown values and numeric parameters SHALL reject out-of-range values.
The contract SHALL set practical input and URL-length limits. Entities SHALL be
addressed by their stable identity -- the full release digest, the collision-free
e2e-run address, the SDLC issue number, the instance id, the gate id, and the step
id -- not by identiname, display text, row index, or an abbreviated SHA.

#### Scenario: Equivalent views produce equivalent URLs

- GIVEN two navigation sequences that arrive at the same view
- WHEN each serializes to a URL
- THEN the URLs SHALL be equal after canonicalization, with default-valued state omitted and multi-value parameters in the defined order

#### Scenario: Stable identity, not display text

- GIVEN a selected release bundle, e2e run, SDLC issue, and map instance
- WHEN each is encoded into the URL
- THEN the bundle SHALL use its full digest (version fallback), the run its collision-free tenant/namespace/run address, the issue its number, and the instance its id
- AND no identiname, abbreviated SHA, display label, or row index SHALL be used as the identifier

### Requirement: GLASS-NAV-03 -- Safe Encoding of Runtime Identifiers

The route codec SHALL encode and decode arbitrary runtime identifiers safely,
including values containing slashes, colons, spaces, Unicode, and URL-reserved
characters. Fleet instance names, cluster topology, tenants, and namespaces remain
runtime data supplied by the BFF payloads; the app SHALL NOT compile private fleet
identifiers into the bundle, and the codec SHALL treat every identifier as opaque
server data, consistent with the fleet-identity firewall
([`e2e-run-log-visibility.spec.md`](e2e-run-log-visibility.spec.md),
GLASS-NAV-16). A value that round-trips through the URL SHALL decode back to the
exact original string.

#### Scenario: Reserved characters round-trip

- GIVEN an entity id containing a slash, a colon, a space, and a non-ASCII character
- WHEN it is encoded into the URL and later decoded
- THEN the decoded value SHALL equal the original byte-for-byte
- AND it SHALL NOT corrupt neighbouring parameters or path segments

### Requirement: GLASS-NAV-04 -- Hexagonal Placement of the Route Layer

The route layer SHALL follow the narrow hexagonal UI boundary
([`specs/standards/ui/hexagonal-architecture.spec.md`](../standards/ui/hexagonal-architecture.spec.md)).
Pure parse, validate, normalize, and serialize logic SHALL live in the
framework-free domain as typed navigation state and actions, with no dependency on
React, `window`, or `history`. Browser History and location integration SHALL live
in a single driving adapter. Components SHALL read route state and dispatch typed
navigation actions through that adapter; scattered component-level `window.location`
or `history` mutations SHALL NOT be introduced. If an existing routing utility is
adopted, or a small routing dependency is added, it SHALL be justified against the
repository dependency, age, provenance, and policy gates
([`specs/web-console/architecture.spec.md`](../web-console/architecture.spec.md));
otherwise the adapter SHALL wrap the platform History API directly.

#### Scenario: Codec is framework-free

- GIVEN the route codec parses, validates, normalizes, and serializes navigation state
- WHEN it is exercised in tests
- THEN it SHALL run without React, `window`, or `history`
- AND History/location access SHALL exist only in the routing adapter

#### Scenario: No scattered location mutations

- GIVEN a component changes a tab, selection, drawer, or step
- WHEN it updates navigation
- THEN it SHALL dispatch a typed navigation action through the routing adapter
- AND it SHALL NOT call `window.location` or `history` directly

### Requirement: GLASS-NAV-05 -- Initial Hydration Before Data Resolves

Glass SHALL parse URL state on initial load, before the BFF payloads resolve, and
render toward the addressed view while data is loading. A refresh, a pasted link,
opening in a new tab, and an authentication round trip SHALL all preserve the
destination. The URL SHALL be the authoritative source of route state; the app
SHALL NOT maintain a second, conflicting state machine that overrides the parsed
URL on load.

#### Scenario: Deep link hydrates before data

- GIVEN a URL addressing the E2E Runs tab with a selected run and its Logs sub-tab
- WHEN the browser loads it cold
- THEN the view SHALL select the E2E Runs tab and show the addressed run opening in its Logs view before the run payload resolves
- AND when the payload arrives it SHALL populate the already-addressed view rather than resetting it

#### Scenario: Destination survives authentication

- GIVEN an unauthenticated browser opens a deep link into Glass
- WHEN it completes the authentication round trip
- THEN it SHALL return to the originally addressed view, not a default landing view

### Requirement: GLASS-NAV-06 -- Push Versus Replace Policy

The route layer SHALL use `pushState` for meaningful navigation -- tab, entity,
drawer, and step transitions -- so Back/Forward retrace discrete investigation
steps. It SHALL use `replaceState` or debounced/coalesced updates for text input
and continuous view changes, so one Back press does not undo every keystroke, live
event, or drag frame. Filter commits and sort changes SHALL have a specified,
documented behavior in the route-contract documentation (GLASS-NAV-17). Continuous
gestures SHALL be committed at sensible boundaries rather than recording every
pointer move or wheel tick.

#### Scenario: Typing does not spam history

- GIVEN an operator types a multi-character query into the run search or SDLC search
- WHEN the URL updates
- THEN the updates SHALL be coalesced via `replaceState`
- AND a single Back press SHALL not undo one keystroke at a time

#### Scenario: Entity navigation is a history step

- GIVEN an operator selects a run, then opens a step's logs
- WHEN they press Back twice
- THEN the first Back SHALL return to the run without the step, and the second SHALL return to the run list without a selection

### Requirement: GLASS-NAV-07 -- Popstate Without Feedback Loops

The route layer SHALL handle `popstate` (Back/Forward) by making the URL
authoritative: it SHALL reconcile view state from the URL without feedback loops,
duplicate history entries, flash resets, or stale local-state overrides. A URL-only
change SHALL NOT be echoed back as a new history entry.

#### Scenario: Back and Forward are stable

- GIVEN an operator has navigated across tabs and drawers
- WHEN they press Back and Forward repeatedly
- THEN each press SHALL move exactly one step with no duplicate entries, no reset flash, and no loop
- AND local state SHALL NOT override the URL-derived route state

### Requirement: GLASS-NAV-08 -- Cross-Tab Navigation and Inactive-Tab State

The route contract SHALL define whether inactive tabs retain their prior state and
how that is represented. Switching tabs SHALL NOT accidentally reuse a different
tab's selected entity or silently discard context; the rule SHALL be documented.
Context-preserving transitions that exist today SHALL remain addressable: SDLC
shipped bundle to Fleet Map bundle, map gate to E2E history attempt, and
bundle/run/target cross-links. Every internal navigation affordance that performs a
navigation -- identicons, bundle links, map nodes and gates, issue cards, shipped
items, and E2E history links -- SHALL resolve to a real link destination so that
copy-link, open-in-new-tab, and Ctrl/Cmd-click work.

#### Scenario: Inactive tab does not leak selection

- GIVEN a run is selected on E2E Runs and a different bundle is selected on Fleet Map
- WHEN the operator switches between tabs
- THEN each tab SHALL show its own addressed state per the documented retention rule
- AND neither tab SHALL adopt the other's selected entity

#### Scenario: Cross-link carries context

- GIVEN a shipped bundle in Agent SDLC that links to Fleet Map
- WHEN the operator activates the link
- THEN the Fleet Map SHALL open with the same bundle selected, and the URL SHALL encode that selection
- AND the link SHALL support copy, open-in-new-tab, and Ctrl/Cmd-click

### Requirement: GLASS-NAV-09 -- Fleet Map Addressable State

The Fleet Map SHALL encode its selected instance/node, promotion gate, or release
bundle, and whether the details drawer is open. Node-detail sub-tabs (Details,
Bundle, Links) and any other nested tabs SHALL be addressable. The gate-execution
drawer SHALL address its pinned attempt, Progress/Logs sub-tab, selected-step or
whole-run scope, and expanded log viewer, sharing the e2e execution-detail contract
(GLASS-NAV-11). Map pan, zoom, and reset SHALL default to transient per-session view
gestures and SHALL NOT create history entries; if a shareable viewport is provided
it SHALL be an explicit, documented, opt-in representation committed at gesture
boundaries and reconciled against changed viewport sizes and topology. Deliberate
layout such as drawer width SHALL be restored from a per-session or local preference,
not the shared URL, and SHALL be documented in the matrix rather than silently
omitted.

#### Scenario: Map selection and nested tab are addressable

- GIVEN an operator opens a node's drawer and switches to its Bundle sub-tab
- WHEN the URL updates
- THEN it SHALL encode the node id, the open drawer, and the Bundle sub-tab
- AND reloading SHALL restore the same node, drawer, and sub-tab

#### Scenario: Viewport gestures do not spam history

- GIVEN an operator pans and zooms the map
- WHEN the gestures settle
- THEN no per-pointer-move history entry SHALL be created
- AND any shareable viewport representation SHALL be an explicit opt-in committed at gesture boundaries

### Requirement: GLASS-NAV-10 -- Trusted Gate Target and Live Versus Pinned

A shared Fleet Map gate link SHALL preserve the gate identity together with the
exact deployed revision and trusted target context it was created against, so it does
not silently resolve to a newer retry or a different bundle after promotion advances.
The contract SHALL define an explicit live/latest mode distinct from a pinned
historical attempt. A link to the live/latest mode SHALL follow the current attempt;
a link to a pinned attempt SHALL continue to address that attempt even after newer
attempts arrive, with the newer-attempt notice shown per
[`e2e-run-log-visibility.spec.md`](e2e-run-log-visibility.spec.md) (E2E-LOG-12).

#### Scenario: Pinned attempt link does not drift

- GIVEN a shared link to a pinned gate attempt at a specific hydrated revision
- WHEN a newer retry has since been promoted
- THEN the link SHALL still resolve to the pinned attempt and revision
- AND it SHALL NOT silently show the newer attempt or a different bundle

#### Scenario: Live mode follows the current attempt

- GIVEN a link that explicitly addresses the gate's live/latest mode
- WHEN it is opened
- THEN it SHALL follow the current attempt
- AND the live/latest mode SHALL be distinguishable in the URL from a pinned attempt

### Requirement: GLASS-NAV-11 -- Shared Execution-Detail Contract

The gate-execution drawer and the E2E run detail SHALL share one addressable
execution-detail contract: the Progress/Logs sub-tab, the selected-step or
whole-run log scope, the log search, the timestamp toggle, the follow/pause state,
and the expanded log viewer. Of these, the sub-tab, the log scope (selected step
versus whole run), and the expanded-viewer state SHALL be URL-represented; log
search SHALL follow the text-input coalescing policy (GLASS-NAV-06); follow/pause
and the timestamp toggle are transient view toggles and are NOT URL-represented.
Navigating between a gate drawer and run history SHALL preserve the same execution
context. A stable anchor for a selected log event, if supported, SHALL use the event
sequence plus the run identity, never an array index; when retention or truncation
removes the anchored event the view SHALL show an explicit unavailable message
rather than jump to an unrelated line.

#### Scenario: Execution context survives gate-to-history navigation

- GIVEN an operator viewing a step's logs in a gate drawer
- WHEN they navigate to that run in E2E history
- THEN the same run, step scope, and Logs sub-tab SHALL be addressed
- AND the live subscription SHALL not restart merely because of the navigation

#### Scenario: Log-event anchor is stable

- GIVEN a deep link anchored to a specific log event by sequence and run identity
- WHEN the anchored event has been truncated out of the retained window
- THEN the view SHALL show an explicit unavailable message
- AND it SHALL NOT jump to an unrelated line or a different event at that array position

### Requirement: GLASS-NAV-12 -- E2E Runs Addressable State

The E2E Runs tab SHALL address its run search, every run-table sort column and
direction, the selected run, and whether the detail drawer is open independently of
the selection (closing the drawer SHALL preserve the selected row). A run SHALL be
addressed by its collision-free identity including trusted cluster/tenant,
namespace, and attempt; a revision prefix alone SHALL NOT identify a run. Step-table
sort columns and directions SHALL be addressable under the shared execution-detail
contract (GLASS-NAV-11). The bundle and target attribution added in
[`e2e-run-log-visibility.spec.md`](e2e-run-log-visibility.spec.md) (E2E-LOG-12)
SHALL remain consistent with the addressed run: a historical run SHALL NOT have its
bundle replaced by today's active bundle.

#### Scenario: Drawer open is independent of selection

- GIVEN a selected run with its detail drawer open
- WHEN the operator closes the drawer
- THEN the selected row SHALL be preserved and the URL SHALL encode the selection without the open drawer
- AND reopening SHALL return to the same run detail

#### Scenario: Collision-free run identity

- GIVEN two runs on different clusters sharing a short revision prefix
- WHEN each is addressed in the URL
- THEN the address SHALL include the trusted cluster/tenant, namespace, and attempt so each resolves to the correct run
- AND a revision prefix alone SHALL NOT be accepted as the identifier

### Requirement: GLASS-NAV-13 -- Agent SDLC Addressable State

The Agent SDLC tab SHALL address its scope multi-select (with canonical ordering
for multiple scopes) and its issue-number/title search, the selected board card by
issue identity rather than a serialized card object or array position, the card
detail drawer, the escalations notification drawer and the selected escalation, the
shipped selection, and expanded shipped-bundle rows. Expanded/collapsed escalation
buckets MAY remain transient disclosure state if the matrix classifies them so.
Browser Back and the drawer's internal Back control SHALL have coherent semantics:
the escalation-detail Back-to-list path and browser Back SHALL both return to the
escalation list without leaving Glass or skipping the list. A selected issue reached
through a collision-group popover SHALL be directly addressable regardless of later
collision regrouping; whether opening a popover is itself navigation SHALL be
decided and documented. Board and matrix scroll positions MAY be restored but SHALL
NOT turn wheel ticks into browser-history entries.

#### Scenario: Selected card addressed by issue identity

- GIVEN a selected SDLC board card
- WHEN the URL encodes it
- THEN it SHALL use the issue number, not a serialized card or a board position
- AND after a background refresh reorders the board the same issue SHALL remain selected

#### Scenario: Escalation Back semantics are coherent

- GIVEN an operator opens the escalations drawer and then an escalation's detail
- WHEN they press browser Back or the drawer's Back-to-list control
- THEN both SHALL return to the escalation list
- AND neither SHALL leave Glass nor skip the list

#### Scenario: Popover-reached issue stays addressable

- GIVEN an issue selected through a collision-group popover
- WHEN collisions later regroup
- THEN the issue SHALL remain directly addressable by its identity
- AND the URL SHALL not depend on the transient popover grouping

### Requirement: GLASS-NAV-14 -- Missing, Malformed, and Degraded Targets

Unknown or malformed parameters SHALL degrade to a safe, understandable view and a
canonical URL rather than crash. The view SHALL differentiate loading, an
unauthorized or unavailable target, a deleted issue or instance, an expired run or
log history, and an unavailable bundle, keeping enough of the requested identity
visible to explain what is missing. It SHALL NOT silently select the first or
current run, issue, or bundle instead of the addressed-but-missing one. A selection
SHALL survive background refresh, reordering, filtered-out rows, promotion changes,
and new retries; when a selected item no longer matches the active filters the
drawer SHALL remain accessible and the mismatch SHALL be explained rather than the
selection being dropped.

#### Scenario: Malformed parameter degrades safely

- GIVEN a URL with an unknown enum value or an out-of-range numeric parameter
- WHEN Glass loads it
- THEN it SHALL render a safe default view and normalize to a canonical URL
- AND it SHALL NOT crash

#### Scenario: Missing target is explained, not reassigned

- GIVEN a deep link to a run whose history has expired
- WHEN Glass resolves it
- THEN it SHALL show the requested run identity as unavailable with the expired-history state
- AND it SHALL NOT silently select the latest or first run instead

#### Scenario: Selection survives a filter mismatch

- GIVEN a selected item that a changed filter would exclude
- WHEN the filter is applied
- THEN the drawer SHALL remain accessible and the mismatch SHALL be explained
- AND the selection SHALL NOT be silently dropped

#### Scenario: Incompatible selections canonicalize to a single winner

- GIVEN a crafted or stale URL that sets two mutually exclusive selections at once
  (for example an Agent SDLC board card and a shipped issue, or a shipped issue and
  the escalations list)
- WHEN Glass parses it
- THEN it SHALL keep exactly one selection and clear the others
- AND it SHALL serialize back to a canonical URL carrying only the retained selection
  rather than round-tripping the impossible combination

### Requirement: GLASS-NAV-15 -- Accessibility, Focus, and Responsive Layout

Deep links and Back/Forward navigation SHALL preserve PatternFly keyboard
navigation and accessible selected/expanded state
([`specs/standards/ui/accessibility.spec.md`](../standards/ui/accessibility.spec.md)).
Focus SHALL be managed and restored across navigation; a directly loaded drawer or
modal link, where no triggering element exists, SHALL receive a sensible initial
focus. Nested dialogs and popovers SHALL handle Escape before their parent drawer.
Closing a drawer or modal through the close button, Escape, browser Back, or a
supported backdrop action SHALL update route state consistently; a directly loaded
drawer link SHALL close safely to its parent view without assuming an earlier in-app
history entry and without navigating the user off-site. The responsive overlay
versus inline drawer layout SHALL represent the same route state; resizing SHALL
NOT create history entries or discard the selection.

#### Scenario: Direct drawer link closes safely

- GIVEN a browser opens a drawer deep link cold, with no prior in-app history
- WHEN the operator closes the drawer via the close button, Escape, or Back
- THEN it SHALL return to the drawer's parent view within Glass
- AND it SHALL NOT navigate off-site or leave a dangling modal

#### Scenario: Responsive resize preserves route state

- GIVEN a selected run shown in an inline drawer on a wide viewport
- WHEN the viewport narrows to the overlay layout
- THEN the same run SHALL remain addressed and selected
- AND the resize SHALL NOT create a history entry

### Requirement: GLASS-NAV-16 -- URL Security Boundary

URL parameters SHALL identify requested data only; they SHALL NEVER grant access.
The BFF authentication middleware and SAR authorization firewall SHALL remain the
authorization boundary ([`e2e-run-log-visibility.spec.md`](e2e-run-log-visibility.spec.md),
E2E-LOG-15); a route or parameter SHALL NOT widen what a user may read.
URL-supplied strings SHALL be rendered as text and SHALL NOT be interpreted as
markup. Credentials, tokens, private log contents, and whole API payloads SHALL NOT
be placed in the URL. Query parameters owned by the authentication or deployment
layers SHALL be preserved where appropriate, and Glass SHALL NOT manufacture
arbitrary external redirects from URL input.

#### Scenario: Parameter does not grant access

- GIVEN a user not authorized for a run by the SAR firewall
- WHEN they open a deep link addressing that run
- THEN the BFF SHALL deny the data through the existing authorization path
- AND the URL parameter SHALL NOT grant any additional access

#### Scenario: URL strings render as text

- GIVEN a URL parameter containing markup-like characters
- WHEN Glass renders the requested identity
- THEN the value SHALL be escaped and rendered as text
- AND it SHALL NOT inject markup or script

### Requirement: GLASS-NAV-17 -- Documented Route Contract and Spec Updates

The change SHALL document the typed route contract -- the paths, the query
parameters, their canonical defaults, multi-value ordering, validation and limits,
and the `pushState`/`replaceState`/coalesce decision for each -- and the route/state
matrix (GLASS-NAV-01). The relevant desired-state specs SHALL be updated so the
route contract is coherent with the Agent SDLC and e2e-run specifications. The
documentation SHALL follow Simplified Technical English and SHALL NOT contain em
dashes, consistent with repository text conventions.

#### Scenario: Route contract is documented

- GIVEN the URL-addressable navigation work is complete
- WHEN the documentation is reviewed
- THEN it SHALL describe every path and query parameter, its default, ordering, validation, limits, and history behavior
- AND the route/state matrix SHALL be included

### Requirement: GLASS-NAV-18 -- Bounded Subscriptions Under URL-Driven Navigation

URL-driven navigation SHALL keep live subscriptions bounded and correctly disposed,
consistent with [`e2e-run-log-visibility.spec.md`](e2e-run-log-visibility.spec.md)
(E2E-LOG-05, E2E-LOG-06). A URL-only change -- a sub-tab switch, a step selection, a
sort, a log search, or a timestamp toggle on the same run -- SHALL NOT open a
duplicate stream, refetch or reduce all logs unnecessarily, or reset follow/pause
unexpectedly. Changing the selected run SHALL close the previous run's subscription
and open exactly one for the new run. No polling loop SHALL be added solely to keep
navigation in sync with the URL.

#### Scenario: Same-run URL change keeps one stream

- GIVEN an operator viewing a running e2e run over a live stream
- WHEN they switch the Progress/Logs sub-tab, select a step, or change a sort via the URL
- THEN exactly one stream for that run SHALL remain open
- AND the logs SHALL NOT be fully refetched and follow/pause SHALL NOT reset

#### Scenario: Changing the run swaps exactly one stream

- GIVEN a live stream open for the selected run
- WHEN the operator addresses a different run
- THEN the previous run's subscription SHALL close and exactly one new subscription SHALL open

### Requirement: GLASS-NAV-19 -- Deployed Deep-Link Delivery

Deep links SHALL work with the embedded SPA server, the reverse proxy, any supported
base path, and the authentication redirect behavior. The embedded SPA fallback in
`components/fleet-dashboard/pkg/server` SHALL return the SPA document for direct
navigation and reload of any Glass application route while continuing to serve API
and asset paths without falling back to the SPA document. Existing SPA fallback
behavior SHALL NOT be treated as sufficient evidence on its own; direct HTTP
navigation and reload of representative routes SHALL be tested.

#### Scenario: Direct navigation to a Glass route

- GIVEN a deployed Glass behind its reverse proxy and any supported base path
- WHEN a browser directly navigates to or reloads a deep link into a tab, drawer, and step
- THEN the server SHALL return the SPA document and the addressed view SHALL render
- AND API and asset paths SHALL NOT be served the SPA document

### Requirement: GLASS-NAV-20 -- Tests

The change SHALL include tests that validate behavior without a live fleet. Pure
codec tests SHALL cover round trips, canonicalization, defaults, duplicate, unknown,
and malformed parameters, escaping of reserved and Unicode characters, length
limits, and incompatible selection combinations. Integration tests SHALL exercise
initial URL hydration before async data resolves, external history changes
(Back/Forward), sort and filter persistence, direct-load drawer close behavior,
stale/expired/unauthorized selections, and the absence of silent reassignment to the
latest data. Subscription tests SHALL prove exactly one stream per visible run and
proper cleanup when navigating, closing, or switching runs. Browser tests SHALL
cover reload, Back/Forward, copy and open-in-new-tab, keyboard/Escape/focus,
desktop and narrow layouts, and deployed-style SPA fallback and base-path behavior.

#### Scenario: Codec round-trip and edge cases are tested

- GIVEN the route codec
- WHEN its tests run
- THEN they SHALL assert round trips, canonicalization, defaults, duplicate/unknown/malformed parameters, escaping, limits, and incompatible selection combinations

#### Scenario: Hydration and history are tested

- GIVEN integration tests for the route layer
- WHEN they run
- THEN they SHALL assert hydration before data, external history changes, sort/filter persistence, direct-load close behavior, stale/expired/unauthorized selections, and no silent reassignment to latest data

#### Scenario: One stream per visible run is tested

- GIVEN subscription tests over URL-driven navigation
- WHEN they run
- THEN they SHALL assert exactly one stream per visible run and correct teardown when navigating, closing, or switching runs

### Requirement: GLASS-NAV-21 -- No Approved Mockup

No approved mockup exists for URL-addressable navigation; it changes addressing and
browser behavior, not page composition. Per the repository no-mockup rule, the work
SHALL be built against the UI standards and the existing Glass views without
inventing an approval block or placeholder mockup, and the `ui-build-gate`
mockup-to-production parity checks SHALL be skipped while all non-parity checks
(PatternFly 6 usage, hexagonal boundary, accessibility, localization, tests) still
run.

#### Scenario: Built without a fabricated mockup

- GIVEN no approved mockup for URL-addressable navigation
- WHEN the work is built and verified
- THEN no mockup or approval block SHALL be fabricated
- AND the non-parity UI checks SHALL still be run

## Non-Goals

- Changing the BFF data planes, the Loki or GitHub sources, or the promotion verdict
  semantics. This spec adds the URL layer around the existing Fleet Map, Agent SDLC,
  and e2e-run views; it consumes their contracts
  ([`agent-sdlc-visibility-dashboard.spec.md`](agent-sdlc-visibility-dashboard.spec.md),
  [`e2e-run-log-visibility.spec.md`](e2e-run-log-visibility.spec.md)).
- Serializing fetched records, log buffers, or any live or derived data into the URL.
  The URL identifies which data to request; the data stays with its authorized source.
- Introducing a new durable navigation store or a server-side session for navigation
  state. Route state lives in the URL and browser history.
- Mandating a specific routing framework. A small routing dependency MAY be adopted
  only if it passes the repository dependency and policy gates; otherwise the History
  API is wrapped directly.
- Compiling any private fleet identifier, tenant, or cluster topology into the app
  bundle. Identifiers remain opaque runtime data.
