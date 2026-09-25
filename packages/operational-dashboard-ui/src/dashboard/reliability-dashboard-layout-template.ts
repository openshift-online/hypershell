import type { ExtendedTemplateConfig } from "@patternfly/widgetized-dashboard";
import type { IntlShape } from "react-intl";

import { messages } from "../messages";

export const RELIABILITY_DASHBOARD_COLUMN_COUNT = 3;
export const RELIABILITY_SUMMARY_WIDGET_HEIGHT = 4;
export const RELIABILITY_TREND_WIDGET_HEIGHT = 5;

const WIDGET_TITLE_MESSAGES = {
  "reliability-summary": messages.reliabilitySummaryWidget,
  "api-request-rate": messages.apiRequestRateWidget,
  "api-error-rate": messages.apiErrorRateWidget,
  "api-latency": messages.apiLatencyWidget,
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
] as const;

const mobileLayout = [
  {
    h: RELIABILITY_SUMMARY_WIDGET_HEIGHT,
    i: "reliability-summary#1",
    title: "Reliability summary",
    w: 1,
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
    x: 0,
    y: RELIABILITY_SUMMARY_WIDGET_HEIGHT + RELIABILITY_TREND_WIDGET_HEIGHT,
  },
  {
    h: RELIABILITY_TREND_WIDGET_HEIGHT,
    i: "api-latency#1",
    title: "API latency",
    w: 1,
    widgetType: "api-latency",
    x: 0,
    y: RELIABILITY_SUMMARY_WIDGET_HEIGHT + RELIABILITY_TREND_WIDGET_HEIGHT * 2,
  },
] as const;

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
