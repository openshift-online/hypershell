import { useMemo } from "react";
import { useIntl } from "react-intl";

import type { OperationalMetric } from "../application/dashboard-types";
import { messages } from "../messages";
import { DashboardStatPanel } from "./dashboard-stat-panel";
import { buildGatewayReleaseListEntries } from "./gateway-release-distribution-data";
import { inventoryDimensionLabel } from "./inventory-dimension-donut-data";
import { formatOperationalMetricDisplayValue } from "./operational-metric-display";

export function GatewayReleasesChart({
  metric,
}: Readonly<{ metric: OperationalMetric }>) {
  const intl = useIntl();
  const entries = useMemo(
    () => buildGatewayReleaseListEntries(metric.releaseDistribution),
    [metric.releaseDistribution],
  );

  if (entries.length === 0) {
    return null;
  }

  return (
    <DashboardStatPanel
      ariaLabel={intl.formatMessage(messages.gatewayReleasesSummaryAriaLabel)}
      rows={entries.map((entry) => ({
        key: entry.label,
        label: inventoryDimensionLabel(intl, entry.label),
        value: formatOperationalMetricDisplayValue(String(entry.count), intl),
      }))}
    />
  );
}
