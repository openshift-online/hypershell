import { defineMessages } from "react-intl";

export const messages = defineMessages({
  breadcrumbLabel: {
    id: "app.breadcrumb.ariaLabel",
    defaultMessage: "Breadcrumb",
    description: "Accessible label for the application breadcrumb navigation.",
  },
  clients: {
    id: "app.nav.clients",
    defaultMessage: "Clients",
    description: "Page and breadcrumb label for gateway clients.",
  },
  clientsDescription: {
    id: "app.page.clients.description",
    defaultMessage: "Review clients associated with this sector.",
    description: "Supporting text on the clients scaffold page.",
  },
  clientsEmptyBody: {
    id: "app.page.clients.empty.body",
    defaultMessage: "When client data is available, it will appear here.",
    description: "Body text shown when the clients scaffold has no data.",
  },
  clientsEmptyTitle: {
    id: "app.page.clients.empty.title",
    defaultMessage: "No clients to display",
    description: "Empty-state heading on the clients scaffold page.",
  },
  developmentPreview: {
    id: "app.environment.developmentPreview",
    defaultMessage: "Development preview",
    description:
      "Label identifying that the current console is a development preview.",
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
  gatewayDescription: {
    id: "app.page.gateway.description",
    defaultMessage: "Review gateway configuration, placement, and status.",
    description: "Supporting text on the gateway details scaffold page.",
  },
  gatewayContext: {
    id: "app.breadcrumb.gatewayContext",
    defaultMessage: "Gateway {gatewayId}",
    description:
      "Breadcrumb label identifying the selected gateway by its route identifier.",
  },
  gatewayDetails: {
    id: "app.page.gateway.title",
    defaultMessage: "Gateway details",
    description: "Page and breadcrumb title for an individual gateway.",
  },
  gatewayEmptyBody: {
    id: "app.page.gateway.empty.body",
    defaultMessage:
      "Gateway configuration and operational status will appear here.",
    description: "Body text shown on the gateway details scaffold.",
  },
  gatewayEmptyTitle: {
    id: "app.page.gateway.empty.title",
    defaultMessage: "Gateway details are not available",
    description: "Empty-state heading on the gateway details scaffold.",
  },
  gateways: {
    id: "app.nav.gateways",
    defaultMessage: "Gateways",
    description: "Navigation and page label for gateways.",
  },
  gatewaysCardBody: {
    id: "app.page.overview.gateways.body",
    defaultMessage: "Inspect gateway configuration after selecting a sector.",
    description: "Body text for the gateways card on the overview page.",
  },
  gatewaysDescription: {
    id: "app.page.gateways.description",
    defaultMessage: "Review gateways within the selected sector.",
    description: "Supporting text on the gateways scaffold page.",
  },
  gatewaysEmptyBody: {
    id: "app.page.gateways.empty.body",
    defaultMessage: "When gateways are available, they will appear here.",
    description: "Body text shown when the gateways scaffold has no data.",
  },
  gatewaysEmptyTitle: {
    id: "app.page.gateways.empty.title",
    defaultMessage: "No gateways to display",
    description: "Empty-state heading on the gateways scaffold page.",
  },
  sectorOverview: {
    id: "app.nav.sectorOverview",
    defaultMessage: "Sector overview",
    description: "Navigation and page label for the selected sector overview.",
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
  keys: {
    id: "app.nav.keys",
    defaultMessage: "Keys",
    description: "Page and breadcrumb label for access keys.",
  },
  keysDescription: {
    id: "app.page.keys.description",
    defaultMessage: "Review keys associated with this sector.",
    description: "Supporting text on the keys scaffold page.",
  },
  keysEmptyBody: {
    id: "app.page.keys.empty.body",
    defaultMessage: "When key data is available, it will appear here.",
    description: "Body text shown when the keys scaffold has no data.",
  },
  keysEmptyTitle: {
    id: "app.page.keys.empty.title",
    defaultMessage: "No keys to display",
    description: "Empty-state heading on the keys scaffold page.",
  },
  navigationToggle: {
    id: "app.nav.toggle",
    defaultMessage: "Toggle primary navigation",
    description:
      "Accessible label for the button that opens and closes the sidebar.",
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
  overview: {
    id: "app.nav.overview",
    defaultMessage: "Overview",
    description: "Navigation and page label for the console overview.",
  },
  overviewDescription: {
    id: "app.page.overview.description",
    defaultMessage:
      "Manage gateway infrastructure and configuration from one place.",
    description: "Supporting text on the console overview page.",
  },
  platformConnections: {
    id: "app.page.overview.connections.title",
    defaultMessage: "Platform connections",
    description: "Heading for the platform connections overview card.",
  },
  platformConnectionsCardBody: {
    id: "app.page.overview.connections.body",
    defaultMessage:
      "Cluster, database, release, and network inventory stays within its sector context.",
    description:
      "Body text for the platform connections card on the overview page.",
  },
  primaryNavigation: {
    id: "app.nav.primary",
    defaultMessage: "Primary navigation",
    description: "Accessible label for the primary sidebar navigation.",
  },
  productName: {
    id: "app.productName",
    defaultMessage: "HyperShell",
    description: "HyperShell product name.",
  },
  sectorDescription: {
    id: "app.page.sector.description",
    defaultMessage:
      "Review gateway and infrastructure activity for the selected sector.",
    description: "Supporting text on the sector overview scaffold.",
  },
  sectorContext: {
    id: "app.breadcrumb.sectorContext",
    defaultMessage: "Sector {sectorId}",
    description:
      "Breadcrumb label identifying the selected sector by its route identifier.",
  },
  sectorContextToolbar: {
    id: "app.sector.context.ariaLabel",
    defaultMessage: "Sector context",
    description:
      "Accessible label for the toolbar used to change the selected sector.",
  },
  sectorEmptyBody: {
    id: "app.page.sector.empty.body",
    defaultMessage:
      "Gateway and infrastructure summaries will appear here when they are available.",
    description: "Body text on the sector overview empty state.",
  },
  sectorEmptyTitle: {
    id: "app.page.sector.empty.title",
    defaultMessage: "No resource activity to display",
    description: "Empty-state heading on the sector overview.",
  },
  sectorSelectorError: {
    id: "app.sector.selector.error",
    defaultMessage: "Sectors could not be loaded",
    description:
      "Error shown in the sector selector when available sectors cannot be loaded.",
  },
  sectorSelectorLabel: {
    id: "app.sector.selector.label",
    defaultMessage: "Sector:",
    description: "Visible label placed before the sector selector.",
  },
  sectorSelectorLoading: {
    id: "app.sector.selector.loading",
    defaultMessage: "Loading sectors",
    description:
      "Loading state shown while the sector selector retrieves available sectors.",
  },
  sectorSelectorToggle: {
    id: "app.sector.selector.toggle",
    defaultMessage: "Select sector, currently {sectorName}",
    description:
      "Accessible label for the sector selector toggle, including its current value.",
  },
  sectors: {
    id: "app.nav.sectors",
    defaultMessage: "Sectors",
    description: "Navigation and page label for the collection of sectors.",
  },
  sectorsCardBody: {
    id: "app.page.overview.sectors.body",
    defaultMessage:
      "Choose the operational boundary used to organize clusters and gateways.",
    description: "Body text for the sectors overview card.",
  },
  sectorsDescription: {
    id: "app.page.sectors.description",
    defaultMessage: "Choose a sector to view its gateways and settings.",
    description: "Supporting text on the sectors scaffold page.",
  },
  sectorsEmptyBody: {
    id: "app.page.sectors.empty.body",
    defaultMessage: "When sectors are available, they will appear here.",
    description: "Body text shown when no sectors are available.",
  },
  sectorsEmptyTitle: {
    id: "app.page.sectors.empty.title",
    defaultMessage: "No sectors to display",
    description: "Empty-state heading on the sectors scaffold page.",
  },
  selectedSector: {
    id: "app.nav.selectedSector",
    defaultMessage: "Selected sector",
    description:
      "Navigation group and breadcrumb label used when a sector is selected.",
  },
  settings: {
    id: "app.nav.settings",
    defaultMessage: "Settings",
    description: "Navigation, breadcrumb, and page label for settings.",
  },
  settingsDescription: {
    id: "app.page.settings.description",
    defaultMessage: "Manage configuration for the selected sector.",
    description: "Supporting text on the settings scaffold page.",
  },
  settingsEmptyBody: {
    id: "app.page.settings.empty.body",
    defaultMessage:
      "Editable sector configuration will appear here when it is available.",
    description: "Body text on the settings scaffold empty state.",
  },
  settingsEmptyTitle: {
    id: "app.page.settings.empty.title",
    defaultMessage: "No settings to display",
    description: "Empty-state heading on the settings scaffold page.",
  },
  skipToContent: {
    id: "app.skipToContent",
    defaultMessage: "Skip to content",
    description:
      "Accessibility link that moves focus to the main page content.",
  },
  viewSectors: {
    id: "app.page.overview.sectors.action",
    defaultMessage: "View sectors",
    description: "Link from the overview page to the sectors collection.",
  },
});
