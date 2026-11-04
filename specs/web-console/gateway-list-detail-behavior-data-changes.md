# Gateway list and detail behavior and data changes

**Date:** 2026-10-30
**Status:** Draft for team review
**Scope:** Behavior and data implications of the gateway list and detail mockups

## TLDR;

The visual changes need a small set of behavior and data changes behind them. The most important pieces are URL-preserving table state, a derived cluster type, attention continuity from the homepage, and clearer sandbox deep links. Some changes can use existing APIs. Others need new fields or API support before we can implement them safely.

This document separates current behavior from proposed changes so the team can size the work accurately.

## Application shell

**Change**

- Add persistent primary navigation to the application shell.
- Use these destinations and routes:
  - Overview: `/`
  - Gateways: `/gateways`
  - Dashboard: `/dashboard`
- Derive the active navigation item from the current route.
- Treat gateway list, create, and detail routes as the **Gateways** section.
- Treat all dashboard routes as the **Dashboard** section.
- On narrow viewports, use PatternFly's managed sidebar toggle rather than removing navigation.

## Gateway list behavior

### Active sandboxes

**Current behavior**

- The gateway list uses each Gateway's `active_sandbox_count`.
- The dashboard Active total sums the same Gateway counts.
- The shared definition is an active agent-sandbox pod in a Running or Pending phase. Terminated pods do not count.

**Change**

- Keep the list and dashboard aligned on this definition.
- Do not include orphaned sandboxes in the Active count. Orphaned sandboxes have no live Gateway and are surfaced separately as an attention state.

### Table state in the URL

**Current behavior**

- The real page has table controls, but the mockup work calls for explicitly preserving state.

**Change**

- Store search, sort column, sort direction, page, and page size in the URL.
- Use the URL as the source of truth when a user loads, reloads, bookmarks, or shares the view.
- Remove query parameters when they match the default state so common URLs stay readable.
- Apply defaults when an unknown or invalid parameter is present:
  - Sort: Created
  - Direction: descending
  - Page: 1
  - Page size: 20

### Polling

**Current behavior**

- Gateway detail polling runs every 5 seconds.
- Polling is conditional: it runs while the Gateway still needs status updates and stops when that is no longer the case.
- Polling is disabled when the browser tab is in the background.

**Recommended change**

- Increase the status poll interval to 15-30 seconds, or replace full-record polling with a lightweight status endpoint or server-sent events.
- Keep background-tab pausing.
- Do not refresh every detail tab on a header or gateway status poll. Each tab should refresh only its own query.

## Cluster type

### Values

The UI should support only these display values:

- `IBM Public`
- `AWS VPN`
- `IBM Public Kata`

These values reflect the current cluster categories. Do not model a general or open-ended cluster type in the UI.

### Data derivation

**Current behavior**

- Gateway placement data already exposes provider and region.
- Visibility and sandbox runtime are not currently mapped into the Gateway placement object used by the web console.

**Change**

- Derive cluster type server-side or in the BFF from placement metadata. Do not infer it from the cluster display name in the UI.
- Add the metadata needed to distinguish:
  - Public IBM clusters
  - AWS VPN clusters
  - IBM clusters using the Kata runtime
- Return the derived value with the Gateway or Gateway placement response.

### Filtering

**Current behavior**

- There is no server-side Gateway filter for cluster type.

**Change**

- Do not add a Cluster type dropdown filter yet.
- Add the filter only after the list API supports server-side filtering by the derived cluster type. Client-only filtering would be misleading with server-side pagination.

## Copy actions

### Gateway endpoint

- Copy the exact gateway endpoint.
- Do not display the endpoint text in the table; the copy action is enough.
- Use a distinct tooltip and screen-reader name that includes the gateway name.

### CLI connection command

- Open a command modal from the row's code icon.
- Show the full command the CLI needs, including the gateway name and endpoint.
- Keep this separate from direct endpoint copying.
- Use the approved Connection-tab command-block component and copy action.

## Gateway detail behavior

### Connection guidance

- Keep the Claude example for this iteration.
- Present setup as a sequential, task-oriented workflow rather than a single dense script.
- Treat commands without a timing label as reusable every-time commands.
- Let users copy one command at a time or all setup commands at once.
- Keep every setup step expandable in any order. Do not gate later steps or imply completion because HyperShell cannot verify whether users completed the external commands.
- Explain that the install command installs the OpenShell CLI version that matches this gateway and that later steps require the CLI.
- Explain that adding the gateway registers its name and endpoint locally so later commands know which gateway to act on.
- Explain user-facing concepts without requiring prior knowledge of providers or profiles:
  - A connection template tells the CLI how to create a Google Vertex AI connection.
  - Importing a connection template adds its configuration to the gateway.
  - The Claude connection requires the ANTHROPIC_VERTEX_PROJECT_ID environment variable and uses existing Google credentials without pasting an API key.
- Remove the obsolete `openshell inference set` step. Current OpenShell provider creation does not select a model; the sandbox command passes the selected Claude model to the client.
- Replace editable tokens inside command text with read-only commands generated from visible form controls.
- Keep provider name, sandbox name, and Claude model synchronized across the commands that use them.
- Ensure both the copy button and a manual text selection copy the exact command rendered on screen.
- Attach the selected provider when creating the sandbox with `--provider`; do not use the removed managed inference route environment overrides.
- Link the final step to **OpenShell connection documentation**.
- Provide a gateway-console alternative for users who prefer not to work in a terminal.
- Link automation use cases to Service accounts from both User access and Connection.

### Provisioning timeline continuity

- After a user creates a gateway, land on that gateway's Commands tab and show the existing provisioning progress timeline with its current behavior.
- When the user later opens the gateway detail page, land on the default Details tab with the Overview card.
- On that later detail-page entry, show the provisioning timeline only if the installation is still in progress or ended with an error. If the Gateway is already successfully installed at page load, do not show the timeline.
- Once an installation timeline is shown during a visit, keep it visible across tab navigation and after installation succeeds. The completed timeline remains available until the user leaves that gateway and returns, which starts the entry rule above again.

### Tab loading and refresh

**Current behavior**

- Tab panels mount on entry and unmount on exit.
- User access and Service accounts fetch only while their tab is active.
- Each list query has a 5-second stale time. Returning to a tab shows cached data and refetches automatically only if the cache is stale.
- The existing refresh button on User access refetches only the user-access query.

**Change**

- Keep lazy tab loading.
- Add a refresh action to every tab.
- Scope each refresh button to that tab's query:
  - Details and Connection: Gateway details
  - User access: access list
  - Service accounts: service-account list
- Keep the user informed with the dashboard-style last-refreshed treatment where the page already supports it.

### Optional detail fields

- Show Region only when the Gateway placement has a non-empty region.
- Show Runtime only when the Gateway uses a distinct runtime, such as Kata.
- Hide both rows when the data is absent. An absent optional value is a normal state, not a system failure.

## Attention continuity

### Homepage needs attention

The homepage should route users to the owning Gateway detail page for:

- Gateway health problems
- Expiring sandboxes
- Idle sandboxes
- Expiring service accounts

The detail page should continue the same warning. The user should not land on a generic overview and have to reconstruct the reason they came here.

### Attention context

**Proposed change**

- Add an attention context to the detail URL, for example `?attention=service-account-expiring`.
- Map the context to the appropriate detail-page alert.
- Keep the URL context bookmarkable and remove it after the user dismisses the alert or navigates away.

### Gateway health

- Use the existing Gateway health state.
- Show a warning alert on the detail Overview when connection health is degraded.
- Include the affected active-sandbox count when it is known.

### Service-account expiration

**Current data**

- Service accounts expose `expires_at`.

**Change**

- Define expiring-soon as expiration within 7 days.
- Derive the state server-side or in the BFF rather than duplicating threshold logic in the homepage and detail page.
- Add service-account expiration to the homepage **Needs attention** model.
- Show the alert on the Gateway detail Overview and Service accounts tabs.
- Link or navigate to the Service accounts tab so the user can rotate or update the affected account.

### Sandbox expiration and idle states

**Current data**

- The observability spec defines expiring and idle sandbox attention counts.
- Expiring means the sandbox `shutdownTime` is overdue or within 24 hours.
- Idle is derived from sandbox activity signals.
- Current aggregate metrics intentionally do not include sandbox, namespace, or Gateway identifiers.

**Change**

- Surface gateway-scoped expiring and idle counts on the Gateway detail Overview.
- Link to the OpenShell all-sandboxes page when the user can access it.
- If we later need row-level sandbox details in HyperShell, add a Gateway-scoped sandbox attention API. Do not try to infer row identity from aggregate metrics.

### Orphaned sandboxes

Orphaned sandboxes are active sandbox resources in namespaces with no live Gateway. They should stay on the operational dashboard:

- There is no owning Gateway detail page to link to.
- A gateway user cannot fix the orphan through the gateway UI.
- This is an administrative or operational task.

## OpenShell sandbox links

### Stable route

The OpenShell console exposes a stable `/sandboxes` route.

### Namespace behavior

- In `shared` workspace mode, sandbox pods use the Gateway namespace.
- In `managed` mode, sandbox namespaces follow `openshell-{gateway_id}-{workspace}`.
- In `operator` mode, namespaces are pre-provisioned.

The all-sandboxes route does not require the UI to know a sandbox namespace, so the aggregate attention data is sufficient for the navigation action.

### Role behavior

- The upstream `/sandboxes` route is platform-admin only.

Use a role-aware action:

- Gateway owners and admins mapped to `openshell-admin`: link to `/sandboxes`.
- Gateway users mapped to `openshell-user`: link to `/workspaces` or a user-scoped sandbox surface.

Label the action according to the destination, such as **View all sandboxes** or **View workspaces**.

## User access behavior

### Availability during provisioning

**Current behavior**

- The creator's `gateway:owner` RoleBinding is created in the same transaction as the Gateway.
- User access is a HyperShell management-plane operation backed by RoleBindings and the Keycloak directory.
- It does not require the OpenShell Gateway to be healthy.
- The OIDC Role Bridge retries Keycloak client-role assignment if the per-gateway client is not ready yet.

**Change**

- Keep User access available while the Gateway is provisioning.
- Show the provisioning info alert described in the visual document.
- Newly added users can be granted now, but they cannot sign in to OpenShell until Gateway provisioning completes.

### Directory and identities

- Add user searches the Keycloak realm directory projection.
- It can include realm users who have never signed in to HyperShell.
- Granting access pre-provisions the HyperShell User record and creates or updates the Gateway RoleBinding.
- Directory lookup can fail independently if the directory projection is unavailable. That failure is not caused by Gateway provisioning.

### Workspace membership

Gateway access and OpenShell workspace membership are different concepts:

- Owner and Admin gateway roles map to OpenShell admin access.
- User gateway roles map to OpenShell user access.
- An OpenShell user still needs membership in a workspace before they can use that workspace.

**Current data**

- The Gateway access list does not return OpenShell workspace memberships.

**Change**

- Do not add a workspace column yet.
- If the team wants it later, add a joined API response that returns the user's workspace memberships or a clear **No workspace access** state.
- Keep the copyable workspace membership command for User-tier grants until HyperShell manages workspace membership directly.
- Open the User workspace command from a code icon and modal, matching the Service account setup command pattern.

### Table behavior

- Keep search, role filtering, sorting, and pagination server-side.
- Treat the User access table text control as a filter (**Filter by name or user ID**), not the Add user directory search (**Find people**).
- Add bottom pagination to the visual surface.
- Refresh only the active User access query.

### Add user accessibility

**Current behavior**

- The production dialog uses supported PatternFly components: `Modal`, `Form`, `Select`, `MenuToggle`, `TextInputGroup`, and `Radio`.
- Use **Find people** as the directory typeahead's accessible name. The visible field label is replaced by explanatory modal copy.
- The modal focus trap is supported by PatternFly.
- The custom typeahead does not fully implement the official PatternFly keyboard behavior.

**Change**

- Update the typeahead to follow PatternFly's typeahead select example:
  - Arrow Up and Arrow Down move through options
  - Enter selects the focused option
  - Escape closes the menu
  - Clear restores focus to the input
  - Toggle activation restores focus to the input
- Add keyboard interaction tests for tab order, menu traversal, selection, cancellation, and focus restoration.

## Service account behavior

### Availability during provisioning

**Current behavior**

- Service-account creation requires the Gateway phase to be `Running`, status to be `Healthy`, and its OIDC issuer, client ID, and audience to be present.
- The create API returns `409 gateway_not_ready` while the Gateway is provisioning.
- The list and capabilities query do not themselves require a ready Gateway.
- The OpenShell role is assigned during service-account provisioning; there is no separate HyperShell gateway-role assignment step.

**Change**

- Keep the Service accounts tab visible while provisioning.
- Disable service-account creation and show the provisioning info alert described in the visual document.
- After the Gateway becomes ready, allow creation and role assignment in the create dialog.
- Hide the completed provisioning timeline when the Gateway is running without an error.

### Table behavior

- Keep the default sort by Name, ascending.
- Keep search, sorting, and pagination server-side.
- Add bottom pagination.
- Refresh only the active service-account query.
- Open account-specific setup instructions from the row's code icon in the **Setup** column and from the row action menu.

### Role changes

- Keep service-account role changes separate from User access role changes.
- Service accounts select an OpenShell role, such as `openshell-admin` or `openshell-user`.
- User access selects a HyperShell gateway role, such as `owner`, `admin`, or `user`.
- Do not merge the two workflows or reuse their labels interchangeably.
- Show workspace setup only for `openshell-user`. `openshell-admin` bypasses workspace membership.

### Workspace usage

**Current data**

- The service-account API returns account identity, role, status, creation, and expiration fields.
- It does not return the OpenShell workspaces in which a service account is used.

**Change**

- To show workspace usage in production, add a joined service-account API field or an OpenShell integration that returns workspace memberships.
- The mockup shows future-state workspace data to illustrate the intended design.

### JWT and setup semantics

- The stored client ID and client secret are not JWTs.
- Automation sends the client credentials to Keycloak with `grant_type=client_credentials`.
- Keycloak returns a signed, short-lived JWT; the configured lifetime is about five minutes.
- Exchanging credentials does not deactivate the client secret or other credentials.
- Disabling or deleting the service account prevents future token issuance.
- Previously issued JWTs remain valid until their `exp`; normal maximum revocation delay is the token lifetime.
- Account expiration is separate from JWT lifetime.
- In revoke and delete dialogs, communicate the user-visible outcome: no new access tokens are issued, existing tokens expire within about five minutes, and revocation or deletion cannot be recovered.
- Remove the acknowledgement checkbox and **Finish setup** action because they do not verify secret storage or perform another operation. Use a clear **Close** action and one warning that the secret cannot be shown again.
- Existing-account setup instructions must not reveal or regenerate the client secret. The one-time secret is shown only immediately after creation.
- Default the workspace membership command to `default`; users can replace `WORKSPACE_NAME` for another workspace.
