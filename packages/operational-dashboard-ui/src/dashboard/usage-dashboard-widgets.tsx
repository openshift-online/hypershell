import {
  Bullseye,
  Card,
  CardBody,
  Content,
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
  Label,
  Stack,
  StackItem,
  Title,
} from "@patternfly/react-core";
import { FormattedMessage, useIntl, type IntlShape } from "react-intl";

import type { UsageMetric } from "../application/dashboard-types";
import { TrendSparklineChart } from "./trend-sparkline-chart";
import { UtilizationChart } from "./utilization-chart";
import { messages } from "../messages";
import "../pages/dashboard-widget.css";
import "./usage-dashboard-widgets.css";

const NON_DISPLAYABLE_USAGE_VALUES = new Set([
  "",
  "NaN",
  "Infinity",
  "-Infinity",
]);

function WidgetContent({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <Card isFullHeight isPlain>
      <CardBody className="hypershell-dashboard-widget-card">
        {children}
      </CardBody>
    </Card>
  );
}

function UsageMetricUnavailableEmptyState() {
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

function formatUsageMetricValue(metric: UsageMetric, intl: IntlShape): string {
  if (NON_DISPLAYABLE_USAGE_VALUES.has(metric.value)) {
    return intl.formatMessage(messages.metricCouldNotBeDetermined);
  }

  if (metric.unit === undefined || metric.unit === "") {
    return metric.value;
  }

  return intl.formatMessage(messages.utilizationLabel, {
    unit: metric.unit,
    value: metric.value,
  });
}

function hasPartialTrendData(metric: UsageMetric): boolean {
  return (
    metric.trend?.isPartial === true ||
    metric.trend?.points.some((point) => point.value === null) === true
  );
}

function PartialDataLabel() {
  return (
    <Label color="blue" isCompact>
      <FormattedMessage {...messages.usagePartialDataLabel} />
    </Label>
  );
}

function UsageRow({
  label,
  value,
}: Readonly<{ label: string; value: string | undefined }>) {
  return (
    <DescriptionListGroup>
      <DescriptionListTerm>{label}</DescriptionListTerm>
      <DescriptionListDescription>
        <strong>
          {value ?? <FormattedMessage {...messages.metricUnavailableTitle} />}
        </strong>
      </DescriptionListDescription>
    </DescriptionListGroup>
  );
}

function UsageSummaryList({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  return (
    <DescriptionList className="hypershell-dashboard-summary-list" isHorizontal>
      {children}
    </DescriptionList>
  );
}

function UsageValue({
  metric,
  subtitle,
}: Readonly<{ metric: UsageMetric; subtitle?: string }>) {
  const intl = useIntl();

  return (
    <Flex justifyContent={{ default: "justifyContentCenter" }}>
      <FlexItem>
        <Title headingLevel="h3" size="lg">
          <>
            {formatUsageMetricValue(metric, intl)}
            {subtitle ? (
              <>
                <br />
                <small>{subtitle}</small>
              </>
            ) : null}
          </>
        </Title>
      </FlexItem>
    </Flex>
  );
}

function formatUsageLifetime(
  intl: IntlShape,
  value: string | undefined,
): string | undefined {
  const seconds = Number(value);
  if (value === undefined || !Number.isFinite(seconds) || seconds < 0) {
    return undefined;
  }

  const wholeSeconds = Math.round(seconds);
  if (wholeSeconds < 3600) {
    return intl.formatMessage(messages.durationMinutesSeconds, {
      minutes: Math.floor(wholeSeconds / 60),
      seconds: String(wholeSeconds % 60).padStart(2, "0"),
    });
  }
  if (wholeSeconds < 86400) {
    return intl.formatMessage(messages.durationHoursMinutes, {
      hours: Math.floor(wholeSeconds / 3600),
      minutes: Math.floor((wholeSeconds % 3600) / 60),
    });
  }

  return intl.formatMessage(messages.durationDaysHours, {
    days: Math.floor(wholeSeconds / 86400),
    hours: Math.floor((wholeSeconds % 86400) / 3600),
  });
}

export function UsageTrendCard({
  metric,
  title,
}: Readonly<{ metric: UsageMetric | undefined; title: string }>) {
  const intl = useIntl();

  if (!metric) {
    return (
      <WidgetContent>
        <UsageMetricUnavailableEmptyState />
      </WidgetContent>
    );
  }

  const subtitle = intl.formatMessage(
    metric.id === "gateway-error-rate"
      ? messages.usageErrorRateSubtitle
      : messages.usageRequestVolumeSubtitle,
  );

  return (
    <WidgetContent>
      <Content className="hypershell-dashboard-metric-card">
        <Stack hasGutter>
          <StackItem>
            <UsageValue metric={metric} subtitle={subtitle} />
          </StackItem>
          {metric.trend && metric.trend.points.length >= 2 ? (
            <StackItem>
              <TrendSparklineChart
                title={title}
                trend={metric.trend}
                valueFormatter={(value) => value.toFixed(3)}
              />
            </StackItem>
          ) : null}
          {hasPartialTrendData(metric) ? (
            <StackItem>
              <PartialDataLabel />
            </StackItem>
          ) : null}
        </Stack>
      </Content>
    </WidgetContent>
  );
}

export function UsageGatewayActivityCard({
  metric,
}: Readonly<{ metric: UsageMetric | undefined }>) {
  const intl = useIntl();

  if (!metric) {
    return (
      <WidgetContent>
        <UsageMetricUnavailableEmptyState />
      </WidgetContent>
    );
  }

  return (
    <WidgetContent>
      <Stack hasGutter>
        <StackItem>
          <UsageValue metric={metric} />
        </StackItem>
        {metric.id === "active-gateways" ? (
          <StackItem>
            <UsageSummaryList>
              <UsageRow
                label={intl.formatMessage(messages.usageProvisionedGateways)}
                value={metric.total}
              />
              <UsageRow
                label={intl.formatMessage(messages.usageActiveShare)}
                value={
                  metric.activeSharePercent === undefined
                    ? undefined
                    : `${metric.activeSharePercent}%`
                }
              />
            </UsageSummaryList>
          </StackItem>
        ) : (
          <StackItem>
            <Divider />
            <p className="hypershell-dashboard-widget-help-text">
              <FormattedMessage {...messages.usageUnusedGatewaysHelp} />
            </p>
          </StackItem>
        )}
        {hasPartialTrendData(metric) ? (
          <StackItem>
            <PartialDataLabel />
          </StackItem>
        ) : null}
      </Stack>
    </WidgetContent>
  );
}

export function UsageSandboxUtilizationCard({
  metric,
}: Readonly<{ metric: UsageMetric | undefined }>) {
  const intl = useIntl();

  if (!metric) {
    return (
      <WidgetContent>
        <UsageMetricUnavailableEmptyState />
      </WidgetContent>
    );
  }

  return (
    <WidgetContent>
      <Stack hasGutter>
        <StackItem>
          <UtilizationChart metric={metric} />
        </StackItem>
        <StackItem>
          <UsageSummaryList>
            <UsageRow
              label={intl.formatMessage(messages.usageActiveSandboxes)}
              value={metric.value}
            />
            <UsageRow
              label={intl.formatMessage(messages.usageProvisionedSandboxes)}
              value={metric.total}
            />
          </UsageSummaryList>
        </StackItem>
        {hasPartialTrendData(metric) ? (
          <StackItem>
            <PartialDataLabel />
          </StackItem>
        ) : null}
      </Stack>
    </WidgetContent>
  );
}

export function UsageSandboxLifetimeCard({
  metric,
}: Readonly<{ metric: UsageMetric | undefined }>) {
  const intl = useIntl();

  if (!metric) {
    return (
      <WidgetContent>
        <UsageMetricUnavailableEmptyState />
      </WidgetContent>
    );
  }

  const lifetime = metric.sandboxLifetime;
  const mean = formatUsageLifetime(intl, lifetime?.mean ?? metric.value);

  return (
    <WidgetContent>
      <Stack hasGutter>
        <StackItem>
          <UsageValue
            metric={{ ...metric, value: mean ?? "NaN" }}
            subtitle={intl.formatMessage(messages.usageSandboxLifetimeMean)}
          />
        </StackItem>
        <StackItem>
          <UsageSummaryList>
            <UsageRow
              label={intl.formatMessage(messages.usageSandboxLifetimeP50)}
              value={formatUsageLifetime(intl, lifetime?.p50)}
            />
            <UsageRow
              label={intl.formatMessage(messages.usageSandboxLifetimeP95)}
              value={formatUsageLifetime(intl, lifetime?.p95)}
            />
          </UsageSummaryList>
        </StackItem>
        {metric.trend && metric.trend.points.length >= 2 ? (
          <StackItem>
            <TrendSparklineChart
              title={intl.formatMessage(messages.usageWidgetSandboxLifetime)}
              trend={metric.trend}
              valueFormatter={(value) => value.toFixed(3)}
            />
          </StackItem>
        ) : null}
        {hasPartialTrendData(metric) ? (
          <StackItem>
            <PartialDataLabel />
          </StackItem>
        ) : null}
      </Stack>
    </WidgetContent>
  );
}
