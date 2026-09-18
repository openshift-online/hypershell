import type { ExtendedTemplateConfig } from "@patternfly/widgetized-dashboard";
import type { IntlShape, MessageDescriptor } from "react-intl";

import { messages } from "../messages";

const METRIC_WIDGET_HEIGHT = 3;
const METRIC_ROW_GAP = 1;
export const METRIC_ROW_STEP = METRIC_WIDGET_HEIGHT + METRIC_ROW_GAP;
/** Full-width section title row at the top of the dashboard grid. */
export const TITLE_WIDGET_HEIGHT = 1;
export const DASHBOARD_COLUMN_COUNT = 4;
const TITLE_ROW_OFFSET = TITLE_WIDGET_HEIGHT + METRIC_ROW_GAP;
/** One row taller than standard metric widgets; fits a compact status donut. */
export const NODE_STATUS_WIDGET_HEIGHT = METRIC_WIDGET_HEIGHT + 1;
/** Pod capacity donut shares the same height as the nodes status widget. */
export const POD_CAPACITY_WIDGET_HEIGHT = NODE_STATUS_WIDGET_HEIGHT;
const SUMMARY_COLUMN_HEIGHT = METRIC_WIDGET_HEIGHT + 2 * METRIC_ROW_STEP;
const BASE_SUMMARY_WIDGET_HEIGHT = (SUMMARY_COLUMN_HEIGHT - METRIC_ROW_GAP) / 2;
/** Equal height for usage and system summary widgets in the left column. */
export const USAGE_SUMMARY_WIDGET_HEIGHT = BASE_SUMMARY_WIDGET_HEIGHT + 1;
/** Gateway status matches usage summary height in the platform adoption section. */
export const GATEWAY_STATUS_WIDGET_HEIGHT = USAGE_SUMMARY_WIDGET_HEIGHT;
/** Gateway releases stat panel height in the platform adoption right column. */
export const GATEWAY_RELEASES_WIDGET_HEIGHT = GATEWAY_STATUS_WIDGET_HEIGHT;
/** Taller widget for the Users adoption card (stat grid and sparkline). */
export const REGISTERED_USERS_WIDGET_HEIGHT = USAGE_SUMMARY_WIDGET_HEIGHT + 2;
/** Gateway status height in the platform adoption right column. */
export const ADOPTION_GATEWAY_STATUS_WIDGET_HEIGHT =
  REGISTERED_USERS_WIDGET_HEIGHT - METRIC_ROW_STEP;
const ADOPTION_SECTION_START_Y = TITLE_ROW_OFFSET;
/** Grid row where the gateway releases widget begins (below gateway status). */
export const ADOPTION_GATEWAY_RELEASES_Y =
  ADOPTION_SECTION_START_Y +
  ADOPTION_GATEWAY_STATUS_WIDGET_HEIGHT +
  METRIC_ROW_GAP;
const ADOPTION_RIGHT_COLUMN_HEIGHT =
  ADOPTION_GATEWAY_STATUS_WIDGET_HEIGHT +
  METRIC_ROW_GAP +
  GATEWAY_RELEASES_WIDGET_HEIGHT;
const ADOPTION_SECTION_HEIGHT = Math.max(
  REGISTERED_USERS_WIDGET_HEIGHT,
  ADOPTION_RIGHT_COLUMN_HEIGHT,
);
/** Stats list and P95 note. */
export const PROVISION_TIME_WIDGET_HEIGHT = METRIC_WIDGET_HEIGHT + 1;
/** Donut and hourly success-rate sparkline. */
export const PROVISION_RELIABILITY_WIDGET_HEIGHT =
  POD_CAPACITY_WIDGET_HEIGHT + 2;
/** Height of the two-row hub cluster grid beside system-summary. */
export const HUB_CLUSTER_SECTION_HEIGHT =
  NODE_STATUS_WIDGET_HEIGHT +
  METRIC_ROW_GAP +
  Math.max(
    POD_CAPACITY_WIDGET_HEIGHT,
    PROVISION_TIME_WIDGET_HEIGHT,
    PROVISION_RELIABILITY_WIDGET_HEIGHT,
  );
/** Spans both hub cluster rows; fits provision duration and success-rate rows. */
export const SYSTEM_SUMMARY_WIDGET_HEIGHT = HUB_CLUSTER_SECTION_HEIGHT;
/** Grid row for the hub cluster section title. */
const HUB_CLUSTER_TITLE_Y =
  ADOPTION_SECTION_START_Y + ADOPTION_SECTION_HEIGHT + METRIC_ROW_GAP;
/** Grid row where hub-cluster capacity widgets begin (below hub cluster title). */
export const HUB_CLUSTER_START_Y = HUB_CLUSTER_TITLE_Y + TITLE_ROW_OFFSET;
const HUB_CLUSTER_ROW_2_Y =
  HUB_CLUSTER_START_Y + NODE_STATUS_WIDGET_HEIGHT + METRIC_ROW_GAP;
/** Grid row for the platform inventory section title. */
export const PLATFORM_INVENTORY_TITLE_Y =
  HUB_CLUSTER_START_Y + HUB_CLUSTER_SECTION_HEIGHT + METRIC_ROW_GAP;
/** Grid row where the inventory summary widget begins. */
export const PLATFORM_INVENTORY_START_Y =
  PLATFORM_INVENTORY_TITLE_Y + TITLE_ROW_OFFSET;
/** Height for the inventory summary DescriptionList widget. */
export const INVENTORY_SUMMARY_WIDGET_HEIGHT = USAGE_SUMMARY_WIDGET_HEIGHT;

export const SECTION_TITLE_WIDGET_TYPE = "section-title";

const SECTION_TITLE_MESSAGE_BY_ID: Record<string, MessageDescriptor> = {
  "section-title#hub-cluster": messages.sectionTitleHubCluster,
  "section-title#platform-adoption": messages.sectionTitlePlatformAdoption,
  "section-title#platform-inventory": messages.sectionTitlePlatformInventory,
};

const WIDGET_TITLE_MESSAGES = {
  "usage-summary": messages.usageSummaryWidget,
  "system-summary": messages.systemSummaryWidget,
  "inventory-summary": messages.inventorySummaryWidget,
  "registered-users": messages.registeredUsers,
  "managed-cluster-providers": messages.widgetManagedClusterProviders,
  "managed-cluster-regions": messages.widgetManagedClusterRegions,
  "gateway-status": messages.gatewayStatusWidget,
  "gateway-releases": messages.gatewayReleasesWidget,
  memory: messages.widgetMemory,
  nodes: messages.nodes,
  "provision-reliability": messages.provisionReliabilityWidget,
  "provision-time": messages.provisionTimeWidget,
  "provisioned-sandboxes": messages.widgetSandboxes,
  cpu: messages.widgetCpu,
  pods: messages.widgetPods,
} as const;

type DashboardWidgetType = keyof typeof WIDGET_TITLE_MESSAGES;

const fourColumnLayout = [
  {
    h: TITLE_WIDGET_HEIGHT,
    i: "section-title#platform-adoption",
    title: "Platform adoption",
    w: DASHBOARD_COLUMN_COUNT,
    widgetType: SECTION_TITLE_WIDGET_TYPE,
    x: 0,
    y: 0,
  },
  {
    h: REGISTERED_USERS_WIDGET_HEIGHT,
    i: "usage-summary#1",
    title: "Usage summary",
    w: 1,
    widgetType: "usage-summary",
    x: 0,
    y: ADOPTION_SECTION_START_Y,
  },
  {
    h: REGISTERED_USERS_WIDGET_HEIGHT,
    i: "registered-users#1",
    title: "Users",
    w: 2,
    widgetType: "registered-users",
    x: 1,
    y: ADOPTION_SECTION_START_Y,
  },
  {
    h: ADOPTION_GATEWAY_STATUS_WIDGET_HEIGHT,
    i: "gateway-status#1",
    title: "Gateway status",
    w: 1,
    widgetType: "gateway-status",
    x: 3,
    y: ADOPTION_SECTION_START_Y,
  },
  {
    h: GATEWAY_RELEASES_WIDGET_HEIGHT,
    i: "gateway-releases#1",
    title: "Gateway releases",
    w: 1,
    widgetType: "gateway-releases",
    x: 3,
    y: ADOPTION_GATEWAY_RELEASES_Y,
  },
  {
    h: TITLE_WIDGET_HEIGHT,
    i: "section-title#hub-cluster",
    title: "Hub cluster",
    w: DASHBOARD_COLUMN_COUNT,
    widgetType: SECTION_TITLE_WIDGET_TYPE,
    x: 0,
    y: HUB_CLUSTER_TITLE_Y,
  },
  {
    h: SYSTEM_SUMMARY_WIDGET_HEIGHT,
    i: "system-summary#1",
    title: "System summary",
    w: 1,
    widgetType: "system-summary",
    x: 0,
    y: HUB_CLUSTER_START_Y,
  },
  {
    h: NODE_STATUS_WIDGET_HEIGHT,
    i: "memory#1",
    title: "Memory",
    w: 1,
    widgetType: "memory",
    x: 1,
    y: HUB_CLUSTER_START_Y,
  },
  {
    h: NODE_STATUS_WIDGET_HEIGHT,
    i: "cpu#1",
    title: "CPU",
    w: 1,
    widgetType: "cpu",
    x: 2,
    y: HUB_CLUSTER_START_Y,
  },
  {
    h: NODE_STATUS_WIDGET_HEIGHT,
    i: "nodes#1",
    title: "Nodes",
    w: 1,
    widgetType: "nodes",
    x: 3,
    y: HUB_CLUSTER_START_Y,
  },
  {
    h: POD_CAPACITY_WIDGET_HEIGHT,
    i: "pods#1",
    title: "Pods",
    w: 1,
    widgetType: "pods",
    x: 1,
    y: HUB_CLUSTER_ROW_2_Y,
  },
  {
    h: PROVISION_TIME_WIDGET_HEIGHT,
    i: "provision-time#1",
    title: "Provision time",
    w: 1,
    widgetType: "provision-time",
    x: 2,
    y: HUB_CLUSTER_ROW_2_Y,
  },
  {
    h: PROVISION_RELIABILITY_WIDGET_HEIGHT,
    i: "provision-reliability#1",
    title: "Provision reliability",
    w: 1,
    widgetType: "provision-reliability",
    x: 3,
    y: HUB_CLUSTER_ROW_2_Y,
  },
  {
    h: TITLE_WIDGET_HEIGHT,
    i: "section-title#platform-inventory",
    title: "Platform inventory",
    w: DASHBOARD_COLUMN_COUNT,
    widgetType: SECTION_TITLE_WIDGET_TYPE,
    x: 0,
    y: PLATFORM_INVENTORY_TITLE_Y,
  },
  {
    h: INVENTORY_SUMMARY_WIDGET_HEIGHT,
    i: "inventory-summary#1",
    title: "Inventory summary",
    w: 1,
    widgetType: "inventory-summary",
    x: 0,
    y: PLATFORM_INVENTORY_START_Y,
  },
  {
    h: NODE_STATUS_WIDGET_HEIGHT,
    i: "managed-cluster-providers#1",
    title: "Cluster providers",
    w: 1,
    widgetType: "managed-cluster-providers",
    x: 1,
    y: PLATFORM_INVENTORY_START_Y,
  },
  {
    h: NODE_STATUS_WIDGET_HEIGHT,
    i: "managed-cluster-regions#1",
    title: "Cluster regions",
    // The ManagedDatabase resource does not exist (see
    // platform/openshell-gateway-database.spec.md), so there is no
    // managed-database-status widget to occupy columns 2-3 of this row. This
    // widget widens to fill them instead of leaving a gap.
    w: 2,
    widgetType: "managed-cluster-regions",
    x: 2,
    y: PLATFORM_INVENTORY_START_Y,
  },
] as const;

function stackMobileY(
  items: readonly {
    h: number;
    i: string;
    title: string;
    w: number;
    widgetType: string;
    x: number;
    y: number;
  }[],
) {
  let nextY = 0;

  return items.map((item) => {
    const positioned = {
      ...item,
      w: 1,
      x: 0,
      y: nextY,
    };
    nextY += item.h + METRIC_ROW_GAP;
    return positioned;
  });
}

const mobileLayoutOrder = [
  "section-title#platform-adoption",
  "usage-summary",
  "gateway-status",
  "gateway-releases",
  "registered-users",
  "section-title#hub-cluster",
  "system-summary",
  "memory",
  "cpu",
  "nodes",
  "pods",
  "provision-time",
  "provision-reliability",
  "section-title#platform-inventory",
  "inventory-summary",
  "managed-cluster-providers",
  "managed-cluster-regions",
] as const;

function findLayoutItem(
  items: readonly {
    h: number;
    i: string;
    title: string;
    w: number;
    widgetType: string;
    x: number;
    y: number;
  }[],
  layoutKey: (typeof mobileLayoutOrder)[number],
) {
  const item = items.find(
    (layoutItem) =>
      layoutItem.i === layoutKey || layoutItem.widgetType === layoutKey,
  );
  if (!item) {
    throw new Error(`expected ${layoutKey} in default layout`);
  }

  return item;
}

const mobileLayout = stackMobileY(
  mobileLayoutOrder.map((layoutKey) =>
    findLayoutItem(fourColumnLayout, layoutKey),
  ),
);

export const defaultDashboardLayoutTemplate: ExtendedTemplateConfig = {
  xl: [...fourColumnLayout],
  lg: [...fourColumnLayout],
  md: [...fourColumnLayout],
  sm: mobileLayout,
};

export function localizeDashboardLayoutTemplate(
  template: ExtendedTemplateConfig,
  intl: IntlShape,
): ExtendedTemplateConfig {
  return (Object.keys(template) as (keyof ExtendedTemplateConfig)[]).reduce(
    (localized, variant) => {
      localized[variant] = template[variant].map((item) => {
        if (item.widgetType === SECTION_TITLE_WIDGET_TYPE) {
          const message = SECTION_TITLE_MESSAGE_BY_ID[item.i];

          return {
            ...item,
            title: message ? intl.formatMessage(message) : item.title,
          };
        }

        const widgetType = item.widgetType as DashboardWidgetType;

        return {
          ...item,
          title: intl.formatMessage(WIDGET_TITLE_MESSAGES[widgetType]),
        };
      });
      return localized;
    },
    {} as ExtendedTemplateConfig,
  );
}
