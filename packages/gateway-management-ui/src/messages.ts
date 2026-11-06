import { defineMessages } from "react-intl";

export const messages = defineMessages({
  actions: {
    id: "app.table.column.actions",
    defaultMessage: "Actions",
    description: "Accessible heading for table row actions.",
  },
  activeSandboxes: {
    id: "app.gateway.column.activeSandboxes",
    defaultMessage: "Active sandboxes",
    description:
      "Heading for the gateways table column showing the number of active sandbox sessions.",
  },
  attentionRequired: {
    id: "app.gateway.status.attentionRequired",
    defaultMessage: "Attention required",
    description:
      "Status text shown for unhealthy gateways (warning or danger states).",
  },
  bulkDeleteActiveSandboxWarning: {
    id: "app.gateway.bulkDelete.activeSandboxWarning",
    defaultMessage:
      "{gatewayCount, plural, one {# selected gateway has} other {# selected gateways have}} active sandboxes that will be disrupted by deletion.",
    description:
      "Warning in the bulk-delete confirmation dialog when selected gateways still have running sandboxes.",
  },
  bulkDeleteButton: {
    id: "app.gateway.bulkDelete.button",
    defaultMessage: "Delete",
    description: "Toolbar action that deletes the selected gateways in bulk.",
  },
  bulkDeleteButtonAria: {
    id: "app.gateway.bulkDelete.buttonAria",
    defaultMessage:
      "Delete {count, plural, one {# selected gateway} other {# selected gateways}}",
    description:
      "Accessible label for the bulk-delete toolbar action, stating how many gateways are selected.",
  },
  bulkDeleteClearSelection: {
    id: "app.gateway.bulkDelete.clearSelection",
    defaultMessage: "Clear selection",
    description: "Action that deselects every selected gateway.",
  },
  bulkDeleteConfirmBody: {
    id: "app.gateway.bulkDelete.confirmBody",
    defaultMessage:
      "Deleting a gateway removes it from the platform and starts asynchronous teardown of its workload, database, and routing. This cannot be undone. The following gateways will be deleted:",
    description:
      "Explanation shown in the bulk-delete confirmation dialog before deletion.",
  },
  bulkDeleteConfirmButton: {
    id: "app.gateway.bulkDelete.confirmButton",
    defaultMessage:
      "Delete {count, plural, one {# gateway} other {# gateways}}",
    description:
      "Confirming action in the bulk-delete dialog, stating how many gateways will be deleted.",
  },
  bulkDeleteConfirmTitle: {
    id: "app.gateway.bulkDelete.confirmTitle",
    defaultMessage:
      "Delete {count, plural, one {# gateway} other {# gateways}}?",
    description: "Title of the bulk-delete confirmation dialog.",
  },
  bulkDeleteResultAccepted: {
    id: "app.gateway.bulkDelete.resultAccepted",
    defaultMessage:
      "{gatewayName}: deletion accepted; resource teardown in progress.",
    description:
      "Per-gateway result line for an accepted asynchronous deletion whose teardown has not completed.",
  },
  bulkDeleteResultFailed: {
    id: "app.gateway.bulkDelete.resultFailed",
    defaultMessage:
      "{gatewayName}: deletion failed. This gateway stays selected so you can retry it.",
    description:
      "Per-gateway result line for a failed deletion that remains selected for retry.",
  },
  bulkDeleteResultTitleAllAccepted: {
    id: "app.gateway.bulkDelete.resultTitleAllAccepted",
    defaultMessage:
      "{count, plural, one {# gateway deletion accepted} other {# gateway deletions accepted}}",
    description:
      "Results banner title when every bulk deletion was accepted for asynchronous teardown.",
  },
  bulkDeleteResultTitleAllFailed: {
    id: "app.gateway.bulkDelete.resultTitleAllFailed",
    defaultMessage:
      "{count, plural, one {# gateway deletion failed} other {# gateway deletions failed}}",
    description: "Results banner title when every bulk deletion failed.",
  },
  bulkDeleteResultTitlePartial: {
    id: "app.gateway.bulkDelete.resultTitlePartial",
    defaultMessage:
      "{failed, number} of {total, number} gateway deletions failed",
    description: "Results banner title when some bulk deletions failed.",
  },
  bulkDeleteRetry: {
    id: "app.gateway.bulkDelete.retry",
    defaultMessage: "Retry failed deletions",
    description:
      "Action that retries only the failed deletions without resubmitting the accepted ones.",
  },
  bulkDeleteRetrying: {
    id: "app.gateway.bulkDelete.retrying",
    defaultMessage: "Retrying failed deletions",
    description:
      "Results banner title shown while a retry of the failed deletions is in progress.",
  },
  bulkSelectAriaLabel: {
    id: "app.gateway.bulkSelect.ariaLabel",
    defaultMessage: "Select gateways on this page",
    description:
      "Accessible label for the header bulk-selection control, whose scope is the current page.",
  },
  bulkSelectNone: {
    id: "app.gateway.bulkSelect.none",
    defaultMessage: "Select none ({count, number} items)",
    description:
      "Menu option that clears the selection in the bulk-selection control.",
  },
  bulkSelectOptions: {
    id: "app.gateway.bulkSelect.options",
    defaultMessage: "Bulk selection options",
    description:
      "Accessible label for the caret button that opens the bulk-selection scope menu (select none or select page).",
  },
  bulkSelectPage: {
    id: "app.gateway.bulkSelect.page",
    defaultMessage: "Select page ({count, number} items)",
    description:
      "Menu option that selects every eligible gateway on the current page.",
  },
  bulkSelectSelectedCount: {
    id: "app.gateway.bulkSelect.selectedCount",
    defaultMessage: "{count, number} selected",
    description:
      "Count of selected gateways shown on the header bulk-selection control.",
  },
  cancel: {
    id: "app.action.cancel",
    defaultMessage: "Cancel",
    description: "Action that leaves a form without submitting it.",
  },
  clearClusterSearch: {
    id: "app.gateway.cluster.clearSearch",
    defaultMessage: "Clear cluster search",
    description: "Accessible label for clearing the cluster typeahead input.",
  },
  clearFilters: {
    id: "app.table.clearFilters",
    defaultMessage: "Clear filters",
    description: "Action that clears collection filters.",
  },
  cliConnection: {
    id: "app.gateway.cliConnection",
    defaultMessage: "CLI connection",
    description: "Label for a gateway's OpenShell CLI connection command.",
  },
  cliConnectionCommandCopied: {
    id: "app.gateway.cliConnectionCommandCopied",
    defaultMessage: "CLI connection command for {gatewayName} copied",
    description: "Success notification after copying a gateway CLI command.",
  },
  cliConnectionCommandCopyFailed: {
    id: "app.gateway.cliConnectionCommandCopyFailed",
    defaultMessage:
      "CLI connection command for {gatewayName} could not be copied",
    description: "Error notification when copying a gateway CLI command fails.",
  },
  clipboardCopyFailed: {
    id: "app.clipboard.copyFailed",
    defaultMessage: "Copy failed",
    description:
      "Failure feedback shown when text could not be copied to the clipboard.",
  },
  close: {
    id: "app.action.close",
    defaultMessage: "Close",
    description: "Accessible label for closing an overlay.",
  },
  cluster: {
    id: "app.gateway.cluster",
    defaultMessage: "Cluster",
    description: "Heading for the gateway placement cluster column.",
  },
  clusterLoadError: {
    id: "app.gateway.cluster.loadError.title",
    defaultMessage: "Managed clusters could not be loaded",
    description: "Title shown when remote gateway placements cannot be loaded.",
  },
  clusterLoadErrorBody: {
    id: "app.gateway.cluster.loadError.body",
    defaultMessage:
      "A registered cluster is required to provision a gateway. Try loading managed clusters again.",
    description:
      "Recovery guidance when remote gateway placements cannot be loaded.",
  },
  clusterType: {
    id: "app.gateway.column.clusterType",
    defaultMessage: "Cluster type",
    description:
      "Heading for the gateway cluster type column showing the derived placement category.",
  },
  connect: {
    id: "app.gateway.column.connect",
    defaultMessage: "Connect",
    description: "Heading for the gateway CLI connection command column.",
  },
  connectionAddGatewayDescription: {
    id: "app.gateway.connection.addGateway.description",
    defaultMessage:
      "Adds and authenticates the gateway if needed, then selects it. Existing gateway registrations are reused.",
    description: "Supporting text for the add-gateway connection step.",
  },
  connectionAddGatewayTitle: {
    id: "app.gateway.connection.addGateway.title",
    defaultMessage: "Add this gateway",
    description: "Title for the add-gateway connection step.",
  },
  connectionBeforeYouBegin: {
    id: "app.gateway.connection.beforeYouBegin",
    defaultMessage:
      "Export your EnMaaS key as ENMAAS_KEY in your terminal. The provider command will prompt for it if the variable is not set.",
    description: "Prerequisite guidance shown above the connection steps.",
  },
  connectionBeforeYouBeginTitle: {
    id: "app.gateway.connection.beforeYouBegin.title",
    defaultMessage: "Before you begin",
    description: "Title for the connection prerequisite guidance.",
  },
  connectionCommandAction: {
    id: "app.gateway.rowActions.connectionCommand",
    defaultMessage: "Connection command",
    description: "Menu action that opens a gateway connection command modal.",
  },
  connectionCommandHelper: {
    id: "app.gateway.connectionCommandModal.helper",
    defaultMessage:
      "Run this command in a terminal to add the gateway to the OpenShell CLI.",
    description:
      "Helper text for the modal showing a gateway's CLI connection command.",
  },
  connectionCommandModal: {
    id: "app.gateway.connectionCommandModal.title",
    defaultMessage: "Connect to {gatewayName}",
    description: "Title for the modal showing how to connect to a gateway.",
  },
  connectionConnectEnmaasDescription: {
    id: "app.gateway.connection.connectEnmaas.description",
    defaultMessage:
      "Creates the provider if needed. It prompts for your EnMaaS key only when creating a new provider.",
    description: "Supporting text for the connect-gateway-to-EnMaaS step.",
  },
  connectionConnectEnmaasTitle: {
    id: "app.gateway.connection.connectEnmaas.title",
    defaultMessage: "Connect this gateway to EnMaaS",
    description: "Title for the connect-gateway-to-EnMaaS connection step.",
  },
  connectionCreateProfileDescription: {
    id: "app.gateway.connection.createProfile.description",
    defaultMessage:
      "Creates the local provider profile that binds your EnMaaS key to the EnMaaS endpoint.",
    description: "Supporting text for the create-profile connection step.",
  },
  connectionCreateProfileTitle: {
    id: "app.gateway.connection.createProfile.title",
    defaultMessage: "Create the EnMaaS provider profile",
    description: "Title for the create-profile connection step.",
  },
  connectionEditorOptionsLink: {
    id: "app.gateway.connection.editorOptionsLink",
    defaultMessage: "OpenShell connection documentation",
    description:
      "Link text pointing to the OpenShell sandbox connect CLI documentation.",
  },
  connectionEditorOptionsLinkNewTab: {
    id: "app.gateway.connection.editorOptionsLinkNewTab",
    defaultMessage: "OpenShell connection documentation (opens in a new tab)",
    description:
      "Accessible name for the sandbox connect docs link, including that it opens in a new tab.",
  },
  connectionEnmaasLink: {
    id: "app.gateway.connection.enmaas.link",
    defaultMessage: "Learn about EnMaaS and get a key",
    description: "Link text for the EnMaaS documentation.",
  },
  connectionEnmaasLinkNewTab: {
    id: "app.gateway.connection.enmaas.linkNewTab",
    defaultMessage: "Learn about EnMaaS and get a key (opens in a new tab)",
    description:
      "Accessible name for the EnMaaS documentation link, including that it opens in a new tab.",
  },
  connectionImportProfileDescription: {
    id: "app.gateway.connection.importProfile.description",
    defaultMessage:
      "Validates and imports the EnMaaS provider profile if it is not already installed.",
    description: "Supporting text for the import-profile connection step.",
  },
  connectionImportProfileTitle: {
    id: "app.gateway.connection.importProfile.title",
    defaultMessage: "Import the EnMaaS provider profile",
    description: "Title for the import-profile connection step.",
  },
  connectionInstallLink: {
    id: "app.gateway.connection.installLink",
    defaultMessage: "View installation documentation",
    description:
      "Link text pointing to the OpenShell CLI installation documentation.",
  },
  connectionInstallLinkNewTab: {
    id: "app.gateway.connection.installLinkNewTab",
    defaultMessage: "View installation documentation (opens in a new tab)",
    description:
      "Accessible name for the OpenShell CLI installation documentation link, including that it opens in a new tab.",
  },
  connectionInstallStepDescription: {
    id: "app.gateway.connection.install.description",
    defaultMessage:
      "Only needed once. Installs the OpenShell CLI version that matches this gateway.",
    description: "Supporting text for the CLI installation connection step.",
  },
  connectionInstallStepTitle: {
    id: "app.gateway.connection.install.title",
    defaultMessage: "Install the OpenShell CLI",
    description: "Title for the CLI installation connection step.",
  },
  connectionLoginUnavailable: {
    id: "app.gateway.connection.login.unavailable",
    defaultMessage:
      "This gateway is still provisioning. Its connection command becomes available once the gateway is running.",
    description:
      "Shown in the login step while the gateway has not yet reached a running, ready-to-connect phase.",
  },
  connectionModelNameLabel: {
    id: "app.gateway.connection.modelName.label",
    defaultMessage: "Claude model",
    description: "Label for the Claude model used by the sandbox command.",
  },
  connectionOneTime: {
    id: "app.gateway.connection.oneTime",
    defaultMessage: "One-time",
    description: "Label marking a connection step that only needs to run once.",
  },
  connectionOpenClaudeDescription: {
    id: "app.gateway.connection.openClaude.description",
    defaultMessage:
      "Attaches directly to the running Claude process in the sandbox.",
    description: "Supporting text for the open-Claude connection step.",
  },
  connectionOpenClaudeTitle: {
    id: "app.gateway.connection.openClaude.title",
    defaultMessage: "Open Claude in the sandbox",
    description: "Title for the open-Claude-in-sandbox connection step.",
  },
  connectionProviderNameHelper: {
    id: "app.gateway.connection.providerName.helper",
    defaultMessage: "Also used when creating the sandbox.",
    description: "Helper text for the connection provider name field.",
  },
  connectionProviderNameLabel: {
    id: "app.gateway.connection.providerName.label",
    defaultMessage: "Provider name",
    description: "Label for the connection provider name field.",
  },
  connectionSandboxDescription: {
    id: "app.gateway.connection.sandbox.description",
    defaultMessage:
      "Creates the sandbox if needed. Exit Claude to explore the sandbox file system.",
    description: "Supporting text for the create-sandbox connection step.",
  },
  connectionSandboxNameHelper: {
    id: "app.gateway.connection.sandboxName.helper",
    defaultMessage: "The sandbox name is also used when connecting to Claude.",
    description: "Helper text for the connection sandbox name field.",
  },
  connectionSandboxNameLabel: {
    id: "app.gateway.connection.sandboxName.label",
    defaultMessage: "Sandbox name",
    description: "Label for the connection sandbox name field.",
  },
  connectionSandboxTitle: {
    id: "app.gateway.connection.sandbox.title",
    defaultMessage: "Create a Claude sandbox",
    description: "Title for the create-sandbox connection step.",
  },
  connectionTab: {
    id: "app.gateway.connection.tab",
    defaultMessage: "Connection",
    description: "Label for the default gateway detail Connection tab.",
  },
  connectionTabsLabel: {
    id: "app.gateway.connection.tabsLabel",
    defaultMessage:
      "Gateway connection, manage access, service accounts, and details",
    description: "Accessible label for the gateway detail tabs.",
  },
  connectionWaitingForProvisioning: {
    id: "app.gateway.connection.waitingForProvisioning",
    defaultMessage: "Waiting for gateway provisioning...",
    description:
      "Shown in the connection tab while the gateway is still being provisioned.",
  },
  copied: {
    id: "app.clipboard.copied",
    defaultMessage: "Copied",
    description: "Confirmation shown after text is copied to the clipboard.",
  },
  copy: {
    id: "app.clipboard.copy",
    defaultMessage: "Copy",
    description: "Tooltip for a button that copies text to the clipboard.",
  },
  copyAllConnectionCommands: {
    id: "app.gateway.connection.copyAll",
    defaultMessage: "Copy all commands",
    description: "Action that copies every visible connection command.",
  },
  copyCliConnectionCommand: {
    id: "app.gateway.copyCliConnectionCommand",
    defaultMessage: "Copy CLI connection command",
    description: "Menu action that copies a gateway's CLI connection command.",
  },
  copyConnectionCommand: {
    id: "app.gateway.copyConnectionCommand",
    defaultMessage: "Copy connection command for {gatewayName}",
    description:
      "Accessible label for copying a specific gateway's CLI connection command.",
  },
  copyConnectionStepCommand: {
    id: "app.gateway.connection.copyStep",
    defaultMessage: "Copy command for step {number}, {title}",
    description:
      "Accessible label for copying one command in the connection guide.",
  },
  copyEndpoint: {
    id: "app.gateway.column.copyEndpoint",
    defaultMessage: "Copy endpoint for {gatewayName}",
    description:
      "Accessible label and tooltip for the gateway endpoint copy action.",
  },
  copyEndpointAction: {
    id: "app.gateway.rowActions.copyEndpoint",
    defaultMessage: "Copy endpoint",
    description: "Menu action that copies a gateway's network endpoint.",
  },
  copyGatewayEndpoint: {
    id: "app.gateway.copyEndpoint",
    defaultMessage: "Copy gateway endpoint for {gatewayName}",
    description:
      "Accessible label for copying a specific gateway's network endpoint.",
  },
  copyInstallCommand: {
    id: "app.gateway.connection.copyInstallCommand",
    defaultMessage: "Copy the OpenShell installation command",
    description:
      "Accessible label for copying the version-matched OpenShell installation command.",
  },
  copySandboxCommand: {
    id: "app.gateway.connection.copySandboxCommand",
    defaultMessage: "Copy the create-sandbox command",
    description: "Accessible label for copying the create-sandbox command.",
  },
  copySandboxConnectCommand: {
    id: "app.gateway.connection.copySandboxConnectCommand",
    defaultMessage: "Copy the connect-sandbox command",
    description:
      "Accessible label for the button that copies the sandbox connect command.",
  },
  copySetupCommand: {
    id: "app.gateway.connection.copySetupCommand",
    defaultMessage: "Copy the one-time setup commands",
    description:
      "Accessible label for copying the gateway registration, provider profile import, provider creation, and model selection commands.",
  },
  created: {
    id: "app.table.column.created",
    defaultMessage: "Created",
    description: "Heading for a resource creation date column.",
  },
  createdBy: {
    id: "app.gateway.column.createdBy",
    defaultMessage: "Created by",
    description: "Heading for the gateway creator column.",
  },
  deleteGateway: {
    id: "app.gateway.delete",
    defaultMessage: "Delete gateway",
    description: "Action that permanently deletes a gateway.",
  },
  deleteGatewayActiveSandboxWarning: {
    id: "app.gateway.delete.activeSandboxWarning",
    defaultMessage:
      "This gateway has {count, plural, one {# active sandbox} other {# active sandboxes}} that will be disrupted by deletion.",
    description:
      "Warning shown before deleting a gateway that still has running sandboxes.",
  },
  deleteGatewayConfirmation: {
    id: "app.gateway.delete.confirmation",
    defaultMessage:
      "Deleting {gatewayName} will permanently remove the gateway. This action cannot be undone.",
    description: "Warning shown before permanently deleting a gateway.",
  },
  deleteGatewayNotPermitted: {
    id: "app.gateway.delete.notPermitted",
    defaultMessage:
      "Only a gateway owner or platform admin can delete this gateway.",
    description:
      "Tooltip explaining why the Delete gateway action is disabled for a caller who lacks delete permission.",
  },
  deleteGatewayTitle: {
    id: "app.gateway.delete.title",
    defaultMessage: "Delete {gatewayName}?",
    description: "Title for the gateway deletion confirmation dialog.",
  },
  deletingGateway: {
    id: "app.gateway.delete.pending",
    defaultMessage: "Deleting gateway",
    description: "Accessible progress text while a gateway is being deleted.",
  },
  detailsTab: {
    id: "app.gateway.detailsTab",
    defaultMessage: "Details",
    description: "Label for the gateway detail Details tab.",
  },
  editEditor: {
    id: "app.gateway.connection.editEditor",
    defaultMessage: "Editor",
    description:
      "Accessible label for the editor selector in the connect-sandbox command.",
  },
  editExistingSandboxName: {
    id: "app.gateway.connection.editExistingSandboxName",
    defaultMessage: "Existing sandbox name (editable)",
    description:
      "Accessible label for the inline-editable sandbox name in the connect-sandbox command.",
  },
  editModel: {
    id: "app.gateway.connection.editModel",
    defaultMessage: "Model (editable)",
    description:
      "Accessible label for the inline-editable model name in the setup command.",
  },
  editProviderName: {
    id: "app.gateway.connection.editProviderName",
    defaultMessage: "Provider name (editable)",
    description:
      "Accessible label for the inline-editable provider name in the setup command.",
  },
  editSandboxName: {
    id: "app.gateway.connection.editSandboxName",
    defaultMessage: "Sandbox name (editable)",
    description:
      "Accessible label for the inline-editable sandbox name in the create-sandbox command.",
  },
  error: {
    id: "app.status.error",
    defaultMessage: "Error:",
    description: "Screen-reader prefix for an error message.",
  },
  filterGateways: {
    id: "app.page.gateways.filter",
    defaultMessage: "Filter by name, status, or endpoint",
    description: "Placeholder and accessible label for gateway search.",
  },
  gateway: {
    id: "app.gateway.singular",
    defaultMessage: "Gateway",
    description: "Fallback breadcrumb label while a gateway is loading.",
  },
  gatewayDeleted: {
    id: "app.gateway.deleted",
    defaultMessage: "Gateway {gatewayName} deleted",
    description: "Success notification after deleting a gateway.",
  },
  gatewayDeleteError: {
    id: "app.gateway.delete.error.title",
    defaultMessage: "Gateway could not be deleted",
    description: "Title shown when gateway deletion fails.",
  },
  gatewayDeleteErrorBody: {
    id: "app.gateway.delete.error.body",
    defaultMessage: "No changes were made. Try again.",
    description: "Recovery guidance when gateway deletion fails.",
  },
  gatewayDescription: {
    id: "app.page.gateway.description",
    defaultMessage: "Connect to this gateway and review its configuration.",
    description: "Metadata description for the gateway detail page.",
  },
  gatewayDetailsTitle: {
    id: "app.page.gatewayDetails.title",
    defaultMessage: "Gateway details",
    description: "Metadata title for a user-facing gateway details page.",
  },
  gatewayEndpoint: {
    id: "app.gateway.endpoint",
    defaultMessage: "Gateway endpoint",
    description: "Label for an OpenShell gateway network endpoint.",
  },
  gatewayEndpointCopied: {
    id: "app.gateway.endpoint.copied",
    defaultMessage: "Endpoint for {gatewayName} copied",
    description: "Confirmation shown after a gateway endpoint is copied.",
  },
  gatewayEndpointCopyFailed: {
    id: "app.gateway.endpoint.copyFailed",
    defaultMessage: "Endpoint for {gatewayName} could not be copied",
    description: "Error shown when copying a gateway endpoint fails.",
  },
  gatewayLoadError: {
    id: "app.gateway.load.error.title",
    defaultMessage: "Gateways could not be loaded",
    description: "Title shown when gateway data cannot be loaded from the API.",
  },
  gatewayLoadErrorBody: {
    id: "app.gateway.load.error.body",
    defaultMessage: "Refresh the page to try again.",
    description: "Recovery guidance when gateway data cannot be loaded.",
  },
  gatewayName: {
    id: "app.gateway.name",
    defaultMessage: "Gateway name",
    description: "Label for a gateway name form field.",
  },
  gatewayProvisionError: {
    id: "app.page.gatewayProvision.error.title",
    defaultMessage: "Gateway could not be provisioned",
    description: "Title shown when gateway provisioning fails.",
  },
  gatewayProvisionErrorBody: {
    id: "app.page.gatewayProvision.error.body",
    defaultMessage: "Check the values and try again.",
    description: "Recovery guidance when gateway provisioning fails.",
  },
  gatewayRenamed: {
    id: "app.gateway.renamed",
    defaultMessage: "Gateway renamed to {gatewayName}",
    description: "Success notification after renaming a gateway.",
  },
  gatewayRenameError: {
    id: "app.gateway.rename.error.title",
    defaultMessage: "Gateway could not be renamed",
    description: "Title shown when gateway renaming fails.",
  },
  gatewayRenameErrorBody: {
    id: "app.gateway.rename.error.body",
    defaultMessage:
      "No changes were made. Choose a different name or try again.",
    description: "Recovery guidance when gateway renaming fails.",
  },
  gatewayRowActions: {
    id: "app.gateway.rowActions",
    defaultMessage: "Actions for {gatewayName}",
    description: "Accessible label for a gateway row actions menu.",
  },
  gateways: {
    id: "app.nav.gateways",
    defaultMessage: "OpenShell Gateways",
    description: "Page and resource collection label for OpenShell gateways.",
  },
  gatewaysDescription: {
    id: "app.page.gateways.description",
    defaultMessage: "HyperShell gateways.",
    description: "Browser metadata description for the gateways page.",
  },
  gatewaysEmptyBody: {
    id: "app.page.gateways.empty.body",
    defaultMessage: "Provisioned gateways will appear here.",
    description: "Body text when the gateway list is empty.",
  },
  gatewaysEmptyTitle: {
    id: "app.page.gateways.empty.title",
    defaultMessage: "No gateways",
    description: "Heading when the gateway list is empty.",
  },
  loadingClusterName: {
    id: "app.gateway.cluster.loadingName",
    defaultMessage: "Loading cluster name",
    description:
      "Status shown while a gateway row resolves its placement name.",
  },
  loadingClusters: {
    id: "app.gateway.cluster.loading",
    defaultMessage: "Loading managed clusters",
    description: "Status shown while remote gateway placements are loading.",
  },
  loadingGateways: {
    id: "app.gateway.loading",
    defaultMessage: "Loading gateways",
    description: "Accessible label shown while gateway data is loading.",
  },
  manageServiceAccounts: {
    id: "app.gateway.manageServiceAccounts",
    defaultMessage: "Manage service accounts",
    description:
      "Link text that navigates to the service accounts tab from the connection tab.",
  },
  moreClustersAvailable: {
    id: "app.gateway.cluster.moreResults",
    defaultMessage:
      "More clusters are available. Refine your search to find a specific cluster.",
    description:
      "Guidance when a bounded cluster search has additional API results.",
  },
  namespace: {
    id: "app.gateway.namespace",
    defaultMessage: "Namespace",
    description: "Label for a gateway namespace value.",
  },
  noMatchingClusters: {
    id: "app.gateway.cluster.noResults",
    defaultMessage: "No matching clusters",
    description: "Status shown when no managed clusters match the search.",
  },
  noMatchingGateways: {
    id: "app.page.gateways.noResults.title",
    defaultMessage: "No matching gateways",
    description: "Heading when no gateways match the current filter.",
  },
  noMatchingGatewaysBody: {
    id: "app.page.gateways.noResults.body",
    defaultMessage: "Adjust or clear the filter to see gateways.",
    description: "Guidance when no gateways match the current filter.",
  },
  noOwnedGatewaysBody: {
    id: "app.page.gateways.noOwned.body",
    defaultMessage:
      "You do not own any gateways yet. Turn off the ownership filter to see the full list.",
    description:
      "Guidance when the ownership filter is on but the user owns no gateways.",
  },
  noOwnedGatewaysTitle: {
    id: "app.page.gateways.noOwned.title",
    defaultMessage: "No gateways you own",
    description:
      "Heading when the ownership filter is on but the user owns no gateways.",
  },
  notAvailable: {
    id: "app.value.notAvailable",
    defaultMessage: "Not available",
    description: "Shown when the API does not provide a value.",
  },
  notifications: {
    id: "app.notifications",
    defaultMessage: "Notifications",
    description: "Accessible label for transient application notifications.",
  },
  openGatewayConsole: {
    id: "app.gateway.openConsole",
    defaultMessage: "Open gateway console",
    description: "Action that opens an OpenShell gateway's console.",
  },
  openGatewayConsoleFor: {
    id: "app.gateway.openConsoleFor",
    defaultMessage: "Open console for {gatewayName} in a new tab",
    description:
      "Accessible label for opening a specific gateway console in a new tab.",
  },
  owner: {
    id: "app.gateway.column.owner",
    defaultMessage: "Created by",
    description:
      "Heading for the gateways table column showing who created the gateway.",
  },
  provisionGateway: {
    id: "app.page.gatewayProvision.title",
    defaultMessage: "Provision gateway",
    description: "Page title and action for provisioning a gateway.",
  },
  provisionGatewayDescription: {
    id: "app.page.gatewayProvision.description",
    defaultMessage: "Configure a new OpenShell gateway.",
    description: "Supporting text on the gateway provisioning page.",
  },
  sandboxes: {
    id: "app.gateway.column.sandboxes",
    defaultMessage: "Sandboxes",
    description:
      "Heading for the gateways table column showing the active sandbox count.",
  },
  viewConnectionCommand: {
    id: "app.gateway.column.viewConnectionCommand",
    defaultMessage: "View connection command for {gatewayName}",
    description:
      "Accessible label and tooltip for the gateway connection command action.",
  },
  /* eslint-disable sort-keys -- Keep gateway placement descriptors together for localization review. */
  networkAccess: {
    id: "app.page.gatewayProvision.networkAccess",
    defaultMessage: "Network access",
    description: "Network exposure mode for a gateway.",
  },
  internalServicesQuestion: {
    id: "app.page.gatewayProvision.internalServicesQuestion",
    defaultMessage:
      "Does the gateway need to reach GitLab or other internal Red Hat services?",
    description:
      "Question determining whether a gateway requires Red Hat VPN access.",
  },
  internalServicesNo: {
    id: "app.page.gatewayProvision.internalServicesNo",
    defaultMessage: "No",
    description: "Option for a gateway that does not need internal services.",
  },
  internalServicesYes: {
    id: "app.page.gatewayProvision.internalServicesYes",
    defaultMessage: "Yes",
    description: "Option for a gateway that needs internal Red Hat services.",
  },
  internalServicesYesVpn: {
    id: "app.page.gatewayProvision.internalServicesYesVpn",
    defaultMessage: "Yes (Requires Red Hat VPN)",
    description:
      "Option for a gateway that needs internal Red Hat services and VPN access.",
  },
  choiceUnavailable: {
    id: "app.page.gatewayProvision.choiceUnavailable",
    defaultMessage: "This option is not available for the selected placement.",
    description: "Fallback reason shown when a placement choice is disabled.",
  },
  cloudProvider: {
    id: "app.page.gatewayProvision.cloudProvider",
    defaultMessage: "Cloud provider",
    description: "Cloud provider for public gateway placement.",
  },
  publicNetwork: {
    id: "app.page.gatewayProvision.publicNetwork",
    defaultMessage: "Public",
    description: "Public network placement option.",
  },
  publicNetworkDescription: {
    id: "app.page.gatewayProvision.publicNetworkDescription",
    defaultMessage: "Accessible through a public endpoint.",
    description: "Description of public network placement.",
  },
  vpnNetwork: {
    id: "app.page.gatewayProvision.vpnNetwork",
    defaultMessage: "VPN",
    description: "VPN network placement option.",
  },
  vpnNetworkDescription: {
    id: "app.page.gatewayProvision.vpnNetworkDescription",
    defaultMessage:
      "For gateways that need to reach GitLab and other internal Red Hat services.",
    description: "Description of VPN placement.",
  },
  awsProvider: {
    id: "app.page.gatewayProvision.awsProvider",
    defaultMessage: "Amazon Web Services",
    description: "AWS cloud provider option.",
  },
  awsProviderDescription: {
    id: "app.page.gatewayProvision.awsProviderDescription",
    defaultMessage:
      "For publicly accessible workloads that rely heavily on AWS services or data.",
    description: "Description of the AWS provider option.",
  },
  awsProviderRadioLabel: {
    id: "app.page.gatewayProvision.awsProviderRadioLabel",
    defaultMessage:
      "AWS (For workloads that rely heavily on AWS services or data)",
    description: "Accessible label for the AWS provider radio.",
  },
  ibmCloudProvider: {
    id: "app.page.gatewayProvision.ibmCloudProvider",
    defaultMessage: "IBM Cloud",
    description: "IBM Cloud provider option.",
  },
  ibmCloudProviderDescription: {
    id: "app.page.gatewayProvision.ibmCloudProviderDescription",
    defaultMessage:
      "General-purpose workloads with no special network or data needs.",
    description: "Description of the IBM Cloud provider option.",
  },
  ibmProviderRadioLabel: {
    id: "app.page.gatewayProvision.ibmProviderRadioLabel",
    defaultMessage: "IBM (default)",
    description: "Accessible label for the default IBM provider radio.",
  },
  unavailableProvider: {
    id: "app.page.gatewayProvision.unavailableProvider",
    defaultMessage: "{provider} is unavailable: {reason}",
    description:
      "Explanation shown when a provider cannot currently accept placement.",
  },
  unavailableNetwork: {
    id: "app.page.gatewayProvision.unavailableNetwork",
    defaultMessage: "{network} is unavailable: {reason}",
    description:
      "Explanation shown when a network cannot currently accept placement.",
  },
  noEligibleCluster: {
    id: "app.page.gatewayProvision.noEligibleCluster",
    defaultMessage: "no eligible managed cluster is connected.",
    description:
      "Provider unavailability reason when no matching connected cluster exists.",
  },
  noManagedPlacement: {
    id: "app.page.gatewayProvision.noManagedPlacement",
    defaultMessage: "No managed placement is currently available.",
    description:
      "Warning shown when no provider or local development placement can be selected.",
  },
  vpnRequiresAws: {
    id: "app.page.gatewayProvision.vpnRequiresAws",
    defaultMessage: "VPN placement requires AWS.",
    description: "Explanation shown when IBM Cloud is disabled for VPN.",
  },
  vpnRequiresLabel: {
    id: "app.page.gatewayProvision.vpnRequiresLabel",
    defaultMessage: "Red Hat VPN required",
    description: "Label shown above the VPN placement guidance.",
  },
  kindPlacement: {
    id: "app.page.gatewayProvision.kindPlacement",
    defaultMessage: "Use Kind",
    description: "Local Kind placement option.",
  },
  kindPlacementDescription: {
    id: "app.page.gatewayProvision.kindPlacementDescription",
    defaultMessage: "Use the local Kind cluster for development.",
    description: "Description of local Kind placement.",
  },
  localDevelopment: {
    id: "app.page.gatewayProvision.localDevelopment",
    defaultMessage: "Local development",
    description: "Form group label for local Kind placement.",
  },
  placementHelp: {
    id: "app.page.gatewayProvision.placementHelp",
    defaultMessage:
      "A matching managed cluster is selected at random for the chosen network and provider.",
    description: "Explanation of automatic gateway placement.",
  },
  placementAvailabilityError: {
    id: "app.page.gatewayProvision.placementAvailabilityError",
    defaultMessage:
      "Placement availability could not be loaded. Cloud placement is disabled until availability can be verified.",
    description:
      "Warning shown when current gateway placement availability cannot be retrieved.",
  },
  /* eslint-enable sort-keys */
  provisioningGateway: {
    id: "app.page.gatewayProvision.pending",
    defaultMessage: "Provisioning gateway",
    description: "Accessible progress text while a gateway is provisioning.",
  },
  provisioningGatewayConsole: {
    id: "app.gateway.provisioningConsole",
    defaultMessage: "Provisioning console...",
    description:
      "Tooltip on the disabled console button while the console is not yet ready.",
  },
  refreshGateways: {
    id: "app.gateways.refresh",
    defaultMessage: "Refresh gateways",
    description: "Accessible label for refreshing the gateway list.",
  },
  registeredClustersOnly: {
    id: "app.gateway.cluster.registeredOnly",
    defaultMessage:
      "Only clusters with a connected control plane can host a gateway.",
    description:
      "Helper text under the cluster selector explaining which clusters are offered.",
  },
  renameGateway: {
    id: "app.gateway.rename",
    defaultMessage: "Rename gateway",
    description: "Action that changes a gateway's name.",
  },
  renameGatewayNotPermitted: {
    id: "app.gateway.rename.notPermitted",
    defaultMessage: "Only a gateway owner or admin can rename this gateway.",
    description:
      "Tooltip explaining why the Rename gateway action is disabled for a caller who lacks edit permission.",
  },
  renameGatewayTitle: {
    id: "app.gateway.rename.title",
    defaultMessage: "Rename {gatewayName}",
    description: "Title for the gateway rename dialog.",
  },
  renamingGateway: {
    id: "app.gateway.rename.pending",
    defaultMessage: "Renaming gateway",
    description: "Accessible progress text while a gateway is being renamed.",
  },
  requiredField: {
    id: "app.form.requiredField",
    defaultMessage: "This field is required.",
    description: "Validation message for an empty required form field.",
  },
  results: {
    id: "app.table.results",
    defaultMessage: "results",
    description: "Context announced after a filtered collection result count.",
  },
  retry: {
    id: "app.action.retry",
    defaultMessage: "Retry",
    description: "Action that repeats a failed request.",
  },
  selectCluster: {
    id: "app.gateway.cluster.select",
    defaultMessage: "Select a cluster",
    description: "Accessible label and placeholder for the cluster selector.",
  },
  selectGatewayRow: {
    id: "app.gateway.bulkSelect.row",
    defaultMessage: "Select gateway {gatewayName}",
    description:
      "Accessible name for a gateway row's selection checkbox, identifying the gateway.",
  },
  selectGatewayRowDisabled: {
    id: "app.gateway.bulkSelect.rowDisabled",
    defaultMessage: "Select gateway {gatewayName} (deletion not permitted)",
    description:
      "Accessible name for a disabled gateway row selection checkbox, explaining why selection is unavailable.",
  },
  selectionColumn: {
    id: "app.gateway.bulkSelect.column",
    defaultMessage: "Row selection",
    description:
      "Screen-reader heading for the gateway table's row-selection column.",
  },
  showOnlyMyGateways: {
    id: "app.page.gateways.showOnlyMine",
    defaultMessage: "Show only my gateways",
    description:
      "On/off switch label that filters the gateway list to only the signed-in user's gateways.",
  },
  status: {
    id: "app.table.column.status",
    defaultMessage: "Status",
    description: "Heading for a resource status column.",
  },
  /* eslint-disable sort-keys -- Keep this secret-sensitive workflow's descriptors together so localization review can audit the complete journey as one unit. */
  serviceAccountsTab: {
    id: "app.gateway.serviceAccounts.tab",
    defaultMessage: "Service accounts",
    description: "Label for the gateway detail service-accounts tab.",
  },
  serviceAccountsHeading: {
    id: "app.gateway.serviceAccounts.heading",
    defaultMessage: "Service accounts",
    description: "Heading for gateway-scoped service-account management.",
  },
  serviceAccountsDescription: {
    id: "app.gateway.serviceAccounts.description",
    defaultMessage:
      "Use service accounts for automation. Each account exchanges client credentials for short-lived JWTs that work only with this gateway.",
    description: "Introductory text for gateway service accounts.",
  },
  createServiceAccount: {
    id: "app.gateway.serviceAccounts.create",
    defaultMessage: "Create service account",
    description: "Action that creates a gateway service account.",
  },
  creatingServiceAccount: {
    id: "app.gateway.serviceAccounts.creating",
    defaultMessage: "Creating service account",
    description: "Progress text while a service account is created.",
  },
  refreshServiceAccounts: {
    id: "app.gateway.serviceAccounts.refresh",
    defaultMessage: "Refresh service accounts",
    description: "Accessible label for refreshing service accounts.",
  },
  filterServiceAccounts: {
    id: "app.gateway.serviceAccounts.filter",
    defaultMessage: "Filter by name or client ID",
    description: "Label and placeholder for service-account search.",
  },
  serviceAccountRole: {
    id: "app.gateway.serviceAccounts.role",
    defaultMessage: "OpenShell role",
    description: "Service-account role field and table heading.",
  },
  serviceAccountStatusProvisioning: {
    id: "app.gateway.serviceAccounts.status.provisioning",
    defaultMessage: "Provisioning",
    description: "Status while a service-account identity is being created.",
  },
  serviceAccountStatusReady: {
    id: "app.gateway.serviceAccounts.status.ready",
    defaultMessage: "Ready",
    description: "Status for a service account that can issue access tokens.",
  },
  serviceAccountStatusExpired: {
    id: "app.gateway.serviceAccounts.status.expired",
    defaultMessage: "Expired",
    description: "Status for an expired service account.",
  },
  serviceAccountStatusRevoking: {
    id: "app.gateway.serviceAccounts.status.revoking",
    defaultMessage: "Revoking",
    description: "Status while a service account is being revoked.",
  },
  serviceAccountStatusRevoked: {
    id: "app.gateway.serviceAccounts.status.revoked",
    defaultMessage: "Revoked",
    description: "Status for a permanently revoked service account.",
  },
  serviceAccountStatusDeleting: {
    id: "app.gateway.serviceAccounts.status.deleting",
    defaultMessage: "Deleting",
    description: "Status while a service account is being deleted.",
  },
  serviceAccountStatusError: {
    id: "app.gateway.serviceAccounts.status.error",
    defaultMessage: "Error",
    description: "Status for a service account that needs replacement.",
  },
  serviceAccountStatusDegraded: {
    id: "app.gateway.serviceAccounts.status.degraded",
    defaultMessage: "Degraded",
    description:
      "Status for a previously-ready service account whose reconciliation failed part-way and is being repaired.",
  },
  expiration: {
    id: "app.gateway.serviceAccounts.expiration",
    defaultMessage: "Expiration",
    description: "Service-account expiration field and table heading.",
  },
  serviceAccountsEmptyTitle: {
    id: "app.gateway.serviceAccounts.empty.title",
    defaultMessage: "No service accounts",
    description: "Heading for a gateway with no service accounts.",
  },
  serviceAccountsEmptyBody: {
    id: "app.gateway.serviceAccounts.empty.body",
    defaultMessage:
      "Create a service account when automation needs this gateway.",
    description: "Guidance for a gateway with no service accounts.",
  },
  noMatchingServiceAccounts: {
    id: "app.gateway.serviceAccounts.noResults.title",
    defaultMessage: "No matching service accounts",
    description: "Heading when service-account filters have no matches.",
  },
  noMatchingServiceAccountsBody: {
    id: "app.gateway.serviceAccounts.noResults.body",
    defaultMessage: "Clear the search and status filter to see all accounts.",
    description:
      "Recovery guidance for service-account filters with no matches.",
  },
  serviceAccountsLoadError: {
    id: "app.gateway.serviceAccounts.loadError.title",
    defaultMessage: "Service accounts could not be loaded",
    description: "Title for a service-account collection error.",
  },
  serviceAccountsLoadErrorBody: {
    id: "app.gateway.serviceAccounts.loadError.body",
    defaultMessage: "Your filters are unchanged. Try loading this page again.",
    description: "Recovery guidance for a service-account collection error.",
  },
  serviceAccountOwnScope: {
    id: "app.gateway.serviceAccounts.ownScope",
    defaultMessage:
      "This list contains only service accounts that you created.",
    description: "Explains creator-scoped service-account visibility.",
  },
  serviceAccountCreateDenied: {
    id: "app.gateway.serviceAccounts.createDenied",
    defaultMessage:
      "You do not have permission to create a service account for this gateway.",
    description: "Explanation for a disabled service-account create action.",
  },
  gatewayNeedsAttention: {
    id: "app.gateway.detail.attention.gatewayHealth.title",
    defaultMessage: "Gateway needs attention",
    description: "Title for a gateway health warning.",
  },
  gatewayHealthAttentionBody: {
    id: "app.gateway.detail.attention.gatewayHealth.body",
    defaultMessage:
      "Connection health is degraded. Active sandboxes may be affected.",
    description:
      "Explains the effect of degraded gateway health when the count is unknown.",
  },
  gatewayHealthAttentionBodyWithCount: {
    id: "app.gateway.detail.attention.gatewayHealth.bodyWithCount",
    defaultMessage:
      "Connection health is degraded. {activeSandboxCount, plural, =0 {No active sandboxes are affected.} one {# active sandbox may be affected.} other {# active sandboxes may be affected.}}",
    description:
      "Explains the effect of degraded gateway health with the affected count.",
  },
  serviceAccountExpiringTitle: {
    id: "app.gateway.detail.attention.serviceAccountExpiring.title",
    defaultMessage:
      "{count, plural, one {# service account expires} other {# service accounts expire}} within 7 days",
    description: "Title for service accounts expiring within seven days.",
  },
  serviceAccountExpiringBody: {
    id: "app.gateway.detail.attention.serviceAccountExpiring.body",
    defaultMessage:
      "Review and rotate credentials before automation access stops.",
    description: "Guidance for service accounts expiring within seven days.",
  },
  serviceAccountExpiringTabBody: {
    id: "app.gateway.detail.attention.serviceAccountExpiring.tabBody",
    defaultMessage:
      "Review the affected accounts and rotate credentials before expiration.",
    description:
      "Guidance shown on the service-accounts tab for accounts expiring within seven days.",
  },
  viewAllSandboxes: {
    id: "app.gateway.detail.attention.gatewayHealth.viewAllSandboxes",
    defaultMessage: "View all sandboxes",
    description: "Action that opens all gateway sandboxes in a new tab.",
  },
  reviewServiceAccounts: {
    id: "app.gateway.detail.attention.serviceAccountExpiring.action",
    defaultMessage: "Review service accounts",
    description: "Action that opens the service-accounts tab.",
  },
  dismissGatewayAttention: {
    id: "app.gateway.detail.attention.dismiss",
    defaultMessage: "Dismiss attention alert",
    description: "Accessible label for dismissing a gateway attention alert.",
  },
  viewServiceAccountDetails: {
    id: "app.gateway.serviceAccounts.viewDetails",
    defaultMessage: "View account details",
    description: "Disclosure control for non-secret service-account metadata.",
  },
  serviceAccountDescription: {
    id: "app.gateway.serviceAccounts.field.description",
    defaultMessage: "Description (optional)",
    description: "Label for an optional service-account description.",
  },
  editServiceAccountRole: {
    id: "app.gateway.serviceAccounts.editRole.action",
    defaultMessage: "Edit role",
    description: "Row action that opens the in-place role edit dialog.",
  },
  editServiceAccountRoleSave: {
    id: "app.gateway.serviceAccounts.editRole.save",
    defaultMessage: "Save changes",
    description:
      "Confirmation button for the service-account role edit dialog.",
  },
  editServiceAccountRoleTitle: {
    id: "app.gateway.serviceAccounts.editRole.title",
    defaultMessage: "Edit {serviceAccountName}",
    description: "Title of the service-account role edit dialog.",
  },
  editServiceAccountRoleIntroduction: {
    id: "app.gateway.serviceAccounts.editRole.introduction",
    defaultMessage:
      "Change the OpenShell role or description of {serviceAccountName}. Its client credentials are unchanged.",
    description: "Introduction of the service-account role edit dialog.",
  },
  editServiceAccountRoleEffect: {
    id: "app.gateway.serviceAccounts.editRole.effect.title",
    defaultMessage: "When the change takes effect",
    description:
      "Heading for the token-lag explanation in the role edit dialog.",
  },
  editServiceAccountRoleEffectBody: {
    id: "app.gateway.serviceAccounts.editRole.effect.body",
    defaultMessage:
      "The next token exchange reflects the new role. Access tokens issued before the change keep the prior role until they expire (at most a few minutes).",
    description:
      "Token-lag explanation in the service-account role edit dialog.",
  },
  serviceAccountRoleNotAllowed: {
    id: "app.gateway.serviceAccounts.editRole.notAllowed.title",
    defaultMessage: "The requested role is not permitted",
    description:
      "Title shown when a role update exceeds the creator's gateway role.",
  },
  serviceAccountRoleNotAllowedBody: {
    id: "app.gateway.serviceAccounts.editRole.notAllowed.body",
    defaultMessage:
      "A service account is capped by its creator's current gateway role, so this account cannot receive that role.",
    description:
      "Guidance shown when a role update exceeds the creator's gateway role.",
  },
  clientId: {
    id: "app.gateway.serviceAccounts.clientId",
    defaultMessage: "Client ID",
    description: "Label for an OIDC client identifier.",
  },
  clientSecret: {
    id: "app.gateway.serviceAccounts.clientSecret",
    defaultMessage: "Client secret",
    description: "Label for an OIDC client secret.",
  },
  subject: {
    id: "app.gateway.serviceAccounts.subject",
    defaultMessage: "Service account subject",
    description: "Label for the stable OIDC subject of a service account.",
  },
  subjectHelp: {
    id: "app.gateway.serviceAccounts.subjectHelp",
    defaultMessage:
      "This stable ID is the JWT subject (sub). Gateway administrators use it to grant workspace access.",
    description: "Explains the service-account subject identifier.",
  },
  serviceAccountName: {
    id: "app.gateway.serviceAccounts.name",
    defaultMessage: "Service account name",
    description: "Label for a service-account name.",
  },
  optional: {
    id: "app.form.optional",
    defaultMessage: "Optional",
    description: "Indicates that a form field is optional.",
  },
  openshellUserRoleDescription: {
    id: "app.gateway.serviceAccounts.role.user.description",
    defaultMessage:
      "Can authenticate as openshell-user. A gateway administrator must separately add its subject to each workspace it needs.",
    description: "Explanation of the openshell-user service-account role.",
  },
  openshellAdminRoleDescription: {
    id: "app.gateway.serviceAccounts.role.admin.description",
    defaultMessage:
      "Can perform OpenShell administrative operations on this gateway.",
    description: "Explanation of the openshell-admin service-account role.",
  },
  serviceAccountExpirationOption: {
    id: "app.gateway.serviceAccounts.expiration.option",
    defaultMessage: "{days, plural, one {# day} other {# days}}",
    description: "Duration option for service-account expiration.",
  },
  serviceAccountExpiresPreview: {
    id: "app.gateway.serviceAccounts.expiration.preview",
    defaultMessage: "Expires {expiration}.",
    description: "Absolute expiration preview before service-account creation.",
  },
  serviceAccountTokenLifetimeNote: {
    id: "app.gateway.serviceAccounts.expiration.tokenNote",
    defaultMessage:
      "Account expiration stops future token grants. Each access token has a separate short lifetime.",
    description: "Distinguishes service-account and access-token expiration.",
  },
  serviceAccountCreateError: {
    id: "app.gateway.serviceAccounts.createError.title",
    defaultMessage: "Service account could not be created",
    description: "Title for a definitive service-account create failure.",
  },
  serviceAccountCreateErrorBody: {
    id: "app.gateway.serviceAccounts.createError.body",
    defaultMessage: "Review the values and try again.",
    description: "Recovery guidance for a definitive create failure.",
  },
  serviceAccountNameExists: {
    id: "app.gateway.serviceAccounts.nameExists.title",
    defaultMessage: "A service account with this name already exists",
    description: "Title when a service-account name is already in use.",
  },
  serviceAccountNameExistsBody: {
    id: "app.gateway.serviceAccounts.nameExists.body",
    defaultMessage:
      "Choose a different name, or delete the active service account before reusing this name.",
    description: "Recovery guidance when a service-account name is in use.",
  },
  serviceAccountCreateUncertain: {
    id: "app.gateway.serviceAccounts.createUncertain.title",
    defaultMessage: "The create result is uncertain",
    description: "Title when one-time credential delivery fails ambiguously.",
  },
  serviceAccountCreateUncertainBody: {
    id: "app.gateway.serviceAccounts.createUncertain.body",
    defaultMessage:
      "Refresh the list. If the account exists, delete it and create a replacement. Retrying this request cannot recover its client secret.",
    description: "Recovery guidance for an uncertain create result.",
  },
  serviceAccountSetupTitle: {
    id: "app.gateway.serviceAccounts.setup.title",
    defaultMessage: "Set up {serviceAccountName}",
    description: "Title for one-time or repeatable service-account setup.",
  },
  serviceAccountSecretOnce: {
    id: "app.gateway.serviceAccounts.secretOnce",
    defaultMessage:
      "Save this client secret now. HyperShell cannot show it again.",
    description: "One-time client-secret warning.",
  },
  serviceAccountSecretUnavailable: {
    id: "app.gateway.serviceAccounts.secretUnavailable",
    defaultMessage:
      "The client secret is no longer available. Use the secret you saved, or create a replacement service account.",
    description: "Explanation for repeatable setup without a client secret.",
  },
  showClientSecret: {
    id: "app.gateway.serviceAccounts.secret.show",
    defaultMessage: "Show client secret",
    description: "Action that reveals a masked client secret.",
  },
  hideClientSecret: {
    id: "app.gateway.serviceAccounts.secret.hide",
    defaultMessage: "Hide client secret",
    description: "Action that masks a client secret.",
  },
  copyClientSecret: {
    id: "app.gateway.serviceAccounts.secret.copy",
    defaultMessage: "Copy client secret",
    description: "Action that copies the client secret.",
  },
  serviceAccountCopyFailed: {
    id: "app.gateway.serviceAccounts.copyFailed",
    defaultMessage:
      "Could not copy to the clipboard. Copy the value manually or try again.",
    description:
      "Recovery guidance when copying service-account setup data fails.",
  },
  openShellCliSetup: {
    id: "app.gateway.serviceAccounts.setup.openshell",
    defaultMessage: "Use with the OpenShell CLI",
    description: "Heading for supported OpenShell CLI setup commands.",
  },
  exchangeCredentialsForJwt: {
    id: "app.gateway.serviceAccounts.setup.jwt",
    defaultMessage: "Exchange credentials for a JWT",
    description: "Heading for advanced Client Credentials commands.",
  },
  copyOpenShellServiceAccountCommands: {
    id: "app.gateway.serviceAccounts.setup.copyOpenShell",
    defaultMessage: "Copy OpenShell CLI setup commands",
    description: "Accessible label for copying the OpenShell command group.",
  },
  copyJwtExchangeCommands: {
    id: "app.gateway.serviceAccounts.setup.copyJwt",
    defaultMessage: "Copy JWT exchange commands",
    description: "Accessible label for copying the JWT command group.",
  },
  serviceAccountCommandsUnavailable: {
    id: "app.gateway.serviceAccounts.setup.unavailable",
    defaultMessage:
      "Setup commands are unavailable because required connection metadata is missing.",
    description: "Explanation when a complete command cannot be generated.",
  },
  workspaceMembershipNote: {
    id: "app.gateway.serviceAccounts.workspaceMembership",
    defaultMessage:
      "This service account does not inherit your workspace access. Give the service account subject above to a gateway administrator. The administrator must add it to each required workspace.",
    description: "Explains the separate OpenShell workspace grant.",
  },
  grantWorkspaceAccess: {
    id: "app.gateway.serviceAccounts.workspaceGrant.title",
    defaultMessage: "Grant workspace access",
    description: "Heading for the manual gateway-administrator command.",
  },
  workspaceGrantInstructions: {
    id: "app.gateway.serviceAccounts.workspaceGrant.instructions",
    defaultMessage:
      "After selecting this gateway, a gateway administrator can replace the workspace name and run:",
    description: "Instructions for granting service-account workspace access.",
  },
  copyWorkspaceGrantCommand: {
    id: "app.gateway.serviceAccounts.workspaceGrant.copy",
    defaultMessage: "Copy workspace access command",
    description: "Accessible label for copying the workspace grant command.",
  },
  acknowledgeSecretSaved: {
    id: "app.gateway.serviceAccounts.secret.acknowledge",
    defaultMessage: "I saved the client secret in a secure secret manager.",
    description: "Required acknowledgement before completing one-time setup.",
  },
  finishSetup: {
    id: "app.gateway.serviceAccounts.setup.finish",
    defaultMessage: "Finish setup",
    description: "Action that completes acknowledged one-time setup.",
  },
  closeSetup: {
    id: "app.gateway.serviceAccounts.setup.close",
    defaultMessage: "Close setup instructions",
    description: "Action that closes repeatable non-secret setup instructions.",
  },
  leaveWithoutSecretTitle: {
    id: "app.gateway.serviceAccounts.secret.leave.title",
    defaultMessage: "Leave without saving the client secret?",
    description: "Title for the one-time secret-loss confirmation.",
  },
  leaveWithoutSecretBody: {
    id: "app.gateway.serviceAccounts.secret.leave.body",
    defaultMessage:
      "HyperShell cannot recover this secret. You must delete this service account and create a replacement.",
    description: "Consequence of closing one-time setup without saving.",
  },
  leaveSetup: {
    id: "app.gateway.serviceAccounts.secret.leave",
    defaultMessage: "Leave setup",
    description: "Confirms loss of an unsaved one-time secret.",
  },
  returnToSetup: {
    id: "app.gateway.serviceAccounts.secret.return",
    defaultMessage: "Return to setup",
    description: "Returns from secret-loss confirmation to one-time setup.",
  },
  serviceAccountRowActions: {
    id: "app.gateway.serviceAccounts.rowActions",
    defaultMessage: "Actions for service account {serviceAccountName}",
    description: "Accessible label for a service-account row actions menu.",
  },
  viewSetupInstructions: {
    id: "app.gateway.serviceAccounts.viewSetup",
    defaultMessage: "View setup instructions",
    description: "Action that opens repeatable non-secret setup instructions.",
  },
  revokeServiceAccount: {
    id: "app.gateway.serviceAccounts.revoke",
    defaultMessage: "Revoke service account",
    description: "Action that permanently revokes a service account.",
  },
  deleteServiceAccount: {
    id: "app.gateway.serviceAccounts.delete",
    defaultMessage: "Delete service account",
    description: "Action that deletes a service account and Keycloak identity.",
  },
  revokeServiceAccountTitle: {
    id: "app.gateway.serviceAccounts.revoke.title",
    defaultMessage: "Revoke {serviceAccountName}?",
    description: "Title for service-account revoke confirmation.",
  },
  revokeServiceAccountBody: {
    id: "app.gateway.serviceAccounts.revoke.body",
    defaultMessage:
      "Keycloak will permanently stop issuing new access tokens. Tokens already issued can work until they expire. The account will remain here for audit and later deletion.",
    description: "Consequences shown before service-account revocation.",
  },
  deleteServiceAccountTitle: {
    id: "app.gateway.serviceAccounts.delete.title",
    defaultMessage: "Delete {serviceAccountName}?",
    description: "Title for service-account delete confirmation.",
  },
  deleteServiceAccountBody: {
    id: "app.gateway.serviceAccounts.delete.body",
    defaultMessage:
      "This removes the Keycloak identity and the visible service account. A ready account is revoked first. Tokens already issued can work until they expire.",
    description: "Consequences shown before service-account deletion.",
  },
  revokingServiceAccount: {
    id: "app.gateway.serviceAccounts.revoking",
    defaultMessage: "Revoking service account",
    description: "Progress text while a service account is revoked.",
  },
  deletingServiceAccount: {
    id: "app.gateway.serviceAccounts.deleting",
    defaultMessage: "Deleting service account",
    description: "Progress text while a service account is deleted.",
  },
  serviceAccountActionError: {
    id: "app.gateway.serviceAccounts.actionError.title",
    defaultMessage: "The service account could not be updated",
    description: "Title for a service-account lifecycle action failure.",
  },
  serviceAccountActionErrorBody: {
    id: "app.gateway.serviceAccounts.actionError.body",
    defaultMessage:
      "No confirmed change was reported. Try again or refresh the list.",
    description: "Recovery guidance for a lifecycle action failure.",
  },
  loadingServiceAccount: {
    id: "app.gateway.serviceAccounts.loadingOne",
    defaultMessage: "Loading service account",
    description: "Status while loading service-account setup metadata.",
  },
  provisioningProgressLabel: {
    id: "app.gateway.provisioningProgress.label",
    defaultMessage: "Provisioning progress",
    description:
      "Accessible label for the progress stepper showing provisioning steps.",
  },
  provisioningStepConfiguringIdentityProvider: {
    id: "app.gateway.provisioningProgress.step.identityProvider",
    defaultMessage: "Configuring identity provider",
    description: "Label for the identity provider provisioning step.",
  },
  provisioningStepDeployingGateway: {
    id: "app.gateway.provisioningProgress.step.deploy",
    defaultMessage: "Deploying gateway",
    description: "Label for the gateway deployment provisioning step.",
  },
  provisioningStepPreparingEnvironment: {
    id: "app.gateway.provisioningProgress.step.environment",
    defaultMessage: "Preparing environment",
    description: "Label for the environment preparation provisioning step.",
  },
  provisioningStepProvisioningDatabase: {
    id: "app.gateway.provisioningProgress.step.database",
    defaultMessage: "Provisioning database",
    description: "Label for the database provisioning step.",
  },
  provisioningStepVerifyingHealth: {
    id: "app.gateway.provisioningProgress.step.health",
    defaultMessage: "Verifying gateway health",
    description: "Label for the gateway health verification provisioning step.",
  },
  provisioningStepProvisioned: {
    id: "app.gateway.provisioningProgress.step.provisioned",
    defaultMessage: "Provisioned",
    description: "Label for the final provisioning step indicating completion.",
  },
  provisioningStepEnvironmentReady: {
    id: "app.gateway.provisioningProgress.step.environmentReady",
    defaultMessage: "Environment ready",
    description: "Completed-state label for the environment preparation step.",
  },
  provisioningStepDatabaseReady: {
    id: "app.gateway.provisioningProgress.step.databaseReady",
    defaultMessage: "Database ready",
    description: "Completed-state label for the database provisioning step.",
  },
  provisioningStepIdentityProviderReady: {
    id: "app.gateway.provisioningProgress.step.identityProviderReady",
    defaultMessage: "IdP ready",
    description:
      "Completed-state label for the identity provider configuration step.",
  },
  provisioningStepGatewayDeployed: {
    id: "app.gateway.provisioningProgress.step.gatewayDeployed",
    defaultMessage: "Gateway deployed",
    description: "Completed-state label for the gateway deployment step.",
  },
  provisioningStepHealthVerified: {
    id: "app.gateway.provisioningProgress.step.healthVerified",
    defaultMessage: "Health verified",
    description:
      "Completed-state label for the gateway health verification step.",
  },
  provisioningStepStartingConsole: {
    id: "app.gateway.provisioningProgress.step.startingConsole",
    defaultMessage: "Starting console",
    description:
      "Active-state label for the console readiness provisioning step.",
  },
  provisioningStepConsoleReady: {
    id: "app.gateway.provisioningProgress.step.consoleReady",
    defaultMessage: "Console ready",
    description:
      "Completed-state label for the console readiness provisioning step.",
  },
  /* eslint-enable sort-keys */
  unavailableGatewayConsole: {
    id: "app.gateway.unavailableConsole",
    defaultMessage: "Console unavailable for this gateway",
    description:
      "Tooltip on the disabled console button once the console failed to become available within the expected time.",
  },
  /* eslint-disable sort-keys -- Keep the Manage access tab strings together so localization review can audit the complete surface as one unit. */
  manageAccessTab: {
    id: "app.gateway.access.tab",
    defaultMessage: "Manage access",
    description: "Label for the gateway detail Manage access tab.",
  },
  userAccessTab: {
    id: "app.gateway.userAccess.tab",
    defaultMessage: "User access",
    description: "Label for the gateway detail User access tab.",
  },
  accessHeading: {
    id: "app.gateway.access.heading",
    defaultMessage: "Gateway access",
    description: "Heading for the Manage access tab.",
  },
  accessDescription: {
    id: "app.gateway.access.description",
    defaultMessage: "Manage who can access this gateway and at what role.",
    description: "Description under the Manage access heading.",
  },
  accessUserName: {
    id: "app.gateway.access.column.userName",
    defaultMessage: "Name",
    description: "Access table column for the user's display name.",
  },
  accessUserId: {
    id: "app.gateway.access.column.userId",
    defaultMessage: "User ID",
    description: "Access table column for the username.",
  },
  accessRoleColumn: {
    id: "app.gateway.access.column.role",
    defaultMessage: "Role",
    description: "Access table column for the role.",
  },
  accessRoleOwner: {
    id: "app.gateway.access.role.owner",
    defaultMessage: "Owner",
    description: "Owner role label.",
  },
  accessRoleAdmin: {
    id: "app.gateway.access.role.admin",
    defaultMessage: "Admin",
    description: "Admin role label.",
  },
  accessRoleUser: {
    id: "app.gateway.access.role.user",
    defaultMessage: "User",
    description: "User role label.",
  },
  accessRoleOwnerDescription: {
    id: "app.gateway.access.role.owner.description",
    defaultMessage:
      "Full control, including deleting the gateway and assigning other owners.",
    description: "Owner role description in the role picker.",
  },
  accessRoleAdminDescription: {
    id: "app.gateway.access.role.admin.description",
    defaultMessage: "Manage the gateway and grant admin or user access.",
    description: "Admin role description in the role picker.",
  },
  accessRoleUserDescription: {
    id: "app.gateway.access.role.user.description",
    defaultMessage: "Read-only access to the gateway.",
    description: "User role description in the role picker.",
  },
  accessFindPeople: {
    id: "app.gateway.access.findPeople",
    defaultMessage: "Find people...",
    description: "Placeholder and label for the access list search input.",
  },
  accessRoleFilterLabel: {
    id: "app.gateway.access.roleFilter.label",
    defaultMessage: "Filter by role",
    description: "Accessible label for the role filter.",
  },
  accessRoleFilterAll: {
    id: "app.gateway.access.roleFilter.all",
    defaultMessage: "All roles",
    description: "Role filter option that clears the role filter.",
  },
  accessAddUsers: {
    id: "app.gateway.access.addUsers",
    defaultMessage: "Add users",
    description: "Button that opens the directory picker.",
  },
  accessDirectorySearchLabel: {
    id: "app.gateway.access.directory.searchLabel",
    defaultMessage: "Search the identity directory",
    description: "Accessible label for the directory typeahead.",
  },
  accessDirectoryLoading: {
    id: "app.gateway.access.directory.loading",
    defaultMessage: "Searching directory...",
    description: "Loading sentinel for the directory typeahead.",
  },
  accessDirectoryNoResults: {
    id: "app.gateway.access.directory.noResults",
    defaultMessage: "No matching users",
    description: "No-results sentinel for the directory typeahead.",
  },
  accessDirectoryError: {
    id: "app.gateway.access.directory.error",
    defaultMessage: "The directory could not be searched",
    description: "Error title for the directory typeahead.",
  },
  accessDirectoryClear: {
    id: "app.gateway.access.directory.clear",
    defaultMessage: "Clear directory search",
    description: "Accessible label for clearing the directory search.",
  },
  accessSelectRole: {
    id: "app.gateway.access.selectRole",
    defaultMessage: "Select a role",
    description: "Label for the role radio group in the grant modal.",
  },
  accessGrant: {
    id: "app.gateway.access.grant",
    defaultMessage: "Grant access",
    description: "Confirm button for granting access.",
  },
  accessGranting: {
    id: "app.gateway.access.granting",
    defaultMessage: "Granting access...",
    description: "Spinner label while a grant is in flight.",
  },
  accessChangeRoleLabel: {
    id: "app.gateway.access.changeRole.label",
    defaultMessage: "Change role",
    description: "Accessible label for the inline role control.",
  },
  accessSelfRoleChangeTitle: {
    id: "app.gateway.access.selfRoleChange.title",
    defaultMessage: "Change your own role?",
    description:
      "Title of the confirmation modal shown when a user changes their own access role.",
  },
  accessSelfRoleChangeBody: {
    id: "app.gateway.access.selfRoleChange.body",
    defaultMessage:
      "You are about to change your own role on this gateway to {role}. This may reduce your own access. Do you want to continue?",
    description:
      "Body of the confirmation modal shown when a user changes their own access role.",
  },
  accessSelfRoleChangeConfirm: {
    id: "app.gateway.access.selfRoleChange.confirm",
    defaultMessage: "Change my role",
    description:
      "Confirm button in the modal shown when a user changes their own access role.",
  },
  accessRemove: {
    id: "app.gateway.access.remove",
    defaultMessage: "Remove access",
    description: "Action that revokes a user's access.",
  },
  accessRemoveTitle: {
    id: "app.gateway.access.remove.title",
    defaultMessage: "Remove access for {userName}?",
    description: "Title of the remove-access confirmation.",
  },
  accessRemoveBody: {
    id: "app.gateway.access.remove.body",
    defaultMessage: "This revokes the user's access to the gateway.",
    description: "Body of the remove-access confirmation.",
  },
  accessRemoving: {
    id: "app.gateway.access.removing",
    defaultMessage: "Removing access...",
    description: "Spinner label while a removal is in flight.",
  },
  accessEmptyTitle: {
    id: "app.gateway.access.empty.title",
    defaultMessage: "No access grants",
    description: "Empty-state title for the access list.",
  },
  accessEmptyBody: {
    id: "app.gateway.access.empty.body",
    defaultMessage: "No users have access to this gateway yet.",
    description: "Empty-state body for the access list.",
  },
  accessNoResultsTitle: {
    id: "app.gateway.access.noResults.title",
    defaultMessage: "No matching users",
    description: "No-results title for the access list.",
  },
  accessNoResultsBody: {
    id: "app.gateway.access.noResults.body",
    defaultMessage: "No access grants match your filters.",
    description: "No-results body for the access list.",
  },
  accessLoadError: {
    id: "app.gateway.access.loadError",
    defaultMessage: "Access could not be loaded",
    description: "Error title when the access list fails to load.",
  },
  accessLoadErrorBody: {
    id: "app.gateway.access.loadError.body",
    defaultMessage: "Try refreshing the list.",
    description: "Error body when the access list fails to load.",
  },
  accessLoading: {
    id: "app.gateway.access.loading",
    defaultMessage: "Loading access...",
    description: "Spinner label while the access list loads.",
  },
  accessRefresh: {
    id: "app.gateway.access.refresh",
    defaultMessage: "Refresh access list",
    description: "Accessible label for the access list refresh button.",
  },
  accessLastOwnerDisabled: {
    id: "app.gateway.access.lastOwnerDisabled",
    defaultMessage: "A gateway must keep at least one owner.",
    description:
      "Accessible reason shown when the last owner's controls are disabled.",
  },
  accessOwnerOnlyDisabled: {
    id: "app.gateway.access.ownerOnlyDisabled",
    defaultMessage: "Only owners can change or remove an owner.",
    description:
      "Accessible reason shown when a non-owner admin cannot act on an owner row.",
  },
  accessActionErrorLastOwner: {
    id: "app.gateway.access.actionError.lastOwner",
    defaultMessage:
      "A gateway must keep at least one owner. Assign another owner before demoting or removing this one.",
    description: "Error shown when the server rejects a last-owner mutation.",
  },
  accessActionErrorDenied: {
    id: "app.gateway.access.actionError.denied",
    defaultMessage: "You are not allowed to perform this action.",
    description: "Error shown when the server denies an access mutation.",
  },
  accessActionErrorGeneric: {
    id: "app.gateway.access.actionError.generic",
    defaultMessage: "The action could not be completed. Try again.",
    description: "Generic error shown when an access mutation fails.",
  },
  accessGrantErrorNotFound: {
    id: "app.gateway.access.grantError.notFound",
    defaultMessage: "That user was not found in the identity directory.",
    description: "Error shown when a grant target is absent from the realm.",
  },
  accessViewerReadOnly: {
    id: "app.gateway.access.viewerReadOnly",
    defaultMessage: "You have read-only access to this list.",
    description: "Notice shown to viewers who cannot manage access.",
  },
  accessWorkspaceGrantIntro: {
    id: "app.gateway.access.workspaceGrant.intro",
    defaultMessage:
      "This user cannot use the gateway until they are added to a workspace. A user with admin access on this gateway must run the below commands (change the workspace name if needed):",
    description:
      "Instructions for adding a newly granted user to an OpenShell workspace.",
  },
  accessWorkspaceLookupPending: {
    id: "app.gateway.access.workspaceGrant.lookupPending",
    defaultMessage: "Looking up the user's subject ID...",
    description:
      "Loading text shown while resolving a user's subject for the workspace command.",
  },
  accessWorkspaceSubjectUnavailable: {
    id: "app.gateway.access.workspaceGrant.subjectUnavailable",
    defaultMessage:
      "Could not determine this user's subject ID. The identity directory may be unavailable; try again.",
    description:
      "Error shown when a user's subject cannot be resolved for the workspace command.",
  },
  /* eslint-enable sort-keys */
  /* eslint-disable sort-keys -- Keep gateway version-visibility descriptors together so localization review can audit the running-version column, chip, and drift indicator as one unit. */
  version: {
    id: "app.gateway.column.version",
    defaultMessage: "Version",
    description:
      "Heading for the gateway running-version column and the Details tab version field.",
  },
  versionPlaceholder: {
    id: "app.gateway.version.placeholder",
    defaultMessage: "-",
    description:
      "Placeholder shown for a gateway that has no observed running version yet.",
  },
  versionChipAriaLabel: {
    id: "app.gateway.version.chipAriaLabel",
    defaultMessage: "Running version {version}",
    description:
      "Accessible name for the running-version chip beside the gateway status indicator.",
  },
  advancedOptions: {
    id: "app.page.gatewayProvision.advancedOptions",
    defaultMessage: "Advanced options",
    description: "Expandable section containing advanced gateway settings.",
  },
  kataQuestion: {
    id: "app.page.gatewayProvision.kataQuestion",
    defaultMessage: "Is Kata required?",
    description:
      "Question asking whether the gateway needs Kata runtime isolation.",
  },
  kataNo: {
    id: "app.page.gatewayProvision.kataNo",
    defaultMessage: "No, use the default runtime",
    description: "Option for a gateway that does not need Kata runtime.",
  },
  kataYes: {
    id: "app.page.gatewayProvision.kataYes",
    defaultMessage: "Yes, use Kata",
    description: "Option for a gateway that needs Kata runtime.",
  },
  kataUnavailableReason: {
    id: "app.page.gatewayProvision.kataUnavailableReason",
    defaultMessage: "Kata is not available for the selected placement.",
    description: "Tooltip explaining why Kata cannot be selected.",
  },
  kataFeature: {
    id: "app.page.gatewayProvision.kataFeature",
    defaultMessage: "Kata",
    description: "Recommendation-card feature label for Kata runtime.",
  },
  kataProviderDescription: {
    id: "app.page.gatewayProvision.kataProviderDescription",
    defaultMessage:
      "For workloads that require special security considerations.",
    description: "Recommendation-card description for Kata runtime placement.",
  },
  recommendationTitle: {
    id: "app.page.gatewayProvision.recommendationTitle",
    defaultMessage: "{network} {provider}",
    description: "Title for a gateway placement recommendation.",
  },
  recommendationKataTitle: {
    id: "app.page.gatewayProvision.recommendationKataTitle",
    defaultMessage: "{network} Kata {provider}",
    description: "Title for a Kata gateway placement recommendation.",
  },
  recommended: {
    id: "app.page.gatewayProvision.recommended",
    defaultMessage: "Recommended",
    description: "Label for the default gateway placement recommendation.",
  },
  specialized: {
    id: "app.page.gatewayProvision.specialized",
    defaultMessage: "Specialized",
    description:
      "Label for a gateway placement requiring specialized runtime support.",
  },
  gatewaySetFeatures: {
    id: "app.page.gatewayProvision.gatewaySetFeatures",
    defaultMessage: "Gateway set features",
    description: "Accessible label for recommendation card feature labels.",
  },
  awsProviderAlt: {
    id: "app.page.gatewayProvision.awsProviderAlt",
    defaultMessage: "Amazon Web Services",
    description: "Alternative text for the AWS provider logo.",
  },
  ibmProviderAlt: {
    id: "app.page.gatewayProvision.ibmProviderAlt",
    defaultMessage: "IBM Cloud",
    description: "Alternative text for the IBM Cloud provider logo.",
  },
  details: {
    id: "app.gateway.details.heading",
    defaultMessage: "Details",
    description: "Heading for the gateway Details tab content.",
  },
  detailsTabDescription: {
    id: "app.gateway.details.description",
    defaultMessage: "Review the gateway's current state and configuration.",
    description: "Description for the gateway Details tab.",
  },
  overview: {
    id: "app.gateway.details.overview",
    defaultMessage: "Overview",
    description: "Heading for the gateway Overview card.",
  },
  identity: {
    id: "app.gateway.details.identity",
    defaultMessage: "Identity",
    description: "Heading for the gateway Identity card.",
  },
  placement: {
    id: "app.gateway.details.placement",
    defaultMessage: "Placement",
    description: "Heading for the gateway Placement card.",
  },
  refreshGatewayDetails: {
    id: "app.gateway.details.refresh",
    defaultMessage: "Refresh gateway details",
    description: "Accessible label for the refresh gateway details button.",
  },
  gatewayId: {
    id: "app.gateway.details.gatewayId",
    defaultMessage: "Gateway ID",
    description: "Label for the gateway identifier field.",
  },
  connectionTabDescription: {
    id: "app.gateway.connection.description",
    defaultMessage:
      "Connect your terminal to this gateway and start a Claude sandbox attached to EnMaaS.",
    description: "Description for the gateway Connection tab.",
  },
  automateThisGateway: {
    id: "app.gateway.connection.automateThisGateway",
    defaultMessage: "Automate this gateway",
    description: "Heading for the automation entry point card.",
  },
  automateThisGatewayDescription: {
    id: "app.gateway.connection.automateThisGatewayDescription",
    defaultMessage: "CI/CD pipelines and bots should use service accounts.",
    description: "Description for the automation entry point card.",
  },
  viewServiceAccounts: {
    id: "app.gateway.connection.viewServiceAccounts",
    defaultMessage: "View service accounts",
    description: "Link to the service accounts tab from the automation card.",
  },
  /* eslint-enable sort-keys */
});
