import type { IntlShape, MessageDescriptor } from "react-intl";

import type {
  OperationalMetric,
  ProvisionSuccessCountWindow,
} from "../application/dashboard-types";
import { messages } from "../messages";
import { STATUS_DONUT_COLORS } from "./status-donut-colors";
import {
  buildStatusDonutData,
  type StatusDonutSeries,
} from "./status-donut-data";

export const PROVISION_RELIABILITY_HEALTHY_THRESHOLD_PERCENT = 99;
export const PROVISION_RELIABILITY_WARNING_THRESHOLD_PERCENT = 95;

export function getProvisionReliabilityWindowCaptionMessage(
  window: ProvisionSuccessCountWindow | undefined,
): MessageDescriptor {
  if (window === "duration_lifetime") {
    return messages.provisionReliabilityLifetimeSuccessCount;
  }

  return messages.provisionReliabilityLast24Hours;
}

export function getProvisionReliabilitySummaryTermMessage(
  window: ProvisionSuccessCountWindow | undefined,
): MessageDescriptor {
  if (window === "duration_lifetime") {
    return messages.provisionSuccessRateLifetime;
  }

  return messages.provisionSuccessRate24h;
}

export type ProvisionReliabilityStatusLevel = "danger" | "healthy" | "warning";

export interface ProvisionReliabilityStats {
  failureCount: number;
  successCount: number;
  successRatePercent: number;
}

export function getProvisionReliabilityStatusLevel(
  successRatePercent: number,
): ProvisionReliabilityStatusLevel {
  if (successRatePercent >= PROVISION_RELIABILITY_HEALTHY_THRESHOLD_PERCENT) {
    return "healthy";
  }
  if (successRatePercent >= PROVISION_RELIABILITY_WARNING_THRESHOLD_PERCENT) {
    return "warning";
  }
  return "danger";
}

export function getProvisionSuccessColor(successRatePercent: number): string {
  switch (getProvisionReliabilityStatusLevel(successRatePercent)) {
    case "healthy":
      return STATUS_DONUT_COLORS.healthy;
    case "warning":
      return STATUS_DONUT_COLORS.degraded;
    case "danger":
      return STATUS_DONUT_COLORS.failed;
  }
}

export function parseProvisionReliabilityStats(
  metric: OperationalMetric,
): ProvisionReliabilityStats | undefined {
  const outcomes = metric.provisionOutcomes;
  if (outcomes === undefined) {
    return undefined;
  }

  const successCount = Number(outcomes.successCount24h);
  const failureCount = Number(outcomes.failureCount24h);
  const successRatePercent = Number(outcomes.successRatePercent);

  if (
    !Number.isFinite(successCount) ||
    !Number.isFinite(failureCount) ||
    !Number.isFinite(successRatePercent) ||
    successCount < 0 ||
    failureCount < 0
  ) {
    return undefined;
  }

  return {
    failureCount,
    successCount,
    successRatePercent,
  };
}

export function buildProvisionReliabilityData(
  intl: IntlShape,
  stats: ProvisionReliabilityStats,
): StatusDonutSeries {
  const successLabel = intl.formatMessage(
    messages.provisionReliabilitySuccesses,
  );
  const failureLabel = intl.formatMessage(
    messages.provisionReliabilityFailures,
  );

  return buildStatusDonutData([
    {
      color: getProvisionSuccessColor(stats.successRatePercent),
      count: stats.successCount,
      label: successLabel,
      legendName: intl.formatMessage(messages.statusDonutLegend, {
        count: stats.successCount,
        status: successLabel,
      }),
    },
    {
      color: STATUS_DONUT_COLORS.failed,
      count: stats.failureCount,
      label: failureLabel,
      legendName: intl.formatMessage(messages.statusDonutLegend, {
        count: stats.failureCount,
        status: failureLabel,
      }),
    },
  ]);
}
