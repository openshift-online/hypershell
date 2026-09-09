import type { ExtendedTemplateConfig } from "@patternfly/widgetized-dashboard";
import type { IntlShape, MessageDescriptor } from "react-intl";

import { messages } from "../messages";

const METRIC_WIDGET_HEIGHT = 3;
const METRIC_ROW_GAP = 1;
const METRIC_ROW_STEP = METRIC_WIDGET_HEIGHT + METRIC_ROW_GAP;
/** Full-width section title row at the top of the dashboard grid. */
export const TITLE_WIDGET_HEIGHT = 1;
export const DASHBOARD_COLUMN_COUNT = 4;
const TITLE_ROW_OFFSET = TITLE_WIDGET_HEIGHT + METRIC_ROW_GAP;
/** One row taller than standard metric widgets; fits a compact status donut. */
export const NODE_STATUS_WIDGET_HEIGHT = METRIC_WIDGET_HEIGHT + 1;
/** Pod capacity donut shares the same height as the nodes status widget. */
export const POD_CAPACITY_WIDGET_HEIGHT = NODE_STATUS_WIDGET_HEIGHT;
/** Gateway status spans two metric rows plus the row gap between them. */
export const GATEWAY_STATUS_WIDGET_HEIGHT =
  METRIC_WIDGET_HEIGHT * 2 + METRIC_ROW_GAP;
const SUMMARY_COLUMN_HEIGHT = METRIC_WIDGET_HEIGHT + 2 * METRIC_ROW_STEP;
const BASE_SUMMARY_WIDGET_HEIGHT = (SUMMARY_COLUMN_HEIGHT - METRIC_ROW_GAP) / 2;
/** Equal height for usage and system summary widgets in the left column. */
export const USAGE_SUMMARY_WIDGET_HEIGHT = BASE_SUMMARY_WIDGET_HEIGHT + 1;
/** One row taller than usage summary; fits exception status rows on pods and nodes. */
export const SYSTEM_SUMMARY_WIDGET_HEIGHT = USAGE_SUMMARY_WIDGET_HEIGHT + 1;
/** Stats list and P95 note. */
export const PROVISION_TIME_WIDGET_HEIGHT = METRIC_WIDGET_HEIGHT + 1;
const ADOPTION_SECTION_START_Y = TITLE_ROW_OFFSET;
/** Grid row for the hub cluster section title. */
const HUB_CLUSTER_TITLE_Y =
  ADOPTION_SECTION_START_Y + GATEWAY_STATUS_WIDGET_HEIGHT + METRIC_ROW_GAP;
/** Grid row where hub-cluster capacity widgets begin (below hub cluster title). */
export const HUB_CLUSTER_START_Y = HUB_CLUSTER_TITLE_Y + TITLE_ROW_OFFSET;

export const SECTION_TITLE_WIDGET_TYPE = "section-title";

const SECTION_TITLE_MESSAGE_BY_ID: Record<string, MessageDescriptor> = {
  "section-title#hub-cluster": messages.sectionTitleHubCluster,
  "section-title#platform-adoption": messages.sectionTitlePlatformAdoption,
};

const WIDGET_TITLE_MESSAGES = {
  "usage-summary": messages.usageSummaryWidget,
  "system-summary": messages.systemSummaryWidget,
  "registered-users": messages.registeredUsers,
  "gateway-status": messages.gatewayStatusWidget,
  memory: messages.widgetMemory,
  nodes: messages.nodes,
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
    h: USAGE_SUMMARY_WIDGET_HEIGHT,
    i: "usage-summary#1",
    title: "Usage summary",
    w: 1,
    widgetType: "usage-summary",
    x: 0,
    y: ADOPTION_SECTION_START_Y,
  },
  {
    h: GATEWAY_STATUS_WIDGET_HEIGHT,
    i: "gateway-status#1",
    title: "Gateway status",
    w: 2,
    widgetType: "gateway-status",
    x: 1,
    y: ADOPTION_SECTION_START_Y,
  },
  {
    h: PROVISION_TIME_WIDGET_HEIGHT,
    i: "provision-time#1",
    title: "Provision time",
    w: 1,
    widgetType: "provision-time",
    x: 1,
    y: GATEWAY_STATUS_WIDGET_HEIGHT,
  },
  {
    h: METRIC_WIDGET_HEIGHT,
    i: "provisioned-sandboxes#1",
    title: "Sandboxes",
    w: 1,
    widgetType: "provisioned-sandboxes",
    x: 3,
    y: ADOPTION_SECTION_START_Y,
  },
  {
    h: METRIC_WIDGET_HEIGHT,
    i: "registered-users#1",
    title: "Registered users",
    w: 1,
    widgetType: "registered-users",
    x: 3,
    y: ADOPTION_SECTION_START_Y + METRIC_ROW_STEP,
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
    h: METRIC_WIDGET_HEIGHT,
    i: "memory#1",
    title: "Memory",
    w: 1,
    widgetType: "memory",
    x: 1,
    y: HUB_CLUSTER_START_Y,
  },
  {
    h: METRIC_WIDGET_HEIGHT,
    i: "cpu#1",
    title: "CPU",
    w: 1,
    widgetType: "cpu",
    x: 2,
    y: HUB_CLUSTER_START_Y,
  },
  {
    h: POD_CAPACITY_WIDGET_HEIGHT,
    i: "pods#1",
    title: "Pods",
    w: 1,
    widgetType: "pods",
    x: 3,
    y: HUB_CLUSTER_START_Y,
  },
  {
    h: NODE_STATUS_WIDGET_HEIGHT,
    i: "nodes#1",
    title: "Nodes",
    w: 1,
    widgetType: "nodes",
    x: 1,
    y: HUB_CLUSTER_START_Y + METRIC_ROW_STEP,
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
  "provision-time",
  "provisioned-sandboxes",
  "registered-users",
  "section-title#hub-cluster",
  "system-summary",
  "memory",
  "cpu",
  "pods",
  "nodes",
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
