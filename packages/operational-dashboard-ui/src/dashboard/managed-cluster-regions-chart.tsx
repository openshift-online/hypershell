import { useMemo } from "react";
import { useIntl } from "react-intl";

import type { OperationalMetric } from "../application/dashboard-types";
import { buildInventoryDimensionDonutData } from "./inventory-dimension-donut-data";
import { messages } from "../messages";
import { formatOperationalMetricDisplayValue } from "./operational-metric-display";
import { StatusDonutChart } from "./status-donut-chart";
import type { StatusDonutDatum } from "./status-donut-data";

export function ManagedClusterRegionsChart({
  metric,
}: Readonly<{ metric: OperationalMetric }>) {
  const intl = useIntl();

  const { colorScale, data, legendData } = useMemo(
    () => buildInventoryDimensionDonutData(intl, metric.inventoryRegions),
    [intl, metric.inventoryRegions],
  );

  if (data.length === 0) {
    return null;
  }

  return (
    <StatusDonutChart
      ariaDesc={intl.formatMessage(messages.managedClusterRegionsAriaDesc)}
      ariaTitle={intl.formatMessage(messages.managedClusterRegionsChartTitle)}
      colorScale={colorScale}
      data={data}
      dataLabel={(datum: StatusDonutDatum) =>
        datum.x
          ? intl.formatMessage(messages.statusDonutDataLabel, {
              count: datum.y,
              status: datum.x,
            })
          : null
      }
      legendData={legendData}
      size="compact"
      title={formatOperationalMetricDisplayValue(metric.value, intl)}
    />
  );
}
