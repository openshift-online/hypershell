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

export const GATEWAY_STATUS_COLORS = STATUS_DONUT_COLORS;

export const GATEWAY_STATUS_ORDER = [
  "healthy",
  "provisioning",
  "degraded",
  "failed",
] as const satisfies readonly (keyof OperationalMetricStatus)[];

export type GatewayStatusKey = (typeof GATEWAY_STATUS_ORDER)[number];

export type GatewayStatusDatum = StatusDonutDatum;
export type GatewayStatusLegendDatum = StatusDonutLegendDatum;

function gatewayStatusLabel(intl: IntlShape, status: GatewayStatusKey): string {
  switch (status) {
    case "healthy":
      return intl.formatMessage(messages.gatewayStatusHealthy);
    case "provisioning":
      return intl.formatMessage(messages.gatewayStatusProvisioning);
    case "degraded":
      return intl.formatMessage(messages.gatewayStatusDegraded);
    case "failed":
      return intl.formatMessage(messages.gatewayStatusFailed);
  }
}

export function buildGatewayStatusData(
  intl: IntlShape,
  status: OperationalMetricStatus,
): StatusDonutSeries {
  return buildStatusDonutData(
    GATEWAY_STATUS_ORDER.map((key) => {
      const count = status[key] ?? 0;
      const label = gatewayStatusLabel(intl, key);

      return {
        color: GATEWAY_STATUS_COLORS[key],
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
