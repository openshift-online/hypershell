import { defineMessages } from "react-intl";

export const messages = defineMessages({
  actions: {
    id: "app.table.column.actions",
    defaultMessage: "Actions",
    description: "Accessible heading for table row actions.",
  },
  administration: {
    id: "app.nav.administration",
    defaultMessage: "Administration",
    description: "Navigation and page label for HyperShell administration.",
  },
  adminGatewayDescription: {
    id: "app.page.adminGateway.description",
    defaultMessage:
      "Review this gateway's placement, configuration, and operational status.",
    description: "Supporting text on the administrative gateway detail page.",
  },
  adminGatewayDetails: {
    id: "app.page.adminGateway.title",
    defaultMessage: "Gateway administration",
    description: "Title for the administrative gateway detail page.",
  },
  adminGatewayEmptyBody: {
    id: "app.page.adminGateway.empty.body",
    defaultMessage:
      "Gateway placement and configuration will appear here when the API is connected.",
    description: "Body text on the administrative gateway detail empty state.",
  },
  adminGatewayEmptyTitle: {
    id: "app.page.adminGateway.empty.title",
    defaultMessage: "Gateway configuration is not available",
    description: "Heading on the administrative gateway detail empty state.",
  },
  adminGatewaysCardBody: {
    id: "app.page.adminOverview.gateways.body",
    defaultMessage: "Provision and manage gateways on available clusters.",
    description: "Body text for the gateways card on the admin overview.",
  },
  adminGatewaysDescription: {
    id: "app.page.adminGateways.description",
    defaultMessage:
      "Provision gateways and manage their infrastructure placement.",
    description: "Supporting text on the administrative gateways page.",
  },
  adminGatewaysEmptyBody: {
    id: "app.page.adminGateways.empty.body",
    defaultMessage: "Provisioned gateways will appear here.",
    description: "Body text when the administrative gateway list is empty.",
  },
  adminGatewaysEmptyTitle: {
    id: "app.page.adminGateways.empty.title",
    defaultMessage: "No gateways to administer",
    description: "Heading when the administrative gateway list is empty.",
  },
  adminOverviewDescription: {
    id: "app.page.adminOverview.description",
    defaultMessage:
      "Configure the infrastructure used to provision OpenShell gateways.",
    description: "Supporting text on the HyperShell administration overview.",
  },
  adminProductName: {
    id: "app.adminProductName",
    defaultMessage: "HyperShell Administration",
    description: "Product name shown in the administrative shell.",
  },
  breadcrumbLabel: {
    id: "app.breadcrumb.ariaLabel",
    defaultMessage: "Breadcrumb",
    description: "Accessible label for the application breadcrumb navigation.",
  },
  cancel: {
    id: "app.action.cancel",
    defaultMessage: "Cancel",
    description: "Action that leaves a form without submitting it.",
  },
  clearFilters: {
    id: "app.table.clearFilters",
    defaultMessage: "Clear filters",
    description: "Action that clears collection filters.",
  },
  clusters: {
    id: "app.nav.clusters",
    defaultMessage: "Clusters",
    description: "Navigation and page label for managed clusters.",
  },
  cluster: {
    id: "app.gateway.cluster",
    defaultMessage: "Cluster",
    description: "Label for the selected gateway placement cluster.",
  },
  clusterActionsColumn: {
    id: "app.page.clusters.column.actions",
    defaultMessage: "Actions",
    description: "Accessible heading for cluster row actions.",
  },
  clusterDescriptionColumn: {
    id: "app.page.clusters.column.description",
    defaultMessage: "Description",
    description: "Heading for the cluster description column.",
  },
  clusterNameColumn: {
    id: "app.page.clusters.column.name",
    defaultMessage: "Name",
    description: "Heading for the cluster name column.",
  },
  clustersCardBody: {
    id: "app.page.adminOverview.clusters.body",
    defaultMessage: "Configure clusters available for gateway placement.",
    description: "Body text for the clusters card on the admin overview.",
  },
  clustersDescription: {
    id: "app.page.clusters.description",
    defaultMessage:
      "View the cluster currently available for gateway placement.",
    description: "Supporting text on the administrative clusters page.",
  },
  connectWithCli: {
    id: "app.gateway.connectWithCli",
    defaultMessage: "Connect with the CLI",
    description: "Heading above a gateway's OpenShell CLI connection command.",
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
  copyConnectionCommand: {
    id: "app.gateway.copyConnectionCommand",
    defaultMessage: "Copy connection command for {gatewayName}",
    description:
      "Accessible label for copying a specific gateway's CLI connection command.",
  },
  errorBody: {
    id: "app.error.body",
    defaultMessage: "Refresh the page to try again.",
    description: "Recovery guidance shown after an unexpected route failure.",
  },
  errorTitle: {
    id: "app.error.title",
    defaultMessage: "The page could not be loaded",
    description: "Title shown after an unexpected route failure.",
  },
  filterClusters: {
    id: "app.page.clusters.filter",
    defaultMessage: "Filter by name or description",
    description: "Placeholder and accessible label for cluster search.",
  },
  filterGateways: {
    id: "app.page.adminGateways.filter",
    defaultMessage: "Filter by name, status, or endpoint",
    description: "Placeholder and accessible label for gateway search.",
  },
  gatewayContext: {
    id: "app.breadcrumb.gatewayContext",
    defaultMessage: "Gateway {gatewayId}",
    description:
      "Breadcrumb label identifying an administrative gateway by its route identifier.",
  },
  gatewayDetailsDescription: {
    id: "app.page.gatewayDetails.description",
    defaultMessage:
      "Open the gateway console or add this gateway to the OpenShell CLI.",
    description: "Supporting text on the user-facing gateway details page.",
  },
  gatewayDetailsTitle: {
    id: "app.page.gatewayDetails.title",
    defaultMessage: "Gateway details",
    description: "Metadata title for a user-facing gateway details page.",
  },
  gatewayDirectory: {
    id: "app.nav.gatewayDirectory",
    defaultMessage: "Gateway directory",
    description:
      "Prominent navigation from administration to the user-facing gateway directory.",
  },
  gatewayDirectoryDescription: {
    id: "app.page.gatewayDirectory.description",
    defaultMessage:
      "Choose a gateway to open its console or connect with the OpenShell CLI.",
    description: "Supporting text on the user-facing gateway directory.",
  },
  gatewayEndpoint: {
    id: "app.gateway.endpoint",
    defaultMessage: "Gateway endpoint",
    description: "Label for an OpenShell gateway network endpoint.",
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
    description: "Label for the gateway release identifier form field.",
  },
  gateways: {
    id: "app.nav.gateways",
    defaultMessage: "Gateways",
    description: "Navigation and page label for gateways.",
  },
  getStarted: {
    id: "app.gateway.getStarted",
    defaultMessage: "Get started",
    description: "Heading for gateway connection methods.",
  },
  helloDescription: {
    id: "app.hello.description",
    defaultMessage: "The HyperShell web console is ready for development.",
    description: "Supporting text on the initial web-console landing page.",
  },
  helloTitle: {
    id: "app.hello.title",
    defaultMessage: "Hello world",
    description: "Main heading on the initial web-console landing page.",
  },
  managedDatabaseId: {
    id: "app.gateway.managedDatabaseId",
    defaultMessage: "Managed database ID",
    description: "Label for the managed database identifier form field.",
  },
  navigationToggle: {
    id: "app.nav.toggle",
    defaultMessage: "Toggle primary navigation",
    description:
      "Accessible label for the button that opens and closes the sidebar.",
  },
  namespace: {
    id: "app.gateway.namespace",
    defaultMessage: "Namespace",
    description: "Label for a gateway namespace form field.",
  },
  noGatewaysAvailable: {
    id: "app.page.gatewayDirectory.empty.title",
    defaultMessage: "No gateways available",
    description: "Heading when a user cannot see any OpenShell gateways.",
  },
  noGatewaysAvailableBody: {
    id: "app.page.gatewayDirectory.empty.body",
    defaultMessage: "Ask your OpenShell administrator for access to a gateway.",
    description: "Guidance when a user cannot see any OpenShell gateways.",
  },
  noClustersAvailable: {
    id: "app.page.clusters.empty.title",
    defaultMessage: "No clusters available",
    description: "Heading when no clusters are available for placement.",
  },
  noClustersAvailableBody: {
    id: "app.page.clusters.empty.body",
    defaultMessage:
      "Clusters available for gateway placement will appear here.",
    description: "Guidance when no clusters are available for placement.",
  },
  noMatchingClusters: {
    id: "app.page.clusters.noResults.title",
    defaultMessage: "No matching clusters",
    description: "Heading when no clusters match the current filter.",
  },
  noMatchingClustersBody: {
    id: "app.page.clusters.noResults.body",
    defaultMessage: "Adjust or clear the filter to see clusters.",
    description: "Guidance when no clusters match the current filter.",
  },
  noMatchingGateways: {
    id: "app.page.adminGateways.noResults.title",
    defaultMessage: "No matching gateways",
    description: "Heading when no gateways match the current filter.",
  },
  noMatchingGatewaysBody: {
    id: "app.page.adminGateways.noResults.body",
    defaultMessage: "Adjust or clear the filter to see gateways.",
    description: "Guidance when no gateways match the current filter.",
  },
  notFoundBody: {
    id: "app.notFound.body",
    defaultMessage: "Check the address and try again.",
    description:
      "Recovery guidance shown when an application route does not exist.",
  },
  notFoundTitle: {
    id: "app.notFound.title",
    defaultMessage: "Page not found",
    description: "Heading shown when an application route does not exist.",
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
  openAdministration: {
    id: "app.nav.openAdministration",
    defaultMessage: "Administration",
    description: "Subtle navigation from OpenShell to administration.",
  },
  openShellGateways: {
    id: "app.page.gatewayDirectory.title",
    defaultMessage: "OpenShell gateways",
    description: "Main heading on the user-facing gateway directory.",
  },
  overview: {
    id: "app.nav.overview",
    defaultMessage: "Overview",
    description: "Navigation label for the administration overview.",
  },
  primaryNavigation: {
    id: "app.nav.primary",
    defaultMessage: "Primary navigation",
    description: "Accessible label for the administrative sidebar navigation.",
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
  productName: {
    id: "app.productName",
    defaultMessage: "HyperShell",
    description: "HyperShell product name.",
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
  skipToContent: {
    id: "app.skipToContent",
    defaultMessage: "Skip to content",
    description:
      "Accessibility link that moves focus to the main page content.",
  },
  status: {
    id: "app.table.column.status",
    defaultMessage: "Status",
    description: "Heading for a resource status column.",
  },
  viewAdminGateways: {
    id: "app.page.adminOverview.gateways.action",
    defaultMessage: "View gateways",
    description: "Link from the admin overview to gateway administration.",
  },
  viewClusters: {
    id: "app.page.adminOverview.clusters.action",
    defaultMessage: "View clusters",
    description: "Link from the admin overview to cluster administration.",
  },
  viewGatewayDetails: {
    id: "app.gateway.viewDetails",
    defaultMessage: "View details",
    description: "Link to a user-facing gateway detail page.",
  },
});
