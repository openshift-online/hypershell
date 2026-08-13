import { defineMessages } from "react-intl";

export const messages = defineMessages({
  actions: {
    id: "app.table.column.actions",
    defaultMessage: "Actions",
    description: "Accessible heading for table row actions.",
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
  connectionLoginDescription: {
    id: "app.gateway.connection.login.description",
    defaultMessage: "Authenticate the OpenShell CLI to this gateway.",
    description: "Supporting text for the gateway login connection step.",
  },
  connectionLoginTitle: {
    id: "app.gateway.connection.login.title",
    defaultMessage: "Log in to the gateway",
    description: "Title for the first gateway connection step.",
  },
  connectionLoginUnavailable: {
    id: "app.gateway.connection.login.unavailable",
    defaultMessage:
      "Gateway login is unavailable until this gateway reports its endpoint and OIDC connection details.",
    description:
      "Shown in the login step when the gateway lacks required connection values.",
  },
  connectionProviderAdcDescription: {
    id: "app.gateway.connection.provider.adc.description",
    defaultMessage:
      "Run this once so the gateway can obtain your Google Cloud credentials.",
    description:
      "Explains the Application Default Credentials prerequisite command.",
  },
  connectionProviderAdcTitle: {
    id: "app.gateway.connection.provider.adc.title",
    defaultMessage: "Configure Application Default Credentials",
    description: "Title for the ADC prerequisite within the provider step.",
  },
  connectionProviderCaveat: {
    id: "app.gateway.connection.provider.caveat",
    defaultMessage:
      "Inside the sandbox, reach Vertex AI through inference.local. Do not set CLAUDE_CODE_USE_VERTEX=1, which makes Claude Code bypass the gateway and fail Google Cloud credential discovery.",
    description: "Sandbox routing caveat shown in the provider step details.",
  },
  connectionProviderDescription: {
    id: "app.gateway.connection.provider.description",
    defaultMessage:
      "Register Claude on Google Vertex AI using credentials pulled from your environment. No key is entered in the browser.",
    description: "Supporting text for the add-provider connection step.",
  },
  connectionProviderDetailsToggle: {
    id: "app.gateway.connection.provider.detailsToggle",
    defaultMessage: "Prerequisites and options",
    description:
      "Toggle label for the expandable provider prerequisites and options.",
  },
  connectionProviderFromEnvDescription: {
    id: "app.gateway.connection.provider.fromEnv.description",
    defaultMessage:
      "Reads the credential plus VERTEX_AI_PROJECT_ID and VERTEX_AI_REGION from your shell. These OpenShell variables differ from Claude Code's ANTHROPIC_VERTEX_PROJECT_ID and CLOUD_ML_REGION.",
    description:
      "Explains the environment-variable alternative for creating the provider.",
  },
  connectionProviderFromEnvTitle: {
    id: "app.gateway.connection.provider.fromEnv.title",
    defaultMessage: "Or create the provider from environment variables",
    description:
      "Title for the environment-variable alternative in the provider step.",
  },
  connectionProviderRoutingDescription: {
    id: "app.gateway.connection.provider.routing.description",
    defaultMessage: "Replace {model} with a Vertex Claude model ID.",
    description: "Explains the model-routing command in the provider step.",
  },
  connectionProviderRoutingTitle: {
    id: "app.gateway.connection.provider.routing.title",
    defaultMessage: "Route a Claude model through the provider",
    description: "Title for the model-routing command in the provider step.",
  },
  connectionProviderTitle: {
    id: "app.gateway.connection.provider.title",
    defaultMessage: "Add a Claude on Vertex AI provider",
    description: "Title for the second gateway connection step.",
  },
  connectionSandboxDescription: {
    id: "app.gateway.connection.sandbox.description",
    defaultMessage:
      "Start a sandbox that runs Claude through this gateway. Replace {sandbox} with a name.",
    description: "Supporting text for the create-sandbox connection step.",
  },
  connectionSandboxTitle: {
    id: "app.gateway.connection.sandbox.title",
    defaultMessage: "Create a sandbox",
    description: "Title for the third gateway connection step.",
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
  copyAdcLoginCommand: {
    id: "app.gateway.connection.copyAdcLoginCommand",
    defaultMessage: "Copy the credentials login command",
    description: "Accessible label for copying the ADC login command.",
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
  copyInferenceCommand: {
    id: "app.gateway.connection.copyInferenceCommand",
    defaultMessage: "Copy the model-routing command",
    description: "Accessible label for copying the model-routing command.",
  },
  copyProviderCommand: {
    id: "app.gateway.connection.copyProviderCommand",
    defaultMessage: "Copy the add-provider command",
    description: "Accessible label for copying the add-provider command.",
  },
  copyProviderFromExistingCommand: {
    id: "app.gateway.connection.copyProviderFromExistingCommand",
    defaultMessage: "Copy the environment-based add-provider command",
    description:
      "Accessible label for copying the environment-based add-provider command.",
  },
  copySandboxCommand: {
    id: "app.gateway.connection.copySandboxCommand",
    defaultMessage: "Copy the create-sandbox command",
    description: "Accessible label for copying the create-sandbox command.",
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
    defaultMessage:
      "Review this gateway's configuration and operational status.",
    description: "Supporting text on the gateway detail page.",
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
});
