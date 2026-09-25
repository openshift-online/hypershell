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
} from "@patternfly/react-core";
import { TrendDownIcon, TrendUpIcon } from "@patternfly/react-icons";
import type { PropsWithChildren } from "react";
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

function WidgetContent({ children }: Readonly<PropsWithChildren>) {
  return (
    <Card isPlain isFullHeight>
      <CardBody className="hypershell-dashboard-widget-card">
        {children}
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
  const unit =
    metric.id === "reconciliation-failures"
      ? "failures"
      : metric.id === "reconciliation-retries"
        ? "retries"
        : metric.unit;
  if (
    !isDisplayableOperationalMetricValue(metric.value) ||
    unit === undefined ||
    unit === ""
  ) {
    return displayValue;
  }

  return intl.formatMessage(messages.utilizationLabel, {
    unit,
    value: displayValue,
  });
}

const RELIABILITY_SUMMARY_METRIC_IDS = [
  "api-request-rate",
  "api-error-rate",
  "api-latency",
  "reconciliation-failures",
  "reconciliation-retries",
  "reconciliation-lag",
  "stale-resource-status-count",
] as const;

type ReliabilitySummaryMetricId =
  (typeof RELIABILITY_SUMMARY_METRIC_IDS)[number];

const RELIABILITY_SUMMARY_LABELS = {
  "api-request-rate": messages.reliabilitySummaryRequestRate,
  "api-error-rate": messages.reliabilitySummaryErrorRate,
  "api-latency": messages.reliabilitySummaryLatency,
  "reconciliation-failures": messages.reliabilitySummaryReconciliationFailures,
  "reconciliation-retries": messages.reliabilitySummaryReconciliationRetries,
  "reconciliation-lag": messages.reliabilitySummaryReconciliationLag,
  "stale-resource-status-count": messages.reliabilitySummaryStaleResourceStatus,
} as const;

const API_RELIABILITY_SUMMARY_METRIC_IDS = RELIABILITY_SUMMARY_METRIC_IDS.slice(
  0,
  3,
);
const RECONCILIATION_SUMMARY_METRIC_IDS =
  RELIABILITY_SUMMARY_METRIC_IDS.slice(3);

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

  const renderSummaryColumn = (
    metricIds: readonly ReliabilitySummaryMetricId[],
  ) => (
    <DescriptionList isHorizontal>
      {metricIds.map((metricId) => {
        const metric = metrics.find((entry) => entry.id === metricId);

        return (
          <DescriptionListGroup key={metricId}>
            <DescriptionListTerm className="hypershell-dashboard-reliability-summary__term">
              <FormattedMessage {...RELIABILITY_SUMMARY_LABELS[metricId]} />
            </DescriptionListTerm>
            <DescriptionListDescription>
              {metric ? (
                <ReliabilitySummaryValue metric={metric} metricId={metricId} />
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
  );

  return (
    <WidgetContent>
      <div
        className="hypershell-dashboard-reliability-summary"
        role="group"
        aria-label={intl.formatMessage(messages.reliabilitySummaryAriaLabel)}
      >
        <div className="hypershell-dashboard-reliability-summary__column">
          {renderSummaryColumn(API_RELIABILITY_SUMMARY_METRIC_IDS)}
        </div>
        <div className="hypershell-dashboard-reliability-summary__column">
          {renderSummaryColumn(RECONCILIATION_SUMMARY_METRIC_IDS)}
        </div>
      </div>
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

  const isCount =
    metric.id === "reconciliation-failures" ||
    metric.id === "reconciliation-retries";
  const displayValue = formatMetricWithUnit(metric, intl);

  return (
    <WidgetContent>
      <Content className="hypershell-dashboard-metric-card">
        <Stack hasGutter>
          <StackItem>
            <Flex justifyContent={{ default: "justifyContentCenter" }}>
              <FlexItem>
                <Title headingLevel="h3" size="lg">
                  <>
                    {displayValue}
                    <br />
                    <small>
                      {intl.formatMessage(
                        isCount
                          ? messages.reconciliationFailuresLast24Hours
                          : messages.apiReliabilityLast5Minutes,
                      )}
                    </small>
                  </>
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
                valueFormatter={(value) => value.toFixed(3)}
              />
            </StackItem>
          ) : null}
        </Stack>
      </Content>
    </WidgetContent>
  );
}
