import type { ExtendedTemplateConfig } from "@patternfly/widgetized-dashboard";
import type { IntlShape } from "react-intl";

import { messages } from "../messages";

export const USAGE_DASHBOARD_COLUMN_COUNT = 4;
export const USAGE_GATEWAY_WIDGET_HEIGHT = 5;
export const USAGE_DETAIL_WIDGET_HEIGHT = 6;

const WIDGET_TITLE_MESSAGES = {
  "gateway-request-volume": messages.usageWidgetGatewayRequestVolume,
  "gateway-error-rate": messages.usageWidgetGatewayErrorRate,
  "active-gateways": messages.usageWidgetActiveGateways,
  "unused-gateways": messages.usageWidgetUnusedGateways,
  "sandbox-utilization": messages.usageWidgetSandboxUtilization,
  "sandbox-lifetime": messages.usageWidgetSandboxLifetime,
} as const;

type UsageWidgetType = keyof typeof WIDGET_TITLE_MESSAGES;

const desktopLayout = [
  {
    h: USAGE_GATEWAY_WIDGET_HEIGHT,
    i: "gateway-request-volume#1",
    title: "Gateway request volume",
    w: 1,
    widgetType: "gateway-request-volume",
    x: 0,
    y: 0,
  },
  {
    h: USAGE_GATEWAY_WIDGET_HEIGHT,
    i: "gateway-error-rate#1",
    title: "Gateway error rate",
    w: 1,
    widgetType: "gateway-error-rate",
    x: 1,
    y: 0,
  },
  {
    h: USAGE_GATEWAY_WIDGET_HEIGHT,
    i: "active-gateways#1",
    title: "Active gateways",
    w: 1,
    widgetType: "active-gateways",
    x: 2,
    y: 0,
  },
  {
    h: USAGE_GATEWAY_WIDGET_HEIGHT,
    i: "unused-gateways#1",
    title: "Provisioned but unused",
    w: 1,
    widgetType: "unused-gateways",
    x: 3,
    y: 0,
  },
  {
    h: USAGE_DETAIL_WIDGET_HEIGHT,
    i: "sandbox-utilization#1",
    title: "Sandbox utilization",
    w: 2,
    widgetType: "sandbox-utilization",
    x: 0,
    y: USAGE_GATEWAY_WIDGET_HEIGHT,
  },
  {
    h: USAGE_DETAIL_WIDGET_HEIGHT,
    i: "sandbox-lifetime#1",
    title: "Sandbox lifetime",
    w: 2,
    widgetType: "sandbox-lifetime",
    x: 2,
    y: USAGE_GATEWAY_WIDGET_HEIGHT,
  },
] as const;

let stackedY = 0;
const mobileLayout = desktopLayout.map((item) => {
  const positioned = { ...item, x: 0, y: stackedY };
  stackedY += item.h;
  return positioned;
});

export const defaultUsageDashboardLayoutTemplate: ExtendedTemplateConfig = {
  xl: [...desktopLayout],
  lg: [...desktopLayout],
  md: [...desktopLayout],
  sm: [...mobileLayout],
};

export function localizeUsageDashboardLayoutTemplate(
  template: ExtendedTemplateConfig,
  intl: IntlShape,
): ExtendedTemplateConfig {
  return (Object.keys(template) as (keyof ExtendedTemplateConfig)[]).reduce(
    (localized, variant) => {
      localized[variant] = template[variant].map((item) => ({
        ...item,
        title: intl.formatMessage(
          WIDGET_TITLE_MESSAGES[item.widgetType as UsageWidgetType],
        ),
      }));
      return localized;
    },
    {} as ExtendedTemplateConfig,
  );
}
