/* eslint-disable formatjs/no-literal-string-in-jsx */
import {
  Content,
  DescriptionList,
  DescriptionListDescription,
  DescriptionListGroup,
  DescriptionListTerm,
  Stack,
  StackItem,
} from "@patternfly/react-core";
import { useMemo } from "react";
import { FormattedMessage, useIntl } from "react-intl";

import type { OperationalMetric } from "../application/dashboard-types";
import { messages } from "../messages";
import { TrendSparklineChart } from "./trend-sparkline-chart";
import {
  formatProvisionDurationValue,
  parseProvisionDurationStats,
} from "./provision-time-data";

const STAT_ROWS = [
  { key: "mean", label: messages.provisionTimeAverage },
  { key: "p50", label: messages.provisionTimeMedian },
  { key: "p95", label: messages.provisionTimeP95Label },
] as const;

export function ProvisionTimeChart({
  metric,
}: Readonly<{ metric: OperationalMetric }>) {
  const intl = useIntl();
  const stats = useMemo(() => parseProvisionDurationStats(metric), [metric]);

  if (!stats) {
    return null;
  }

  const statValues = {
    mean: stats.meanSeconds,
    p50: stats.p50Seconds,
    p95: stats.p95Seconds,
  } as const;
  const optionalRows = [
    ...(stats.p99Seconds === undefined
      ? []
      : [
          {
            key: "p99",
            label: messages.provisionTimeP99Label,
            value: stats.p99Seconds,
          },
        ]),
    ...(stats.targetSeconds === undefined
      ? []
      : [
          {
            key: "target",
            label: messages.provisionTimeTargetLabel,
            value: stats.targetSeconds,
          },
        ]),
  ] as const;
  const p99ExceedsTarget =
    stats.p99Seconds !== undefined &&
    stats.targetSeconds !== undefined &&
    stats.p99Seconds > stats.targetSeconds;

  return (
    <Stack hasGutter className="hypershell-dashboard-provision-time">
      <StackItem>
        <DescriptionList
          isHorizontal
          aria-label={intl.formatMessage(messages.provisionTimeStatsAriaLabel)}
        >
          {STAT_ROWS.map((row) => (
            <DescriptionListGroup key={row.key} className="pf-v6-u-pb-sm">
              <DescriptionListTerm>
                <FormattedMessage {...row.label} />
              </DescriptionListTerm>
              <DescriptionListDescription>
                {formatProvisionDurationValue(
                  intl,
                  statValues[row.key],
                  metric.unit,
                )}
              </DescriptionListDescription>
            </DescriptionListGroup>
          ))}
          {optionalRows.map((row) => (
            <DescriptionListGroup key={row.key} className="pf-v6-u-pb-sm">
              <DescriptionListTerm>
                <FormattedMessage {...row.label} />
              </DescriptionListTerm>
              <DescriptionListDescription>
                {formatProvisionDurationValue(intl, row.value, metric.unit)}
              </DescriptionListDescription>
            </DescriptionListGroup>
          ))}
        </DescriptionList>
      </StackItem>
      {p99ExceedsTarget ? (
        <StackItem>
          <Content component="p" className="pf-m-warning-color-100">
            <FormattedMessage {...messages.provisionTimeP99Warning} />
          </Content>
        </StackItem>
      ) : null}
      {metric.provisionDuration?.p50Trend &&
      metric.provisionDuration.p95Trend ? (
        <StackItem>
          <TrendSparklineChart
            caption="Selected range - daily duration"
            trend={metric.provisionDuration.p50Trend}
            title="P50"
          />
          <TrendSparklineChart
            caption=""
            trend={metric.provisionDuration.p95Trend}
            title="P95"
          />
        </StackItem>
      ) : null}
      {stats.p99Seconds !== undefined && metric.provisionDuration?.p99Trend ? (
        <StackItem>
          <TrendSparklineChart
            caption="Selected range - daily P99"
            trend={metric.provisionDuration.p99Trend}
            title="P99"
          />
        </StackItem>
      ) : null}
      <StackItem>
        <Content
          component="p"
          className="hypershell-dashboard-provision-time__note"
        >
          <FormattedMessage
            {...messages.provisionTimeP95Note}
            values={{
              duration: stats.p95Seconds.toFixed(2),
              unit: metric.unit ?? "",
            }}
          />
        </Content>
      </StackItem>
    </Stack>
  );
}
