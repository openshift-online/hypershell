import { Stack, StackItem, Title } from "@patternfly/react-core";
import { useMemo } from "react";
import { FormattedMessage, useIntl } from "react-intl";

import type { OperationalMetric } from "../application/dashboard-types";
import { messages } from "../messages";
import {
  buildProvisionReliabilityData,
  getProvisionReliabilityStatusLevel,
  getProvisionReliabilityWindowCaptionMessage,
  parseProvisionReliabilityStats,
} from "./provision-reliability-data";
import { StatusDonutChart } from "./status-donut-chart";
import type { StatusDonutDatum } from "./status-donut-data";
import { TrendSparklineChart } from "./trend-sparkline-chart";

export function ProvisionReliabilityChart({
  metric,
}: Readonly<{ metric: OperationalMetric }>) {
  const intl = useIntl();
  const stats = useMemo(() => parseProvisionReliabilityStats(metric), [metric]);

  const donutData = useMemo(() => {
    if (!stats) {
      return { colorScale: [], data: [], legendData: [] };
    }

    return buildProvisionReliabilityData(intl, stats);
  }, [intl, stats]);

  if (!stats) {
    return null;
  }

  const statusLevel = getProvisionReliabilityStatusLevel(
    stats.successRatePercent,
  );
  const title = intl.formatMessage(messages.provisionReliabilityRate, {
    rate: stats.successRatePercent.toFixed(1),
  });
  const windowCaptionMessage = getProvisionReliabilityWindowCaptionMessage(
    metric.provisionOutcomes?.successCountWindow,
  );
  const subtitle = intl.formatMessage(windowCaptionMessage);
  const sparklineTitle = intl.formatMessage(
    messages.provisionReliabilityHourlySuccessRate,
  );
  const sparklineCaption = intl.formatMessage(windowCaptionMessage);

  return (
    <Stack hasGutter className="hypershell-dashboard-provision-reliability">
      <StackItem>
        <div
          className={`hypershell-dashboard-provision-reliability__donut hypershell-dashboard-provision-reliability__donut--${statusLevel}`}
        >
          <StatusDonutChart
            ariaDesc={intl.formatMessage(messages.provisionReliabilityAriaDesc)}
            ariaTitle={intl.formatMessage(
              messages.provisionReliabilityChartTitle,
            )}
            colorScale={donutData.colorScale}
            data={donutData.data}
            dataLabel={(datum: StatusDonutDatum) =>
              datum.x
                ? intl.formatMessage(messages.statusDonutDataLabel, {
                    count: datum.y,
                    status: datum.x,
                  })
                : null
            }
            legendData={donutData.legendData}
            size="compact"
            subTitle={subtitle}
            title={title}
          />
        </div>
      </StackItem>
      {metric.successRateTrend && metric.successRateTrend.points.length >= 2 ? (
        <StackItem>
          <Title headingLevel="h4" size="md">
            <FormattedMessage
              {...messages.provisionReliabilityHourlySuccessRate}
            />
          </Title>
          <TrendSparklineChart
            caption={sparklineCaption}
            trend={metric.successRateTrend}
            title={sparklineTitle}
          />
        </StackItem>
      ) : null}
    </Stack>
  );
}
