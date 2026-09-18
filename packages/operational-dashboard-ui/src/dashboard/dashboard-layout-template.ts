import type { ExtendedTemplateConfig } from "@patternfly/widgetized-dashboard";
import type { IntlShape, MessageDescriptor } from "react-intl";

import { messages } from "../messages";

const METRIC_WIDGET_HEIGHT = 3;
const METRIC_ROW_GAP = 1;
export const METRIC_ROW_STEP = METRIC_WIDGET_HEIGHT + METRIC_ROW_GAP;
/** Full-width section title row at the top of the dashboard grid. */
export const TITLE_WIDGET_HEIGHT = 1;
export const DASHBOARD_COLUMN_COUNT = 4;
const TITLE_CONTENT_OFFSET = TITLE_WIDGET_HEIGHT;
/** One row taller than standard metric widgets; fits a compact status donut. */
export const NODE_STATUS_WIDGET_HEIGHT = METRIC_WIDGET_HEIGHT + 1;
/** Utilization widgets with a seven-day sparkline below the donut. */
export const UTILIZATION_WIDGET_HEIGHT = NODE_STATUS_WIDGET_HEIGHT + 2;
/** Pod capacity donut with a seven-day sparkline below the chart. */
export const POD_CAPACITY_WIDGET_HEIGHT = UTILIZATION_WIDGET_HEIGHT;
/** Equal height for usage and inventory summary widgets in the left column. */
export const USAGE_SUMMARY_WIDGET_HEIGHT = 6;
/** Gateway status matches usage summary height in the platform adoption section. */
export const GATEWAY_STATUS_WIDGET_HEIGHT = USAGE_SUMMARY_WIDGET_HEIGHT;
/** Gateway releases stat panel height in the platform adoption right column. */
export const GATEWAY_RELEASES_WIDGET_HEIGHT = GATEWAY_STATUS_WIDGET_HEIGHT;
/** Shared height for Users and Gateways on platform adoption row 1. */
export const ADOPTION_TOP_ROW_WIDGET_HEIGHT = 7;
/** Users adoption card (stat grid and sparkline) on platform adoption row 1. */
export const REGISTERED_USERS_WIDGET_HEIGHT = ADOPTION_TOP_ROW_WIDGET_HEIGHT;
/** Gateway status on platform adoption row 1, aligned with Users. */
export const ADOPTION_GATEWAY_STATUS_WIDGET_HEIGHT =
  ADOPTION_TOP_ROW_WIDGET_HEIGHT;
/** Sandbox status with hourly and daily sparklines (row 2, columns 1-2). */
export const ADOPTION_SANDBOX_STATUS_WIDGET_HEIGHT = 7;
/** Gateway releases beside sandbox on row 2 (column 3). */
export const ADOPTION_GATEWAY_RELEASES_WIDGET_HEIGHT = 7;
const ADOPTION_SECTION_START_Y = TITLE_WIDGET_HEIGHT;
/** Grid row where sandbox status and gateway releases begin (below Users). */
export const ADOPTION_ROW_2_Y =
  ADOPTION_SECTION_START_Y + ADOPTION_TOP_ROW_WIDGET_HEIGHT;
const ADOPTION_SECTION_HEIGHT =
  ADOPTION_TOP_ROW_WIDGET_HEIGHT + ADOPTION_SANDBOX_STATUS_WIDGET_HEIGHT;
/** Stats list and P95 note. */
export const PROVISION_TIME_WIDGET_HEIGHT = METRIC_WIDGET_HEIGHT + 1;
/** Donut and hourly success-rate sparkline. */
export const PROVISION_RELIABILITY_WIDGET_HEIGHT =
  POD_CAPACITY_WIDGET_HEIGHT + 2;
/** Spans both hub cluster rows; fits provision duration and success-rate rows. */
export const SYSTEM_SUMMARY_WIDGET_HEIGHT = 13;
/** Height of the two-row hub cluster grid beside system-summary. */
export const HUB_CLUSTER_SECTION_HEIGHT = SYSTEM_SUMMARY_WIDGET_HEIGHT;
/** Grid row for the hub cluster section title. */
const HUB_CLUSTER_TITLE_Y = ADOPTION_SECTION_START_Y + ADOPTION_SECTION_HEIGHT;
/** Grid row where hub-cluster capacity widgets begin (below hub cluster title). */
export const HUB_CLUSTER_START_Y = HUB_CLUSTER_TITLE_Y + TITLE_CONTENT_OFFSET;
const HUB_CLUSTER_ROW_2_Y = HUB_CLUSTER_START_Y + UTILIZATION_WIDGET_HEIGHT;
const HUB_CLUSTER_SECTION_BOTTOM =
  HUB_CLUSTER_START_Y +
  Math.max(
    SYSTEM_SUMMARY_WIDGET_HEIGHT,
    HUB_CLUSTER_ROW_2_Y -
      HUB_CLUSTER_START_Y +
      PROVISION_RELIABILITY_WIDGET_HEIGHT,
  );
/** Grid row for the platform inventory section title. */
export const PLATFORM_INVENTORY_TITLE_Y = HUB_CLUSTER_SECTION_BOTTOM;
/** Grid row where the inventory summary widget begins. */
export const PLATFORM_INVENTORY_START_Y =
  PLATFORM_INVENTORY_TITLE_Y + TITLE_CONTENT_OFFSET;
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
  "managed-database-status": messages.widgetManagedDatabaseStatus,
  "gateway-status": messages.gatewayStatusWidget,
  "sandbox-status": messages.sandboxStatusWidget,
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
    h: ADOPTION_SECTION_HEIGHT,
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
    title: "Gateways",
    w: 1,
    widgetType: "gateway-status",
    x: 3,
    y: ADOPTION_SECTION_START_Y,
  },
  {
    h: ADOPTION_SANDBOX_STATUS_WIDGET_HEIGHT,
    i: "sandbox-status#1",
    title: "Sandbox status",
    w: 2,
    widgetType: "sandbox-status",
    x: 1,
    y: ADOPTION_ROW_2_Y,
  },
  {
    h: ADOPTION_GATEWAY_RELEASES_WIDGET_HEIGHT,
    i: "gateway-releases#1",
    title: "Gateway releases",
    w: 1,
    widgetType: "gateway-releases",
    x: 3,
    y: ADOPTION_ROW_2_Y,
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
    h: UTILIZATION_WIDGET_HEIGHT,
    i: "memory#1",
    title: "Memory",
    w: 1,
    widgetType: "memory",
    x: 1,
    y: HUB_CLUSTER_START_Y,
  },
  {
    h: UTILIZATION_WIDGET_HEIGHT,
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
    w: 1,
    widgetType: "managed-cluster-regions",
    x: 2,
    y: PLATFORM_INVENTORY_START_Y,
  },
  {
    h: NODE_STATUS_WIDGET_HEIGHT,
    i: "managed-database-status#1",
    title: "Database status",
    w: 1,
    widgetType: "managed-database-status",
    x: 3,
    y: PLATFORM_INVENTORY_START_Y,
  },
] as const;

const mobileLayout = [
  {
    h: TITLE_WIDGET_HEIGHT,
    i: "section-title#platform-adoption",
    title: "Platform adoption",
    w: 1,
    widgetType: SECTION_TITLE_WIDGET_TYPE,
    x: 0,
    y: 0,
  },
  {
    h: USAGE_SUMMARY_WIDGET_HEIGHT,
    i: "usage-summary#1",
    title: "Usage summary",
    w: 1,
    widgetType: "usage-summary",
    x: 0,
    y: 1,
  },
  {
    h: REGISTERED_USERS_WIDGET_HEIGHT,
    i: "registered-users#1",
    title: "Users",
    w: 1,
    widgetType: "registered-users",
    x: 0,
    y: 7,
  },
  {
    h: GATEWAY_STATUS_WIDGET_HEIGHT,
    i: "gateway-status#1",
    title: "Gateways",
    w: 1,
    widgetType: "gateway-status",
    x: 0,
    y: 14,
  },
  {
    h: ADOPTION_SANDBOX_STATUS_WIDGET_HEIGHT,
    i: "sandbox-status#1",
    title: "Sandbox status",
    w: 1,
    widgetType: "sandbox-status",
    x: 0,
    y: 20,
  },
  {
    h: 5,
    i: "gateway-releases#1",
    title: "Gateway releases",
    w: 1,
    widgetType: "gateway-releases",
    x: 0,
    y: 27,
  },
  {
    h: TITLE_WIDGET_HEIGHT,
    i: "section-title#hub-cluster",
    title: "Hub cluster",
    w: 1,
    widgetType: SECTION_TITLE_WIDGET_TYPE,
    x: 0,
    y: 32,
  },
  {
    h: SYSTEM_SUMMARY_WIDGET_HEIGHT,
    i: "system-summary#1",
    title: "System summary",
    w: 1,
    widgetType: "system-summary",
    x: 0,
    y: 33,
  },
  {
    h: UTILIZATION_WIDGET_HEIGHT,
    i: "memory#1",
    title: "Memory",
    w: 1,
    widgetType: "memory",
    x: 0,
    y: 46,
  },
  {
    h: UTILIZATION_WIDGET_HEIGHT,
    i: "cpu#1",
    title: "CPU",
    w: 1,
    widgetType: "cpu",
    x: 0,
    y: 52,
  },
  {
    h: NODE_STATUS_WIDGET_HEIGHT,
    i: "nodes#1",
    title: "Nodes",
    w: 1,
    widgetType: "nodes",
    x: 0,
    y: 58,
  },
  {
    h: POD_CAPACITY_WIDGET_HEIGHT,
    i: "pods#1",
    title: "Pods",
    w: 1,
    widgetType: "pods",
    x: 0,
    y: 62,
  },
  {
    h: PROVISION_TIME_WIDGET_HEIGHT,
    i: "provision-time#1",
    title: "Provision time",
    w: 1,
    widgetType: "provision-time",
    x: 0,
    y: 68,
  },
  {
    h: PROVISION_RELIABILITY_WIDGET_HEIGHT,
    i: "provision-reliability#1",
    title: "Provision reliability",
    w: 1,
    widgetType: "provision-reliability",
    x: 0,
    y: 72,
  },
  {
    h: TITLE_WIDGET_HEIGHT,
    i: "section-title#platform-inventory",
    title: "Platform inventory",
    w: 1,
    widgetType: SECTION_TITLE_WIDGET_TYPE,
    x: 0,
    y: 80,
  },
  {
    h: INVENTORY_SUMMARY_WIDGET_HEIGHT,
    i: "inventory-summary#1",
    title: "Inventory summary",
    w: 1,
    widgetType: "inventory-summary",
    x: 0,
    y: 81,
  },
  {
    h: NODE_STATUS_WIDGET_HEIGHT,
    i: "managed-cluster-providers#1",
    title: "Cluster providers",
    w: 1,
    widgetType: "managed-cluster-providers",
    x: 0,
    y: 87,
  },
  {
    h: NODE_STATUS_WIDGET_HEIGHT,
    i: "managed-cluster-regions#1",
    title: "Cluster regions",
    w: 1,
    widgetType: "managed-cluster-regions",
    x: 0,
    y: 91,
  },
  {
    h: NODE_STATUS_WIDGET_HEIGHT,
    i: "managed-database-status#1",
    title: "Database status",
    w: 1,
    widgetType: "managed-database-status",
    x: 0,
    y: 95,
  },
] as const;

export const defaultDashboardLayoutTemplate: ExtendedTemplateConfig = {
  xl: [...fourColumnLayout],
  lg: [...fourColumnLayout],
  md: [...fourColumnLayout],
  sm: [...mobileLayout],
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
