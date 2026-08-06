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
      "Review this gateway's configuration and operational status.",
    description: "Supporting text on the administrative gateway detail page.",
  },
  adminGatewayDetails: {
    id: "app.page.adminGateway.title",
    defaultMessage: "Gateway administration",
    description: "Title for the administrative gateway detail page.",
  },
  adminGatewaysDescription: {
    id: "app.page.adminGateways.description",
    defaultMessage: "Provision and manage OpenShell gateways.",
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
  cluster: {
    id: "app.gateway.cluster",
    defaultMessage: "Cluster",
    description: "Heading for the gateway placement cluster column.",
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
  filterGateways: {
    id: "app.page.adminGateways.filter",
    defaultMessage: "Filter by name, cluster, status, or endpoint",
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
    description: "Label for a gateway's managed database identifier.",
  },
  loadingGateways: {
    id: "app.gateway.loading",
    defaultMessage: "Loading gateways",
    description: "Accessible label shown while gateway data is loading.",
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
  refreshGateways: {
    id: "app.gateways.refresh",
    defaultMessage: "Refresh gateways",
    description: "Accessible label for refreshing the gateway list.",
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
  viewGatewayDetails: {
    id: "app.gateway.viewDetails",
    defaultMessage: "View details",
    description: "Link to a user-facing gateway detail page.",
  },
});
