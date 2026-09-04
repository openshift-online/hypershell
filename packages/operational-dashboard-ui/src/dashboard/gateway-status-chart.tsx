import { useMemo } from "react";
import { useIntl } from "react-intl";

import type { OperationalMetric } from "../application/dashboard-types";
import { messages } from "../messages";
import { buildGatewayStatusData } from "./gateway-status-data";
import { formatOperationalMetricDisplayValue } from "./operational-metric-display";
import { isStatusDonutMetric } from "./status-donut-metric";
import { StatusDonutChart } from "./status-donut-chart";
import type { StatusDonutDatum } from "./status-donut-data";

export { isStatusDonutMetric as isGatewayStatusMetric } from "./status-donut-metric";

export function GatewayStatusChart({
  metric,
}: Readonly<{ metric: OperationalMetric }>) {
  const intl = useIntl();

  const { colorScale, data, legendData } = useMemo(() => {
    if (!isStatusDonutMetric(metric)) {
      return { colorScale: [], data: [], legendData: [] };
    }

    return buildGatewayStatusData(intl, metric.status);
  }, [intl, metric]);

  if (!isStatusDonutMetric(metric)) {
    return null;
  }

  return (
    <StatusDonutChart
      ariaDesc={intl.formatMessage(messages.gatewayStatusAriaDesc)}
      ariaTitle={intl.formatMessage(messages.gatewayStatusChartTitle)}
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
      subTitle={intl.formatMessage(messages.gateways)}
      title={formatOperationalMetricDisplayValue(metric.value, intl)}
    />
  );
}
