import type { IntlShape } from "react-intl";

import type { OperationalMetricStatus } from "../application/dashboard-types";
import { messages } from "../messages";
import {
  GATEWAY_STATUS_COLORS,
  GATEWAY_STATUS_ORDER,
} from "./gateway-status-data";
import {
  buildStatusDonutData,
  type StatusDonutDatum,
  type StatusDonutLegendDatum,
  type StatusDonutSeries,
} from "./status-donut-data";

export const SANDBOX_STATUS_COLORS = GATEWAY_STATUS_COLORS;

export const SANDBOX_STATUS_ORDER = GATEWAY_STATUS_ORDER;

export type SandboxStatusKey = (typeof SANDBOX_STATUS_ORDER)[number];

export type SandboxStatusDatum = StatusDonutDatum;
export type SandboxStatusLegendDatum = StatusDonutLegendDatum;

function sandboxStatusLabel(intl: IntlShape, status: SandboxStatusKey): string {
  switch (status) {
    case "healthy":
      return intl.formatMessage(messages.sandboxStatusActive);
    case "provisioning":
      return intl.formatMessage(messages.gatewayStatusProvisioning);
    case "degraded":
      return intl.formatMessage(messages.gatewayStatusDegraded);
    case "failed":
      return intl.formatMessage(messages.gatewayStatusFailed);
  }
}

function buildSandboxStatusBreakdownData(
  intl: IntlShape,
  status: OperationalMetricStatus,
): StatusDonutSeries {
  return buildStatusDonutData(
    SANDBOX_STATUS_ORDER.map((key) => {
      const count = status[key] ?? 0;
      const label = sandboxStatusLabel(intl, key);

      return {
        color: SANDBOX_STATUS_COLORS[key],
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

function buildSandboxActiveTotalData(
  intl: IntlShape,
  activeCount: number,
): StatusDonutSeries {
  const label = intl.formatMessage(messages.sandboxStatusActive);

  return buildStatusDonutData([
    {
      color: SANDBOX_STATUS_COLORS.healthy,
      count: activeCount,
      label,
      legendName: intl.formatMessage(messages.statusDonutLegend, {
        count: activeCount,
        status: label,
      }),
    },
  ]);
}

export function buildSandboxStatusData(
  intl: IntlShape,
  metric: { status?: OperationalMetricStatus; value: string },
): StatusDonutSeries {
  if (metric.status !== undefined) {
    return buildSandboxStatusBreakdownData(intl, metric.status);
  }

  const parsed = Number(metric.value);
  const activeCount = Number.isFinite(parsed) ? Math.max(0, parsed) : 0;

  return buildSandboxActiveTotalData(intl, activeCount);
}
