# Gateway Access Management Console Specification

**Date:** 2026-10-05
**Status:** Draft
**Applies to:** `packages/gateway-management-ui` (gateway detail pages, application ports, probes), `components/web-console` (host routes, API adapter, composition root, BFF)
**Parent:** `web-console/architecture.spec.md` -- console stack and trust boundaries
**Related:** `platform/gateway-access-management.spec.md` -- the access API, roles, directory search, and last-owner protection this surface consumes; `web-console/user_flows.md` -- "Sharing a team gateway"; `standards/ui/hexagonal-architecture.spec.md`, `standards/ui/patternfly.spec.md`, `standards/ui/domain-observability.spec.md`, `standards/ui/interaction.spec.md`, `standards/ui/content-localization.spec.md`, `standards/ui/accessibility.spec.md`

---

## Purpose

Define the **Manage access** tab on the gateway details page: a console surface where a gateway's administrators see who has access to the gateway, change or revoke that access, and add other users from the Keycloak realm directory.

This surface is a presentation and workflow layer over the gateway access API (`platform/gateway-access-management.spec.md`). It introduces no new authorization logic; the server is authoritative. It is structurally a sibling of the existing Service Accounts tab and SHALL reuse the same ports, table, query, and dialog patterns rather than introducing new ones.

## Requirements

### Requirement: GAM-UI-01 -- Manage Access Tab

The gateway details page SHALL present a **Manage access** tab adjacent to the **Connection** tab. The tab SHALL be available to any user with access to the gateway; management controls within it SHALL follow GAM-UI-07.

The tab SHALL be registered in the gateway detail tab set and participate in the existing `?tab=` URL routing and the tab aria-label enumeration. The tab content SHALL mount when selected (consistent with other detail tabs) and SHALL gate its data loading on being active.

#### Scenario: Tab appears next to Connection

- GIVEN a user opens a gateway they can access
- WHEN the gateway details page renders
- THEN a **Manage access** tab SHALL appear next to **Connection**
- AND selecting it SHALL load the access list for that gateway

---

### Requirement: GAM-UI-02 -- Access Table

The Manage access tab SHALL render the gateway's access grants in the canonical shared resource table (PatternFly 6; reuse `ResourceTable`, do not fork a new table).

Each row SHALL show:

| Column | Source (GAM-03) |
| --- | --- |
| Name | `name` (display name) |
| User ID | `username` |
| Role | `role` (`owner`, `admin`, or `user`), rendered as an inline control per GAM-UI-05 |
| (row action) | Remove access per GAM-UI-06 |

The creator's row (`is_creator: true`) SHALL be indicated (for example a "Creator" marker) for context; this is informational and does not by itself lock the row. A row's role control and remove action SHALL be disabled with an accessible explanation when the server would reject the action (GAM-UI-08): the user is the last remaining owner, or the current caller is not an owner and the row is an owner.

The table SHALL show localized empty, loading, no-results, and error states and SHALL expose a manual refresh consistent with other gateway resource tables.

#### Scenario: Access rows render user name, user ID, and role

- GIVEN the access list returns an owner (the creator), another owner, one admin, and one user
- WHEN the table renders
- THEN each row SHALL show the display name, username, and role (Owner/Admin/User)
- AND the creator's row SHALL carry the Creator marker

---

### Requirement: GAM-UI-03 -- Search and Role Filter

The table toolbar SHALL provide a **Find people...** search input and a **role** filter (All, Owner, Admin, User).

Search SHALL be debounced, SHALL cancel superseded in-flight requests, SHALL treat the entered text as a literal, and SHALL issue a bounded number of requests. Search and the role filter SHALL be applied server-side via the access list query (GAM-03). A localized no-results state SHALL offer clearing the filters.

#### Scenario: Filtering narrows the list

- GIVEN the gateway has several grants
- WHEN the user types in **Find people...** and selects role **User**
- THEN the table SHALL show only user-tier grants matching the search text
- AND clearing the filters SHALL restore the full list

---

### Requirement: GAM-UI-04 -- Add Users Directory Picker

The toolbar SHALL provide an **Add users** control that opens a searchable picker of people from the Keycloak realm directory (GAM-09).

The picker SHALL be a PatternFly typeahead with its own search box, modeled on the existing async typeahead (`gateway-placement-select`): debounced input, cancellation, a loading sentinel, a localized no-results state, and an error state with retry. It SHALL list candidates by display name and username and SHALL allow selecting any realm user, including users who have never signed in to HyperShell.

Selecting a candidate SHALL open the role assignment modal (GAM-UI-05) for that user.

#### Scenario: Add users searches the realm directory

- GIVEN realm user `dana` has never signed in to HyperShell
- WHEN an administrator opens **Add users** and searches `da`
- THEN `dana` SHALL appear as a selectable candidate
- AND selecting `dana` SHALL open the role assignment modal for `dana`

---

### Requirement: GAM-UI-05 -- Role Assignment and Change

Assigning a role to a newly selected user SHALL use a modal containing a **radio list** of the roles the current caller may assign, each with a short localized description. Owners SHALL see **User**, **Admin**, and **Owner**; admins (non-owners) SHALL see only **User** and **Admin** (the Owner option SHALL be absent or disabled with an explanation, per GAM-08). Confirming SHALL grant the selected role (GAM-04) and, on success, close the modal and refresh the access list.

Changing an existing grant's role SHALL use the inline role control in that row (GAM-UI-02), offering the same role set the caller may assign. Selecting a different role SHALL change the user's access (GAM-05). The change SHALL reflect in the row on success.

Role submissions SHALL disable their confirm/select control while in flight and SHALL surface a localized error (keeping the user's selection) on failure.

#### Scenario: Owner assigns the Owner role via the radio modal

- GIVEN an owner selected `dana` from the directory picker
- WHEN the role modal shows **User**, **Admin**, and **Owner** radio options and the owner chooses **Owner** and confirms
- THEN `dana` SHALL be granted owner access
- AND the modal SHALL close and the access list SHALL show `dana` as Owner

#### Scenario: Admin does not see the Owner option

- GIVEN a non-owner admin opens the role modal or inline role control
- THEN only **User** and **Admin** SHALL be selectable (no Owner option)

#### Scenario: Change a role inline

- GIVEN `dana` currently has **Admin**
- WHEN an owner changes `dana`'s row role to **User**
- THEN `dana`'s access SHALL change to user
- AND the row SHALL show **User** on success

---

### Requirement: GAM-UI-06 -- Remove Access

Each row the caller may manage SHALL offer a **Remove access** action that opens a confirmation modal (reusing the lifecycle-dialog pattern) and, on confirm, revokes the grant (GAM-06), then closes and refreshes the list. The action SHALL be disabled per GAM-UI-07 (owner rows for non-owners) and GAM-UI-08 (last owner).

The confirmation SHALL name the user being removed. Removing one's own access SHALL be permitted unless the caller is the last remaining owner (GAM-UI-08). Removing an owner SHALL be offered only to owners.

#### Scenario: Remove a user's access with confirmation

- GIVEN `dana` has User access to the gateway
- WHEN an administrator removes `dana` and confirms
- THEN `dana`'s access SHALL be revoked
- AND the access list SHALL no longer include `dana`

---

### Requirement: GAM-UI-07 -- Management Controls Gated to Administrators

Access management controls (Add users, the inline role control, and Remove access) SHALL be presented only to callers the server authorizes to manage access -- those for whom the access-list capabilities report `can_manage_access` (a gateway owner or admin, or a `platform:admin`; GAM-08). Viewers SHALL see the access list read-only, without those controls. The console SHALL drive these gates from the server-reported capabilities (`can_manage_access`, `can_manage_owners`), not from the caller's own listed role, so a `platform:admin` who is not on the access list still receives the full controls.

Controls that touch the **Owner tier** -- assigning the Owner role, or changing/removing a user who is an owner -- SHALL be offered only when the capabilities report `can_manage_owners` (a gateway owner, or a `platform:admin`); for non-owner admins these SHALL be absent or disabled with an accessible explanation.

The console SHALL rely on server authorization as the source of truth: it SHALL handle `403` and `409` responses with localized messaging and SHALL NOT present a management outcome the server rejected as if it succeeded.

#### Scenario: Viewer sees a read-only list

- GIVEN a viewer opens the Manage access tab
- WHEN the list renders
- THEN no Add users, role-change, or Remove access controls SHALL be offered

#### Scenario: Admin cannot act on owner rows

- GIVEN a non-owner admin opens the Manage access tab
- WHEN the table renders
- THEN owner rows SHALL have their role control and Remove access action disabled with an accessible reason

---

### Requirement: GAM-UI-08 -- Last-Owner Protection in the UI

When a user is the **last remaining owner**, that row's role control and Remove access action SHALL be disabled with an accessible explanation that a gateway must keep at least one owner (GAM-07). The explanation SHALL be surfaced as a tooltip on hover and focus over the disabled role control and Remove access action (the control stays focusable, e.g. aria-disabled, so the reason is announced), consistent with the disabled "Delete gateway" affordance (WEB-UI-03). The same disabled-with-tooltip treatment SHALL apply to owner rows a non-owner caller may not manage (GAM-UI-07). This mirrors the server guarantee rather than replacing it; if the server rejects a mutation with `409`, the console SHALL surface a localized explanation. When more than one owner exists, owner rows SHALL be demotable/removable by owner callers.

#### Scenario: Sole owner controls are disabled

- GIVEN gw-1 has exactly one owner
- WHEN the table renders
- THEN that owner's role control and Remove access action SHALL be disabled with an accessible reason

#### Scenario: Owner rows become actionable once a second owner exists

- GIVEN gw-1 has two owners and the caller is an owner
- WHEN the table renders
- THEN each owner row's role control and Remove access action SHALL be enabled

---

### Requirement: GAM-UI-09 -- Hexagonal Boundary and Domain Probes

Access-management workflows SHALL be expressed as application-owned operations on the existing gateway ports (`GatewayControlPlane` driven port and `GatewayOperations` driving port), named in domain language (for example `listGatewayAccess`, `grantGatewayAccess`, `changeGatewayAccessRole`, `revokeGatewayAccess`, `searchGatewayDirectory`), not as raw RoleBinding CRUD. React, TanStack Query, the generated SDK, and the BFF SHALL remain outside the application core (`standards/ui/hexagonal-architecture.spec.md`).

Every access operation SHALL route through the shared use-case executor so that each invocation emits exactly one started probe and one terminal probe (succeeded, failed, cancelled, denied, or conflicted), and dependency attempts emit their own, through the existing fan-out probe port. New `action` values SHALL be added to the gateway action catalog and kept in agreement with the probe schema. No `console.*` or vendor telemetry SHALL appear in production code (`standards/ui/domain-observability.spec.md`). Correlation and trace context SHALL propagate through the invocation context on every call.

The API adapter SHALL map the access endpoints (GAM-03 through GAM-09) to domain types, mapping transport errors to typed operation errors (including `403` -> denied and `409` -> conflicted), and SHALL rely on the server-joined user fields rather than issuing per-row user lookups.

#### Scenario: An access operation emits one started and one terminal probe

- WHEN a grant operation runs and the server returns `409` (last-owner protection)
- THEN exactly one started probe and one terminal `conflicted` probe SHALL be published
- AND no raw console or vendor telemetry SHALL be emitted

---

### Requirement: GAM-UI-10 -- Server-State Freshness and Localization

The access list SHALL load through TanStack Query keyed by gateway id and the active search/role/pagination parameters, with cancellation via request signals. Mutations (grant, change, revoke) SHALL invalidate the access list on success so the table reflects server state. The query SHALL only run while the tab is active.

All user-visible strings (tab label, column headers, filter labels, **Find people...**, **Add users**, role names and descriptions, creator marker, confirmations, and error/empty/no-results states) SHALL be defined with `defineMessages` and extracted into the web-console locale catalog. User-supplied display names used as fallbacks SHALL be passed through localized presentation helpers, not concatenated into English strings.

#### Scenario: Mutations refresh the list

- GIVEN the access list is displayed
- WHEN a grant, role change, or removal succeeds
- THEN the access list query SHALL be invalidated and the table SHALL reflect the change

---

### Requirement: GAM-UI-11 -- Verification

The gateway management UI package SHALL include unit/component tests (running the use cases against in-memory/fake adapters) covering: table rendering with creator marking, sole-owner disabled controls, and owner rows disabled for non-owner callers; search and role filtering across the three tiers; the directory typeahead including a never-signed-in candidate; the radio role modal grant including the owner-only Owner option; inline role change; remove-access confirmation; viewer read-only presentation; and probe emission (one started + one terminal per operation, including `denied`/`conflicted`).

The production API adapter SHALL have contract tests mapping the access endpoints and their error codes to domain types. Storybook/fixtures SHALL include an access list with two owners (one the creator), an admin, and a user.

#### Scenario: CI exercises last-owner protection and probes in the UI

- WHEN the UI test suite runs
- THEN it SHALL assert the sole owner's controls are disabled
- AND assert that a server `409` on a last-owner mutation yields a localized message and a single terminal `conflicted` probe
