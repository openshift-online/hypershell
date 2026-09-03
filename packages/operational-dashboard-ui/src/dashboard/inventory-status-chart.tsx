import { useMemo } from "react";
import { useIntl } from "react-intl";

import type { OperationalMetric } from "../application/dashboard-types";
import { messages } from "../messages";
import { buildInventoryStatusData } from "./inventory-status-data";
import { formatOperationalMetricDisplayValue } from "./operational-metric-display";
import { StatusDonutChart } from "./status-donut-chart";
import type { StatusDonutDatum } from "./status-donut-data";

export function InventoryStatusChart({
  ariaDescMessage,
  ariaTitleMessage,
  metric,
}: Readonly<{
  ariaDescMessage: (typeof messages)[keyof typeof messages];
  ariaTitleMessage: (typeof messages)[keyof typeof messages];
  metric: OperationalMetric;
}>) {
  const intl = useIntl();

  const { colorScale, data, legendData } = useMemo(
    () => buildInventoryStatusData(intl, metric.inventoryStatus),
    [intl, metric.inventoryStatus],
  );

  if (data.length === 0) {
    return null;
  }

  return (
    <StatusDonutChart
      ariaDesc={intl.formatMessage(ariaDescMessage)}
      ariaTitle={intl.formatMessage(ariaTitleMessage)}
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
