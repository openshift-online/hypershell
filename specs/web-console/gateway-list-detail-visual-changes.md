# Gateway list and detail visual changes

**Date:** 2026-10-30
**Status:** Draft for team review
**Scope:** Gateway list and Gateway detail mockups in `specs/web-console/mockups/gateways/`

## TLDR;

This pass makes the gateway list and detail page easier to scan, gives the detail page a real overview surface, and makes access and attention states clearer. The changes keep PatternFly components and patterns, reduce duplicate or low-value text, and make destructive actions visually distinct from reversible ones.

The mockups are the source for this document. They are not a production implementation plan by themselves; see `gateway-list-detail-behavior-data-changes.md` for the behavior and data work.

## Application shell

- Add a persistent left navigation using PatternFly `PageSidebar`, `PageSidebarBody`, and a simple `Nav`.
- Use the hamburger `PageToggleButton` with PatternFly's managed sidebar behavior for narrow viewports.
- Keep the masthead global and unchanged; do not move page-level actions into the sidebar.
- Show three primary destinations:
  1. Overview
  2. Gateways
  3. Dashboard
- Mark the active destination with PatternFly's current navigation-item state.

## Gateway list

### Table layout

- Rebalance column widths so long values do not crowd out short values.
- Use this column order:
  1. Gateway name
  2. Sandboxes
  3. Status
  4. Version
  5. Connect
  6. Created
  7. Created by
  8. Cluster type
  9. Gateway endpoint
  10. Actions
- Keep Version. It is operationally important and is not redundant with the other columns.
- Remove the raw cluster name. Most users do not need it to choose or operate a gateway.
- Add Cluster type after Created by. It exposes the deployment category users care about: `IBM Public`, `AWS VPN`, or `IBM Public Kata`.
- Use a warning icon with **Attention required** status so the unhealthy state is not conveyed by text alone.
- Keep pagination at the top and add a bottom pagination control for long lists.

### Copy actions

- Do not print the long gateway endpoint in the table. Show a copy action under the **Gateway endpoint** column instead.
- Keep the CLI connection command under the **Connect** column. Use a code icon that opens a command modal with the approved Connection-tab code-block treatment and copy action.
- Keep the standard PatternFly copy icon for the gateway endpoint.
- Separate the two actions in the column order so users do not see two identical icons side by side without context.
- Give each action a distinct visible tooltip and screen-reader label, such as **Copy endpoint for payments-gateway** and **View connection command for payments-gateway**.

### Table state

- Default to sorting by Created, newest first.
- Preserve search, sort, page, and page size in the URL so filtered and sorted views are bookmarkable.
- Use the dashboard-style subtle **Last refreshed** indicator rather than a prominent refresh treatment.
- Keep the existing refresh affordance on the real page.

## Gateway detail

### Page and tab structure

- Make **Details** the first and default tab.
- Use white backgrounds for the tab bar and tab content.
- Remove the divider between the page header and tabs. Let white space separate the regions and reduce the number of horizontal gray lines.
- Show the provisioning timeline only while the gateway is provisioning. Hide it after the gateway is successfully provisioned and running without an error.
- Give each tab a consistent heading, description, and plain refresh button:
  - Details
  - Connection
  - User access
  - Service accounts
- The refresh button represents the active tab's data. It does not refresh every tab at once.

### Overview card

- Make Overview the primary landing card.
- Include the fields users need for orientation:
  - Status
  - Active sandboxes
  - Cluster type
  - Version
  - Created
  - Created by
  - Gateway endpoint
- Put Cluster type below Active sandboxes and Created by below Created.
- Use a longer gateway endpoint copy control so the endpoint is less likely to be visually truncated.
- Bottom-align the **Open gateway console** button with the endpoint copy control. This avoids the odd vertical centering against the label and control.

## Connection tab

### Guided setup

- Lead with the user outcome: connect a terminal to this gateway and start a Claude sandbox.
- Mention that most setup can also be completed in the gateway console.
- Replace the wall of terminal commands with a bordered PatternFly accordion. Only one command is expanded at a time, and each step heading has a token-neutral outlined circular number.
- Number the steps. Mark only one-time steps with a compact outlined label; leave every-time steps unmarked:
  1. Install the OpenShell CLI
  2. Add this gateway
  3. Download the Claude connection template
  4. Import the Claude connection template
  5. Connect this gateway to Claude
  6. Create a Claude sandbox
  7. Open the sandbox in your editor
- Explain each command in user-focused language. In particular:
  - A connection template is a small configuration file that tells OpenShell how to create a Google Vertex AI connection.
  - Importing the template adds that configuration to the gateway.
  - Connecting to Claude requires setting the ANTHROPIC_VERTEX_PROJECT_ID environment variable and creates a named gateway connection using existing Google credentials; the user does not paste an API key.
  - Model selection happens when the sandbox launches Claude; do not show a separate inference command.
- Keep each command in the approved multiline `CommandBlock` with its own copy action.
- Put compact PatternFly form controls above commands that have customizable values:
  - Provider name
  - Sandbox name
  - Claude model
- Keep commands read-only. Regenerate the displayed command from the control values so the copy button and manual text selection both copy exactly what is shown.
- Link the final step to **OpenShell connection documentation**.
- Add **Copy all setup commands** for experienced users who want the full sequence at once.
- Keep Claude as the single example for this iteration.

### Automation entry point

- Move the service-account link out of the top of the Connection flow.
- Add a quiet **Automate this gateway** card at the bottom for CI/CD pipelines and bots.

### Card layout and typography

- Group details into Overview, Identity, and Placement cards.
- Use a two-column card grid on large screens. On smaller screens, the second column stacks below the first.
- Make Placement stack below Identity on narrow screens instead of compressing both cards.
- Use PatternFly vertical description lists, with the label above the value.
- Put card titles in `CardHeader` and `CardTitle`.
- Render card titles with `Title headingLevel="h2" size="xl"` to match PatternFly dashboard examples.
- Hide Region and Runtime rows when the values are absent. Do not show an ambiguous **Not available** state.

### Attention and lifecycle alerts

- Replace the weak **Related activity** card with alerts that only appear when action is needed.
- Use warning styling for:
  - Gateway health is degraded
  - Sandboxes expire soon
  - Sandboxes are idle
  - Service accounts expire soon
- Continue the attention story from the homepage into the gateway detail page. The user should not have to remember why they navigated here.
- Show service-account expiration alerts on both the Overview and Service accounts tabs.
- For sandbox expiration and idle alerts, use **View all sandboxes** as the primary action.
- Keep orphaned sandboxes out of gateway detail. They have no owning gateway, so the operational dashboard is the correct surface.

## User access tab

### Naming and layout

- Rename **Manage access** to **User access**.
- Keep Service accounts as a separate tab because they also control access, but they are a different object and workflow.
- Use **User access** for people selected from the LDAP-backed Keycloak directory.
- Separate filter controls from the **Add user** action with PatternFly toolbar groups. This makes the action look like a table-level action rather than part of the search and role filter.
- Label the table's text filter **Filter by name or user ID**. Keep **Find people** only in the Add user directory-search dialog.
- Rename the toolbar action and modal title to **Add user** because the UI grants one user at a time.
- In the Add user modal, replace the visible field label with normal-weight guidance: **Search the identity directory for a user to add to the gateway.** Keep **Find people** as the input's accessible name and placeholder.
- Add a **Need automation access?** card after the table. It explains that CI/CD pipelines and bots should use service accounts and links to the Service accounts tab.
- Add sorting to Name, User ID, Role, and Granted.
- Add a Granted column and bottom pagination.
- Use the same code icon as Service accounts for the User workspace command. The icon opens a **Grant workspace access** modal with one copyable command.

### Provisioning state

- Keep User access available while a gateway is provisioning.
- Show a standard inline info alert:
  - Title: **User access can be granted now**
  - Body: **People you add will be able to sign in once gateway provisioning completes.**

## Dialog treatments

- **Delete gateway** is a destructive, irreversible action. Use danger styling and a warning icon beside the modal title.
- **Edit service account** removes the distracting client-credential note, shortens the role-change alert, and adds spacing between the alert and description field.
- **Revoke service account** is destructive. Use a warning icon in the title and a danger confirmation button.
- **Delete service account** is destructive. Use a warning icon in the title and a danger confirmation button.
- **Remove access** is reversible. Use a primary confirmation button without danger styling or a warning icon.
- **Change your own role** is consequential but reversible by another owner. Use a primary confirmation button without danger styling.
- Block changing or removing the final owner before a confirmation modal is needed, with an explanatory disabled state.

## Service accounts tab

### Table layout and terminology

- Keep Name as the default sort.
- Keep pagination at the top and add a bottom pagination control.
- Replace the nested expandable section with the official PatternFly expandable table row.
- Show expanded account details in a details row spanning the visible table columns.
- Rename **Service account subject** to **Principal subject** to match the OpenShell console.
- Label the role column **OpenShell role** to distinguish it from HyperShell gateway roles used on User access.
- Show the account's workspaces in the expanded row. This is future-state data because the current service-account API does not return workspace usage.
- Add a **Setup** column between Service account name and OpenShell role. Each row uses a code icon that opens that account's setup instructions, with a tooltip and screen-reader label such as **Open setup instructions for ci-pipeline**.

### Setup modal

- Use one concise warning for the one-time client secret.
- Remove the duplicate workspace info alert.
- Present setup in a clear order:
  1. Save the client ID, client secret, and principal subject
  2. Grant workspace access when the account uses the `openshell-user` role
  3. Request an access token
- Use the same code icon and command-modal pattern as User access.
- Use the approved multiline command-block treatment for workspace-access and access-token commands.
- For `openshell-user`, explain that the service account cannot use the gateway until its principal subject is added to a workspace. Offer the gateway console or terminal command, and default `WORKSPACE_NAME` to `default`.
- Collapse **Request an access token** by default because it is an optional automation step.
- Explain that the JWT is a short-lived credential used by automation while the client secret remains protected in the caller's secret manager.
- For an existing service account, show setup instructions without the one-time client secret. Show the secret only in the setup modal that appears immediately after account creation.
- Remove the **I saved the client secret** acknowledgement checkbox.
- Replace **Finish setup** with **Close**. The modal only presents setup information; it does not verify that the user saved the secret.

### Provisioning state

- Keep the Service accounts tab available while a gateway is provisioning.
- Show an inline info alert explaining that service accounts can be created after provisioning completes.
- Disable **Create service account** while the gateway is provisioning.

## PatternFly notes

- Keep the standard PatternFly copy icon for every copyable value. A custom copy icon would be confusing.
- Put actions that affect table data in the table toolbar, which matches PatternFly's action guidance.
- Use plain icon buttons only with accessible names and tooltips.
- Use standard `Alert`, `Modal`, `Card`, `DescriptionList`, `Toolbar`, `Pagination`, and `Table` components rather than custom visual equivalents.
