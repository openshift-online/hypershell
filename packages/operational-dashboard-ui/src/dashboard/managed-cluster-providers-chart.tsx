import { useMemo } from "react";
import { useIntl } from "react-intl";

import type { OperationalMetric } from "../application/dashboard-types";
import { buildInventoryDimensionDonutData } from "./inventory-dimension-donut-data";
import { messages } from "../messages";
import { formatOperationalMetricDisplayValue } from "./operational-metric-display";
import { StatusDonutChart } from "./status-donut-chart";
import type { StatusDonutDatum } from "./status-donut-data";

export function ManagedClusterProvidersChart({
  metric,
}: Readonly<{ metric: OperationalMetric }>) {
  const intl = useIntl();

  const { colorScale, data, legendData } = useMemo(
    () => buildInventoryDimensionDonutData(intl, metric.inventoryProviders),
    [intl, metric.inventoryProviders],
  );

  if (data.length === 0) {
    return null;
  }

  return (
    <StatusDonutChart
      ariaDesc={intl.formatMessage(messages.managedClusterProvidersAriaDesc)}
      ariaTitle={intl.formatMessage(messages.managedClusterProvidersChartTitle)}
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
