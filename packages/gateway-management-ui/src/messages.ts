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
    defaultMessage: "View installation documentation",
    description:
      "Link text pointing to the NVIDIA OpenShell installation documentation.",
  },
  connectionInstallLinkNewTab: {
    id: "app.gateway.connection.installLinkNewTab",
    defaultMessage: "View installation documentation (opens in a new tab)",
    description:
      "Accessible name for the OpenShell CLI installation documentation link, including that it opens in a new tab.",
  },
  connectionInstallPrereq: {
    id: "app.gateway.connection.installPrereq",
    defaultMessage:
      "Install the OpenShell CLI version for this gateway before you add the provider.",
    description: "Prerequisite note shown before the one-time setup commands.",
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
      "Run these commands in order to register the gateway, add the Claude on Vertex AI provider, and select the model.",
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
    defaultMessage: "Gateway connection, service accounts, and details",
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
  copySetupCommand: {
    id: "app.gateway.connection.copySetupCommand",
    defaultMessage: "Copy the one-time setup commands",
    description:
      "Accessible label for copying the gateway registration, provider creation, and model selection commands.",
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
  /* eslint-enable sort-keys */
  unavailableGatewayConsole: {
    id: "app.gateway.unavailableConsole",
    defaultMessage: "Console unavailable for this gateway",
    description:
      "Tooltip on the disabled console button once the console failed to become available within the expected time.",
  },
});
