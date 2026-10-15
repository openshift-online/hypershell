import { Content, PageSection, Spinner, Title } from "@patternfly/react-core";
import {
  ClusterIcon,
  DatabaseIcon,
  ExclamationCircleIcon,
  ExclamationTriangleIcon,
} from "@patternfly/react-icons";
import {
  GridLayout,
  WidgetDrawer,
  type ExtendedTemplateConfig,
  type Variants,
  type WidgetMapping,
} from "@patternfly/widgetized-dashboard";
import "@patternfly/widgetized-dashboard/dist/esm/styles.css";
import { useMemo, useState } from "react";
import { FormattedMessage, useIntl } from "react-intl";

import type {
  DashboardTimeRange,
  OperationalDashboardMetrics,
  OperationalMetric,
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
import { messages } from "../messages";
import { dashboardResizeWidgetConfig } from "./dashboard-resize-handle";
import { DashboardPurpose } from "./dashboard-purpose";
import {
  DashboardMetricUnavailable,
  getInventoryExceptionCounts,
  ManagedClusterProvidersCard,
  ManagedClusterRegionsCard,
  DashboardSummaryList,
  DashboardWidgetContent,
} from "./dashboard-widget";
import "./dashboard-widget.css";
import "./platform-inventory-dashboard-page.css";

const STORAGE_KEY = "hypershell.platform-inventory-dashboard.layout.v1";
const columns: Record<Variants, number> = { xl: 4, lg: 4, md: 4, sm: 1 };

function getDesktopLayout() {
  return [
    {
      i: "inventory-summary#1",
      widgetType: "inventory-summary",
      x: 0,
      y: 0,
      w: 1,
      h: 8,
      minH: 5,
      title: "Inventory summary",
    },
    {
      i: "managed-cluster-providers#1",
      widgetType: "managed-cluster-providers",
      x: 1,
      y: 0,
      w: 1,
      h: 5,
      title: "Cluster providers",
    },
    {
      i: "managed-cluster-regions#1",
      widgetType: "managed-cluster-regions",
      x: 2,
      y: 0,
      w: 2,
      h: 5,
      title: "Cluster regions",
    },
  ] as const;
}

function createDefaultLayout(): ExtendedTemplateConfig {
  const desktop = getDesktopLayout();
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

function readLayout(): ExtendedTemplateConfig {
  if (typeof window === "undefined") return createDefaultLayout();
  try {
    const saved = window.localStorage.getItem(STORAGE_KEY);
    return saved
      ? (JSON.parse(saved) as ExtendedTemplateConfig)
      : createDefaultLayout();
  } catch {
    return createDefaultLayout();
  }
}

function getHealthyClusterCount(metric: OperationalMetric): number {
  return Object.entries(metric.inventoryStatus ?? {}).reduce(
    (total, [status, count]) =>
      status.trim().toLowerCase() === "ready" ? total + count : total,
    0,
  );
}

export interface PlatformInventoryDashboardPageProps {
  isLoading: boolean;
  isRefreshing: boolean;
  metrics?: OperationalDashboardMetrics;
  onRefresh: () => void;
  onTimeRangeChange: (timeRange: DashboardTimeRange) => void;
  timeRange: DashboardTimeRange;
}

export function PlatformInventoryDashboardPage({
  isLoading,
  isRefreshing,
  metrics,
  onRefresh,
  onTimeRangeChange,
  timeRange,
}: Readonly<PlatformInventoryDashboardPageProps>) {
  const intl = useIntl();
  const [layout, setLayout] = useState<ExtendedTemplateConfig>(readLayout);
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [droppingWidgetType, setDroppingWidgetType] = useState<string>();
  const [gridLayoutKey, setGridLayoutKey] = useState(0);
  const metric = metrics?.metrics.find(
    (candidate) => candidate.id === "managed-clusters",
  );
  const activeWidgetTypes = useMemo(
    () => getActiveWidgetTypes(layout),
    [layout],
  );
  const mapping = useMemo<WidgetMapping>(() => {
    const summaryValue = (value: string | undefined) => value ?? "-";
    const recentValue =
      timeRange === 7 ? metric?.createdLast7Days : metric?.createdLast30Days;
    const healthyClusters = metric ? getHealthyClusterCount(metric) : undefined;
    const clustersNeedingAttention =
      metric && healthyClusters !== undefined
        ? Number(metric.value) - healthyClusters
        : undefined;
    const inventoryExceptions = metric
      ? getInventoryExceptionCounts(metric.inventoryStatus)
      : undefined;
    const unavailable = (
      <DashboardWidgetContent>
        <DashboardMetricUnavailable />
      </DashboardWidgetContent>
    );

    return {
      "inventory-summary": {
        defaults: { h: 8, w: 1, minH: 5, maxH: 12 },
        config: {
          icon: <DatabaseIcon />,
          title: intl.formatMessage(messages.inventorySummaryWidget),
          wrapperProps: {
            className: "hypershell-platform-inventory-widget",
          },
        },
        renderWidget: () =>
          metric ? (
            <DashboardWidgetContent>
              <DashboardSummaryList
                ariaLabel={intl.formatMessage(
                  messages.inventorySummaryAriaLabel,
                )}
                items={[
                  {
                    key: "managed-clusters",
                    label: (
                      <FormattedMessage
                        {...messages.platformInventoryManagedClusters}
                      />
                    ),
                    value: summaryValue(metric.value),
                  },
                  ...(inventoryExceptions
                    ? [
                        {
                          indicator: {
                            content: (
                              <>
                                {inventoryExceptions.danger > 0 ? (
                                  <ExclamationCircleIcon aria-hidden />
                                ) : null}
                                {inventoryExceptions.warning > 0 ? (
                                  <ExclamationTriangleIcon aria-hidden />
                                ) : null}
                              </>
                            ),
                            tone:
                              inventoryExceptions.danger > 0
                                ? ("negative" as const)
                                : ("warning" as const),
                          },
                          key: "inventory-exceptions",
                          label: (
                            <FormattedMessage
                              {...messages.platformInventoryExceptions}
                            />
                          ),
                          value: String(
                            inventoryExceptions.danger +
                              inventoryExceptions.warning,
                          ),
                        },
                      ]
                    : []),
                  {
                    key: "recently-added",
                    label: (
                      <FormattedMessage
                        {...messages.platformInventoryRecentlyAdded}
                        values={{ days: timeRange }}
                      />
                    ),
                    value: summaryValue(recentValue),
                  },
                  {
                    key: "healthy-clusters",
                    label: (
                      <FormattedMessage
                        {...messages.platformInventoryHealthyClusters}
                      />
                    ),
                    value: summaryValue(String(healthyClusters)),
                  },
                  {
                    key: "clusters-needing-attention",
                    label: (
                      <FormattedMessage
                        {...messages.platformInventoryClustersNeedingAttention}
                      />
                    ),
                    value:
                      clustersNeedingAttention === undefined
                        ? "-"
                        : String(clustersNeedingAttention),
                  },
                ]}
              />
              <small>
                <FormattedMessage {...messages.platformInventoryCompleteNote} />
              </small>
            </DashboardWidgetContent>
          ) : (
            unavailable
          ),
      },
      "managed-cluster-providers": {
        defaults: { h: 5, w: 1, minH: 2, maxH: 8 },
        config: {
          icon: <ClusterIcon />,
          title: intl.formatMessage(messages.widgetManagedClusterProviders),
          wrapperProps: {
            className: "hypershell-platform-inventory-widget",
          },
        },
        renderWidget: () =>
          metric ? (
            <ManagedClusterProvidersCard metric={metric} />
          ) : (
            unavailable
          ),
      },
      "managed-cluster-regions": {
        defaults: { h: 5, w: 2, minH: 2, maxH: 8 },
        config: {
          icon: <ClusterIcon />,
          title: intl.formatMessage(messages.widgetManagedClusterRegions),
          wrapperProps: {
            className: "hypershell-platform-inventory-widget",
          },
        },
        renderWidget: () =>
          metric ? <ManagedClusterRegionsCard metric={metric} /> : unavailable,
      },
    };
  }, [intl, metric, timeRange]);

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
    if (Object.keys(mapping).every((type) => sanitizedTypes.includes(type))) {
      setDrawerOpen(false);
    }

    window.localStorage.setItem(STORAGE_KEY, JSON.stringify(sanitized));
  };

  const reset = () => {
    setLayout(createDefaultLayout());
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
            <FormattedMessage {...messages.dashboardInventoryTitle} />
          </Title>
          <Content component="p">
            <FormattedMessage {...messages.dashboardInventoryDescription} />
          </Content>
        </Content>
      </PageSection>
      <PageSection data-page-region="body" isFilled variant="secondary">
        <DashboardNavigation
          active="inventory"
          isRefreshing={isRefreshing}
          onRefresh={onRefresh}
        />
        <DashboardPurpose
          heading={
            <FormattedMessage {...messages.dashboardInventoryPurposeHeading} />
          }
        >
          <FormattedMessage {...messages.dashboardInventoryPurpose} />
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
    </>
  );
}
