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

function buildSandboxLifecycleData(
  intl: IntlShape,
  metric: {
    expiringSandboxes?: number;
    idleSandboxes?: number;
    orphanedSandboxes?: number;
    value: string;
  },
): StatusDonutSeries {
  const total = Number(metric.value);
  const expiring = metric.expiringSandboxes ?? 0;
  const idle = metric.idleSandboxes ?? 0;
  const orphaned = metric.orphanedSandboxes ?? 0;
  const active = Math.max(0, total - expiring - idle - orphaned);
  const entries = [
    [messages.sandboxStatusActive, active, SANDBOX_STATUS_COLORS.healthy],
    [
      messages.sandboxAttentionExpiringSoon,
      expiring,
      SANDBOX_STATUS_COLORS.degraded,
    ],
    [messages.sandboxAttentionIdle, idle, SANDBOX_STATUS_COLORS.provisioning],
    [messages.sandboxAttentionOrphaned, orphaned, SANDBOX_STATUS_COLORS.failed],
  ] as const;

  return buildStatusDonutData(
    entries.map(([message, count, color]) => {
      const label = intl
        .formatMessage(message, { count })
        .replace(/^\d+\s*/, "");
      return {
        color,
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

export function buildSandboxStatusData(
  intl: IntlShape,
  metric: {
    expiringSandboxes?: number;
    idleSandboxes?: number;
    orphanedSandboxes?: number;
    status?: OperationalMetricStatus;
    value: string;
  },
): StatusDonutSeries {
  if (metric.status !== undefined) {
    return buildSandboxStatusBreakdownData(intl, metric.status);
  }

  if (
    metric.expiringSandboxes !== undefined ||
    metric.idleSandboxes !== undefined ||
    metric.orphanedSandboxes !== undefined
  ) {
    return buildSandboxLifecycleData(intl, metric);
  }

  const parsed = Number(metric.value);
  const activeCount = Number.isFinite(parsed) ? Math.max(0, parsed) : 0;

  return buildSandboxActiveTotalData(intl, activeCount);
}
