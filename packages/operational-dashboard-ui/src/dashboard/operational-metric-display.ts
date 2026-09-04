import type { IntlShape } from "react-intl";

import { messages } from "../messages";

const NON_DISPLAYABLE_LITERALS = new Set(["NaN", "Infinity", "-Infinity"]);

export function isDisplayableOperationalMetricValue(value: string): boolean {
  if (NON_DISPLAYABLE_LITERALS.has(value)) {
    return false;
  }

  return Number.isFinite(Number(value));
}

export function formatOperationalMetricDisplayValue(
  value: string,
  intl: IntlShape,
): string {
  if (!isDisplayableOperationalMetricValue(value)) {
    return intl.formatMessage(messages.metricCouldNotBeDetermined);
  }

  return value;
}
