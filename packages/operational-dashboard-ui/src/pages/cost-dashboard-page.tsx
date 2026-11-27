import {
  Alert,
  AlertActionLink,
  Bullseye,
  Button,
  Card,
  CardBody,
  CardHeader,
  CardTitle,
  Content,
  EmptyState,
  EmptyStateBody,
  Label,
  PageSection,
  Spinner,
  Title,
  ToggleGroup,
  ToggleGroupItem,
} from "@patternfly/react-core";
import { Table, Tbody, Td, Th, Thead, Tr } from "@patternfly/react-table";
import {
  Fragment,
  useEffect,
  useMemo,
  useRef,
  useState,
  type CSSProperties,
  type ReactElement,
  type ReactNode,
} from "react";
import {
  GridLayout,
  WidgetDrawer,
  type ExtendedTemplateConfig,
  type Variants,
  type WidgetMapping,
} from "@patternfly/widgetized-dashboard";
import "@patternfly/widgetized-dashboard/dist/esm/styles.css";
import { FormattedMessage, useIntl, type IntlShape } from "react-intl";

import type {
  CostDashboardMetrics,
  CostDashboardView,
  CostTrendPoint,
  MyCostGatewayRow,
} from "../application/cost-dashboard-types";
import type { DashboardTimeRange } from "../application/dashboard-types";
import { costMessages } from "../cost-messages";
import type { DashboardProbe } from "../application/dashboard-probes";
import {
  noopDashboardProbePublisher,
  type DashboardProbePublisher,
} from "../application/dashboard-probes";
import {
  COST_DASHBOARD_COLUMN_COUNT,
  costWidgetDefaults,
  defaultCostDashboardLayoutTemplates,
  localizeCostDashboardLayoutTemplate,
  type CostWidgetType,
} from "../dashboard/cost-dashboard-layout-template";
import {
  getActiveWidgetTypes,
  isValidSavedTemplate,
  sanitizeDashboardTemplate,
} from "../dashboard/dashboard-layout-persistence";
import {
  DashboardFilterToolbar,
  DashboardNavigation,
} from "../dashboard/dashboard-toolbar";
import { TrendSparklineChart } from "../dashboard/trend-sparkline-chart";
import { useDashboardUi } from "../dashboard-ui-provider";
import { messages } from "../messages";
import { DashboardPurpose } from "./dashboard-purpose";
import { dashboardResizeWidgetConfig } from "./dashboard-resize-handle";
import "./dashboard-widget.css";
import "./cost-dashboard-page.css";
import { useGetCostMetricsData } from "./get-cost-metrics-data";

const NO_VALUE = "-";
const SPACE = " ";

type Money = (value: number | undefined) => string | undefined;

function moneyFormatter(intl: IntlShape, currency: string | undefined): Money {
  return (value) => {
    if (value === undefined || currency === undefined) return undefined;
    // Whole amounts and amounts of 1,000 or more drop the cents.
    const digits = value >= 1000 || Number.isInteger(value) ? 0 : 2;
    return intl.formatNumber(value, {
      currency,
      maximumFractionDigits: digits,
      minimumFractionDigits: digits,
      style: "currency",
    });
  };
}

interface CostWidgetProps {
  children: ReactNode;
  isSystem?: boolean;
  title: string;
  widgetId: CostWidgetType;
}

type CostWidgetElement = ReactElement<CostWidgetProps>;

/** Body of one cost widget; the dashboard grid tile renders its header. */
function CostWidget({ children }: Readonly<CostWidgetProps>) {
  return <div className="hypershell-cost-widget__body">{children}</div>;
}

function Row({ label, value }: Readonly<{ label: ReactNode; value?: string }>) {
  const intl = useIntl();
  return (
    <div className="hypershell-cost-row">
      <span>{label}</span>
      <strong>
        {value ?? intl.formatMessage(costMessages.costUnavailable)}
      </strong>
    </div>
  );
}

function Headline({ value }: Readonly<{ value?: string }>) {
  const intl = useIntl();
  return (
    <Title headingLevel="h3" size="lg">
      {value === undefined
        ? intl.formatMessage(costMessages.costUnavailable)
        : intl.formatMessage(costMessages.costPerMonth, { amount: value })}
    </Title>
  );
}

function Sparkline({
  money,
  points,
  title,
}: Readonly<{
  money: Money;
  points?: readonly CostTrendPoint[];
  title: string;
}>) {
  const intl = useIntl();
  if (points === undefined || points.length < 2) return null;
  return (
    <TrendSparklineChart
      caption={intl.formatMessage(costMessages.costTrendCaption)}
      title={title}
      trend={{ points }}
      valueFormatter={(value) => money(value) ?? ""}
    />
  );
}

function sumTrends(
  trends: readonly (readonly CostTrendPoint[] | undefined)[],
): CostTrendPoint[] | undefined {
  const present = trends.filter(
    (trend): trend is readonly CostTrendPoint[] => trend !== undefined,
  );
  const first = present[0];
  if (first === undefined) return undefined;
  return first.map((point, index) => {
    const values = present.map((trend) => trend[index]?.value ?? null);
    return {
      label: point.label,
      value: values.every((value) => value === null)
        ? null
        : values.reduce<number>((sum, value) => sum + (value ?? 0), 0),
    };
  });
}

function Unavailable() {
  return (
    <Content component="small">
      <FormattedMessage {...costMessages.costUnavailable} />
    </Content>
  );
}

function NotConfigured() {
  const intl = useIntl();
  return (
    <EmptyState
      headingLevel="h3"
      titleText={intl.formatMessage(costMessages.costNotConfiguredTitle)}
      variant="xs"
    >
      <EmptyStateBody>
        <FormattedMessage {...costMessages.costNotConfiguredBody} />
      </EmptyStateBody>
    </EmptyState>
  );
}

function roleMessage(role: MyCostGatewayRow["role"]) {
  if (role === "owner") return costMessages.costRoleOwner;
  if (role === "admin") return costMessages.costRoleAdmin;
  return costMessages.costRoleUser;
}

function platformWidgets(
  metrics: CostDashboardMetrics,
  money: Money,
  intl: IntlShape,
): CostWidgetElement[] {
  const summary = metrics.platform?.summary;
  const breakdown = metrics.platform?.breakdown;
  const gateways = metrics.platform?.topGateways;
  const notConfigured = metrics.status === "not-configured";
  const body = (available: boolean, content: () => ReactNode) =>
    notConfigured ? <NotConfigured /> : available ? content() : <Unavailable />;
  const envLabel = (key: string) =>
    key === "" ? intl.formatMessage(costMessages.costEnvironmentUnknown) : key;

  return [
    <CostWidget
      key="cost-summary"
      widgetId="cost-summary"
      isSystem
      title={intl.formatMessage(costMessages.costWidgetSummary)}
    >
      {body(summary !== undefined, () => (
        <>
          <Headline value={money(summary?.monthlyTotal)} />
          <div className="hypershell-cost-rows">
            <Row
              label={intl.formatMessage(costMessages.costWidgetIdle)}
              value={money(summary?.idleMonthly)}
            />
            <Row
              label={intl.formatMessage(costMessages.costWidgetPerUser)}
              value={money(summary?.perActiveUser)}
            />
            <Row
              label={intl.formatMessage(costMessages.costPerGateway)}
              value={money(summary?.perGateway)}
            />
            <Row
              label={intl.formatMessage(costMessages.costPerSandbox)}
              value={money(summary?.perSandbox)}
            />
          </div>
        </>
      ))}
    </CostWidget>,
    <CostWidget
      key="cost-idle"
      widgetId="cost-idle"
      isSystem
      title={intl.formatMessage(costMessages.costWidgetIdle)}
    >
      {body(summary !== undefined, () => (
        <>
          <Title headingLevel="h3" size="lg">
            {money(summary?.idleMonthly)}
          </Title>
          <Content component="small">
            <FormattedMessage
              {...costMessages.costIdleShare}
              values={{ percent: summary?.idlePercentOfTotal ?? 0 }}
            />
          </Content>
          <Sparkline
            money={money}
            points={summary?.idleTrend}
            title={intl.formatMessage(costMessages.costWidgetIdle)}
          />
          <Row
            label={intl.formatMessage(costMessages.costUnusedGateways)}
            value={money(summary?.unusedGatewaysMonthly)}
          />
        </>
      ))}
    </CostWidget>,
    <CostWidget
      key="cost-per-user"
      widgetId="cost-per-user"
      isSystem
      title={intl.formatMessage(costMessages.costWidgetPerUser)}
    >
      {body(summary?.perActiveUser !== undefined, () => (
        <>
          <Title headingLevel="h3" size="lg">
            {money(summary?.perActiveUser)}
          </Title>
          <Content component="small">
            <FormattedMessage {...costMessages.costPerUserCaption} />
          </Content>
        </>
      ))}
    </CostWidget>,
    <CostWidget
      key="cost-by-environment"
      widgetId="cost-by-environment"
      isSystem
      title={intl.formatMessage(costMessages.costWidgetByEnvironment)}
    >
      {body(breakdown !== undefined, () => (
        <>
          <Content component="small">
            <FormattedMessage {...costMessages.costEstimatedMonthly} />
          </Content>
          <Sparkline
            money={money}
            points={sumTrends(
              breakdown?.byEnvironment.map((row) => row.trend) ?? [],
            )}
            title={intl.formatMessage(costMessages.costWidgetByEnvironment)}
          />
          {breakdown?.byEnvironment.map((row) => (
            <Row
              key={row.key}
              label={envLabel(row.key)}
              value={money(row.monthly)}
            />
          ))}
        </>
      ))}
    </CostWidget>,
    <CostWidget
      key="cost-by-cluster"
      widgetId="cost-by-cluster"
      isSystem
      title={intl.formatMessage(costMessages.costWidgetByCluster)}
    >
      {body(breakdown !== undefined, () => (
        <>
          <Content component="small">
            <FormattedMessage {...costMessages.costEstimatedMonthly} />
          </Content>
          <Sparkline
            money={money}
            points={sumTrends(
              breakdown?.byCluster.map((row) => row.trend) ?? [],
            )}
            title={intl.formatMessage(costMessages.costWidgetByCluster)}
          />
          {breakdown?.byCluster.slice(0, 5).map((row) => (
            <Row key={row.key} label={row.key} value={money(row.monthly)} />
          ))}
        </>
      ))}
    </CostWidget>,
    <CostWidget
      key="cost-by-resource-class"
      widgetId="cost-by-resource-class"
      isSystem
      title={intl.formatMessage(costMessages.costWidgetByResourceClass)}
    >
      {body(breakdown !== undefined, () => (
        <>
          <Content component="small">
            <FormattedMessage {...costMessages.costEstimatedMonthly} />
          </Content>
          <Row
            label={intl.formatMessage(costMessages.costClassGateways)}
            value={money(breakdown?.byResourceClass.gateways)}
          />
          <Row
            label={intl.formatMessage(costMessages.costClassSandboxes)}
            value={money(breakdown?.byResourceClass.sandboxes)}
          />
          <Row
            label={intl.formatMessage(costMessages.costClassPlatform)}
            value={money(breakdown?.byResourceClass.platformOverhead)}
          />
          <Row
            label={intl.formatMessage(costMessages.costClassIdle)}
            value={money(breakdown?.byResourceClass.idle)}
          />
        </>
      ))}
    </CostWidget>,
    <CostWidget
      key="cost-top-gateways"
      widgetId="cost-top-gateways"
      isSystem
      title={intl.formatMessage(costMessages.costWidgetTopGateways)}
    >
      {body(gateways !== undefined, () => (
        <Table
          aria-label={intl.formatMessage(costMessages.costWidgetTopGateways)}
          variant="compact"
        >
          <Thead>
            <Tr>
              <Th>{intl.formatMessage(costMessages.costColumnGateway)}</Th>
              <Th>{intl.formatMessage(costMessages.costColumnCluster)}</Th>
              <Th>{intl.formatMessage(costMessages.costColumnBillingOwner)}</Th>
              <Th>{intl.formatMessage(costMessages.costColumnMonthly)}</Th>
            </Tr>
          </Thead>
          <Tbody>
            {gateways?.map((row) => (
              <Tr key={row.id}>
                <Td
                  dataLabel={intl.formatMessage(costMessages.costColumnGateway)}
                >
                  {row.name}
                  {row.unused ? (
                    <Label className="pf-v6-u-ml-sm" color="orange" isCompact>
                      {intl.formatMessage(costMessages.costUnusedLabel)}
                    </Label>
                  ) : null}
                </Td>
                <Td
                  dataLabel={intl.formatMessage(costMessages.costColumnCluster)}
                >
                  {row.cluster}
                </Td>
                <Td
                  dataLabel={intl.formatMessage(
                    costMessages.costColumnBillingOwner,
                  )}
                >
                  {row.billingOwner ?? NO_VALUE}
                </Td>
                <Td
                  dataLabel={intl.formatMessage(costMessages.costColumnMonthly)}
                >
                  {money(row.monthly)}
                </Td>
              </Tr>
            ))}
          </Tbody>
        </Table>
      ))}
    </CostWidget>,
    <CostWidget
      key="cost-by-billing-owner"
      widgetId="cost-by-billing-owner"
      isSystem
      title={intl.formatMessage(costMessages.costWidgetByBillingOwner)}
    >
      {body(breakdown?.byBillingOwner !== undefined, () => (
        <Table
          aria-label={intl.formatMessage(costMessages.costWidgetByBillingOwner)}
          variant="compact"
        >
          <Thead>
            <Tr>
              <Th>{intl.formatMessage(costMessages.costColumnBillingOwner)}</Th>
              <Th>{intl.formatMessage(costMessages.costColumnGateways)}</Th>
              <Th>{intl.formatMessage(costMessages.costColumnMonthly)}</Th>
            </Tr>
          </Thead>
          <Tbody>
            {breakdown?.byBillingOwner?.slice(0, 10).map((row) => (
              <Tr key={row.key}>
                <Td
                  dataLabel={intl.formatMessage(
                    costMessages.costColumnBillingOwner,
                  )}
                >
                  {row.key || NO_VALUE}
                </Td>
                <Td
                  dataLabel={intl.formatMessage(
                    costMessages.costColumnGateways,
                  )}
                >
                  {row.gatewayCount}
                </Td>
                <Td
                  dataLabel={intl.formatMessage(costMessages.costColumnMonthly)}
                >
                  {money(row.monthly)}
                </Td>
              </Tr>
            ))}
          </Tbody>
        </Table>
      ))}
    </CostWidget>,
  ];
}

function myCostWidgets(
  metrics: CostDashboardMetrics,
  money: Money,
  intl: IntlShape,
): CostWidgetElement[] {
  const summary = metrics.mine?.summary;
  const gateways = metrics.mine?.gateways;
  const workspaces = metrics.mine?.workspaces;
  const notConfigured = metrics.status === "not-configured";

  const body = (available: boolean, content: () => ReactNode) =>
    notConfigured ? <NotConfigured /> : available ? content() : <Unavailable />;
  const noRows = (
    <Content component="small">
      <FormattedMessage {...costMessages.costNoRows} />
    </Content>
  );

  return [
    <CostWidget
      key="my-cost-charged"
      widgetId="my-cost-charged"
      title={intl.formatMessage(costMessages.costWidgetCharged)}
    >
      {body(summary !== undefined, () => (
        <>
          <Headline value={money(summary?.chargedMonthly)} />
          <Content component="small">
            <FormattedMessage
              {...costMessages.costChargedCaption}
              values={{ count: summary?.chargedGatewayCount ?? 0 }}
            />
          </Content>
        </>
      ))}
    </CostWidget>,
    <CostWidget
      key="my-cost-shared"
      widgetId="my-cost-shared"
      title={intl.formatMessage(costMessages.costWidgetShared)}
    >
      {body(summary !== undefined, () => (
        <>
          <Headline value={money(summary?.sharedMonthly)} />
          <Content component="small">
            <FormattedMessage
              {...costMessages.costSharedCaption}
              values={{ count: summary?.sharedGatewayCount ?? 0 }}
            />
          </Content>
        </>
      ))}
    </CostWidget>,
    <CostWidget
      key="my-cost-trend"
      widgetId="my-cost-trend"
      title={intl.formatMessage(costMessages.costWidgetTrend)}
    >
      {body(summary !== undefined, () => (
        <Sparkline
          money={money}
          points={summary?.trend}
          title={intl.formatMessage(costMessages.costWidgetTrend)}
        />
      ))}
    </CostWidget>,
    <CostWidget
      key="my-cost-by-gateway"
      widgetId="my-cost-by-gateway"
      title={intl.formatMessage(costMessages.costWidgetByGateway)}
    >
      {body(gateways !== undefined, () =>
        gateways?.length === 0 ? (
          noRows
        ) : (
          <Table
            aria-label={intl.formatMessage(costMessages.costWidgetByGateway)}
            variant="compact"
          >
            <Thead>
              <Tr>
                <Th>{intl.formatMessage(costMessages.costColumnGateway)}</Th>
                <Th>{intl.formatMessage(costMessages.costColumnMyRole)}</Th>
                <Th>
                  {intl.formatMessage(costMessages.costColumnBillingOwner)}
                </Th>
                <Th>{intl.formatMessage(costMessages.costColumnCluster)}</Th>
                <Th>{intl.formatMessage(costMessages.costColumnMonthly)}</Th>
              </Tr>
            </Thead>
            <Tbody>
              {gateways?.map((row) => (
                <Tr key={row.id}>
                  <Td
                    dataLabel={intl.formatMessage(
                      costMessages.costColumnGateway,
                    )}
                  >
                    {row.name}
                    {row.unused ? (
                      <Label className="pf-v6-u-ml-sm" color="orange" isCompact>
                        {intl.formatMessage(costMessages.costUnusedLabel)}
                      </Label>
                    ) : null}
                  </Td>
                  <Td
                    dataLabel={intl.formatMessage(
                      costMessages.costColumnMyRole,
                    )}
                  >
                    {intl.formatMessage(roleMessage(row.role))}
                  </Td>
                  <Td
                    dataLabel={intl.formatMessage(
                      costMessages.costColumnBillingOwner,
                    )}
                  >
                    {row.charged ? (
                      <Label color="blue" isCompact>
                        {intl.formatMessage(costMessages.costChargedToMeLabel)}
                      </Label>
                    ) : (
                      (row.billingOwner ?? NO_VALUE)
                    )}
                  </Td>
                  <Td
                    dataLabel={intl.formatMessage(
                      costMessages.costColumnCluster,
                    )}
                  >
                    {row.cluster}
                  </Td>
                  <Td
                    dataLabel={intl.formatMessage(
                      costMessages.costColumnMonthly,
                    )}
                  >
                    {money(row.monthly)}
                  </Td>
                </Tr>
              ))}
            </Tbody>
          </Table>
        ),
      )}
    </CostWidget>,
    <CostWidget
      key="my-cost-by-workspace"
      widgetId="my-cost-by-workspace"
      title={intl.formatMessage(costMessages.costWidgetByWorkspace)}
    >
      {body(workspaces !== undefined, () =>
        workspaces?.length === 0 ? (
          noRows
        ) : (
          <Table
            aria-label={intl.formatMessage(costMessages.costWidgetByWorkspace)}
            variant="compact"
          >
            <Thead>
              <Tr>
                <Th>{intl.formatMessage(costMessages.costColumnWorkspace)}</Th>
                <Th>{intl.formatMessage(costMessages.costColumnGateway)}</Th>
                <Th>{intl.formatMessage(costMessages.costColumnMonthly)}</Th>
              </Tr>
            </Thead>
            <Tbody>
              {workspaces?.map((row) => (
                <Tr key={`${row.gatewayId}/${row.workspace}`}>
                  <Td
                    dataLabel={intl.formatMessage(
                      costMessages.costColumnWorkspace,
                    )}
                  >
                    {row.workspace}
                  </Td>
                  <Td
                    dataLabel={intl.formatMessage(
                      costMessages.costColumnGateway,
                    )}
                  >
                    {row.gatewayName}
                  </Td>
                  <Td
                    dataLabel={intl.formatMessage(
                      costMessages.costColumnMonthly,
                    )}
                  >
                    {money(row.monthly)}
                  </Td>
                </Tr>
              ))}
            </Tbody>
          </Table>
        ),
      )}
    </CostWidget>,
  ];
}

const COST_COLUMNS: Record<Variants, number> = {
  xl: COST_DASHBOARD_COLUMN_COUNT,
  lg: COST_DASHBOARD_COLUMN_COUNT,
  md: COST_DASHBOARD_COLUMN_COUNT,
  sm: 1,
};

function layoutStorageKey(view: CostDashboardView): string {
  return `hypershell.cost-dashboard.${view}.layout.v1`;
}

function layoutProbe(
  name: DashboardProbe["name"],
  outcome: DashboardProbe["fields"]["outcome"],
): DashboardProbe {
  return Object.freeze({
    context: Object.freeze({ correlationId: crypto.randomUUID() }),
    fields: Object.freeze({ action: "persist-layout-template", outcome }),
    name,
    occurredAt: new Date().toISOString(),
    schemaVersion: 1,
  });
}

function readSavedLayout(
  view: CostDashboardView,
  intl: IntlShape,
): { invalid: boolean; template: ExtendedTemplateConfig } {
  const base = localizeCostDashboardLayoutTemplate(
    defaultCostDashboardLayoutTemplates[view],
    intl,
  );
  try {
    const raw = window.localStorage.getItem(layoutStorageKey(view));
    if (!raw) return { invalid: false, template: base };
    const parsed = JSON.parse(raw) as ExtendedTemplateConfig;
    if (!isValidSavedTemplate(parsed, base)) {
      return { invalid: true, template: base };
    }
    return {
      invalid: false,
      template: localizeCostDashboardLayoutTemplate(parsed, intl),
    };
  } catch {
    return { invalid: true, template: base };
  }
}

function persistLayout(
  view: CostDashboardView,
  template: ExtendedTemplateConfig,
  probes: DashboardProbePublisher,
) {
  try {
    window.localStorage.setItem(
      layoutStorageKey(view),
      JSON.stringify(template),
    );
  } catch {
    probes.publish(
      layoutProbe("dashboard.layout.template.persistence-failed", "failed"),
    );
  }
}

function createCostWidgetMapping(
  widgets: readonly CostWidgetElement[],
  intl: IntlShape,
): WidgetMapping {
  const systemLabel = intl.formatMessage(costMessages.costSystemLabel);
  return Object.fromEntries(
    widgets.map((widget) => {
      const { isSystem = false, title, widgetId } = widget.props;
      const defaults = costWidgetDefaults[widgetId];
      return [
        widgetId,
        {
          config: {
            title,
            // WEB-COST-02: platform widgets carry the System label.
            wrapperProps: isSystem
              ? {
                  "aria-label": `${title} ${systemLabel}`,
                  className:
                    "hypershell-cost-widget hypershell-dashboard-widget--system",
                  style: {
                    "--hypershell-system-label": JSON.stringify(systemLabel),
                  } as CSSProperties,
                }
              : { className: "hypershell-cost-widget" },
          },
          defaults: {
            h: defaults.h,
            maxH: defaults.h + 4,
            minH: 2,
            w: defaults.w,
          },
          renderWidget: () => widget,
        },
      ];
    }),
  );
}

/**
 * Cost widgets on the shared widgetized dashboard grid (WEB-COST-08). Each
 * view keeps its own saved layout.
 */
function CostWidgetGrid({
  drawerOpen,
  onDrawerOpenChange,
  view,
  widgets,
}: Readonly<{
  drawerOpen: boolean;
  onDrawerOpenChange: (open: boolean) => void;
  view: CostDashboardView;
  widgets: readonly CostWidgetElement[];
}>) {
  const intl = useIntl();
  const { probes = noopDashboardProbePublisher } = useDashboardUi();
  const saved = useMemo(() => readSavedLayout(view, intl), [view, intl]);
  const [template, setTemplate] = useState(saved.template);
  const [gridKey, setGridKey] = useState(0);
  const [droppingWidgetType, setDroppingWidgetType] = useState<string>();
  const invalidPublishedRef = useRef(false);

  useEffect(() => {
    if (!saved.invalid || invalidPublishedRef.current) return;
    invalidPublishedRef.current = true;
    probes.publish(layoutProbe("dashboard.layout.template.invalid", "failed"));
  }, [probes, saved.invalid]);

  const persist = (next: ExtendedTemplateConfig) => {
    persistLayout(view, next, probes);
  };

  const displayTemplate = useMemo(
    () => localizeCostDashboardLayoutTemplate(template, intl),
    [template, intl],
  );
  const activeWidgetTypes = useMemo(
    () => getActiveWidgetTypes(displayTemplate),
    [displayTemplate],
  );
  const widgetMapping = useMemo(
    () => createCostWidgetMapping(widgets, intl),
    [widgets, intl],
  );

  const handleTemplateChange = (next: ExtendedTemplateConfig) => {
    const current = new Set(activeWidgetTypes);
    const added = getActiveWidgetTypes(next).filter(
      (type) => !current.has(type),
    );
    if (
      (added.length > 0 && droppingWidgetType === undefined) ||
      (droppingWidgetType !== undefined && current.has(droppingWidgetType))
    ) {
      setGridKey((key) => key + 1);
      return;
    }
    const sanitized = sanitizeDashboardTemplate(next);
    setTemplate(sanitized);
    const types = getActiveWidgetTypes(sanitized);
    if (Object.keys(widgetMapping).every((type) => types.includes(type))) {
      onDrawerOpenChange(false);
    }
    persist(sanitized);
  };

  return (
    <WidgetDrawer
      currentlyUsedWidgets={activeWidgetTypes}
      isOpen={drawerOpen}
      onOpenChange={onDrawerOpenChange}
      onWidgetDragEnd={() => {
        setDroppingWidgetType(undefined);
      }}
      onWidgetDragStart={setDroppingWidgetType}
      widgetMapping={widgetMapping}
    >
      <GridLayout
        key={gridKey}
        columns={COST_COLUMNS}
        droppingWidgetType={droppingWidgetType}
        onDrawerExpandChange={onDrawerOpenChange}
        onTemplateChange={handleTemplateChange}
        resizeWidgetConfig={dashboardResizeWidgetConfig}
        template={displayTemplate}
        widgetMapping={widgetMapping}
      />
    </WidgetDrawer>
  );
}

function MyCostEmpty() {
  const intl = useIntl();
  return (
    <EmptyState
      headingLevel="h2"
      titleText={intl.formatMessage(costMessages.costEmptyTitle)}
    >
      <EmptyStateBody>
        <FormattedMessage {...costMessages.costEmptyBody} />
      </EmptyStateBody>
    </EmptyState>
  );
}

function AboutEstimates({
  metrics,
}: Readonly<{ metrics?: CostDashboardMetrics }>) {
  const intl = useIntl();
  return (
    <Card id="cost-estimates" isCompact>
      <CardHeader>
        <CardTitle component="h2">
          {intl.formatMessage(costMessages.costAboutTitle)}
        </CardTitle>
      </CardHeader>
      <CardBody>
        <Content>
          {metrics?.rateCardVersion !== undefined &&
          metrics.currency !== undefined ? (
            <p>
              <FormattedMessage
                {...costMessages.costAboutModel}
                values={{
                  currency: metrics.currency,
                  version: metrics.rateCardVersion,
                }}
              />
            </p>
          ) : null}
          <p>
            <FormattedMessage {...costMessages.costAboutAttribution} />
          </p>
          <p>
            <FormattedMessage {...costMessages.costAboutExcluded} />
          </p>
          <p>
            <FormattedMessage {...costMessages.costAboutNotBilling} />
          </p>
        </Content>
      </CardBody>
    </Card>
  );
}

export interface CostDashboardPageProps {
  /** Static metrics (stories, tests); the page loads its own when omitted. */
  metrics?: CostDashboardMetrics;
  onTimeRangeChange?: (timeRange: DashboardTimeRange) => void;
  onViewChange?: (view: CostDashboardView) => void;
  timeRange?: DashboardTimeRange;
  title?: string;
  view?: CostDashboardView;
}

export function CostDashboardPage({
  metrics,
  onTimeRangeChange,
  onViewChange,
  timeRange,
  title,
  view,
}: Readonly<CostDashboardPageProps>) {
  const intl = useIntl();
  const { isAdmin = false, probes = noopDashboardProbePublisher } =
    useDashboardUi();
  const [internalTimeRange, setInternalTimeRange] =
    useState<DashboardTimeRange>(metrics?.timeRange ?? 7);
  const [internalView, setInternalView] = useState<CostDashboardView>(
    metrics?.view ?? (isAdmin ? "platform" : "mine"),
  );
  // WEB-COST-02: only administrators can open the platform view.
  const selectedView: CostDashboardView = isAdmin
    ? (view ?? internalView)
    : "mine";
  const selectedTimeRange = timeRange ?? internalTimeRange;
  const metricsQuery = useGetCostMetricsData({
    enabled: metrics === undefined,
    timeRange: selectedTimeRange,
    view: selectedView,
  });
  const dashboardMetrics = metrics ?? metricsQuery.data;
  const [currentTime, setCurrentTime] = useState(0);
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [resetCount, setResetCount] = useState(0);

  useEffect(() => {
    const update = () => {
      setCurrentTime(Date.now());
    };
    const timeoutId = window.setTimeout(update, 0);
    const intervalId = window.setInterval(update, 60000);
    return () => {
      window.clearTimeout(timeoutId);
      window.clearInterval(intervalId);
    };
  }, []);

  const updatedMinutesAgo = dashboardMetrics?.lastSuccessfulRefresh
    ? Math.max(
        0,
        Math.floor(
          (currentTime - dashboardMetrics.lastSuccessfulRefresh.getTime()) /
            60000,
        ),
      )
    : undefined;
  const money = moneyFormatter(intl, dashboardMetrics?.currency);
  const showTotalError =
    metrics === undefined && metricsQuery.isError && !metricsQuery.data;
  const showPartialSources =
    metrics === undefined && (dashboardMetrics?.failedSources?.length ?? 0) > 0;
  const showRefreshError =
    metrics === undefined &&
    metricsQuery.isError &&
    Boolean(metricsQuery.data) &&
    !metricsQuery.isFetching;

  const changeView = (next: CostDashboardView) => {
    if (onViewChange) onViewChange(next);
    else setInternalView(next);
  };

  return (
    <Fragment>
      <PageSection
        data-page-region="header"
        hasBodyWrapper={false}
        variant="default"
      >
        <Content>
          <Title headingLevel="h1">
            {title ?? intl.formatMessage(costMessages.costTitle)}
          </Title>
          <p>
            <FormattedMessage {...costMessages.costDescription} />
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
          active="cost"
          isRefreshing={metrics === undefined && metricsQuery.isFetching}
          onRefresh={() => {
            if (metrics === undefined) void metricsQuery.refetch();
          }}
          updatedMinutesAgo={updatedMinutesAgo}
        />
        <DashboardPurpose
          heading={<FormattedMessage {...costMessages.costPurposeHeading} />}
        >
          {intl.formatMessage(
            selectedView === "platform"
              ? costMessages.costPurposePlatform
              : costMessages.costPurposeMine,
          )}
        </DashboardPurpose>
        <div className="hypershell-cost-toolbar">
          <DashboardFilterToolbar
            onAddWidgets={() => {
              setDrawerOpen(!drawerOpen);
            }}
            onResetToDefault={() => {
              setDrawerOpen(false);
              persistLayout(
                selectedView,
                localizeCostDashboardLayoutTemplate(
                  defaultCostDashboardLayoutTemplates[selectedView],
                  intl,
                ),
                probes,
              );
              setResetCount((count) => count + 1);
            }}
            onTimeRangeChange={(next) => {
              if (onTimeRangeChange) onTimeRangeChange(next);
              else setInternalTimeRange(next);
            }}
            timeRange={selectedTimeRange}
          >
            {isAdmin ? (
              <ToggleGroup
                aria-label={intl.formatMessage(costMessages.costViewLabel)}
              >
                <ToggleGroupItem
                  buttonId="cost-view-platform"
                  isSelected={selectedView === "platform"}
                  onChange={() => {
                    changeView("platform");
                  }}
                  text={intl.formatMessage(costMessages.costViewPlatform)}
                />
                <ToggleGroupItem
                  buttonId="cost-view-mine"
                  isSelected={selectedView === "mine"}
                  onChange={() => {
                    changeView("mine");
                  }}
                  text={intl.formatMessage(costMessages.costViewMine)}
                />
              </ToggleGroup>
            ) : null}
          </DashboardFilterToolbar>
        </div>
        <Alert
          isInline
          title={intl.formatMessage(costMessages.costEstimatesAlertTitle)}
          variant="info"
        >
          <FormattedMessage {...costMessages.costEstimatesAlertBody} />
          {SPACE}
          <Button component="a" href="#cost-estimates" isInline variant="link">
            {intl.formatMessage(costMessages.costEstimatesLink)}
          </Button>
        </Alert>
        {metrics === undefined && metricsQuery.isPending ? (
          <Bullseye>
            <Spinner
              aria-label={intl.formatMessage(costMessages.costLoading)}
            />
          </Bullseye>
        ) : null}
        {showTotalError ? (
          <Alert
            title={intl.formatMessage(costMessages.costLoadErrorTitle)}
            variant="danger"
          >
            <FormattedMessage {...costMessages.costLoadErrorBody} />
          </Alert>
        ) : null}
        {dashboardMetrics?.coverageStart !== undefined ? (
          <Alert
            isInline
            title={intl.formatMessage(costMessages.costPartialDataTitle)}
            variant="info"
          >
            <FormattedMessage
              {...costMessages.costPartialDataBody}
              values={{ start: dashboardMetrics.coverageStart }}
            />
          </Alert>
        ) : null}
        {(dashboardMetrics?.unpricedClusters?.length ?? 0) > 0 ? (
          <Alert
            isInline
            title={intl.formatMessage(costMessages.costUnpricedTitle)}
            variant="warning"
          >
            <FormattedMessage
              {...costMessages.costUnpricedBody}
              values={{
                clusters: dashboardMetrics?.unpricedClusters?.join(", "),
              }}
            />
          </Alert>
        ) : null}
        {showPartialSources ? (
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
            isInline
            title={intl.formatMessage(messages.partialLoadWarningTitle)}
            variant="warning"
          >
            <FormattedMessage {...messages.partialLoadWarningBody} />
          </Alert>
        ) : null}
        {showRefreshError ? (
          <Alert
            isInline
            title={intl.formatMessage(messages.refreshErrorTitle)}
            variant="warning"
          >
            <FormattedMessage {...messages.refreshErrorBody} />
          </Alert>
        ) : null}
        {dashboardMetrics === undefined ? null : dashboardMetrics.view ===
            "mine" &&
          dashboardMetrics.status !== "not-configured" &&
          dashboardMetrics.empty === true ? (
          <MyCostEmpty />
        ) : (
          <CostWidgetGrid
            key={`${dashboardMetrics.view}-${String(resetCount)}`}
            drawerOpen={drawerOpen}
            onDrawerOpenChange={setDrawerOpen}
            view={dashboardMetrics.view}
            widgets={(dashboardMetrics.view === "platform"
              ? platformWidgets
              : myCostWidgets)(dashboardMetrics, money, intl)}
          />
        )}
        <AboutEstimates metrics={dashboardMetrics} />
      </PageSection>
    </Fragment>
  );
}
