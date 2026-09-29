import { Title } from "@patternfly/react-core";
import { useMemo } from "react";
import { useIntl } from "react-intl";

import type { OperationalMetric } from "../application/dashboard-types";
import { messages } from "../messages";
import { formatOperationalMetricDisplayValue } from "./operational-metric-display";
import { buildSandboxStatusData } from "./sandbox-status-data";
import { StatusDonutChart } from "./status-donut-chart";
import type { StatusDonutDatum } from "./status-donut-data";

export function SandboxStatusChart({
  metric,
}: Readonly<{ metric: OperationalMetric }>) {
  const intl = useIntl();

  const { colorScale, data, legendData } = useMemo(
    () => buildSandboxStatusData(intl, metric),
    [intl, metric],
  );

  const title = formatOperationalMetricDisplayValue(metric.value, intl);
  const subTitle = intl.formatMessage(messages.widgetSandboxes);

  if (data.length === 0) {
    return (
      <div className="hypershell-dashboard-sandbox-status-total">
        <Title headingLevel="h3" size="4xl">
          {title}
        </Title>
        <Title headingLevel="h4" size="md">
          {subTitle}
        </Title>
      </div>
    );
  }

  return (
    <StatusDonutChart
      ariaDesc={intl.formatMessage(messages.sandboxStatusAriaDesc)}
      ariaTitle={intl.formatMessage(messages.sandboxStatusChartTitle)}
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
      subTitle={subTitle}
      title={title}
    />
  );
}
