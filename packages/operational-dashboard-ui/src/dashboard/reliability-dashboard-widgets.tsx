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
import { getDashboardHelpMessage, messages } from "../messages";
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
  const unit =
    metric.id === "reconciliation-failures"
      ? "failures"
      : metric.id === "reconciliation-retries"
        ? "retries"
        : metric.id === "reconciliation-successes"
          ? "successes"
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

type ReliabilitySummaryMetricId =
  | "api-request-rate"
  | "api-error-rate"
  | "api-latency"
  | "api-p95-latency"
  | "oidc-client-sync"
  | "state-accuracy"
  | "reconciliation-failures"
  | "reconciliation-retries"
  | "reconciliation-successes"
  | "reconciliation-lag"
  | "stale-resource-status-count";

const RELIABILITY_SUMMARY_LABELS: Record<
  ReliabilitySummaryMetricId,
  (typeof messages)[keyof typeof messages]
> = {
  "api-request-rate": messages.reliabilitySummaryRequestRate,
  "api-error-rate": messages.reliabilitySummaryErrorRate,
  "api-latency": messages.reliabilitySummaryLatency,
  "api-p95-latency": messages.apiP95LatencyWidget,
  "oidc-client-sync": messages.oidcClientSyncWidget,
  "state-accuracy": messages.stateAccuracyWidget,
  "reconciliation-failures": messages.reliabilitySummaryReconciliationFailures,
  "reconciliation-retries": messages.reliabilitySummaryReconciliationRetries,
  "reconciliation-successes":
    messages.reliabilitySummaryReconciliationSuccesses,
  "reconciliation-lag": messages.reliabilitySummaryReconciliationLag,
  "stale-resource-status-count": messages.reliabilitySummaryStaleResourceStatus,
};

const API_RELIABILITY_SUMMARY_METRIC_IDS: readonly ReliabilitySummaryMetricId[] =
  ["api-request-rate", "api-error-rate", "api-latency", "api-p95-latency"];

const RECONCILIATION_SUMMARY_METRIC_IDS: readonly ReliabilitySummaryMetricId[] =
  [
    "reconciliation-failures",
    "reconciliation-retries",
    "reconciliation-successes",
    "reconciliation-lag",
    "stale-resource-status-count",
    "oidc-client-sync",
    "state-accuracy",
  ];

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
  const summaryUnavailableValue = "-";

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
                  {summaryUnavailableValue}
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

function UnknownValue() {
  return <FormattedMessage {...messages.unknown} />;
}

export function OidcClientSyncCard({
  metric,
}: Readonly<{ metric: OperationalMetric | undefined }>) {
  const intl = useIntl();
  const sync = metric?.oidcClientSync;

  if (!sync) {
    return (
      <WidgetContent>
        <MetricUnavailableEmptyState />
      </WidgetContent>
    );
  }

  const keycloakStatus =
    sync.keycloakStatus === "connected"
      ? messages.connected
      : sync.keycloakStatus === "disconnected"
        ? messages.disconnected
        : messages.unknown;

  return (
    <WidgetContent
      helpText={<FormattedMessage {...messages.oidcClientSyncHelp} />}
    >
      <DescriptionList
        aria-label={intl.formatMessage(messages.oidcClientSyncAriaLabel)}
        isHorizontal
      >
        <DescriptionListGroup>
          <DescriptionListTerm>
            <FormattedMessage {...messages.accuracy} />
          </DescriptionListTerm>
          <DescriptionListDescription>
            {sync.accuracyPercent === undefined ? (
              <UnknownValue />
            ) : (
              <FormattedMessage
                {...messages.percentValue}
                values={{ value: sync.accuracyPercent }}
              />
            )}
          </DescriptionListDescription>
        </DescriptionListGroup>
        <DescriptionListGroup>
          <DescriptionListTerm>
            <FormattedMessage {...messages.syncedClients} />
          </DescriptionListTerm>
          <DescriptionListDescription>
            {sync.syncedClients === undefined ||
            sync.expectedClients === undefined ? (
              <UnknownValue />
            ) : (
              <FormattedMessage
                {...messages.countOfTotal}
                values={{
                  count: sync.syncedClients,
                  total: sync.expectedClients,
                }}
              />
            )}
          </DescriptionListDescription>
        </DescriptionListGroup>
        <DescriptionListGroup>
          <DescriptionListTerm>
            <FormattedMessage {...messages.expectedClients} />
          </DescriptionListTerm>
          <DescriptionListDescription>
            {sync.expectedClients ?? <UnknownValue />}
          </DescriptionListDescription>
        </DescriptionListGroup>
        <DescriptionListGroup>
          <DescriptionListTerm>
            <FormattedMessage {...messages.keycloakStatus} />
          </DescriptionListTerm>
          <DescriptionListDescription>
            <FormattedMessage {...keycloakStatus} />
          </DescriptionListDescription>
        </DescriptionListGroup>
      </DescriptionList>
    </WidgetContent>
  );
}

export function StateAccuracyCard({
  metric,
}: Readonly<{ metric: OperationalMetric | undefined }>) {
  const intl = useIntl();
  const state = metric?.stateAccuracy;

  if (!state) {
    return (
      <WidgetContent>
        <MetricUnavailableEmptyState />
      </WidgetContent>
    );
  }

  return (
    <WidgetContent
      helpText={<FormattedMessage {...messages.stateAccuracyHelp} />}
    >
      <DescriptionList
        aria-label={intl.formatMessage(messages.stateAccuracyAriaLabel)}
        isHorizontal
      >
        <DescriptionListGroup>
          <DescriptionListTerm>
            <FormattedMessage {...messages.accuracy} />
          </DescriptionListTerm>
          <DescriptionListDescription>
            {state.accuracyPercent === undefined ? (
              <UnknownValue />
            ) : (
              <FormattedMessage
                {...messages.percentValue}
                values={{ value: state.accuracyPercent }}
              />
            )}
          </DescriptionListDescription>
        </DescriptionListGroup>
        <DescriptionListGroup>
          <DescriptionListTerm>
            <FormattedMessage {...messages.observedMatchingDesired} />
          </DescriptionListTerm>
          <DescriptionListDescription>
            {state.observedMatchingDesired === undefined ||
            state.totalResources === undefined ? (
              <UnknownValue />
            ) : (
              <FormattedMessage
                {...messages.countOfTotal}
                values={{
                  count: state.observedMatchingDesired,
                  total: state.totalResources,
                }}
              />
            )}
          </DescriptionListDescription>
        </DescriptionListGroup>
        <DescriptionListGroup>
          <DescriptionListTerm>
            <FormattedMessage {...messages.staleResources} />
          </DescriptionListTerm>
          <DescriptionListDescription>
            {state.staleResources}
          </DescriptionListDescription>
        </DescriptionListGroup>
      </DescriptionList>
    </WidgetContent>
  );
}

function formatReasonCode(reason: string): string {
  return reason
    .split("_")
    .map((word) => word.charAt(0).toUpperCase() + word.slice(1))
    .join(" ");
}

function ReasonBreakdown({
  breakdown,
}: Readonly<{ breakdown: Record<string, number> }>) {
  const entries = Object.entries(breakdown).sort(([, a], [, b]) => b - a);

  return (
    <DescriptionList
      className="hypershell-dashboard-reason-breakdown"
      isCompact
      isHorizontal
    >
      {entries.map(([reason, count]) => (
        <DescriptionListGroup key={reason}>
          <DescriptionListTerm>{formatReasonCode(reason)}</DescriptionListTerm>
          <DescriptionListDescription>
            {Math.round(count)}
          </DescriptionListDescription>
        </DescriptionListGroup>
      ))}
    </DescriptionList>
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

  const subtitleMessage =
    metric.id === "reconciliation-failures" ||
    metric.id === "reconciliation-retries" ||
    metric.id === "reconciliation-successes"
      ? messages.reconciliationFailuresLast24Hours
      : metric.id === "reconciliation-lag"
        ? messages.reconciliationLagCurrent
        : metric.id === "stale-resource-status-count"
          ? messages.staleResourceStatusCurrent
          : messages.apiReliabilityLast5Minutes;
  const displayValue = formatMetricWithUnit(metric, intl);
  const helpMessage = getDashboardHelpMessage(metric.id);

  return (
    <WidgetContent
      helpText={
        helpMessage ? (
          <FormattedMessage {...helpMessage} values={{ br: () => <br /> }} />
        ) : undefined
      }
    >
      <Content className="hypershell-dashboard-metric-card">
        <Stack hasGutter>
          <StackItem>
            <Flex justifyContent={{ default: "justifyContentCenter" }}>
              <FlexItem>
                <Title headingLevel="h3" size="lg">
                  <>
                    {displayValue}
                    <br />
                    <small>{intl.formatMessage(subtitleMessage)}</small>
                  </>
                </Title>
              </FlexItem>
            </Flex>
          </StackItem>
          {metric.reasonBreakdown ? (
            <StackItem>
              <ReasonBreakdown breakdown={metric.reasonBreakdown} />
            </StackItem>
          ) : null}
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
