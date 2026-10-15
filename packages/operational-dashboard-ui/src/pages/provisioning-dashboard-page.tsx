import {
  GridLayout,
  WidgetDrawer,
  type ExtendedTemplateConfig,
  type Variants,
  type WidgetMapping,
} from "@patternfly/widgetized-dashboard";
import "@patternfly/widgetized-dashboard/dist/esm/styles.css";
import {
  Alert,
  Content,
  PageSection,
  Spinner,
  Title,
} from "@patternfly/react-core";
import {
  ChartLineIcon,
  CubesIcon,
  HourglassHalfIcon,
  MemoryIcon,
  MicrochipIcon,
  ServerIcon,
  TachometerAltIcon,
  TrendDownIcon,
  TrendUpIcon,
} from "@patternfly/react-icons";
import { useMemo, useState } from "react";
import { FormattedMessage, useIntl, type IntlShape } from "react-intl";
import type {
  DashboardTimeRange,
  OperationalDashboardMetrics,
  OperationalMetric,
} from "../application/dashboard-types";
import {
  DashboardFilterToolbar,
  DashboardNavigation,
} from "../dashboard/dashboard-toolbar";
import { dashboardResizeWidgetConfig } from "./dashboard-resize-handle";
import { DashboardPurpose } from "./dashboard-purpose";
import { messages } from "../messages";
import {
  DashboardMetricUnavailable,
  DashboardSummaryList,
  DashboardWidgetContent,
  type DashboardSummaryIndicator,
  NodeStatusCard,
  PodCapacityCard,
  UtilizationCard,
} from "./dashboard-widget";
import "./dashboard-widget.css";
import "./provisioning-dashboard-page.css";
import { TrendSparklineChart } from "../dashboard/trend-sparkline-chart";
import { parseProvisionDurationStats } from "../dashboard/provision-time-data";
import { getMetricTrendChange } from "../dashboard/metric-trend-change";
import { getUtilizationPercentage } from "../dashboard/utilization-chart";
import { isDisplayableOperationalMetricValue } from "../dashboard/operational-metric-display";
import {
  getActiveWidgetTypes,
  sanitizeDashboardTemplate,
  stripRemovedWidgetTypes,
} from "../dashboard/dashboard-layout-persistence";

const STORAGE_KEY = "hypershell.provisioning-dashboard.layout.v1";
const columns: Record<Variants, number> = { xl: 4, lg: 4, md: 4, sm: 1 };

function utilizationSummaryValue(metric: OperationalMetric | undefined) {
  if (
    !metric?.total ||
    !isDisplayableOperationalMetricValue(metric.value) ||
    !isDisplayableOperationalMetricValue(metric.total) ||
    Number(metric.total) <= 0
  ) {
    return "-";
  }

  return `${String(getUtilizationPercentage(metric.value, metric.total))}%`;
}

function utilizationSummaryIndicator(
  metric: OperationalMetric | undefined,
): DashboardSummaryIndicator | undefined {
  const trendChange = metric ? getMetricTrendChange(metric) : undefined;
  if (!trendChange) return undefined;

  const isIncrease = trendChange.direction === "increase";
  return {
    content: isIncrease ? (
      <TrendUpIcon aria-hidden />
    ) : (
      <TrendDownIcon aria-hidden />
    ),
    tone: isIncrease ? "negative" : "positive",
  };
}

function getDesktopLayout(intl: IntlShape) {
  return [
    {
      i: "system-summary#1",
      widgetType: "system-summary",
      x: 0,
      y: 0,
      w: 1,
      h: 8,
      minH: 5,
      title: intl.formatMessage(messages.systemSummaryWidget),
    },
    {
      i: "memory#1",
      widgetType: "memory",
      x: 1,
      y: 0,
      w: 1,
      h: 5,
      title: intl.formatMessage(messages.widgetMemory),
    },
    {
      i: "cpu#1",
      widgetType: "cpu",
      x: 2,
      y: 0,
      w: 1,
      h: 5,
      title: intl.formatMessage(messages.widgetCpu),
    },
    {
      i: "nodes#1",
      widgetType: "nodes",
      x: 3,
      y: 0,
      w: 1,
      h: 5,
      title: intl.formatMessage(messages.nodes),
    },
    {
      i: "pods#1",
      widgetType: "pods",
      x: 1,
      y: 5,
      w: 1,
      h: 5,
      title: intl.formatMessage(messages.widgetPods),
    },
    {
      i: "provision-time#1",
      widgetType: "provision-time",
      x: 2,
      y: 5,
      w: 1,
      h: 5,
      title: intl.formatMessage(messages.provisioningProvisionTimeWidget),
    },
    {
      i: "provision-reliability#1",
      widgetType: "provision-reliability",
      x: 3,
      y: 5,
      w: 1,
      h: 5,
      title: intl.formatMessage(
        messages.provisioningProvisionReliabilityWidget,
      ),
    },
  ] as const;
}
function createDefaultLayout(intl: IntlShape): ExtendedTemplateConfig {
  const desktop = getDesktopLayout(intl);
  let stackedY = 0;
  const stacked = desktop.map((item) => {
    const positioned = { ...item, x: 0, y: stackedY, w: 1 };
    stackedY += item.h;
    return positioned;
  });

  return {
    xl: [...desktop],
    lg: [...desktop],
    md: [...desktop],
    sm: stacked,
  };
}

function getAddedWidgetTypes(
  currentTemplate: ExtendedTemplateConfig,
  nextTemplate: ExtendedTemplateConfig,
): string[] {
  const currentTypes = new Set(getActiveWidgetTypes(currentTemplate));

  return getActiveWidgetTypes(nextTemplate).filter(
    (type) => !currentTypes.has(type),
  );
}

function readLayout(intl: IntlShape): ExtendedTemplateConfig {
  if (typeof window === "undefined") return createDefaultLayout(intl);
  try {
    const saved = window.localStorage.getItem(STORAGE_KEY);
    return saved
      ? (JSON.parse(saved) as ExtendedTemplateConfig)
      : createDefaultLayout(intl);
  } catch {
    return createDefaultLayout(intl);
  }
}
function createWidgetMapping(
  metrics: OperationalDashboardMetrics,
  intl: IntlShape,
  timeRange: DashboardTimeRange,
): WidgetMapping {
  const byId = new Map(metrics.metrics.map((metric) => [metric.id, metric]));
  const metric = (id: string) => byId.get(id);
  const defaults = { h: 5, w: 1, minH: 2, maxH: 8 };
  const unavailable = (
    <DashboardWidgetContent>
      <DashboardMetricUnavailable />
    </DashboardWidgetContent>
  );
  const trendCaption = intl.formatMessage(messages.trendLastDays, {
    days: timeRange,
  });
  return {
    "system-summary": {
      defaults: { h: 8, w: 1, minH: 5, maxH: 12 },
      config: {
        icon: <TachometerAltIcon />,
        title: intl.formatMessage(messages.systemSummaryWidget),
        wrapperProps: { className: "hypershell-provisioning-widget" },
      },
      renderWidget: () => (
        <DashboardWidgetContent>
          <DashboardSummaryList
            items={[
              {
                key: "managed-clusters",
                label: (
                  <FormattedMessage {...messages.provisioningManagedClusters} />
                ),
                value: metric("managed-clusters")?.value ?? "-",
              },
              {
                key: "nodes",
                label: <FormattedMessage {...messages.nodes} />,
                value: metric("nodes")?.value ?? "-",
              },
              {
                key: "pods",
                label: <FormattedMessage {...messages.widgetPods} />,
                value: metric("pods")?.value ?? "-",
              },
              {
                key: "gateways",
                label: <FormattedMessage {...messages.gateways} />,
                value: metric("provisioned-gateways")?.value ?? "-",
              },
              {
                indicator: utilizationSummaryIndicator(metric("memory")),
                key: "memory-used",
                label: (
                  <FormattedMessage {...messages.provisioningMemoryUsed} />
                ),
                value: utilizationSummaryValue(metric("memory")),
              },
              {
                indicator: utilizationSummaryIndicator(metric("cpu")),
                key: "cpu-used",
                label: <FormattedMessage {...messages.provisioningCpuUsed} />,
                value: utilizationSummaryValue(metric("cpu")),
              },
            ]}
          />
          <small>
            <FormattedMessage {...messages.provisioningScopeNote} />
          </small>
        </DashboardWidgetContent>
      ),
    },
    memory: {
      defaults,
      config: {
        icon: <MemoryIcon />,
        title: intl.formatMessage(messages.widgetMemory),
        wrapperProps: { className: "hypershell-provisioning-widget" },
      },
      renderWidget: () => {
        const value = metric("memory");
        return value ? (
          <UtilizationCard
            color="#0066cc"
            metric={value}
            trendCaption={trendCaption}
          />
        ) : (
          unavailable
        );
      },
    },
    cpu: {
      defaults,
      config: {
        icon: <MicrochipIcon />,
        title: intl.formatMessage(messages.widgetCpu),
        wrapperProps: { className: "hypershell-provisioning-widget" },
      },
      renderWidget: () => {
        const value = metric("cpu");
        return value ? (
          <UtilizationCard
            color="#6753ac"
            metric={value}
            trendCaption={trendCaption}
          />
        ) : (
          unavailable
        );
      },
    },
    nodes: {
      defaults,
      config: {
        icon: <ServerIcon />,
        title: intl.formatMessage(messages.nodes),
        wrapperProps: { className: "hypershell-provisioning-widget" },
      },
      renderWidget: () => {
        const value = metric("nodes");
        if (!value) return unavailable;
        const healthy = value.status?.healthy ?? 0;
        const failed = value.status?.failed ?? 0;
        return (
          <NodeStatusCard
            metric={value}
            title={intl.formatMessage(messages.nodeStatusCardTitle, {
              ready: healthy,
              total: healthy + failed,
            })}
          />
        );
      },
    },
    pods: {
      defaults,
      config: {
        icon: <CubesIcon />,
        title: intl.formatMessage(messages.widgetPods),
        wrapperProps: { className: "hypershell-provisioning-widget" },
      },
      renderWidget: () => {
        const value = metric("pods");
        return value ? (
          <PodCapacityCard metric={value} showTrend={false} />
        ) : (
          unavailable
        );
      },
    },
    "provision-time": {
      defaults,
      config: {
        icon: <HourglassHalfIcon />,
        title: intl.formatMessage(messages.provisioningProvisionTimeWidget),
        wrapperProps: { className: "hypershell-provisioning-widget" },
      },
      renderWidget: () => {
        const value = metric("provision-time");
        return <ProvisionTimeWidget metric={value} />;
      },
    },
    "provision-reliability": {
      defaults,
      config: {
        icon: <ChartLineIcon />,
        title: intl.formatMessage(
          messages.provisioningProvisionReliabilityWidget,
        ),
        wrapperProps: { className: "hypershell-provisioning-widget" },
      },
      renderWidget: () => {
        const value = metric("provision-reliability");
        return value ? (
          <ProvisionReliabilityWidget metric={value} timeRange={timeRange} />
        ) : (
          unavailable
        );
      },
    },
  };
}
function ProvisionTimeWidget({
  metric,
}: Readonly<{ metric: OperationalMetric | undefined }>) {
  const intl = useIntl();
  const stats = metric ? parseProvisionDurationStats(metric) : undefined;
  if (!stats) {
    return (
      <DashboardWidgetContent>
        <DashboardMetricUnavailable />
      </DashboardWidgetContent>
    );
  }

  const formatDuration = (seconds: number) =>
    intl.formatMessage(messages.durationMinutesSeconds, {
      minutes: Math.floor(seconds / 60),
      seconds: String(Math.round(seconds % 60)).padStart(2, "0"),
    });
  return (
    <DashboardWidgetContent>
      <div className="hypershell-provisioning-trend-widget">
        <div>
          <span>
            <FormattedMessage {...messages.provisionTimeAverage} />
          </span>
          <strong>{formatDuration(stats.meanSeconds)}</strong>
        </div>
        <div>
          <span>
            <FormattedMessage {...messages.provisionTimeP50Label} />
          </span>
          <strong>{formatDuration(stats.p50Seconds)}</strong>
        </div>
        <div>
          <span>
            <FormattedMessage {...messages.provisionTimeP95Label} />
          </span>
          <strong>{formatDuration(stats.p95Seconds)}</strong>
        </div>
      </div>
    </DashboardWidgetContent>
  );
}
function ProvisionReliabilityWidget({
  metric,
  timeRange,
}: Readonly<{
  metric: OperationalMetric;
  timeRange: DashboardTimeRange;
}>) {
  const intl = useIntl();
  const outcomes = metric.provisionOutcomes;
  const trendChange = getMetricTrendChange(metric);
  const successRateChange = trendChange
    ? intl.formatMessage(
        trendChange.direction === "increase"
          ? messages.percentChangeIncrease
          : messages.percentChangeDecrease,
        { percent: trendChange.percent },
      )
    : "-";

  return (
    <DashboardWidgetContent>
      <div className="hypershell-provisioning-trend-widget">
        <DashboardSummaryList
          ariaLabel={intl.formatMessage(
            messages.provisioningSuccessRateWidgetAriaLabel,
          )}
          items={[
            {
              key: "success-rate",
              label: <FormattedMessage {...messages.provisionSuccessRate24h} />,
              value: `${outcomes?.successRatePercent ?? metric.value}%`,
            },
            {
              key: "successful",
              label: <FormattedMessage {...messages.provisioningSuccessful} />,
              value: outcomes?.successCount24h ?? "-",
            },
            {
              key: "failed",
              label: <FormattedMessage {...messages.gatewayStatusFailed} />,
              value: outcomes?.failureCount24h ?? "-",
            },
            {
              indicator: trendChange
                ? {
                    content:
                      trendChange.direction === "increase" ? (
                        <TrendUpIcon aria-hidden />
                      ) : (
                        <TrendDownIcon aria-hidden />
                      ),
                    tone:
                      trendChange.direction === "increase"
                        ? "positive"
                        : "negative",
                  }
                : undefined,
              key: "success-rate-change",
              label: (
                <FormattedMessage {...messages.provisioningSuccessRateChange} />
              ),
              value: successRateChange,
            },
          ]}
        />
        {metric.successRateTrend ? (
          <TrendSparklineChart
            caption={intl.formatMessage(
              messages.provisionReliabilityDailySuccessRateCaption,
              { days: timeRange },
            )}
            title={intl.formatMessage(messages.provisionReliabilitySuccessRate)}
            trend={metric.successRateTrend}
          />
        ) : null}
      </div>
    </DashboardWidgetContent>
  );
}
export interface ProvisioningDashboardPageProps {
  metrics: OperationalDashboardMetrics | undefined;
  timeRange: DashboardTimeRange;
  hasInitialLoadError?: boolean;
  isLoading?: boolean;
  isRefreshing?: boolean;
  onRefresh: () => void;
  onTimeRangeChange: (timeRange: DashboardTimeRange) => void;
}
export function ProvisioningDashboardPage({
  metrics,
  timeRange,
  hasInitialLoadError = false,
  isLoading = false,
  isRefreshing = false,
  onRefresh,
  onTimeRangeChange,
}: Readonly<ProvisioningDashboardPageProps>) {
  const intl = useIntl();
  const [layout, setLayout] = useState<ExtendedTemplateConfig>(() =>
    readLayout(intl),
  );
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [gridLayoutKey, setGridLayoutKey] = useState(0);
  const [droppingWidgetType, setDroppingWidgetType] = useState<
    string | undefined
  >();
  const mapping = useMemo(
    () => (metrics ? createWidgetMapping(metrics, intl, timeRange) : undefined),
    [intl, metrics, timeRange],
  );
  const activeWidgetTypes = useMemo(
    () => getActiveWidgetTypes(layout),
    [layout],
  );
  const handleTemplateChange = (next: ExtendedTemplateConfig) => {
    const addedTypes = getAddedWidgetTypes(layout, next);

    if (addedTypes.length > 0 && droppingWidgetType === undefined) {
      setGridLayoutKey((currentKey) => currentKey + 1);
      return;
    }

    if (
      droppingWidgetType !== undefined &&
      activeWidgetTypes.includes(droppingWidgetType)
    ) {
      setGridLayoutKey((currentKey) => currentKey + 1);
      return;
    }

    const sanitized = stripRemovedWidgetTypes(sanitizeDashboardTemplate(next));

    setLayout(sanitized);

    const sanitizedTypes = getActiveWidgetTypes(sanitized);
    if (
      mapping &&
      Object.keys(mapping).every((type) => sanitizedTypes.includes(type))
    ) {
      setDrawerOpen(false);
    }

    window.localStorage.setItem(STORAGE_KEY, JSON.stringify(sanitized));
  };
  const reset = () => {
    setLayout(createDefaultLayout(intl));
    setGridLayoutKey((currentKey) => currentKey + 1);
    window.localStorage.removeItem(STORAGE_KEY);
  };
  return (
    <>
      <PageSection
        data-page-region="header"
        hasBodyWrapper={false}
        variant="default"
      >
        <Content>
          <Title headingLevel="h1">
            <FormattedMessage {...messages.dashboardProvisioningTitle} />
          </Title>
          <Content component="p">
            <FormattedMessage {...messages.dashboardProvisioningDescription} />
          </Content>
        </Content>
      </PageSection>
      <PageSection data-page-region="body" isFilled variant="secondary">
        <DashboardNavigation
          active="provisioning"
          isRefreshing={isRefreshing}
          onRefresh={onRefresh}
        />
        <DashboardPurpose
          heading={
            <FormattedMessage
              {...messages.dashboardProvisioningPurposeHeading}
            />
          }
        >
          <FormattedMessage {...messages.dashboardProvisioningPurpose} />
        </DashboardPurpose>
        <DashboardFilterToolbar
          onAddWidgets={() => {
            setDrawerOpen(!drawerOpen);
          }}
          onResetToDefault={reset}
          onTimeRangeChange={onTimeRangeChange}
          timeRange={timeRange}
        />
        {!metrics && isLoading ? (
          <Spinner aria-label={intl.formatMessage(messages.loading)} />
        ) : null}
        {!metrics && hasInitialLoadError ? (
          <Alert
            className="hypershell-dashboard-data-alert"
            title={intl.formatMessage(messages.loadErrorTitle)}
            variant="danger"
          >
            <FormattedMessage {...messages.loadErrorBody} />
          </Alert>
        ) : null}
        {mapping ? (
          <WidgetDrawer
            currentlyUsedWidgets={activeWidgetTypes}
            isOpen={drawerOpen}
            onOpenChange={setDrawerOpen}
            onWidgetDragEnd={() => {
              setDroppingWidgetType(undefined);
            }}
            onWidgetDragStart={setDroppingWidgetType}
            widgetMapping={mapping}
          >
            <div className="hypershell-dashboard-widget-layout">
              <GridLayout
                key={gridLayoutKey}
                columns={columns}
                droppingWidgetType={droppingWidgetType}
                onDrawerExpandChange={setDrawerOpen}
                onTemplateChange={handleTemplateChange}
                resizeWidgetConfig={dashboardResizeWidgetConfig}
                template={layout}
                widgetMapping={mapping}
              />
            </div>
          </WidgetDrawer>
        ) : null}
      </PageSection>
    </>
  );
}
