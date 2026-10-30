/* eslint-disable @typescript-eslint/no-unused-vars, formatjs/no-literal-string-in-jsx */
import {
  Alert,
  AlertActionLink,
  Bullseye,
  Card,
  CardBody,
  CardHeader,
  CardTitle,
  Content,
  EmptyState,
  EmptyStateBody,
  EmptyStateVariant,
  PageSection,
  PageSectionTypes,
  Spinner,
  Grid,
  GridItem,
  Title,
} from "@patternfly/react-core";
import {
  CheckCircleIcon,
  ClusterIcon,
  CodeBranchIcon,
  ContainerNodeIcon,
  CubesIcon,
  DatabaseIcon,
  ExclamationCircleIcon,
  HourglassHalfIcon,
  UsersIcon,
  MicrochipIcon,
  MemoryIcon,
  ServerIcon,
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
  useMemo,
  useRef,
  useState,
  type CSSProperties,
  type ReactNode,
} from "react";
import { FormattedMessage, useIntl, type IntlShape } from "react-intl";

import type {
  DashboardTimeRange,
  OperationalDashboardMetrics,
} from "../application/dashboard-types";
import type { DashboardProbe } from "../application/dashboard-probes";
import { noopDashboardProbePublisher } from "../application/dashboard-probes";
import {
  ADOPTION_GATEWAY_RELEASES_WIDGET_HEIGHT,
  ADOPTION_SANDBOX_STATUS_WIDGET_HEIGHT,
  GATEWAY_STATUS_WIDGET_HEIGHT,
  defaultDashboardLayoutTemplate,
  DASHBOARD_COLUMN_COUNT,
  INVENTORY_SUMMARY_WIDGET_HEIGHT,
  localizeDashboardLayoutTemplate,
  NODE_STATUS_WIDGET_HEIGHT,
  POD_CAPACITY_WIDGET_HEIGHT,
  PROVISION_RELIABILITY_WIDGET_HEIGHT,
  PROVISION_TIME_WIDGET_HEIGHT,
  REGISTERED_USERS_WIDGET_HEIGHT,
  SECTION_TITLE_WIDGET_TYPE,
  SYSTEM_SUMMARY_WIDGET_HEIGHT,
  TITLE_WIDGET_HEIGHT,
  USAGE_SUMMARY_WIDGET_HEIGHT,
  UTILIZATION_WIDGET_HEIGHT,
} from "../dashboard/dashboard-layout-template";
import {
  getActiveWidgetTypes,
  isValidSavedTemplate,
  sanitizeDashboardTemplate,
  stripRemovedWidgetTypes,
} from "../dashboard/dashboard-layout-persistence";
import {
  DashboardFilterToolbar,
  DashboardNavigation,
} from "../dashboard/dashboard-toolbar";
import { useDashboardUi } from "../dashboard-ui-provider";
import { messages } from "../messages";
import { dashboardResizeWidgetConfig } from "./dashboard-resize-handle";
import { DashboardPurpose } from "./dashboard-purpose";
import "./dashboard-widget.css";
import {
  GatewayReleasesCard,
  DashboardMetricUnavailable,
  GatewayStatusCard,
  AttentionRequiredCard,
  ActiveSandboxesCard,
  SandboxStatusCard,
  InventorySummaryCard,
  ManagedClusterProvidersCard,
  ManagedClusterRegionsCard,
  MetricCard,
  NodeStatusCard,
  PodCapacityCard,
  ProvisionReliabilityCard,
  ProvisioningSnapshotCard,
  ProvisionTimeCard,
  ResourceReadinessCard,
  CertificateHealthCard,
  SystemSummaryCard,
  SectionTitleCard,
  UsageSummaryCard,
  UsersCard,
  UtilizationCard,
} from "./dashboard-widget";
import { useGetMetricsData } from "./get-metrics-data";

const baseTemplate = defaultDashboardLayoutTemplate;

const LAYOUT_STORAGE_KEY = "hypershell.operational-dashboard.layout.v48";
const CUSTOM_COLUMNS: Record<Variants, number> = {
  xl: 4,
  lg: 4,
  md: 4,
  sm: 1,
};

function OverviewCard({
  title,
  children,
  span = 1,
}: Readonly<{
  title: string;
  children: ReactNode;
  span?: number;
}>) {
  return (
    <Card isFullHeight style={{ gridColumn: `span ${String(span)}` }}>
      <CardHeader>
        <CardTitle>{title}</CardTitle>
      </CardHeader>
      <CardBody>{children}</CardBody>
    </Card>
  );
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
      template: localizeDashboardLayoutTemplate(
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

const METRIC_WIDGET_DEFAULTS = { h: 3, maxH: 5, minH: 2, w: 1 };

function findLayoutItemTitle(
  template: ExtendedTemplateConfig,
  widgetId: string,
): string {
  for (const variant of Object.keys(template) as Variants[]) {
    const item = template[variant].find((entry) => entry.i === widgetId);
    if (item) {
      return item.title;
    }
  }

  return "";
}

function createWidgetMapping(
  metrics: OperationalDashboardMetrics,
  intl: IntlShape,
  template: ExtendedTemplateConfig,
  isAdmin: boolean,
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
      | "users"
      | "active-sandboxes"
      | "gateway-status"
      | "sandbox-status"
      | "gateway-releases"
      | "node-status"
      | "pod-capacity"
      | "provision-time"
      | "provision-reliability"
      | "inventory-providers"
      | "inventory-regions"
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

    if (metricType === "users") {
      return <UsersCard metric={metric} />;
    }

    if (metricType === "active-sandboxes") {
      return <MetricCard metric={metric} subtitle="" title={title} />;
    }

    if (metricType === "metric") {
      return <MetricCard metric={metric} subtitle={subtitle} title={title} />;
    }

    if (metricType === "gateway-status") {
      return <GatewayStatusCard metric={metric} />;
    }

    if (metricType === "sandbox-status") {
      return <SandboxStatusCard metric={metric} />;
    }

    if (metricType === "gateway-releases") {
      return <GatewayReleasesCard metric={metric} />;
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

    if (metricType === "provision-reliability") {
      return <ProvisionReliabilityCard metric={metric} />;
    }

    if (metricType === "inventory-providers") {
      return <ManagedClusterProvidersCard metric={metric} />;
    }

    if (metricType === "inventory-regions") {
      return <ManagedClusterRegionsCard metric={metric} />;
    }

    return <UtilizationCard metric={metric} />;
  };

  const systemWidgetConfig = (
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
              className: "hypershell-dashboard-widget--system",
              style: {
                "--hypershell-system-label": JSON.stringify(systemLabel),
              } as CSSProperties,
            }
          : undefined,
      },
    };
  };

  const mapping: WidgetMapping = {
    [SECTION_TITLE_WIDGET_TYPE]: {
      defaults: {
        h: TITLE_WIDGET_HEIGHT,
        maxH: TITLE_WIDGET_HEIGHT,
        minH: TITLE_WIDGET_HEIGHT,
        w: DASHBOARD_COLUMN_COUNT,
      },
      config: {
        title: intl.formatMessage(messages.sectionTitleDefault),
        wrapperProps: {
          className: "hypershell-dashboard-title-widget",
        },
        cardBodyProps: {
          className: "hypershell-dashboard-title-widget__body",
        },
      },
      renderWidget: (widgetId) => {
        const resolvedTitle = findLayoutItemTitle(template, widgetId);

        return (
          <SectionTitleCard
            title={
              resolvedTitle || intl.formatMessage(messages.sectionTitleDefault)
            }
          />
        );
      },
    },
    "resource-summary": {
      ...systemWidgetConfig(messages.resourceSummaryWidget, <ClusterIcon />),
      defaults: {
        h: USAGE_SUMMARY_WIDGET_HEIGHT,
        maxH: USAGE_SUMMARY_WIDGET_HEIGHT + 2,
        minH: METRIC_WIDGET_DEFAULTS.minH,
        w: 1,
      },
      renderWidget: () => <UsageSummaryCard metrics={metrics.metrics} />,
    },
    "attention-required": {
      ...systemWidgetConfig(
        messages.attentionRequiredWidget,
        <ExclamationCircleIcon />,
      ),
      defaults: {
        h: ADOPTION_SANDBOX_STATUS_WIDGET_HEIGHT,
        maxH: ADOPTION_SANDBOX_STATUS_WIDGET_HEIGHT + 2,
        minH: METRIC_WIDGET_DEFAULTS.minH,
        w: 1,
      },
      renderWidget: () => <AttentionRequiredCard metrics={metrics.metrics} />,
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
      defaults: {
        h: REGISTERED_USERS_WIDGET_HEIGHT,
        maxH: REGISTERED_USERS_WIDGET_HEIGHT + 2,
        minH: METRIC_WIDGET_DEFAULTS.minH,
        w: 2,
      },
      config: {
        icon: <UsersIcon />,
        title: intl.formatMessage(messages.registeredUsers),
      },
      renderWidget: () => (
        <UsersCard
          metric={metrics.metrics.find(
            (metric) => metric.id === "registered-users",
          )}
        />
      ),
    },
    "gateway-status": {
      ...systemWidgetConfig(
        messages.gatewayStatusWidget,
        <ContainerNodeIcon />,
      ),
      defaults: {
        h: GATEWAY_STATUS_WIDGET_HEIGHT,
        maxH: REGISTERED_USERS_WIDGET_HEIGHT,
        minH: METRIC_WIDGET_DEFAULTS.minH,
        w: 1,
      },
      renderWidget: () =>
        renderMetric(
          "provisioned-gateways",
          "",
          messages.gatewayStatusWidget,
          "gateway-status",
        ),
    },
    "sandbox-status": {
      ...systemWidgetConfig(messages.sandboxStatusWidget, <CubesIcon />),
      defaults: {
        h: GATEWAY_STATUS_WIDGET_HEIGHT,
        maxH: ADOPTION_SANDBOX_STATUS_WIDGET_HEIGHT + 2,
        minH: METRIC_WIDGET_DEFAULTS.minH,
        w: 2,
      },
      renderWidget: () =>
        renderMetric(
          "provisioned-sandboxes",
          "",
          messages.sandboxStatusWidget,
          "sandbox-status",
        ),
    },
    "active-sandboxes": {
      ...systemWidgetConfig(messages.activeSandboxesWidget, <CubesIcon />),
      defaults: {
        h: ADOPTION_SANDBOX_STATUS_WIDGET_HEIGHT,
        maxH: ADOPTION_SANDBOX_STATUS_WIDGET_HEIGHT + 2,
        minH: METRIC_WIDGET_DEFAULTS.minH,
        w: 2,
      },
      renderWidget: () => {
        const metric = metricById.get("provisioned-sandboxes");
        return metric ? (
          <ActiveSandboxesCard metric={metric} />
        ) : (
          renderMetric(
            "provisioned-sandboxes",
            "",
            messages.activeSandboxesWidget,
            "active-sandboxes",
          )
        );
      },
    },
    "gateway-releases": {
      ...systemWidgetConfig(messages.gatewayReleasesWidget, <CodeBranchIcon />),
      defaults: {
        h: ADOPTION_GATEWAY_RELEASES_WIDGET_HEIGHT,
        maxH: ADOPTION_GATEWAY_RELEASES_WIDGET_HEIGHT + 2,
        minH: METRIC_WIDGET_DEFAULTS.minH,
        w: 1,
      },
      renderWidget: () =>
        renderMetric(
          "gateway-releases",
          "",
          messages.gatewayReleasesWidget,
          "gateway-releases",
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
    "provisioning-snapshot": {
      ...systemWidgetConfig(
        messages.provisioningSnapshotWidget,
        <CheckCircleIcon />,
      ),
      defaults: {
        h: PROVISION_RELIABILITY_WIDGET_HEIGHT,
        maxH: PROVISION_RELIABILITY_WIDGET_HEIGHT + 2,
        minH: METRIC_WIDGET_DEFAULTS.minH,
        w: 1,
      },
      renderWidget: () => (
        <ProvisioningSnapshotCard metrics={metrics.metrics} />
      ),
    },
    "pod-and-sandbox-readiness": {
      ...systemWidgetConfig(
        messages.podAndSandboxReadinessWidget,
        <CheckCircleIcon />,
      ),
      defaults: {
        h: USAGE_SUMMARY_WIDGET_HEIGHT,
        maxH: USAGE_SUMMARY_WIDGET_HEIGHT + 2,
        minH: METRIC_WIDGET_DEFAULTS.minH,
        w: 1,
      },
      renderWidget: () => {
        const metric = metricById.get("pod-and-sandbox-readiness");
        return metric ? (
          <ResourceReadinessCard metric={metric} />
        ) : (
          <DashboardMetricUnavailable />
        );
      },
    },
    "certificate-health": {
      ...systemWidgetConfig(
        messages.certificateHealthWidget,
        <ExclamationCircleIcon />,
      ),
      defaults: {
        h: USAGE_SUMMARY_WIDGET_HEIGHT,
        maxH: USAGE_SUMMARY_WIDGET_HEIGHT + 2,
        minH: METRIC_WIDGET_DEFAULTS.minH,
        w: 1,
      },
      renderWidget: () => {
        const metric = metricById.get("certificate-health");
        return metric ? (
          <CertificateHealthCard metric={metric} />
        ) : (
          <DashboardMetricUnavailable />
        );
      },
    },
    cpu: {
      defaults: {
        h: UTILIZATION_WIDGET_HEIGHT,
        maxH: UTILIZATION_WIDGET_HEIGHT + 2,
        minH: METRIC_WIDGET_DEFAULTS.minH,
        w: 1,
      },
      config: {
        icon: <MicrochipIcon />,
        title: intl.formatMessage(messages.widgetCpu),
      },
      renderWidget: () =>
        renderMetric("cpu", "", messages.widgetCpu, "utilization"),
    },
    memory: {
      defaults: {
        h: UTILIZATION_WIDGET_HEIGHT,
        maxH: UTILIZATION_WIDGET_HEIGHT + 2,
        minH: METRIC_WIDGET_DEFAULTS.minH,
        w: 1,
      },
      config: {
        icon: <MemoryIcon />,
        title: intl.formatMessage(messages.widgetMemory),
      },
      renderWidget: () =>
        renderMetric("memory", "", messages.widgetMemory, "utilization"),
    },
    pods: {
      defaults: {
        h: POD_CAPACITY_WIDGET_HEIGHT,
        maxH: POD_CAPACITY_WIDGET_HEIGHT + 2,
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
    "inventory-summary": {
      defaults: {
        h: INVENTORY_SUMMARY_WIDGET_HEIGHT,
        maxH: INVENTORY_SUMMARY_WIDGET_HEIGHT + 2,
        minH: METRIC_WIDGET_DEFAULTS.minH,
        w: 1,
      },
      config: {
        icon: <DatabaseIcon />,
        title: intl.formatMessage(messages.inventorySummaryWidget),
      },
      renderWidget: () => <InventorySummaryCard metrics={metrics.metrics} />,
    },
    "managed-cluster-providers": {
      defaults: {
        h: NODE_STATUS_WIDGET_HEIGHT,
        maxH: NODE_STATUS_WIDGET_HEIGHT + 2,
        minH: METRIC_WIDGET_DEFAULTS.minH,
        w: 1,
      },
      config: {
        icon: <ClusterIcon />,
        title: intl.formatMessage(messages.widgetManagedClusterProviders),
      },
      renderWidget: () =>
        renderMetric(
          "managed-clusters",
          "",
          messages.widgetManagedClusterProviders,
          "inventory-providers",
        ),
    },
    "managed-cluster-regions": {
      defaults: {
        h: NODE_STATUS_WIDGET_HEIGHT,
        maxH: NODE_STATUS_WIDGET_HEIGHT + 2,
        minH: METRIC_WIDGET_DEFAULTS.minH,
        w: 2,
      },
      config: {
        icon: <ClusterIcon />,
        title: intl.formatMessage(messages.widgetManagedClusterRegions),
      },
      renderWidget: () =>
        renderMetric(
          "managed-clusters",
          "",
          messages.widgetManagedClusterRegions,
          "inventory-regions",
        ),
    },
  };

  const overviewWidgetTypes = [
    "resource-summary",
    "gateway-status",
    "sandbox-status",
    "active-sandboxes",
    "provisioning-snapshot",
    "gateway-releases",
    "attention-required",
    ...(isAdmin ? ["pod-and-sandbox-readiness", "certificate-health"] : []),
  ];

  return Object.fromEntries(
    overviewWidgetTypes.map((widgetType) => [widgetType, mapping[widgetType]]),
  ) as WidgetMapping;
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
  const { isAdmin } = useDashboardUi();
  const pageTitle = title ?? intl.formatMessage(messages.title);
  const [timeRange, setTimeRange] = useState<DashboardTimeRange>(7);
  const [currentTime, setCurrentTime] = useState(0);
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
  const metricsQuery = useGetMetricsData({
    enabled: metrics === undefined,
    timeRange,
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
  const showPartialLoadWarning =
    metrics === undefined && (dashboardMetrics?.failedSources?.length ?? 0) > 0;
  const showRefreshError =
    metrics === undefined &&
    metricsQuery.isError &&
    Boolean(metricsQuery.data) &&
    !metricsQuery.isFetching;
  const localizedBaseTemplate = useMemo(() => {
    const localized = localizeDashboardLayoutTemplate(baseTemplate, intl);
    if (isAdmin === true) {
      return localized;
    }
    return Object.fromEntries(
      Object.entries(localized).map(([variant, items]) => [
        variant,
        items.filter(
          (item) =>
            item.widgetType !== "pod-and-sandbox-readiness" &&
            item.widgetType !== "certificate-health",
        ),
      ]),
    ) as ExtendedTemplateConfig;
  }, [intl, isAdmin]);
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
        ? createWidgetMapping(
            dashboardMetrics,
            intl,
            displayTemplate,
            isAdmin === true,
          )
        : undefined,
    [dashboardMetrics, displayTemplate, intl, isAdmin],
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

  const metric = (id: string) =>
    dashboardMetrics?.metrics.find((item) => item.id === id);
  const requiredMetric = (id: string) => {
    const item = metric(id);
    if (!item) {
      throw new Error(`Missing dashboard metric: ${id}`);
    }
    return item;
  };
  const overviewMetric = (id: string, fallback: ReactNode) => {
    const item = metric(id);
    return item ? (
      <MetricCard metric={item} title={id} subtitle="" />
    ) : (
      fallback
    );
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
            <FormattedMessage {...messages.description} />
          </p>
        </Content>
      </PageSection>
      <PageSection data-page-region="body" isFilled variant="secondary">
        <DashboardNavigation
          active="operational"
          isRefreshing={metrics === undefined && metricsQuery.isFetching}
          onRefresh={() => {
            if (metrics === undefined) {
              void metricsQuery.refetch();
            }
          }}
          updatedMinutesAgo={updatedMinutesAgo}
        />
        <DashboardPurpose heading="What this dashboard answers">
          What needs attention right now? Are my gateways and sandboxes healthy?
          Did provisioning or capacity change enough to investigate?
        </DashboardPurpose>
        <DashboardFilterToolbar
          onAddWidgets={() => {
            setDrawerOpen(!drawerOpen);
          }}
          onResetToDefault={handleResetToDefault}
          onTimeRangeChange={setTimeRange}
          timeRange={timeRange}
        />
        {metricsQuery.isPending && metrics === undefined ? (
          <Bullseye>
            <Spinner aria-label={intl.formatMessage(messages.loading)} />
          </Bullseye>
        ) : null}
        {showTotalInitialLoadError ? (
          <Alert
            className="hypershell-dashboard-data-alert"
            title={intl.formatMessage(messages.loadErrorTitle)}
            variant="danger"
          >
            <FormattedMessage {...messages.loadErrorBody} />
          </Alert>
        ) : null}
        {showPartialLoadWarning ? (
          <Alert
            className="hypershell-dashboard-data-alert"
            title={intl.formatMessage(messages.partialLoadWarningTitle)}
            variant="warning"
          >
            <FormattedMessage {...messages.partialLoadWarningBody} />
          </Alert>
        ) : null}
        {showRefreshError ? (
          <Alert
            className="hypershell-dashboard-data-alert"
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
            <div className="hypershell-dashboard-widget-layout">
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
            </div>
          </WidgetDrawer>
        ) : null}
      </PageSection>
    </Fragment>
  );
}
