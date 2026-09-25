import {
  Bullseye,
  Button,
  Content,
  DescriptionList,
  DescriptionListDescription,
  DescriptionListGroup,
  DescriptionListTerm,
  EmptyState,
  EmptyStateBody,
  EmptyStateVariant,
  Flex,
  FlexItem,
  Stack,
  StackItem,
  Title,
  Tooltip,
  Card,
  CardBody,
  Divider,
} from "@patternfly/react-core";
import { TrendDownIcon, TrendUpIcon } from "@patternfly/react-icons";
import type { PropsWithChildren, ReactNode } from "react";
import { FormattedMessage, useIntl, type IntlShape } from "react-intl";

import type { OperationalMetric } from "../application/dashboard-types";
import {
  getMetricTrendChange,
  type MetricTrendChange,
} from "./metric-trend-change";
import {
  formatOperationalMetricDisplayValue,
  isDisplayableOperationalMetricValue,
} from "./operational-metric-display";
import { TrendSparklineChart } from "./trend-sparkline-chart";
import { messages } from "../messages";
import "../pages/dashboard-widget.css";

function WidgetContent({
  children,
  helpText,
}: Readonly<PropsWithChildren<{ helpText?: ReactNode }>>) {
  return (
    <Card isPlain isFullHeight>
      <CardBody className="hypershell-dashboard-widget-card">
        {children}
        {helpText ? (
          <>
            <Divider />
            <p className="hypershell-dashboard-widget-help-text">{helpText}</p>
          </>
        ) : null}
      </CardBody>
    </Card>
  );
}

function MetricUnavailableEmptyState() {
  return (
    <Bullseye>
      <EmptyState headingLevel="h3" variant={EmptyStateVariant.sm}>
        <Title headingLevel="h3">
          <FormattedMessage {...messages.metricUnavailableTitle} />
        </Title>
        <EmptyStateBody>
          <FormattedMessage {...messages.metricUnavailableBody} />
        </EmptyStateBody>
      </EmptyState>
    </Bullseye>
  );
}

function formatMetricWithUnit(
  metric: OperationalMetric,
  intl: ReturnType<typeof useIntl>,
): string {
  const displayValue = formatOperationalMetricDisplayValue(metric.value, intl);
  if (
    !isDisplayableOperationalMetricValue(metric.value) ||
    metric.unit === undefined ||
    metric.unit === ""
  ) {
    return displayValue;
  }

  return intl.formatMessage(messages.utilizationLabel, {
    unit: metric.unit,
    value: displayValue,
  });
}

const RELIABILITY_SUMMARY_METRIC_IDS = [
  "api-request-rate",
  "api-error-rate",
  "api-latency",
] as const;

type ReliabilitySummaryMetricId =
  (typeof RELIABILITY_SUMMARY_METRIC_IDS)[number];

const RELIABILITY_SUMMARY_LABELS = {
  "api-request-rate": messages.reliabilitySummaryRequestRate,
  "api-error-rate": messages.reliabilitySummaryErrorRate,
  "api-latency": messages.reliabilitySummaryLatency,
} as const;

function reliabilitySummaryTrendSubject(
  metricId: ReliabilitySummaryMetricId,
  intl: IntlShape,
): string {
  return intl.formatMessage(RELIABILITY_SUMMARY_LABELS[metricId]);
}

function ReliabilitySummaryTrendIndicator({
  metricId,
  trendChange,
}: Readonly<{
  metricId: ReliabilitySummaryMetricId;
  trendChange: MetricTrendChange;
}>) {
  const intl = useIntl();
  const isIncrease = trendChange.direction === "increase";
  const tooltipContent = intl.formatMessage(
    isIncrease ? messages.summaryTrendIncrease : messages.summaryTrendDecrease,
    {
      percent: trendChange.percent,
      subject: reliabilitySummaryTrendSubject(metricId, intl),
    },
  );

  return (
    <Tooltip content={tooltipContent} aria="labelledby">
      <Button
        aria-label={tooltipContent}
        className={
          isIncrease
            ? "hypershell-dashboard-summary-trend hypershell-dashboard-summary-trend--increase"
            : "hypershell-dashboard-summary-trend hypershell-dashboard-summary-trend--decrease"
        }
        isInline
        variant="plain"
      >
        {isIncrease ? <TrendUpIcon /> : <TrendDownIcon />}
      </Button>
    </Tooltip>
  );
}

function ReliabilitySummaryValue({
  metric,
  metricId,
}: Readonly<{
  metric: OperationalMetric;
  metricId: ReliabilitySummaryMetricId;
}>) {
  const intl = useIntl();
  const trendChange = getMetricTrendChange(metric);

  return (
    <Flex
      alignItems={{ default: "alignItemsCenter" }}
      spaceItems={{ default: "spaceItemsSm" }}
    >
      <FlexItem>{formatMetricWithUnit(metric, intl)}</FlexItem>
      {trendChange ? (
        <FlexItem>
          <ReliabilitySummaryTrendIndicator
            metricId={metricId}
            trendChange={trendChange}
          />
        </FlexItem>
      ) : null}
    </Flex>
  );
}

export function ReliabilitySummaryCard({
  metrics,
}: Readonly<{ metrics: readonly OperationalMetric[] }>) {
  const intl = useIntl();

  return (
    <WidgetContent>
      <DescriptionList
        isHorizontal
        aria-label={intl.formatMessage(messages.reliabilitySummaryAriaLabel)}
      >
        {RELIABILITY_SUMMARY_METRIC_IDS.map((metricId) => {
          const metric = metrics.find((entry) => entry.id === metricId);

          return (
            <DescriptionListGroup key={metricId}>
              <DescriptionListTerm>
                <FormattedMessage {...RELIABILITY_SUMMARY_LABELS[metricId]} />
              </DescriptionListTerm>
              <DescriptionListDescription>
                {metric ? (
                  <ReliabilitySummaryValue
                    metric={metric}
                    metricId={metricId}
                  />
                ) : (
                  <span className="hypershell-dashboard-summary-unavailable">
                    <FormattedMessage {...messages.metricUnavailableTitle} />
                  </span>
                )}
              </DescriptionListDescription>
            </DescriptionListGroup>
          );
        })}
      </DescriptionList>
    </WidgetContent>
  );
}

export function ApiReliabilityTrendCard({
  metric,
  title,
}: Readonly<{
  metric: OperationalMetric | undefined;
  title: string;
}>) {
  const intl = useIntl();

  if (!metric) {
    return (
      <WidgetContent>
        <MetricUnavailableEmptyState />
      </WidgetContent>
    );
  }

  const displayValue = formatMetricWithUnit(metric, intl);
  const heading = isDisplayableOperationalMetricValue(metric.value)
    ? intl.formatMessage(messages.metricValue, {
        label: title,
        value: displayValue,
      })
    : displayValue;
  const helpMessage = {
    "api-request-rate": messages.dashboardHelpApiRequestRate,
    "api-error-rate": messages.dashboardHelpApiErrorRate,
    "api-latency": messages.dashboardHelpApiLatency,
    "reconciliation-failures": messages.dashboardHelpReconciliationFailures,
    "reconciliation-retries": messages.dashboardHelpReconciliationRetries,
    "reconciliation-lag": messages.dashboardHelpReconciliationLag,
  }[metric.id];

  return (
    <WidgetContent
      helpText={helpMessage ? <FormattedMessage {...helpMessage} /> : undefined}
    >
      <Content className="hypershell-dashboard-metric-card">
        <Stack hasGutter>
          <StackItem>
            <Flex justifyContent={{ default: "justifyContentCenter" }}>
              <FlexItem>
                <Title headingLevel="h3" size="lg">
                  {heading}
                </Title>
              </FlexItem>
            </Flex>
          </StackItem>
          {metric.hourlyTrend && metric.hourlyTrend.points.length >= 2 ? (
            <StackItem>
              <TrendSparklineChart
                caption={intl.formatMessage(messages.trendLast24Hours)}
                title={title}
                trend={metric.hourlyTrend}
              />
            </StackItem>
          ) : null}
        </Stack>
      </Content>
    </WidgetContent>
  );
}
