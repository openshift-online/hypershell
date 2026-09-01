import { defineMessages } from "react-intl";

export const gatewayProfileMessages = defineMessages({
  clearGatewayProfileSearch: {
    id: "app.gatewayProfile.clearSearch",
    defaultMessage: "Clear gateway profile search",
    description:
      "Accessible label for clearing the gateway profile typeahead input.",
  },
  containerCpuLimitMax: {
    id: "app.gatewayProfile.containerCpuLimitMax",
    defaultMessage: "Container CPU limit (max)",
    description:
      "Label for the maximum CPU limit allowed on a single container.",
  },
  containerCpuRequestDefault: {
    id: "app.gatewayProfile.containerCpuRequestDefault",
    defaultMessage: "Container CPU request (default)",
    description: "Label for the default CPU request applied to a container.",
  },
  containerMemoryLimitMax: {
    id: "app.gatewayProfile.containerMemoryLimitMax",
    defaultMessage: "Container memory limit (max)",
    description:
      "Label for the maximum memory limit allowed on a single container.",
  },
  containerMemoryRequestDefault: {
    id: "app.gatewayProfile.containerMemoryRequestDefault",
    defaultMessage: "Container memory request (default)",
    description: "Label for the default memory request applied to a container.",
  },
  cpuLimitTotal: {
    id: "app.gatewayProfile.cpuLimitTotal",
    defaultMessage: "Total CPU limit",
    description:
      "Label for the total CPU limit across a gateway profile quota.",
  },
  cpuRequestTotal: {
    id: "app.gatewayProfile.cpuRequestTotal",
    defaultMessage: "Total CPU request",
    description:
      "Label for the total CPU request across a gateway profile quota.",
  },
  createGatewayProfile: {
    id: "app.page.gatewayProfileCreate.title",
    defaultMessage: "Create gateway profile",
    description: "Page title and action for creating a gateway profile.",
  },
  createGatewayProfileDescription: {
    id: "app.page.gatewayProfileCreate.description",
    defaultMessage: "Configure a new gateway resource quota profile.",
    description: "Supporting text on the gateway profile creation page.",
  },
  creatingGatewayProfile: {
    id: "app.page.gatewayProfileCreate.pending",
    defaultMessage: "Creating gateway profile",
    description:
      "Accessible progress text while a gateway profile is being created.",
  },
  deleteGatewayProfile: {
    id: "app.gatewayProfile.delete",
    defaultMessage: "Delete gateway profile",
    description: "Action that permanently deletes a gateway profile.",
  },
  deleteGatewayProfileConfirmation: {
    id: "app.gatewayProfile.delete.confirmation",
    defaultMessage:
      "Deleting {gatewayProfileName} will permanently remove the gateway profile. This action cannot be undone.",
    description: "Warning shown before permanently deleting a gateway profile.",
  },
  deleteGatewayProfileTitle: {
    id: "app.gatewayProfile.delete.title",
    defaultMessage: "Delete {gatewayProfileName}?",
    description: "Title for the gateway profile deletion confirmation dialog.",
  },
  deletingGatewayProfile: {
    id: "app.gatewayProfile.delete.pending",
    defaultMessage: "Deleting gateway profile",
    description:
      "Accessible progress text while a gateway profile is being deleted.",
  },
  ephemeralStorageTotal: {
    id: "app.gatewayProfile.ephemeralStorageTotal",
    defaultMessage: "Total ephemeral storage",
    description:
      "Label for the total ephemeral storage across a gateway profile quota.",
  },
  filterGatewayProfiles: {
    id: "app.page.gatewayProfiles.filter",
    defaultMessage: "Filter by name",
    description: "Placeholder and accessible label for gateway profile search.",
  },
  gatewayProfile: {
    id: "app.gatewayProfile.singular",
    defaultMessage: "Gateway profile",
    description:
      "Fallback breadcrumb label while a gateway profile is loading.",
  },
  gatewayProfileDeleteConflict: {
    id: "app.gatewayProfile.delete.conflict.title",
    defaultMessage: "Gateway profile is in use",
    description:
      "Title shown when a gateway profile cannot be deleted because gateways reference it.",
  },
  gatewayProfileDeleteConflictBody: {
    id: "app.gatewayProfile.delete.conflict.body",
    defaultMessage:
      "Reassign or delete the gateways that use this profile before deleting it.",
    description:
      "Recovery guidance when a gateway profile is still referenced by gateways.",
  },
  gatewayProfileDeleted: {
    id: "app.gatewayProfile.deleted",
    defaultMessage: "Gateway profile {gatewayProfileName} deleted",
    description: "Success notification after deleting a gateway profile.",
  },
  gatewayProfileDeleteError: {
    id: "app.gatewayProfile.delete.error.title",
    defaultMessage: "Gateway profile could not be deleted",
    description: "Title shown when gateway profile deletion fails.",
  },
  gatewayProfileDeleteErrorBody: {
    id: "app.gatewayProfile.delete.error.body",
    defaultMessage: "No changes were made. Try again.",
    description: "Recovery guidance when gateway profile deletion fails.",
  },
  gatewayProfileDescription: {
    id: "app.gatewayProfile.description",
    defaultMessage: "Description",
    description: "Label for a gateway profile description field and column.",
  },
  gatewayProfileDetailDescription: {
    id: "app.page.gatewayProfileDetails.description",
    defaultMessage: "Review this gateway profile's resource quota.",
    description: "Metadata description for the gateway profile detail page.",
  },
  gatewayProfileDetailsTitle: {
    id: "app.page.gatewayProfileDetails.title",
    defaultMessage: "Gateway profile details",
    description: "Metadata title for a gateway profile details page.",
  },
  gatewayProfileLoadError: {
    id: "app.gatewayProfile.load.error.title",
    defaultMessage: "Gateway profiles could not be loaded",
    description:
      "Title shown when gateway profile data cannot be loaded from the API.",
  },
  gatewayProfileLoadErrorBody: {
    id: "app.gatewayProfile.load.error.body",
    defaultMessage: "Refresh the page to try again.",
    description:
      "Recovery guidance when gateway profile data cannot be loaded.",
  },
  gatewayProfileName: {
    id: "app.gatewayProfile.name",
    defaultMessage: "Profile name",
    description: "Label for a gateway profile name field and column.",
  },
  gatewayProfileNone: {
    id: "app.gatewayProfile.none",
    defaultMessage: "No profile",
    description:
      "Option and value indicating that no gateway profile is selected.",
  },
  gatewayProfiles: {
    id: "app.nav.gatewayProfiles",
    defaultMessage: "Gateway profiles",
    description:
      "Page and resource collection label for gateway resource quota profiles.",
  },
  gatewayProfilesDescription: {
    id: "app.page.gatewayProfiles.description",
    defaultMessage: "HyperShell gateway resource quota profiles.",
    description: "Browser metadata description for the gateway profiles page.",
  },
  gatewayProfilesEmptyBody: {
    id: "app.page.gatewayProfiles.empty.body",
    defaultMessage: "Created gateway profiles will appear here.",
    description: "Body text when the gateway profile list is empty.",
  },
  gatewayProfilesEmptyTitle: {
    id: "app.page.gatewayProfiles.empty.title",
    defaultMessage: "No gateway profiles",
    description: "Heading when the gateway profile list is empty.",
  },
  gatewayProfilesProvisionError: {
    id: "app.page.gatewayProfileCreate.error.title",
    defaultMessage: "Gateway profile could not be created",
    description: "Title shown when gateway profile creation fails.",
  },
  gatewayProfilesProvisionErrorBody: {
    id: "app.page.gatewayProfileCreate.error.body",
    defaultMessage: "Check the values and try again.",
    description: "Recovery guidance when gateway profile creation fails.",
  },
  gatewayProfileRowActions: {
    id: "app.gatewayProfile.rowActions",
    defaultMessage: "Actions for {gatewayProfileName}",
    description: "Accessible label for a gateway profile row actions menu.",
  },
  loadingGatewayProfiles: {
    id: "app.gatewayProfile.loading",
    defaultMessage: "Loading gateway profiles",
    description:
      "Accessible label shown while gateway profile data is loading.",
  },
  memoryLimitTotal: {
    id: "app.gatewayProfile.memoryLimitTotal",
    defaultMessage: "Total memory limit",
    description:
      "Label for the total memory limit across a gateway profile quota.",
  },
  memoryRequestTotal: {
    id: "app.gatewayProfile.memoryRequestTotal",
    defaultMessage: "Total memory request",
    description:
      "Label for the total memory request across a gateway profile quota.",
  },
  moreGatewayProfilesAvailable: {
    id: "app.gatewayProfile.moreResults",
    defaultMessage:
      "More gateway profiles are available. Refine your search to find a specific profile.",
    description:
      "Guidance when a bounded gateway profile search has additional API results.",
  },
  noMatchingGatewayProfiles: {
    id: "app.page.gatewayProfiles.noResults.title",
    defaultMessage: "No matching gateway profiles",
    description: "Heading when no gateway profiles match the current filter.",
  },
  noMatchingGatewayProfilesBody: {
    id: "app.page.gatewayProfiles.noResults.body",
    defaultMessage: "Adjust or clear the filter to see gateway profiles.",
    description: "Guidance when no gateway profiles match the current filter.",
  },
  podCount: {
    id: "app.gatewayProfile.podCount",
    defaultMessage: "Pods",
    description: "Label for the maximum pod count in a gateway profile quota.",
  },
  pvcCount: {
    id: "app.gatewayProfile.pvcCount",
    defaultMessage: "Persistent volume claims",
    description:
      "Label for the maximum persistent volume claim count in a gateway profile quota.",
  },
  quotaHeading: {
    id: "app.gatewayProfile.quota.heading",
    defaultMessage: "Resource quota",
    description: "Heading for the gateway profile resource quota section.",
  },
  refreshGatewayProfiles: {
    id: "app.gatewayProfiles.refresh",
    defaultMessage: "Refresh gateway profiles",
    description: "Accessible label for refreshing the gateway profile list.",
  },
  selectGatewayProfile: {
    id: "app.gatewayProfile.select",
    defaultMessage: "Select a gateway profile",
    description:
      "Accessible label and placeholder for the gateway profile selector.",
  },
});
