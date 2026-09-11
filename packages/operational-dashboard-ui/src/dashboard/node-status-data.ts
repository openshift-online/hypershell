import type { IntlShape } from "react-intl";

import type { OperationalMetricStatus } from "../application/dashboard-types";
import { messages } from "../messages";
import { STATUS_DONUT_COLORS } from "./status-donut-colors";
import {
  buildStatusDonutData,
  type StatusDonutDatum,
  type StatusDonutLegendDatum,
  type StatusDonutSeries,
} from "./status-donut-data";

export const NODE_STATUS_ORDER = [
  "healthy",
  "failed",
] as const satisfies readonly (keyof OperationalMetricStatus)[];

type NodeStatusKey = (typeof NODE_STATUS_ORDER)[number];

export type NodeStatusDatum = StatusDonutDatum;
export type NodeStatusLegendDatum = StatusDonutLegendDatum;

function nodeStatusLabel(intl: IntlShape, status: NodeStatusKey): string {
  switch (status) {
    case "healthy":
      return intl.formatMessage(messages.nodeStatusReady);
    case "failed":
      return intl.formatMessage(messages.nodeStatusNotReady);
  }
}

export function buildNodeStatusData(
  intl: IntlShape,
  status: OperationalMetricStatus,
): StatusDonutSeries {
  return buildStatusDonutData(
    NODE_STATUS_ORDER.map((key) => {
      const count = status[key] ?? 0;
      const label = nodeStatusLabel(intl, key);

      return {
        color: STATUS_DONUT_COLORS[key],
        count,
        label,
        legendName: intl.formatMessage(messages.statusDonutLegend, {
          count,
          status: label,
        }),
      };
    }),
  );
}
