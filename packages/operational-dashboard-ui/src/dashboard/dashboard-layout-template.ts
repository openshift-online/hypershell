import type { ExtendedTemplateConfig } from "@patternfly/widgetized-dashboard";
import type { IntlShape } from "react-intl";

import { messages } from "../messages";

const METRIC_WIDGET_HEIGHT = 3;
const METRIC_ROW_GAP = 1;
const METRIC_ROW_STEP = METRIC_WIDGET_HEIGHT + METRIC_ROW_GAP;
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
const SYSTEM_SUMMARY_WIDGET_Y = USAGE_SUMMARY_WIDGET_HEIGHT + METRIC_ROW_GAP;

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
    h: USAGE_SUMMARY_WIDGET_HEIGHT,
    i: "usage-summary#1",
    title: "Usage summary",
    w: 1,
    widgetType: "usage-summary",
    x: 0,
    y: 0,
  },
  {
    h: GATEWAY_STATUS_WIDGET_HEIGHT,
    i: "gateway-status#1",
    title: "Gateway status",
    w: 1,
    widgetType: "gateway-status",
    x: 1,
    y: 0,
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
    x: 2,
    y: 0,
  },
  {
    h: METRIC_WIDGET_HEIGHT,
    i: "memory#1",
    title: "Memory",
    w: 1,
    widgetType: "memory",
    x: 3,
    y: 0,
  },
  {
    h: SYSTEM_SUMMARY_WIDGET_HEIGHT,
    i: "system-summary#1",
    title: "System summary",
    w: 1,
    widgetType: "system-summary",
    x: 0,
    y: SYSTEM_SUMMARY_WIDGET_Y,
  },
  {
    h: METRIC_WIDGET_HEIGHT,
    i: "registered-users#1",
    title: "Registered users",
    w: 1,
    widgetType: "registered-users",
    x: 2,
    y: METRIC_ROW_STEP,
  },
  {
    h: METRIC_WIDGET_HEIGHT,
    i: "cpu#1",
    title: "CPU",
    w: 1,
    widgetType: "cpu",
    x: 3,
    y: METRIC_ROW_STEP,
  },
  {
    h: POD_CAPACITY_WIDGET_HEIGHT,
    i: "pods#1",
    title: "Pods",
    w: 1,
    widgetType: "pods",
    x: 3,
    y: METRIC_ROW_STEP * 2,
  },
  {
    h: NODE_STATUS_WIDGET_HEIGHT,
    i: "nodes#1",
    title: "Nodes",
    w: 1,
    widgetType: "nodes",
    x: 2,
    y: METRIC_ROW_STEP * 2,
  },
] as const;

export const defaultDashboardLayoutTemplate: ExtendedTemplateConfig = {
  xl: [...fourColumnLayout],
  lg: [...fourColumnLayout],
  md: [...fourColumnLayout],
  sm: [
    {
      h: USAGE_SUMMARY_WIDGET_HEIGHT,
      i: "usage-summary#1",
      title: "Usage summary",
      w: 1,
      widgetType: "usage-summary",
      x: 0,
      y: 0,
    },
    {
      h: SYSTEM_SUMMARY_WIDGET_HEIGHT,
      i: "system-summary#1",
      title: "System summary",
      w: 1,
      widgetType: "system-summary",
      x: 0,
      y: SYSTEM_SUMMARY_WIDGET_Y,
    },
    {
      h: GATEWAY_STATUS_WIDGET_HEIGHT,
      i: "gateway-status#1",
      title: "Gateway status",
      w: 1,
      widgetType: "gateway-status",
      x: 0,
      y:
        USAGE_SUMMARY_WIDGET_HEIGHT +
        METRIC_ROW_GAP +
        SYSTEM_SUMMARY_WIDGET_HEIGHT,
    },
    {
      h: PROVISION_TIME_WIDGET_HEIGHT,
      i: "provision-time#1",
      title: "Provision time",
      w: 1,
      widgetType: "provision-time",
      x: 0,
      y:
        USAGE_SUMMARY_WIDGET_HEIGHT +
        METRIC_ROW_GAP +
        SYSTEM_SUMMARY_WIDGET_HEIGHT +
        GATEWAY_STATUS_WIDGET_HEIGHT,
    },
    {
      h: METRIC_WIDGET_HEIGHT,
      i: "provisioned-sandboxes#1",
      title: "Sandboxes",
      w: 1,
      widgetType: "provisioned-sandboxes",
      x: 0,
      y:
        USAGE_SUMMARY_WIDGET_HEIGHT +
        METRIC_ROW_GAP +
        SYSTEM_SUMMARY_WIDGET_HEIGHT +
        GATEWAY_STATUS_WIDGET_HEIGHT +
        PROVISION_TIME_WIDGET_HEIGHT,
    },
    {
      h: METRIC_WIDGET_HEIGHT,
      i: "memory#1",
      title: "Memory",
      w: 1,
      widgetType: "memory",
      x: 0,
      y:
        USAGE_SUMMARY_WIDGET_HEIGHT +
        METRIC_ROW_GAP +
        SYSTEM_SUMMARY_WIDGET_HEIGHT +
        GATEWAY_STATUS_WIDGET_HEIGHT +
        PROVISION_TIME_WIDGET_HEIGHT +
        METRIC_ROW_STEP,
    },
    {
      h: METRIC_WIDGET_HEIGHT,
      i: "registered-users#1",
      title: "Registered users",
      w: 1,
      widgetType: "registered-users",
      x: 0,
      y:
        USAGE_SUMMARY_WIDGET_HEIGHT +
        METRIC_ROW_GAP +
        SYSTEM_SUMMARY_WIDGET_HEIGHT +
        GATEWAY_STATUS_WIDGET_HEIGHT +
        PROVISION_TIME_WIDGET_HEIGHT +
        METRIC_ROW_STEP * 2,
    },
    {
      h: METRIC_WIDGET_HEIGHT,
      i: "cpu#1",
      title: "CPU",
      w: 1,
      widgetType: "cpu",
      x: 0,
      y:
        USAGE_SUMMARY_WIDGET_HEIGHT +
        METRIC_ROW_GAP +
        SYSTEM_SUMMARY_WIDGET_HEIGHT +
        GATEWAY_STATUS_WIDGET_HEIGHT +
        PROVISION_TIME_WIDGET_HEIGHT +
        METRIC_ROW_STEP * 3,
    },
    {
      h: POD_CAPACITY_WIDGET_HEIGHT,
      i: "pods#1",
      title: "Pods",
      w: 1,
      widgetType: "pods",
      x: 0,
      y:
        USAGE_SUMMARY_WIDGET_HEIGHT +
        METRIC_ROW_GAP +
        SYSTEM_SUMMARY_WIDGET_HEIGHT +
        GATEWAY_STATUS_WIDGET_HEIGHT +
        PROVISION_TIME_WIDGET_HEIGHT +
        METRIC_ROW_STEP * 4,
    },
    {
      h: NODE_STATUS_WIDGET_HEIGHT,
      i: "nodes#1",
      title: "Nodes",
      w: 1,
      widgetType: "nodes",
      x: 0,
      y:
        USAGE_SUMMARY_WIDGET_HEIGHT +
        METRIC_ROW_GAP +
        SYSTEM_SUMMARY_WIDGET_HEIGHT +
        GATEWAY_STATUS_WIDGET_HEIGHT +
        PROVISION_TIME_WIDGET_HEIGHT +
        METRIC_ROW_STEP * 5,
    },
  ],
};

export function localizeDashboardLayoutTemplate(
  template: ExtendedTemplateConfig,
  intl: IntlShape,
): ExtendedTemplateConfig {
  return (Object.keys(template) as (keyof ExtendedTemplateConfig)[]).reduce(
    (localized, variant) => {
      localized[variant] = template[variant].map((item) => {
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
