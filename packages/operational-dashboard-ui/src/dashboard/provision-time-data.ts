import type { IntlShape } from "react-intl";

import type { OperationalMetric } from "../application/dashboard-types";
import { messages } from "../messages";
import { isDisplayableOperationalMetricValue } from "./operational-metric-display";

export interface ProvisionDurationStats {
  meanSeconds: number;
  p50Seconds: number;
  p95Seconds: number;
}

function parseSeconds(value: string | undefined): number | undefined {
  if (value === undefined || !isDisplayableOperationalMetricValue(value)) {
    return undefined;
  }

  return Number(value);
}

export function parseProvisionDurationStats(
  metric: OperationalMetric,
): ProvisionDurationStats | undefined {
  const meanSeconds = parseSeconds(
    metric.provisionDuration?.mean ?? metric.value,
  );
  const p50Seconds = parseSeconds(metric.provisionDuration?.p50);
  const p95Seconds = parseSeconds(metric.provisionDuration?.p95);

  if (
    meanSeconds === undefined ||
    p50Seconds === undefined ||
    p95Seconds === undefined
  ) {
    return undefined;
  }

  return { meanSeconds, p50Seconds, p95Seconds };
}

export function formatProvisionDurationValue(
  intl: IntlShape,
  seconds: number,
  unit: string | undefined,
): string {
  const value = seconds.toFixed(2);

  if (unit) {
    return intl.formatMessage(messages.utilizationLabel, {
      unit,
      value,
    });
  }

  return value;
}
