import type { ExtendedTemplateConfig } from "@patternfly/widgetized-dashboard";
import type { IntlShape } from "react-intl";

import { messages } from "../messages";

export const RELIABILITY_DASHBOARD_COLUMN_COUNT = 3;
export const RELIABILITY_SUMMARY_WIDGET_HEIGHT = 6;
const RELIABILITY_MOBILE_SUMMARY_WIDGET_HEIGHT = 8;
export const RELIABILITY_TREND_WIDGET_HEIGHT = 5;

const WIDGET_TITLE_MESSAGES = {
  "reliability-summary": messages.reliabilitySummaryWidget,
  "api-request-rate": messages.apiRequestRateWidget,
  "api-error-rate": messages.apiErrorRateWidget,
  "api-latency": messages.apiLatencyWidget,
  "api-p95-latency": messages.apiP95LatencyWidget,
  "oidc-client-sync": messages.oidcClientSyncWidget,
  "state-accuracy": messages.stateAccuracyWidget,
  "reconciliation-failures": messages.widgetReconciliationFailures,
  "reconciliation-retries": messages.widgetReconciliationRetries,
  "reconciliation-lag": messages.widgetReconciliationLag,
  "stale-resource-status-count": messages.widgetStaleResourceStatus,
} as const;

type ReliabilityWidgetType = keyof typeof WIDGET_TITLE_MESSAGES;

const threeColumnLayout = [
  {
    h: RELIABILITY_SUMMARY_WIDGET_HEIGHT,
    i: "reliability-summary#1",
    title: "Reliability summary",
    w: RELIABILITY_DASHBOARD_COLUMN_COUNT,
    widgetType: "reliability-summary",
    x: 0,
    y: 0,
  },
  {
    h: RELIABILITY_TREND_WIDGET_HEIGHT,
    i: "api-request-rate#1",
    title: "API request rate",
    w: 1,
    widgetType: "api-request-rate",
    x: 0,
    y: RELIABILITY_SUMMARY_WIDGET_HEIGHT,
  },
  {
    h: RELIABILITY_TREND_WIDGET_HEIGHT,
    i: "api-error-rate#1",
    title: "API error rate",
    w: 1,
    widgetType: "api-error-rate",
    x: 1,
    y: RELIABILITY_SUMMARY_WIDGET_HEIGHT,
  },
  {
    h: RELIABILITY_TREND_WIDGET_HEIGHT,
    i: "api-latency#1",
    title: "API latency",
    w: 1,
    widgetType: "api-latency",
    x: 2,
    y: RELIABILITY_SUMMARY_WIDGET_HEIGHT,
  },
  {
    h: RELIABILITY_TREND_WIDGET_HEIGHT,
    i: "api-p95-latency#1",
    title: "API p95 latency",
    w: 1,
    widgetType: "api-p95-latency",
    x: 0,
    y: RELIABILITY_SUMMARY_WIDGET_HEIGHT + RELIABILITY_TREND_WIDGET_HEIGHT,
  },
  {
    h: RELIABILITY_TREND_WIDGET_HEIGHT,
    i: "oidc-client-sync#1",
    title: "OIDC client sync",
    w: 1,
    widgetType: "oidc-client-sync",
    x: 1,
    y: RELIABILITY_SUMMARY_WIDGET_HEIGHT + RELIABILITY_TREND_WIDGET_HEIGHT,
  },
  {
    h: RELIABILITY_TREND_WIDGET_HEIGHT,
    i: "state-accuracy#1",
    title: "State accuracy",
    w: 1,
    widgetType: "state-accuracy",
    x: 2,
    y: RELIABILITY_SUMMARY_WIDGET_HEIGHT + RELIABILITY_TREND_WIDGET_HEIGHT,
  },
  {
    h: RELIABILITY_TREND_WIDGET_HEIGHT,
    i: "reconciliation-failures#1",
    title: "Reconciliation failures",
    w: 1,
    widgetType: "reconciliation-failures",
    x: 0,
    y: RELIABILITY_SUMMARY_WIDGET_HEIGHT + RELIABILITY_TREND_WIDGET_HEIGHT * 2,
  },
  {
    h: RELIABILITY_TREND_WIDGET_HEIGHT,
    i: "reconciliation-retries#1",
    title: "Reconciliation retries",
    w: 1,
    widgetType: "reconciliation-retries",
    x: 1,
    y: RELIABILITY_SUMMARY_WIDGET_HEIGHT + RELIABILITY_TREND_WIDGET_HEIGHT * 2,
  },
  {
    h: RELIABILITY_TREND_WIDGET_HEIGHT,
    i: "reconciliation-lag#1",
    title: "Reconciliation lag",
    w: 1,
    widgetType: "reconciliation-lag",
    x: 2,
    y: RELIABILITY_SUMMARY_WIDGET_HEIGHT + RELIABILITY_TREND_WIDGET_HEIGHT * 2,
  },
  {
    h: RELIABILITY_TREND_WIDGET_HEIGHT,
    i: "stale-resource-status-count#1",
    title: "Stale resource status",
    w: 1,
    widgetType: "stale-resource-status-count",
    x: 0,
    y: RELIABILITY_SUMMARY_WIDGET_HEIGHT + RELIABILITY_TREND_WIDGET_HEIGHT * 3,
  },
] as const;

const mobileLayout = threeColumnLayout.map((item, index) => ({
  ...item,
  h: index === 0 ? RELIABILITY_MOBILE_SUMMARY_WIDGET_HEIGHT : item.h,
  w: 1,
  x: 0,
  y:
    index === 0
      ? 0
      : RELIABILITY_MOBILE_SUMMARY_WIDGET_HEIGHT +
        RELIABILITY_TREND_WIDGET_HEIGHT * (index - 1),
}));

export const defaultReliabilityDashboardLayoutTemplate: ExtendedTemplateConfig =
  {
    xl: [...threeColumnLayout],
    lg: [...threeColumnLayout],
    md: [...threeColumnLayout],
    sm: [...mobileLayout],
  };

export function localizeReliabilityDashboardLayoutTemplate(
  template: ExtendedTemplateConfig,
  intl: IntlShape,
): ExtendedTemplateConfig {
  return (Object.keys(template) as (keyof ExtendedTemplateConfig)[]).reduce(
    (localized, variant) => {
      localized[variant] = template[variant].map((item) => {
        const widgetType = item.widgetType as ReliabilityWidgetType;

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
