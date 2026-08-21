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
      "You can provision on Hub cluster (default), or try loading managed clusters again.",
    description:
      "Recovery guidance when remote gateway placements cannot be loaded.",
  },
  clusterProvider: {
    id: "app.gateway.cluster.provider",
    defaultMessage: "Provider: {provider}",
    description:
      "Provider context that distinguishes a managed cluster option.",
  },
  clusterProviderAndRegion: {
    id: "app.gateway.cluster.providerAndRegion",
    defaultMessage: "Provider: {provider}; region: {region}",
    description:
      "Provider and region context that distinguishes a managed cluster option.",
  },
  clusterRegion: {
    id: "app.gateway.cluster.region",
    defaultMessage: "Region: {region}",
    description: "Region context that distinguishes a managed cluster option.",
  },
  connectionInstallLink: {
    id: "app.gateway.connection.installLink",
    defaultMessage: "Install the OpenShell CLI",
    description:
      "Link text pointing to the NVIDIA OpenShell installation documentation.",
  },
  connectionInstallLinkNewTab: {
    id: "app.gateway.connection.installLinkNewTab",
    defaultMessage: "Install the OpenShell CLI (opens in a new tab)",
    description:
      "Accessible name for the OpenShell CLI install docs link, including that it opens in a new tab.",
  },
  connectionInstallPrereq: {
    id: "app.gateway.connection.installPrereq",
    defaultMessage:
      "The OpenShell CLI must be installed before running the commands below.",
    description:
      "Prerequisite note shown above the connection steps directing operators to install the CLI first.",
  },
  connectionInstallPrereqTitle: {
    id: "app.gateway.connection.installPrereqTitle",
    defaultMessage: "Prerequisite",
    description: "Title for the CLI installation prerequisite alert.",
  },
  connectionLoginUnavailable: {
    id: "app.gateway.connection.login.unavailable",
    defaultMessage:
      "This gateway is still provisioning. Its connection command becomes available once the gateway is running.",
    description:
      "Shown in the login step while the gateway has not yet reached a running, ready-to-connect phase.",
  },
  connectionSandboxDescription: {
    id: "app.gateway.connection.sandbox.description",
    defaultMessage:
      "Once setup is done, run this whenever you want a fresh sandbox running Claude through this gateway.",
    description: "Supporting text for the create-sandbox connection step.",
  },
  connectionSandboxTitle: {
    id: "app.gateway.connection.sandbox.title",
    defaultMessage: "Create a sandbox",
    description: "Title for the create-sandbox connection step.",
  },
  connectionSetupDescription: {
    id: "app.gateway.connection.setup.description",
    defaultMessage:
      "Run these once to log in, add the Claude on Vertex AI provider, and select the model.",
    description: "Supporting text for the one-time setup connection step.",
  },
  connectionSetupTitle: {
    id: "app.gateway.connection.setup.title",
    defaultMessage: "One-time setup",
    description: "Title for the consolidated one-time setup connection step.",
  },
  connectionTab: {
    id: "app.gateway.connection.tab",
    defaultMessage: "Connection",
    description: "Label for the default gateway detail Connection tab.",
  },
  connectionTabsLabel: {
    id: "app.gateway.connection.tabsLabel",
    defaultMessage: "Gateway connection and details",
    description: "Accessible label for the gateway detail tabs.",
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
  copyGatewayEndpoint: {
    id: "app.gateway.copyEndpoint",
    defaultMessage: "Copy gateway endpoint for {gatewayName}",
    description:
      "Accessible label for copying a specific gateway's network endpoint.",
  },
  copySandboxCommand: {
    id: "app.gateway.connection.copySandboxCommand",
    defaultMessage: "Copy the create-sandbox command",
    description: "Accessible label for copying the create-sandbox command.",
  },
  copySetupCommand: {
    id: "app.gateway.connection.copySetupCommand",
    defaultMessage: "Copy the one-time setup commands",
    description:
      "Accessible label for copying the consolidated one-time setup script.",
  },
  created: {
    id: "app.table.column.created",
    defaultMessage: "Created",
    description: "Heading for a resource creation date column.",
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
    defaultMessage: "Filter by name, cluster, status, or endpoint",
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
  gatewayReleaseId: {
    id: "app.gateway.releaseId",
    defaultMessage: "Gateway release ID",
    description: "Label for a gateway's release identifier.",
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
  hubCluster: {
    id: "app.gateway.cluster.hub",
    defaultMessage: "Hub cluster",
    description: "Placement label for the cluster hosting HyperShell.",
  },
  hubClusterDefault: {
    id: "app.gateway.cluster.hubDefault",
    defaultMessage: "Hub cluster (default)",
    description: "Default placement option for a gateway provisioning form.",
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
  managedDatabaseId: {
    id: "app.gateway.managedDatabaseId",
    defaultMessage: "Managed database ID",
    description: "Label for a gateway's managed database identifier.",
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
  owner: {
    id: "app.gateway.column.owner",
    defaultMessage: "Created by",
    description:
      "Heading for the gateways table column showing who created the gateway.",
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
  renameGateway: {
    id: "app.gateway.rename",
    defaultMessage: "Rename gateway",
    description: "Action that changes a gateway's name.",
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
  status: {
    id: "app.table.column.status",
    defaultMessage: "Status",
    description: "Heading for a resource status column.",
  },
  unavailableGatewayConsole: {
    id: "app.gateway.unavailableConsole",
    defaultMessage: "Console unavailable for this gateway",
    description:
      "Tooltip on the disabled console button once the console failed to become available within the expected time.",
  },
});
