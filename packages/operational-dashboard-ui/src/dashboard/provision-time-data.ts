import type { IntlShape } from "react-intl";

import type { OperationalMetric } from "../application/dashboard-types";
import { messages } from "../messages";
import { isDisplayableOperationalMetricValue } from "./operational-metric-display";

export interface ProvisionDurationStats {
  meanMinutes: number;
  p50Minutes: number;
  p95Minutes: number;
}

function parseMinutes(value: string | undefined): number | undefined {
  if (value === undefined || !isDisplayableOperationalMetricValue(value)) {
    return undefined;
  }

  return Number(value);
}

export function parseProvisionDurationStats(
  metric: OperationalMetric,
): ProvisionDurationStats | undefined {
  const meanMinutes = parseMinutes(
    metric.provisionDuration?.mean ?? metric.value,
  );
  const p50Minutes = parseMinutes(metric.provisionDuration?.p50);
  const p95Minutes = parseMinutes(metric.provisionDuration?.p95);

  if (
    meanMinutes === undefined ||
    p50Minutes === undefined ||
    p95Minutes === undefined
  ) {
    return undefined;
  }

  return { meanMinutes, p50Minutes, p95Minutes };
}

export function formatProvisionDurationValue(
  intl: IntlShape,
  minutes: number,
  unit: string | undefined,
): string {
  const value = minutes.toFixed(2);

  if (unit) {
    return intl.formatMessage(messages.utilizationLabel, {
      unit,
      value,
    });
  }

  return value;
}
