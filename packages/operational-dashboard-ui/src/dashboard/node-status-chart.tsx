import { useMemo } from "react";
import { useIntl } from "react-intl";

import type { OperationalMetric } from "../application/dashboard-types";
import { messages } from "../messages";
import { buildNodeStatusData } from "./node-status-data";
import { formatOperationalMetricDisplayValue } from "./operational-metric-display";
import { isStatusDonutMetric } from "./status-donut-metric";
import { StatusDonutChart } from "./status-donut-chart";
import type { StatusDonutDatum } from "./status-donut-data";

export function NodeStatusChart({
  metric,
  title,
}: Readonly<{ metric: OperationalMetric; title?: string }>) {
  const intl = useIntl();

  const { colorScale, data, legendData } = useMemo(() => {
    if (!isStatusDonutMetric(metric)) {
      return { colorScale: [], data: [], legendData: [] };
    }

    return buildNodeStatusData(intl, metric.status);
  }, [intl, metric]);

  if (!isStatusDonutMetric(metric)) {
    return null;
  }

  return (
    <StatusDonutChart
      ariaDesc={intl.formatMessage(messages.nodeStatusAriaDesc)}
      ariaTitle={intl.formatMessage(messages.nodeStatusChartTitle)}
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
      title={title ?? formatOperationalMetricDisplayValue(metric.value, intl)}
    />
  );
}
