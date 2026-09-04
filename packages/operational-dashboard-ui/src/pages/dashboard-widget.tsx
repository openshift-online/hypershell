import {
  Button,
  Card,
  CardBody,
  Content,
  DescriptionList,
  DescriptionListDescription,
  DescriptionListGroup,
  DescriptionListTerm,
  Divider,
  Flex,
  FlexItem,
  Icon,
  Stack,
  StackItem,
  Title,
  Tooltip,
} from "@patternfly/react-core";
import {
  CheckCircleIcon,
  ExclamationCircleIcon,
  ExclamationTriangleIcon,
  TrendDownIcon,
  TrendUpIcon,
} from "@patternfly/react-icons";
import type { PropsWithChildren } from "react";
import { FormattedMessage, useIntl } from "react-intl";

import type { OperationalMetric } from "../application/dashboard-types";
import {
  getMetricTrendChange,
  type MetricTrendChange,
} from "../dashboard/metric-trend-change";
import {
  formatOperationalMetricDisplayValue,
  isDisplayableOperationalMetricValue,
} from "../dashboard/operational-metric-display";
import { TrendSparklineChart } from "../dashboard/trend-sparkline-chart";
import { getGatewayExceptionStatusCounts } from "../dashboard/gateway-exception-status-counts";
import { GatewayStatusChart } from "../dashboard/gateway-status-chart";
import { NodeStatusChart } from "../dashboard/node-status-chart";
import { PodCapacityChart } from "../dashboard/pod-capacity-chart";
import { ProvisionTimeChart } from "../dashboard/provision-time-chart";
import { isPodCapacityMetric } from "../dashboard/pod-capacity-metric";
import {
  getUtilizationPercentage,
  getUtilizationStatusLevel,
  isUtilizationMetric,
  UtilizationChart,
} from "../dashboard/utilization-chart";
import { messages } from "../messages";

function WidgetContent({
  bodyClassName,
  children,
}: Readonly<PropsWithChildren<{ bodyClassName?: string }>>) {
  return (
    <Card isPlain isFullHeight>
      <CardBody className={bodyClassName}>{children}</CardBody>
    </Card>
  );
}

export function MetricCard({
  metric,
  showTrend = true,
  subtitle,
  title,
}: Readonly<{
  metric: OperationalMetric;
  showTrend?: boolean;
  subtitle: string;
  title: string;
}>) {
  const intl = useIntl();
  const displayValue = formatOperationalMetricDisplayValue(metric.value, intl);
  const metricHeading = isDisplayableOperationalMetricValue(metric.value)
    ? intl.formatMessage(messages.metricValue, {
        label: title,
        value: displayValue,
      })
    : displayValue;

  return (
    <WidgetContent>
      <Content className="hypershell-dashboard-metric-card">
        <Stack hasGutter>
          <StackItem>
            <Flex justifyContent={{ default: "justifyContentCenter" }}>
              <FlexItem>
                <Title headingLevel="h3" size="lg">
                  {metricHeading}
                </Title>
                {subtitle ? <small>{subtitle}</small> : null}
              </FlexItem>
            </Flex>
          </StackItem>
          {showTrend && metric.trend ? (
            <StackItem>
              <TrendSparklineChart trend={metric.trend} title={title} />
            </StackItem>
          ) : null}
        </Stack>
      </Content>
    </WidgetContent>
  );
}

export function GatewayStatusCard({
  metric,
}: Readonly<{ metric: OperationalMetric }>) {
  const intl = useIntl();
  const trendTitle = intl.formatMessage(messages.provisionedGateways);

  return (
    <WidgetContent>
      <Content className="hypershell-dashboard-status-donut-card">
        <Stack hasGutter>
          <StackItem>
            <GatewayStatusChart metric={metric} />
          </StackItem>
          {metric.trend ? (
            <StackItem>
              <TrendSparklineChart trend={metric.trend} title={trendTitle} />
            </StackItem>
          ) : null}
        </Stack>
      </Content>
    </WidgetContent>
  );
}

export function NodeStatusCard({
  metric,
}: Readonly<{ metric: OperationalMetric }>) {
  return (
    <WidgetContent bodyClassName="hypershell-dashboard-status-donut-card--compact">
      <Content className="hypershell-dashboard-status-donut-card">
        <NodeStatusChart metric={metric} />
      </Content>
    </WidgetContent>
  );
}

export function PodCapacityCard({
  metric,
}: Readonly<{ metric: OperationalMetric }>) {
  return (
    <WidgetContent bodyClassName="hypershell-dashboard-status-donut-card--compact">
      <Content className="hypershell-dashboard-status-donut-card">
        <PodCapacityChart metric={metric} />
      </Content>
    </WidgetContent>
  );
}

export function ProvisionTimeCard({
  metric,
}: Readonly<{ metric: OperationalMetric }>) {
  return (
    <WidgetContent>
      <Content className="hypershell-dashboard-provision-time-card">
        <ProvisionTimeChart metric={metric} />
      </Content>
    </WidgetContent>
  );
}

export function UtilizationCard({
  metric,
}: Readonly<{ metric: OperationalMetric }>) {
  return (
    <WidgetContent>
      <Content>
        <Stack hasGutter>
          {isUtilizationMetric(metric) ? (
            <StackItem>
              <UtilizationChart metric={metric} />
            </StackItem>
          ) : null}
        </Stack>
      </Content>
    </WidgetContent>
  );
}

function SummaryTrendIndicator({
  trendChange,
}: Readonly<{ trendChange: MetricTrendChange }>) {
  const intl = useIntl();
  const isIncrease = trendChange.direction === "increase";
  const tooltipContent = intl.formatMessage(
    isIncrease ? messages.summaryTrendIncrease : messages.summaryTrendDecrease,
    { percent: trendChange.percent },
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

function UtilizationStatusIcon({
  percentage,
  total,
  unit,
  value,
}: Readonly<{
  percentage: number;
  total: string;
  unit: string;
  value: string;
}>) {
  const intl = useIntl();
  const statusLevel = getUtilizationStatusLevel(percentage);
  const tooltipContent = intl.formatMessage(
    messages.utilizationSummaryTooltip,
    {
      percent: percentage,
      separator: "  | ",
      total,
      unit,
      value,
    },
  );

  const statusIcon = (() => {
    switch (statusLevel) {
      case "ok":
        return (
          <Icon isInline status="success">
            <CheckCircleIcon aria-hidden />
          </Icon>
        );
      case "warning":
        return (
          <Icon isInline status="warning">
            <ExclamationTriangleIcon aria-hidden />
          </Icon>
        );
      case "danger":
        return (
          <Icon isInline status="danger">
            <ExclamationCircleIcon aria-hidden />
          </Icon>
        );
    }
  })();

  return (
    <Tooltip content={tooltipContent} aria="labelledby">
      <Button
        aria-label={tooltipContent}
        className="hypershell-dashboard-summary-utilization-status"
        isInline
        variant="plain"
      >
        {statusIcon}
      </Button>
    </Tooltip>
  );
}

function SummaryProvisionDurationValue({
  metric,
  valueKey,
}: Readonly<{
  metric: OperationalMetric | undefined;
  valueKey: "mean" | "p50" | "p95";
}>) {
  const intl = useIntl();

  if (!metric) {
    return null;
  }

  const durationValue =
    valueKey === "mean"
      ? (metric.provisionDuration?.mean ?? metric.value)
      : metric.provisionDuration?.[valueKey];

  if (
    durationValue === undefined ||
    !isDisplayableOperationalMetricValue(durationValue)
  ) {
    return null;
  }

  return (
    <>
      {metric.unit
        ? intl.formatMessage(messages.utilizationLabel, {
            unit: metric.unit,
            value: durationValue,
          })
        : durationValue}
    </>
  );
}

function SummaryUtilizationValue({
  metric,
}: Readonly<{ metric: OperationalMetric | undefined }>) {
  const intl = useIntl();

  if (!metric) {
    return null;
  }

  if (
    !isDisplayableOperationalMetricValue(metric.value) ||
    (metric.total !== undefined &&
      !isDisplayableOperationalMetricValue(metric.total))
  ) {
    return <>{formatOperationalMetricDisplayValue(metric.value, intl)}</>;
  }

  if (!isUtilizationMetric(metric)) {
    return (
      <>
        {metric.unit
          ? intl.formatMessage(messages.utilizationLabel, {
              unit: metric.unit,
              value: metric.value,
            })
          : metric.value}
      </>
    );
  }

  const percentage = getUtilizationPercentage(metric.value, metric.total);

  return (
    <Flex
      alignItems={{ default: "alignItemsCenter" }}
      spaceItems={{ default: "spaceItemsSm" }}
    >
      <FlexItem>
        {intl.formatMessage(messages.utilizationLabel, {
          unit: metric.unit,
          value: metric.value,
        })}
      </FlexItem>
      <FlexItem>
        <UtilizationStatusIcon
          percentage={percentage}
          total={metric.total}
          unit={metric.unit}
          value={metric.value}
        />
      </FlexItem>
    </Flex>
  );
}

function SummaryMetricValue({
  metric,
}: Readonly<{ metric: OperationalMetric | undefined }>) {
  const intl = useIntl();
  const trendChange = metric ? getMetricTrendChange(metric) : undefined;
  const displayValue = metric
    ? formatOperationalMetricDisplayValue(metric.value, intl)
    : undefined;

  return (
    <Flex
      alignItems={{ default: "alignItemsCenter" }}
      spaceItems={{ default: "spaceItemsSm" }}
    >
      <FlexItem>{displayValue}</FlexItem>
      {trendChange ? (
        <FlexItem>
          <SummaryTrendIndicator trendChange={trendChange} />
        </FlexItem>
      ) : null}
    </Flex>
  );
}

function SummaryGatewayStatusCount({
  count,
  statusLabel,
  variant,
}: Readonly<{
  count: number;
  statusLabel: string;
  variant: "danger" | "warning";
}>) {
  const intl = useIntl();
  const accessibleLabel = intl.formatMessage(messages.gatewayStatusLegend, {
    count,
    status: statusLabel,
  });
  const statusIcon =
    variant === "danger" ? (
      <Icon isInline status="danger">
        <ExclamationCircleIcon aria-hidden />
      </Icon>
    ) : (
      <Icon isInline status="warning">
        <ExclamationTriangleIcon aria-hidden />
      </Icon>
    );

  return (
    <Flex
      alignItems={{ default: "alignItemsCenter" }}
      direction={{ default: "row" }}
      flexWrap={{ default: "nowrap" }}
      spaceItems={{ default: "spaceItemsXs" }}
    >
      <FlexItem>
        <Tooltip content={statusLabel} aria="labelledby">
          <Button
            aria-label={statusLabel}
            className="hypershell-dashboard-summary-gateway-status__icon"
            isInline
            variant="plain"
          >
            {statusIcon}
          </Button>
        </Tooltip>
      </FlexItem>
      <FlexItem>
        <span aria-label={accessibleLabel}>{count}</span>
      </FlexItem>
    </Flex>
  );
}

function SummaryGatewayStatusCounts({
  metric,
}: Readonly<{ metric: OperationalMetric }>) {
  const intl = useIntl();

  if (metric.status === undefined) {
    return null;
  }

  const { failed: failedCount, degraded: degradedCount } =
    getGatewayExceptionStatusCounts(metric.status);

  if (failedCount === 0 && degradedCount === 0) {
    return null;
  }

  const failedLabel = intl.formatMessage(messages.gatewayStatusFailed);
  const degradedLabel = intl.formatMessage(messages.gatewayStatusDegraded);

  return (
    <Flex
      alignItems={{ default: "alignItemsCenter" }}
      className="hypershell-dashboard-summary-gateway-status"
      direction={{ default: "row" }}
      flexWrap={{ default: "nowrap" }}
      spaceItems={{ default: "spaceItemsSm" }}
    >
      {failedCount > 0 ? (
        <FlexItem>
          <SummaryGatewayStatusCount
            count={failedCount}
            statusLabel={failedLabel}
            variant="danger"
          />
        </FlexItem>
      ) : null}
      {failedCount > 0 && degradedCount > 0 ? (
        <FlexItem>
          <Divider
            className="hypershell-dashboard-summary-gateway-status__divider"
            orientation={{ default: "vertical" }}
          />
        </FlexItem>
      ) : null}
      {degradedCount > 0 ? (
        <FlexItem>
          <SummaryGatewayStatusCount
            count={degradedCount}
            statusLabel={degradedLabel}
            variant="warning"
          />
        </FlexItem>
      ) : null}
    </Flex>
  );
}

function SummaryGatewayValue({
  metric,
}: Readonly<{ metric: OperationalMetric | undefined }>) {
  return (
    <Stack hasGutter className="hypershell-dashboard-summary-gateway-value">
      <StackItem>
        <SummaryMetricValue metric={metric} />
      </StackItem>
      {metric ? (
        <StackItem>
          <SummaryGatewayStatusCounts metric={metric} />
        </StackItem>
      ) : null}
    </Stack>
  );
}

function SummaryPodFailedCount({
  metric,
}: Readonly<{ metric: OperationalMetric }>) {
  const intl = useIntl();

  if (!isPodCapacityMetric(metric)) {
    return null;
  }

  const failedCount = metric.podPhases.failed;

  if (failedCount === 0) {
    return null;
  }

  return (
    <SummaryGatewayStatusCount
      count={failedCount}
      statusLabel={intl.formatMessage(messages.podStatusFailed)}
      variant="danger"
    />
  );
}

function SummaryPodsValue({
  metric,
}: Readonly<{ metric: OperationalMetric | undefined }>) {
  return (
    <Stack hasGutter className="hypershell-dashboard-summary-gateway-value">
      <StackItem>
        <SummaryUtilizationValue metric={metric} />
      </StackItem>
      {metric ? (
        <StackItem>
          <SummaryPodFailedCount metric={metric} />
        </StackItem>
      ) : null}
    </Stack>
  );
}

const USAGE_SUMMARY_METRIC_IDS = [
  "registered-users",
  "provisioned-gateways",
  "provisioned-sandboxes",
] as const;

const USAGE_SUMMARY_LABELS = {
  "registered-users": messages.registeredUsersSummary,
  "provisioned-gateways": messages.gateways,
  "provisioned-sandboxes": messages.widgetSandboxes,
} as const;

export function UsageSummaryCard({
  metrics,
}: Readonly<{ metrics: readonly OperationalMetric[] }>) {
  const intl = useIntl();

  return (
    <WidgetContent>
      <DescriptionList
        isHorizontal
        aria-label={intl.formatMessage(messages.summaryUsageAriaLabel)}
      >
        {USAGE_SUMMARY_METRIC_IDS.map((metricId) => (
          <DescriptionListGroup key={metricId}>
            <DescriptionListTerm>
              <FormattedMessage {...USAGE_SUMMARY_LABELS[metricId]} />
            </DescriptionListTerm>
            <DescriptionListDescription>
              {metricId === "provisioned-gateways" ? (
                <SummaryGatewayValue
                  metric={metrics.find((metric) => metric.id === metricId)}
                />
              ) : (
                <SummaryMetricValue
                  metric={metrics.find((metric) => metric.id === metricId)}
                />
              )}
            </DescriptionListDescription>
          </DescriptionListGroup>
        ))}
      </DescriptionList>
    </WidgetContent>
  );
}

export function SystemSummaryCard({
  metrics,
}: Readonly<{ metrics: readonly OperationalMetric[] }>) {
  const intl = useIntl();

  return (
    <WidgetContent>
      <DescriptionList
        isHorizontal
        aria-label={intl.formatMessage(messages.summarySystemAriaLabel)}
      >
        <DescriptionListGroup>
          <DescriptionListTerm>
            <FormattedMessage {...messages.memory} />
          </DescriptionListTerm>
          <DescriptionListDescription>
            <SummaryUtilizationValue
              metric={metrics.find((metric) => metric.id === "memory")}
            />
          </DescriptionListDescription>
        </DescriptionListGroup>
        <DescriptionListGroup>
          <DescriptionListTerm>
            <FormattedMessage {...messages.cpus} />
          </DescriptionListTerm>
          <DescriptionListDescription>
            <SummaryUtilizationValue
              metric={metrics.find((metric) => metric.id === "cpu")}
            />
          </DescriptionListDescription>
        </DescriptionListGroup>
        <DescriptionListGroup>
          <DescriptionListTerm>
            <FormattedMessage {...messages.pods} />
          </DescriptionListTerm>
          <DescriptionListDescription>
            <SummaryPodsValue
              metric={metrics.find((metric) => metric.id === "pods")}
            />
          </DescriptionListDescription>
        </DescriptionListGroup>
        <DescriptionListGroup>
          <DescriptionListTerm>
            <FormattedMessage {...messages.nodes} />
          </DescriptionListTerm>
          <DescriptionListDescription>
            <SummaryGatewayValue
              metric={metrics.find((metric) => metric.id === "nodes")}
            />
          </DescriptionListDescription>
        </DescriptionListGroup>
        <DescriptionListGroup>
          <DescriptionListTerm>
            <FormattedMessage {...messages.provisionTime} />
          </DescriptionListTerm>
          <DescriptionListDescription>
            <SummaryProvisionDurationValue
              metric={metrics.find((metric) => metric.id === "provision-time")}
              valueKey="mean"
            />
          </DescriptionListDescription>
        </DescriptionListGroup>
      </DescriptionList>
    </WidgetContent>
  );
}
