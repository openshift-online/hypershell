import {
  Alert,
  AlertActionLink,
  Bullseye,
  Button,
  Content,
  Flex,
  FlexItem,
  PageSection,
  PageSectionTypes,
  Spinner,
  Timestamp,
  TimestampFormat,
  Title,
} from "@patternfly/react-core";
import {
  ChartLineIcon,
  ExclamationCircleIcon,
  OutlinedClockIcon,
  SyncAltIcon,
  TachometerAltIcon,
} from "@patternfly/react-icons";
import {
  AddWidgetsButton,
  GridLayout,
  WidgetDrawer,
  type ExtendedTemplateConfig,
  type Variants,
  type WidgetMapping,
} from "@patternfly/widgetized-dashboard";
import "@patternfly/widgetized-dashboard/dist/esm/styles.css";
import { Fragment, useEffect, useMemo, useRef, useState } from "react";
import { FormattedMessage, useIntl, type IntlShape } from "react-intl";

import type { OperationalDashboardMetrics } from "../application/dashboard-types";
import type { DashboardProbe } from "../application/dashboard-probes";
import { noopDashboardProbePublisher } from "../application/dashboard-probes";
import { DashboardSecondaryNav } from "../dashboard/dashboard-secondary-nav";
import {
  getActiveWidgetTypes,
  isValidSavedTemplate,
  sanitizeDashboardTemplate,
  stripRemovedWidgetTypes,
} from "../dashboard/dashboard-layout-persistence";
import {
  defaultReliabilityDashboardLayoutTemplate,
  localizeReliabilityDashboardLayoutTemplate,
  RELIABILITY_DASHBOARD_COLUMN_COUNT,
  RELIABILITY_SUMMARY_WIDGET_HEIGHT,
  RELIABILITY_TREND_WIDGET_HEIGHT,
} from "../dashboard/reliability-dashboard-layout-template";
import {
  ApiReliabilityTrendCard,
  ReliabilitySummaryCard,
} from "../dashboard/reliability-dashboard-widgets";
import { useDashboardUi } from "../dashboard-ui-provider";
import { messages } from "../messages";
import { ResourceRefreshButton } from "../shared/resource-refresh-button";
import { dashboardResizeWidgetConfig } from "./dashboard-resize-handle";
import "./dashboard-widget.css";
import { useGetReliabilityMetricsData } from "./get-reliability-metrics-data";

const baseTemplate = defaultReliabilityDashboardLayoutTemplate;

const LAYOUT_STORAGE_KEY = "hypershell.reliability-dashboard.layout.v2";
const CUSTOM_COLUMNS: Record<Variants, number> = {
  xl: RELIABILITY_DASHBOARD_COLUMN_COUNT,
  lg: RELIABILITY_DASHBOARD_COLUMN_COUNT,
  md: RELIABILITY_DASHBOARD_COLUMN_COUNT,
  sm: 1,
};

const METRIC_WIDGET_DEFAULTS = { h: 3, maxH: 5, minH: 2, w: 1 };

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
      template: localizeReliabilityDashboardLayoutTemplate(
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
  metrics: OperationalDashboardMetrics,
  intl: IntlShape,
): WidgetMapping {
  const metricById = new Map(
    metrics.metrics.map((metric) => [metric.id, metric]),
  );

  const renderTrend = (
    metricId: string,
    titleMessage: (typeof messages)[keyof typeof messages],
  ) => {
    const title = intl.formatMessage(titleMessage);
    return (
      <ApiReliabilityTrendCard
        metric={metricById.get(metricId)}
        title={title}
      />
    );
  };

  return {
    "reliability-summary": {
      defaults: {
        h: RELIABILITY_SUMMARY_WIDGET_HEIGHT,
        maxH: RELIABILITY_SUMMARY_WIDGET_HEIGHT + 2,
        minH: METRIC_WIDGET_DEFAULTS.minH,
        w: RELIABILITY_DASHBOARD_COLUMN_COUNT,
      },
      config: {
        icon: <TachometerAltIcon />,
        title: intl.formatMessage(messages.reliabilitySummaryWidget),
      },
      renderWidget: () => <ReliabilitySummaryCard metrics={metrics.metrics} />,
    },
    "api-request-rate": {
      defaults: {
        h: RELIABILITY_TREND_WIDGET_HEIGHT,
        maxH: RELIABILITY_TREND_WIDGET_HEIGHT + 2,
        minH: METRIC_WIDGET_DEFAULTS.minH,
        w: 1,
      },
      config: {
        icon: <ChartLineIcon />,
        title: intl.formatMessage(messages.apiRequestRateWidget),
      },
      renderWidget: () =>
        renderTrend("api-request-rate", messages.apiRequestRateWidget),
    },
    "api-error-rate": {
      defaults: {
        h: RELIABILITY_TREND_WIDGET_HEIGHT,
        maxH: RELIABILITY_TREND_WIDGET_HEIGHT + 2,
        minH: METRIC_WIDGET_DEFAULTS.minH,
        w: 1,
      },
      config: {
        icon: <ExclamationCircleIcon />,
        title: intl.formatMessage(messages.apiErrorRateWidget),
      },
      renderWidget: () =>
        renderTrend("api-error-rate", messages.apiErrorRateWidget),
    },
    "api-latency": {
      defaults: {
        h: RELIABILITY_TREND_WIDGET_HEIGHT,
        maxH: RELIABILITY_TREND_WIDGET_HEIGHT + 2,
        minH: METRIC_WIDGET_DEFAULTS.minH,
        w: 1,
      },
      config: {
        icon: <OutlinedClockIcon />,
        title: intl.formatMessage(messages.apiLatencyWidget),
      },
      renderWidget: () => renderTrend("api-latency", messages.apiLatencyWidget),
    },
    "reconciliation-failures": {
      defaults: {
        h: RELIABILITY_TREND_WIDGET_HEIGHT,
        maxH: RELIABILITY_TREND_WIDGET_HEIGHT + 2,
        minH: METRIC_WIDGET_DEFAULTS.minH,
        w: 1,
      },
      config: {
        icon: <ExclamationCircleIcon />,
        title: intl.formatMessage(messages.widgetReconciliationFailures),
      },
      renderWidget: () =>
        renderTrend(
          "reconciliation-failures",
          messages.widgetReconciliationFailures,
        ),
    },
    "reconciliation-retries": {
      defaults: {
        h: RELIABILITY_TREND_WIDGET_HEIGHT,
        maxH: RELIABILITY_TREND_WIDGET_HEIGHT + 2,
        minH: METRIC_WIDGET_DEFAULTS.minH,
        w: 1,
      },
      config: {
        icon: <SyncAltIcon />,
        title: intl.formatMessage(messages.widgetReconciliationRetries),
      },
      renderWidget: () =>
        renderTrend(
          "reconciliation-retries",
          messages.widgetReconciliationRetries,
        ),
    },
    "reconciliation-lag": {
      defaults: {
        h: RELIABILITY_TREND_WIDGET_HEIGHT,
        maxH: RELIABILITY_TREND_WIDGET_HEIGHT + 2,
        minH: METRIC_WIDGET_DEFAULTS.minH,
        w: 1,
      },
      config: {
        icon: <OutlinedClockIcon />,
        title: intl.formatMessage(messages.widgetReconciliationLag),
      },
      renderWidget: () =>
        renderTrend("reconciliation-lag", messages.widgetReconciliationLag),
    },
    "stale-resource-status-count": {
      defaults: {
        h: RELIABILITY_TREND_WIDGET_HEIGHT,
        maxH: RELIABILITY_TREND_WIDGET_HEIGHT + 2,
        minH: METRIC_WIDGET_DEFAULTS.minH,
        w: 1,
      },
      config: {
        icon: <ExclamationCircleIcon />,
        title: intl.formatMessage(messages.widgetStaleResourceStatus),
      },
      renderWidget: () =>
        renderTrend(
          "stale-resource-status-count",
          messages.widgetStaleResourceStatus,
        ),
    },
  };
}

export interface ReliabilityDashboardPageProps {
  metrics?: OperationalDashboardMetrics;
  title?: string;
}

export function ReliabilityDashboardPage({
  metrics,
  title,
}: Readonly<ReliabilityDashboardPageProps>) {
  const intl = useIntl();
  const { probes = noopDashboardProbePublisher } = useDashboardUi();
  const pageTitle = title ?? intl.formatMessage(messages.reliabilityTitle);
  const metricsQuery = useGetReliabilityMetricsData({
    enabled: metrics === undefined,
  });
  const dashboardMetrics = metrics ?? metricsQuery.data;
  const showTotalInitialLoadError =
    metrics === undefined && metricsQuery.isError && !metricsQuery.data;
  const showPartialLoadWarning =
    metrics === undefined && (dashboardMetrics?.failedSources?.length ?? 0) > 0;
  const showRefreshError =
    metrics === undefined &&
    metricsQuery.isError &&
    Boolean(metricsQuery.data) &&
    !metricsQuery.isFetching;
  const localizedBaseTemplate = useMemo(
    () => localizeReliabilityDashboardLayoutTemplate(baseTemplate, intl),
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
    () => localizeReliabilityDashboardLayoutTemplate(dashboardTemplate, intl),
    [dashboardTemplate, intl],
  );
  const activeWidgetTypes = useMemo(
    () => getActiveWidgetTypes(displayTemplate),
    [displayTemplate],
  );
  const widgetMapping = useMemo(
    () =>
      dashboardMetrics
        ? createWidgetMapping(dashboardMetrics, intl)
        : undefined,
    [dashboardMetrics, intl],
  );

  const hasWidgetsToAdd = useMemo(() => {
    if (!widgetMapping) {
      return false;
    }

    return Object.keys(widgetMapping).some(
      (type) => !activeWidgetTypes.includes(type),
    );
  }, [widgetMapping, activeWidgetTypes]);

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
    const defaultTemplate = localizeReliabilityDashboardLayoutTemplate(
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
      <PageSection type={PageSectionTypes.subNav}>
        <DashboardSecondaryNav active="reliability" />
      </PageSection>
      <PageSection isFilled padding={{ default: "padding" }}>
        <Flex
          alignItems={{ default: "alignItemsFlexStart" }}
          justifyContent={{ default: "justifyContentSpaceBetween" }}
        >
          <FlexItem>
            <Content>
              <Title headingLevel="h1">{pageTitle}</Title>
              <p>
                <FormattedMessage {...messages.reliabilityDescription} />
              </p>
            </Content>
          </FlexItem>
          {metrics === undefined || widgetMapping ? (
            <FlexItem className="hypershell-dashboard-header-actions">
              <Flex
                alignItems={{ default: "alignItemsCenter" }}
                direction={{ default: "column" }}
                spaceItems={{ default: "spaceItemsSm" }}
              >
                {metrics === undefined ? (
                  <Flex
                    alignItems={{ default: "alignItemsCenter" }}
                    spaceItems={{ default: "spaceItemsSm" }}
                  >
                    {dashboardMetrics?.lastSuccessfulRefresh ? (
                      <FlexItem>
                        <span className="hypershell-dashboard-last-refreshed">
                          <FormattedMessage
                            {...messages.lastRefreshed}
                            values={{
                              timestamp: (
                                <Timestamp
                                  date={dashboardMetrics.lastSuccessfulRefresh}
                                  dateFormat={TimestampFormat.medium}
                                  timeFormat={TimestampFormat.medium}
                                />
                              ),
                            }}
                          />
                        </span>
                      </FlexItem>
                    ) : null}
                    <FlexItem>
                      <ResourceRefreshButton
                        ariaLabel={intl.formatMessage(messages.refresh)}
                        isRefreshing={metricsQuery.isFetching}
                        onRefresh={() => {
                          void metricsQuery.refetch();
                        }}
                      />
                    </FlexItem>
                  </Flex>
                ) : null}
                {widgetMapping ? (
                  <Flex
                    alignItems={{ default: "alignItemsCenter" }}
                    spaceItems={{ default: "spaceItemsSm" }}
                  >
                    <FlexItem>
                      <Button variant="link" onClick={handleResetToDefault}>
                        {intl.formatMessage(messages.resetToDefault)}
                      </Button>
                    </FlexItem>
                    {hasWidgetsToAdd ? (
                      <FlexItem>
                        <AddWidgetsButton
                          onClick={() => {
                            setDrawerOpen(!drawerOpen);
                          }}
                        >
                          {intl.formatMessage(messages.addWidgets)}
                        </AddWidgetsButton>
                      </FlexItem>
                    ) : null}
                  </Flex>
                ) : null}
              </Flex>
            </FlexItem>
          ) : null}
        </Flex>
        {metricsQuery.isPending && metrics === undefined ? (
          <Bullseye>
            <Spinner
              aria-label={intl.formatMessage(messages.reliabilityLoading)}
            />
          </Bullseye>
        ) : null}
        {showTotalInitialLoadError ? (
          <Alert
            title={intl.formatMessage(messages.reliabilityLoadErrorTitle)}
            variant="danger"
          >
            <FormattedMessage {...messages.reliabilityLoadErrorBody} />
          </Alert>
        ) : null}
        {showPartialLoadWarning ? (
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
