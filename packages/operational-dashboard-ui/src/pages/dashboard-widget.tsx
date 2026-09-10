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
} from "@patternfly/react-icons";
import type { PropsWithChildren } from "react";
import { FormattedMessage, useIntl } from "react-intl";

import type { OperationalMetric } from "../application/dashboard-types";
import {
  getMetricTrendChange,
  getTrendChange,
} from "../dashboard/metric-trend-change";
import {
  MetricTrendIndicator,
  type MetricTrendTooltipMessages,
} from "../dashboard/metric-trend-indicator";
import {
  formatOperationalMetricDisplayValue,
  isDisplayableOperationalMetricValue,
} from "../dashboard/operational-metric-display";
import {
  TrendSparklineChart,
  USERS_SPARKLINE_PLOT_HEIGHT,
} from "../dashboard/trend-sparkline-chart";
import { getGatewayExceptionStatusCounts } from "../dashboard/gateway-exception-status-counts";
import { GatewayStatusChart } from "../dashboard/gateway-status-chart";
import { InventoryStatusChart } from "../dashboard/inventory-status-chart";
import { ManagedClusterProvidersChart } from "../dashboard/managed-cluster-providers-chart";
import { ManagedClusterRegionsChart } from "../dashboard/managed-cluster-regions-chart";
import { NodeStatusChart } from "../dashboard/node-status-chart";
import { PodCapacityChart } from "../dashboard/pod-capacity-chart";
import { DashboardStatPanel } from "../dashboard/dashboard-stat-panel";
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

function RegisteredUsersStatValue({
  metric,
  value,
}: Readonly<{
  metric?: OperationalMetric;
  value?: string;
}>) {
  if (value === undefined) {
    return <SummaryUnavailableValue />;
  }

  return (
    <SummaryMetricValue
      metric={{
        id: metric?.id ?? "registered-users-stat",
        value,
      }}
    />
  );
}

export function RegisteredUsersCard({
  metric,
}: Readonly<{ metric: OperationalMetric }>) {
  const intl = useIntl();
  const displayValue = formatOperationalMetricDisplayValue(metric.value, intl);
  const metricHeading = isDisplayableOperationalMetricValue(metric.value)
    ? intl.formatMessage(messages.metricValue, {
        label: intl.formatMessage(messages.registeredUsersHeading),
        value: displayValue,
      })
    : displayValue;
  const activeTrendTitle = intl.formatMessage(messages.activeUsersDaily);
  const activeTrendTooltipLabel = intl.formatMessage(
    messages.activeUsersDailyTooltip,
  );

  return (
    <WidgetContent>
      <Content className="hypershell-dashboard-metric-card">
        <DashboardStatPanel
          ariaLabel={intl.formatMessage(messages.registeredUsersStatsAriaLabel)}
          columns="two"
          heading={
            <Title headingLevel="h3" size="lg">
              {metricHeading}
            </Title>
          }
          rows={[
            {
              id: "added-7-days",
              label: (
                <FormattedMessage {...messages.registeredUsersLast7Days} />
              ),
              value: (
                <RegisteredUsersStatValue
                  metric={metric}
                  value={metric.createdLast7Days}
                />
              ),
            },
            {
              id: "logins-7-days",
              label: <FormattedMessage {...messages.activeUsersLast7Days} />,
              value: (
                <RegisteredUsersStatValue
                  metric={metric}
                  value={metric.activeLast7Days}
                />
              ),
            },
            {
              id: "added-30-days",
              label: (
                <FormattedMessage {...messages.registeredUsersLast30Days} />
              ),
              value: (
                <RegisteredUsersStatValue
                  metric={metric}
                  value={metric.createdLast30Days}
                />
              ),
            },
            {
              id: "logins-30-days",
              label: <FormattedMessage {...messages.activeUsersLast30Days} />,
              value: (
                <RegisteredUsersStatValue
                  metric={metric}
                  value={metric.activeLast30Days}
                />
              ),
            },
          ]}
          sparkline={
            metric.activeTrend
              ? {
                  plotHeight: USERS_SPARKLINE_PLOT_HEIGHT,
                  title: activeTrendTitle,
                  tooltipLabel: activeTrendTooltipLabel,
                  trend: metric.activeTrend,
                }
              : undefined
          }
        />
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
    <WidgetContent bodyClassName="hypershell-dashboard-status-donut-card--compact">
      <Content className="hypershell-dashboard-status-donut-card hypershell-dashboard-gateway-status-card">
        <GatewayStatusChart metric={metric} />
        {metric.trend ? (
          <TrendSparklineChart trend={metric.trend} title={trendTitle} />
        ) : null}
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

export function ManagedClusterProvidersCard({
  metric,
}: Readonly<{ metric: OperationalMetric }>) {
  return (
    <WidgetContent bodyClassName="hypershell-dashboard-status-donut-card--compact">
      <Content className="hypershell-dashboard-status-donut-card">
        <ManagedClusterProvidersChart metric={metric} />
      </Content>
    </WidgetContent>
  );
}

export function ManagedClusterRegionsCard({
  metric,
}: Readonly<{ metric: OperationalMetric }>) {
  return (
    <WidgetContent bodyClassName="hypershell-dashboard-status-donut-card--compact">
      <Content className="hypershell-dashboard-status-donut-card">
        <ManagedClusterRegionsChart metric={metric} />
      </Content>
    </WidgetContent>
  );
}

export function ManagedDatabaseStatusCard({
  metric,
}: Readonly<{ metric: OperationalMetric }>) {
  return (
    <WidgetContent bodyClassName="hypershell-dashboard-status-donut-card--compact">
      <Content className="hypershell-dashboard-status-donut-card">
        <InventoryStatusChart
          ariaDescMessage={messages.managedDatabaseStatusAriaDesc}
          ariaTitleMessage={messages.managedDatabaseStatusChartTitle}
          metric={metric}
        />
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
    <WidgetContent bodyClassName="hypershell-dashboard-status-donut-card--compact">
      <Content className="hypershell-dashboard-status-donut-card">
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
    return <SummaryUnavailableValue />;
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

function SummaryUnavailableValue() {
  return (
    <span className="hypershell-dashboard-summary-unavailable">
      <FormattedMessage {...messages.metricUnavailableTitle} />
    </span>
  );
}

const USERS_SUMMARY_TREND_TOOLTIP_MESSAGES: MetricTrendTooltipMessages = {
  decrease: messages.summaryUsersTrendDecrease,
  increase: messages.summaryUsersTrendIncrease,
};

function SummaryMetricValue({
  metric,
  trendChange,
  trendTooltipMessages,
}: Readonly<{
  metric: OperationalMetric | undefined;
  trendChange?: ReturnType<typeof getMetricTrendChange>;
  trendTooltipMessages?: MetricTrendTooltipMessages;
}>) {
  const intl = useIntl();

  if (!metric) {
    return <SummaryUnavailableValue />;
  }

  const resolvedTrendChange = trendChange ?? getMetricTrendChange(metric);
  const displayValue = formatOperationalMetricDisplayValue(metric.value, intl);

  return (
    <Flex
      alignItems={{ default: "alignItemsCenter" }}
      spaceItems={{ default: "spaceItemsSm" }}
    >
      <FlexItem>{displayValue}</FlexItem>
      {resolvedTrendChange ? (
        <FlexItem>
          <MetricTrendIndicator
            trendChange={resolvedTrendChange}
            tooltipMessages={trendTooltipMessages}
          />
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
  if (!metric) {
    return <SummaryUnavailableValue />;
  }

  return (
    <Stack hasGutter className="hypershell-dashboard-summary-gateway-value">
      <StackItem>
        <SummaryMetricValue metric={metric} />
      </StackItem>
      <StackItem>
        <SummaryGatewayStatusCounts metric={metric} />
      </StackItem>
    </Stack>
  );
}

interface InventoryExceptionCounts {
  danger: number;
  warning: number;
}

function getInventoryExceptionCounts(
  inventoryStatus: Record<string, number> | undefined,
): InventoryExceptionCounts | undefined {
  if (inventoryStatus === undefined) {
    return undefined;
  }

  let danger = 0;
  let warning = 0;

  for (const [label, count] of Object.entries(inventoryStatus)) {
    if (count <= 0) {
      continue;
    }

    if (/fail/i.test(label)) {
      danger += count;
      continue;
    }

    if (/degrad/i.test(label) || /pending/i.test(label)) {
      warning += count;
    }
  }

  if (danger === 0 && warning === 0) {
    return undefined;
  }

  return { danger, warning };
}

function SummaryInventoryStatusCounts({
  metric,
}: Readonly<{ metric: OperationalMetric }>) {
  const intl = useIntl();
  const exceptionCounts = getInventoryExceptionCounts(metric.inventoryStatus);

  if (exceptionCounts === undefined) {
    return null;
  }

  const { danger, warning } = exceptionCounts;

  if (danger === 0 && warning === 0) {
    return null;
  }

  return (
    <Flex
      alignItems={{ default: "alignItemsCenter" }}
      className="hypershell-dashboard-summary-gateway-status"
      direction={{ default: "row" }}
      flexWrap={{ default: "nowrap" }}
      spaceItems={{ default: "spaceItemsSm" }}
    >
      {danger > 0 ? (
        <FlexItem>
          <SummaryGatewayStatusCount
            count={danger}
            statusLabel={intl.formatMessage(messages.gatewayStatusFailed)}
            variant="danger"
          />
        </FlexItem>
      ) : null}
      {danger > 0 && warning > 0 ? (
        <FlexItem>
          <Divider
            className="hypershell-dashboard-summary-gateway-status__divider"
            orientation={{ default: "vertical" }}
          />
        </FlexItem>
      ) : null}
      {warning > 0 ? (
        <FlexItem>
          <SummaryGatewayStatusCount
            count={warning}
            statusLabel={intl.formatMessage(messages.inventoryStatusWarning)}
            variant="warning"
          />
        </FlexItem>
      ) : null}
    </Flex>
  );
}

function SummaryInventoryValue({
  metric,
}: Readonly<{ metric: OperationalMetric | undefined }>) {
  if (!metric) {
    return <SummaryUnavailableValue />;
  }

  return (
    <Stack hasGutter className="hypershell-dashboard-summary-gateway-value">
      <StackItem>
        <SummaryMetricValue metric={metric} />
      </StackItem>
      <StackItem>
        <SummaryInventoryStatusCounts metric={metric} />
      </StackItem>
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
        {USAGE_SUMMARY_METRIC_IDS.map((metricId) => {
          const metric = metrics.find((entry) => entry.id === metricId);

          return (
            <DescriptionListGroup key={metricId}>
              <DescriptionListTerm>
                <FormattedMessage {...USAGE_SUMMARY_LABELS[metricId]} />
              </DescriptionListTerm>
              <DescriptionListDescription>
                {metricId === "provisioned-gateways" ? (
                  <SummaryGatewayValue metric={metric} />
                ) : (
                  <SummaryMetricValue
                    metric={metric}
                    trendChange={
                      metricId === "registered-users"
                        ? getTrendChange(metric?.activeTrend)
                        : undefined
                    }
                    trendTooltipMessages={
                      metricId === "registered-users"
                        ? USERS_SUMMARY_TREND_TOOLTIP_MESSAGES
                        : undefined
                    }
                  />
                )}
              </DescriptionListDescription>
            </DescriptionListGroup>
          );
        })}
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

export function InventorySummaryCard({
  metrics,
}: Readonly<{ metrics: readonly OperationalMetric[] }>) {
  const intl = useIntl();
  const managedClusters = metrics.find(
    (metric) => metric.id === "managed-clusters",
  );
  const managedDatabases = metrics.find(
    (metric) => metric.id === "managed-databases",
  );

  return (
    <WidgetContent>
      <DescriptionList
        isHorizontal
        aria-label={intl.formatMessage(messages.inventorySummaryAriaLabel)}
      >
        <DescriptionListGroup>
          <DescriptionListTerm>
            <FormattedMessage {...messages.managedClustersSummary} />
          </DescriptionListTerm>
          <DescriptionListDescription>
            <SummaryInventoryValue metric={managedClusters} />
          </DescriptionListDescription>
        </DescriptionListGroup>
        <DescriptionListGroup>
          <DescriptionListTerm>
            <FormattedMessage {...messages.managedClustersCreatedLast30Days} />
          </DescriptionListTerm>
          <DescriptionListDescription>
            {managedClusters?.createdLast30Days !== undefined ? (
              <SummaryMetricValue
                metric={{
                  id: "managed-clusters-created-last-30-days",
                  value: managedClusters.createdLast30Days,
                }}
              />
            ) : (
              <SummaryUnavailableValue />
            )}
          </DescriptionListDescription>
        </DescriptionListGroup>
        <DescriptionListGroup>
          <DescriptionListTerm>
            <FormattedMessage {...messages.managedDatabasesSummary} />
          </DescriptionListTerm>
          <DescriptionListDescription>
            <SummaryInventoryValue metric={managedDatabases} />
          </DescriptionListDescription>
        </DescriptionListGroup>
      </DescriptionList>
    </WidgetContent>
  );
}

export function SectionTitleCard({ title }: Readonly<{ title: string }>) {
  return <h2 className="hypershell-dashboard-section-title">{title}</h2>;
}
