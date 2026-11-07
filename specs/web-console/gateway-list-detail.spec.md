# Gateway List and Detail UI Redesign

**Date:** 2026-11-04
**Status:** Draft
**Applies to:** `packages/gateway-management-ui` (gateway list, detail pages, connection guidance, application shell), `components/web-console` (host routes, BFF, composition root)
**Parent:** `web-console/architecture.spec.md` -- console stack and trust boundaries
**Related:** `web-console/gateway-access-management.spec.md` -- user access tab; `web-console/homepage.spec.md` -- attention continuity from homepage; `platform/openshell-gateway.spec.md` -- gateway provisioning states; `platform/openshell-gateway-service-accounts.spec.md` -- service account lifecycle; `standards/ui/hexagonal-architecture.spec.md`, `standards/ui/patternfly.spec.md`, `standards/ui/domain-observability.spec.md`, `standards/ui/interaction.spec.md`, `standards/ui/content-localization.spec.md`, `standards/ui/accessibility.spec.md`
**Source mockups:** `specs/web-console/mockups/gateways/gateway-list.parity.stories.tsx`, `specs/web-console/mockups/gateways/gateway-detail.parity.stories.tsx`
**Source design:** `specs/web-console/gateway-list-detail-visual-changes.md`, `specs/web-console/gateway-list-detail-behavior-data-changes.md`

---

## Purpose

Define the gateway list and detail page redesign: an improved application shell with persistent navigation, a rebalanced gateway list table with cluster type and state persistence, and a gateway detail experience with an overview card, guided connection setup, and clearer provisioning-state handling for user access and service accounts.

This redesign makes the gateway list easier to scan by rebalancing columns and preserving table state in the URL. It gives the detail page a real overview surface, moves connection guidance to a sequential task-oriented workflow, and makes attention states and provisioning availability clearer. The changes keep PatternFly components and patterns, reduce duplicate or low-value text, and make destructive actions visually distinct from reversible ones.

## Requirements

### Requirement: WEB-GLIS-01 -- Application Shell Navigation

The web console SHALL add persistent primary navigation to the application shell using PatternFly `PageSidebar`, `PageSidebarBody`, and `Nav`. The hamburger `PageToggleButton` SHALL use PatternFly's managed sidebar behavior for narrow viewports. The masthead SHALL remain global and unchanged; page-level actions SHALL NOT move into the sidebar.

The navigation SHALL show three primary destinations:

1. **Overview**: `/` (homepage)
2. **Gateways**: `/gateways` (gateway list)
3. **Dashboard**: `/dashboard` (operational dashboard)

The active destination SHALL be marked with PatternFly's current navigation-item state. The active item SHALL be derived from the current route: gateway list, create, and detail routes SHALL be treated as the **Gateways** section; all dashboard routes SHALL be treated as the **Dashboard** section.

On narrow viewports, the console SHALL use PatternFly's managed sidebar toggle rather than removing navigation.

#### Scenario: WEB-GLIS-01.01 -- Navigation shows three destinations

- GIVEN an authenticated user opens the web console
- WHEN the page renders
- THEN the sidebar SHALL show Overview, Gateways, and Dashboard
- AND the active route SHALL mark the corresponding navigation item as current

#### Scenario: WEB-GLIS-01.02 -- Gateway routes mark Gateways as active

- GIVEN a user navigates to `/gateways`, `/gateways/new`, or `/gateways/:gatewayId`
- WHEN the navigation renders
- THEN the **Gateways** item SHALL be marked as current

#### Scenario: WEB-GLIS-01.03 -- Dashboard routes mark Dashboard as active

- GIVEN a user navigates to `/dashboard` or any dashboard subroute
- WHEN the navigation renders
- THEN the **Dashboard** item SHALL be marked as current

#### Scenario: WEB-GLIS-01.04 -- Narrow viewports use managed sidebar toggle

- GIVEN a user opens the console on a narrow viewport
- WHEN the page renders
- THEN the hamburger toggle SHALL be visible
- AND activating it SHALL show/hide the sidebar using PatternFly's managed behavior

---

### Requirement: WEB-GLIS-02 -- Gateway List Table Layout

The gateway list table SHALL use this column order:

1. **Gateway name**
2. **Sandboxes** (active sandbox count)
3. **Status**
4. **Version**
5. **Connect** (CLI connection command copy action)
6. **Created** (timestamp)
7. **Created by** (creator username)
8. **Cluster type** (derived placement category)
9. **Gateway endpoint** (copy action)
10. **Actions** (row action menu)

The console SHALL remove the raw cluster name column. It SHALL keep the **Version** column because it is operationally important and not redundant with other columns.

The **Sandboxes** column SHALL use each Gateway's `active_sandbox_count`. The definition SHALL be an active agent-sandbox pod in a Running or Pending phase. Terminated pods SHALL NOT count. Orphaned sandboxes (active sandbox resources in namespaces with no live Gateway) SHALL NOT be included in the count.

Column widths SHALL be rebalanced so long values do not crowd out short values. The console SHALL keep pagination at the top and add a bottom pagination control for long lists.

#### Scenario: WEB-GLIS-02.01 -- Table uses the approved column order

- GIVEN the gateway list renders with several gateways
- WHEN the table is inspected
- THEN columns SHALL appear in the order: Gateway name, Sandboxes, Status, Version, Connect, Created, Created by, Cluster type, Gateway endpoint, Actions
- AND the raw cluster name column SHALL NOT be present

#### Scenario: WEB-GLIS-02.02 -- Active sandboxes exclude orphaned pods

- GIVEN a gateway has 3 running agent-sandbox pods and 1 orphaned pod in a deleted gateway namespace
- WHEN the table renders
- THEN the **Sandboxes** column SHALL show 3

#### Scenario: WEB-GLIS-02.03 -- Bottom pagination is available

- GIVEN the gateway list spans multiple pages
- WHEN the table renders
- THEN pagination controls SHALL appear at both the top and bottom

---

### Requirement: WEB-GLIS-03 -- Gateway List Status Display

The **Status** column SHALL use a warning icon with **Attention required** text for unhealthy states so the unhealthy state is not conveyed by text alone. The status SHALL meet WCAG 2.1 Level AA accessibility requirements and SHALL NOT rely solely on color to convey meaning.

#### Scenario: WEB-GLIS-03.01 -- Unhealthy status uses warning icon

- GIVEN a gateway has a degraded health status
- WHEN the table renders
- THEN the **Status** column SHALL show a warning icon beside **Attention required**
- AND the status SHALL be accessible without relying on color alone

---

### Requirement: WEB-GLIS-04 -- Cluster Type Display

The **Cluster type** column SHALL show a derived placement category, not the raw cluster name. The console SHALL support only these display values:

- `IBM Public`
- `AWS VPN`
- `IBM Public Kata`

These values reflect the current cluster categories. The console SHALL NOT model a general or open-ended cluster type.

The cluster type SHALL be derived server-side or in the BFF from placement metadata (provider, region, visibility, runtime). The console SHALL NOT infer cluster type from the cluster display name in the UI. The BFF SHALL return the derived value with the Gateway or Gateway placement response.

The console SHALL NOT add a Cluster type dropdown filter until the list API supports server-side filtering by the derived cluster type. Client-only filtering would be misleading with server-side pagination.

#### Scenario: WEB-GLIS-04.01 -- Cluster type is data-driven

- GIVEN a gateway is deployed on an IBM Cloud public cluster
- WHEN the table renders
- THEN the **Cluster type** column SHALL show `IBM Public`

#### Scenario: WEB-GLIS-04.02 -- Cluster type distinguishes Kata runtime

- GIVEN a gateway is deployed on an IBM cluster using the Kata runtime
- WHEN the table renders
- THEN the **Cluster type** column SHALL show `IBM Public Kata`

#### Scenario: WEB-GLIS-04.03 -- Cluster type is not inferred from cluster name

- GIVEN the server returns cluster type with the Gateway response
- WHEN the table renders
- THEN the **Cluster type** SHALL match the server value
- AND the console SHALL NOT parse or infer the type from the cluster display name

---

### Requirement: WEB-GLIS-05 -- Gateway List Copy Actions

The **Gateway endpoint** column SHALL show a copy action instead of printing the long gateway endpoint text in the table. The console SHALL use the standard PatternFly copy icon. Activating the copy action SHALL copy the exact gateway endpoint.

The **Connect** column SHALL show a code icon that opens a command modal with the CLI connection command. The modal SHALL use the approved Connection-tab `CommandBlock` treatment with its own copy action. The command SHALL include the gateway name and endpoint.

The two actions SHALL be separated in the column order so users do not see two identical icons side by side without context. Each action SHALL have a distinct visible tooltip and screen-reader label, such as **Copy endpoint for payments-gateway** and **View connection command for payments-gateway**.

#### Scenario: WEB-GLIS-05.01 -- Gateway endpoint copy action

- GIVEN a gateway named `payments-gateway` with endpoint `https://payments.example.com`
- WHEN the user activates the endpoint copy action
- THEN the exact endpoint `https://payments.example.com` SHALL be copied to the clipboard
- AND the tooltip SHALL read **Copy endpoint for payments-gateway**

#### Scenario: WEB-GLIS-05.02 -- CLI connection command modal

- GIVEN a gateway named `payments-gateway`
- WHEN the user activates the **Connect** code icon
- THEN a modal SHALL open with the full CLI connection command
- AND the modal SHALL use the approved `CommandBlock` component with a copy action
- AND the command SHALL include the gateway name and endpoint

#### Scenario: WEB-GLIS-05.03 -- Copy actions have distinct labels

- GIVEN the table renders
- WHEN the user hovers over or focuses on the endpoint and connection copy actions
- THEN each SHALL show a distinct tooltip identifying the gateway and action

---

### Requirement: WEB-GLIS-06 -- Gateway List Table State Persistence

The gateway list SHALL store search, sort column, sort direction, page, and page size in the URL using query parameters. The URL SHALL be the source of truth when a user loads, reloads, bookmarks, or shares the view. Query parameters SHALL be removed when they match the default state so common URLs stay readable.

The console SHALL apply these defaults when an unknown or invalid parameter is present:

- **Sort**: Created
- **Direction**: descending (newest first)
- **Page**: 1
- **Page size**: 20

The console SHALL use the dashboard-style subtle **Last refreshed** indicator rather than a prominent refresh treatment. The console SHALL keep the existing refresh affordance on the real page.

#### Scenario: WEB-GLIS-06.01 -- Table state persists in URL

- GIVEN a user filters by search term `team`, sorts by Created descending, and views page 2
- WHEN the user bookmarks or shares the URL
- THEN the URL SHALL contain the search, sort, direction, and page parameters
- AND reloading SHALL restore the filtered and sorted view

#### Scenario: WEB-GLIS-06.02 -- Default parameters are omitted

- GIVEN the user clears all filters and uses default sort and page size
- WHEN the URL is inspected
- THEN query parameters matching defaults SHALL be omitted for readability

#### Scenario: WEB-GLIS-06.03 -- Invalid parameters fall back to defaults

- GIVEN the URL contains an invalid sort column or page number
- WHEN the page renders
- THEN the console SHALL apply the default sort (Created descending) and page (1)

---

### Requirement: WEB-GLIS-07 -- Gateway Detail Page and Tab Structure

The gateway detail page SHALL show these tabs in order:

1. **Details** (default)
2. **Connection**
3. **User access**
4. **Service accounts**

The **Details** tab SHALL be the first and default tab. Tab navigation SHALL use the existing `?tab=` URL routing. The tab bar and tab content SHALL use white backgrounds. The console SHALL remove the divider between the page header and tabs, using white space to separate the regions and reduce horizontal gray lines.

Each tab SHALL have a consistent heading, description, and plain refresh button. The refresh button SHALL represent the active tab's data; it SHALL NOT refresh every tab at once.

The provisioning timeline SHALL be shown only while the gateway is provisioning. It SHALL be hidden after the gateway is successfully provisioned and running without an error. The timeline visibility SHALL follow the rules in WEB-GLIS-08.

#### Scenario: WEB-GLIS-07.01 -- Details is the default tab

- GIVEN a user navigates to a gateway detail page without a `?tab=` parameter
- WHEN the page renders
- THEN the **Details** tab SHALL be active
- AND the overview card SHALL be visible

#### Scenario: WEB-GLIS-07.02 -- Tabs have consistent structure

- GIVEN a user opens any gateway detail tab
- WHEN the tab content renders
- THEN it SHALL show a heading, description, and refresh button
- AND the refresh button SHALL only refresh that tab's data

#### Scenario: WEB-GLIS-07.03 -- Provisioning timeline is hidden after success

- GIVEN a gateway is successfully provisioned and running
- WHEN the user opens the detail page
- THEN the provisioning timeline SHALL NOT be visible
- AND the Details tab SHALL be the default landing

---

### Requirement: WEB-GLIS-08 -- Provisioning Timeline Continuity

After a user creates a gateway, the console SHALL land on that gateway's **Connection** tab (not Details) and show the existing provisioning progress timeline with its current behavior.

When the user later opens the gateway detail page (after navigating away and returning), the console SHALL land on the default **Details** tab. On that later entry, the console SHALL show the provisioning timeline only if the installation is still in progress or ended with an error. If the Gateway is already successfully installed at page load, the console SHALL NOT show the timeline.

Once an installation timeline is shown during a visit, it SHALL remain visible across tab navigation and after installation succeeds. The completed timeline SHALL remain available until the user leaves that gateway and returns, which SHALL start the entry rule above again.

#### Scenario: WEB-GLIS-08.01 -- Fresh creation lands on Connection tab with timeline

- GIVEN a user creates a new gateway
- WHEN the creation succeeds
- THEN the console SHALL navigate to the gateway's Connection tab
- AND the provisioning timeline SHALL be visible

#### Scenario: WEB-GLIS-08.02 -- Later entry to running gateway hides timeline

- GIVEN a gateway is successfully provisioned and running
- WHEN a user navigates to the detail page from the list
- THEN the console SHALL land on the Details tab
- AND the provisioning timeline SHALL NOT be visible

#### Scenario: WEB-GLIS-08.03 -- Timeline persists during a visit

- GIVEN a user is viewing a gateway with the timeline visible (provisioning in progress)
- WHEN the user switches tabs
- THEN the timeline SHALL remain visible
- AND it SHALL remain visible even after provisioning completes during the visit

#### Scenario: WEB-GLIS-08.04 -- Timeline is shown if provisioning is still in progress

- GIVEN a gateway is still provisioning
- WHEN a user opens the detail page
- THEN the timeline SHALL be visible on the Details tab

---

### Requirement: WEB-GLIS-09 -- Gateway Detail Overview Card

The **Details** tab SHALL show an **Overview** card as the primary landing card. The card SHALL include these fields:

- **Status**
- **Active sandboxes**
- **Cluster type**
- **Version**
- **Created** (timestamp)
- **Created by** (creator username)
- **Gateway endpoint** (copy control)

**Cluster type** SHALL appear below **Active sandboxes**. **Created by** SHALL appear below **Created**. The console SHALL use a longer gateway endpoint copy control so the endpoint is less likely to be visually truncated. The **Open gateway console** button SHALL be bottom-aligned with the endpoint copy control to avoid odd vertical centering against the label and control.

The card SHALL use PatternFly vertical description lists, with the label above the value. Card titles SHALL be rendered in `CardHeader` and `CardTitle` using `Title headingLevel="h2" size="xl"` to match PatternFly dashboard examples.

#### Scenario: WEB-GLIS-09.01 -- Overview card shows required fields

- GIVEN a gateway detail page is loaded
- WHEN the Details tab renders
- THEN the Overview card SHALL show Status, Active sandboxes, Cluster type, Version, Created, Created by, and Gateway endpoint
- AND fields SHALL be arranged in the approved order

#### Scenario: WEB-GLIS-09.02 -- Gateway endpoint is not visually truncated

- GIVEN a gateway with a long endpoint URL
- WHEN the Overview card renders
- THEN the endpoint copy control SHALL be wide enough to show the full URL without truncation where practical

#### Scenario: WEB-GLIS-09.03 -- Open gateway console button is bottom-aligned

- GIVEN the Overview card renders
- WHEN the layout is inspected
- THEN the **Open gateway console** button SHALL be bottom-aligned with the endpoint copy control

---

### Requirement: WEB-GLIS-10 -- Gateway Detail Optional Fields

The **Overview** and **Connection** cards SHALL show **Region** only when the Gateway placement has a non-empty region. They SHALL show **Runtime** only when the Gateway uses a distinct runtime, such as Kata. Both rows SHALL be hidden when the data is absent. An absent optional value is a normal state, not a system failure; the console SHALL NOT show an ambiguous **Not available** state.

#### Scenario: WEB-GLIS-10.01 -- Region is shown when present

- GIVEN a gateway is deployed in `us-east-1`
- WHEN the card renders
- THEN the **Region** row SHALL show `us-east-1`

#### Scenario: WEB-GLIS-10.02 -- Region is hidden when absent

- GIVEN a gateway has no region value
- WHEN the card renders
- THEN the **Region** row SHALL NOT be rendered

#### Scenario: WEB-GLIS-10.03 -- Runtime is shown for Kata

- GIVEN a gateway uses the Kata runtime
- WHEN the card renders
- THEN the **Runtime** row SHALL show `Kata`

#### Scenario: WEB-GLIS-10.04 -- Runtime is hidden when not distinct

- GIVEN a gateway uses the default runtime
- WHEN the card renders
- THEN the **Runtime** row SHALL NOT be rendered

---

### Requirement: WEB-GLIS-11 -- Connection Tab Guided Setup

The **Connection** tab SHALL lead with the user outcome: connect a terminal to this gateway and start a Claude sandbox. The console SHALL mention that most setup can also be completed in the gateway console.

The console SHALL replace the wall of terminal commands with a bordered PatternFly accordion. Only one command SHALL be expanded at a time. Each step heading SHALL have a token-neutral outlined circular number. The console SHALL number the steps and mark only one-time steps with a compact outlined label; it SHALL leave every-time steps unmarked:

1. **Install the OpenShell CLI** (one-time)
2. **Add this gateway** (one-time)
3. **Download the Claude connection template** (one-time)
4. **Import the Claude connection template** (one-time)
5. **Connect this gateway to Claude**
6. **Create a Claude sandbox**
7. **Open the sandbox in your editor**

The console SHALL explain each command in user-focused language:

- A connection template is a small configuration file that tells OpenShell how to create a Google Vertex AI connection.
- Importing the template adds that configuration to the gateway.
- Connecting to Claude requires setting the `ANTHROPIC_VERTEX_PROJECT_ID` environment variable and creates a named gateway connection using existing Google credentials; the user does not paste an API key.
- Model selection happens when the sandbox launches Claude; the console SHALL NOT show a separate inference command.

Each command SHALL use the approved multiline `CommandBlock` with its own copy action. The console SHALL put compact PatternFill form controls above commands that have customizable values:

- **Provider name**
- **Sandbox name**
- **Claude model**

Commands SHALL be read-only. The console SHALL regenerate the displayed command from the control values so the copy button and manual text selection both copy exactly what is shown. The console SHALL link the final step to **OpenShell connection documentation**. It SHALL add **Copy all setup commands** for experienced users who want the full sequence at once.

The console SHALL keep Claude as the single example for this iteration.

#### Scenario: WEB-GLIS-11.01 -- Connection tab shows seven setup steps

- GIVEN a user opens the Connection tab
- WHEN the page renders
- THEN seven setup steps SHALL be shown in an accordion
- AND steps 1-4 SHALL be marked as one-time

#### Scenario: WEB-GLIS-11.02 -- Only one step expands at a time

- GIVEN the user expands step 5
- WHEN step 6 is expanded
- THEN step 5 SHALL collapse
- AND only step 6 SHALL be expanded

#### Scenario: WEB-GLIS-11.03 -- Customizable commands update on input

- GIVEN the provider name control is set to `my-provider`
- WHEN the sandbox creation command is rendered
- THEN it SHALL include `--provider my-provider`
- AND both the copy button and text selection SHALL copy the updated command

#### Scenario: WEB-GLIS-11.04 -- Copy all setup commands includes full sequence

- GIVEN the user activates **Copy all setup commands**
- WHEN the copy action completes
- THEN all seven commands SHALL be copied in sequence

#### Scenario: WEB-GLIS-11.05 -- User-facing explanations are clear

- GIVEN the Connection tab renders
- WHEN the user reads the connection template step
- THEN it SHALL explain that a connection template tells the CLI how to create a Google Vertex AI connection
- AND it SHALL NOT require prior knowledge of providers or profiles

---

### Requirement: WEB-GLIS-12 -- Connection Tab Automation Entry Point

The console SHALL move the service-account link out of the top of the Connection flow. It SHALL add a quiet **Automate this gateway** card at the bottom for CI/CD pipelines and bots. The card SHALL link to the **Service accounts** tab.

#### Scenario: WEB-GLIS-12.01 -- Automation card links to Service accounts

- GIVEN the Connection tab is rendered
- WHEN the user scrolls to the bottom
- THEN an **Automate this gateway** card SHALL be visible
- AND it SHALL link to the Service accounts tab

---

### Requirement: WEB-GLIS-13 -- Connection Tab Card Layout

The **Connection** tab SHALL group details into **Overview**, **Identity**, and **Placement** cards. It SHALL use a two-column card grid on large screens. On smaller screens, the second column SHALL stack below the first. **Placement** SHALL stack below **Identity** on narrow screens instead of compressing both cards.

Card titles SHALL be in `CardHeader` and `CardTitle`. They SHALL be rendered with `Title headingLevel="h2" size="xl"` to match PatternFly dashboard examples.

#### Scenario: WEB-GLIS-13.01 -- Large screens use two-column grid

- GIVEN the Connection tab renders on a large screen
- WHEN the layout is inspected
- THEN the cards SHALL be arranged in two columns

#### Scenario: WEB-GLIS-13.02 -- Narrow screens stack cards

- GIVEN the Connection tab renders on a narrow screen
- WHEN the layout is inspected
- THEN the second column SHALL stack below the first
- AND Placement SHALL stack below Identity without compression

---

### Requirement: WEB-GLIS-14 -- Attention and Lifecycle Alerts

The **Details** tab SHALL replace the weak **Related activity** card with alerts that only appear when action is needed. The console SHALL use warning styling for:

- Gateway health is degraded
- Sandboxes expire soon
- Sandboxes are idle
- Service accounts expire soon

The console SHALL continue the attention story from the homepage into the gateway detail page. The user SHALL NOT have to remember why they navigated here. The console SHALL show service-account expiration alerts on both the **Overview** (Details tab) and **Service accounts** tabs.

For sandbox expiration and idle alerts, the console SHALL use **View all sandboxes** as the primary action. The action SHALL be role-aware:

- Gateway owners and admins mapped to `openshell-admin` SHALL link to `/sandboxes`.
- Gateway users mapped to `openshell-user` SHALL link to `/workspaces` or a user-scoped sandbox surface.

The action label SHALL match the destination, such as **View all sandboxes** or **View workspaces**.

The console SHALL keep orphaned sandboxes out of gateway detail. Orphaned sandboxes have no owning gateway, so the operational dashboard is the correct surface.

#### Scenario: WEB-GLIS-14.01 -- Degraded health shows alert

- GIVEN a gateway has degraded connection health
- WHEN the Details tab renders
- THEN a warning alert SHALL show on the Overview
- AND it SHALL include the affected active-sandbox count when known

#### Scenario: WEB-GLIS-14.02 -- Service account expiration on Overview and Service accounts tabs

- GIVEN a gateway has a service account expiring within 7 days
- WHEN the user views the Details tab
- THEN a warning alert SHALL be shown on the Overview
- AND the same alert SHALL appear on the Service accounts tab
- AND it SHALL link to the Service accounts tab for rotation

#### Scenario: WEB-GLIS-14.03 -- Sandbox attention is role-aware

- GIVEN a gateway owner has expiring sandboxes
- WHEN the alert renders
- THEN the action SHALL read **View all sandboxes** and link to `/sandboxes`

- GIVEN a gateway user has idle sandboxes
- WHEN the alert renders
- THEN the action SHALL read **View workspaces** and link to `/workspaces`

#### Scenario: WEB-GLIS-14.04 -- Attention continuity from homepage

- GIVEN the homepage links to a gateway with `?attention=service-account-expiring`
- WHEN the gateway detail page loads
- THEN the appropriate service-account expiration alert SHALL be shown
- AND the URL context SHALL remain bookmarkable

#### Scenario: WEB-GLIS-14.05 -- Orphaned sandboxes are not shown

- GIVEN the operational dashboard reports orphaned sandboxes
- WHEN the gateway detail page renders
- THEN no orphaned sandbox alert SHALL appear
- AND orphaned sandboxes SHALL NOT be included in the active sandbox count

---

### Requirement: WEB-GLIS-15 -- User Access Tab During Provisioning

The **User access** tab SHALL remain available while a gateway is provisioning. The console SHALL show a standard inline info alert with:

- **Title**: **User access can be granted now**
- **Body**: **People you add will be able to sign in once gateway provisioning completes.**

User access is a HyperShell management-plane operation backed by RoleBindings and the Keycloak directory. It does not require the OpenShell Gateway to be healthy. The creator's `gateway:owner` RoleBinding is created in the same transaction as the Gateway. Newly added users can be granted now, but they cannot sign in to OpenShell until Gateway provisioning completes.

The **User access** tab SHALL keep search, role filtering, sorting, and pagination server-side. The console SHALL add bottom pagination to the table. Refreshing SHALL only refresh the active User access query, not other tabs.

#### Scenario: WEB-GLIS-15.01 -- User access available during provisioning

- GIVEN a gateway is currently provisioning
- WHEN a user opens the User access tab
- THEN the tab SHALL be accessible
- AND an info alert SHALL explain that users can be granted now but cannot sign in until provisioning completes

#### Scenario: WEB-GLIS-15.02 -- Add user succeeds while provisioning

- GIVEN a gateway is provisioning
- WHEN an owner adds a user and assigns a role
- THEN the grant SHALL succeed
- AND the user SHALL appear in the access list

#### Scenario: WEB-GLIS-15.03 -- Bottom pagination is shown

- GIVEN the User access list spans multiple pages
- WHEN the table renders
- THEN pagination controls SHALL appear at the bottom

---

### Requirement: WEB-GLIS-16 -- User Access Tab Naming and Layout

The **User access** tab SHALL rename **Manage access** to **User access**. It SHALL keep **Service accounts** as a separate tab because they also control access, but they are a different object and workflow. **User access** SHALL be used for people selected from the LDAP-backed Keycloak directory.

The console SHALL separate filter controls from the **Add user** action with PatternFly toolbar groups. This makes the action look like a table-level action rather than part of the search and role filter. The table's text filter SHALL be labeled **Filter by name or user ID**. The console SHALL keep **Find people** only in the Add user directory-search dialog.

The toolbar action and modal title SHALL be renamed to **Add user** because the UI grants one user at a time. In the Add user modal, the console SHALL replace the visible field label with normal-weight guidance: **Search the identity directory for a user to add to the gateway.** It SHALL keep **Find people** as the input's accessible name and placeholder.

The console SHALL add a **Need automation access?** card after the table. The card SHALL explain that CI/CD pipelines and bots should use service accounts and link to the Service accounts tab.

The table SHALL add sorting to **Name**, **User ID**, **Role**, and **Granted**. It SHALL add a **Granted** column showing the timestamp when access was granted. The table SHALL add bottom pagination.

The console SHALL use the same code icon as Service accounts for the **User workspace command**. The icon SHALL open a **Grant workspace access** modal with one copyable command.

#### Scenario: WEB-GLIS-16.01 -- Tab is named User access

- GIVEN the gateway detail page renders
- WHEN the tabs are inspected
- THEN the tab SHALL be labeled **User access**, not **Manage access**

#### Scenario: WEB-GLIS-16.02 -- Toolbar groups separate filter and action

- GIVEN the User access tab renders
- WHEN the toolbar is inspected
- THEN filter controls SHALL be in one toolbar group
- AND the **Add user** action SHALL be in a separate group

#### Scenario: WEB-GLIS-16.03 -- Add user modal has clear guidance

- GIVEN an owner opens the **Add user** modal
- WHEN the modal renders
- THEN guidance SHALL read **Search the identity directory for a user to add to the gateway.**
- AND **Find people** SHALL be the input's accessible name and placeholder

#### Scenario: WEB-GLIS-16.04 -- Need automation access card links to Service accounts

- GIVEN the User access tab renders
- WHEN the user scrolls below the table
- THEN a **Need automation access?** card SHALL be visible
- AND it SHALL link to the Service accounts tab

#### Scenario: WEB-GLIS-16.05 -- Table columns are sortable

- GIVEN the User access table renders
- WHEN the user inspects column headers
- THEN Name, User ID, Role, and Granted SHALL be sortable

#### Scenario: WEB-GLIS-16.06 -- Granted column shows timestamp

- GIVEN the User access table renders with a user granted access yesterday
- WHEN the table renders
- THEN the **Granted** column SHALL show the grant timestamp

#### Scenario: WEB-GLIS-16.07 -- User workspace command opens modal

- GIVEN a user row with the User role
- WHEN the code icon is activated
- THEN a **Grant workspace access** modal SHALL open
- AND it SHALL show one copyable workspace membership command

---

### Requirement: WEB-GLIS-17 -- Service Accounts Tab During Provisioning

The **Service accounts** tab SHALL remain visible while a gateway is provisioning. The console SHALL show an inline info alert explaining that service accounts can be created after provisioning completes. It SHALL disable the **Create service account** action while the gateway is provisioning.

Service-account creation requires the Gateway phase to be `Running`, status to be `Healthy`, and its OIDC issuer, client ID, and audience to be present. The create API returns `409 gateway_not_ready` while the Gateway is provisioning. The list and capabilities query do not themselves require a ready Gateway.

After the Gateway becomes ready, the console SHALL allow creation and role assignment in the create dialog. The console SHALL hide the completed provisioning timeline when the Gateway is running without an error.

#### Scenario: WEB-GLIS-17.01 -- Service accounts tab visible during provisioning

- GIVEN a gateway is currently provisioning
- WHEN a user opens the Service accounts tab
- THEN the tab SHALL be accessible
- AND an info alert SHALL explain that service accounts can be created after provisioning completes

#### Scenario: WEB-GLIS-17.02 -- Create service account disabled while provisioning

- GIVEN a gateway is provisioning
- WHEN the Service accounts tab renders
- THEN the **Create service account** action SHALL be disabled

#### Scenario: WEB-GLIS-17.03 -- Create enabled after provisioning completes

- GIVEN a gateway was provisioning and is now running and healthy
- WHEN the Service accounts tab renders
- THEN the **Create service account** action SHALL be enabled
- AND creation and role assignment SHALL succeed

---

### Requirement: WEB-GLIS-18 -- Service Accounts Tab Table Layout

The Service accounts table SHALL keep **Name** as the default sort. It SHALL keep pagination at the top and add a bottom pagination control. The console SHALL replace the nested expandable section with the official PatternFly expandable table row. Expanded account details SHALL be shown in a details row spanning the visible table columns.

The console SHALL rename **Service account subject** to **Principal subject** to match the OpenShell console. It SHALL label the role column **OpenShell role** to distinguish it from HyperShell gateway roles used on User access.

The console SHALL show the account's workspaces in the expanded row. This is future-state data because the current service-account API does not return workspace usage. To show workspace usage in production, the BFF SHALL add a joined service-account API field or an OpenShell integration that returns workspace memberships.

The console SHALL add a **Setup** column between **Service account name** and **OpenShell role**. Each row SHALL use a code icon that opens that account's setup instructions. The icon SHALL have a tooltip and screen-reader label such as **Open setup instructions for ci-pipeline**.

#### Scenario: WEB-GLIS-18.01 -- Table uses PatternFly expandable rows

- GIVEN a service account row is collapsed
- WHEN the user expands it
- THEN a details row SHALL span the table columns
- AND it SHALL show the principal subject and workspace usage

#### Scenario: WEB-GLIS-18.02 -- Principal subject replaces Service account subject

- GIVEN the expanded row renders
- WHEN the details are inspected
- THEN the field SHALL be labeled **Principal subject**, not **Service account subject**

#### Scenario: WEB-GLIS-18.03 -- Role column is labeled OpenShell role

- GIVEN the Service accounts table renders
- WHEN the column headers are inspected
- THEN the role column SHALL be labeled **OpenShell role**

#### Scenario: WEB-GLIS-18.04 -- Setup column opens setup instructions

- GIVEN a service account named `ci-pipeline`
- WHEN the user activates the **Setup** code icon
- THEN a modal SHALL open with setup instructions for `ci-pipeline`
- AND the tooltip SHALL read **Open setup instructions for ci-pipeline**

#### Scenario: WEB-GLIS-18.05 -- Bottom pagination is available

- GIVEN the service account list spans multiple pages
- WHEN the table renders
- THEN pagination controls SHALL appear at both the top and bottom

---

### Requirement: WEB-GLIS-19 -- Service Account Setup Modal

The Service accounts setup modal SHALL use one concise warning for the one-time client secret. It SHALL remove the duplicate workspace info alert. It SHALL present setup in a clear order:

1. **Save the client ID, client secret, and principal subject**
2. **Grant workspace access** (when the account uses the `openshell-user` role)
3. **Request an access token**

The console SHALL use the same code icon and command-modal pattern as User access. It SHALL use the approved multiline `CommandBlock` treatment for workspace-access and access-token commands.

For `openshell-user`, the console SHALL explain that the service account cannot use the gateway until its principal subject is added to a workspace. It SHALL offer the gateway console or terminal command, and SHALL default `WORKSPACE_NAME` to `default`.

The console SHALL collapse **Request an access token** by default because it is an optional automation step. It SHALL explain that the JWT is a short-lived credential used by automation while the client secret remains protected in the caller's secret manager.

For an existing service account, the console SHALL show setup instructions without the one-time client secret. It SHALL show the secret only in the setup modal that appears immediately after account creation. The console SHALL remove the **I saved the client secret** acknowledgement checkbox. It SHALL replace **Finish setup** with **Close**. The modal only presents setup information; it does not verify that the user saved the secret.

#### Scenario: WEB-GLIS-19.01 -- Setup modal shows three sections

- GIVEN a user creates a service account with the `openshell-user` role
- WHEN the setup modal opens
- THEN it SHALL show sections for saving credentials, granting workspace access, and requesting an access token
- AND the workspace section SHALL be visible because the role is `openshell-user`

#### Scenario: WEB-GLIS-19.02 -- Admin role skips workspace section

- GIVEN a user creates a service account with the `openshell-admin` role
- WHEN the setup modal opens
- THEN it SHALL show sections for saving credentials and requesting an access token
- AND the workspace section SHALL NOT be shown because `openshell-admin` bypasses workspace membership

#### Scenario: WEB-GLIS-19.03 -- Request access token is collapsed by default

- GIVEN the setup modal opens
- WHEN the user views the modal
- THEN **Request an access token** SHALL be collapsed
- AND it SHALL explain that the JWT is short-lived

#### Scenario: WEB-GLIS-19.04 -- Existing account hides client secret

- GIVEN a user opens setup instructions for an existing service account
- WHEN the modal renders
- THEN the client secret SHALL NOT be shown
- AND the modal SHALL show client ID, principal subject, workspace command (if applicable), and access token command

#### Scenario: WEB-GLIS-19.05 -- Fresh creation shows one-time secret

- GIVEN a user just created a service account
- WHEN the setup modal opens
- THEN the client secret SHALL be shown with a warning
- AND the warning SHALL explain it cannot be shown again

#### Scenario: WEB-GLIS-19.06 -- Modal uses Close instead of Finish setup

- GIVEN the setup modal renders
- WHEN the user inspects the actions
- THEN the primary action SHALL be **Close**, not **Finish setup**
- AND no acknowledgement checkbox SHALL be present

#### Scenario: WEB-GLIS-19.07 -- Workspace command defaults to default workspace

- GIVEN the setup modal shows a workspace membership command
- WHEN the command is rendered
- THEN it SHALL use `WORKSPACE_NAME=default` as the default

---

### Requirement: WEB-GLIS-20 -- Dialog Treatments

The console SHALL use danger styling and warning icons for destructive, irreversible actions. It SHALL use primary confirmation buttons without danger styling or warning icons for reversible actions.

**Destructive, irreversible actions** (danger styling + warning icon):

- **Delete gateway**: danger button, warning icon beside modal title
- **Revoke service account**: danger button, warning icon in title
- **Delete service account**: danger button, warning icon in title

**Reversible actions** (primary button, no danger styling or warning icon):

- **Remove access** (user access): primary button, no warning icon
- **Change your own role**: primary button, no warning icon (consequential but reversible by another owner)

The console SHALL block changing or removing the final owner before a confirmation modal is needed, with an explanatory disabled state. The **Edit service account** modal SHALL remove the distracting client-credential note, shorten the role-change alert, and add spacing between the alert and description field.

#### Scenario: WEB-GLIS-20.01 -- Delete gateway uses danger styling

- GIVEN a user opens the **Delete gateway** dialog
- WHEN the modal renders
- THEN the confirm button SHALL use danger styling
- AND a warning icon SHALL appear beside the modal title

#### Scenario: WEB-GLIS-20.02 -- Revoke service account uses danger styling

- GIVEN a user opens the **Revoke service account** dialog
- WHEN the modal renders
- THEN the confirm button SHALL use danger styling
- AND a warning icon SHALL appear in the title

#### Scenario: WEB-GLIS-20.03 -- Delete service account uses danger styling

- GIVEN a user opens the **Delete service account** dialog
- WHEN the modal renders
- THEN the confirm button SHALL use danger styling
- AND a warning icon SHALL appear in the title

#### Scenario: WEB-GLIS-20.04 -- Remove access uses primary styling

- GIVEN a user opens the **Remove access** dialog for a user
- WHEN the modal renders
- THEN the confirm button SHALL use primary styling
- AND no warning icon SHALL appear

#### Scenario: WEB-GLIS-20.05 -- Change your own role uses primary styling

- GIVEN an owner changes their own role and a confirmation modal appears
- WHEN the modal renders
- THEN the confirm button SHALL use primary styling
- AND no warning icon SHALL appear

#### Scenario: WEB-GLIS-20.06 -- Final owner controls are disabled

- GIVEN a gateway has exactly one owner
- WHEN the owner views their own access row
- THEN the role control and Remove access action SHALL be disabled
- AND an accessible explanation SHALL state that a gateway must keep at least one owner

---

### Requirement: WEB-GLIS-21 -- Tab Loading and Refresh

Tab panels SHALL mount on entry and unmount on exit. **User access** and **Service accounts** SHALL fetch only while their tab is active. Each list query SHALL have a 5-second stale time. Returning to a tab SHALL show cached data and refetch automatically only if the cache is stale.

Each tab's refresh button SHALL scope its refresh to that tab's query:

- **Details** and **Connection**: Gateway details
- **User access**: access list
- **Service accounts**: service-account list

The console SHALL keep the user informed with the dashboard-style last-refreshed treatment where the page already supports it.

#### Scenario: WEB-GLIS-21.01 -- Tabs mount on entry

- GIVEN a user opens the Service accounts tab for the first time
- WHEN the tab activates
- THEN the tab panel SHALL mount
- AND the service-account query SHALL be initiated

#### Scenario: WEB-GLIS-21.02 -- Tabs unmount on exit

- GIVEN a user is viewing the User access tab
- WHEN the user switches to the Details tab
- THEN the User access tab panel SHALL unmount

#### Scenario: WEB-GLIS-21.03 -- Refresh button is scoped

- GIVEN a user is viewing the Service accounts tab
- WHEN the user activates the refresh button
- THEN only the service-account query SHALL be refetched
- AND the gateway details query SHALL NOT be refetched

#### Scenario: WEB-GLIS-21.04 -- Stale cache triggers refetch

- GIVEN a user viewed the User access tab 10 seconds ago
- WHEN the user returns to the tab
- THEN cached data SHALL be shown immediately
- AND a background refetch SHALL occur because the cache is stale

---

### Requirement: WEB-GLIS-22 -- Hexagonal Boundary and Domain Probes

All gateway list and detail workflows SHALL be expressed as application-owned operations on the existing gateway ports (`GatewayControlPlane` driven port and `GatewayOperations` driving port), named in domain language. React, TanStack Query, the generated SDK, and the BFF SHALL remain outside the application core (`standards/ui/hexagonal-architecture.spec.md`).

Every operation SHALL route through the shared use-case executor so that each invocation emits exactly one started probe and one terminal probe (succeeded, failed, cancelled, denied, or conflicted), and dependency attempts emit their own, through the existing fan-out probe port. New `action` values SHALL be added to the gateway action catalog and kept in agreement with the probe schema. No `console.*` or vendor telemetry SHALL appear in production code (`standards/ui/domain-observability.spec.md`). Correlation and trace context SHALL propagate through the invocation context on every call.

The API adapter SHALL map the gateway endpoints to domain types, mapping transport errors to typed operation errors (including `403` -> denied and `409` -> conflicted).

#### Scenario: WEB-GLIS-22.01 -- Gateway operations emit started and terminal probes

- WHEN a gateway list query runs and succeeds
- THEN exactly one started probe and one terminal `succeeded` probe SHALL be published
- AND no raw console or vendor telemetry SHALL be emitted

#### Scenario: WEB-GLIS-22.02 -- Operations use domain language

- GIVEN the gateway list and detail workflows
- WHEN the application is inspected
- THEN operations SHALL be named in domain language (e.g., `listGateways`, `getGatewayDetails`, `copyGatewayEndpoint`)
- AND React, TanStack Query, and SDK SHALL remain outside the application core

---

### Requirement: WEB-GLIS-23 -- Server-State Freshness and Localization

The gateway list and detail queries SHALL load through TanStack Query keyed by gateway id and active parameters (search, role, pagination, tab), with cancellation via request signals. Mutations SHALL invalidate affected queries on success so tables reflect server state. Queries SHALL only run while their tab or page is active.

All user-visible strings (tab labels, column headers, filter labels, button text, alert titles and messages, confirmations, and error/empty/no-results states) SHALL be defined with `defineMessages` and extracted into the web-console locale catalog. User-supplied display names used as fallbacks SHALL be passed through localized presentation helpers, not concatenated into English strings.

#### Scenario: WEB-GLIS-23.01 -- Mutations refresh affected queries

- GIVEN the gateway list is displayed
- WHEN a gateway is created or deleted
- THEN the gateway list query SHALL be invalidated
- AND the table SHALL reflect the change

#### Scenario: WEB-GLIS-23.02 -- Queries are keyed by active parameters

- GIVEN the gateway list is filtered by search term `team` on page 2
- WHEN the query key is inspected
- THEN it SHALL include the search term and page number

#### Scenario: WEB-GLIS-23.03 -- All strings are localized

- GIVEN the gateway list and detail pages render
- WHEN the code is inspected
- THEN all user-visible strings SHALL be defined with `defineMessages`
- AND no hard-coded English strings SHALL be concatenated with user data

---

### Requirement: WEB-GLIS-24 -- Polling Behavior

Gateway detail polling SHALL run conditionally: it SHALL run while the Gateway still needs status updates and stop when that is no longer the case. Polling SHALL be disabled when the browser tab is in the background.

The console SHOULD increase the status poll interval to 15-30 seconds, or replace full-record polling with a lightweight status endpoint or server-sent events. The console SHALL keep background-tab pausing. It SHALL NOT refresh every detail tab on a header or gateway status poll. Each tab SHALL refresh only its own query.

#### Scenario: WEB-GLIS-24.01 -- Polling stops when gateway is stable

- GIVEN a gateway is successfully provisioned and healthy
- WHEN polling is evaluated
- THEN status polling SHALL stop

#### Scenario: WEB-GLIS-24.02 -- Polling is disabled in background tabs

- GIVEN the browser tab is in the background
- WHEN polling is evaluated
- THEN no polling requests SHALL be issued

#### Scenario: WEB-GLIS-24.03 -- Status poll does not refresh all tabs

- GIVEN the Details tab is active
- WHEN a status poll completes
- THEN only the Details tab query SHALL be refreshed
- AND the User access and Service accounts queries SHALL NOT be refetched

---

### Requirement: WEB-GLIS-25 -- Verification

The gateway management UI package SHALL include unit/component tests (running the use cases against in-memory/fake adapters) covering:

- Application shell navigation rendering and active state
- Gateway list table column order, sorting, pagination, and state persistence
- Cluster type display and data derivation
- Copy actions for gateway endpoint and CLI connection command
- Gateway detail tab structure and default tab
- Provisioning timeline visibility rules
- Overview card field rendering and optional fields
- Connection tab accordion, customizable commands, and copy-all action
- User access tab availability during provisioning, filter and action separation, and workspace command modal
- Service accounts tab availability during provisioning, expandable rows, and setup modal
- Attention alerts and role-aware sandbox links
- Dialog treatments (danger vs. primary styling)
- Tab loading, refresh scoping, and stale cache behavior
- Probe emission (one started + one terminal per operation)

The production API adapter SHALL have contract tests mapping the gateway endpoints and their error codes to domain types. Storybook/fixtures SHALL include gateway list and detail examples with all states (provisioning, running, degraded, attention).

The production pages SHALL preserve the approved mockup's icon choices, descriptions, card hierarchy, spacing, responsive stacking, and PatternFly dark-mode chart text treatment.

The repository UI completion gate SHALL be executed per `skills/build/ui-build-gate/SKILL.md`. When visual comparison is unavailable in the current run, the gate SHALL complete without visual review, skip screenshot capture and comparison, record that capability reason, and continue all non-parity checks.

#### Scenario: WEB-GLIS-25.01 -- CI exercises gateway list table state

- WHEN the UI test suite runs
- THEN it SHALL assert column order, sorting, pagination, and URL state persistence

#### Scenario: WEB-GLIS-25.02 -- CI exercises provisioning timeline visibility

- WHEN the UI test suite runs
- THEN it SHALL assert that fresh creation lands on Connection with timeline
- AND later entry to a running gateway lands on Details without timeline

#### Scenario: WEB-GLIS-25.03 -- CI exercises service account setup modal

- WHEN the UI test suite runs
- THEN it SHALL assert that fresh creation shows the client secret
- AND existing account setup hides the client secret
- AND `openshell-user` shows workspace commands while `openshell-admin` does not

#### Scenario: WEB-GLIS-25.04 -- CI exercises dialog treatments

- WHEN the UI test suite runs
- THEN it SHALL assert that Delete gateway, Revoke service account, and Delete service account use danger styling with warning icons
- AND Remove access and Change your own role use primary styling without warning icons

#### Scenario: WEB-GLIS-25.05 -- Visual comparison capability is recorded

- GIVEN the current runtime is prohibited from taking or comparing snapshots
- WHEN the UI completion gate runs
- THEN screenshot capture and comparison SHALL be skipped
- AND the handoff SHALL record that visual review was unavailable
- AND all non-parity checks SHALL still run

---

## Non-Goals

- This specification does not add a Cluster type dropdown filter to the gateway list; that requires server-side filtering support.
- This specification does not add workspace membership columns to the User access or Service accounts tables; that requires joined API responses.
- This specification does not add row-level sandbox details to HyperShell; gateway-scoped aggregate counts are sufficient.
- This specification does not surface orphaned sandboxes in gateway detail; they remain on the operational dashboard.
- This specification does not add connection providers beyond Claude; Claude is the single example for this iteration.
- This specification does not make the homepage or dashboard attention navigation use gateway detail deep links; attention context is bookmarkable but not enforced.
- This specification does not change the existing service account revocation or deletion authorization logic; it only clarifies the UI dialog treatments.
