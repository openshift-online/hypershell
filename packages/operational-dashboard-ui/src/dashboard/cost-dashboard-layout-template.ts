import type { ExtendedTemplateConfig } from "@patternfly/widgetized-dashboard";
import type { IntlShape } from "react-intl";

import type { CostDashboardView } from "../application/cost-dashboard-types";
import { costMessages } from "../cost-messages";

export const COST_DASHBOARD_COLUMN_COUNT = 4;

/** Widget types per view, with their title message (WEB-COST-08, WEB-COST-10). */
export const COST_WIDGET_TITLE_MESSAGES = {
  "cost-summary": costMessages.costWidgetSummary,
  "cost-idle": costMessages.costWidgetIdle,
  "cost-per-user": costMessages.costWidgetPerUser,
  "cost-by-environment": costMessages.costWidgetByEnvironment,
  "cost-by-cluster": costMessages.costWidgetByCluster,
  "cost-by-resource-class": costMessages.costWidgetByResourceClass,
  "cost-top-gateways": costMessages.costWidgetTopGateways,
  "cost-by-billing-owner": costMessages.costWidgetByBillingOwner,
  "my-cost-charged": costMessages.costWidgetCharged,
  "my-cost-shared": costMessages.costWidgetShared,
  "my-cost-trend": costMessages.costWidgetTrend,
  "my-cost-by-gateway": costMessages.costWidgetByGateway,
  "my-cost-by-workspace": costMessages.costWidgetByWorkspace,
} as const;

export type CostWidgetType = keyof typeof COST_WIDGET_TITLE_MESSAGES;

interface Placement {
  h: number;
  /** Height on narrow screens, where table rows stack into cards. */
  narrowH?: number;
  type: CostWidgetType;
  w: number;
  x: number;
  y: number;
}

const platformPlacements: readonly Placement[] = [
  { type: "cost-summary", h: 4, w: 2, x: 0, y: 0 },
  { type: "cost-idle", h: 4, w: 1, x: 2, y: 0 },
  { type: "cost-per-user", h: 4, w: 1, x: 3, y: 0 },
  { type: "cost-by-environment", h: 5, w: 2, x: 0, y: 4 },
  { type: "cost-by-cluster", h: 5, w: 1, x: 2, y: 4 },
  { type: "cost-by-resource-class", h: 5, w: 1, x: 3, y: 4 },
  { type: "cost-top-gateways", h: 4, narrowH: 8, w: 2, x: 0, y: 9 },
  { type: "cost-by-billing-owner", h: 4, narrowH: 10, w: 2, x: 2, y: 9 },
];

const minePlacements: readonly Placement[] = [
  { type: "my-cost-charged", h: 3, w: 2, x: 0, y: 0 },
  { type: "my-cost-shared", h: 3, w: 2, x: 2, y: 0 },
  { type: "my-cost-trend", h: 3, w: 4, x: 0, y: 3 },
  { type: "my-cost-by-gateway", h: 4, narrowH: 11, w: 4, x: 0, y: 6 },
  { type: "my-cost-by-workspace", h: 4, narrowH: 7, w: 4, x: 0, y: 10 },
];

function toTemplate(placements: readonly Placement[]): ExtendedTemplateConfig {
  const desktop = placements.map((placement) => ({
    h: placement.h,
    i: `${placement.type}#1`,
    title: placement.type,
    w: placement.w,
    widgetType: placement.type,
    x: placement.x,
    y: placement.y,
  }));
  let stackedY = 0;
  const mobile = desktop.map((item, index) => {
    const h = placements[index]?.narrowH ?? item.h;
    const positioned = { ...item, h, w: 1, x: 0, y: stackedY };
    stackedY += h;
    return positioned;
  });
  return {
    xl: desktop.map((item) => ({ ...item })),
    lg: desktop.map((item) => ({ ...item })),
    md: desktop.map((item) => ({ ...item })),
    sm: mobile,
  };
}

export const defaultCostDashboardLayoutTemplates: Readonly<
  Record<CostDashboardView, ExtendedTemplateConfig>
> = {
  mine: toTemplate(minePlacements),
  platform: toTemplate(platformPlacements),
};

/** Default height and width of each widget type, used when it is re-added. */
export const costWidgetDefaults: Readonly<
  Record<CostWidgetType, { h: number; w: number }>
> = Object.fromEntries(
  [...platformPlacements, ...minePlacements].map((placement) => [
    placement.type,
    { h: placement.h, w: placement.w },
  ]),
) as Record<CostWidgetType, { h: number; w: number }>;

export function isCostWidgetType(type: string): type is CostWidgetType {
  return Object.hasOwn(COST_WIDGET_TITLE_MESSAGES, type);
}

export function localizeCostDashboardLayoutTemplate(
  template: ExtendedTemplateConfig,
  intl: IntlShape,
): ExtendedTemplateConfig {
  return (Object.keys(template) as (keyof ExtendedTemplateConfig)[]).reduce(
    (localized, variant) => {
      localized[variant] = template[variant]
        .filter((item) => isCostWidgetType(item.widgetType))
        .map((item) => ({
          ...item,
          title: intl.formatMessage(
            COST_WIDGET_TITLE_MESSAGES[item.widgetType as CostWidgetType],
          ),
        }));
      return localized;
    },
    {} as ExtendedTemplateConfig,
  );
}
