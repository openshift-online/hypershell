import { ChartDonutUtilization } from "@patternfly/react-charts/victory";
import { Flex, FlexItem, Title } from "@patternfly/react-core";
import { useIntl } from "react-intl";

import type {
  OperationalMetric,
  OperationalMetricTrend,
} from "../application/dashboard-types";
import { messages } from "../messages";
import {
  formatOperationalMetricDisplayValue,
  isDisplayableOperationalMetricValue,
} from "./operational-metric-display";
import "../pages/dashboard-widget.css";

interface UsageData {
  x: string;
  y: number;
}

interface UtilizationMetric {
  id: string;
  total: string;
  trend?: OperationalMetricTrend;
  unit: string;
  value: string;
}

export const UTILIZATION_WARNING_THRESHOLD_PERCENT = 60;
export const UTILIZATION_DANGER_THRESHOLD_PERCENT = 90;

export const UTILIZATION_THRESHOLDS = [
  { value: UTILIZATION_WARNING_THRESHOLD_PERCENT },
  { value: UTILIZATION_DANGER_THRESHOLD_PERCENT },
] as const;

export type UtilizationStatusLevel = "danger" | "ok" | "warning";

export function getUtilizationPercentage(value: string, total: string): number {
  return Math.round((Number(value) / Number(total)) * 100);
}

export function getUtilizationStatusLevel(
  percentage: number,
): UtilizationStatusLevel {
  if (percentage >= UTILIZATION_DANGER_THRESHOLD_PERCENT) {
    return "danger";
  }
  if (percentage >= UTILIZATION_WARNING_THRESHOLD_PERCENT) {
    return "warning";
  }
  return "ok";
}

/** Rendered pixel size; must match the donut wrapper and ChartDonutUtilization height/width. */
const UTILIZATION_CHART_SIZE = 130;

/** Extra left padding keeps hover tooltips inside the clipped widget body. */
const UTILIZATION_CHART_PADDING = {
  bottom: 10,
  left: 32,
  right: 10,
  top: 10,
} as const;

export function isUtilizationMetric(
  metric: OperationalMetric,
): metric is UtilizationMetric {
  return typeof metric.unit === "string" && typeof metric.total === "string";
}

export function UtilizationChart({
  metric,
}: Readonly<{ metric: OperationalMetric }>) {
  const intl = useIntl();

  if (!isUtilizationMetric(metric)) {
    return null;
  }

  const { unit, total, value } = metric;

  if (
    !isDisplayableOperationalMetricValue(value) ||
    !isDisplayableOperationalMetricValue(total)
  ) {
    return (
      <Title headingLevel="h3" size="lg">
        {formatOperationalMetricDisplayValue(value, intl)}
      </Title>
    );
  }

  const percentage = getUtilizationPercentage(value, total);
  const capacityLabel = intl.formatMessage(messages.utilizationCapacity, {
    unit,
  });

  const data: UsageData = { x: capacityLabel, y: percentage };
  const valueLabel = intl.formatMessage(messages.utilizationLabel, {
    unit,
    value,
  });
  const capacitySubtitle = intl.formatMessage(messages.utilizationSubtitle, {
    total,
    unit,
  });

  return (
    <Flex
      alignItems={{ default: "alignItemsCenter" }}
      className="hypershell-dashboard-utilization-chart"
      gap={{ default: "gapSm" }}
    >
      <FlexItem
        className="hypershell-dashboard-utilization-chart__donut"
        style={{
          height: UTILIZATION_CHART_SIZE,
          width: UTILIZATION_CHART_SIZE,
        }}
      >
        <ChartDonutUtilization
          ariaDesc={capacityLabel}
          ariaTitle={intl.formatMessage(messages.utilizationChartTitle, {
            unit,
          })}
          constrainToVisibleArea
          data={data}
          height={UTILIZATION_CHART_SIZE}
          labels={({ datum }: { datum: UsageData }) =>
            datum.x
              ? intl.formatMessage(messages.utilizationDataLabel, {
                  capacity: datum.x,
                  percentage: datum.y,
                })
              : null
          }
          padding={UTILIZATION_CHART_PADDING}
          thresholds={[...UTILIZATION_THRESHOLDS]}
          width={UTILIZATION_CHART_SIZE}
        />
      </FlexItem>
      <FlexItem className="hypershell-dashboard-utilization-chart__label">
        <Title headingLevel="h3" size="lg">
          {valueLabel}
        </Title>
        <small>{capacitySubtitle}</small>
      </FlexItem>
    </Flex>
  );
}
