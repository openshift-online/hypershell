import { useMemo } from "react";
import { useIntl } from "react-intl";

import type { OperationalMetric } from "../application/dashboard-types";
import { messages } from "../messages";
import { buildPodCapacityData } from "./pod-capacity-data";
import { isPodCapacityMetric } from "./pod-capacity-metric";
import {
  formatOperationalMetricDisplayValue,
  isDisplayableOperationalMetricValue,
} from "./operational-metric-display";
import { StatusDonutChart } from "./status-donut-chart";
import type { StatusDonutDatum } from "./status-donut-data";

export function PodCapacityChart({
  metric,
}: Readonly<{ metric: OperationalMetric }>) {
  const intl = useIntl();
  const displayValue = formatOperationalMetricDisplayValue(metric.value, intl);

  const { colorScale, data, legendData } = useMemo(() => {
    if (!isPodCapacityMetric(metric)) {
      return { colorScale: [], data: [], legendData: [] };
    }

    return buildPodCapacityData(
      intl,
      Number(metric.value),
      Number(metric.total),
      metric.podPhases,
    );
  }, [intl, metric]);

  if (!isPodCapacityMetric(metric)) {
    return null;
  }

  return (
    <StatusDonutChart
      ariaDesc={intl.formatMessage(messages.podStatusAriaDesc)}
      ariaTitle={intl.formatMessage(messages.podStatusChartTitle)}
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
      subTitle={
        isDisplayableOperationalMetricValue(metric.total)
          ? intl.formatMessage(messages.utilizationSubtitle, {
              total: metric.total,
              unit: metric.unit,
            })
          : undefined
      }
      title={displayValue}
    />
  );
}
