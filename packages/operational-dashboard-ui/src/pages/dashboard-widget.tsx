import {
  Bullseye,
  Button,
  Card,
  CardBody,
  Content,
  DataList,
  DataListCell,
  DataListItem,
  DataListItemCells,
  DataListItemRow,
  DescriptionList,
  DescriptionListDescription,
  DescriptionListGroup,
  DescriptionListTerm,
  Divider,
  EmptyState,
  EmptyStateBody,
  EmptyStateVariant,
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
import type { PropsWithChildren, ReactNode } from "react";
import { FormattedMessage, useIntl, type IntlShape } from "react-intl";

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
import { GatewayReleasesChart } from "../dashboard/gateway-releases-chart";
import { GatewayStatusChart } from "../dashboard/gateway-status-chart";
import { SandboxStatusChart } from "../dashboard/sandbox-status-chart";
import { ManagedClusterProvidersChart } from "../dashboard/managed-cluster-providers-chart";
import { ManagedClusterRegionsChart } from "../dashboard/managed-cluster-regions-chart";
import { NodeStatusChart } from "../dashboard/node-status-chart";
import { PodCapacityChart } from "../dashboard/pod-capacity-chart";
import { ProvisionReliabilityChart } from "../dashboard/provision-reliability-chart";
import { getProvisionReliabilitySummaryTermMessage } from "../dashboard/provision-reliability-data";
import { ProvisionTimeChart } from "../dashboard/provision-time-chart";
import { isPodCapacityMetric } from "../dashboard/pod-capacity-metric";
import {
  formatProvisionDurationValue,
  parseProvisionDurationStats,
} from "../dashboard/provision-time-data";
import {
  getUtilizationPercentage,
  getUtilizationStatusLevel,
  isUtilizationMetric,
  UtilizationChart,
} from "../dashboard/utilization-chart";
import { getDashboardHelpMessage, messages } from "../messages";

function WidgetContent({
  bodyClassName,
  children,
  helpText,
}: Readonly<
  PropsWithChildren<{ bodyClassName?: string; helpText?: ReactNode }>
>) {
  return (
    <Card isPlain isFullHeight>
      <CardBody
        className={
          bodyClassName
            ? `${bodyClassName} hypershell-dashboard-widget-card`
            : "hypershell-dashboard-widget-card"
        }
      >
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

export function DashboardWidgetContent({
  bodyClassName,
  children,
}: Readonly<PropsWithChildren<{ bodyClassName?: string }>>) {
  return (
    <WidgetContent bodyClassName={bodyClassName}>{children}</WidgetContent>
  );
}

export function DashboardMetricUnavailable() {
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

export type DashboardSummaryTone = "positive" | "negative" | "warning";

export type DashboardSummaryIndicator = Readonly<{
  content: ReactNode;
  tone: DashboardSummaryTone;
}>;

export type DashboardSummaryItem = Readonly<{
  indicator?: DashboardSummaryIndicator;
  key: string;
  label: ReactNode;
  value: ReactNode;
}>;

export function DashboardSummaryList({
  ariaLabel,
  items,
}: Readonly<{
  ariaLabel?: string;
  items: readonly DashboardSummaryItem[];
}>) {
  return (
    <DescriptionList
      aria-label={ariaLabel}
      className="hypershell-dashboard-summary-list"
      isHorizontal
    >
      {items.map((item) => (
        <DescriptionListGroup key={item.key}>
          <DescriptionListTerm>{item.label}</DescriptionListTerm>
          <DescriptionListDescription>
            <span className="hypershell-dashboard-summary-list__value">
              <span
                className={
                  item.indicator
                    ? `hypershell-dashboard-summary-list__indicator--${item.indicator.tone}`
                    : "hypershell-dashboard-summary-list__indicator"
                }
              >
                {item.indicator?.content}
              </span>
              <strong>{item.value}</strong>
            </span>
          </DescriptionListDescription>
        </DescriptionListGroup>
      ))}
    </DescriptionList>
  );
}

export function MetricCard({
  metric,
  showTrend = true,
  helpText,
  subtitle,
  title,
}: Readonly<{
  metric: OperationalMetric;
  showTrend?: boolean;
  subtitle: string;
  title: string;
  helpText?: ReactNode;
}>) {
  const intl = useIntl();
  const displayValue = formatOperationalMetricDisplayValue(metric.value, intl);
  const metricHeading = isDisplayableOperationalMetricValue(metric.value)
    ? intl.formatMessage(messages.metricValue, {
        label: title,
        value: displayValue,
      })
    : displayValue;
  const helpMessage = getDashboardHelpMessage(metric.id);

  return (
    <WidgetContent
      helpText={
        helpText ??
        (helpMessage ? (
          <FormattedMessage {...helpMessage} values={{ br: () => <br /> }} />
        ) : undefined)
      }
    >
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

function UsersStatValue({ value }: Readonly<{ value: string | undefined }>) {
  const intl = useIntl();

  if (value === undefined) {
    return <SummaryUnavailableValue />;
  }

  return <span>{formatOperationalMetricDisplayValue(value, intl)}</span>;
}

function UsersStatCell({
  label,
  value,
}: Readonly<{
  label: ReactNode;
  value: string | undefined;
}>) {
  return (
    <DataListCell className="hypershell-dashboard-users-stat-cell" width={2}>
      <span className="hypershell-dashboard-users-stat-cell__label">
        {label}
      </span>
      <span className="hypershell-dashboard-users-stat-cell__value">
        <UsersStatValue value={value} />
      </span>
    </DataListCell>
  );
}

function UsersAdoptionStats({
  metric,
}: Readonly<{ metric: OperationalMetric }>) {
  const intl = useIntl();
  const statsAriaLabel = intl.formatMessage(messages.registeredUsers);

  return (
    <DataList
      aria-label={statsAriaLabel}
      className="hypershell-dashboard-users-card__stats"
      isCompact
      isPlain
    >
      <DataListItem aria-labelledby="users-adoption-stats">
        <DataListItemRow>
          <DataListItemCells
            dataListCells={[
              <UsersStatCell
                key="added-7-days"
                label={
                  <FormattedMessage {...messages.registeredUsersAdded7Days} />
                }
                value={metric.createdLast7Days}
              />,
              <UsersStatCell
                key="added-30-days"
                label={
                  <FormattedMessage {...messages.registeredUsersAdded30Days} />
                }
                value={metric.createdLast30Days}
              />,
            ]}
            rowid="users-adoption-added"
          />
        </DataListItemRow>
        <DataListItemRow>
          <DataListItemCells
            dataListCells={[
              <UsersStatCell
                key="unique-logins-7-days"
                label={
                  <FormattedMessage
                    {...messages.registeredUsersUniqueLogins7Days}
                  />
                }
                value={metric.uniqueLoginsLast7Days}
              />,
              <UsersStatCell
                key="unique-logins-30-days"
                label={
                  <FormattedMessage
                    {...messages.registeredUsersUniqueLogins30Days}
                  />
                }
                value={metric.uniqueLoginsLast30Days}
              />,
            ]}
            rowid="users-adoption-logins"
          />
        </DataListItemRow>
      </DataListItem>
    </DataList>
  );
}

export function UsersCard({
  metric,
}: Readonly<{ metric: OperationalMetric | undefined }>) {
  const intl = useIntl();

  if (!metric) {
    return (
      <WidgetContent>
        <SummaryUnavailableValue />
      </WidgetContent>
    );
  }

  const heroValue = formatOperationalMetricDisplayValue(metric.value, intl);
  const sparklineTitle = intl.formatMessage(
    messages.registeredUsersUniqueLoginsPerDay,
  );
  const sparklineTooltipMetric = intl.formatMessage(
    messages.registeredUsersUniqueLogins,
  );

  return (
    <WidgetContent
      helpText={
        <FormattedMessage
          {...messages.dashboardHelpUsers}
          values={{ br: () => <br /> }}
        />
      }
    >
      <Content className="hypershell-dashboard-users-card">
        <Stack hasGutter>
          <StackItem>
            <Flex justifyContent={{ default: "justifyContentCenter" }}>
              <FlexItem>
                <Title headingLevel="h3" size="lg">
                  {isDisplayableOperationalMetricValue(metric.value)
                    ? intl.formatMessage(messages.registeredUsersHero, {
                        value: heroValue,
                      })
                    : heroValue}
                </Title>
              </FlexItem>
            </Flex>
          </StackItem>
          <StackItem>
            <UsersAdoptionStats metric={metric} />
            {metric.trend ? (
              <Stack
                hasGutter
                className="hypershell-dashboard-users-card__sparkline"
              >
                <StackItem>
                  <Title headingLevel="h4" size="md">
                    {sparklineTitle}
                  </Title>
                </StackItem>
                <StackItem>
                  <TrendSparklineChart
                    trend={metric.trend}
                    title={sparklineTooltipMetric}
                  />
                </StackItem>
              </Stack>
            ) : (
              <SummaryUnavailableValue />
            )}
          </StackItem>
        </Stack>
      </Content>
    </WidgetContent>
  );
}

export function GatewayStatusCard({
  metric,
}: Readonly<{ metric: OperationalMetric }>) {
  return (
    <WidgetContent bodyClassName="hypershell-dashboard-status-donut-card--compact">
      <Content className="hypershell-dashboard-status-donut-card">
        <GatewayStatusChart metric={metric} />
      </Content>
    </WidgetContent>
  );
}

export function SandboxStatusCard({
  metric,
}: Readonly<{ metric: OperationalMetric }>) {
  return (
    <WidgetContent bodyClassName="hypershell-dashboard-status-donut-card--compact">
      <div className="hypershell-dashboard-status-donut-card">
        <Content>
          <SandboxStatusChart metric={metric} />
        </Content>
      </div>
    </WidgetContent>
  );
}

export function NodeStatusCard({
  metric,
  title,
}: Readonly<{ metric: OperationalMetric; title?: string }>) {
  return (
    <WidgetContent bodyClassName="hypershell-dashboard-status-donut-card--compact">
      <Content className="hypershell-dashboard-status-donut-card">
        <NodeStatusChart metric={metric} title={title} />
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

export function PodCapacityCard({
  metric,
  showTrend = true,
}: Readonly<{ metric: OperationalMetric; showTrend?: boolean }>) {
  const intl = useIntl();
  const trendTitle = intl.formatMessage(messages.widgetPods);

  return (
    <WidgetContent bodyClassName="hypershell-dashboard-status-donut-card--compact">
      <Content className="hypershell-dashboard-status-donut-card">
        <Stack hasGutter>
          <StackItem>
            <PodCapacityChart metric={metric} />
          </StackItem>
          {showTrend && metric.trend && metric.trend.points.length >= 2 ? (
            <StackItem>
              <TrendSparklineChart
                caption={intl.formatMessage(messages.trendLast7Days)}
                trend={metric.trend}
                title={trendTitle}
              />
            </StackItem>
          ) : null}
        </Stack>
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

export function GatewayReleasesCard({
  metric,
}: Readonly<{ metric: OperationalMetric }>) {
  return (
    <WidgetContent
      helpText={<FormattedMessage {...messages.gatewayReleasesHelp} />}
    >
      <Content className="hypershell-dashboard-provision-time-card">
        <GatewayReleasesChart metric={metric} />
      </Content>
    </WidgetContent>
  );
}

export function ActiveSandboxesCard({
  metric,
}: Readonly<{
  metric: OperationalMetric;
}>) {
  const intl = useIntl();

  return (
    <WidgetContent>
      <Stack hasGutter>
        {metric.trend && metric.trend.points.length >= 2 ? (
          <StackItem>
            <TrendSparklineChart
              trend={metric.trend}
              title={intl.formatMessage(messages.activeSandboxesWidget)}
            />
          </StackItem>
        ) : (
          <SummaryUnavailableValue />
        )}
      </Stack>
    </WidgetContent>
  );
}

export function ProvisioningSnapshotCard({
  metrics,
}: Readonly<{ metrics: readonly OperationalMetric[] }>) {
  const intl = useIntl();
  const gateways = metrics.find(
    (metric) => metric.id === "provisioned-gateways",
  );
  const duration = metrics.find((metric) => metric.id === "provision-time");
  const reliability = metrics.find(
    (metric) => metric.id === "provision-reliability",
  );
  const durationStats = duration ? parseProvisionDurationStats(duration) : null;
  const successRate = reliability?.provisionOutcomes?.successRatePercent;
  const medianDuration = durationStats
    ? durationStats.p50Seconds >= 60
      ? intl.formatMessage(messages.durationMinutesSeconds, {
          minutes: Math.floor(durationStats.p50Seconds / 60),
          seconds: Math.round(durationStats.p50Seconds % 60),
        })
      : formatProvisionDurationValue(
          intl,
          durationStats.p50Seconds,
          duration?.unit,
        )
    : undefined;

  return (
    <WidgetContent
      helpText={<FormattedMessage {...messages.provisioningSnapshotHelp} />}
    >
      <DescriptionList
        isHorizontal
        className="hypershell-dashboard-compact-list hypershell-dashboard-provisioning-snapshot"
      >
        <DescriptionListGroup>
          <DescriptionListTerm>
            <FormattedMessage {...messages.provisionSuccessRate24h} />
          </DescriptionListTerm>
          <DescriptionListDescription>
            {successRate ? (
              intl.formatMessage(messages.provisionReliabilityRate, {
                rate: successRate,
              })
            ) : (
              <SummaryUnavailableValue />
            )}
          </DescriptionListDescription>
        </DescriptionListGroup>
        <DescriptionListGroup>
          <DescriptionListTerm>
            <FormattedMessage {...messages.provisioningMedianTime} />
          </DescriptionListTerm>
          <DescriptionListDescription>
            {medianDuration ?? <SummaryUnavailableValue />}
          </DescriptionListDescription>
        </DescriptionListGroup>
        <DescriptionListGroup>
          <DescriptionListTerm>
            <FormattedMessage {...messages.provisioningInProgress} />
          </DescriptionListTerm>
          <DescriptionListDescription>
            {gateways?.status?.provisioning ?? 0}
          </DescriptionListDescription>
        </DescriptionListGroup>
      </DescriptionList>
    </WidgetContent>
  );
}

function SummaryProvisionSuccessRateTerm({
  metric,
}: Readonly<{ metric: OperationalMetric | undefined }>) {
  const intl = useIntl();

  return (
    <span>
      {intl.formatMessage(
        getProvisionReliabilitySummaryTermMessage(
          metric?.provisionOutcomes?.successCountWindow,
        ),
      )}
    </span>
  );
}

function SummaryProvisionSuccessRateValue({
  metric,
}: Readonly<{ metric: OperationalMetric | undefined }>) {
  const intl = useIntl();

  if (!metric?.provisionOutcomes) {
    return <SummaryUnavailableValue />;
  }

  const rate = metric.provisionOutcomes.successRatePercent;
  if (!isDisplayableOperationalMetricValue(rate)) {
    return <SummaryUnavailableValue />;
  }

  const trendChange = getMetricTrendChange(metric);

  return (
    <Flex
      alignItems={{ default: "alignItemsCenter" }}
      spaceItems={{ default: "spaceItemsSm" }}
    >
      <FlexItem>
        {intl.formatMessage(messages.provisionReliabilityRate, { rate })}
      </FlexItem>
      {trendChange ? (
        <FlexItem>
          <SummaryTrendIndicator metric={metric} trendChange={trendChange} />
        </FlexItem>
      ) : null}
    </Flex>
  );
}

export function ProvisionReliabilityCard({
  metric,
}: Readonly<{ metric: OperationalMetric }>) {
  return (
    <WidgetContent bodyClassName="hypershell-dashboard-status-donut-card--compact">
      <Content className="hypershell-dashboard-status-donut-card">
        <ProvisionReliabilityChart metric={metric} />
      </Content>
    </WidgetContent>
  );
}

function utilizationTrendTitle(
  metric: OperationalMetric,
  intl: IntlShape,
): string {
  switch (metric.id) {
    case "memory":
      return intl.formatMessage(messages.widgetMemory);
    case "cpu":
      return intl.formatMessage(messages.widgetCpu);
    default:
      return intl.formatMessage(messages.summaryUsage);
  }
}

export function UtilizationCard({
  trendCaption,
  metric,
  color,
}: Readonly<{
  metric: OperationalMetric;
  color?: string;
  trendCaption?: string;
}>) {
  const intl = useIntl();
  const trendTitle = utilizationTrendTitle(metric, intl);

  return (
    <WidgetContent bodyClassName="hypershell-dashboard-status-donut-card--compact">
      <Content className="hypershell-dashboard-status-donut-card">
        <Stack hasGutter>
          {isUtilizationMetric(metric) ? (
            <StackItem>
              <UtilizationChart color={color} metric={metric} />
            </StackItem>
          ) : null}
          {metric.trend && metric.trend.points.length >= 2 ? (
            <StackItem>
              <TrendSparklineChart
                caption={
                  trendCaption ?? intl.formatMessage(messages.trendLast7Days)
                }
                trend={metric.trend}
                title={trendTitle}
              />
            </StackItem>
          ) : null}
        </Stack>
      </Content>
    </WidgetContent>
  );
}

function getSummaryTrendSubject(
  metric: OperationalMetric,
  intl: IntlShape,
): string {
  switch (metric.id) {
    case "memory":
      return intl.formatMessage(messages.memory);
    case "cpu":
      return intl.formatMessage(messages.cpus);
    case "pods":
      return intl.formatMessage(messages.pods);
    case "provision-reliability":
      return intl.formatMessage(
        getProvisionReliabilitySummaryTermMessage(
          metric.provisionOutcomes?.successCountWindow,
        ),
      );
    case "registered-users":
      return intl.formatMessage(messages.registeredUsersUniqueLogins);
    case "provisioned-gateways":
      return intl.formatMessage(messages.gateways);
    case "provisioned-sandboxes":
      return intl.formatMessage(messages.widgetSandboxes);
    case "managed-clusters":
      return intl.formatMessage(messages.managedClustersSummary);
    default:
      return intl.formatMessage(messages.summaryUsage);
  }
}

function SummaryTrendIndicator({
  metric,
  trendChange,
}: Readonly<{ metric: OperationalMetric; trendChange: MetricTrendChange }>) {
  const intl = useIntl();
  const isIncrease = trendChange.direction === "increase";
  const tooltipContent = intl.formatMessage(
    isIncrease ? messages.summaryTrendIncrease : messages.summaryTrendDecrease,
    {
      percent: trendChange.percent,
      subject: getSummaryTrendSubject(metric, intl),
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
  const trendChange = getMetricTrendChange(metric);

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
      {trendChange ? (
        <FlexItem>
          <SummaryTrendIndicator metric={metric} trendChange={trendChange} />
        </FlexItem>
      ) : null}
    </Flex>
  );
}

const summaryUnavailableValue = "-";

function SummaryUnavailableValue() {
  return (
    <span className="hypershell-dashboard-summary-unavailable">
      {summaryUnavailableValue}
    </span>
  );
}

function SummaryMetricValue({
  metric,
}: Readonly<{ metric: OperationalMetric | undefined }>) {
  const intl = useIntl();

  if (!metric) {
    return <SummaryUnavailableValue />;
  }

  const trendChange = getMetricTrendChange(metric);
  const displayValue = formatOperationalMetricDisplayValue(metric.value, intl);

  return (
    <Flex
      alignItems={{ default: "alignItemsCenter" }}
      spaceItems={{ default: "spaceItemsSm" }}
    >
      <FlexItem>{displayValue}</FlexItem>
      {trendChange ? (
        <FlexItem>
          <SummaryTrendIndicator metric={metric} trendChange={trendChange} />
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

export interface InventoryExceptionCounts {
  danger: number;
  warning: number;
}

export function getInventoryExceptionCounts(
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
  "provisioned-gateways",
  "provisioned-sandboxes",
] as const;

const USAGE_SUMMARY_LABELS = {
  "provisioned-gateways": messages.gateways,
  "provisioned-sandboxes": messages.widgetSandboxes,
} as const;

function getResourceSummaryCounts(metrics: readonly OperationalMetric[]) {
  const gateways = metrics.find(
    (metric) => metric.id === "provisioned-gateways",
  );
  const sandboxes = metrics.find(
    (metric) => metric.id === "provisioned-sandboxes",
  );
  const gatewayTotal = Number(gateways?.value ?? 0);
  const sandboxTotal = Number(sandboxes?.value ?? 0);
  const gatewayAttention =
    (gateways?.status?.degraded ?? 0) + (gateways?.status?.failed ?? 0);
  const sandboxAttention =
    (sandboxes?.expiringSandboxes ?? 0) +
    (sandboxes?.idleSandboxes ?? 0) +
    (sandboxes?.orphanedSandboxes ?? 0);
  return {
    attention: gatewayAttention + sandboxAttention,
    healthy: gatewayTotal - gatewayAttention + sandboxTotal - sandboxAttention,
  };
}

export function UsageSummaryCard({
  metrics,
}: Readonly<{ metrics: readonly OperationalMetric[] }>) {
  const intl = useIntl();
  const resourceSummary = getResourceSummaryCounts(metrics);
  const items: DashboardSummaryItem[] = USAGE_SUMMARY_METRIC_IDS.map(
    (metricId) => {
      const metric = metrics.find((metric) => metric.id === metricId);
      const trendChange = metric ? getMetricTrendChange(metric) : undefined;

      return {
        indicator:
          metric && trendChange
            ? {
                content: (
                  <SummaryTrendIndicator
                    metric={metric}
                    trendChange={trendChange}
                  />
                ),
                tone:
                  trendChange.direction === "increase"
                    ? "positive"
                    : "negative",
              }
            : undefined,
        key: metricId,
        label: <FormattedMessage {...USAGE_SUMMARY_LABELS[metricId]} />,
        value: metric ? (
          formatOperationalMetricDisplayValue(metric.value, intl)
        ) : (
          <SummaryUnavailableValue />
        ),
      };
    },
  );

  return (
    <WidgetContent
      helpText={<FormattedMessage {...messages.resourceSummaryHelp} />}
    >
      <DashboardSummaryList
        ariaLabel={intl.formatMessage(messages.summaryUsageAriaLabel)}
        items={[
          ...items,
          {
            key: "healthy-resources",
            label: <FormattedMessage {...messages.summaryHealthyResources} />,
            value: resourceSummary.healthy,
          },
          {
            indicator:
              resourceSummary.attention > 0
                ? {
                    content: <ExclamationCircleIcon aria-hidden />,
                    tone: "negative",
                  }
                : undefined,
            key: "resources-needing-attention",
            label: (
              <FormattedMessage
                {...messages.summaryResourcesNeedingAttention}
              />
            ),
            value: resourceSummary.attention,
          },
        ]}
      />
    </WidgetContent>
  );
}

export function AttentionRequiredCard({
  metrics,
}: Readonly<{ metrics: readonly OperationalMetric[] }>) {
  const intl = useIntl();
  const gateway = metrics.find(
    (metric) => metric.id === "provisioned-gateways",
  );
  const sandboxes = metrics.find(
    (metric) => metric.id === "provisioned-sandboxes",
  );
  const counts = [
    [messages.gatewayStatusFailedGateways, gateway?.status?.failed ?? 0],
    [messages.gatewayStatusDegradedGateways, gateway?.status?.degraded ?? 0],
    [
      messages.sandboxAttentionExpiringSandboxes,
      sandboxes?.expiringSandboxes ?? 0,
    ],
    [
      messages.sandboxAttentionIdleOrOrphaned,
      (sandboxes?.idleSandboxes ?? 0) + (sandboxes?.orphanedSandboxes ?? 0),
    ],
  ] as const;
  const hasAttention = counts.some(([, count]) => count > 0);

  return (
    <WidgetContent>
      {hasAttention ? (
        <DescriptionList
          isHorizontal
          className="hypershell-dashboard-attention-required"
          aria-label={intl.formatMessage(messages.attentionRequiredWidget)}
        >
          {counts.map(([label, count]) =>
            count > 0 ? (
              <DescriptionListGroup key={label.id}>
                <DescriptionListTerm>
                  <FormattedMessage {...label} />
                </DescriptionListTerm>
                <DescriptionListDescription>{count}</DescriptionListDescription>
              </DescriptionListGroup>
            ) : null,
          )}
        </DescriptionList>
      ) : (
        <Content>
          <FormattedMessage {...messages.noResourcesNeedAttention} />
        </Content>
      )}
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
        <DescriptionListGroup>
          <DescriptionListTerm>
            <SummaryProvisionSuccessRateTerm
              metric={metrics.find(
                (metric) => metric.id === "provision-reliability",
              )}
            />
          </DescriptionListTerm>
          <DescriptionListDescription>
            <SummaryProvisionSuccessRateValue
              metric={metrics.find(
                (metric) => metric.id === "provision-reliability",
              )}
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
      </DescriptionList>
    </WidgetContent>
  );
}

export function SectionTitleCard({ title }: Readonly<{ title: string }>) {
  return <h2 className="hypershell-dashboard-section-title">{title}</h2>;
}
