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
  ChartLineIcon,
  ContainerNodeIcon,
  CubesIcon,
  ExclamationCircleIcon,
  HourglassHalfIcon,
  OutlinedClockIcon,
} from "@patternfly/react-icons";
import {
  GridLayout,
  WidgetDrawer,
  type ExtendedTemplateConfig,
  type Variants,
  type WidgetMapping,
} from "@patternfly/widgetized-dashboard";
import "@patternfly/widgetized-dashboard/dist/esm/styles.css";
import { Fragment, useEffect, useMemo, useRef, useState } from "react";
import { FormattedMessage, useIntl, type IntlShape } from "react-intl";

import type {
  DashboardTimeRange,
  UsageDashboardMetrics,
} from "../application/dashboard-types";
import type { DashboardProbe } from "../application/dashboard-probes";
import { noopDashboardProbePublisher } from "../application/dashboard-probes";
import {
  DashboardFilterToolbar,
  DashboardNavigation,
} from "../dashboard/dashboard-toolbar";
import {
  getActiveWidgetTypes,
  isValidSavedTemplate,
  sanitizeDashboardTemplate,
  stripRemovedWidgetTypes,
} from "../dashboard/dashboard-layout-persistence";
import {
  defaultUsageDashboardLayoutTemplate,
  localizeUsageDashboardLayoutTemplate,
  USAGE_DASHBOARD_COLUMN_COUNT,
  USAGE_DETAIL_WIDGET_HEIGHT,
  USAGE_GATEWAY_WIDGET_HEIGHT,
} from "../dashboard/usage-dashboard-layout-template";
import {
  UsageGatewayActivityCard,
  UsageSandboxLifetimeCard,
  UsageSandboxUtilizationCard,
  UsageTrendCard,
} from "../dashboard/usage-dashboard-widgets";
import { useDashboardUi } from "../dashboard-ui-provider";
import { messages } from "../messages";
import { DashboardPurpose } from "./dashboard-purpose";
import { dashboardResizeWidgetConfig } from "./dashboard-resize-handle";
import "./dashboard-widget.css";
import { useGetUsageMetricsData } from "./get-usage-metrics-data";

const baseTemplate = defaultUsageDashboardLayoutTemplate;

const LAYOUT_STORAGE_KEY = "hypershell.usage-dashboard.layout.v1";
const CUSTOM_COLUMNS: Record<Variants, number> = {
  xl: USAGE_DASHBOARD_COLUMN_COUNT,
  lg: USAGE_DASHBOARD_COLUMN_COUNT,
  md: USAGE_DASHBOARD_COLUMN_COUNT,
  sm: 1,
};

const METRIC_WIDGET_DEFAULTS = { h: 3, maxH: 6, minH: 2, w: 1 };

function getAddedWidgetTypes(
  currentTemplate: ExtendedTemplateConfig,
  nextTemplate: ExtendedTemplateConfig,
): string[] {
  const currentTypes = new Set(getActiveWidgetTypes(currentTemplate));

  return getActiveWidgetTypes(nextTemplate).filter(
    (type) => !currentTypes.has(type),
  );
}

function readSavedTemplate(
  localizedBaseTemplate: ExtendedTemplateConfig,
  intl: IntlShape,
): { invalid: boolean; template: ExtendedTemplateConfig } {
  if (typeof window === "undefined") {
    return { invalid: false, template: localizedBaseTemplate };
  }

  try {
    const rawTemplate = window.localStorage.getItem(LAYOUT_STORAGE_KEY);
    if (!rawTemplate) {
      return { invalid: false, template: localizedBaseTemplate };
    }

    const parsed = JSON.parse(rawTemplate) as ExtendedTemplateConfig;
    if (!isValidSavedTemplate(parsed, localizedBaseTemplate)) {
      return { invalid: true, template: localizedBaseTemplate };
    }

    return {
      invalid: false,
      template: localizeUsageDashboardLayoutTemplate(
        stripRemovedWidgetTypes(parsed),
        intl,
      ),
    };
  } catch {
    return { invalid: true, template: localizedBaseTemplate };
  }
}

function layoutProbe(
  correlationId: string,
  name: DashboardProbe["name"],
  outcome: DashboardProbe["fields"]["outcome"],
): DashboardProbe {
  return Object.freeze({
    context: Object.freeze({ correlationId }),
    fields: Object.freeze({
      action: "persist-layout-template",
      outcome,
    }),
    name,
    occurredAt: new Date().toISOString(),
    schemaVersion: 1,
  });
}

function createWidgetMapping(
  metrics: UsageDashboardMetrics,
  intl: IntlShape,
  isAdmin: boolean,
): WidgetMapping {
  const metricById = new Map(
    metrics.metrics.map((metric) => [metric.id, metric]),
  );
  const widgetClassName = isAdmin
    ? "hypershell-usage-widget hypershell-usage-widget--system"
    : "hypershell-usage-widget";

  const widgetConfig = (
    titleMessage: (typeof messages)[keyof typeof messages],
    icon: React.ReactNode,
  ) => ({
    config: {
      icon,
      title: intl.formatMessage(titleMessage),
      wrapperProps: isAdmin
        ? {
            "aria-label": `${intl.formatMessage(titleMessage)} ${intl.formatMessage(
              messages.summarySystem,
            )}`,
            className: widgetClassName,
            style: {
              "--hypershell-system-label": JSON.stringify(
                intl.formatMessage(messages.summarySystem),
              ),
            } as React.CSSProperties,
          }
        : { className: widgetClassName },
    },
  });

  return {
    "gateway-request-volume": {
      ...widgetConfig(
        messages.usageWidgetGatewayRequestVolume,
        <ChartLineIcon />,
      ),
      defaults: {
        h: USAGE_GATEWAY_WIDGET_HEIGHT,
        maxH: USAGE_GATEWAY_WIDGET_HEIGHT + 3,
        minH: METRIC_WIDGET_DEFAULTS.minH,
        w: 1,
      },
      renderWidget: () => (
        <UsageTrendCard
          metric={metricById.get("gateway-request-volume")}
          title={intl.formatMessage(messages.usageWidgetGatewayRequestVolume)}
        />
      ),
    },
    "gateway-error-rate": {
      ...widgetConfig(
        messages.usageWidgetGatewayErrorRate,
        <ExclamationCircleIcon />,
      ),
      defaults: {
        h: USAGE_GATEWAY_WIDGET_HEIGHT,
        maxH: USAGE_GATEWAY_WIDGET_HEIGHT + 3,
        minH: METRIC_WIDGET_DEFAULTS.minH,
        w: 1,
      },
      renderWidget: () => (
        <UsageTrendCard
          metric={metricById.get("gateway-error-rate")}
          title={intl.formatMessage(messages.usageWidgetGatewayErrorRate)}
        />
      ),
    },
    "active-gateways": {
      ...widgetConfig(
        messages.usageWidgetActiveGateways,
        <ContainerNodeIcon />,
      ),
      defaults: {
        h: USAGE_GATEWAY_WIDGET_HEIGHT,
        maxH: USAGE_GATEWAY_WIDGET_HEIGHT + 3,
        minH: METRIC_WIDGET_DEFAULTS.minH,
        w: 1,
      },
      renderWidget: () => (
        <UsageGatewayActivityCard metric={metricById.get("active-gateways")} />
      ),
    },
    "unused-gateways": {
      ...widgetConfig(
        messages.usageWidgetUnusedGateways,
        <HourglassHalfIcon />,
      ),
      defaults: {
        h: USAGE_GATEWAY_WIDGET_HEIGHT,
        maxH: USAGE_GATEWAY_WIDGET_HEIGHT + 3,
        minH: METRIC_WIDGET_DEFAULTS.minH,
        w: 1,
      },
      renderWidget: () => (
        <UsageGatewayActivityCard metric={metricById.get("unused-gateways")} />
      ),
    },
    "sandbox-utilization": {
      ...widgetConfig(messages.usageWidgetSandboxUtilization, <CubesIcon />),
      defaults: {
        h: USAGE_DETAIL_WIDGET_HEIGHT,
        maxH: USAGE_DETAIL_WIDGET_HEIGHT + 3,
        minH: METRIC_WIDGET_DEFAULTS.minH,
        w: 2,
      },
      renderWidget: () => (
        <UsageSandboxUtilizationCard
          metric={metricById.get("sandbox-utilization")}
        />
      ),
    },
    "sandbox-lifetime": {
      ...widgetConfig(
        messages.usageWidgetSandboxLifetime,
        <OutlinedClockIcon />,
      ),
      defaults: {
        h: USAGE_DETAIL_WIDGET_HEIGHT,
        maxH: USAGE_DETAIL_WIDGET_HEIGHT + 3,
        minH: METRIC_WIDGET_DEFAULTS.minH,
        w: 2,
      },
      renderWidget: () => (
        <UsageSandboxLifetimeCard metric={metricById.get("sandbox-lifetime")} />
      ),
    },
  };
}

function hasPartialTrendData(metrics: UsageDashboardMetrics): boolean {
  return metrics.metrics.some(
    (metric) =>
      metric.trend?.isPartial === true ||
      metric.trend?.points.some((point) => point.value === null) === true,
  );
}

export interface UsageDashboardPageProps {
  metrics?: UsageDashboardMetrics;
  onTimeRangeChange?: (timeRange: DashboardTimeRange) => void;
  timeRange?: DashboardTimeRange;
  title?: string;
}

export function UsageDashboardPage({
  metrics,
  onTimeRangeChange,
  timeRange,
  title,
}: Readonly<UsageDashboardPageProps>) {
  const intl = useIntl();
  const { isAdmin = false, probes = noopDashboardProbePublisher } =
    useDashboardUi();
  const pageTitle = title ?? intl.formatMessage(messages.usageTitle);
  const purposeText = `${intl.formatMessage(messages.usagePurpose)} ${intl.formatMessage(
    isAdmin
      ? messages.usagePurposeSystemScope
      : messages.usagePurposeAuthorizedScope,
  )}`;
  const [internalTimeRange, setInternalTimeRange] =
    useState<DashboardTimeRange>(metrics?.timeRange ?? 7);
  const selectedTimeRange = timeRange ?? internalTimeRange;
  const [currentTime, setCurrentTime] = useState(0);
  const metricsQuery = useGetUsageMetricsData({
    enabled: metrics === undefined,
    timeRange: selectedTimeRange,
  });
  const dashboardMetrics = metrics ?? metricsQuery.data;
  const updatedMinutesAgo = dashboardMetrics?.lastSuccessfulRefresh
    ? Math.max(
        0,
        Math.floor(
          (currentTime - dashboardMetrics.lastSuccessfulRefresh.getTime()) /
            60000,
        ),
      )
    : undefined;
  const showTotalInitialLoadError =
    metrics === undefined && metricsQuery.isError && !metricsQuery.data;
  const showPartialSourceWarning =
    metrics === undefined && (dashboardMetrics?.failedSources?.length ?? 0) > 0;
  const showPartialDataNotice =
    dashboardMetrics !== undefined && hasPartialTrendData(dashboardMetrics);
  const showRefreshError =
    metrics === undefined &&
    metricsQuery.isError &&
    Boolean(metricsQuery.data) &&
    !metricsQuery.isFetching;
  const localizedBaseTemplate = useMemo(
    () => localizeUsageDashboardLayoutTemplate(baseTemplate, intl),
    [intl],
  );
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [gridLayoutKey, setGridLayoutKey] = useState(0);
  const [droppingWidgetType, setDroppingWidgetType] = useState<
    string | undefined
  >();

  const savedTemplateResult = useMemo(
    () => readSavedTemplate(localizedBaseTemplate, intl),
    [intl, localizedBaseTemplate],
  );

  const invalidTemplateProbePublishedRef = useRef(false);

  useEffect(() => {
    const updateCurrentTime = () => {
      setCurrentTime(Date.now());
    };
    const timeoutId = window.setTimeout(updateCurrentTime, 0);
    const intervalId = window.setInterval(updateCurrentTime, 60000);
    return () => {
      window.clearTimeout(timeoutId);
      window.clearInterval(intervalId);
    };
  }, []);

  useEffect(() => {
    if (
      !savedTemplateResult.invalid ||
      invalidTemplateProbePublishedRef.current
    ) {
      return;
    }

    invalidTemplateProbePublishedRef.current = true;
    probes.publish(
      layoutProbe(
        crypto.randomUUID(),
        "dashboard.layout.template.invalid",
        "failed",
      ),
    );
  }, [probes, savedTemplateResult.invalid]);

  const [dashboardTemplate, setDashboardTemplate] =
    useState<ExtendedTemplateConfig>(savedTemplateResult.template);
  const displayTemplate = useMemo(
    () => localizeUsageDashboardLayoutTemplate(dashboardTemplate, intl),
    [dashboardTemplate, intl],
  );
  const activeWidgetTypes = useMemo(
    () => getActiveWidgetTypes(displayTemplate),
    [displayTemplate],
  );
  const widgetMapping = useMemo(
    () =>
      dashboardMetrics
        ? createWidgetMapping(dashboardMetrics, intl, isAdmin)
        : undefined,
    [dashboardMetrics, intl, isAdmin],
  );

  const handleTimeRangeChange = (nextTimeRange: DashboardTimeRange) => {
    if (onTimeRangeChange) {
      onTimeRangeChange(nextTimeRange);
    } else {
      setInternalTimeRange(nextTimeRange);
    }
  };

  const handleTemplateChange = (nextTemplate: ExtendedTemplateConfig) => {
    const addedTypes = getAddedWidgetTypes(dashboardTemplate, nextTemplate);

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

    const sanitized = stripRemovedWidgetTypes(
      sanitizeDashboardTemplate(nextTemplate),
    );
    const correlationId = crypto.randomUUID();

    setDashboardTemplate(sanitized);

    const sanitizedTypes = getActiveWidgetTypes(sanitized);
    if (
      widgetMapping &&
      Object.keys(widgetMapping).every((type) => sanitizedTypes.includes(type))
    ) {
      setDrawerOpen(false);
    }

    if (typeof window === "undefined") {
      return;
    }

    try {
      window.localStorage.setItem(
        LAYOUT_STORAGE_KEY,
        JSON.stringify(sanitized),
      );
    } catch {
      probes.publish(
        layoutProbe(
          correlationId,
          "dashboard.layout.template.persistence-failed",
          "failed",
        ),
      );
    }
  };

  const handleResetToDefault = () => {
    const defaultTemplate = localizeUsageDashboardLayoutTemplate(
      baseTemplate,
      intl,
    );
    const correlationId = crypto.randomUUID();

    setDashboardTemplate(defaultTemplate);
    setDrawerOpen(false);
    setGridLayoutKey((currentKey) => currentKey + 1);

    if (typeof window === "undefined") {
      return;
    }

    try {
      window.localStorage.setItem(
        LAYOUT_STORAGE_KEY,
        JSON.stringify(defaultTemplate),
      );
    } catch {
      probes.publish(
        layoutProbe(
          correlationId,
          "dashboard.layout.template.persistence-failed",
          "failed",
        ),
      );
    }
  };

  return (
    <Fragment>
      <PageSection
        data-page-region="header"
        hasBodyWrapper={false}
        variant="default"
      >
        <Content>
          <Title headingLevel="h1">{pageTitle}</Title>
          <p>
            <FormattedMessage {...messages.usageDescription} />
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
          active="usage"
          isRefreshing={metrics === undefined && metricsQuery.isFetching}
          onRefresh={() => {
            if (metrics === undefined) {
              void metricsQuery.refetch();
            }
          }}
          updatedMinutesAgo={updatedMinutesAgo}
        />
        <DashboardPurpose
          heading={<FormattedMessage {...messages.usagePurposeHeading} />}
        >
          {purposeText}
        </DashboardPurpose>
        <DashboardFilterToolbar
          onAddWidgets={() => {
            setDrawerOpen(!drawerOpen);
          }}
          onResetToDefault={handleResetToDefault}
          onTimeRangeChange={handleTimeRangeChange}
          timeRange={selectedTimeRange}
        />
        {metricsQuery.isPending && metrics === undefined ? (
          <Bullseye>
            <Spinner aria-label={intl.formatMessage(messages.usageLoading)} />
          </Bullseye>
        ) : null}
        {showTotalInitialLoadError ? (
          <Alert
            title={intl.formatMessage(messages.usageLoadErrorTitle)}
            variant="danger"
          >
            <FormattedMessage {...messages.usageLoadErrorBody} />
          </Alert>
        ) : null}
        {showPartialDataNotice ? (
          <Alert
            title={intl.formatMessage(messages.usagePartialDataTitle)}
            variant="info"
          >
            <FormattedMessage {...messages.usagePartialDataBody} />
          </Alert>
        ) : null}
        {showPartialSourceWarning ? (
          <Alert
            actionLinks={
              <AlertActionLink
                isDisabled={metricsQuery.isFetching}
                onClick={() => {
                  void metricsQuery.refetch();
                }}
              >
                {intl.formatMessage(messages.partialLoadWarningRefreshAction)}
              </AlertActionLink>
            }
            title={intl.formatMessage(messages.partialLoadWarningTitle)}
            variant="warning"
          >
            <FormattedMessage {...messages.partialLoadWarningBody} />
          </Alert>
        ) : null}
        {showRefreshError ? (
          <Alert
            title={intl.formatMessage(messages.refreshErrorTitle)}
            variant="warning"
          >
            <FormattedMessage {...messages.refreshErrorBody} />
          </Alert>
        ) : null}
        {widgetMapping ? (
          <WidgetDrawer
            currentlyUsedWidgets={activeWidgetTypes}
            isOpen={drawerOpen}
            onOpenChange={setDrawerOpen}
            onWidgetDragEnd={() => {
              setDroppingWidgetType(undefined);
            }}
            onWidgetDragStart={setDroppingWidgetType}
            widgetMapping={widgetMapping}
          >
            <GridLayout
              key={gridLayoutKey}
              columns={CUSTOM_COLUMNS}
              droppingWidgetType={droppingWidgetType}
              onDrawerExpandChange={setDrawerOpen}
              onTemplateChange={handleTemplateChange}
              resizeWidgetConfig={dashboardResizeWidgetConfig}
              template={displayTemplate}
              widgetMapping={widgetMapping}
            />
          </WidgetDrawer>
        ) : null}
      </PageSection>
    </Fragment>
  );
}
