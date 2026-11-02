import {
  Alert,
  AlertActionLink,
  Bullseye,
  Content,
  PageSection,
  Spinner,
  Title,
} from "@patternfly/react-core";
import {
  ArrowRightIcon,
  ChartLineIcon,
  UsersIcon,
} from "@patternfly/react-icons";
import {
  GridLayout,
  WidgetDrawer,
  type ExtendedTemplateConfig,
  type Variants,
  type WidgetMapping,
} from "@patternfly/widgetized-dashboard";
import "@patternfly/widgetized-dashboard/dist/esm/styles.css";
import {
  Fragment,
  useEffect,
  useState,
  type CSSProperties,
  type ReactNode,
} from "react";
import { FormattedMessage, useIntl, type IntlShape } from "react-intl";

import type {
  DashboardTimeRange,
  OperationalDashboardMetrics,
  OperationalMetric,
  OperationalMetricTrend,
} from "../application/dashboard-types";
import {
  DashboardFilterToolbar,
  DashboardNavigation,
} from "../dashboard/dashboard-toolbar";
import {
  getActiveWidgetTypes,
  sanitizeDashboardTemplate,
  stripRemovedWidgetTypes,
} from "../dashboard/dashboard-layout-persistence";
import { TrendSparklineChart } from "../dashboard/trend-sparkline-chart";
import { AdoptionConversionChart } from "../dashboard/adoption-conversion-chart";
import { messages } from "../messages";
import { useDashboardUi } from "../dashboard-ui-provider";
import { dashboardResizeWidgetConfig } from "./dashboard-resize-handle";
import { DashboardPurpose } from "./dashboard-purpose";
import {
  DashboardMetricUnavailable,
  DashboardSummaryList,
  DashboardWidgetContent,
} from "./dashboard-widget";
import { useGetAdoptionMetricsData } from "./get-adoption-metrics-data";
import "./dashboard-widget.css";

const STORAGE_KEY = "hypershell.adoption-dashboard.layout.v2";
const columns: Record<Variants, number> = { xl: 3, lg: 3, md: 3, sm: 1 };

function UnavailableMetricValue() {
  return (
    <span className="hypershell-adoption-dashboard__unavailable">
      <strong>
        <FormattedMessage {...messages.metricUnavailableTitle} />
      </strong>
      <small>
        <FormattedMessage {...messages.metricUnavailableBody} />
      </small>
    </span>
  );
}

function limitTrendToTimeRange(
  trend: OperationalMetricTrend | undefined,
  timeRange: DashboardTimeRange,
): OperationalMetricTrend | undefined {
  if (trend === undefined) {
    return undefined;
  }

  return { ...trend, points: trend.points.slice(-timeRange) };
}

function getDesktopLayout(intl: IntlShape) {
  return [
    {
      i: "adoption-summary#1",
      widgetType: "adoption-summary",
      x: 0,
      y: 0,
      w: 2,
      h: 5,
      minH: 4,
      title: intl.formatMessage(messages.adoptionWidgetSummary),
    },
    {
      i: "adoption-conversion#1",
      widgetType: "adoption-conversion",
      x: 2,
      y: 0,
      w: 1,
      h: 5,
      minH: 4,
      title: intl.formatMessage(messages.adoptionWidgetConversion),
    },
    {
      i: "adoption-trends#1",
      widgetType: "adoption-trends",
      x: 0,
      y: 5,
      w: 3,
      h: 6,
      minH: 4,
      title: intl.formatMessage(messages.adoptionTrendsWidget),
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

function AdoptionSummaryCard({
  metric,
  timeRange,
}: Readonly<{
  metric: OperationalMetric | undefined;
  timeRange: DashboardTimeRange;
}>) {
  const intl = useIntl();

  if (metric === undefined) {
    return <DashboardMetricUnavailable />;
  }

  const newRegistrations =
    timeRange === 7 ? metric.createdLast7Days : metric.createdLast30Days;

  return (
    <DashboardWidgetContent>
      <Title headingLevel="h3" size="lg">
        <FormattedMessage
          {...messages.adoptionRegisteredUsers}
          values={{ value: metric.value }}
        />
      </Title>
      <DashboardSummaryList
        ariaLabel={intl.formatMessage(messages.adoptionWidgetSummary)}
        items={[
          {
            key: "new-registrations",
            label: <FormattedMessage {...messages.adoptionNewRegistrations} />,
            value: newRegistrations ?? "-",
          },
          {
            key: "daily-active-users",
            label: <FormattedMessage {...messages.adoptionDailyActiveUsers} />,
            value: metric.dailyActiveUsers ?? "-",
          },
          {
            key: "weekly-active-users",
            label: <FormattedMessage {...messages.adoptionWeeklyActiveUsers} />,
            value: metric.uniqueLoginsLast7Days ?? "-",
          },
          {
            key: "monthly-active-users",
            label: (
              <FormattedMessage {...messages.adoptionMonthlyActiveUsers} />
            ),
            value: metric.uniqueLoginsLast30Days ?? "-",
          },
          {
            key: "inactive-users",
            label: <FormattedMessage {...messages.adoptionInactiveUsers} />,
            value: metric.inactiveUsersLast30Days ?? "-",
          },
          {
            key: "retention-7-days",
            label: <FormattedMessage {...messages.adoptionRetention} />,
            value: metric.retention7DaysPercent ?? "-",
          },
          {
            key: "retention-30-days",
            label: <FormattedMessage {...messages.adoptionRetention30Days} />,
            value: "-",
          },
        ]}
      />
    </DashboardWidgetContent>
  );
}

function AdoptionConversionCard({
  metric,
}: Readonly<{
  metric: OperationalMetric | undefined;
}>) {
  const intl = useIntl();

  if (metric === undefined) {
    return <DashboardMetricUnavailable />;
  }

  return (
    <DashboardWidgetContent bodyClassName="hypershell-dashboard-status-donut-card--compact">
      <Content className="hypershell-dashboard-status-donut-card">
        {metric.gatewayConversionPercent !== undefined ? (
          <AdoptionConversionChart metric={metric} />
        ) : (
          <Title headingLevel="h3" size="lg">
            <UnavailableMetricValue />
          </Title>
        )}
      </Content>
      <DashboardSummaryList
        ariaLabel={intl.formatMessage(messages.adoptionWidgetConversion)}
        items={[
          {
            key: "first-gateway-conversion",
            label: (
              <FormattedMessage {...messages.adoptionFirstGatewayConversion} />
            ),
            value: metric.usersWithGateway ?? "-",
          },
          {
            key: "first-sandbox-conversion",
            label: (
              <FormattedMessage {...messages.adoptionFirstSandboxConversion} />
            ),
            value: "-",
          },
          {
            key: "median-time-to-first-resource",
            label: (
              <FormattedMessage
                {...messages.adoptionMedianTimeToFirstResource}
              />
            ),
            value: "-",
          },
        ]}
      />
    </DashboardWidgetContent>
  );
}

function AdoptionTrendsCard({
  metric,
  timeRange,
}: Readonly<{
  metric: OperationalMetric | undefined;
  timeRange: DashboardTimeRange;
}>) {
  const intl = useIntl();

  if (metric === undefined) {
    return <DashboardMetricUnavailable />;
  }

  const createdTrend = limitTrendToTimeRange(metric.createdTrend, timeRange);
  const activeUsersTrend = limitTrendToTimeRange(metric.trend, timeRange);
  const trendCaption = intl.formatMessage(messages.adoptionTrendCaption, {
    days: timeRange,
  });

  return (
    <DashboardWidgetContent>
      <div className="hypershell-adoption-dashboard__trends">
        <Title headingLevel="h4" size="md">
          <FormattedMessage {...messages.adoptionRegistrationsTrendTitle} />
        </Title>
        {createdTrend ? (
          <TrendSparklineChart
            caption={trendCaption}
            title={intl.formatMessage(messages.adoptionRegistrationsTrendTitle)}
            trend={createdTrend}
          />
        ) : (
          <UnavailableMetricValue />
        )}
        <Title headingLevel="h4" size="md">
          <FormattedMessage {...messages.adoptionActiveUsersTrendTitle} />
        </Title>
        {activeUsersTrend ? (
          <TrendSparklineChart
            caption={trendCaption}
            title={intl.formatMessage(messages.adoptionActiveUsersTrendTitle)}
            trend={activeUsersTrend}
          />
        ) : (
          <UnavailableMetricValue />
        )}
      </div>
    </DashboardWidgetContent>
  );
}

export interface AdoptionDashboardPageProps {
  metrics?: OperationalDashboardMetrics;
  title?: string;
}

export function AdoptionDashboardPage({
  metrics,
  title,
}: Readonly<AdoptionDashboardPageProps>) {
  const intl = useIntl();
  const { isAdmin } = useDashboardUi();
  const [timeRange, setTimeRange] = useState<DashboardTimeRange>(7);
  const [currentTime, setCurrentTime] = useState(0);
  const [layout, setLayout] = useState<ExtendedTemplateConfig>(() =>
    readLayout(intl),
  );
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [droppingWidgetType, setDroppingWidgetType] = useState<string>();
  const [gridLayoutKey, setGridLayoutKey] = useState(0);
  const metricsQuery = useGetAdoptionMetricsData({
    enabled: metrics === undefined,
    timeRange,
  });
  const dashboardMetrics = metrics ?? metricsQuery.data;
  const metric = dashboardMetrics?.metrics.find(
    (candidate) => candidate.id === "adoption",
  );
  const adoptionWidgetConfig = (
    titleMessage: (typeof messages)[keyof typeof messages],
    icon: ReactNode,
  ) => {
    const title = intl.formatMessage(titleMessage);
    const systemLabel = intl.formatMessage(messages.summarySystem);

    return {
      config: {
        icon,
        title,
        wrapperProps: isAdmin
          ? {
              "aria-label": `${title} ${systemLabel}`,
              className:
                "hypershell-adoption-widget hypershell-dashboard-widget--system",
              style: {
                "--hypershell-system-label": JSON.stringify(systemLabel),
              } as CSSProperties,
            }
          : { className: "hypershell-adoption-widget" },
      },
    };
  };
  const activeWidgetTypes = getActiveWidgetTypes(layout);
  const mapping: WidgetMapping = {
    "adoption-summary": {
      ...adoptionWidgetConfig(messages.adoptionWidgetSummary, <UsersIcon />),
      defaults: { h: 5, w: 2, minH: 4, maxH: 8 },
      renderWidget: () => (
        <AdoptionSummaryCard metric={metric} timeRange={timeRange} />
      ),
    },
    "adoption-conversion": {
      ...adoptionWidgetConfig(
        messages.adoptionWidgetConversion,
        <ArrowRightIcon />,
      ),
      defaults: { h: 5, w: 1, minH: 4, maxH: 8 },
      renderWidget: () => <AdoptionConversionCard metric={metric} />,
    },
    "adoption-trends": {
      ...adoptionWidgetConfig(messages.adoptionTrendsWidget, <ChartLineIcon />),
      defaults: { h: 6, w: 3, minH: 4, maxH: 10 },
      renderWidget: () => (
        <AdoptionTrendsCard metric={metric} timeRange={timeRange} />
      ),
    },
  };

  const handleTemplateChange = (nextTemplate: ExtendedTemplateConfig) => {
    const addedTypes = getAddedWidgetTypes(layout, nextTemplate);

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

    const sanitizedTemplate = stripRemovedWidgetTypes(
      sanitizeDashboardTemplate(nextTemplate),
    );
    setLayout(sanitizedTemplate);

    const sanitizedTypes = getActiveWidgetTypes(sanitizedTemplate);
    if (Object.keys(mapping).every((type) => sanitizedTypes.includes(type))) {
      setDrawerOpen(false);
    }

    window.localStorage.setItem(STORAGE_KEY, JSON.stringify(sanitizedTemplate));
  };

  const reset = () => {
    setLayout(createDefaultLayout(intl));
    setGridLayoutKey((currentKey) => currentKey + 1);
    window.localStorage.removeItem(STORAGE_KEY);
  };
  const updatedMinutesAgo = dashboardMetrics?.lastSuccessfulRefresh
    ? Math.max(
        0,
        Math.floor(
          (currentTime - dashboardMetrics.lastSuccessfulRefresh.getTime()) /
            60000,
        ),
      )
    : undefined;

  useEffect(() => {
    const intervalId = window.setInterval(() => {
      setCurrentTime(Date.now());
    }, 60_000);

    return () => {
      window.clearInterval(intervalId);
    };
  }, []);

  return (
    <Fragment>
      <PageSection
        data-page-region="header"
        hasBodyWrapper={false}
        variant="default"
      >
        <Content>
          <Title headingLevel="h1">
            {title ?? intl.formatMessage(messages.dashboardNavAdoption)}
          </Title>
          <p>
            <FormattedMessage {...messages.adoptionDescription} />
          </p>
        </Content>
      </PageSection>
      <PageSection
        data-page-region="body"
        hasBodyWrapper={false}
        isFilled
        variant="secondary"
      >
        <DashboardNavigation
          active="adoption"
          isRefreshing={metrics === undefined && metricsQuery.isFetching}
          onRefresh={() => {
            if (metrics === undefined) {
              void metricsQuery.refetch();
            }
          }}
          updatedMinutesAgo={updatedMinutesAgo}
        />
        <DashboardPurpose
          heading={<FormattedMessage {...messages.adoptionPurposeHeading} />}
        >
          <FormattedMessage {...messages.adoptionPurpose} />
        </DashboardPurpose>
        <DashboardFilterToolbar
          onAddWidgets={() => {
            setDrawerOpen(!drawerOpen);
          }}
          onResetToDefault={() => {
            reset();
          }}
          onTimeRangeChange={setTimeRange}
          timeRange={timeRange}
        />
        {metricsQuery.isPending && metrics === undefined ? (
          <Bullseye>
            <Spinner
              aria-label={intl.formatMessage(messages.adoptionLoading)}
            />
          </Bullseye>
        ) : null}
        {metrics === undefined && metricsQuery.isError && !metricsQuery.data ? (
          <Alert
            title={intl.formatMessage(messages.adoptionLoadErrorTitle)}
            variant="danger"
          >
            <FormattedMessage {...messages.adoptionLoadErrorBody} />
          </Alert>
        ) : null}
        {(dashboardMetrics?.failedSources?.length ?? 0) > 0 ? (
          <Alert
            actionLinks={
              <AlertActionLink
                isDisabled={metricsQuery.isFetching}
                onClick={() => {
                  void metricsQuery.refetch();
                }}
              >
                <FormattedMessage
                  {...messages.partialLoadWarningRefreshAction}
                />
              </AlertActionLink>
            }
            title={intl.formatMessage(messages.partialLoadWarningTitle)}
            variant="warning"
          >
            <FormattedMessage {...messages.partialLoadWarningBody} />
          </Alert>
        ) : null}
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
      </PageSection>
    </Fragment>
  );
}
