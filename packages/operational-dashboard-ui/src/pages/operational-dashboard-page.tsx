import {
  Alert,
  Bullseye,
  Button,
  Content,
  EmptyState,
  EmptyStateBody,
  EmptyStateVariant,
  PageSection,
  Spinner,
  Flex,
  FlexItem,
  Title,
  Toolbar,
  ToolbarContent,
  ToolbarGroup,
  ToolbarItem,
} from "@patternfly/react-core";
import {
  ClusterIcon,
  CubesIcon,
  HourglassHalfIcon,
  UsersIcon,
  MicrochipIcon,
  MemoryIcon,
  ServerIcon,
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
import { useEffect, useMemo, useRef, useState } from "react";
import { FormattedMessage, useIntl, type IntlShape } from "react-intl";

import type { OperationalDashboardMetrics } from "../application/dashboard-types";
import type { DashboardProbe } from "../application/dashboard-probes";
import { noopDashboardProbePublisher } from "../application/dashboard-probes";
import {
  defaultDashboardLayoutTemplate,
  GATEWAY_STATUS_WIDGET_HEIGHT,
  NODE_STATUS_WIDGET_HEIGHT,
  localizeDashboardLayoutTemplate,
  PROVISION_TIME_WIDGET_HEIGHT,
  SYSTEM_SUMMARY_WIDGET_HEIGHT,
  USAGE_SUMMARY_WIDGET_HEIGHT,
} from "../dashboard/dashboard-layout-template";
import {
  getActiveWidgetTypes,
  isValidSavedTemplate,
  sanitizeDashboardTemplate,
} from "../dashboard/dashboard-layout-persistence";
import { UtilizationChart } from "../dashboard/utilization-chart";
import { useDashboardUi } from "../dashboard-ui-provider";
import { messages } from "../messages";
import { ResourceRefreshButton } from "../shared/resource-refresh-button";
import "./dashboard-widget.css";
import {
  GatewayStatusCard,
  MetricCard,
  NodeStatusCard,
  PodCapacityCard,
  ProvisionTimeCard,
  SystemSummaryCard,
  UsageSummaryCard,
} from "./dashboard-widget";
import { useGetMetricsData } from "./get-metrics-data";

const baseTemplate = defaultDashboardLayoutTemplate;

const LAYOUT_STORAGE_KEY = "hypershell.operational-dashboard.layout.v21";
const CUSTOM_COLUMNS: Record<Variants, number> = {
  xl: 4,
  lg: 4,
  md: 4,
  sm: 1,
};

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
      template: localizeDashboardLayoutTemplate(parsed, intl),
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

const METRIC_WIDGET_DEFAULTS = { h: 3, maxH: 5, minH: 2, w: 1 };

function createWidgetMapping(
  metrics: OperationalDashboardMetrics,
  intl: IntlShape,
): WidgetMapping {
  const metricById = new Map(
    metrics.metrics.map((metric) => [metric.id, metric]),
  );

  const renderMetric = (
    metricId: string,
    subtitle: string,
    titleMessage: (typeof messages)[keyof typeof messages],
    metricType:
      | "metric"
      | "gateway-status"
      | "node-status"
      | "pod-capacity"
      | "provision-time"
      | "utilization",
  ) => {
    const metric = metricById.get(metricId);
    const title = intl.formatMessage(titleMessage);

    if (!metric) {
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

    if (metricType === "metric") {
      return <MetricCard metric={metric} subtitle={subtitle} title={title} />;
    }

    if (metricType === "gateway-status") {
      return <GatewayStatusCard metric={metric} />;
    }

    if (metricType === "node-status") {
      return <NodeStatusCard metric={metric} />;
    }

    if (metricType === "pod-capacity") {
      return <PodCapacityCard metric={metric} />;
    }

    if (metricType === "provision-time") {
      return <ProvisionTimeCard metric={metric} />;
    }

    return <UtilizationChart metric={metric} />;
  };

  return {
    "usage-summary": {
      defaults: {
        h: USAGE_SUMMARY_WIDGET_HEIGHT,
        maxH: USAGE_SUMMARY_WIDGET_HEIGHT + 2,
        minH: METRIC_WIDGET_DEFAULTS.minH,
        w: 1,
      },
      config: {
        icon: <UsersIcon />,
        title: intl.formatMessage(messages.usageSummaryWidget),
      },
      renderWidget: () => <UsageSummaryCard metrics={metrics.metrics} />,
    },
    "system-summary": {
      defaults: {
        h: SYSTEM_SUMMARY_WIDGET_HEIGHT,
        maxH: SYSTEM_SUMMARY_WIDGET_HEIGHT + 2,
        minH: METRIC_WIDGET_DEFAULTS.minH,
        w: 1,
      },
      config: {
        icon: <MemoryIcon />,
        title: intl.formatMessage(messages.systemSummaryWidget),
      },
      renderWidget: () => <SystemSummaryCard metrics={metrics.metrics} />,
    },
    "registered-users": {
      defaults: METRIC_WIDGET_DEFAULTS,
      config: {
        icon: <UsersIcon />,
        title: intl.formatMessage(messages.registeredUsers),
      },
      renderWidget: () =>
        renderMetric(
          "registered-users",
          "",
          messages.registeredUsers,
          "metric",
        ),
    },
    "gateway-status": {
      defaults: {
        h: GATEWAY_STATUS_WIDGET_HEIGHT,
        maxH: GATEWAY_STATUS_WIDGET_HEIGHT + 2,
        minH: METRIC_WIDGET_DEFAULTS.minH,
        w: 1,
      },
      config: {
        icon: <ClusterIcon />,
        title: intl.formatMessage(messages.gatewayStatusWidget),
      },
      renderWidget: () =>
        renderMetric(
          "provisioned-gateways",
          "",
          messages.gatewayStatusWidget,
          "gateway-status",
        ),
    },
    "provision-time": {
      defaults: {
        h: PROVISION_TIME_WIDGET_HEIGHT,
        maxH: PROVISION_TIME_WIDGET_HEIGHT + 2,
        minH: METRIC_WIDGET_DEFAULTS.minH,
        w: 1,
      },
      config: {
        icon: <HourglassHalfIcon />,
        title: intl.formatMessage(messages.provisionTimeWidget),
      },
      renderWidget: () =>
        renderMetric(
          "provision-time",
          "",
          messages.provisionTimeWidget,
          "provision-time",
        ),
    },
    "provisioned-sandboxes": {
      defaults: METRIC_WIDGET_DEFAULTS,
      config: {
        icon: <CubesIcon />,
        title: intl.formatMessage(messages.widgetSandboxes),
      },
      renderWidget: () =>
        renderMetric(
          "provisioned-sandboxes",
          "",
          messages.widgetSandboxes,
          "metric",
        ),
    },
    cpu: {
      defaults: METRIC_WIDGET_DEFAULTS,
      config: {
        icon: <MicrochipIcon />,
        title: intl.formatMessage(messages.widgetCpu),
      },
      renderWidget: () =>
        renderMetric("cpu", "", messages.widgetCpu, "utilization"),
    },
    memory: {
      defaults: METRIC_WIDGET_DEFAULTS,
      config: {
        icon: <MemoryIcon />,
        title: intl.formatMessage(messages.widgetMemory),
      },
      renderWidget: () =>
        renderMetric("memory", "", messages.widgetMemory, "utilization"),
    },
    pods: {
      defaults: {
        h: NODE_STATUS_WIDGET_HEIGHT,
        maxH: NODE_STATUS_WIDGET_HEIGHT + 2,
        minH: METRIC_WIDGET_DEFAULTS.minH,
        w: 1,
      },
      config: {
        icon: <CubesIcon />,
        title: intl.formatMessage(messages.widgetPods),
      },
      renderWidget: () =>
        renderMetric("pods", "", messages.widgetPods, "pod-capacity"),
    },
    nodes: {
      defaults: {
        h: NODE_STATUS_WIDGET_HEIGHT,
        maxH: NODE_STATUS_WIDGET_HEIGHT + 2,
        minH: METRIC_WIDGET_DEFAULTS.minH,
        w: 1,
      },
      config: {
        icon: <ServerIcon />,
        title: intl.formatMessage(messages.nodes),
      },
      renderWidget: () =>
        renderMetric("nodes", "", messages.nodes, "node-status"),
    },
  };
}

export interface OperationalDashboardPageProps {
  metrics?: OperationalDashboardMetrics;
  title?: string;
}

export function OperationalDashboardPage({
  metrics,
  title,
}: Readonly<OperationalDashboardPageProps>) {
  const intl = useIntl();
  const { probes = noopDashboardProbePublisher } = useDashboardUi();
  const pageTitle = title ?? intl.formatMessage(messages.title);
  const metricsQuery = useGetMetricsData({
    enabled: metrics === undefined,
  });
  const dashboardMetrics = metrics ?? metricsQuery.data;
  const showInitialLoadError =
    metrics === undefined && metricsQuery.isError && !metricsQuery.data;
  const showRefreshError =
    metrics === undefined &&
    metricsQuery.isError &&
    Boolean(metricsQuery.data) &&
    !metricsQuery.isFetching;
  const localizedBaseTemplate = useMemo(
    () => localizeDashboardLayoutTemplate(baseTemplate, intl),
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
    () => localizeDashboardLayoutTemplate(dashboardTemplate, intl),
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

    const sanitized = sanitizeDashboardTemplate(nextTemplate);
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
    const defaultTemplate = localizeDashboardLayoutTemplate(baseTemplate, intl);
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
    <PageSection isFilled padding={{ default: "padding" }}>
      <Flex
        alignItems={{ default: "alignItemsFlexStart" }}
        justifyContent={{ default: "justifyContentSpaceBetween" }}
      >
        <FlexItem>
          <Content>
            <Title headingLevel="h1">{pageTitle}</Title>
            <p>
              <FormattedMessage {...messages.description} />
            </p>
          </Content>
        </FlexItem>
        {metrics === undefined ? (
          <FlexItem>
            <ResourceRefreshButton
              ariaLabel={intl.formatMessage(messages.refresh)}
              isRefreshing={metricsQuery.isFetching}
              onRefresh={() => {
                void metricsQuery.refetch();
              }}
            />
          </FlexItem>
        ) : null}
      </Flex>
      {metricsQuery.isPending && metrics === undefined ? (
        <Bullseye>
          <Spinner aria-label={intl.formatMessage(messages.loading)} />
        </Bullseye>
      ) : null}
      {showInitialLoadError ? (
        <Alert
          title={intl.formatMessage(messages.loadErrorTitle)}
          variant="danger"
        >
          <FormattedMessage {...messages.loadErrorBody} />
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
        <>
          <Toolbar isSticky>
            <ToolbarContent>
              <ToolbarGroup align={{ default: "alignEnd" }}>
                <ToolbarItem>
                  <Button variant="link" onClick={handleResetToDefault}>
                    {intl.formatMessage(messages.resetToDefault)}
                  </Button>
                </ToolbarItem>
                {hasWidgetsToAdd ? (
                  <ToolbarItem>
                    <AddWidgetsButton
                      onClick={() => {
                        setDrawerOpen(!drawerOpen);
                      }}
                    >
                      {intl.formatMessage(messages.addWidgets)}
                    </AddWidgetsButton>
                  </ToolbarItem>
                ) : null}
              </ToolbarGroup>
            </ToolbarContent>
          </Toolbar>
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
              template={displayTemplate}
              widgetMapping={widgetMapping}
            />
          </WidgetDrawer>
        </>
      ) : null}
    </PageSection>
  );
}
